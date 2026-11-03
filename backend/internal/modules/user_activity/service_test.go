package user_activity

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// 所有测试数据均为合成数据（example.test），不涉及真实站点或用户。

var testWorkspace = Workspace{UserID: "user-1", AdminAccountID: "account-1"}

// 2026-11-02 10:30（Asia/Shanghai）
var testNow = time.Date(2026, 11, 2, 2, 30, 0, 0, time.UTC)

func day(value string) time.Time {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		panic(err)
	}
	return parsed
}

type fakeStore struct {
	mu          sync.Mutex
	states      map[Workspace]SyncState
	snapshots   []Snapshot
	replaceErr  error
	periodCalls [][2]string
	listRows    []UserRow
	listTotal   int
	listFilters []UserFilter
	listLimits  []int

	rechargeDays  map[string]RechargeDaySummary
	balanceTotals BalanceTotals
	siteBalances  map[string]float64
}

func newFakeStore() *fakeStore {
	return &fakeStore{states: map[Workspace]SyncState{}, rechargeDays: map[string]RechargeDaySummary{}, siteBalances: map[string]float64{}}
}

func (f *fakeStore) ReplaceSnapshot(ctx context.Context, ws Workspace, snapshot Snapshot) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.replaceErr != nil {
		return f.replaceErr
	}
	f.snapshots = append(f.snapshots, snapshot)
	return nil
}

func (f *fakeStore) GetSyncState(ctx context.Context, ws Workspace) (SyncState, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.states[ws]
	return state, ok, nil
}

func (f *fakeStore) SaveSyncState(ctx context.Context, ws Workspace, state SyncState) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.states[ws] = state
	return nil
}

func (f *fakeStore) ResetRunningSyncs(ctx context.Context, errorKey string) error { return nil }

func (f *fakeStore) ListSyncedWorkspaces(ctx context.Context) ([]Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	result := make([]Workspace, 0, len(f.states))
	for ws := range f.states {
		result = append(result, ws)
	}
	return result, nil
}

func (f *fakeStore) ProfileCounts(ctx context.Context, ws Workspace, now time.Time, days []int) (ProfileCounts, error) {
	counts := ProfileCounts{Total: 10}
	for _, d := range days {
		counts.Inactive = append(counts.Inactive, InactiveBucket{Days: d, Count: 1})
	}
	return counts, nil
}

func (f *fakeStore) PeriodMetrics(ctx context.Context, ws Workspace, siteUserID string, from time.Time, to time.Time, loc *time.Location) (PeriodMetrics, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.periodCalls = append(f.periodCalls, [2]string{formatDay(from), formatDay(to)})
	return PeriodMetrics{From: formatDay(from), To: formatDay(to), Requests: 1}, nil
}

func (f *fakeStore) PeriodRetention(ctx context.Context, ws Workspace, current [2]time.Time, previous [2]time.Time) (Retention, error) {
	return Retention{Retained: 1}, nil
}

func (f *fakeStore) DailySeries(ctx context.Context, ws Workspace, siteUserID string, from time.Time, to time.Time) ([]DailyPoint, error) {
	// 与 Repository 一致：每一天都带上入账值（没有入账为 0），由 service 按覆盖区间置空。
	series := fillDailySeries(map[string]DailyPoint{}, from, to)
	for i := range series {
		zero := 0.0
		series[i].Recharge = &zero
	}
	return series, nil
}

func (f *fakeStore) SuppressionCount(ctx context.Context, ws Workspace) (int, error) { return 0, nil }

func (f *fakeStore) ListUsers(ctx context.Context, ws Workspace, windows Windows, filter UserFilter, sortKey string, order string, limit int, offset int) ([]UserRow, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listFilters = append(f.listFilters, filter)
	f.listLimits = append(f.listLimits, limit)
	return f.listRows, f.listTotal, nil
}

func (f *fakeStore) RecipientPreview(ctx context.Context, ws Workspace, windows Windows, filter UserFilter) (RecipientPreview, error) {
	return RecipientPreview{}, nil
}

func (f *fakeStore) RechargeDay(ctx context.Context, ws Workspace, day time.Time) (RechargeDaySummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if summary, ok := f.rechargeDays[formatDay(day)]; ok {
		return summary, nil
	}
	return RechargeDaySummary{Channels: []ChannelAmount{}}, nil
}

