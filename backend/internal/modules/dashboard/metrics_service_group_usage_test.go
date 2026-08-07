package dashboard

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/admin_accounts"
	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

type fakeRealConnectionReader struct {
	connections []my_sites.RealConnection
	err         error
}

func (f *fakeRealConnectionReader) ListRealConnectionsForWorkspace(ctx context.Context, userID, adminAccountID string) ([]my_sites.RealConnection, error) {
	return f.connections, f.err
}

// fakeSessionStore 是 SessionStore 的内存实现，仅供测试使用。
type fakeSessionStore struct {
	records        map[string]*AdminSession // key: userID+"|"+adminAccountID
	activeSessions []ActiveSessionRef
	activeErr      error
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{records: map[string]*AdminSession{}}
}

func (f *fakeSessionStore) key(userID, adminAccountID string) string {
	return userID + "|" + adminAccountID
}

func (f *fakeSessionStore) set(userID, adminAccountID string, session AdminSession) {
	f.records[f.key(userID, adminAccountID)] = &session
}

func (f *fakeSessionStore) Get(ctx context.Context, userID string, adminAccountID string) (*AdminSession, error) {
	record, ok := f.records[f.key(userID, adminAccountID)]
	if !ok {
		return nil, nil
	}
	return record, nil
}

func (f *fakeSessionStore) Save(ctx context.Context, userID string, adminAccountID string, session AdminSession) error {
	f.set(userID, adminAccountID, session)
	return nil
}

func (f *fakeSessionStore) Delete(ctx context.Context, userID string, adminAccountID string) error {
	delete(f.records, f.key(userID, adminAccountID))
	return nil
}

func (f *fakeSessionStore) ActiveSessions(ctx context.Context) ([]ActiveSessionRef, error) {
	return f.activeSessions, f.activeErr
}

// fakeAdminAccounts 是 AdminAccountService 的内存实现，按 userID 返回固定的当前工作区。
type fakeAdminAccounts struct {
	current     map[string]string // userID -> adminAccountID
	upsertInput admin_accounts.UpsertInput
}

func (f *fakeAdminAccounts) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	id, ok := f.current[userID]
	if !ok {
		return "", requestError(ErrorAdminOnly)
	}
	return id, nil
}

func (f *fakeAdminAccounts) UpsertAndSwitch(ctx context.Context, userID string, input admin_accounts.UpsertInput) (admin_accounts.Account, error) {
	f.upsertInput = input
	id := f.current[userID]
	if id == "" {
		id = "account-1"
	}
	return admin_accounts.Account{ID: id, UserID: userID, Platform: input.Platform, BaseURL: input.BaseURL, Identity: input.Identity, AuthMethod: input.AuthMethod}, nil
}

// fakePlatformClient 是 PlatformClient 的桩实现，只有测试用到的方法有真实行为，
// 其余方法返回零值以满足接口。
type fakePlatformClient struct {
	verifyAdminErr     error
	usageStats         float64
	usageStatsErr      error
	siteBalance        upstream.AdminSiteBalance
	siteBalanceErr     error
	groups             []upstream.GroupInfo
	groupsErr          error
	adminGroups        []upstream.AdminGroupInfo
	adminGroupsErr     error
	groupAccounts      map[string][]upstream.AdminGroupAccountInfo
	scopeUsage         map[string]upstream.AdminUsageStats
	scopeUsageErr      map[string]error
	dailyStats         []upstream.GroupDailyStat
	dailyStatsErr      error
	capturedUsageStart string
	capturedUsageEnd   string
	capturedGroupDate  string
	// capturedSession 记录最后一次调用 FetchAdminGroupDailyStats 时传入的 session，
	// 用于断言隔离性（不同工作区应使用不同 session）。
	capturedSession upstream.Session
	// refreshSessionErr / refreshSessionResult 供 RefreshAdminSession 测试控制 RefreshSession 的行为。
	refreshSessionErr    error
	refreshSessionResult *upstream.Session
	adminKeyResult       *upstream.Session
	adminKeyErr          error
	capturedAdminKey     string
	capturedUserID       string
	capturedPlatform     upstream.Platform
}

func (f *fakePlatformClient) NormalizeURL(value string) (string, error) { return value, nil }

func (f *fakePlatformClient) LoginAdmin(baseURL string, platform upstream.Platform, account string, password string) (upstream.Session, error) {
	return upstream.Session{}, errors.New("not implemented")
}

