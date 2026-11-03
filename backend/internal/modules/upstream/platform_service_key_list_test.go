package upstream

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
)

// pagedKeysServer 记录每次请求的页码，便于断言分页次数。
type pagedKeysServer struct {
	mu    sync.Mutex
	pages []string
}

func (p *pagedKeysServer) record(page string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pages = append(p.pages, page)
}

func (p *pagedKeysServer) requestedPages() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.pages...)
}

func pageSlice(total int, page int, pageSize int) (int, int) {
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return start, end
}

// TestListSub2APIKeys_FetchesAllPages 验证 sub2api key 列表会翻完所有页（以前只取第 1 页的 100 条）。
func TestListSub2APIKeys_FetchesAllPages(t *testing.T) {
	const total = 150
	tracker := &pagedKeysServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/keys" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("page_size") != "100" || query.Get("sort_by") != "created_at" || query.Get("sort_order") != "desc" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		tracker.record(query.Get("page"))
		page, _ := strconv.Atoi(query.Get("page"))
		start, end := pageSlice(total, page, 100)
		items := make([]map[string]any, 0, end-start)
		for i := start; i < end; i++ {
			items = append(items, map[string]any{
				"id": i + 1, "name": "key-" + strconv.Itoa(i+1), "key": "sk-" + strconv.Itoa(i+1),
				"status": "active", "group_id": 7, "group": map[string]any{"id": 7, "name": "vip"},
			})
		}
		writeJSON(w, map[string]any{"code": 0, "data": map[string]any{
			"items": items, "total": total, "page": page, "page_size": 100, "pages": 2,
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	keys, err := service.ListSub2APIKeys(Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(keys) != total {
		t.Fatalf("expected %d keys across pages, got %d", total, len(keys))
	}
	if pages := tracker.requestedPages(); len(pages) != 2 || pages[0] != "1" || pages[1] != "2" {
		t.Fatalf("expected pages [1 2], got %v", pages)
	}
	last := keys[total-1]
	if last.ID != "150" || last.Key != "sk-150" || last.GroupID != "7" || last.GroupName != "vip" || last.Status != "active" {
		t.Fatalf("unexpected mapping for key on page 2: %+v", last)
	}
}

// TestListNewAPITokens_FetchesAllPagesWithoutTotal 验证旧版 new-api（data 直接是数组、没有 total）
// 也能翻完所有页：以空页作为结束信号，而不是在第一页就停止。
func TestListNewAPITokens_FetchesAllPagesWithoutTotal(t *testing.T) {
	const total = 120
	tracker := &pagedKeysServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/token/" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		tracker.record(r.URL.Query().Get("p"))
		page, _ := strconv.Atoi(r.URL.Query().Get("p"))
		start, end := pageSlice(total, page, 100)
		items := make([]map[string]any, 0, end-start)
		for i := start; i < end; i++ {
			items = append(items, map[string]any{"id": i + 1, "name": "token-" + strconv.Itoa(i+1), "status": 1, "group": "default"})
		}
		writeJSON(w, map[string]any{"success": true, "data": items})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	tokens, err := service.ListNewAPITokens(Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != total {
		t.Fatalf("expected %d tokens, got %d", total, len(tokens))
	}
	if pages := tracker.requestedPages(); len(pages) != 3 {
		t.Fatalf("expected pages 1,2 plus one empty page as the stop signal, got %v", pages)
	}
	if tokens[0].Status != "1" || tokens[0].GroupName != "default" || tokens[0].GroupID != "default" {
		t.Fatalf("unexpected token mapping: %+v", tokens[0])
	}
}

// TestListNewAPITokens_StopsAfterTotalWithoutExtraRequest 验证新版 new-api 返回 data.items + total 时，
// 拿满 total 即停止，不再多发一次请求。
func TestListNewAPITokens_StopsAfterTotalWithoutExtraRequest(t *testing.T) {
	tracker := &pagedKeysServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracker.record(r.URL.Query().Get("p"))
		items := make([]map[string]any, 0, 100)
		for i := 0; i < 100; i++ {
			items = append(items, map[string]any{"id": i + 1, "name": "token", "status": 1, "group": "vip"})
		}
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"items": items, "total": 100, "page": 1, "page_size": 100}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	tokens, err := service.ListNewAPITokens(Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "nap_token", TokenType: "Bearer"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 100 {
		t.Fatalf("expected 100 tokens, got %d", len(tokens))
	}
	if pages := tracker.requestedPages(); len(pages) != 1 {
		t.Fatalf("expected a single request when total is satisfied, got %v", pages)
	}
}

// TestListNewAPITokens_StopsWhenUpstreamIgnoresPaging 验证上游忽略分页参数、每页都返回同一批数据时，
// 第二页没有新 ID 就停止，不会重复拉取到上限，也不会返回重复的 token。
func TestListNewAPITokens_StopsWhenUpstreamIgnoresPaging(t *testing.T) {
	tracker := &pagedKeysServer{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tracker.record(r.URL.Query().Get("p"))
		items := make([]map[string]any, 0, 30)
		for i := 0; i < 30; i++ {
			items = append(items, map[string]any{"id": i + 1, "name": "token", "status": 1})
		}
		writeJSON(w, map[string]any{"success": true, "data": items})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	tokens, err := service.ListNewAPITokens(Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=abc", UserID: "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tokens) != 30 {
		t.Fatalf("expected 30 unique tokens, got %d", len(tokens))
	}
	if pages := tracker.requestedPages(); len(pages) != 2 {
		t.Fatalf("expected to stop after a page with no new IDs, got %v", pages)
	}
}