func (f *fakeStore) BalanceTotals(ctx context.Context, ws Workspace, now time.Time) (BalanceTotals, error) {
	return f.balanceTotals, nil
}

func (f *fakeStore) SiteBalance(ctx context.Context, ws Workspace, day time.Time) (*float64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if value, ok := f.siteBalances[formatDay(day)]; ok {
		return &value, nil
	}
	return nil, nil
}

func (f *fakeStore) state(t *testing.T) SyncState {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	state, ok := f.states[testWorkspace]
	if !ok {
		t.Fatal("sync state was not saved")
	}
	return state
}

type fakeSessions struct {
	mu      sync.Mutex
	session upstream.Session
	err     error
	calls   int
}

func (f *fakeSessions) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.session, f.err
}

type fakePlatform struct {
	mu sync.Mutex

	sub2apiUsers []upstream.Sub2APIAdminUser
	sub2apiTotal int
	trend        func(date string) ([]upstream.Sub2APIUserUsageTrendPoint, error)
	trendDates   []string
	trendLimits  []int
	trendTZ      []string
	usersBlock   chan struct{}
	usersCalls   int

	newapiUsers []upstream.NewAPIAdminUser
	quota       func(start time.Time, end time.Time) ([]upstream.NewAPIUserQuotaBucket, error)
	quotaRanges [][2]time.Time

	// 入账记录（每种类型按使用时间倒序）。
	redeem         map[string][]upstream.Sub2APIRedeemRecord
	redeemErr      error
	redeemCalls    []string
	affiliate      []upstream.Sub2APIAffiliateTransfer
	affiliateErr   error
	affiliateCalls []string
}

func (f *fakePlatform) FetchSub2APIUsedRedeemRecords(session upstream.Session, codeType string, page int, pageSize int) (upstream.Sub2APIRedeemRecordsPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.redeemCalls = append(f.redeemCalls, fmt.Sprintf("%s:%d", codeType, page))
	if f.redeemErr != nil {
		return upstream.Sub2APIRedeemRecordsPage{}, f.redeemErr
	}
	records := f.redeem[codeType]
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(records) {
		start = len(records)
	}
	if end > len(records) {
		end = len(records)
	}
	return upstream.Sub2APIRedeemRecordsPage{Items: records[start:end], Total: len(records), TotalKnown: true}, nil
}

