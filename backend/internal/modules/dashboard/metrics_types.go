package dashboard

import "time"

// 结算状态常量。
const (
	SettlementStatusProvisional = "provisional" // 首页访问写入的临时快照
	SettlementStatusPartial     = "partial"     // 部分站点成功的日结结果
	SettlementStatusFinal       = "final"       // 所有站点成功的完整日结结果
	SettlementStatusMissing     = "missing"     // 占位：该日期无任何记录
)

// 数据来源常量。
const (
	SnapshotSourceLiveCache  = "live_cache"  // 首页请求时从内存缓存写入
	SnapshotSourceDatedQuery = "dated_query" // 日结任务按日期精确查询写入
	SnapshotSourceBackfill   = "backfill"    // 受控回填操作写入
)

// MetricsResponse 是 GET /api/dashboard/metrics 返回的实时指标数据。
// 所有金额均以 CNY 计价，上游指标已乘以站点的 rechargeRate。
// TodayProfit/TodayPurchase/NetProfit 为 *float64，nil 表示该指标不可用。
type MetricsResponse struct {
	Date            string            `json:"date"`                   // 指标所属的固定上海业务日
	Timezone        string            `json:"timezone"`               // 业务日时区，固定为 Asia/Shanghai
	TodayProfit     *float64          `json:"todayProfit"`            // 今日盈利额度；nil 表示营收不可用
	SiteBalance     float64           `json:"siteBalance"`            // 站点用户总余额：所有非 admin 用户余额之和
	TodayPurchase   *float64          `json:"todayPurchase"`          // 今日进货额度；nil 表示成本不完整
	NetProfit       *float64          `json:"netProfit"`              // 今日净利润；任一分项为 nil 时为 nil
	UpstreamBalance float64           `json:"upstreamBalance"`        // 上游总余额：所有上游站点余额（CNY）之和
	GroupCount      int               `json:"groupCount"`             // 管理员站点分组总数，省去前端单独请求
	MetricErrors    map[string]string `json:"metricErrors,omitempty"` // 局部指标拉取失败原因
	CostQuality     *CostQuality      `json:"costQuality,omitempty"`  // 成本质量信息，成本不完整时必须存在
}

// CostQuality 描述本次成本采集的完整性与质量信息，前端根据此字段分级展示。
type CostQuality struct {
	BusinessDate   string          `json:"businessDate"`
	ConfirmedCost  float64         `json:"confirmedCost"` // 已确认站点成本之和（真实下限）
	Complete       bool            `json:"complete"`      // 所有目标站点均成功采集时为 true
	ExpectedSites  int             `json:"expectedSites"`
	CollectedSites int             `json:"collectedSites"`
	FailedSites    int             `json:"failedSites"`
	ObservedAt     *time.Time      `json:"observedAt,omitempty"`
	Failures       []SiteCostFault `json:"failures,omitempty"` // 失败站点脱敏原因
}

// SiteCostFault 记录单个站点的成本采集失败原因。
type SiteCostFault struct {
	SiteName string `json:"siteName"`
	Reason   string `json:"reason"` // "date_mismatch" / "stale" / "fetch_error"
}

// TrendResponse 是 GET /api/dashboard/trends 返回的历史趋势数据。
type TrendResponse struct {
	Points []TrendPoint `json:"points"`
}

// TrendPoint 代表一天的指标快照，用于趋势图渲染。
// 成本/营收/净利润允许 NULL：NULL 表示该天数据未采集，前端渲染断点而非伪零。
type TrendPoint struct {
	Date            string   `json:"date"`        // 日期，格式 "2006-01-02"
	TodayProfit     *float64 `json:"todayProfit"` // nil 表示该天营收未采集
	SiteBalance     float64  `json:"siteBalance"`
	TodayPurchase   *float64 `json:"todayPurchase"` // nil 表示该天成本未采集
	NetProfit       *float64 `json:"netProfit"`     // nil 表示该天净利润不可用
	UpstreamBalance float64  `json:"upstreamBalance"`
}

