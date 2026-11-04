package upstream

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
)

// 调价影响估算：按分组查询自己账号在上游一段时间内的消费，只读。
func TestFetchSelfGroupConsume(t *testing.T) {
	var mu sync.Mutex
	requests := []*url.URL{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL)
		mu.Unlock()
		switch r.URL.Path {
		case "/api/v1/usage/stats":
			writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"total_actual_cost": 12.5}})
		case "/api/log/self/stat":
			writeJSON(w, map[string]any{"success": true, "data": map[string]any{"quota": 1_000_000}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := NewPlatformService(NewHTTPClient(server.Client()))

	sub2 := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}
	amount, err := service.FetchSelfGroupConsume(sub2, GroupInfo{ID: "5", Name: "claude"}, "2026-10-27", "2026-11-02", "Asia/Shanghai")
	if err != nil || amount != 12.5 {
		t.Fatalf("sub2api group consume: %v %v", amount, err)
	}
	if _, err := service.FetchSelfGroupConsume(sub2, GroupInfo{ID: "claude", Name: "claude"}, "2026-10-27", "2026-11-02", "Asia/Shanghai"); err == nil {
		t.Fatal("sub2api needs a numeric group id")
	}

	newAPI := Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "token", UserID: "7", QuotaPerUnit: 500_000}
	amount, err = service.FetchSelfGroupConsume(newAPI, GroupInfo{ID: "vip", Name: "vip"}, "2026-10-27", "2026-11-02", "Asia/Shanghai")
	if err != nil || amount != 2 {
		t.Fatalf("new-api group consume: %v %v", amount, err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(requests) != 2 {
		t.Fatalf("invalid groups must not be queried: %v", requests)
	}
	query := requests[0].Query()
	if query.Get("group_id") != "5" || query.Get("start_date") != "2026-10-27" || query.Get("end_date") != "2026-11-02" || query.Get("timezone") != "Asia/Shanghai" {
		t.Fatalf("unexpected sub2api query: %v", query)
	}
	query = requests[1].Query()
	// 2026-10-27 00:00 ~ 2026-11-02 23:59:59（北京时间）。
	if query.Get("group") != "vip" || query.Get("type") != "2" || query.Get("start_timestamp") != "1793030400" || query.Get("end_timestamp") != "1793635199" {
		t.Fatalf("unexpected new-api query: %v", query)
	}
}
