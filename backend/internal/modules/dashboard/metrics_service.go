package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/bizclock"
)

// UpstreamLister 抽象上游站点读取，由 upstream.Service 实现。
// List 用于用户请求路径（自动使用当前工作区），
// ListForAccount 用于后台调度等需要显式指定工作区的内部流程。
type UpstreamLister interface {
	List(ctx context.Context, userID string) []upstream.Response
	ListForAccount(ctx context.Context, userID, adminAccountID string) []upstream.Response
	// KeyUsageToday 和 BalanceBreakdown 是「今日成本」「上游总余额」下钻弹窗的数据源，
	// 由 upstream.Service 实现（持有 session/cache，能校验站点归属和当前工作区）。
	KeyUsageToday(ctx context.Context, userID string) ([]upstream.KeyUsageTodayItem, error)
	BalanceBreakdown(ctx context.Context, userID string) ([]upstream.BalanceBreakdownItem, error)
	// RefreshStaleSites 补同步今天（since 之后）还没有同步成功的站点，最多等待 wait。
	RefreshStaleSites(ctx context.Context, userID string, adminAccountID string, since time.Time, wait time.Duration)
	// ConsumeOnDate 直接按日期查询各站点某天的消费 × 充值倍率（历史快照定稿用）。
	ConsumeOnDate(ctx context.Context, userID string, adminAccountID string, date string, timezone string) (upstream.DailyConsume, error)
}

// metricsStore 是 MetricsRepository 在本服务中用到的方法（便于测试替换）。
type metricsStore interface {
	Upsert(ctx context.Context, snapshot DailySnapshot) error
	ListRange(ctx context.Context, userID, adminAccountID string, today time.Time, days int) ([]DailySnapshot, error)
	GetBalanceFilter(ctx context.Context, userID, adminAccountID string) (BalanceFilterConfig, error)
	SaveBalanceFilter(ctx context.Context, config BalanceFilterConfig) error
	GetPricing(ctx context.Context, userID, adminAccountID string) (PricingConfig, error)
	SavePricing(ctx context.Context, config PricingConfig) error
	FinalizeSnapshot(ctx context.Context, snapshot DailySnapshot, keepBalances bool) error
	UnfinalizedDates(ctx context.Context, userID, adminAccountID string, from time.Time, to time.Time) ([]time.Time, error)
	IsFinalized(ctx context.Context, userID, adminAccountID string, date time.Time) (bool, error)
}

const (
	// dashboardTimezone 是仪表盘「今天 / 昨天」的时区，即统一的业务时区（见 bizclock）。
	dashboardTimezone = bizclock.Name
	// staleRefreshWait 是打开仪表盘时等待补同步的最长时间，超时的站点先不计入，同步在后台继续。
	staleRefreshWait = 8 * time.Second
	// finalizeInterval 是定稿任务的检查间隔：零点后尽快定稿昨天；遇到站点暂时查询失败时下一轮重试。
	finalizeInterval     = time.Hour
	startupFinalizeDelay = time.Minute
	// finalizeLookbackDays 内已有快照但还没定稿的日期都会尝试定稿；finalizeRetryDays 内遇到站点查询失败时
	// 先不定稿、下一轮重试，更早的日期则用能查到的站点数据定稿，避免一直停留在白天的部分数据。
	finalizeLookbackDays = 7
	finalizeRetryDays    = 2
	// endOfDayWindow 内定稿昨天时，此刻的余额可以当作昨天结束时的余额；更晚则保留快照里已有的余额。
	endOfDayWindow   = 2 * time.Hour
	maxSiteUnitPrice = 10000
)

// MetricsService 负责仪表盘指标的实时计算、历史快照存储与定稿调度。
// 与同包的 Service（admin 会话管理）职责分离，共享 SessionStore 和 PlatformClient。
type MetricsService struct {
	store       SessionStore
	platform    PlatformClient
	upstreams   UpstreamLister
	metricsRepo metricsStore
	accounts    AdminAccountService
	sessionSync MySiteStateSync
	now         func() time.Time
}

func (s *MetricsService) SetMySiteSync(sync MySiteStateSync) {
	s.sessionSync = sync
}