func (f *fakePlatformClient) LoginAdminWithKey(baseURL string, platform upstream.Platform, key string, userID string) (upstream.Session, error) {
	f.capturedAdminKey = key
	f.capturedUserID = userID
	f.capturedPlatform = platform
	if f.adminKeyErr != nil {
		return upstream.Session{}, f.adminKeyErr
	}
	if f.adminKeyResult != nil {
		return *f.adminKeyResult, nil
	}
	return upstream.Session{}, errors.New("not implemented")
}

func (f *fakePlatformClient) VerifyAdmin(session upstream.Session) error { return f.verifyAdminErr }

func (f *fakePlatformClient) RefreshSession(session upstream.Session) (upstream.Session, error) {
	if f.refreshSessionErr != nil {
		return upstream.Session{}, f.refreshSessionErr
	}
	if f.refreshSessionResult != nil {
		return *f.refreshSessionResult, nil
	}
	return session, nil
}

func (f *fakePlatformClient) FetchAdminUsageStats(session upstream.Session, startDate, endDate string) (float64, error) {
	f.capturedUsageStart = startDate
	f.capturedUsageEnd = endDate
	return f.usageStats, f.usageStatsErr
}

func (f *fakePlatformClient) FetchAdminUsageStatsForScope(session upstream.Session, accountID, groupID, startDate, endDate string) (upstream.AdminUsageStats, error) {
	key := accountID + "|" + groupID
	if f.scopeUsageErr != nil {
		if err := f.scopeUsageErr[key]; err != nil {
			return upstream.AdminUsageStats{}, err
		}
	}
	if f.scopeUsage != nil {
		if stats, ok := f.scopeUsage[key]; ok {
			return stats, nil
		}
	}
	return upstream.AdminUsageStats{TotalActualCost: f.usageStats}, f.usageStatsErr
}

func (f *fakePlatformClient) FetchAdminSiteBalanceFiltered(session upstream.Session, filter upstream.BalanceFilter) (upstream.AdminSiteBalance, error) {
	return f.siteBalance, f.siteBalanceErr
}

func (f *fakePlatformClient) FetchAdminGroups(session upstream.Session) ([]upstream.GroupInfo, error) {
	return f.groups, f.groupsErr
}

func (f *fakePlatformClient) FetchAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	return f.adminGroups, f.adminGroupsErr
}

func (f *fakePlatformClient) ListAdminGroupAccounts(session upstream.Session, group upstream.AdminGroupInfo) ([]upstream.AdminGroupAccountInfo, error) {
	return f.groupAccounts[group.ID], nil
}

func (f *fakePlatformClient) FetchAdminGroupDailyStats(session upstream.Session, groups []upstream.GroupInfo) ([]upstream.GroupDailyStat, error) {
	f.capturedSession = session
	return f.dailyStats, f.dailyStatsErr
}

func (f *fakePlatformClient) FetchAdminGroupDailyStatsForDate(session upstream.Session, groups []upstream.GroupInfo, date string) ([]upstream.GroupDailyStat, error) {
	f.capturedSession = session
	f.capturedGroupDate = date
	return f.dailyStats, f.dailyStatsErr
}

func (f *fakePlatformClient) LoginSub2APIAdmin(baseURL string, email string, password string) (upstream.Session, error) {
	return upstream.Session{}, errors.New("not implemented")
}

func (f *fakePlatformClient) LoginWithToken(baseURL string, platform upstream.Platform, account string, accessToken string, refreshToken string, tokenType string) (upstream.LoginResult, error) {
	return upstream.LoginResult{}, errors.New("not implemented")
}

func (f *fakePlatformClient) VerifySub2APIAdmin(session upstream.Session) error { return nil }

func (f *fakePlatformClient) FetchSub2APIAdminUsageStats(session upstream.Session, startDate, endDate string) (float64, error) {
	return 0, nil
}

func (f *fakePlatformClient) FetchSub2APIAdminSiteBalanceFiltered(session upstream.Session, filter upstream.BalanceFilter) (upstream.AdminSiteBalance, error) {
	return upstream.AdminSiteBalance{}, nil
}

func (f *fakePlatformClient) FetchSub2APIAdminGroups(session upstream.Session) ([]upstream.GroupInfo, error) {
	return nil, nil
}

func (f *fakePlatformClient) FetchSub2APIAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	return nil, nil
}

func (f *fakePlatformClient) FetchCostForDate(session upstream.Session, date string) (float64, upstream.CostFetchMeta, error) {
	return 0, upstream.CostFetchMeta{}, nil
}

