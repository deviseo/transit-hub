package upstream

import (
	"context"
	"strings"
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
	Platform     Platform
	BaseURL      string
	Cookie       string
	UserID       string
	AccessToken  string
	RefreshToken string
	TokenType    string
	// ExpiresAt 是 access token 过期的毫秒时间戳，来自登录/刷新响应的 expires_in。
	// 临期时由 refreshIfNeeded 用 refresh token 自动换新（refresh token 本身无过期时间）。
	ExpiresAt *int64
	// QuotaPerUnit 是 new-api 的 quota 换算单位（来自 /api/status 的 quota_per_unit 字段）。
	// sub2api 不使用此字段。为 0 时回退到默认值 500000。
	QuotaPerUnit float64
}

// IsAuthenticated 按平台判断会话是否有效（已持有登录凭证）。
// sub2api 需要 AccessToken，new-api 需要 Cookie + UserID。
func (s Session) IsAuthenticated() bool {
	switch s.Platform {
	case PlatformNewAPI:
		return strings.TrimSpace(s.Cookie) != "" && strings.TrimSpace(s.UserID) != ""
	case PlatformSub2API:
		return strings.TrimSpace(s.AccessToken) != ""
	default:
		return strings.TrimSpace(s.AccessToken) != "" || (strings.TrimSpace(s.Cookie) != "" && strings.TrimSpace(s.UserID) != "")
	}
}

// Sub2APIKeyItem 表示从上游 Sub2API 站点获取的单个 API Key 信息。
// 用于手动绑定时展示 key 列表供用户选择。
type Sub2APIKeyItem struct {
	ID        string `json:"id"`
	Key       string `json:"key"`
	Name      string `json:"name"`
	GroupID   string `json:"groupId"`
	GroupName string `json:"groupName"`
	Status    string `json:"status"`
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
