package dashboard

import (
	"context"
	"log"
	"strings"
	"time"

	"transithub/backend/internal/modules/admin_accounts"
	"transithub/backend/internal/modules/upstream"
)

// 后台刷新协程的扫描间隔。upstream 的刷新阈值（refreshSkewMS）为 60 秒，
// 这里用 30 秒扫描，确保临期会话能在过期前被刷新。
const refreshInterval = 30 * time.Second

// SessionStore 抽象 admin 会话的持久化，便于解耦与测试，由 Redis Repository 实现。
type SessionStore interface {
	Get(ctx context.Context, userID string, adminAccountID string) (*AdminSession, error)
	Save(ctx context.Context, userID string, adminAccountID string, session AdminSession) error
	Delete(ctx context.Context, userID string, adminAccountID string) error
	ActiveSessions(ctx context.Context) ([]ActiveSessionRef, error)
}

type AdminAccountService interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
	UpsertAndSwitch(ctx context.Context, userID string, input admin_accounts.UpsertInput) (admin_accounts.Account, error)
}

// PlatformClient 是仪表盘需要复用的上游平台能力子集，由 upstream.PlatformService 实现。
// 仪表盘不重复实现底层 HTTP 调用，只在其之上做自己的会话管理与指标采集。
type PlatformClient interface {
	NormalizeURL(value string) (string, error)
	// 平台中性方法
	LoginAdmin(baseURL string, platform upstream.Platform, account string, password string) (upstream.Session, error)
	VerifyAdmin(session upstream.Session) error
	RefreshSession(session upstream.Session) (upstream.Session, error)
	FetchAdminUsageStats(session upstream.Session, startDate, endDate string) (float64, error)
	FetchAdminSiteBalanceFiltered(session upstream.Session, filter upstream.BalanceFilter) (upstream.AdminSiteBalance, error)
	FetchAdminGroups(session upstream.Session) ([]upstream.GroupInfo, error)
	FetchAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error)
	FetchAdminGroupDailyStats(session upstream.Session, groups []upstream.GroupInfo) ([]upstream.GroupDailyStat, error)
	// sub2api 专用方法（token 登录和旧接口保留）
	LoginSub2APIAdmin(baseURL string, email string, password string) (upstream.Session, error)
	LoginWithToken(baseURL string, platform upstream.Platform, account string, accessToken string, refreshToken string, tokenType string) (upstream.LoginResult, error)
	VerifySub2APIAdmin(session upstream.Session) error
	FetchSub2APIAdminUsageStats(session upstream.Session, startDate, endDate string) (float64, error)
	FetchSub2APIAdminSiteBalanceFiltered(session upstream.Session, filter upstream.BalanceFilter) (upstream.AdminSiteBalance, error)
	FetchSub2APIAdminGroups(session upstream.Session) ([]upstream.GroupInfo, error)
	FetchSub2APIAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error)
}

// MySiteStateSync 用于在 dashboard 登录成功后将 admin session 同步到 my_site_states 表，
// 使 RealConnect 等依赖 my_site_states 的功能可以使用 admin 会话。
type MySiteStateSync interface {
	SyncAdminSession(ctx context.Context, userID string, adminAccountID string, session upstream.Session, identity string)
}

// Service 负责仪表盘 admin 账户的登录、状态校验、退出与令牌自动刷新。
type Service struct {
	store      SessionStore
	platform   PlatformClient
	mySiteSync MySiteStateSync
	accounts   AdminAccountService
}

func NewService(store SessionStore, platform PlatformClient) *Service {
	return &Service{store: store, platform: platform}
}

// SetMySiteSync 注入 MySiteStateSync 实现，在 httpserver 组装时调用。
func (s *Service) SetMySiteSync(sync MySiteStateSync) {
	s.mySiteSync = sync
}

func (s *Service) SetAdminAccountService(accounts AdminAccountService) {
	s.accounts = accounts
}

func nowMillis() int64 {
	return time.Now().UnixMilli()
}

// Status 返回当前用户的 admin 登录状态。
// 若存在会话：先按需刷新令牌并持久化，再校验 admin 角色；
// 校验失败则报告未登录，让前端重新弹窗（不主动删除，避免误删临时网络故障的会话）。
func (s *Service) Status(ctx context.Context, userID string) (StatusResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return StatusResponse{}, err
	}
	record, err := s.store.Get(ctx, userID, adminAccountID)
	if err != nil {
		return StatusResponse{}, err
	}
	if record == nil || !record.Session.IsAuthenticated() {
		return StatusResponse{Authenticated: false}, nil
	}

	if refreshed, changed := s.refreshIfNeeded(record); changed {
		if err := s.store.Save(ctx, userID, adminAccountID, *refreshed); err != nil {
			return StatusResponse{}, err
		}
		record = refreshed
	}

	if err := s.platform.VerifyAdmin(record.Session); err != nil {
		return statusFromRecord(*record, false), nil
	}
	return statusFromRecord(*record, true), nil
}

