package upstream

import (
	"net/http"
	"net/url"
	"strings"
)

// 独立探活凭据解析层：server-only 地临时获取某个 admin 账号(sub2api)/渠道(new-api)的
// 明文 base_url + key，供 connection_health 独立探活构造 ProbeRequest。
//
// 安全约束（贯穿全文件）：
//   - 明文 key/credentials 只在返回的 ProbeCredential 里短暂存在，调用方用完即弃。
//   - 绝不写日志、绝不写库、绝不返回前端、绝不放入 error 原文。
//   - 解析失败一律返回类型化的 *ProbeCredentialError（只含 reason 枚举，不含任何密钥或上游报文）。

// ProbeCredential 是独立探活所需的最小上游凭据集合。Key 为明文，调用方必须只在内存中短暂使用。
type ProbeCredential struct {
	BaseURL        string
	Key            string
	ProviderFamily string
	Models         []string
}

// 探活不可用原因枚举。这些 reason 会作为 i18n key 前缀透出给前端，绝不携带任何密钥/报文。
const (
	ReasonCredentialUnavailable      = "credential_unavailable"
	ReasonSecureVerificationRequired = "secure_verification_required"
	ReasonBaseURLUnavailable         = "base_url_unavailable"
	ReasonModelUnavailable           = "model_unavailable"
	ReasonExportUnavailable          = "export_unavailable"
	ReasonCredentialsRedacted        = "credentials_redacted"
)

// ProbeCredentialError 是凭据解析失败的类型化错误：只携带脱敏后的 reason 枚举，
// Error() 也只返回 reason，绝不包含上游返回的错误明文或密钥。
type ProbeCredentialError struct {
	Reason string
}

func (e *ProbeCredentialError) Error() string { return e.Reason }

func newProbeCredentialError(reason string) *ProbeCredentialError {
	return &ProbeCredentialError{Reason: reason}
}

// ProbeCredentialReason 从任意 error 中提取脱敏 reason；非 ProbeCredentialError 时回退到
// credential_unavailable，保证任何失败路径都不会把上游明文错误透传出去。
func ProbeCredentialReason(err error) string {
	if err == nil {
		return ""
	}
	if credErr, ok := err.(*ProbeCredentialError); ok {
		return credErr.Reason
	}
	return ReasonCredentialUnavailable
}

// sub2apiCredentialKeyFields 是 sub2api 导出凭据里可能承载「上游 API key」的字段名。
// 与 sub2api RedactCredentials 的敏感键集合对应：只要能取到其中之一的明文即可探活。
var sub2apiCredentialKeyFields = []string{
	"api_key", "apiKey", "access_token", "accessToken", "session_key", "sessionKey", "key", "token",
}

// sub2apiCredentialBaseURLFields 是 sub2api 导出凭据/账号里可能承载 base_url 的字段名。
var sub2apiCredentialBaseURLFields = []string{
	"base_url", "baseUrl", "endpoint", "api_base", "apiBase", "url",
}

// ResolveProbeCredential 平台中性地解析某个账号/渠道的探活凭据。
// 失败时返回 *ProbeCredentialError（reason 已脱敏）。
func (s *PlatformService) ResolveProbeCredential(session Session, account AdminGroupAccountInfo) (ProbeCredential, error) {
	switch session.Platform {
	case PlatformNewAPI:
		return s.resolveNewAPIChannelCredential(session, account)
	default:
		return s.resolveSub2APIAccountCredential(session, account)
	}
}

// resolveNewAPIChannelCredential 解析 new-api channel 的探活凭据：
//   - base_url 用 channel.base_url；为空则标记 base_url_unavailable（不猜测上游端点）。
//   - key 只能通过 POST /api/channel/:id/key 临时获取；根权限/安全验证不足时标记
//     secure_verification_required，其它失败标记 credential_unavailable。
func (s *PlatformService) resolveNewAPIChannelCredential(session Session, account AdminGroupAccountInfo) (ProbeCredential, error) {
	baseURL := strings.TrimSpace(account.BaseURL)
	if baseURL == "" {
		return ProbeCredential{}, newProbeCredentialError(ReasonBaseURLUnavailable)
	}
	if strings.TrimSpace(account.ID) == "" {
		return ProbeCredential{}, newProbeCredentialError(ReasonCredentialUnavailable)
	}
	key, err := s.FetchNewAPIChannelKey(session, account.ID)
	if err != nil {
		return ProbeCredential{}, err // 已是脱敏后的 *ProbeCredentialError
	}
	return ProbeCredential{
		BaseURL: baseURL,
		Key:     key,
		Models:  splitModels(account.Models),
	}, nil
}

