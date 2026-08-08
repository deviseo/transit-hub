package connection_health

import (
	"context"
	"errors"
	"log"
	"sort"
	"time"
)

const (
	priorityReconcileScanInterval = time.Second
	priorityWritebackScanInterval = time.Second
)

var errInventorySnapshotUnavailable = errors.New("inventory snapshot unavailable")

type adminInventorySnapshot struct {
	inventory  *adminWorkspaceInventory
	fetchedAt  time.Time
	expiresAt  time.Time
	generation uint64
	ctx        context.Context
	cancel     context.CancelFunc
	expiry     *time.Timer
}

type prioritySchedulerInputs struct {
	policies         []Policy
	assignments      []PolicyAssignment
	groupAssignments []GroupPolicyAssignment
	exclusions       []GroupTargetExclusion
	priorityStates   []PrioritySyncState
	workspaceStates  []PriorityWorkspaceSyncState
	targetActions    []TargetActionState
}

func (s *Service) startFrequencySchedulers(ctx context.Context) {
	s.schedulerStartOnce.Do(func() {
		s.ensureSchedulerSignals()
		schedulerCtx, cancel := context.WithCancel(ctx)
		s.schedulerCancel = cancel
		s.schedulerWG.Add(3)
		go func() { defer s.schedulerWG.Done(); s.runPriorityReconcileLoop(schedulerCtx) }()
		go func() { defer s.schedulerWG.Done(); s.runPriorityWritebackLoop(schedulerCtx) }()
		go func() { defer s.schedulerWG.Done(); s.runProbeSchedulerLoop(schedulerCtx) }()
	})
}

func (s *Service) StopScheduler(ctx context.Context) error {
	if s.schedulerCancel == nil {
		return nil
	}
	s.schedulerCancel()
	done := make(chan struct{})
	go func() {
		s.schedulerWG.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (s *Service) ensureSchedulerSignals() {
	s.schedulerSignalsOnce.Do(func() {
		s.probeSchedulerWake = make(chan struct{}, 1)
		s.priorityReconcileWake = make(chan struct{}, 1)
		s.priorityWritebackWake = make(chan struct{}, 1)
	})
}

func signalScheduler(ch chan struct{}) {
	if ch == nil {
		return
	}
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (s *Service) runProbeSchedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(probeSchedulerScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runSchedulerTickSafely(ctx)
		case <-s.probeSchedulerWake:
			s.runSchedulerTickSafely(ctx)
		}
	}
}

func (s *Service) runPriorityReconcileLoop(ctx context.Context) {
	s.runPriorityReconcileTickSafely(ctx)
	ticker := time.NewTicker(priorityReconcileScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runPriorityReconcileTickSafely(ctx)
		case <-s.priorityReconcileWake:
			s.runPriorityReconcileTickSafely(ctx)
		}
	}
}

func (s *Service) runPriorityWritebackLoop(ctx context.Context) {
	ticker := time.NewTicker(priorityWritebackScanInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runPriorityWritebackTickSafely(ctx)
		case <-s.priorityWritebackWake:
			s.runPriorityWritebackTickSafely(ctx)
		}
	}
}

func (s *Service) runPriorityReconcileTickSafely(ctx context.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[connection-health] priority reconcile tick panic recovered: %v", recovered)
		}
	}()
	s.runPriorityReconcileTick(ctx)
}

func (s *Service) runPriorityWritebackTickSafely(ctx context.Context) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("[connection-health] priority writeback tick panic recovered: %v", recovered)
		}
	}()
	s.runPriorityWritebackTick(ctx)
}

