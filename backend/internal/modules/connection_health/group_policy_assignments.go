package connection_health

import (
	"context"
	"sort"
	"strings"

	"transithub/backend/internal/modules/upstream"
)

// AdminGroupPolicyConfiguration 是简化配置抽屉使用的分组级策略响应。ExcludedTargetIDs 只表示
// 不继承分组策略的例外目标；目标自己的旧版显式策略分配不受影响。
type AdminGroupPolicyConfiguration struct {
	AdminGroupID      string                  `json:"adminGroupId"`
	AdminGroupName    string                  `json:"adminGroupName"`
	PolicyIDs         []string                `json:"policyIds"`
	Policies          []AssignedPolicySummary `json:"policies"`
	ExcludedTargetIDs []string                `json:"excludedTargetIds"`
}

type AdminGroupPolicyConfigurationInput struct {
	PolicyIDs         []string `json:"policyIds"`
	ExcludedTargetIDs []string `json:"excludedTargetIds"`
	// QuickPolicy 仅供首次启用向导使用；后端在同一事务中创建策略并完成分组绑定。
	// 旧客户端不传该字段时完全沿用原有 policyIds 行为。
	QuickPolicy *PolicyInput `json:"quickPolicy,omitempty"`
}

// adminGroupContext 是保存分组配置时从上游实时解析出的可信上下文。客户端只提交 groupId 和
// targetId；分组名、平台和账号集合一律以后端读取结果为准，避免跨 workspace/分组注入。
type adminGroupContext struct {
	adminAccountID string
	group          upstream.AdminGroupInfo
	targetIDs      map[string]struct{}
}

func (s *Service) resolveAdminGroupContext(ctx context.Context, userID string, adminGroupID string) (adminGroupContext, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return adminGroupContext{}, err
	}
	if s.platformGroups == nil {
		return adminGroupContext{}, requestError(ErrorUnknown)
	}
	session, err := s.mySites.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		return adminGroupContext{}, err
	}
	groups, err := s.platformGroups.FetchAdminAllGroups(session)
	if err != nil {
		return adminGroupContext{}, err
	}
	for _, group := range groups {
		if group.ID != adminGroupID {
			continue
		}
		accounts, err := s.platformGroups.ListAdminGroupAccounts(session, group)
		if err != nil {
			return adminGroupContext{}, requestError(ErrorAccountsFetch)
		}
		targetIDs := make(map[string]struct{}, len(accounts))
		for _, account := range accounts {
			targetIDs[buildTargetID(string(session.Platform), adminAccountID, account.ID)] = struct{}{}
		}
		return adminGroupContext{adminAccountID: adminAccountID, group: group, targetIDs: targetIDs}, nil
	}
	return adminGroupContext{}, requestError(ErrorNotFound)
}

func (s *Service) GetAdminGroupPolicyConfiguration(ctx context.Context, userID string, adminGroupID string) (AdminGroupPolicyConfiguration, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	assignments, err := s.repo.ListGroupPolicyAssignmentsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	exclusions, err := s.repo.ListGroupTargetExclusionsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	policies, err := s.repo.ListPolicies(ctx, userID, adminAccountID)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	return buildAdminGroupPolicyConfiguration(adminGroupID, assignments, exclusions, policies), nil
}

