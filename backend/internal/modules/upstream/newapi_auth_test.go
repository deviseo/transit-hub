package upstream

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newAPIPanelStub 应答 new-api 登录后拉取指标用到的面板接口，并记录每次请求携带的鉴权信息。
type newAPIPanelStub struct {
	mu       sync.Mutex
	requests []http.Header
}

func (p *newAPIPanelStub) serve(t *testing.T, w http.ResponseWriter, r *http.Request, userID int, role int) {
	t.Helper()
	p.mu.Lock()
	p.requests = append(p.requests, r.Header.Clone())
	p.mu.Unlock()
	switch r.URL.Path {
	case "/api/status":
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"quota_per_unit": 500000}})
	case "/api/user/self":
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"id": userID, "role": role, "group": "default", "quota": 1000000, "used_quota": 500000}})
	case "/api/user/self/groups", "/api/user/groups":
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"default": map[string]any{"ratio": 1, "desc": "default"}}})
	case "/api/pricing":
		writeJSON(w, map[string]any{"success": true, "data": []any{}})
	case "/api/log/self/stat":
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"quota": 0}})
	default:
		t.Errorf("unexpected panel request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}
}

func (p *newAPIPanelStub) headers() []http.Header {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]http.Header(nil), p.requests...)
}

func writeJSONStatus(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// TestLoginNewAPIStoresBearerTokenAndRefreshCookie 验证新版 new-api 密码登录：保存 15 分钟 JWT 与
// refresh cookie，后续面板请求只带 Authorization: Bearer，refresh cookie 不会被发到面板接口。
func TestLoginNewAPIStoresBearerTokenAndRefreshCookie(t *testing.T) {
	expiresAt := time.Now().Add(15 * time.Minute).Unix()
	panel := &newAPIPanelStub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/login" {
			http.SetCookie(w, &http.Cookie{Name: "new_api_refresh", Value: "sid-1.secret-1", Path: "/api/user/auth", HttpOnly: true, MaxAge: 2592000})
			http.SetCookie(w, &http.Cookie{Name: "new_api_has_session", Value: "1", Path: "/", MaxAge: 2592000})
			writeJSON(w, map[string]any{"success": true, "message": "", "data": map[string]any{
				"access_token": "jwt-1", "token_type": "Bearer", "access_expires_at": expiresAt,
				"session": map[string]any{"sid": "sid-1", "login_method": "password"},
				"user":    map[string]any{"id": 9, "username": "admin", "role": 100, "group": "default"},
			}})
			return
		}
		panel.serve(t, w, r, 9, 100)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	result, err := service.Login(server.URL, PlatformNewAPI, "admin", "password")
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	session := result.Session
	if session.AccessToken != "jwt-1" || session.TokenType != "Bearer" || session.RefreshToken != "sid-1.secret-1" {
		t.Fatalf("unexpected bearer session: %+v", session)
	}
	if session.Cookie != "" || session.UserID != "9" || session.QuotaPerUnit != 500000 || !session.IsAuthenticated() {
		t.Fatalf("unexpected session fields: %+v", session)
	}
	if session.ExpiresAt == nil || *session.ExpiresAt != expiresAt*1000 {
		t.Fatalf("ExpiresAt = %v, want %d (access_expires_at in ms)", session.ExpiresAt, expiresAt*1000)
	}
	headers := panel.headers()
	if len(headers) == 0 {
		t.Fatal("expected panel requests after login")
	}
	for _, header := range headers {
		if header.Get("Cookie") != "" {
			t.Fatalf("refresh cookie must not be sent to panel endpoints, got Cookie=%q", header.Get("Cookie"))
		}
	}
	for _, header := range headers[1:] { // 第一个请求是公开的 /api/status
		if header.Get("Authorization") != "Bearer jwt-1" {
			t.Fatalf("panel request must use the bearer access token, got %q", header.Get("Authorization"))
		}
	}
}

