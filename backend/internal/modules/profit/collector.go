package profit

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/bizclock"
)

// SessionProvider 提供工作区「我的站点」的管理员会话（my_sites.Service 实现，会自动刷新令牌）。
type SessionProvider interface {
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}

// PlatformClient 是采集用到的自己站点只读接口（upstream.PlatformService 实现）。
type PlatformClient interface {
	ListOwnSiteRoutes(session upstream.Session) ([]upstream.OwnSiteRoute, error)
	FetchSub2APIAccountGroupRevenue(session upstream.Session, accountID string, date string) ([]upstream.RouteGroupRevenue, error)
	FetchNewAPIFlowRevenue(session upstream.Session, start int64, end int64) ([]upstream.RouteGroupRevenue, error)
	FetchNewAPIChannelRevenue(session upstream.Session, channelID string, group string, start int64, end int64) (float64, error)
	FetchAdminUsageStats(session upstream.Session, startDate, endDate string) (float64, error)
}

// UpstreamSites 提供已登记上游站点的每日消费、当前余额和分组消费（upstream.Service 实现）。
type UpstreamSites interface {
	SiteConsumesOnDate(ctx context.Context, userID string, adminAccountID string, date string, timezone string) ([]upstream.SiteDailyConsume, error)
	ListForAccount(ctx context.Context, userID, adminAccountID string) []upstream.Response
	GroupConsumesInRange(ctx context.Context, userID string, siteID string, groups []upstream.GroupInfo, startDate string, endDate string, timezone string) (upstream.SiteGroupConsumes, error)
}

// PriceProvider 返回工作区「我的站点售价」（站点余额 1 个单位卖多少元），与仪表盘营收换算一致。
type PriceProvider interface {
	SiteUnitPrice(ctx context.Context, userID string, adminAccountID string) (float64, error)
}

// WorkspaceLister 列出保存了自己站点管理员会话的工作区，定时任务据此为还没采集过的工作区在后台自动开始首次采集。
type WorkspaceLister interface {
	SessionWorkspaces(ctx context.Context) ([]Workspace, error)
}

// AccountResolver 解析用户当前选中的工作区。
type AccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

type store interface {
	GetSyncState(ctx context.Context, ws Workspace) (SyncState, bool, error)
	SaveSyncState(ctx context.Context, ws Workspace, state SyncState) error
	ResetRunningSyncs(ctx context.Context) error
	ListSyncedWorkspaces(ctx context.Context) ([]Workspace, error)
	UpsertRoutes(ctx context.Context, ws Workspace, routes []Route, now time.Time) error
	ListRoutes(ctx context.Context, ws Workspace) ([]Route, error)
	ReplaceDay(ctx context.Context, ws Workspace, day time.Time, revenue []RouteRevenue, costs []SiteCost) error
	RouteRevenue(ctx context.Context, ws Workspace, from, to time.Time) ([]RouteRevenue, error)
	SiteCosts(ctx context.Context, ws Workspace, from, to time.Time) ([]SiteCost, error)
	SiteDailyCosts(ctx context.Context, ws Workspace, from, to time.Time) (map[string]map[string]float64, error)
	RouteDailyRevenue(ctx context.Context, ws Workspace, from, to time.Time) (map[string]map[string]float64, error)
	DailyTotals(ctx context.Context, ws Workspace, from, to time.Time) ([]DayTotals, error)
	Prune(ctx context.Context, ws Workspace, before time.Time) error
	InsertPriceChange(ctx context.Context, ws Workspace, change PriceChange) (PriceChange, error)
	ListPriceChanges(ctx context.Context, ws Workspace, since time.Time, until time.Time, limit int) ([]PriceChange, error)
}

const (
	// initialBackfillDays 是首次采集回补的天数。
	initialBackfillDays = 30
	// finalizeAfter：某一天结束后至少过了这么久再采集，这一天的数据才视为最终（站点的用量日志异步写入）。
	finalizeAfter = time.Hour
	// retryRecentDays 内有上游站点查询失败的日期不视为最终，下一轮重试；更早的日期按已有数据定稿。
	retryRecentDays = 2
	retentionDays   = 400
	maxRoutes       = 500
	syncInterval    = time.Hour
	// scheduleOffset：每小时的 10 分 30 秒执行，与用户分析（整点 30 秒）错开。
	scheduleOffset      = 10*time.Minute + 30*time.Second
	defaultRequestPause = 100 * time.Millisecond
	// startupDelay：服务启动后等这么久，在后台继续被重启打断的采集，并为新工作区开始首次采集。
	startupDelay = 2 * time.Minute
)

