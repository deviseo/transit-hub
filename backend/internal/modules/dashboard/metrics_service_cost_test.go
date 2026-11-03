package dashboard

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// 仪表盘「今日成本 / 营收口径 / 历史定稿」测试，数据均为合成数据。

type fakeMetricsStore struct {
	pricing     map[string]float64
	snapshots   map[string]DailySnapshot // key: userID|accountID|date
	finalized   map[string]bool
	keptBalance map[string]bool
	upserts     []DailySnapshot
}

func newFakeMetricsStore() *fakeMetricsStore {
	return &fakeMetricsStore{pricing: map[string]float64{}, snapshots: map[string]DailySnapshot{}, finalized: map[string]bool{}, keptBalance: map[string]bool{}}
}

func snapshotKey(userID, adminAccountID string, date time.Time) string {
	return userID + "|" + adminAccountID + "|" + date.Format("2006-01-02")
}

func (f *fakeMetricsStore) Upsert(ctx context.Context, snapshot DailySnapshot) error {
	f.upserts = append(f.upserts, snapshot)
	key := snapshotKey(snapshot.UserID, snapshot.AdminAccountID, snapshot.Date)
	if !f.finalized[key] {
		f.snapshots[key] = snapshot
	}
	return nil
}

func (f *fakeMetricsStore) ListRange(ctx context.Context, userID, adminAccountID string, today time.Time, days int) ([]DailySnapshot, error) {
	return nil, nil
}

func (f *fakeMetricsStore) GetBalanceFilter(ctx context.Context, userID, adminAccountID string) (BalanceFilterConfig, error) {
	return BalanceFilterConfig{ExcludeAdmin: true, ExcludeBalances: []float64{}}, nil
}

func (f *fakeMetricsStore) SaveBalanceFilter(ctx context.Context, config BalanceFilterConfig) error {
	return nil
}

func (f *fakeMetricsStore) GetPricing(ctx context.Context, userID, adminAccountID string) (PricingConfig, error) {
	price, ok := f.pricing[userID+"|"+adminAccountID]
	if !ok {
		price = DefaultSiteUnitPrice
	}
	return PricingConfig{UserID: userID, AdminAccountID: adminAccountID, SiteUnitPrice: price}, nil
}

func (f *fakeMetricsStore) SavePricing(ctx context.Context, config PricingConfig) error {
	f.pricing[config.UserID+"|"+config.AdminAccountID] = config.SiteUnitPrice
	return nil
}

func (f *fakeMetricsStore) FinalizeSnapshot(ctx context.Context, snapshot DailySnapshot, keepBalances bool) error {
	key := snapshotKey(snapshot.UserID, snapshot.AdminAccountID, snapshot.Date)
	if existing, ok := f.snapshots[key]; ok && keepBalances {
		snapshot.SiteBalance = existing.SiteBalance
		snapshot.UpstreamBalance = existing.UpstreamBalance
	}
	f.snapshots[key] = snapshot
	f.finalized[key] = true
	f.keptBalance[key] = keepBalances
	return nil
}

func (f *fakeMetricsStore) UnfinalizedDates(ctx context.Context, userID, adminAccountID string, from time.Time, to time.Time) ([]time.Time, error) {
	dates := make([]time.Time, 0)
	for key, snapshot := range f.snapshots {
		if !strings.HasPrefix(key, userID+"|"+adminAccountID+"|") || f.finalized[key] {
			continue
		}
		if !snapshot.Date.Before(calendarDate(from)) && !snapshot.Date.After(calendarDate(to)) {
			dates = append(dates, snapshot.Date)
		}
	}
	return dates, nil
}

func (f *fakeMetricsStore) IsFinalized(ctx context.Context, userID, adminAccountID string, date time.Time) (bool, error) {
	return f.finalized[snapshotKey(userID, adminAccountID, date)], nil
}

func floatValue(value float64) *float64 { return &value }

func site(rate float64, todayConsume float64, balance float64, lastSyncedAt *int64) upstream.Response {
	return upstream.Response{
		RechargeRate: rate,
		Metrics:      upstream.Metrics{TodayConsume: upstream.MetricValue{Value: floatValue(todayConsume)}, Balance: upstream.MetricValue{Value: floatValue(balance)}},
		LastSyncedAt: lastSyncedAt,
	}
}

func millis(t time.Time) *int64 {
	value := t.UnixMilli()
	return &value
}