// TestLoginNewAPILegacyCookieSessionStillWorks 验证旧版 new-api（Gin Session Cookie + New-Api-User）不受影响。
func TestLoginNewAPILegacyCookieSessionStillWorks(t *testing.T) {
	panel := &newAPIPanelStub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/user/login" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "legacy-cookie", Path: "/"})
			writeJSON(w, map[string]any{"success": true, "data": map[string]any{"id": 3, "username": "user", "group": "vip"}})
			return
		}
		panel.serve(t, w, r, 3, 1)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	result, err := service.Login(server.URL, PlatformNewAPI, "user", "password")
	if err != nil {
		t.Fatalf("login returned error: %v", err)
	}
	session := result.Session
	if session.Cookie != "session=legacy-cookie" || session.UserID != "3" || session.AccessToken != "" || session.RefreshToken != "" {
		t.Fatalf("unexpected legacy session: %+v", session)
	}
	for _, header := range panel.headers() {
		if header.Get("New-Api-User") != "3" || header.Get("Cookie") != "session=legacy-cookie" {
			t.Fatalf("legacy panel requests need Cookie + New-Api-User, got Cookie=%q New-Api-User=%q", header.Get("Cookie"), header.Get("New-Api-User"))
		}
	}
}

// TestLoginNewAPIRequiresVerificationReturnsClearError 验证开启二次验证的账号返回明确错误；
// 自动识别平台时也不会再尝试 sub2api 登录把该提示掩盖掉。
func TestLoginNewAPIRequiresVerificationReturnsClearError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/login" {
			t.Errorf("unexpected request after verification challenge: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{
			"require_verification": true, "flow_token": "flow-1", "expires_at": time.Now().Add(5 * time.Minute).Unix(),
			"methods": []map[string]any{{"method": "2fa", "available": true}},
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	for _, platform := range []Platform{PlatformNewAPI, PlatformAuto} {
		_, err := service.Login(server.URL, platform, "admin", "password")
		if errorKey(err) != ErrorVerificationRequired {
			t.Fatalf("platform=%s: expected %s, got %v", platform, ErrorVerificationRequired, err)
		}
	}
}

// TestRefreshSessionRotatesNewAPIRefreshCookie 验证临期的新版会话通过 refresh cookie 换新：
// 请求携带 refresh cookie 与站点自身 Origin、不带 Bearer，返回的会话保存轮换后的 cookie。
func TestRefreshSessionRotatesNewAPIRefreshCookie(t *testing.T) {
	expiresAt := time.Now().Add(15 * time.Minute).Unix()
	var mu sync.Mutex
	var gotCookie, gotOrigin, gotAuth, gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/auth/refresh" {
			t.Errorf("unexpected request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		mu.Lock()
		gotCookie, gotOrigin, gotAuth, gotMethod = r.Header.Get("Cookie"), r.Header.Get("Origin"), r.Header.Get("Authorization"), r.Method
		mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "new_api_refresh", Value: "sid-1.secret-2", Path: "/api/user/auth", HttpOnly: true, MaxAge: 2592000})
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{
			"access_token": "jwt-2", "token_type": "Bearer", "access_expires_at": expiresAt,
			"user": map[string]any{"id": 9},
		}})
	}))
	defer server.Close()

	nearExpiry := time.Now().Add(time.Minute).UnixMilli()
	session := Session{
		Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "jwt-1", TokenType: "Bearer",
		RefreshToken: "sid-1.secret-1", UserID: "9", QuotaPerUnit: 500000, ExpiresAt: &nearExpiry,
	}
	service := NewPlatformService(NewHTTPClient(server.Client()))
	refreshed, err := service.RefreshSession(session)
	if err != nil {
		t.Fatalf("refresh returned error: %v", err)
	}
	if refreshed.AccessToken != "jwt-2" || refreshed.RefreshToken != "sid-1.secret-2" || refreshed.UserID != "9" || refreshed.QuotaPerUnit != 500000 {
		t.Fatalf("unexpected refreshed session: %+v", refreshed)
	}
	if refreshed.ExpiresAt == nil || *refreshed.ExpiresAt != expiresAt*1000 {
		t.Fatalf("ExpiresAt = %v, want %d", refreshed.ExpiresAt, expiresAt*1000)
	}
	mu.Lock()
	defer mu.Unlock()
	if gotMethod != http.MethodPost || gotCookie != "new_api_refresh=sid-1.secret-1" || gotAuth != "" {
		t.Fatalf("unexpected refresh request: method=%s cookie=%q auth=%q", gotMethod, gotCookie, gotAuth)
	}
	if gotOrigin != server.URL {
		t.Fatalf("Origin = %q, want the site origin %q", gotOrigin, server.URL)
	}
}

