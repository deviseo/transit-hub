// Package tickets 是 TransitLink 用户和站长之间的工单。
//
// 用户在 TransitLink 里提交问题：客户端先用自己在站点上的登录凭证换一个工单会话（TransitHub 去站点核实身份，
// 站点地址只用服务端配置里的，不信任客户端给的地址），之后用会话列出、提交、回复自己的工单。
// 站长在 TransitHub 的「工单」页面回复和处理；新工单和用户追问会推送到工作区的通知机器人。
// 所有数据按工作区（user_id, admin_account_id）隔离，用户只能看到自己在这个站点上的工单。
package tickets

import "time"

// 工单状态。
const (
	// StatusOpen：用户新提的，等站长回复。
	StatusOpen = "open"
	// StatusPending：站长回复后用户又追问了，等站长回复。
	StatusPending = "pending"
	// StatusReplied：站长回复了，等用户。
	StatusReplied = "replied"
	// StatusClosed：已关闭，用户不能再回复（可以新建工单）。
	StatusClosed = "closed"
)

// 消息作者。
const (
	AuthorCustomer = "customer"
	AuthorAdmin    = "admin"
)

const (
	maxTitleRunes       = 100
	maxBodyRunes        = 5000
	maxContactRunes     = 100
	maxClientInfoRunes  = 100
	maxImagesPerMessage = 5
	// maxUploadRequestBytes：带图片的请求最大 30 MB（5 张 × 5 MB 加上文字）。
	maxUploadRequestBytes = 30 << 20
	maxJSONRequestBytes   = 64 << 10
	clientListLimit       = 100
	defaultAdminPageSize  = 20
	maxAdminPageSize      = 100
	sessionTTL            = 7 * 24 * time.Hour
)

// DefaultCategories 是站长没设置时的问题分类。
var DefaultCategories = []string{"通用问题", "余额/计费", "接口调用", "账号/登录"}

// 错误（返回给前端的翻译 key）。
const (
	ErrorInvalid          = "tickets.errors.invalid"
	ErrorNotFound         = "tickets.errors.notFound"
	ErrorClosed           = "tickets.errors.closed"
	ErrorDisabled         = "tickets.errors.disabled"
	ErrorSessionExpired   = "tickets.errors.sessionExpired"
	ErrorSiteLoginExpired = "tickets.errors.siteLoginExpired"
	ErrorSiteUnavailable  = "tickets.errors.siteUnavailable"
	ErrorTooManyRequests  = "tickets.errors.tooManyRequests"
	ErrorTooManyImages    = "tickets.errors.tooManyImages"
	ErrorImageTooLarge    = "tickets.errors.imageTooLarge"
	ErrorInvalidImage     = "tickets.errors.invalidImage"
	ErrorUploadsDisabled  = "tickets.errors.uploadsDisabled"
	ErrorNoCurrentAccount = "tickets.errors.noCurrentAccount"
)

type requestError string

func (e requestError) Error() string { return string(e) }

// Workspace 是站长的一个站点（工作区）。
type Workspace struct {
	UserID         string
	AdminAccountID string
}

// SiteUser 是提问的用户在站点上的身份。
type SiteUser struct {
	Platform string
	ID       string
	Username string
	Email    string
}

// Ticket 是数据库里的一个工单。
type Ticket struct {
	ID string
	Workspace
	User           SiteUser
	Contact        string
	ClientInfo     string
	Title          string
	Category       string
	Status         string
	LastAuthor     string
	LastMessageAt  time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CustomerSeenAt *time.Time
}

// Message 是工单里的一条消息。
type Message struct {
	ID          string
	TicketID    string
	AuthorType  string
	AuthorName  string
	Body        string
	CreatedAt   time.Time
	Attachments []Attachment
}

// Attachment 是消息里的一张图片。StoragePath 是数据目录 ticket-uploads 里的文件名，不返回给前端。
type Attachment struct {
	ID           string
	TicketID     string
	MessageID    string
	AuthorType   string
	OriginalName string
	ContentType  string
	SizeBytes    int64
	StoragePath  string
	CreatedAt    time.Time
}

// AttachmentUpload 是一张待保存的图片。
type AttachmentUpload struct {
	OriginalName string
	ContentType  string
	Data         []byte
}

// Portal 是一个专属链接对应的工单入口：哪个工作区、站点在哪、开没开工单、有哪些分类。
type Portal struct {
	Workspace
	Platform   string
	BaseURL    string
	SiteName   string
	Enabled    bool
	Categories []string
}

