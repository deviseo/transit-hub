package profit

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// 采集测试：内存仓储 + 假站点，数据均为合成数据。

type memoryStore struct {
	mu      sync.Mutex
	state   SyncState // 测试工作区 ws 的状态
	exists  bool
	others  map[Workspace]SyncState // 其它工作区的状态
	listed  []Workspace             // ListSyncedWorkspaces 的返回值
	routes  []Route
	revenue map[string][]RouteRevenue // key: day
	costs   map[string][]SiteCost
	saves   []SyncState
	prices  []PriceChange
}

func newMemoryStore() *memoryStore {
	return &memoryStore{revenue: map[string][]RouteRevenue{}, costs: map[string][]SiteCost{}}
}

func (m *memoryStore) GetSyncState(ctx context.Context, w Workspace) (SyncState, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w != ws {
		state, ok := m.others[w]
		return state, ok, nil
	}
	return m.state, m.exists, nil
}

func (m *memoryStore) SaveSyncState(ctx context.Context, w Workspace, state SyncState) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w != ws {
		if m.others == nil {
			m.others = map[Workspace]SyncState{}
		}
		m.others[w] = state
		return nil
	}
	m.state, m.exists = state, true
	m.saves = append(m.saves, state)
	return nil
}

func (m *memoryStore) other(w Workspace) (SyncState, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state, ok := m.others[w]
	return state, ok
}

func (m *memoryStore) current() SyncState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.state
}

func (m *memoryStore) ResetRunningSyncs(ctx context.Context) error { return nil }
func (m *memoryStore) ListSyncedWorkspaces(ctx context.Context) ([]Workspace, error) {
	return m.listed, nil
}

func (m *memoryStore) UpsertRoutes(ctx context.Context, ws Workspace, routes []Route, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.routes = routes
	return nil
}

func (m *memoryStore) ListRoutes(ctx context.Context, ws Workspace) ([]Route, error) {
	return m.routes, nil
}

func (m *memoryStore) ReplaceDay(ctx context.Context, ws Workspace, day time.Time, revenue []RouteRevenue, costs []SiteCost) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.revenue[formatDay(day)] = revenue
	m.costs[formatDay(day)] = costs
	return nil
}

func (m *memoryStore) RouteRevenue(ctx context.Context, ws Workspace, from, to time.Time) ([]RouteRevenue, error) {
	return nil, nil
}
func (m *memoryStore) SiteCosts(ctx context.Context, ws Workspace, from, to time.Time) ([]SiteCost, error) {
	return nil, nil
}
func (m *memoryStore) SiteDailyCosts(ctx context.Context, ws Workspace, from, to time.Time) (map[string]map[string]float64, error) {
	return nil, nil
}
func (m *memoryStore) RouteDailyRevenue(ctx context.Context, ws Workspace, from, to time.Time) (map[string]map[string]float64, error) {
	return nil, nil
}
func (m *memoryStore) DailyTotals(ctx context.Context, ws Workspace, from, to time.Time) ([]DayTotals, error) {
	return nil, nil
}
func (m *memoryStore) Prune(ctx context.Context, ws Workspace, before time.Time) error { return nil }

func (m *memoryStore) InsertPriceChange(ctx context.Context, ws Workspace, change PriceChange) (PriceChange, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	change.ID = int64(len(m.prices) + 1)
	m.prices = append(m.prices, change)
	return change, nil
}

func (m *memoryStore) ListPriceChanges(ctx context.Context, ws Workspace, since time.Time, until time.Time, limit int) ([]PriceChange, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]PriceChange, 0)
	for i := len(m.prices) - 1; i >= 0 && len(result) < limit; i-- {
		if change := m.prices[i]; !change.DetectedAt.Before(since) && change.DetectedAt.Before(until) {
			result = append(result, change)
		}
	}
	return result, nil
}

func (m *memoryStore) collectedDays() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	days := make([]string, 0, len(m.revenue))
	for day := range m.revenue {
		days = append(days, day)
	}
	sort.Strings(days)
	return days
}

type fakePlatform struct {
	mu            sync.Mutex
	routes        []upstream.OwnSiteRoute
	routesErr     error
	totals        map[string]float64 // date -> whole-site revenue
	accountRows   map[string][]upstream.RouteGroupRevenue
	flow          map[int64][]upstream.RouteGroupRevenue
	flowErr       error
	channelValues map[string]float64 // channel|group -> revenue
	calls         []string
}

func (f *fakePlatform) record(call string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, call)
}

