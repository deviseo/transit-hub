package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

type fakeMetricsRepository struct {
	snapshots            []DailySnapshot
	upsertErr            error
	listRangeSnapshots   []DailySnapshot
	listRangeErr         error
	listRangeBusinessDay string
	balanceFilter        BalanceFilterConfig
	balanceFilterErr     error
	saveBalanceFilterErr error
}

func (f *fakeMetricsRepository) Upsert(ctx context.Context, snapshot DailySnapshot) error {
	f.snapshots = append(f.snapshots, snapshot)
	return f.upsertErr
}

func (f *fakeMetricsRepository) ListRange(ctx context.Context, userID, adminAccountID string, days int, businessDate string) ([]DailySnapshot, error) {
	f.listRangeBusinessDay = businessDate
	return f.listRangeSnapshots, f.listRangeErr
}

func (f *fakeMetricsRepository) GetBalanceFilter(ctx context.Context, userID, adminAccountID string) (BalanceFilterConfig, error) {
	return f.balanceFilter, f.balanceFilterErr
}

func (f *fakeMetricsRepository) SaveBalanceFilter(ctx context.Context, config BalanceFilterConfig) error {
	return f.saveBalanceFilterErr
}

func newLiveMetricsTestService(platform *fakePlatformClient, upstreams *fakeUpstreamLister, metricsRepo *fakeMetricsRepository) *MetricsService {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	return NewMetricsService(store, platform, upstreams, metricsRepo, accounts)
}

func metricsResponseJSON(t *testing.T, response MetricsResponse) map[string]any {
	t.Helper()
	payload, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("json.Marshal() error: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error: %v", err)
	}
	return decoded
}

func ptrFloat64(value float64) *float64 {
	return &value
}

func TestLiveMetricsCostFailureDoesNotPersistSnapshot(t *testing.T) {
	errorKey := upstream.ErrorAuth
	staleCost := 10.0
	repo := &fakeMetricsRepository{}
	service := newLiveMetricsTestService(
		&fakePlatformClient{usageStats: 30},
		&fakeUpstreamLister{cachedSites: []upstream.Response{{
			Status: upstream.StatusError, ErrorKey: &errorKey, RechargeRate: 2,
			Metrics: upstream.Metrics{TodayConsume: upstream.MetricValue{Value: &staleCost}},
		}}},
		repo,
	)

	response, err := service.LiveMetrics(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("LiveMetrics() error = %v, want partial response", err)
	}
	decoded := metricsResponseJSON(t, response)
	if decoded["todayProfit"] != 30.0 {
		t.Fatalf("todayProfit = %#v, want 30", decoded["todayProfit"])
	}
	if decoded["todayPurchase"] != 0.0 || decoded["netProfit"] != 0.0 {
		t.Fatalf("cost failure fallback amounts: todayPurchase=%#v netProfit=%#v", decoded["todayPurchase"], decoded["netProfit"])
	}
	metricErrors, ok := decoded["metricErrors"].(map[string]any)
	if !ok || metricErrors["todayPurchase"] != upstream.ErrorAuth || metricErrors["netProfit"] != upstream.ErrorAuth {
		t.Fatalf("metricErrors = %#v, want cached cost reason on cost and net profit", decoded["metricErrors"])
	}
	if len(repo.snapshots) != 0 {
		t.Fatalf("cost failure persisted %d snapshot(s): %+v", len(repo.snapshots), repo.snapshots)
	}
}

func TestLiveMetricsUsesCachedCostsAndKeepsPartialSuccess(t *testing.T) {
	failedKey := upstream.ErrorAuth
	successCost := 3.0
	staleFailedCost := 99.0
	upstreams := &fakeUpstreamLister{cachedSites: []upstream.Response{
		{
			ID: "site-success", Status: upstream.StatusConnected, RechargeRate: 2,
			Metrics: upstream.Metrics{TodayConsume: upstream.MetricValue{Value: &successCost}},
		},
		{
			ID: "site-failure", Status: upstream.StatusError, ErrorKey: &failedKey, RechargeRate: 2,
			Metrics: upstream.Metrics{TodayConsume: upstream.MetricValue{Value: &staleFailedCost}},
		},
	}}
	repo := &fakeMetricsRepository{}
	service := newLiveMetricsTestService(&fakePlatformClient{usageStats: 30}, upstreams, repo)

	response, err := service.LiveMetrics(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("LiveMetrics() error = %v, want partial response", err)
	}
	if response.TodayPurchase != 6 || response.NetProfit != 24 {
		t.Fatalf("partial cached cost = %.2f, net profit = %.2f, want 6.00 and 24.00", response.TodayPurchase, response.NetProfit)
	}
	if upstreams.keyUsageCalls != 0 {
		t.Fatalf("LiveMetrics() called active upstream cost query %d time(s)", upstreams.keyUsageCalls)
	}
	if len(repo.snapshots) != 0 {
		t.Fatalf("partial cached cost persisted %d snapshot(s): %+v", len(repo.snapshots), repo.snapshots)
	}
}

