package upstream

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// 利润分析只读接口测试：用本地假站点返回合成数据。

func TestListOwnSiteRoutesSub2API(t *testing.T) {
	var mu sync.Mutex
	pages := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/admin/groups":
			writeJSON(w, map[string]any{"code": 0, "data": []any{
				map[string]any{"id": 1, "name": "claude-pro"},
				map[string]any{"id": 2, "name": "gpt-plus"},
			}})
		case "/api/v1/admin/accounts":
			mu.Lock()
			pages = append(pages, r.URL.Query().Get("page"))
			mu.Unlock()
			if r.URL.Query().Get("page") == "1" {
				writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"total": 3, "items": []any{
					map[string]any{"id": 11, "name": "relay-a", "type": "apikey", "status": "active",
						"credentials": map[string]any{"base_url": " https://api.relay-a.example.test/v1 "},
						"groups":      []any{map[string]any{"id": 1, "name": "claude-pro"}}},
					map[string]any{"id": 12, "name": "oauth", "type": "oauth", "status": "active",
						"credentials": map[string]any{}, "group_ids": []any{1.0, 2.0}},
				}}})
				return
			}
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"total": 3, "items": []any{
				map[string]any{"id": 13, "name": "relay-b", "type": "apikey", "status": "disabled",
					"credentials":    map[string]any{"endpoint": "https://relay-b.example.test"},
					"account_groups": []any{map[string]any{"group_id": 2}}},
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewPlatformService(NewHTTPClient(server.Client()))

	routes, err := service.ListOwnSiteRoutes(Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatalf("list routes: %v", err)
	}
	if len(routes) != 3 {
		t.Fatalf("expected 3 routes, got %+v", routes)
	}
	if routes[0].ID != "11" || routes[0].BaseURL != "https://api.relay-a.example.test/v1" || strings.Join(routes[0].Groups, ",") != "claude-pro" || routes[0].Kind != "apikey" {
		t.Fatalf("unexpected first route: %+v", routes[0])
	}
	if routes[1].BaseURL != "" || strings.Join(routes[1].Groups, ",") != "claude-pro,gpt-plus" {
		t.Fatalf("group ids must be resolved to names: %+v", routes[1])
	}
	if routes[2].BaseURL != "https://relay-b.example.test" || strings.Join(routes[2].Groups, ",") != "gpt-plus" || routes[2].Status != "disabled" {
		t.Fatalf("unexpected third route: %+v", routes[2])
	}
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(pages, ",") != "1,2" {
		t.Fatalf("pagination must stop at the reported total: %v", pages)
	}
}

func TestListOwnSiteRoutesNewAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/channel/" {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(r.URL.RawQuery, "key") {
			t.Errorf("channel keys must never be requested: %s", r.URL.RawQuery)
		}
		// 旧版本忽略分页参数，每页都返回同样的数据：第二页没有新增时必须停止。
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"items": []any{
			map[string]any{"id": 7, "name": "openai-relay", "type": 1, "status": 1, "base_url": "https://up.example.test", "group": "default, vip"},
			map[string]any{"id": 8, "name": "official", "type": 14, "status": 2, "base_url": "", "group": "default"},
		}}})
	}))
	defer server.Close()
	service := NewPlatformService(NewHTTPClient(server.Client()))

	routes, err := service.ListOwnSiteRoutes(Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "nap_token"})
	if err != nil || len(routes) != 2 {
		t.Fatalf("list routes: %+v %v", routes, err)
	}
	if routes[0].ID != "7" || routes[0].Kind != "1" || strings.Join(routes[0].Groups, "|") != "default|vip" || routes[0].BaseURL != "https://up.example.test" {
		t.Fatalf("unexpected channel: %+v", routes[0])
	}
}

