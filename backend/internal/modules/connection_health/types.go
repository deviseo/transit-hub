package connection_health

import (
	"context"
	"time"

	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

// State 是链路健康状态机的六个状态。disabled 只能人工恢复。
type State string

const (
	StateHealthy    State = "healthy"
	StateDegraded   State = "degraded"
	StateSuspended  State = "suspended"
	StateObserving  State = "observing"
	StateRecovering State = "recovering"
	StateDisabled   State = "disabled"
)

// ResultKey 是真实探活的错误分类，用于状态机决策和事件记录，不含敏感信息。
type ResultKey string

const (
	ResultOK                 ResultKey = "ok"
	ResultNetworkFluctuation ResultKey = "network_fluctuation"
	ResultRateLimited        ResultKey = "rate_limited"
	ResultServerError        ResultKey = "server_error"
	ResultAuth               ResultKey = "auth"
	ResultModelNotFound      ResultKey = "model_not_found"
	ResultInvalidResponse    ResultKey = "invalid_response"
	ResultUnsupported        ResultKey = "unsupported"
)

// ProviderFamily 探活请求的最小形态按此分类选择。
const (
	ProviderGemini    = "gemini"
	ProviderAnthropic = "anthropic"
	ProviderOpenAI    = "openai"
	ProviderCustom    = "custom"
)

const (
	ErrorRequest          = "admin.connectionHealth.errors.request"
	ErrorUnknown          = "admin.connectionHealth.errors.unknown"
	ErrorNotFound         = "admin.connectionHealth.errors.notFound"
	ErrorNoCurrentAccount = "admin.adminAccounts.errors.noCurrentAccount"
	// ErrorNoMatchingModels: 手动探活请求体显式指定了 models，但没有一个模型命中该连接
	// 当前匹配到的启用策略/启用模型目标。与"models 为空时探活全部匹配目标但目标本身为空"
	// 的旧行为（200 + 空数组）区分开，让前端能区分"策略配置问题"和"探活完成但结果为空"。
	ErrorNoMatchingModels = "admin.connectionHealth.errors.noMatchingModels"
	// ErrorAccountsFetch: admin 分组健康主列表中，单个分组的账号/渠道列表拉取失败时挂在
	// AdminGroupHealth.AccountsError 上的安全 i18n key（不含上游错误明文），前端据此展示
	// 该分组「账号列表加载失败」，其余分组不受影响。
	ErrorAccountsFetch = "admin.connectionHealth.errors.accountsFetch"
	// ErrorModelListUnavailable / ErrorModelListInvalid：手动模型发现接口的结构化错误——
	// 上游 /v1/models 不可达/非 2xx，或返回的不是可识别的 OpenAI 兼容结构。
	ErrorModelListUnavailable = "admin.connectionHealth.errors.modelListUnavailable"
	ErrorModelListInvalid     = "admin.connectionHealth.errors.modelListInvalid"
	// ErrorManualModelsRequired：手动一次性探活请求体的 models 为空。
	ErrorManualModelsRequired = "admin.connectionHealth.errors.manualModelsRequired"
	// ErrorPolicyNotFound：分配策略时传入的 policyId 不属于当前 workspace 或不存在。
	ErrorPolicyNotFound = "admin.connectionHealth.errors.policyNotFound"
)

// PolicyAssignment 对应 connection_health_policy_assignments 表：一条「target 显式绑定某条策略」
// 的分配关系。调度器只对已分配 enabled 策略的 target 自动探活，未分配的 target 永不自动探活。
type PolicyAssignment struct {
	ID             string    `json:"id"`
	UserID         string    `json:"-"`
	AdminAccountID string    `json:"-"`
	TargetID       string    `json:"targetId"`
	PolicyID       string    `json:"policyId"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// Policy 对应 connection_health_policies 表：一条健康探活/降级策略，
// 按 own_group_id 匹配对接链路（own_group_id 为空表示匹配该 workspace 下全部已对接分组）。
type Policy struct {
	ID                      string    `json:"id"`
	UserID                  string    `json:"-"`
	AdminAccountID          string    `json:"-"`
	Name                    string    `json:"name"`
	Enabled                 bool      `json:"enabled"`
	OwnGroupID              string    `json:"ownGroupId"`
	OwnGroupName            string    `json:"ownGroupName"`
	ModelPattern            string    `json:"modelPattern"`
	ProbeMode               string    `json:"probeMode"`
	ProbeIntervalSeconds    int       `json:"probeIntervalSeconds"`
	FailureThreshold        int       `json:"failureThreshold"`
	SuccessThreshold        int       `json:"successThreshold"`
	CooldownSeconds         int       `json:"cooldownSeconds"`
	ObservationSeconds      int       `json:"observationSeconds"`
	RecoveryStepPercent     int       `json:"recoveryStepPercent"`
	AutoDegradeEnabled      bool      `json:"autoDegradeEnabled"`
	AutoRemoteActionEnabled bool      `json:"autoRemoteActionEnabled"`
	DailyProbeBudget        int       `json:"dailyProbeBudget"`
	CreatedAt               time.Time `json:"createdAt"`
	UpdatedAt               time.Time `json:"updatedAt"`
	// ModelTargets 不是数据库列，是查询时一并装载的关联目标（connection_health_model_targets）。
	ModelTargets []ModelTarget `json:"modelTargets"`
}

// ModelTarget 对应 connection_health_model_targets 表：策略下具体要探活的模型。
type ModelTarget struct {
	ID             string    `json:"id"`
	PolicyID       string    `json:"policyId"`
	UserID         string    `json:"-"`
	AdminAccountID string    `json:"-"`
	ModelName      string    `json:"modelName"`
	ProviderFamily string    `json:"providerFamily"`
	Enabled        bool      `json:"enabled"`
	ProbePrompt    string    `json:"probePrompt"`
	MaxProbeTokens int       `json:"maxProbeTokens"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// ConnectionHealthState 对应 connection_health_states 表：一条对接链路在某个模型上的当前健康状态。
// model_name 为 "*" 表示链路级（非模型级）状态。
type ConnectionHealthState struct {
	ConnectionID         string
	ModelName            string
	UserID               string
	AdminAccountID       string
	OwnGroupID           string
	OwnGroupName         string
	UpstreamSiteID       string
	UpstreamGroupID      string
	UpstreamGroupName    string
	State                State
	CurrentWeight        int
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	LastProbeAt          *time.Time
	LastSuccessAt        *time.Time
	LastFailureAt        *time.Time
	CooldownUntil        *time.Time
	ObservingUntil       *time.Time
	LastLatencyMs        *int
	LastErrorKey         string
	LastErrorDetail      string
	LastRemoteAction     string
	UpdatedAt            time.Time
}

// ConnectionHealthEvent 对应 connection_health_events 表：探活或远端动作的一条留痕记录。
type ConnectionHealthEvent struct {
	ID                string
	ConnectionID      string
	ModelName         string
	UserID            string
	AdminAccountID    string
	OwnGroupName      string
	UpstreamSiteID    string
	UpstreamGroupName string
	Result            string
	FromState         string
	ToState           string
	LatencyMs         *int
	ErrorKey          string
	ErrorDetail       string
	RemoteAction      string
	CreatedAt         time.Time
}

// ProbeOutcome 是一次真实探活的结果，供状态机和事件记录消费。
type ProbeOutcome struct {
	Result    ResultKey
	LatencyMs int
	Detail    string
}

// MySitesReader 是 connection_health 对 my_sites 模块的全部只读依赖，
// 由 *my_sites.Service 结构性满足。定义为窄接口避免直接耦合 my_sites 内部实现。
type MySitesReader interface {
	ListRealConnections(ctx context.Context, userID string) ([]my_sites.RealConnection, error)
	// ListRealConnectionsForWorkspace 按显式 userID+adminAccountID 读取，不依赖请求态"当前
	// workspace"解析。后台 scheduler 用 context.Background() 启动、没有 HTTP 请求上下文，
	// 必须用这个方法按策略自带的 workspace 读取连接，否则可能读到错误 workspace 或读不到数据。
	ListRealConnectionsForWorkspace(ctx context.Context, userID string, adminAccountID string) ([]my_sites.RealConnection, error)
	MappingOptions(ctx context.Context, userID string) (my_sites.MappingOptionsResponse, error)
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}

// SiteLookup 是 connection_health 对 upstream 模块的只读依赖：按站点 ID 取 base_url 和平台类型。
type SiteLookup interface {
	GetSite(ctx context.Context, siteID string) (*upstream.Site, error)
}

// PlatformGroupReader 是 connection_health 对 upstream.PlatformService 的窄只读依赖：
// 读取当前 admin workspace 下的全量分组，以及某个分组下的账号(sub2api)/渠道(new-api)列表。
// 供 admin 分组健康主列表聚合使用，返回值均不含敏感字段。
// 由 *upstream.PlatformService 结构性满足，通过 SetPlatformGroupReader 注入。
type PlatformGroupReader interface {
	FetchAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error)
	ListAdminGroupAccounts(session upstream.Session, group upstream.AdminGroupInfo) ([]upstream.AdminGroupAccountInfo, error)
	// ResolveProbeCredential 在探活前 server-only 地临时解析某账号/渠道的明文 base_url + key。
	// 明文凭据只在返回值里短暂存在，绝不落库/日志/前端；失败返回 *upstream.ProbeCredentialError。
	ResolveProbeCredential(session upstream.Session, account upstream.AdminGroupAccountInfo) (upstream.ProbeCredential, error)
}

// AdminAccountResolver 解析当前用户所在的 workspace（admin_account_id），
// 与其余模块使用的同一注入模式一致，由 admin_accounts.Service 实现。
type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

// requestError 是模块内部的请求级错误，值为 i18n key，供 handler 按类型映射 HTTP 状态码。
type requestError string

func (e requestError) Error() string { return string(e) }
