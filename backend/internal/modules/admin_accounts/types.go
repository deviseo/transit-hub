package admin_accounts

import "time"

const (
	ErrorNoCurrentAccount = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorNotFound         = "admin.adminAccounts.errors.notFound"
	ErrorRequest          = "admin.adminAccounts.errors.request"
	DeleteConfirmation    = "DELETE WORKSPACE"
)

// Account describes one isolated admin workspace owned by a Transit Hub user.
type Account struct {
	ID          string     `json:"id"`
	UserID      string     `json:"-"`
	Platform    string     `json:"platform"`
	BaseURL     string     `json:"baseUrl"`
	Identity    string     `json:"identity"`
	DisplayName string     `json:"displayName"`
	AuthMethod  string     `json:"authMethod"`
	Current     bool       `json:"current"`
	LastUsedAt  *time.Time `json:"lastUsedAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

type UpsertInput struct {
	Platform    string
	BaseURL     string
	Identity    string
	DisplayName string
	AuthMethod  string
}

type UpdateRequest struct {
	DisplayName string `json:"displayName"`
}

type DeleteRequest struct {
	Confirmation string `json:"confirmation"`
}

type DeleteResponse struct {
	DeletedID             string `json:"deletedId"`
	HasCurrent            bool   `json:"hasCurrent"`
	CurrentAdminAccountID string `json:"currentAdminAccountId"`
	CleanupComplete       bool   `json:"cleanupComplete"`
	CleanupPending        bool   `json:"cleanupPending"`
}

type DeleteResult struct {
	CleanupJobID           string
	DeletedID              string
	WasCurrent             bool
	CurrentAdminAccountID  string
	AttachmentStoragePaths []string
	UpstreamSiteIDs        []string
}

type CleanupJob struct {
	ID                     string
	UserID                 string
	AdminAccountID         string
	AttachmentStoragePaths []string
	UpstreamSiteIDs        []string
	Attempts               int
	NextAttemptAt          time.Time
	LastError              string
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type WorkspaceCleanupPayload struct {
	UserID                 string
	AdminAccountID         string
	AttachmentStoragePaths []string
	UpstreamSiteIDs        []string
}

type requestError string

func (e requestError) Error() string { return string(e) }
