package leaderboard

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

type leaderboardRepository interface {
	EnsureSchema(ctx context.Context) error
	GetEmbedConfigByToken(ctx context.Context, embedToken string) (*EmbedConfig, error)
	GetEmbedConfigByWorkspace(ctx context.Context, userID string, adminAccountID string) (*EmbedConfig, error)
	InsertEmbedConfig(ctx context.Context, config EmbedConfig) error
	UpdateEmbedConfig(ctx context.Context, userID string, adminAccountID string, origin string) error
	RotateEmbedToken(ctx context.Context, userID string, adminAccountID string, newToken string) error
}

type embedSessionStore interface {
	Save(ctx context.Context, token string, session EmbedSession) error
	Get(ctx context.Context, token string) (*EmbedSession, error)
	DeleteWorkspace(ctx context.Context, userID string, adminAccountID string) error
}

type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

type AdminSessionProvider interface {
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}

type PlatformClient interface {
	FetchSub2APIAdminUserBreakdown(session upstream.Session, query upstream.Sub2APIUserBreakdownQuery) (upstream.Sub2APIUserBreakdown, error)
}

type sub2APIFetcher interface {
	FetchCurrentUser(srcHost string, token string) (Sub2APIUser, error)
}

type Service struct {
	repository    leaderboardRepository
	sessions      embedSessionStore
	accounts      AdminAccountResolver
	adminSessions AdminSessionProvider
	platform      PlatformClient
	sub2api       sub2APIFetcher
	newToken      func() (string, error)
	now           func() time.Time
}

func NewService(repository *Repository, sessions *EmbedSessionStore, sub2api *Sub2APIClient, platform PlatformClient, adminSessions AdminSessionProvider) *Service {
	return &Service{repository: repository, sessions: sessions, sub2api: sub2api, platform: platform, adminSessions: adminSessions, newToken: randomToken, now: time.Now}
}

func (s *Service) EnsureSchema(ctx context.Context) error { return s.repository.EnsureSchema(ctx) }

func (s *Service) SetAdminAccountResolver(accounts AdminAccountResolver) { s.accounts = accounts }

func (s *Service) GetData(ctx context.Context, userID string, query LeaderboardQuery) (LeaderboardResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return LeaderboardResponse{}, err
	}
	return s.fetchData(ctx, userID, adminAccountID, query, false)
}

