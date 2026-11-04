package upstream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"transithub/backend/internal/shared/bizclock"
)

// 按天统计的 sub2api 接口都要显式带上业务时区，不能依赖对方服务器或本进程的默认时区。
func TestSub2APIDailyQueriesSendBusinessTimezone(t *testing.T) {
	var mu sync.Mutex
	queries := map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		queries[r.URL.Path] = r.URL.RawQuery
		mu.Unlock()
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{"total_actual_cost": 1.5}})
	}))
	defer server.Close()
	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}

	if _, err := service.FetchSub2APIAdminUsageStats(session, "2026-11-02", "2026-11-02"); err != nil {
		t.Fatalf("admin usage stats: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	query := queries["/api/v1/admin/usage/stats"]
	if !strings.Contains(query, "start_date=2026-11-02") || !strings.Contains(query, "timezone=Asia%2FShanghai") {
		t.Fatalf("unexpected admin usage stats query %q", query)
	}
	if bizclock.Name != "Asia/Shanghai" {
		t.Fatalf("business timezone changed unexpectedly: %s", bizclock.Name)
	}
}