func (s *Service) SetAdminGroupPolicyConfiguration(ctx context.Context, userID string, adminGroupID string, input AdminGroupPolicyConfigurationInput) (AdminGroupPolicyConfiguration, error) {
	groupContext, err := s.resolveAdminGroupContext(ctx, userID, adminGroupID)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}

	policyIDs, err := s.validateWorkspacePolicyIDs(ctx, userID, groupContext.adminAccountID, input.PolicyIDs)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	excludedTargetIDs := dedupeStrings(input.ExcludedTargetIDs)
	for _, targetID := range excludedTargetIDs {
		parsed, ok := parseTargetID(targetID)
		if !ok || parsed.adminAccountID != groupContext.adminAccountID {
			return AdminGroupPolicyConfiguration{}, requestError(ErrorProbeTargetNotFound)
		}
		if _, belongsToGroup := groupContext.targetIDs[targetID]; !belongsToGroup {
			return AdminGroupPolicyConfiguration{}, requestError(ErrorProbeTargetNotFound)
		}
	}
	// 暂时从上游分组消失的目标仍保留原排除项，防止它重新出现时意外继承自动动作策略。
	existingExclusions, err := s.repo.ListGroupTargetExclusionsByWorkspace(ctx, userID, groupContext.adminAccountID)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	for _, exclusion := range existingExclusions {
		if exclusion.AdminGroupID != groupContext.group.ID {
			continue
		}
		if _, currentlyPresent := groupContext.targetIDs[exclusion.TargetID]; !currentlyPresent {
			excludedTargetIDs = append(excludedTargetIDs, exclusion.TargetID)
		}
	}
	excludedTargetIDs = dedupeStrings(excludedTargetIDs)
	responsePolicies, err := s.repo.ListPolicies(ctx, userID, groupContext.adminAccountID)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	activeTargetIDs := make(map[string]struct{}, len(groupContext.targetIDs))
	excludedSet := make(map[string]struct{}, len(excludedTargetIDs))
	for _, targetID := range excludedTargetIDs {
		excludedSet[targetID] = struct{}{}
	}
	for targetID := range groupContext.targetIDs {
		if _, excluded := excludedSet[targetID]; !excluded {
			activeTargetIDs[targetID] = struct{}{}
		}
	}

	if input.QuickPolicy != nil {
		policyID, genErr := newID()
		if genErr != nil {
			return AdminGroupPolicyConfiguration{}, genErr
		}
		policy, targets, buildErr := buildPolicyAndTargets(userID, groupContext.adminAccountID, policyID, *input.QuickPolicy)
		if buildErr != nil {
			return AdminGroupPolicyConfiguration{}, buildErr
		}
		policyIDs = append(policyIDs, policyID)
		if err := s.repo.CreatePolicyAndReplaceGroupConfiguration(
			ctx, policy, targets, groupContext.group.ID, groupContext.group.Name,
			policyIDs, excludedTargetIDs, sortedStringSet(activeTargetIDs),
		); err != nil {
			return AdminGroupPolicyConfiguration{}, err
		}
		responsePolicies = append(responsePolicies, policy)
		return savedAdminGroupPolicyConfiguration(groupContext.group, policyIDs, excludedTargetIDs, responsePolicies), nil
	}

	if err := s.repo.ReplaceGroupPolicyConfiguration(
		ctx, userID, groupContext.adminAccountID, groupContext.group.ID, groupContext.group.Name,
		policyIDs, excludedTargetIDs, sortedStringSet(activeTargetIDs),
	); err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	return savedAdminGroupPolicyConfiguration(groupContext.group, policyIDs, excludedTargetIDs, responsePolicies), nil
}

// Build the mutation response from values already validated before the transaction. Querying
// again after commit can turn a successful write into an HTTP error and make a client retry
// create a duplicate quick policy.
func savedAdminGroupPolicyConfiguration(group upstream.AdminGroupInfo, policyIDs []string, excludedTargetIDs []string, policies []Policy) AdminGroupPolicyConfiguration {
	policyByID := make(map[string]Policy, len(policies))
	for _, policy := range policies {
		policyByID[policy.ID] = policy
	}
	policyIDs, summaries := assignedPolicySummariesFromIDs(dedupeStrings(policyIDs), policyByID)
	return AdminGroupPolicyConfiguration{
		AdminGroupID: group.ID, AdminGroupName: group.Name,
		PolicyIDs: policyIDs, Policies: summaries, ExcludedTargetIDs: dedupeStrings(excludedTargetIDs),
	}
}

// sortedStringSet 为事务中的冲突清理提供稳定的 targetId 参数顺序，便于测试和日志排查。
func sortedStringSet(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (s *Service) validateWorkspacePolicyIDs(ctx context.Context, userID string, adminAccountID string, rawPolicyIDs []string) ([]string, error) {
	policyIDs := dedupeStrings(rawPolicyIDs)
	for _, policyID := range policyIDs {
		policy, err := s.repo.GetPolicy(ctx, policyID, userID, adminAccountID)
		if err != nil {
			return nil, err
		}
		if policy == nil {
			return nil, requestError(ErrorPolicyNotFound)
		}
	}
	return policyIDs, nil
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func buildAdminGroupPolicyConfiguration(adminGroupID string, assignments []GroupPolicyAssignment, exclusions []GroupTargetExclusion, policies []Policy) AdminGroupPolicyConfiguration {
	policyByID := make(map[string]Policy, len(policies))
	for _, policy := range policies {
		policyByID[policy.ID] = policy
	}
	result := AdminGroupPolicyConfiguration{
		AdminGroupID: adminGroupID, PolicyIDs: []string{}, Policies: []AssignedPolicySummary{}, ExcludedTargetIDs: []string{},
	}
	for _, assignment := range assignments {
		if assignment.AdminGroupID != adminGroupID {
			continue
		}
		result.AdminGroupName = assignment.AdminGroupName
		result.PolicyIDs = append(result.PolicyIDs, assignment.PolicyID)
		if policy, ok := policyByID[assignment.PolicyID]; ok {
			result.Policies = append(result.Policies, AssignedPolicySummary{
				PolicyID: policy.ID, PolicyName: policy.Name, Enabled: policy.Enabled,
				PriorityMode: normalizePriorityMode(policy.PriorityMode), AutoRemoteActionEnabled: policyRemoteActionEnabled(policy),
			})
		} else {
			result.Policies = append(result.Policies, AssignedPolicySummary{PolicyID: assignment.PolicyID})
		}
	}
	for _, exclusion := range exclusions {
		if exclusion.AdminGroupID == adminGroupID {
			result.ExcludedTargetIDs = append(result.ExcludedTargetIDs, exclusion.TargetID)
		}
	}
	return result
}
