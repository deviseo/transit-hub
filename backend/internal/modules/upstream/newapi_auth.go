package upstream

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// 新版 new-api（2026-11 起的 main 分支）重写了面板鉴权：
//   - 登录返回 15 分钟有效的 JWT access token，通过 Authorization: Bearer 发送；
//   - refresh token 只通过 HttpOnly Cookie（new_api_refresh，Path=/api/user/auth）下发，
//     POST /api/user/auth/refresh 每次都会轮换它；登录会话自创建起最长 30 天；
//   - New-Api-User 请求头不再参与鉴权，旧版 32 位系统访问令牌升级后 30 天失效，
//     长期自动化应使用个人访问令牌（PAT，nap_ 开头）。
//
// 旧版 new-api 仍是 Gin Session Cookie + New-Api-User，两种格式都需要兼容。
const (
	// newAPIRefreshCookieName 对应 new-api service/auth_session.go 的 RefreshCookieName。
	newAPIRefreshCookieName = "new_api_refresh"
	// newAPIRefreshSkewMS：新版 access token 只有 15 分钟，提前 3 分钟换新，覆盖请求耗时与两端时钟偏差。
	newAPIRefreshSkewMS int64 = 180_000
	// newAPIFallbackAccessTTLMS：响应缺少 access_expires_at 时按 14 分钟估算过期时间，
	// 避免把过期时间当作未知而在每次调用时都触发轮换。
	newAPIFallbackAccessTTLMS int64 = 14 * 60 * 1000
)

// newAPIBearerSession 解析新版 new-api 登录 / 刷新响应。返回 ok=false 表示响应不是新版鉴权格式
// （没有 data.access_token，即旧版 Cookie 会话）。userData 是 data.user（新版）以便复用分组等字段。
func newAPIBearerSession(baseURL string, response jsonResponse) (Session, map[string]any, bool) {
	data := dataRecord(response.Payload)
	accessToken := firstString(data, []string{"access_token"})
	if accessToken == nil || strings.TrimSpace(*accessToken) == "" {
		return Session{}, nil, false
	}
	userData := data
	if nested, ok := data["user"].(map[string]any); ok {
		userData = nested
	}
	tokenType := "Bearer"
	if value := firstString(data, []string{"token_type"}); value != nil && strings.TrimSpace(*value) != "" {
		tokenType = strings.TrimSpace(*value)
	}
	session := Session{
		Platform:     PlatformNewAPI,
		BaseURL:      baseURL,
		UserID:       newAPIUserID(userData),
		AccessToken:  strings.TrimSpace(*accessToken),
		TokenType:    tokenType,
		RefreshToken: newAPIRefreshCookie(response.Header),
	}
	expiresAt := time.Now().UnixMilli() + newAPIFallbackAccessTTLMS
	if value := firstNumber(data, []string{"access_expires_at"}); value != nil && *value > 0 {
		expiresAt = int64(*value)
		if expiresAt < 1_000_000_000_000 {
			// new-api 返回的是 Unix 秒；兼容未来改为毫秒的情况。
			expiresAt *= 1000
		}
	}
	session.ExpiresAt = &expiresAt
	return session, userData, true
}

// newAPIRefreshCookie 从 Set-Cookie 中取出 new_api_refresh 的值；被清除（Max-Age<=0）时返回空。
func newAPIRefreshCookie(header http.Header) string {
	for _, cookie := range (&http.Response{Header: header}).Cookies() {
		if cookie.Name == newAPIRefreshCookieName && cookie.Value != "" && cookie.MaxAge >= 0 {
			return cookie.Value
		}
	}
	return ""
}

// newAPILoginRequiresVerification 判断登录是否停在二次验证步骤。新版 new-api 返回
// {success: true, data: {require_verification: true, flow_token, methods}}，不含任何会话凭据；
// 旧版开启 2FA 时返回 {require_2fa: true}。
func newAPILoginRequiresVerification(loginData map[string]any) bool {
	for _, key := range []string{"require_verification", "require_2fa"} {
		if required, ok := loginData[key].(bool); ok && required {
			return true
		}
	}
	return false
}

// refreshNewAPISession 用 refresh cookie 调用 POST /api/user/auth/refresh 换取新的 access token。
// new-api 每次刷新都会轮换 refresh token，调用方必须持久化返回的新会话（RefreshSession 已负责去重）。
// Secure 模式下 new-api 会校验 Origin，这里使用站点自身的 Origin，与浏览器在面板页发起刷新一致。
func (s *PlatformService) refreshNewAPISession(session Session) (Session, error) {
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/user/auth/refresh", requestOptions{
		Method: http.MethodPost,
		Cookie: newAPIRefreshCookieName + "=" + session.RefreshToken,
		Origin: siteOrigin(session.BaseURL),
	})
	if err != nil {
		return Session{}, err
	}
	next, _, ok := newAPIBearerSession(session.BaseURL, response)
	if !ok {
		return Session{}, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	if next.RefreshToken == "" {
		// 正常情况下每次刷新都会下发轮换后的 Cookie；缺失时只能沿用旧值，避免直接丢失会话。
		next.RefreshToken = session.RefreshToken
	}
	if next.UserID == "" {
		next.UserID = session.UserID
	}
	next.QuotaPerUnit = session.QuotaPerUnit
	return next, nil
}

// discoverNewAPIUserID 在未填写用户 ID 时，用访问令牌调用 /api/user/self 读取所属用户 ID。
// 新版 new-api 不需要 New-Api-User，可以直接识别；旧版系统访问令牌缺少 New-Api-User 会被拒绝，
// 此时返回 ErrorNewAPITokenRejected，由前端提示「旧版令牌需要填写用户 ID」。
func (s *PlatformService) discoverNewAPIUserID(session Session) (string, error) {
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/user/self", newAPIAuthOptions(session))
	if err != nil {
		var requestErr *RequestError
		if errors.As(err, &requestErr) && (requestErr.MessageKey == ErrorAuth || requestErr.MessageKey == ErrorRequest) {
			return "", newRequestError(ErrorNewAPITokenRejected, PlatformNewAPI)
		}
		return "", err
	}
	userID := newAPIUserID(dataRecord(response.Payload))
	if userID == "" {
		return "", newRequestError(ErrorNewAPITokenRejected, PlatformNewAPI)
	}
	return userID, nil
}

// siteOrigin 返回站点地址的 Origin（scheme://host[:port]）。
func siteOrigin(baseURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
