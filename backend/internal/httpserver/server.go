package httpserver

import (
	"context"
	"errors"
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
	"transithub/backend/internal/modules/dashboard"
	"transithub/backend/internal/modules/group_rates"
	"transithub/backend/internal/modules/health"
	"transithub/backend/internal/modules/mass_email"
	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/modules/system"
	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/modules/user_activity"
	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
	"transithub/backend/internal/shared/outbound"
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
	// 注意：曾经存在的 GET /api/users 未经鉴权即返回全部 TransitHub 账号邮箱，且前端从未使用，
	// 已整体移除。不要以「加登录校验」的方式恢复它——任何已注册用户仍可借此枚举其他租户邮箱。
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

	// 出站代理：访问上游站点和自己的 sub2api/new-api 站点共用同一套代理配置。
	// 配置保存在「系统设置 - 网络代理」中，修改后立即生效；从未保存过时沿用启动环境变量（HTTPS_PROXY/NO_PROXY），
	// 保持旧部署行为不变。
	outboundProxy := outbound.NewController()
	systemService := system.NewService(cfg)
	systemService.SetOutboundProxy(system.NewRepository(db), outboundProxy, systemAdminChecker{auth: authService, adminEmail: cfg.AdminEmail})
	if err := systemService.LoadOutboundProxy(context.Background()); err != nil {
		log.Printf("[outbound-proxy] load saved settings failed, falling back to environment variables: %v", err)
	}

	upstreamHTTPClient := outboundProxy.NewClient(upstreamRequestTimeout)
	platformService := upstream.NewPlatformService(upstream.NewHTTPClient(upstreamHTTPClient))
	upstreamCache := upstream.NewRedisSiteCache(redisClient)
	upstreamService := upstream.NewService(platformService, upstreamRepository, groupRateSnapshotWriter{service: groupRatesService}, upstreamCache)
	upstreamService.SetAdminAccountResolver(adminAccountsService)
	upstream.RegisterRoutes(server.mux, upstreamService, adminAccountsService)
	// 「我的站点」管理员会话：仪表盘登录后写入，仪表盘指标、用户活跃和群发邮件读取（必要时刷新令牌）。
	mySitesService := my_sites.NewService(my_sites.NewRepository(db), platformService)
	if err := mySitesService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}

	settingsService := settings.NewService(http.DefaultClient, settings.NewRepository(db))
	settingsService.SetAdminAccountResolver(adminAccountsService)
	if err := settingsService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	// SMTP_ENCRYPTION_KEY 是可选项：空值不影响启动；显式配置了非法值（非 base64 或非 32 字节）
	// 必须尽早启动失败，避免运行时才发现加密能力不可用。抽成 configureSMTPEncryptionKey
	// 这个窄 seam，便于在不启动真实 DB/Redis 依赖的情况下单元测试这条组装路径。
	if _, err := configureSMTPEncryptionKey(settingsService, cfg.SMTPEncryptionKey); err != nil {
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

	// 批量邮件模块只复用已保存的模板/SMTP 配置和当前 workspace 的 Sub2API admin 会话；
	// 创建批次时只解析收件人，真正 SMTP 发送由 Postgres-backed worker 异步执行。
	massEmailService := mass_email.NewService(
		mass_email.NewRepository(db),
		mySitesService,
		platformService,
		settingsService,
	)
	if err := massEmailService.EnsureSchema(context.Background()); err != nil {
		panic(err)
	}
	mass_email.RegisterRoutes(server.mux, massEmailService, adminAccountsService)
	// 退订页面供邮件收件人访问，不需要 TransitHub 登录（/api/email-unsubscribe 不在 protectedPath 中）。
	mass_email.RegisterPublicRoutes(server.mux, massEmailService)
	massEmailWorker := mass_email.NewWorker(massEmailService)

	// 用户活跃分析：把当前工作区「我的站点」（sub2api / new-api）的用户与每日用量同步到本地后分析；
	// 只通过 my_sites 的管理员会话读取自己站点的数据。群发邮件的 segment 方式按这里的筛选条件解析收件人。
	userActivityService := user_activity.NewService(user_activity.NewRepository(db), mySitesService, platformService, adminAccountsService)
	user_activity.RegisterRoutes(server.mux, userActivityService)
	massEmailService.SetSegmentResolver(activitySegmentResolver{service: userActivityService})

	// 所有 workspace 表 schema 完成后再补 legacy 归属；随后才启动 restore、worker 和 scheduler，
	// 避免后台任务在旧行尚未补齐 workspace 时读取或写回数据。
	if err := adminAccountsService.AssignLegacyRows(context.Background()); err != nil {
		panic(err)
	}
	if err := upstreamService.RestoreSavedSites(context.Background()); err != nil {
		panic(err)
	}
	massEmailWorker.Start(context.Background())
	userActivityService.StartScheduler(context.Background())

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
	upstreamService.AfterSync = func(ctx context.Context, userID, _ /* adminAccountID */, siteID, siteName string, oldMetrics, newMetrics upstream.Metrics) {
		strategy, err := settingsService.GetFirstStrategy(ctx)
		if err != nil {
			return
		}
		checkBalanceWarning(ctx, settingsService, upstreamService, strategy, userID, siteID, siteName, oldMetrics, newMetrics)
		checkMultiplierChanges(ctx, settingsService, strategy, userID, siteID, siteName, oldMetrics, newMetrics)
	}

	settings.RegisterRoutes(server.mux, settingsService)

	// 仪表盘 admin 登录门禁：复用 sub2api 平台客户端（platformService），会话存于 Redis，
	// 并启动后台协程对临期令牌做自动刷新。
	dashboardSessionStore := dashboard.NewRepository(redisClient)
	dashboardService := dashboard.NewService(dashboardSessionStore, platformService)
	dashboardService.SetAdminAccountService(adminAccountsService)
	dashboardService.SetMySiteSync(mySitesService)
	adminAccountsService.SetWorkspaceCleanup(workspaceCleanup{
		dashboardSessions: dashboardSessionStore,
		upstreamSites:     upstreamService,
	})
	adminAccountsService.StartCleanupWorker(context.Background(), time.Minute)
	dashboardService.StartRefresher(context.Background())

	// 仪表盘指标服务：实时计算五项核心指标 + 历史趋势快照。
	// 复用 dashboard 的 Redis 会话存储与 sub2api 平台客户端，
	// 并复用 upstreamService 读取已同步的上游站点数据（无额外 API 调用）。
	metricsService := dashboard.NewMetricsService(dashboardSessionStore, platformService, upstreamService, metricsRepo, adminAccountsService)
	metricsService.SetMySiteSync(mySitesService)
	metricsService.StartScheduler(context.Background())
	dashboard.RegisterRoutes(server.mux, dashboardService, metricsService)

	// 系统信息 API：版本号与系统级设置（出站代理），systemService 已在上方创建并加载代理配置。
	system.RegisterRoutes(server.mux, systemService)

	return server
}

