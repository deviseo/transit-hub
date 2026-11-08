package mass_email

import (
	"context"
	"time"
)

const (
	BatchStatusQueued              = "queued"
	BatchStatusRunning             = "running"
	BatchStatusCancelling          = "cancelling"
	BatchStatusCancelled           = "cancelled"
	BatchStatusCompleted           = "completed"
	BatchStatusCompletedWithErrors = "completed_with_errors"
	BatchStatusFailed              = "failed"
)

const (
	ItemStatusPending   = "pending"
	ItemStatusSending   = "sending"
	ItemStatusSent      = "sent"
	ItemStatusFailed    = "failed"
	ItemStatusUncertain = "uncertain"
	ItemStatusCancelled = "cancelled"
)

type SelectionMode string

const (
	SelectionModeSelected SelectionMode = "selected"
	SelectionModeAll      SelectionMode = "all"
	// SelectionModeSegment 按「用户活跃分析」的筛选条件，从本地同步的用户快照解析收件人（sub2api / new-api 通用）。
	SelectionModeSegment SelectionMode = "segment"
	// SelectionModeLeads 发给「线索」里的邮箱，收件人由线索模块给出（见 CreateLeadBatch）。
	SelectionModeLeads SelectionMode = "leads"
)

// AdLabel 是营销邮件标题前的「广告」标识（《互联网电子邮件服务管理办法》要求在标题前部注明）。
const AdLabel = "【广告】"

type requestError string

func (e requestError) Error() string { return string(e) }

const (
	ErrInvalidRequest        = requestError("admin.massEmail.errors.invalidRequest")
	ErrInvalidSelection      = requestError("admin.massEmail.errors.invalidSelection")
	ErrTemplateNotFound      = requestError("admin.massEmail.errors.templateNotFound")
	ErrSMTPNotReady          = requestError("admin.massEmail.errors.smtpNotReady")
	ErrUpstreamAuth          = requestError("admin.massEmail.errors.upstreamAuth")
	ErrUpstreamRequest       = requestError("admin.massEmail.errors.upstreamRequest")
	ErrNoCurrentAccount      = requestError("admin.adminAccounts.errors.noCurrentAccount")
	ErrNotFound              = requestError("admin.massEmail.errors.notFound")
	ErrInvalidState          = requestError("admin.massEmail.errors.invalidState")
	ErrPersistence           = requestError("admin.massEmail.errors.persistence")
	ErrSendFailed            = requestError("admin.massEmail.errors.sendFailed")
	ErrActiveBatchExists     = requestError("admin.massEmail.errors.activeBatchExists")
	ErrRecipientLimitReached = requestError("admin.massEmail.errors.recipientLimitReached")
	ErrSegmentUnavailable    = requestError("admin.massEmail.errors.segmentUnavailable")
	ErrSegmentNotSynced      = requestError("admin.massEmail.errors.segmentNotSynced")
	ErrInvalidPublicURL      = requestError("admin.massEmail.errors.invalidPublicUrl")
	ErrInvalidSuppression    = requestError("admin.massEmail.errors.invalidSuppression")
	// ErrSuppressed 是发送时发现收件人已退订的 item 错误 key。
	ErrSuppressed = requestError("admin.massEmail.errors.suppressed")
	// ErrRecipientRejected 是收件服务器明确拒收这个地址（地址不存在等）的 item 错误 key：邮件肯定没发出去，
	// 线索里显示为退信，以后不再给它发。
	ErrRecipientRejected = requestError("admin.massEmail.errors.recipientRejected")
)

// LeadRecipient 是线索模块给出的收件人。
type LeadRecipient struct {
	LeadID string
	Email  string
}

// SegmentRecipient 是按活跃分群解析出的收件人。
type SegmentRecipient struct {
	UpstreamUserID string
	Email          string
	Username       string
}

// SegmentRecipientResolver 按筛选条件返回收件人（由 user_activity 模块实现，server 中适配）。
// 超过 max 个时返回 ErrRecipientLimitReached；工作区从未同步过时返回 ErrSegmentNotSynced。
type SegmentRecipientResolver interface {
	ResolveSegmentRecipients(ctx context.Context, userID string, adminAccountID string, filters BatchFilters, max int) ([]SegmentRecipient, error)
}

