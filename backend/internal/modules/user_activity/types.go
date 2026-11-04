// Package user_activity 分析当前工作区「我的站点」（sub2api / new-api）用户的活跃情况：
// 定时把站点用户和每天的用量同步到本地，在本地计算多久没用、近期用量变化以及日/周/月环比与同比，
// 并按筛选条件为群发邮件提供收件人。只读取管理员会话下自己站点的数据。
package user_activity

import "time"

// 错误 key（前端 i18n）。
const (
	ErrorAuthRequired     = "admin.userActivity.errors.authRequired"
	ErrorPlatform         = "admin.userActivity.errors.platformUnsupported"
	ErrorSyncInProgress   = "admin.userActivity.errors.syncInProgress"
	ErrorUsersFetch       = "admin.userActivity.errors.usersFetch"
	ErrorUsageFetch       = "admin.userActivity.errors.usageFetch"
	ErrorInterrupted      = "admin.userActivity.errors.interrupted"
	ErrorRequest          = "admin.userActivity.errors.request"
	ErrorInvalidFilter    = "admin.userActivity.errors.invalidFilter"
	ErrorNotFound         = "admin.userActivity.errors.notFound"
	ErrorTooManyUsers     = "admin.userActivity.errors.tooManyUsers"
	ErrorTooManyRecipient = "admin.userActivity.errors.tooManyRecipients"
	ErrorNotSynced        = "admin.userActivity.errors.notSynced"
	// 不影响同步结果的警告（入账数据）。
	WarningRechargeFetch       = "admin.userActivity.warnings.rechargeFetch"
	WarningRechargeUnsupported = "admin.userActivity.warnings.rechargeUnsupported"
	WarningRechargeTruncated   = "admin.userActivity.warnings.rechargeTruncated"
	ErrorNoCurrentAccount      = "admin.adminAccounts.errors.noCurrentAccount"
	// 上游网络错误沿用上游模块的文案（提示检查站点地址或网络代理）。
	ErrorNetwork = "admin.upstream.errors.network"
)

// 同步状态。
const (
	SyncStatusRunning = "running"
	SyncStatusSuccess = "success"
	SyncStatusFailed  = "failed"
)

// 同步是怎么开始的（前端据此决定完成后是否提醒）。
const (
	StartedByManual   = "manual"
	StartedByAuto     = "auto"
	StartedByResume   = "resume"
	StartedBySchedule = "schedule"
)

// 用户分群（筛选预设）。
const (
	SegmentAll         = "all"
	SegmentActive1d    = "active_1d"
	SegmentActive7d    = "active_7d"
	SegmentActive30d   = "active_30d"
	SegmentInactive2d  = "inactive_2d"
	SegmentInactive7d  = "inactive_7d"
	SegmentInactive14d = "inactive_14d"
	SegmentInactive30d = "inactive_30d"
	SegmentInactive60d = "inactive_60d"
	SegmentInactive90d = "inactive_90d"
	SegmentNoUsage     = "no_usage"
	SegmentChurned7d   = "churned_7d" // 前 7 天有使用、近 7 天没有使用
	SegmentDeclining   = "declining"  // 近 7 天消费不到前 7 天的一半
	SegmentGrowing     = "growing"    // 近 7 天消费达到前 7 天的 1.5 倍
	SegmentInactive    = "inactive"   // 自定义天数，配合 InactiveDays
	// SegmentRechargedToday 今天有入账（充值、兑换、管理员加款等）的用户。
	SegmentRechargedToday = "recharged_today"
	// SegmentLowBalance 近 7 天用过、余额不足 1 美元的用户（适合发余额提醒）。
	SegmentLowBalance = "low_balance"
)

// 入账渠道（user_activity_recharge_daily.channel），金额为美元口径、带正负号。
const (
	ChannelPayment     = "payment"      // sub2api 在线充值
	ChannelRedeem      = "redeem"       // sub2api 兑换码
	ChannelAdminAdd    = "admin_add"    // sub2api 管理员加款
	ChannelAdminDeduct = "admin_deduct" // sub2api 管理员扣款（负数）
	ChannelAffiliate   = "affiliate"    // sub2api 邀请返利转入余额
	ChannelCredit      = "credit"       // new-api 两次同步之间「剩余额度 + 累计已用」的增加
	ChannelDebit       = "debit"        // new-api 两次同步之间「剩余额度 + 累计已用」的减少（负数）
)

