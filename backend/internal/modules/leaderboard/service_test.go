package leaderboard

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

type fakeRepo struct {
	configs     map[string]EmbedConfig
	insertCalls int
	updateCalls int
}

func newFakeRepo() *fakeRepo                               { return &fakeRepo{configs: map[string]EmbedConfig{}} }
func (f *fakeRepo) EnsureSchema(ctx context.Context) error { return nil }
func (f *fakeRepo) GetEmbedConfigByToken(ctx context.Context, token string) (*EmbedConfig, error) {
	for _, config := range f.configs {
		if config.EmbedToken == token {
			c := config
			return &c, nil
		}
	}
	return nil, nil
}
func (f *fakeRepo) GetEmbedConfigByWorkspace(ctx context.Context, userID string, adminAccountID string) (*EmbedConfig, error) {
	if config, ok := f.configs[userID+"|"+adminAccountID]; ok {
		c := config
		return &c, nil
	}
	return nil, nil
}
func (f *fakeRepo) InsertEmbedConfig(ctx context.Context, config EmbedConfig) error {
	f.insertCalls++
	f.configs[config.UserID+"|"+config.AdminAccountID] = config
	return nil
}
func (f *fakeRepo) UpdateEmbedConfig(ctx context.Context, userID string, adminAccountID string, origin string) error {
	f.updateCalls++
	c := f.configs[userID+"|"+adminAccountID]
	c.Sub2apiSourceOrigin = origin
	f.configs[userID+"|"+adminAccountID] = c
	return nil
}
func (f *fakeRepo) RotateEmbedToken(ctx context.Context, userID string, adminAccountID string, token string) error {
	c := f.configs[userID+"|"+adminAccountID]
	c.EmbedToken = token
	f.configs[userID+"|"+adminAccountID] = c
	return nil
}

type fakeSessions struct {
	sessions    map[string]EmbedSession
	deleteErr   error
	deleteCalls int
}

func (f *fakeSessions) Save(ctx context.Context, token string, session EmbedSession) error {
	f.sessions[token] = session
	return nil
}
func (f *fakeSessions) Get(ctx context.Context, token string) (*EmbedSession, error) {
	if s, ok := f.sessions[token]; ok {
		return &s, nil
	}
	return nil, nil
}
func (f *fakeSessions) DeleteWorkspace(ctx context.Context, userID string, adminAccountID string) error {
	f.deleteCalls++
	if f.deleteErr != nil {
		return f.deleteErr
	}
	for token, session := range f.sessions {
		if session.UserID == userID && session.AdminAccountID == adminAccountID {
			delete(f.sessions, token)
		}
	}
	return nil
}

type fakeAccounts struct{ gotUserID string }

func (f *fakeAccounts) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	f.gotUserID = userID
	return "account-1", nil
}

type fakeAdminSessions struct {
	gotUserID, gotAccountID string
	err                     error
	baseURL                 string
	platform                upstream.Platform
}

func (f *fakeAdminSessions) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	f.gotUserID, f.gotAccountID = userID, adminAccountID
	if f.err != nil {
		return upstream.Session{}, f.err
	}
	baseURL := f.baseURL
	if baseURL == "" {
		baseURL = "https://src.example.com"
	}
	platform := f.platform
	if platform == "" {
		platform = upstream.PlatformSub2API
	}
	return upstream.Session{Platform: platform, BaseURL: baseURL, AccessToken: "admin"}, nil
}

type fakePlatform struct {
	query upstream.Sub2APIUserBreakdownQuery
	err   error
}

func (f *fakePlatform) FetchSub2APIAdminUserBreakdown(session upstream.Session, query upstream.Sub2APIUserBreakdownQuery) (upstream.Sub2APIUserBreakdown, error) {
	f.query = query
	if f.err != nil {
		return upstream.Sub2APIUserBreakdown{}, f.err
	}
	return upstream.Sub2APIUserBreakdown{Users: []upstream.Sub2APIUserBreakdownItem{{UserID: "u-low", Email: "low@example.com", TotalTokens: 10, Requests: 1, ActualCost: 0.1}, {UserID: "u-high-a", Email: "alice@example.com", TotalTokens: 30, Requests: 3, ActualCost: 0.3}, {UserID: "u-high-b", Email: "bob@example.com", TotalTokens: 30, Requests: 2, ActualCost: 0.2}}}, nil
}

