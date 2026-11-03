package upstream

import (
	"context"
	"strings"
	"time"
)

type Platform string

const (
	PlatformAuto    Platform = "auto"
	PlatformNewAPI  Platform = "newapi"
	PlatformSub2API Platform = "sub2api"
)

type Status string

const (
	StatusConnecting Status = "connecting"
	StatusSyncing    Status = "syncing"
	StatusConnected  Status = "connected"
	StatusError      Status = "error"
)

const (
	ErrorNotFound        = "admin.upstream.errors.notFound"
	ErrorInvalidURL      = "admin.upstream.errors.invalidUrl"
	ErrorAuth            = "admin.upstream.errors.auth"
	ErrorNetwork         = "admin.upstream.errors.network"
	ErrorRequest         = "admin.upstream.errors.request"
	ErrorInvalidResponse = "admin.upstream.errors.invalidResponse"
	ErrorUnknown         = "admin.upstream.errors.unknown"
	// ErrorVerificationRequired 表示账号密码登录要求二次验证（2FA / Passkey 等），无法无人值守地完成，
	// 需要改用访问令牌接入。
	ErrorVerificationRequired = "admin.upstream.errors.verificationRequired"
	// ErrorNewAPITokenRejected 表示未填写用户 ID 时 new-api 访问令牌校验失败：旧版 new-api 的系统访问令牌
	// 必须同时提供用户 ID（New-Api-User）；新版 PAT 则可能是令牌无效、过期或缺少所需 scope。
	ErrorNewAPITokenRejected = "admin.upstream.errors.newApiTokenRejected"
)

// SSE 同步流事件类型。
const (
	SyncEventSyncing  = "syncing"
	SyncEventDone     = "done"
	SyncEventError    = "error"
	SyncEventComplete = "complete"
)

// SyncEvent 是 SSE 同步流中每个 data: 行的 JSON 载荷。
// 前端通过 event 字段判断当前阶段并更新对应站点卡片的进度。
type SyncEvent struct {
	Event    string    `json:"event"`
	SiteID   string    `json:"siteId"`
	Attempt  int       `json:"attempt,omitempty"`
	MaxRetry int       `json:"maxRetry,omitempty"`
	Site     *Response `json:"site,omitempty"`
	ErrorKey string    `json:"errorKey,omitempty"`
}

// SyncEventCallback 是 SSE 事件的推送回调，由 handler 注入，
// 负责将事件序列化并写入 ResponseWriter。
type SyncEventCallback func(SyncEvent)

type AuthMode string

const (
	AuthModePassword AuthMode = "password"
	AuthModeToken    AuthMode = "token"
	AuthModeUserKey  AuthMode = "user_key"
)

type MetricValue struct {
	Value   *float64 `json:"value"`
	Display string   `json:"display"`
}

type GroupInfo struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Platform          *string  `json:"platform"`
	Multiplier        *float64 `json:"multiplier"`
	MultiplierDisplay string   `json:"multiplierDisplay"`
	MultiplierMode    string   `json:"multiplierMode,omitempty"`
	// 以下字段为 sub2api 专属倍率合并规则新增的向后兼容字段：/groups/available 默认倍率
	// 与 /groups/rates 专属倍率覆盖后，Multiplier 始终表示最终生效倍率；这些字段仅供前端
	// 展示"默认倍率 -> 专属倍率"提示，不参与业务计算。旧数据缺少这些字段时 omitempty 生效，
	// 前端按无专属倍率处理。
	DefaultMultiplier          *float64 `json:"defaultMultiplier,omitempty"`
	DefaultMultiplierDisplay   string   `json:"defaultMultiplierDisplay,omitempty"`
	DedicatedMultiplier        *float64 `json:"dedicatedMultiplier,omitempty"`
	DedicatedMultiplierDisplay string   `json:"dedicatedMultiplierDisplay,omitempty"`
	HasDedicatedMultiplier     bool     `json:"hasDedicatedMultiplier"`
}

// SnapshotGroup and SnapshotWriter keep upstream decoupled from the group_rates
// module while still allowing successful metric refreshes to publish multiplier
// history. Any implementation can be injected by the HTTP server assembly layer.
type SnapshotGroup struct {
	ID         string
	Name       string
	Platform   *string
	Multiplier *float64
}

type SnapshotWriter interface {
	SaveSiteSnapshot(ctx context.Context, userID string, adminAccountID string, siteID string, siteName string, sitePlatform Platform, groups []SnapshotGroup) error
}

type Metrics struct {
	Balance         MetricValue `json:"balance"`
	TodayConsume    MetricValue `json:"todayConsume"`
	HistoryRecharge MetricValue `json:"historyRecharge"`
	Group           GroupInfo   `json:"group"`
	Groups          []GroupInfo `json:"groups"`
}

