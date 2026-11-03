package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// 「今日成本」相关测试，数据均为合成数据（example.test）。

func TestFetchSelfConsumeOnDate(t *testing.T) {
	var mu sync.Mutex
	seen := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen[r.URL.Path] = r.URL.RawQuery
		mu.Unlock()
		switch r.URL.Path {
		case "/sub/api/v1/usage/stats":
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"total_requests": 9, "total_cost": 3, "total_actual_cost": 2.5}})
		case "/new/api/log/self/stat":
			writeJSON(w, map[string]any{"success": true, "data": map[string]any{"quota": 250000}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewPlatformService(NewHTTPClient(server.Client()))

	cost, err := service.FetchSelfConsumeOnDate(Session{Platform: PlatformSub2API, BaseURL: server.URL + "/sub", AccessToken: "token"}, "2026-11-02", "Asia/Shanghai")
	if err != nil || cost != 2.5 {
		t.Fatalf("sub2api cost = %v, %v", cost, err)
	}
	cost, err = service.FetchSelfConsumeOnDate(Session{Platform: PlatformNewAPI, BaseURL: server.URL + "/new", AccessToken: "nap_token", QuotaPerUnit: 500000}, "2026-11-02", "Asia/Shanghai")
	if err != nil || cost != 0.5 {
		t.Fatalf("new-api cost = %v, %v", cost, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if q := seen["/sub/api/v1/usage/stats"]; !strings.Contains(q, "start_date=2026-11-02") || !strings.Contains(q, "end_date=2026-11-02") || !strings.Contains(q, "timezone=Asia%2FShanghai") || strings.Contains(q, "api_key_id") {
		t.Fatalf("unexpected sub2api query %q", q)
	}
	// 2026-11-02 00:00 上海 = 1793548800；当天 23:59:59 = 1793635199。
	if q := seen["/new/api/log/self/stat"]; !strings.Contains(q, "start_timestamp=1793548800") || !strings.Contains(q, "end_timestamp=1793635199") || !strings.Contains(q, "type=2") {
		t.Fatalf("unexpected new-api query %q", q)
	}
	if _, err := service.FetchSelfConsumeOnDate(Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, "11/02", "Asia/Shanghai"); err == nil {
		t.Fatal("invalid dates must be rejected")
	}
}

func TestRefreshStaleSitesOnlySyncsTodaysStaleSites(t *testing.T) {
	var mu sync.Mutex
	requested := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requested[strings.SplitN(strings.TrimPrefix(r.URL.Path, "/"), "/", 2)[0]]++
		mu.Unlock()
		http.Error(w, `{"code":500}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	since := time.Date(2026, 11, 3, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	before := since.Add(-time.Hour).UnixMilli()
	after := since.Add(time.Hour).UnixMilli()
	cache := newFakeSiteCache()
	session := func(name string) *Session {
		return &Session{Platform: PlatformSub2API, BaseURL: server.URL + "/" + name, AccessToken: "token"}
	}
	cache.add(&Site{ID: "stale", UserID: "u1", AdminAccountID: "a1", RechargeRate: 1, Session: session("stale"), LastSyncedAt: &before})
	cache.add(&Site{ID: "never", UserID: "u1", AdminAccountID: "a1", RechargeRate: 2, Session: session("never")})
	cache.add(&Site{ID: "fresh", UserID: "u1", AdminAccountID: "a1", RechargeRate: 1, Session: session("fresh"), LastSyncedAt: &after})
	cache.add(&Site{ID: "other-workspace", UserID: "u1", AdminAccountID: "a2", RechargeRate: 1, Session: session("other"), LastSyncedAt: &before})
	cache.add(&Site{ID: "no-session", UserID: "u1", AdminAccountID: "a1", RechargeRate: 1, LastSyncedAt: &before})
	cache.add(&Site{ID: "no-rate", UserID: "u1", AdminAccountID: "a1", RechargeRate: 0, Session: session("norate"), LastSyncedAt: &before})
	service := NewService(NewPlatformService(NewHTTPClient(server.Client())), nil, nil, cache)

	service.RefreshStaleSites(context.Background(), "u1", "a1", since, 5*time.Second)
	mu.Lock()
	got := map[string]int{}
	for key, value := range requested {
		got[key] = value
	}
	mu.Unlock()
	if len(got) != 2 || got["stale"] == 0 || got["never"] == 0 {
		t.Fatalf("only stale sites of the workspace with a session and a rate may be synced, got %v", got)
	}
	if site, _ := cache.Get(context.Background(), "stale"); site.Status != StatusError || *site.LastSyncedAt != before {
		t.Fatalf("a failed sync must keep the old sync time so the site stays excluded: %+v", site)
	}

	// 冷却期内不重复触发（同步一直失败的站点不会在每次打开仪表盘时都被请求）。
	service.RefreshStaleSites(context.Background(), "u1", "a1", since, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if requested["stale"] != got["stale"] || requested["never"] != got["never"] {
		t.Fatalf("sites must not be retried within the cooldown: before=%v after=%v", got, requested)
	}
}

func TestConsumeOnDateQueriesSitesAndFallsBackToSameDayCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok/api/v1/usage/stats":
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"total_actual_cost": 2.5}})
		default:
			http.Error(w, `{"code":500}`, http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	loc, _ := time.LoadLocation("Asia/Shanghai")
	sameDay := time.Date(2026, 11, 2, 23, 0, 0, 0, loc).UnixMilli()
	nextDay := time.Date(2026, 11, 3, 0, 10, 0, 0, loc).UnixMilli()
	cached := func(value float64) Metrics {
		return Metrics{TodayConsume: metric(&value)}
	}
	cache := newFakeSiteCache()
	cache.add(&Site{ID: "ok", UserID: "u1", AdminAccountID: "a1", RechargeRate: 2, Session: &Session{Platform: PlatformSub2API, BaseURL: server.URL + "/ok", AccessToken: "t"}})
	cache.add(&Site{ID: "fallback", UserID: "u1", AdminAccountID: "a1", RechargeRate: 1, Session: &Session{Platform: PlatformSub2API, BaseURL: server.URL + "/broken", AccessToken: "t"}, Metrics: cached(3), LastSyncedAt: &sameDay})
	cache.add(&Site{ID: "unknown", UserID: "u1", AdminAccountID: "a1", RechargeRate: 1, Metrics: cached(7), LastSyncedAt: &nextDay})
	cache.add(&Site{ID: "other", UserID: "u1", AdminAccountID: "a2", RechargeRate: 1, Session: &Session{Platform: PlatformSub2API, BaseURL: server.URL + "/ok", AccessToken: "t"}})
	service := NewService(NewPlatformService(NewHTTPClient(server.Client())), nil, nil, cache)

	result, err := service.ConsumeOnDate(context.Background(), "u1", "a1", "2026-11-02", "Asia/Shanghai")
	if err != nil {
		t.Fatalf("ConsumeOnDate: %v", err)
	}
	if result.Total != 8 || result.Sites != 2 || result.Approximated != 1 || result.Failed != 1 {
		t.Fatalf("unexpected result: %+v (want 2.5×2 + 3 approximated, one site unknown)", result)
	}
}