func (f *fakePlatform) FetchSub2APIAffiliateTransfers(session upstream.Session, startDate string, endDate string, timezone string, page int, pageSize int) (upstream.Sub2APIAffiliateTransfersPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.affiliateCalls = append(f.affiliateCalls, fmt.Sprintf("%s..%s:%d", startDate, endDate, page))
	if f.affiliateErr != nil {
		return upstream.Sub2APIAffiliateTransfersPage{}, f.affiliateErr
	}
	loc, _ := time.LoadLocation(BusinessTimezone)
	matched := make([]upstream.Sub2APIAffiliateTransfer, 0)
	for _, item := range f.affiliate {
		local := item.CreatedAt.In(loc).Format("2006-01-02")
		if local >= startDate && local <= endDate {
			matched = append(matched, item)
		}
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(matched) {
		start = len(matched)
	}
	if end > len(matched) {
		end = len(matched)
	}
	return upstream.Sub2APIAffiliateTransfersPage{Items: matched[start:end], Total: len(matched), TotalKnown: true}, nil
}

func (f *fakePlatform) FetchSub2APIAdminUsersPage(session upstream.Session, query upstream.Sub2APIAdminUsersQuery) (upstream.Sub2APIAdminUsersPage, error) {
	f.mu.Lock()
	f.usersCalls++
	block := f.usersBlock
	f.mu.Unlock()
	if block != nil {
		<-block
	}
	start := (query.Page - 1) * query.PageSize
	end := start + query.PageSize
	if start > len(f.sub2apiUsers) {
		start = len(f.sub2apiUsers)
	}
	if end > len(f.sub2apiUsers) {
		end = len(f.sub2apiUsers)
	}
	total := f.sub2apiTotal
	if total == 0 {
		total = len(f.sub2apiUsers)
	}
	pages := (total + query.PageSize - 1) / query.PageSize
	return upstream.Sub2APIAdminUsersPage{Items: f.sub2apiUsers[start:end], Total: total, Page: query.Page, PageSize: query.PageSize,
		Pages: pages, TotalKnown: true, PagesKnown: true}, nil
}

func (f *fakePlatform) FetchSub2APIUserUsageTrend(session upstream.Session, startDate string, endDate string, timezone string, limit int) ([]upstream.Sub2APIUserUsageTrendPoint, error) {
	f.mu.Lock()
	f.trendDates = append(f.trendDates, startDate+".."+endDate)
	f.trendLimits = append(f.trendLimits, limit)
	f.trendTZ = append(f.trendTZ, timezone)
	trend := f.trend
	f.mu.Unlock()
	if trend == nil {
		return nil, nil
	}
	return trend(startDate)
}

func (f *fakePlatform) FetchNewAPIAdminUsersPage(session upstream.Session, page int, pageSize int) (upstream.NewAPIAdminUsersPage, error) {
	start := (page - 1) * pageSize
	end := start + pageSize
	if start > len(f.newapiUsers) {
		start = len(f.newapiUsers)
	}
	if end > len(f.newapiUsers) {
		end = len(f.newapiUsers)
	}
	return upstream.NewAPIAdminUsersPage{Items: f.newapiUsers[start:end], Total: len(f.newapiUsers), TotalKnown: true}, nil
}

func (f *fakePlatform) FetchNewAPIUserQuotaData(session upstream.Session, start time.Time, end time.Time) ([]upstream.NewAPIUserQuotaBucket, error) {
	f.mu.Lock()
	f.quotaRanges = append(f.quotaRanges, [2]time.Time{start, end})
	quota := f.quota
	f.mu.Unlock()
	if quota == nil {
		return nil, nil
	}
	return quota(start, end)
}

type fakeAccounts struct{}

func (fakeAccounts) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	return testWorkspace.AdminAccountID, nil
}

func newTestService(store *fakeStore, sessions *fakeSessions, platform *fakePlatform) *Service {
	service := newService(store, sessions, platform, fakeAccounts{})
	service.now = func() time.Time { return testNow }
	service.pause = 0
	return service
}

func sub2apiUsers(count int) []upstream.Sub2APIAdminUser {
	users := make([]upstream.Sub2APIAdminUser, 0, count)
	for i := 1; i <= count; i++ {
		users = append(users, upstream.Sub2APIAdminUser{ID: strconv.Itoa(i), Email: fmt.Sprintf("u%d@example.test", i), Username: fmt.Sprintf("u%d", i), Role: "user", Status: "active"})
	}
	return users
}

func floatPtr(value float64) *float64 { return &value }

