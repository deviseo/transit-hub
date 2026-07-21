package connection_health

import (
	"context"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

func TestReconcileTargetRemoteAction_SuspendedSiblingBlocksRestore(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	service := &Service{repo: repo, dispatcher: newRemoteActionDispatcher(nil, nil, platform)}
	targetID := "sub2api:ws1:acc-1"
	repo.states[targetID] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: targetID, ModelName: "model-a", State: StateSuspended, CurrentWeight: 0},
		"model-b": {ConnectionID: targetID, ModelName: "model-b", State: StateRecovering, CurrentWeight: 25},
	}
	repo.targetActionStates["user1|ws1|"+targetID] = TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "active", LastAppliedStatus: "inactive",
	}
	policy := Policy{ID: "p1", Enabled: true, AutoDegradeEnabled: true, AutoRemoteActionEnabled: true}
	specs := []probeModelSpec{{modelName: "model-a", policy: policy}, {modelName: "model-b", policy: policy}}
	target := AdminProbeTarget{TargetID: targetID, Platform: string(upstream.PlatformSub2API), AccountID: "acc-1", AccountStatus: "inactive"}

	action, err := service.reconcileTargetRemoteAction(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformSub2API}, target, specs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != "" || len(platform.sub2APICalls) != 0 {
		t.Fatalf("suspended sibling must keep account inactive, action=%q calls=%+v", action, platform.sub2APICalls)
	}

	repo.states[targetID]["model-a"] = ConnectionHealthState{ConnectionID: targetID, ModelName: "model-a", State: StateHealthy, CurrentWeight: 100}
	action, err = service.reconcileTargetRemoteAction(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformSub2API}, target, specs)
	if err != nil {
		t.Fatalf("unexpected restore error: %v", err)
	}
	if action != RemoteActionSub2APIStatusActive || len(platform.sub2APICalls) != 1 || platform.sub2APICalls[0].status != "active" {
		t.Fatalf("account should restore only after every model is safe, action=%q calls=%+v", action, platform.sub2APICalls)
	}
}

func TestReconcileTargetRemoteAction_RestoresOriginalNewAPIWeight(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	service := &Service{repo: repo, dispatcher: newRemoteActionDispatcher(nil, nil, platform)}
	targetID := "newapi:ws1:100"
	originalWeight, appliedWeight, currentWeight := 37, 25, 25
	repo.states[targetID] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: targetID, ModelName: "model-a", State: StateHealthy, CurrentWeight: 100},
	}
	repo.targetActionStates["user1|ws1|"+targetID] = TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "1", OriginalWeight: &originalWeight, LastAppliedStatus: "1", LastAppliedWeight: &appliedWeight,
	}
	policy := Policy{ID: "p1", Enabled: true, AutoDegradeEnabled: true, AutoRemoteActionEnabled: true}
	target := AdminProbeTarget{
		TargetID: targetID, Platform: string(upstream.PlatformNewAPI), AccountID: "100",
		AccountStatus: "1", AccountWeight: &currentWeight,
	}

	action, err := service.reconcileTargetRemoteAction(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformNewAPI}, target, []probeModelSpec{{modelName: "model-a", policy: policy}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != "newapi_channel_weight_37" || len(platform.calls) != 1 || platform.calls[0].weight != 37 || platform.calls[0].status != 1 {
		t.Fatalf("expected exact original weight restore, action=%q calls=%+v", action, platform.calls)
	}
	if _, exists := repo.targetActionStates["user1|ws1|"+targetID]; exists {
		t.Fatal("action snapshot should be removed after the original state is restored")
	}
}

