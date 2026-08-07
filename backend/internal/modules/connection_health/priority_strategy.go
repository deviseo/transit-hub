package connection_health

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"sort"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// TargetPriorityActioner 是倍率排序策略对 upstream 模块的唯一写依赖。真实实现根据 session
// 平台更新 New API channel 或 Sub2API account 的 priority，并由 upstream 模块保证字段级写入安全。
type TargetPriorityActioner interface {
	UpdateAdminTargetPriority(session upstream.Session, targetID string, priority int) error
}

type priorityTargetInventory struct {
	target              AdminProbeTarget
	account             upstream.AdminGroupAccountInfo
	policies            []Policy
	multipliers         []float64
	fallbackMultipliers []float64
	upstreamMultiplier  upstreamMultiplierResolution
	currentPriority     int
	priorityPresent     bool
}

type healthPriorityCandidate struct {
	targetID       string
	item           *priorityTargetInventory
	multiplier     float64
	states         []ConnectionHealthState
	expectedModels int
	healthBand     int
	latencyMs      *int
}

// syncMultiplierPriorities 在每轮探活前同步上游优先级。普通倍率策略仍然「健康优先、倍率次之」，
// 仅倍率策略则完全忽略探活状态。它故意与 job 生成分开，确保未到探活时间的目标也能更新顺序。
func (s *Service) syncMultiplierPriorities(
	ctx context.Context,
	policies []Policy,
	targetAssignments []PolicyAssignment,
	groupAssignments []GroupPolicyAssignment,
	exclusions []GroupTargetExclusion,
	allSyncStates []PrioritySyncState,
) {
	s.syncMultiplierPrioritiesWithCache(ctx, policies, targetAssignments, groupAssignments, exclusions, allSyncStates, make(adminInventoryCache))
}

func (s *Service) syncCurrentWorkspacePriorities(ctx context.Context, userID string, adminAccountID string) {
	if s.priorityActions == nil || s.platformGroups == nil {
		return
	}
	release, err := s.repo.AcquirePrioritySyncLease(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[connection-health] priority sync acquire workspace lease failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
		return
	}
	defer release()
	s.syncCurrentWorkspacePrioritiesLocked(ctx, userID, adminAccountID)
}

func (s *Service) syncCurrentWorkspacePrioritiesLocked(ctx context.Context, userID string, adminAccountID string) {
	policies, err := s.repo.ListPolicies(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[connection-health] formal probe priority sync list policies failed: %v", err)
		return
	}
	enabled := make([]Policy, 0, len(policies))
	for _, policy := range policies {
		if policy.Enabled {
			enabled = append(enabled, policy)
		}
	}
	assignments, err := s.repo.ListPolicyAssignmentsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[connection-health] formal probe priority sync list target assignments failed: %v", err)
		return
	}
	groupAssignments, err := s.repo.ListGroupPolicyAssignmentsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[connection-health] formal probe priority sync list group assignments failed: %v", err)
		return
	}
	exclusions, err := s.repo.ListGroupTargetExclusionsByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[connection-health] formal probe priority sync list exclusions failed: %v", err)
		return
	}
	states, err := s.repo.ListPrioritySyncStates(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[connection-health] formal probe priority sync list checkpoints failed: %v", err)
		return
	}
	s.syncMultiplierPrioritiesWithCacheLocked(ctx, enabled, assignments, groupAssignments, exclusions, states, make(adminInventoryCache))
}

func (s *Service) syncMultiplierPrioritiesWithCache(
	ctx context.Context,
	policies []Policy,
	targetAssignments []PolicyAssignment,
	groupAssignments []GroupPolicyAssignment,
	exclusions []GroupTargetExclusion,
	allSyncStates []PrioritySyncState,
	inventoryCache adminInventoryCache,
) {
	s.syncMultiplierPrioritiesWithCacheMode(ctx, policies, targetAssignments, groupAssignments, exclusions, allSyncStates, inventoryCache, true)
}

func (s *Service) syncMultiplierPrioritiesWithCacheLocked(
	ctx context.Context,
	policies []Policy,
	targetAssignments []PolicyAssignment,
	groupAssignments []GroupPolicyAssignment,
	exclusions []GroupTargetExclusion,
	allSyncStates []PrioritySyncState,
	inventoryCache adminInventoryCache,
) {
	s.syncMultiplierPrioritiesWithCacheMode(ctx, policies, targetAssignments, groupAssignments, exclusions, allSyncStates, inventoryCache, false)
}