type fakeSub2API struct {
	gotHost, gotToken string
	user              Sub2APIUser
	err               error
}

func (f *fakeSub2API) FetchCurrentUser(srcHost string, token string) (Sub2APIUser, error) {
	f.gotHost, f.gotToken = srcHost, token
	return f.user, f.err
}

func newTestService(repo *fakeRepo, sessions *fakeSessions, platform *fakePlatform, sub2api *fakeSub2API) *Service {
	svc := &Service{repository: repo, sessions: sessions, accounts: &fakeAccounts{}, adminSessions: &fakeAdminSessions{}, platform: platform, sub2api: sub2api, now: func() time.Time { return time.Date(2026, 7, 12, 8, 30, 0, 0, time.UTC) }}
	tokens := []string{"token-1", "token-2", "token-3"}
	svc.newToken = func() (string, error) { token := tokens[0]; tokens = tokens[1:]; return token, nil }
	return svc
}

func TestGetData_DefaultShanghaiDateSortMaskAndWorkspace(t *testing.T) {
	repo := newFakeRepo()
	sessions := &fakeSessions{sessions: map[string]EmbedSession{}}
	platform := &fakePlatform{}
	svc := newTestService(repo, sessions, platform, &fakeSub2API{})
	resp, err := svc.GetData(context.Background(), "user-1", LeaderboardQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if platform.query.StartDate != "2026-07-12" || platform.query.EndDate != "2026-07-13" || platform.query.SortBy != "total_tokens" || platform.query.Limit != 50 || platform.query.Timezone != "Asia/Shanghai" {
		t.Fatalf("unexpected query: %+v", platform.query)
	}
	if resp.Rows[0].UserID != "u-high-a" || resp.Rows[1].UserID != "u-high-b" {
		t.Fatalf("stable total_tokens sort not preserved: %+v", resp.Rows)
	}
	if resp.Rows[0].Email != "a***e@example.com" || resp.Rows[1].Email != "b***b@example.com" {
		t.Fatalf("emails not masked: %+v", resp.Rows)
	}
}

func TestNormalizeQueryRejectsInvalidDateRange(t *testing.T) {
	now := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	bad := []LeaderboardQuery{{StartDate: "2026-07-12", EndDate: "2026-07-12"}, {StartDate: "2026-7-12", EndDate: "2026-07-13"}, {StartDate: "2026-07-01", EndDate: "2026-08-15"}, {StartDate: "2026-07-12"}}
	for _, query := range bad {
		if _, err := normalizeQuery(query, now); err == nil {
			t.Fatalf("expected invalid query to fail: %+v", query)
		}
	}
}

func TestNormalizeSrcHostRejectsLocalhostAndReservedIP(t *testing.T) {
	for _, value := range []string{"localhost", "http://localhost", "127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.1.1", "0.0.0.0"} {
		if _, err := normalizeSrcHost(value); err == nil {
			t.Fatalf("expected %q to be rejected", value)
		}
	}
	if got, err := normalizeSrcHost("https://src.example.com/path?q=1"); err != nil || got != "https://src.example.com" {
		t.Fatalf("expected public origin normalization, got %q err=%v", got, err)
	}
}

func TestGetEmbedConfigAutoBindsInitialConfig(t *testing.T) {
	repo := newFakeRepo()
	sessions := &fakeSessions{sessions: map[string]EmbedSession{}}
	svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
	resp, err := svc.GetEmbedConfig(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EmbedToken != "token-1" || resp.Sub2apiSourceOrigin != "https://src.example.com" {
		t.Fatalf("unexpected auto-bound config: %+v", resp)
	}
	if repo.insertCalls != 1 || repo.updateCalls != 0 || sessions.deleteCalls != 0 {
		t.Fatalf("unexpected side effects: inserts=%d updates=%d deletes=%d", repo.insertCalls, repo.updateCalls, sessions.deleteCalls)
	}
}

func TestGetEmbedConfigSameOriginDoesNotUpdateOrRevoke(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	sessions := &fakeSessions{sessions: map[string]EmbedSession{"session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed"}}}
	svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
	resp, err := svc.GetEmbedConfig(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EmbedToken != "embed" || resp.Sub2apiSourceOrigin != "https://src.example.com" {
		t.Fatalf("unexpected config: %+v", resp)
	}
	if repo.updateCalls != 0 || sessions.deleteCalls != 0 {
		t.Fatalf("same origin should not update or revoke sessions: updates=%d deletes=%d", repo.updateCalls, sessions.deleteCalls)
	}
}

func TestGetEmbedConfigRepairsStaleAndEmptyOriginAndRevokesSessions(t *testing.T) {
	for _, existingOrigin := range []string{"", "https://old-src.example.com"} {
		repo := newFakeRepo()
		repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: existingOrigin}
		sessions := &fakeSessions{sessions: map[string]EmbedSession{"session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed"}}}
		svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
		resp, err := svc.GetEmbedConfig(context.Background(), "user-1")
		if err != nil {
			t.Fatalf("origin %q unexpected error: %v", existingOrigin, err)
		}
		if resp.EmbedToken != "embed" || resp.Sub2apiSourceOrigin != "https://src.example.com" {
			t.Fatalf("origin %q unexpected repaired config: %+v", existingOrigin, resp)
		}
		if repo.updateCalls != 1 || sessions.deleteCalls != 1 || len(sessions.sessions) != 0 {
			t.Fatalf("origin %q expected update+revoke, updates=%d deletes=%d sessions=%+v", existingOrigin, repo.updateCalls, sessions.deleteCalls, sessions.sessions)
		}
	}
}