// 入账统计方式。
const (
	RechargeMethodRecords  = "records"  // sub2api：按站点充值记录，可区分渠道、可回补历史
	RechargeMethodSnapshot = "snapshot" // new-api：按余额快照差值，从首次同步开始统计
)

const (
	lowBalanceThreshold  = 1.0
	lowBalanceActiveDays = 7
)

// 统一后的用户状态与角色（sub2api 与 new-api 的取值不同，同步时归一）。
const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	RoleUser       = "user"
	RoleAdmin      = "admin"
	RoleRoot       = "root"
)

// 用户列表排序字段。
const (
	SortLastUsedAt     = "lastUsedAt"
	SortCostToday      = "costToday"
	SortCost7d         = "cost7d"
	SortCost30d        = "cost30d"
	SortRequests7d     = "requests7d"
	SortChange7d       = "change7d"
	SortActiveDays30d  = "activeDays30d"
	SortBalance        = "balance"
	SortTotalRecharged = "totalRecharged"
	SortTotalUsed      = "totalUsed"
	SortCreatedAt      = "createdAt"
	SortRechargeToday  = "rechargeToday"
	SortRecharge30d    = "recharge30d"
)

const (
	maxInactiveDays = 3650
	maxSearchRunes  = 100
)

// Workspace 标识一个 TransitHub 用户的一个 admin 工作区。
type Workspace struct {
	UserID         string
	AdminAccountID string
}

// Profile 是本地保存的站点用户快照。
type Profile struct {
	SiteUserID     string
	Email          string
	Username       string
	Role           string
	Status         string
	Group          string
	Balance        *float64
	TotalRecharged *float64
	TotalUsed      *float64
	// FundedTotal 是 new-api 的「剩余额度 + 累计已用」（美元口径），用于按快照差值统计入账；sub2api 为 nil。
	FundedTotal   *float64
	EverUsed      bool
	SiteCreatedAt *time.Time
	LastUsedAt    *time.Time
}

// RechargeDaily 是某用户某天某渠道的入账合计。
type RechargeDaily struct {
	SiteUserID string
	Day        time.Time
	Channel    string
	Amount     float64
	Events     int
}

// SiteDaily 是全站某天的快照（当天最后一次同步时的数值）。
type SiteDaily struct {
	Day              time.Time
	TotalBalance     float64
	UsersWithBalance int
	UserCount        int
}

// DailyUsage 是某用户在某个自然日（业务时区）的用量。Day 以 UTC 零点表示该自然日。
type DailyUsage struct {
	SiteUserID string
	Day        time.Time
	Requests   int64
	Tokens     int64
	Cost       float64
}

// SyncState 是工作区最近一次同步的状态。SyncedFrom ~ SyncedTo 是本地已连续拉取过用量的日期区间。
type SyncState struct {
	Platform      string
	Status        string
	ErrorKey      string
	SyncedFrom    *time.Time
	SyncedTo      *time.Time
	UserCount     int
	LastStartedAt *time.Time
	LastSyncedAt  *time.Time
	// RechargeSyncedFrom ~ RechargeSyncedTo 是本地入账数据连续覆盖的日期；RechargeSince 是 new-api 开始按快照统计的时间。
	RechargeSyncedFrom *time.Time
	RechargeSyncedTo   *time.Time
	RechargeSince      *time.Time
	WarningKey         string
	StartedBy          string
}

