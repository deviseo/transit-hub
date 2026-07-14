package lottery

import "time"

const (
	ErrorRequest             = "admin.lottery.errors.request"
	ErrorUnknown             = "admin.lottery.errors.unknown"
	ErrorNoCurrentAccount    = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorAdminOnly           = "admin.dashboard.adminAuth.errors.adminOnly"
	ErrorInvalidSourceOrigin = "admin.lottery.errors.invalidSourceOrigin"
	ErrorNotFound            = "admin.lottery.errors.notFound"
	ErrorInvalidState        = "admin.lottery.errors.invalidState"
	ErrorValidation          = "admin.lottery.errors.validation"
	ErrorAlreadyEntered      = "embed.lottery.errors.alreadyEntered"

	ErrorEmbedRequest         = "embed.lottery.errors.request"
	ErrorEmbedConfigNotFound  = "embed.lottery.errors.configNotFound"
	ErrorEmbedInvalidSrcHost  = "embed.lottery.errors.invalidSrcHost"
	ErrorEmbedSrcHostMismatch = "embed.lottery.errors.srcHostMismatch"
	ErrorEmbedSub2apiAuth     = "embed.lottery.errors.sub2apiAuth"
	ErrorEmbedSub2apiRequest  = "embed.lottery.errors.sub2apiRequest"
	ErrorEmbedUserMismatch    = "embed.lottery.errors.userMismatch"
	ErrorEmbedUserInactive    = "embed.lottery.errors.userInactive"
	ErrorEmbedSessionInvalid  = "embed.lottery.errors.sessionInvalid"
	ErrorEmbedAdminSession    = "embed.lottery.errors.adminSession"
	ErrorEmbedSourceBinding   = "embed.lottery.errors.sourceBinding"
	ErrorEmbedCampaignNotOpen = "embed.lottery.errors.campaignNotOpen"
	ErrorEmbedEntryNotFound   = "embed.lottery.errors.entryNotFound"
	ErrorEmbedUpstreamRequest = "embed.lottery.errors.upstreamRequest"
	ErrorRewardUnsupported    = "admin.lottery.errors.rewardUnsupported"
	ErrorRewardAdminSession   = "admin.lottery.errors.rewardAdminSession"
	ErrorRewardManualRequired = "admin.lottery.errors.manualRedemptionRequired"
	ErrorSubscriptionGroups   = "admin.lottery.errors.subscriptionGroups"
)

const (
	StatusDraft      = "draft"
	StatusScheduled  = "scheduled"
	StatusOpen       = "open"
	StatusClosed     = "closed"
	StatusDrawing    = "drawing"
	StatusDrawn      = "drawn"
	StatusFulfilling = "fulfilling"
	StatusCompleted  = "completed"
	StatusPartial    = "partial"
	StatusCancelled  = "cancelled"

	DrawModeManual    = "manual"
	DrawModeScheduled = "scheduled"

	PrizeTypeBalance      = "balance"
	PrizeTypeSubscription = "subscription"
	DeliverySub2APIAuto   = "sub2api_auto"
	DeliveryVoucher       = "voucher"
	DeliveryManual        = "manual"

	EntryStatusActive    = "active"
	EntryStatusWithdrawn = "withdrawn"

	RewardPending          = "pending"
	RewardProcessing       = "processing"
	RewardFulfilled        = "fulfilled"
	RewardRetryableFailed  = "retryable_failed"
	RewardManualAttention  = "manual_attention"
	RewardFailed           = "failed"
	RateCleanupPending     = "pending"
	RateCleanupProcessing  = "processing"
	RateCleanupRetryable   = "retryable_failed"
	RateCleanupCompleted   = "completed"
	AlgorithmVersionV1     = "lottery-hmac-sha256-v1"
	AlgorithmVersionV2     = "lottery-hmac-sha256-public-v2"
	AlgorithmVersion       = AlgorithmVersionV2
	DefaultLotteryTimezone = "Asia/Shanghai"
)

type requestError string

func (e requestError) Error() string { return string(e) }