// Credential 是用户在站点上的登录凭证，只用来核实一次身份，不保存。
type Credential struct {
	// Kind：bearer（new-api 个人访问令牌、sub2api 访问令牌）或 cookie（旧版 new-api 的会话）。
	Kind   string `json:"kind"`
	Token  string `json:"token"`
	Cookie string `json:"cookie"`
	// UserID：旧版 new-api 需要（New-Api-User 头）。
	UserID string `json:"userId"`
}

// ClientSession 是一个工单会话（存在 Redis，七天后过期，客户端过期后重新换取）。
type ClientSession struct {
	Workspace  Workspace `json:"workspace"`
	User       SiteUser  `json:"user"`
	ClientInfo string    `json:"clientInfo"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

// CreateSessionRequest 是 POST /api/client/v1/sites/{code}/tickets/session 的请求。
type CreateSessionRequest struct {
	Credential Credential `json:"credential"`
	// ClientInfo：客户端版本和系统，例如 "TransitLink 0.2.0 (macOS)"，显示给站长。
	ClientInfo string `json:"clientInfo"`
}

// SessionResponse 是换取工单会话的响应。
type SessionResponse struct {
	SessionToken string       `json:"sessionToken"`
	ExpiresAt    time.Time    `json:"expiresAt"`
	User         SessionUser  `json:"user"`
	Categories   []string     `json:"categories"`
	Limits       UploadLimits `json:"limits"`
}

type SessionUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UploadLimits 告诉客户端图片的限制。
type UploadLimits struct {
	Enabled       bool  `json:"enabled"`
	MaxImages     int   `json:"maxImages"`
	MaxImageBytes int64 `json:"maxImageBytes"`
}

// CreateTicketRequest 是新建工单（JSON 或 multipart 表单的字段）。
type CreateTicketRequest struct {
	Title    string `json:"title"`
	Category string `json:"category"`
	Body     string `json:"body"`
	Contact  string `json:"contact"`
}

// ReplyRequest 是回复（JSON 或 multipart 表单的字段）。
type ReplyRequest struct {
	Body string `json:"body"`
}

// StatusRequest 是站长修改状态。
type StatusRequest struct {
	Status string `json:"status"`
}

// AttachmentView 是返回给前端的附件信息（下载要另外请求）。
type AttachmentView struct {
	ID           string    `json:"id"`
	OriginalName string    `json:"originalName"`
	ContentType  string    `json:"contentType"`
	SizeBytes    int64     `json:"sizeBytes"`
	CreatedAt    time.Time `json:"createdAt"`
}

type MessageView struct {
	ID          string           `json:"id"`
	AuthorType  string           `json:"authorType"`
	AuthorName  string           `json:"authorName"`
	Body        string           `json:"body"`
	CreatedAt   time.Time        `json:"createdAt"`
	Attachments []AttachmentView `json:"attachments"`
}

// ClientTicketView 是用户看到的工单。Unread：站长回复了、用户还没看。
type ClientTicketView struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Category      string    `json:"category"`
	Status        string    `json:"status"`
	Contact       string    `json:"contact"`
	Unread        bool      `json:"unread"`
	LastMessageAt time.Time `json:"lastMessageAt"`
	CreatedAt     time.Time `json:"createdAt"`
}

type ClientTicketDetail struct {
	ClientTicketView
	Messages []MessageView `json:"messages"`
}

type ClientListResponse struct {
	Items  []ClientTicketView `json:"items"`
	Unread int                `json:"unread"`
}

// AdminTicketView 是站长看到的工单。
type AdminTicketView struct {
	ID            string    `json:"id"`
	Title         string    `json:"title"`
	Category      string    `json:"category"`
	Status        string    `json:"status"`
	Contact       string    `json:"contact"`
	Platform      string    `json:"platform"`
	SiteUserID    string    `json:"siteUserId"`
	SiteUsername  string    `json:"siteUsername"`
	SiteEmail     string    `json:"siteEmail"`
	ClientInfo    string    `json:"clientInfo"`
	LastAuthor    string    `json:"lastAuthor"`
	LastMessageAt time.Time `json:"lastMessageAt"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type AdminTicketDetail struct {
	AdminTicketView
	Messages []MessageView `json:"messages"`
}

// StatusCounts 是各状态的工单数。
type StatusCounts struct {
	Open    int `json:"open"`
	Pending int `json:"pending"`
	Replied int `json:"replied"`
	Closed  int `json:"closed"`
}

type AdminListResponse struct {
	Items      []AdminTicketView `json:"items"`
	Total      int               `json:"total"`
	Page       int               `json:"page"`
	PageSize   int               `json:"pageSize"`
	TotalPages int               `json:"totalPages"`
	Counts     StatusCounts      `json:"counts"`
}