func TestGetEmbedConfigReturnsCleanupErrorAfterDBOriginUpdate(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://old-src.example.com"}
	sessions := &fakeSessions{sessions: map[string]EmbedSession{"session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", SrcHost: "https://old-src.example.com"}}, deleteErr: errors.New("redis failed")}
	svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
	if _, err := svc.GetEmbedConfig(context.Background(), "user-1"); err == nil {
		t.Fatal("expected cleanup error")
	}
	config, _ := repo.GetEmbedConfigByWorkspace(context.Background(), "user-1", "account-1")
	if config.Sub2apiSourceOrigin != "https://src.example.com" {
		t.Fatalf("DB origin should be updated before cleanup error, got %+v", config)
	}
	if _, err := svc.GetEmbedData(context.Background(), "session", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedSourceBinding)) {
		t.Fatalf("old session should be unusable after DB origin update, got %v", err)
	}
}

func TestUpdateEmbedConfigIgnoresLegacyRequestOrigin(t *testing.T) {
	repo := newFakeRepo()
	svc := newTestService(repo, &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{})
	resp, err := svc.UpdateEmbedConfig(context.Background(), "user-1", UpdateEmbedConfigRequest{Sub2apiSourceOrigin: "https://evil.example.com"})
	if err != nil {
		t.Fatalf("legacy request body should be ignored: %v", err)
	}
	if resp.Sub2apiSourceOrigin != "https://src.example.com" {
		t.Fatalf("expected server-derived origin, got %+v", resp)
	}
}

func TestGetEmbedConfigRequiresSub2APIAdminSession(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{})
	svc.adminSessions = &fakeAdminSessions{err: errors.New("missing")}
	if _, err := svc.GetEmbedConfig(context.Background(), "user-1"); !errors.Is(err, requestError(ErrorAdminOnly)) {
		t.Fatalf("expected admin-only error, got %v", err)
	}
	svc.adminSessions = &fakeAdminSessions{platform: upstream.PlatformNewAPI, baseURL: "https://src.example.com"}
	if _, err := svc.GetEmbedConfig(context.Background(), "user-1"); !errors.Is(err, requestError(ErrorInvalidSourceOrigin)) {
		t.Fatalf("expected invalid source origin for non-sub2api, got %v", err)
	}
	svc.adminSessions = &fakeAdminSessions{baseURL: "http://127.0.0.1"}
	if _, err := svc.GetEmbedConfig(context.Background(), "user-1"); !errors.Is(err, requestError(ErrorInvalidSourceOrigin)) {
		t.Fatalf("expected invalid source origin for unsafe URL, got %v", err)
	}
}

