package connection_health

import (
	"context"
	"log"
	"strings"

	"transithub/backend/internal/modules/upstream"
)

const (
	RemoteActionSkippedTargetConflict          = "skipped_target_conflict"
	RemoteActionSkippedTargetInitiallyDisabled = "skipped_target_initially_disabled"
)

// reconcileTargetRemoteAction 把同一账号当前仍启用的全部模型状态聚合成一次上游动作。
// 模型仍独立记录健康，但账号/渠道是共享资源，不能让后执行的健康模型覆盖先前故障模型的停用决定。
func (s *Service) reconcileTargetRemoteAction(
	ctx context.Context,
	userID string,
	adminAccountID string,
	session upstream.Session,
	target AdminProbeTarget,
	specs []probeModelSpec,
) (string, error) {
	controlledModels := make(map[string]struct{})
	for _, spec := range specs {
		if spec.policy.Enabled && policyRemoteActionEnabled(spec.policy) {
			controlledModels[spec.modelName] = struct{}{}
		}
	}
	if len(controlledModels) == 0 {
		return "", nil
	}

	allStates, err := s.repo.ListStatesByConnection(ctx, target.TargetID)
	if err != nil {
		return "", err
	}
	states := make([]ConnectionHealthState, 0, len(controlledModels))
	for _, state := range allStates {
		if _, active := controlledModels[state.ModelName]; active {
			states = append(states, state)
		}
	}
	if len(states) == 0 {
		return "", nil
	}
	statesComplete := len(states) == len(controlledModels)

	stored, err := s.repo.GetTargetActionState(ctx, userID, adminAccountID, target.TargetID)
	if err != nil {
		return "", err
	}
	allHealthy, blocked, minWeight := aggregateTargetStates(states)
	allHealthy = allHealthy && statesComplete
	// 普通 degraded 只记录模型健康；只有已经接管或进入暂停/观察/恢复阶段时才修改上游。
	if stored == nil && (!statesComplete || (!blocked && !hasRecoveringState(states))) {
		return "", nil
	}
	// 已接管目标只有在全部受控模型都有状态后才能开始恢复。缺失状态不能被当作健康，
	// 但如果已有模型明确进入暂停，仍需允许下面的 blocked 分支继续执行降级动作。
	if stored != nil && !statesComplete && !blocked {
		return "", nil
	}

	currentStatus := normalizeTargetStatus(target.Platform, target.AccountStatus)
	currentWeight := normalizedTargetWeight(target)
	if stored == nil {
		originalStatus := currentStatus
		originalWeight := cloneIntPointer(currentWeight)
		// 用户原本就在上游暂停的账号不属于自动恢复对象，探活可以继续，但绝不替用户启用。
		if !targetStatusEnabled(target.Platform, currentStatus) {
			if !legacyTargetWasManaged(states) {
				return RemoteActionSkippedTargetInitiallyDisabled, nil
			}
			// 升级前已由健康模块停用的目标没有动作快照。仅在历史 remote_action 能明确证明
			// 是系统执行的情况下，按旧默认 active/100 建立一次兼容快照。
			originalStatus, originalWeight = legacyOriginalTargetState(target.Platform)
		}
		stored = &TargetActionState{
			UserID: userID, AdminAccountID: adminAccountID, TargetID: target.TargetID,
			OriginalStatus: originalStatus, OriginalWeight: cloneIntPointer(originalWeight),
			LastAppliedStatus: currentStatus, LastAppliedWeight: cloneIntPointer(currentWeight),
		}
		if err := s.repo.UpsertTargetActionState(ctx, *stored); err != nil {
			return "", err
		}
	} else if targetActionCheckpointConflicted(target, stored, currentStatus, currentWeight) {
		stored.Conflict = true
		stored.PendingStatus = ""
		stored.PendingWeight = nil
		if err := s.repo.UpsertTargetActionState(ctx, *stored); err != nil {
			return "", err
		}
		return RemoteActionSkippedTargetConflict, nil
	}
	if stored.Conflict {
		return RemoteActionSkippedTargetConflict, nil
	}

	desiredStatus, desiredWeight := desiredTargetState(target.Platform, allHealthy, blocked, minWeight, *stored)
	if targetStateEqual(target, currentStatus, currentWeight, desiredStatus, desiredWeight) {
		stored.LastAppliedStatus = desiredStatus
		stored.LastAppliedWeight = cloneIntPointer(desiredWeight)
		stored.PendingStatus = ""
		stored.PendingWeight = nil
		if allHealthy {
			return "", s.repo.DeleteTargetActionState(ctx, userID, adminAccountID, target.TargetID)
		}
		return "", s.repo.UpsertTargetActionState(ctx, *stored)
	}

	// Persist the intended value before touching the upstream. A later database failure can
	// then be recognized as a completed system write instead of a manual conflict.
	stored.PendingStatus = desiredStatus
	stored.PendingWeight = cloneIntPointer(desiredWeight)
	if err := s.repo.UpsertTargetActionState(ctx, *stored); err != nil {
		return "", err
	}
	action, actionErr := s.dispatcher.ApplyTargetState(ctx, session, target, desiredWeight, desiredStatus)
	if actionErr != nil {
		log.Printf("[connection-health] aggregate target action failed target_id=%s action=%s err=%v", target.TargetID, action, actionErr)
		return action, actionErr
	}
	stored.LastAppliedStatus = desiredStatus
	stored.LastAppliedWeight = cloneIntPointer(desiredWeight)
	stored.PendingStatus = ""
	stored.PendingWeight = nil
	if allHealthy {
		return action, s.repo.DeleteTargetActionState(ctx, userID, adminAccountID, target.TargetID)
	}
	return action, s.repo.UpsertTargetActionState(ctx, *stored)
}

