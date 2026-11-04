package user_activity

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"time"

	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/bizclock"
)

// BusinessTimezone 是统计自然日使用的时区，即统一的业务时区（见 bizclock）。
const BusinessTimezone = bizclock.Name

const (
	defaultPageSize   = 20
	maxPageSize       = 100
	defaultSeriesDays = 60
	maxSeriesDays     = 400
	detailSeriesDays  = 90
	maxSiteUserIDLen  = 128
	// MaxRecipients 与群发邮件的单批上限保持一致。
	MaxRecipients = 10000
)

// inactiveBucketDays 是概览中「超过 N 天未使用」统计的档位。
var inactiveBucketDays = []int{2, 7, 14, 30, 60, 90}

// SessionProvider 提供当前工作区「我的站点」的管理员会话（my_sites.Service 实现，会自动刷新令牌）。
type SessionProvider interface {
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}

// PlatformClient 是同步用到的站点接口（upstream.PlatformService 实现）。
type PlatformClient interface {
	FetchSub2APIAdminUsersPage(session upstream.Session, query upstream.Sub2APIAdminUsersQuery) (upstream.Sub2APIAdminUsersPage, error)
	FetchSub2APIUserUsageTrend(session upstream.Session, startDate string, endDate string, timezone string, limit int) ([]upstream.Sub2APIUserUsageTrendPoint, error)
	FetchNewAPIAdminUsersPage(session upstream.Session, page int, pageSize int) (upstream.NewAPIAdminUsersPage, error)
	FetchNewAPIUserQuotaData(session upstream.Session, start time.Time, end time.Time) ([]upstream.NewAPIUserQuotaBucket, error)
	FetchSub2APIUsedRedeemRecords(session upstream.Session, codeType string, page int, pageSize int) (upstream.Sub2APIRedeemRecordsPage, error)
	FetchSub2APIAffiliateTransfers(session upstream.Session, startDate string, endDate string, timezone string, page int, pageSize int) (upstream.Sub2APIAffiliateTransfersPage, error)
}

// AccountResolver 解析当前 admin 工作区。
type AccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

type store interface {
	ReplaceSnapshot(ctx context.Context, ws Workspace, snapshot Snapshot) error
	GetSyncState(ctx context.Context, ws Workspace) (SyncState, bool, error)
	SaveSyncState(ctx context.Context, ws Workspace, state SyncState) error
	ResetRunningSyncs(ctx context.Context, errorKey string) error
	ListSyncedWorkspaces(ctx context.Context) ([]Workspace, error)
	ProfileCounts(ctx context.Context, ws Workspace, now time.Time, days []int) (ProfileCounts, error)
	PeriodMetrics(ctx context.Context, ws Workspace, siteUserID string, from time.Time, to time.Time, loc *time.Location) (PeriodMetrics, error)
	PeriodRetention(ctx context.Context, ws Workspace, current [2]time.Time, previous [2]time.Time) (Retention, error)
	DailySeries(ctx context.Context, ws Workspace, siteUserID string, from time.Time, to time.Time) ([]DailyPoint, error)
	SuppressionCount(ctx context.Context, ws Workspace) (int, error)
	ListUsers(ctx context.Context, ws Workspace, windows Windows, filter UserFilter, sortKey string, order string, limit int, offset int) ([]UserRow, int, error)
	RecipientPreview(ctx context.Context, ws Workspace, windows Windows, filter UserFilter) (RecipientPreview, error)
	RechargeDay(ctx context.Context, ws Workspace, day time.Time) (RechargeDaySummary, error)
	BalanceTotals(ctx context.Context, ws Workspace, now time.Time) (BalanceTotals, error)
	SiteBalance(ctx context.Context, ws Workspace, day time.Time) (*float64, error)
}

// Service 负责同步与分析。
type Service struct {
	store    store
	sessions SessionProvider
	platform PlatformClient
	accounts AccountResolver
	loc      *time.Location
	tzName   string
	now      func() time.Time
	// pause 是连续请求站点接口之间的间隔，避免同步时给自己的站点造成压力。
	pause time.Duration

	// workspaces 为 nil 时定时任务只同步已经同步过的工作区，不自动开始首次同步。
	workspaces WorkspaceLister

	mu         sync.Mutex
	running    map[Workspace]struct{}
	background sync.WaitGroup
}

// WorkspaceLister 列出保存了自己站点管理员会话的工作区，定时任务据此为还没同步过的工作区在后台自动开始首次同步。
type WorkspaceLister interface {
	SessionWorkspaces(ctx context.Context) ([]Workspace, error)
}

// SetWorkspaceLister 让定时任务为保存了管理员会话、但还没同步过的工作区在后台自动开始首次同步。
func (s *Service) SetWorkspaceLister(lister WorkspaceLister) {
	s.workspaces = lister
}