func (f *fakePlatform) ListOwnSiteRoutes(session upstream.Session) ([]upstream.OwnSiteRoute, error) {
	f.record("routes")
	return f.routes, f.routesErr
}

func (f *fakePlatform) FetchSub2APIAccountGroupRevenue(session upstream.Session, accountID string, date string) ([]upstream.RouteGroupRevenue, error) {
	f.record("account:" + accountID + "@" + date)
	return f.accountRows[accountID+"@"+date], nil
}

func (f *fakePlatform) FetchNewAPIFlowRevenue(session upstream.Session, start int64, end int64) ([]upstream.RouteGroupRevenue, error) {
	f.record(fmt.Sprintf("flow:%d", start))
	if f.flowErr != nil {
		return nil, f.flowErr
	}
	return f.flow[start], nil
}

func (f *fakePlatform) FetchNewAPIChannelRevenue(session upstream.Session, channelID string, group string, start int64, end int64) (float64, error) {
	f.record("channel:" + channelID + "|" + group)
	return f.channelValues[channelID+"|"+group], nil
}

func (f *fakePlatform) FetchAdminUsageStats(session upstream.Session, startDate, endDate string) (float64, error) {
	f.record("total:" + startDate)
	return f.totals[startDate], nil
}

func (f *fakePlatform) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, call := range f.calls {
		if strings.HasPrefix(call, prefix) {
			n++
		}
	}
	return n
}

type fakeUpstreams struct {
	failing map[string]bool // date -> site s2 fails that day
	// 分组消费（调价影响估算）：只有站点 s1 存在。
	rate          float64
	groupConsumes map[string]float64 // upstream.GroupKey -> 窗口内消费
	ranges        *[]string
}

func (f fakeUpstreams) SiteConsumesOnDate(ctx context.Context, userID string, adminAccountID string, date string, timezone string) ([]upstream.SiteDailyConsume, error) {
	status := upstream.ConsumeStatusOK
	if f.failing[date] {
		status = upstream.ConsumeStatusFailed
	}
	return []upstream.SiteDailyConsume{
		{SiteID: "s1", SiteName: "Alpha", BaseURL: "https://alpha.example", RechargeRate: 2, Consume: 5, Cost: 10, Status: upstream.ConsumeStatusOK},
		{SiteID: "s2", SiteName: "Beta", BaseURL: "https://beta.example", RechargeRate: 1, Status: status},
	}, nil
}

func (f fakeUpstreams) ListForAccount(ctx context.Context, userID, adminAccountID string) []upstream.Response {
	return nil
}

func (f fakeUpstreams) GroupConsumesInRange(ctx context.Context, userID string, siteID string, groups []upstream.GroupInfo, startDate string, endDate string, timezone string) (upstream.SiteGroupConsumes, error) {
	if siteID != "s1" {
		return upstream.SiteGroupConsumes{}, errors.New("site not found")
	}
	if f.ranges != nil {
		*f.ranges = append(*f.ranges, fmt.Sprintf("%s..%s@%s#%d", startDate, endDate, timezone, len(groups)))
	}
	result := upstream.SiteGroupConsumes{SiteID: "s1", SiteName: "Alpha", BaseURL: "https://api.alpha.example", RechargeRate: f.rate, Consumes: map[string]float64{}}
	for _, group := range groups {
		if value, ok := f.groupConsumes[upstream.GroupKey(group)]; ok {
			result.Consumes[upstream.GroupKey(group)] = value
		}
	}
	return result, nil
}

type fakeSessions struct {
	session upstream.Session
	err     error
}

func (f fakeSessions) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	return f.session, f.err
}

func dayOf(value string) time.Time {
	parsed, _ := time.Parse("2006-01-02", value)
	return parsed
}

func newCollector(store *memoryStore, platform *fakePlatform, sessions fakeSessions, now time.Time) *Service {
	return &Service{store: store, sessions: sessions, platform: platform, upstreams: fakeUpstreams{}, now: func() time.Time { return now }, running: map[Workspace]struct{}{}}
}

var ws = Workspace{UserID: "u", AdminAccountID: "a"}

