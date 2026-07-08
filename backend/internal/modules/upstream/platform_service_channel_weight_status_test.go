package upstream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestUpdateNewAPIChannelWeightStatus_PutBodyIsTopLevel 验证 PUT /api/channel/ 的请求体
// 字段在 JSON 顶层（new-api UpdateChannel 用 ShouldBindJSON(&PatchChannel) 直接绑定），
// 不能像 CreateNewAPIChannel 那样包一层 "channel"，否则后端解析不到任何字段。
func TestUpdateNewAPIChannelWeightStatus_PutBodyIsTopLevel(t *testing.T) {
	var putBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/channel/42":
			writeJSON(w, map[string]any{
				"data": map[string]any{
					"id": 42, "type": 1, "key": "sk-secret", "name": "my-channel",
					"base_url": "https://example.com", "models": "gpt-4o", "group": "default",
					"weight": 100, "status": 1, "priority": 0,
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/channel/":
			body, err := readJSONBody(r)
			if err != nil {
				t.Fatalf("failed to decode PUT body: %v", err)
			}
			putBody = body
			writeJSON(w, map[string]any{"success": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "1"}

	if err := service.UpdateNewAPIChannelWeightStatus(session, "42", 0, 2); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if putBody == nil {
		t.Fatalf("PUT request was never made")
	}
	// 顶层必须直接包含 id/weight/status，不能被包在 "channel" 键下面。
	if _, wrapped := putBody["channel"]; wrapped {
		t.Fatalf("request body must not be wrapped in a top-level \"channel\" key: %+v", putBody)
	}
	if _, ok := putBody["id"]; !ok {
		t.Fatalf("expected top-level \"id\" field, got %+v", putBody)
	}
	weight, ok := putBody["weight"].(float64)
	if !ok || weight != 0 {
		t.Fatalf("expected top-level weight=0, got %+v", putBody["weight"])
	}
	status, ok := putBody["status"].(float64)
	if !ok || status != 2 {
		t.Fatalf("expected top-level status=2, got %+v", putBody["status"])
	}
	// 必须保留 GET 回来的 key/base_url/group 等字段，不能被覆盖丢失。
	if putBody["key"] != "sk-secret" || putBody["base_url"] != "https://example.com" || putBody["group"] != "default" {
		t.Fatalf("expected original channel fields (key/base_url/group) to be preserved, got %+v", putBody)
	}
}

// TestUpdateNewAPIChannelWeightStatus_GetFailurePropagates 验证 GET 单条 channel 失败时
// 直接把错误透传给调用方，不发起任何猜测性的 PUT 请求（调用方应记录 remote_action=unsupported）。
func TestUpdateNewAPIChannelWeightStatus_GetFailurePropagates(t *testing.T) {
	putCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putCalled = true
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "1"}

	if err := service.UpdateNewAPIChannelWeightStatus(session, "42", 0, 2); err == nil {
		t.Fatalf("expected error when GET fails")
	}
	if putCalled {
		t.Fatalf("must not issue a PUT request when GET fails")
	}
}

func readJSONBody(r *http.Request) (map[string]any, error) {
	var body map[string]any
	err := json.NewDecoder(r.Body).Decode(&body)
	return body, err
}
