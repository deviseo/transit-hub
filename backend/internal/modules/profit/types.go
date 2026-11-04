package profit

import (
	"time"

	"transithub/backend/internal/modules/upstream"
)

// 错误 / 提示 key 直接作为 i18n key 返回给前端。
const (
	ErrorNoCurrentAccount = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorAuthRequired     = "admin.profit.errors.authRequired"
	ErrorPlatform         = "admin.profit.errors.platformUnsupported"
	ErrorSyncInProgress   = "admin.profit.errors.syncInProgress"
	ErrorRoutesFetch      = "admin.profit.errors.routesFetch"
	ErrorRevenueFetch     = "admin.profit.errors.revenueFetch"
	ErrorNetwork          = "admin.profit.errors.network"
	ErrorInterrupted      = "admin.profit.errors.interrupted"
	ErrorInvalidRange     = "admin.profit.errors.invalidRange"
	ErrorRequest          = "admin.profit.errors.request"

	WarningCostFailed    = "admin.profit.warnings.costFailed"
	WarningFlowFallback  = "admin.profit.warnings.flowFallback"
	WarningTooManyRoutes = "admin.profit.warnings.tooManyRoutes"
)

// 同步状态。
const (
	StatusRunning     = "running"
	StatusSuccess     = "success"
	StatusFailed      = "failed"
	StatusInterrupted = "interrupted"
)

// 采集是怎么开始的（前端据此决定完成后是否提醒）。
const (
	StartedByManual   = "manual"
	StartedByAuto     = "auto"
	StartedByResume   = "resume"
	StartedBySchedule = "schedule"
)

// 营收数据来源。
const (
	SourceSub2APIGroups = "sub2api_groups"
	SourceNewAPIFlow    = "newapi_flow"
	SourceNewAPILogStat = "newapi_log_stat"
)

// 未归因营收的类别。
const (
	CategoryOfficial     = "official"
	CategoryUnregistered = "unregistered"
	CategoryUnknown      = "unknown"
)

// 提示类型（前端按类型渲染文案）。
const (
	InsightNegativeMargin = "negativeMargin"
	InsightUnmatchedCost  = "unmatchedCost"
	InsightUnregistered   = "unregistered"
	InsightLowRunway      = "lowRunway"
	InsightCostSpike      = "costSpike"
	InsightNoRate         = "noRate"
	InsightCostFailed     = "costFailed"
	InsightPriceIncrease  = "priceIncrease"
)

// 调价影响估算的状态。
const (
	EstimateOK      = "ok"
	EstimateNoUsage = "no_usage"
	EstimateNoRate  = "no_rate"
	EstimateFailed  = "failed"
)

// Workspace 是数据隔离单元（TransitHub 用户 + admin 工作区）。
type Workspace struct {
	UserID         string
	AdminAccountID string
}

// SyncState 是工作区的采集状态（数据库行）。
type SyncState struct {
	Platform        string
	Status          string
	ErrorKey        string
	WarningKey      string
	RevenueSource   string
	RouteCount      int
	SyncedFrom      *time.Time
	SyncedTo        *time.Time
	CompleteThrough *time.Time
	LastStartedAt   *time.Time
	LastSyncedAt    *time.Time
	StartedBy       string
}

// SyncStatus 是返回给前端的采集状态。
type SyncStatus struct {
	Platform        string     `json:"platform"`
	Status          string     `json:"status"`
	ErrorKey        string     `json:"errorKey,omitempty"`
	WarningKey      string     `json:"warningKey,omitempty"`
	RevenueSource   string     `json:"revenueSource,omitempty"`
	RouteCount      int        `json:"routeCount"`
	SyncedFrom      string     `json:"syncedFrom,omitempty"`
	SyncedTo        string     `json:"syncedTo,omitempty"`
	CompleteThrough string     `json:"completeThrough,omitempty"`
	LastStartedAt   *time.Time `json:"lastStartedAt"`
	LastSyncedAt    *time.Time `json:"lastSyncedAt"`
	StartedBy       string     `json:"startedBy,omitempty"`
}

// Route 是自己站点上的一条路由（sub2api 上游账号 / new-api 渠道）。
type Route struct {
	ID      string
	Name    string
	Kind    string
	Status  string
	BaseURL string
	Groups  []string
	Deleted bool
}

// RouteRevenue 是一条路由在某个分组下的营收（站点额度，美元口径）。
type RouteRevenue struct {
	RouteID  string
	Group    string
	Revenue  float64
	Requests int64
}

// SiteCost 是一个上游站点某一天（或一段时间）的成本。
type SiteCost struct {
	SiteID       string
	SiteName     string
	BaseURL      string
	Consume      float64
	RechargeRate float64
	Cost         float64
	Status       string
	// 区间汇总时：各状态的天数。
	FailedDays       int
	ApproximatedDays int
	NoRateDays       int
	Days             int
}

// DayTotals 是某一天的营收（站点额度）与成本（人民币）。
type DayTotals struct {
	Day         time.Time
	Revenue     float64
	Cost        float64
	HasRevenue  bool
	HasCost     bool
	FailedSites int
}

// RangeQuery 是统计区间：Days（截至昨天的最近 N 个完整自然日）或 From / To（含两端，不晚于昨天）。
type RangeQuery struct {
	Days int
	From string
	To   string
}

// DayRange 是含两端的业务日区间。
type DayRange struct {
	From string `json:"from"`
	To   string `json:"to"`
	Days int    `json:"days"`
}