// Suppression 是退订名单中的一条记录。
type Suppression struct {
	Email     string    `json:"email"`
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

type SuppressionPage struct {
	Items    []Suppression `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"pageSize"`
	Pages    int           `json:"pages"`
}

type AddSuppressionsRequest struct {
	Emails []string `json:"emails"`
}

type AddSuppressionsResult struct {
	Added   int `json:"added"`
	Invalid int `json:"invalid"`
}

const (
	SuppressionReasonUnsubscribe = "unsubscribe"
	SuppressionReasonManual      = "manual"
)

type UserQuery struct {
	Page      int
	PageSize  int
	Status    string
	Role      string
	Search    string
	SortBy    string
	SortOrder string
	Timezone  string
}

type UserDTO struct {
	ID        string     `json:"id"`
	Email     string     `json:"email"`
	Username  string     `json:"username"`
	Role      string     `json:"role"`
	Status    string     `json:"status"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

type UsersPage struct {
	Items    []UserDTO `json:"items"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
	Pages    int       `json:"pages"`
}

type BatchFilters struct {
	Status string `json:"status"`
	Role   string `json:"role"`
	Search string `json:"search,omitempty"`
	// Segment / InactiveDays 只用于 segment 选择方式。
	Segment      string `json:"segment,omitempty"`
	InactiveDays int    `json:"inactiveDays,omitempty"`
}

type CreateBatchRequest struct {
	TemplateID    string        `json:"templateId"`
	SelectionMode SelectionMode `json:"selectionMode"`
	UserIDs       []string      `json:"userIds,omitempty"`
	Filters       BatchFilters  `json:"filters,omitempty"`
	RequestID     string        `json:"requestId"`
	// PublicBaseURL 非空时，每封邮件附带指向该地址（TransitHub 访问地址）的退订链接。
	PublicBaseURL string `json:"publicBaseUrl,omitempty"`
	// AdLabel 为 true 时，邮件标题前加上「【广告】」。
	AdLabel bool `json:"adLabel,omitempty"`
}

type Batch struct {
	ID              string
	UserID          string
	AdminAccountID  string
	RequestID       string
	TemplateID      string
	TemplateName    string
	TemplateSubject string
	TemplateHTML    string
	SelectionMode   SelectionMode
	Filters         BatchFilters
	PublicBaseURL   string
	Status          string
	RecipientCount  int
	SkippedCount    int
	SentCount       int
	FailedCount     int
	UncertainCount  int
	CancelledCount  int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	StartedAt       *time.Time
	FinishedAt      *time.Time
	CancelledAt     *time.Time
}

type BatchDTO struct {
	ID              string        `json:"id"`
	RequestID       string        `json:"requestId"`
	TemplateID      string        `json:"templateId"`
	TemplateName    string        `json:"templateName"`
	TemplateSubject string        `json:"templateSubject"`
	SelectionMode   SelectionMode `json:"selectionMode"`
	Filters         BatchFilters  `json:"filters"`
	UnsubscribeLink bool          `json:"unsubscribeLink"`
	Status          string        `json:"status"`
	RecipientCount  int           `json:"recipientCount"`
	SkippedCount    int           `json:"skippedCount"`
	SentCount       int           `json:"sentCount"`
	FailedCount     int           `json:"failedCount"`
	UncertainCount  int           `json:"uncertainCount"`
	CancelledCount  int           `json:"cancelledCount"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
	StartedAt       *time.Time    `json:"startedAt,omitempty"`
	FinishedAt      *time.Time    `json:"finishedAt,omitempty"`
	CancelledAt     *time.Time    `json:"cancelledAt,omitempty"`
}

type BatchItem struct {
	ID             string
	BatchID        string
	UserID         string
	AdminAccountID string
	UpstreamUserID string
	// LeadID 是收件人对应的线索（发给站点用户时为空）。
	LeadID          string
	RecipientEmail  string
	NormalizedEmail string
	Username        string
	Status          string
	ErrorKey        string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ClaimedAt       *time.Time
	SentAt          *time.Time
	FinishedAt      *time.Time
}

type BatchItemDTO struct {
	ID             string     `json:"id"`
	BatchID        string     `json:"batchId"`
	UpstreamUserID string     `json:"upstreamUserId"`
	RecipientEmail string     `json:"recipientEmail"`
	Username       string     `json:"username"`
	Status         string     `json:"status"`
	ErrorKey       string     `json:"errorKey,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	ClaimedAt      *time.Time `json:"claimedAt,omitempty"`
	SentAt         *time.Time `json:"sentAt,omitempty"`
	FinishedAt     *time.Time `json:"finishedAt,omitempty"`
}

type BatchPage struct {
	Items    []BatchDTO `json:"items"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
	Pages    int        `json:"pages"`
}

type ItemPage struct {
	Items    []BatchItemDTO `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
	Pages    int            `json:"pages"`
}

type recipientCandidate struct {
	UpstreamUserID string
	LeadID         string
	Email          string
	Username       string
}
