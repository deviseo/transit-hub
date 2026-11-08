package sales

import "time"

// 错误 key 直接作为 i18n key 返回给前端。
const (
	ErrorNoCurrentAccount = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorInvalidRange     = "admin.sales.errors.invalidRange"
	ErrorRequest          = "admin.sales.errors.request"
	ErrorInvalidEntry     = "admin.sales.errors.invalidEntry"
	ErrorInvalidChannel   = "admin.sales.errors.invalidChannel"
	ErrorEntryNotFound    = "admin.sales.errors.entryNotFound"
)

const (
	defaultDays    = 30
	maxDays        = 366
	topPayersLimit = 20
	// RechargeMethodRecords / RechargeMethodSnapshot 与 user_activity 一致：sub2api 按充值记录逐笔统计，
	// new-api 按「剩余额度 + 累计已用」在两次同步之间的差值统计（无法区分来源、无法追溯首次同步之前）。
	RechargeMethodRecords  = "records"
	RechargeMethodSnapshot = "snapshot"
)

// 入账渠道（与 user_activity_recharge_daily.channel 一致）。
const (
	ChannelPayment     = "payment"
	ChannelRedeem      = "redeem"
	ChannelAdminAdd    = "admin_add"
	ChannelAdminDeduct = "admin_deduct"
	ChannelAffiliate   = "affiliate"
	ChannelCredit      = "credit"
	ChannelDebit       = "debit"
)

// countableChannels 是可以选择不计入的渠道（正向入账）。扣减本来就不计入入账合计。
var countableChannels = []string{ChannelPayment, ChannelRedeem, ChannelAdminAdd, ChannelAffiliate, ChannelCredit}

func countable(channel string) bool {
	for _, known := range countableChannels {
		if known == channel {
			return true
		}
	}
	return false
}

const (
	// entriesPageSize 是入账明细每页多少条。
	entriesPageSize = 50
	// maxEntryKeys 是一次最多排除或恢复多少条明细。
	maxEntryKeys = 200
)

// Workspace 是数据隔离单元（TransitHub 用户 + 当前 admin 工作区）。
type Workspace struct {
	UserID         string
	AdminAccountID string
}

// RangeQuery 是请求的统计区间：要么给出 Days（截至昨天的最近 N 个完整自然日），要么给出 From / To（含两端）。
type RangeQuery struct {
	Days int
	From string
	To   string
}

// DayRange 是含两端的业务日区间（YYYY-MM-DD）。
type DayRange struct {
	From string `json:"from"`
	To   string `json:"to"`
	Days int    `json:"days"`
}

// Coverage 说明本地数据对所选区间的覆盖情况；覆盖不全时前端提示，环比只在上一周期完整覆盖时给出。
type Coverage struct {
	Synced           bool       `json:"synced"`
	Platform         string     `json:"platform"`
	Method           string     `json:"method"`
	RechargeFrom     string     `json:"rechargeFrom,omitempty"`
	RechargeTo       string     `json:"rechargeTo,omitempty"`
	UsageFrom        string     `json:"usageFrom,omitempty"`
	UsageTo          string     `json:"usageTo,omitempty"`
	RechargeComplete bool       `json:"rechargeComplete"`
	UsageComplete    bool       `json:"usageComplete"`
	PreviousComplete bool       `json:"previousComplete"`
	LastSyncedAt     *time.Time `json:"lastSyncedAt"`
}

// PeriodTotals 是一个统计区间的汇总；金额均已按「我的站点售价」换算成人民币。
type PeriodTotals struct {
	// Credits 是入账合计（所有正向入账：在线支付、兑换码、管理员加款、邀请返利转入；new-api 为余额快照的增加）。
	Credits    float64 `json:"credits"`
	Deductions float64 `json:"deductions"`
	Events     int     `json:"events"`
	Payers     int     `json:"payers"`
	// NewPayers 是区间内首次出现入账记录的用户数；只有按充值记录统计、且区间之前已有数据时才可靠，否则为 nil。
	NewPayers    *int `json:"newPayers"`
	RepeatPayers int  `json:"repeatPayers"`
	// Consumption 是用户在区间内的消耗（站点营收口径）；用量数据未覆盖时为 nil。
	Consumption    *float64 `json:"consumption"`
	Registered     int      `json:"registered"`
	RegisteredPaid int      `json:"registeredPaid"`
	// BalanceStart / BalanceEnd 是区间开始前一天、最后一天结束时的全站用户余额（有当天快照时才有值）。
	BalanceStart *float64 `json:"balanceStart"`
	BalanceEnd   *float64 `json:"balanceEnd"`
}