// SyncStatus 是返回给前端的同步状态。
type SyncStatus struct {
	Platform      string     `json:"platform"`
	Status        string     `json:"status"`
	ErrorKey      string     `json:"errorKey,omitempty"`
	SyncedFrom    string     `json:"syncedFrom,omitempty"`
	SyncedTo      string     `json:"syncedTo,omitempty"`
	UserCount     int        `json:"userCount"`
	LastStartedAt *time.Time `json:"lastStartedAt,omitempty"`
	LastSyncedAt  *time.Time `json:"lastSyncedAt,omitempty"`
	// HistoryLimitDays 是该平台最多能回补的天数（sub2api 使用记录默认保留 90 天）。
	HistoryLimitDays int    `json:"historyLimitDays"`
	WarningKey       string `json:"warningKey,omitempty"`
	StartedBy        string `json:"startedBy,omitempty"`
}

// PeriodMetrics 是某个日期区间（含两端）的汇总。
type PeriodMetrics struct {
	From        string  `json:"from"`
	To          string  `json:"to"`
	ActiveUsers int     `json:"activeUsers"`
	NewUsers    int     `json:"newUsers"`
	Requests    int64   `json:"requests"`
	Tokens      int64   `json:"tokens"`
	Cost        float64 `json:"cost"`
}

// Retention 对比本期与上期的活跃用户。
type Retention struct {
	Retained int `json:"retained"` // 两期都活跃
	Churned  int `json:"churned"`  // 上期活跃、本期未活跃
	Gained   int `json:"gained"`   // 本期活跃、上期未活跃
}

// PeriodComparison 是「本期 / 上期 / 去年同期」三组数据，前端据此计算环比与同比。
// 日：昨天 vs 前天；周：近 7 天 vs 前 7 天；月：近 30 天 vs 前 30 天（均为完整自然日，截止到昨天）。
// Previous / LastYear 为 nil 表示本地还没有覆盖该区间的数据。
type PeriodComparison struct {
	Days      int            `json:"days"`
	Current   PeriodMetrics  `json:"current"`
	Previous  *PeriodMetrics `json:"previous"`
	LastYear  *PeriodMetrics `json:"lastYear"`
	Retention *Retention     `json:"retention,omitempty"`
}

// InactiveBucket 是「超过 N 天未使用」的人数。
type InactiveBucket struct {
	Days  int `json:"days"`
	Count int `json:"count"`
}

// DailyPoint 是每天的活跃人数、用量与入账。Recharge 为 nil 表示本地入账数据没有覆盖这一天。
type DailyPoint struct {
	Day         string   `json:"day"`
	ActiveUsers int      `json:"activeUsers"`
	Requests    int64    `json:"requests"`
	Tokens      int64    `json:"tokens"`
	Cost        float64  `json:"cost"`
	Recharge    *float64 `json:"recharge"`
}

// ChannelAmount 是某个入账渠道的合计。
type ChannelAmount struct {
	Channel string  `json:"channel"`
	Amount  float64 `json:"amount"`
	Events  int     `json:"events"`
	Users   int     `json:"users"`
}

// MoneySummary 是资金概览：今日入账（按渠道）、今日消耗、全站用户余额等，金额为美元口径。
// 入账 = 正向渠道之和；扣减 = 负向渠道（管理员扣款等）之和的绝对值。
type MoneySummary struct {
	Method             string          `json:"method"`
	RechargeSince      *time.Time      `json:"rechargeSince,omitempty"`
	RechargeFrom       string          `json:"rechargeFrom,omitempty"`
	RechargeUpToDate   bool            `json:"rechargeUpToDate"`
	TodayRecharge      float64         `json:"todayRecharge"`
	TodayDeduction     float64         `json:"todayDeduction"`
	TodayRechargeUsers int             `json:"todayRechargeUsers"`
	TodayChannels      []ChannelAmount `json:"todayChannels"`
	YesterdayRecharge  *float64        `json:"yesterdayRecharge"`
	TodayCost          float64         `json:"todayCost"`
	YesterdayCost      *float64        `json:"yesterdayCost"`
	TotalBalance       float64         `json:"totalBalance"`
	UsersWithBalance   int             `json:"usersWithBalance"`
	BalanceYesterday   *float64        `json:"balanceYesterday"`
	LifetimeRecharged  *float64        `json:"lifetimeRecharged"`
	LowBalanceActive   int             `json:"lowBalanceActive"`
}