// SyncStatusOf 返回工作区当前的同步状态。
func (s *Service) SyncStatusOf(ctx context.Context, ws Workspace) (SyncStatus, error) {
	state, ok, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		return SyncStatus{}, err
	}
	return s.syncStatus(state, ok), nil
}

func NewService(repository *Repository, sessions SessionProvider, platform PlatformClient, accounts AccountResolver) *Service {
	return newService(repository, sessions, platform, accounts)
}

func newService(store store, sessions SessionProvider, platform PlatformClient, accounts AccountResolver) *Service {
	loc := bizclock.Location()
	return &Service{
		store:    store,
		sessions: sessions,
		platform: platform,
		accounts: accounts,
		loc:      loc,
		tzName:   BusinessTimezone,
		now:      time.Now,
		pause:    defaultRequestPause,
		running:  map[Workspace]struct{}{},
	}
}

// CurrentWorkspace 返回用户当前选中的 admin 工作区。
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

// Summary 返回概览：用户数、未活跃分档、今日数据、日/周/月对比和每日趋势。
func (s *Service) Summary(ctx context.Context, ws Workspace, seriesDays int) (SummaryResponse, error) {
	if seriesDays < 7 || seriesDays > maxSeriesDays {
		seriesDays = defaultSeriesDays
	}
	state, ok, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		return SummaryResponse{}, err
	}
	response := SummaryResponse{
		Sync:     s.syncStatus(state, ok),
		Timezone: s.tzName,
		Inactive: []InactiveBucket{},
		Periods:  map[string]PeriodComparison{},
		Daily:    []DailyPoint{},
	}
	if !ok || state.SyncedTo == nil {
		return response, nil
	}
	response.HasData = true
	now := s.now()
	today := s.dayOf(now)
	counts, err := s.store.ProfileCounts(ctx, ws, now, inactiveBucketDays)
	if err != nil {
		return SummaryResponse{}, err
	}
	response.TotalUsers = counts.Total
	response.Disabled = counts.Disabled
	response.WithEmail = counts.WithEmail
	response.NoUsage = counts.NoUsage
	response.Inactive = counts.Inactive
	if response.Today, err = s.store.PeriodMetrics(ctx, ws, "", today, today, s.loc); err != nil {
		return SummaryResponse{}, err
	}
	if response.Periods, err = s.periods(ctx, ws, "", state.SyncedFrom, today); err != nil {
		return SummaryResponse{}, err
	}
	if response.Daily, err = s.store.DailySeries(ctx, ws, "", today.AddDate(0, 0, -(seriesDays-1)), today); err != nil {
		return SummaryResponse{}, err
	}
	maskUncoveredRecharge(response.Daily, state)
	var yesterdayCost *float64
	if day, ok := response.Periods["day"]; ok && covered(state.SyncedFrom, today.AddDate(0, 0, -1)) {
		cost := day.Current.Cost
		yesterdayCost = &cost
	}
	if response.Money, err = s.moneySummary(ctx, ws, state, now, today, response.Today.Cost, yesterdayCost); err != nil {
		return SummaryResponse{}, err
	}
	if response.Suppressions, err = s.store.SuppressionCount(ctx, ws); err != nil {
		return SummaryResponse{}, err
	}
	return response, nil
}

type periodSpec struct {
	key  string
	days int
}

var periodSpecs = []periodSpec{{key: "day", days: 1}, {key: "week", days: 7}, {key: "month", days: 30}}

// periods 计算日/周/月三组「本期 / 上期 / 去年同期」。本地数据（coverageFrom 之后）覆盖不到的区间返回 nil，
// 避免把「没有同步过」误当成「用量为 0」。siteUserID 为空时统计全部用户并附带留存/流失人数。
func (s *Service) periods(ctx context.Context, ws Workspace, siteUserID string, coverageFrom *time.Time, today time.Time) (map[string]PeriodComparison, error) {
	yesterday := today.AddDate(0, 0, -1)
	result := make(map[string]PeriodComparison, len(periodSpecs))
	for _, spec := range periodSpecs {
		currentTo := yesterday
		currentFrom := yesterday.AddDate(0, 0, -(spec.days - 1))
		previousTo := currentFrom.AddDate(0, 0, -1)
		previousFrom := previousTo.AddDate(0, 0, -(spec.days - 1))
		lastYearFrom := currentFrom.AddDate(-1, 0, 0)
		lastYearTo := currentTo.AddDate(-1, 0, 0)

		comparison := PeriodComparison{Days: spec.days}
		current, err := s.store.PeriodMetrics(ctx, ws, siteUserID, currentFrom, currentTo, s.loc)
		if err != nil {
			return nil, err
		}
		comparison.Current = current
		if covered(coverageFrom, previousFrom) {
			previous, err := s.store.PeriodMetrics(ctx, ws, siteUserID, previousFrom, previousTo, s.loc)
			if err != nil {
				return nil, err
			}
			comparison.Previous = &previous
			if siteUserID == "" {
				retention, err := s.store.PeriodRetention(ctx, ws, [2]time.Time{currentFrom, currentTo}, [2]time.Time{previousFrom, previousTo})
				if err != nil {
					return nil, err
				}
				comparison.Retention = &retention
			}
		}
		if covered(coverageFrom, lastYearFrom) {
			lastYear, err := s.store.PeriodMetrics(ctx, ws, siteUserID, lastYearFrom, lastYearTo, s.loc)
			if err != nil {
				return nil, err
			}
			comparison.LastYear = &lastYear
		}
		result[spec.key] = comparison
	}
	return result, nil
}

