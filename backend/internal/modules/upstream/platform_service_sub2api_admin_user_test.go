package upstream

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestFetchSub2APIAdminUsersPage_RequestQueryAuthAndParsing(t *testing.T) {
	var gotPath, gotAuth string
	var gotQuery url.Values
	var gotRawQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		gotRawQuery = r.URL.RawQuery
		writeJSON(w, map[string]any{
			"data": map[string]any{
				"items": []map[string]any{
					{"id": "42", "email": "user@example.com", "username": "alice", "role": "admin", "status": "active", "created_at": "2025-01-02T03:04:05Z"},
					{"id": "", "email": "ignored@example.com"},
				},
				"total":     101,
				"page":      1,
				"page_size": 100,
				"pages":     2,
			},
		})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}
	page, err := service.FetchSub2APIAdminUsersPage(session, Sub2APIAdminUsersQuery{
		Page: -2, PageSize: 500, Status: "active", Role: "admin", Search: " alice+notes & keys ", SortBy: "not_allowed", SortOrder: "sideways", Timezone: "Asia/Shanghai",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/admin/users" {
		t.Fatalf("expected path /api/v1/admin/users, got %q", gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("expected Bearer auth, got %q", gotAuth)
	}
	assertQueryValue(t, gotQuery, "include_subscriptions", "true")
	assertQueryValue(t, gotQuery, "page", "1")
	assertQueryValue(t, gotQuery, "page_size", "100")
	assertQueryValue(t, gotQuery, "status", "active")
	assertQueryValue(t, gotQuery, "role", "admin")
	assertQueryValue(t, gotQuery, "search", "alice+notes & keys")
	if !strings.Contains(gotRawQuery, "search=alice%2Bnotes+%26+keys") {
		t.Fatalf("expected encoded search in raw query, got %q", gotRawQuery)
	}
	assertQueryValue(t, gotQuery, "timezone", "Asia/Shanghai")
	assertQueryValue(t, gotQuery, "sort_by", "created_at")
	assertQueryValue(t, gotQuery, "sort_order", "desc")
	if page.Total != 101 || page.Page != 1 || page.PageSize != 100 || page.Pages != 2 {
		t.Fatalf("unexpected pagination: %+v", page)
	}
	if !page.TotalKnown || !page.PagesKnown {
		t.Fatalf("expected explicit upstream pagination metadata to be marked known: %+v", page)
	}
	if len(page.Items) != 1 {
		t.Fatalf("expected blank-id item to be skipped, got %+v", page.Items)
	}
	wantTime := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	item := page.Items[0]
	if item.ID != "42" || item.Email != "user@example.com" || item.Username != "alice" || item.Role != "admin" || item.Status != "active" {
		t.Fatalf("unexpected parsed item: %+v", item)
	}
	if item.CreatedAt == nil || !item.CreatedAt.Equal(wantTime) {
		t.Fatalf("expected createdAt %v, got %+v", wantTime, item.CreatedAt)
	}
}

func TestFetchSub2APIAdminUserBreakdown_RequestQueryAuthAndParsing(t *testing.T) {
	var gotPath, gotAuth string
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotQuery = r.URL.Query()
		writeJSON(w, map[string]any{
			"data": map[string]any{
				"start_date": "2026-07-12",
				"end_date":   "2026-07-13",
				"users": []map[string]any{
					{"user_id": "u-1", "email": "u1@example.com", "requests": 12, "total_tokens": 456, "actual_cost": 1.25},
					{"user_id": "u-2", "email": "u2@example.com", "requests": 3, "input_tokens": 10, "output_tokens": 20, "cache_tokens": 5, "total_tokens": 35, "cost": 0.7, "actual_cost": 0.5},
				},
			},
		})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}
	breakdown, err := service.FetchSub2APIAdminUserBreakdown(session, Sub2APIUserBreakdownQuery{StartDate: "2026-07-12", EndDate: "2026-07-13", SortBy: "email", Limit: 500, Timezone: "Asia/Shanghai"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotPath != "/api/v1/admin/dashboard/user-breakdown" {
		t.Fatalf("expected breakdown path, got %q", gotPath)
	}
	if gotAuth != "Bearer admin-token" {
		t.Fatalf("expected Bearer auth, got %q", gotAuth)
	}
	assertQueryValue(t, gotQuery, "start_date", "2026-07-12")
	assertQueryValue(t, gotQuery, "end_date", "2026-07-13")
	assertQueryValue(t, gotQuery, "sort_by", "total_tokens")
	assertQueryValue(t, gotQuery, "limit", "200")
	assertQueryValue(t, gotQuery, "timezone", "Asia/Shanghai")
	if breakdown.StartDate != "2026-07-12" || breakdown.EndDate != "2026-07-13" || len(breakdown.Users) != 2 {
		t.Fatalf("unexpected parsed breakdown: %+v", breakdown)
	}
	first := breakdown.Users[0]
	if first.UserID != "u-1" || first.Email != "u1@example.com" || first.Requests != 12 || first.TotalTokens != 456 || first.ActualCost != 1.25 {
		t.Fatalf("unexpected first row: %+v", first)
	}
	if first.InputTokens != 0 || first.OutputTokens != 0 || first.CacheTokens != 0 || first.Cost != 0 {
		t.Fatalf("missing token breakdown fields should stay zero, got %+v", first)
	}
}

func TestFetchSub2APIAdminUserBreakdown_Unsupported404Status(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		writeJSON(w, map[string]any{"code": 404, "message": "not found", "data": map[string]any{}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	_, err := service.FetchSub2APIAdminUserBreakdown(Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token"}, Sub2APIUserBreakdownQuery{StartDate: "2026-07-12", EndDate: "2026-07-13"})
	requestErr, ok := err.(*RequestError)
	if !ok || requestErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected RequestError with 404 status, got %#v", err)
	}
}

func TestFetchSub2APIAdminUsersPage_UsesNormalizedPaginationFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"data": map[string]any{
				"items": []map[string]any{{"id": "1", "email": "user@example.com"}},
			},
		})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}
	page, err := service.FetchSub2APIAdminUsersPage(session, Sub2APIAdminUsersQuery{Page: -2, PageSize: 500})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Page != 1 || page.PageSize != 100 {
		t.Fatalf("expected normalized fallback page=1 pageSize=100, got %+v", page)
	}
}

