package my_sites

import (
	"context"

	"transithub/backend/internal/modules/upstream"
)

// StateRepository 是管理员会话的持久化接口，由 Repository 实现。
type StateRepository interface {
	Get(ctx context.Context, userID string, adminAccountID string) (*State, error)
	Save(ctx context.Context, state State) error
}

// SessionPlatform 是刷新令牌和校验 admin 角色所需的站点能力，由 upstream.PlatformService 实现。
type SessionPlatform interface {
	RefreshSession(session upstream.Session) (upstream.Session, error)
	VerifyAdmin(session upstream.Session) error
}

// Service 保存每个工作区「我的站点」的管理员会话：仪表盘登录或刷新令牌后通过 SyncAdminSession 写入，
// 仪表盘指标、用户活跃和群发邮件通过 RequireSession 读取（必要时刷新令牌并校验 admin 角色）。
type Service struct {
	repository StateRepository
	platform   SessionPlatform
}

func NewService(repository StateRepository, platform SessionPlatform) *Service {
	return &Service{repository: repository, platform: platform}
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	if repo, ok := s.repository.(*Repository); ok {
		return repo.EnsureSchema(ctx)
	}
	return nil
}

// RequireSession 返回工作区可用的 admin 会话：令牌临期时先刷新并写回，再校验仍是管理员。
func (s *Service) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	state, err := s.repository.Get(ctx, userID, adminAccountID)
	if err != nil {
		return upstream.Session{}, err
	}
	if state == nil || !state.Session.IsAuthenticated() {
		return upstream.Session{}, requestError(ErrorAuthRequired)
	}
	refreshed, err := s.platform.RefreshSession(state.Session)
	if err != nil {
		return upstream.Session{}, requestError(ErrorAdminOnly)
	}
	if refreshed.AccessToken != state.Session.AccessToken || refreshed.RefreshToken != state.Session.RefreshToken ||
		refreshed.Cookie != state.Session.Cookie {
		state.Session = refreshed
		if err := s.repository.Save(ctx, *state); err != nil {
			return upstream.Session{}, err
		}
	}
	if err := s.platform.VerifyAdmin(state.Session); err != nil {
		return upstream.Session{}, requestError(ErrorAdminOnly)
	}
	return state.Session, nil
}

// SyncAdminSession 实现 dashboard.MySiteStateSync：仪表盘登录成功或刷新令牌后，把 admin 会话写入 my_site_states。
func (s *Service) SyncAdminSession(ctx context.Context, userID string, adminAccountID string, session upstream.Session, identity string) error {
	return s.repository.Save(ctx, State{
		UserID:         userID,
		AdminAccountID: adminAccountID,
		BaseURL:        session.BaseURL,
		Email:          identity,
		Session:        session,
	})
}

// StoredSession 只读取已保存的会话，不刷新也不访问站点。仪表盘据此判断哪一份会话更新，
// 避免站点偶发请求失败时用旧会话覆盖 PostgreSQL 里更新的会话。
func (s *Service) StoredSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, bool, error) {
	state, err := s.repository.Get(ctx, userID, adminAccountID)
	if err != nil {
		return upstream.Session{}, false, err
	}
	if state == nil || !state.Session.IsAuthenticated() {
		return upstream.Session{}, false, nil
	}
	return state.Session, true, nil
}
