package leads

import "time"

// 线索的状态由发送记录和退订名单算出来，不单独保存：
//   - unsubscribed：在工作区的退订名单里（以后的群发都会跳过它）；
//   - bounced：收件服务器明确拒收过这个地址（地址不存在等），以后不再发；
//   - sent：成功发出过至少一封；
//   - not_sent：还没发过。
const (
	StatusNotSent      = "not_sent"
	StatusSent         = "sent"
	StatusUnsubscribed = "unsubscribed"
	StatusBounced      = "bounced"
)

const (
	maxSourceRunes    = 40
	maxNoteRunes      = 200
	maxSearchRunes    = 100
	maxImportEmails   = 5000
	maxSelectedIDs    = 5000
	maxRecipients     = 10000
	defaultDailyLimit = 200
	maxDailyLimit     = 10000
	defaultPageSize   = 20
	maxPageSize       = 100
	maxSources        = 200
)

type requestError string

func (e requestError) Error() string { return string(e) }

const (
	ErrInvalidRequest      = requestError("admin.leads.errors.invalidRequest")
	ErrInvalidEmail        = requestError("admin.leads.errors.invalidEmail")
	ErrDuplicate           = requestError("admin.leads.errors.duplicate")
	ErrSuppressed          = requestError("admin.leads.errors.suppressed")
	ErrTooMany             = requestError("admin.leads.errors.tooMany")
	ErrNotFound            = requestError("admin.leads.errors.notFound")
	ErrNoRecipients        = requestError("admin.leads.errors.noRecipients")
	ErrDailyLimit          = requestError("admin.leads.errors.dailyLimit")
	ErrUnsubscribeRequired = requestError("admin.leads.errors.unsubscribeRequired")
	ErrInvalidDailyLimit   = requestError("admin.leads.errors.invalidDailyLimit")
	ErrPersistence         = requestError("admin.leads.errors.persistence")
	ErrNoCurrentAccount    = requestError("admin.adminAccounts.errors.noCurrentAccount")
)

// Workspace 是一个 TransitHub 用户当前选中的站点（admin 账号），线索按它隔离。
type Workspace struct {
	UserID         string
	AdminAccountID string
}

type Lead struct {
	ID         string     `json:"id"`
	Email      string     `json:"email"`
	Source     string     `json:"source"`
	Note       string     `json:"note"`
	Status     string     `json:"status"`
	SentCount  int        `json:"sentCount"`
	LastSentAt *time.Time `json:"lastSentAt,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}

type ListQuery struct {
	Page     int
	PageSize int
	Source   string
	Status   string
	Search   string
}

// StatusCounts 是当前来源、搜索条件下各状态的数量（不受状态筛选影响），给页面上的状态标签用。
type StatusCounts struct {
	All          int `json:"all"`
	NotSent      int `json:"notSent"`
	Sent         int `json:"sent"`
	Unsubscribed int `json:"unsubscribed"`
	Bounced      int `json:"bounced"`
}

type ListResult struct {
	Items    []Lead       `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
	Pages    int          `json:"pages"`
	Counts   StatusCounts `json:"counts"`
}

type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

type AddRequest struct {
	Email  string `json:"email"`
	Source string `json:"source"`
	Note   string `json:"note"`
}

type UpdateRequest struct {
	Source string `json:"source"`
	Note   string `json:"note"`
}

// ImportRequest 是批量导入：Text 是粘贴的内容或上传文件的内容，Source 给这一批统一的来源。
type ImportRequest struct {
	Text   string `json:"text"`
	Source string `json:"source"`
}

type ImportResult struct {
	Added      int `json:"added"`
	Duplicates int `json:"duplicates"`
	Invalid    int `json:"invalid"`
	Suppressed int `json:"suppressed"`
}

type DeleteRequest struct {
	IDs []string `json:"ids"`
}

type DeleteResult struct {
	Deleted int `json:"deleted"`
}

// Settings 是线索发信设置：最近 24 小时最多发给线索多少封，以及已经发了多少。
type Settings struct {
	DailyLimit  int `json:"dailyLimit"`
	SentLast24h int `json:"sentLast24h"`
	Remaining   int `json:"remaining"`
}

type SettingsRequest struct {
	DailyLimit int `json:"dailyLimit"`
}

const (
	SelectionSelected = "selected"
	SelectionFilter   = "filter"
)

// Selection 是要发给哪些线索：勾选的（IDs），或者当前筛选条件下的全部。
// 已退订、退信的线索总是跳过。
type Selection struct {
	Mode   string   `json:"mode"`
	IDs    []string `json:"ids,omitempty"`
	Source string   `json:"source,omitempty"`
	Status string   `json:"status,omitempty"`
	Search string   `json:"search,omitempty"`
}

type PreviewResult struct {
	Sendable   int `json:"sendable"`
	Skipped    int `json:"skipped"`
	DailyLimit int `json:"dailyLimit"`
	Remaining  int `json:"remaining"`
}

type SendRequest struct {
	TemplateID    string    `json:"templateId"`
	Selection     Selection `json:"selection"`
	PublicBaseURL string    `json:"publicBaseUrl"`
	AdLabel       bool      `json:"adLabel"`
	RequestID     string    `json:"requestId"`
}