func (s *Service) GetEmbedConfig(ctx context.Context, userID string) (EmbedConfigResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	origin, err := s.currentWorkspaceSourceOrigin(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	config, err := s.autoBindEmbedConfig(ctx, userID, adminAccountID, origin)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	return embedConfigResponse(*config), nil
}

func (s *Service) UpdateEmbedConfig(ctx context.Context, userID string, req UpdateEmbedConfigRequest) (EmbedConfigResponse, error) {
	// req is intentionally retained for legacy clients, but source origin is now
	// always derived from the current workspace's refreshed Sub2API admin session.
	_ = req
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	origin, err := s.currentWorkspaceSourceOrigin(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	config, err := s.autoBindEmbedConfig(ctx, userID, adminAccountID, origin)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	return embedConfigResponse(*config), nil
}

func (s *Service) RotateEmbedToken(ctx context.Context, userID string) (EmbedConfigResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	origin, err := s.currentWorkspaceSourceOrigin(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	config, err := s.autoBindEmbedConfig(ctx, userID, adminAccountID, origin)
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	newToken, err := s.newToken()
	if err != nil {
		return EmbedConfigResponse{}, err
	}
	if err := s.repository.RotateEmbedToken(ctx, userID, adminAccountID, newToken); err != nil {
		return EmbedConfigResponse{}, err
	}
	if err := s.sessions.DeleteWorkspace(ctx, userID, adminAccountID); err != nil {
		return EmbedConfigResponse{}, err
	}
	config.EmbedToken = newToken
	config.UpdatedAt = s.now()
	return embedConfigResponse(*config), nil
}

func (s *Service) CreateEmbedSession(ctx context.Context, req CreateSessionRequest) (CreateSessionResponse, error) {
	embedToken := strings.TrimSpace(req.EmbedToken)
	viewerToken := strings.TrimSpace(req.Sub2apiToken)
	if embedToken == "" || viewerToken == "" {
		return CreateSessionResponse{}, requestError(ErrorEmbedRequest)
	}
	config, err := s.repository.GetEmbedConfigByToken(ctx, embedToken)
	if err != nil {
		return CreateSessionResponse{}, err
	}
	if config == nil || strings.TrimSpace(config.Sub2apiSourceOrigin) == "" {
		return CreateSessionResponse{}, requestError(ErrorEmbedConfigNotFound)
	}
	normalizedSrcHost, err := normalizeSrcHost(req.SrcHost)
	if err != nil {
		return CreateSessionResponse{}, err
	}
	adminSession, err := s.adminSessions.RequireSession(ctx, config.UserID, config.AdminAccountID)
	if err != nil {
		return CreateSessionResponse{}, requestError(ErrorEmbedAdminSession)
	}
	adminOrigin, err := normalizeSrcHost(adminSession.BaseURL)
	if err != nil || adminSession.Platform != upstream.PlatformSub2API || adminOrigin != config.Sub2apiSourceOrigin || adminOrigin != normalizedSrcHost {
		return CreateSessionResponse{}, requestError(ErrorEmbedSourceBinding)
	}
	user, err := s.sub2api.FetchCurrentUser(normalizedSrcHost, viewerToken)
	if err != nil {
		var sub2apiErr *sub2APIError
		if errors.As(err, &sub2apiErr) && sub2apiErr.unauthorized {
			return CreateSessionResponse{}, requestError(ErrorEmbedSub2apiAuth)
		}
		return CreateSessionResponse{}, requestError(ErrorEmbedSub2apiRequest)
	}
	if urlUserID := strings.TrimSpace(req.UrlUserID); urlUserID != "" && urlUserID != user.ID {
		return CreateSessionResponse{}, requestError(ErrorEmbedUserMismatch)
	}
	sessionToken, err := s.newToken()
	if err != nil {
		return CreateSessionResponse{}, err
	}
	if err := s.sessions.Save(ctx, sessionToken, EmbedSession{UserID: config.UserID, AdminAccountID: config.AdminAccountID, EmbedToken: embedToken, SrcHost: normalizedSrcHost, Sub2apiUserID: user.ID}); err != nil {
		return CreateSessionResponse{}, err
	}
	return CreateSessionResponse{SessionToken: sessionToken, ExpiresIn: int64(embedSessionTTL.Seconds())}, nil
}

func (s *Service) GetEmbedData(ctx context.Context, sessionToken string, query LeaderboardQuery) (LeaderboardResponse, error) {
	session, err := s.requireEmbedSession(ctx, sessionToken)
	if err != nil {
		return LeaderboardResponse{}, err
	}
	config, err := s.repository.GetEmbedConfigByWorkspace(ctx, session.UserID, session.AdminAccountID)
	if err != nil {
		return LeaderboardResponse{}, err
	}
	if config == nil || strings.TrimSpace(config.Sub2apiSourceOrigin) == "" || session.EmbedToken != config.EmbedToken {
		return LeaderboardResponse{}, requestError(ErrorEmbedSessionInvalid)
	}
	adminSession, err := s.validateCurrentSourceBinding(ctx, session.UserID, session.AdminAccountID, config.Sub2apiSourceOrigin, session.SrcHost)
	if err != nil {
		return LeaderboardResponse{}, err
	}
	return s.fetchDataWithSession(adminSession, query, true)
}

func (s *Service) fetchData(ctx context.Context, userID string, adminAccountID string, query LeaderboardQuery, embed bool) (LeaderboardResponse, error) {
	session, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		if embed {
			return LeaderboardResponse{}, requestError(ErrorEmbedAdminSession)
		}
		return LeaderboardResponse{}, requestError(ErrorAdminOnly)
	}
	return s.fetchDataWithSession(session, query, embed)
}

func (s *Service) fetchDataWithSession(session upstream.Session, query LeaderboardQuery, embed bool) (LeaderboardResponse, error) {
	period, err := normalizeQuery(query, s.now())
	if err != nil {
		return LeaderboardResponse{}, err
	}
	breakdown, err := s.platform.FetchSub2APIAdminUserBreakdown(session, upstream.Sub2APIUserBreakdownQuery{StartDate: period.StartDate, EndDate: period.EndDate, SortBy: DefaultSortBy, Limit: DefaultLimit, Timezone: DefaultTimezone})
	if err != nil {
		var requestErr *upstream.RequestError
		if errors.As(err, &requestErr) && requestErr.StatusCode == 404 {
			if embed {
				return LeaderboardResponse{}, requestError(ErrorEmbedUpstreamUnsupported)
			}
			return LeaderboardResponse{}, requestError(ErrorUpstreamUnsupported)
		}
		if embed {
			return LeaderboardResponse{}, requestError(ErrorEmbedUpstreamRequest)
		}
		return LeaderboardResponse{}, err
	}
	rows := rankedRows(breakdown.Users)
	return LeaderboardResponse{StartDate: period.StartDate, EndDate: period.EndDate, Timezone: DefaultTimezone, SortBy: DefaultSortBy, Limit: DefaultLimit, Rows: rows}, nil
}

// FrameAncestorOrigin resolves a public embed token to the normalized origin
// stored in PostgreSQL. httpserver uses this narrow callback for the static
// /embed/leaderboard route; it never reads or trusts src_host from the request.
func (s *Service) FrameAncestorOrigin(ctx context.Context, embedToken string) (string, bool) {
	trimmed := strings.TrimSpace(embedToken)
	if trimmed == "" {
		return "", false
	}
	config, err := s.repository.GetEmbedConfigByToken(ctx, trimmed)
	if err != nil || config == nil || strings.TrimSpace(config.Sub2apiSourceOrigin) == "" {
		return "", false
	}
	if _, err := s.validateCurrentSourceBinding(ctx, config.UserID, config.AdminAccountID, config.Sub2apiSourceOrigin, config.Sub2apiSourceOrigin); err != nil {
		if errors.Is(err, requestError(ErrorEmbedAdminSession)) {
			return config.Sub2apiSourceOrigin, true
		}
		return "", false
	}
	return config.Sub2apiSourceOrigin, true
}

func (s *Service) validateCurrentSourceBinding(ctx context.Context, userID string, adminAccountID string, storedOrigin string, sessionSrcHost string) (upstream.Session, error) {
	adminSession, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return upstream.Session{}, requestError(ErrorEmbedAdminSession)
	}
	adminOrigin, err := normalizeSrcHost(adminSession.BaseURL)
	if err != nil || adminSession.Platform != upstream.PlatformSub2API || adminOrigin != storedOrigin || adminOrigin != sessionSrcHost {
		return upstream.Session{}, requestError(ErrorEmbedSourceBinding)
	}
	return adminSession, nil
}

type normalizedPeriod struct{ StartDate, EndDate string }

func normalizeQuery(query LeaderboardQuery, now time.Time) (normalizedPeriod, error) {
	loc, err := time.LoadLocation(DefaultTimezone)
	if err != nil {
		return normalizedPeriod{}, err
	}
	startDate := strings.TrimSpace(query.StartDate)
	endDate := strings.TrimSpace(query.EndDate)
	if startDate == "" && endDate == "" {
		localNow := now.In(loc)
		start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
		return normalizedPeriod{StartDate: start.Format(time.DateOnly), EndDate: start.AddDate(0, 0, 1).Format(time.DateOnly)}, nil
	}
	if startDate == "" || endDate == "" {
		return normalizedPeriod{}, requestError(ErrorRequest)
	}
	start, err := time.ParseInLocation(time.DateOnly, startDate, loc)
	if err != nil || start.Format(time.DateOnly) != startDate {
		return normalizedPeriod{}, requestError(ErrorRequest)
	}
	end, err := time.ParseInLocation(time.DateOnly, endDate, loc)
	if err != nil || end.Format(time.DateOnly) != endDate || !start.Before(end) || end.Sub(start) > maxDateRange {
		return normalizedPeriod{}, requestError(ErrorRequest)
	}
	return normalizedPeriod{StartDate: startDate, EndDate: endDate}, nil
}

func rankedRows(users []upstream.Sub2APIUserBreakdownItem) []LeaderboardRow {
	indexed := append([]upstream.Sub2APIUserBreakdownItem(nil), users...)
	sort.SliceStable(indexed, func(i, j int) bool { return indexed[i].TotalTokens > indexed[j].TotalTokens })
	if len(indexed) > DefaultLimit {
		indexed = indexed[:DefaultLimit]
	}
	rows := make([]LeaderboardRow, 0, len(indexed))
	for i, user := range indexed {
		rows = append(rows, LeaderboardRow{Rank: i + 1, UserID: user.UserID, Email: maskEmail(user.Email), Requests: user.Requests, TotalTokens: user.TotalTokens, ActualCost: user.ActualCost})
	}
	return rows
}

func maskEmail(email string) string {
	parts := strings.Split(strings.TrimSpace(email), "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	local := []rune(parts[0])
	if len(local) == 1 {
		return string(local[0]) + "***@" + parts[1]
	}
	return string(local[0]) + "***" + string(local[len(local)-1]) + "@" + parts[1]
}

func (s *Service) requireCurrentAdminAccountID(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", requestError(ErrorNoCurrentAccount)
	}
	return s.accounts.RequireCurrentID(ctx, userID)
}

func (s *Service) currentWorkspaceSourceOrigin(ctx context.Context, userID string, adminAccountID string) (string, error) {
	adminSession, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return "", requestError(ErrorAdminOnly)
	}
	origin, err := normalizeSrcHost(adminSession.BaseURL)
	if err != nil || adminSession.Platform != upstream.PlatformSub2API {
		return "", requestError(ErrorInvalidSourceOrigin)
	}
	return origin, nil
}

