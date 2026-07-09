package httpserver

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"transithub/backend/internal/config"
	"transithub/backend/internal/modules/admin_accounts"
	"transithub/backend/internal/modules/auth"
	"transithub/backend/internal/modules/connection_health"
	"transithub/backend/internal/modules/dashboard"
	"transithub/backend/internal/modules/group_rate_campaigns"
	"transithub/backend/internal/modules/group_rates"
	"transithub/backend/internal/modules/health"
	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/modules/system"
	"transithub/backend/internal/modules/tickets"
	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/modules/users"
	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

const (
	apiPrefix              = "/api"
	upstreamRequestTimeout = 60 * time.Second
)

type Server struct {
	cfg         config.Config
	mux         *http.ServeMux
	allowed     map[string]struct{}
	authService *auth.Service
}

func New(cfg config.Config, db *pgxpool.Pool, redisClient *redis.Client) *Server {
	server := &Server{
		cfg:     cfg,
		mux:     http.NewServeMux(),
		allowed: makeAllowedOrigins(cfg.CORSOrigins),
	}

	health.RegisterRoutes(server.mux)
	authService := auth.NewService(auth.NewRepository(db))
	server.authService = authService
	if err := authService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}

	// 管理员初始化：数据库就绪后、注册路由前执行
	if err := authService.BootstrapAdmin(context.Background(), cfg.AdminEmail, cfg.AdminPassword); err != nil {
		panic(err)
	}

	auth.RegisterRoutes(server.mux, authService, cfg.AllowPublicRegister)
	users.RegisterRoutes(server.mux, users.NewService(users.NewRepository(db)))
	adminAccountsService := admin_accounts.NewService(admin_accounts.NewRepository(db))
	upstreamRepository := upstream.NewRepository(db)
	if err := upstreamRepository.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	groupRatesService := group_rates.NewService(group_rates.NewRepository(db))
	if err := groupRatesService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	group_rates.RegisterRoutes(server.mux, groupRatesService, adminAccountsService)
	upstreamHTTPClient := &http.Client{Timeout: upstreamRequestTimeout}
	platformService := upstream.NewPlatformService(upstream.NewHTTPClient(upstreamHTTPClient))
	upstreamCache := upstream.NewRedisSiteCache(redisClient)
	upstreamService := upstream.NewService(platformService, upstreamRepository, groupRateSnapshotWriter{service: groupRatesService}, upstreamCache)
	upstreamService.SetAdminAccountResolver(adminAccountsService)
	upstream.RegisterRoutes(server.mux, upstreamService, adminAccountsService)
	mySitesService := my_sites.NewService(my_sites.NewRepository(db), platformService, upstreamService)
	if err := mySitesService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	my_sites.RegisterRoutes(server.mux, mySitesService)
	mySitesService.SetAdminAccountResolver(adminAccountsService)

	// 工单模块：iframe 嵌入配置 + 工单/回复。公开 iframe 接口鉴权完全依赖 embedToken/Sub2API
	// token 换取的 embed session，与 TransitHub 登录态无关，因此不加入 protectedPath（见下方）。
	ticketsRepository := tickets.NewRepository(db)
	if err := ticketsRepository.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	ticketsSub2APIClient := tickets.NewSub2APIClient(&http.Client{Timeout: upstreamRequestTimeout})
	ticketsSessions := tickets.NewEmbedSessionStore(redisClient)
	ticketsStorage, err := tickets.NewAttachmentStorage(cfg.TicketUploadDir)
	if err != nil {
		panic(err)
	}
	ticketsService := tickets.NewService(ticketsRepository, ticketsSessions, ticketsSub2APIClient, ticketsStorage)
	ticketsService.SetAdminAccountResolver(adminAccountsService)
	// Sub2API 用户资料弹窗按当前 workspace 的 admin 会话（mySitesService）实时查询用户详情/余额
	// 历史（platformService），复用已有的会话存储和刷新逻辑，不新增第二套 admin token 存储。
	ticketsService.SetAdminSessionProvider(mySitesService)
	ticketsService.SetSub2APIAdminClient(platformService)
	tickets.RegisterRoutes(server.mux, ticketsService)

	settingsService := settings.NewService(http.DefaultClient, settings.NewRepository(db))
	settingsService.SetAdminAccountResolver(adminAccountsService)
	if err := settingsService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}

	// dashboard 指标表必须在 admin_accounts 之前完成 schema，
	// 因为 admin_accounts.EnsureSchema 的 legacy 迁移会 UPDATE dashboard 表。
	metricsRepo := dashboard.NewMetricsRepository(db)
	if err := metricsRepo.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}

	// admin_accounts 最后执行 schema：此时所有业务表和 workspace 字段已存在，
	// legacy 迁移可以安全地 UPDATE 所有业务表的 admin_account_id。
	if err := adminAccountsService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	admin_accounts.RegisterRoutes(server.mux, adminAccountsService)
	if err := upstreamService.RestoreSavedSites(context.Background()); err != nil {
		panic(err)
	}

	// 注入机器人通知能力，供自动调价成功后发送通知。
	mySitesService.SetBotNotifier(settingsService)

	// 活动调价中心：批量修改 admin 自有分组倍率的独立模块，不复用/不污染 my_sites 的自动调价逻辑。
	// mySitesService 提供 admin 会话与分组倍率读写能力，groupRatesService 提供分组类型标签查询，
	// settingsService 提供机器人通知发送能力。
	campaignsService := group_rate_campaigns.NewService(
		group_rate_campaigns.NewRepository(db),
		mySitesService,
		settingsService,
		groupRatesService,
		group_rate_campaigns.Config{
			NotifyEnabledDefault: cfg.GroupRateCampaignNotifyEnabled,
			DefaultNotifyBotIDs:  cfg.GroupRateCampaignDefaultNotifyBots,
			StartTemplateDefault: cfg.GroupRateCampaignStartTemplate,
			EndTemplateDefault:   cfg.GroupRateCampaignEndTemplate,
			SchedulerInterval:    cfg.GroupRateCampaignSchedulerInterval,
		},
	)
	if err := campaignsService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	campaignsService.SetAdminAccountResolver(adminAccountsService)
	group_rate_campaigns.RegisterRoutes(server.mux, campaignsService, adminAccountsService)
	campaignsService.StartScheduler(context.Background())

	// 分组健康探活模块：数据源为 real_connections（通过 mySitesService 只读接口），
	// upstreamService 提供站点 base_url/平台类型查询，platformService 提供 new-api 远端降级/恢复能力。
	// 不新增手动配置的探活目标，也不改变 my_sites/upstream 现有数据语义。
	connHealthService := connection_health.NewService(
		connection_health.NewRepository(db),
		mySitesService,
		upstreamService,
		platformService,
	)
	if err := connHealthService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	connHealthService.SetAdminAccountResolver(adminAccountsService)
	// 注入平台中性的分组/账号读取能力：admin 分组健康主列表用它拉取 admin 全量分组及
	// 分组下账号/渠道，叠加 real_connections 探活状态。platformService 已实现所需方法。
	connHealthService.SetPlatformGroupReader(platformService)
	connection_health.RegisterRoutes(server.mux, connHealthService)
	connHealthService.StartScheduler(context.Background())

	// 策略设置变更时通知上游服务更新定时同步配置。
	applyRefreshConfig := func(s settings.StrategySettings) {
		upstreamService.SetRefreshConfig(upstream.RefreshConfig{
			Enabled:  s.EnableRefreshInterval,
			Interval: time.Duration(s.RefreshInterval) * time.Second,
		})
	}
	settingsService.OnStrategyChanged = applyRefreshConfig

	// 启动时读取已保存的策略设置，按配置决定是否开启定时同步。
	if strategy, err := settingsService.GetFirstStrategy(context.Background()); err == nil {
		applyRefreshConfig(strategy)
	}

	// 站点同步成功后检查余额预警和倍率变更，按配置发送通知。
	upstreamService.AfterSync = func(ctx context.Context, userID, adminAccountID, siteID, siteName string, oldMetrics, newMetrics upstream.Metrics) {
		strategy, err := settingsService.GetFirstStrategy(ctx)
		if err != nil {
			return
		}
		checkBalanceWarning(ctx, settingsService, upstreamService, strategy, userID, siteID, siteName, oldMetrics, newMetrics)
		checkMultiplierChanges(ctx, settingsService, strategy, userID, siteID, siteName, oldMetrics, newMetrics)
		// 自动调价：分组级 enableAutoPricing 是唯一开关，Service 内部逐 mapping 判断。
		mySitesService.ApplyAutoPricingAfterSync(ctx, userID, adminAccountID, siteID, siteName, oldMetrics, newMetrics)
	}

	settings.RegisterRoutes(server.mux, settingsService)

	// 仪表盘 admin 登录门禁：复用 sub2api 平台客户端（platformService），会话存于 Redis，
	// 并启动后台协程对临期令牌做自动刷新。
	dashboardSessionStore := dashboard.NewRepository(redisClient)
	dashboardService := dashboard.NewService(dashboardSessionStore, platformService)
	dashboardService.SetAdminAccountService(adminAccountsService)
	dashboardService.SetMySiteSync(mySitesService)
	dashboardService.StartRefresher(context.Background())

	// 仪表盘指标服务：实时计算五项核心指标 + 历史趋势快照。
	// 复用 dashboard 的 Redis 会话存储与 sub2api 平台客户端，
	// 并复用 upstreamService 读取已同步的上游站点数据（无额外 API 调用）。
	metricsService := dashboard.NewMetricsService(dashboardSessionStore, platformService, upstreamService, metricsRepo, adminAccountsService)
	metricsService.StartScheduler(context.Background())
	dashboard.RegisterRoutes(server.mux, dashboardService, metricsService)

	// 系统信息 API：开源版仅保留版本号展示
	systemService := system.NewService(cfg)
	system.RegisterRoutes(server.mux, systemService)

	return server
}

