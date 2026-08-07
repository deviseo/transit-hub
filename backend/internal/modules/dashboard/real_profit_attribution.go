package dashboard

import (
	"strings"

	"transithub/backend/internal/modules/my_sites"
)

const (
	ProfitAllocationExact         = "exact"
	ProfitAllocationUnallocatable = "unallocatable"
	ProfitAllocationUnavailable   = "unavailable"
	ProfitIssueDuplicateBinding   = "duplicate_binding"
	ProfitIssueMultiGroup         = "multi_group_unallocatable"
	ProfitIssueKeyMissing         = "upstream_key_missing"
)

// ProfitIssue is a sanitized attribution failure returned to the dashboard.
type ProfitIssue struct {
	Code         string `json:"code"`
	Source       string `json:"source,omitempty"`
	Stage        string `json:"stage,omitempty"`
	ConnectionID string `json:"connectionId,omitempty"`
	SiteID       string `json:"siteId,omitempty"`
	KeyID        string `json:"keyId,omitempty"`
	AccountID    string `json:"accountId,omitempty"`
	GroupID      string `json:"groupId,omitempty"`
	HTTPStatus   int    `json:"httpStatus,omitempty"`
	Retryable    bool   `json:"retryable,omitempty"`
	Detail       string `json:"detail,omitempty"`
	ObservedAt   string `json:"observedAt,omitempty"`
	RunID        string `json:"runId,omitempty"`
}

type profitAllocationGroup struct {
	Cost   *float64
	Profit *float64
	Status string
}

type profitAllocationResult struct {
	Groups                   map[string]profitAllocationGroup
	Connections              map[string]profitAllocationGroup
	Issues                   []ProfitIssue
	UnboundCost              float64
	ResolvedConnections      int
	UnallocatableConnections int
	FailedConnections        int
}

