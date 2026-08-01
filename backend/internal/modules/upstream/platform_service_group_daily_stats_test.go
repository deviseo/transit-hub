package upstream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestFetchAdminGroupDailyStats_DispatchesByPlatform 验证平台中性包装方法按
// session.Platform 正确路由到 sub2api / new-api 具体实现，不重复实现底层抓取逻辑。
func TestFetchAdminGroupDailyStats_DispatchesByPlatform(t *testing.T) {
	t.Run("sub2api uses usage-summary endpoint", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v1/groups/available":
				writeJSON(w, map[string]any{"data": []map[string]any{
					{"id": 1, "name": "default"},
					{"id": 2, "name": "vip"},
				}})
			case "/api/v1/admin/groups/usage-summary":
				writeJSON(w, map[string]any{"data": []map[string]any{
					{"group_id": 1, "today_actual_cost": 12.5},
					{"group_id": 2, "today_cost": 7.25},
				}})
			default:
				t.Fatalf("unexpected sub2api path: %s", r.URL.Path)
			}
		}))
		defer server.Close()

		service := NewPlatformService(NewHTTPClient(server.Client()))
		session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}

		stats, err := service.FetchAdminGroupDailyStats(session, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("expected 2 stats, got %d", len(stats))
		}
		byName := map[string]float64{}
		for _, s := range stats {
			byName[s.GroupName] = s.TodayActualCost
		}
		if byName["default"] != 12.5 {
			t.Errorf("default cost = %.2f, want 12.50", byName["default"])
		}
		if byName["vip"] != 7.25 {
			t.Errorf("vip cost = %.2f, want 7.25", byName["vip"])
		}
	})

	t.Run("new-api uses per-group log stat with quota conversion", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/log/self/stat" {
				t.Fatalf("unexpected new-api path: %s", r.URL.Path)
			}
			group := r.URL.Query().Get("group")
			quota := map[string]float64{"default": 100000, "vip": 250000}[group]
			writeJSON(w, map[string]any{"data": map[string]any{"quota": quota}})
		}))
		defer server.Close()

		service := NewPlatformService(NewHTTPClient(server.Client()))
		session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "1", QuotaPerUnit: 100000}
		groups := []GroupInfo{{Name: "default"}, {Name: "vip"}}

		stats, err := service.FetchAdminGroupDailyStats(session, groups)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(stats) != 2 {
			t.Fatalf("expected 2 stats, got %d", len(stats))
		}
		byName := map[string]float64{}
		for _, s := range stats {
			byName[s.GroupName] = s.TodayActualCost
		}
		// quota / quotaPerUnit: 100000/100000 = 1.0, 250000/100000 = 2.5
		if byName["default"] != 1.0 {
			t.Errorf("default amount = %.4f, want 1.0000 (quota/quotaPerUnit conversion)", byName["default"])
		}
		if byName["vip"] != 2.5 {
			t.Errorf("vip amount = %.4f, want 2.5000 (quota/quotaPerUnit conversion)", byName["vip"])
		}
	})
}

func TestFetchAdminGroupDailyStatsForDate_Sub2APIUsesRequestedBusinessDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/keys":
			writeJSON(w, map[string]any{"data": []map[string]any{{
				"id":    1,
				"group": map[string]any{"name": "vip"},
			}}})
		case "/api/v1/usage/stats":
			if got := r.URL.Query().Get("start_date"); got != "2026-07-31" {
				t.Errorf("start_date = %q, want 2026-07-31", got)
			}
			if got := r.URL.Query().Get("end_date"); got != "2026-07-31" {
				t.Errorf("end_date = %q, want 2026-07-31", got)
			}
			if got := r.URL.Query().Get("timezone"); got != "Asia/Shanghai" {
				t.Errorf("timezone = %q, want Asia/Shanghai", got)
			}
			writeJSON(w, map[string]any{"data": map[string]any{"total_actual_cost": 12.5}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	stats, err := service.FetchAdminGroupDailyStatsForDate(
		Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"},
		nil,
		"2026-07-31",
	)
	if err != nil {
		t.Fatalf("FetchAdminGroupDailyStatsForDate() error: %v", err)
	}
	if len(stats) != 1 || stats[0].GroupName != "vip" || stats[0].TodayActualCost != 12.5 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestFetchAdminGroupDailyStatsForDate_NewAPIUsesOneRequestedBusinessDay(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/log/self/stat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("start_timestamp"); got != "1785427200" {
			t.Errorf("start_timestamp = %q, want Shanghai 2026-07-31 00:00:00", got)
		}
		if got := r.URL.Query().Get("end_timestamp"); got != "1785513599" {
			t.Errorf("end_timestamp = %q, want Shanghai 2026-07-31 23:59:59", got)
		}
		writeJSON(w, map[string]any{"data": map[string]any{"quota": 250000}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	stats, err := service.FetchAdminGroupDailyStatsForDate(
		Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "1", QuotaPerUnit: 100000},
		[]GroupInfo{{Name: "vip"}},
		"2026-07-31",
	)
	if err != nil {
		t.Fatalf("FetchAdminGroupDailyStatsForDate() error: %v", err)
	}
	if len(stats) != 1 || stats[0].GroupName != "vip" || stats[0].TodayActualCost != 2.5 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

// TestSub2APICostParsers 验证 sub2api 各降级路径的字段解析覆盖文档要求的
// today_actual_cost / total_actual_cost / actual_cost 语义。
func TestSub2APICostParsers(t *testing.T) {
	t.Run("group usage summary prefers today_actual_cost", func(t *testing.T) {
		got := sub2APIUsageSummaryCost(map[string]any{"today_actual_cost": 9.5})
		if got != 9.5 {
			t.Errorf("got %.2f, want 9.50", got)
		}
	})

	t.Run("per-key usage stats prefers total_actual_cost", func(t *testing.T) {
		got := sub2APIUsageStatsCost(map[string]any{"total_actual_cost": 4.2})
		if got != 4.2 {
			t.Errorf("got %.2f, want 4.20", got)
		}
	})

	t.Run("per-key usage stats falls back to actual_cost", func(t *testing.T) {
		got := sub2APIUsageStatsCost(map[string]any{"actual_cost": 3.1})
		if got != 3.1 {
			t.Errorf("got %.2f, want 3.10", got)
		}
	})

	t.Run("admin dashboard groups fallback parses today_actual_cost", func(t *testing.T) {
		got := sub2APIGroupDailyCost(map[string]any{"today_actual_cost": 6.6})
		if got != 6.6 {
			t.Errorf("got %.2f, want 6.60", got)
		}
	})

	t.Run("admin dashboard groups fallback falls back to actual_cost", func(t *testing.T) {
		got := sub2APIGroupDailyCost(map[string]any{"actual_cost": 1.23})
		if got != 1.23 {
			t.Errorf("got %.2f, want 1.23", got)
		}
	})
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