// DailySnapshot 是 dashboard_daily_stats 表的行结构。
// 每天至多一行（user_id + admin_account_id + date 唯一）。
// 金额字段允许 NULL：nil 表示该指标未采集到数据，0.0 表示真实零值。
type DailySnapshot struct {
	ID                 string
	UserID             string
	AdminAccountID     string
	Date               time.Time
	TodayProfit        *float64
	SiteBalance        *float64
	TodayPurchase      *float64
	NetProfit          *float64
	UpstreamBalance    *float64
	CreatedAt          time.Time
	SettlementStatus   string     // provisional/partial/final
	SnapshotSource     string     // live_cache/dated_query/backfill
	ObservedAt         *time.Time // 数据实际采集时间
	FinalizedAt        *time.Time // 日结成功写入时间，仅 final 行有值
	CostExpectedCount  *int       // 应包含的上游站点数
	CostCollectedCount *int       // 实际成功取到成本的站点数
	BalanceObservedAt  *time.Time // 余额观测时间
}

// SiteDailyCost 是 upstream_site_daily_costs 表的行结构。
// 记录每个上游站点每天的成本明细，由日结任务和回填任务写入。
type SiteDailyCost struct {
	ID             string
	UserID         string
	AdminAccountID string
	Date           time.Time
	SiteID         string
	SiteName       string // 冗余快照，站点删除后历史仍可读
	Platform       string
	RawCost        *float64 // 上游原始成本（允许 NULL）
	RechargeRate   float64
	AdjustedCost   *float64 // raw_cost × recharge_rate（允许 NULL）
	Status         string   // ok/partial/failed/expired/date_mismatch
	Source         string   // dated_query/backfill/best_effort
	ErrorReason    string
	ObservedAt     time.Time
}

// DailyStatItem 是 GET /api/dashboard/daily-stats 返回的单日数据。
type DailyStatItem struct {
	Date               string           `json:"date"`
	SettlementStatus   string           `json:"settlementStatus"` // missing/provisional/partial/final
	SnapshotSource     string           `json:"snapshotSource,omitempty"`
	TodayProfit        *float64         `json:"todayProfit,omitempty"`
	ConfirmedCost      *float64         `json:"confirmedCost,omitempty"`    // 成本下限
	NetProfitCeiling   *float64         `json:"netProfitCeiling,omitempty"` // 暂估上限：todayProfit - confirmedCost
	MarginCeiling      *float64         `json:"marginCeiling,omitempty"`
	CostExpectedCount  *int             `json:"costExpectedCount,omitempty"`
	CostCollectedCount *int             `json:"costCollectedCount,omitempty"`
	FinalizedAt        *string          `json:"finalizedAt,omitempty"`
	SiteCosts          []SiteCostDetail `json:"siteCosts,omitempty"`          // expand=true 时填充
	SiteCostsLoadError bool             `json:"siteCostsLoadError,omitempty"` // expand=true 但查询失败
}

// SiteCostDetail 是逐日明细中单个站点的成本展示数据。
type SiteCostDetail struct {
	SiteID       string   `json:"siteId"`
	SiteName     string   `json:"siteName"`
	Platform     string   `json:"platform"`
	RawCost      *float64 `json:"rawCost,omitempty"`
	RechargeRate float64  `json:"rechargeRate"`
	AdjustedCost *float64 `json:"adjustedCost,omitempty"` // rawCost × rechargeRate
	Status       string   `json:"status"`                 // ok/partial/failed/expired/date_mismatch
	Source       string   `json:"source"`                 // dated_query/backfill/best_effort
	ErrorReason  string   `json:"errorReason,omitempty"`
	ObservedAt   string   `json:"observedAt"`
}

// AdminGroupsResponse 是 GET /api/dashboard/groups 返回的管理员站点分组数据。
type AdminGroupsResponse struct {
	Count  int              `json:"count"`
	Groups []AdminGroupItem `json:"groups"`
}

// AdminGroupItem 是管理员站点中单个分组的展示数据。
type AdminGroupItem struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Platform   string `json:"platform"`
	Multiplier string `json:"multiplier"`
}