func TestCreateEmbedSession_VerifiesOriginTokenAndUser(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	sessions := &fakeSessions{sessions: map[string]EmbedSession{}}
	sub2api := &fakeSub2API{user: Sub2APIUser{ID: "42", Email: "raw@example.com", Role: "user"}}
	svc := newTestService(repo, sessions, &fakePlatform{}, sub2api)
	resp, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{EmbedToken: "embed", Sub2apiToken: "viewer", SrcHost: "https://src.example.com/path", UrlUserID: "42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SessionToken != "token-1" || sub2api.gotToken != "viewer" || sub2api.gotHost != "https://src.example.com" {
		t.Fatalf("unexpected session verification: resp=%+v host=%s token=%s", resp, sub2api.gotHost, sub2api.gotToken)
	}
	stored := sessions.sessions["token-1"]
	if stored.UserID != "user-1" || stored.AdminAccountID != "account-1" || stored.EmbedToken != "embed" || stored.SrcHost != "https://src.example.com" || stored.Sub2apiUserID != "42" {
		t.Fatalf("authorization session fields not stored: %+v", stored)
	}
	payload, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal stored session: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		t.Fatalf("unmarshal stored session: %v", err)
	}
	for _, forbidden := range []string{"sub2apiEmail", "sub2apiRole", "sub2apiToken"} {
		if _, ok := raw[forbidden]; ok {
			t.Fatalf("stored session leaked %s: %s", forbidden, string(payload))
		}
	}
}

func TestCreateEmbedSessionRejectsOriginAndUserMismatch(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	svc := newTestService(repo, &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{user: Sub2APIUser{ID: "42"}})
	if _, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{EmbedToken: "embed", Sub2apiToken: "viewer", SrcHost: "https://evil.example.com"}); !errors.Is(err, requestError(ErrorEmbedSourceBinding)) {
		t.Fatalf("expected origin mismatch, got %v", err)
	}
	if _, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{EmbedToken: "embed", Sub2apiToken: "viewer", SrcHost: "https://src.example.com", UrlUserID: "99"}); !errors.Is(err, requestError(ErrorEmbedUserMismatch)) {
		t.Fatalf("expected user mismatch, got %v", err)
	}
}

func TestCreateEmbedSessionRequiresCurrentAdminSessionBinding(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	sub2api := &fakeSub2API{user: Sub2APIUser{ID: "42"}}
	svc := newTestService(repo, &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, sub2api)
	svc.adminSessions = &fakeAdminSessions{baseURL: "https://new-src.example.com"}
	if _, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{EmbedToken: "embed", Sub2apiToken: "viewer", SrcHost: "https://src.example.com", UrlUserID: "42"}); !errors.Is(err, requestError(ErrorEmbedSourceBinding)) {
		t.Fatalf("expected stale source binding error, got %v", err)
	}
	if sub2api.gotToken != "" {
		t.Fatalf("viewer token must not be verified against stale binding, got token %q", sub2api.gotToken)
	}

	svc.adminSessions = &fakeAdminSessions{err: errors.New("no admin session")}
	if _, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{EmbedToken: "embed", Sub2apiToken: "viewer", SrcHost: "https://src.example.com", UrlUserID: "42"}); !errors.Is(err, requestError(ErrorEmbedAdminSession)) {
		t.Fatalf("expected embed admin session error, got %v", err)
	}
}

func TestRotateEmbedTokenRevokesOldTokenLookup(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "old", Sub2apiSourceOrigin: "https://src.example.com"}
	sessions := &fakeSessions{sessions: map[string]EmbedSession{"old-session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "old", SrcHost: "https://src.example.com", Sub2apiUserID: "42"}}}
	svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
	resp, err := svc.RotateEmbedToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EmbedToken != "token-1" {
		t.Fatalf("unexpected token: %+v", resp)
	}
	if config, _ := repo.GetEmbedConfigByToken(context.Background(), "old"); config != nil {
		t.Fatalf("old token still resolves: %+v", config)
	}
	if _, err := svc.GetEmbedData(context.Background(), "old-session", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedSessionInvalid)) {
		t.Fatalf("expected old session to be revoked after rotation, got %v", err)
	}
}