// restoreUnmanagedTargetActions 恢复已经失去有效自动动作策略的目标。用户解绑分组、禁用策略、
// 删除最后一个模型或把目标加入排除列表后，都不能把此前由系统暂停的账号永久留在上游。
func (s *Service) restoreUnmanagedTargetActions(
	ctx context.Context,
	policies []Policy,
	targetAssignments []PolicyAssignment,
	groupAssignments []GroupPolicyAssignment,
	exclusions []GroupTargetExclusion,
	states []TargetActionState,
	inventoryCache adminInventoryCache,
) {
	if len(states) == 0 {
		return
	}
	targetPolicies := assignedEnabledPoliciesByTarget(policies, targetAssignments)
	groupPolicies := assignedEnabledPoliciesByGroup(policies, groupAssignments)
	excluded := groupTargetExclusionIndex(exclusions)
	for _, stored := range states {
		inventory, err := s.loadAdminInventory(ctx, stored.UserID, stored.AdminAccountID, inventoryCache)
		if err != nil {
			log.Printf("[connection-health] restore unmanaged target inventory failed target_id=%s err=%v", stored.TargetID, err)
			continue
		}
		inventoryComplete := true
		for _, groupInventory := range inventory.groups {
			if groupInventory.err != nil {
				inventoryComplete = false
				break
			}
		}
		if !inventoryComplete {
			// 任一分组成员读取失败时无法证明目标已经失去全部管理关系，保持当前状态更安全。
			continue
		}
		var target AdminProbeTarget
		found := false
		effectivePolicies := append([]Policy(nil), targetPolicies[stored.UserID+"|"+stored.AdminAccountID][stored.TargetID]...)
		for _, groupInventory := range inventory.groups {
			if groupInventory.err != nil {
				continue
			}
			for _, account := range groupInventory.accounts {
				targetID := buildTargetID(string(inventory.session.Platform), stored.AdminAccountID, account.ID)
				if targetID != stored.TargetID {
					continue
				}
				if !found {
					target = AdminProbeTarget{
						TargetID: targetID, Platform: string(inventory.session.Platform),
						AdminGroupID: groupInventory.group.ID, AdminGroupName: groupInventory.group.Name,
						AccountID: account.ID, AccountName: account.Name, AccountStatus: account.Status,
						AccountWeight: cloneIntPointer(account.Weight), ProviderFamily: account.Platform,
						Models: splitModelList(account.Models),
					}
					found = true
				}
				workspaceKey := stored.UserID + "|" + stored.AdminAccountID
				if !excluded[workspaceKey][groupInventory.group.ID][targetID] {
					effectivePolicies = mergePoliciesByID(effectivePolicies, groupPolicies[workspaceKey][groupInventory.group.ID])
				}
			}
		}
		if hasRemoteActionModel(candidateModelSpecs(target.Models, effectivePolicies)) {
			continue
		}
		targetVisible := found
		if !found {
			parsed, ok := parseTargetID(stored.TargetID)
			if !ok || parsed.adminAccountID != stored.AdminAccountID || parsed.platform != string(inventory.session.Platform) {
				continue
			}
			// The account can remain upstream after being removed from every group. We no longer
			// have a list snapshot for conflict detection, but restoring the captured original
			// value is safer than leaving a system-disabled account stuck forever.
			target = AdminProbeTarget{
				TargetID: stored.TargetID, Platform: parsed.platform, AccountID: parsed.accountID,
				AccountStatus: stored.LastAppliedStatus, AccountWeight: cloneIntPointer(stored.LastAppliedWeight),
			}
		}
		currentStatus := normalizeTargetStatus(target.Platform, target.AccountStatus)
		currentWeight := normalizedTargetWeight(target)
		if stored.Conflict || (targetVisible && targetActionCheckpointConflicted(target, &stored, currentStatus, currentWeight)) {
			stored.Conflict = true
			stored.PendingStatus = ""
			stored.PendingWeight = nil
			if err := s.repo.UpsertTargetActionState(ctx, stored); err != nil {
				log.Printf("[connection-health] store unmanaged target conflict failed target_id=%s err=%v", stored.TargetID, err)
			}
			continue
		}
		if targetVisible && targetStateEqual(target, currentStatus, currentWeight, stored.OriginalStatus, stored.OriginalWeight) {
			if err := s.repo.DeleteTargetActionState(ctx, stored.UserID, stored.AdminAccountID, stored.TargetID); err != nil {
				log.Printf("[connection-health] clear restored target action state failed target_id=%s err=%v", stored.TargetID, err)
			}
			continue
		}
		stored.PendingStatus = stored.OriginalStatus
		stored.PendingWeight = cloneIntPointer(stored.OriginalWeight)
		if err := s.repo.UpsertTargetActionState(ctx, stored); err != nil {
			log.Printf("[connection-health] store unmanaged target restore intent failed target_id=%s err=%v", stored.TargetID, err)
			continue
		}
		action, actionErr := s.dispatcher.ApplyTargetState(ctx, inventory.session, target, stored.OriginalWeight, stored.OriginalStatus)
		if actionErr != nil {
			log.Printf("[connection-health] restore unmanaged target failed target_id=%s action=%s err=%v", stored.TargetID, action, actionErr)
			continue
		}
		s.recordTargetEvent(ctx, stored.UserID, stored.AdminAccountID, target, "", "*", "policy_unmanaged_restore", "", "", nil, "", "", action)
		if err := s.repo.DeleteTargetActionState(ctx, stored.UserID, stored.AdminAccountID, stored.TargetID); err != nil {
			log.Printf("[connection-health] clear unmanaged target action state failed target_id=%s err=%v", stored.TargetID, err)
		}
	}
}