// Login 处理 sub2api 和 new-api 的登录方式，登录成功后保存会话。
// sub2api 支持 password 和 token 两种方式，new-api 只支持 password（账号密码）。
func (s *Service) Login(ctx context.Context, userID string, req LoginRequest) (StatusResponse, error) {
	platform := strings.TrimSpace(req.Platform)
	if platform == "" {
		platform = PlatformSub2API
	}

	siteURL := strings.TrimSpace(req.SiteURL)
	if siteURL == "" {
		return StatusResponse{}, requestError(ErrorMissingCredentials)
	}

	method := strings.TrimSpace(req.AuthMethod)
	var session upstream.Session
	var identity string

	switch platform {
	case PlatformNewAPI:
		// new-api 只支持账号密码登录
		account := strings.TrimSpace(req.Email)
		if account == "" || strings.TrimSpace(req.Password) == "" {
			return StatusResponse{}, requestError(ErrorMissingCredentials)
		}
		sess, err := s.platform.LoginAdmin(siteURL, upstream.PlatformNewAPI, account, req.Password)
		if err != nil {
			return StatusResponse{}, mapPlatformError(err)
		}
		session = sess
		identity = account
		method = AuthMethodPassword

	case PlatformSub2API:
		switch method {
		case AuthMethodPassword:
			email := strings.TrimSpace(req.Email)
			if email == "" || strings.TrimSpace(req.Password) == "" {
				return StatusResponse{}, requestError(ErrorMissingCredentials)
			}
			sess, err := s.platform.LoginSub2APIAdmin(siteURL, email, req.Password)
			if err != nil {
				return StatusResponse{}, mapPlatformError(err)
			}
			session = sess
			identity = email

		case AuthMethodToken:
			accessToken := strings.TrimSpace(req.AccessToken)
			refreshToken := strings.TrimSpace(req.RefreshToken)
			if accessToken == "" && refreshToken == "" {
				return StatusResponse{}, requestError(ErrorMissingCredentials)
			}
			result, err := s.platform.LoginWithToken(siteURL, upstream.PlatformSub2API, "", accessToken, refreshToken, req.TokenType)
			if err != nil {
				return StatusResponse{}, mapPlatformError(err)
			}
			if err := s.platform.VerifySub2APIAdmin(result.Session); err != nil {
				return StatusResponse{}, mapPlatformError(err)
			}
			session = result.Session

		default:
			return StatusResponse{}, requestError(ErrorMissingCredentials)
		}

	default:
		return StatusResponse{}, requestError(ErrorPlatformUnsupported)
	}

	now := nowMillis()
	baseURL := session.BaseURL
	if baseURL == "" {
		baseURL = siteURL
	}
	adminAccount, err := s.upsertAndSwitchAdminAccount(ctx, userID, admin_accounts.UpsertInput{
		Platform:    platform,
		BaseURL:     baseURL,
		Identity:    identity,
		DisplayName: identity,
		AuthMethod:  method,
	})
	if err != nil {
		return StatusResponse{}, err
	}
	record := AdminSession{
		Platform:        platform,
		BaseURL:         baseURL,
		AuthMethod:      method,
		Identity:        identity,
		Session:         session,
		CreatedAt:       now,
		LastRefreshedAt: now,
	}
	if err := s.store.Save(ctx, userID, adminAccount.ID, record); err != nil {
		return StatusResponse{}, err
	}

	// 同步写入 my_site_states，确保 RealConnect 等功能可使用 admin 会话
	if s.mySiteSync != nil {
		s.mySiteSync.SyncAdminSession(ctx, userID, adminAccount.ID, session, identity)
	}

	return statusFromRecord(record, true), nil
}

// Logout 退出当前 admin 账户（仅清除仪表盘 admin 会话，不影响 TransitHub 后台登录）。
func (s *Service) Logout(ctx context.Context, userID string) error {
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return err
	}
	return s.store.Delete(ctx, userID, adminAccountID)
}

// RefreshAdminSession 由前端主动触发，刷新当前 admin session 并重新校验 admin 身份。
// 与 refreshIfNeeded（后台定时刷新，未变化时跳过校验）不同：这里无论 token 是否实际变化，
// 都必须重新调用 VerifyAdmin，因为“更新管理员凭证”按钮的语义就是主动确认当前凭证仍然有效。
// 刷新失败或校验失败统一返回 ErrorAdminOnly，不删除旧 session，让前端弹窗要求重新登录。
func (s *Service) RefreshAdminSession(ctx context.Context, userID string) (StatusResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccount(ctx, userID)
	if err != nil {
		return StatusResponse{}, err
	}
	record, err := s.store.Get(ctx, userID, adminAccountID)
	if err != nil {
		return StatusResponse{}, err
	}
	if record == nil || !record.Session.IsAuthenticated() {
		return StatusResponse{}, requestError(ErrorAdminOnly)
	}

	refreshedSession, err := s.platform.RefreshSession(record.Session)
	if err != nil {
		return StatusResponse{}, requestError(ErrorAdminOnly)
	}
	if err := s.platform.VerifyAdmin(refreshedSession); err != nil {
		return StatusResponse{}, requestError(ErrorAdminOnly)
	}

	next := *record
	next.Session = refreshedSession
	next.LastRefreshedAt = nowMillis()
	if err := s.store.Save(ctx, userID, adminAccountID, next); err != nil {
		return StatusResponse{}, err
	}

	// 同步写入 my_site_states，确保 RealConnect 等功能使用最新的 admin 会话
	if s.mySiteSync != nil {
		s.mySiteSync.SyncAdminSession(ctx, userID, adminAccountID, next.Session, next.Identity)
	}

	return statusFromRecord(next, true), nil
}