type groupRateSnapshotWriter struct {
	service *group_rates.Service
}

func (w groupRateSnapshotWriter) SaveSiteSnapshot(ctx context.Context, userID string, adminAccountID string, siteID string, siteName string, sitePlatform upstream.Platform, groups []upstream.SnapshotGroup) error {
	snapshots := make([]group_rates.SnapshotGroup, 0, len(groups))
	for _, group := range groups {
		snapshots = append(snapshots, group_rates.SnapshotGroup{
			ID:         group.ID,
			Name:       group.Name,
			Platform:   group.Platform,
			Multiplier: group.Multiplier,
		})
	}
	return w.service.SaveSiteSnapshot(ctx, userID, adminAccountID, siteID, siteName, string(sitePlatform), snapshots)
}

func (s *Server) Handler() http.Handler {
	// 非 /api 路径交给静态文件服务，支持 Vue history 路由回退
	static := staticHandler(s.cfg.PublicDir)

	return s.logRequests(s.cors(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, apiPrefix) {
			static.ServeHTTP(w, r)
			return
		}
		if s.protectedPath(r.URL.Path) {
			user, err := s.authService.CurrentUser(r.Context(), bearerToken(r.Header.Get("Authorization")))
			if err != nil {
				httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
				return
			}
			r = r.WithContext(authctx.WithUserID(r.Context(), user.ID))
		}
		s.mux.ServeHTTP(w, r)
	})))
}