func hasRemoteActionModel(specs []probeModelSpec) bool {
	for _, spec := range specs {
		if spec.policy.Enabled && policyRemoteActionEnabled(spec.policy) {
			return true
		}
	}
	return false
}

func legacyTargetWasManaged(states []ConnectionHealthState) bool {
	for _, state := range states {
		switch state.LastRemoteAction {
		case RemoteActionSub2APIStatusInactive, "newapi_channel_disabled":
			return true
		}
	}
	return false
}

func legacyOriginalTargetState(platform string) (string, *int) {
	if platform == string(upstream.PlatformNewAPI) {
		weight := 100
		return "1", &weight
	}
	return "active", nil
}

func aggregateTargetStates(states []ConnectionHealthState) (allHealthy bool, blocked bool, minWeight int) {
	allHealthy = true
	minWeight = 100
	for _, state := range states {
		if state.State != StateHealthy {
			allHealthy = false
		}
		if state.CurrentWeight < minWeight {
			minWeight = state.CurrentWeight
		}
		if state.State == StateSuspended || state.State == StateObserving || state.State == StateDisabled || state.CurrentWeight <= 0 {
			blocked = true
		}
	}
	return allHealthy, blocked, minWeight
}

func hasRecoveringState(states []ConnectionHealthState) bool {
	for _, state := range states {
		if state.State == StateRecovering {
			return true
		}
	}
	return false
}