// Totals 是一个区间的汇总；金额均为人民币。
type Totals struct {
	Revenue             float64  `json:"revenue"`
	Cost                float64  `json:"cost"`
	Profit              float64  `json:"profit"`
	Margin              *float64 `json:"margin"`
	AttributedRevenue   float64  `json:"attributedRevenue"`
	UnattributedRevenue float64  `json:"unattributedRevenue"`
	// AttributedShare 是有成本对应（已匹配上游）的营收占比。
	AttributedShare *float64 `json:"attributedShare"`
	UnmatchedCost   float64  `json:"unmatchedCost"`
	FailedSiteDays  int      `json:"failedSiteDays"`
}

// DailyPoint 是每日营收、成本与毛利；没有数据的日期为 nil。
type DailyPoint struct {
	Day     string   `json:"day"`
	Revenue *float64 `json:"revenue"`
	Cost    *float64 `json:"cost"`
	Profit  *float64 `json:"profit"`
}

// RouteRef 是展示用的路由信息。
type RouteRef struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Groups  []string `json:"groups"`
	Status  string   `json:"status"`
	Deleted bool     `json:"deleted"`
	Revenue float64  `json:"revenue"`
}

// UpstreamSite 是一个上游站点的成本与续航。
type UpstreamSite struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Platform       string   `json:"platform"`
	RechargeRate   float64  `json:"rechargeRate"`
	Cost           float64  `json:"cost"`
	FailedDays     int      `json:"failedDays"`
	NoRate         bool     `json:"noRate"`
	Deleted        bool     `json:"deleted"`
	Balance        *float64 `json:"balance"`
	AvgDailyCost   *float64 `json:"avgDailyCost"`
	RunwayDays     *float64 `json:"runwayDays"`
	SuggestedTopUp *float64 `json:"suggestedTopUp"`
}

// UpstreamRow 是一个上游（按可注册域名合并的站点）的营收、成本与毛利。
type UpstreamRow struct {
	Key     string         `json:"key"`
	Sites   []UpstreamSite `json:"sites"`
	Routes  []RouteRef     `json:"routes"`
	Revenue float64        `json:"revenue"`
	Cost    float64        `json:"cost"`
	Profit  float64        `json:"profit"`
	Margin  *float64       `json:"margin"`
}

// GroupRow 是本站一个分组的营收与分摊成本。
type GroupRow struct {
	Group               string   `json:"group"`
	Revenue             float64  `json:"revenue"`
	AttributedRevenue   float64  `json:"attributedRevenue"`
	UnattributedRevenue float64  `json:"unattributedRevenue"`
	Cost                float64  `json:"cost"`
	Profit              float64  `json:"profit"`
	Margin              *float64 `json:"margin"`
}

// UnattributedRow 是没有对应已登记上游的营收（按上游地址合并）。
type UnattributedRow struct {
	Key      string     `json:"key"`
	Category string     `json:"category"`
	Routes   []RouteRef `json:"routes"`
	Revenue  float64    `json:"revenue"`
}

// Insight 是一条需要关注的提示。
type Insight struct {
	Kind     string   `json:"kind"`
	Severity string   `json:"severity"`
	Subject  string   `json:"subject"`
	Value    *float64 `json:"value,omitempty"`
	Extra    *float64 `json:"extra,omitempty"`
}

// SummaryResponse 是利润分析页的全部数据。
type SummaryResponse struct {
	Range         DayRange          `json:"range"`
	PreviousRange DayRange          `json:"previousRange"`
	UnitPrice     float64           `json:"unitPrice"`
	Sync          SyncStatus        `json:"sync"`
	Synced        bool              `json:"synced"`
	Complete      bool              `json:"complete"`
	Totals        Totals            `json:"totals"`
	Previous      *Totals           `json:"previous"`
	Daily         []DailyPoint      `json:"daily"`
	Upstreams     []UpstreamRow     `json:"upstreams"`
	Groups        []GroupRow        `json:"groups"`
	Unattributed  []UnattributedRow `json:"unattributed"`
	Insights      []Insight         `json:"insights"`
	// PriceChanges 是最近 priceChangeListDays 天检测到的上游分组调价（与所选区间无关，最新的在前）。
	PriceChanges []PriceChange `json:"priceChanges"`
}

// PriceChangeInput 是一次检测到的上游分组倍率变化。
type PriceChangeInput struct {
	Group         upstream.GroupInfo
	OldMultiplier float64
	NewMultiplier float64
}

// PriceChange 是一次上游分组调价及其对每天成本的估算（人民币）：
// 日均成本 = 该分组近 WindowDays 个完整自然日在上游的消费 ÷ 天数 × 充值倍率；每天影响 = 日均成本 × (新倍率 ÷ 旧倍率 − 1)。
type PriceChange struct {
	ID             int64     `json:"id"`
	SiteID         string    `json:"siteId"`
	SiteName       string    `json:"siteName"`
	UpstreamKey    string    `json:"upstreamKey"`
	GroupName      string    `json:"groupName"`
	OldMultiplier  float64   `json:"oldMultiplier"`
	NewMultiplier  float64   `json:"newMultiplier"`
	WindowDays     int       `json:"windowDays"`
	EstimateStatus string    `json:"estimateStatus"`
	AvgDailyCost   *float64  `json:"avgDailyCost"`
	DailyImpact    *float64  `json:"dailyImpact"`
	DetectedAt     time.Time `json:"detectedAt"`

	GroupID       string   `json:"-"`
	BaseURL       string   `json:"-"`
	RechargeRate  float64  `json:"-"`
	WindowConsume *float64 `json:"-"`
}

type requestError string

func (e requestError) Error() string { return string(e) }
