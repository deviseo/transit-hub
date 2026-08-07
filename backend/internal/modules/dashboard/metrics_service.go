package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/businesstime"
)

// UpstreamLister 抽象上游站点列表读取，由 upstream.Service 实现。
// 仪表盘只需要读取已同步的站点数据，不需要修改或触发同步。
// List 用于用户请求路径（自动使用当前工作区），
// ListForAccount 用于后台调度等需要显式指定工作区的内部流程。
type UpstreamLister interface {
	List(ctx context.Context, userID string) []upstream.Response
	ListForAccount(ctx context.Context, userID, adminAccountID string) []upstream.Response
	// KeyUsageToday 和 BalanceBreakdown 是「今日成本」「上游总余额」下钻弹窗的数据源，
	// 由 upstream.Service 实现（持有 session/cache，能校验站点归属和当前工作区）。
	KeyUsageToday(ctx context.Context, userID string) ([]upstream.KeyUsageTodayItem, error)
	BalanceBreakdown(ctx context.Context, userID string) ([]upstream.BalanceBreakdownItem, error)
	// FetchSiteCostsForDate 用各站点自己的 session 查询指定日期的原始成本，
	// 不依赖仪表盘 admin 账户 session，与 KeyUsageToday 同一模式。
	FetchSiteCostsForDate(ctx context.Context, userID, adminAccountID, date string) ([]upstream.SiteCostForDateResult, error)
}

type metricsStore interface {
	Upsert(ctx context.Context, snapshot DailySnapshot) error
	ListRange(ctx context.Context, userID, adminAccountID string, days int, businessDate string) ([]DailySnapshot, error)
	GetBalanceFilter(ctx context.Context, userID, adminAccountID string) (BalanceFilterConfig, error)
	SaveBalanceFilter(ctx context.Context, config BalanceFilterConfig) error
	UpsertSiteCost(ctx context.Context, cost SiteDailyCost) error
	ListSiteCosts(ctx context.Context, userID, adminAccountID string, date string) ([]SiteDailyCost, error)
	ListDailyStats(ctx context.Context, userID, adminAccountID string, from, to string) ([]DailySnapshot, error)
}

// MetricsService 负责仪表盘指标的实时计算、历史快照存储与午夜调度。
// 与同包的 Service（admin 会话管理）职责分离，共享 SessionStore 和 PlatformClient。
type MetricsService struct {
	store           SessionStore
	platform        PlatformClient
	upstreams       UpstreamLister
	metricsRepo     metricsStore
	accounts        AdminAccountService
	sessionSync     MySiteStateSync
	refreshInterval time.Duration // 用于推导 maxStaleness；0 表示使用默认值 2h
}

// SetRefreshInterval 注入上游站点同步间隔，用于推导缓存时效阈值。
// 由 httpserver 装配层在初始化后调用；未调用时 maxStaleness 使用默认值 2h。
func (s *MetricsService) SetRefreshInterval(d time.Duration) {
	s.refreshInterval = d
}

// maxStaleness 返回缓存时效阈值：refreshInterval × 3，最小 2h。
func (s *MetricsService) maxStaleness() time.Duration {
	if s.refreshInterval > 0 {
		v := s.refreshInterval * 3
		if v < 2*time.Hour {
			return 2 * time.Hour
		}
		return v
	}
	return 2 * time.Hour
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
	return &MetricsService{store: store, platform: platform, upstreams: upstreams, metricsRepo: metricsRepo, accounts: accounts}
}

// summarizeCachedUpstreamCosts 汇总已同步的上游站点缓存（简化版，供日结路径使用）。
// failedOrUnavailable 为 true 时表示不能把结果当作完整日数据写入快照；
// 只有全部目标站点都不可用时才返回 err。
func summarizeCachedUpstreamCosts(sites []upstream.Response) (total float64, complete bool, err error) {
	targets := 0
	available := 0
	var firstErr error
	for _, site := range sites {
		if site.RechargeRate <= 0 {
			continue
		}
		targets++
		if site.Status == upstream.StatusError || site.Metrics.TodayConsume.Value == nil {
			if firstErr == nil {
				if site.ErrorKey != nil && strings.TrimSpace(*site.ErrorKey) != "" {
					firstErr = errors.New(*site.ErrorKey)
				} else {
					firstErr = errors.New(upstream.ErrorRequest)
				}
			}
			continue
		}
		available++
		total += *site.Metrics.TodayConsume.Value * site.RechargeRate
	}
	if targets == 0 {
		return total, true, nil
	}
	if available == 0 {
		return 0, false, firstErr
	}
	return total, available == targets, nil
}