type EmbedConfig struct {
	UserID              string
	AdminAccountID      string
	EmbedToken          string
	Sub2apiSourceOrigin string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type Campaign struct {
	ID                string
	UserID            string
	AdminAccountID    string
	Name              string
	Description       string
	Status            string
	RegistrationStart *time.Time
	RegistrationEnd   *time.Time
	DrawAt            *time.Time
	DrawMode          string
	PublicWinners     bool
	SeedSecret        string
	SeedCommitment    string
	EntrySnapshotHash string
	RevealedSeed      string
	AlgorithmVersion  string
	EntryCount        int
	WinnerCount       int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	PublishedAt       *time.Time
	OpenedAt          *time.Time
	ClosedAt          *time.Time
	DrawnAt           *time.Time
	CompletedAt       *time.Time
	CancelledAt       *time.Time
}

type Prize struct {
	ID             string   `json:"id"`
	CampaignID     string   `json:"campaignId"`
	UserID         string   `json:"-"`
	AdminAccountID string   `json:"-"`
	Type           string   `json:"type"`
	Name           string   `json:"name"`
	Quantity       int      `json:"quantity"`
	SortOrder      int      `json:"sortOrder"`
	BalanceAmount  string   `json:"balanceAmount,omitempty"`
	GroupID        string   `json:"groupId,omitempty"`
	GroupName      string   `json:"groupName,omitempty"`
	Multiplier     string   `json:"multiplier,omitempty"`
	ValidityDays   *int     `json:"validityDays,omitempty"`
	DeliveryMode   string   `json:"deliveryMode"`
	ManualContact  string   `json:"manualContact,omitempty"`
	VoucherCodes   []string `json:"voucherCodes,omitempty"`
	ValueMarker    int      `json:"valueMarker"`
}

type Entry struct {
	ID             string
	CampaignID     string
	UserID         string
	AdminAccountID string
	Sub2apiUserID  string
	MaskedEmail    string
	ReceiptToken   string
	ReceiptHash    string
	Status         string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	WithdrawnAt    *time.Time
}

type Winner struct {
	ID             string    `json:"id"`
	CampaignID     string    `json:"campaignId"`
	PrizeID        string    `json:"prizeId"`
	DrawID         string    `json:"drawId"`
	UserID         string    `json:"-"`
	AdminAccountID string    `json:"-"`
	EntryID        string    `json:"entryId"`
	Sub2apiUserID  string    `json:"sub2apiUserId,omitempty"`
	MaskedEmail    string    `json:"maskedEmail"`
	PrizeSlot      int       `json:"prizeSlot"`
	CreatedAt      time.Time `json:"-"`
}

type RewardJob struct {
	ID             string
	CampaignID     string
	WinnerID       string
	PrizeID        string
	UserID         string
	AdminAccountID string
	Status         string
	AttemptCount   int
	NextAttemptAt  time.Time
	LockedAt       *time.Time
	ErrorKey       string
	ErrorDetail    string
	RemoteRef      string
	IdempotencyKey string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	FulfilledAt    *time.Time
	Winner         Winner
	Prize          Prize
}

// RateCleanupJob 是已成功设置用户专属倍率、等待到期回收的奖励任务视图。
// 清理状态独立于 RewardJob.Status，避免到期维护影响已经公开为“已发放”的中奖结果。
type RateCleanupJob struct {
	RewardJob
	CleanupAttemptCount int
	CleanupAt           time.Time
}

// RateCleanupReplacement 表示同一用户、同一分组仍然有效的另一份抽奖倍率奖励。
// 较早奖励到期时恢复此倍率，而不是直接删除仍受另一份奖励保护的专属倍率。
type RateCleanupReplacement struct {
	RewardJobID string
	Multiplier  string
	CleanupAt   time.Time
}

type AuditLog struct {
	ID             string
	CampaignID     string
	UserID         string
	AdminAccountID string
	ActorType      string
	ActorID        string
	Event          string
	Detail         map[string]any
	CreatedAt      time.Time
}

type Sub2APIUser struct {
	ID     string
	Email  string
	Role   string
	Status string
}

type EmbedSession struct {
	UserID             string
	AdminAccountID     string
	EmbedToken         string
	SrcHost            string
	SrcURL             string
	Sub2apiUserID      string
	Sub2apiEmailMasked string
	Sub2apiRole        string
	CreatedAt          time.Time
}

type CreateCampaignRequest struct {
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	RegistrationStart string         `json:"registrationStart"`
	RegistrationEnd   string         `json:"registrationEnd"`
	DrawAt            string         `json:"drawAt"`
	DrawMode          string         `json:"drawMode"`
	PublicWinners     bool           `json:"publicWinners"`
	Prizes            []PrizeRequest `json:"prizes"`
}

type UpdateCampaignRequest = CreateCampaignRequest

type PrizeRequest struct {
	Type          string   `json:"type"`
	Name          string   `json:"name"`
	Quantity      int      `json:"quantity"`
	SortOrder     int      `json:"sortOrder"`
	BalanceAmount string   `json:"balanceAmount"`
	GroupID       string   `json:"groupId"`
	GroupName     string   `json:"groupName"`
	Multiplier    string   `json:"multiplier"`
	ValidityDays  *int     `json:"validityDays"`
	DeliveryMode  string   `json:"deliveryMode"`
	ManualContact string   `json:"manualContact"`
	VoucherCodes  []string `json:"voucherCodes"`
}

type CreateSessionRequest struct {
	EmbedToken   string `json:"embedToken"`
	Sub2apiToken string `json:"sub2apiToken"`
	ViewerToken  string `json:"viewerToken"`
	SrcHost      string `json:"srcHost"`
	SrcURL       string `json:"srcUrl"`
	UrlUserID    string `json:"userId"`
}

type CreateSessionResponse struct {
	SessionToken string `json:"sessionToken"`
}

type EmbedConfigResponse struct {
	EmbedToken          string `json:"embedToken"`
	Sub2apiSourceOrigin string `json:"sub2apiSourceOrigin"`
	CreatedAt           string `json:"createdAt"`
	UpdatedAt           string `json:"updatedAt"`
}

// SubscriptionGroupResponse 是创建订阅奖品时可选择的当前 Sub2API 分组。
// Multiplier 使用十进制字符串，保存草稿时可原样写入 numeric 字段，避免前端浮点格式化引入额外误差。
type SubscriptionGroupResponse struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Multiplier string `json:"multiplier"`
}