// StartRefresher 启动后台协程，周期性扫描活跃会话并刷新临期令牌，随 ctx 结束而退出。
func (s *Service) StartRefresher(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.refreshDueSessions(ctx)
			}
		}
	}()
}

// refreshDueSessions 遍历所有活跃用户的会话，刷新临期令牌并持久化。
// 单个会话出错只记录日志、不影响其余会话与调度循环。
func (s *Service) refreshDueSessions(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("dashboard admin refresh panic recovered: %v", r)
		}
	}()

	refs, err := s.store.ActiveSessions(ctx)
	if err != nil {
		log.Printf("dashboard admin refresh list active failed: %v", err)
		return
	}
	for _, ref := range refs {
		record, err := s.store.Get(ctx, ref.UserID, ref.AdminAccountID)
		if err != nil || record == nil {
			continue
		}
		refreshed, changed := s.refreshIfNeeded(record)
		if !changed {
			continue
		}
		if err := s.store.Save(ctx, ref.UserID, ref.AdminAccountID, *refreshed); err != nil {
			log.Printf("dashboard admin refresh save failed user_id=%s admin_account_id=%s err=%v", ref.UserID, ref.AdminAccountID, err)
			continue
		}
		log.Printf("dashboard admin token refreshed user_id=%s admin_account_id=%s base_url=%s", ref.UserID, ref.AdminAccountID, refreshed.BaseURL)
	}
}

func (s *Service) requireCurrentAdminAccount(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", requestError(ErrorAdminOnly)
	}
	return s.accounts.RequireCurrentID(ctx, userID)
}

func (s *Service) upsertAndSwitchAdminAccount(ctx context.Context, userID string, input admin_accounts.UpsertInput) (admin_accounts.Account, error) {
	if s.accounts == nil {
		return admin_accounts.Account{}, requestError(ErrorAdminOnly)
	}
	return s.accounts.UpsertAndSwitch(ctx, userID, input)
}

// refreshIfNeeded 在会话带有 refresh token 且令牌临期时刷新它。
// upstream.RefreshSession 内部已按过期时间判定是否真正刷新，未刷新时返回原会话。
func (s *Service) refreshIfNeeded(record *AdminSession) (*AdminSession, bool) {
	if strings.TrimSpace(record.Session.RefreshToken) == "" {
		return record, false
	}
	refreshedSession, err := s.platform.RefreshSession(record.Session)
	if err != nil {
		// 刷新失败（例如 refresh token 失效）保留原会话，后续 Status 校验会判定未登录。
		return record, false
	}
	if sessionEqual(refreshedSession, record.Session) {
		return record, false
	}
	next := *record
	next.Session = refreshedSession
	next.LastRefreshedAt = nowMillis()
	return &next, true
}

func statusFromRecord(record AdminSession, authenticated bool) StatusResponse {
	return StatusResponse{
		Authenticated: authenticated,
		Platform:      record.Platform,
		BaseURL:       record.BaseURL,
		AuthMethod:    record.AuthMethod,
		Identity:      record.Identity,
		ExpiresAt:     record.Session.ExpiresAt,
	}
}

// mapPlatformError 把 upstream 客户端的错误 key 归并到仪表盘自己的 i18n key。
func mapPlatformError(err error) requestError {
	switch err.Error() {
	case upstream.ErrorInvalidURL:
		return requestError(ErrorInvalidURL)
	case upstream.ErrorNetwork:
		return requestError(ErrorNetwork)
	default:
		// 登录失败或 /api/v1/auth/me 校验未通过，统一归为「非 admin / 鉴权失败」。
		return requestError(ErrorAdminOnly)
	}
}

// sessionEqual 判断刷新前后会话是否发生变化，用于决定是否需要持久化。
func sessionEqual(a, b upstream.Session) bool {
	if a.AccessToken != b.AccessToken || a.RefreshToken != b.RefreshToken || a.TokenType != b.TokenType {
		return false
	}
	if (a.ExpiresAt == nil) != (b.ExpiresAt == nil) {
		return false
	}
	if a.ExpiresAt != nil && b.ExpiresAt != nil && *a.ExpiresAt != *b.ExpiresAt {
		return false
	}
	return true
}