// TestRefreshSessionExchangesEachRefreshTokenOnce 验证同一个 refresh token 只向上游兑换一次：
// 并发刷新合并成一次请求；之后持有旧副本的调用方直接拿到已轮换的最新会话，旧 token 不会再被发出。
func TestRefreshSessionExchangesEachRefreshTokenOnce(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&calls, 1)
		time.Sleep(50 * time.Millisecond) // 让并发请求确实重叠
		http.SetCookie(w, &http.Cookie{Name: "new_api_refresh", Value: "sid.secret-" + strconv.Itoa(int(n)+1), Path: "/api/user/auth", MaxAge: 2592000})
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{
			"access_token": "jwt-" + strconv.Itoa(int(n)+1), "access_expires_at": time.Now().Add(15 * time.Minute).Unix(),
			"user": map[string]any{"id": 9},
		}})
	}))
	defer server.Close()

	nearExpiry := time.Now().Add(30 * time.Second).UnixMilli()
	stale := Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "jwt-1", TokenType: "Bearer", RefreshToken: "sid.secret-1", UserID: "9", ExpiresAt: &nearExpiry}
	service := NewPlatformService(NewHTTPClient(server.Client()))

	const workers = 8
	results := make([]Session, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i], errs[i] = service.RefreshSession(stale)
		}(i)
	}
	wg.Wait()
	for i := 0; i < workers; i++ {
		if errs[i] != nil || results[i].AccessToken != "jwt-2" || results[i].RefreshToken != "sid.secret-2" {
			t.Fatalf("worker %d got session=%+v err=%v", i, results[i], errs[i])
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("concurrent refreshes must share one upstream exchange, got %d calls", got)
	}

	// 稍后某个流程仍拿着刷新前的旧副本：直接拿到最新会话，不能把旧 refresh token 再发给上游。
	latest, err := service.RefreshSession(stale)
	if err != nil || latest.AccessToken != "jwt-2" || latest.RefreshToken != "sid.secret-2" {
		t.Fatalf("stale copy should resolve to the rotated session, got %+v err=%v", latest, err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("stale refresh token must not be replayed upstream, got %d calls", got)
	}
}

// TestRefreshSessionLeavesSessionsWithoutRefreshUntouched 验证未临期会话、旧版 Cookie 会话和访问令牌
// （PAT / 系统访问令牌）都不会触发刷新请求。
func TestRefreshSessionLeavesSessionsWithoutRefreshUntouched(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected refresh request: %s", r.URL.Path)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	fresh := time.Now().Add(10 * time.Minute).UnixMilli()
	service := NewPlatformService(NewHTTPClient(server.Client()))
	for name, session := range map[string]Session{
		"fresh bearer":  {Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "jwt", RefreshToken: "sid.secret", ExpiresAt: &fresh},
		"legacy cookie": {Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "session=a", UserID: "1"},
		"access token":  {Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "nap_token", TokenType: "Bearer"},
	} {
		refreshed, err := service.RefreshSession(session)
		if err != nil || !refreshed.sameCredentials(session) {
			t.Fatalf("%s: expected session unchanged, got %+v err=%v", name, refreshed, err)
		}
	}
}

// TestRefreshSessionFailureIsNotCached 验证刷新失败不会被缓存，下一次调用会重新向上游请求。
func TestRefreshSessionFailureIsNotCached(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{"success": false, "code": "AUTH_INTERNAL_ERROR"})
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "new_api_refresh", Value: "sid.secret-2", MaxAge: 2592000})
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"access_token": "jwt-2", "access_expires_at": time.Now().Add(15 * time.Minute).Unix()}})
	}))
	defer server.Close()

	nearExpiry := time.Now().Add(time.Minute).UnixMilli()
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, AccessToken: "jwt-1", RefreshToken: "sid.secret-1", UserID: "9", ExpiresAt: &nearExpiry}
	service := NewPlatformService(NewHTTPClient(server.Client()))
	if _, err := service.RefreshSession(session); err == nil {
		t.Fatal("expected the first refresh to fail")
	}
	refreshed, err := service.RefreshSession(session)
	if err != nil || refreshed.AccessToken != "jwt-2" || refreshed.UserID != "9" {
		t.Fatalf("expected the retry to reach upstream and succeed, got %+v err=%v", refreshed, err)
	}
}

