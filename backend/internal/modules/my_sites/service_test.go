package my_sites

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

// 会话服务单元测试：仓储与站点均为内存假实现，不访问网络。

type memoryStateRepo struct {
	states map[string]State
	saves  int
}

func newMemoryStateRepo() *memoryStateRepo {
	return &memoryStateRepo{states: map[string]State{}}
}

func (r *memoryStateRepo) Get(ctx context.Context, userID string, adminAccountID string) (*State, error) {
	state, ok := r.states[userID+"|"+adminAccountID]
	if !ok {
		return nil, nil
	}
	return &state, nil
}

func (r *memoryStateRepo) Save(ctx context.Context, state State) error {
	r.saves++
	r.states[state.UserID+"|"+state.AdminAccountID] = state
	return nil
}

type fakeSessionPlatform struct {
	refreshed  *upstream.Session
	refreshErr error
	verifyErr  error
	verified   []upstream.Session
}

func (p *fakeSessionPlatform) RefreshSession(session upstream.Session) (upstream.Session, error) {
	if p.refreshErr != nil {
		return upstream.Session{}, p.refreshErr
	}
	if p.refreshed != nil {
		return *p.refreshed, nil
	}
	return session, nil
}

func (p *fakeSessionPlatform) VerifyAdmin(session upstream.Session) error {
	p.verified = append(p.verified, session)
	return p.verifyErr
}

func adminSession(token string) upstream.Session {
	return upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://site.example.test", AccessToken: token, RefreshToken: "refresh-" + token}
}

func TestRequireSessionWithoutStoredSessionAsksForLogin(t *testing.T) {
	service := NewService(newMemoryStateRepo(), &fakeSessionPlatform{})
	if _, err := service.RequireSession(context.Background(), "user-1", "account-1"); err == nil || err.Error() != ErrorAuthRequired {
		t.Fatalf("expected %s, got %v", ErrorAuthRequired, err)
	}
}

func TestRequireSessionPersistsRefreshedTokensBeforeVerifying(t *testing.T) {
	repo := newMemoryStateRepo()
	service := NewService(repo, &fakeSessionPlatform{})
	if err := service.SyncAdminSession(context.Background(), "user-1", "account-1", adminSession("old"), "admin@example.test"); err != nil {
		t.Fatalf("sync: %v", err)
	}
	rotated := adminSession("new")
	platform := &fakeSessionPlatform{refreshed: &rotated}
	service.platform = platform

	session, err := service.RequireSession(context.Background(), "user-1", "account-1")
	if err != nil || session.AccessToken != "new" {
		t.Fatalf("expected the refreshed session, got %+v %v", session, err)
	}
	stored := repo.states["user-1|account-1"]
	if stored.Session.AccessToken != "new" || stored.Session.RefreshToken != "refresh-new" || stored.Email != "admin@example.test" {
		t.Fatalf("rotated tokens must be written back: %+v", stored)
	}
	if len(platform.verified) != 1 || platform.verified[0].AccessToken != "new" {
		t.Fatalf("admin role must be verified with the refreshed session: %+v", platform.verified)
	}

	// 令牌未变化时不重复写库。
	saves := repo.saves
	if _, err := service.RequireSession(context.Background(), "user-1", "account-1"); err != nil {
		t.Fatalf("second RequireSession: %v", err)
	}
	if repo.saves != saves {
		t.Fatalf("unchanged sessions must not be saved again (saves %d -> %d)", saves, repo.saves)
	}
}

func TestRequireSessionRejectsNonAdminOrFailedRefresh(t *testing.T) {
	repo := newMemoryStateRepo()
	repo.states["user-1|account-1"] = State{UserID: "user-1", AdminAccountID: "account-1", Session: adminSession("token")}

	service := NewService(repo, &fakeSessionPlatform{verifyErr: errors.New("not admin")})
	if _, err := service.RequireSession(context.Background(), "user-1", "account-1"); err == nil || err.Error() != ErrorAdminOnly {
		t.Fatalf("non-admin sessions must be rejected, got %v", err)
	}
	service = NewService(repo, &fakeSessionPlatform{refreshErr: errors.New("expired")})
	if _, err := service.RequireSession(context.Background(), "user-1", "account-1"); err == nil || err.Error() != ErrorAdminOnly {
		t.Fatalf("failed refreshes must be rejected, got %v", err)
	}
}

func TestStoredSessionDoesNotContactTheSite(t *testing.T) {
	repo := newMemoryStateRepo()
	platform := &fakeSessionPlatform{refreshErr: errors.New("must not be called")}
	service := NewService(repo, platform)

	if _, ok, err := service.StoredSession(context.Background(), "user-1", "account-1"); err != nil || ok {
		t.Fatalf("missing sessions must report ok=false: %v %v", ok, err)
	}
	repo.states["user-1|account-1"] = State{UserID: "user-1", AdminAccountID: "account-1", Session: upstream.Session{Platform: upstream.PlatformSub2API}}
	if _, ok, err := service.StoredSession(context.Background(), "user-1", "account-1"); err != nil || ok {
		t.Fatalf("sessions without credentials must report ok=false: %v %v", ok, err)
	}
	repo.states["user-1|account-1"] = State{UserID: "user-1", AdminAccountID: "account-1", Session: adminSession("token")}
	session, ok, err := service.StoredSession(context.Background(), "user-1", "account-1")
	if err != nil || !ok || session.AccessToken != "token" {
		t.Fatalf("unexpected stored session: %+v %v %v", session, ok, err)
	}
	if len(platform.verified) != 0 {
		t.Fatal("StoredSession must not verify against the site")
	}
}
