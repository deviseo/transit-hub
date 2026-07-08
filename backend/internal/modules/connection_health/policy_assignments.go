package connection_health

import (
	"context"
	"strings"
)

// 本文件实现「账号/channel 显式分配策略」管理：调度器只对已分配 enabled 策略的 target 自动探活
// （见 scheduler.go），未分配的 target 永不自动探活、不进策略探活事件列表。
// 分配关系落在独立的 connection_health_policy_assignments 表，不改动/不影响已有策略表语义。

// AssignedPolicySummary 是分配弹窗/账号弹窗展示用的策略摘要，不含任何敏感字段。
type AssignedPolicySummary struct {
	PolicyID   string `json:"policyId"`
	PolicyName string `json:"policyName"`
	Enabled    bool   `json:"enabled"`
}

// TargetPolicyAssignments 是分配管理接口的响应体：policyIds 供前端勾选态回填，
// policies 携带展示用的 name/enabled，避免前端另外拉一次策略详情做拼接。
type TargetPolicyAssignments struct {
	PolicyIDs []string                `json:"policyIds"`
	Policies  []AssignedPolicySummary `json:"policies"`
}

// validateManualTargetID 只做「targetId 归属当前 workspace」的结构性校验（parseTargetID +
// adminAccountID 比对），不解析凭据、不打上游 API，供策略分配管理这类不需要真实探活的
// 轻量接口使用。真正探活/模型发现需要更完整的 resolveManualTarget（见 admin_targets.go）。
func (s *Service) validateManualTargetID(ctx context.Context, userID string, targetID string) (string, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return "", err
	}
	parsed, ok := parseTargetID(targetID)
	if !ok || parsed.adminAccountID != adminAccountID {
		return "", requestError(ErrorProbeTargetNotFound)
	}
	return adminAccountID, nil
}

// GetTargetPolicyAssignments 返回某个 target 在当前 workspace 下已分配的策略列表。
func (s *Service) GetTargetPolicyAssignments(ctx context.Context, userID string, targetID string) (TargetPolicyAssignments, error) {
	adminAccountID, err := s.validateManualTargetID(ctx, userID, targetID)
	if err != nil {
		return TargetPolicyAssignments{}, err
	}
	assignments, err := s.repo.ListPolicyAssignmentsForTarget(ctx, userID, adminAccountID, targetID)
	if err != nil {
		return TargetPolicyAssignments{}, err
	}
	policies, err := s.repo.ListPolicies(ctx, userID, adminAccountID)
	if err != nil {
		return TargetPolicyAssignments{}, err
	}
	return buildTargetPolicyAssignments(assignments, policies), nil
}

// SetTargetPolicyAssignments 整体替换某个 target 的策略分配。policyIds 必须全部属于当前
// workspace 且真实存在，否则拒绝（不允许分配跨 workspace 或已删除的策略）。policyIds 为空
// 表示清空该 target 的全部分配。
func (s *Service) SetTargetPolicyAssignments(ctx context.Context, userID string, targetID string, policyIDs []string) (TargetPolicyAssignments, error) {
	adminAccountID, err := s.validateManualTargetID(ctx, userID, targetID)
	if err != nil {
		return TargetPolicyAssignments{}, err
	}

	seen := make(map[string]struct{}, len(policyIDs))
	deduped := make([]string, 0, len(policyIDs))
	for _, id := range policyIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		policy, getErr := s.repo.GetPolicy(ctx, trimmed, userID, adminAccountID)
		if getErr != nil {
			return TargetPolicyAssignments{}, getErr
		}
		if policy == nil {
			return TargetPolicyAssignments{}, requestError(ErrorPolicyNotFound)
		}
		deduped = append(deduped, trimmed)
	}

	if err := s.repo.ReplacePolicyAssignments(ctx, userID, adminAccountID, targetID, deduped); err != nil {
		return TargetPolicyAssignments{}, err
	}
	return s.GetTargetPolicyAssignments(ctx, userID, targetID)
}

// buildTargetPolicyAssignments 把分配行 + 当前 workspace 全量策略拼装成响应体，
// 即使策略已被停用也要能展示名字（用 ListPolicies 而不是 ListEnabledPolicies）。
func buildTargetPolicyAssignments(assignments []PolicyAssignment, policies []Policy) TargetPolicyAssignments {
	policyByID := make(map[string]Policy, len(policies))
	for _, p := range policies {
		policyByID[p.ID] = p
	}
	ids := make([]string, 0, len(assignments))
	summaries := make([]AssignedPolicySummary, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.PolicyID)
		if p, ok := policyByID[a.PolicyID]; ok {
			summaries = append(summaries, AssignedPolicySummary{PolicyID: p.ID, PolicyName: p.Name, Enabled: p.Enabled})
		} else {
			// 策略行已被删除但分配未清理（理论上不应发生，ReplacePolicyAssignments 有校验）：
			// 仍然把 id 透出，名字留空，避免因为一个脏数据行整体隐藏分配信息。
			summaries = append(summaries, AssignedPolicySummary{PolicyID: a.PolicyID})
		}
	}
	return TargetPolicyAssignments{PolicyIDs: ids, Policies: summaries}
}

// assignedEnabledPoliciesByTarget 把当前已启用的策略 + 全部分配行归拢成
// wsKey(userID|adminAccountID) -> targetID -> []Policy 的索引，供调度器（scheduler.go）和
// 事件过滤共用。只保留分配指向的策略确实存在于 enabledPolicies 中的行，被禁用/已删除的
// 策略对应的分配会被自然过滤掉（调度器不会为其生成任务）。
func assignedEnabledPoliciesByTarget(enabledPolicies []Policy, assignments []PolicyAssignment) map[string]map[string][]Policy {
	policyByID := make(map[string]Policy, len(enabledPolicies))
	for _, p := range enabledPolicies {
		policyByID[p.ID] = p
	}
	result := make(map[string]map[string][]Policy)
	for _, a := range assignments {
		policy, ok := policyByID[a.PolicyID]
		if !ok {
			continue
		}
		wsKey := a.UserID + "|" + a.AdminAccountID
		byTarget, ok := result[wsKey]
		if !ok {
			byTarget = make(map[string][]Policy)
			result[wsKey] = byTarget
		}
		byTarget[a.TargetID] = append(byTarget[a.TargetID], policy)
	}
	return result
}