func TestRotateEmbedTokenRepairsStaleOrigin(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "old", Sub2apiSourceOrigin: "https://old-src.example.com"}
	sessions := &fakeSessions{sessions: map[string]EmbedSession{"session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "old", SrcHost: "https://old-src.example.com", Sub2apiUserID: "42"}}}
	svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
	resp, err := svc.RotateEmbedToken(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EmbedToken != "token-1" || resp.Sub2apiSourceOrigin != "https://src.example.com" {
		t.Fatalf("expected rotated token with repaired origin, got %+v", resp)
	}
	if repo.updateCalls != 1 || sessions.deleteCalls != 2 || len(sessions.sessions) != 0 {
		t.Fatalf("expected origin repair and rotation revocation, updates=%d deletes=%d sessions=%+v", repo.updateCalls, sessions.deleteCalls, sessions.sessions)
	}
}

func TestGetEmbedDataRejectsSessionAfterDBTokenChanges(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "old", Sub2apiSourceOrigin: "https://src.example.com"}
	sessions := &fakeSessions{sessions: map[string]EmbedSession{"session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "old", SrcHost: "https://src.example.com", Sub2apiUserID: "42"}}}
	svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "new", Sub2apiSourceOrigin: "https://src.example.com"}
	if _, err := svc.GetEmbedData(context.Background(), "session", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedSessionInvalid)) {
		t.Fatalf("expected stale session to fail after DB token change, got %v", err)
	}
}

func TestRotateEmbedTokenUpdatesDBBeforeCleanupFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "old", Sub2apiSourceOrigin: "https://src.example.com"}
	sessions := &fakeSessions{sessions: map[string]EmbedSession{"session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "old"}}, deleteErr: errors.New("redis cleanup failed")}
	svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
	if _, err := svc.RotateEmbedToken(context.Background(), "user-1"); err == nil {
		t.Fatal("expected cleanup error to be returned")
	}
	config, err := repo.GetEmbedConfigByWorkspace(context.Background(), "user-1", "account-1")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if config.EmbedToken != "token-1" {
		t.Fatalf("expected DB token rotated before cleanup failure, got %+v", config)
	}
	if _, err := svc.GetEmbedData(context.Background(), "session", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedSessionInvalid)) {
		t.Fatalf("expected old session unusable even after cleanup failure, got %v", err)
	}
}

func TestGetEmbedDataRequiresSession(t *testing.T) {
	svc := newTestService(newFakeRepo(), &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{})
	if _, err := svc.GetEmbedData(context.Background(), "missing", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedSessionInvalid)) {
		t.Fatalf("expected invalid session, got %v", err)
	}
}

func TestGetEmbedDataMapsAdminAndUpstreamErrorsToEmbedKeys(t *testing.T) {
	sessions := &fakeSessions{sessions: map[string]EmbedSession{"session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", SrcHost: "https://src.example.com", Sub2apiUserID: "42"}}}
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	svc := newTestService(repo, sessions, &fakePlatform{}, &fakeSub2API{})
	svc.adminSessions = &fakeAdminSessions{err: errors.New("missing admin session")}
	if _, err := svc.GetEmbedData(context.Background(), "session", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedAdminSession)) {
		t.Fatalf("expected embed admin session error, got %v", err)
	}

	svc.adminSessions = &fakeAdminSessions{}
	svc.platform = &fakePlatform{err: &upstream.RequestError{MessageKey: upstream.ErrorRequest, StatusCode: http.StatusNotFound}}
	if _, err := svc.GetEmbedData(context.Background(), "session", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedUpstreamUnsupported)) {
		t.Fatalf("expected embed unsupported error, got %v", err)
	}

	svc.platform = &fakePlatform{err: &upstream.RequestError{MessageKey: upstream.ErrorNetwork, StatusCode: http.StatusBadGateway}}
	if _, err := svc.GetEmbedData(context.Background(), "session", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedUpstreamRequest)) {
		t.Fatalf("expected embed upstream request error, got %v", err)
	}
}

