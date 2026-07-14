package leaderboard

import "time"

const (
	ErrorRequest             = "admin.leaderboard.errors.request"
	ErrorUnknown             = "admin.leaderboard.errors.unknown"
	ErrorNoCurrentAccount    = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorAdminOnly           = "admin.dashboard.adminAuth.errors.adminOnly"
	ErrorUpstreamUnsupported = "admin.leaderboard.errors.upstreamUnsupported"
	ErrorInvalidSourceOrigin = "admin.leaderboard.errors.invalidSourceOrigin"

	ErrorEmbedRequest             = "embed.leaderboard.errors.request"
	ErrorEmbedConfigNotFound      = "embed.leaderboard.errors.configNotFound"
	ErrorEmbedInvalidSrcHost      = "embed.leaderboard.errors.invalidSrcHost"
	ErrorEmbedSrcHostMismatch     = "embed.leaderboard.errors.srcHostMismatch"
	ErrorEmbedSub2apiAuth         = "embed.leaderboard.errors.sub2apiAuth"
	ErrorEmbedSub2apiRequest      = "embed.leaderboard.errors.sub2apiRequest"
	ErrorEmbedUserMismatch        = "embed.leaderboard.errors.userMismatch"
	ErrorEmbedSessionInvalid      = "embed.leaderboard.errors.sessionInvalid"
	ErrorEmbedAdminSession        = "embed.leaderboard.errors.adminSession"
	ErrorEmbedSourceBinding       = "embed.leaderboard.errors.sourceBinding"
	ErrorEmbedUpstreamUnsupported = "embed.leaderboard.errors.upstreamUnsupported"
	ErrorEmbedUpstreamRequest     = "embed.leaderboard.errors.upstreamRequest"
)

const (
	DefaultTimezone = "Asia/Shanghai"
	DefaultSortBy   = "total_tokens"
	DefaultLimit    = 50
	maxDateRange    = 31 * 24 * time.Hour
)

type requestError string

func (e requestError) Error() string { return string(e) }

type EmbedConfig struct {
	UserID              string
	AdminAccountID      string
	EmbedToken          string
	Sub2apiSourceOrigin string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type LeaderboardQuery struct {
	StartDate string
	EndDate   string
}

type LeaderboardRow struct {
	Rank        int     `json:"rank"`
	UserID      string  `json:"userId"`
	Email       string  `json:"email"`
	Requests    int     `json:"requests"`
	TotalTokens int64   `json:"totalTokens"`
	ActualCost  float64 `json:"actualCost"`
}

type LeaderboardResponse struct {
	StartDate string           `json:"startDate"`
	EndDate   string           `json:"endDate"`
	Timezone  string           `json:"timezone"`
	SortBy    string           `json:"sortBy"`
	Limit     int              `json:"limit"`
	Rows      []LeaderboardRow `json:"rows"`
}

type EmbedConfigResponse struct {
	EmbedToken          string `json:"embedToken"`
	Sub2apiSourceOrigin string `json:"sub2apiSourceOrigin"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

type UpdateEmbedConfigRequest struct {
	Sub2apiSourceOrigin string `json:"sub2apiSourceOrigin"`
}

type CreateSessionRequest struct {
	EmbedToken   string `json:"embedToken"`
	Sub2apiToken string `json:"sub2apiToken"`
	SrcHost      string `json:"srcHost"`
	UrlUserID    string `json:"userId"`
}

type CreateSessionResponse struct {
	SessionToken string `json:"sessionToken"`
	ExpiresIn    int64  `json:"expiresIn"`
}

type EmbedSession struct {
	UserID         string `json:"userId"`
	AdminAccountID string `json:"adminAccountId"`
	EmbedToken     string `json:"embedToken"`
	SrcHost        string `json:"srcHost"`
	Sub2apiUserID  string `json:"sub2apiUserId"`
}

type Sub2APIUser struct {
	ID    string
	Email string
	Role  string
}