func TestSyncSub2APIFirstRunFetchesRecentDaysThenBackfills(t *testing.T) {
	store := newFakeStore()
	users := sub2apiUsers(150)
	lastUsed := time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC)
	users[0].Role = "admin"
	users[0].LastUsedAt = &lastUsed
	users[1].TotalRecharged = floatPtr(20)
	users[2].Status = "disabled"
	platform := &fakePlatform{sub2apiUsers: users, trend: func(date string) ([]upstream.Sub2APIUserUsageTrendPoint, error) {
		if date != "2026-11-02" {
			return nil, nil
		}
		// sub2api 用数据库时区标注日期：同一天的数据可能被标成两个日期，必须都归到请求的那一天。
		return []upstream.Sub2APIUserUsageTrendPoint{
			{Date: "2026-11-01", UserID: "1", Requests: 3, Tokens: 300, Cost: 2, ActualCost: 1.5},
			{Date: "2026-11-02", UserID: "1", Requests: 2, Tokens: 200, Cost: 1, ActualCost: 0.5},
			{Date: "2026-11-02", UserID: "2", Requests: 1, Tokens: 50, Cost: 0.2, ActualCost: 0.1},
		}, nil
	}}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://site.example.test", AccessToken: "token"}}
	service := newTestService(store, sessions, platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	if len(platform.trendDates) != 90 {
		t.Fatalf("expected 62 recent + 28 backfill day requests, got %d", len(platform.trendDates))
	}
	if platform.trendDates[0] != "2026-09-02..2026-09-02" || platform.trendDates[61] != "2026-11-02..2026-11-02" {
		t.Fatalf("recent window must be requested one day at a time in ascending order, got %s .. %s", platform.trendDates[0], platform.trendDates[61])
	}
	if platform.trendDates[62] != "2026-09-01..2026-09-01" || platform.trendDates[89] != "2026-08-05..2026-08-05" {
		t.Fatalf("backfill must go from newest to oldest down to the 90-day limit, got %s .. %s", platform.trendDates[62], platform.trendDates[89])
	}
	if platform.trendLimits[0] != 150+trendLimitMargin || platform.trendTZ[0] != BusinessTimezone {
		t.Fatalf("unexpected trend limit/timezone: %d %s", platform.trendLimits[0], platform.trendTZ[0])
	}

	// 最近窗口、用量回补、入账回补各写一次。
	if len(store.snapshots) != 3 {
		t.Fatalf("expected recent, usage backfill and recharge backfill snapshots, got %d", len(store.snapshots))
	}
	recent := store.snapshots[0]
	if len(recent.Profiles) != 150 || len(recent.Days) != 62 || !recent.PruneMissingProfiles || !recent.SyncStamp.Equal(testNow) {
		t.Fatalf("unexpected recent snapshot: profiles=%d days=%d prune=%v", len(recent.Profiles), len(recent.Days), recent.PruneMissingProfiles)
	}
	if !recent.RetainFrom.Equal(day("2026-11-02").AddDate(0, 0, -(retentionDays - 1))) {
		t.Fatalf("unexpected retention start %s", formatDay(recent.RetainFrom))
	}
	if len(recent.Daily) != 2 {
		t.Fatalf("expected per-user aggregation, got %+v", recent.Daily)
	}
	first := recent.Daily[0]
	if first.SiteUserID != "1" || !first.Day.Equal(day("2026-11-02")) || first.Requests != 5 || first.Tokens != 500 || first.Cost != 2 {
		t.Fatalf("rows of the requested day must be summed with actual_cost as cost, got %+v", first)
	}
	if recent.Profiles[0].Role != RoleAdmin || !recent.Profiles[0].EverUsed || recent.Profiles[1].EverUsed != true || recent.Profiles[2].Status != StatusDisabled || recent.Profiles[3].EverUsed {
		t.Fatalf("unexpected profile mapping: %+v %+v %+v %+v", recent.Profiles[0], recent.Profiles[1], recent.Profiles[2], recent.Profiles[3])
	}
	backfill := store.snapshots[1]
	if len(backfill.Profiles) != 0 || len(backfill.Days) != 28 || backfill.PruneMissingProfiles || len(backfill.RechargeDays) != 0 {
		t.Fatalf("backfill snapshot must only replace history days, got profiles=%d days=%d", len(backfill.Profiles), len(backfill.Days))
	}
	if len(recent.RechargeDays) != 62 || recent.SiteDaily == nil || recent.SiteDaily.UserCount != 150 || !recent.FundedDeltaDay.IsZero() {
		t.Fatalf("sub2api recent snapshot must replace recharge records for the same days and record a site snapshot: %+v", recent.SiteDaily)
	}

	state := store.state(t)
	if state.Status != SyncStatusSuccess || state.ErrorKey != "" || state.UserCount != 150 || state.Platform != string(upstream.PlatformSub2API) {
		t.Fatalf("unexpected state: %+v", state)
	}
	if formatDay(*state.SyncedFrom) != "2026-08-05" || formatDay(*state.SyncedTo) != "2026-11-02" {
		t.Fatalf("unexpected coverage %s..%s", formatDay(*state.SyncedFrom), formatDay(*state.SyncedTo))
	}
}