// summarizeCachedUpstreamCostsWithQuality 汇总缓存成本并返回 CostQuality 结构。
// businessDate：当次请求的上海业务日期（"2006-01-02"）。
// maxStaleness：缓存最大有效时长；0 表示不检查时效。
func summarizeCachedUpstreamCostsWithQuality(sites []upstream.Response, businessDate string, maxStaleness time.Duration) (total float64, quality *CostQuality) {
	now := time.Now()
	quality = &CostQuality{
		BusinessDate: businessDate,
		ObservedAt:   &now,
	}

	for _, site := range sites {
		if site.RechargeRate <= 0 {
			continue
		}
		quality.ExpectedSites++

		// 日期归属校验：仅当日期字段有值且与当次业务日不匹配时才拒绝。
		// TodayConsumeDate=="" 表示 V0.1.16 上线前的旧缓存，放行（等下次同步自然补上日期）；
		// 已知日期与今天不符才是真正的日期错配。
		if site.Metrics.TodayConsumeDate != "" && site.Metrics.TodayConsumeDate != businessDate {
			quality.FailedSites++
			quality.Failures = append(quality.Failures, SiteCostFault{
				SiteName: site.Name,
				Reason:   "date_mismatch",
			})
			continue
		}

		// 时效校验：只有采集时间有值且超过阈值时才拒绝；nil 表示旧缓存，放行。
		if maxStaleness > 0 && site.Metrics.TodayConsumeAt != nil &&
			now.Sub(*site.Metrics.TodayConsumeAt) > maxStaleness {
			quality.FailedSites++
			quality.Failures = append(quality.Failures, SiteCostFault{
				SiteName: site.Name,
				Reason:   "stale",
			})
			continue
		}

		if site.Status == upstream.StatusError || site.Metrics.TodayConsume.Value == nil {
			quality.FailedSites++
			quality.Failures = append(quality.Failures, SiteCostFault{
				SiteName: site.Name,
				Reason:   "fetch_error",
			})
			continue
		}
		quality.CollectedSites++
		quality.ConfirmedCost += *site.Metrics.TodayConsume.Value * site.RechargeRate
	}

	if quality.ExpectedSites == 0 {
		quality.Complete = true
	} else {
		quality.Complete = quality.FailedSites == 0
	}
	return quality.ConfirmedCost, quality
}

func cachedUpstreamCostSiteCounts(sites []upstream.Response) (totalSites, failedSites int) {
	for _, site := range sites {
		if site.RechargeRate <= 0 {
			continue
		}
		totalSites++
		if site.Status == upstream.StatusError || site.Metrics.TodayConsume.Value == nil {
			failedSites++
		}
	}
	return totalSites, failedSites
}