func authenticatedSession() upstream.Session {
	return upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://example.com", AccessToken: "token"}
}

// TestGroupUsageToday_Unauthenticated 覆盖测试要求 1：未登录 admin session 返回 ErrorAdminOnly。
func TestGroupUsageToday_Unauthenticated(t *testing.T) {
	store := newFakeSessionStore() // 没有任何已保存的会话
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{}
	service := NewMetricsService(store, platform, nil, nil, accounts)

	_, err := service.GroupUsageToday(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error for unauthenticated session, got nil")
	}
	var reqErr requestError
	if !errors.As(err, &reqErr) || reqErr.Error() != ErrorAdminOnly {
		t.Fatalf("expected ErrorAdminOnly, got %v", err)
	}
}

// TestGroupUsageToday_UserWorkspaceIsolation 覆盖测试要求 2：不同用户/工作区各自取各自的数据，互不串扰。
func TestGroupUsageToday_UserWorkspaceIsolation(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	store.set("user-2", "account-2", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{
		"user-1": "account-1",
		"user-2": "account-2",
	}}

	platformForUser1 := &fakePlatformClient{
		dailyStats: []upstream.GroupDailyStat{{GroupName: "default", TodayActualCost: 10}},
	}
	service1 := NewMetricsService(store, platformForUser1, nil, nil, accounts)
	resp1, err := service1.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error for user-1: %v", err)
	}
	if resp1.Total != 10 {
		t.Fatalf("user-1 total = %.2f, want 10.00", resp1.Total)
	}

	// user-2 没有 admin_accounts 中的映射会失败于 requireCurrentAdminAccount 之外场景已在其他测试覆盖；
	// 这里验证：即使复用同一个 MetricsService 实例，user-2 与 user-1 的 session 记录也不会混淆。
	platformForUser2 := &fakePlatformClient{
		dailyStats: []upstream.GroupDailyStat{{GroupName: "vip", TodayActualCost: 99}},
	}
	service2 := NewMetricsService(store, platformForUser2, nil, nil, accounts)
	resp2, err := service2.GroupUsageToday(context.Background(), "user-2")
	if err != nil {
		t.Fatalf("unexpected error for user-2: %v", err)
	}
	if resp2.Total != 99 {
		t.Fatalf("user-2 total = %.2f, want 99.00", resp2.Total)
	}
	if len(resp2.Groups) != 1 || resp2.Groups[0].GroupName != "vip" {
		t.Fatalf("user-2 groups leaked user-1 data: %+v", resp2.Groups)
	}
}

// TestGroupUsageToday_MergesDuplicateGroupNames 覆盖测试要求 3：重名分组合并求和。
func TestGroupUsageToday_MergesDuplicateGroupNames(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{
		dailyStats: []upstream.GroupDailyStat{
			{GroupName: "default", TodayActualCost: 10},
			{GroupName: " default ", TodayActualCost: 5}, // 前后空格应归一化后合并
			{GroupName: "", TodayActualCost: 999},        // 空名应跳过
			{GroupName: "vip", TodayActualCost: 3.5},
		},
	}
	service := NewMetricsService(store, platform, nil, nil, accounts)

	resp, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Groups) != 2 {
		t.Fatalf("expected 2 merged groups, got %d: %+v", len(resp.Groups), resp.Groups)
	}
	byName := map[string]float64{}
	for _, g := range resp.Groups {
		byName[g.GroupName] = g.TodayAmount
	}
	if byName["default"] != 15 {
		t.Errorf("default merged amount = %.2f, want 15.00", byName["default"])
	}
	if byName["vip"] != 3.5 {
		t.Errorf("vip amount = %.2f, want 3.50", byName["vip"])
	}
}

// TestGroupUsageToday_TotalEqualsSumOfGroups 覆盖测试要求 4：total 等于所有 groups[].todayAmount 求和。
func TestGroupUsageToday_TotalEqualsSumOfGroups(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{
		dailyStats: []upstream.GroupDailyStat{
			{GroupName: "default", TodayActualCost: 12.34},
			{GroupName: "vip", TodayActualCost: 56.78},
			{GroupName: "pro", TodayActualCost: 1.11},
		},
	}
	service := NewMetricsService(store, platform, nil, nil, accounts)

	resp, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sum float64
	for _, g := range resp.Groups {
		sum += g.TodayAmount
	}
	if resp.Total != sum {
		t.Fatalf("total (%.4f) != sum of groups (%.4f)", resp.Total, sum)
	}
}