// Service 采集并分析利润：自己站点按路由、分组的营收，已登记上游站点的成本，按上游域名自动归因。
type Service struct {
	store     store
	sessions  SessionProvider
	platform  PlatformClient
	upstreams UpstreamSites
	prices    PriceProvider
	accounts  AccountResolver
	// workspaces 为 nil 时只继续已经采集过的工作区，不自动开始首次采集。
	workspaces WorkspaceLister
	now        func() time.Time
	pause      time.Duration

	mu      sync.Mutex
	running map[Workspace]struct{}
}

func NewService(repository *Repository, sessions SessionProvider, platform PlatformClient, upstreams UpstreamSites, prices PriceProvider, accounts AccountResolver) *Service {
	return &Service{
		store:     repository,
		sessions:  sessions,
		platform:  platform,
		upstreams: upstreams,
		prices:    prices,
		accounts:  accounts,
		now:       time.Now,
		pause:     defaultRequestPause,
		running:   map[Workspace]struct{}{},
	}
}

// SetWorkspaceLister 让定时任务为保存了管理员会话、但还没采集过的工作区在后台自动开始首次采集。
func (s *Service) SetWorkspaceLister(lister WorkspaceLister) {
	s.workspaces = lister
}

// CurrentWorkspace 返回用户当前选中的工作区。
func (s *Service) CurrentWorkspace(ctx context.Context, userID string) (Workspace, error) {
	if s.accounts == nil {
		return Workspace{}, requestError(ErrorNoCurrentAccount)
	}
	adminAccountID, err := s.accounts.RequireCurrentID(ctx, userID)
	if err != nil || strings.TrimSpace(adminAccountID) == "" {
		return Workspace{}, requestError(ErrorNoCurrentAccount)
	}
	return Workspace{UserID: userID, AdminAccountID: adminAccountID}, nil
}

func (s *Service) acquire(ws Workspace) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, busy := s.running[ws]; busy {
		return false
	}
	s.running[ws] = struct{}{}
	return true
}

func (s *Service) release(ws Workspace) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.running, ws)
}

func supportedPlatform(platform upstream.Platform) bool {
	return platform == upstream.PlatformSub2API || platform == upstream.PlatformNewAPI
}

// TriggerSync 手动触发采集：同步校验会话后在后台执行，立即返回 running 状态。
func (s *Service) TriggerSync(ctx context.Context, ws Workspace) (SyncStatus, error) {
	if !s.acquire(ws) {
		return SyncStatus{}, requestError(ErrorSyncInProgress)
	}
	session, err := s.sessions.RequireSession(ctx, ws.UserID, ws.AdminAccountID)
	if err != nil {
		s.release(ws)
		return SyncStatus{}, mapSessionError(err)
	}
	if !supportedPlatform(session.Platform) {
		s.release(ws)
		return SyncStatus{}, requestError(ErrorPlatform)
	}
	state, _, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		s.release(ws)
		return SyncStatus{}, err
	}
	now := s.now()
	state.Status = StatusRunning
	state.ErrorKey = ""
	state.Platform = string(session.Platform)
	state.LastStartedAt = &now
	state.StartedBy = StartedByManual
	if err := s.store.SaveSyncState(ctx, ws, state); err != nil {
		s.release(ws)
		return SyncStatus{}, err
	}
	go func() {
		defer s.release(ws)
		if err := s.run(context.Background(), ws, session, state); err != nil {
			log.Printf("[profit] sync failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
		}
	}()
	return statusOf(state), nil
}

// SyncWorkspace 同步执行一次例行采集（定时任务使用）。
func (s *Service) SyncWorkspace(ctx context.Context, ws Workspace) error {
	return s.SyncWorkspaceAs(ctx, ws, StartedBySchedule)
}