// allocateRealConnectionProfit applies the strict V2.0.5 attribution boundary.
// It only allocates a key to one group when the persisted relationship is active,
// complete, and one-to-one. Unknown or ambiguous costs remain nil.
func allocateRealConnectionProfit(
	connections []my_sites.RealConnection,
	revenueByGroup map[string]float64,
	costByKey map[string]float64,
) profitAllocationResult {
	result := profitAllocationResult{
		Groups:      make(map[string]profitAllocationGroup, len(revenueByGroup)),
		Connections: make(map[string]profitAllocationGroup, len(connections)),
	}
	for groupID := range revenueByGroup {
		result.Groups[groupID] = profitAllocationGroup{Status: ProfitAllocationUnavailable}
	}

	keyOwners := make(map[string][]string)
	accountOwners := make(map[string][]string)
	validConnections := make(map[string]my_sites.RealConnection)
	for _, connection := range connections {
		if strings.TrimSpace(connection.Status) != "active" {
			continue
		}
		groupIDs := normalizedIDs(connection.OwnGroupIDs)
		if len(groupIDs) != 1 {
			for _, groupID := range groupIDs {
				result.Groups[groupID] = profitAllocationGroup{Status: ProfitAllocationUnallocatable}
			}
			result.Connections[connection.ID] = profitAllocationGroup{Status: ProfitAllocationUnallocatable}
			result.Issues = append(result.Issues, ProfitIssue{Code: ProfitIssueMultiGroup, Source: "real_connections", Stage: "binding", GroupID: strings.Join(groupIDs, ","), ConnectionID: connection.ID, AccountID: connection.AdminAccountID, SiteID: connection.UpstreamSiteID, KeyID: connection.UpstreamKeyID})
			result.UnallocatableConnections++
			continue
		}
		groupID := groupIDs[0]
		key := strings.TrimSpace(connection.UpstreamSiteID) + "\x00" + strings.TrimSpace(connection.UpstreamKeyID)
		if strings.TrimSpace(connection.UpstreamSiteID) == "" || strings.TrimSpace(connection.UpstreamKeyID) == "" || strings.TrimSpace(connection.AdminAccountID) == "" {
			result.Groups[groupID] = profitAllocationGroup{Status: ProfitAllocationUnavailable}
			result.Connections[connection.ID] = profitAllocationGroup{Status: ProfitAllocationUnavailable}
			result.Issues = append(result.Issues, ProfitIssue{Code: ProfitIssueKeyMissing, Source: "upstream", Stage: "key_cost", GroupID: groupID, ConnectionID: connection.ID, AccountID: connection.AdminAccountID, SiteID: connection.UpstreamSiteID, KeyID: connection.UpstreamKeyID})
			result.FailedConnections++
			continue
		}
		keyOwners[key] = append(keyOwners[key], connection.ID)
		accountOwners[strings.TrimSpace(connection.AdminAccountID)] = append(accountOwners[strings.TrimSpace(connection.AdminAccountID)], connection.ID)
		validConnections[key] = connection
	}

	conflictedConnections := make(map[string]struct{})
	for key, owners := range keyOwners {
		if len(owners) < 2 {
			continue
		}
		parts := strings.SplitN(key, "\x00", 2)
		issue := ProfitIssue{Code: ProfitIssueDuplicateBinding, Source: "real_connections", Stage: "binding", ConnectionID: strings.Join(owners, ",")}
		if len(parts) == 2 {
			issue.SiteID, issue.KeyID = parts[0], parts[1]
		}
		issue.GroupID = connectionGroups(connections, owners)
		result.Issues = append(result.Issues, issue)
		for _, ownerID := range owners {
			conflictedConnections[ownerID] = struct{}{}
		}
	}
	for accountID, owners := range accountOwners {
		if len(owners) < 2 {
			continue
		}
		result.Issues = append(result.Issues, ProfitIssue{Code: ProfitIssueDuplicateBinding, Source: "real_connections", Stage: "binding", AccountID: accountID, GroupID: connectionGroups(connections, owners), ConnectionID: strings.Join(owners, ",")})
		for _, ownerID := range owners {
			conflictedConnections[ownerID] = struct{}{}
		}
	}
	for _, connection := range connections {
		if _, conflicted := conflictedConnections[connection.ID]; !conflicted {
			continue
		}
		groupIDs := normalizedIDs(connection.OwnGroupIDs)
		if len(groupIDs) != 1 {
			continue
		}
		result.Groups[groupIDs[0]] = profitAllocationGroup{Status: ProfitAllocationUnallocatable}
		result.Connections[connection.ID] = profitAllocationGroup{Status: ProfitAllocationUnallocatable}
		result.UnallocatableConnections++
	}

	usedCosts := make(map[string]struct{})
	for _, connection := range validConnections {
		groupID := normalizedIDs(connection.OwnGroupIDs)[0]
		key := strings.TrimSpace(connection.UpstreamSiteID) + "\x00" + strings.TrimSpace(connection.UpstreamKeyID)
		if _, conflicted := conflictedConnections[connection.ID]; conflicted {
			continue
		}
		cost, ok := costByKey[key]
		if !ok {
			result.Groups[groupID] = profitAllocationGroup{Status: ProfitAllocationUnavailable}
			result.Connections[connection.ID] = profitAllocationGroup{Status: ProfitAllocationUnavailable}
			result.Issues = append(result.Issues, ProfitIssue{Code: ProfitIssueKeyMissing, Source: "upstream", Stage: "key_cost", GroupID: groupID, ConnectionID: connection.ID, AccountID: connection.AdminAccountID, SiteID: connection.UpstreamSiteID, KeyID: connection.UpstreamKeyID})
			result.FailedConnections++
			continue
		}
		revenue, revenueOK := revenueByGroup[groupID]
		if !revenueOK {
			result.Groups[groupID] = profitAllocationGroup{Status: ProfitAllocationUnavailable}
			result.Connections[connection.ID] = profitAllocationGroup{Status: ProfitAllocationUnavailable}
			continue
		}
		profit := revenue - cost
		allocated := profitAllocationGroup{Cost: floatPtr(cost), Profit: floatPtr(profit), Status: ProfitAllocationExact}
		result.Groups[groupID] = allocated
		result.Connections[connection.ID] = allocated
		result.ResolvedConnections++
		usedCosts[key] = struct{}{}
	}

	for key, cost := range costByKey {
		if _, used := usedCosts[key]; !used {
			result.UnboundCost += cost
		}
	}
	return result
}

func normalizedIDs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func connectionGroups(connections []my_sites.RealConnection, ownerIDs []string) string {
	seen := make(map[string]struct{})
	groups := make([]string, 0, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		for _, connection := range connections {
			if connection.ID != ownerID {
				continue
			}
			for _, groupID := range normalizedIDs(connection.OwnGroupIDs) {
				if _, exists := seen[groupID]; exists {
					continue
				}
				seen[groupID] = struct{}{}
				groups = append(groups, groupID)
			}
		}
	}
	return strings.Join(groups, ",")
}

func hasProfitIssue(issues []ProfitIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func floatPtr(value float64) *float64 { return &value }