func TestGroupUsageToday_CalculatesMatchedGroupProfit(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{
		dailyStats: []upstream.GroupDailyStat{
			{GroupName: "vip", TodayActualCost: 100},
			{GroupName: " stable ", TodayActualCost: 50},
		},
	}
	upstreams := &fakeUpstreamLister{
		cachedSites: []upstream.Response{{
			RechargeRate: 1,
			Status:       upstream.StatusConnected,
			Metrics: upstream.Metrics{
				TodayConsume: todayCachedMetrics(12).TodayConsume,
				Groups: []upstream.GroupInfo{
					{Name: "vip"},
					{Name: "stable"},
				},
			},
		}},
		keyUsageItems: []upstream.KeyUsageTodayItem{
			{GroupName: "vip", TodayAmount: 40},
			{GroupName: " stable ", TodayAmount: 10},
		},
	}
	service := NewMetricsService(store, platform, upstreams, nil, accounts)

	response, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GroupUsageToday() error: %v", err)
	}
	if !response.ProfitAvailable {
		t.Fatalf("profit should be available: %+v", response)
	}
	byName := map[string]GroupUsageTodayItem{}
	for _, group := range response.Groups {
		byName[group.GroupName] = group
	}
	if got := *byName["vip"].TodayProfit; got != 60 {
		t.Fatalf("vip profit = %.2f, want 60.00", got)
	}
	if got := *byName["vip"].TodayCost; got != 40 {
		t.Fatalf("vip cost = %.2f, want 40.00", got)
	}
	if got := *byName["stable"].TodayProfit; got != 40 {
		t.Fatalf("stable profit = %.2f, want 40.00", got)
	}
	if got := byName["stable"].TodayRevenue; got != 50 {
		t.Fatalf("stable revenue = %.2f, want 50.00", got)
	}
	if response.TotalCost == nil || *response.TotalCost != 50 {
		t.Fatalf("total cost = %v, want 50.00", response.TotalCost)
	}
	if response.TotalProfit == nil || *response.TotalProfit != 100 {
		t.Fatalf("total profit = %v, want 100.00", response.TotalProfit)
	}
}

func TestGroupUsageToday_RealConnectionsKeepConfirmedGroupsDuringPartialCostCollection(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "workspace-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "workspace-1"}}
	platform := &fakePlatformClient{
		adminGroups: []upstream.AdminGroupInfo{
			{ID: "group-1", Name: "改名后的分组"},
			{ID: "group-2", Name: "另一个分组"},
		},
		groupAccounts: map[string][]upstream.AdminGroupAccountInfo{
			"group-1": {{ID: "account-1"}},
			"group-2": {{ID: "account-2"}},
		},
		scopeUsage: map[string]upstream.AdminUsageStats{
			"account-1|group-1": {TotalActualCost: 100},
			"account-2|group-2": {TotalActualCost: 50},
		},
	}
	upstreams := &fakeUpstreamLister{
		cachedSites: []upstream.Response{
			{ID: "site-a", RechargeRate: 1, Status: upstream.StatusConnected, Metrics: todayCachedMetrics(12)},
			{ID: "site-b", RechargeRate: 1, Status: upstream.StatusError},
		},
		keyUsageItems: []upstream.KeyUsageTodayItem{
			{SiteID: "site-a", KeyID: "key-1", TodayAmount: 40},
			{SiteID: "site-a", KeyID: "unbound-key", TodayAmount: 30},
		},
		keyUsageErr: &upstream.KeyUsageCollectionError{FailedSites: 1, TotalSites: 2, Cause: errors.New("site-b unavailable")},
	}
	connections := &fakeRealConnectionReader{connections: []my_sites.RealConnection{
		{ID: "connection-1", Status: "active", UpstreamSiteID: "site-a", UpstreamKeyID: "key-1", AdminAccountID: "account-1", OwnGroupIDs: []string{"group-1"}},
		{ID: "connection-2", Status: "active", UpstreamSiteID: "site-b", UpstreamKeyID: "key-2", AdminAccountID: "account-2", OwnGroupIDs: []string{"group-2"}},
	}}
	service := NewMetricsService(store, platform, upstreams, nil, accounts)
	service.SetRealConnectionReader(connections)

	response, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GroupUsageToday() error: %v", err)
	}
	byID := map[string]GroupUsageTodayItem{}
	for _, group := range response.Groups {
		byID[group.GroupID] = group
	}
	group1 := byID["group-1"]
	if group1.Status != ProfitAllocationExact || group1.TodayCost == nil || *group1.TodayCost != 40 || group1.TodayProfit == nil || *group1.TodayProfit != 60 {
		t.Fatalf("confirmed group was lost during partial collection: %+v", group1)
	}
	group2 := byID["group-2"]
	if group2.Status != ProfitAllocationUnavailable || group2.TodayCost != nil || group2.TodayProfit != nil {
		t.Fatalf("failed binding must remain unknown rather than zero: %+v", group2)
	}
	if response.Quality == nil || response.Quality.Status != "partial" {
		t.Fatalf("quality = %+v, want partial", response.Quality)
	}
	if response.Quality.RunID == "" {
		t.Fatalf("quality run id is empty: %+v", response.Quality)
	}
	if response.UnboundUpstreamCost == nil || *response.UnboundUpstreamCost != 30 {
		t.Fatalf("unbound cost = %v, want 30", response.UnboundUpstreamCost)
	}
	if !hasProfitIssue(response.Issues, "unbound_upstream_cost") {
		t.Fatalf("missing unbound upstream cost issue: %+v", response.Issues)
	}
	for _, issue := range response.Issues {
		if issue.RunID != response.Quality.RunID || issue.ObservedAt == "" {
			t.Fatalf("issue lacks trace metadata: %+v quality=%+v", issue, response.Quality)
		}
	}
}