// GroupUsageTodayResponse 是 GET /api/dashboard/group-usage-today 返回的分组今日营收及实时利润明细。
type GroupUsageTodayResponse struct {
	Date                    string                `json:"date"`
	Total                   float64               `json:"total"` // 兼容字段：今日营收
	TotalRevenue            float64               `json:"totalRevenue"`
	TotalCost               *float64              `json:"totalCost,omitempty"`
	TotalProfit             *float64              `json:"totalProfit,omitempty"`
	ProfitAvailable         bool                  `json:"profitAvailable"`
	ProfitUnavailableReason string                `json:"profitUnavailableReason,omitempty"`
	Groups                  []GroupUsageTodayItem `json:"groups"`
}

// GroupUsageTodayItem 是单个分组的今日营收、成本及实时利润。
type GroupUsageTodayItem struct {
	GroupName    string   `json:"groupName"`
	TodayAmount  float64  `json:"todayAmount"` // 兼容字段：今日营收
	TodayRevenue float64  `json:"todayRevenue"`
	TodayCost    *float64 `json:"todayCost,omitempty"`
	TodayProfit  *float64 `json:"todayProfit,omitempty"`
}

// UpstreamKeyUsageTodayResponse 是 GET /api/dashboard/upstream-key-usage-today 返回的
// 「今日成本」下钻明细：当前工作区所有上游站点中，今天有消费的 key 列表。
type UpstreamKeyUsageTodayResponse struct {
	Date        string                      `json:"date"`
	Total       float64                     `json:"total"`
	Keys        []UpstreamKeyUsageTodayItem `json:"keys"`
	FailedSites int                         `json:"failedSites,omitempty"` // 按首页缓存成本口径统计的不可用站点数
	TotalSites  int                         `json:"totalSites,omitempty"`  // 按首页缓存成本口径统计的目标站点数
}

// UpstreamKeyUsageTodayItem 是单个 key 的今日消费明细。
// TodayAmount 已乘以所属站点的 rechargeRate，口径与仪表盘「今日成本」卡片一致；RawAmount 为上游平台原始金额。
type UpstreamKeyUsageTodayItem struct {
	SiteID       string  `json:"siteId"`
	SiteName     string  `json:"siteName"`
	Platform     string  `json:"platform"`
	KeyID        string  `json:"keyId"`
	KeyName      string  `json:"keyName"`
	GroupName    string  `json:"groupName"`
	TodayAmount  float64 `json:"todayAmount"`
	RawAmount    float64 `json:"rawAmount"`
	RechargeRate float64 `json:"rechargeRate"`
}

// UpstreamBalanceBreakdownResponse 是 GET /api/dashboard/upstream-balance-breakdown 返回的
// 「上游总余额」下钻明细：当前工作区所有上游站点的缓存余额列表。
type UpstreamBalanceBreakdownResponse struct {
	Total float64                        `json:"total"`
	Sites []UpstreamBalanceBreakdownItem `json:"sites"`
}

// UpstreamBalanceBreakdownItem 是单个上游站点的余额明细。
// Balance/RawBalance 为 null 表示该站点余额尚未同步或未配置 rechargeRate。
type UpstreamBalanceBreakdownItem struct {
	SiteID       string   `json:"siteId"`
	SiteName     string   `json:"siteName"`
	Platform     string   `json:"platform"`
	Balance      *float64 `json:"balance"`
	RawBalance   *float64 `json:"rawBalance"`
	RechargeRate float64  `json:"rechargeRate"`
	LastSyncedAt *int64   `json:"lastSyncedAt"`
	Status       string   `json:"status"`
}

// BalanceFilterConfig 是用户自定义的站点用户余额筛选条件，持久化在 dashboard_balance_filter 表中。
// 每个 (user_id, admin_account_id) 最多一行配置，控制 LiveMetrics 计算 siteBalance 时的过滤行为。
type BalanceFilterConfig struct {
	UserID          string    `json:"-"`
	AdminAccountID  string    `json:"-"`
	ExcludeAdmin    bool      `json:"excludeAdmin"`    // 是否排除 admin 角色用户（默认 true）
	ExcludeBalances []float64 `json:"excludeBalances"` // 需要排除的精确余额值列表
}