// SyncWorkspaceAs 同步执行一次采集，startedBy 记录这次采集是怎么开始的。
// 还没采集过的工作区会话不可用或平台不支持时直接返回，不创建采集状态（页面保持「尚未采集」）。
func (s *Service) SyncWorkspaceAs(ctx context.Context, ws Workspace, startedBy string) error {
	if !s.acquire(ws) {
		return requestError(ErrorSyncInProgress)
	}
	defer s.release(ws)
	state, exists, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		return err
	}
	// 会话不可用也算一次尝试：记下尝试时间和方式，页面据此提醒「采集失败」。
	attempt := func(errorKey string) {
		if !exists {
			return
		}
		attemptedAt := s.now()
		state.LastStartedAt = &attemptedAt
		state.StartedBy = startedBy
		s.saveFailure(ctx, ws, state, errorKey)
	}
	session, err := s.sessions.RequireSession(ctx, ws.UserID, ws.AdminAccountID)
	if err != nil {
		mapped := mapSessionError(err)
		attempt(mapped.Error())
		return mapped
	}
	if !supportedPlatform(session.Platform) {
		attempt(ErrorPlatform)
		return requestError(ErrorPlatform)
	}
	now := s.now()
	state.Status = StatusRunning
	state.ErrorKey = ""
	state.Platform = string(session.Platform)
	state.LastStartedAt = &now
	state.StartedBy = startedBy
	if err := s.store.SaveSyncState(ctx, ws, state); err != nil {
		return err
	}
	return s.run(ctx, ws, session, state)
}

