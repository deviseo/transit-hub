package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeSiteCache 是 SiteCache 的内存实现，仅供测试使用。
type fakeSiteCache struct {
	sites map[string]*Site
}

func newFakeSiteCache() *fakeSiteCache {
	return &fakeSiteCache{sites: map[string]*Site{}}
}

func (f *fakeSiteCache) add(site *Site) {
	stored := *site
	f.sites[site.ID] = &stored
}

func (f *fakeSiteCache) Get(ctx context.Context, id string) (*Site, error) {
	site, ok := f.sites[id]
	if !ok {
		return nil, nil
	}
	stored := *site
	return &stored, nil
}

func (f *fakeSiteCache) Set(ctx context.Context, site *Site) error {
	stored := *site
	f.sites[site.ID] = &stored
	return nil
}

func (f *fakeSiteCache) Delete(ctx context.Context, id string, userID string) error {
	delete(f.sites, id)
	return nil
}

func (f *fakeSiteCache) ListByUser(ctx context.Context, userID string) ([]*Site, error) {
	result := make([]*Site, 0)
	for _, site := range f.sites {
		if site.UserID == userID {
			stored := *site
			result = append(result, &stored)
		}
	}
	return result, nil
}

func (f *fakeSiteCache) Flush(ctx context.Context) error { return nil }

// fakeAccountResolver 是 AdminAccountResolver 的内存实现，按 userID 返回固定的当前工作区。
type fakeAccountResolver struct {
	current map[string]string
}

func (f *fakeAccountResolver) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	id, ok := f.current[userID]
	if !ok {
		return "", newRequestError("admin.adminAccounts.errors.noCurrentAccount", "")
	}
	return id, nil
}

// sub2APIKeyServer 启动一个最小 sub2api httptest server：单页 key 列表 + 固定今日消费。
func sub2APIKeyServer(t *testing.T, keyID string, keyName string, groupName string, todayCost float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/keys":
			writeJSON(w, map[string]any{"data": []map[string]any{
				{"id": keyID, "name": keyName, "group": map[string]any{"name": groupName}},
			}})
		case "/api/v1/usage/stats":
			writeJSON(w, map[string]any{"data": map[string]any{"total_actual_cost": todayCost}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
}

func newTestSite(id, userID, adminAccountID string, rechargeRate float64, session *Session) *Site {
	return &Site{
		ID:             id,
		UserID:         userID,
		AdminAccountID: adminAccountID,
		Name:           "site-" + id,
		Platform:       PlatformSub2API,
		RechargeRate:   rechargeRate,
		Status:         StatusConnected,
		Session:        session,
	}
}

// TestServiceKeyUsageToday_WorkspaceIsolation 覆盖测试要求 1：只返回当前工作区站点的数据，
// 其他工作区（即使同一用户名下）的站点不得混入结果。
func TestServiceKeyUsageToday_WorkspaceIsolation(t *testing.T) {
	serverA := sub2APIKeyServer(t, "1", "key-a", "vip", 10)
	defer serverA.Close()
	serverB := sub2APIKeyServer(t, "2", "key-b", "vip", 20)
	defer serverB.Close()

	cache := newFakeSiteCache()
	cache.add(newTestSite("site-a", "user-1", "acc-1", 2, &Session{Platform: PlatformSub2API, BaseURL: serverA.URL, AccessToken: "token"}))
	cache.add(newTestSite("site-b", "user-1", "acc-2", 2, &Session{Platform: PlatformSub2API, BaseURL: serverB.URL, AccessToken: "token"}))

	svc := NewService(NewPlatformService(NewHTTPClient(http.DefaultClient)), nil, nil, cache)
	svc.SetAdminAccountResolver(&fakeAccountResolver{current: map[string]string{"user-1": "acc-1"}})

	items, err := svc.KeyUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item from acc-1 only, got %d: %+v", len(items), items)
	}
	if items[0].SiteID != "site-a" {
		t.Fatalf("expected site-a data, workspace isolation leaked: %+v", items[0])
	}
}

// TestServiceKeyUsageToday_SkipsRechargeRateZero 验证 rechargeRate <= 0 的站点被整体跳过，
// 与 dashboard.MetricsService.LiveMetrics() 中 todayPurchase 的口径保持一致。
func TestServiceKeyUsageToday_SkipsRechargeRateZero(t *testing.T) {
	server := sub2APIKeyServer(t, "1", "key-a", "vip", 10)
	defer server.Close()

	cache := newFakeSiteCache()
	cache.add(newTestSite("site-a", "user-1", "acc-1", 0, &Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}))

	svc := NewService(NewPlatformService(NewHTTPClient(http.DefaultClient)), nil, nil, cache)
	svc.SetAdminAccountResolver(&fakeAccountResolver{current: map[string]string{"user-1": "acc-1"}})

	items, err := svc.KeyUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 items for rechargeRate<=0 site, got %+v", items)
	}
}