func TestReconcileTargetRemoteAction_ScalesNewAPIWeightFromOriginal(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	service := &Service{repo: repo, dispatcher: newRemoteActionDispatcher(nil, nil, platform)}
	targetID := "newapi:ws1:100"
	originalWeight, currentWeight := 37, 37
	repo.states[targetID] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: targetID, ModelName: "model-a", State: StateDegraded, CurrentWeight: 75},
	}
	policy := Policy{ID: "p1", Enabled: true, AutoDegradeEnabled: true, AutoRemoteActionEnabled: true}
	target := AdminProbeTarget{
		TargetID: targetID, Platform: string(upstream.PlatformNewAPI), AccountID: "100",
		AccountStatus: "1", AccountWeight: &currentWeight,
	}

	action, err := service.reconcileTargetRemoteAction(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformNewAPI}, target, []probeModelSpec{{modelName: "model-a", policy: policy}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != "" || len(platform.calls) != 0 {
		t.Fatalf("ordinary degraded state should not take over an unmanaged target: action=%q calls=%+v", action, platform.calls)
	}

	appliedWeight := 0
	repo.targetActionStates["user1|ws1|"+targetID] = TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "1", OriginalWeight: &originalWeight, LastAppliedStatus: "2", LastAppliedWeight: &appliedWeight,
	}
	target.AccountStatus = "2"
	target.AccountWeight = &appliedWeight
	action, err = service.reconcileTargetRemoteAction(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformNewAPI}, target, []probeModelSpec{{modelName: "model-a", policy: policy}})
	if err != nil {
		t.Fatalf("unexpected managed recovery error: %v", err)
	}
	if action != "newapi_channel_weight_28" || len(platform.calls) != 1 || platform.calls[0].weight != 28 {
		t.Fatalf("75%% recovery of original weight 37 must write 28, action=%q calls=%+v", action, platform.calls)
	}
}

func TestReconcileTargetRemoteAction_DoesNotRestoreWithUnprobedControlledModel(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	service := &Service{repo: repo, dispatcher: newRemoteActionDispatcher(nil, nil, platform)}
	targetID := "sub2api:ws1:acc-1"
	repo.states[targetID] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: targetID, ModelName: "model-a", State: StateHealthy, CurrentWeight: 100},
	}
	repo.targetActionStates["user1|ws1|"+targetID] = TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "active", LastAppliedStatus: "inactive",
	}
	policy := Policy{ID: "p1", Enabled: true, AutoDegradeEnabled: true, AutoRemoteActionEnabled: true}
	specs := []probeModelSpec{{modelName: "model-a", policy: policy}, {modelName: "model-b", policy: policy}}
	target := AdminProbeTarget{TargetID: targetID, Platform: string(upstream.PlatformSub2API), AccountID: "acc-1", AccountStatus: "inactive"}

	action, err := service.reconcileTargetRemoteAction(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformSub2API}, target, specs)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != "" || len(platform.sub2APICalls) != 0 {
		t.Fatalf("missing model state must keep the managed target inactive: action=%q calls=%+v", action, platform.sub2APICalls)
	}
}

func TestReconcileTargetRemoteAction_DoesNotEnableInitiallyDisabledTarget(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	service := &Service{repo: repo, dispatcher: newRemoteActionDispatcher(nil, nil, platform)}
	targetID := "sub2api:ws1:acc-1"
	repo.states[targetID] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: targetID, ModelName: "model-a", State: StateRecovering, CurrentWeight: 25},
	}
	policy := Policy{ID: "p1", Enabled: true, AutoDegradeEnabled: true, AutoRemoteActionEnabled: true}
	target := AdminProbeTarget{TargetID: targetID, Platform: string(upstream.PlatformSub2API), AccountID: "acc-1", AccountStatus: "inactive"}

	action, err := service.reconcileTargetRemoteAction(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformSub2API}, target, []probeModelSpec{{modelName: "model-a", policy: policy}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != RemoteActionSkippedTargetInitiallyDisabled || len(platform.sub2APICalls) != 0 {
		t.Fatalf("initially disabled target must not be enabled, action=%q calls=%+v", action, platform.sub2APICalls)
	}
}

