package connection_health

import (
	"context"
	"errors"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

type gatePriorityActioner struct {
	calls []priorityUpdateCall
	err   error
}

func (f *gatePriorityActioner) UpdateAdminTargetPriority(session upstream.Session, targetID string, priority int) error {
	f.calls = append(f.calls, priorityUpdateCall{targetID: targetID, priority: priority})
	return f.err
}

func TestPriorityWriteGate_SkipsUnchangedOrderAndCoalescesLatestPending(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 0, 0, 0, 0, time.UTC)
	actions := &gatePriorityActioner{}
	service := &Service{repo: repo, priorityActions: actions, now: func() time.Time { return now }}
	policy := priorityGatePolicy()
	inventory, healthStates := priorityGateInventory(policy, map[string]int{"a": 100, "b": 200, "c": 300})

	runPriorityWriteGate(service, repo, inventory, healthStates)
	if len(actions.calls) != 2 {
		t.Fatalf("initial managed ordering should update only changed targets, calls=%+v", actions.calls)
	}
	applyPriorityActionCalls(inventory, actions.calls)
	firstWriteCount := len(actions.calls)

	// All raw latencies move by the same factor but the stable production order remains
	// a,b,c. The B signature must remain unchanged and perform no upstream priority write.
	setPriorityGateLatencies(healthStates, map[string]int{"a": 200, "b": 400, "c": 600})
	now = now.Add(10 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	if len(actions.calls) != firstWriteCount {
		t.Fatalf("unchanged stable order must not write priority: %+v", actions.calls)
	}
	workspace := priorityWorkspaceState(t, repo)
	if workspace.LastDecision != "skipped" || workspace.LastSuppressionReason != "signature_unchanged" {
		t.Fatalf("unchanged order decision = %+v", workspace)
	}

	// The first order change writes after the preceding write interval. Two later changes
	// within that window are coalesced; only the final a,c,b order may be written.
	setPriorityGateLatencies(healthStates, map[string]int{"a": 200, "b": 50, "c": 600})
	now = now.Add(21 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	if len(actions.calls) == firstWriteCount {
		t.Fatal("a changed production order must write after the interval")
	}
	applyPriorityActionCalls(inventory, actions.calls[firstWriteCount:])
	firstChangedWriteCount := len(actions.calls)

	setPriorityGateLatencies(healthStates, map[string]int{"a": 200, "b": 50, "c": 10})
	now = now.Add(time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	setPriorityGateLatencies(healthStates, map[string]int{"a": 5, "b": 50, "c": 10})
	now = now.Add(10 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	if len(actions.calls) != firstChangedWriteCount {
		t.Fatalf("window changes must stay pending without a write: %+v", actions.calls)
	}
	workspace = priorityWorkspaceState(t, repo)
	if workspace.PendingSince == nil || workspace.LastDecision != "pending" || workspace.WindowSuppressionCount < 2 {
		t.Fatalf("latest order must remain coalesced pending: %+v", workspace)
	}

	now = now.Add(20 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	latest := actions.calls[firstChangedWriteCount:]
	if len(latest) != 3 {
		t.Fatalf("the final pending order should produce one three-target write, calls=%+v", latest)
	}
	priorities := make(map[string]int, len(latest))
	for _, call := range latest {
		priorities[call.targetID] = call.priority
	}
	if !(priorities["a"] > priorities["c"] && priorities["c"] > priorities["b"]) {
		t.Fatalf("only latest a,c,b order may be written, priorities=%+v", priorities)
	}
}

func TestPriorityWriteGate_FailureStaysPendingAndRecoversAfterRestart(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 1, 0, 0, 0, time.UTC)
	actions := &gatePriorityActioner{err: errors.New("upstream write unavailable")}
	service := &Service{repo: repo, priorityActions: actions, now: func() time.Time { return now }}
	policy := priorityGatePolicy()
	inventory, healthStates := priorityGateInventory(policy, map[string]int{"a": 100, "b": 200})

	runPriorityWriteGate(service, repo, inventory, healthStates)
	failed := priorityWorkspaceState(t, repo)
	if failed.LastDecision != "failed" || failed.PendingSignature == "" || failed.AppliedSignature != "" || failed.WriteFailureCount == 0 {
		t.Fatalf("failed write must remain pending and unapplied: %+v", failed)
	}

	// A fresh service instance simulates a process restart. It recomputes from the same
	// persisted pending signature and must retry only after the configured interval.
	actions.err = nil
	restarted := &Service{repo: repo, priorityActions: actions, now: func() time.Time { return now }}
	now = now.Add(29 * time.Second)
	runPriorityWriteGate(restarted, repo, inventory, healthStates)
	if priorityWorkspaceState(t, repo).LastDecision != "pending" {
		t.Fatal("restart must retain pending state during the write interval")
	}
	now = now.Add(time.Second)
	runPriorityWriteGate(restarted, repo, inventory, healthStates)
	recovered := priorityWorkspaceState(t, repo)
	if recovered.LastDecision != "applied" || recovered.PendingSignature != "" || recovered.AppliedSignature == "" {
		t.Fatalf("recovered write must apply the persisted latest signature: %+v", recovered)
	}
}

func TestPriorityWriteGate_AlertsOnManualPriorityDriftWithoutOverwrite(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 2, 0, 0, 0, time.UTC)
	actions := &gatePriorityActioner{}
	service := &Service{repo: repo, priorityActions: actions, now: func() time.Time { return now }}
	policy := priorityGatePolicy()
	inventory, healthStates := priorityGateInventory(policy, map[string]int{"a": 100, "b": 200})
	runPriorityWriteGate(service, repo, inventory, healthStates)
	applyPriorityActionCalls(inventory, actions.calls)
	beforeDrift := len(actions.calls)

	inventory["newapi:ws1:b"].currentPriority = 7
	now = now.Add(31 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	workspace := priorityWorkspaceState(t, repo)
	if len(actions.calls) != beforeDrift || workspace.LastDecision != "drift_alert" || workspace.DriftCount == 0 {
		t.Fatalf("manual drift must alert without another write: state=%+v calls=%+v", workspace, actions.calls)
	}
	checkpoint := repo.priorityStates["user1|ws1|newapi:ws1:b"]
	if !checkpoint.Conflict || checkpoint.LastConflictPriority == nil || *checkpoint.LastConflictPriority != 7 {
		t.Fatalf("manual priority must be retained as an alert-only conflict: %+v", checkpoint)
	}
	driftCount := workspace.DriftCount
	now = now.Add(31 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	workspace = priorityWorkspaceState(t, repo)
	if workspace.LastDecision != "drift_alert" || workspace.DriftCount != driftCount {
		t.Fatalf("an existing drift must stay visible without incrementing its event count: before=%d state=%+v", driftCount, workspace)
	}

	setPriorityGateLatencies(healthStates, map[string]int{"a": 300, "b": 100})
	actions.err = errors.New("upstream write unavailable")
	now = now.Add(31 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	failed := priorityWorkspaceState(t, repo)
	if failed.LastDecision != "failed" || failed.LastError != "priority_write_failed" || failed.LastSuppressionReason != "manual_priority_drift" || failed.PendingSignature == "" {
		t.Fatalf("write failure must remain primary while retaining drift context: %+v", failed)
	}
	actions.err = nil
	now = now.Add(31 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	workspace = priorityWorkspaceState(t, repo)
	if workspace.AppliedSignature == "" || workspace.PendingSignature != "" || workspace.LastDecision != "drift_alert" {
		t.Fatalf("a new order should converge around an alert-only conflict: %+v", workspace)
	}
	if len(actions.calls) != beforeDrift+2 || actions.calls[len(actions.calls)-1].targetID != "a" {
		t.Fatalf("non-conflicted targets may still follow the new order, calls=%+v", actions.calls)
	}
}

func TestPriorityWriteGate_Sub2APIPriorityReadRecoveryRetriesUnappliedSignature(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 3, 0, 0, 0, time.UTC)
	actions := &gatePriorityActioner{}
	service := &Service{repo: repo, priorityActions: actions, now: func() time.Time { return now }}
	policy := priorityGatePolicy()
	targetID := "sub2api:ws1:100"
	latency := 100
	inventory := map[string]*priorityTargetInventory{
		targetID: {
			target:              AdminProbeTarget{TargetID: targetID, AccountID: "100", Models: []string{"gpt-4o"}},
			policies:            []Policy{policy},
			fallbackMultipliers: []float64{0.4},
			currentPriority:     100,
			priorityPresent:     false,
		},
	}
	healthStates := []ConnectionHealthState{{ConnectionID: targetID, ModelName: "gpt-4o", State: StateHealthy, LastSuccessLatencyMs: &latency}}
	runPriorityWriteGateForPlatform(service, repo, inventory, healthStates, upstream.PlatformSub2API)
	suppressed := priorityWorkspaceState(t, repo)
	if suppressed.LastSuppressionReason != "priority_input_unavailable" || suppressed.AppliedSignature != "" {
		t.Fatalf("unreadable Sub2API priority must suppress without applying the signature: %+v", suppressed)
	}

	inventory[targetID].priorityPresent = true
	now = now.Add(31 * time.Second)
	runPriorityWriteGateForPlatform(service, repo, inventory, healthStates, upstream.PlatformSub2API)
	recovered := priorityWorkspaceState(t, repo)
	if len(actions.calls) != 1 || recovered.AppliedSignature == "" || recovered.LastDecision != "applied" {
		t.Fatalf("priority becoming readable must retry the unchanged signature: calls=%+v state=%+v", actions.calls, recovered)
	}
	inventory[targetID].currentPriority = actions.calls[0].priority
	inventory[targetID].policies = nil
	inventory[targetID].priorityPresent = false
	managedSignature := recovered.AppliedSignature
	now = now.Add(31 * time.Second)
	runPriorityWriteGateForPlatform(service, repo, inventory, healthStates, upstream.PlatformSub2API)
	unreadableRestore := priorityWorkspaceState(t, repo)
	if unreadableRestore.LastSuppressionReason != "priority_input_unavailable" || unreadableRestore.AppliedSignature != managedSignature || len(actions.calls) != 1 {
		t.Fatalf("unreadable priority must not apply ownership removal: calls=%+v state=%+v", actions.calls, unreadableRestore)
	}

	inventory[targetID].priorityPresent = true
	now = now.Add(31 * time.Second)
	runPriorityWriteGateForPlatform(service, repo, inventory, healthStates, upstream.PlatformSub2API)
	restored := priorityWorkspaceState(t, repo)
	if len(actions.calls) != 2 || actions.calls[1].priority != 100 || restored.AppliedSignature == managedSignature || restored.PendingSignature != "" {
		t.Fatalf("readable ownership removal must restore the original priority: calls=%+v state=%+v", actions.calls, restored)
	}
}

func TestPriorityWriteGate_PendingOverdueReasonSurvivesWriteFailure(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 4, 0, 0, 0, time.UTC)
	actions := &gatePriorityActioner{}
	service := &Service{repo: repo, priorityActions: actions, now: func() time.Time { return now }}
	policy := priorityGatePolicy()
	policy.PrioritySyncPreset.MinWriteIntervalSeconds = 1
	policy.PrioritySyncPreset.MaxPendingAgeSeconds = 1
	inventory, healthStates := priorityGateInventory(policy, map[string]int{"a": 100, "b": 200})
	runPriorityWriteGate(service, repo, inventory, healthStates)
	applyPriorityActionCalls(inventory, actions.calls)
	actions.err = errors.New("upstream write unavailable")
	setPriorityGateLatencies(healthStates, map[string]int{"a": 300, "b": 100})
	now = now.Add(2 * time.Second)
	runPriorityWriteGate(service, repo, inventory, healthStates)
	now = now.Add(2 * time.Second)
	states, _ := repo.ListPrioritySyncStates(context.Background(), "user1", "ws1")
	service.syncWorkspacePriorities(context.Background(), upstream.Session{Platform: upstream.PlatformNewAPI}, "user1", "ws1", inventory, false, healthStates, states)
	suppressed := priorityWorkspaceState(t, repo)
	if suppressed.LastDecision != "suppressed" || suppressed.LastSuppressionReason != "inventory_incomplete" || suppressed.LastError != "priority_pending_overdue" {
		t.Fatalf("input suppression must retain an overdue pending error: %+v", suppressed)
	}
	runPriorityWriteGate(service, repo, inventory, healthStates)
	state := priorityWorkspaceState(t, repo)
	if state.LastDecision != "failed" || state.LastError != "priority_write_failed" || state.LastSuppressionReason != "priority_pending_overdue" {
		t.Fatalf("overdue pending reason must remain visible alongside write failure: %+v", state)
	}
}

func TestPrioritySyncPreset_UsesShortestManagedPolicyInterval(t *testing.T) {
	short := priorityGatePolicy()
	short.PrioritySyncPreset.MinWriteIntervalSeconds = 10
	long := priorityGatePolicy()
	long.PrioritySyncPreset.MinWriteIntervalSeconds = 30
	managed := map[string]*priorityTargetInventory{
		"newapi:ws1:a": {policies: []Policy{short}},
		"newapi:ws1:b": {policies: []Policy{long}},
	}
	got := prioritySyncPresetForManagedTargets(managed)
	if got.MinWriteIntervalSeconds != 10 {
		t.Fatalf("workspace effective interval must honor the shortest managed policy: %+v", got)
	}
}

func priorityGatePolicy() Policy {
	return Policy{
		ID: "policy", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		PriorityMode: PriorityModeMultiplier, StrategyMode: StrategyModeHealthProbe, AutoDegradeEnabled: true,
		PrioritySyncPreset: PrioritySyncPreset{
			MinWriteIntervalSeconds: 30,
			MaxPendingAgeSeconds:    300,
			DriftAction:             PriorityDriftActionAlertOnly,
			ReadMode:                PriorityReadModeInventory,
		},
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
}

func priorityGateInventory(policy Policy, latencies map[string]int) (map[string]*priorityTargetInventory, []ConnectionHealthState) {
	inventory := make(map[string]*priorityTargetInventory, len(latencies))
	states := make([]ConnectionHealthState, 0, len(latencies))
	for accountID, latencyValue := range latencies {
		targetID := "newapi:ws1:" + accountID
		latency := latencyValue
		inventory[targetID] = &priorityTargetInventory{
			target:              AdminProbeTarget{TargetID: targetID, AccountID: accountID, Models: []string{"gpt-4o"}},
			currentPriority:     desiredHealthPriorityForPlatform(upstream.PlatformNewAPI, 0, 0),
			priorityPresent:     true,
			policies:            []Policy{policy},
			fallbackMultipliers: []float64{0.4},
		}
		states = append(states, ConnectionHealthState{
			ConnectionID: targetID, ModelName: "gpt-4o", State: StateHealthy, LastSuccessLatencyMs: &latency,
		})
	}
	return inventory, states
}

func setPriorityGateLatencies(states []ConnectionHealthState, values map[string]int) {
	for index := range states {
		accountID := states[index].ConnectionID[len("newapi:ws1:"):]
		latency := values[accountID]
		states[index].LastSuccessLatencyMs = &latency
	}
}

func runPriorityWriteGate(service *Service, repo *fakeRepository, inventory map[string]*priorityTargetInventory, healthStates []ConnectionHealthState) {
	runPriorityWriteGateForPlatform(service, repo, inventory, healthStates, upstream.PlatformNewAPI)
}

func runPriorityWriteGateForPlatform(service *Service, repo *fakeRepository, inventory map[string]*priorityTargetInventory, healthStates []ConnectionHealthState, platform upstream.Platform) {
	states, _ := repo.ListPrioritySyncStates(context.Background(), "user1", "ws1")
	service.syncWorkspacePriorities(context.Background(), upstream.Session{Platform: platform}, "user1", "ws1", inventory, true, healthStates, states)
}

func applyPriorityActionCalls(inventory map[string]*priorityTargetInventory, calls []priorityUpdateCall) {
	for _, call := range calls {
		targetID := "newapi:ws1:" + call.targetID
		inventory[targetID].currentPriority = call.priority
	}
}

func priorityWorkspaceState(t *testing.T, repo *fakeRepository) PriorityWorkspaceSyncState {
	t.Helper()
	state, err := repo.GetPriorityWorkspaceSyncState(context.Background(), "user1", "ws1")
	if err != nil || state == nil {
		t.Fatalf("workspace sync state missing: state=%+v err=%v", state, err)
	}
	return *state
}
