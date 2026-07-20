package group_rates

import "time"

// SnapshotGroup is the normalized group multiplier payload accepted from upstream.
// The HTTP server maps upstream's narrow callback DTO into this module-local type so
// group_rates does not import upstream internals or repository details from other modules.
type SnapshotGroup struct {
	ID         string
	Name       string
	Platform   *string
	Multiplier *float64
}

// RateRow is the list API shape consumed by the admin UI. Delta values compare the latest
// snapshot with the immediately previous snapshot for the same site/group/platform tuple.
type RateRow struct {
	SiteID             string    `json:"siteId"`
	SiteName           string    `json:"siteName"`
	GroupID            string    `json:"groupId"`
	GroupName          string    `json:"groupName"`
	Platform           string    `json:"platform"`
	Type               string    `json:"type"`
	Mapped             bool      `json:"mapped"`
	Connected          bool      `json:"connected"`
	PricingMapped      bool      `json:"pricingMapped"`
	Deleted            bool      `json:"deleted"`
	UpstreamMultiplier float64   `json:"upstreamMultiplier"`
	RechargeRate       float64   `json:"rechargeRate"`
	CurrentMultiplier  float64   `json:"currentMultiplier"`
	Delta              *float64  `json:"delta"`
	DeltaPercent       *float64  `json:"deltaPercent"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// ListQuery describes all list controls exposed by the admin page. PageSize is
// intentionally fixed by the handler to 30 for now, but keeping it explicit here
// makes repository pagination deterministic and easy to test.
type ListQuery struct {
	Page     int
	PageSize int
	Search   string
	Type     string
	Platform string
	Status   string
	Sort     string
}

// StatusCounts provides tab totals after search/type/platform filters but before
// the active status filter. Deleted rows are kept separate from active totals.
type StatusCounts struct {
	All      int `json:"all"`
	Mapped   int `json:"mapped"`
	Unmapped int `json:"unmapped"`
	Deleted  int `json:"deleted"`
}

type ListResult struct {
	Items        []RateRow    `json:"items"`
	Total        int          `json:"total"`
	Page         int          `json:"page"`
	PageSize     int          `json:"pageSize"`
	TotalPages   int          `json:"totalPages"`
	Types        []string     `json:"types"`
	Platforms    []string     `json:"platforms"`
	StatusCounts StatusCounts `json:"statusCounts"`
}

type UpdateTypeRequest struct {
	SiteID    string `json:"siteId"`
	GroupName string `json:"groupName"`
	Type      string `json:"type"`
}

type GroupRef struct {
	SiteID    string
	GroupName string
}

// HistoryRow is a multiplier record for one site/group/platform tuple. It includes the same
// change calculation as the list endpoint so charts can explain each snapshot transition.
type HistoryRow struct {
	ID                string    `json:"id"`
	SiteID            string    `json:"siteId"`
	SiteName          string    `json:"siteName"`
	GroupID           string    `json:"groupId"`
	GroupName         string    `json:"groupName"`
	Platform          string    `json:"platform"`
	Type              string    `json:"type"`
	Multiplier        float64   `json:"multiplier"`
	CurrentMultiplier float64   `json:"currentMultiplier"`
	Deleted           bool      `json:"deleted"`
	Delta             *float64  `json:"delta"`
	DeltaPercent      *float64  `json:"deltaPercent"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

type snapshotRecord struct {
	ID                 string
	UserID             string
	AdminAccountID     string
	SiteID             string
	SiteName           string
	GroupID            string
	GroupName          string
	Platform           string
	Type               string
	Mapped             bool
	PricingMapped      bool
	Deleted            bool
	Multiplier         float64
	RechargeRate       float64
	CreatedAt          time.Time
	LastSeenAt         time.Time
	PreviousMultiplier *float64
}

// latestGroupKey 用于标记已删除分组和判断倍率变化：记录某站点每个分组最新快照的 ID、倍率和删除状态。
type latestGroupKey struct {
	ID         string
	GroupID    string
	GroupName  string
	Deleted    bool
	Multiplier float64
}

type listRecords struct {
	Items        []snapshotRecord
	Total        int
	Types        []string
	Platforms    []string
	StatusCounts StatusCounts
}
