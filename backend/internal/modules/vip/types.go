package vip

import "time"

// 错误 key 直接作为 i18n key 返回给前端。
const (
	ErrorNoCurrentAccount = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorRequest          = "admin.vip.errors.request"
	ErrorInvalidUser      = "admin.vip.errors.invalidUser"
	ErrorUserNotFound     = "admin.vip.errors.userNotFound"
	ErrorInvalidSettings  = "admin.vip.errors.invalidSettings"
)

// 大户的状态。优先级：停用 > 用完了 > 快用完 > 用量下降 > 正常。
const (
	StatusNormal    = "normal"
	StatusLow       = "low"
	StatusEmpty     = "empty"
	StatusDeclining = "declining"
	StatusDisabled  = "disabled"
)

// 关注名单里的记录类型。
const (
	kindManual    = "manual"
	kindDismissed = "dismissed"
)

const (
	// activeDays：最近这么多天用过才算「在用」，快用完、用完了才提醒（不在用的不打扰）。
	activeDays = 7
	// decliningRatio：近 7 天日均不到前 7 天的这个比例，算用量下降。
	decliningRatio = 0.5
	// decliningMinimum：前 7 天日均至少这么多（人民币）才判断下降，避免很小的用量波动。
	decliningMinimum = 1.0
	// searchLimit：添加关注时搜索结果最多几条。
	searchLimit      = 20
	maxNoteRunes     = 200
	maxReminderRunes = 500
	maxBots          = 20
)

// Workspace 是数据隔离单元（TransitHub 用户 + 当前 admin 工作区）。
type Workspace struct {
	UserID         string
	AdminAccountID string
}

// Settings 是大户关注的设置。金额是人民币（和页面显示一致）。
type Settings struct {
	// DailyTop / LifetimeTop：推荐近 7 天日均消耗、累计消耗前几名（0 表示不按这一项推荐）。
	DailyTop    int `json:"dailyTop"`
	LifetimeTop int `json:"lifetimeTop"`
	// AlertDays / AlertBalance：预计几天内用完、或者余额低于多少元算快用完（0 表示不按这一项判断）。
	AlertDays    float64 `json:"alertDays"`
	AlertBalance float64 `json:"alertBalance"`
	// AlertsEnabled / BotIDs：快用完、用完了时用这些机器人提醒站长。
	AlertsEnabled bool     `json:"alertsEnabled"`
	BotIDs        []string `json:"botIds"`
	// ReminderText / EmptyReminderText：「复制提醒」的话术，空表示用默认的。
	ReminderText      string `json:"reminderText"`
	EmptyReminderText string `json:"emptyReminderText"`
}

// DefaultSettings 是没有保存过设置时用的值。
func DefaultSettings() Settings {
	return Settings{DailyTop: 20, LifetimeTop: 20, AlertDays: 2, AlertBalance: 20, AlertsEnabled: true, BotIDs: []string{}}
}

// Source 说明这个大户为什么在名单里：手动关注（带备注），或者系统按日均、累计消耗推荐（第几名）。
type Source struct {
	Manual       bool   `json:"manual"`
	Note         string `json:"note"`
	DailyRank    int    `json:"dailyRank,omitempty"`
	LifetimeRank int    `json:"lifetimeRank,omitempty"`
}

// Row 是名单里的一个大户。金额都已按「我的站点售价」换算成人民币。
type Row struct {
	SiteUserID string `json:"siteUserId"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	Status     string `json:"status"`
	Source     Source `json:"source"`
	// Balance 是最近一次同步时的余额；站点没给时为 nil。
	Balance *float64 `json:"balance"`
	// DailyAverage / PreviousAverage：近 7 天、再往前 7 天（完整自然日）的日均消耗。
	DailyAverage    float64 `json:"dailyAverage"`
	PreviousAverage float64 `json:"previousAverage"`
	// Trend 是近 7 天比前 7 天的变化（0.12 表示多了 12%）；前 7 天没用过时为 nil。
	Trend *float64 `json:"trend"`
	// DaysLeft 是余额 ÷ 近 7 天日均消耗，预计还能用几天；近 7 天没用过、或者没有余额数据时为 nil。
	DaysLeft *float64 `json:"daysLeft"`
	// Lifetime 是累计消耗（new-api 用站点的累计已用，sub2api 用本地保存的每日消耗合计）。
	Lifetime         float64    `json:"lifetime"`
	LastUsedAt       *time.Time `json:"lastUsedAt"`
	LastCreditDay    string     `json:"lastCreditDay,omitempty"`
	LastCreditAmount float64    `json:"lastCreditAmount,omitempty"`
}

// Counts 是各状态的人数（页面上的筛选）。
type Counts struct {
	All       int `json:"all"`
	Low       int `json:"low"`
	Empty     int `json:"empty"`
	Declining int `json:"declining"`
}

// Summary 是大户关注页的全部数据。
type Summary struct {
	Synced       bool       `json:"synced"`
	Platform     string     `json:"platform"`
	LastSyncedAt *time.Time `json:"lastSyncedAt"`
	UnitPrice    float64    `json:"unitPrice"`
	// SiteName 用在「复制提醒」的话术里：站长在客户端页设置的品牌名，没有时用工作区名称。
	SiteName       string   `json:"siteName"`
	Settings       Settings `json:"settings"`
	Counts         Counts   `json:"counts"`
	Items          []Row    `json:"items"`
	DismissedCount int      `json:"dismissedCount"`
}

// Candidate 是添加关注时搜到的用户。
type Candidate struct {
	SiteUserID   string   `json:"siteUserId"`
	Email        string   `json:"email"`
	Username     string   `json:"username"`
	Balance      *float64 `json:"balance"`
	DailyAverage float64  `json:"dailyAverage"`
	// Watched：已经手动关注了；Dismissed：之前移出过（再关注就回来了）。
	Watched   bool `json:"watched"`
	Dismissed bool `json:"dismissed"`
}

type requestError string

func (e requestError) Error() string { return string(e) }
