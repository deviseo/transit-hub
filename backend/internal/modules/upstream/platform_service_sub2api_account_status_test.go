package upstream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestUpdateSub2APIAdminAccountStatus_PutBodyPreservesFieldsAndOnlyChangesStatus 验证
// GET+PUT-merge：PUT 请求体保留 GET 回来的 credentials/group_ids/priority/concurrency/
// rate_multiplier/load_factor 等字段，只替换 status。
func TestUpdateSub2APIAdminAccountStatus_PutBodyPreservesFieldsAndOnlyChangesStatus(t *testing.T) {
	var putBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/acc-1":
			writeJSON(w, map[string]any{
				"data": map[string]any{
					"id": "acc-1", "name": "my-account", "notes": "note", "type": "openai",
					"credentials": map[string]any{"api_key": "sk-secret"},
					"extra":       map[string]any{"foo": "bar"},
					"proxy_id":    "proxy-1", "concurrency": 5, "priority": 10,
					"rate_multiplier": 1.5, "load_factor": 2, "status": "active",
					"group_ids": []any{"g1", "g2"}, "expires_at": "2027-01-01T00:00:00Z",
					"auto_pause_on_expired": true,
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/acc-1":
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
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}

	if err := service.UpdateSub2APIAdminAccountStatus(session, "acc-1", "inactive"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if putBody == nil {
		t.Fatalf("PUT request was never made")
	}
	if putBody["status"] != "inactive" {
		t.Fatalf("expected status=inactive, got %+v", putBody["status"])
	}
	if putBody["name"] != "my-account" || putBody["notes"] != "note" || putBody["type"] != "openai" {
		t.Fatalf("expected name/notes/type preserved, got %+v", putBody)
	}
	creds, ok := putBody["credentials"].(map[string]any)
	if !ok || creds["api_key"] != "sk-secret" {
		t.Fatalf("expected credentials preserved verbatim, got %+v", putBody["credentials"])
	}
	if putBody["proxy_id"] != "proxy-1" {
		t.Fatalf("expected proxy_id preserved, got %+v", putBody["proxy_id"])
	}
	concurrency, _ := putBody["concurrency"].(float64)
	if concurrency != 5 {
		t.Fatalf("expected concurrency=5 preserved, got %+v", putBody["concurrency"])
	}
	priority, _ := putBody["priority"].(float64)
	if priority != 10 {
		t.Fatalf("expected priority=10 preserved (not mapped from state weight), got %+v", putBody["priority"])
	}
	rateMultiplier, _ := putBody["rate_multiplier"].(float64)
	if rateMultiplier != 1.5 {
		t.Fatalf("expected rate_multiplier=1.5 preserved, got %+v", putBody["rate_multiplier"])
	}
	loadFactor, _ := putBody["load_factor"].(float64)
	if loadFactor != 2 {
		t.Fatalf("expected load_factor=2 preserved, got %+v", putBody["load_factor"])
	}
	groupIDs, ok := putBody["group_ids"].([]any)
	if !ok || len(groupIDs) != 2 || groupIDs[0] != "g1" || groupIDs[1] != "g2" {
		t.Fatalf("expected group_ids=[g1,g2] preserved, got %+v", putBody["group_ids"])
	}
	if putBody["expires_at"] != "2027-01-01T00:00:00Z" || putBody["auto_pause_on_expired"] != true {
		t.Fatalf("expected expires_at/auto_pause_on_expired preserved, got %+v", putBody)
	}
}

// TestUpdateSub2APIAdminAccountStatus_NumericGroupIDsStayNumeric 是本次整改的核心回归测试：
// GET 响应的 group_ids 是数字数组（sub2api 线上真实形态，如 [50]）时，PUT body 里的
// group_ids 元素必须仍是数字，不能被转成字符串（["50"]）——线上已确认字符串化的 group_ids
// 会被 sub2api 拒绝返回 400，导致自动降级/恢复的远端动作实际从未生效。
func TestUpdateSub2APIAdminAccountStatus_NumericGroupIDsStayNumeric(t *testing.T) {
	var putBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, map[string]any{
				"data": map[string]any{"id": "1515", "status": "active", "group_ids": []any{float64(50)}},
			})
		case http.MethodPut:
			body, err := readJSONBody(r)
			if err != nil {
				t.Fatalf("failed to decode PUT body: %v", err)
			}
			putBody = body
			writeJSON(w, map[string]any{"success": true})
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}

	if err := service.UpdateSub2APIAdminAccountStatus(session, "1515", "inactive"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	groupIDs, ok := putBody["group_ids"].([]any)
	if !ok || len(groupIDs) != 1 {
		t.Fatalf("expected one group id, got %+v", putBody["group_ids"])
	}
	if _, ok := groupIDs[0].(float64); !ok {
		t.Fatalf("expected numeric group id to stay numeric, got %T %+v", groupIDs[0], groupIDs[0])
	}
	if groupIDs[0].(float64) != 50 {
		t.Fatalf("expected group id 50, got %v", groupIDs[0])
	}
}

// TestUpdateSub2APIAdminAccountStatus_NumericGroupsFieldFallbackStaysNumeric 验证只有
// groups[].id 展开数组、没有 group_ids 字段时，数字 id 同样必须保持数字类型。
func TestUpdateSub2APIAdminAccountStatus_NumericGroupsFieldFallbackStaysNumeric(t *testing.T) {
	var putBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, map[string]any{
				"data": map[string]any{
					"id": "1515", "status": "active",
					"groups": []any{map[string]any{"id": float64(50), "name": "vip"}},
				},
			})
		case http.MethodPut:
			body, err := readJSONBody(r)
			if err != nil {
				t.Fatalf("failed to decode PUT body: %v", err)
			}
			putBody = body
			writeJSON(w, map[string]any{"success": true})
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}

	if err := service.UpdateSub2APIAdminAccountStatus(session, "1515", "inactive"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	groupIDs, ok := putBody["group_ids"].([]any)
	if !ok || len(groupIDs) != 1 {
		t.Fatalf("expected one group id derived from groups[].id, got %+v", putBody["group_ids"])
	}
	if _, ok := groupIDs[0].(float64); !ok {
		t.Fatalf("expected numeric group id from groups[].id to stay numeric, got %T %+v", groupIDs[0], groupIDs[0])
	}
}

// TestUpdateSub2APIAdminAccountStatus_FallsBackToGroupsFieldForGroupIDs 验证 GET 响应只有
// groups[] 展开数组、没有 group_ids 字段时，能从 groups[].id 解析出分组 ID 列表。
func TestUpdateSub2APIAdminAccountStatus_FallsBackToGroupsFieldForGroupIDs(t *testing.T) {
	var putBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			writeJSON(w, map[string]any{
				"data": map[string]any{
					"id": "acc-1", "status": "active",
					"groups": []any{
						map[string]any{"id": "g1", "name": "vip"},
						map[string]any{"id": "g2", "name": "default"},
					},
				},
			})
		case r.Method == http.MethodPut:
			body, err := readJSONBody(r)
			if err != nil {
				t.Fatalf("failed to decode PUT body: %v", err)
			}
			putBody = body
			writeJSON(w, map[string]any{"success": true})
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}

	if err := service.UpdateSub2APIAdminAccountStatus(session, "acc-1", "active"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	groupIDs, ok := putBody["group_ids"].([]any)
	if !ok || len(groupIDs) != 2 || groupIDs[0] != "g1" || groupIDs[1] != "g2" {
		t.Fatalf("expected group_ids derived from groups[].id, got %+v", putBody["group_ids"])
	}
}

// TestUpdateSub2APIAdminAccountStatus_NoGroupInfoDoesNotSendEmptyGroupIDs 验证两种来源都
// 解析不到分组 ID 时，PUT 请求体完全不带 group_ids 字段，不会用空数组覆盖账号原有分组绑定。
func TestUpdateSub2APIAdminAccountStatus_NoGroupInfoDoesNotSendEmptyGroupIDs(t *testing.T) {
	var putBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet:
			writeJSON(w, map[string]any{"data": map[string]any{"id": "acc-1", "status": "active"}})
		case r.Method == http.MethodPut:
			body, err := readJSONBody(r)
			if err != nil {
				t.Fatalf("failed to decode PUT body: %v", err)
			}
			putBody = body
			writeJSON(w, map[string]any{"success": true})
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}

	if err := service.UpdateSub2APIAdminAccountStatus(session, "acc-1", "inactive"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := putBody["group_ids"]; ok {
		t.Fatalf("must not send group_ids when it cannot be resolved, got %+v", putBody)
	}
}

// TestUpdateSub2APIAdminAccountStatus_GetFailurePropagates 验证 GET 失败时直接透传错误，
// 不发起任何猜测性的 PUT 请求。
func TestUpdateSub2APIAdminAccountStatus_GetFailurePropagates(t *testing.T) {
	putCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			putCalled = true
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}

	if err := service.UpdateSub2APIAdminAccountStatus(session, "acc-1", "inactive"); err == nil {
		t.Fatalf("expected error when GET fails")
	}
	if putCalled {
		t.Fatalf("must not issue a PUT request when GET fails")
	}
}