func TestDaysToCollect(t *testing.T) {
	today := dayOf("2026-11-03")
	days := daysToCollect(SyncState{}, today)
	if len(days) != initialBackfillDays || formatDay(days[0]) != "2026-10-04" || formatDay(days[len(days)-1]) != "2026-11-02" {
		t.Fatalf("first run backfills %d complete days: %v..%v (%d)", initialBackfillDays, days[0], days[len(days)-1], len(days))
	}
	complete := dayOf("2026-11-01")
	if days := daysToCollect(SyncState{CompleteThrough: &complete}, today); len(days) != 1 || formatDay(days[0]) != "2026-11-02" {
		t.Fatalf("later runs start after the last final day: %v", days)
	}
	// 首轮回补被中断：从已采集到的最后一天之后继续。
	syncedTo := dayOf("2026-10-20")
	if days := daysToCollect(SyncState{SyncedTo: &syncedTo}, today); formatDay(days[0]) != "2026-10-21" || len(days) != 13 {
		t.Fatalf("interrupted backfills resume: %v", days)
	}
	old := dayOf("2026-01-01")
	if days := daysToCollect(SyncState{CompleteThrough: &old}, today); len(days) != initialBackfillDays {
		t.Fatalf("long gaps are capped to the backfill window: %d", len(days))
	}
	if days := daysToCollect(SyncState{CompleteThrough: &syncedTo}, dayOf("2026-10-21")); len(days) != 0 {
		t.Fatalf("nothing to collect when yesterday is final: %v", days)
	}
}

func TestSub2APICollectionSkipsQuietDaysAndKeepsTotals(t *testing.T) {
	store := newMemoryStore()
	complete := dayOf("2026-10-31")
	store.state, store.exists = SyncState{CompleteThrough: &complete, SyncedFrom: &complete, SyncedTo: &complete}, true
	platform := &fakePlatform{
		routes: []upstream.OwnSiteRoute{{ID: "11", Name: "relay", BaseURL: "https://api.alpha.example", Groups: []string{"claude"}}, {ID: "12", Name: "oauth"}},
		totals: map[string]float64{"2026-11-01": 0, "2026-11-02": 9},
		accountRows: map[string][]upstream.RouteGroupRevenue{
			"11@2026-11-02": {{RouteID: "11", Group: "claude", Revenue: 6, Requests: 3}},
			"12@2026-11-02": {{RouteID: "12", Group: "gpt", Revenue: 2, Requests: 1}},
		},
	}
	// 北京时间 11-03 00:30：11-02 刚结束不到 1 小时，不能定稿。
	now := time.Date(2026, 11, 2, 16, 30, 0, 0, time.UTC)
	service := newCollector(store, platform, fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API}}, now)

	if err := service.SyncWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if got := strings.Join(store.collectedDays(), ","); got != "2026-11-01,2026-11-02" {
		t.Fatalf("unexpected days: %s", got)
	}
	if platform.count("account:") != 2 {
		t.Fatalf("a day without revenue must not query each account: %v", platform.calls)
	}
	rows := store.revenue["2026-11-02"]
	if len(rows) != 3 || rows[2].RouteID != "" || rows[2].Revenue != 1 {
		t.Fatalf("revenue not attributed to a current account is kept as a residual: %+v", rows)
	}
	if len(store.costs["2026-11-02"]) != 2 {
		t.Fatalf("upstream costs must be stored with the day: %+v", store.costs)
	}
	state := store.state
	if state.Status != StatusSuccess || state.RevenueSource != SourceSub2APIGroups || state.RouteCount != 2 || state.LastSyncedAt == nil {
		t.Fatalf("unexpected final state: %+v", state)
	}
	if formatDay(*state.CompleteThrough) != "2026-11-01" || formatDay(*state.SyncedTo) != "2026-11-02" {
		t.Fatalf("yesterday is not final within an hour after midnight: %+v", state)
	}

	// 01:10 再采集一次：昨天定稿。
	service.now = func() time.Time { return time.Date(2026, 11, 2, 17, 10, 0, 0, time.UTC) }
	if err := service.SyncWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if formatDay(*store.state.CompleteThrough) != "2026-11-02" {
		t.Fatalf("yesterday must be final after 01:00: %+v", store.state)
	}
}

func TestFailedRecentCostIsRetried(t *testing.T) {
	store := newMemoryStore()
	complete := dayOf("2026-11-01")
	store.state, store.exists = SyncState{CompleteThrough: &complete}, true
	platform := &fakePlatform{routes: []upstream.OwnSiteRoute{{ID: "11"}}, totals: map[string]float64{}}
	service := newCollector(store, platform, fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API}}, time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC))
	service.upstreams = fakeUpstreams{failing: map[string]bool{"2026-11-02": true}}

	if err := service.SyncWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if formatDay(*store.state.CompleteThrough) != "2026-11-01" || store.state.WarningKey != WarningCostFailed {
		t.Fatalf("a recent day with a failed upstream must be retried: %+v", store.state)
	}
}

