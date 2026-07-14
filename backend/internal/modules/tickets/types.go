package tickets

import (
	"context"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// 工单状态机（第一版仅四态）：
//   - open：iframe 用户新建，后台尚未处理。
//   - pending：用户在收到后台回复后又补充了消息，等待后台再次处理；也用于"客服还没来得及处理，
//     用户又追加信息"的场景（open 状态下用户继续回复同样落到 pending，语义上更贴近"待处理"）。
//   - replied：后台已回复，等待用户查看或继续反馈。
//   - closed：已关闭，iframe 用户不能再追加回复。
const (
	StatusOpen    = "open"
	StatusPending = "pending"
	StatusReplied = "replied"
	StatusClosed  = "closed"
)

// 工单消息的作者类型：customer 来自 iframe 内的 Sub2API 用户，admin 来自 TransitHub 后台。
const (
	AuthorCustomer = "customer"
	AuthorAdmin    = "admin"
)

// 嵌入页面风格模板（第二阶段新增）。只影响 /embed/tickets 的视觉表现，不改变任何接口、
// 身份校验、工单状态或数据隔离规则。
const (
	TemplateDefault = "default"
	TemplateMinimal = "minimal"
	TemplateSupport = "support"
)

// 错误 i18n key。admin.tickets.* 用于后台接口，embed.tickets.* 用于公开 iframe 接口，
// 两套命名空间分别对应前端 admin 和 embed 两个独立模块的文案。
const (
	ErrorRequest          = "admin.tickets.errors.request"
	ErrorUnknown          = "admin.tickets.errors.unknown"
	ErrorNotFound         = "admin.tickets.errors.notFound"
	ErrorNoCurrentAccount = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorInvalidStatus    = "admin.tickets.errors.invalidStatus"
	ErrorBodyRequired     = "admin.tickets.errors.bodyRequired"
	ErrorTicketClosed     = "admin.tickets.errors.ticketClosed"
	ErrorInvalidTemplate  = "admin.tickets.errors.invalidTemplate"

	ErrorEmbedRequest         = "embed.tickets.errors.request"
	ErrorEmbedConfigNotFound  = "embed.tickets.errors.configNotFound"
	ErrorEmbedDisabled        = "embed.tickets.errors.disabled"
	ErrorEmbedInvalidSrcHost  = "embed.tickets.errors.invalidSrcHost"
	ErrorEmbedSrcHostMismatch = "embed.tickets.errors.srcHostMismatch"
	ErrorEmbedSub2apiAuth     = "embed.tickets.errors.sub2apiAuth"
	ErrorEmbedSub2apiRequest  = "embed.tickets.errors.sub2apiRequest"
	ErrorEmbedUserMismatch    = "embed.tickets.errors.userMismatch"
	ErrorEmbedSessionInvalid  = "embed.tickets.errors.sessionInvalid"
	ErrorEmbedInvalidEmail    = "embed.tickets.errors.invalidEmail"
	ErrorEmbedTitleRequired   = "embed.tickets.errors.titleRequired"
	ErrorEmbedBodyRequired    = "embed.tickets.errors.bodyRequired"

	// 图片附件相关错误（第三阶段新增）。
	ErrorInvalidMaxImages      = "admin.tickets.errors.invalidMaxImages"
	ErrorEmbedTooManyImages    = "embed.tickets.errors.tooManyImages"
	ErrorEmbedInvalidImageType = "embed.tickets.errors.invalidImageType"
	ErrorEmbedImageTooLarge    = "embed.tickets.errors.imageTooLarge"
	ErrorEmbedEmptyImage       = "embed.tickets.errors.emptyImage"

	// 工单分类/优先级可配置选项相关错误（增量新增）。
	ErrorInvalidCategoryOptions = "admin.tickets.errors.invalidCategoryOptions"
	ErrorInvalidPriorityOptions = "admin.tickets.errors.invalidPriorityOptions"
	ErrorEmbedCategoryRequired  = "embed.tickets.errors.categoryRequired"
	ErrorEmbedPriorityRequired  = "embed.tickets.errors.priorityRequired"
	ErrorEmbedInvalidCategory   = "embed.tickets.errors.invalidCategory"
	ErrorEmbedInvalidPriority   = "embed.tickets.errors.invalidPriority"
)

// DefaultCategoryOptions/DefaultPriorityOptions 是新 workspace（以及历史上没有显式配置过
// 分类/优先级选项的旧 workspace）自动获得的默认选项，与任务文档要求的默认值保持一致。
// 调用方如需可变副本，必须自行拷贝（append([]string(nil), DefaultCategoryOptions...)），
// 不能直接修改这两个包级切片。
var (
	DefaultCategoryOptions = []string{"通用问题", "余额/计费", "接口调用", "生图问题", "账号/登录"}
	DefaultPriorityOptions = []string{"低", "普通", "高", "紧急"}
)

// Sub2API 用户资料弹窗实时字段的不可用原因（i18n key）。这些不是请求错误——接口本身仍返回
// 200，只是把原因写进 Sub2apiUserProfileResponse.RemoteUnavailableReason，供前端展示具体原因，
// 而不是笼统的"暂不可用"。
const (
	Sub2apiRemoteUnavailableNoUserID       = "admin.tickets.sub2apiProfile.remoteUnavailable.noUserId"
	Sub2apiRemoteUnavailableNoAdminSession = "admin.tickets.sub2apiProfile.remoteUnavailable.noAdminSession"
	Sub2apiRemoteUnavailableUserNotFound   = "admin.tickets.sub2apiProfile.remoteUnavailable.userNotFound"
)

// requestError 是本模块的轻量错误类型，Error() 直接返回 i18n key，供 handler 透传给前端，
// 与 group_rate_campaigns/connection_health 等模块的既有约定一致。
type requestError string

func (e requestError) Error() string { return string(e) }

// EmbedConfig 对应 ticket_embed_configs 表的一行：一个 TransitHub admin 工作区的 iframe 嵌入配置。
//
// Enabled/AllowedSrcHost 是第一阶段字段，第二阶段（本次）取消了这两项配置能力：为了线上兼容，
// 数据库列和结构体字段都不删除，但 Service 不再依据它们限制 iframe 访问——见 service.go 中
// CreateEmbedSession 的说明。
type EmbedConfig struct {
	UserID             string
	AdminAccountID     string
	EmbedToken         string
	Enabled            bool
	AllowedSrcHost     string
	Template           string
	MaxImagesPerTicket int
	CategoryOptions    []string
	PriorityOptions    []string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Ticket 对应 tickets 表的一行。Sub2api* 字段是 /auth/me 解析出的身份快照，ManualEmail 是用户
// 新建工单时手动输入的联系邮箱，两者按文档要求都必须在后台列表/详情中展示。
type Ticket struct {
	ID             string
	UserID         string
	AdminAccountID string
	Sub2apiSrcHost string
	Sub2apiSrcURL  string
	Sub2apiUserID  string
	Sub2apiEmail   string
	Sub2apiRole    string
	ManualEmail    string
	Title          string
	Status         string
	Category       string
	Priority       string
	LastMessageAt  time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// TicketMessage 对应 ticket_messages 表的一行。
type TicketMessage struct {
	ID             string
	TicketID       string
	UserID         string
	AdminAccountID string
	AuthorType     string
	AuthorName     string
	Body           string
	CreatedAt      time.Time
}

// TicketAttachment 对应 ticket_attachments 表的一行。图片二进制不落库，只存 metadata 和服务端
// 生成的存储路径（StoragePath 是相对 TICKET_UPLOAD_DIR 的相对路径，从不暴露给前端）。
// 第一版附件只在"新建工单"时随首条消息一起产生，因此总是与 MessageID 一一对应。
type TicketAttachment struct {
	ID             string
	TicketID       string
	MessageID      string
	UserID         string
	AdminAccountID string
	AuthorType     string
	OriginalName   string
	ContentType    string
	SizeBytes      int64
	StoragePath    string
	CreatedAt      time.Time
}

// AttachmentUpload 是 handler 从 multipart/form-data 请求中解析出的单个图片文件，写入磁盘前
// 由 Service 统一校验（content-type 嗅探、大小、数量）。
type AttachmentUpload struct {
	OriginalName string
	ContentType  string
	Data         []byte
}

// EmbedSession 是 iframe 会话在 Redis 中的存储形态：Sub2API 身份解析结果 + 所属 TransitHub 工作区，
// 不含任何 Sub2API token（安全边界要求，token 只在换取会话的这一次请求中短暂使用）。
type EmbedSession struct {
	UserID             string   `json:"userId"`
	AdminAccountID     string   `json:"adminAccountId"`
	EmbedToken         string   `json:"embedToken"`
	SrcHost            string   `json:"srcHost"`
	SrcURL             string   `json:"srcUrl"`
	Sub2apiUserID      string   `json:"sub2apiUserId"`
	Sub2apiEmail       string   `json:"sub2apiEmail"`
	Sub2apiRole        string   `json:"sub2apiRole"`
	Template           string   `json:"template"`
	MaxImagesPerTicket int      `json:"maxImagesPerTicket"`
	CategoryOptions    []string `json:"categoryOptions"`
	PriorityOptions    []string `json:"priorityOptions"`
}

// ---- 公开 iframe 接口 DTO ----

// CreateSessionRequest 是 POST /api/embed/tickets/session 的请求体，字段对齐任务文档示例。
type CreateSessionRequest struct {
	EmbedToken   string `json:"embedToken"`
	Sub2apiToken string `json:"sub2apiToken"`
	UrlUserID    string `json:"urlUserId"`
	SrcHost      string `json:"srcHost"`
	SrcURL       string `json:"srcUrl"`
}

type CreateSessionResponse struct {
	SessionToken       string   `json:"sessionToken"`
	Template           string   `json:"template"`
	MaxImagesPerTicket int      `json:"maxImagesPerTicket"`
	CategoryOptions    []string `json:"categoryOptions"`
	PriorityOptions    []string `json:"priorityOptions"`
}

type EmbedTicketListItem struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Status        string    `json:"status"`
	ManualEmail   string    `json:"manualEmail"`
	Category      string    `json:"category"`
	Priority      string    `json:"priority"`
	LastMessageAt time.Time `json:"lastMessageAt"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type EmbedTicketListResponse struct {
	Items []EmbedTicketListItem `json:"items"`
}

// TicketAttachmentView 是附件在工单详情响应中的展示形状，只暴露展示所需的 metadata；
// 实际图片内容必须通过受鉴权的 /attachments/{id} 接口按需拉取，这里不下发任何直接可访问的 URL。
type TicketAttachmentView struct {
	ID           string    `json:"id"`
	OriginalName string    `json:"originalName"`
	ContentType  string    `json:"contentType"`
	SizeBytes    int64     `json:"sizeBytes"`
	CreatedAt    time.Time `json:"createdAt"`
}

type TicketMessageView struct {
	ID          string                 `json:"id"`
	AuthorType  string                 `json:"authorType"`
	AuthorName  string                 `json:"authorName"`
	Body        string                 `json:"body"`
	CreatedAt   time.Time              `json:"createdAt"`
	Attachments []TicketAttachmentView `json:"attachments"`
}

type EmbedTicketDetail struct {
	EmbedTicketListItem
	Messages []TicketMessageView `json:"messages"`
}

type CreateTicketRequest struct {
	ManualEmail string `json:"manualEmail"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	Category    string `json:"category"`
	Priority    string `json:"priority"`
}

type CreateMessageRequest struct {
	Body string `json:"body"`
}

// ---- TransitHub 后台接口 DTO ----

// AdminTicketListItem 后台列表行，必须同时展示手动邮箱与 Sub2API 身份信息（用户 ID/邮箱/角色/来源域名）。
type AdminTicketListItem struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	Status         string    `json:"status"`
	ManualEmail    string    `json:"manualEmail"`
	Category       string    `json:"category"`
	Priority       string    `json:"priority"`
	Sub2apiUserID  string    `json:"sub2apiUserId"`
	Sub2apiEmail   string    `json:"sub2apiEmail"`
	Sub2apiRole    string    `json:"sub2apiRole"`
	Sub2apiSrcHost string    `json:"sub2apiSrcHost"`
	LastMessageAt  time.Time `json:"lastMessageAt"`
	CreatedAt      time.Time `json:"createdAt"`
}

type AdminTicketListResponse struct {
	Items      []AdminTicketListItem `json:"items"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"pageSize"`
	TotalPages int                   `json:"totalPages"`
}

type AdminTicketDetail struct {
	AdminTicketListItem
	Sub2apiSrcURL string              `json:"sub2apiSrcUrl"`
	Messages      []TicketMessageView `json:"messages"`
}

// AdminListQuery 描述后台工单列表的分页与状态筛选控制。
type AdminListQuery struct {
	Status   string
	Page     int
	PageSize int
}

type UpdateStatusRequest struct {
	Status string `json:"status"`
}

// EmbedConfigResponse 保留 Enabled/AllowedSrcHost 字段以兼容旧前端调用方，但取消了这两项配置
// 能力后，值恒为 true / ""（见 handler.go toEmbedConfigResponse）。Template 是第二阶段新增字段。
type EmbedConfigResponse struct {
	Enabled            bool     `json:"enabled"`
	EmbedToken         string   `json:"embedToken"`
	AllowedSrcHost     string   `json:"allowedSrcHost"`
	EmbedURL           string   `json:"embedUrl"`
	Template           string   `json:"template"`
	MaxImagesPerTicket int      `json:"maxImagesPerTicket"`
	CategoryOptions    []string `json:"categoryOptions"`
	PriorityOptions    []string `json:"priorityOptions"`
}

// UpdateEmbedConfigRequest 是保存嵌入配置的请求体。Enabled/AllowedSrcHost 仅为兼容旧前端保留
// （Enabled 用指针区分"未传"和"显式传 false"，避免旧请求体的默认零值误判为用户主动关闭），
// Service 不再依据它们改变实际行为，只使用 Template 和 MaxImagesPerTicket。
// MaxImagesPerTicket 同样用指针区分"未传"（保持现有值）和"显式传 0"（关闭图片上传）。
// CategoryOptions/PriorityOptions 是可选字段（nil 表示未传，沿用已有配置；非 nil，包括空数组，
// 都会走 normalizeTicketOptions 校验），用来兼容只提交 template/maxImagesPerTicket 的旧前端请求体。
type UpdateEmbedConfigRequest struct {
	Enabled            *bool    `json:"enabled,omitempty"`
	AllowedSrcHost     string   `json:"allowedSrcHost,omitempty"`
	Template           string   `json:"template"`
	MaxImagesPerTicket *int     `json:"maxImagesPerTicket,omitempty"`
	CategoryOptions    []string `json:"categoryOptions,omitempty"`
	PriorityOptions    []string `json:"priorityOptions,omitempty"`
}

// Sub2apiUserProfileResponse 是后台"Sub2API 用户资料"只读弹窗的响应体。
//
// 工单创建时保存的身份快照（用户 ID/邮箱/角色/来源域名）永远可用。余额/总充值/注册时间/
// 充值记录尽量通过当前 workspace 的 Sub2API admin 会话（RequireSession 已刷新并校验过 admin
// 身份）实时查询 GET /api/v1/admin/users/:id 与 .../balance-history 得到；查询链路上任意一步
// 失败都只把对应 *Available 标记为 false 并在 RemoteUnavailableReason 写明原因，接口本身仍返回
// 200，不因为拿不到实时数据就让整个弹窗失败。不伪造数据。
type Sub2apiUserProfileResponse struct {
	Sub2apiUserID  string `json:"sub2apiUserId"`
	Sub2apiEmail   string `json:"sub2apiEmail"`
	Sub2apiRole    string `json:"sub2apiRole"`
	Sub2apiSrcHost string `json:"sub2apiSrcHost"`
	Sub2apiSrcURL  string `json:"sub2apiSrcUrl"`

	BalanceAvailable bool     `json:"balanceAvailable"`
	Balance          *float64 `json:"balance,omitempty"`

	TotalRechargedAvailable bool     `json:"totalRechargedAvailable"`
	TotalRecharged          *float64 `json:"totalRecharged,omitempty"`

	RegisteredAtAvailable bool       `json:"registeredAtAvailable"`
	RegisteredAt          *time.Time `json:"registeredAt,omitempty"`

	RechargeHistoryAvailable bool                         `json:"rechargeHistoryAvailable"`
	RechargeHistory          []Sub2apiRechargeHistoryItem `json:"rechargeHistory,omitempty"`

	// 以下字段第一版不存在，全部新增且为可选（omitempty），旧前端可以安全忽略。
	Username      string     `json:"username,omitempty"`
	Status        string     `json:"status,omitempty"`
	Concurrency   *int       `json:"concurrency,omitempty"`
	RPMLimit      *int       `json:"rpmLimit,omitempty"`
	FrozenBalance *float64   `json:"frozenBalance,omitempty"`
	LastUsedAt    *time.Time `json:"lastUsedAt,omitempty"`

	// RemoteUnavailableReason 是实时字段不可用时的 i18n key（见 Sub2apiRemoteUnavailable* 常量），
	// 全部实时字段都成功获取时为空字符串。
	RemoteUnavailableReason string `json:"remoteUnavailableReason,omitempty"`
}

// Sub2apiRechargeHistoryItem 是 Sub2API 用户余额/充值历史中的单条记录，供后台资料弹窗展示。
type Sub2apiRechargeHistoryItem struct {
	ID        string     `json:"id"`
	Type      string     `json:"type"`
	Amount    *float64   `json:"amount,omitempty"`
	Note      string     `json:"note,omitempty"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

// AdminAccountResolver 解析当前用户所在的 workspace（admin_account_id），
// 与其余模块使用的同一注入模式一致，由 admin_accounts.Service 实现。
type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

// isValidTicketStatus 校验第一版允许的四个工单状态。
func isValidTicketStatus(status string) bool {
	switch status {
	case StatusOpen, StatusPending, StatusReplied, StatusClosed:
		return true
	default:
		return false
	}
}

// isValidEmbedTemplate 校验第二阶段允许的三个嵌入页面模板。
func isValidEmbedTemplate(template string) bool {
	switch template {
	case TemplateDefault, TemplateMinimal, TemplateSupport:
		return true
	default:
		return false
	}
}

// maxImagesPerTicketLowerBound/UpperBound 是每次工单允许上传的图片数量配置范围；
// 下界 0 表示关闭图片上传，是线上旧 workspace 迁移后的默认值。
const (
	maxImagesPerTicketLowerBound = 0
	maxImagesPerTicketUpperBound = 9
)

func isValidMaxImagesPerTicket(value int) bool {
	return value >= maxImagesPerTicketLowerBound && value <= maxImagesPerTicketUpperBound
}

// ticketOptionsMinCount/MaxCount/OptionMaxLength 是分类/优先级选项组的校验边界：至少 1 项、
// 最多 20 项，单项 trim 后最多 40 个 Unicode rune（用 rune 计数兼容中文场景下的字符长度直觉）。
const (
	ticketOptionsMinCount = 1
	ticketOptionsMaxCount = 20
	ticketOptionMaxLength = 40
)

// normalizeTicketOptions 校验并规范化一组分类/优先级选项：逐项 trim、拒绝空值/超长项，
// 按用户输入顺序去重（去重前先 trim，保证"内容相同但首尾有空格"的两项被视为同一项）。
// options 为 nil 时也按"数量不足"处理，交由调用方决定 nil 的语义（写入路径用 nil 表示未传，
// 读取兜底路径用 nil 表示"没有可用数据，退回默认值"，两种场景都不应该把 nil 当作合法选项组）。
func normalizeTicketOptions(options []string) ([]string, bool) {
	if len(options) < ticketOptionsMinCount || len(options) > ticketOptionsMaxCount {
		return nil, false
	}
	seen := make(map[string]struct{}, len(options))
	result := make([]string, 0, len(options))
	for _, raw := range options {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || utf8.RuneCountInString(trimmed) > ticketOptionMaxLength {
			return nil, false
		}
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	if len(result) == 0 {
		return nil, false
	}
	return result, true
}

// withDefaultTicketOptions 保证一个 EmbedConfig 的 CategoryOptions/PriorityOptions 始终是合法的
// 非空选项组：数据库中已经存在但尚未回填的历史行、或测试用的 fake repository 没有显式设置这两个
// 字段时，读取路径一律退回默认选项，而不是把 nil/非法值原样透传给上层导致 panic 或前端渲染空列表。
func withDefaultTicketOptions(c EmbedConfig) EmbedConfig {
	if normalized, ok := normalizeTicketOptions(c.CategoryOptions); ok {
		c.CategoryOptions = normalized
	} else {
		c.CategoryOptions = append([]string(nil), DefaultCategoryOptions...)
	}
	if normalized, ok := normalizeTicketOptions(c.PriorityOptions); ok {
		c.PriorityOptions = normalized
	} else {
		c.PriorityOptions = append([]string(nil), DefaultPriorityOptions...)
	}
	return c
}

// containsTicketOption 判断 value 是否在 options 中，用于校验用户提交的 category/priority
// 是否属于当前 workspace 的实时配置。
func containsTicketOption(options []string, value string) bool {
	return slices.Contains(options, value)
}