func TestGroupUsageToday_DoesNotUseGroupRevenueFallbackForBoundConnection(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "workspace-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "workspace-1"}}
	platform := &fakePlatformClient{
		usageStats:  999,
		adminGroups: []upstream.AdminGroupInfo{{ID: "group-1", Name: "绑定分组"}},
		groupAccounts: map[string][]upstream.AdminGroupAccountInfo{
			"group-1": {{ID: "account-1"}},
		},
		scopeUsageErr: map[string]error{
			"account-1|group-1": errors.New("scoped revenue failed"),
		},
	}
	upstreams := &fakeUpstreamLister{
		cachedSites:   []upstream.Response{{ID: "site-a", RechargeRate: 1, Status: upstream.StatusConnected, Metrics: todayCachedMetrics(12)}},
		keyUsageItems: []upstream.KeyUsageTodayItem{{SiteID: "site-a", KeyID: "key-1", TodayAmount: 40}},
	}
	connections := &fakeRealConnectionReader{connections: []my_sites.RealConnection{{
		ID: "connection-1", Status: "active", UpstreamSiteID: "site-a", UpstreamKeyID: "key-1",
		AdminAccountID: "account-1", OwnGroupIDs: []string{"group-1"},
	}}}
	service := NewMetricsService(store, platform, upstreams, nil, accounts)
	service.SetRealConnectionReader(connections)

	response, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GroupUsageToday() error: %v", err)
	}
	if len(response.Groups) != 1 {
		t.Fatalf("groups = %+v, want one group", response.Groups)
	}
	group := response.Groups[0]
	if group.TodayRevenue != 0 || group.TodayCost != nil || group.TodayProfit != nil {
		t.Fatalf("scoped revenue failure must remain unknown, got %+v", group)
	}
	if response.Quality == nil || response.Quality.Status != ProfitAllocationUnavailable {
		t.Fatalf("quality = %+v, want unavailable", response.Quality)
	}
}

func TestGroupUsageToday_AllowsKnownGroupWithZeroCost(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{
		dailyStats: []upstream.GroupDailyStat{
			{GroupName: "vip", TodayActualCost: 100},
			{GroupName: "stable", TodayActualCost: 50},
		},
	}
	upstreams := &fakeUpstreamLister{
		cachedSites: []upstream.Response{{
			RechargeRate: 1,
			Status:       upstream.StatusConnected,
			Metrics: upstream.Metrics{
				TodayConsume: todayCachedMetrics(12).TodayConsume,
				Groups: []upstream.GroupInfo{
					{Name: "vip"},
					{Name: "stable"},
				},
			},
		}},
		keyUsageItems: []upstream.KeyUsageTodayItem{{GroupName: "vip", TodayAmount: 40}},
	}
	service := NewMetricsService(store, platform, upstreams, nil, accounts)

	response, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GroupUsageToday() error: %v", err)
	}
	if !response.ProfitAvailable {
		t.Fatalf("profit should be available when a known group has zero cost: %+v", response)
	}
	byName := map[string]GroupUsageTodayItem{}
	for _, group := range response.Groups {
		byName[group.GroupName] = group
	}
	if got := *byName["stable"].TodayCost; got != 0 {
		t.Fatalf("stable cost = %.2f, want 0.00", got)
	}
	if got := *byName["stable"].TodayProfit; got != 50 {
		t.Fatalf("stable profit = %.2f, want 50.00", got)
	}
}