func covered(coverageFrom *time.Time, from time.Time) bool {
	return coverageFrom != nil && !coverageFrom.After(from)
}

// moneySummary 汇总资金概览：今日入账（按渠道）、昨日入账、今日 / 昨日消耗、全站用户余额及较昨日末的变化。
// 入账只在本地入账数据覆盖到的日期上计算，没有覆盖时对应字段为空，前端显示「—」而不是 0。
func (s *Service) moneySummary(ctx context.Context, ws Workspace, state SyncState, now time.Time, today time.Time, todayCost float64, yesterdayCost *float64) (MoneySummary, error) {
	money := MoneySummary{TodayChannels: []ChannelAmount{}, TodayCost: todayCost, YesterdayCost: yesterdayCost, RechargeSince: state.RechargeSince}
	switch upstream.Platform(state.Platform) {
	case upstream.PlatformSub2API:
		money.Method = RechargeMethodRecords
	case upstream.PlatformNewAPI:
		money.Method = RechargeMethodSnapshot
	}
	yesterday := today.AddDate(0, 0, -1)
	if state.RechargeSyncedFrom != nil {
		money.RechargeFrom = formatDay(*state.RechargeSyncedFrom)
		money.RechargeUpToDate = state.RechargeSyncedTo != nil && !state.RechargeSyncedTo.Before(today)
		todaySummary, err := s.store.RechargeDay(ctx, ws, today)
		if err != nil {
			return MoneySummary{}, err
		}
		money.TodayRecharge = todaySummary.Positive
		money.TodayDeduction = -todaySummary.Negative
		money.TodayRechargeUsers = todaySummary.Users
		money.TodayChannels = todaySummary.Channels
		// new-api 第一天只统计了首次同步之后的部分，不能当作「昨日全天」。
		yesterdayComplete := !state.RechargeSyncedFrom.After(yesterday)
		if money.Method == RechargeMethodSnapshot {
			yesterdayComplete = state.RechargeSyncedFrom.Before(yesterday)
		}
		if yesterdayComplete {
			yesterdaySummary, err := s.store.RechargeDay(ctx, ws, yesterday)
			if err != nil {
				return MoneySummary{}, err
			}
			amount := yesterdaySummary.Positive
			money.YesterdayRecharge = &amount
		}
	}
	totals, err := s.store.BalanceTotals(ctx, ws, now)
	if err != nil {
		return MoneySummary{}, err
	}
	money.TotalBalance = totals.TotalBalance
	money.UsersWithBalance = totals.UsersWithBalance
	money.LifetimeRecharged = totals.LifetimeRecharged
	money.LowBalanceActive = totals.LowBalanceActive
	if money.BalanceYesterday, err = s.store.SiteBalance(ctx, ws, yesterday); err != nil {
		return MoneySummary{}, err
	}
	return money, nil
}

// maskUncoveredRecharge 把本地入账数据没有覆盖的日期置空（图表断开，而不是显示为 0）。
func maskUncoveredRecharge(points []DailyPoint, state SyncState) {
	var from, to string
	if state.RechargeSyncedFrom != nil {
		from = formatDay(*state.RechargeSyncedFrom)
	}
	if state.RechargeSyncedTo != nil {
		to = formatDay(*state.RechargeSyncedTo)
	}
	for i := range points {
		if from == "" || points[i].Day < from || (to != "" && points[i].Day > to) {
			points[i].Recharge = nil
		}
	}
}