// SummaryResponse 是 GET /api/user-activity/summary 的响应。
// NoUsage / Inactive 不含已禁用用户。
type SummaryResponse struct {
	Sync         SyncStatus                  `json:"sync"`
	HasData      bool                        `json:"hasData"`
	TotalUsers   int                         `json:"totalUsers"`
	Disabled     int                         `json:"disabled"`
	WithEmail    int                         `json:"withEmail"`
	NoUsage      int                         `json:"noUsage"`
	Today        PeriodMetrics               `json:"today"`
	Money        MoneySummary                `json:"money"`
	Inactive     []InactiveBucket            `json:"inactive"`
	Periods      map[string]PeriodComparison `json:"periods"`
	Daily        []DailyPoint                `json:"daily"`
	Timezone     string                      `json:"timezone"`
	Suppressions int                         `json:"suppressions"`
}

// UserFilter 是用户列表与群发收件人共用的筛选条件。
type UserFilter struct {
	Segment      string `json:"segment"`
	InactiveDays int    `json:"inactiveDays,omitempty"`
	Search       string `json:"search,omitempty"`
	Status       string `json:"status,omitempty"`
	Role         string `json:"role,omitempty"`
	HasEmail     bool   `json:"hasEmail,omitempty"`
	// SiteUserID 只供内部按 ID 精确查询（用户详情），不接受前端传入。
	SiteUserID string `json:"-"`
}

// UserListQuery 是 GET /api/user-activity/users 的查询参数。
type UserListQuery struct {
	Filter   UserFilter
	Sort     string
	Order    string
	Page     int
	PageSize int
}

// UserRow 是用户列表中的一行。「近 N 天」都是截止到昨天的完整自然日，今天单独列出。
type UserRow struct {
	SiteUserID       string     `json:"siteUserId"`
	Email            string     `json:"email"`
	Username         string     `json:"username"`
	Role             string     `json:"role"`
	Status           string     `json:"status"`
	Group            string     `json:"group"`
	Balance          *float64   `json:"balance"`
	TotalRecharged   *float64   `json:"totalRecharged"`
	TotalUsed        *float64   `json:"totalUsed"`
	EverUsed         bool       `json:"everUsed"`
	CreatedAt        *time.Time `json:"createdAt"`
	LastUsedAt       *time.Time `json:"lastUsedAt"`
	DaysSinceLastUse *int       `json:"daysSinceLastUse"`
	RequestsToday    int64      `json:"requestsToday"`
	CostToday        float64    `json:"costToday"`
	Requests7d       int64      `json:"requests7d"`
	RequestsPrev7d   int64      `json:"requestsPrev7d"`
	Cost7d           float64    `json:"cost7d"`
	CostPrev7d       float64    `json:"costPrev7d"`
	Cost30d          float64    `json:"cost30d"`
	CostPrev30d      float64    `json:"costPrev30d"`
	ActiveDays30d    int        `json:"activeDays30d"`
	RechargeToday    float64    `json:"rechargeToday"`
	Recharge30d      float64    `json:"recharge30d"`
}

// UserListResponse 是用户列表的一页。
type UserListResponse struct {
	Items    []UserRow `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	Pages    int       `json:"pages"`
}

// UserDetailResponse 是单个用户的活跃详情。
type UserDetailResponse struct {
	User    UserRow                     `json:"user"`
	Daily   []DailyPoint                `json:"daily"`
	Periods map[string]PeriodComparison `json:"periods"`
}

// RecipientPreview 是按筛选条件群发前的人数预览。
type RecipientPreview struct {
	Matched    int `json:"matched"`
	WithEmail  int `json:"withEmail"`
	Suppressed int `json:"suppressed"`
	Sendable   int `json:"sendable"`
}

// Recipient 是按筛选条件解析出的邮件收件人。
type Recipient struct {
	SiteUserID string
	Email      string
	Username   string
}

// requestError 让 service 返回可直接作为 i18n key 的业务错误。
type requestError string

func (e requestError) Error() string { return string(e) }