func (s *Service) loadPrioritySchedulerInputs(ctx context.Context) (prioritySchedulerInputs, error) {
	inputs := prioritySchedulerInputs{}
	var err error
	if inputs.policies, err = s.repo.ListEnabledPolicies(ctx); err != nil {
		return inputs, err
	}
	if inputs.assignments, err = s.repo.ListAllPolicyAssignments(ctx); err != nil {
		return inputs, err
	}
	if inputs.groupAssignments, err = s.repo.ListAllGroupPolicyAssignments(ctx); err != nil {
		return inputs, err
	}
	if inputs.exclusions, err = s.repo.ListAllGroupTargetExclusions(ctx); err != nil {
		return inputs, err
	}
	if inputs.priorityStates, err = s.repo.ListAllPrioritySyncStates(ctx); err != nil {
		return inputs, err
	}
	if inputs.workspaceStates, err = s.repo.ListAllPriorityWorkspaceSyncStates(ctx); err != nil {
		return inputs, err
	}
	if inputs.targetActions, err = s.repo.ListAllTargetActionStates(ctx); err != nil {
		return inputs, err
	}
	return inputs, nil
}

func priorityWorkspaceIdentities(inputs prioritySchedulerInputs) map[string][2]string {
	identities := make(map[string][2]string)
	add := func(userID string, adminAccountID string) {
		if userID == "" || adminAccountID == "" {
			return
		}
		identities[priorityWorkspaceKey(userID, adminAccountID)] = [2]string{userID, adminAccountID}
	}
	for _, assignment := range inputs.assignments {
		add(assignment.UserID, assignment.AdminAccountID)
	}
	for _, assignment := range inputs.groupAssignments {
		add(assignment.UserID, assignment.AdminAccountID)
	}
	for _, state := range inputs.priorityStates {
		add(state.UserID, state.AdminAccountID)
	}
	for _, state := range inputs.workspaceStates {
		if state.PendingSignature != "" {
			add(state.UserID, state.AdminAccountID)
		}
	}
	for _, state := range inputs.targetActions {
		add(state.UserID, state.AdminAccountID)
	}
	return identities
}

func sortedPriorityWorkspaceKeys(identities map[string][2]string) []string {
	keys := make([]string, 0, len(identities))
	for key := range identities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func priorityWorkspaceKey(userID string, adminAccountID string) string {
	return userID + "|" + adminAccountID
}

func (s *Service) schedulerNow() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) putAdminInventorySnapshot(parent context.Context, userID string, adminAccountID string, inventory *adminWorkspaceInventory, fetchedAt time.Time, ttl time.Duration) {
	s.inventorySnapshotMu.Lock()
	defer s.inventorySnapshotMu.Unlock()
	if s.inventorySnapshots == nil {
		s.inventorySnapshots = make(map[string]adminInventorySnapshot)
	}
	key := priorityWorkspaceKey(userID, adminAccountID)
	if previous, exists := s.inventorySnapshots[key]; exists {
		previous.cancel()
		if previous.expiry != nil {
			previous.expiry.Stop()
		}
	}
	s.inventorySnapshotGeneration++
	generationCtx, cancel := context.WithCancel(parent)
	snapshot := adminInventorySnapshot{
		inventory: inventory, fetchedAt: fetchedAt, expiresAt: fetchedAt.Add(ttl),
		generation: s.inventorySnapshotGeneration, ctx: generationCtx, cancel: cancel,
	}
	snapshot.expiry = time.AfterFunc(ttl, cancel)
	s.inventorySnapshots[key] = snapshot
}

func (s *Service) getAdminInventorySnapshot(userID string, adminAccountID string, now time.Time) (adminInventorySnapshot, bool) {
	s.inventorySnapshotMu.RLock()
	defer s.inventorySnapshotMu.RUnlock()
	snapshot, ok := s.inventorySnapshots[priorityWorkspaceKey(userID, adminAccountID)]
	if !ok || snapshot.inventory == nil || !now.Before(snapshot.expiresAt) {
		return adminInventorySnapshot{}, false
	}
	return snapshot, true
}

