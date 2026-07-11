package mass_email

import "time"

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
)

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
}

type CreateBatchRequest struct {
	TemplateID    string        `json:"templateId"`
	SelectionMode SelectionMode `json:"selectionMode"`
	UserIDs       []string      `json:"userIds,omitempty"`
	Filters       BatchFilters  `json:"filters,omitempty"`
	RequestID     string        `json:"requestId"`
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
	ID              string
	BatchID         string
	UserID          string
	AdminAccountID  string
	UpstreamUserID  string
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
	Email          string
	Username       string
}
