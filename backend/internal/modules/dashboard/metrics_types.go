package dashboard

import "time"

// MetricsResponse 是 GET /api/dashboard/metrics 返回的实时指标数据。
// 所有金额均以 CNY 计价，上游指标已乘以站点的 rechargeRate。
type MetricsResponse struct {
	TodayProfit     float64 `json:"todayProfit"`     // 今日盈利额度：管理员站点今日总实际消费
	SiteBalance     float64 `json:"siteBalance"`     // 站点用户总余额：所有非 admin 用户余额之和
	TodayPurchase   float64 `json:"todayPurchase"`   // 今日进货额度：所有上游站点今日消费（CNY）之和
	NetProfit       float64 `json:"netProfit"`       // 今日净利润：todayProfit - todayPurchase
	UpstreamBalance float64 `json:"upstreamBalance"` // 上游总余额：所有上游站点余额（CNY）之和
	GroupCount      int     `json:"groupCount"`      // 管理员站点分组总数，省去前端单独请求
}

// TrendResponse 是 GET /api/dashboard/trends 返回的历史趋势数据。
type TrendResponse struct {
	Points []TrendPoint `json:"points"`
}

// TrendPoint 代表一天的指标快照，用于趋势图渲染。
type TrendPoint struct {
	Date            string  `json:"date"` // 日期，格式 "2006-01-02"
	TodayProfit     float64 `json:"todayProfit"`
	SiteBalance     float64 `json:"siteBalance"`
	TodayPurchase   float64 `json:"todayPurchase"`
	NetProfit       float64 `json:"netProfit"`
	UpstreamBalance float64 `json:"upstreamBalance"`
}

// DailySnapshot 是 dashboard_daily_stats 表的行结构。
// 每天至多一行（user_id + admin_account_id + date 唯一），
// 通过 LiveMetrics 调用时的 upsert 和午夜调度器持续更新。
type DailySnapshot struct {
	ID              string
	UserID          string
	AdminAccountID  string
	Date            time.Time
	TodayProfit     float64
	SiteBalance     float64
	TodayPurchase   float64
	NetProfit       float64
	UpstreamBalance float64
	CreatedAt       time.Time
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

// GroupUsageTodayResponse 是 GET /api/dashboard/group-usage-today 返回的分组今日用量明细。
type GroupUsageTodayResponse struct {
	Date   string                `json:"date"`
	Total  float64               `json:"total"`
	Groups []GroupUsageTodayItem `json:"groups"`
}

// GroupUsageTodayItem 是单个分组的今日使用额度。
type GroupUsageTodayItem struct {
	GroupName   string  `json:"groupName"`
	TodayAmount float64 `json:"todayAmount"`
}

// UpstreamKeyUsageTodayResponse 是 GET /api/dashboard/upstream-key-usage-today 返回的
// 「今日成本」下钻明细：当前工作区所有上游站点中，今天有消费的 key 列表。
type UpstreamKeyUsageTodayResponse struct {
	Date        string                      `json:"date"`
	Total       float64                     `json:"total"`
	Keys        []UpstreamKeyUsageTodayItem `json:"keys"`
	FailedSites int                         `json:"failedSites,omitempty"`
	TotalSites  int                         `json:"totalSites,omitempty"`
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