// activitySegmentResolver 把用户活跃分析的筛选条件适配为群发邮件 segment 方式的收件人解析。
type activitySegmentResolver struct {
	service *user_activity.Service
}

func (r activitySegmentResolver) ResolveSegmentRecipients(ctx context.Context, userID string, adminAccountID string, filters mass_email.BatchFilters, max int) ([]mass_email.SegmentRecipient, error) {
	recipients, err := r.service.Recipients(ctx, user_activity.Workspace{UserID: userID, AdminAccountID: adminAccountID}, user_activity.UserFilter{
		Segment:      filters.Segment,
		InactiveDays: filters.InactiveDays,
		Search:       filters.Search,
		Status:       filters.Status,
		Role:         filters.Role,
	}, max)
	if err != nil {
		if user_activity.IsRequestError(err) {
			switch err.Error() {
			case user_activity.ErrorTooManyRecipient:
				return nil, mass_email.ErrRecipientLimitReached
			case user_activity.ErrorNotSynced:
				return nil, mass_email.ErrSegmentNotSynced
			case user_activity.ErrorInvalidFilter:
				return nil, mass_email.ErrInvalidSelection
			}
		}
		return nil, err
	}
	result := make([]mass_email.SegmentRecipient, 0, len(recipients))
	for _, recipient := range recipients {
		result = append(result, mass_email.SegmentRecipient{UpstreamUserID: recipient.SiteUserID, Email: recipient.Email, Username: recipient.Username})
	}
	return result, nil
}