func (s *MetricsService) freshAdminSession(ctx context.Context, userID string, adminAccountID string, record *AdminSession) (upstream.Session, error) {
	if s.sessionSync != nil {
		stored, exists, err := s.sessionSync.StoredSession(ctx, userID, adminAccountID)
		if err != nil {
			return upstream.Session{}, err
		}
		if !exists || sessionAppearsNewer(record.Session, stored) {
			if err := s.sessionSync.SyncAdminSession(ctx, userID, adminAccountID, record.Session, record.Identity); err != nil {
				return upstream.Session{}, err
			}
		}
		canonical, err := s.sessionSync.RequireSession(ctx, userID, adminAccountID)
		if err != nil {
			return upstream.Session{}, err
		}
		if !sessionEqual(canonical, record.Session) {
			record.Session = canonical
			record.LastRefreshedAt = nowMillis()
			if err := s.store.Save(ctx, userID, adminAccountID, *record); err != nil {
				return upstream.Session{}, err
			}
		}
		return canonical, nil
	}

	refreshed, err := s.platform.RefreshSession(record.Session)
	if err != nil {
		return upstream.Session{}, err
	}
	if !sessionEqual(refreshed, record.Session) {
		record.Session = refreshed
		record.LastRefreshedAt = nowMillis()
		if err := s.store.Save(ctx, userID, adminAccountID, *record); err != nil {
			return upstream.Session{}, err
		}
	}
	return refreshed, nil
}

func NewMetricsService(store SessionStore, platform PlatformClient, upstreams UpstreamLister, metricsRepo metricsStore, accounts AdminAccountService) *MetricsService {
	return &MetricsService{store: store, platform: platform, upstreams: upstreams, metricsRepo: metricsRepo, accounts: accounts, now: time.Now}
}

// LiveMetrics 实时计算五项核心指标并返回。
// 同时将当天的指标作为快照 upsert 到数据库（零点后由定稿任务用全天数据覆盖），确保趋势图数据持续积累。
//
// 计算逻辑（金额均为人民币）：
//   - todayProfit:     我的站点今日总实际消费 × 我的站点售价
//   - siteBalance:     我的站点所有非 admin 用户余额之和 × 我的站点售价
//   - todayPurchase:   今天已同步成功的上游站点「今日消费」× 充值倍率之和；今天还没同步成功的站点先补同步，
//     仍未成功的不计入（它们缓存里是之前某天的值），在 costStaleSites 中报告
//   - upstreamBalance: 所有上游站点余额 × 充值倍率之和（读取缓存）
//   - netProfit:       todayProfit - todayPurchase
func (s *MetricsService) LiveMetrics(ctx context.Context, userID string) (MetricsResponse, error) {
	// 获取并校验 admin 会话（平台感知：sub2api 检查 AccessToken，new-api 检查 Cookie+UserID）。
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return MetricsResponse{}, err
	}
	record, err := s.store.Get(ctx, userID, adminAccountID)
	if err != nil {
		return MetricsResponse{}, err
	}
	if record == nil || !record.Session.IsAuthenticated() {
		return MetricsResponse{}, requestError(ErrorAdminOnly)
	}

	// 如有必要先刷新令牌（new-api 不使用 refresh token，RefreshSession 会直接返回原会话）。
	session, err := s.freshAdminSession(ctx, userID, adminAccountID, record)
	if err != nil {
		return MetricsResponse{}, requestError(ErrorAdminOnly)
	}

	// 校验 admin 角色（平台中性）。
	if err := s.platform.VerifyAdmin(session); err != nil {
		return MetricsResponse{}, requestError(ErrorAdminOnly)
	}

	// 并行获取四项独立数据：今日盈利、站点余额、分组数量、上游指标。
	// 各 goroutine 出错只记日志、降级为零值，不阻塞整体返回。
	now := s.now().In(dashboardLocation())
	today := now.Format(bizclock.DateLayout)
	todayStart := bizclock.StartOfDay(now)
	price := s.siteUnitPrice(ctx, userID, adminAccountID)
	var (
		todayProfit float64
		siteBalance float64
		groupCount  int
		cost        upstreamCost
		wg          sync.WaitGroup
	)

	// goroutine 1: 今日盈利额度（平台中性）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		profit, err := s.platform.FetchAdminUsageStats(session, today, today)
		if err != nil {
			log.Printf("dashboard metrics: fetch usage stats failed user_id=%s err=%v", userID, err)
			return
		}
		todayProfit = profit * price
	}()

	// goroutine 2: 站点用户总余额（平台中性）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		filterConfig := BalanceFilterConfig{ExcludeAdmin: true, ExcludeBalances: []float64{}}
		if s.metricsRepo != nil {
			loaded, err := s.metricsRepo.GetBalanceFilter(ctx, userID, adminAccountID)
			if err != nil {
				log.Printf("dashboard metrics: load balance filter failed user_id=%s err=%v, using defaults", userID, err)
			} else {
				filterConfig = loaded
			}
		}
		balanceResult, err := s.platform.FetchAdminSiteBalanceFiltered(session, upstream.BalanceFilter{
			ExcludeAdmin:    filterConfig.ExcludeAdmin,
			ExcludeBalances: filterConfig.ExcludeBalances,
		})
		if err != nil {
			log.Printf("dashboard metrics: fetch site balance failed user_id=%s err=%v", userID, err)
			return
		}
		siteBalance = balanceResult.Balance * price
	}()

	// goroutine 3: 管理员站点分组数量（平台中性）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		groups, err := s.platform.FetchAdminAllGroups(session)
		if err != nil {
			log.Printf("dashboard metrics: fetch admin groups failed user_id=%s err=%v", userID, err)
			return
		}
		groupCount = len(groups)
	}()

	// goroutine 4: 今日成本与上游总余额。先补同步今天还没同步成功的站点（最多等 staleRefreshWait），
	// 再读取缓存；仍未同步的站点不计入今日成本。
	wg.Add(1)
	go func() {
		defer wg.Done()
		s.upstreams.RefreshStaleSites(ctx, userID, adminAccountID, todayStart, staleRefreshWait)
		cost = summarizeUpstreamCost(s.upstreams.ListForAccount(ctx, userID, adminAccountID), todayStart)
	}()

	wg.Wait()

	result := MetricsResponse{
		TodayProfit:     todayProfit,
		SiteBalance:     siteBalance,
		TodayPurchase:   cost.Purchase,
		NetProfit:       todayProfit - cost.Purchase,
		UpstreamBalance: cost.Balance,
		GroupCount:      groupCount,
		SiteUnitPrice:   price,
		CostSites:       cost.Sites,
		CostStaleSites:  cost.Stale,
		CostSyncedAt:    cost.OldestSync,
	}

	// 将当天指标 upsert 到数据库，即使部分指标获取失败也保存已有数据，
	// 后续调用会用更完整的数据覆盖。
	s.upsertSnapshot(ctx, userID, adminAccountID, today, result)

	return result, nil
}