type CreateRequest struct {
	Name         string   `json:"name"`
	SiteURL      string   `json:"siteUrl"`
	Platform     Platform `json:"platform"`
	AuthMode     AuthMode `json:"authMode"`
	Account      string   `json:"account"`
	Password     string   `json:"password"`
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	TokenType    string   `json:"tokenType"`
	UserID       string   `json:"userId"`
	Remark       string   `json:"remark"`
	RechargeRate float64  `json:"rechargeRate"`
}

type UpdateRequest struct {
	Name         string   `json:"name"`
	SiteURL      string   `json:"siteUrl"`
	Platform     Platform `json:"platform"`
	AuthMode     AuthMode `json:"authMode"`
	Account      string   `json:"account"`
	Password     string   `json:"password"`
	AccessToken  string   `json:"accessToken"`
	RefreshToken string   `json:"refreshToken"`
	TokenType    string   `json:"tokenType"`
	UserID       string   `json:"userId"`
	Remark       string   `json:"remark"`
	RechargeRate float64  `json:"rechargeRate"`
}

// SiteSettings 站点级预警覆盖配置。nil 表示使用全局默认值。
type SiteSettings struct {
	BalanceThreshold *float64 `json:"balanceThreshold"`
}

type Site struct {
	ID                string       `json:"id"`
	UserID            string       `json:"-"`
	AdminAccountID    string       `json:"-"`
	Name              string       `json:"name"`
	BaseURL           string       `json:"baseUrl"`
	Platform          Platform     `json:"platform"`
	RequestedPlatform Platform     `json:"requestedPlatform"`
	Account           string       `json:"account"`
	Remark            string       `json:"remark"`
	RechargeRate      float64      `json:"rechargeRate"`
	Status            Status       `json:"status"`
	ErrorKey          *string      `json:"errorKey"`
	Metrics           Metrics      `json:"metrics"`
	Settings          SiteSettings `json:"settings"`
	LastSyncedAt      *int64       `json:"lastSyncedAt"`
	Session           *Session     `json:"-"`
}

type Response struct {
	ID                string       `json:"id"`
	UserID            string       `json:"-"`
	AdminAccountID    string       `json:"-"`
	Name              string       `json:"name"`
	BaseURL           string       `json:"baseUrl"`
	Platform          Platform     `json:"platform"`
	RequestedPlatform Platform     `json:"requestedPlatform"`
	Account           string       `json:"account"`
	Remark            string       `json:"remark"`
	RechargeRate      float64      `json:"rechargeRate"`
	Status            Status       `json:"status"`
	ErrorKey          *string      `json:"errorKey"`
	Metrics           Metrics      `json:"metrics"`
	Settings          SiteSettings `json:"settings"`
	LastSyncedAt      *int64       `json:"lastSyncedAt"`
}

type Session struct {
	Platform    Platform
	BaseURL     string
	Cookie      string
	UserID      string
	AccessToken string
	// AdminAPIKey 是 Sub2API 管理路由使用的 Admin API Key，通过 x-api-key 发送。
	// 它与用户 JWT/AccessToken 分开保存，避免被误发到普通用户路由。
	AdminAPIKey string `json:",omitempty"`
	// RefreshToken：sub2api 为登录返回的 refresh_token；新版 new-api 密码登录时为 HttpOnly refresh
	// cookie（new_api_refresh）的值。两者每次刷新都会轮换，刷新后必须持久化新值——new-api 会把
	// 30 秒容错窗口之后重新出现的上一代 refresh token 视为重放并吊销整个登录会话。
	RefreshToken string
	TokenType    string
	// ExpiresAt 是 access token 过期的毫秒时间戳：sub2api 来自登录/刷新响应的 expires_in，新版 new-api
	// 来自 access_expires_at（只有 15 分钟）。临期时由 RefreshSession 用 refresh 凭据自动换新。
	ExpiresAt *int64
	// QuotaPerUnit 是 new-api 的 quota 换算单位（来自 /api/status 的 quota_per_unit 字段）。
	// sub2api 不使用此字段。为 0 时回退到默认值 500000。
	QuotaPerUnit float64
}

// IsAuthenticated 按平台判断会话是否有效（已持有登录凭证）。
// sub2api 支持用户 AccessToken 或 Admin API Key。new-api 的 Bearer 访问令牌本身就是凭据——
// 新版密码登录得到的 JWT、PAT（nap_ 开头）或旧版系统访问令牌都属于这一类；只有旧版 Cookie 会话
// 还必须配合 UserID（New-Api-User）。
func (s Session) IsAuthenticated() bool {
	switch s.Platform {
	case PlatformNewAPI:
		return strings.TrimSpace(s.AccessToken) != "" ||
			(strings.TrimSpace(s.Cookie) != "" && strings.TrimSpace(s.UserID) != "")
	case PlatformSub2API:
		return strings.TrimSpace(s.AccessToken) != "" || strings.TrimSpace(s.AdminAPIKey) != ""
	default:
		return strings.TrimSpace(s.AccessToken) != "" || strings.TrimSpace(s.AdminAPIKey) != "" ||
			(strings.TrimSpace(s.Cookie) != "" && strings.TrimSpace(s.UserID) != "")
	}
}