// TestSessionRefresherPrunesOldRotations 验证轮换记录超过保留期后被清理，不会无限增长。
func TestSessionRefresherPrunesOldRotations(t *testing.T) {
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	refresher := newSessionRefresher()
	refresher.now = func() time.Time { return now }
	exchanges := 0
	exchange := func(session Session) (Session, error) {
		exchanges++
		next := session
		next.RefreshToken = session.RefreshToken + "-next"
		return next, nil
	}
	always := func(Session) bool { return true }
	original := Session{Platform: PlatformNewAPI, BaseURL: "https://example.com", RefreshToken: "token"}

	if _, err := refresher.refresh(original, always, exchange); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now = now.Add(rotatedSessionTTL + time.Second)
	if _, err := refresher.refresh(original, always, exchange); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exchanges != 2 {
		t.Fatalf("expected the expired rotation record to be pruned, exchanges=%d", exchanges)
	}
	if len(refresher.rotated) != 1 {
		t.Fatalf("expected only the latest rotation to be retained, got %d records", len(refresher.rotated))
	}
}

// TestLoginWithUserKeyDiscoversUserIDForPAT 验证新版 new-api 的 PAT 可以不填用户 ID：
// 通过 /api/user/self 自动识别，识别请求本身不带 New-Api-User。
func TestLoginWithUserKeyDiscoversUserIDForPAT(t *testing.T) {
	panel := &newAPIPanelStub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panel.serve(t, w, r, 21, 1)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	result, err := service.LoginWithUserKey(server.URL, "", "nap_token")
	if err != nil {
		t.Fatalf("LoginWithUserKey returned error: %v", err)
	}
	if result.Session.UserID != "21" || result.Session.AccessToken != "nap_token" || !result.Session.IsAuthenticated() {
		t.Fatalf("unexpected session: %+v", result.Session)
	}
	first := panel.headers()[0]
	if first.Get("Authorization") != "Bearer nap_token" || first.Get("New-Api-User") != "" {
		t.Fatalf("discovery must use only the bearer token, got Authorization=%q New-Api-User=%q", first.Get("Authorization"), first.Get("New-Api-User"))
	}
}

// TestLoginWithUserKeyLegacyTokenNeedsUserID 验证旧版 new-api 系统访问令牌缺少用户 ID 时返回明确提示；
// 补填用户 ID 后正常登录。
func TestLoginWithUserKeyLegacyTokenNeedsUserID(t *testing.T) {
	panel := &newAPIPanelStub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status" && r.Header.Get("New-Api-User") == "" {
			writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"success": false, "message": "无权进行此操作，未提供 New-Api-User"})
			return
		}
		panel.serve(t, w, r, 5, 1)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	if _, err := service.LoginWithUserKey(server.URL, "", "legacy-token"); errorKey(err) != ErrorNewAPITokenRejected {
		t.Fatalf("expected %s without user ID, got %v", ErrorNewAPITokenRejected, err)
	}
	result, err := service.LoginWithUserKey(server.URL, "5", "legacy-token")
	if err != nil || result.Session.UserID != "5" {
		t.Fatalf("expected login with explicit user ID to succeed, got %+v err=%v", result.Session, err)
	}
}

// TestLoginAdminWithKeyNewAPIDiscoversUserID 验证 admin 访问令牌登录同样可以不填用户 ID，并继续校验 admin 角色。
func TestLoginAdminWithKeyNewAPIDiscoversUserID(t *testing.T) {
	panel := &newAPIPanelStub{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panel.serve(t, w, r, 1, 100)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session, err := service.LoginAdminWithKey(server.URL, PlatformNewAPI, "nap_root", "")
	if err != nil {
		t.Fatalf("LoginAdminWithKey returned error: %v", err)
	}
	if session.UserID != "1" || session.AccessToken != "nap_root" || session.QuotaPerUnit != 500000 {
		t.Fatalf("unexpected admin session: %+v", session)
	}
}

// TestNewAPISessionIsAuthenticated 锁定 new-api 会话的有效性判断：Bearer 令牌本身即可，旧版 Cookie 需要用户 ID。
func TestNewAPISessionIsAuthenticated(t *testing.T) {
	cases := map[string]struct {
		session Session
		want    bool
	}{
		"bearer without user id": {Session{Platform: PlatformNewAPI, AccessToken: "nap_token"}, true},
		"cookie with user id":    {Session{Platform: PlatformNewAPI, Cookie: "session=a", UserID: "1"}, true},
		"cookie without user id": {Session{Platform: PlatformNewAPI, Cookie: "session=a"}, false},
		"empty":                  {Session{Platform: PlatformNewAPI, UserID: "1"}, false},
	}
	for name, tc := range cases {
		if got := tc.session.IsAuthenticated(); got != tc.want {
			t.Fatalf("%s: IsAuthenticated = %v, want %v", name, got, tc.want)
		}
	}
}