// Trends 查询历史趋势数据，返回最近 days 天的每日快照（不含当天）。
// 当天的数据由前端通过 LiveMetrics 获取后追加到序列末尾。
func (s *MetricsService) Trends(ctx context.Context, userID string, days int) (TrendResponse, error) {
	if days != 7 && days != 30 {
		days = 7
	}
	// 按当前工作区过滤趋势数据。
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return TrendResponse{}, err
	}
	snapshots, err := s.metricsRepo.ListRange(ctx, userID, adminAccountID, time.Now(), days)
	if err != nil {
		return TrendResponse{}, err
	}
	points := make([]TrendPoint, 0, len(snapshots))
	for _, snap := range snapshots {
		points = append(points, TrendPoint{
			Date:            snap.Date.Format("2006-01-02"),
			TodayProfit:     snap.TodayProfit,
			SiteBalance:     snap.SiteBalance,
			TodayPurchase:   snap.TodayPurchase,
			NetProfit:       snap.NetProfit,
			UpstreamBalance: snap.UpstreamBalance,
		})
	}
	return TrendResponse{Points: points}, nil
}

// upstreamCost 是从上游站点缓存汇总出的今日成本与余额。
type upstreamCost struct {
	Purchase   float64
	Balance    float64
	Sites      int
	Stale      int
	OldestSync *int64
}

// summarizeUpstreamCost 汇总今日成本：只计入 since（今天零点）之后同步成功过的站点；
// 之前同步的站点缓存里的「今日消费」属于之前某天，计为 Stale 不计入。余额不按天重置，所有站点都计入。
func summarizeUpstreamCost(sites []upstream.Response, since time.Time) upstreamCost {
	var cost upstreamCost
	for _, site := range sites {
		if site.RechargeRate <= 0 {
			continue
		}
		if site.Metrics.Balance.Value != nil {
			cost.Balance += *site.Metrics.Balance.Value * site.RechargeRate
		}
		if !upstream.SyncedSince(site.LastSyncedAt, since) {
			cost.Stale++
			continue
		}
		cost.Sites++
		if site.Metrics.TodayConsume.Value != nil {
			cost.Purchase += *site.Metrics.TodayConsume.Value * site.RechargeRate
		}
		if cost.OldestSync == nil || *site.LastSyncedAt < *cost.OldestSync {
			synced := *site.LastSyncedAt
			cost.OldestSync = &synced
		}
	}
	return cost
}

