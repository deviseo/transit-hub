package httpserver

import (
	"context"
	"errors"
	"fmt"
	"html"
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
	"transithub/backend/internal/modules/client_config"
	"transithub/backend/internal/modules/dashboard"
	"transithub/backend/internal/modules/group_rates"
	"transithub/backend/internal/modules/health"
	"transithub/backend/internal/modules/mass_email"
	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/profit"
	"transithub/backend/internal/modules/reports"
	"transithub/backend/internal/modules/sales"
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
	// 「我的站点」管理员会话：仪表盘登录后写入，仪表盘指标、用户分析、利润分析和群发邮件读取（必要时刷新令牌）。
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

	// 用户分析（模块名 user_activity）：把当前工作区「我的站点」（sub2api / new-api）的用户与每日用量同步到本地后分析；
	// 只通过 my_sites 的管理员会话读取自己站点的数据。群发邮件的 segment 方式按这里的筛选条件解析收件人。
	userActivityService := user_activity.NewService(user_activity.NewRepository(db), mySitesService, platformService, adminAccountsService)
	userActivityService.SetWorkspaceLister(activityWorkspaces{sites: mySitesService})
	user_activity.RegisterRoutes(server.mux, userActivityService)
	massEmailService.SetSegmentResolver(activitySegmentResolver{service: userActivityService})

	// 销售分析：基于用户分析同步到本地的入账、用量与余额数据计算，只读；金额按仪表盘的「我的站点售价」换算。
	salesService := sales.NewService(sales.NewRepository(db), sitePriceProvider{repo: metricsRepo}, adminAccountsService)
	sales.RegisterRoutes(server.mux, salesService)

	// 经营日报：按各工作区设置的时间，把昨天的经营、销售、用户和上游数据发到机器人；数据全部来自本地，不访问站点。
	reportBuilder := reports.NewBuilder(metricsRepo, salesService, userActivityService, workspaceNames{accounts: adminAccountsService})
	reportsService := reports.NewService(
		reportBuilder,
		settingsService,
		settingsService,
		adminAccountsService,
		reports.NewRedisOnceStore(redisClient),
	)
	reports.RegisterRoutes(server.mux, reportsService)

	// 利润分析：自己站点按路由、分组的营收 + 已登记上游站点的成本，按上游域名自动归因；每小时采集完整自然日，只读。
	profitService := profit.NewService(profit.NewRepository(db), mySitesService, platformService, upstreamService, sitePriceProvider{repo: metricsRepo}, adminAccountsService)
	profitService.SetWorkspaceLister(profitWorkspaces{sites: mySitesService})
	profit.RegisterRoutes(server.mux, profitService)

	// TransitLink 客户端配置：「客户端」页面（需要登录）和客户端读取配置的公开接口 /api/client/v1/。
	clientConfigService := client_config.NewService(
		client_config.NewRepository(db),
		clientSites{accounts: adminAccountsService},
		clientGroups{sites: mySitesService, platform: platformService},
	)
	client_config.RegisterRoutes(server.mux, clientConfigService)
	reportBuilder.SetPriceChanges(profitService)

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
	reportsService.StartScheduler(context.Background())
	profitService.StartScheduler(context.Background())

	// 定时同步按工作区配置：保存策略时只更新该工作区，启动时逐个工作区恢复。
	applyRefreshConfig := func(userID string, adminAccountID string, s settings.StrategySettings) {
		upstreamService.SetWorkspaceRefreshConfig(userID, adminAccountID, upstream.RefreshConfig{
			Enabled:  s.EnableRefreshInterval,
			Interval: time.Duration(s.RefreshInterval) * time.Second,
		})
	}
	settingsService.OnStrategyChanged = applyRefreshConfig
	if strategies, err := settingsService.ListStrategies(context.Background()); err != nil {
		log.Printf("[settings] load workspace strategies failed: %v", err)
	} else {
		for _, strategy := range strategies {
			applyRefreshConfig(strategy.UserID, strategy.AdminAccountID, strategy.Settings)
		}
	}

	// 站点同步成功后按站点所属工作区的策略检查余额预警和倍率变更；冷却记录存 Redis，重启后不重复发送。
	upstreamService.AfterSync = syncAlerts{
		strategies: settingsService,
		notifier:   settingsService,
		sites:      upstreamService,
		cooldown:   newRedisAlertCooldown(redisClient),
		prices:     profitService,
	}.AfterSync

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

// clientSites 给 TransitLink 客户端配置提供当前工作区和站点信息（站点就是 admin 账号对应的站长自己的站）。
type clientSites struct {
	accounts *admin_accounts.Service
}

func (c clientSites) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	return c.accounts.RequireCurrentID(ctx, userID)
}