// systemAdminChecker 把 auth.Service 与 ADMIN_EMAIL 适配为 system.SystemAdminChecker。
type systemAdminChecker struct {
	auth       *auth.Service
	adminEmail string
}

func (c systemAdminChecker) IsSystemAdmin(ctx context.Context, userID string) (bool, error) {
	return c.auth.IsSystemAdmin(ctx, userID, c.adminEmail)
}

type groupRateSnapshotWriter struct {
	service *group_rates.Service
}

type dashboardSessionCleaner interface {
	Delete(ctx context.Context, userID string, adminAccountID string) error
}

type upstreamSiteCleaner interface {
	CleanupDeletedWorkspaceSites(ctx context.Context, userID string, siteIDs []string) error
}

// workspaceCleanup 在工作区删除提交后清理数据库之外的状态（Redis 会话、上游站点缓存）。
type workspaceCleanup struct {
	dashboardSessions dashboardSessionCleaner
	upstreamSites     upstreamSiteCleaner
}

func (c workspaceCleanup) CleanupDeletedWorkspace(ctx context.Context, payload admin_accounts.WorkspaceCleanupPayload) error {
	var errs []error
	if c.dashboardSessions != nil {
		if err := c.dashboardSessions.Delete(ctx, payload.UserID, payload.AdminAccountID); err != nil {
			errs = append(errs, fmt.Errorf("dashboard session cleanup: %w", err))
		}
	}
	if c.upstreamSites != nil {
		if err := c.upstreamSites.CleanupDeletedWorkspaceSites(ctx, payload.UserID, payload.UpstreamSiteIDs); err != nil {
			errs = append(errs, fmt.Errorf("upstream site cleanup: %w", err))
		}
	}
	return errors.Join(errs...)
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
		s.setSecurityHeaders(w, r)
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

// Shutdown 预留给需要优雅停止的后台任务；目前的后台任务都随进程退出，无需等待。
func (s *Server) Shutdown(ctx context.Context) error {
	return nil
}

// protectedPathPrefixes 列出需要 TransitHub 登录的接口前缀；未列出的 /api 路径（健康检查、登录、
// 邮件退订页）由各自的 handler 负责鉴权。
var protectedPathPrefixes = []string{
	"/api/admin-accounts",
	"/api/upstream-sites",
	"/api/group-rates",
	"/api/settings",
	"/api/dashboard",
	"/api/system",
	"/api/mass-email",
	"/api/user-activity",
}

func (s *Server) protectedPath(path string) bool {
	for _, prefix := range protectedPathPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func (s *Server) setSecurityHeaders(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
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
	svc.SendFormattedToBots(ctx, userID, strategy.BalanceNotifyBotIDs, msg, strategy.BalanceTemplateFormat)
}

// checkMultiplierChanges 对比同步前后的分组倍率，任何变化都发送通知。
// 只受系统设置全局开关 strategy.EnableMultiplierAlert 控制。
func checkMultiplierChanges(ctx context.Context, svc *settings.Service, strategy settings.StrategySettings, userID, siteID, siteName string, oldMetrics, newMetrics upstream.Metrics) {
	_ = siteID
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
		svc.SendFormattedToBots(ctx, userID, strategy.MultiplierNotifyBotIDs, msg, strategy.MultiplierTemplateFormat)
	}
}

const defaultBalanceTemplate = "🔴 余额预警\n🏷️ 站点：{siteName}\n💰 当前余额：¥{balance}\n⚠️ 预警阈值：¥{threshold}\n请及时检查并充值，避免服务中断。"
const defaultMultiplierTemplate = "🟠 倍率变更预警\n🏷️ 站点：{siteName}\n📦 分组：{groupName}\n📊 变更：{oldRate}x → {newRate}x（{changeDirection}）\n请及时确认定价与路由策略。"

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
