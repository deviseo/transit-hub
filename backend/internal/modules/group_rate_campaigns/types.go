package group_rate_campaigns

import "time"

// 活动状态机：draft 可编辑/删除；scheduled 等待调度器开始；running 运行中，只能手动结束；
// ending 正在恢复原倍率（调度器/手动结束的中间态）；ended/partial/failed/cancelled 均为终态，不再被调度器处理。
const (
	StatusDraft     = "draft"
	StatusScheduled = "scheduled"
	StatusRunning   = "running"
	StatusEnding    = "ending"
	StatusEnded     = "ended"
	StatusPartial   = "partial"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
)

// SelectionMode 决定活动目标分组的圈选方式。
type SelectionMode string

const (
	SelectionAll           SelectionMode = "all"
	SelectionType          SelectionMode = "type"
	SelectionManual        SelectionMode = "manual"
	SelectionCurrentFilter SelectionMode = "currentFilter"
)

// AdjustmentMode 决定活动倍率相对原倍率的计算方式。
type AdjustmentMode string

const (
	AdjustmentSet      AdjustmentMode = "set"
	AdjustmentMultiply AdjustmentMode = "multiply"
	AdjustmentAdd      AdjustmentMode = "add"
)

type StartMode string

const (
	StartNow       StartMode = "now"
	StartScheduled StartMode = "scheduled"
	StartDraft     StartMode = "draft"
)

type EndMode string

const (
	EndScheduled EndMode = "scheduled"
	EndManual    EndMode = "manual"
)

// 分组开启/恢复的执行状态，落在 group_rate_campaign_items 表的 apply_status/restore_status 列。
const (
	ItemPending = "pending"
	ItemApplied = "applied"
	ItemFailed  = "failed"
	// ItemRestored 与 ItemUnchanged 仅用于 restore_status；unchanged 表示活动开始前后倍率相同，
	// 恢复阶段直接跳过真实调用，但仍标记为已处理，避免调度器重复尝试。
	ItemRestored  = "restored"
	ItemUnchanged = "unchanged"
)

// requestError 是本模块的轻量错误类型，Error() 直接返回 i18n key，供 handler 透传给前端。
type requestError string

func (e requestError) Error() string { return string(e) }

// 校验/领域错误的 i18n key，全部落在 admin.groupRateCampaigns.errors 命名空间下。
const (
	ErrEmptySelection    = requestError("admin.groupRateCampaigns.errors.emptySelection")
	ErrNoNotifyBots      = requestError("admin.groupRateCampaigns.errors.noNotifyBots")
	ErrInvalidName       = requestError("admin.groupRateCampaigns.errors.invalidName")
	ErrInvalidAdjustment = requestError("admin.groupRateCampaigns.errors.invalidAdjustment")
	ErrInvalidSchedule   = requestError("admin.groupRateCampaigns.errors.invalidSchedule")
	ErrNotFound          = requestError("admin.groupRateCampaigns.errors.notFound")
	ErrInvalidState      = requestError("admin.groupRateCampaigns.errors.invalidState")
	ErrNoCurrentAccount  = requestError("admin.adminAccounts.errors.noCurrentAccount")
	ErrDuplicateGroup    = requestError("admin.groupRateCampaigns.errors.duplicateGroup")
)

// SelectionGroupRef 手动选择模式下的一个目标分组引用。
// CampaignMultiplier 是该分组在活动期间的固定活动倍率：新建请求（手动选择+固定倍率）必填，
// 历史活动（all/type/currentFilter 选择方式）读取时可能为空，只做只读展示。
type SelectionGroupRef struct {
	GroupName          string   `json:"groupName"`
	CampaignMultiplier *float64 `json:"campaignMultiplier,omitempty"`
}

// SelectionFilter 复用分组倍率页面的筛选条件（currentFilter 模式）。
type SelectionFilter struct {
	Search   string `json:"search"`
	Type     string `json:"type"`
	Platform string `json:"platform"`
}

// Selection 描述活动的目标分组范围，整体以 JSONB 存入 group_rate_campaigns.selection。
type Selection struct {
	Mode   SelectionMode       `json:"mode"`
	Types  []string            `json:"types"`
	Groups []SelectionGroupRef `json:"groups"`
	Filter SelectionFilter     `json:"filter"`
}

// Adjustment 描述活动倍率相对原倍率的计算方式，整体以 JSONB 存入 group_rate_campaigns.adjustment。
type Adjustment struct {
	Mode  AdjustmentMode `json:"mode"`
	Value float64        `json:"value"`
}

// Notify 描述活动的通知配置，整体以 JSONB 存入 group_rate_campaigns.notify。
type Notify struct {
	Enabled       bool     `json:"enabled"`
	BotIDs        []string `json:"botIds"`
	StartTemplate string   `json:"startTemplate"`
	EndTemplate   string   `json:"endTemplate"`
}

// Schedule 描述活动的开始/结束方式，仅用于请求 DTO，落库时拆分进 campaign 的独立列。
type Schedule struct {
	StartMode StartMode  `json:"startMode"`
	StartAt   *time.Time `json:"startAt"`
	EndMode   EndMode    `json:"endMode"`
	EndAt     *time.Time `json:"endAt"`
}