func (c clientSites) Site(ctx context.Context, userID string, adminAccountID string) (client_config.SiteInfo, error) {
	accounts, err := c.accounts.List(ctx, userID)
	if err != nil {
		return client_config.SiteInfo{}, err
	}
	for _, account := range accounts {
		if account.ID != adminAccountID {
			continue
		}
		baseURL := strings.TrimRight(account.BaseURL, "/")
		name := strings.TrimSpace(account.DisplayName)
		if name == "" {
			name = strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
		}
		return client_config.SiteInfo{Platform: account.Platform, BaseURL: baseURL, Name: name}, nil
	}
	return client_config.SiteInfo{}, errors.New("admin account not found")
}

// clientGroups 用工作区保存的 admin 会话读取站点自己的用户分组，给推荐分组的下拉框用。
type clientGroups struct {
	sites    *my_sites.Service
	platform *upstream.PlatformService
}

func (c clientGroups) Groups(ctx context.Context, userID string, adminAccountID string) ([]client_config.Group, error) {
	session, err := c.sites.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		// 站点登录失效、不是管理员：原样返回错误 key，前端提示重新登录站点。
		if strings.HasPrefix(err.Error(), "admin.mySites.errors.") {
			return nil, client_config.RequestError(err.Error())
		}
		return nil, err
	}
	groups, err := c.platform.FetchAdminGroups(session)
	if err != nil {
		return nil, err
	}
	result := make([]client_config.Group, 0, len(groups))
	for _, group := range groups {
		result = append(result, client_config.Group{ID: group.ID, Name: group.Name, Platform: group.Platform, Multiplier: group.Multiplier})
	}
	return result, nil
}

// profitWorkspaces / activityWorkspaces 把「保存了自己站点管理员会话的工作区」交给利润分析和用户分析，
// 让后台任务为还没采集过的工作区自动开始首次采集。
type profitWorkspaces struct {
	sites *my_sites.Service
}

func (p profitWorkspaces) SessionWorkspaces(ctx context.Context) ([]profit.Workspace, error) {
	items, err := p.sites.ListSessionWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	workspaces := make([]profit.Workspace, 0, len(items))
	for _, item := range items {
		workspaces = append(workspaces, profit.Workspace{UserID: item.UserID, AdminAccountID: item.AdminAccountID})
	}
	return workspaces, nil
}

type activityWorkspaces struct {
	sites *my_sites.Service
}

func (a activityWorkspaces) SessionWorkspaces(ctx context.Context) ([]user_activity.Workspace, error) {
	items, err := a.sites.ListSessionWorkspaces(ctx)
	if err != nil {
		return nil, err
	}
	workspaces := make([]user_activity.Workspace, 0, len(items))
	for _, item := range items {
		workspaces = append(workspaces, user_activity.Workspace{UserID: item.UserID, AdminAccountID: item.AdminAccountID})
	}
	return workspaces, nil
}

// sitePriceProvider 把仪表盘的「我的站点售价」提供给销售分析，两处金额使用同一个换算口径。
type sitePriceProvider struct {
	repo *dashboard.MetricsRepository
}

func (p sitePriceProvider) SiteUnitPrice(ctx context.Context, userID string, adminAccountID string) (float64, error) {
	config, err := p.repo.GetPricing(ctx, userID, adminAccountID)
	if err != nil {
		return 0, err
	}
	return config.SiteUnitPrice, nil
}

// workspaceNames 为经营日报提供工作区显示名称。
type workspaceNames struct {
	accounts *admin_accounts.Service
}