// siteUnitPrice 读取工作区的「我的站点售价」，读取失败或未配置时按 1 元换算。
func (s *MetricsService) siteUnitPrice(ctx context.Context, userID string, adminAccountID string) float64 {
	if s.metricsRepo == nil {
		return DefaultSiteUnitPrice
	}
	pricing, err := s.metricsRepo.GetPricing(ctx, userID, adminAccountID)
	if err != nil || pricing.SiteUnitPrice <= 0 {
		if err != nil {
			log.Printf("dashboard metrics: load pricing failed user_id=%s err=%v", userID, err)
		}
		return DefaultSiteUnitPrice
	}
	return pricing.SiteUnitPrice
}

// GetPricing 返回当前工作区的售价配置。
func (s *MetricsService) GetPricing(ctx context.Context, userID string) (PricingConfig, error) {
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return PricingConfig{}, err
	}
	return PricingConfig{UserID: userID, AdminAccountID: adminAccountID, SiteUnitPrice: s.siteUnitPrice(ctx, userID, adminAccountID)}, nil
}

// SavePricing 保存当前工作区的售价配置（每 1 个站点余额单位卖多少元）。
func (s *MetricsService) SavePricing(ctx context.Context, userID string, siteUnitPrice float64) (PricingConfig, error) {
	if math.IsNaN(siteUnitPrice) || math.IsInf(siteUnitPrice, 0) || siteUnitPrice <= 0 || siteUnitPrice > maxSiteUnitPrice {
		return PricingConfig{}, requestError(ErrorInvalidPricing)
	}
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return PricingConfig{}, err
	}
	config := PricingConfig{UserID: userID, AdminAccountID: adminAccountID, SiteUnitPrice: siteUnitPrice}
	if err := s.metricsRepo.SavePricing(ctx, config); err != nil {
		return PricingConfig{}, err
	}
	return config, nil
}

// StartScheduler 启动快照定稿任务：启动后先补一次（进程在零点时可能没有运行），之后每小时检查一次。
// 白天写入的快照只是某次打开仪表盘时的数，零点后用按日期查询到的全天营收与成本覆盖（定稿）。
func (s *MetricsService) StartScheduler(ctx context.Context) {
	go func() {
		timer := time.NewTimer(startupFinalizeDelay)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			s.finalizeRecent(ctx)
			timer.Reset(finalizeInterval)
		}
	}()
}

func dashboardLocation() *time.Location {
	return bizclock.Location()
}

// finalizeRecent 为所有持有 admin 会话的工作区定稿最近的快照：昨天（还没有快照时新建）以及最近几天里
// 已有快照但还没定稿的日期。单个工作区出错只记日志，不影响其他工作区。
func (s *MetricsService) finalizeRecent(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("dashboard finalize panic recovered: %v", r)
		}
	}()
	if s.metricsRepo == nil {
		return
	}
	refs, err := s.store.ActiveSessions(ctx)
	if err != nil {
		log.Printf("dashboard finalize: list active users failed: %v", err)
		return
	}
	now := s.now().In(dashboardLocation())
	today := calendarDate(now)
	yesterday := today.AddDate(0, 0, -1)
	for _, ref := range refs {
		dates := map[string]time.Time{}
		finalized, err := s.metricsRepo.IsFinalized(ctx, ref.UserID, ref.AdminAccountID, yesterday)
		if err != nil {
			log.Printf("dashboard finalize: check failed user_id=%s err=%v", ref.UserID, err)
			continue
		}
		if !finalized {
			dates[yesterday.Format("2006-01-02")] = yesterday
		}
		pending, err := s.metricsRepo.UnfinalizedDates(ctx, ref.UserID, ref.AdminAccountID, today.AddDate(0, 0, -finalizeLookbackDays), yesterday)
		if err != nil {
			log.Printf("dashboard finalize: list pending failed user_id=%s err=%v", ref.UserID, err)
			continue
		}
		for _, date := range pending {
			dates[date.Format("2006-01-02")] = calendarDate(date)
		}
		keys := make([]string, 0, len(dates))
		for key := range dates {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			if err := s.finalizeDay(ctx, ref.UserID, ref.AdminAccountID, dates[key], now); err != nil {
				log.Printf("dashboard finalize: skipped user_id=%s admin_account_id=%s date=%s err=%v", ref.UserID, ref.AdminAccountID, key, err)
			}
		}
	}
}

