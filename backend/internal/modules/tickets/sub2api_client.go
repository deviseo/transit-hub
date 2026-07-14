package tickets

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Sub2APIUser 是从 Sub2API `/api/v1/auth/me` 响应中解析出的用户身份，字段名做了常见形式兼容
// （id/user_id/userId），因为不同 Sub2API 部署返回的字段命名可能不完全一致。
type Sub2APIUser struct {
	ID    string
	Email string
	Role  string
}

// sub2APIError 是调用 Sub2API 失败时的领域错误。Unauthorized 对应 401/403（token 无效或过期），
// 其余网络/非 2xx/解析失败统一归为普通请求错误。两者的 Error() 都不包含 token 明文。
type sub2APIError struct {
	unauthorized bool
	detail       string
}

func (e *sub2APIError) Error() string { return e.detail }

// Sub2APIClient 封装对 Sub2API 只读身份接口的调用。本模块只需要一个 GET /api/v1/auth/me 调用，
// 不需要 upstream.HTTPClient 那种完整的平台适配层，因此按文档要求新增一个模块私有的精简客户端，
// 不强耦合 upstream 模块。
type Sub2APIClient struct {
	client *http.Client
}

func NewSub2APIClient(client *http.Client) *Sub2APIClient {
	return &Sub2APIClient{client: client}
}

// FetchCurrentUser 用用户的 Sub2API token 向 srcHost 请求当前用户信息。
// srcHost 必须已经过 normalizeSrcHost 校验和规范化。请求/响应均不记录 token 或响应体明文到日志，
// 满足"不打印 Sub2API token"的安全边界。
func (c *Sub2APIClient) FetchCurrentUser(srcHost string, token string) (Sub2APIUser, error) {
	reqURL := strings.TrimRight(srcHost, "/") + "/api/v1/auth/me"
	req, err := http.NewRequest(http.MethodGet, reqURL, nil)
	if err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "build sub2api request failed"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.client.Do(req)
	if err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "sub2api network error"}
	}
	defer resp.Body.Close()

	// 限制响应体读取大小，避免异常大响应占用过多内存；auth/me 的响应体正常情况下很小。
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "read sub2api response failed"}
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Sub2APIUser{}, &sub2APIError{unauthorized: true, detail: fmt.Sprintf("sub2api auth failed status=%d", resp.StatusCode)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Sub2APIUser{}, &sub2APIError{detail: fmt.Sprintf("sub2api request failed status=%d", resp.StatusCode)}
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "invalid json response from sub2api"}
	}

	// Sub2API 的 auth/me 响应通常把用户信息包在 data 字段下；也兼容极少数直接在顶层返回用户字段的实现。
	fields := payload
	if nested, ok := payload["data"].(map[string]any); ok {
		fields = nested
	}

	user := Sub2APIUser{
		ID:    firstStringField(fields, "id", "user_id", "userId"),
		Email: firstStringField(fields, "email"),
		Role:  firstStringField(fields, "role"),
	}
	if user.ID == "" {
		return Sub2APIUser{}, &sub2APIError{detail: "sub2api response missing user id"}
	}
	return user, nil
}

// firstStringField 依次尝试多个候选字段名，兼容 id 字段既可能是字符串也可能是数字的情况。
func firstStringField(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := fields[key]
		if !ok || value == nil {
			continue
		}
		switch v := value.(type) {
		case string:
			trimmed := strings.TrimSpace(v)
			if trimmed != "" {
				return trimmed
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case json.Number:
			return v.String()
		}
	}
	return ""
}