// ListUsers 按筛选条件分页返回用户。
func (s *Service) ListUsers(ctx context.Context, ws Workspace, query UserListQuery) (UserListResponse, error) {
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	filter := sanitizeFilter(query.Filter)
	rows, total, err := s.store.ListUsers(ctx, ws, s.windows(), filter, query.Sort, query.Order, pageSize, (page-1)*pageSize)
	if err != nil {
		return UserListResponse{}, err
	}
	return UserListResponse{Items: rows, Total: total, Page: page, PageSize: pageSize, Pages: int(math.Ceil(float64(total) / float64(pageSize)))}, nil
}

// UserDetail 返回单个用户的近期用量、每日趋势与日/周/月对比。
func (s *Service) UserDetail(ctx context.Context, ws Workspace, siteUserID string) (UserDetailResponse, error) {
	id := strings.TrimSpace(siteUserID)
	if id == "" || len(id) > maxSiteUserIDLen {
		return UserDetailResponse{}, requestError(ErrorNotFound)
	}
	windows := s.windows()
	rows, _, err := s.store.ListUsers(ctx, ws, windows, UserFilter{SiteUserID: id}, SortLastUsedAt, "desc", 1, 0)
	if err != nil {
		return UserDetailResponse{}, err
	}
	if len(rows) == 0 {
		return UserDetailResponse{}, requestError(ErrorNotFound)
	}
	state, _, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		return UserDetailResponse{}, err
	}
	daily, err := s.store.DailySeries(ctx, ws, id, windows.Today.AddDate(0, 0, -(detailSeriesDays-1)), windows.Today)
	if err != nil {
		return UserDetailResponse{}, err
	}
	maskUncoveredRecharge(daily, state)
	periods, err := s.periods(ctx, ws, id, state.SyncedFrom, windows.Today)
	if err != nil {
		return UserDetailResponse{}, err
	}
	return UserDetailResponse{User: rows[0], Daily: daily, Periods: periods}, nil
}

// PreviewRecipients 统计筛选结果中可以收到邮件的人数。
func (s *Service) PreviewRecipients(ctx context.Context, ws Workspace, filter UserFilter) (RecipientPreview, error) {
	return s.store.RecipientPreview(ctx, ws, s.windows(), sanitizeFilter(filter))
}

// Recipients 按筛选条件返回有邮箱的用户（最多 max 个，超过时报错而不是静默截断），供群发邮件使用。
// 退订名单由群发邮件模块统一过滤。
func (s *Service) Recipients(ctx context.Context, ws Workspace, filter UserFilter, max int) ([]Recipient, error) {
	if max < 1 || max > MaxRecipients {
		max = MaxRecipients
	}
	state, ok, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		return nil, err
	}
	if !ok || state.SyncedTo == nil {
		return nil, requestError(ErrorNotSynced)
	}
	filter = sanitizeFilter(filter)
	filter.HasEmail = true
	rows, total, err := s.store.ListUsers(ctx, ws, s.windows(), filter, SortLastUsedAt, "desc", max+1, 0)
	if err != nil {
		return nil, err
	}
	if total > max || len(rows) > max {
		return nil, requestError(ErrorTooManyRecipient)
	}
	recipients := make([]Recipient, 0, len(rows))
	for _, row := range rows {
		recipients = append(recipients, Recipient{SiteUserID: row.SiteUserID, Email: row.Email, Username: row.Username})
	}
	return recipients, nil
}

// IsRequestError 判断错误是否为可直接返回给前端的业务错误。
func IsRequestError(err error) bool {
	var target requestError
	return errors.As(err, &target)
}

func sanitizeFilter(filter UserFilter) UserFilter {
	filter.Segment = strings.TrimSpace(filter.Segment)
	filter.Search = strings.TrimSpace(filter.Search)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.Role = strings.TrimSpace(filter.Role)
	filter.SiteUserID = ""
	return filter
}

func (s *Service) windows() Windows {
	now := s.now()
	return NewWindows(now, s.dayOf(now))
}

// dayOf 返回 t 在业务时区所属的自然日（以 UTC 零点表示，用于 date 列）。
func (s *Service) dayOf(t time.Time) time.Time {
	local := t.In(s.loc)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func (s *Service) syncStatus(state SyncState, ok bool) SyncStatus {
	status := SyncStatus{Platform: state.Platform, Status: state.Status, ErrorKey: state.ErrorKey, UserCount: state.UserCount,
		LastStartedAt: state.LastStartedAt, LastSyncedAt: state.LastSyncedAt, WarningKey: state.WarningKey, StartedBy: state.StartedBy}
	if !ok {
		status.Status = ""
	}
	if state.SyncedFrom != nil {
		status.SyncedFrom = formatDay(*state.SyncedFrom)
	}
	if state.SyncedTo != nil {
		status.SyncedTo = formatDay(*state.SyncedTo)
	}
	status.HistoryLimitDays = historyLimitDays(upstream.Platform(state.Platform))
	return status
}