// LiveMetrics 实时计算五项核心指标并返回。
// 同时将当天的指标作为快照 upsert 到数据库，确保趋势图数据持续积累。
//
// 计算逻辑：
//   - todayProfit:     管理员站点今日总实际消费，通过 sub2api /api/v1/admin/usage/stats 获取
//   - siteBalance:     管理员站点所有非 admin 用户余额之和，通过 sub2api /api/v1/admin/users 分页求和
//   - todayPurchase:   所有上游站点已同步的今日消费 × 站点倍率之和（成本不完整时为 nil）
//   - upstreamBalance: 所有上游站点余额 × 站点倍率之和（复用已同步的内存数据）
//   - netProfit:       todayProfit - todayPurchase；任一为 nil 时 netProfit 为 nil
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
	// 营收或成本失败时保留其他指标，用 metricErrors 标注失败项且不写快照。
	// 余额和分组数量保持原有零值降级行为。
	today := businesstime.Today()
	var (
		todayProfitVal  float64
		todayProfitErr  error
		siteBalance     float64
		groupCount      int
		upstreamBalance float64
		costQuality     *CostQuality
		wg              sync.WaitGroup
	)

	// goroutine 1: 今日盈利额度（平台中性）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		profit, err := s.platform.FetchAdminUsageStats(session, today, today)
		if err != nil {
			log.Printf("dashboard metrics: fetch usage stats failed user_id=%s err=%v", userID, err)
			todayProfitErr = err
			return
		}
		todayProfitVal = profit
	}()

	// goroutine 2: 站点用户总余额（平台中性）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		filterConfig, err := s.metricsRepo.GetBalanceFilter(ctx, userID, adminAccountID)
		if err != nil {
			log.Printf("dashboard metrics: load balance filter failed user_id=%s err=%v, using defaults", userID, err)
			filterConfig = BalanceFilterConfig{ExcludeAdmin: true, ExcludeBalances: []float64{}}
		}
		balanceResult, err := s.platform.FetchAdminSiteBalanceFiltered(session, upstream.BalanceFilter{
			ExcludeAdmin:    filterConfig.ExcludeAdmin,
			ExcludeBalances: filterConfig.ExcludeBalances,
		})
		if err != nil {
			log.Printf("dashboard metrics: fetch site balance failed user_id=%s err=%v", userID, err)
			return
		}
		siteBalance = balanceResult.Balance
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

	// goroutine 4: 读取上游站点已同步缓存中的今日进货额度和总余额，不触发上游请求。
	// 使用 List（用户请求路径，自动过滤当前工作区站点）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		sites := s.upstreams.List(ctx, userID)
		total, cq := summarizeCachedUpstreamCostsWithQuality(sites, today, s.maxStaleness())
		costQuality = cq
		for _, site := range sites {
			if site.RechargeRate <= 0 {
				continue
			}
			if site.Metrics.Balance.Value != nil {
				upstreamBalance += *site.Metrics.Balance.Value * site.RechargeRate
			}
		}
		_ = total // total 在 costQuality.ConfirmedCost 中
	}()

	wg.Wait()

	// 构建响应：使用指针类型区分"不可用"和"0"。
	var todayProfit *float64
	if todayProfitErr == nil {
		todayProfit = ptrF64(todayProfitVal)
	}
	var todayPurchase *float64
	if costQuality != nil && costQuality.Complete {
		todayPurchase = ptrF64(costQuality.ConfirmedCost)
	} else if costQuality != nil {
		// 部分成本：todayPurchase = nil（不能作为完整值），但 confirmedCost 是下限
		todayPurchase = nil
	}
	var netProfit *float64
	if todayProfit != nil && costQuality != nil && costQuality.Complete {
		np := *todayProfit - costQuality.ConfirmedCost
		netProfit = &np
	}

	result := MetricsResponse{
		Date:            today,
		Timezone:        businesstime.Timezone,
		TodayProfit:     todayProfit,
		SiteBalance:     siteBalance,
		TodayPurchase:   todayPurchase,
		NetProfit:       netProfit,
		UpstreamBalance: upstreamBalance,
		GroupCount:      groupCount,
		CostQuality:     costQuality,
	}

	if todayProfitErr != nil {
		result.MetricErrors = map[string]string{
			"todayProfit": todayProfitErr.Error(),
		}
	}

	// 仅在营收和成本都完整时写入快照（live_cache 来源，不覆盖 final 行）。
	if todayProfit != nil && costQuality != nil && costQuality.Complete {
		s.upsertSnapshot(ctx, userID, adminAccountID, today, result)
	}

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
	today := businesstime.Today()
	snapshots, err := s.metricsRepo.ListRange(ctx, userID, adminAccountID, days, today)
	if err != nil {
		return TrendResponse{}, err
	}
	points := make([]TrendPoint, 0, len(snapshots))
	for _, snap := range snapshots {
		points = append(points, TrendPoint{
			Date:            snap.Date.Format("2006-01-02"),
			TodayProfit:     snap.TodayProfit, // 保留 *float64，NULL 表示该天未采集
			SiteBalance:     derefF64(snap.SiteBalance),
			TodayPurchase:   snap.TodayPurchase, // 保留 *float64
			NetProfit:       snap.NetProfit,     // 保留 *float64
			UpstreamBalance: derefF64(snap.UpstreamBalance),
		})
	}
	return TrendResponse{Points: points}, nil
}

// StartScheduler 启动午夜快照调度协程（00:05/00:15/00:30 三次触发）和启动补结扫描。
// 每天三次触发确保即使用户当天未访问仪表盘，趋势图也不会出现空缺。
func (s *MetricsService) StartScheduler(ctx context.Context) {
	// 启动时执行一次恢复扫描（补结 SETTLEMENT_BASELINE_DATE 到昨日的缺口）。
	go s.startupRecovery(ctx)

	go func() {
		loc := businesstime.Location()
		// 每天午夜后依次触发：00:05、00:15、00:30。
		retryMinutes := []int{5, 15, 30}

		for {
			now := time.Now().In(loc)
			nextDay := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)

			for _, minute := range retryMinutes {
				fireAt := nextDay.Add(time.Duration(minute) * time.Minute)
				timer := time.NewTimer(time.Until(fireAt))
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
					s.snapshotAll(ctx)
				}
			}
		}
	}()
}

// snapshotAll 遍历所有活跃 admin 用户，对昨天的日期执行精确日结。
// 按 SPEC 4.3：调用 finalizeBusinessDate，营收和成本都精确按日期查询，不再读取缓存。
func (s *MetricsService) snapshotAll(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("dashboard scheduler panic recovered: %v", r)
		}
	}()

	refs, err := s.store.ActiveSessions(ctx)
	if err != nil {
		log.Printf("dashboard scheduler: list active users failed: %v", err)
		return
	}

	loc := businesstime.Location()
	yesterday := businesstime.DateAt(time.Now().In(loc).AddDate(0, 0, -1))

	for _, ref := range refs {
		if err := s.finalizeBusinessDate(ctx, ref, yesterday, SnapshotSourceDatedQuery); err != nil {
			log.Printf("dashboard scheduler: finalize failed user_id=%s date=%s err=%v",
				ref.UserID, yesterday, err)
		}
	}
}