func TestSummarizeUpstreamCostOnlyCountsSitesSyncedToday(t *testing.T) {
	since := time.Date(2026, 11, 3, 0, 0, 0, 0, time.FixedZone("CST", 8*3600))
	early, late := since.Add(10*time.Minute), since.Add(5*time.Hour)
	cost := summarizeUpstreamCost([]upstream.Response{
		site(0.5, 10, 100, millis(late)),
		site(1, 4, 10, millis(early)),
		site(2, 99, 20, millis(since.Add(-time.Minute))), // 昨天同步的：今日消费是昨天的数
		site(1, 50, 30, nil),                             // 从未同步成功
		site(0, 1000, 1000, millis(late)),                // 没有充值倍率：不计入
	}, since)
	if cost.Purchase != 9 || cost.Sites != 2 || cost.Stale != 2 {
		t.Fatalf("unexpected cost: %+v", cost)
	}
	if cost.Balance != 50+10+40+30 {
		t.Fatalf("balances do not reset daily and must include stale sites: %v", cost.Balance)
	}
	if cost.OldestSync == nil || *cost.OldestSync != early.UnixMilli() {
		t.Fatalf("oldest sync must be the earliest counted site: %v", cost.OldestSync)
	}
}

func liveMetricsService(t *testing.T, now time.Time, upstreams *fakeUpstreamLister, store *fakeMetricsStore, platform *fakePlatformClient) *MetricsService {
	t.Helper()
	sessions := newFakeSessionStore()
	sessions.set("user-1", "account-1", AdminSession{Platform: "sub2api", Session: upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://site.example.test", AccessToken: "token"}})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	service := NewMetricsService(sessions, platform, upstreams, store, accounts)
	service.now = func() time.Time { return now }
	return service
}

func TestLiveMetricsConvertsRevenueAndSkipsStaleUpstreams(t *testing.T) {
	loc := dashboardLocation()
	now := time.Date(2026, 11, 3, 10, 30, 0, 0, loc)
	todayStart := time.Date(2026, 11, 3, 0, 0, 0, 0, loc)
	upstreams := &fakeUpstreamLister{sites: []upstream.Response{
		site(0.5, 10, 100, millis(now.Add(-time.Hour))),
		site(1, 99, 20, millis(todayStart.Add(-time.Hour))),
	}}
	store := newFakeMetricsStore()
	store.pricing["user-1|account-1"] = 0.8
	platform := &fakePlatformClient{usageStats: map[string]float64{"2026-11-03": 100}, siteBalance: 50}
	service := liveMetricsService(t, now, upstreams, store, platform)

	metrics, err := service.LiveMetrics(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("LiveMetrics: %v", err)
	}
	if metrics.TodayProfit != 80 || metrics.SiteBalance != 40 || metrics.SiteUnitPrice != 0.8 {
		t.Fatalf("revenue and site balance must be converted with the site unit price: %+v", metrics)
	}
	if metrics.TodayPurchase != 5 || metrics.CostSites != 1 || metrics.CostStaleSites != 1 || metrics.NetProfit != 75 || metrics.UpstreamBalance != 70 {
		t.Fatalf("a site not synced today must not contribute yesterday's consumption: %+v", metrics)
	}
	if metrics.CostSyncedAt == nil || *metrics.CostSyncedAt != now.Add(-time.Hour).UnixMilli() {
		t.Fatalf("unexpected cost sync time: %v", metrics.CostSyncedAt)
	}
	if len(upstreams.refreshSince) != 1 || !upstreams.refreshSince[0].Equal(todayStart) {
		t.Fatalf("stale sites must be refreshed from today's midnight: %v", upstreams.refreshSince)
	}
	if len(store.upserts) != 1 || store.upserts[0].TodayPurchase != 5 || store.upserts[0].TodayProfit != 80 {
		t.Fatalf("today's live snapshot must use the same numbers: %+v", store.upserts)
	}
}

func TestSavePricingValidatesInput(t *testing.T) {
	store := newFakeMetricsStore()
	service := liveMetricsService(t, time.Now(), &fakeUpstreamLister{}, store, &fakePlatformClient{})
	for _, price := range []float64{0, -1, math.NaN(), math.Inf(1), maxSiteUnitPrice + 1} {
		if _, err := service.SavePricing(context.Background(), "user-1", price); err == nil || err.Error() != ErrorInvalidPricing {
			t.Fatalf("price %v must be rejected, got %v", price, err)
		}
	}
	saved, err := service.SavePricing(context.Background(), "user-1", 0.75)
	if err != nil || saved.SiteUnitPrice != 0.75 {
		t.Fatalf("unexpected save: %+v %v", saved, err)
	}
	if loaded, err := service.GetPricing(context.Background(), "user-1"); err != nil || loaded.SiteUnitPrice != 0.75 {
		t.Fatalf("unexpected load: %+v %v", loaded, err)
	}
}

func finalizeService(t *testing.T, now time.Time, upstreams *fakeUpstreamLister, store *fakeMetricsStore, platform *fakePlatformClient) *MetricsService {
	t.Helper()
	service := liveMetricsService(t, now, upstreams, store, platform)
	service.store.(*fakeSessionStore).active = []ActiveSessionRef{{UserID: "user-1", AdminAccountID: "account-1"}}
	return service
}

func TestFinalizeReplacesYesterdaysPartialSnapshotWithFullDayData(t *testing.T) {
	loc := dashboardLocation()
	now := time.Date(2026, 11, 3, 0, 1, 0, 0, loc)
	yesterday := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	store := newFakeMetricsStore()
	store.pricing["user-1|account-1"] = 0.9
	store.snapshots[snapshotKey("user-1", "account-1", yesterday)] = DailySnapshot{UserID: "user-1", AdminAccountID: "account-1", Date: yesterday, TodayProfit: 10, TodayPurchase: 3, SiteBalance: 111, UpstreamBalance: 222}
	upstreams := &fakeUpstreamLister{
		sites:   []upstream.Response{site(1, 0, 300, millis(now))},
		consume: map[string]upstream.DailyConsume{"2026-11-02": {Total: 60, Sites: 2}},
	}
	platform := &fakePlatformClient{usageStats: map[string]float64{"2026-11-02": 200}, siteBalance: 1000}
	service := finalizeService(t, now, upstreams, store, platform)

	service.finalizeRecent(context.Background())
	key := snapshotKey("user-1", "account-1", yesterday)
	got := store.snapshots[key]
	if !store.finalized[key] || got.TodayProfit != 180 || got.TodayPurchase != 60 || got.NetProfit != 120 {
		t.Fatalf("yesterday must be finalized with full-day revenue (×0.9) and cost: %+v", got)
	}
	if store.keptBalance[key] || got.SiteBalance != 900 || got.UpstreamBalance != 300 {
		t.Fatalf("right after midnight the current balances are yesterday's closing balances: %+v kept=%v", got, store.keptBalance[key])
	}
	if len(upstreams.consumeCalled) != 1 || upstreams.consumeCalled[0] != "2026-11-02@Asia/Shanghai" {
		t.Fatalf("unexpected consume queries: %v", upstreams.consumeCalled)
	}

	// 已定稿的日期不会被白天的实时数覆盖，也不会被重复定稿。
	_ = store.Upsert(context.Background(), DailySnapshot{UserID: "user-1", AdminAccountID: "account-1", Date: yesterday, TodayProfit: 1})
	service.finalizeRecent(context.Background())
	if store.snapshots[key].TodayProfit != 180 || len(upstreams.consumeCalled) != 1 {
		t.Fatalf("finalized days must stay untouched: %+v calls=%v", store.snapshots[key], upstreams.consumeCalled)
	}
}

func TestFinalizeRetriesRecentDaysWithFailedSitesButSettlesOlderOnes(t *testing.T) {
	loc := dashboardLocation()
	now := time.Date(2026, 11, 3, 9, 0, 0, 0, loc)
	yesterday := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	store := newFakeMetricsStore()
	store.snapshots[snapshotKey("user-1", "account-1", older)] = DailySnapshot{UserID: "user-1", AdminAccountID: "account-1", Date: older, TodayProfit: 5, SiteBalance: 70, UpstreamBalance: 80}
	upstreams := &fakeUpstreamLister{consume: map[string]upstream.DailyConsume{
		"2026-11-02": {Total: 10, Sites: 1, Failed: 1},
		"2026-10-29": {Total: 20, Sites: 1, Failed: 1},
	}}
	platform := &fakePlatformClient{usageStats: map[string]float64{"2026-11-02": 50, "2026-10-29": 40}, siteBalance: 999}
	service := finalizeService(t, now, upstreams, store, platform)

	service.finalizeRecent(context.Background())
	if store.finalized[snapshotKey("user-1", "account-1", yesterday)] {
		t.Fatal("a recent day with failed sites must be retried later instead of being finalized")
	}
	olderKey := snapshotKey("user-1", "account-1", older)
	got := store.snapshots[olderKey]
	if !store.finalized[olderKey] || got.TodayProfit != 40 || got.TodayPurchase != 20 {
		t.Fatalf("older days are finalized with the data that is available: %+v", got)
	}
	if !store.keptBalance[olderKey] || got.SiteBalance != 70 || got.UpstreamBalance != 80 {
		t.Fatalf("late finalization must keep the balances recorded on that day: %+v", got)
	}

	upstreams.consumeErr = errors.New("boom")
	upstreams.consume["2026-11-02"] = upstream.DailyConsume{Total: 10, Sites: 1}
	service.finalizeRecent(context.Background())
	if store.finalized[snapshotKey("user-1", "account-1", yesterday)] {
		t.Fatal("errors must not finalize a day")
	}
}