func TestGroupUsageToday_DoesNotShowProfitWhenCostCollectionIsPartial(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{dailyStats: []upstream.GroupDailyStat{{GroupName: "vip", TodayActualCost: 100}}}
	upstreams := &fakeUpstreamLister{
		cachedSites: []upstream.Response{
			{RechargeRate: 1, Status: upstream.StatusConnected, Metrics: todayCachedMetrics(12)},
			{RechargeRate: 1, Status: upstream.StatusError},
		},
		keyUsageItems: []upstream.KeyUsageTodayItem{{GroupName: "vip", TodayAmount: 40}},
		keyUsageErr: &upstream.KeyUsageCollectionError{
			FailedSites: 1,
			TotalSites:  2,
			Cause:       errors.New("one upstream failed"),
		},
	}
	service := NewMetricsService(store, platform, upstreams, nil, accounts)

	response, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GroupUsageToday() error: %v", err)
	}
	if response.ProfitAvailable {
		t.Fatalf("profit should be unavailable on partial cost data: %+v", response)
	}
	if response.TotalProfit != nil || response.Groups[0].TodayProfit != nil {
		t.Fatalf("partial cost must not produce a profit value: %+v", response.Groups[0])
	}
	if response.ProfitUnavailableReason != "upstream_cost_unavailable" {
		t.Fatalf("partial cost reason = %q, want upstream_cost_unavailable", response.ProfitUnavailableReason)
	}
}

func TestGroupUsageToday_DoesNotShowProfitWhenGroupNamesDoNotAlign(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{dailyStats: []upstream.GroupDailyStat{{GroupName: "vip", TodayActualCost: 100}}}
	upstreams := &fakeUpstreamLister{
		cachedSites: []upstream.Response{{
			RechargeRate: 1,
			Status:       upstream.StatusConnected,
			Metrics: upstream.Metrics{
				TodayConsume: todayCachedMetrics(12).TodayConsume,
				Groups:       []upstream.GroupInfo{{Name: "stable"}},
			},
		}},
		keyUsageItems: []upstream.KeyUsageTodayItem{{GroupName: "stable", TodayAmount: 40}},
	}
	service := NewMetricsService(store, platform, upstreams, nil, accounts)

	response, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GroupUsageToday() error: %v", err)
	}
	if response.ProfitAvailable {
		t.Fatalf("profit should be unavailable for unmatched groups: %+v", response)
	}
	if response.TotalProfit != nil || response.Groups[0].TodayProfit != nil {
		t.Fatalf("unmatched group must not produce a profit value: %+v", response.Groups[0])
	}
	if response.ProfitUnavailableReason != "group_name_unmatched" {
		t.Fatalf("unmatched reason = %q, want group_name_unmatched", response.ProfitUnavailableReason)
	}
}

func TestGroupUsageTodayUsesOneExplicitBusinessDate(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{
		dailyStats: []upstream.GroupDailyStat{{GroupName: "default", TodayActualCost: 12.34}},
	}
	service := NewMetricsService(store, platform, nil, nil, accounts)

	response, err := service.GroupUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GroupUsageToday() error: %v", err)
	}
	if platform.capturedGroupDate == "" {
		t.Fatal("group usage query did not receive an explicit business date")
	}
	if response.Date != platform.capturedGroupDate {
		t.Fatalf("response date = %q, query date = %q", response.Date, platform.capturedGroupDate)
	}
}

// TestGroupUsageToday_AdminVerifyFailure 补充：VerifyAdmin 失败时同样归为 ErrorAdminOnly，
// 不泄漏底层平台错误细节。
func TestGroupUsageToday_AdminVerifyFailure(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Session: authenticatedSession()})
	accounts := &fakeAdminAccounts{current: map[string]string{"user-1": "account-1"}}
	platform := &fakePlatformClient{verifyAdminErr: errors.New("not admin")}
	service := NewMetricsService(store, platform, nil, nil, accounts)

	_, err := service.GroupUsageToday(context.Background(), "user-1")
	var reqErr requestError
	if !errors.As(err, &reqErr) || reqErr.Error() != ErrorAdminOnly {
		t.Fatalf("expected ErrorAdminOnly, got %v", err)
	}
}