// CreateCampaignRequest 是创建活动的请求体，字段对齐规划文档中的 JSON 示例。
type CreateCampaignRequest struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Selection   Selection  `json:"selection"`
	Adjustment  Adjustment `json:"adjustment"`
	Schedule    Schedule   `json:"schedule"`
	Notify      Notify     `json:"notify"`
}

// Campaign 是活动的领域模型，对应 group_rate_campaigns 表的一行。
type Campaign struct {
	ID             string
	UserID         string
	AdminAccountID string
	Name           string
	Description    string
	Status         string
	Selection      Selection
	Adjustment     Adjustment
	Notify         Notify
	StartMode      StartMode
	StartAt        *time.Time
	EndMode        EndMode
	EndAt          *time.Time
	StartedAt      *time.Time
	EndedAt        *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// CampaignItem 是活动下单个目标分组的执行记录，对应 group_rate_campaign_items 表的一行。
type CampaignItem struct {
	ID                 string
	CampaignID         string
	UserID             string
	AdminAccountID     string
	GroupID            string
	GroupName          string
	OriginalMultiplier *float64
	CampaignMultiplier float64
	RestoredMultiplier *float64
	ApplyStatus        string
	RestoreStatus      string
	ApplyReason        string
	RestoreReason      string
	AppliedAt          *time.Time
	RestoredAt         *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// PreviewItem 是预览接口返回的单个受影响分组。预览阶段的 original/restored 恒等
// （活动尚未开始，恢复倍率即当前倍率），仅 campaignMultiplier 反映调价方式计算结果。
type PreviewItem struct {
	GroupID            string  `json:"groupId"`
	GroupName          string  `json:"groupName"`
	OriginalMultiplier float64 `json:"originalMultiplier"`
	CampaignMultiplier float64 `json:"campaignMultiplier"`
	RestoredMultiplier float64 `json:"restoredMultiplier"`
}

// PreviewResponse 是预览接口的响应体：只读计算，不落库、不改远端、不发通知。
type PreviewResponse struct {
	Items []PreviewItem `json:"items"`
	Total int           `json:"total"`
}

// Summary 汇总活动的目标/执行结果计数，用于列表和详情展示。
type Summary struct {
	Total         int `json:"total"`
	Applied       int `json:"applied"`
	ApplyFailed   int `json:"applyFailed"`
	Restored      int `json:"restored"`
	RestoreFailed int `json:"restoreFailed"`
}

// CampaignListItem 是活动列表接口的单行响应。
type CampaignListItem struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	Status         string     `json:"status"`
	StartMode      StartMode  `json:"startMode"`
	StartAt        *time.Time `json:"startAt"`
	EndMode        EndMode    `json:"endMode"`
	EndAt          *time.Time `json:"endAt"`
	StartedAt      *time.Time `json:"startedAt"`
	EndedAt        *time.Time `json:"endedAt"`
	Summary        Summary    `json:"summary"`
	NotifyEnabled  bool       `json:"notifyEnabled"`
	CreatedBy      string     `json:"createdBy"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	LastExecutedAt *time.Time `json:"lastExecutedAt"`
}

// NotifyDefaults 是环境变量提供的通知默认值，随列表接口下发，供前端创建活动时预填表单。
type NotifyDefaults struct {
	Enabled       bool     `json:"enabled"`
	BotIDs        []string `json:"botIds"`
	StartTemplate string   `json:"startTemplate"`
	EndTemplate   string   `json:"endTemplate"`
}

// ListResult 是活动列表接口的分页响应。
type ListResult struct {
	Items      []CampaignListItem `json:"items"`
	Total      int                `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"pageSize"`
	TotalPages int                `json:"totalPages"`
	Defaults   NotifyDefaults     `json:"defaults"`
}

// ListQuery 描述活动列表的分页与状态筛选控制。
type ListQuery struct {
	Page     int
	PageSize int
	Status   string
}

// CampaignItemView 是活动详情接口中单个分组明细的响应形状。
type CampaignItemView struct {
	GroupID            string     `json:"groupId"`
	GroupName          string     `json:"groupName"`
	OriginalMultiplier *float64   `json:"originalMultiplier"`
	CampaignMultiplier float64    `json:"campaignMultiplier"`
	RestoredMultiplier *float64   `json:"restoredMultiplier"`
	ApplyStatus        string     `json:"applyStatus"`
	RestoreStatus      string     `json:"restoreStatus"`
	ApplyReason        string     `json:"applyReason"`
	RestoreReason      string     `json:"restoreReason"`
	AppliedAt          *time.Time `json:"appliedAt"`
	RestoredAt         *time.Time `json:"restoredAt"`
}

// CampaignDetail 是活动详情接口的响应体：活动配置快照 + 每个分组的执行明细。
type CampaignDetail struct {
	CampaignListItem
	Description string             `json:"description"`
	Selection   Selection          `json:"selection"`
	Adjustment  Adjustment         `json:"adjustment"`
	Notify      Notify             `json:"notify"`
	Items       []CampaignItemView `json:"items"`
}
