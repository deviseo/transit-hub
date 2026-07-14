package lottery

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

type RewardResult struct {
	Status          string
	ErrorKey        string
	Detail          string
	RemoteRef       string
	SkipRateCleanup bool
}

type RewardClient struct {
	client              *http.Client
	allowPrivateTargets bool
}

func NewRewardClient(client *http.Client) *RewardClient {
	return NewRewardClientWithPrivateTargets(client, false)
}

// NewRewardClientWithPrivateTargets 与嵌入来源校验使用同一调试开关，避免页面可加载但发奖仍被私网策略拦截。
func NewRewardClientWithPrivateTargets(client *http.Client, allowPrivateTargets bool) *RewardClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if client.Transport == nil || client.Transport == http.DefaultTransport {
		clone := *client
		clone.Transport = newViewerTransport(net.DefaultResolver, allowPrivateTargets)
		client = &clone
	}
	return &RewardClient{client: client, allowPrivateTargets: allowPrivateTargets}
}

func (c *RewardClient) Redeem(ctx context.Context, session upstream.Session, job RewardJob) RewardResult {
	if session.Platform != upstream.PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorRewardAdminSession, Detail: "missing sub2api admin session"}
	}
	baseURL, err := normalizeRewardBaseURLWithPrivateTargets(session.BaseURL, c.allowPrivateTargets)
	if err != nil {
		return RewardResult{Status: RewardManualAttention, ErrorKey: ErrorInvalidSourceOrigin, Detail: err.Error()}
	}
	targetUserID, err := strconv.ParseInt(strings.TrimSpace(job.Winner.Sub2apiUserID), 10, 64)
	if err != nil || targetUserID <= 0 {
		return RewardResult{Status: RewardFailed, ErrorKey: ErrorValidation, Detail: "invalid numeric sub2api user id"}
	}
	// 新版订阅奖品把 multiplier 解释为管理员填写的用户专属倍率。历史奖品可能没有
	// multiplier；这些存量任务继续走原有订阅兑换码路径，避免升级后破坏已发布活动。
	if job.Prize.Type == PrizeTypeSubscription && strings.TrimSpace(job.Prize.Multiplier) != "" {
		return c.applyDedicatedRate(ctx, session, baseURL, targetUserID, job)
	}
	rewardCode := sub2APIRewardCode(job.IdempotencyKey)
	body := map[string]any{
		"code":    rewardCode,
		"type":    job.Prize.Type,
		"user_id": targetUserID,
		"notes":   fmt.Sprintf("TransitHub lottery campaign=%s winner=%s", job.CampaignID, job.WinnerID),
	}
	switch job.Prize.Type {
	case PrizeTypeBalance:
		amount, err := strconv.ParseFloat(strings.TrimSpace(job.Prize.BalanceAmount), 64)
		if err != nil || amount <= 0 {
			return RewardResult{Status: RewardFailed, ErrorKey: ErrorValidation, Detail: "invalid balance reward amount"}
		}
		body["value"] = amount
	case PrizeTypeSubscription:
		groupID, err := strconv.ParseInt(strings.TrimSpace(job.Prize.GroupID), 10, 64)
		if err != nil || groupID <= 0 {
			return RewardResult{Status: RewardFailed, ErrorKey: ErrorValidation, Detail: "invalid numeric sub2api group id"}
		}
		body["value"] = 1
		body["group_id"] = groupID
		if job.Prize.ValidityDays != nil {
			body["validity_days"] = *job.Prize.ValidityDays
		}
	default:
		return RewardResult{Status: RewardManualAttention, ErrorKey: ErrorRewardUnsupported, Detail: "unsupported prize type"}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return RewardResult{Status: RewardFailed, ErrorKey: ErrorUnknown, Detail: err.Error()}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v1/admin/redeem-codes/create-and-redeem", bytes.NewReader(payload))
	if err != nil {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorRewardUnsupported, Detail: err.Error()}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", session.TokenType+" "+session.AccessToken)
	req.Header.Set("Idempotency-Key", rewardCode)
	resp, err := c.client.Do(req)
	if err != nil {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorEmbedUpstreamRequest, Detail: err.Error()}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorRewardAdminSession, Detail: string(raw)}
	}
	if resp.StatusCode == http.StatusConflict && job.Prize.Type == PrizeTypeSubscription {
		return RewardResult{Status: RewardManualAttention, ErrorKey: ErrorRewardUnsupported, Detail: "subscription conflict"}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorEmbedUpstreamRequest, Detail: fmt.Sprintf("status=%d body=%s", resp.StatusCode, string(raw))}
	}
	return RewardResult{Status: RewardFulfilled, RemoteRef: remoteReference(raw)}
}

