package tickets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	verifyTimeout       = 15 * time.Second
	maxVerifyBodyBytes  = 1 << 20
	maxCredentialLength = 8192
)

// SiteVerifier 去站点核实用户身份：sub2api 请求 /api/v1/auth/me，new-api 请求 /api/user/self。
// 站点地址来自服务端配置；用户的凭证只用这一次，不保存、不写日志。
type SiteVerifier struct {
	newClient func(timeout time.Duration) *http.Client
}

// NewSiteVerifier 用出站代理的 HTTP 客户端访问站点（newClient 为 nil 时直连）。
func NewSiteVerifier(newClient func(timeout time.Duration) *http.Client) *SiteVerifier {
	if newClient == nil {
		newClient = func(timeout time.Duration) *http.Client { return &http.Client{Timeout: timeout} }
	}
	return &SiteVerifier{newClient: newClient}
}

// Verify 核实凭证，返回用户在站点上的身份。凭证无效时返回 ErrorSiteLoginExpired，站点连不上时返回 ErrorSiteUnavailable。
func (v *SiteVerifier) Verify(ctx context.Context, platform string, baseURL string, credential Credential) (SiteUser, error) {
	request, err := buildVerifyRequest(ctx, platform, baseURL, credential)
	if err != nil {
		return SiteUser{}, err
	}
	response, err := v.newClient(verifyTimeout).Do(request)
	if err != nil {
		return SiteUser{}, requestError(ErrorSiteUnavailable)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxVerifyBodyBytes))
	if err != nil {
		return SiteUser{}, requestError(ErrorSiteUnavailable)
	}
	switch {
	case response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden:
		return SiteUser{}, requestError(ErrorSiteLoginExpired)
	case response.StatusCode != http.StatusOK:
		return SiteUser{}, requestError(ErrorSiteUnavailable)
	}
	user, err := parseCurrentUser(platform, body)
	if err != nil {
		return SiteUser{}, err
	}
	if credential.Kind == "cookie" && user.ID != strings.TrimSpace(credential.UserID) {
		return SiteUser{}, requestError(ErrorSiteLoginExpired)
	}
	return user, nil
}

func buildVerifyRequest(ctx context.Context, platform string, baseURL string, credential Credential) (*http.Request, error) {
	path := ""
	switch platform {
	case "sub2api":
		path = "/api/v1/auth/me"
	case "newapi":
		path = "/api/user/self"
	default:
		return nil, requestError(ErrorDisabled)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+path, nil)
	if err != nil {
		return nil, requestError(ErrorSiteUnavailable)
	}
	request.Header.Set("Accept", "application/json")
	token, cookie, userID := strings.TrimSpace(credential.Token), strings.TrimSpace(credential.Cookie), strings.TrimSpace(credential.UserID)
	switch {
	case credential.Kind == "bearer" && validHeaderValue(token):
		request.Header.Set("Authorization", "Bearer "+token)
	case credential.Kind == "cookie" && platform == "newapi" && validHeaderValue(cookie) && validUserID(userID):
		request.Header.Set("Cookie", cookie)
		request.Header.Set("New-Api-User", userID)
	default:
		return nil, requestError(ErrorInvalid)
	}
	return request, nil
}

func validHeaderValue(value string) bool {
	if value == "" || len(value) > maxCredentialLength {
		return false
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func validUserID(value string) bool {
	if value == "" || len(value) > 20 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// parseCurrentUser 读取「当前用户」接口的响应：sub2api 是 {code, data}，new-api 是 {success, data}。
func parseCurrentUser(platform string, body []byte) (SiteUser, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var envelope struct {
		Code    *json.Number   `json:"code"`
		Success *bool          `json:"success"`
		Data    map[string]any `json:"data"`
	}
	if err := decoder.Decode(&envelope); err != nil {
		return SiteUser{}, requestError(ErrorSiteUnavailable)
	}
	switch platform {
	case "sub2api":
		if envelope.Code != nil && envelope.Code.String() != "0" {
			return SiteUser{}, requestError(ErrorSiteLoginExpired)
		}
	case "newapi":
		if envelope.Success != nil && !*envelope.Success {
			return SiteUser{}, requestError(ErrorSiteLoginExpired)
		}
	}
	data := envelope.Data
	user := SiteUser{
		Platform: platform,
		ID:       firstText(data, "id", "user_id", "userId"),
		Email:    firstText(data, "email"),
		Username: firstText(data, "display_name", "username", "name"),
	}
	if user.ID == "" {
		return SiteUser{}, requestError(ErrorSiteLoginExpired)
	}
	if user.Username == "" {
		user.Username = user.Email
	}
	return user, nil
}

func firstText(data map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := data[key].(type) {
		case string:
			if text := strings.TrimSpace(value); text != "" {
				return text
			}
		case json.Number:
			return value.String()
		}
	}
	return ""
}

// isRequestError 判断是不是可以直接返回给客户端的错误。
func isRequestError(err error) (requestError, bool) {
	var key requestError
	if errors.As(err, &key) {
		return key, true
	}
	return "", false
}