func (n workspaceNames) WorkspaceName(ctx context.Context, userID string, adminAccountID string) string {
	accounts, err := n.accounts.List(ctx, userID)
	if err != nil {
		return ""
	}
	for _, account := range accounts {
		if account.ID != adminAccountID {
			continue
		}
		if name := strings.TrimSpace(account.DisplayName); name != "" {
			return name
		}
		return account.BaseURL
	}
	return ""
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
	"/api/sales",
	"/api/reports",
	"/api/profit",
	"/api/client-config",
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

// balanceAlertCooldown 余额预警冷却时间，同一站点在此时间段内不会重复发送通知。
const balanceAlertCooldown = 1 * time.Hour

type alertStrategyReader interface {
	GetStrategyForWorkspace(ctx context.Context, userID string, adminAccountID string) (settings.StrategySettings, error)
}

type alertNotifier interface {
	SendFormattedToWorkspaceBots(ctx context.Context, userID string, adminAccountID string, botIDs []string, message string, format settings.NotificationTemplateFormat)
}

type alertSiteReader interface {
	GetSite(ctx context.Context, siteID string) (*upstream.Site, error)
}

// alertCooldown 控制余额预警的冷却期：首次低于阈值立即通知，冷却期内不再重复，冷却期过后仍低于阈值再次通知；
// 余额恢复到阈值以上时清除记录，下次跌破可立即通知。
type alertCooldown interface {
	// Acquire 在冷却期外返回 true 并开始新的冷却期；冷却期内返回 false。
	Acquire(ctx context.Context, key string, ttl time.Duration) bool
	Reset(ctx context.Context, key string)
}

// syncAlerts 在站点同步成功后检查余额预警和倍率变更。策略、机器人都取站点所属工作区的配置，
// 不能用用户当前选中的工作区（后台同步时用户可能正在看别的工作区）。
type syncAlerts struct {
	strategies alertStrategyReader
	notifier   alertNotifier
	sites      alertSiteReader
	cooldown   alertCooldown
	// prices 估算并记录上游分组调价对每天成本的影响（利润分析使用）；为 nil 时只发通知、不估算。
	prices priceChangeAssessor
}

// priceChangeAssessor 由 profit.Service 实现：只读查询分组在上游近 7 天的消费，估算调价影响并记录。
type priceChangeAssessor interface {
	AssessPriceChanges(ctx context.Context, ws profit.Workspace, siteID string, changes []profit.PriceChangeInput) []profit.PriceChange
}

func (a syncAlerts) AfterSync(ctx context.Context, userID, adminAccountID, siteID, siteName string, oldMetrics, newMetrics upstream.Metrics) {
	if userID == "" || adminAccountID == "" {
		return
	}
	strategy, err := a.strategies.GetStrategyForWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		// 读不到策略时不发通知，但调价仍要记录（利润分析页使用）。
		log.Printf("[alert] load strategy failed admin_account_id=%s err=%v", adminAccountID, err)
		strategy = settings.StrategySettings{}
	}
	a.checkBalanceWarning(ctx, strategy, userID, adminAccountID, siteID, siteName, newMetrics)
	a.checkMultiplierChanges(ctx, strategy, userID, adminAccountID, siteID, siteName, oldMetrics, newMetrics)
}