func TestSyncIncrementalResumesFromLastSyncedDay(t *testing.T) {
	store := newFakeStore()
	from, to := day("2026-08-05"), day("2026-11-01")
	store.states[testWorkspace] = SyncState{Platform: string(upstream.PlatformSub2API), Status: SyncStatusSuccess, SyncedFrom: &from, SyncedTo: &to}
	platform := &fakePlatform{sub2apiUsers: sub2apiUsers(2)}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API, AccessToken: "token"}}
	service := newTestService(store, sessions, platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	if strings.Join(platform.trendDates, ",") != "2026-11-01..2026-11-01,2026-11-02..2026-11-02" {
		t.Fatalf("incremental sync must re-fetch the last synced day and today only, got %v", platform.trendDates)
	}
	if len(store.snapshots) != 2 || len(store.snapshots[1].Days) != 0 || len(store.snapshots[1].RechargeDays) == 0 {
		t.Fatalf("usage coverage already reaches the platform limit; only the recharge history may be backfilled, got %d snapshots", len(store.snapshots))
	}
	state := store.state(t)
	if formatDay(*state.SyncedFrom) != "2026-08-05" || formatDay(*state.SyncedTo) != "2026-11-02" {
		t.Fatalf("contiguous coverage must be kept, got %s..%s", formatDay(*state.SyncedFrom), formatDay(*state.SyncedTo))
	}
}

func TestSyncAfterLongGapRestartsCoverage(t *testing.T) {
	store := newFakeStore()
	from, to := day("2026-01-01"), day("2026-02-01")
	store.states[testWorkspace] = SyncState{Platform: string(upstream.PlatformNewAPI), Status: SyncStatusSuccess, SyncedFrom: &from, SyncedTo: &to}
	platform := &fakePlatform{}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformNewAPI, AccessToken: "nap_token"}}
	service := newTestService(store, sessions, platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	state := store.state(t)
	// 间隔太久：最近窗口最多 62 天，与旧区间不相接，覆盖区间从本次重新累计，并再向前回补 31 天。
	if formatDay(*state.SyncedFrom) != "2026-08-02" {
		t.Fatalf("coverage must restart from the recent window plus one backfill chunk, got %s", formatDay(*state.SyncedFrom))
	}
}

func TestSyncNewAPIMapsUsernamesAndBusinessDays(t *testing.T) {
	store := newFakeStore()
	created := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	platform := &fakePlatform{
		newapiUsers: []upstream.NewAPIAdminUser{
			{ID: "5", Username: "alice", Email: "alice@example.test", Group: "vip", Role: 1, Status: 1, Balance: 2, UsedQuota: 7.5, EverUsed: true, CreatedAt: &created},
			{ID: "6", Username: "bob", Email: "", Group: "default", Role: 100, Status: 2},
		},
		quota: func(start time.Time, end time.Time) ([]upstream.NewAPIUserQuotaBucket, error) {
			if !start.Equal(time.Date(2026, 11, 1, 16, 0, 0, 0, time.UTC)) {
				return nil, nil
			}
			return []upstream.NewAPIUserQuotaBucket{
				{Username: "alice", HourStart: time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC), Requests: 3, Tokens: 30, Cost: 0.5},
				{Username: "alice", HourStart: time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC), Requests: 1, Tokens: 10, Cost: 0.25},
				{Username: "ghost", HourStart: time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC), Requests: 2, Tokens: 20, Cost: 1},
				{Username: "alice", HourStart: time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)},
			}, nil
		},
	}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformNewAPI, AccessToken: "nap_token"}}
	service := newTestService(store, sessions, platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	var todayRange [2]time.Time
	for _, r := range platform.quotaRanges {
		if r[0].Equal(time.Date(2026, 11, 1, 16, 0, 0, 0, time.UTC)) {
			todayRange = r
		}
	}
	if todayRange[0].IsZero() || !todayRange[1].Equal(time.Date(2026, 11, 2, 15, 59, 59, 0, time.UTC)) {
		t.Fatalf("each day must be requested with Asia/Shanghai boundaries (inclusive end), got %v", todayRange)
	}
	recent := store.snapshots[0]
	if len(recent.Daily) != 2 {
		t.Fatalf("expected alice and the unmatched username, got %+v", recent.Daily)
	}
	alice, ghost := recent.Daily[0], recent.Daily[1]
	if alice.SiteUserID != "5" || alice.Requests != 4 || alice.Tokens != 40 || alice.Cost != 0.75 || !alice.Day.Equal(day("2026-11-02")) {
		t.Fatalf("unexpected alice usage: %+v", alice)
	}
	if ghost.SiteUserID != unmatchedUserPrefix+"ghost" || ghost.Requests != 2 {
		t.Fatalf("usage of unknown usernames must be kept for totals, got %+v", ghost)
	}
	// 最近一小时的桶结束于 11:00（本地），晚于当前 10:30，取当前时间作为上界。
	if at, ok := recent.LastUsed["5"]; !ok || !at.Equal(testNow) {
		t.Fatalf("last used must be the bucket end capped at now, got %v", recent.LastUsed)
	}
	if _, ok := recent.LastUsed[unmatchedUserPrefix+"ghost"]; ok {
		t.Fatal("unmatched usernames must not get a last-used time")
	}
	profiles := recent.Profiles
	if profiles[0].Role != RoleUser || profiles[0].Status != StatusActive || *profiles[0].TotalUsed != 7.5 || profiles[0].LastUsedAt != nil {
		t.Fatalf("unexpected alice profile: %+v", profiles[0])
	}
	if profiles[1].Role != RoleRoot || profiles[1].Status != StatusDisabled || profiles[1].EverUsed {
		t.Fatalf("unexpected bob profile: %+v", profiles[1])
	}
	state := store.state(t)
	if formatDay(*state.SyncedFrom) != "2026-08-02" {
		t.Fatalf("new-api first sync must backfill one chunk beyond the recent window, got %s", formatDay(*state.SyncedFrom))
	}
}