func TestNewAPICollectionFallsBackWhenFlowIsUnavailable(t *testing.T) {
	store := newMemoryStore()
	complete := dayOf("2026-11-01")
	store.state, store.exists = SyncState{CompleteThrough: &complete}, true
	platform := &fakePlatform{
		routes:        []upstream.OwnSiteRoute{{ID: "7", Groups: []string{"default", "vip"}}, {ID: "8", Groups: []string{"default"}}},
		totals:        map[string]float64{"2026-11-02": 3},
		flow:          map[int64][]upstream.RouteGroupRevenue{},
		channelValues: map[string]float64{"7|default": 1, "7|vip": 0.5, "8|": 1.5},
	}
	now := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	service := newCollector(store, platform, fakeSessions{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, now)

	// 关闭了数据看板：flow 为空，但当天日志合计大于 0，改为逐渠道查询。
	if err := service.SyncWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("sync: %v", err)
	}
	rows := store.revenue["2026-11-02"]
	if len(rows) != 3 || store.state.RevenueSource != SourceNewAPILogStat || store.state.WarningKey != WarningFlowFallback {
		t.Fatalf("expected per-channel fallback rows: %+v state=%+v", rows, store.state)
	}
	if rows[2].RouteID != "8" || rows[2].Group != "default" || rows[2].Revenue != 1.5 {
		t.Fatalf("single-group channels are queried without a group filter: %+v", rows)
	}

	// 新版本：flow 有数据时直接使用。
	store.state.CompleteThrough = &complete
	platform.flow = map[int64][]upstream.RouteGroupRevenue{}
	start := dayStart(dayOf("2026-11-02")).Unix()
	platform.flow[start] = []upstream.RouteGroupRevenue{{RouteID: "7", Group: "vip", Revenue: 4}}
	if err := service.SyncWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("sync with flow: %v", err)
	}
	if rows := store.revenue["2026-11-02"]; len(rows) != 1 || rows[0].Revenue != 4 || store.state.RevenueSource != SourceNewAPIFlow {
		t.Fatalf("flow rows must be used directly: %+v %+v", rows, store.state)
	}

	// 旧版本：接口不存在。
	store.state.CompleteThrough = &complete
	platform.flowErr = upstream.ErrFlowUnsupported
	if err := service.SyncWorkspace(context.Background(), ws); err != nil {
		t.Fatalf("sync without flow: %v", err)
	}
	if store.state.RevenueSource != SourceNewAPILogStat {
		t.Fatalf("missing flow endpoints fall back to log stats: %+v", store.state)
	}
}

func TestSyncErrorsAreMappedAndConcurrentSyncsRejected(t *testing.T) {
	store := newMemoryStore()
	platform := &fakePlatform{routesErr: errors.New(upstream.ErrorNetwork)}
	service := newCollector(store, platform, fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API}}, time.Now())
	if err := service.SyncWorkspace(context.Background(), ws); err == nil || store.state.Status != StatusFailed || store.state.ErrorKey != ErrorNetwork {
		t.Fatalf("route fetch failures must be stored: %v %+v", err, store.state)
	}

	service.sessions = fakeSessions{err: errors.New("admin.mySites.errors.authRequired")}
	if _, err := service.TriggerSync(context.Background(), ws); err == nil || err.Error() != ErrorAuthRequired {
		t.Fatalf("session errors ask for a re-login: %v", err)
	}
	service.sessions = fakeSessions{session: upstream.Session{Platform: "other"}}
	if _, err := service.TriggerSync(context.Background(), ws); err == nil || err.Error() != ErrorPlatform {
		t.Fatalf("unsupported platforms are rejected: %v", err)
	}
	service.running[ws] = struct{}{}
	if _, err := service.TriggerSync(context.Background(), ws); err == nil || err.Error() != ErrorSyncInProgress {
		t.Fatalf("concurrent syncs are rejected: %v", err)
	}
}

func TestUntilNextSlot(t *testing.T) {
	now := time.Date(2026, 11, 3, 9, 5, 0, 0, time.UTC)
	if got := untilNextSlot(now); got != 5*time.Minute+30*time.Second {
		t.Fatalf("next slot at 09:10:30, got %v", got)
	}
	if got := untilNextSlot(time.Date(2026, 11, 3, 9, 10, 30, 0, time.UTC)); got != time.Hour {
		t.Fatalf("exactly on a slot waits for the next hour, got %v", got)
	}
}

type workspaceList []Workspace

func (l workspaceList) SessionWorkspaces(ctx context.Context) ([]Workspace, error) { return l, nil }