// checkBalanceWarning 检测余额是否低于阈值并发送通知。
// 优先使用站点级 BalanceThreshold 覆盖工作区的 DefaultBalanceThreshold；站点设置为 nil 时使用工作区默认值。
func (a syncAlerts) checkBalanceWarning(ctx context.Context, strategy settings.StrategySettings, userID, adminAccountID, siteID, siteName string, newMetrics upstream.Metrics) {
	if !strategy.EnableBalanceWarning || len(strategy.BalanceNotifyBotIDs) == 0 {
		return
	}
	if newMetrics.Balance.Value == nil {
		return
	}

	// 获取站点充值倍率，用于将 USD 余额转换为 CNY 后与阈值比较。
	site, err := a.sites.GetSite(ctx, siteID)
	if err != nil || site == nil {
		return
	}
	rechargeRate := site.RechargeRate
	if rechargeRate <= 0 {
		rechargeRate = 1
	}

	newBalCNY := *newMetrics.Balance.Value * rechargeRate

	// 站点级阈值覆盖：有值则用站点配置，否则使用工作区默认（均为 CNY）。
	threshold := strategy.DefaultBalanceThreshold
	if site.Settings.BalanceThreshold != nil {
		threshold = *site.Settings.BalanceThreshold
	}

	cooldownKey := "balance:" + siteID
	if newBalCNY >= threshold {
		a.cooldown.Reset(ctx, cooldownKey)
		return
	}
	if !a.cooldown.Acquire(ctx, cooldownKey, balanceAlertCooldown) {
		return
	}

	msg := formatBalanceWarning(siteName, newBalCNY, threshold, strategy.BalanceTemplate)
	log.Printf("[alert] 余额预警触发 site=%s balanceCNY=%.2f threshold=%.2f rechargeRate=%.2f", siteName, newBalCNY, threshold, rechargeRate)
	a.notifier.SendFormattedToWorkspaceBots(ctx, userID, adminAccountID, strategy.BalanceNotifyBotIDs, msg, strategy.BalanceTemplateFormat)
}

// multiplierChanges 对比同步前后的分组倍率（按分组 ID + 名称匹配；新出现或消失的分组不算调价）。
func multiplierChanges(oldMetrics, newMetrics upstream.Metrics) []profit.PriceChangeInput {
	if len(oldMetrics.Groups) == 0 {
		return nil
	}
	oldMap := make(map[string]float64, len(oldMetrics.Groups))
	for _, g := range oldMetrics.Groups {
		if g.Multiplier != nil {
			oldMap[upstream.GroupKey(g)] = *g.Multiplier
		}
	}
	changes := make([]profit.PriceChangeInput, 0)
	for _, g := range newMetrics.Groups {
		if g.Multiplier == nil {
			continue
		}
		oldVal, existed := oldMap[upstream.GroupKey(g)]
		if !existed || oldVal == *g.Multiplier {
			continue
		}
		changes = append(changes, profit.PriceChangeInput{Group: g, OldMultiplier: oldVal, NewMultiplier: *g.Multiplier})
	}
	return changes
}

// checkMultiplierChanges 检测分组倍率变化：先估算并记录对每天成本的影响（与通知开关无关，利润分析页使用），
// 再按工作区策略 EnableMultiplierAlert 发送通知，通知里附带成本影响。只是提示，不会修改任何价格。
func (a syncAlerts) checkMultiplierChanges(ctx context.Context, strategy settings.StrategySettings, userID, adminAccountID, siteID, siteName string, oldMetrics, newMetrics upstream.Metrics) {
	changes := multiplierChanges(oldMetrics, newMetrics)
	if len(changes) == 0 {
		return
	}
	var assessed []profit.PriceChange
	if a.prices != nil {
		assessed = a.prices.AssessPriceChanges(ctx, profit.Workspace{UserID: userID, AdminAccountID: adminAccountID}, siteID, changes)
	}
	if !strategy.EnableMultiplierAlert || len(strategy.MultiplierNotifyBotIDs) == 0 {
		return
	}
	for i, change := range changes {
		impact := ""
		if i < len(assessed) {
			impact = assessed[i].ImpactText()
		}
		msg := formatMultiplierChange(siteName, change.Group.Name, change.OldMultiplier, change.NewMultiplier, impact, strategy.MultiplierTemplate, strategy.MultiplierTemplateFormat)
		log.Printf("[alert] 倍率变更触发 site=%s group=%s old=%.4f new=%.4f", siteName, change.Group.Name, change.OldMultiplier, change.NewMultiplier)
		a.notifier.SendFormattedToWorkspaceBots(ctx, userID, adminAccountID, strategy.MultiplierNotifyBotIDs, msg, strategy.MultiplierTemplateFormat)
	}
}

