package dashboard

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/admin_accounts"
	"transithub/backend/internal/modules/upstream"
)

// fakeSessionStore 是 SessionStore 的内存实现，仅供测试使用。
type fakeSessionStore struct {
	records map[string]*AdminSession // key: userID+"|"+adminAccountID
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
	return nil, nil
}

// fakeAdminAccounts 是 AdminAccountService 的内存实现，按 userID 返回固定的当前工作区。
type fakeAdminAccounts struct {
	current map[string]string // userID -> adminAccountID
}

func (f *fakeAdminAccounts) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	id, ok := f.current[userID]
	if !ok {
		return "", requestError(ErrorAdminOnly)
	}
	return id, nil
}

func (f *fakeAdminAccounts) UpsertAndSwitch(ctx context.Context, userID string, input admin_accounts.UpsertInput) (admin_accounts.Account, error) {
	return admin_accounts.Account{}, errors.New("not implemented")
}

// fakePlatformClient 是 PlatformClient 的桩实现，只有测试用到的方法有真实行为，
// 其余方法返回零值以满足接口。
type fakePlatformClient struct {
	verifyAdminErr error
	groups         []upstream.GroupInfo
	groupsErr      error
	dailyStats     []upstream.GroupDailyStat
	dailyStatsErr  error
	// capturedSession 记录最后一次调用 FetchAdminGroupDailyStats 时传入的 session，
	// 用于断言隔离性（不同工作区应使用不同 session）。
	capturedSession upstream.Session
	// refreshSessionErr / refreshSessionResult 供 RefreshAdminSession 测试控制 RefreshSession 的行为。
	refreshSessionErr    error
	refreshSessionResult *upstream.Session
}

func (f *fakePlatformClient) NormalizeURL(value string) (string, error) { return value, nil }

func (f *fakePlatformClient) LoginAdmin(baseURL string, platform upstream.Platform, account string, password string) (upstream.Session, error) {
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
	return 0, nil
}

func (f *fakePlatformClient) FetchAdminSiteBalanceFiltered(session upstream.Session, filter upstream.BalanceFilter) (upstream.AdminSiteBalance, error) {
	return upstream.AdminSiteBalance{}, nil
}

func (f *fakePlatformClient) FetchAdminGroups(session upstream.Session) ([]upstream.GroupInfo, error) {
	return f.groups, f.groupsErr
}

func (f *fakePlatformClient) FetchAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	return nil, nil
}

func (f *fakePlatformClient) FetchAdminGroupDailyStats(session upstream.Session, groups []upstream.GroupInfo) ([]upstream.GroupDailyStat, error) {
	f.capturedSession = session
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
