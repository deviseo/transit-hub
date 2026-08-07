package upstream

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
)

// TestFetchKeyUsageToday_Sub2API_PaginatesKeysAndFiltersZeroCost 覆盖测试要求 5：
// sub2api key 列表必须分页拉取完整（不能只取第一页），且只保留今日消费 > 0 的 key。
func TestFetchKeyUsageToday_Sub2API_PaginatesKeysAndFiltersZeroCost(t *testing.T) {
	const totalKeys = 150 // 超过单页 100 条，强制触发第 2 页请求
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/keys":
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			const pageSize = 100
			start := (page - 1) * pageSize
			if start >= totalKeys {
				writeJSON(w, map[string]any{"data": []map[string]any{}, "total": totalKeys})
				return
			}
			end := start + pageSize
			if end > totalKeys {
				end = totalKeys
			}
			items := make([]map[string]any, 0, end-start)
			for i := start; i < end; i++ {
				items = append(items, map[string]any{
					"id":    i + 1,
					"name":  fmt.Sprintf("key-%d", i+1),
					"group": map[string]any{"name": "vip"},
				})
			}
			writeJSON(w, map[string]any{"data": items, "total": totalKeys})
		case "/api/v1/usage/stats":
			apiKeyID := r.URL.Query().Get("api_key_id")
			cost := 0.0
			switch apiKeyID {
			case "1":
				cost = 12.5
			case "150":
				cost = 3.25
			}
			writeJSON(w, map[string]any{"data": map[string]any{"total_actual_cost": cost}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}

	stats, err := service.FetchKeyUsageToday(session, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected 2 keys with nonzero cost, got %d: %+v", len(stats), stats)
	}
	byID := map[string]float64{}
	for _, s := range stats {
		byID[s.KeyID] = s.TodayAmount
		if s.GroupName != "vip" {
			t.Errorf("unexpected group name %q", s.GroupName)
		}
	}
	if byID["1"] != 12.5 {
		t.Errorf("key 1 cost = %.2f, want 12.50", byID["1"])
	}
	if byID["150"] != 3.25 {
		t.Errorf("key 150 (only reachable via page 2) cost = %.2f, want 3.25 — pagination may have stopped at page 1", byID["150"])
	}
}

func TestFetchKeyUsageTodayIncludingZero_Sub2APIKeepsExistingZeroCostKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/keys":
			writeJSON(w, map[string]any{"data": []map[string]any{
				{"id": 1, "name": "zero", "group": map[string]any{"name": "vip"}},
				{"id": 2, "name": "used", "group": map[string]any{"name": "vip"}},
			}, "total": 2})
		case "/api/v1/usage/stats":
			if got := r.URL.Query().Get("start_date"); got != "2026-08-07" || r.URL.Query().Get("end_date") != "2026-08-07" {
				t.Fatalf("unexpected business date: start=%q end=%q", got, r.URL.Query().Get("end_date"))
			}
			cost := 0.0
			if r.URL.Query().Get("api_key_id") == "2" {
				cost = 4.5
			}
			writeJSON(w, map[string]any{"data": map[string]any{"total_actual_cost": cost}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	stats, err := NewPlatformService(NewHTTPClient(server.Client())).FetchKeyUsageTodayIncludingZero(
		Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, nil, "2026-08-07",
	)
	if err != nil {
		t.Fatalf("FetchKeyUsageTodayIncludingZero() error: %v", err)
	}
	if len(stats) != 2 {
		t.Fatalf("expected existing zero-cost key to remain visible, got %+v", stats)
	}
	byID := map[string]float64{}
	for _, stat := range stats {
		byID[stat.KeyID] = stat.TodayAmount
	}
	if byID["1"] != 0 || byID["2"] != 4.5 {
		t.Fatalf("unexpected zero-inclusive key costs: %+v", byID)
	}
}

// TestFetchKeyUsageToday_NewAPI_UsesTokenNameAndGroupFilter 覆盖测试要求 6：
// new-api token 列表分页 + token_name/group 统计路径：带分组的 token 按 token_name+group 查询，
// 无分组的 token 只按 token_name 查询（不做全分组穷举）。
func TestFetchKeyUsageToday_NewAPI_UsesTokenNameAndGroupFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/token/":
			if r.URL.Query().Get("p") != "1" {
				writeJSON(w, map[string]any{"data": []map[string]any{}, "total": 2})
				return
			}
			writeJSON(w, map[string]any{
				"data": []map[string]any{
					{"id": 1, "name": "prod-key", "group": "vip"},
					{"id": 2, "name": "no-group-key"},
				},
				"total": 2,
			})
		case "/api/log/self/stat":
			tokenName := r.URL.Query().Get("token_name")
			group := r.URL.Query().Get("group")
			switch tokenName {
			case "prod-key":
				if group != "vip" {
					t.Fatalf("expected group=vip for prod-key, got %q", group)
				}
				writeJSON(w, map[string]any{"data": map[string]any{"quota": 250000}})
			case "no-group-key":
				if group != "" {
					t.Fatalf("expected no group param for ungrouped token, got %q", group)
				}
				writeJSON(w, map[string]any{"data": map[string]any{"quota": 0}})
			default:
				t.Fatalf("unexpected token_name: %s", tokenName)
			}
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "1", QuotaPerUnit: 100000}

	stats, err := service.FetchKeyUsageToday(session, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 1 {
		t.Fatalf("expected 1 key with nonzero cost, got %d: %+v", len(stats), stats)
	}
	if stats[0].KeyName != "prod-key" || stats[0].GroupName != "vip" {
		t.Fatalf("unexpected stat: %+v", stats[0])
	}
	if stats[0].TodayAmount != 2.5 {
		t.Errorf("todayAmount = %.4f, want 2.5000 (250000/100000 quota conversion)", stats[0].TodayAmount)
	}
}

func TestFetchKeyUsageToday_Sub2APIRetriesTransientKeyStatsFailure(t *testing.T) {
	var statsCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/keys":
			writeJSON(w, map[string]any{"data": []map[string]any{{
				"id": 1, "name": "retry-key", "group": map[string]any{"name": "vip"},
			}}, "total": 1})
		case "/api/v1/usage/stats":
			if statsCalls.Add(1) == 1 {
				http.Error(w, "temporary upstream failure", http.StatusBadGateway)
				return
			}
			writeJSON(w, map[string]any{"data": map[string]any{"total_actual_cost": 12.5}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	stats, err := NewPlatformService(NewHTTPClient(server.Client())).FetchKeyUsageToday(
		Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, nil)
	if err != nil {
		t.Fatalf("FetchKeyUsageToday() error: %v", err)
	}
	if statsCalls.Load() != 2 {
		t.Fatalf("usage stats attempts = %d, want 2", statsCalls.Load())
	}
	if len(stats) != 1 || stats[0].TodayAmount != 12.5 {
		t.Fatalf("stats = %+v, want recovered key usage", stats)
	}
}

func TestFetchKeyUsageToday_NewAPIRetriesTransientTokenStatsFailure(t *testing.T) {
	var statsCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/token/":
			writeJSON(w, map[string]any{"data": []map[string]any{{
				"id": 1, "name": "retry-token", "group": "vip",
			}}, "total": 1})
		case "/api/log/self/stat":
			if statsCalls.Add(1) == 1 {
				http.Error(w, "temporary upstream failure", http.StatusBadGateway)
				return
			}
			writeJSON(w, map[string]any{"data": map[string]any{"quota": 250000}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	stats, err := NewPlatformService(NewHTTPClient(server.Client())).FetchKeyUsageToday(
		Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "1", QuotaPerUnit: 100000}, nil)
	if err != nil {
		t.Fatalf("FetchKeyUsageToday() error: %v", err)
	}
	if statsCalls.Load() != 2 {
		t.Fatalf("usage stats attempts = %d, want 2", statsCalls.Load())
	}
	if len(stats) != 1 || stats[0].TodayAmount != 2.5 {
		t.Fatalf("stats = %+v, want recovered token usage", stats)
	}
}

func TestFetchKeyUsageToday_DoesNotRetryUnauthorizedKeyStats(t *testing.T) {
	var statsCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/keys":
			writeJSON(w, map[string]any{"data": []map[string]any{{
				"id": 1, "name": "private-key", "group": map[string]any{"name": "vip"},
			}}, "total": 1})
		case "/api/v1/usage/stats":
			statsCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	_, err := NewPlatformService(NewHTTPClient(server.Client())).FetchKeyUsageToday(
		Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, nil)
	if err == nil {
		t.Fatal("expected unauthorized key stats to fail")
	}
	if statsCalls.Load() != 1 {
		t.Fatalf("usage stats attempts = %d, want 1 for unauthorized response", statsCalls.Load())
	}
}

func TestFetchKeyUsageToday_StopsAfterRetryBudget(t *testing.T) {
	var statsCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/keys":
			writeJSON(w, map[string]any{"data": []map[string]any{{
				"id": 1, "name": "unavailable-key", "group": map[string]any{"name": "vip"},
			}}, "total": 1})
		case "/api/v1/usage/stats":
			statsCalls.Add(1)
			http.Error(w, "temporary upstream failure", http.StatusBadGateway)
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	_, err := NewPlatformService(NewHTTPClient(server.Client())).FetchKeyUsageToday(
		Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, nil)
	if err == nil {
		t.Fatal("expected persistent key stats failure")
	}
	if statsCalls.Load() != keyUsageRequestAttempts {
		t.Fatalf("usage stats attempts = %d, want retry budget %d", statsCalls.Load(), keyUsageRequestAttempts)
	}
}

// TestFetchKeyUsageToday_UnsupportedPlatform 验证未知平台会话直接返回错误，而不是静默返回空结果。
func TestFetchKeyUsageToday_UnsupportedPlatform(t *testing.T) {
	service := NewPlatformService(NewHTTPClient(http.DefaultClient))
	_, err := service.FetchKeyUsageToday(Session{Platform: PlatformAuto}, nil)
	if err == nil {
		t.Fatal("expected error for unsupported platform, got nil")
	}
}