func TestFetchSub2APIAccountGroupRevenue(t *testing.T) {
	var query string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		writeJSON(w, map[string]any{"code": 0, "data": []any{
			map[string]any{"group_id": 1, "group_name": "claude-pro", "requests": 12, "actual_cost": 3.5, "account_cost": 1.2},
			map[string]any{"group_id": 2, "group_name": "gpt-plus", "requests": 0, "actual_cost": 0},
		}})
	}))
	defer server.Close()
	service := NewPlatformService(NewHTTPClient(server.Client()))

	rows, err := service.FetchSub2APIAccountGroupRevenue(Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}, "11", "2026-11-02")
	if err != nil || len(rows) != 1 || rows[0] != (RouteGroupRevenue{RouteID: "11", Group: "claude-pro", Revenue: 3.5, Requests: 12}) {
		t.Fatalf("unexpected rows: %+v %v", rows, err)
	}
	for _, want := range []string{"start_date=2026-11-02", "end_date=2026-11-02", "timezone=Asia%2FShanghai", "account_id=11"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query %q is missing %s", query, want)
		}
	}
}

func TestFetchNewAPIFlowRevenue(t *testing.T) {
	mode := "flow"
	var mu sync.Mutex
	queries := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		mu.Unlock()
		switch {
		case r.URL.Path == "/api/data/flow" && mode == "flow":
			writeJSON(w, map[string]any{"success": true, "data": []any{
				map[string]any{"user_id": 1, "use_group": "vip", "model_name": "a", "channel_id": 7, "count": 3, "quota": 500000},
				map[string]any{"user_id": 2, "use_group": "vip", "model_name": "b", "channel_id": 7, "count": 1, "quota": 250000},
				map[string]any{"user_id": 2, "use_group": "default", "model_name": "b", "channel_id": 8, "count": 2, "quota": 100000},
				map[string]any{"user_id": 3, "use_group": "default", "model_name": "c", "channel_id": 0, "count": 1, "quota": 50000},
			}})
		case r.URL.Path == "/api/data/flow" && mode == "html":
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<!doctype html><html></html>"))
		case r.URL.Path == "/api/log/stat":
			writeJSON(w, map[string]any{"success": true, "data": map[string]any{"quota": 1000000}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "nap_token", QuotaPerUnit: 500000}

	rows, err := service.FetchNewAPIFlowRevenue(session, 100, 200)
	if err != nil || len(rows) != 3 {
		t.Fatalf("flow: %+v %v", rows, err)
	}
	if rows[0] != (RouteGroupRevenue{RouteID: "7", Group: "vip", Revenue: 1.5, Requests: 4}) || rows[1].RouteID != "8" || rows[2].RouteID != "" || rows[2].Revenue != 0.1 {
		t.Fatalf("flow rows must be summed by channel and group: %+v", rows)
	}

	mode = "missing"
	if _, err := service.FetchNewAPIFlowRevenue(session, 100, 200); !errors.Is(err, ErrFlowUnsupported) {
		t.Fatalf("a missing endpoint must report ErrFlowUnsupported, got %v", err)
	}
	mode = "html"
	if _, err := service.FetchNewAPIFlowRevenue(session, 100, 200); !errors.Is(err, ErrFlowUnsupported) {
		t.Fatalf("an HTML fallback page must report ErrFlowUnsupported, got %v", err)
	}

	revenue, err := service.FetchNewAPIChannelRevenue(session, "7", "vip", 100, 200)
	if err != nil || revenue != 2 {
		t.Fatalf("channel revenue: %v %v", revenue, err)
	}
	mu.Lock()
	defer mu.Unlock()
	last := queries[len(queries)-1]
	for _, want := range []string{"/api/log/stat?", "type=2", "channel=7", "group=vip", "start_timestamp=100", "end_timestamp=200"} {
		if !strings.Contains(last, want) {
			t.Fatalf("log stat query %q is missing %s", last, want)
		}
	}
	if !strings.Contains(queries[0], "start_timestamp=100") || !strings.Contains(queries[0], "end_timestamp=200") {
		t.Fatalf("unexpected flow query %q", queries[0])
	}
}
