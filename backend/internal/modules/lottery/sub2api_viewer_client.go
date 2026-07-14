package lottery

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type sub2APIError struct {
	unauthorized bool
	detail       string
}

func (e *sub2APIError) Error() string { return e.detail }

type Sub2APIViewerClient struct{ client *http.Client }

func NewSub2APIViewerClient(client *http.Client) *Sub2APIViewerClient {
	return NewSub2APIViewerClientWithPrivateTargets(client, false)
}

// NewSub2APIViewerClientWithPrivateTargets 保留严格构造器作为默认路径，私网模式仅供本地抽奖联调。
func NewSub2APIViewerClientWithPrivateTargets(client *http.Client, allowPrivateTargets bool) *Sub2APIViewerClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if client.Transport == nil || client.Transport == http.DefaultTransport {
		clone := *client
		clone.Transport = newViewerTransport(net.DefaultResolver, allowPrivateTargets)
		client = &clone
	}
	return &Sub2APIViewerClient{client: client}
}

func (c *Sub2APIViewerClient) FetchCurrentUser(srcHost string, token string) (Sub2APIUser, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(srcHost, "/")+"/api/v1/auth/me", nil)
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
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
	if err := json.Unmarshal(body, &payload); err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "invalid json response from sub2api"}
	}
	fields := payload
	if nested, ok := payload["data"].(map[string]any); ok {
		fields = nested
	}
	user := Sub2APIUser{ID: firstStringField(fields, "id", "user_id", "userId"), Email: firstStringField(fields, "email"), Role: firstStringField(fields, "role"), Status: firstStringField(fields, "status")}
	if user.ID == "" {
		return Sub2APIUser{}, &sub2APIError{detail: "sub2api response missing user id"}
	}
	return user, nil
}

func firstStringField(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := fields[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case float64:
			return strconv.FormatInt(int64(value), 10)
		case json.Number:
			return value.String()
		}
	}
	return ""
}