// errFinalizeIncomplete 表示有上游站点暂时查不到这一天的消费，稍后重试。
var errFinalizeIncomplete = errors.New("some upstream sites could not be queried")

// finalizeDay 按日期直接查询某天的全天营收（我的站点）和成本（各上游站点）并写入快照。
func (s *MetricsService) finalizeDay(ctx context.Context, userID string, adminAccountID string, day time.Time, now time.Time) error {
	record, err := s.store.Get(ctx, userID, adminAccountID)
	if err != nil {
		return err
	}
	if record == nil || !record.Session.IsAuthenticated() {
		return requestError(ErrorAdminOnly)
	}
	session, err := s.freshAdminSession(ctx, userID, adminAccountID, record)
	if err != nil {
		return err
	}
	date := day.Format("2006-01-02")
	revenue, err := s.platform.FetchAdminUsageStats(session, date, date)
	if err != nil {
		return err
	}
	consume, err := s.upstreams.ConsumeOnDate(ctx, userID, adminAccountID, date, dashboardTimezone)
	if err != nil {
		return err
	}
	today := calendarDate(now)
	if consume.Failed > 0 && !day.Before(today.AddDate(0, 0, -finalizeRetryDays)) {
		return fmt.Errorf("%w: failed=%d", errFinalizeIncomplete, consume.Failed)
	}
	if consume.Failed > 0 || consume.Approximated > 0 {
		log.Printf("dashboard finalize: partial upstream data user_id=%s date=%s failed=%d approximated=%d", userID, date, consume.Failed, consume.Approximated)
	}
	price := s.siteUnitPrice(ctx, userID, adminAccountID)
	revenue *= price

	// 零点后不久定稿昨天时，此刻的余额就是昨天结束时的余额；更晚定稿则保留快照里已有的余额（新建快照时才用当前值）。
	keepBalances := !(day.Equal(today.AddDate(0, 0, -1)) && now.Sub(bizclock.StartOfDay(now)) < endOfDayWindow)
	var siteBalance float64
	filterCfg, _ := s.metricsRepo.GetBalanceFilter(ctx, userID, adminAccountID)
	if result, err := s.platform.FetchAdminSiteBalanceFiltered(session, upstream.BalanceFilter{
		ExcludeAdmin:    filterCfg.ExcludeAdmin,
		ExcludeBalances: filterCfg.ExcludeBalances,
	}); err == nil {
		siteBalance = result.Balance * price
	}
	upstreamBalance := summarizeUpstreamCost(s.upstreams.ListForAccount(ctx, userID, adminAccountID), now).Balance

	id, err := metricsRandomID()
	if err != nil {
		return err
	}
	return s.metricsRepo.FinalizeSnapshot(ctx, DailySnapshot{
		ID:              id,
		UserID:          userID,
		AdminAccountID:  adminAccountID,
		Date:            day,
		TodayProfit:     revenue,
		SiteBalance:     siteBalance,
		TodayPurchase:   consume.Total,
		NetProfit:       revenue - consume.Total,
		UpstreamBalance: upstreamBalance,
		CreatedAt:       now,
	}, keepBalances)
}

// upsertSnapshot 将指标写入 dashboard_daily_stats 表。
// 冲突时更新已有行，保证同一天内多次调用始终保留最新数据。
func (s *MetricsService) upsertSnapshot(ctx context.Context, userID, adminAccountID, date string, metrics MetricsResponse) {
	if s.metricsRepo == nil {
		return
	}
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		log.Printf("dashboard metrics: invalid date %s: %v", date, err)
		return
	}
	id, err := metricsRandomID()
	if err != nil {
		log.Printf("dashboard metrics: generate id failed: %v", err)
		return
	}
	snapshot := DailySnapshot{
		ID:              id,
		UserID:          userID,
		AdminAccountID:  adminAccountID,
		Date:            parsedDate,
		TodayProfit:     metrics.TodayProfit,
		SiteBalance:     metrics.SiteBalance,
		TodayPurchase:   metrics.TodayPurchase,
		NetProfit:       metrics.NetProfit,
		UpstreamBalance: metrics.UpstreamBalance,
		CreatedAt:       time.Now(),
	}
	if err := s.metricsRepo.Upsert(ctx, snapshot); err != nil {
		log.Printf("dashboard metrics: upsert snapshot failed user_id=%s date=%s err=%v", userID, date, err)
	}
}