func TestReconcileTargetRemoteAction_ConfirmsPendingSystemWrite(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	service := &Service{repo: repo, dispatcher: newRemoteActionDispatcher(nil, nil, platform)}
	targetID := "sub2api:ws1:acc-1"
	repo.states[targetID] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: targetID, ModelName: "model-a", State: StateSuspended, CurrentWeight: 0},
	}
	repo.targetActionStates["user1|ws1|"+targetID] = TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "active", LastAppliedStatus: "active", PendingStatus: "inactive",
	}
	policy := Policy{ID: "p1", Enabled: true, AutoDegradeEnabled: true, AutoRemoteActionEnabled: true}
	target := AdminProbeTarget{TargetID: targetID, Platform: string(upstream.PlatformSub2API), AccountID: "acc-1", AccountStatus: "inactive"}

	action, err := service.reconcileTargetRemoteAction(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformSub2API}, target, []probeModelSpec{{modelName: "model-a", policy: policy}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	stored := repo.targetActionStates["user1|ws1|"+targetID]
	if action != "" || stored.Conflict || stored.PendingStatus != "" || stored.LastAppliedStatus != "inactive" {
		t.Fatalf("pending system write should be confirmed without conflict: action=%q stored=%+v", action, stored)
	}
	if len(platform.sub2APICalls) != 0 {
		t.Fatalf("already-applied pending action must not be repeated: %+v", platform.sub2APICalls)
	}
}

func TestRestoreUnmanagedTargetActions_RestoresAfterPolicyUnbound(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "acc-1", Status: "inactive", Models: "gpt-4o"}},
		},
	}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}},
		platformGroups: reader, dispatcher: newRemoteActionDispatcher(nil, nil, platform),
	}
	targetID := "sub2api:ws1:acc-1"
	stored := TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "active", LastAppliedStatus: "inactive",
	}
	repo.targetActionStates["user1|ws1|"+targetID] = stored

	service.restoreUnmanagedTargetActions(context.Background(), nil, nil, nil, nil, []TargetActionState{stored}, make(adminInventoryCache))
	if len(platform.sub2APICalls) != 1 || platform.sub2APICalls[0].status != "active" {
		t.Fatalf("unbound policy should restore the original upstream state: %+v", platform.sub2APICalls)
	}
	if _, exists := repo.targetActionStates["user1|ws1|"+targetID]; exists {
		t.Fatal("restored unmanaged target must release its action snapshot")
	}
	if len(repo.events) != 1 || repo.events[0].Result != "policy_unmanaged_restore" {
		t.Fatalf("restore should be traceable in events: %+v", repo.events)
	}
}

func TestRestoreUnmanagedTargetActions_RestoresWhenAutoDegradeDisabled(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "acc-1", Status: "inactive", Models: "gpt-4o"}},
		},
	}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}},
		platformGroups: reader, dispatcher: newRemoteActionDispatcher(nil, nil, platform),
	}
	targetID := "sub2api:ws1:acc-1"
	stored := TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "active", LastAppliedStatus: "inactive",
	}
	repo.targetActionStates["user1|ws1|"+targetID] = stored
	policy := Policy{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		AutoDegradeEnabled: false, AutoRemoteActionEnabled: true,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	assignment := GroupPolicyAssignment{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID,
	}

	service.restoreUnmanagedTargetActions(
		context.Background(), []Policy{policy}, nil, []GroupPolicyAssignment{assignment}, nil,
		[]TargetActionState{stored}, make(adminInventoryCache),
	)
	if len(platform.sub2APICalls) != 1 || platform.sub2APICalls[0].status != "active" {
		t.Fatalf("turning off auto degrade must release the captured upstream state: %+v", platform.sub2APICalls)
	}
	if _, exists := repo.targetActionStates["user1|ws1|"+targetID]; exists {
		t.Fatal("restored target must release its action snapshot")
	}
}

func TestRestoreUnmanagedTargetActions_RestoresTargetRemovedFromAllGroups(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}},
		platformGroups: fakePlatformGroupReader{}, dispatcher: newRemoteActionDispatcher(nil, nil, platform),
	}
	targetID := "sub2api:ws1:acc-1"
	stored := TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "active", LastAppliedStatus: "inactive",
	}
	repo.targetActionStates["user1|ws1|"+targetID] = stored

	service.restoreUnmanagedTargetActions(context.Background(), nil, nil, nil, nil, []TargetActionState{stored}, make(adminInventoryCache))
	if len(platform.sub2APICalls) != 1 || platform.sub2APICalls[0].status != "active" {
		t.Fatalf("target removed from every group should still restore by stable target id: %+v", platform.sub2APICalls)
	}
	if _, exists := repo.targetActionStates["user1|ws1|"+targetID]; exists {
		t.Fatal("restored missing target must release its action snapshot")
	}
}
