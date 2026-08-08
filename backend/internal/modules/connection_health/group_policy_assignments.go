package connection_health

import (
	"context"
	"math"
	"sort"
	"strings"

	"transithub/backend/internal/modules/upstream"
)

// AdminGroupPolicyConfiguration 是简化配置抽屉使用的分组级策略响应。ExcludedTargetIDs 只表示
// 不继承分组策略的例外目标；目标自己的旧版显式策略分配不受影响。
type AdminGroupPolicyConfiguration struct {
	AdminGroupID                string                  `json:"adminGroupId"`
	AdminGroupName              string                  `json:"adminGroupName"`
	PolicyIDs                   []string                `json:"policyIds"`
	Policies                    []AssignedPolicySummary `json:"policies"`
	ExcludedTargetIDs           []string                `json:"excludedTargetIds"`
	ProbeSortFallbackMultiplier *float64                `json:"probeSortFallbackMultiplier,omitempty"`
}

type AdminGroupPolicyConfigurationInput struct {
	PolicyIDs                   []string `json:"policyIds"`
	ExcludedTargetIDs           []string `json:"excludedTargetIds"`
	ProbeSortFallbackMultiplier *float64 `json:"probeSortFallbackMultiplier"`
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
	settings, err := s.repo.ListGroupProbeSortSettings(ctx, userID, adminAccountID)
	if err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	result := buildAdminGroupPolicyConfiguration(adminGroupID, assignments, exclusions, policies)
	result.ProbeSortFallbackMultiplier = fallbackMultiplierForGroup(settings, adminGroupID)
	return result, nil
}

func (s *Service) SetAdminGroupPolicyConfiguration(ctx context.Context, userID string, adminGroupID string, input AdminGroupPolicyConfigurationInput) (AdminGroupPolicyConfiguration, error) {
	if input.ProbeSortFallbackMultiplier != nil && (*input.ProbeSortFallbackMultiplier <= 0 || math.IsNaN(*input.ProbeSortFallbackMultiplier) || math.IsInf(*input.ProbeSortFallbackMultiplier, 0)) {
		return AdminGroupPolicyConfiguration{}, requestError(ErrorMultiplierRequired)
	}
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
	if groupContext.group.Multiplier == nil && groupConfigurationUsesMultiplier(policyIDs, input.QuickPolicy, responsePolicies) {
		// 倍率为空时拒绝启用倍率策略，避免旧客户端绕过前端提示后创建一个看似生效、实际
		// 无法安全计算的配置。解除倍率策略绑定仍然允许，便于用户从错误配置中退出。
		return AdminGroupPolicyConfiguration{}, requestError(ErrorMultiplierRequired)
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
			policyIDs, excludedTargetIDs, sortedStringSet(activeTargetIDs), input.ProbeSortFallbackMultiplier,
		); err != nil {
			return AdminGroupPolicyConfiguration{}, err
		}
		responsePolicies = append(responsePolicies, policy)
		// 配置变更只更新本地 pending；独立校验和写回循环负责主站读写。
		s.evaluateCurrentWorkspacePriorities(ctx, userID, groupContext.adminAccountID, priorityActionPolicy)
		return savedAdminGroupPolicyConfiguration(groupContext.group, policyIDs, excludedTargetIDs, responsePolicies, input.ProbeSortFallbackMultiplier), nil
	}

	if err := s.repo.ReplaceGroupPolicyConfiguration(
		ctx, userID, groupContext.adminAccountID, groupContext.group.ID, groupContext.group.Name,
		policyIDs, excludedTargetIDs, sortedStringSet(activeTargetIDs), input.ProbeSortFallbackMultiplier,
	); err != nil {
		return AdminGroupPolicyConfiguration{}, err
	}
	// 与首次向导保持一致：立即计算本地排序，但不把配置保存和主站读写绑在一起。
	s.evaluateCurrentWorkspacePriorities(ctx, userID, groupContext.adminAccountID, priorityActionPolicy)
	return savedAdminGroupPolicyConfiguration(groupContext.group, policyIDs, excludedTargetIDs, responsePolicies, input.ProbeSortFallbackMultiplier), nil
}

// groupConfigurationUsesMultiplier 判断本次将要保存的分组绑定是否包含启用中的倍率策略。
// QuickPolicy 尚未落库，需要直接检查输入；已有策略则只信任当前 workspace 查询结果。
func groupConfigurationUsesMultiplier(policyIDs []string, quickPolicy *PolicyInput, policies []Policy) bool {
	if quickPolicy != nil && normalizeStrategyMode(quickPolicy.StrategyMode) == StrategyModeMultiplierOnly {
		return true
	}
	selected := make(map[string]struct{}, len(policyIDs))
	for _, policyID := range policyIDs {
		selected[policyID] = struct{}{}
	}
	for _, policy := range policies {
		if _, ok := selected[policy.ID]; ok && policy.Enabled && normalizeStrategyMode(policy.StrategyMode) == StrategyModeMultiplierOnly {
			return true
		}
	}
	return false
}

// Build the mutation response from values already validated before the transaction. Querying
// again after commit can turn a successful write into an HTTP error and make a client retry
// create a duplicate quick policy.
func savedAdminGroupPolicyConfiguration(group upstream.AdminGroupInfo, policyIDs []string, excludedTargetIDs []string, policies []Policy, fallbackMultiplier *float64) AdminGroupPolicyConfiguration {
	policyByID := make(map[string]Policy, len(policies))
	for _, policy := range policies {
		policyByID[policy.ID] = policy
	}
	policyIDs, summaries := assignedPolicySummariesFromIDs(dedupeStrings(policyIDs), policyByID)
	return AdminGroupPolicyConfiguration{
		AdminGroupID: group.ID, AdminGroupName: group.Name,
		PolicyIDs: policyIDs, Policies: summaries, ExcludedTargetIDs: dedupeStrings(excludedTargetIDs),
		ProbeSortFallbackMultiplier: cloneFloat64Pointer(fallbackMultiplier),
	}
}

func fallbackMultiplierForGroup(settings []GroupProbeSortSetting, adminGroupID string) *float64 {
	for _, setting := range settings {
		if setting.AdminGroupID == adminGroupID {
			return cloneFloat64Pointer(setting.FallbackMultiplier)
		}
	}
	return nil
}

func cloneFloat64Pointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
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
				PriorityMode: normalizePriorityMode(policy.PriorityMode), StrategyMode: normalizeStrategyMode(policy.StrategyMode),
				AutoRemoteActionEnabled: policyRemoteActionEnabled(policy),
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