// ChannelTotal 是一个入账渠道在区间内的合计（金额带符号，已减去不计入的明细）。
type ChannelTotal struct {
	Channel string  `json:"channel"`
	Amount  float64 `json:"amount"`
	Events  int     `json:"events"`
	Users   int     `json:"users"`
	// Excluded：整个渠道都不计入（Amount 是重新计入时会加上的金额）。
	Excluded bool `json:"excluded,omitempty"`
	// ExcludedAmount / ExcludedEntries 是这个渠道里选择不计入的明细（金额、条数）。
	ExcludedAmount  float64 `json:"excludedAmount,omitempty"`
	ExcludedEntries int     `json:"excludedEntries,omitempty"`
}

// ExclusionSummary 说明区间内有多少入账没有计入（上面的数字已经去掉了这些）。
type ExclusionSummary struct {
	// Channels 是整个不计入的渠道。
	Channels []string `json:"channels"`
	// Entries 是计入的渠道里单独排除的明细条数。
	Entries int `json:"entries"`
	// Amount 是没有计入的金额合计（单独排除的明细加上整个不计入的渠道）。
	Amount float64 `json:"amount"`
}

// EntryKey 指向一条入账明细（「用户 × 天 × 渠道」）。
type EntryKey struct {
	SiteUserID string `json:"siteUserId"`
	Day        string `json:"day"`
	Channel    string `json:"channel"`
	day        time.Time
}

// EntryQuery 是入账明细的查询条件。
type EntryQuery struct {
	From         string
	To           string
	Channel      string
	Search       string
	ExcludedOnly bool
	Page         int
}

// EntryRow 是一条入账明细（金额已换算成人民币）。
type EntryRow struct {
	SiteUserID string  `json:"siteUserId"`
	Email      string  `json:"email"`
	Username   string  `json:"username"`
	Day        string  `json:"day"`
	Channel    string  `json:"channel"`
	Amount     float64 `json:"amount"`
	Events     int     `json:"events"`
	// ExcludedAmount / ExcludedEvents 是不计入的部分；整条都不计入时等于 Amount / Events，0 表示计入。
	ExcludedAmount float64 `json:"excludedAmount"`
	ExcludedEvents int     `json:"excludedEvents"`
}

// EntriesResponse 是入账明细的一页。
type EntriesResponse struct {
	Items    []EntryRow `json:"items"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	// ExcludedChannels 是整个不计入的渠道（这些渠道的明细不管排不排除都不计入）。
	ExcludedChannels []string `json:"excludedChannels"`
}

// DailyPoint 是每日序列；数据未覆盖的日期对应字段为 nil（图表断开，而不是显示为 0）。
type DailyPoint struct {
	Day         string             `json:"day"`
	Credits     *float64           `json:"credits"`
	Deductions  *float64           `json:"deductions"`
	Channels    map[string]float64 `json:"channels"`
	Consumption *float64           `json:"consumption"`
}

// Concentration 是收入集中度：前 1 / 前 10 名入账用户的金额占比。
type Concentration struct {
	Top1Share  float64 `json:"top1Share"`
	Top10Share float64 `json:"top10Share"`
}

// DistributionBucket 是按区间内人均入账金额（人民币）划分的人数分布，Max 为 nil 表示无上限。
type DistributionBucket struct {
	Min    float64  `json:"min"`
	Max    *float64 `json:"max"`
	Users  int      `json:"users"`
	Amount float64  `json:"amount"`
}

// PayerRow 是入账排行中的一行。
type PayerRow struct {
	SiteUserID      string         `json:"siteUserId"`
	Email           string         `json:"email"`
	Username        string         `json:"username"`
	Status          string         `json:"status"`
	Credits         float64        `json:"credits"`
	Events          int            `json:"events"`
	ActiveDays      int            `json:"activeDays"`
	LastCreditDay   string         `json:"lastCreditDay"`
	Consumption     float64        `json:"consumption"`
	Balance         *float64       `json:"balance"`
	ChannelBreakout []ChannelTotal `json:"channels"`
}

// SummaryResponse 是销售分析页的全部数据。
type SummaryResponse struct {
	Range         DayRange             `json:"range"`
	PreviousRange DayRange             `json:"previousRange"`
	UnitPrice     float64              `json:"unitPrice"`
	Coverage      Coverage             `json:"coverage"`
	Current       PeriodTotals         `json:"current"`
	Previous      *PeriodTotals        `json:"previous"`
	Channels      []ChannelTotal       `json:"channels"`
	Daily         []DailyPoint         `json:"daily"`
	Concentration Concentration        `json:"concentration"`
	Distribution  []DistributionBucket `json:"distribution"`
	TopPayers     []PayerRow           `json:"topPayers"`
	Exclusions    ExclusionSummary     `json:"exclusions"`
}

type requestError string

func (e requestError) Error() string { return string(e) }