func TestSyncRecentUsageFailureFailsWithoutWriting(t *testing.T) {
	store := newFakeStore()
	platform := &fakePlatform{sub2apiUsers: sub2apiUsers(1), trend: func(date string) ([]upstream.Sub2APIUserUsageTrendPoint, error) {
		if date == "2026-10-30" {
			return nil, &upstream.RequestError{MessageKey: upstream.ErrorNetwork}
		}
		return nil, nil
	}}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API, AccessToken: "token"}}
	service := newTestService(store, sessions, platform)

	err := service.SyncWorkspace(context.Background(), testWorkspace)
	if err == nil {
		t.Fatal("expected sync failure")
	}
	if len(store.snapshots) != 0 {
		t.Fatal("a failed recent window must not write partial data")
	}
	state := store.state(t)
	if state.Status != SyncStatusFailed || state.ErrorKey != ErrorNetwork || state.SyncedTo != nil {
		t.Fatalf("unexpected failed state: %+v", state)
	}
}

func TestSyncBackfillFailureKeepsContiguousDays(t *testing.T) {
	store := newFakeStore()
	platform := &fakePlatform{sub2apiUsers: sub2apiUsers(1), trend: func(date string) ([]upstream.Sub2APIUserUsageTrendPoint, error) {
		if date == "2026-08-20" {
			return nil, errors.New("boom")
		}
		return nil, nil
	}}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API, AccessToken: "token"}}
	service := newTestService(store, sessions, platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("backfill failures must not fail the sync: %v", err)
	}
	state := store.state(t)
	if state.Status != SyncStatusSuccess || formatDay(*state.SyncedFrom) != "2026-08-21" {
		t.Fatalf("coverage must stop at the last contiguous backfilled day, got %+v from=%s", state, formatDay(*state.SyncedFrom))
	}
	if days := store.snapshots[1].Days; len(days) != 12 || formatDay(days[0]) != "2026-09-01" || formatDay(days[11]) != "2026-08-21" {
		t.Fatalf("unexpected backfilled days: %v", days)
	}
}

func TestSyncRejectsTooManyUsers(t *testing.T) {
	store := newFakeStore()
	platform := &fakePlatform{sub2apiUsers: sub2apiUsers(1), sub2apiTotal: maxSiteUsers + 1}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API, AccessToken: "token"}}
	service := newTestService(store, sessions, platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err == nil || errorKeyOf(err) != ErrorTooManyUsers {
		t.Fatalf("expected too many users error, got %v", err)
	}
	if len(platform.trendDates) != 0 {
		t.Fatal("usage must not be fetched after the user list was rejected")
	}
}