// applyDedicatedRate 将奖品倍率写成中奖用户在目标分组上的专属倍率。Sub2API 的
// group_rates 是按分组 ID 局部同步的 map，因此只提交一个键不会触碰该用户的其他分组。
func (c *RewardClient) applyDedicatedRate(ctx context.Context, session upstream.Session, baseURL string, targetUserID int64, job RewardJob) RewardResult {
	groupID, err := positiveNumericID(job.Prize.GroupID)
	if err != nil {
		return RewardResult{Status: RewardFailed, ErrorKey: ErrorValidation, Detail: "invalid numeric sub2api group id"}
	}
	multiplier, err := positiveFiniteMultiplier(job.Prize.Multiplier)
	if err != nil {
		return RewardResult{Status: RewardFailed, ErrorKey: ErrorValidation, Detail: "invalid dedicated reward multiplier"}
	}
	status, raw, err := c.updateUserGroupRate(ctx, session, baseURL, targetUserID, groupID, &multiplier)
	if err != nil {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorEmbedUpstreamRequest, Detail: err.Error()}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorRewardAdminSession, Detail: string(raw)}
	}
	// 中奖后、任务执行前用户可能已经被删除。此时已经没有可授予的主体，按幂等完成
	// 处理，避免一个不可恢复的 404 让活动永久停留在部分失败状态。
	if status == http.StatusNotFound {
		return RewardResult{Status: RewardFulfilled, RemoteRef: fmt.Sprintf("group-rate-user-missing:%d:%d", targetUserID, groupID), SkipRateCleanup: true}
	}
	if status >= 400 && status < 500 {
		return RewardResult{Status: RewardManualAttention, ErrorKey: ErrorRewardUnsupported, Detail: fmt.Sprintf("status=%d body=%s", status, string(raw))}
	}
	if status < 200 || status >= 300 {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorEmbedUpstreamRequest, Detail: fmt.Sprintf("status=%d body=%s", status, string(raw))}
	}
	return RewardResult{Status: RewardFulfilled, RemoteRef: fmt.Sprintf("group-rate:%d:%d", targetUserID, groupID)}
}

