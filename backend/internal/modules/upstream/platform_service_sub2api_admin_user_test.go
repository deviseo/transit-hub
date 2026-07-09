package upstream

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestFetchSub2APIAdminUser_RequestPathAndAuthHeader 验证请求路径拼接和 Authorization header，
// 并覆盖 snake_case 字段解析（id/email/username/role/status/balance/frozen_balance/concurrency/
// rpm_limit/created_at）。
func TestFetchSub2APIAdminUser_RequestPathAndAuthHeader(t *testing.T) {
	var gotPath, gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		writeJSON(w, map[string]any{
			"data": map[string]any{
				"id": "42", "email": "sub2api@example.com", "username": "alice", "role": "member",
				"status": "active", "balance": 12.5, "frozen_balance": 1.5,
				"concurrency": 3, "rpm_limit": 60, "created_at": "2025-01-02T03:04:05Z",
			},
		})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}

	user, err := service.FetchSub2APIAdminUser(session, "42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/admin/users/42" {
		t.Fatalf("expected request path /api/v1/admin/users/42, got %q", gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("expected Authorization header 'Bearer admin-token', got %q", gotAuth)
	}
	if user.ID != "42" || user.Email != "sub2api@example.com" || user.Username != "alice" || user.Role != "member" || user.Status != "active" {
		t.Fatalf("unexpected parsed identity fields: %+v", user)
	}
	if user.Balance == nil || *user.Balance != 12.5 {
		t.Fatalf("expected balance 12.5, got %+v", user.Balance)
	}
	if user.FrozenBalance == nil || *user.FrozenBalance != 1.5 {
		t.Fatalf("expected frozen balance 1.5, got %+v", user.FrozenBalance)
	}
	if user.Concurrency == nil || *user.Concurrency != 3 {
		t.Fatalf("expected concurrency 3, got %+v", user.Concurrency)
	}
	if user.RPMLimit == nil || *user.RPMLimit != 60 {
		t.Fatalf("expected rpmLimit 60, got %+v", user.RPMLimit)
	}
	wantTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if user.CreatedAt == nil || !user.CreatedAt.Equal(wantTime) {
		t.Fatalf("expected createdAt %v, got %+v", wantTime, user.CreatedAt)
	}
}

// TestFetchSub2APIAdminUser_CamelCaseFieldsAndTimestamp 验证 camelCase 字段名和 unix 秒时间戳解析。
func TestFetchSub2APIAdminUser_CamelCaseFieldsAndTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"userId": "7", "frozenBalance": 2.0, "rpmLimit": 30, "lastUsedAt": float64(1735689600),
		})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}

	user, err := service.FetchSub2APIAdminUser(session, "7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if user.ID != "7" {
		t.Fatalf("expected id parsed from userId, got %q", user.ID)
	}
	if user.FrozenBalance == nil || *user.FrozenBalance != 2.0 {
		t.Fatalf("expected frozenBalance 2.0, got %+v", user.FrozenBalance)
	}
	if user.RPMLimit == nil || *user.RPMLimit != 30 {
		t.Fatalf("expected rpmLimit 30, got %+v", user.RPMLimit)
	}
	if user.LastUsedAt == nil {
		t.Fatalf("expected lastUsedAt to be parsed from unix seconds timestamp")
	}
}

// TestFetchSub2APIAdminUser_RejectsNonSub2APISession 验证非 sub2api session 直接拒绝，不发请求。
func TestFetchSub2APIAdminUser_RejectsNonSub2APISession(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		writeJSON(w, map[string]any{})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "token"}

	if _, err := service.FetchSub2APIAdminUser(session, "42"); err == nil {
		t.Fatalf("expected error for non-sub2api session")
	}
	if called {
		t.Fatalf("must not issue a request for a non-sub2api session")
	}
}

// TestFetchSub2APIAdminUser_PropagatesNotFound 验证远端 404 时错误透传，不返回伪造数据。
func TestFetchSub2APIAdminUser_PropagatesNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token"}

	if _, err := service.FetchSub2APIAdminUser(session, "42"); err == nil {
		t.Fatalf("expected error propagated from 404 response")
	}
}

// TestFetchSub2APIAdminUserBalanceHistory_RequestPathAndPagination 验证分页 query 参数拼接、
// type 参数透传，以及 items/total/total_recharged 解析。
func TestFetchSub2APIAdminUserBalanceHistory_RequestPathAndPagination(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		writeJSON(w, map[string]any{
			"data": map[string]any{
				"total": 2, "total_recharged": 99.5,
				"items": []any{
					map[string]any{"id": "h1", "type": "balance", "amount": 10.0, "notes": "recharge", "created_at": "2025-02-03T04:05:06Z"},
					map[string]any{"id": "h2", "code_type": "balance", "balance": 5.0, "created_at": float64(1738540800)},
				},
			},
		})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}

	history, err := service.FetchSub2APIAdminUserBalanceHistory(session, "42", 1, 20, "balance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/admin/users/42/balance-history" {
		t.Fatalf("expected balance-history path, got %q", gotPath)
	}
	if gotQuery != "page=1&page_size=20&type=balance" {
		t.Fatalf("unexpected query string: %q", gotQuery)
	}
	if history.Total != 2 {
		t.Fatalf("expected total 2, got %d", history.Total)
	}
	if history.TotalRecharged == nil || *history.TotalRecharged != 99.5 {
		t.Fatalf("expected totalRecharged 99.5, got %+v", history.TotalRecharged)
	}
	if len(history.Items) != 2 {
		t.Fatalf("expected 2 history items, got %d", len(history.Items))
	}
	if history.Items[0].ID != "h1" || history.Items[0].Type != "balance" || history.Items[0].Amount == nil || *history.Items[0].Amount != 10.0 || history.Items[0].Note != "recharge" {
		t.Fatalf("unexpected first history item: %+v", history.Items[0])
	}
	if history.Items[0].CreatedAt == nil {
		t.Fatalf("expected first history item createdAt to be parsed")
	}
	if history.Items[1].Type != "balance" || history.Items[1].Amount == nil || *history.Items[1].Amount != 5.0 {
		t.Fatalf("unexpected second history item (code_type/balance fallback fields): %+v", history.Items[1])
	}
}

// TestFetchSub2APIAdminUserBalanceHistory_InvalidPagingFallsBackToDefaults 验证越界分页参数
// 回退到 page=1/pageSize=20。
func TestFetchSub2APIAdminUserBalanceHistory_InvalidPagingFallsBackToDefaults(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		writeJSON(w, map[string]any{"data": map[string]any{"items": []any{}}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token"}

	if _, err := service.FetchSub2APIAdminUserBalanceHistory(session, "42", 0, -5, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotQuery != "page=1&page_size=20" {
		t.Fatalf("expected default paging query without type, got %q", gotQuery)
	}
}

// TestFetchSub2APIAdminUserBalanceHistory_RejectsEmptyUserID 验证空用户 ID 直接拒绝，不发请求。
func TestFetchSub2APIAdminUserBalanceHistory_RejectsEmptyUserID(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token"}

	if _, err := service.FetchSub2APIAdminUserBalanceHistory(session, "  ", 1, 20, ""); err == nil {
		t.Fatalf("expected error for empty user id")
	}
	if called {
		t.Fatalf("must not issue a request for an empty user id")
	}
}