func desiredTargetState(platform string, allHealthy bool, blocked bool, minWeight int, stored TargetActionState) (string, *int) {
	if allHealthy {
		return stored.OriginalStatus, cloneIntPointer(stored.OriginalWeight)
	}
	if platform == string(upstream.PlatformNewAPI) {
		if blocked {
			weight := 0
			return "2", &weight
		}
		weight := scaledTargetWeight(stored.OriginalWeight, minWeight)
		return "1", &weight
	}
	if blocked {
		return "inactive", nil
	}
	return "active", nil
}

// scaledTargetWeight converts the state machine's 0-100 recovery percentage into the
// channel's real weight. Writing the percentage directly could increase traffic for a
// channel whose original weight was below the current recovery percentage.
func scaledTargetWeight(originalWeight *int, percentage int) int {
	base := 100
	if originalWeight != nil {
		base = maxInt(0, *originalWeight)
	}
	percentage = maxInt(0, minInt(100, percentage))
	if base == 0 || percentage == 0 {
		return 0
	}
	// Round up so a positive original weight receives at least one unit during recovery.
	return (base*percentage + 99) / 100
}

func normalizeTargetStatus(platform string, status string) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	if platform == string(upstream.PlatformNewAPI) {
		// New API 只有状态 1 表示启用；2（手动禁用）以及未来/其它非 1 状态都按禁用保护。
		// 空值仅用于兼容旧上游和测试夹具未返回 status 的情况。
		if normalized == "" || normalized == "1" || normalized == "active" || normalized == "enabled" {
			return "1"
		}
		return "2"
	}
	if normalized == "inactive" || normalized == "disabled" || normalized == "2" {
		return "inactive"
	}
	return "active"
}

func targetStatusEnabled(platform string, status string) bool {
	if platform == string(upstream.PlatformNewAPI) {
		return status == "1"
	}
	return status == "active"
}

func normalizedTargetWeight(target AdminProbeTarget) *int {
	if target.Platform != string(upstream.PlatformNewAPI) {
		return nil
	}
	if target.AccountWeight != nil {
		return cloneIntPointer(target.AccountWeight)
	}
	weight := 100
	return &weight
}

func targetActionConflicted(target AdminProbeTarget, stored TargetActionState, currentStatus string, currentWeight *int) bool {
	if currentStatus != normalizeTargetStatus(target.Platform, stored.LastAppliedStatus) {
		return true
	}
	// 老版本/部分上游列表可能不返回 weight；缺失时只比较状态，不能凭空制造人工冲突。
	if target.Platform == string(upstream.PlatformNewAPI) && target.AccountWeight != nil {
		return !equalIntPointers(currentWeight, stored.LastAppliedWeight)
	}
	return false
}

// targetActionCheckpointConflicted reconciles the two-phase action checkpoint. A current
// value matching Pending means the previous upstream write succeeded but its final database
// acknowledgement did not. A value matching neither Pending nor LastApplied is a real manual
// conflict and must not be overwritten.
func targetActionCheckpointConflicted(target AdminProbeTarget, stored *TargetActionState, currentStatus string, currentWeight *int) bool {
	if stored.PendingStatus == "" {
		return targetActionConflicted(target, *stored, currentStatus, currentWeight)
	}
	if targetStateEqual(target, currentStatus, currentWeight, stored.PendingStatus, stored.PendingWeight) {
		stored.LastAppliedStatus = stored.PendingStatus
		stored.LastAppliedWeight = cloneIntPointer(stored.PendingWeight)
		stored.PendingStatus = ""
		stored.PendingWeight = nil
		return false
	}
	return targetActionConflicted(target, *stored, currentStatus, currentWeight)
}

func targetStateEqual(target AdminProbeTarget, currentStatus string, currentWeight *int, desiredStatus string, desiredWeight *int) bool {
	if currentStatus != normalizeTargetStatus(target.Platform, desiredStatus) {
		return false
	}
	if target.Platform == string(upstream.PlatformNewAPI) && target.AccountWeight != nil {
		return equalIntPointers(currentWeight, desiredWeight)
	}
	return target.Platform != string(upstream.PlatformNewAPI) || target.AccountWeight == nil
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func equalIntPointers(left *int, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