// sameCredentials 判断两个会话携带的凭据是否完全一致。刷新后只在凭据确实变化时写回，
// 避免把调用方手里较旧的会话副本覆盖到其他流程刚轮换出的新会话上。
func (s Session) sameCredentials(other Session) bool {
	if s.Platform != other.Platform || s.BaseURL != other.BaseURL || s.Cookie != other.Cookie || s.UserID != other.UserID ||
		s.AccessToken != other.AccessToken || s.AdminAPIKey != other.AdminAPIKey || s.RefreshToken != other.RefreshToken ||
		s.TokenType != other.TokenType || s.QuotaPerUnit != other.QuotaPerUnit {
		return false
	}
	if (s.ExpiresAt == nil) != (other.ExpiresAt == nil) {
		return false
	}
	return s.ExpiresAt == nil || *s.ExpiresAt == *other.ExpiresAt
}

type LoginResult struct {
	Platform Platform
	Session  Session
	Metrics  Metrics
}

type GroupDailyStat struct {
	GroupName       string  `json:"groupName"`
	TodayActualCost float64 `json:"todayActualCost"`
}

type AdminSiteBalance struct {
	Balance float64 `json:"balance"`
}

// BalanceFilter 控制统计站点用户余额时的过滤条件。
// 由仪表盘模块创建，传递给 PlatformService 在分页遍历用户时应用。
type BalanceFilter struct {
	ExcludeAdmin    bool      // 是否排除 admin 角色用户
	ExcludeBalances []float64 // 需要排除的精确余额值（如 0、0.1、1 等）
}

// Sub2APIAdminUser 是 Sub2API admin 用户接口（详情 / 列表）返回的只读字段。
// 字段在远端响应中不存在或类型不匹配时保持零值/nil，由调用方按需降级，不在这里伪造数据。
type Sub2APIAdminUser struct {
	ID            string
	Email         string
	Username      string
	Role          string
	Status        string
	Balance       *float64
	FrozenBalance *float64
	Concurrency   *int
	RPMLimit      *int
	CreatedAt     *time.Time
	LastUsedAt    *time.Time
	// TotalRecharged 是用户累计充值金额（sub2api total_recharged），旧版本没有该字段时为 nil。
	TotalRecharged *float64
}

// Sub2APIAdminUsersQuery 是 Sub2API admin 用户分页列表的安全查询对象。
// 调用方只能通过这些显式字段影响远端查询，PlatformService 会继续做白名单和分页夹紧。
type Sub2APIAdminUsersQuery struct {
	Page      int
	PageSize  int
	Status    string
	Role      string
	Search    string
	SortBy    string
	SortOrder string
	Timezone  string
}

type Sub2APIAdminUsersPage struct {
	Items    []Sub2APIAdminUser
	Total    int
	Page     int
	PageSize int
	Pages    int
	// TotalKnown/PagesKnown let batch jobs distinguish real upstream pagination
	// metadata from local fallbacks, so all-mode jobs never silently truncate an
	// unknown-length user stream.
	TotalKnown bool
	PagesKnown bool
}

// KeyUsageTodayStat 是平台层返回的单个 key 今日消费统计（上游平台原始金额，未乘以站点 rechargeRate）。
type KeyUsageTodayStat struct {
	KeyID       string
	KeyName     string
	GroupName   string
	TodayAmount float64
}

// KeyUsageTodayItem 是仪表盘「今日成本」下钻明细中单个 key 的聚合结果（已按站点 rechargeRate 换算）。
type KeyUsageTodayItem struct {
	SiteID       string
	SiteName     string
	Platform     Platform
	KeyID        string
	KeyName      string
	GroupName    string
	TodayAmount  float64
	RawAmount    float64
	RechargeRate float64
}

// KeyUsageCollectionError 表示跨多个上游站点采集 Key 用量时有站点失败。
// Items 仍由调用方通过正常返回值获得；FailedSites < TotalSites 时属于部分成功，
// 调用方可以展示已成功数据并明确标注缺失范围，而不必把失败站点静默当成零消费。
type KeyUsageCollectionError struct {
	FailedSites int
	TotalSites  int
	Cause       error
}

func (e *KeyUsageCollectionError) Error() string {
	if e == nil || e.Cause == nil {
		return ErrorRequest
	}
	return e.Cause.Error()
}

func (e *KeyUsageCollectionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// BalanceBreakdownItem 是仪表盘「上游总余额」下钻明细中单个站点的余额展示数据。
// Balance/RawBalance 为 nil 表示该站点余额未知（未配置 rechargeRate 或尚未同步成功）。
type BalanceBreakdownItem struct {
	SiteID       string
	SiteName     string
	Platform     Platform
	Balance      *float64
	RawBalance   *float64
	RechargeRate float64
	LastSyncedAt *int64
	Status       Status
}