func (s *Service) invalidateAdminInventorySnapshot(userID string, adminAccountID string) {
	s.inventorySnapshotMu.Lock()
	defer s.inventorySnapshotMu.Unlock()
	key := priorityWorkspaceKey(userID, adminAccountID)
	if snapshot, exists := s.inventorySnapshots[key]; exists {
		snapshot.cancel()
		if snapshot.expiry != nil {
			snapshot.expiry.Stop()
		}
		delete(s.inventorySnapshots, key)
	}
}

func (s *Service) inventoryCacheForIdentities(identities map[string][2]string) adminInventoryCache {
	now := s.schedulerNow()
	cache := make(adminInventoryCache, len(identities))
	for key, identity := range identities {
		if snapshot, ok := s.getAdminInventorySnapshot(identity[0], identity[1], now); ok {
			cache[key] = adminInventoryCacheEntry{
				inventory: snapshot.inventory, snapshot: true,
				snapshotFetchedAt: snapshot.fetchedAt, snapshotExpiresAt: snapshot.expiresAt,
				snapshotGeneration: snapshot.generation,
			}
		} else {
			cache[key] = adminInventoryCacheEntry{err: errInventorySnapshotUnavailable}
		}
	}
	return cache
}

// inventoryCacheOperationContext joins the caller context with the snapshot generation
// represented by cache. Any invalidate, replacement or real-time expiry cancels follow-up
// work before it can write state derived from that old inventory.
func (s *Service) inventoryCacheOperationContext(
	parent context.Context,
	userID string,
	adminAccountID string,
	cache adminInventoryCache,
) (context.Context, func(), error) {
	entry, cached := cache[priorityWorkspaceKey(userID, adminAccountID)]
	if !cached || !entry.snapshot {
		return parent, func() {}, nil
	}
	now := s.schedulerNow()
	snapshot, valid := s.getAdminInventorySnapshot(userID, adminAccountID, now)
	if !valid || snapshot.generation != entry.snapshotGeneration ||
		!snapshot.fetchedAt.Equal(entry.snapshotFetchedAt) || !snapshot.expiresAt.Equal(entry.snapshotExpiresAt) ||
		!now.Before(entry.snapshotExpiresAt) {
		return nil, func() {}, errInventorySnapshotUnavailable
	}
	// Make the operation a direct child of the generation. context cancellation
	// propagates synchronously to direct children, so invalidate cannot race a
	// following remote write through an asynchronously scheduled callback.
	operationCtx, cancel := context.WithCancel(snapshot.ctx)
	stopParentCancellation := context.AfterFunc(parent, cancel)
	if err := snapshot.ctx.Err(); err != nil || parent.Err() != nil {
		stopParentCancellation()
		cancel()
		return nil, func() {}, errInventorySnapshotUnavailable
	}
	return operationCtx, func() {
		stopParentCancellation()
		cancel()
	}, nil
}

func (s *Service) inventoryCacheForSchedulerInputs(policies []Policy, assignments []PolicyAssignment, groupAssignments []GroupPolicyAssignment) adminInventoryCache {
	inputs := prioritySchedulerInputs{policies: policies, assignments: assignments, groupAssignments: groupAssignments}
	return s.inventoryCacheForIdentities(priorityWorkspaceIdentities(inputs))
}

// assignedPoliciesForWorkspace is deliberately derived from bindings rather than every
// enabled policy in the workspace. The priority plan contains only policies that can
// affect a target; including an unbound policy here would make its version oscillate
// with the version persisted by the plan and force an inventory read every tick.
func assignedPoliciesForWorkspace(
	policies []Policy,
	assignments []PolicyAssignment,
	groupAssignments []GroupPolicyAssignment,
	userID string,
	adminAccountID string,
) []Policy {
	assignedPolicyIDs := make(map[string]struct{})
	for _, assignment := range assignments {
		if assignment.UserID == userID && assignment.AdminAccountID == adminAccountID {
			assignedPolicyIDs[assignment.PolicyID] = struct{}{}
		}
	}
	for _, assignment := range groupAssignments {
		if assignment.UserID == userID && assignment.AdminAccountID == adminAccountID {
			assignedPolicyIDs[assignment.PolicyID] = struct{}{}
		}
	}
	result := make([]Policy, 0, len(assignedPolicyIDs))
	for _, policy := range policies {
		if policy.UserID != userID || policy.AdminAccountID != adminAccountID || !policy.Enabled {
			continue
		}
		if _, assigned := assignedPolicyIDs[policy.ID]; assigned {
			result = append(result, policy)
		}
	}
	return result
}

