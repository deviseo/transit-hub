package dashboard

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

// TestSnapshotAllUsesFinalizeBusinessDateNotCachedCosts 验证 snapshotAll 通过 finalizeBusinessDate
// 查询成本，而不是读取 TodayConsume 缓存；站点成本采集失败时不写快照。
func TestSnapshotAllUsesFinalizeBusinessDateNotCachedCosts(t *testing.T) {
	fetchErr := errors.New("upstream unavailable")
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	store.activeSessions = []ActiveSessionRef{{UserID: "user-1", AdminAccountID: "account-1"}}

	// FetchSiteCostsForDate 返回错误 → finalizeBusinessDate 应该 return err，不写快照。
	upstreams := &fakeUpstreamLister{
		siteCostErr: fetchErr,
		// cachedSites 有数据，但新路径不应该读取它。
		cachedSites: []upstream.Response{{
			Status:       upstream.StatusConnected,
			RechargeRate: 1,
			Metrics:      upstream.Metrics{TodayConsume: upstream.MetricValue{Value: ptrFloat64(99.0)}},
		}},
	}
	repo := &fakeMetricsRepository{}
	service := NewMetricsService(
		store,
		&fakePlatformClient{usageStats: 30},
		upstreams,
		repo,
		nil,
	)

	service.snapshotAll(context.Background())

	if len(repo.snapshots) != 0 {
		t.Fatalf("snapshotAll 不应写快照：FetchSiteCostsForDate 失败时不得写 %d 个快照", len(repo.snapshots))
	}
}

// TestFinalizeBusinessDateErrorsWhenSiteCostFetchFails 验证成本采集失败时不写快照。
func TestFinalizeBusinessDateErrorsWhenSiteCostFetchFails(t *testing.T) {
	fetchErr := errors.New("redis down")
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})

	repo := &fakeMetricsRepository{}
	upstreams := &fakeUpstreamLister{siteCostErr: fetchErr}
	service := NewMetricsService(
		store,
		&fakePlatformClient{usageStats: 10},
		upstreams,
		repo,
		&fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}},
	)

	ref := ActiveSessionRef{UserID: "user-1", AdminAccountID: "account-1"}
	err := service.finalizeBusinessDate(context.Background(), ref, "2026-08-01", SnapshotSourceDatedQuery)
	if err == nil {
		t.Fatal("FetchSiteCostsForDate 失败时 finalizeBusinessDate 应返回错误")
	}
	if len(repo.snapshots) != 0 {
		t.Fatalf("成本采集失败时不得写 %d 个快照", len(repo.snapshots))
	}
}

// TestFinalizeBusinessDateSuccessWritesFinalSnapshot 验证全站点成功时写 final 快照。
func TestFinalizeBusinessDateSuccessWritesFinalSnapshot(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})

	cost := 5.0
	upstreams := &fakeUpstreamLister{
		siteCostResults: []upstream.SiteCostForDateResult{{
			SiteID:       "site-1",
			SiteName:     "test",
			Platform:     upstream.PlatformSub2API,
			RechargeRate: 1.0,
			RawCost:      cost,
			Meta:         upstream.CostFetchMeta{Source: "account_level"},
		}},
	}
	repo := &fakeMetricsRepository{}
	service := NewMetricsService(
		store,
		&fakePlatformClient{usageStats: 20},
		upstreams,
		repo,
		&fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}},
	)

	ref := ActiveSessionRef{UserID: "user-1", AdminAccountID: "account-1"}
	err := service.finalizeBusinessDate(context.Background(), ref, "2026-08-01", SnapshotSourceDatedQuery)
	if err != nil {
		t.Fatalf("finalizeBusinessDate() 意外错误: %v", err)
	}
	if len(repo.snapshots) != 1 {
		t.Fatalf("期望写入 1 个快照，实际 %d 个", len(repo.snapshots))
	}
	snap := repo.snapshots[0]
	if snap.SettlementStatus != SettlementStatusFinal {
		t.Fatalf("全站点成功应写 final，实际 %s", snap.SettlementStatus)
	}
	if snap.SnapshotSource != SnapshotSourceDatedQuery {
		t.Fatalf("source 应为 dated_query，实际 %s", snap.SnapshotSource)
	}
}