func TestTriggerSyncValidatesSessionBeforeStarting(t *testing.T) {
	store := newFakeStore()
	sessions := &fakeSessions{err: errors.New("admin.mySites.errors.authRequired")}
	service := newTestService(store, sessions, &fakePlatform{})

	_, err := service.TriggerSync(context.Background(), testWorkspace)
	if err == nil || err.Error() != ErrorAuthRequired {
		t.Fatalf("expected auth required error, got %v", err)
	}
	if len(store.states) != 0 {
		t.Fatal("a rejected trigger must not create a sync state")
	}
	if !service.acquire(testWorkspace) {
		t.Fatal("the workspace lock must be released after a rejected trigger")
	}
}

func TestTriggerSyncRunsOncePerWorkspace(t *testing.T) {
	store := newFakeStore()
	block := make(chan struct{})
	platform := &fakePlatform{sub2apiUsers: sub2apiUsers(1), usersBlock: block}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API, AccessToken: "token"}}
	service := newTestService(store, sessions, platform)

	first, err := service.TriggerSync(context.Background(), testWorkspace)
	if err != nil || first.Status != SyncStatusRunning {
		t.Fatalf("unexpected first trigger: %+v %v", first, err)
	}
	second, err := service.TriggerSync(context.Background(), testWorkspace)
	if err != nil || second.Status != SyncStatusRunning {
		t.Fatalf("a concurrent trigger must report the running sync: %+v %v", second, err)
	}
	close(block)
	service.WaitBackground()
	if sessions.calls != 1 {
		t.Fatalf("only one sync may run per workspace, session calls = %d", sessions.calls)
	}
	if state := store.state(t); state.Status != SyncStatusSuccess {
		t.Fatalf("unexpected final state: %+v", state)
	}
}

func TestSummaryComparisonsRespectLocalCoverage(t *testing.T) {
	store := newFakeStore()
	from, to := day("2026-09-23"), day("2026-11-02") // 覆盖最近 41 天
	store.states[testWorkspace] = SyncState{Platform: string(upstream.PlatformSub2API), Status: SyncStatusSuccess, SyncedFrom: &from, SyncedTo: &to}
	service := newTestService(store, &fakeSessions{}, &fakePlatform{})

	summary, err := service.Summary(context.Background(), testWorkspace, 30)
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	if !summary.HasData || len(summary.Daily) != 30 || summary.Daily[29].Day != "2026-11-02" || summary.Timezone != BusinessTimezone {
		t.Fatalf("unexpected summary basics: %+v", summary)
	}
	dayCmp, week, month := summary.Periods["day"], summary.Periods["week"], summary.Periods["month"]
	if dayCmp.Current.From != "2026-11-01" || dayCmp.Current.To != "2026-11-01" || dayCmp.Previous == nil || dayCmp.Previous.From != "2026-10-31" {
		t.Fatalf("unexpected day comparison: %+v", dayCmp)
	}
	if week.Current.From != "2026-10-26" || week.Current.To != "2026-11-01" || week.Previous == nil || week.Previous.From != "2026-10-19" || week.Previous.To != "2026-10-25" || week.Retention == nil {
		t.Fatalf("unexpected week comparison: %+v", week)
	}
	if month.Current.From != "2026-10-03" || month.Previous != nil || month.Retention != nil {
		t.Fatalf("month previous window starts before local coverage and must be nil: %+v", month)
	}
	if dayCmp.LastYear != nil || week.LastYear != nil || month.LastYear != nil {
		t.Fatal("year-over-year needs local data from last year")
	}
}

func TestSummaryWithoutSyncReturnsEmptyState(t *testing.T) {
	service := newTestService(newFakeStore(), &fakeSessions{}, &fakePlatform{})
	summary, err := service.Summary(context.Background(), testWorkspace, 60)
	if err != nil || summary.HasData || summary.Sync.Status != "" || summary.Daily == nil || summary.Periods == nil {
		t.Fatalf("unexpected empty summary: %+v %v", summary, err)
	}
}