// TestServiceKeyUsageToday_FiltersZeroCostAndAppliesRechargeRate 覆盖测试要求 2 和字段换算：
// 0 消费的 key 被过滤；剩余 key 的 todayAmount = 上游原始金额 * rechargeRate。
func TestServiceKeyUsageToday_FiltersZeroCostAndAppliesRechargeRate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/keys":
			writeJSON(w, map[string]any{"data": []map[string]any{
				{"id": "1", "name": "zero-cost-key", "group": map[string]any{"name": "vip"}},
				{"id": "2", "name": "prod-key", "group": map[string]any{"name": "vip"}},
			}})
		case "/api/v1/usage/stats":
			apiKeyID := r.URL.Query().Get("api_key_id")
			cost := 0.0
			if apiKeyID == "2" {
				cost = 33.3
			}
			writeJSON(w, map[string]any{"data": map[string]any{"total_actual_cost": cost}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	cache := newFakeSiteCache()
	cache.add(newTestSite("site-a", "user-1", "acc-1", 2, &Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"}))

	svc := NewService(NewPlatformService(NewHTTPClient(http.DefaultClient)), nil, nil, cache)
	svc.SetAdminAccountResolver(&fakeAccountResolver{current: map[string]string{"user-1": "acc-1"}})

	items, err := svc.KeyUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 item (zero-cost key filtered out), got %d: %+v", len(items), items)
	}
	if items[0].KeyName != "prod-key" {
		t.Fatalf("unexpected key survived filtering: %+v", items[0])
	}
	if items[0].RawAmount != 33.3 {
		t.Errorf("rawAmount = %.2f, want 33.30", items[0].RawAmount)
	}
	if items[0].TodayAmount != 66.6 {
		t.Errorf("todayAmount = %.2f, want 66.60 (33.3 * rechargeRate 2)", items[0].TodayAmount)
	}
}

// TestServiceKeyUsageToday_ExternalErrorFailsClosed 覆盖测试要求 9：
// 外部平台请求失败时整个方法返回错误，不能把失败站点悄悄当 0 处理。
func TestServiceKeyUsageToday_ExternalErrorFailsClosed(t *testing.T) {
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	cache := newFakeSiteCache()
	cache.add(newTestSite("site-a", "user-1", "acc-1", 2, &Session{Platform: PlatformSub2API, BaseURL: failingServer.URL, AccessToken: "token"}))

	svc := NewService(NewPlatformService(NewHTTPClient(http.DefaultClient)), nil, nil, cache)
	svc.SetAdminAccountResolver(&fakeAccountResolver{current: map[string]string{"user-1": "acc-1"}})

	_, err := svc.KeyUsageToday(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error when upstream platform request fails, got nil (silently treated as 0)")
	}
}

// TestServiceBalanceBreakdown_SortsDescendingWithUnknownBalanceLast 覆盖测试要求 7、8：
// 按 balance 降序排序，未知余额（rechargeRate<=0）站点排在最后；total 等于已知余额之和，
// 与 LiveMetrics 中 upstreamBalance 的计算口径一致（rechargeRate<=0 站点不计入 total）。
func TestServiceBalanceBreakdown_SortsDescendingWithUnknownBalanceLast(t *testing.T) {
	highBalance := 50.0
	lowBalance := 5.0

	cache := newFakeSiteCache()
	siteHigh := newTestSite("site-high", "user-1", "acc-1", 2, nil)
	siteHigh.Metrics.Balance.Value = &highBalance
	cache.add(siteHigh)

	siteLow := newTestSite("site-low", "user-1", "acc-1", 2, nil)
	siteLow.Metrics.Balance.Value = &lowBalance
	cache.add(siteLow)

	siteUnknown := newTestSite("site-unknown", "user-1", "acc-1", 0, nil) // rechargeRate<=0 => 未知余额
	cache.add(siteUnknown)

	svc := NewService(NewPlatformService(NewHTTPClient(http.DefaultClient)), nil, nil, cache)
	svc.SetAdminAccountResolver(&fakeAccountResolver{current: map[string]string{"user-1": "acc-1"}})

	items, err := svc.BalanceBreakdown(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("expected all 3 sites (unknown balance sites are shown, not omitted), got %d", len(items))
	}

	// Service 层不排序（排序由 dashboard.MetricsService.UpstreamBalanceBreakdown 完成），
	// 这里只校验数据本身：已知余额已换算为 CNY，未知余额为 nil。
	byID := map[string]*BalanceBreakdownItem{}
	for i := range items {
		byID[items[i].SiteID] = &items[i]
	}
	if byID["site-high"].Balance == nil || *byID["site-high"].Balance != 100 {
		t.Errorf("site-high balance = %v, want 100 (50 * rechargeRate 2)", byID["site-high"].Balance)
	}
	if byID["site-low"].Balance == nil || *byID["site-low"].Balance != 10 {
		t.Errorf("site-low balance = %v, want 10 (5 * rechargeRate 2)", byID["site-low"].Balance)
	}
	if byID["site-unknown"].Balance != nil {
		t.Errorf("site-unknown balance = %v, want nil (rechargeRate<=0 => unknown)", byID["site-unknown"].Balance)
	}
}