// redisAlertCooldown 把冷却记录存进 Redis（SET NX EX），进程重启后不会重复发送；Redis 不可用时退回进程内记录。
type redisAlertCooldown struct {
	client   *redis.Client
	fallback *memoryAlertCooldown
}

const alertCooldownKeyPrefix = "transithub:alert-cooldown:"

func newRedisAlertCooldown(client *redis.Client) redisAlertCooldown {
	return redisAlertCooldown{client: client, fallback: newMemoryAlertCooldown()}
}

func (c redisAlertCooldown) Acquire(ctx context.Context, key string, ttl time.Duration) bool {
	if c.client == nil {
		return c.fallback.Acquire(ctx, key, ttl)
	}
	acquired, err := c.client.SetNX(ctx, alertCooldownKeyPrefix+key, "1", ttl).Result()
	if err != nil {
		log.Printf("[alert] redis cooldown unavailable, using in-process cooldown: %v", err)
		return c.fallback.Acquire(ctx, key, ttl)
	}
	return acquired
}

func (c redisAlertCooldown) Reset(ctx context.Context, key string) {
	c.fallback.Reset(ctx, key)
	if c.client == nil {
		return
	}
	if err := c.client.Del(ctx, alertCooldownKeyPrefix+key).Err(); err != nil {
		log.Printf("[alert] redis cooldown reset failed: %v", err)
	}
}

// memoryAlertCooldown 是进程内的冷却记录（Redis 不可用时的兜底，也用于测试）。
type memoryAlertCooldown struct {
	mu       sync.Mutex
	now      func() time.Time
	lastSent map[string]time.Time
}

func newMemoryAlertCooldown() *memoryAlertCooldown {
	return &memoryAlertCooldown{now: time.Now, lastSent: make(map[string]time.Time)}
}

func (c *memoryAlertCooldown) Acquire(ctx context.Context, key string, ttl time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if last, ok := c.lastSent[key]; ok && now.Sub(last) < ttl {
		return false
	}
	c.lastSent[key] = now
	return true
}

func (c *memoryAlertCooldown) Reset(ctx context.Context, key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.lastSent, key)
}

const defaultBalanceTemplate = "🔴 余额预警\n🏷️ 站点：{siteName}\n💰 当前余额：¥{balance}\n⚠️ 预警阈值：¥{threshold}\n请及时检查并充值，避免服务中断。"
const defaultMultiplierTemplate = "🟠 倍率变更预警\n🏷️ 站点：{siteName}\n📦 分组：{groupName}\n📊 变更：{oldRate}x → {newRate}x（{changeDirection}）\n💸 成本影响：{costImpact}\n请及时确认定价与路由策略。"

// costImpactUnavailable 是没有估算结果时 {costImpact} 的取值。
const costImpactUnavailable = "暂时无法估算"

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

func formatMultiplierChange(siteName, groupName string, oldRate, newRate float64, impact string, customTemplate string, format settings.NotificationTemplateFormat) string {
	tpl := customTemplate
	if tpl == "" {
		tpl = defaultMultiplierTemplate
	}
	changeDirection := "上升"
	if newRate < oldRate {
		changeDirection = "下降"
	}
	impactValue := impact
	if impactValue == "" {
		impactValue = costImpactUnavailable
	}
	r := strings.NewReplacer(
		"{siteName}", siteName,
		"{groupName}", groupName,
		"{oldRate}", fmt.Sprintf("%.4f", oldRate),
		"{newRate}", fmt.Sprintf("%.4f", newRate),
		"{changeDirection}", changeDirection,
		"{costImpact}", impactValue,
	)
	message := r.Replace(tpl)
	// 之前保存的模板没有 {costImpact}：有估算结果时追加在末尾，保证通知里能看到成本影响。
	if impact != "" && !strings.Contains(tpl, "{costImpact}") {
		switch format {
		case settings.NotificationTemplateFormatHTML:
			message += `<p style="margin:10px 0 0">💸 成本影响：` + html.EscapeString(impact) + `</p>`
		case settings.NotificationTemplateFormatMarkdown:
			message += "\n\n💸 **成本影响：** " + impact
		default:
			message += "\n💸 成本影响：" + impact
		}
	}
	return message
}