func (s *Service) syncMultiplierPrioritiesWithCacheMode(
	ctx context.Context,
	policies []Policy,
	targetAssignments []PolicyAssignment,
	groupAssignments []GroupPolicyAssignment,
	exclusions []GroupTargetExclusion,
	allSyncStates []PrioritySyncState,
	inventoryCache adminInventoryCache,
	acquireWorkspaceLease bool,
) {
	if s.priorityActions == nil || s.platformGroups == nil {
		return
	}

	assignedTargets := assignedEnabledPoliciesByTarget(policies, targetAssignments)
	assignedGroups := assignedEnabledPoliciesByGroup(policies, groupAssignments)
	excluded := groupTargetExclusionIndex(exclusions)
	workspaceIdentity := make(map[string][2]string)
	for _, state := range allSyncStates {
		key := state.UserID + "|" + state.AdminAccountID
		workspaceIdentity[key] = [2]string{state.UserID, state.AdminAccountID}
	}
	for _, policy := range policies {
		key := policy.UserID + "|" + policy.AdminAccountID
		workspaceIdentity[key] = [2]string{policy.UserID, policy.AdminAccountID}
	}
	for _, assignment := range targetAssignments {
		key := assignment.UserID + "|" + assignment.AdminAccountID
		workspaceIdentity[key] = [2]string{assignment.UserID, assignment.AdminAccountID}
	}
	for _, assignment := range groupAssignments {
		key := assignment.UserID + "|" + assignment.AdminAccountID
		workspaceIdentity[key] = [2]string{assignment.UserID, assignment.AdminAccountID}
	}

	workspaceKeys := make([]string, 0, len(workspaceIdentity))
	for workspaceKey := range workspaceIdentity {
		workspaceKeys = append(workspaceKeys, workspaceKey)
	}
	sort.Strings(workspaceKeys)
	for _, workspaceKey := range workspaceKeys {
		identity := workspaceIdentity[workspaceKey]
		userID, adminAccountID := identity[0], identity[1]
		release := func() {}
		if acquireWorkspaceLease {
			var err error
			release, err = s.repo.AcquirePrioritySyncLease(ctx, userID, adminAccountID)
			if err != nil {
				log.Printf("[connection-health] priority sync acquire workspace lease failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
				continue
			}
		}
		func() {
			defer release()
			inventorySnapshot, err := s.loadAdminInventory(ctx, userID, adminAccountID, inventoryCache)
			if err != nil {
				log.Printf("[connection-health] priority sync load admin inventory failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
				return
			}
			session := inventorySnapshot.session
			settings, err := s.repo.ListGroupProbeSortSettings(ctx, userID, adminAccountID)
			if err != nil {
				log.Printf("[connection-health] priority sync list fallback multipliers failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
				return
			}
			fallbackByGroup := make(map[string]*float64, len(settings))
			for _, setting := range settings {
				fallbackByGroup[setting.AdminGroupID] = cloneFloat64Pointer(setting.FallbackMultiplier)
			}
			multiplierLookup := s.upstreamMultiplierResolutionsByAdminAccount(ctx, userID, adminAccountID, string(session.Platform))
			inventory, inventoryComplete, err := s.priorityInventoryForSnapshot(
				inventorySnapshot, adminAccountID, assignedTargets[workspaceKey], assignedGroups[workspaceKey], excluded[workspaceKey],
				fallbackByGroup, multiplierLookup,
			)
			if err != nil {
				log.Printf("[connection-health] priority sync inventory failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
				return
			}
			states, err := s.repo.ListStatesByWorkspace(ctx, userID, adminAccountID)
			if err != nil {
				log.Printf("[connection-health] priority sync list health states failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
				return
			}
			// Checkpoints must be read after acquiring the same workspace lease that covers
			// inventory reads and remote writes. A pre-lease snapshot can be stale even when
			// the write phase itself is serialized.
			syncStates, err := s.repo.ListPrioritySyncStates(ctx, userID, adminAccountID)
			if err != nil {
				log.Printf("[connection-health] priority sync list checkpoints failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
				return
			}
			s.syncWorkspacePriorities(ctx, session, userID, adminAccountID, inventory, inventoryComplete, states, syncStates)
		}()
	}
}

func (s *Service) priorityInventoryForSnapshot(
	snapshot *adminWorkspaceInventory,
	adminAccountID string,
	targetPolicies map[string][]Policy,
	groupPolicies map[string][]Policy,
	excludedByGroup map[string]map[string]bool,
	fallbackByGroup map[string]*float64,
	multiplierLookup upstreamMultiplierLookup,
) (map[string]*priorityTargetInventory, bool, error) {
	session := snapshot.session
	platform := string(session.Platform)
	inventory := make(map[string]*priorityTargetInventory)
	inventoryComplete := true
	for _, groupInventory := range snapshot.groups {
		group := groupInventory.group
		if groupInventory.err != nil {
			// 单个分组失败不阻断其它分组排序；目标如果只存在于失败分组，本轮保持原值。
			inventoryComplete = false
			log.Printf("[connection-health] priority sync group accounts failed group_id=%s err=%v", group.ID, groupInventory.err)
			continue
		}
		for _, account := range groupInventory.accounts {
			targetID := buildTargetID(platform, adminAccountID, account.ID)
			item := inventory[targetID]
			if item == nil {
				item = &priorityTargetInventory{
					target: AdminProbeTarget{
						TargetID: targetID, Platform: platform, AdminGroupID: group.ID, AdminGroupName: group.Name,
						AccountID: account.ID, AccountName: account.Name, AccountStatus: account.Status, AccountWeight: cloneIntPointer(account.Weight),
						ProviderFamily: account.Platform, Models: splitModelList(account.Models),
					},
					account: account,
				}
				if account.Priority != nil {
					item.currentPriority = *account.Priority
					item.priorityPresent = true
				}
				inventory[targetID] = item
			}
			item.upstreamMultiplier = resolutionForAdminAccount(multiplierLookup, account.ID)
			inherited := groupPolicies[group.ID]
			excluded := excludedByGroup[group.ID][targetID]
			if excluded {
				inherited = nil
			}
			// 倍率只来自目标实际参与策略继承的分组。先前在排除判断前收集倍率，会让已排除
			// 或无倍率策略的其它成员分组错误地压低当前目标优先级。
			explicitMultiplier := hasMultiplierPriorityPolicy(targetPolicies[targetID])
			inheritedMultiplier := !excluded && hasMultiplierPriorityPolicy(inherited)
			if group.Multiplier != nil && (explicitMultiplier || inheritedMultiplier) {
				item.multipliers = append(item.multipliers, *group.Multiplier)
			}
			explicitHealthMultiplier := hasHealthMultiplierPriorityPolicy(targetPolicies[targetID])
			inheritedHealthMultiplier := !excluded && hasHealthMultiplierPriorityPolicy(inherited)
			if fallback := fallbackByGroup[group.ID]; fallback != nil && (explicitHealthMultiplier || inheritedHealthMultiplier) {
				item.fallbackMultipliers = append(item.fallbackMultipliers, *fallback)
			}
			item.policies = mergePoliciesByID(item.policies, targetPolicies[targetID], inherited)
		}
	}
	return inventory, inventoryComplete, nil
}

type priorityWriteResult struct {
	attempts  int
	successes int
	failures  int
	drifts    int
	complete  bool
}

type priorityDriftObservation struct {
	active int
	new    int
}

type priorityWorkspacePlan struct {
	signature            string
	preset               PrioritySyncPreset
	managed              map[string]*priorityTargetInventory
	missingSortInput     bool
	missingPriorityInput bool
	inventoryComplete    bool
}

// syncWorkspacePriorities adds the B-phase gate around the existing per-target checkpoint
// writer. It deliberately builds the signature from ownership and stable production order only:
// latency values and health colors that do not change order cannot trigger an upstream write.
func (s *Service) syncWorkspacePriorities(
	ctx context.Context,
	session upstream.Session,
	userID string,
	adminAccountID string,
	inventory map[string]*priorityTargetInventory,
	inventoryComplete bool,
	healthStates []ConnectionHealthState,
	syncStates []PrioritySyncState,
) {
	plan := buildPriorityWorkspacePlan(session, inventory, inventoryComplete, healthStates)
	if session.Platform == upstream.PlatformSub2API && !plan.missingPriorityInput {
		for _, stored := range syncStates {
			if _, stillManaged := plan.managed[stored.TargetID]; stillManaged {
				continue
			}
			item := inventory[stored.TargetID]
			if item != nil && !item.priorityPresent {
				plan.missingPriorityInput = true
				break
			}
		}
	}
	state, err := s.repo.GetPriorityWorkspaceSyncState(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[connection-health] priority sync load workspace state failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, err)
		return
	}
	if state == nil {
		state = &PriorityWorkspaceSyncState{UserID: userID, AdminAccountID: adminAccountID}
	}
	now := s.prioritySyncNow()
	state.LastEvaluationAt = &now
	state.EvaluationCount++
	state.MinWriteIntervalSeconds = plan.preset.MinWriteIntervalSeconds
	state.MaxPendingAgeSeconds = plan.preset.MaxPendingAgeSeconds
	state.DriftAction = plan.preset.DriftAction
	state.ReadMode = plan.preset.ReadMode
	pendingOverdue := priorityPendingOverdue(*state, plan.preset, now)

	if !plan.inventoryComplete {
		state.LastDecision = "suppressed"
		state.LastSuppressionReason = "inventory_incomplete"
		state.LastError = priorityPendingError(pendingOverdue)
		s.persistPriorityWorkspaceSyncState(ctx, state)
		return
	}
	if plan.missingSortInput {
		state.LastDecision = "suppressed"
		state.LastSuppressionReason = "sort_input_unavailable"
		state.LastError = priorityPendingError(pendingOverdue)
		s.persistPriorityWorkspaceSyncState(ctx, state)
		return
	}
	if plan.missingPriorityInput {
		state.LastDecision = "suppressed"
		state.LastSuppressionReason = "priority_input_unavailable"
		state.LastError = priorityPendingError(pendingOverdue)
		s.persistPriorityWorkspaceSyncState(ctx, state)
		return
	}

	drift := s.observePriorityDrift(ctx, session, inventory, plan.managed, syncStates)
	if drift.new > 0 {
		refreshed, refreshErr := s.repo.ListPrioritySyncStates(ctx, userID, adminAccountID)
		if refreshErr != nil {
			state.LastDecision = "failed"
			state.LastSuppressionReason = ""
			state.LastError = "priority_checkpoint_read_failed"
			s.persistPriorityWorkspaceSyncState(ctx, state)
			return
		}
		syncStates = refreshed
	}
	if plan.signature == state.AppliedSignature {
		if state.PendingSignature != "" {
			state.PendingSignature = ""
			state.PendingSince = nil
			state.LastDecision = "signature_reverted"
			state.LastSuppressionReason = ""
		} else if drift.active > 0 {
			state.LastDecision = "drift_alert"
			state.LastSuppressionReason = "manual_priority_drift"
			state.LastDriftAt = &now
			state.DriftCount += int64(drift.new)
		} else {
			state.LastDecision = "skipped"
			state.LastSuppressionReason = "signature_unchanged"
			state.UnchangedSkipCount++
		}
		state.LastError = ""
		s.persistPriorityWorkspaceSyncState(ctx, state)
		return
	}

	if state.PendingSignature != plan.signature {
		if state.PendingSignature != "" {
			// Keep the original pending time: this is a write-rate window, not a
			// debounce that can postpone the latest state forever.
			state.SignatureChangeCount++
		} else {
			state.SignatureChangeCount++
			timeCopy := now
			state.PendingSince = &timeCopy
		}
		state.PendingSignature = plan.signature
	}

	if waitUntil, waiting := priorityWriteWaitUntil(*state, plan.preset); waiting && now.Before(waitUntil) {
		state.LastDecision = "pending"
		state.LastSuppressionReason = "min_write_interval"
		state.LastError = priorityPendingError(priorityPendingOverdue(*state, plan.preset, now))
		state.WindowSuppressionCount++
		s.persistPriorityWorkspaceSyncState(ctx, state)
		return
	}

	pendingOverdue = priorityPendingOverdue(*state, plan.preset, now)
	if pendingOverdue {
		state.LastError = "priority_pending_overdue"
	}
	result := s.applyWorkspacePriorities(ctx, session, userID, adminAccountID, inventory, inventoryComplete, healthStates, syncStates)
	if result.attempts > 0 {
		attemptedAt := now
		state.LastWriteAttemptAt = &attemptedAt
		state.WriteAttemptCount += int64(result.attempts)
	}
	state.WriteSuccessCount += int64(result.successes)
	state.WriteFailureCount += int64(result.failures)
	if result.drifts > 0 {
		state.LastDriftAt = &now
		state.DriftCount += int64(drift.new)
	}
	if !result.complete || result.failures > 0 {
		state.LastDecision = "failed"
		if pendingOverdue {
			state.LastSuppressionReason = "priority_pending_overdue"
		} else if result.drifts > 0 {
			state.LastSuppressionReason = "manual_priority_drift"
		} else {
			state.LastSuppressionReason = ""
		}
		state.LastError = "priority_write_failed"
		s.persistPriorityWorkspaceSyncState(ctx, state)
		return
	}
	if result.drifts > 0 {
		state.LastDecision = "drift_alert"
		state.LastSuppressionReason = "manual_priority_drift"
		if !pendingOverdue {
			state.LastError = ""
		}
		state.AppliedSignature = plan.signature
		state.PendingSignature = ""
		state.PendingSince = nil
		s.persistPriorityWorkspaceSyncState(ctx, state)
		return
	}

	state.AppliedSignature = plan.signature
	state.PendingSignature = ""
	state.PendingSince = nil
	if pendingOverdue {
		state.LastSuppressionReason = "priority_pending_overdue"
	} else {
		state.LastSuppressionReason = ""
	}
	state.LastError = ""
	if result.attempts == 0 {
		state.LastDecision = "observed_applied"
	} else {
		state.LastDecision = "applied"
		succeededAt := now
		state.LastWriteSuccessAt = &succeededAt
	}
	s.persistPriorityWorkspaceSyncState(ctx, state)
}

func (s *Service) prioritySyncNow() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) persistPriorityWorkspaceSyncState(ctx context.Context, state *PriorityWorkspaceSyncState) {
	if err := s.repo.UpsertPriorityWorkspaceSyncState(ctx, *state); err != nil {
		log.Printf("[connection-health] priority sync save workspace state failed user_id=%s admin_account_id=%s err=%v", state.UserID, state.AdminAccountID, err)
	}
}

func priorityWriteWaitUntil(state PriorityWorkspaceSyncState, preset PrioritySyncPreset) (time.Time, bool) {
	if state.PendingSince == nil {
		return time.Time{}, false
	}
	// The first managed write has no earlier write to protect, preserving the A-stage
	// immediate convergence after a policy save. Every following write is rate-limited.
	if state.LastWriteAttemptAt == nil && state.LastWriteSuccessAt == nil {
		return time.Time{}, false
	}
	interval := time.Duration(preset.MinWriteIntervalSeconds) * time.Second
	waitUntil := time.Time{}
	if state.LastWriteAttemptAt != nil && state.LastWriteAttemptAt.Add(interval).After(waitUntil) {
		waitUntil = state.LastWriteAttemptAt.Add(interval)
	}
	if state.LastWriteSuccessAt != nil && state.LastWriteSuccessAt.Add(interval).After(waitUntil) {
		waitUntil = state.LastWriteSuccessAt.Add(interval)
	}
	return waitUntil, true
}

func priorityPendingOverdue(state PriorityWorkspaceSyncState, preset PrioritySyncPreset, now time.Time) bool {
	return state.PendingSince != nil && now.Sub(*state.PendingSince) > time.Duration(preset.MaxPendingAgeSeconds)*time.Second
}

func priorityPendingError(overdue bool) string {
	if overdue {
		return "priority_pending_overdue"
	}
	return ""
}

func buildPriorityWorkspacePlan(
	session upstream.Session,
	inventory map[string]*priorityTargetInventory,
	inventoryComplete bool,
	healthStates []ConnectionHealthState,
) priorityWorkspacePlan {
	statesByTarget := make(map[string][]ConnectionHealthState)
	for _, state := range healthStates {
		if _, isTarget := parseTargetID(state.ConnectionID); isTarget {
			statesByTarget[state.ConnectionID] = append(statesByTarget[state.ConnectionID], state)
		}
	}
	managed := make(map[string]*priorityTargetInventory)
	missingSortInput := false
	missingPriorityInput := false
	multiplierOnlyTargets := make(map[string]float64)
	healthCandidates := make([]healthPriorityCandidate, 0)
	for targetID, item := range inventory {
		if !hasMultiplierPriorityPolicy(item.policies) {
			continue
		}
		if hasMultiplierOnlyPolicy(item.policies) {
			if len(item.multipliers) == 0 {
				missingSortInput = true
				continue
			}
			managed[targetID] = item
			if session.Platform == upstream.PlatformSub2API && !item.priorityPresent {
				missingPriorityInput = true
			}
			multiplierOnlyTargets[targetID] = minFloat(item.multipliers)
			continue
		}
		multiplier, available := effectiveHealthSortMultiplier(item)
		if !available {
			missingSortInput = true
			continue
		}
		activeModels := activeHealthPriorityModels(item)
		activeStates := activeHealthPriorityStates(statesByTarget[targetID], activeModels)
		managed[targetID] = item
		if session.Platform == upstream.PlatformSub2API && !item.priorityPresent {
			missingPriorityInput = true
		}
		healthCandidates = append(healthCandidates, healthPriorityCandidate{
			targetID: targetID, item: item, multiplier: multiplier, states: activeStates,
			expectedModels: len(activeModels), healthBand: priorityHealthBand(activeStates, len(activeModels)),
			latencyMs: completeTargetSuccessLatency(activeStates, activeModels),
		})
	}

	type signatureEntry struct {
		targetID string
		owner    string
		order    int
	}
	entries := make([]signatureEntry, 0, len(managed))
	multiplierTargetIDs := make([]string, 0, len(multiplierOnlyTargets))
	for targetID := range multiplierOnlyTargets {
		multiplierTargetIDs = append(multiplierTargetIDs, targetID)
	}
	sort.SliceStable(multiplierTargetIDs, func(i int, j int) bool {
		left, right := multiplierOnlyTargets[multiplierTargetIDs[i]], multiplierOnlyTargets[multiplierTargetIDs[j]]
		if left != right {
			return left < right
		}
		return multiplierTargetIDs[i] < multiplierTargetIDs[j]
	})
	for index, targetID := range multiplierTargetIDs {
		entries = append(entries, signatureEntry{targetID: targetID, owner: StrategyModeMultiplierOnly, order: index})
	}
	sortHealthPriorityCandidates(healthCandidates)
	for index, candidate := range healthCandidates {
		entries = append(entries, signatureEntry{targetID: candidate.targetID, owner: StrategyModeHealthProbe, order: index})
	}
	// The two priority ownership domains have disjoint platform ranges. Preserve their
	// production ordering while never serializing raw priority or latency into the signature.
	sort.SliceStable(entries, func(i int, j int) bool {
		if entries[i].owner != entries[j].owner {
			if session.Platform == upstream.PlatformSub2API {
				return entries[i].owner == StrategyModeMultiplierOnly
			}
			return entries[i].owner == StrategyModeHealthProbe
		}
		if entries[i].order != entries[j].order {
			return entries[i].order < entries[j].order
		}
		return entries[i].targetID < entries[j].targetID
	})
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, entry.owner+":"+entry.targetID)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return priorityWorkspacePlan{
		signature:            hex.EncodeToString(digest[:]),
		preset:               prioritySyncPresetForManagedTargets(managed),
		managed:              managed,
		missingSortInput:     missingSortInput,
		missingPriorityInput: missingPriorityInput,
		inventoryComplete:    inventoryComplete,
	}
}

func prioritySyncPresetForManagedTargets(managed map[string]*priorityTargetInventory) PrioritySyncPreset {
	preset := PrioritySyncPreset{
		MinWriteIntervalSeconds: defaultPriorityMinWriteIntervalSeconds,
		MaxPendingAgeSeconds:    defaultPriorityMaxPendingAgeSeconds,
		DriftAction:             PriorityDriftActionAlertOnly,
		ReadMode:                PriorityReadModeInventory,
	}
	hasCandidate := false
	for _, item := range managed {
		for _, policy := range item.policies {
			if !policy.Enabled || !hasMultiplierPriorityPolicy([]Policy{policy}) {
				continue
			}
			candidate, err := normalizePrioritySyncPreset(&policy.PrioritySyncPreset)
			if err != nil {
				continue
			}
			if !hasCandidate || candidate.MinWriteIntervalSeconds < preset.MinWriteIntervalSeconds {
				preset.MinWriteIntervalSeconds = candidate.MinWriteIntervalSeconds
			}
			if !hasCandidate || candidate.MaxPendingAgeSeconds < preset.MaxPendingAgeSeconds {
				preset.MaxPendingAgeSeconds = candidate.MaxPendingAgeSeconds
			}
			hasCandidate = true
		}
	}
	if !hasCandidate {
		return PrioritySyncPreset{
			MinWriteIntervalSeconds: defaultPriorityMinWriteIntervalSeconds,
			MaxPendingAgeSeconds:    defaultPriorityMaxPendingAgeSeconds,
			DriftAction:             PriorityDriftActionAlertOnly,
			ReadMode:                PriorityReadModeInventory,
		}
	}
	return preset
}

func (s *Service) observePriorityDrift(
	ctx context.Context,
	session upstream.Session,
	inventory map[string]*priorityTargetInventory,
	managed map[string]*priorityTargetInventory,
	syncStates []PrioritySyncState,
) priorityDriftObservation {
	observation := priorityDriftObservation{}
	for _, stored := range syncStates {
		item, isManaged := managed[stored.TargetID]
		if !isManaged || (session.Platform == upstream.PlatformSub2API && !item.priorityPresent) {
			continue
		}
		if stored.Conflict {
			observation.active++
			continue
		}
		if stored.PendingPriority != nil && item.currentPriority == *stored.PendingPriority {
			stored.LastAppliedPriority = *stored.PendingPriority
			stored.PendingPriority = nil
			if err := s.repo.UpsertPrioritySyncState(ctx, stored); err != nil {
				log.Printf("[connection-health] priority sync confirm checkpoint failed target_id=%s err=%v", stored.TargetID, err)
			}
			continue
		}
		if item.currentPriority == stored.LastAppliedPriority {
			continue
		}
		current := item.currentPriority
		stored.Conflict = true
		stored.LastConflictPriority = &current
		if err := s.repo.UpsertPrioritySyncState(ctx, stored); err != nil {
			log.Printf("[connection-health] priority drift checkpoint failed target_id=%s err=%v", stored.TargetID, err)
			continue
		}
		observation.active++
		observation.new++
	}
	return observation
}

func (s *Service) applyWorkspacePriorities(
	ctx context.Context,
	session upstream.Session,
	userID string,
	adminAccountID string,
	inventory map[string]*priorityTargetInventory,
	inventoryComplete bool,
	healthStates []ConnectionHealthState,
	syncStates []PrioritySyncState,
) priorityWriteResult {
	result := priorityWriteResult{complete: true}
	statesByTarget := make(map[string][]ConnectionHealthState)
	for _, state := range healthStates {
		if _, isTarget := parseTargetID(state.ConnectionID); isTarget {
			statesByTarget[state.ConnectionID] = append(statesByTarget[state.ConnectionID], state)
		}
	}

	managed := make(map[string]*priorityTargetInventory)
	missingMultiplier := make(map[string]struct{})
	effectiveMultiplierByTarget := make(map[string]float64)
	desiredByTarget := make(map[string]int)
	multiplierOnlyTargets := make(map[string]float64)
	healthCandidates := make([]healthPriorityCandidate, 0)
	for targetID, item := range inventory {
		if !hasMultiplierPriorityPolicy(item.policies) {
			continue
		}
		if hasMultiplierOnlyPolicy(item.policies) {
			if len(item.multipliers) == 0 {
				missingMultiplier[targetID] = struct{}{}
				continue
			}
			multiplier := minFloat(item.multipliers)
			managed[targetID] = item
			effectiveMultiplierByTarget[targetID] = multiplier
			multiplierOnlyTargets[targetID] = multiplier
			continue
		}

		multiplier, available := effectiveHealthSortMultiplier(item)
		if !available {
			// Deterministic missing/conflict without a fallback and transient lookup
			// failures both hold any existing checkpoint without guessing or restoring.
			missingMultiplier[targetID] = struct{}{}
			continue
		}
		activeModels := activeHealthPriorityModels(item)
		activeStates := activeHealthPriorityStates(statesByTarget[targetID], activeModels)
		candidate := healthPriorityCandidate{
			targetID: targetID, item: item, multiplier: multiplier, states: activeStates,
			expectedModels: len(activeModels), healthBand: priorityHealthBand(activeStates, len(activeModels)),
			latencyMs: completeTargetSuccessLatency(activeStates, activeModels),
		}
		managed[targetID] = item
		effectiveMultiplierByTarget[targetID] = multiplier
		healthCandidates = append(healthCandidates, candidate)
	}

	distinctMultiplierOnly := make([]float64, 0)
	seenMultiplierOnly := make(map[float64]struct{})
	for _, multiplier := range multiplierOnlyTargets {
		if _, exists := seenMultiplierOnly[multiplier]; !exists {
			seenMultiplierOnly[multiplier] = struct{}{}
			distinctMultiplierOnly = append(distinctMultiplierOnly, multiplier)
		}
	}
	sort.Float64s(distinctMultiplierOnly)
	multiplierOnlyRank := make(map[float64]int, len(distinctMultiplierOnly))
	for rank, multiplier := range distinctMultiplierOnly {
		multiplierOnlyRank[multiplier] = rank
	}
	for targetID, multiplier := range multiplierOnlyTargets {
		rank := multiplierOnlyRank[multiplier]
		desired := desiredManagedPriorityForPlatformWithExpected(session.Platform, nil, rank, 0)
		if session.Platform == upstream.PlatformSub2API {
			desired = desiredSub2APIMultiplierOnlyPriority(rank)
		}
		desiredByTarget[targetID] = desired
	}

	sortHealthPriorityCandidates(healthCandidates)
	currentBand, bandRank := -1, 0
	for _, candidate := range healthCandidates {
		if candidate.healthBand != currentBand {
			currentBand = candidate.healthBand
			bandRank = 0
		}
		desiredByTarget[candidate.targetID] = desiredHealthPriorityForPlatform(session.Platform, candidate.healthBand, bandRank)
		bandRank++
	}

	storedByTarget := make(map[string]PrioritySyncState, len(syncStates))
	for _, state := range syncStates {
		storedByTarget[state.TargetID] = state
	}

	for targetID, item := range managed {
		// A missing upstream priority is a real value, not zero. The existing
		// field-level priority API cannot clear a priority back to NULL safely,
		// so leave such targets untouched rather than materializing 0.
		if session.Platform == upstream.PlatformSub2API && !item.priorityPresent {
			continue
		}
		multiplier := effectiveMultiplierByTarget[targetID]
		desired := desiredByTarget[targetID]
		stored, exists := storedByTarget[targetID]
		if !exists {
			stored = PrioritySyncState{
				UserID: userID, AdminAccountID: adminAccountID, TargetID: targetID,
				OriginalPriority: item.currentPriority, LastAppliedPriority: item.currentPriority,
			}
		}
		if stored.Conflict {
			result.drifts++
			continue
		}
		if stored.PendingPriority != nil && item.currentPriority == *stored.PendingPriority {
			stored.LastAppliedPriority = *stored.PendingPriority
			stored.PendingPriority = nil
		}
		if exists && item.currentPriority != stored.LastAppliedPriority && stored.PendingPriority == nil {
			current := item.currentPriority
			stored.Conflict = true
			stored.LastConflictPriority = &current
			stored.EffectiveMultiplier = multiplier
			if err := s.repo.UpsertPrioritySyncState(ctx, stored); err != nil {
				log.Printf("[connection-health] priority conflict state save failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
			}
			result.drifts++
			continue
		}
		if exists && stored.PendingPriority != nil && item.currentPriority != stored.LastAppliedPriority {
			current := item.currentPriority
			stored.Conflict = true
			stored.LastConflictPriority = &current
			stored.EffectiveMultiplier = multiplier
			if err := s.repo.UpsertPrioritySyncState(ctx, stored); err != nil {
				log.Printf("[connection-health] priority pending conflict state save failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
			}
			result.drifts++
			continue
		}
		if item.currentPriority != desired {
			pending := desired
			stored.PendingPriority = &pending
			stored.EffectiveMultiplier = multiplier
			if err := s.repo.UpsertPrioritySyncState(ctx, stored); err != nil {
				log.Printf("[connection-health] priority sync intent save failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
				continue
			}
			result.attempts++
			if err := s.priorityActions.UpdateAdminTargetPriority(session, item.target.AccountID, desired); err != nil {
				log.Printf("[connection-health] priority sync update failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
				continue
			}
			result.successes++
		}
		stored.LastAppliedPriority = desired
		stored.PendingPriority = nil
		stored.EffectiveMultiplier = multiplier
		stored.Conflict = false
		stored.LastConflictPriority = nil
		if err := s.repo.UpsertPrioritySyncState(ctx, stored); err != nil {
			log.Printf("[connection-health] priority sync state save failed target_id=%s err=%v", targetID, err)
			result.failures++
			result.complete = false
		}
	}

	// 不再被任何倍率策略覆盖的目标恢复接管前优先级。若管理员已经人工改过，则保留人工值。
	for targetID, stored := range storedByTarget {
		if _, stillManaged := managed[targetID]; stillManaged {
			continue
		}
		if _, waitingForMultiplier := missingMultiplier[targetID]; waitingForMultiplier {
			continue
		}
		item := inventory[targetID]
		if session.Platform == upstream.PlatformSub2API && item != nil && !item.priorityPresent {
			// Preserve an upstream NULL priority and keep any prior checkpoint
			// for a later, explicit reconciliation once the value is readable.
			continue
		}
		if item == nil {
			if !inventoryComplete {
				// 分组读取失败时无法证明目标已经消失，保留当前优先级和同步快照，
				// 等下一次完整扫描再决定是否恢复。
				continue
			}
			if stored.Conflict {
				result.drifts++
				// 已确认目标不再受策略管理，但人工修改过的值不能被原始快照覆盖。
				if err := s.repo.DeletePrioritySyncState(ctx, userID, adminAccountID, targetID); err != nil {
					log.Printf("[connection-health] missing conflicted target priority state delete failed target_id=%s err=%v", targetID, err)
				}
				continue
			}
			parsed, ok := parseTargetID(targetID)
			if !ok || parsed.adminAccountID != adminAccountID || parsed.platform != string(session.Platform) {
				continue
			}
			pending := stored.OriginalPriority
			stored.PendingPriority = &pending
			if err := s.repo.UpsertPrioritySyncState(ctx, stored); err != nil {
				log.Printf("[connection-health] missing target priority restore intent save failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
				continue
			}
			result.attempts++
			if err := s.priorityActions.UpdateAdminTargetPriority(session, parsed.accountID, stored.OriginalPriority); err != nil {
				log.Printf("[connection-health] missing target priority restore failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
				continue
			}
			result.successes++
			if err := s.repo.DeletePrioritySyncState(ctx, userID, adminAccountID, targetID); err != nil {
				log.Printf("[connection-health] missing target priority state delete failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
			}
			continue
		}
		if stored.PendingPriority != nil && item.currentPriority == *stored.PendingPriority {
			stored.LastAppliedPriority = *stored.PendingPriority
			stored.PendingPriority = nil
		}
		if !stored.Conflict && item.currentPriority == stored.LastAppliedPriority && item.currentPriority != stored.OriginalPriority {
			pending := stored.OriginalPriority
			stored.PendingPriority = &pending
			if err := s.repo.UpsertPrioritySyncState(ctx, stored); err != nil {
				log.Printf("[connection-health] priority restore intent save failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
				continue
			}
			result.attempts++
			if err := s.priorityActions.UpdateAdminTargetPriority(session, item.target.AccountID, stored.OriginalPriority); err != nil {
				log.Printf("[connection-health] priority restore failed target_id=%s err=%v", targetID, err)
				result.failures++
				result.complete = false
				continue
			}
			result.successes++
		}
		if err := s.repo.DeletePrioritySyncState(ctx, userID, adminAccountID, targetID); err != nil {
			log.Printf("[connection-health] priority sync state delete failed target_id=%s err=%v", targetID, err)
			result.failures++
			result.complete = false
		}
	}
	return result
}

// desiredManagedPriorityForPlatform 按平台真实语义计算优先级：NewAPI 沿用「分数越高越优先」；
// Sub2API 使用紧凑的小数值状态分段，数值越小越优先。
func desiredManagedPriorityForPlatform(platform upstream.Platform, states []ConnectionHealthState, multiplierRank int) int {
	if platform == upstream.PlatformSub2API {
		return desiredSub2APIManagedPriority(states, multiplierRank, len(states))
	}
	return desiredManagedPriority(states, multiplierRank)
}

func desiredManagedPriorityForPlatformWithExpected(platform upstream.Platform, states []ConnectionHealthState, multiplierRank int, expectedModels int) int {
	if platform == upstream.PlatformSub2API {
		return desiredSub2APIManagedPriority(states, multiplierRank, expectedModels)
	}
	score := desiredManagedPriority(states, multiplierRank)
	if len(states) < expectedModels && score != 1 {
		// Missing model states are unconfigured, not healthy. A known suspended/disabled
		// state remains the lowest tier even when another model has not been probed yet.
		priceScore := maxInt(0, 999-multiplierRank)
		score = 10000 + priceScore
	}
	return score
}

func desiredHealthPriorityForPlatform(platform upstream.Platform, healthBand int, tupleRank int) int {
	if platform == upstream.PlatformSub2API {
		bases := []int{10, 100, 1000, 10000, 100000}
		nextBases := []int{100, 1000, 10000, 100000, 100001}
		if healthBand < 0 || healthBand >= len(bases) {
			healthBand = 3
		}
		if healthBand == 4 {
			return bases[healthBand]
		}
		return sub2APIPriorityWithinBand(bases[healthBand], nextBases[healthBand], tupleRank)
	}
	bases := []int{40000, 30000, 20000, 10000, 1}
	if healthBand < 0 || healthBand >= len(bases) {
		healthBand = 3
	}
	if healthBand == 4 {
		return 1
	}
	return bases[healthBand] + maxInt(0, 999-tupleRank)
}

// desiredSub2APIManagedPriority 使用 Sub2API「数值越小越优先」的原生语义，并为不同健康
// 状态预留互不重叠的区间：健康 10-99、恢复中 100-999、降级/观察 1000-9999、待探活
// 10000-99999、暂停/禁用 100000。同一状态内 multiplierRank 越小，priority 越小。
// rank 超出区间容量时在区间末尾并列，避免价格排序跨越健康状态边界。
func desiredSub2APIManagedPriority(states []ConnectionHealthState, multiplierRank int, expectedModels int) int {
	for _, state := range states {
		if state.State == StateDisabled || state.State == StateSuspended {
			return 100000
		}
	}
	if len(states) < expectedModels {
		return sub2APIPriorityWithinBand(10000, 100000, multiplierRank)
	}

	base, nextBase := 10, 100
	for _, state := range states {
		switch state.State {
		case StateDegraded, StateObserving:
			base, nextBase = 1000, 10000
		case StateRecovering:
			if base < 100 {
				base, nextBase = 100, 1000
			}
		}
	}
	return sub2APIPriorityWithinBand(base, nextBase, multiplierRank)
}

// sortHealthPriorityCandidates 按生产调度使用的完整比较元组排序。该比较器同时用于
// priority 写回和 admin 详情 rank，避免 priority 在区间末位并列时退化成分组局部顺序。
func sortHealthPriorityCandidates(candidates []healthPriorityCandidate) {
	sort.SliceStable(candidates, func(i int, j int) bool {
		return compareHealthPriorityCandidates(candidates[i], candidates[j]) < 0
	})
}

func compareHealthPriorityCandidates(left healthPriorityCandidate, right healthPriorityCandidate) int {
	if left.healthBand != right.healthBand {
		if left.healthBand < right.healthBand {
			return -1
		}
		return 1
	}
	if left.multiplier != right.multiplier {
		if left.multiplier < right.multiplier {
			return -1
		}
		return 1
	}
	if left.latencyMs == nil || right.latencyMs == nil {
		if left.latencyMs != nil {
			return -1
		}
		if right.latencyMs != nil {
			return 1
		}
	} else if *left.latencyMs != *right.latencyMs {
		if *left.latencyMs < *right.latencyMs {
			return -1
		}
		return 1
	}
	if left.targetID < right.targetID {
		return -1
	}
	if left.targetID > right.targetID {
		return 1
	}
	return 0
}

// multiplier_only 不参与 P 版健康状态分段，继续使用原有 1-9 独立倍率区间。
func desiredSub2APIMultiplierOnlyPriority(multiplierRank int) int {
	return sub2APIPriorityWithinBand(1, 10, multiplierRank)
}

func sub2APIPriorityWithinBand(base int, nextBase int, multiplierRank int) int {
	offset := maxInt(0, multiplierRank)
	return base + minInt(offset, nextBase-base-1)
}

func hasMultiplierPriorityPolicy(policies []Policy) bool {
	for _, policy := range policies {
		if policy.Enabled && normalizePriorityMode(policy.PriorityMode) == PriorityModeMultiplier {
			return true
		}
	}
	return false
}

func hasHealthMultiplierPriorityPolicy(policies []Policy) bool {
	return hasMultiplierPriorityPolicy(policies) && !hasMultiplierOnlyPolicy(policies)
}

func effectiveHealthSortMultiplier(item *priorityTargetInventory) (float64, bool) {
	if item.upstreamMultiplier.status == MultiplierResolutionResolved && item.upstreamMultiplier.info.multiplier != nil {
		return *item.upstreamMultiplier.info.multiplier, true
	}
	if item.upstreamMultiplier.status == MultiplierResolutionUnavailable {
		return 0, false
	}
	return uniqueFloat(item.fallbackMultipliers)
}

func uniqueFloat(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	value := values[0]
	for _, candidate := range values[1:] {
		if candidate != value {
			return 0, false
		}
	}
	return value, true
}

func activeHealthPriorityModels(item *priorityTargetInventory) map[string]struct{} {
	models := make(map[string]struct{})
	for _, spec := range candidateModelSpecs(item.target.Models, item.policies) {
		if spec.policy.AutoDegradeEnabled {
			models[spec.modelName] = struct{}{}
		}
	}
	return models
}

func activeHealthPriorityStates(states []ConnectionHealthState, activeModels map[string]struct{}) []ConnectionHealthState {
	active := make([]ConnectionHealthState, 0, len(activeModels))
	for _, state := range states {
		if _, ok := activeModels[state.ModelName]; ok {
			active = append(active, state)
		}
	}
	return active
}

func priorityHealthBand(states []ConnectionHealthState, expectedModels int) int {
	for _, state := range states {
		if state.State == StateDisabled || state.State == StateSuspended {
			return 4
		}
	}
	if len(states) < expectedModels {
		return 3
	}
	band := 0
	for _, state := range states {
		switch state.State {
		case StateDegraded, StateObserving:
			return 2
		case StateRecovering:
			band = 1
		}
	}
	return band
}

func completeTargetSuccessLatency(states []ConnectionHealthState, activeModels map[string]struct{}) *int {
	if len(activeModels) == 0 {
		return nil
	}
	latencyByModel := make(map[string]*int, len(states))
	for _, state := range states {
		latencyByModel[state.ModelName] = state.LastSuccessLatencyMs
	}
	maxLatency := 0
	for model := range activeModels {
		latency := latencyByModel[model]
		if latency == nil {
			return nil
		}
		if *latency > maxLatency {
			maxLatency = *latency
		}
	}
	return &maxLatency
}

// hasMultiplierOnlyPolicy 让明确的仅倍率策略成为同一目标的优先级依据。即使目标还叠加了
// 一条负责记录健康状态的探活策略，健康状态也不会重新参与 priority 排名。
func hasMultiplierOnlyPolicy(policies []Policy) bool {
	for _, policy := range policies {
		if policy.Enabled && normalizeStrategyMode(policy.StrategyMode) == StrategyModeMultiplierOnly {
			return true
		}
	}
	return false
}

func minFloat(values []float64) float64 {
	minValue := values[0]
	for _, value := range values[1:] {
		if value < minValue {
			minValue = value
		}
	}
	return minValue
}

// desiredManagedPriority 计算平台无关的路由分数，并使用互不重叠的区间保证健康状态始终压过价格：
// healthy > recovering > degraded/observing > unconfigured > suspended/disabled。
// 同一健康层级内，倍率排名越靠前（倍率越低）分数越大；平台数值方向由上层映射。
func desiredManagedPriority(states []ConnectionHealthState, multiplierRank int) int {
	priceScore := 999 - multiplierRank
	if priceScore < 0 {
		priceScore = 0
	}
	if len(states) == 0 {
		return 10000 + priceScore
	}

	base := 40000
	weight := 100
	for _, state := range states {
		if state.CurrentWeight < weight {
			weight = state.CurrentWeight
		}
		switch state.State {
		case StateDisabled, StateSuspended:
			return 1
		case StateDegraded, StateObserving:
			if base > 20000 {
				base = 20000
			}
		case StateRecovering:
			if base > 30000 {
				base = 30000
			}
		}
	}
	if base == 30000 {
		base += maxInt(0, minInt(100, weight)) * 50
	} else if base == 20000 {
		base += maxInt(0, minInt(100, weight)) * 10
	}
	return base + priceScore
}