type ListSubscriptionGroupsResponse struct {
	Items []SubscriptionGroupResponse `json:"items"`
}

type CampaignResponse struct {
	ID                string           `json:"id"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	Status            string           `json:"status"`
	RegistrationStart string           `json:"registrationStart,omitempty"`
	RegistrationEnd   string           `json:"registrationEnd,omitempty"`
	DrawAt            string           `json:"drawAt,omitempty"`
	DrawMode          string           `json:"drawMode"`
	PublicWinners     bool             `json:"publicWinners"`
	SeedCommitment    string           `json:"seedCommitment,omitempty"`
	EntrySnapshotHash string           `json:"entrySnapshotHash,omitempty"`
	RevealedSeed      string           `json:"revealedSeed,omitempty"`
	AlgorithmVersion  string           `json:"algorithmVersion"`
	EntryCount        int              `json:"entryCount"`
	WinnerCount       int              `json:"winnerCount"`
	Prizes            []Prize          `json:"prizes"`
	Entries           []EntryResponse  `json:"entries,omitempty"`
	Winners           []WinnerResponse `json:"winners,omitempty"`
	RewardStatuses    []RewardStatus   `json:"rewardStatuses,omitempty"`
	MyEntry           *EntryResponse   `json:"myEntry,omitempty"`
	MyWinner          *WinnerResponse  `json:"myWinner,omitempty"`
	MyRewardStatus    *MyRewardStatus  `json:"myRewardStatus,omitempty"`
	CreatedAt         string           `json:"createdAt"`
	UpdatedAt         string           `json:"updatedAt"`
}

type WinnerResponse struct {
	ID          string `json:"id"`
	PrizeID     string `json:"prizeId"`
	EntryID     string `json:"entryId"`
	MaskedEmail string `json:"maskedEmail"`
	PrizeSlot   int    `json:"prizeSlot"`
}

type RewardStatus struct {
	ID          string `json:"id"`
	WinnerID    string `json:"winnerId"`
	PrizeID     string `json:"prizeId"`
	Status      string `json:"status"`
	ErrorKey    string `json:"errorKey,omitempty"`
	ErrorDetail string `json:"errorDetail,omitempty"`
}

type MyRewardStatus struct {
	ID            string `json:"id"`
	WinnerID      string `json:"winnerId"`
	PrizeID       string `json:"prizeId"`
	Status        string `json:"status"`
	ErrorKey      string `json:"errorKey,omitempty"`
	DeliveryMode  string `json:"deliveryMode"`
	VoucherCode   string `json:"voucherCode,omitempty"`
	ManualContact string `json:"manualContact,omitempty"`
}

type EntryResponse struct {
	ID          string `json:"id"`
	CampaignID  string `json:"campaignId"`
	MaskedEmail string `json:"maskedEmail"`
	ReceiptHash string `json:"receiptHash"`
	Status      string `json:"status"`
	CreatedAt   string `json:"createdAt"`
}

type ListCampaignsResponse struct {
	Items []CampaignResponse `json:"items"`
}

type ListEntriesResponse struct {
	Items []EntryResponse `json:"items"`
}

type AuditResponse struct {
	Items []AuditLog `json:"items"`
}