func TestGetEmbedDataRejectsSessionAfterWorkspaceBaseURLChanges(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	sessions := &fakeSessions{sessions: map[string]EmbedSession{"session": {UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", SrcHost: "https://src.example.com", Sub2apiUserID: "42"}}}
	platform := &fakePlatform{}
	svc := newTestService(repo, sessions, platform, &fakeSub2API{})
	svc.adminSessions = &fakeAdminSessions{baseURL: "https://new-src.example.com"}
	if _, err := svc.GetEmbedData(context.Background(), "session", LeaderboardQuery{}); !errors.Is(err, requestError(ErrorEmbedSourceBinding)) {
		t.Fatalf("expected source binding error after reconnect, got %v", err)
	}
	if platform.query.StartDate != "" {
		t.Fatalf("leaderboard data must not be fetched after stale source binding: %+v", platform.query)
	}
}

func TestFrameAncestorOriginResolvesOnlyConfiguredToken(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	svc := newTestService(repo, &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{})
	if origin, ok := svc.FrameAncestorOrigin(context.Background(), "embed"); !ok || origin != "https://src.example.com" {
		t.Fatalf("expected configured origin, got origin=%q ok=%v", origin, ok)
	}
	if origin, ok := svc.FrameAncestorOrigin(context.Background(), "missing"); ok || origin != "" {
		t.Fatalf("expected missing token to be denied, got origin=%q ok=%v", origin, ok)
	}
}

func TestFrameAncestorOriginFallsBackWhenAdminSessionUnavailable(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	svc := newTestService(repo, &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{})
	svc.adminSessions = &fakeAdminSessions{err: errors.New("missing admin session")}
	if origin, ok := svc.FrameAncestorOrigin(context.Background(), "embed"); !ok || origin != "https://src.example.com" {
		t.Fatalf("expected stored origin fallback, got origin=%q ok=%v", origin, ok)
	}
}

func TestFrameAncestorOriginRejectsCurrentMismatchedAdminSession(t *testing.T) {
	repo := newFakeRepo()
	repo.configs["user-1|account-1"] = EmbedConfig{UserID: "user-1", AdminAccountID: "account-1", EmbedToken: "embed", Sub2apiSourceOrigin: "https://src.example.com"}
	svc := newTestService(repo, &fakeSessions{sessions: map[string]EmbedSession{}}, &fakePlatform{}, &fakeSub2API{})
	svc.adminSessions = &fakeAdminSessions{baseURL: "https://new-src.example.com"}
	if origin, ok := svc.FrameAncestorOrigin(context.Background(), "embed"); ok || origin != "" {
		t.Fatalf("expected stale frame origin to be denied, got origin=%q ok=%v", origin, ok)
	}
	svc.adminSessions = &fakeAdminSessions{platform: upstream.PlatformNewAPI, baseURL: "https://src.example.com"}
	if origin, ok := svc.FrameAncestorOrigin(context.Background(), "embed"); ok || origin != "" {
		t.Fatalf("expected invalid platform frame origin to be denied, got origin=%q ok=%v", origin, ok)
	}
}

func TestWriteEmbedErrorUsesEmbedKeys(t *testing.T) {
	cases := []struct {
		err        error
		statusCode int
		message    string
	}{
		{requestError(ErrorEmbedAdminSession), http.StatusUnauthorized, ErrorEmbedAdminSession},
		{requestError(ErrorEmbedUpstreamUnsupported), http.StatusBadGateway, ErrorEmbedUpstreamUnsupported},
		{requestError(ErrorEmbedUpstreamRequest), http.StatusBadGateway, ErrorEmbedUpstreamRequest},
	}
	for _, tc := range cases {
		recorder := httptest.NewRecorder()
		writeEmbedError(recorder, tc.err)
		if recorder.Code != tc.statusCode {
			t.Fatalf("expected status %d for %v, got %d", tc.statusCode, tc.err, recorder.Code)
		}
		var body struct {
			Message string `json:"message"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode error body: %v", err)
		}
		if body.Message != tc.message {
			t.Fatalf("expected message %q, got %q", tc.message, body.Message)
		}
	}
}