func (s *Service) runPriorityReconcileTick(ctx context.Context) {
	if s.platformGroups == nil {
		return
	}
	inputs, err := s.loadPrioritySchedulerInputs(ctx)
	if err != nil {
		log.Printf("[connection-health] priority reconcile load scheduler inputs failed: %v", err)
		return
	}
	identities := priorityWorkspaceIdentities(inputs)
	refreshed := make(map[string]struct{})
	now := s.schedulerNow()
	for _, key := range sortedPriorityWorkspaceKeys(identities) {
		identity := identities[key]
		userID, adminAccountID := identity[0], identity[1]
		release, leaseErr := s.repo.AcquirePrioritySyncLease(ctx, userID, adminAccountID)
		if leaseErr != nil {
			log.Printf("[connection-health] priority reconcile acquire workspace lease failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, leaseErr)
			continue
		}
		func() {
			defer release()
			workspacePolicies := assignedPoliciesForWorkspace(
				inputs.policies,
				inputs.assignments,
				inputs.groupAssignments,
				userID,
				adminAccountID,
			)
			preset := prioritySyncPresetForPolicies(workspacePolicies)
			state, stateErr := s.repo.GetPriorityWorkspaceSyncState(ctx, userID, adminAccountID)
			if stateErr != nil {
				log.Printf("[connection-health] priority reconcile load state failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, stateErr)
				return
			}
			if state == nil {
				state = &PriorityWorkspaceSyncState{UserID: userID, AdminAccountID: adminAccountID}
			}
			previousPolicyVersion := state.PolicyVersion
			applyPriorityPresetToState(state, preset)
			state.PolicyVersion = priorityPolicyVersion(workspacePolicies)
			_, hasSnapshot := s.getAdminInventorySnapshot(userID, adminAccountID, now)
			due := state.NextReconcileAt == nil || !now.Before(*state.NextReconcileAt)
			if state.PolicyVersion != previousPolicyVersion {
				due = true
			}
			if !hasSnapshot && state.InventoryStatus != "unknown" {
				due = true
			}
			if !due {
				return
			}

			attemptedAt := now
			state.LastReconcileAttemptAt = &attemptedAt
			state.LastActionSource = priorityActionReconcile
			state.ReconcileAttemptCount++
			startedAt := time.Now()
			inventory, readErr := s.loadAdminInventory(ctx, userID, adminAccountID, make(adminInventoryCache))
			state.LastInventoryReadDurationMs = time.Since(startedAt).Milliseconds()
			if readErr == nil && (inventory == nil || !inventory.complete) {
				readErr = errors.New("inventory accounts read incomplete")
			}
			if readErr != nil {
				s.invalidateAdminInventorySnapshot(userID, adminAccountID)
				failedAt := now
				nextAt := now.Add(time.Duration(preset.ReconcileFailureBackoffSeconds) * time.Second)
				state.LastReconcileFailureAt = &failedAt
				state.NextReconcileAt = &nextAt
				state.InventorySnapshotExpiresAt = nil
				state.InventoryStatus = "unknown"
				state.LastInventoryError = "inventory_read_failed"
				state.LastError = "inventory_read_failed"
				state.LastDecision = "suppressed"
				state.LastSuppressionReason = "inventory_unknown"
				state.ReconcileFailureCount++
				s.persistPriorityWorkspaceSyncState(ctx, state)
				log.Printf("[connection-health] priority reconcile inventory read failed user_id=%s admin_account_id=%s err=%v", userID, adminAccountID, readErr)
				return
			}

			if err := ctx.Err(); err != nil {
				return
			}
			inventory.multiplierLookup = s.upstreamMultiplierResolutionsByAdminAccount(ctx, userID, adminAccountID, string(inventory.session.Platform))
			inventory.multiplierLookupLoaded = true
			if err := ctx.Err(); err != nil {
				return
			}
			ttl := time.Duration(preset.InventorySnapshotTTLSeconds) * time.Second
			s.putAdminInventorySnapshot(ctx, userID, adminAccountID, inventory, now, ttl)
			succeededAt := now
			expiresAt := now.Add(ttl)
			nextAt := now.Add(time.Duration(preset.ReconcileIntervalSeconds) * time.Second)
			if expiresAt.Before(nextAt) {
				nextAt = expiresAt
			}
			state.LastReconcileSuccessAt = &succeededAt
			state.NextReconcileAt = &nextAt
			state.InventorySnapshotExpiresAt = &expiresAt
			state.InventoryStatus = "ready"
			state.LastInventoryError = ""
			state.LastError = ""
			state.ReconcileSuccessCount++
			s.persistPriorityWorkspaceSyncState(ctx, state)
			refreshed[key] = struct{}{}
		}()
	}
	if len(refreshed) == 0 {
		return
	}
	cache := s.inventoryCacheForIdentities(identities)
	mode := prioritySyncRunMode{
		source: priorityActionReconcile, reconcile: true, workspaceFilter: refreshed, workspaceIdentities: identities,
	}
	s.syncMultiplierPrioritiesWithCacheRunMode(ctx, inputs.policies, inputs.assignments, inputs.groupAssignments, inputs.exclusions, inputs.priorityStates, cache, mode)

	// Unmanaged status/weight restoration is no longer tied to a probe tick. It only consumes
	// snapshots from workspaces refreshed by this reconcile pass.
	refreshOnlyCache := make(adminInventoryCache, len(identities))
	for key := range identities {
		if _, ok := refreshed[key]; ok {
			refreshOnlyCache[key] = cache[key]
		} else {
			refreshOnlyCache[key] = adminInventoryCacheEntry{err: errInventorySnapshotUnavailable}
		}
	}
	s.restoreUnmanagedTargetActions(ctx, inputs.policies, inputs.assignments, inputs.groupAssignments, inputs.exclusions, inputs.targetActions, refreshOnlyCache)
	signalScheduler(s.priorityWritebackWake)
	signalScheduler(s.probeSchedulerWake)
}

func (s *Service) runPriorityWritebackTick(ctx context.Context) {
	inputs, err := s.loadPrioritySchedulerInputs(ctx)
	if err != nil {
		log.Printf("[connection-health] priority writeback load scheduler inputs failed: %v", err)
		return
	}
	identities := priorityWorkspaceIdentities(inputs)
	pending := make(map[string]struct{})
	for key, identity := range identities {
		state, stateErr := s.repo.GetPriorityWorkspaceSyncState(ctx, identity[0], identity[1])
		if stateErr != nil {
			log.Printf("[connection-health] priority writeback load state failed user_id=%s admin_account_id=%s err=%v", identity[0], identity[1], stateErr)
			continue
		}
		if state != nil && state.PendingSignature != "" {
			pending[key] = struct{}{}
		}
	}
	if len(pending) == 0 {
		return
	}
	cache := s.inventoryCacheForIdentities(identities)
	mode := prioritySyncRunMode{
		source: priorityActionWriteback, write: true, workspaceFilter: pending, workspaceIdentities: identities,
	}
	s.syncMultiplierPrioritiesWithCacheRunMode(ctx, inputs.policies, inputs.assignments, inputs.groupAssignments, inputs.exclusions, inputs.priorityStates, cache, mode)
}
