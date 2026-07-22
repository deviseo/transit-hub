package dashboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"transithub/backend/internal/modules/upstream"
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
}

// MetricsService 负责仪表盘指标的实时计算、历史快照存储与午夜调度。
// 与同包的 Service（admin 会话管理）职责分离，共享 SessionStore 和 PlatformClient。
type MetricsService struct {
	store       SessionStore
	platform    PlatformClient
	upstreams   UpstreamLister
	metricsRepo *MetricsRepository
	accounts    AdminAccountService
	sessionSync MySiteStateSync
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

func NewMetricsService(store SessionStore, platform PlatformClient, upstreams UpstreamLister, metricsRepo *MetricsRepository, accounts AdminAccountService) *MetricsService {
	return &MetricsService{store: store, platform: platform, upstreams: upstreams, metricsRepo: metricsRepo, accounts: accounts}
}

// LiveMetrics 实时计算五项核心指标并返回。
// 同时将当天的指标作为快照 upsert 到数据库，确保趋势图数据持续积累。
//
// 计算逻辑：
//   - todayProfit:     管理员站点今日总实际消费，通过 sub2api /api/v1/admin/usage/stats 获取
//   - siteBalance:     管理员站点所有非 admin 用户余额之和，通过 sub2api /api/v1/admin/users 分页求和
//   - todayPurchase:   所有上游站点今日消费 × 站点倍率之和（复用已同步的内存数据，无额外请求）
//   - upstreamBalance: 所有上游站点余额 × 站点倍率之和（复用已同步的内存数据）
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
	today := time.Now().Format("2006-01-02")
	var (
		todayProfit     float64
		siteBalance     float64
		groupCount      int
		todayPurchase   float64
		upstreamBalance float64
		wg              sync.WaitGroup
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
		todayProfit = profit
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

	// goroutine 4: 今日进货额度与上游总余额（读取 Redis 缓存，无外部 API 调用）。
	// 使用 List（用户请求路径，自动过滤当前工作区站点）。
	wg.Add(1)
	go func() {
		defer wg.Done()
		for _, site := range s.upstreams.List(ctx, userID) {
			if site.RechargeRate <= 0 {
				continue
			}
			if site.Metrics.TodayConsume.Value != nil {
				todayPurchase += *site.Metrics.TodayConsume.Value * site.RechargeRate
			}
			if site.Metrics.Balance.Value != nil {
				upstreamBalance += *site.Metrics.Balance.Value * site.RechargeRate
			}
		}
	}()

	wg.Wait()

	netProfit := todayProfit - todayPurchase

	result := MetricsResponse{
		TodayProfit:     todayProfit,
		SiteBalance:     siteBalance,
		TodayPurchase:   todayPurchase,
		NetProfit:       netProfit,
		UpstreamBalance: upstreamBalance,
		GroupCount:      groupCount,
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
	snapshots, err := s.metricsRepo.ListRange(ctx, userID, adminAccountID, days)
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

// StartScheduler 启动午夜快照调度协程。
// 每天午夜（Asia/Shanghai 时区）为所有活跃 admin 用户保存当天的指标快照，
// 确保即使用户当天未访问仪表盘，趋势图也不会出现空缺。
func (s *MetricsService) StartScheduler(ctx context.Context) {
	go func() {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			log.Printf("dashboard scheduler: failed to load Asia/Shanghai timezone, using UTC: %v", err)
			loc = time.UTC
		}

		for {
			now := time.Now().In(loc)
			nextMidnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, loc)
			timer := time.NewTimer(time.Until(nextMidnight))

			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				s.snapshotAll(ctx)
			}
		}
	}()
}

// snapshotAll 遍历所有活跃 admin 用户，为昨天的日期保存指标快照。
// 午夜执行时，"今天"已经翻到新的一天，因此用昨天的日期查询 sub2api 的 usage stats，
// 而上游站点的余额取当前值（余额不按天重置）。
// 单用户出错只记日志，不影响其他用户和调度循环（与 refreshDueSessions 相同的容错模式）。
//
// 注意：此方法是后台调度路径，使用 ListForAccount 显式传入 adminAccountID，
// 不依赖当前工作区上下文（避免 fail-closed 的 List 在无 workspace 上下文时返回空）。
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

	loc, _ := time.LoadLocation("Asia/Shanghai")
	if loc == nil {
		loc = time.UTC
	}
	yesterday := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")

	for _, ref := range refs {
		userID := ref.UserID
		adminAccountID := ref.AdminAccountID
		yesterdayDate, _ := time.Parse("2006-01-02", yesterday)
		exists, err := s.metricsRepo.Exists(ctx, userID, adminAccountID, yesterdayDate)
		if err != nil {
			log.Printf("dashboard scheduler: check exists failed user_id=%s err=%v", userID, err)
			continue
		}
		// 如果昨天的快照已存在（由白天的 LiveMetrics 调用写入），跳过该用户。
		if exists {
			continue
		}

		record, err := s.store.Get(ctx, userID, adminAccountID)
		if err != nil || record == nil || !record.Session.IsAuthenticated() {
			continue
		}

		session, err := s.freshAdminSession(ctx, userID, adminAccountID, record)
		if err != nil {
			log.Printf("dashboard scheduler: refresh session failed user_id=%s err=%v", userID, err)
			continue
		}

		// 昨日盈利（平台中性）。
		todayProfit, err := s.platform.FetchAdminUsageStats(session, yesterday, yesterday)
		if err != nil {
			log.Printf("dashboard scheduler: fetch usage stats failed user_id=%s err=%v", userID, err)
			todayProfit = 0
		}

		// 站点用户总余额（平台中性）。
		var siteBalance float64
		filterCfg, _ := s.metricsRepo.GetBalanceFilter(ctx, userID, adminAccountID)
		if result, err := s.platform.FetchAdminSiteBalanceFiltered(session, upstream.BalanceFilter{
			ExcludeAdmin:    filterCfg.ExcludeAdmin,
			ExcludeBalances: filterCfg.ExcludeBalances,
		}); err == nil {
			siteBalance = result.Balance
		}

		// 上游指标：使用 ListForAccount 显式传入 adminAccountID，
		// 确保后台调度路径不依赖当前工作区上下文。
		var todayPurchase, upstreamBalance float64
		for _, site := range s.upstreams.ListForAccount(ctx, userID, adminAccountID) {
			if site.RechargeRate <= 0 {
				continue
			}
			if site.Metrics.TodayConsume.Value != nil {
				todayPurchase += *site.Metrics.TodayConsume.Value * site.RechargeRate
			}
			if site.Metrics.Balance.Value != nil {
				upstreamBalance += *site.Metrics.Balance.Value * site.RechargeRate
			}
		}

		result := MetricsResponse{
			TodayProfit:     todayProfit,
			SiteBalance:     siteBalance,
			TodayPurchase:   todayPurchase,
			NetProfit:       todayProfit - todayPurchase,
			UpstreamBalance: upstreamBalance,
		}
		s.upsertSnapshot(ctx, userID, adminAccountID, yesterday, result)
		log.Printf("dashboard scheduler: snapshot saved user_id=%s admin_account_id=%s date=%s", userID, adminAccountID, yesterday)
	}
}

// upsertSnapshot 将指标写入 dashboard_daily_stats 表。
// 冲突时更新已有行，保证同一天内多次调用始终保留最新数据。
func (s *MetricsService) upsertSnapshot(ctx context.Context, userID, adminAccountID, date string, metrics MetricsResponse) {
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
		Date:   time.Now().Format("2006-01-02"),
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
		Date:        time.Now().Format("2006-01-02"),
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