// AdminGroups 获取管理员站点的所有分组列表（平台中性）。
func (s *MetricsService) AdminGroups(ctx context.Context, userID string) (AdminGroupsResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return AdminGroupsResponse{}, err
	}
	record, err := s.store.Get(ctx, userID, adminAccountID)
	if err != nil {
		return AdminGroupsResponse{}, err
	}
	if record == nil || !record.Session.IsAuthenticated() {
		return AdminGroupsResponse{}, requestError(ErrorAdminOnly)
	}

	session, err := s.freshAdminSession(ctx, userID, adminAccountID, record)
	if err != nil {
		return AdminGroupsResponse{}, requestError(ErrorAdminOnly)
	}

	groups, err := s.platform.FetchAdminGroups(session)
	if err != nil {
		return AdminGroupsResponse{}, err
	}

	items := make([]AdminGroupItem, 0, len(groups))
	for _, g := range groups {
		platform := ""
		if g.Platform != nil {
			platform = *g.Platform
		}
		items = append(items, AdminGroupItem{
			ID:         g.ID,
			Name:       g.Name,
			Platform:   platform,
			Multiplier: g.MultiplierDisplay,
		})
	}
	return AdminGroupsResponse{Count: len(items), Groups: items}, nil
}

// GroupUsageToday 获取当前工作区「我的站点」所有分组今日的使用额度（平台中性）。
// 数据只在弹窗打开时按需请求，不参与 LiveMetrics 的批量指标计算。
func (s *MetricsService) GroupUsageToday(ctx context.Context, userID string) (GroupUsageTodayResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return GroupUsageTodayResponse{}, err
	}
	record, err := s.store.Get(ctx, userID, adminAccountID)
	if err != nil {
		return GroupUsageTodayResponse{}, err
	}
	if record == nil || !record.Session.IsAuthenticated() {
		return GroupUsageTodayResponse{}, requestError(ErrorAdminOnly)
	}

	session, err := s.freshAdminSession(ctx, userID, adminAccountID, record)
	if err != nil {
		return GroupUsageTodayResponse{}, requestError(ErrorAdminOnly)
	}

	if err := s.platform.VerifyAdmin(session); err != nil {
		return GroupUsageTodayResponse{}, requestError(ErrorAdminOnly)
	}

	groups, err := s.platform.FetchAdminGroups(session)
	if err != nil {
		return GroupUsageTodayResponse{}, err
	}

	stats, err := s.platform.FetchAdminGroupDailyStats(session, groups)
	if err != nil {
		return GroupUsageTodayResponse{}, err
	}

	// 归一化：分组名去空格、空名跳过、重名分组合并求和；顺序按首次出现排列。
	order := make([]string, 0, len(stats))
	totals := make(map[string]float64, len(stats))
	for _, stat := range stats {
		name := strings.TrimSpace(stat.GroupName)
		if name == "" {
			continue
		}
		if _, exists := totals[name]; !exists {
			order = append(order, name)
		}
		totals[name] += stat.TodayActualCost
	}

	items := make([]GroupUsageTodayItem, 0, len(order))
	var total float64
	for _, name := range order {
		amount := totals[name]
		items = append(items, GroupUsageTodayItem{GroupName: name, TodayAmount: amount})
		total += amount
	}

	return GroupUsageTodayResponse{
		Date:   bizclock.DateOf(s.now()),
		Total:  total,
		Groups: items,
	}, nil
}

