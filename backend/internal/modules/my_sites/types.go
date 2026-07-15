package my_sites

import (
	"time"

	"transithub/backend/internal/modules/upstream"
)

const (
	ErrorAuthRequired           = "admin.mySites.errors.authRequired"
	ErrorAdminOnly              = "admin.mySites.errors.adminOnly"
	ErrorRequest                = "admin.mySites.errors.request"
	ErrorUnknown                = "admin.mySites.errors.unknown"
	ErrorInvalidAutoPricingConf = "admin.mySites.errors.invalidAutoPricingConfig"
)

// MappingRequest 前端保存映射关系时的请求体，包含自动调价配置字段。
// FixedIncrease / PercentageIncrease / AdjustThresholdPercent 使用指针区分「未传」和「传了 0」。
type MappingRequest struct {
	OwnGroup                  string             `json:"ownGroup"`
	UpstreamTargets           []UpstreamGroupRef `json:"upstreamTargets"`
	EnableAutoPricing         bool               `json:"enableAutoPricing"`
	AutoPricingSource         string             `json:"autoPricingSource"`
	PrimaryUpstreamSiteID     string             `json:"primaryUpstreamSiteId"`
	PrimaryUpstreamGroupName  string             `json:"primaryUpstreamGroupName"`
	AutoPricingStrategy       string             `json:"autoPricingStrategy"`
	FixedIncrease             *float64           `json:"fixedIncrease"`
	PercentageIncrease        *float64           `json:"percentageIncrease"`
	AdjustThresholdPercent    *float64           `json:"adjustThresholdPercent"`
	MinMultiplier             *float64           `json:"minMultiplier"`
	MaxMultiplier             *float64           `json:"maxMultiplier"`
	EnableAutoPricingNotify   bool               `json:"enableAutoPricingNotify"`
	AutoPricingNotifyBotIDs   []string           `json:"autoPricingNotifyBotIds"`
	AutoPricingNotifyTemplate string             `json:"autoPricingNotifyTemplate"`
	// Accepted for response round-trips only. SaveMappings ignores the client value
	// and restores the latest server-owned status from persistent state.
	LastAutoPricingRun *AutoPricingRunStatus `json:"lastAutoPricingRun,omitempty"`
}

// UpstreamGroupRef 上游分组的引用（站点 ID + 分组名）。
type UpstreamGroupRef struct {
	SiteID    string `json:"siteId"`
	GroupName string `json:"groupName"`
}

// State 用户的分组映射持久化状态，存储于 my_site_states 表。
type State struct {
	UserID         string           `json:"-"`
	AdminAccountID string           `json:"-"`
	BaseURL        string           `json:"baseUrl"`
	Email          string           `json:"email"`
	Session        upstream.Session `json:"-"`
	Mappings       []GroupMapping   `json:"mappings"`
	OwnGroups      []GroupOption    `json:"ownGroups"`
}

// GroupMapping 一个自有分组到多个上游分组的映射关系，并可配置该分组的自动调价策略。
// 自动调价配置绑定在自有分组上，计算时根据 AutoPricingSource 从关联上游中取参考倍率。
type GroupMapping struct {
	OwnGroup                  string                `json:"ownGroup"`
	UpstreamTargets           []UpstreamGroupRef    `json:"upstreamTargets"`
	EnableAutoPricing         bool                  `json:"enableAutoPricing"`
	AutoPricingSource         string                `json:"autoPricingSource"`
	PrimaryUpstreamSiteID     string                `json:"primaryUpstreamSiteId"`
	PrimaryUpstreamGroupName  string                `json:"primaryUpstreamGroupName"`
	AutoPricingStrategy       string                `json:"autoPricingStrategy"`
	FixedIncrease             float64               `json:"fixedIncrease"`
	PercentageIncrease        float64               `json:"percentageIncrease"`
	AdjustThresholdPercent    float64               `json:"adjustThresholdPercent"`
	MinMultiplier             *float64              `json:"minMultiplier"`
	MaxMultiplier             *float64              `json:"maxMultiplier"`
	EnableAutoPricingNotify   bool                  `json:"enableAutoPricingNotify"`
	AutoPricingNotifyBotIDs   []string              `json:"autoPricingNotifyBotIds"`
	AutoPricingNotifyTemplate string                `json:"autoPricingNotifyTemplate"`
	LastAutoPricingRun        *AutoPricingRunStatus `json:"lastAutoPricingRun,omitempty"`
}

// AutoPricingRunStatus 是后端写入的最近一次自动调价执行状态。
// Status、Reason 和 Trigger 均为稳定机器可读值；错误场景只记录安全原因码，不暴露远端响应或凭据。
type AutoPricingRunStatus struct {
	Status           string    `json:"status"`
	Reason           string    `json:"reason,omitempty"`
	Trigger          string    `json:"trigger"`
	RanAt            time.Time `json:"ranAt"`
	OldReference     *float64  `json:"oldReference"`
	NewReference     *float64  `json:"newReference"`
	OldOwnMultiplier *float64  `json:"oldOwnMultiplier"`
	NewOwnMultiplier *float64  `json:"newOwnMultiplier"`
	TargetMultiplier *float64  `json:"targetMultiplier"`
}