// workspaceSessions：expired 里的工作区管理员会话已失效，其余正常。
type workspaceSessions struct {
	session upstream.Session
	expired map[string]bool
}

func (w workspaceSessions) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	if w.expired[adminAccountID] {
		return upstream.Session{}, errors.New("admin.mySites.errors.authRequired")
	}
	return w.session, nil
}

func TestBackgroundSyncResumesInterruptedAndStartsFirstCollections(t *testing.T) {
	store := newMemoryStore()
	complete := dayOf("2026-11-01")
	store.state, store.exists = SyncState{Status: StatusInterrupted, ErrorKey: ErrorInterrupted, CompleteThrough: &complete, SyncedFrom: &complete, SyncedTo: &complete}, true
	routine := Workspace{UserID: "u", AdminAccountID: "routine"}
	stale := Workspace{UserID: "u", AdminAccountID: "stale"}
	store.others = map[Workspace]SyncState{
		routine: {Status: StatusSuccess, CompleteThrough: &complete, SyncedFrom: &complete, SyncedTo: &complete},
		stale:   {Status: StatusInterrupted, ErrorKey: ErrorInterrupted, StartedBy: StartedByManual, CompleteThrough: &complete, SyncedFrom: &complete, SyncedTo: &complete},
	}
	store.listed = []Workspace{ws, routine, stale}
	fresh := Workspace{UserID: "u", AdminAccountID: "fresh"}
	expired := Workspace{UserID: "u", AdminAccountID: "expired"}
	platform := &fakePlatform{routes: []upstream.OwnSiteRoute{{ID: "11"}}, totals: map[string]float64{}}
	now := time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC)
	service := newCollector(store, platform, fakeSessions{}, now)
	service.sessions = workspaceSessions{session: upstream.Session{Platform: upstream.PlatformSub2API}, expired: map[string]bool{expired.AdminAccountID: true, stale.AdminAccountID: true}}
	service.SetWorkspaceLister(workspaceList{ws, fresh, expired})

	// 服务刚启动：继续被打断的采集，为新工作区开始首次采集；例行采集留给整点。
	service.syncAll(context.Background(), true)
	if state := store.current(); state.Status != StatusSuccess || state.StartedBy != StartedByResume {
		t.Fatalf("interrupted collections resume in the background: %+v", state)
	}
	if state, ok := store.other(fresh); !ok || state.Status != StatusSuccess || state.StartedBy != StartedByAuto || state.SyncedTo == nil {
		t.Fatalf("workspaces with a saved session start their first collection automatically: %+v %v", state, ok)
	}
	if _, ok := store.other(expired); ok {
		t.Fatal("a workspace whose session expired must not get a failed state before its first collection")
	}
	if state, _ := store.other(routine); state.LastStartedAt != nil {
		t.Fatalf("routine collections wait for the hourly slot after a restart: %+v", state)
	}
	// 会话失效、没能继续的采集也记下这次尝试（时间、方式、原因），页面据此提醒。
	if state, _ := store.other(stale); state.Status != StatusFailed || state.ErrorKey != ErrorAuthRequired || state.StartedBy != StartedByResume || state.LastStartedAt == nil || !state.LastStartedAt.Equal(now) {
		t.Fatalf("a resume that cannot start records the attempt: %+v", state)
	}

	// 整点：例行采集所有采集过的工作区。
	service.syncAll(context.Background(), false)
	if state, _ := store.other(routine); state.StartedBy != StartedBySchedule || state.LastStartedAt == nil {
		t.Fatalf("hourly runs are routine: %+v", state)
	}
	if state := store.current(); state.StartedBy != StartedBySchedule {
		t.Fatalf("the resumed workspace is routine again on the next hour: %+v", state)
	}
}

func TestManualSyncIsMarkedManual(t *testing.T) {
	store := newMemoryStore()
	platform := &fakePlatform{routes: []upstream.OwnSiteRoute{{ID: "11"}}, totals: map[string]float64{}}
	service := newCollector(store, platform, fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API}}, time.Date(2026, 11, 3, 4, 0, 0, 0, time.UTC))
	status, err := service.TriggerSync(context.Background(), ws)
	if err != nil || status.StartedBy != StartedByManual || status.Status != StatusRunning {
		t.Fatalf("manual syncs report how they started: %+v %v", status, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		service.mu.Lock()
		_, busy := service.running[ws]
		service.mu.Unlock()
		if !busy {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("background sync did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if state := store.current(); state.Status != StatusSuccess || state.StartedBy != StartedByManual {
		t.Fatalf("the finished run keeps its start reason: %+v", state)
	}
}