// UpstreamKeyUsageToday 获取当前工作区所有上游站点中，今天有消费的 key 明细（仪表盘「今日成本」下钻）。
// 数据只在弹窗打开时按需请求，不参与 LiveMetrics 的批量指标计算。
// 排序、总额与筛选逻辑全部由 upstream.Service.KeyUsageToday 保证与 todayPurchase 口径一致，
// 这里只负责排序展示和响应封装。
func (s *MetricsService) UpstreamKeyUsageToday(ctx context.Context, userID string) (UpstreamKeyUsageTodayResponse, error) {
	items, err := s.upstreams.KeyUsageToday(ctx, userID)
	failedSites := 0
	totalSites := 0
	if err != nil {
		var collectionErr *upstream.KeyUsageCollectionError
		if !errors.As(err, &collectionErr) || collectionErr.TotalSites <= 0 || collectionErr.FailedSites >= collectionErr.TotalSites {
			return UpstreamKeyUsageTodayResponse{}, requestError(ErrorUpstreamKeyUsageUnavailable)
		}
		failedSites = collectionErr.FailedSites
		totalSites = collectionErr.TotalSites
		log.Printf("dashboard key usage: partial upstream failure user_id=%s failed_sites=%d total_sites=%d", userID, failedSites, totalSites)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].TodayAmount > items[j].TodayAmount
	})

	responseItems := make([]UpstreamKeyUsageTodayItem, 0, len(items))
	var total float64
	for _, item := range items {
		responseItems = append(responseItems, UpstreamKeyUsageTodayItem{
			SiteID:       item.SiteID,
			SiteName:     item.SiteName,
			Platform:     string(item.Platform),
			KeyID:        item.KeyID,
			KeyName:      item.KeyName,
			GroupName:    item.GroupName,
			TodayAmount:  item.TodayAmount,
			RawAmount:    item.RawAmount,
			RechargeRate: item.RechargeRate,
		})
		total += item.TodayAmount
	}

	return UpstreamKeyUsageTodayResponse{
		Date:        bizclock.DateOf(s.now()),
		Total:       total,
		Keys:        responseItems,
		FailedSites: failedSites,
		TotalSites:  totalSites,
	}, nil
}

// UpstreamBalanceBreakdown 获取当前工作区所有上游站点的余额明细（仪表盘「上游总余额」下钻）。
// 直接复用已同步缓存数据，不触发外部平台请求；未知余额（rechargeRate 未配置或尚未同步成功）的站点排在列表最后，
// total 只对已知余额求和，与 LiveMetrics 中 upstreamBalance 的计算口径一致。
func (s *MetricsService) UpstreamBalanceBreakdown(ctx context.Context, userID string) (UpstreamBalanceBreakdownResponse, error) {
	items, err := s.upstreams.BalanceBreakdown(ctx, userID)
	if err != nil {
		return UpstreamBalanceBreakdownResponse{}, err
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Balance == nil || items[j].Balance == nil {
			return items[i].Balance != nil
		}
		return *items[i].Balance > *items[j].Balance
	})

	responseItems := make([]UpstreamBalanceBreakdownItem, 0, len(items))
	var total float64
	for _, item := range items {
		responseItems = append(responseItems, UpstreamBalanceBreakdownItem{
			SiteID:       item.SiteID,
			SiteName:     item.SiteName,
			Platform:     string(item.Platform),
			Balance:      item.Balance,
			RawBalance:   item.RawBalance,
			RechargeRate: item.RechargeRate,
			LastSyncedAt: item.LastSyncedAt,
			Status:       string(item.Status),
		})
		if item.Balance != nil {
			total += *item.Balance
		}
	}

	return UpstreamBalanceBreakdownResponse{
		Total: total,
		Sites: responseItems,
	}, nil
}

// GetBalanceFilter 读取当前用户当前工作区的余额筛选配置。
func (s *MetricsService) GetBalanceFilter(ctx context.Context, userID string) (BalanceFilterConfig, error) {
	// 按当前工作区隔离筛选配置。
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return BalanceFilterConfig{}, err
	}
	return s.metricsRepo.GetBalanceFilter(ctx, userID, adminAccountID)
}

// SaveBalanceFilter 保存用户当前工作区的余额筛选配置。
func (s *MetricsService) SaveBalanceFilter(ctx context.Context, userID string, config BalanceFilterConfig) error {
	// 按当前工作区隔离筛选配置。
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return err
	}
	config.UserID = userID
	config.AdminAccountID = adminAccountID
	return s.metricsRepo.SaveBalanceFilter(ctx, config)
}

func (s *MetricsService) requireCurrentAdminAccount(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", requestError(ErrorAdminOnly)
	}
	return s.accounts.RequireCurrentID(ctx, userID)
}

func metricsRandomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