// AutoPricingRunRequest 手动触发自动调价请求体。
type AutoPricingRunRequest struct {
	OwnGroup string `json:"ownGroup"`
}

// AutoPricingRunResponse 手动触发自动调价的响应体，保持原有 mapping 字段不变并追加结构化结果。
type AutoPricingRunResponse struct {
	Result  AutoPricingRunStatus `json:"result"`
	Mapping GroupMapping         `json:"mapping"`
}

// StatusResponse 保存映射后返回的状态。
type StatusResponse struct {
	Authenticated bool           `json:"authenticated"`
	BaseURL       string         `json:"baseUrl"`
	Email         string         `json:"email"`
	Mappings      []GroupMapping `json:"mappings"`
}

// MappingOptionsResponse mapping-options 接口的响应体。
type MappingOptionsResponse struct {
	OwnGroups []MappingOwnGroupOption `json:"ownGroups"`
	Mappings  []GroupMapping          `json:"mappings"`
}

// MappingOwnGroupOption 自有分组选项，包含 ID、平台、状态、专属性等属性。
type MappingOwnGroupOption struct {
	ID               string  `json:"id"`
	SiteName         string  `json:"siteName"`
	GroupName        string  `json:"groupName"`
	Multiplier       float64 `json:"multiplier"`
	Platform         string  `json:"platform"`
	Status           string  `json:"status"`
	IsExclusive      bool    `json:"isExclusive"`
	SubscriptionType string  `json:"subscriptionType"`
}

// GroupOption 自有分组的名称与倍率，缓存于 State.OwnGroups。
type GroupOption struct {
	Name       string  `json:"name"`
	Multiplier float64 `json:"multiplier"`
}

// RealConnectRequest 真实对接请求体。
// 前端传入上游站点 ID、上游分组 ID/名称和自有分组 ID 列表。
// GroupType 可选：为空时后端从上游站点缓存的分组列表中自动识别平台类型。
type RealConnectRequest struct {
	UpstreamSiteID    string   `json:"upstreamSiteId"`
	UpstreamGroupID   string   `json:"upstreamGroupId"`
	UpstreamGroupName string   `json:"upstreamGroupName"`
	GroupType         string   `json:"groupType"`
	ChannelType       int      `json:"channelType"`
	OwnGroupIDs       []string `json:"ownGroupIds"`
}

// RealDisconnectRequest 取消真实对接请求体。
// Mode: "unlink" 仅删除本地绑定记录，"full" 同时删除上游 Key 和 admin 转发账号。
type RealDisconnectRequest struct {
	ConnectionID string `json:"connectionId"`
	Mode         string `json:"mode"`
}

// RealBindRequest 手动绑定请求体。
// 用户从上游 key 列表中选择要绑定的 key，此接口仅创建绑定记录，不调用任何 platform API。
type RealBindRequest struct {
	UpstreamSiteID    string   `json:"upstreamSiteId"`
	UpstreamGroupID   string   `json:"upstreamGroupId"`
	UpstreamGroupName string   `json:"upstreamGroupName"`
	UpstreamKeyID     string   `json:"upstreamKeyId"`
	UpstreamKey       string   `json:"upstreamKey"`
	OwnGroupIDs       []string `json:"ownGroupIds"`
	GroupType         string   `json:"groupType"`
}

// RealConnectResponse 真实对接成功后返回的绑定记录。
type RealConnectResponse struct {
	Connection RealConnection `json:"connection"`
}

// RealConnection 一条真实对接的绑定记录，存储于 real_connections 表。
// 记录上游 key、admin 账号、关联的自有分组 ID 列表等完整信息。
//
// 注意：此结构体有两个含义不同的 admin account 字段，不要混淆：
//   - WorkspaceAdminAccountID: TransitHub 工作区归属字段（对应 admin_accounts 表），
//     用于 workspace 数据隔离，标识这条绑定记录属于哪个 admin workspace。
//     数据库列名为 workspace_admin_account_id，与其他业务表的 admin_account_id 语义相同。
//   - AdminAccountID: 上游平台的 admin 转发账号 ID，是真实对接业务逻辑中的字段，
//     表示在上游 sub2api/new-api 站点上为 key 创建或绑定的管理员账号。
type RealConnection struct {
	ID                      string   `json:"id"`
	UserID                  string   `json:"-"`
	WorkspaceAdminAccountID string   `json:"-"` // TransitHub workspace 归属（隔离字段）
	UpstreamSiteID          string   `json:"upstreamSiteId"`
	UpstreamGroupID         string   `json:"upstreamGroupId"`
	UpstreamGroupName       string   `json:"upstreamGroupName"`
	UpstreamKeyID           string   `json:"upstreamKeyId"`
	UpstreamKey             string   `json:"upstreamKey"`
	AdminAccountID          string   `json:"adminAccountId"` // 上游平台 admin 转发账号 ID（业务字段）
	AdminAccountName        string   `json:"adminAccountName"`
	OwnGroupIDs             []string `json:"ownGroupIds"`
	GroupType               string   `json:"groupType"`
	CreatedAt               string   `json:"createdAt"`
}
