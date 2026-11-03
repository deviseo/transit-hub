package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newAPIRefreshThenFailServer 模拟新版 new-api：refresh 成功并轮换 refresh cookie，其余接口全部失败。
// 用于验证「刷新成功、后续业务请求失败」时轮换后的会话仍被写回。
func newAPIRefreshThenFailServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/auth/refresh" {
			http.SetCookie(w, &http.Cookie{Name: "new_api_refresh", Value: "sid.secret-2", Path: "/api/user/auth", MaxAge: 2592000})
			writeJSON(w, map[string]any{"success": true, "data": map[string]any{
				"access_token": "jwt-2", "access_expires_at": time.Now().Add(15 * time.Minute).Unix(),
				"user": map[string]any{"id": 9},
			}})
			return
		}
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"success": false})
	}))
}

func nearExpiryNewAPISession(baseURL string) *Session {
	expiresAt := time.Now().Add(time.Minute).UnixMilli()
	return &Session{
		Platform: PlatformNewAPI, BaseURL: baseURL, AccessToken: "jwt-1", TokenType: "Bearer",
		RefreshToken: "sid.secret-1", UserID: "9", QuotaPerUnit: 500000, ExpiresAt: &expiresAt,
	}
}

func assertRotatedSessionPersisted(t *testing.T, cache *fakeSiteCache, siteID string) {
	t.Helper()
	site, _ := cache.Get(context.Background(), siteID)
	if site == nil || site.Session == nil || site.Session.RefreshToken != "sid.secret-2" || site.Session.AccessToken != "jwt-2" {
		t.Fatalf("rotated session must be persisted even when the follow-up request fails, got %+v", site)
	}
}

// TestSyncPersistsRotatedSessionWhenMetricsFail 验证同步时刷新成功但拉取指标失败，站点进入错误状态，
// 但已轮换的 refresh token 仍被保存（否则下次同步会重放旧 token，new-api 会吊销整个会话）。
func TestSyncPersistsRotatedSessionWhenMetricsFail(t *testing.T) {
	server := newAPIRefreshThenFailServer(t)
	defer server.Close()
	cache := newFakeSiteCache()
	cache.add(newTestSite("site-a", "user-1", "acc-1", 1, nearExpiryNewAPISession(server.URL)))
	svc := NewService(NewPlatformService(NewHTTPClient(server.Client())), nil, nil, cache)

	if _, err := svc.sync(context.Background(), "site-a"); err != nil {
		t.Fatalf("sync returned error: %v", err)
	}
	site, _ := cache.Get(context.Background(), "site-a")
	if site.Status != StatusError || site.ErrorKey == nil {
		t.Fatalf("expected error status after metrics failure, got status=%s errorKey=%v", site.Status, site.ErrorKey)
	}
	assertRotatedSessionPersisted(t, cache, "site-a")
}

// TestKeyUsageTodayPersistsRotatedSessionWhenFetchFails 验证「今日成本」下钻在拉取 key 用量失败时，
// 同样保存已轮换的会话。
func TestKeyUsageTodayPersistsRotatedSessionWhenFetchFails(t *testing.T) {
	server := newAPIRefreshThenFailServer(t)
	defer server.Close()
	cache := newFakeSiteCache()
	cache.add(newTestSite("site-a", "user-1", "acc-1", 2, nearExpiryNewAPISession(server.URL)))
	svc := NewService(NewPlatformService(NewHTTPClient(server.Client())), nil, nil, cache)
	svc.SetAdminAccountResolver(&fakeAccountResolver{current: map[string]string{"user-1": "acc-1"}})

	if _, err := svc.KeyUsageToday(context.Background(), "user-1"); err == nil {
		t.Fatal("expected the key usage fetch to fail")
	}
	assertRotatedSessionPersisted(t, cache, "site-a")
}

// TestFetchGroupDailyStatsPersistsRotatedSessionWhenStatsFail 验证分组用量统计失败时同样保存已轮换的会话。
func TestFetchGroupDailyStatsPersistsRotatedSessionWhenStatsFail(t *testing.T) {
	server := newAPIRefreshThenFailServer(t)
	defer server.Close()
	cache := newFakeSiteCache()
	site := newTestSite("site-a", "user-1", "acc-1", 1, nearExpiryNewAPISession(server.URL))
	site.Metrics.Groups = []GroupInfo{{Name: "default"}}
	cache.add(site)
	svc := NewService(NewPlatformService(NewHTTPClient(server.Client())), nil, nil, cache)
	svc.SetAdminAccountResolver(&fakeAccountResolver{current: map[string]string{"user-1": "acc-1"}})

	if _, err := svc.FetchGroupDailyStats(context.Background(), "user-1", "site-a"); err == nil {
		t.Fatal("expected the group stats fetch to fail")
	}
	assertRotatedSessionPersisted(t, cache, "site-a")
}

// TestValidateAllowsUserKeyWithoutUserID 验证访问令牌模式下用户 ID 改为可选。
func TestValidateAllowsUserKeyWithoutUserID(t *testing.T) {
	create := CreateRequest{Name: "upstream", SiteURL: "https://new-api.example.com", Platform: PlatformNewAPI, AuthMode: AuthModeUserKey, AccessToken: "nap_token", RechargeRate: 1}
	if err := validateCreate(create); err != nil {
		t.Fatalf("validateCreate rejected user_key without user ID: %v", err)
	}
	update := UpdateRequest{Name: "upstream", SiteURL: "https://new-api.example.com", Platform: PlatformNewAPI, AuthMode: AuthModeUserKey, AccessToken: "nap_token", RechargeRate: 1}
	if err := validateUpdate(update); err != nil {
		t.Fatalf("validateUpdate rejected user_key without user ID: %v", err)
	}
	create.AccessToken = ""
	if err := validateCreate(create); err == nil {
		t.Fatal("validateCreate must still require the access token")
	}
}

// TestCreateUserKeySiteBackfillsDiscoveredUserID 验证未填写用户 ID 创建站点时，用令牌识别出的用户 ID
// 作为站点账号展示。
func TestCreateUserKeySiteBackfillsDiscoveredUserID(t *testing.T) {
	panel := &newAPIPanelStub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panel.serve(t, w, r, 21, 1)
	}))
	defer server.Close()
	cache := newFakeSiteCache()
	svc := NewService(NewPlatformService(NewHTTPClient(server.Client())), nil, nil, cache)
	svc.SetAdminAccountResolver(&fakeAccountResolver{current: map[string]string{"user-1": "acc-1"}})

	response, err := svc.Create(context.Background(), "user-1", CreateRequest{
		Name: "upstream", SiteURL: server.URL, Platform: PlatformNewAPI,
		AuthMode: AuthModeUserKey, AccessToken: "nap_token", RechargeRate: 1,
	})
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if response.Status != StatusConnected || response.Account != "21" {
		t.Fatalf("expected connected site with discovered account 21, got status=%s account=%q", response.Status, response.Account)
	}
}
