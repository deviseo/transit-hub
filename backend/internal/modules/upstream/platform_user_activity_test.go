package upstream

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestFetchSub2APIUserUsageTrend(t *testing.T) {
	var mu sync.Mutex
	var query map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/dashboard/users-trend" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		q := r.URL.Query()
		mu.Lock()
		query = map[string]string{"start": q.Get("start_date"), "end": q.Get("end_date"), "granularity": q.Get("granularity"), "metric": q.Get("metric"), "limit": q.Get("limit"), "tz": q.Get("timezone")}
		mu.Unlock()
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"trend": []map[string]any{
			{"date": "2026-11-01", "user_id": 7, "email": "u7@example.test", "username": "u7", "requests": 12, "tokens": 3400, "cost": 1.5, "actual_cost": 1.2},
			{"date": "2026-11-01", "user_id": 0, "requests": 1},
			{"user_id": 8, "requests": 1},
		}}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	points, err := service.FetchSub2APIUserUsageTrend(Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, "2026-10-19", "2026-11-02", "Asia/Shanghai", 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if query["start"] != "2026-10-19" || query["end"] != "2026-11-02" || query["granularity"] != "day" || query["metric"] != "actual_cost" || query["limit"] != "500" || query["tz"] != "Asia/Shanghai" {
		t.Fatalf("unexpected query: %+v", query)
	}
	if len(points) != 1 {
		t.Fatalf("rows without a user id or date must be skipped, got %+v", points)
	}
	got := points[0]
	if got.UserID != "7" || got.Date != "2026-11-01" || got.Requests != 12 || got.Tokens != 3400 || got.ActualCost != 1.2 || got.Email != "u7@example.test" {
		t.Fatalf("unexpected point: %+v", got)
	}
}

func TestFetchNewAPIAdminUsersPage(t *testing.T) {
	created := time.Date(2026, 10, 1, 8, 0, 0, 0, time.UTC).Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/" || r.URL.Query().Get("p") != "2" || r.URL.Query().Get("page_size") != "100" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"total": 101, "items": []map[string]any{
			{"id": 5, "username": "alice", "email": "alice@example.test", "group": "vip", "role": 1, "status": 1, "quota": 1000000, "used_quota": 250000, "request_count": 9, "created_at": created, "last_login_at": 0},
			{"id": 6, "username": "bob", "email": "", "group": "default", "role": 10, "status": 2, "quota": 0, "used_quota": 0, "request_count": 0, "created_at": created},
		}}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	page, err := service.FetchNewAPIAdminUsersPage(Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "nap_token", QuotaPerUnit: 500000}, 2, 500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !page.TotalKnown || page.Total != 101 || len(page.Items) != 2 {
		t.Fatalf("unexpected page: %+v", page)
	}
	alice := page.Items[0]
	if alice.ID != "5" || alice.Group != "vip" || alice.Balance != 2 || alice.UsedQuota != 0.5 || !alice.EverUsed || alice.CreatedAt == nil || alice.CreatedAt.Unix() != created || alice.LastLoginAt != nil {
		t.Fatalf("unexpected first user: %+v", alice)
	}
	if bob := page.Items[1]; bob.EverUsed || bob.Role != 10 || bob.Status != 2 {
		t.Fatalf("unexpected second user: %+v", bob)
	}
}

