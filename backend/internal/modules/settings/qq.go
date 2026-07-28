package settings

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultQQAPIBaseURL = "https://api.sgroup.qq.com"
	defaultQQTokenURL   = "https://bots.qq.com/app/getAppAccessToken"
	qqTokenRefreshLead  = time.Minute
	qqResponseBodyLimit = 1 << 20
)

type qqTokenCacheEntry struct {
	token      string
	secretHash [sha256.Size]byte
	refreshAt  time.Time
}

type qqTokenCache struct {
	mu      sync.Mutex
	entries map[string]qqTokenCacheEntry
}

func newQQTokenCache() *qqTokenCache {
	return &qqTokenCache{entries: make(map[string]qqTokenCacheEntry)}
}

// sendQQ 向指定 QQ 用户 OpenID 发送主动单聊文本消息。QQ 官方接口需要先用
// AppID/AppSecret 换取短期 Access Token，不能复用普通 webhook 发送逻辑。
func (s *Service) sendQQ(ctx context.Context, appID string, clientSecret string, userOpenID string, message string) error {
	return s.sendQQMessage(ctx, appID, clientSecret, userOpenID, notificationMessage{Content: message, Format: NotificationTemplateFormatText})
}

// sendQQMessage 将 Markdown 模板直接交给 QQ，HTML 模板先转换成 QQ 支持的 Markdown
// 内容。纯文本继续使用 msg_type=0，避免改变已有通知与连通性测试的请求语义。
func (s *Service) sendQQMessage(ctx context.Context, appID string, clientSecret string, userOpenID string, message notificationMessage) error {
	if appID == "" || clientSecret == "" || userOpenID == "" {
		return ErrMissingQQConfig
	}

	token, err := s.qqAccessToken(ctx, appID, clientSecret)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"msg_type": 0,
		"content":  message.Content,
	}
	if normalizeNotificationTemplateFormat(message.Format) != NotificationTemplateFormatText {
		content := markdownForChannel(message)
		payload = map[string]any{
			"msg_type": 2,
			"content":  content,
			"markdown": map[string]string{"content": content},
		}
	}
	endpoint := s.qqBaseURL() + "/v2/users/" + url.PathEscape(userOpenID) + "/messages"
	status, responseBody, err := s.qqPostJSON(ctx, endpoint, payload, "QQBot "+token, appID)
	if err != nil {
		return err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices || qqResponseHasError(responseBody) {
		return qqSendError(status, responseBody)
	}
	return nil
}

// qqAccessToken 串行化同一 Service 内的换票流程，避免并发通知重复请求 QQ 凭证接口。
// 缓存使用 AppSecret 摘要校验，修改密钥后不会继续复用旧 Token。
func (s *Service) qqAccessToken(ctx context.Context, appID string, clientSecret string) (string, error) {
	if s.qqTokens == nil {
		s.qqTokens = newQQTokenCache()
	}
	cache := s.qqTokens
	secretHash := sha256.Sum256([]byte(clientSecret))

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if entry, ok := cache.entries[appID]; ok && entry.secretHash == secretHash && time.Now().Before(entry.refreshAt) {
		return entry.token, nil
	}

	token, expiresIn, err := s.fetchQQAccessToken(ctx, appID, clientSecret)
	if err != nil {
		return "", err
	}
	refreshAfter := expiresIn - qqTokenRefreshLead
	if refreshAfter <= 0 {
		refreshAfter = expiresIn / 2
	}
	cache.entries[appID] = qqTokenCacheEntry{
		token:      token,
		secretHash: secretHash,
		refreshAt:  time.Now().Add(refreshAfter),
	}
	return token, nil
}

func (s *Service) fetchQQAccessToken(ctx context.Context, appID string, clientSecret string) (string, time.Duration, error) {
	payload := map[string]string{"appId": appID, "clientSecret": clientSecret}
	status, responseBody, err := s.qqPostJSON(ctx, s.qqTokenEndpoint(), payload, "", "")
	if err != nil {
		return "", 0, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		return "", 0, qqSendError(status, responseBody)
	}

	var response struct {
		AccessToken string          `json:"access_token"`
		ExpiresIn   json.RawMessage `json:"expires_in"`
	}
	if err := json.Unmarshal(responseBody, &response); err != nil {
		return "", 0, fmt.Errorf("%w: invalid QQ token response", ErrSendNotificationFailed)
	}
	seconds, err := parseQQExpiresIn(response.ExpiresIn)
	if err != nil || response.AccessToken == "" {
		return "", 0, fmt.Errorf("%w: invalid QQ access token", ErrSendNotificationFailed)
	}
	return response.AccessToken, time.Duration(seconds) * time.Second, nil
}

func parseQQExpiresIn(raw json.RawMessage) (int64, error) {
	value := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds <= 0 {
		return 0, fmt.Errorf("invalid expires_in")
	}
	return seconds, nil
}

func (s *Service) qqPostJSON(ctx context.Context, endpoint string, payload any, authorization string, appID string) (int, []byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("Accept", "application/json")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if appID != "" {
		request.Header.Set("X-Union-Appid", appID)
	}

	response, err := s.client.Do(request)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrSendNotificationFailed, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, qqResponseBodyLimit))
	if err != nil {
		return 0, nil, fmt.Errorf("%w: %v", ErrSendNotificationFailed, err)
	}
	return response.StatusCode, responseBody, nil
}

func (s *Service) qqBaseURL() string {
	if baseURL := strings.TrimRight(strings.TrimSpace(s.qqAPIBaseURL), "/"); baseURL != "" {
		return baseURL
	}
	return defaultQQAPIBaseURL
}

func (s *Service) qqTokenEndpoint() string {
	if endpoint := strings.TrimSpace(s.qqTokenURL); endpoint != "" {
		return endpoint
	}
	return defaultQQTokenURL
}

func qqResponseHasError(responseBody []byte) bool {
	var response struct {
		Code      json.RawMessage `json:"code"`
		ErrorCode json.RawMessage `json:"err_code"`
	}
	if len(responseBody) == 0 || json.Unmarshal(responseBody, &response) != nil {
		return false
	}
	for _, rawCode := range []json.RawMessage{response.ErrorCode, response.Code} {
		code := strings.Trim(strings.TrimSpace(string(rawCode)), `"`)
		if code != "" && code != "0" && code != "null" {
			return true
		}
	}
	return false
}

func qqSendError(status int, responseBody []byte) error {
	var response struct {
		Code      json.RawMessage `json:"code"`
		ErrorCode json.RawMessage `json:"err_code"`
		Message   string          `json:"message"`
	}
	_ = json.Unmarshal(responseBody, &response)
	code := strings.Trim(strings.TrimSpace(string(response.ErrorCode)), `"`)
	if code == "" || code == "null" {
		code = strings.Trim(strings.TrimSpace(string(response.Code)), `"`)
	}
	return fmt.Errorf("%w: status=%d code=%s message=%s", ErrSendNotificationFailed, status, code, strings.TrimSpace(response.Message))
}