// upsertSnapshot 将实时指标写入 dashboard_daily_stats 表（live_cache 来源）。
// 冲突时更新已有行，但不覆盖 final 行（由 repository 层的 Upsert 保护）。
func (s *MetricsService) upsertSnapshot(ctx context.Context, userID, adminAccountID, date string, metrics MetricsResponse) {
	parsedDate, err := time.ParseInLocation("2006-01-02", date, businesstime.Location())
	if err != nil {
		log.Printf("dashboard metrics: invalid date %s: %v", date, err)
		return
	}
	id, err := metricsRandomID()
	if err != nil {
		log.Printf("dashboard metrics: generate id failed: %v", err)
		return
	}
	now := time.Now()
	snapshot := DailySnapshot{
		ID:               id,
		UserID:           userID,
		AdminAccountID:   adminAccountID,
		Date:             parsedDate,
		TodayProfit:      metrics.TodayProfit,
		SiteBalance:      ptrF64(metrics.SiteBalance),
		TodayPurchase:    metrics.TodayPurchase,
		NetProfit:        metrics.NetProfit,
		UpstreamBalance:  ptrF64(metrics.UpstreamBalance),
		CreatedAt:        now,
		SettlementStatus: SettlementStatusProvisional,
		SnapshotSource:   SnapshotSourceLiveCache,
		ObservedAt:       &now,
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

// GroupUsageToday 获取当前工作区「我的站点」所有分组今日的营收，并在成本口径完整时实时派生利润（平台中性）。
// 数据只在首页运营区加载时按需请求，不参与 LiveMetrics 的批量指标计算。
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

	date := businesstime.Today()
	groups, err := s.platform.FetchAdminGroups(session)
	if err != nil {
		return GroupUsageTodayResponse{}, err
	}

	stats, err := s.platform.FetchAdminGroupDailyStatsForDate(session, groups, date)
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
	revenueByName := make(map[string]float64, len(order))
	for _, name := range order {
		amount := totals[name]
		items = append(items, GroupUsageTodayItem{
			GroupName:    name,
			TodayAmount:  amount,
			TodayRevenue: amount,
		})
		revenueByName[name] = amount
		total += amount
	}

	response := GroupUsageTodayResponse{
		Date:         date,
		Total:        total,
		TotalRevenue: total,
		Groups:       items,
	}

	// 分组利润只在当前上游成本完整、且营收与成本的分组名可对齐时生成。
	// 成本采集失败时保留营收响应，避免把未知成本静默当作零。
	if s.upstreams == nil {
		response.ProfitUnavailableReason = "upstream_cost_unavailable"
		return response, nil
	}
	costResponse, costErr := s.UpstreamKeyUsageToday(ctx, userID)
	if costErr != nil || costResponse.FailedSites > 0 {
		response.ProfitUnavailableReason = "upstream_cost_unavailable"
		return response, nil
	}

	costByName := make(map[string]float64)
	for _, item := range costResponse.Keys {
		name := strings.TrimSpace(item.GroupName)
		if name == "" {
			continue
		}
		costByName[name] += item.TodayAmount
	}

	// 缓存中的上游分组列表用于区分“当日零成本”和“根本无法对齐”。
	knownUpstreamGroups := make(map[string]struct{})
	for _, site := range s.upstreams.List(ctx, userID) {
		if site.RechargeRate <= 0 {
			continue
		}
		for _, group := range site.Metrics.Groups {
			name := strings.TrimSpace(group.Name)
			if name != "" {
				knownUpstreamGroups[name] = struct{}{}
			}
		}
	}

	for name := range costByName {
		if _, exists := revenueByName[name]; !exists {
			response.ProfitUnavailableReason = "group_name_unmatched"
			return response, nil
		}
	}
	if costResponse.TotalSites > 0 {
		for name := range revenueByName {
			if _, hasCost := costByName[name]; hasCost {
				continue
			}
			if _, known := knownUpstreamGroups[name]; !known {
				response.ProfitUnavailableReason = "group_name_unmatched"
				return response, nil
			}
		}
	}

	var totalCost float64
	for index := range response.Groups {
		name := response.Groups[index].GroupName
		cost := costByName[name]
		profit := response.Groups[index].TodayRevenue - cost
		response.Groups[index].TodayCost = ptrF64(cost)
		response.Groups[index].TodayProfit = ptrF64(profit)
		totalCost += cost
	}
	totalProfit := response.TotalRevenue - totalCost
	response.TotalCost = ptrF64(totalCost)
	response.TotalProfit = ptrF64(totalProfit)
	response.ProfitAvailable = true
	return response, nil
}

// UpstreamKeyUsageToday 获取当前工作区所有上游站点中，今天有消费的 key 明细（仪表盘「今日成本」下钻）。
// 数据在首页运营区和成本明细弹窗按需请求，不参与 LiveMetrics 的批量指标计算。
// 排序、总额与筛选逻辑全部由 upstream.Service.KeyUsageToday 保证，
// 这里只负责排序展示和响应封装。
func (s *MetricsService) UpstreamKeyUsageToday(ctx context.Context, userID string) (UpstreamKeyUsageTodayResponse, error) {
	date := businesstime.Today()
	// 复用日期和时效校验逻辑，与首页成本卡片口径一致。
	sites := s.upstreams.List(ctx, userID)
	_, quality := summarizeCachedUpstreamCostsWithQuality(sites, date, s.maxStaleness())
	totalSites := quality.ExpectedSites
	failedSites := quality.FailedSites
	items, err := s.upstreams.KeyUsageToday(ctx, userID)
	if err != nil {
		var collectionErr *upstream.KeyUsageCollectionError
		if !errors.As(err, &collectionErr) || collectionErr.TotalSites <= 0 || collectionErr.FailedSites >= collectionErr.TotalSites {
			return UpstreamKeyUsageTodayResponse{}, requestError(ErrorUpstreamKeyUsageUnavailable)
		}
		if totalSites == 0 {
			failedSites = collectionErr.FailedSites
			totalSites = collectionErr.TotalSites
		}
		log.Printf("dashboard key usage: partial upstream failure user_id=%s failed_sites=%d total_sites=%d", userID, collectionErr.FailedSites, collectionErr.TotalSites)
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
		Date:        date,
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

// derefF64 安全解引用 *float64，nil 时返回 0。
func derefF64(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

// ptrF64 将 float64 转为 *float64。
func ptrF64(v float64) *float64 {
	return &v
}

// finalizeBusinessDate 对指定 SessionRef 的指定上海业务日期执行精确日结。
// date 由调用方传入（"2006-01-02"），函数内部禁止用 time.Now() 推导业务日期。
// 记录 finalized_at、observed_at 使用 time.Now() 是合法的。
func (s *MetricsService) finalizeBusinessDate(ctx context.Context, ref ActiveSessionRef, date string, snapshotSource string) error {
	userID := ref.UserID
	adminAccountID := ref.AdminAccountID
	record, err := s.store.Get(ctx, userID, adminAccountID)
	if err != nil || record == nil || !record.Session.IsAuthenticated() {
		return errors.New("no authenticated session")
	}
	session, err := s.freshAdminSession(ctx, userID, adminAccountID, record)
	if err != nil {
		return err
	}

	parsedDate, err := time.ParseInLocation("2006-01-02", date, businesstime.Location())
	if err != nil {
		return err
	}

	// 营收查询（精确按日期查询，不使用 TodayConsume 缓存）。
	revenue, revenueErr := s.platform.FetchAdminUsageStats(session, date, date)

	// 逐站点成本查询：用各站点自己的 session（不使用仪表盘 admin 账户 session）。
	siteCostResults, fetchErr := s.upstreams.FetchSiteCostsForDate(ctx, userID, adminAccountID, date)
	if fetchErr != nil {
		log.Printf("dashboard finalize: fetch site costs failed user_id=%s date=%s err=%v", userID, date, fetchErr)
		return fetchErr
	}
	expectedCount := len(siteCostResults)
	collectedCount := 0
	now := time.Now()

	for _, r := range siteCostResults {
		siteCostID, idErr := metricsRandomID()
		if idErr != nil {
			log.Printf("dashboard finalize: generate site cost id failed err=%v, skipping site %s", idErr, r.SiteID)
			continue
		}
		siteCost := SiteDailyCost{
			ID:             siteCostID,
			UserID:         userID,
			AdminAccountID: adminAccountID,
			Date:           parsedDate,
			SiteID:         r.SiteID,
			SiteName:       r.SiteName,
			Platform:       string(r.Platform),
			RechargeRate:   r.RechargeRate,
			ObservedAt:     r.Meta.ObservedAt,
			Source:         snapshotSource,
		}
		if r.Err != nil {
			siteCost.Status = "failed"
			siteCost.ErrorReason = r.Err.Error()
			if upsertErr := s.metricsRepo.UpsertSiteCost(ctx, siteCost); upsertErr != nil {
				log.Printf("dashboard finalize: upsert site cost failed user_id=%s site_id=%s date=%s err=%v",
					userID, r.SiteID, date, upsertErr)
			}
		} else {
			adjusted := r.RawCost * r.RechargeRate
			siteCost.RawCost = &r.RawCost
			siteCost.AdjustedCost = &adjusted
			if r.Meta.Source == "key_sum_best_effort" {
				siteCost.Status = "partial"
				siteCost.Source = "best_effort"
			} else {
				siteCost.Status = "ok"
			}
			// 只有站点成本记录写入成功时，才计入 collectedCount；
			// 写入失败则降级处理（不计入），避免 final 汇总缺少明细行。
			if upsertErr := s.metricsRepo.UpsertSiteCost(ctx, siteCost); upsertErr != nil {
				log.Printf("dashboard finalize: upsert site cost failed user_id=%s site_id=%s date=%s err=%v",
					userID, r.SiteID, date, upsertErr)
			} else {
				collectedCount++
			}
		}
	}

	// 营收失败或全部站点失败：不写快照，等待重试。
	if revenueErr != nil {
		log.Printf("dashboard finalize: revenue failed user_id=%s date=%s err=%v", userID, date, revenueErr)
		return revenueErr
	}
	if expectedCount > 0 && collectedCount == 0 {
		log.Printf("dashboard finalize: all sites failed user_id=%s date=%s", userID, date)
		return errors.New("all upstream sites failed")
	}

	// 汇总成本直接从本次采集结果计算，不读取DB已有数据，
	// 避免重试时把旧站点成本混入本次结果。
	var totalCost float64
	allAccountLevel := true
	for _, r := range siteCostResults {
		if r.Err == nil {
			totalCost += r.RawCost * r.RechargeRate
			if r.Meta.Source == "key_sum_best_effort" {
				allAccountLevel = false
			}
		}
	}
	var status string
	var finalizedAt *time.Time
	if collectedCount == expectedCount && expectedCount > 0 && allAccountLevel {
		status = SettlementStatusFinal
		finalizedAt = &now
	} else {
		status = SettlementStatusPartial
	}

	snapshotID, idErr := metricsRandomID()
	if idErr != nil {
		log.Printf("dashboard finalize: generate snapshot id failed err=%v", idErr)
		return idErr
	}
	np := revenue - totalCost
	snapshot := DailySnapshot{
		ID:                 snapshotID,
		UserID:             userID,
		AdminAccountID:     adminAccountID,
		Date:               parsedDate,
		TodayProfit:        ptrF64(revenue),
		SiteBalance:        nil,
		TodayPurchase:      ptrF64(totalCost),
		NetProfit:          &np,
		UpstreamBalance:    nil,
		CreatedAt:          now,
		SettlementStatus:   status,
		SnapshotSource:     snapshotSource,
		ObservedAt:         &now,
		FinalizedAt:        finalizedAt,
		CostExpectedCount:  &expectedCount,
		CostCollectedCount: &collectedCount,
	}
	if upsertErr := s.metricsRepo.Upsert(ctx, snapshot); upsertErr != nil {
		log.Printf("dashboard finalize: upsert snapshot failed user_id=%s date=%s err=%v", userID, date, upsertErr)
		return upsertErr
	}
	log.Printf("dashboard finalize: done user_id=%s date=%s status=%s", userID, date, status)
	return nil
}

// startupRecovery 扫描从 SETTLEMENT_BASELINE_DATE 到昨日的缺口，逐日补结算。
// 不处理 SETTLEMENT_BASELINE_DATE 之前的日期。
func (s *MetricsService) startupRecovery(ctx context.Context) {
	loc := businesstime.Location()
	yesterday := businesstime.DateAt(time.Now().In(loc).AddDate(0, 0, -1))

	baselineStr := os.Getenv("SETTLEMENT_BASELINE_DATE")
	if baselineStr == "" {
		baselineStr = yesterday
	}
	baseline, err := time.ParseInLocation("2006-01-02", baselineStr, loc)
	if err != nil {
		log.Printf("dashboard startup recovery: invalid SETTLEMENT_BASELINE_DATE=%s", baselineStr)
		return
	}
	yesterdayTime, _ := time.ParseInLocation("2006-01-02", yesterday, loc)

	if baseline.After(yesterdayTime) {
		log.Printf("dashboard startup recovery: baseline=%s is after yesterday=%s, skipping", baselineStr, yesterday)
		return
	}

	refs, err := s.store.ActiveSessions(ctx)
	if err != nil {
		log.Printf("dashboard startup recovery: list sessions failed: %v", err)
		return
	}

	log.Printf("dashboard startup recovery: scanning %s to %s for %d sessions", baselineStr, yesterday, len(refs))

	for _, ref := range refs {
		// 获取该 session 的现有快照，找出缺口。
		existing, err := s.metricsRepo.ListDailyStats(ctx, ref.UserID, ref.AdminAccountID, baselineStr, yesterday)
		if err != nil {
			log.Printf("dashboard startup recovery: list stats failed user_id=%s err=%v", ref.UserID, err)
			continue
		}
		existingMap := make(map[string]string)
		for _, snap := range existing {
			existingMap[snap.Date.Format("2006-01-02")] = snap.SettlementStatus
		}

		// 逐日检查，补结未 final 的日期。
		current := baseline
		for !current.After(yesterdayTime) {
			d := current.Format("2006-01-02")
			current = current.AddDate(0, 0, 1)
			if status, ok := existingMap[d]; ok && status == SettlementStatusFinal {
				continue // 已结算，跳过
			}
			if err := s.finalizeBusinessDate(ctx, ref, d, SnapshotSourceDatedQuery); err != nil {
				log.Printf("dashboard startup recovery: finalize failed user_id=%s date=%s err=%v", ref.UserID, d, err)
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	log.Printf("dashboard startup recovery: completed")
}

// DailyStats 查询指定日期范围内每天的结算状态，缺失日期返回 missing 占位。
// page 从 1 开始，pageSize 默认 31，最大 90。
func (s *MetricsService) DailyStats(ctx context.Context, userID string, from, to string, page, pageSize int, expand bool) ([]DailyStatItem, error) {
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return nil, err
	}

	loc := businesstime.Location()
	fromTime, err := time.ParseInLocation("2006-01-02", from, loc)
	if err != nil {
		return nil, requestError("dashboard.errors.invalidDate")
	}
	toTime, err := time.ParseInLocation("2006-01-02", to, loc)
	if err != nil {
		return nil, requestError("dashboard.errors.invalidDate")
	}
	if fromTime.After(toTime) {
		return nil, requestError("dashboard.errors.fromAfterTo")
	}
	totalDays := int(toTime.Sub(fromTime).Hours()/24) + 1
	if totalDays > 90 {
		return nil, requestError("dashboard.errors.rangeTooLarge")
	}

	if pageSize <= 0 || pageSize > 90 {
		pageSize = 31
	}
	if page < 1 {
		page = 1
	}

	snapshots, err := s.metricsRepo.ListDailyStats(ctx, userID, adminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	snapMap := make(map[string]DailySnapshot)
	for _, snap := range snapshots {
		snapMap[snap.Date.Format("2006-01-02")] = snap
	}

	// 生成完整日期序列。
	items := make([]DailyStatItem, 0, totalDays)
	for d := fromTime; !d.After(toTime); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		if snap, ok := snapMap[dateStr]; ok {
			item := DailyStatItem{
				Date:               dateStr,
				SettlementStatus:   snap.SettlementStatus,
				SnapshotSource:     snap.SnapshotSource,
				TodayProfit:        snap.TodayProfit,
				CostExpectedCount:  snap.CostExpectedCount,
				CostCollectedCount: snap.CostCollectedCount,
			}
			if snap.TodayPurchase != nil {
				item.ConfirmedCost = snap.TodayPurchase
			}
			if snap.TodayProfit != nil && snap.TodayPurchase != nil {
				ceiling := *snap.TodayProfit - *snap.TodayPurchase
				item.NetProfitCeiling = &ceiling
				if *snap.TodayProfit > 0 {
					margin := ceiling / *snap.TodayProfit * 100
					item.MarginCeiling = &margin
				}
			}
			if snap.FinalizedAt != nil {
				ts := snap.FinalizedAt.Format(time.RFC3339)
				item.FinalizedAt = &ts
			}
			if expand {
				siteCosts, scErr := s.metricsRepo.ListSiteCosts(ctx, userID, adminAccountID, dateStr)
				if scErr != nil {
					log.Printf("dashboard daily-stats: list site costs failed date=%s err=%v", dateStr, scErr)
					item.SiteCostsLoadError = true
				} else {
					item.SiteCosts = make([]SiteCostDetail, 0, len(siteCosts))
					for _, sc := range siteCosts {
						item.SiteCosts = append(item.SiteCosts, SiteCostDetail{
							SiteID:       sc.SiteID,
							SiteName:     sc.SiteName,
							Platform:     sc.Platform,
							RawCost:      sc.RawCost,
							RechargeRate: sc.RechargeRate,
							AdjustedCost: sc.AdjustedCost,
							Status:       sc.Status,
							Source:       sc.Source,
							ErrorReason:  sc.ErrorReason,
							ObservedAt:   sc.ObservedAt.Format(time.RFC3339),
						})
					}
				}
			}
			items = append(items, item)
		} else {
			items = append(items, DailyStatItem{
				Date:             dateStr,
				SettlementStatus: SettlementStatusMissing,
			})
		}
	}

	// 分页。
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []DailyStatItem{}, nil
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end], nil
}

// BackfillRequest 是 POST /api/dashboard/backfill 的请求体。
type BackfillRequest struct {
	From   string `json:"from"`
	To     string `json:"to"`
	DryRun bool   `json:"dryRun"`
	Force  bool   `json:"force"`
}

// BackfillDayResult 是回填操作单天的结果。
type BackfillDayResult struct {
	Date      string   `json:"date"`
	Status    string   `json:"status"` // "updated"/"skipped"/"failed"
	Reason    string   `json:"reason,omitempty"`
	OldProfit *float64 `json:"oldProfit,omitempty"`
	NewProfit *float64 `json:"newProfit,omitempty"`
	OldCost   *float64 `json:"oldCost,omitempty"`
	NewCost   *float64 `json:"newCost,omitempty"`
}

// BackfillResponse 是 POST /api/dashboard/backfill 的响应体。
type BackfillResponse struct {
	DryRun  bool                `json:"dryRun"`
	Results []BackfillDayResult `json:"results"`
}

// Backfill 受控回填指定日期范围的历史数据（仅管理员可调用）。
// dryRun=true 时只返回预览，不写库；force=true 时允许覆盖 final 行并写审计日志。
func (s *MetricsService) Backfill(ctx context.Context, userID string, req BackfillRequest) (BackfillResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return BackfillResponse{}, err
	}

	// 显式校验 admin 角色，不仅仅检查 session 是否存在。
	record, err := s.store.Get(ctx, userID, adminAccountID)
	if err != nil || record == nil || !record.Session.IsAuthenticated() {
		return BackfillResponse{}, requestError(ErrorAdminOnly)
	}
	session, err := s.freshAdminSession(ctx, userID, adminAccountID, record)
	if err != nil {
		return BackfillResponse{}, requestError(ErrorAdminOnly)
	}
	if err := s.platform.VerifyAdmin(session); err != nil {
		return BackfillResponse{}, requestError(ErrorAdminOnly)
	}

	loc := businesstime.Location()
	fromTime, err := time.ParseInLocation("2006-01-02", req.From, loc)
	if err != nil {
		return BackfillResponse{}, requestError("dashboard.errors.invalidDate")
	}
	toTime, err := time.ParseInLocation("2006-01-02", req.To, loc)
	if err != nil {
		return BackfillResponse{}, requestError("dashboard.errors.invalidDate")
	}
	if fromTime.After(toTime) {
		return BackfillResponse{}, requestError("dashboard.errors.fromAfterTo")
	}
	totalDays := int(toTime.Sub(fromTime).Hours()/24) + 1
	if totalDays > 90 {
		return BackfillResponse{}, requestError("dashboard.errors.rangeTooLarge")
	}

	results := make([]BackfillDayResult, 0)

	for d := fromTime; !d.After(toTime); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")
		result := BackfillDayResult{Date: dateStr}

		// 检查现有快照。
		existing, _ := s.metricsRepo.ListDailyStats(ctx, userID, adminAccountID, dateStr, dateStr)
		var existingSnap *DailySnapshot
		if len(existing) > 0 {
			snap := existing[0]
			existingSnap = &snap
		}

		// 处理保护规则：force=false 时跳过已 final 且非 backfill 的行。
		if !req.Force && existingSnap != nil &&
			existingSnap.SettlementStatus == SettlementStatusFinal &&
			existingSnap.SnapshotSource != SnapshotSourceBackfill {
			result.Status = "skipped"
			result.Reason = "final_row_protected"
			if existingSnap.TodayProfit != nil {
				result.OldProfit = existingSnap.TodayProfit
			}
			if existingSnap.TodayPurchase != nil {
				result.OldCost = existingSnap.TodayPurchase
			}
			results = append(results, result)
			continue
		}

		if req.DryRun {
			// dryRun：查询上游数据以计算预览值，不写库。
			// 错误必须传播：查询失败不能返回伪造的零值预览。
			rec, recErr := s.store.Get(ctx, userID, adminAccountID)
			if recErr != nil || rec == nil || !rec.Session.IsAuthenticated() {
				result.Status = "failed"
				result.Reason = "no_admin_session"
			} else if sess, sessErr := s.freshAdminSession(ctx, userID, adminAccountID, rec); sessErr != nil {
				result.Status = "failed"
				result.Reason = "session_error: " + sessErr.Error()
			} else {
				newRevenue, revErr := s.platform.FetchAdminUsageStats(sess, dateStr, dateStr)
				siteCostResults, costErr := s.upstreams.FetchSiteCostsForDate(ctx, userID, adminAccountID, dateStr)
				if revErr != nil {
					result.Status = "failed"
					result.Reason = "revenue_query_failed: " + revErr.Error()
				} else if costErr != nil {
					result.Status = "failed"
					result.Reason = "cost_query_failed: " + costErr.Error()
				} else {
					var newCost float64
					for _, r := range siteCostResults {
						if r.Err == nil {
							newCost += r.RawCost * r.RechargeRate
						}
					}
					result.NewProfit = ptrF64(newRevenue)
					result.NewCost = ptrF64(newCost)
				}
			}
		} else {
			// 写审计日志（force=true 覆盖 final 行时）。
			if req.Force && existingSnap != nil && existingSnap.SettlementStatus == SettlementStatusFinal {
				log.Printf("dashboard backfill force-overwrite: date=%s old_profit=%v old_cost=%v user_id=%s",
					dateStr, existingSnap.TodayProfit, existingSnap.TodayPurchase, userID)
			}
			ref := ActiveSessionRef{UserID: userID, AdminAccountID: adminAccountID}
			if err := s.finalizeBusinessDate(ctx, ref, dateStr, SnapshotSourceBackfill); err != nil {
				result.Status = "failed"
				result.Reason = err.Error()
				results = append(results, result)
				// 限流：支持取消，避免请求取消后仍阻塞。
				select {
				case <-ctx.Done():
					return BackfillResponse{DryRun: req.DryRun, Results: results}, ctx.Err()
				case <-time.After(500 * time.Millisecond):
				}
				continue
			}
		}

		// 读取最新结果（dryRun 时读取预期值）。
		result.Status = "updated"
		if existingSnap != nil {
			result.OldProfit = existingSnap.TodayProfit
			result.OldCost = existingSnap.TodayPurchase
		}
		results = append(results, result)

		// 限流：每次间隔 500ms。
		if !req.DryRun {
			select {
			case <-ctx.Done():
				return BackfillResponse{DryRun: req.DryRun, Results: results}, nil
			case <-time.After(500 * time.Millisecond):
			}
		}
	}

	return BackfillResponse{DryRun: req.DryRun, Results: results}, nil
}