// FetchNewAPIChannelKey 通过 POST /api/channel/:id/key 临时获取 channel 明文 key。
// 该接口受 AdminAuth + RootAuth + SecureVerificationRequired 保护：
//   - 401/403：当前 session 不满足 root 或未完成安全验证 -> secure_verification_required。
//   - 其它非 2xx / 无 data.key：credential_unavailable。
//
// 返回值是明文 key，调用方必须只在内存中短暂使用，绝不落库/日志/前端。
func (s *PlatformService) FetchNewAPIChannelKey(session Session, channelID string) (string, error) {
	if session.Platform != PlatformNewAPI || strings.TrimSpace(session.Cookie) == "" {
		return "", newProbeCredentialError(ReasonSecureVerificationRequired)
	}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/channel/"+url.PathEscape(channelID)+"/key", requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
		Method: http.MethodPost,
	})
	if err != nil {
		// 把上游错误映射为脱敏 reason，绝不透传上游报文/密钥。
		// key 接口受 RootAuth + SecureVerificationRequired 保护，401 与 403 都表示
		// 当前 session 不满足 root 或未完成安全验证，统一归类为 secure_verification_required。
		if reqErr, ok := err.(*RequestError); ok {
			if reqErr.MessageKey == ErrorAuth || reqErr.StatusCode == http.StatusForbidden {
				return "", newProbeCredentialError(ReasonSecureVerificationRequired)
			}
		}
		return "", newProbeCredentialError(ReasonCredentialUnavailable)
	}
	data := dataRecord(response.Payload)
	if key := firstString(data, []string{"key"}); key != nil && strings.TrimSpace(*key) != "" {
		return strings.TrimSpace(*key), nil
	}
	return "", newProbeCredentialError(ReasonCredentialUnavailable)
}

// resolveSub2APIAccountCredential 解析 sub2api 账号的探活凭据。
// 常规 list/detail 已脱敏，不能用于取明文；只能通过管理员备份导出接口
// GET /api/v1/admin/accounts/data?ids=<id>&include_proxies=false 取单账号明文 credentials。
//
// 能力检测（任一失败即标记不可探活，不编造健康状态）：
//   - 导出接口不可达（404/路由不存在等）-> export_unavailable。
//   - 返回账号数量不是正好 1 -> credential_unavailable（不按 name 猜测映射）。
//   - credentials 里没有任何明文 key（仍是脱敏形态）-> credentials_redacted。
//   - 缺少可用 base_url -> base_url_unavailable。
func (s *PlatformService) resolveSub2APIAccountCredential(session Session, account AdminGroupAccountInfo) (ProbeCredential, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return ProbeCredential{}, newProbeCredentialError(ReasonCredentialUnavailable)
	}
	accountID := strings.TrimSpace(account.ID)
	if accountID == "" {
		return ProbeCredential{}, newProbeCredentialError(ReasonCredentialUnavailable)
	}

	exportURL := session.BaseURL + "/api/v1/admin/accounts/data?ids=" + url.QueryEscape(accountID) + "&include_proxies=false"
	response, err := s.httpClient.requestJSON(exportURL, requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType})
	if err != nil {
		// 导出接口不可用（旧版本路由被 /:id 抢占、404、权限不足等）：统一标记导出不可用。
		return ProbeCredential{}, newProbeCredentialError(ReasonExportUnavailable)
	}

	items := sub2APIExportAccounts(response.Payload)
	// 导出接口按 ids 精确选择账号，必须正好返回一个账号，否则无法确认映射到当前账号。
	if len(items) != 1 {
		return ProbeCredential{}, newProbeCredentialError(ReasonCredentialUnavailable)
	}
	record, ok := items[0].(map[string]any)
	if !ok {
		return ProbeCredential{}, newProbeCredentialError(ReasonCredentialUnavailable)
	}

	credentials, _ := record["credentials"].(map[string]any)
	key := firstPlaintextKey(credentials)
	if key == "" {
		// credentials 缺失或仍是脱敏形态（没有任何明文 key 字段）。
		return ProbeCredential{}, newProbeCredentialError(ReasonCredentialsRedacted)
	}

	baseURL := firstBaseURL(credentials)
	if baseURL == "" {
		// 再尝试从账号顶层字段找 base_url。
		if b := firstString(record, sub2apiCredentialBaseURLFields); b != nil {
			baseURL = strings.TrimSpace(*b)
		}
	}
	if baseURL == "" {
		return ProbeCredential{}, newProbeCredentialError(ReasonBaseURLUnavailable)
	}

	return ProbeCredential{
		BaseURL: baseURL,
		Key:     key,
		Models:  splitModels(account.Models),
	}, nil
}

// sub2APIExportAccounts 从 sub2api 导出接口响应里提取账号数组，兼容两种真实存在的响应结构：
//   - 旧假设：data 本身就是数组 { "data": [ ... ] }。
//   - 真实站点：data 是对象，账号数组在 data.accounts { "data": { "accounts": [ ... ], "proxies": [...] } }。
//
// 两种结构都取不到时返回空数组，交由调用方按“数量不是 1”统一走 credential_unavailable。
func sub2APIExportAccounts(payload any) []any {
	root, ok := payload.(map[string]any)
	if !ok {
		return []any{}
	}
	switch data := root["data"].(type) {
	case []any:
		return data
	case map[string]any:
		if accounts, ok := data["accounts"].([]any); ok {
			return accounts
		}
	}
	return []any{}
}

// firstPlaintextKey 从 credentials map 里取第一个非空明文 key 字段。
func firstPlaintextKey(credentials map[string]any) string {
	if credentials == nil {
		return ""
	}
	if v := firstString(credentials, sub2apiCredentialKeyFields); v != nil {
		return strings.TrimSpace(*v)
	}
	return ""
}

// firstBaseURL 从 credentials map 里取第一个非空 base_url 字段。
func firstBaseURL(credentials map[string]any) string {
	if credentials == nil {
		return ""
	}
	if v := firstString(credentials, sub2apiCredentialBaseURLFields); v != nil {
		return strings.TrimSpace(*v)
	}
	return ""
}

// splitModels 把逗号分隔的 models 字符串拆成去空的模型名列表。
func splitModels(models string) []string {
	if strings.TrimSpace(models) == "" {
		return nil
	}
	parts := strings.Split(models, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