func (s *Service) saveFailure(ctx context.Context, ws Workspace, state SyncState, errorKey string) {
	state.Status = StatusFailed
	state.ErrorKey = errorKey
	if err := s.store.SaveSyncState(ctx, ws, state); err != nil {
		log.Printf("[profit] save failure state failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
	}
}

// calendarDay 把业务日表示为 UTC 零点（与 date 列口径一致）。
func calendarDay(t time.Time) time.Time {
	local := t.In(bizclock.Location())
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func formatDay(day time.Time) string { return day.Format(bizclock.DateLayout) }

// dayStart 返回日历日在业务时区的 00:00。
func dayStart(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, bizclock.Location())
}

// daysToCollect 返回这一轮要采集的完整自然日：从「已定稿的最后一天」（或中断前采集到的最后一天）的下一天到昨天；
// 首次采集回补 initialBackfillDays 天。
func daysToCollect(state SyncState, today time.Time) []time.Time {
	yesterday := today.AddDate(0, 0, -1)
	earliest := today.AddDate(0, 0, -initialBackfillDays)
	start := earliest
	switch {
	case state.CompleteThrough != nil:
		start = state.CompleteThrough.AddDate(0, 0, 1)
	case state.SyncedTo != nil:
		start = state.SyncedTo.AddDate(0, 0, 1)
	}
	if start.Before(earliest) {
		start = earliest
	}
	days := make([]time.Time, 0)
	for day := start; !day.After(yesterday); day = day.AddDate(0, 0, 1) {
		days = append(days, day)
	}
	return days
}

type runState struct {
	flowUnsupported bool
}

func (s *Service) sleep(ctx context.Context) {
	if s.pause <= 0 {
		return
	}
	timer := time.NewTimer(s.pause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

func (s *Service) run(ctx context.Context, ws Workspace, session upstream.Session, state SyncState) error {
	now := s.now()
	today := calendarDay(now)
	ownRoutes, err := s.platform.ListOwnSiteRoutes(session)
	if err != nil {
		key := mapFetchError(err, ErrorRoutesFetch)
		s.saveFailure(ctx, ws, state, key)
		return err
	}
	warnings := make([]string, 0)
	if len(ownRoutes) > maxRoutes {
		ownRoutes = ownRoutes[:maxRoutes]
		warnings = append(warnings, WarningTooManyRoutes)
	}
	routes := make([]Route, 0, len(ownRoutes))
	for _, route := range ownRoutes {
		routes = append(routes, Route{ID: route.ID, Name: route.Name, Kind: route.Kind, Status: route.Status, BaseURL: route.BaseURL, Groups: route.Groups})
	}
	if err := s.store.UpsertRoutes(ctx, ws, routes, now); err != nil {
		s.saveFailure(ctx, ws, state, ErrorRequest)
		return err
	}
	state.RouteCount = len(routes)

	days := daysToCollect(state, today)
	run := &runState{}
	advancing := true
	for _, day := range days {
		revenue, source, err := s.collectRevenue(ctx, session, routes, day, run)
		if err != nil {
			s.saveFailure(ctx, ws, state, mapFetchError(err, ErrorRevenueFetch))
			return err
		}
		if err := s.storeDay(ctx, ws, &state, day, revenue, source, now, &advancing, &warnings); err != nil {
			return err
		}
	}
	return s.finish(ctx, ws, state, warnings, now)
}

// storeDay 采集某一天的上游成本，与营收一起写入，并更新采集进度。
func (s *Service) storeDay(ctx context.Context, ws Workspace, state *SyncState, day time.Time, revenue []RouteRevenue, source string, now time.Time, advancing *bool, warnings *[]string) error {
	consumes, err := s.upstreams.SiteConsumesOnDate(ctx, ws.UserID, ws.AdminAccountID, formatDay(day), bizclock.Name)
	if err != nil {
		s.saveFailure(ctx, ws, *state, ErrorRequest)
		return err
	}
	costs := make([]SiteCost, 0, len(consumes))
	failed := 0
	for _, item := range consumes {
		costs = append(costs, SiteCost{SiteID: item.SiteID, SiteName: item.SiteName, BaseURL: item.BaseURL, Consume: item.Consume, RechargeRate: item.RechargeRate, Cost: item.Cost, Status: item.Status})
		if item.Status == upstream.ConsumeStatusFailed {
			failed++
		}
	}
	if err := s.store.ReplaceDay(ctx, ws, day, revenue, costs); err != nil {
		s.saveFailure(ctx, ws, *state, ErrorRequest)
		return err
	}
	if state.SyncedFrom == nil || day.Before(*state.SyncedFrom) {
		value := day
		state.SyncedFrom = &value
	}
	if state.SyncedTo == nil || day.After(*state.SyncedTo) {
		value := day
		state.SyncedTo = &value
	}
	if source != "" {
		state.RevenueSource = source
		if source == SourceNewAPILogStat {
			appendWarning(warnings, WarningFlowFallback)
		}
	}
	today := calendarDay(now)
	settled := !now.Before(dayStart(day).AddDate(0, 0, 1).Add(finalizeAfter))
	retry := failed > 0 && !day.Before(today.AddDate(0, 0, -retryRecentDays))
	if failed > 0 {
		appendWarning(warnings, WarningCostFailed)
	}
	if *advancing && settled && !retry {
		value := day
		state.CompleteThrough = &value
	} else {
		*advancing = false
	}
	return s.store.SaveSyncState(ctx, ws, *state)
}

func appendWarning(warnings *[]string, key string) {
	for _, existing := range *warnings {
		if existing == key {
			return
		}
	}
	*warnings = append(*warnings, key)
}

func (s *Service) finish(ctx context.Context, ws Workspace, state SyncState, warnings []string, now time.Time) error {
	if err := s.store.Prune(ctx, ws, calendarDay(now).AddDate(0, 0, -retentionDays)); err != nil {
		log.Printf("[profit] prune failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
	}
	state.Status = StatusSuccess
	state.ErrorKey = ""
	state.WarningKey = ""
	if len(warnings) > 0 {
		state.WarningKey = warnings[0]
	}
	finished := s.now()
	state.LastSyncedAt = &finished
	return s.store.SaveSyncState(ctx, ws, state)
}

// collectRevenue 读取某一天自己站点按路由、分组的营收（站点额度）。
func (s *Service) collectRevenue(ctx context.Context, session upstream.Session, routes []Route, day time.Time, run *runState) ([]RouteRevenue, string, error) {
	date := formatDay(day)
	switch session.Platform {
	case upstream.PlatformSub2API:
		total, err := s.platform.FetchAdminUsageStats(session, date, date)
		if err != nil {
			return nil, "", err
		}
		s.sleep(ctx)
		rows := make([]RouteRevenue, 0)
		if total <= 0 {
			return rows, SourceSub2APIGroups, nil
		}
		attributed := 0.0
		for _, route := range routes {
			result, err := s.platform.FetchSub2APIAccountGroupRevenue(session, route.ID, date)
			if err != nil {
				return nil, "", err
			}
			for _, item := range result {
				rows = append(rows, RouteRevenue{RouteID: route.ID, Group: item.Group, Revenue: item.Revenue, Requests: item.Requests})
				attributed += item.Revenue
			}
			s.sleep(ctx)
		}
		// 站点合计里不属于当前任何账号的部分（例如账号已删除）记为「无路由」，保证合计与仪表盘一致。
		if residual := total - attributed; residual > 1e-6 && residual > total*1e-6 {
			rows = append(rows, RouteRevenue{RouteID: "", Group: "", Revenue: residual})
		}
		return rows, SourceSub2APIGroups, nil
	case upstream.PlatformNewAPI:
		startTS, endTS := bizclock.DayBounds(dayStart(day))
		if !run.flowUnsupported {
			flow, err := s.platform.FetchNewAPIFlowRevenue(session, startTS, endTS)
			switch {
			case err == nil && len(flow) > 0:
				s.sleep(ctx)
				return convertRows(flow), SourceNewAPIFlow, nil
			case err == nil:
				// 没有数据：可能当天确实没有消费，也可能站点关闭了数据看板（不写 quota_data）。用日志合计核对。
				total, err := s.platform.FetchAdminUsageStats(session, date, date)
				if err != nil {
					return nil, "", err
				}
				s.sleep(ctx)
				if total <= 0 {
					return []RouteRevenue{}, SourceNewAPIFlow, nil
				}
				run.flowUnsupported = true
			case errors.Is(err, upstream.ErrFlowUnsupported):
				run.flowUnsupported = true
			default:
				return nil, "", err
			}
		}
		rows := make([]RouteRevenue, 0)
		for _, route := range routes {
			groups := route.Groups
			if len(groups) <= 1 {
				group := ""
				if len(groups) == 1 {
					group = groups[0]
				}
				revenue, err := s.platform.FetchNewAPIChannelRevenue(session, route.ID, "", startTS, endTS)
				if err != nil {
					return nil, "", err
				}
				if revenue > 0 {
					rows = append(rows, RouteRevenue{RouteID: route.ID, Group: group, Revenue: revenue})
				}
				s.sleep(ctx)
				continue
			}
			for _, group := range groups {
				revenue, err := s.platform.FetchNewAPIChannelRevenue(session, route.ID, group, startTS, endTS)
				if err != nil {
					return nil, "", err
				}
				if revenue > 0 {
					rows = append(rows, RouteRevenue{RouteID: route.ID, Group: group, Revenue: revenue})
				}
				s.sleep(ctx)
			}
		}
		return rows, SourceNewAPILogStat, nil
	}
	return nil, "", requestError(ErrorPlatform)
}

func convertRows(items []upstream.RouteGroupRevenue) []RouteRevenue {
	rows := make([]RouteRevenue, 0, len(items))
	for _, item := range items {
		rows = append(rows, RouteRevenue{RouteID: item.RouteID, Group: item.Group, Revenue: item.Revenue, Requests: item.Requests})
	}
	return rows
}

// StartScheduler 启动后台采集：重启前未完成的采集标记为中断；服务启动 startupDelay 后在后台继续这些采集，
// 并为新工作区开始首次采集；此后每小时（hh:10:30）为所有工作区补齐数据。
func (s *Service) StartScheduler(ctx context.Context) {
	if err := s.store.ResetRunningSyncs(ctx); err != nil {
		log.Printf("[profit] reset running syncs failed: %v", err)
	}
	go func() {
		wait, startup := startupDelay, true
		for {
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			s.syncAll(ctx, startup)
			wait, startup = untilNextSlot(s.now()), false
		}
	}()
}

// untilNextSlot 返回距离下一个 hh:10:30 的时长。
func untilNextSlot(now time.Time) time.Duration {
	next := now.Truncate(syncInterval).Add(scheduleOffset)
	if !next.After(now) {
		next = next.Add(syncInterval)
	}
	return next.Sub(now)
}

// syncAll 是一轮后台采集：被重启打断的采集从中断处继续（resume）；其余采集过的工作区例行补齐（schedule，
// 服务刚启动的那一轮跳过，留给整点）；保存了管理员会话、还没采集过的工作区自动开始首次采集（auto）。
func (s *Service) syncAll(ctx context.Context, startup bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[profit] scheduler panic recovered: %v", r)
		}
	}()
	workspaces, err := s.store.ListSyncedWorkspaces(ctx)
	if err != nil {
		log.Printf("[profit] list workspaces failed: %v", err)
		return
	}
	known := make(map[Workspace]bool, len(workspaces))
	for _, ws := range workspaces {
		known[ws] = true
		if ctx.Err() != nil {
			return
		}
		state, _, err := s.store.GetSyncState(ctx, ws)
		if err != nil {
			log.Printf("[profit] load sync state failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
			continue
		}
		startedBy := StartedBySchedule
		if state.Status == StatusInterrupted {
			startedBy = StartedByResume
		} else if startup {
			continue
		}
		if err := s.SyncWorkspaceAs(ctx, ws, startedBy); err != nil && !isRequestErrorKey(err, ErrorSyncInProgress) {
			log.Printf("[profit] background sync failed admin_account_id=%s started_by=%s err=%v", ws.AdminAccountID, startedBy, err)
		}
	}
	if s.workspaces == nil {
		return
	}
	candidates, err := s.workspaces.SessionWorkspaces(ctx)
	if err != nil {
		log.Printf("[profit] list session workspaces failed: %v", err)
		return
	}
	for _, ws := range candidates {
		if known[ws] {
			continue
		}
		if ctx.Err() != nil {
			return
		}
		// 会话失效或平台不支持时不创建采集状态，下一轮再试；这类情况不记日志，避免每小时刷屏。
		err := s.SyncWorkspaceAs(ctx, ws, StartedByAuto)
		if err != nil && !isRequestErrorKey(err, ErrorSyncInProgress) && !isRequestErrorKey(err, ErrorAuthRequired) && !isRequestErrorKey(err, ErrorPlatform) {
			log.Printf("[profit] first sync failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
		}
	}
}

func isRequestErrorKey(err error, key string) bool {
	var target requestError
	return errors.As(err, &target) && string(target) == key
}

// mapSessionError 把站点会话错误归为「需要重新登录站点」或通用错误（不返回 401）。
func mapSessionError(err error) error {
	message := err.Error()
	if strings.Contains(message, "authRequired") || strings.Contains(message, "adminOnly") || strings.Contains(message, upstream.ErrorAuth) {
		return requestError(ErrorAuthRequired)
	}
	if strings.Contains(message, upstream.ErrorNetwork) {
		return requestError(ErrorNetwork)
	}
	return requestError(ErrorRequest)
}

func mapFetchError(err error, fallback string) string {
	message := err.Error()
	switch {
	case strings.Contains(message, upstream.ErrorAuth):
		return ErrorAuthRequired
	case strings.Contains(message, upstream.ErrorNetwork):
		return ErrorNetwork
	}
	return fallback
}

// SyncStatus 返回工作区当前的采集状态。
func (s *Service) SyncStatus(ctx context.Context, ws Workspace) (SyncStatus, error) {
	state, _, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		return SyncStatus{}, err
	}
	return statusOf(state), nil
}

func statusOf(state SyncState) SyncStatus {
	status := SyncStatus{
		Platform:      state.Platform,
		Status:        state.Status,
		ErrorKey:      state.ErrorKey,
		WarningKey:    state.WarningKey,
		RevenueSource: state.RevenueSource,
		RouteCount:    state.RouteCount,
		LastStartedAt: state.LastStartedAt,
		LastSyncedAt:  state.LastSyncedAt,
		StartedBy:     state.StartedBy,
	}
	if state.SyncedFrom != nil {
		status.SyncedFrom = formatDay(*state.SyncedFrom)
	}
	if state.SyncedTo != nil {
		status.SyncedTo = formatDay(*state.SyncedTo)
	}
	if state.CompleteThrough != nil {
		status.CompleteThrough = formatDay(*state.CompleteThrough)
	}
	return status
}
