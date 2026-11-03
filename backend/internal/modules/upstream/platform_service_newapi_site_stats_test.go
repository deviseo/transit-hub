package upstream

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestFetchAdminUsageStats_NewAPIUsesSiteWideLogStat 验证本站（admin 工作区）营收走 new-api 管理员
// 全站统计接口 /api/log/stat，而不是只统计当前登录用户的 /api/log/self/stat；
// 时间窗口按服务进程本地时区覆盖起始日 00:00:00 到结束日 23:59:59。
func TestFetchAdminUsageStats_NewAPIUsesSiteWideLogStat(t *testing.T) {
	var mu sync.Mutex
	var gotPath string
	var gotQuery map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		mu.Lock()
		gotPath = r.URL.Path
		gotQuery = map[string]string{
			"type":            query.Get("type"),
			"start_timestamp": query.Get("start_timestamp"),
			"end_timestamp":   query.Get("end_timestamp"),
			"group":           query.Get("group"),
		}
		mu.Unlock()
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"quota": 250000, "rpm": 0, "tpm": 0}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "token", TokenType: "Bearer", UserID: "1", QuotaPerUnit: 500000}

	amount, err := service.FetchAdminUsageStats(session, "2026-11-01", "2026-11-02")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amount != 0.5 {
		t.Fatalf("amount = %v, want 0.5 (250000 / 500000)", amount)
	}

	mu.Lock()
	defer mu.Unlock()
	if gotPath != "/api/log/stat" {
		t.Fatalf("revenue must use site-wide /api/log/stat, got %s", gotPath)
	}
	wantStart, _ := time.ParseInLocation("2006-01-02", "2026-11-01", time.Local)
	wantEnd, _ := time.ParseInLocation("2006-01-02", "2026-11-03", time.Local)
	if gotQuery["type"] != "2" {
		t.Fatalf("type = %q, want consume logs (2)", gotQuery["type"])
	}
	if gotQuery["start_timestamp"] != strconv.FormatInt(wantStart.Unix(), 10) {
		t.Fatalf("start_timestamp = %s, want %d", gotQuery["start_timestamp"], wantStart.Unix())
	}
	if gotQuery["end_timestamp"] != strconv.FormatInt(wantEnd.Unix()-1, 10) {
		t.Fatalf("end_timestamp = %s, want %d", gotQuery["end_timestamp"], wantEnd.Unix()-1)
	}
	if gotQuery["group"] != "" {
		t.Fatalf("site revenue must not filter by group, got %q", gotQuery["group"])
	}
}

// TestFetchNewAPIGroupDailyStats_UpstreamKeepsSelfScope 锁定上游口径：上游站点用普通用户会话登录，
// 「我在上游各分组花了多少」只能用 /api/log/self/stat，不能被本站的全站口径修复误改。
func TestFetchNewAPIGroupDailyStats_UpstreamKeepsSelfScope(t *testing.T) {
	var mu sync.Mutex
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		writeJSON(w, map[string]any{"data": map[string]any{"quota": 100000}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "7", QuotaPerUnit: 100000}

	stats, err := service.FetchNewAPIGroupDailyStats(session, []GroupInfo{{Name: "default"}, {Name: defaultDisplay}, {Name: " "}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(stats) != 1 || stats[0].GroupName != "default" || stats[0].TodayActualCost != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/api/log/self/stat" {
		t.Fatalf("upstream group stats must stay self-scoped, got paths %v", paths)
	}
}

// TestLocalDateRangeUnix 验证日期边界按给定时区计算：北京时间 2026-11-02 对应
// UTC 2026-11-01 16:00:00 到 2026-11-02 15:59:59，而不是按 UTC 切日。
func TestLocalDateRangeUnix(t *testing.T) {
	shanghai := time.FixedZone("UTC+8", 8*60*60)

	start, end, err := localDateRangeUnix("2026-11-02", "2026-11-02", shanghai)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := time.Date(2026, 11, 1, 16, 0, 0, 0, time.UTC).Unix(); start != want {
		t.Fatalf("start = %d (%s), want %d", start, time.Unix(start, 0).UTC(), want)
	}
	if want := time.Date(2026, 11, 2, 15, 59, 59, 0, time.UTC).Unix(); end != want {
		t.Fatalf("end = %d (%s), want %d", end, time.Unix(end, 0).UTC(), want)
	}

	start, end, err = localDateRangeUnix("2026-10-30", "2026-11-01", shanghai)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := end - start + 1; got != 3*24*60*60 {
		t.Fatalf("multi-day window covers %d seconds, want exactly 3 days", got)
	}

	if _, _, err := localDateRangeUnix("2026/11/02", "2026-11-02", shanghai); err == nil {
		t.Fatal("expected an error for a malformed start date")
	}
}
