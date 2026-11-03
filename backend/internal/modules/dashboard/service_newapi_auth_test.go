package dashboard

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

// TestRefreshAdminSession_PersistsRotatedSessionBeforeVerifyFailure 验证 refresh token 已轮换但 admin 校验失败时，
// 新会话仍先落盘（Redis + my_site_states）。否则下次刷新会重放已轮换的旧 token，new-api 会吊销整个会话。
func TestRefreshAdminSession_PersistsRotatedSessionBeforeVerifyFailure(t *testing.T) {
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Platform: PlatformNewAPI, Identity: "admin", Session: authenticatedSession()})
	rotated := upstream.Session{Platform: upstream.PlatformNewAPI, BaseURL: "https://example.com", AccessToken: "jwt-2", RefreshToken: "sid.secret-2"}
	platform := &fakePlatformClient{refreshSessionResult: &rotated, verifyAdminErr: errors.New("role changed")}
	mySync := &fakeMySiteSync{}
	service := newRefreshTestService(store, platform, mySync)

	_, err := service.RefreshAdminSession(context.Background(), "user-1")
	assertAdminOnlyError(t, err)

	saved, _ := store.Get(context.Background(), "user-1", "account-1")
	if saved == nil || saved.Session.RefreshToken != "sid.secret-2" {
		t.Fatalf("expected the rotated session to be saved before verification, got %+v", saved)
	}
	if !mySync.called || mySync.session.RefreshToken != "sid.secret-2" {
		t.Fatalf("expected my_site_states to receive the rotated session, got called=%t session=%+v", mySync.called, mySync.session)
	}
}

// TestRefreshAdminSession_DoesNotDowngradeAuthoritativeSession 验证 Redis 里的会话比 my_site_states 旧时，
// 手动刷新不会把旧副本同步回去覆盖已轮换的新 refresh token。
func TestRefreshAdminSession_DoesNotDowngradeAuthoritativeSession(t *testing.T) {
	olderExpiry := int64(1_000)
	newerExpiry := int64(2_000)
	stale := upstream.Session{Platform: upstream.PlatformNewAPI, BaseURL: "https://example.com", AccessToken: "jwt-1", RefreshToken: "sid.secret-1", ExpiresAt: &olderExpiry}
	store := newFakeSessionStore()
	store.set("user-1", "account-1", AdminSession{Platform: PlatformNewAPI, Identity: "admin", Session: stale})
	mySync := &fakeMySiteSync{session: upstream.Session{
		Platform: upstream.PlatformNewAPI, BaseURL: "https://example.com", AccessToken: "jwt-2", RefreshToken: "sid.secret-2", ExpiresAt: &newerExpiry,
	}}
	service := newRefreshTestService(store, &fakePlatformClient{}, mySync)

	status, err := service.RefreshAdminSession(context.Background(), "user-1")
	if err != nil || !status.Authenticated {
		t.Fatalf("expected authenticated status, got %+v err=%v", status, err)
	}
	if mySync.called {
		t.Fatalf("stale Redis session must not overwrite the newer authoritative session, synced %+v", mySync.session)
	}
}

// TestLoginWithNewAPIAccessTokenWithoutUserID 验证 new-api 访问令牌登录可以不填用户 ID：
// 透传空用户 ID，身份展示使用自动识别出的用户 ID。
func TestLoginWithNewAPIAccessTokenWithoutUserID(t *testing.T) {
	store := newFakeSessionStore()
	accounts := &fakeAdminAccounts{current: map[string]string{"transit-user": "account-1"}}
	discovered := upstream.Session{Platform: upstream.PlatformNewAPI, BaseURL: "https://new-api.example.com", AccessToken: "nap_root", TokenType: "Bearer", UserID: "1"}
	platform := &fakePlatformClient{adminKeyResult: &discovered}
	service := NewService(store, platform)
	service.SetAdminAccountService(accounts)

	status, err := service.Login(context.Background(), "transit-user", LoginRequest{
		Platform: PlatformNewAPI, SiteURL: "https://new-api.example.com", AuthMethod: AuthMethodAdminKey, AdminKey: "nap_root",
	})
	if err != nil {
		t.Fatalf("Login returned error: %v", err)
	}
	if platform.capturedUserID != "" || platform.capturedAdminKey != "nap_root" {
		t.Fatalf("expected empty user ID to be passed through, got userID=%q key=%q", platform.capturedUserID, platform.capturedAdminKey)
	}
	if !status.Authenticated || status.Identity != "User ID 1" {
		t.Fatalf("expected identity from the discovered user ID, got %+v", status)
	}
}

// TestMapPlatformErrorNewAPIAuthErrors 验证新版 new-api 鉴权的两类错误映射为可读提示，而不是笼统的「非 admin」。
func TestMapPlatformErrorNewAPIAuthErrors(t *testing.T) {
	cases := map[string]string{
		upstream.ErrorVerificationRequired: ErrorVerificationRequired,
		upstream.ErrorNewAPITokenRejected:  ErrorTokenRejected,
		upstream.ErrorAuth:                 ErrorAdminOnly,
	}
	for upstreamKey, want := range cases {
		if got := mapPlatformError(errors.New(upstreamKey)); string(got) != want {
			t.Fatalf("mapPlatformError(%s) = %s, want %s", upstreamKey, got, want)
		}
	}
}