// TestUpdateSub2APIAdminAccountStatus_RejectsWrongPlatform 验证只允许 sub2api session 调用。
func TestUpdateSub2APIAdminAccountStatus_RejectsWrongPlatform(t *testing.T) {
	service := NewPlatformService(NewHTTPClient(http.DefaultClient))
	session := Session{Platform: PlatformNewAPI, BaseURL: "https://example.com", AccessToken: "token-1"}
	if err := service.UpdateSub2APIAdminAccountStatus(session, "acc-1", "inactive"); err == nil {
		t.Fatalf("expected error for non-sub2api session")
	}
}

// TestUpdateSub2APIAdminAccountStatus_NeverLogsOrLeaksCredentials 是一个轻量健全性检查：
// 确认响应/错误路径不会把 credentials 明文意外拼进错误字符串（真正的落地防线是本方法从不
// 解析 credentials 内容，只做不透明搬运；这里只做一次端到端 smoke test）。
func TestUpdateSub2APIAdminAccountStatus_NeverLogsOrLeaksCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, map[string]any{"data": map[string]any{"id": "acc-1", "status": "active", "credentials": map[string]any{"api_key": "sk-super-secret"}}})
		case http.MethodPut:
			writeJSON(w, map[string]any{"success": true})
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}

	err := service.UpdateSub2APIAdminAccountStatus(session, "acc-1", "inactive")
	if err != nil && strings.Contains(err.Error(), "sk-super-secret") {
		t.Fatalf("error must never contain plaintext credentials: %v", err)
	}
}