func TestFetchNewAPIUserQuotaData(t *testing.T) {
	start := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	end := start.Add(48 * time.Hour)
	hour := start.Add(3 * time.Hour).Unix()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/data/users" || r.URL.Query().Get("start_timestamp") == "" || r.URL.Query().Get("end_timestamp") == "" {
			t.Errorf("unexpected request %s", r.URL.String())
		}
		writeJSON(w, map[string]any{"success": true, "data": []map[string]any{
			{"username": "alice", "created_at": hour, "count": 4, "quota": 250000, "token_used": 1200},
			{"username": "", "created_at": hour, "count": 1},
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	buckets, err := service.FetchNewAPIUserQuotaData(Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "nap_token", QuotaPerUnit: 500000}, start, end)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(buckets) != 1 || buckets[0].Username != "alice" || buckets[0].Requests != 4 || buckets[0].Tokens != 1200 || buckets[0].Cost != 0.5 || buckets[0].HourStart.Unix() != hour {
		t.Fatalf("unexpected buckets: %+v", buckets)
	}
}

func TestParseSub2APIAdminUserTotalRecharged(t *testing.T) {
	user := parseSub2APIAdminUser(map[string]any{"id": 3, "email": "u3@example.test", "total_recharged": 88.5, "last_used_at": "2026-11-01T10:00:00Z"})
	if user.TotalRecharged == nil || *user.TotalRecharged != 88.5 || user.LastUsedAt == nil {
		t.Fatalf("unexpected user: %+v", user)
	}
	if older := parseSub2APIAdminUser(map[string]any{"id": 4}); older.TotalRecharged != nil {
		t.Fatalf("missing total_recharged must stay nil, got %v", *older.TotalRecharged)
	}
}

func TestFetchSub2APIUsedRedeemRecords(t *testing.T) {
	var mu sync.Mutex
	var query map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/redeem-codes" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		q := r.URL.Query()
		mu.Lock()
		query = map[string]string{"type": q.Get("type"), "status": q.Get("status"), "sort_by": q.Get("sort_by"), "sort_order": q.Get("sort_order"), "page": q.Get("page"), "page_size": q.Get("page_size")}
		mu.Unlock()
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"total": 3, "page": 2, "page_size": 500, "items": []map[string]any{
			{"id": 41, "code": "PAY-17-12345", "type": "balance", "value": 20, "status": "used", "used_by": 7, "used_at": "2026-11-02T01:30:00Z"},
			{"id": 40, "code": "ADJ123", "type": "admin_balance", "value": -5.5, "status": "used", "used_by": 8, "used_at": "2026-11-01T23:00:00+08:00"},
			{"code": "missing-id"},
		}}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	page, err := service.FetchSub2APIUsedRedeemRecords(Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, "balance", 2, 9999)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if query["type"] != "balance" || query["status"] != "used" || query["sort_by"] != "used_at" || query["sort_order"] != "desc" || query["page"] != "2" || query["page_size"] != "500" {
		t.Fatalf("unexpected query: %+v", query)
	}
	if !page.TotalKnown || page.Total != 3 || len(page.Items) != 2 {
		t.Fatalf("unexpected page: %+v", page)
	}
	first, second := page.Items[0], page.Items[1]
	if first.ID != "41" || first.Code != "PAY-17-12345" || first.Value != 20 || first.UsedBy != "7" || first.UsedAt == nil || !first.UsedAt.Equal(time.Date(2026, 11, 2, 1, 30, 0, 0, time.UTC)) {
		t.Fatalf("unexpected first record: %+v", first)
	}
	if second.Type != "admin_balance" || second.Value != -5.5 || second.UsedAt == nil || !second.UsedAt.Equal(time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC)) {
		t.Fatalf("unexpected second record: %+v", second)
	}
}

func TestFetchSub2APIAffiliateTransfers(t *testing.T) {
	var mu sync.Mutex
	var query map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/affiliates/transfers":
		case "/json404/api/v1/admin/affiliates/transfers":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"not found"}`))
			return
		default:
			http.NotFound(w, r)
			return
		}
		q := r.URL.Query()
		mu.Lock()
		query = map[string]string{"start": q.Get("start_at"), "end": q.Get("end_at"), "tz": q.Get("timezone"), "page_size": q.Get("page_size")}
		mu.Unlock()
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"total": 2, "items": []map[string]any{
			{"ledger_id": 9, "action": "transfer", "user_id": 7, "amount": 3.25, "created_at": "2026-11-02T02:00:00Z"},
			{"ledger_id": 8, "action": "withdraw", "user_id": 7, "amount": 1, "created_at": "2026-11-02T01:00:00Z"},
		}}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	page, err := service.FetchSub2APIAffiliateTransfers(Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, "2026-10-01", "2026-11-02", "Asia/Shanghai", 1, 1000)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if query["start"] != "2026-10-01" || query["end"] != "2026-11-02" || query["tz"] != "Asia/Shanghai" || query["page_size"] != "100" {
		t.Fatalf("unexpected query: %+v", query)
	}
	if page.Total != 2 || len(page.Items) != 2 || page.Items[0].Action != "transfer" || page.Items[0].Amount != 3.25 || page.Items[0].UserID != "7" || page.Items[0].CreatedAt == nil {
		t.Fatalf("unexpected page: %+v", page)
	}

	// 旧版本没有该接口：JSON 形式的 404 带出状态码；纯文本 404 页面无法解析，返回 invalidResponse。
	// 调用方（user_activity）把这两种情况都视为不支持，而不是同步失败。
	_, err = service.FetchSub2APIAffiliateTransfers(Session{Platform: PlatformSub2API, BaseURL: server.URL + "/json404", AccessToken: "token"}, "2026-10-01", "2026-11-02", "", 1, 100)
	var requestErr *RequestError
	if !errors.As(err, &requestErr) || requestErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected a 404 request error, got %v", err)
	}
	_, err = service.FetchSub2APIAffiliateTransfers(Session{Platform: PlatformSub2API, BaseURL: server.URL + "/legacy", AccessToken: "token"}, "2026-10-01", "2026-11-02", "", 1, 100)
	if !errors.As(err, &requestErr) || requestErr.MessageKey != ErrorInvalidResponse {
		t.Fatalf("expected an invalid response error for a plain-text 404 page, got %v", err)
	}
}