// CleanupDedicatedRate 清理已到期的抽奖专属倍率。replacement 不为空表示同一用户和
// 分组仍有另一份未到期奖励，此时恢复那份奖励倍率；为空则提交 null 删除专属倍率。
// Sub2API 对不存在的倍率执行 null 更新本身就是幂等操作，用户已删除的 404 也视为完成。
func (c *RewardClient) CleanupDedicatedRate(ctx context.Context, session upstream.Session, job RateCleanupJob, replacement *RateCleanupReplacement) RewardResult {
	if session.Platform != upstream.PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorRewardAdminSession, Detail: "missing sub2api admin session"}
	}
	baseURL, err := normalizeRewardBaseURLWithPrivateTargets(session.BaseURL, c.allowPrivateTargets)
	if err != nil {
		return RewardResult{Status: RewardFailed, ErrorKey: ErrorInvalidSourceOrigin, Detail: err.Error()}
	}
	targetUserID, err := positiveNumericID(job.Winner.Sub2apiUserID)
	if err != nil {
		return RewardResult{Status: RewardFailed, ErrorKey: ErrorValidation, Detail: "invalid numeric sub2api user id"}
	}
	groupID, err := positiveNumericID(job.Prize.GroupID)
	if err != nil {
		return RewardResult{Status: RewardFailed, ErrorKey: ErrorValidation, Detail: "invalid numeric sub2api group id"}
	}
	var rate *float64
	if replacement != nil {
		value, parseErr := positiveFiniteMultiplier(replacement.Multiplier)
		if parseErr != nil {
			return RewardResult{Status: RewardFailed, ErrorKey: ErrorValidation, Detail: "invalid replacement dedicated multiplier"}
		}
		rate = &value
	}
	status, raw, err := c.updateUserGroupRate(ctx, session, baseURL, targetUserID, groupID, rate)
	if err != nil {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorEmbedUpstreamRequest, Detail: err.Error()}
	}
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorRewardAdminSession, Detail: string(raw)}
	}
	if status == http.StatusNotFound {
		return RewardResult{Status: RewardFulfilled}
	}
	if status >= 400 && status < 500 {
		// 无效用户、已删除分组或已不存在的专属倍率都没有继续重试的价值。清理任务记录
		// 详情后结束，不将错误传播到已经完成的抽奖活动。
		return RewardResult{Status: RewardFailed, ErrorKey: ErrorRewardUnsupported, Detail: fmt.Sprintf("status=%d body=%s", status, string(raw))}
	}
	if status < 200 || status >= 300 {
		return RewardResult{Status: RewardRetryableFailed, ErrorKey: ErrorEmbedUpstreamRequest, Detail: fmt.Sprintf("status=%d body=%s", status, string(raw))}
	}
	return RewardResult{Status: RewardFulfilled}
}

// updateUserGroupRate 使用 Sub2API 管理员更新用户接口，只发送 group_rates 字段。
// rate=nil 编码成 JSON null，对应清除这个用户在该分组上的专属倍率。
func (c *RewardClient) updateUserGroupRate(ctx context.Context, session upstream.Session, baseURL string, targetUserID, groupID int64, rate *float64) (int, []byte, error) {
	body := map[string]any{
		"group_rates": map[string]any{strconv.FormatInt(groupID, 10): rate},
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return 0, nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s/api/v1/admin/users/%d", baseURL, targetUserID), bytes.NewReader(payload))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", session.TokenType+" "+session.AccessToken)
	resp, err := c.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, raw, nil
}

func positiveNumericID(value string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid positive numeric id")
	}
	return id, nil
}

func positiveFiniteMultiplier(value string) (float64, error) {
	multiplier, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
		return 0, fmt.Errorf("invalid positive finite multiplier")
	}
	return multiplier, nil
}

// sub2APIRewardCode 将内部任务幂等键压缩为稳定短码，以兼容 Sub2API 兑换码字段的 32 字符限制。
// 请求体兑换码与幂等头共用此值，既能安全重试，也不会复用此前被上游拒绝的超长请求记录。
func sub2APIRewardCode(idempotencyKey string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(idempotencyKey)))
	return fmt.Sprintf("th-lottery-%x", sum[:10])
}

func normalizeRewardBaseURL(value string) (string, error) {
	return normalizeRewardBaseURLWithPrivateTargets(value, false)
}

func normalizeRewardBaseURLWithPrivateTargets(value string, allowPrivateTargets bool) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("missing sub2api reward base url")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || !isAllowedRewardHost(parsed.Hostname(), allowPrivateTargets) {
		return "", fmt.Errorf("invalid sub2api reward base url")
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isAllowedRewardHost(host string, allowPrivateTargets bool) bool {
	if allowPrivateTargets && strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		if allowPrivateTargets {
			return isAllowedDevelopmentIP(ip)
		}
		return isSafeDialIP(ip)
	}
	return hostDomainPattern.MatchString(host)
}

func remoteReference(raw []byte) string {
	var payload map[string]any
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	if data, ok := payload["data"].(map[string]any); ok {
		payload = data
	}
	for _, key := range []string{"id", "code", "redeem_code", "redeemCode"} {
		if value, ok := payload[key].(string); ok {
			return value
		}
	}
	return ""
}
