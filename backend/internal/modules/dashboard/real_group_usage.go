package dashboard

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

func (s *MetricsService) realGroupUsageToday(ctx context.Context, userID, adminAccountID string, session upstream.Session, date string) (GroupUsageTodayResponse, error) {
	runID, runErr := metricsRandomID()
	if runErr != nil {
		return GroupUsageTodayResponse{}, runErr
	}
	quality := &GroupProfitQuality{Status: "unavailable", BusinessDate: date, ObservedAt: time.Now().Format(time.RFC3339), RunID: runID}
	response := GroupUsageTodayResponse{Date: date, Quality: quality}
	groups, err := s.platform.FetchAdminAllGroups(session)
	if err != nil {
		status, retryable := profitErrorMeta(err)
		response.Issues = append(response.Issues, ProfitIssue{Code: "main_groups_failed", Source: "main_admin", Stage: "groups", HTTPStatus: status, Retryable: retryable, Detail: safeProfitError(err)})
		response.ProfitUnavailableReason = "main_groups_failed"
		annotateProfitIssues(&response, runID, quality.ObservedAt)
		return response, nil
	}

	groupByID := make(map[string]upstream.AdminGroupInfo, len(groups))
	for _, group := range groups {
		groupID := strings.TrimSpace(group.ID)
		if groupID != "" {
			groupByID[groupID] = group
		}
	}

	connections, err := s.realConnections.ListRealConnectionsForWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		status, retryable := profitErrorMeta(err)
		response.Issues = append(response.Issues, ProfitIssue{Code: "real_connections_failed", Source: "real_connections", Stage: "binding", HTTPStatus: status, Retryable: retryable, Detail: safeProfitError(err)})
		response.ProfitUnavailableReason = "real_connections_failed"
		annotateProfitIssues(&response, runID, quality.ObservedAt)
		return response, nil
	}
	active := make([]my_sites.RealConnection, 0, len(connections))
	for _, connection := range connections {
		if strings.TrimSpace(connection.Status) == "active" {
			active = append(active, connection)
		} else {
			response.Issues = append(response.Issues, ProfitIssue{Code: "real_connection_inactive", Source: "real_connections", Stage: "binding", ConnectionID: connection.ID, GroupID: strings.Join(normalizedIDs(connection.OwnGroupIDs), ",")})
		}
	}
	quality.ExpectedConnections = len(active)

	// Validate current main group membership once per group before taking any money.
	membership := make(map[string]map[string]bool)
	membershipFailed := make(map[string]bool)
	for _, connection := range active {
		groupIDs := normalizedIDs(connection.OwnGroupIDs)
		if len(groupIDs) != 1 {
			continue
		}
		groupID := groupIDs[0]
		if _, known := groupByID[groupID]; !known {
			response.Issues = append(response.Issues, ProfitIssue{Code: "main_group_missing", Source: "main_admin", Stage: "membership", GroupID: groupID, ConnectionID: connection.ID})
			membershipFailed[groupID] = true
			continue
		}
		if status := strings.TrimSpace(groupByID[groupID].Status); status != "" && !strings.EqualFold(status, "active") {
			response.Issues = append(response.Issues, ProfitIssue{Code: "main_group_inactive", Source: "main_admin", Stage: "membership", GroupID: groupID, ConnectionID: connection.ID, Detail: status})
			membershipFailed[groupID] = true
			continue
		}
		if _, loaded := membership[groupID]; loaded || membershipFailed[groupID] {
			continue
		}
		accounts, accountErr := s.platform.ListAdminGroupAccounts(session, groupByID[groupID])
		if accountErr != nil {
			membershipFailed[groupID] = true
			status, retryable := profitErrorMeta(accountErr)
			response.Issues = append(response.Issues, ProfitIssue{Code: "main_group_membership_failed", Source: "main_admin", Stage: "membership", GroupID: groupID, HTTPStatus: status, Retryable: retryable, Detail: safeProfitError(accountErr)})
			continue
		}
		membership[groupID] = make(map[string]bool, len(accounts))
		for _, account := range accounts {
			if accountID := strings.TrimSpace(account.ID); accountID != "" {
				membership[groupID][accountID] = true
			}
		}
	}

	eligible := make([]my_sites.RealConnection, 0, len(active))
	revenueByGroup := make(map[string]float64)
	connectionRevenue := make(map[string]float64)
	revenueQueried := make(map[string]bool)
	boundGroupIDs := make(map[string]struct{})
	for _, connection := range active {
		groupIDs := normalizedIDs(connection.OwnGroupIDs)
		if len(groupIDs) != 1 {
			continue
		}
		groupID := groupIDs[0]
		boundGroupIDs[groupID] = struct{}{}
		accountID := strings.TrimSpace(connection.AdminAccountID)
		if membershipFailed[groupID] || !membership[groupID][accountID] {
			quality.FailedConnections++
			if !membershipFailed[groupID] {
				response.Issues = append(response.Issues, ProfitIssue{Code: "main_group_membership_changed", Source: "main_admin", Stage: "membership", GroupID: groupID, AccountID: accountID, ConnectionID: connection.ID})
			}
			continue
		}
		eligible = append(eligible, connection)
		cacheKey := accountID + "\x00" + groupID
		if revenueQueried[cacheKey] {
			continue
		}
		revenueQueried[cacheKey] = true
		stats, statsErr := s.platform.FetchAdminUsageStatsForScope(session, accountID, groupID, date, date)
		if statsErr != nil {
			quality.FailedConnections++
			status, retryable := profitErrorMeta(statsErr)
			response.Issues = append(response.Issues, ProfitIssue{Code: "usage_stats_failed", Source: "main_admin", Stage: "revenue", GroupID: groupID, AccountID: accountID, ConnectionID: connection.ID, HTTPStatus: status, Retryable: retryable, Detail: safeProfitError(statsErr)})
			continue
		}
		revenueByGroup[groupID] += stats.TotalActualCost
		connectionRevenue[connection.ID] = stats.TotalActualCost
	}
	// Keep the revenue view complete for groups without a real binding. Profit
	// attribution still uses only the stable real_connection closure above.
	for _, group := range groups {
		groupID := strings.TrimSpace(group.ID)
		if groupID == "" {
			continue
		}
		if _, exists := revenueByGroup[groupID]; exists {
			continue
		}
		// A group with any stable real binding must not fall back to group-only
		// revenue after its account-scoped query fails; that would break the
		// account-to-group attribution boundary and create a false profit.
		if _, bound := boundGroupIDs[groupID]; bound {
			continue
		}
		stats, statsErr := s.platform.FetchAdminUsageStatsForScope(session, "", groupID, date, date)
		if statsErr == nil {
			revenueByGroup[groupID] = stats.TotalActualCost
		}
	}

	costByKey := make(map[string]float64)
	if s.upstreams == nil {
		response.Issues = append(response.Issues, ProfitIssue{Code: "upstream_cost_unavailable", Source: "upstream", Stage: "cost"})
	} else {
		costResponse, costErr := s.upstreamKeyUsageTodayForDate(ctx, userID, date, true)
		for _, item := range costResponse.Keys {
			key := strings.TrimSpace(item.SiteID) + "\x00" + strings.TrimSpace(item.KeyID)
			costByKey[key] = item.TodayAmount
		}
		if costErr != nil {
			status, retryable := profitErrorMeta(costErr)
			response.Issues = append(response.Issues, ProfitIssue{Code: "upstream_cost_failed", Source: "upstream", Stage: "cost", HTTPStatus: status, Retryable: retryable, Detail: safeProfitError(costErr)})
		}
	}

	allocation := allocateRealConnectionProfit(eligible, revenueByGroup, costByKey)
	response.Issues = append(response.Issues, allocation.Issues...)
	if allocation.UnboundCost > 0 {
		response.UnboundUpstreamCost = floatPtr(allocation.UnboundCost)
		response.Issues = append(response.Issues, ProfitIssue{Code: "unbound_upstream_cost", Source: "upstream", Stage: "cost", Detail: fmt.Sprintf("%.2f", allocation.UnboundCost)})
	}

	response.Groups = make([]GroupUsageTodayItem, 0, len(groups))
	for _, group := range groups {
		groupID := strings.TrimSpace(group.ID)
		if groupID == "" {
			continue
		}
		name := strings.TrimSpace(group.Name)
		revenue, hasRevenue := revenueByGroup[groupID]
		item := GroupUsageTodayItem{GroupID: groupID, GroupName: name, Status: ProfitAllocationUnavailable}
		if hasRevenue {
			item.TodayAmount = revenue
			item.TodayRevenue = revenue
			response.TotalRevenue += revenue
		}
		if allocated, ok := allocation.Groups[groupID]; ok {
			item.Status = allocated.Status
			item.TodayCost = allocated.Cost
			item.TodayProfit = allocated.Profit
		}
		for _, issue := range response.Issues {
			if issue.GroupID == groupID || strings.Contains(","+issue.GroupID+",", ","+groupID+",") {
				item.Issues = append(item.Issues, issue)
			}
		}
		for _, connection := range active {
			if ids := normalizedIDs(connection.OwnGroupIDs); len(ids) == 1 && ids[0] == groupID {
				connectionItem := GroupProfitConnection{ConnectionID: connection.ID, AccountID: connection.AdminAccountID, GroupID: groupID, SiteID: connection.UpstreamSiteID, KeyID: connection.UpstreamKeyID, Status: item.Status}
				if value, ok := connectionRevenue[connection.ID]; ok {
					connectionItem.Revenue = floatPtr(value)
				}
				if allocated, ok := allocation.Connections[connection.ID]; ok {
					connectionItem.Status = allocated.Status
					connectionItem.Cost = allocated.Cost
					connectionItem.Profit = allocated.Profit
				}
				item.Connections = append(item.Connections, connectionItem)
			}
		}
		response.Groups = append(response.Groups, item)
	}

	response.Total = response.TotalRevenue
	if allocation.ResolvedConnections > 0 {
		quality.ResolvedConnections = allocation.ResolvedConnections
	}
	quality.UnallocatableConnections = allocation.UnallocatableConnections
	quality.FailedConnections += allocation.FailedConnections
	if quality.ExpectedConnections > 0 && quality.ResolvedConnections == quality.ExpectedConnections && !hasBlockingProfitIssue(response.Issues) {
		quality.Status = "exact"
		response.ProfitAvailable = true
		var totalCost float64
		for _, group := range response.Groups {
			if group.TodayCost != nil {
				totalCost += *group.TodayCost
			}
		}
		response.TotalCost = floatPtr(totalCost)
		response.TotalProfit = floatPtr(response.TotalRevenue - totalCost)
	} else if quality.ResolvedConnections > 0 {
		quality.Status = "partial"
		response.ProfitAvailable = true
		response.ProfitUnavailableReason = "partial_real_connections"
	} else if quality.ExpectedConnections > 0 {
		quality.Status = "unavailable"
		response.ProfitUnavailableReason = "real_connection_profit_unavailable"
	} else {
		response.ProfitUnavailableReason = "no_active_real_connections"
	}
	annotateProfitIssues(&response, runID, quality.ObservedAt)
	log.Printf("dashboard group profit run_id=%s date=%s status=%s expected=%d resolved=%d unallocatable=%d failed=%d issues=%d", runID, date, quality.Status, quality.ExpectedConnections, quality.ResolvedConnections, quality.UnallocatableConnections, quality.FailedConnections, len(response.Issues))
	return response, nil
}

func annotateProfitIssues(response *GroupUsageTodayResponse, runID, observedAt string) {
	for index := range response.Issues {
		response.Issues[index].RunID = runID
		response.Issues[index].ObservedAt = observedAt
	}
}

func safeProfitError(err error) string {
	if err == nil {
		return ""
	}
	var requestErr *upstream.RequestError
	if errors.As(err, &requestErr) {
		return requestErr.MessageKey
	}
	return "request_failed"
}

func profitErrorMeta(err error) (status int, retryable bool) {
	var requestErr *upstream.RequestError
	if !errors.As(err, &requestErr) {
		return 0, true
	}
	status = requestErr.StatusCode
	return status, requestErr.MessageKey == upstream.ErrorRequest && (status == 0 || status >= http.StatusInternalServerError)
}

func hasBlockingProfitIssue(issues []ProfitIssue) bool {
	for _, issue := range issues {
		if issue.Code != "unbound_upstream_cost" {
			return true
		}
	}
	return false
}