func TestLiveMetricsAllCachedCostsUnavailableReturnsZeroAndError(t *testing.T) {
	errorKey := upstream.ErrorAuth
	staleCost := 99.0
	upstreams := &fakeUpstreamLister{cachedSites: []upstream.Response{
		{
			ID: "site-failure", Status: upstream.StatusError, ErrorKey: &errorKey, RechargeRate: 2,
			Metrics: upstream.Metrics{TodayConsume: upstream.MetricValue{Value: &staleCost}},
		},
	}}
	repo := &fakeMetricsRepository{}
	service := newLiveMetricsTestService(&fakePlatformClient{usageStats: 30}, upstreams, repo)

	response, err := service.LiveMetrics(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("LiveMetrics() error = %v, want partial response", err)
	}
	decoded := metricsResponseJSON(t, response)
	if decoded["todayPurchase"] != 0.0 || decoded["netProfit"] != 0.0 {
		t.Fatalf("all cached costs unavailable: todayPurchase=%#v netProfit=%#v", decoded["todayPurchase"], decoded["netProfit"])
	}
	metricErrors, ok := decoded["metricErrors"].(map[string]any)
	if !ok || metricErrors["todayPurchase"] != upstream.ErrorAuth || metricErrors["netProfit"] != upstream.ErrorAuth {
		t.Fatalf("metricErrors = %#v, want cached cost error on cost and net profit", decoded["metricErrors"])
	}
	if len(repo.snapshots) != 0 {
		t.Fatalf("all cached costs unavailable persisted %d snapshot(s): %+v", len(repo.snapshots), repo.snapshots)
	}
}

func TestLiveMetricsRevenueFailureDoesNotPersistSnapshot(t *testing.T) {
	revenueErr := errors.New("admin usage unavailable")
	repo := &fakeMetricsRepository{}
	service := newLiveMetricsTestService(
		&fakePlatformClient{usageStatsErr: revenueErr},
		&fakeUpstreamLister{cachedSites: []upstream.Response{{
			Status: upstream.StatusConnected, RechargeRate: 1,
			Metrics: upstream.Metrics{TodayConsume: upstream.MetricValue{Value: ptrFloat64(2.5)}},
		}}},
		repo,
	)

	response, err := service.LiveMetrics(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("LiveMetrics() error = %v, want partial response", err)
	}
	decoded := metricsResponseJSON(t, response)
	if decoded["todayPurchase"] != 2.5 {
		t.Fatalf("todayPurchase = %#v, want 2.5", decoded["todayPurchase"])
	}
	if decoded["todayProfit"] != 0.0 || decoded["netProfit"] != 0.0 {
		t.Fatalf("revenue failure fallback amounts: todayProfit=%#v netProfit=%#v", decoded["todayProfit"], decoded["netProfit"])
	}
	metricErrors, ok := decoded["metricErrors"].(map[string]any)
	if !ok || metricErrors["todayProfit"] != revenueErr.Error() || metricErrors["netProfit"] != revenueErr.Error() {
		t.Fatalf("metricErrors = %#v, want revenue reason on revenue and net profit", decoded["metricErrors"])
	}
	if len(repo.snapshots) != 0 {
		t.Fatalf("revenue failure persisted %d snapshot(s): %+v", len(repo.snapshots), repo.snapshots)
	}
}

func TestLiveMetricsSuccessPersistsSameDayAmounts(t *testing.T) {
	repo := &fakeMetricsRepository{}
	upstreams := &fakeUpstreamLister{cachedSites: []upstream.Response{{
		Status: upstream.StatusConnected, RechargeRate: 1,
		Metrics: upstream.Metrics{TodayConsume: upstream.MetricValue{Value: ptrFloat64(2.5)}},
	}}}
	platform := &fakePlatformClient{usageStats: 30}
	service := newLiveMetricsTestService(platform, upstreams, repo)

	response, err := service.LiveMetrics(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("LiveMetrics() error: %v", err)
	}
	decoded := metricsResponseJSON(t, response)
	if decoded["todayProfit"] != 30.0 || decoded["todayPurchase"] != 2.5 || decoded["netProfit"] != 27.5 {
		t.Fatalf("unexpected response: %#v", decoded)
	}
	if _, exists := decoded["metricErrors"]; exists {
		t.Fatalf("successful response contains metricErrors: %#v", decoded["metricErrors"])
	}
	if len(repo.snapshots) != 1 {
		t.Fatalf("persisted snapshots = %d, want 1", len(repo.snapshots))
	}
	if platform.capturedUsageStart != response.Date || platform.capturedUsageEnd != response.Date || upstreams.keyUsageCalls != 0 {
		t.Fatalf("date/query mismatch: response=%q revenue=%q..%q keyUsageCalls=%d", response.Date, platform.capturedUsageStart, platform.capturedUsageEnd, upstreams.keyUsageCalls)
	}
	persisted := repo.snapshots[0]
	if persisted.TodayProfit != 30 || persisted.TodayPurchase != 2.5 || persisted.NetProfit != 27.5 {
		t.Fatalf("unexpected persisted snapshot: %+v", persisted)
	}
}

func TestSnapshotAllRevenueFailureKeepsExistingSnapshotUntouched(t *testing.T) {
	revenueErr := errors.New("admin usage unavailable")
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	store.activeSessions = []ActiveSessionRef{{UserID: "user-1", AdminAccountID: "account-1"}}
	repo := &fakeMetricsRepository{}
	service := NewMetricsService(
		store,
		&fakePlatformClient{usageStatsErr: revenueErr},
		&fakeUpstreamLister{cachedSites: []upstream.Response{{
			Status: upstream.StatusConnected, RechargeRate: 1,
			Metrics: upstream.Metrics{TodayConsume: upstream.MetricValue{Value: ptrFloat64(2.5)}},
		}}},
		repo,
		nil,
	)

	service.snapshotAll(context.Background())

	if len(repo.snapshots) != 0 {
		t.Fatalf("revenue failure overwrote snapshot with %+v", repo.snapshots)
	}
}

func TestTrendsPassesExplicitBusinessDateToRepository(t *testing.T) {
	repo := &fakeMetricsRepository{}
	service := NewMetricsService(nil, nil, nil, repo, &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}})

	if _, err := service.Trends(context.Background(), "user-1", 7); err != nil {
		t.Fatalf("Trends() error: %v", err)
	}
	if repo.listRangeBusinessDay == "" {
		t.Fatal("Trends() did not pass an explicit business date")
	}
}