func TestRecipientsRequireSyncAndRespectLimit(t *testing.T) {
	store := newFakeStore()
	service := newTestService(store, &fakeSessions{}, &fakePlatform{})
	if _, err := service.Recipients(context.Background(), testWorkspace, UserFilter{Segment: SegmentInactive7d}, 10); err == nil || err.Error() != ErrorNotSynced {
		t.Fatalf("expected not synced error, got %v", err)
	}

	to := day("2026-11-02")
	store.states[testWorkspace] = SyncState{SyncedFrom: &to, SyncedTo: &to}
	store.listRows = []UserRow{{SiteUserID: "1", Email: "u1@example.test"}, {SiteUserID: "2", Email: "u2@example.test"}}
	store.listTotal = 2
	recipients, err := service.Recipients(context.Background(), testWorkspace, UserFilter{Segment: SegmentInactive7d, SiteUserID: "injected"}, 10)
	if err != nil || len(recipients) != 2 || recipients[1].Email != "u2@example.test" {
		t.Fatalf("unexpected recipients: %+v %v", recipients, err)
	}
	if filter := store.listFilters[0]; !filter.HasEmail || filter.SiteUserID != "" || store.listLimits[0] != 11 {
		t.Fatalf("recipients must require an email, ignore caller-supplied ids and probe max+1 rows: %+v limit=%d", filter, store.listLimits[0])
	}

	store.listTotal = 11
	if _, err := service.Recipients(context.Background(), testWorkspace, UserFilter{Segment: SegmentAll}, 10); err == nil || err.Error() != ErrorTooManyRecipient {
		t.Fatalf("expected too many recipients error, got %v", err)
	}
}

func TestUserWhereBindsParametersAndValidatesFilters(t *testing.T) {
	now := testNow
	args := []any{}
	next := func(value any) string {
		args = append(args, value)
		return "$" + strconv.Itoa(len(args))
	}
	where, err := userWhere(UserFilter{Segment: SegmentInactive, InactiveDays: 3, Search: "50%_off", Status: StatusActive, Role: RoleAdmin, HasEmail: true}, now, next)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, fragment := range []string{"last_used_at < $1", "status <> $2", "role IN ($3, $4)", "ILIKE $5", "site_user_id = $6", "email <> ''"} {
		if !strings.Contains(where, fragment) {
			t.Fatalf("where clause %q is missing %q", where, fragment)
		}
	}
	if args[0].(time.Time) != now.AddDate(0, 0, -3) || args[4] != `%50\%\_off%` {
		t.Fatalf("unexpected bound args: %v", args)
	}
	if strings.Contains(where, "50%") {
		t.Fatal("user input must never be concatenated into SQL")
	}

	for _, filter := range []UserFilter{
		{Segment: "drop table"},
		{Segment: SegmentInactive, InactiveDays: 0},
		{Segment: SegmentInactive, InactiveDays: maxInactiveDays + 1},
		{Status: "deleted"},
		{Role: "owner"},
		{Search: strings.Repeat("x", maxSearchRunes+1)},
	} {
		if _, err := userWhere(filter, now, next); err == nil || err.Error() != ErrorInvalidFilter {
			t.Fatalf("filter %+v must be rejected, got %v", filter, err)
		}
	}
	if where, err := userWhere(UserFilter{}, now, next); err != nil || where != "" {
		t.Fatalf("empty filter must not add conditions: %q %v", where, err)
	}
}

func TestNewWindowsUseCompleteDaysEndingYesterday(t *testing.T) {
	windows := NewWindows(testNow, day("2026-11-02"))
	got := []string{formatDay(windows.Yesterday), formatDay(windows.From7), formatDay(windows.PrevFrom7), formatDay(windows.PrevTo7), formatDay(windows.From30), formatDay(windows.PrevFrom30), formatDay(windows.PrevTo30)}
	want := []string{"2026-11-01", "2026-10-26", "2026-10-19", "2026-10-25", "2026-10-03", "2026-09-03", "2026-10-02"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("windows = %v, want %v", got, want)
	}
}

func TestDayOfUsesBusinessTimezone(t *testing.T) {
	service := newTestService(newFakeStore(), &fakeSessions{}, &fakePlatform{})
	// 2026-11-01 16:30 UTC 已经是上海时间 11 月 2 日 00:30。
	if got := formatDay(service.dayOf(time.Date(2026, 11, 1, 16, 30, 0, 0, time.UTC))); got != "2026-11-02" {
		t.Fatalf("dayOf = %s", got)
	}
}