func (s *Service) autoBindEmbedConfig(ctx context.Context, userID string, adminAccountID string, origin string) (*EmbedConfig, error) {
	config, err := s.ensureEmbedConfig(ctx, userID, adminAccountID, origin)
	if err != nil {
		return nil, err
	}
	if config.Sub2apiSourceOrigin == origin {
		return config, nil
	}
	if err := s.repository.UpdateEmbedConfig(ctx, userID, adminAccountID, origin); err != nil {
		return nil, err
	}
	config.Sub2apiSourceOrigin = origin
	config.UpdatedAt = s.now()
	if err := s.sessions.DeleteWorkspace(ctx, userID, adminAccountID); err != nil {
		return nil, err
	}
	return config, nil
}

func (s *Service) ensureEmbedConfig(ctx context.Context, userID string, adminAccountID string, origin string) (*EmbedConfig, error) {
	config, err := s.repository.GetEmbedConfigByWorkspace(ctx, userID, adminAccountID)
	if err != nil || config != nil {
		return config, err
	}
	token, err := s.newToken()
	if err != nil {
		return nil, err
	}
	config = &EmbedConfig{UserID: userID, AdminAccountID: adminAccountID, EmbedToken: token, Sub2apiSourceOrigin: origin, CreatedAt: s.now(), UpdatedAt: s.now()}
	if err := s.repository.InsertEmbedConfig(ctx, *config); err != nil {
		return nil, err
	}
	return s.repository.GetEmbedConfigByWorkspace(ctx, userID, adminAccountID)
}

func (s *Service) requireEmbedSession(ctx context.Context, token string) (*EmbedSession, error) {
	session, err := s.sessions.Get(ctx, strings.TrimSpace(token))
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, requestError(ErrorEmbedSessionInvalid)
	}
	return session, nil
}

func embedConfigResponse(config EmbedConfig) EmbedConfigResponse {
	return EmbedConfigResponse{EmbedToken: config.EmbedToken, Sub2apiSourceOrigin: config.Sub2apiSourceOrigin, CreatedAt: formatTime(config.CreatedAt), UpdatedAt: formatTime(config.UpdatedAt)}
}