func TestFetchSub2APIAdminUsersPage_OmitsEmptySearch(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		writeJSON(w, map[string]any{"data": map[string]any{"items": []map[string]any{}, "total": 0, "page": 1, "page_size": 20, "pages": 0}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}
	_, err := service.FetchSub2APIAdminUsersPage(session, Sub2APIAdminUsersQuery{Page: 1, PageSize: 20, Search: " \t\n "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := gotQuery["search"]; ok {
		t.Fatalf("expected empty search to be omitted, full query=%v", gotQuery)
	}
}

func TestFetchSub2APIAdminUsersPage_MarksMissingPaginationUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{"items": []map[string]any{{"id": "1", "email": "a@example.com"}}}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}
	page, err := service.FetchSub2APIAdminUsersPage(session, Sub2APIAdminUsersQuery{Page: 1, PageSize: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.TotalKnown || page.PagesKnown {
		t.Fatalf("expected missing upstream pagination metadata to remain unknown: %+v", page)
	}
	if page.Total != 1 || page.Pages != 0 {
		t.Fatalf("unexpected fallback pagination values: %+v", page)
	}
}

func TestFetchSub2APIAdminUsersPage_AllowsKnownSortAndAscendingOrder(t *testing.T) {
	var gotQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		writeJSON(w, map[string]any{"data": map[string]any{"items": []map[string]any{}, "total": 0, "page": 3, "page_size": 20, "pages": 0}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "admin-token", TokenType: "Bearer"}
	_, err := service.FetchSub2APIAdminUsersPage(session, Sub2APIAdminUsersQuery{Page: 3, PageSize: 20, SortBy: "email", SortOrder: "asc"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertQueryValue(t, gotQuery, "sort_by", "email")
	assertQueryValue(t, gotQuery, "sort_order", "asc")
	assertQueryValue(t, gotQuery, "page", "3")
	assertQueryValue(t, gotQuery, "page_size", "20")
}

func assertQueryValue(t *testing.T, values url.Values, key string, want string) {
	t.Helper()
	if got := values.Get(key); got != want {
		t.Fatalf("query %s = %q, want %q; full query=%v", key, got, want, values)
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
