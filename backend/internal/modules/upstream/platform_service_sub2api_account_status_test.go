package upstream

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestUpdateSub2APIAdminAccountStatus_UsesFieldOnlyBulkUpdate 验证状态更新不会读取或
// 回写账号详情。请求体只能包含账号 ID 和目标状态，尤其不能携带倍率、凭据或分组字段。
func TestUpdateSub2APIAdminAccountStatus_UsesFieldOnlyBulkUpdate(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/accounts/bulk-update" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var err error
		body, err = readJSONBody(r)
		if err != nil {
			t.Fatalf("failed to decode bulk update body: %v", err)
		}
		writeJSON(w, map[string]any{"success": true})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}
	if err := service.UpdateSub2APIAdminAccountStatus(session, "1515", "inactive"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSub2APIBulkAccountIDs(t, body, 1515)
	if len(body) != 2 || body["status"] != "inactive" {
		t.Fatalf("status update must contain only account_ids and status: %+v", body)
	}
}

// TestUpdateAdminTargetPriority_Sub2APIUsesFieldOnlyBulkUpdate 是倍率事故的核心回归测试：
// priority 同步绝不能把 rate_multiplier 等详情字段带回上游。
func TestUpdateAdminTargetPriority_Sub2APIUsesFieldOnlyBulkUpdate(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/accounts/bulk-update" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var err error
		body, err = readJSONBody(r)
		if err != nil {
			t.Fatalf("failed to decode bulk update body: %v", err)
		}
		writeJSON(w, map[string]any{"success": true})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}
	if err := service.UpdateAdminTargetPriority(session, "1515", 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertSub2APIBulkAccountIDs(t, body, 1515)
	if len(body) != 2 || body["priority"] != float64(1) {
		t.Fatalf("priority update must contain only account_ids and priority: %+v", body)
	}
	for _, forbidden := range []string{"rate_multiplier", "credentials", "group_ids", "status", "concurrency"} {
		if _, exists := body[forbidden]; exists {
			t.Fatalf("priority update must never include %s: %+v", forbidden, body)
		}
	}
}

func TestSetSub2APIAdminAccountSchedulable_UsesDedicatedFieldEndpoint(t *testing.T) {
	var body map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/accounts/1515/schedulable" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		var err error
		body, err = readJSONBody(r)
		if err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		writeJSON(w, map[string]any{
			"success": true,
			"data":    map[string]any{"id": 1515, "schedulable": false},
		})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token", TokenType: "Bearer"}
	if err := service.SetSub2APIAdminAccountSchedulable(session, "1515", false); err != nil {
		t.Fatalf("SetSub2APIAdminAccountSchedulable() error = %v", err)
	}
	if len(body) != 1 || body["schedulable"] != false {
		t.Fatalf("schedulable update must contain only schedulable: %+v", body)
	}
}

func TestParseSub2APIAccountPreservesObservedUpdateTime(t *testing.T) {
	account := parseSub2APIAccount(map[string]any{
		"id": 1515, "schedulable": false, "updated_at": "2026-08-03T10:20:30Z",
	})
	want := time.Date(2026, 8, 3, 10, 20, 30, 0, time.UTC)
	if account.UpdatedAt == nil || !account.UpdatedAt.Equal(want) {
		t.Fatalf("updated time = %v, want %s", account.UpdatedAt, want)
	}
}

// TestSub2APIBulkAccountUpdate_UnsupportedDoesNotFallback 验证旧版接口不支持时直接失败，
// 不再尝试危险的 GET+PUT 整对象回写。
func TestSub2APIBulkAccountUpdate_UnsupportedDoesNotFallback(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/accounts/bulk-update" {
			t.Fatalf("unexpected fallback request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}
	err := service.UpdateSub2APIAdminAccountStatus(session, "1515", "inactive")
	if err == nil {
		t.Fatal("expected unsupported bulk update to return an error")
	}
	requestErr, ok := err.(*RequestError)
	if !ok || requestErr.MessageKey != ErrorSub2APIBulkUpdateUnsupported || requestErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected explicit unsupported capability error, got %T %+v", err, err)
	}
	if requestCount != 1 {
		t.Fatalf("unsupported endpoint must not trigger a fallback request, count=%d", requestCount)
	}
}

func TestSub2APIBulkAccountUpdate_ServerFailureIsNotMisclassifiedAsUnsupported(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/admin/accounts/bulk-update" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}
	err := service.UpdateAdminTargetPriority(session, "1515", 1)
	requestErr, ok := err.(*RequestError)
	if !ok || requestErr.MessageKey != ErrorRequest || requestErr.StatusCode != http.StatusInternalServerError {
		t.Fatalf("server failures must retain request error, got %T %+v", err, err)
	}
}

func TestSub2APIBulkAccountUpdate_RejectsNonNumericAccountID(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		writeJSON(w, map[string]any{"success": true})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}
	if err := service.UpdateAdminTargetPriority(session, "acc-1", 1); err == nil {
		t.Fatal("expected a non-numeric Sub2API account ID to be rejected")
	}
	if requestCount != 0 {
		t.Fatalf("invalid account ID must be rejected before sending a request, count=%d", requestCount)
	}
}

func TestUpdateSub2APIAdminAccountStatus_RejectsWrongPlatform(t *testing.T) {
	service := NewPlatformService(NewHTTPClient(http.DefaultClient))
	session := Session{Platform: PlatformNewAPI, BaseURL: "https://example.com", AccessToken: "token-1"}
	if err := service.UpdateSub2APIAdminAccountStatus(session, "1515", "inactive"); err == nil {
		t.Fatal("expected error for non-Sub2API session")
	}
}

func assertSub2APIBulkAccountIDs(t *testing.T, body map[string]any, expected float64) {
	t.Helper()
	accountIDs, ok := body["account_ids"].([]any)
	if !ok || len(accountIDs) != 1 || accountIDs[0] != expected {
		t.Fatalf("unexpected account_ids: %+v", body["account_ids"])
	}
}