func (s *Server) protectedPath(path string) bool {
	return strings.HasPrefix(path, "/api/admin-accounts") || strings.HasPrefix(path, "/api/upstream-sites") || strings.HasPrefix(path, "/api/group-rates") || strings.HasPrefix(path, "/api/group-rate-campaigns") || strings.HasPrefix(path, "/api/my-sites") || strings.HasPrefix(path, "/api/settings") || strings.HasPrefix(path, "/api/dashboard") || strings.HasPrefix(path, "/api/system") || strings.HasPrefix(path, "/api/connection-health") || strings.HasPrefix(path, "/api/tickets")
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		writer := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(writer, r)
		log.Printf("request method=%s path=%s status=%d duration=%s", r.Method, r.URL.Path, writer.status, time.Since(startedAt))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// Flush 透传底层 ResponseWriter 的 Flusher 能力，
// 确保 SSE 等流式响应在经过 logRequests 中间件包装后仍能正常刷新。
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if len(s.cfg.CORSOrigins) == 0 {
			if origin == "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		} else if _, ok := s.allowed[origin]; ok {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func makeAllowedOrigins(origins []string) map[string]struct{} {
	allowed := make(map[string]struct{}, len(origins))
	for _, origin := range origins {
		allowed[origin] = struct{}{}
	}
	return allowed
}

// balanceAlertTracker 记录每个站点最后一次余额预警发送时间，防止在冷却期内重复通知。
// 冷却期内余额持续低于阈值时不再重复发送；冷却期过后若仍低于阈值则再次通知。
// 余额恢复到阈值以上时自动清除记录，下次跌破可立即触发。
var balanceAlertTracker = struct {
	mu       sync.Mutex
	lastSent map[string]time.Time
}{lastSent: make(map[string]time.Time)}

// balanceAlertCooldown 余额预警冷却时间，同一站点在此时间段内不会重复发送通知。
const balanceAlertCooldown = 1 * time.Hour

// checkBalanceWarning 检测余额是否低于阈值并发送通知。
// 采用冷却机制：首次低于阈值立即通知，之后在冷却期内不再重复；冷却期过后若仍低于阈值再次通知。
// 优先使用站点级 BalanceThreshold 覆盖全局 DefaultBalanceThreshold；站点设置为 nil 时降级到全局。
func checkBalanceWarning(ctx context.Context, svc *settings.Service, uSvc *upstream.Service, strategy settings.StrategySettings, userID, siteID, siteName string, _ /* oldMetrics */, newMetrics upstream.Metrics) {
	if !strategy.EnableBalanceWarning || len(strategy.BalanceNotifyBotIDs) == 0 {
		return
	}
	if newMetrics.Balance.Value == nil {
		return
	}

	// 获取站点充值倍率，用于将 USD 余额转换为 CNY 后与阈值比较。
	site, err := uSvc.GetSite(ctx, siteID)
	if err != nil {
		return
	}
	rechargeRate := site.RechargeRate
	if rechargeRate <= 0 {
		rechargeRate = 1
	}

	newBalCNY := *newMetrics.Balance.Value * rechargeRate

	// 站点级阈值覆盖：有值则用站点配置，否则使用全局默认（均为 CNY）。
	threshold := strategy.DefaultBalanceThreshold
	if site.Settings.BalanceThreshold != nil {
		threshold = *site.Settings.BalanceThreshold
	}

	if newBalCNY >= threshold {
		// 余额恢复到阈值以上，清除冷却记录，下次跌破可立即触发。
		balanceAlertTracker.mu.Lock()
		delete(balanceAlertTracker.lastSent, siteID)
		balanceAlertTracker.mu.Unlock()
		return
	}

	// 冷却期检查：同一站点在冷却期内不重复通知。
	balanceAlertTracker.mu.Lock()
	if last, ok := balanceAlertTracker.lastSent[siteID]; ok && time.Since(last) < balanceAlertCooldown {
		balanceAlertTracker.mu.Unlock()
		return
	}
	balanceAlertTracker.lastSent[siteID] = time.Now()
	balanceAlertTracker.mu.Unlock()

	msg := formatBalanceWarning(siteName, newBalCNY, threshold, strategy.BalanceTemplate)
	log.Printf("[alert] 余额预警触发 site=%s balanceCNY=%.2f threshold=%.2f rechargeRate=%.2f", siteName, newBalCNY, threshold, rechargeRate)
	svc.SendToBots(ctx, userID, strategy.BalanceNotifyBotIDs, msg)
}

// checkMultiplierChanges 对比同步前后的分组倍率，任何变化都发送通知。
// 只受系统设置全局开关 strategy.EnableMultiplierAlert 控制。
func checkMultiplierChanges(ctx context.Context, svc *settings.Service, strategy settings.StrategySettings, userID, siteID, siteName string, oldMetrics, newMetrics upstream.Metrics) {
	if !strategy.EnableMultiplierAlert || len(strategy.MultiplierNotifyBotIDs) == 0 {
		return
	}
	if len(oldMetrics.Groups) == 0 {
		return
	}
	oldMap := make(map[string]float64, len(oldMetrics.Groups))
	for _, g := range oldMetrics.Groups {
		if g.Multiplier != nil {
			oldMap[g.ID+"|"+g.Name] = *g.Multiplier
		}
	}
	for _, g := range newMetrics.Groups {
		if g.Multiplier == nil {
			continue
		}
		key := g.ID + "|" + g.Name
		oldVal, existed := oldMap[key]
		if !existed || oldVal == *g.Multiplier {
			continue
		}
		msg := formatMultiplierChange(siteName, g.Name, oldVal, *g.Multiplier, strategy.MultiplierTemplate)
		log.Printf("[alert] 倍率变更触发 site=%s group=%s old=%.4f new=%.4f", siteName, g.Name, oldVal, *g.Multiplier)
		svc.SendToBots(ctx, userID, strategy.MultiplierNotifyBotIDs, msg)
	}
}

const defaultBalanceTemplate = "【余额预警】{siteName} 站点余额（CNY）已不足 {threshold} 元，当前余额为 {balance} 元。"
const defaultMultiplierTemplate = "【倍率变更】{siteName} 的 {groupName} 分组倍率已{changeDirection}：{oldRate}x -> {newRate}x。"

func formatBalanceWarning(siteName string, balance, threshold float64, customTemplate string) string {
	tpl := customTemplate
	if tpl == "" {
		tpl = defaultBalanceTemplate
	}
	r := strings.NewReplacer(
		"{siteName}", siteName,
		"{balance}", fmt.Sprintf("%.2f", balance),
		"{threshold}", fmt.Sprintf("%.2f", threshold),
	)
	return r.Replace(tpl)
}

func formatMultiplierChange(siteName, groupName string, oldRate, newRate float64, customTemplate string) string {
	tpl := customTemplate
	if tpl == "" {
		tpl = defaultMultiplierTemplate
	}
	changeDirection := "上升"
	if newRate < oldRate {
		changeDirection = "下降"
	}
	r := strings.NewReplacer(
		"{siteName}", siteName,
		"{groupName}", groupName,
		"{oldRate}", fmt.Sprintf("%.4f", oldRate),
		"{newRate}", fmt.Sprintf("%.4f", newRate),
		"{changeDirection}", changeDirection,
	)
	return r.Replace(tpl)
}
