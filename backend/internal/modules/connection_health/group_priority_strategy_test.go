package connection_health

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

func TestCollectAdminProbeJobs_GroupAssignmentAndExclusion(t *testing.T) {
	repo := newFakeRepository()
	service := &Service{
		repo:           repo,
		mySites:        fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: schedulerReader("100"),
	}
	policies := []Policy{{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}}
	groupAssignments := []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: "p1",
	}}

	jobs := service.collectAdminProbeJobsWithGroups(context.Background(), policies, nil, groupAssignments, nil)
	if len(jobs) != 1 || jobs[0].target.TargetID != "newapi:ws1:100" {
		t.Fatalf("group policy should auto-include target, got %+v", jobs)
	}

	exclusions := []GroupTargetExclusion{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", TargetID: "newapi:ws1:100",
	}}
	jobs = service.collectAdminProbeJobsWithGroups(context.Background(), policies, nil, groupAssignments, exclusions)
	if len(jobs) != 0 {
		t.Fatalf("excluded target must not inherit group policy, got %+v", jobs)
	}

	// 旧版显式 target 分配优先于分组排除，保证已有线上配置不被新功能一棍子打死。
	explicit := []PolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: "p1",
	}}
	jobs = service.collectAdminProbeJobsWithGroups(context.Background(), policies, explicit, groupAssignments, exclusions)
	if len(jobs) != 1 {
		t.Fatalf("legacy explicit assignment must survive group exclusion, got %+v", jobs)
	}
}

func TestCollectAdminProbeJobs_PreservesPolicySourceGroupForSharedTarget(t *testing.T) {
	repo := newFakeRepository()
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "first"}, {ID: "g2", Name: "second"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100", BaseURL: "https://up", Models: "model-a,model-b"}},
			"g2": {{ID: "100", BaseURL: "https://up", Models: "model-a,model-b"}},
		},
	}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: reader,
	}
	policies := []Policy{
		{ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ModelTargets: []ModelTarget{{ModelName: "model-a", Enabled: true}}},
		{ID: "p2", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ModelTargets: []ModelTarget{{ModelName: "model-b", Enabled: true}}},
	}
	assignments := []GroupPolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "first", PolicyID: "p1"},
		{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g2", AdminGroupName: "second", PolicyID: "p2"},
	}

	jobs := service.collectAdminProbeJobsWithGroups(context.Background(), policies, nil, assignments, nil)
	if len(jobs) != 1 || len(jobs[0].dueSpecs) != 2 {
		t.Fatalf("shared target should produce one two-model job, got %+v", jobs)
	}
	groupsByModel := make(map[string]string)
	for _, spec := range jobs[0].dueSpecs {
		groupsByModel[spec.modelName] = spec.eventAdminGroupID
	}
	if groupsByModel["model-a"] != "g1" || groupsByModel["model-b"] != "g2" {
		t.Fatalf("each policy event must retain its source group: %+v", groupsByModel)
	}
}

func TestAdminGroups_ReportsInheritedPolicyAndExclusion(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: "policy-1",
	}}
	repo.groupExclusions = []GroupTargetExclusion{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", TargetID: "newapi:ws1:200",
	}}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.5)}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {
				{ID: "100", Name: "included", BaseURL: "https://up", Models: "gpt-4o"},
				{ID: "200", Name: "excluded", BaseURL: "https://up", Models: "gpt-4o"},
			},
		},
	}
	service := newAdminGroupsService(reader, fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, repo)

	groups, err := service.AdminGroups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 || groups[0].MonitoredAccountCount != 1 || groups[0].ExcludedAccountCount != 1 {
		t.Fatalf("unexpected group assignment summary: %+v", groups)
	}
	for _, account := range groups[0].Accounts {
		if account.ID == "100" && (!account.HasAssignedPolicy || account.PolicyAssignmentSource != "group") {
			t.Fatalf("included account should inherit group policy: %+v", account)
		}
		if account.ID == "200" && (!account.ExcludedFromGroupPolicy || account.HasAssignedPolicy) {
			t.Fatalf("excluded account should not inherit group policy: %+v", account)
		}
	}
}

func TestSetAdminGroupPolicyConfiguration_ValidatesAndPersistsScope(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100", Name: "channel", BaseURL: "https://up", Models: "gpt-4o"}},
		},
	}
	service := newAdminGroupsService(reader, fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, repo)
	targetID := "newapi:ws1:100"

	configuration, err := service.SetAdminGroupPolicyConfiguration(context.Background(), "user1", "g1", AdminGroupPolicyConfigurationInput{
		PolicyIDs: []string{"policy-1", "policy-1"}, ExcludedTargetIDs: []string{targetID, targetID},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(configuration.PolicyIDs) != 1 || configuration.PolicyIDs[0] != "policy-1" ||
		len(configuration.ExcludedTargetIDs) != 1 || configuration.ExcludedTargetIDs[0] != targetID {
		t.Fatalf("configuration should be deduplicated and persisted: %+v", configuration)
	}

	_, err = service.SetAdminGroupPolicyConfiguration(context.Background(), "user1", "g1", AdminGroupPolicyConfigurationInput{
		PolicyIDs: []string{"policy-1"}, ExcludedTargetIDs: []string{"newapi:ws1:not-in-group"},
	})
	if err == nil || err.Error() != ErrorProbeTargetNotFound {
		t.Fatalf("cross-group exclusion must be rejected, got %v", err)
	}
}

func TestSetAdminGroupPolicyConfiguration_QuickPolicyCreatesAndBindsTogether(t *testing.T) {
	repo := newFakeRepository()
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100", Name: "channel", BaseURL: "https://up", Models: "gpt-4o"}},
		},
	}
	service := newAdminGroupsService(reader, fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, repo)
	configuration, err := service.SetAdminGroupPolicyConfiguration(context.Background(), "user1", "g1", AdminGroupPolicyConfigurationInput{
		QuickPolicy: &PolicyInput{
			Name: "quick", Enabled: true, AutoDegradeEnabled: true, AutoRemoteActionEnabled: true,
			ModelTargets: []ModelTargetInput{{ModelName: "gpt-4o", ProviderFamily: ProviderOpenAI, Enabled: true}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(configuration.PolicyIDs) != 1 || len(repo.policies) != 1 || repo.policies[0].ID != configuration.PolicyIDs[0] {
		t.Fatalf("quick policy must be created and bound in one operation: config=%+v policies=%+v", configuration, repo.policies)
	}
}

func TestSetAdminGroupPolicyConfiguration_ReclaimsOnlyConflictedPriorities(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	conflictedTarget := "newapi:ws1:100"
	healthyTarget := "newapi:ws1:200"
	repo.priorityStates["user1|ws1|"+conflictedTarget] = PrioritySyncState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: conflictedTarget, Conflict: true,
	}
	repo.priorityStates["user1|ws1|"+healthyTarget] = PrioritySyncState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: healthyTarget, OriginalPriority: 7, LastAppliedPriority: 40000,
	}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100"}, {ID: "200"}},
		},
	}
	service := newAdminGroupsService(reader, fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, repo)

	if _, err := service.SetAdminGroupPolicyConfiguration(context.Background(), "user1", "g1", AdminGroupPolicyConfigurationInput{PolicyIDs: []string{"policy-1"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, exists := repo.priorityStates["user1|ws1|"+conflictedTarget]; exists {
		t.Fatal("saving group configuration should clear conflicted state so the scheduler can reclaim it")
	}
	if _, exists := repo.priorityStates["user1|ws1|"+healthyTarget]; !exists {
		t.Fatal("non-conflicted state must retain its original priority baseline")
	}
}

type priorityUpdateCall struct {
	targetID string
	priority int
}

type fakeTargetPriorityActioner struct {
	calls []priorityUpdateCall
}

func (f *fakeTargetPriorityActioner) UpdateAdminTargetPriority(session upstream.Session, targetID string, priority int) error {
	f.calls = append(f.calls, priorityUpdateCall{targetID: targetID, priority: priority})
	return nil
}

func TestMultiplierPrioritySyncAndManualConflict(t *testing.T) {
	repo := newFakeRepository()
	priorityActions := &fakeTargetPriorityActioner{}
	accountPriority := 7
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100", Name: "channel", Priority: &accountPriority, BaseURL: "https://up", Models: "gpt-4o"}},
		},
	}
	service := &Service{
		repo:            repo,
		mySites:         fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups:  reader,
		priorityActions: priorityActions,
	}
	policies := []Policy{{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, PriorityMode: PriorityModeMultiplier,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}}
	groupAssignments := []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: "p1",
	}}

	service.syncMultiplierPriorities(context.Background(), policies, nil, groupAssignments, nil, nil)
	if len(priorityActions.calls) != 1 || priorityActions.calls[0].priority <= accountPriority {
		t.Fatalf("expected lower multiplier to receive managed high priority, calls=%+v", priorityActions.calls)
	}
	stored := repo.priorityStates["user1|ws1|newapi:ws1:100"]
	if stored.OriginalPriority != 7 || stored.LastAppliedPriority != priorityActions.calls[0].priority || stored.Conflict {
		t.Fatalf("unexpected stored priority state: %+v", stored)
	}

	// 模拟管理员在上游把系统写入值手动改为 23；下一轮只能标记冲突，不能再次覆盖。
	manualPriority := 23
	reader.accountsByGrp["g1"] = []upstream.AdminGroupAccountInfo{{
		ID: "100", Name: "channel", Priority: &manualPriority, BaseURL: "https://up", Models: "gpt-4o",
	}}
	service.platformGroups = reader
	service.syncMultiplierPriorities(context.Background(), policies, nil, groupAssignments, nil, []PrioritySyncState{stored})
	if len(priorityActions.calls) != 1 {
		t.Fatalf("manual priority change must not be overwritten, calls=%+v", priorityActions.calls)
	}
	stored = repo.priorityStates["user1|ws1|newapi:ws1:100"]
	if !stored.Conflict || stored.LastConflictPriority == nil || *stored.LastConflictPriority != manualPriority {
		t.Fatalf("manual change should be recorded as conflict: %+v", stored)
	}
}

func TestMultiplierPrioritySync_IgnoresFrozenHealthWhenAutoDegradeDisabled(t *testing.T) {
	repo := newFakeRepository()
	priorityActions := &fakeTargetPriorityActioner{}
	targetID := "newapi:ws1:100"
	currentPriority := 1
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100", Priority: &currentPriority, Models: "gpt-4o"}},
		},
	}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: reader, priorityActions: priorityActions,
	}
	policy := Policy{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		AutoDegradeEnabled: false, PriorityMode: PriorityModeMultiplier,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	assignment := GroupPolicyAssignment{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID,
	}
	repo.states[targetID] = map[string]ConnectionHealthState{
		"gpt-4o": {ConnectionID: targetID, ModelName: "gpt-4o", State: StateSuspended, CurrentWeight: 0, UserID: "user1", AdminAccountID: "ws1"},
	}
	stored := PrioritySyncState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalPriority: 7, LastAppliedPriority: currentPriority,
	}
	repo.priorityStates["user1|ws1|"+targetID] = stored

	service.syncMultiplierPriorities(
		context.Background(), []Policy{policy}, nil, []GroupPolicyAssignment{assignment}, nil,
		[]PrioritySyncState{stored},
	)
	want := desiredManagedPriorityForPlatformWithExpected(upstream.PlatformNewAPI, nil, 0, 0)
	if len(priorityActions.calls) != 1 || priorityActions.calls[0].priority != want {
		t.Fatalf("frozen suspended state must not pin multiplier priority, want=%d calls=%+v", want, priorityActions.calls)
	}
}

func TestMultiplierPrioritySync_ConfirmsPendingSystemWrite(t *testing.T) {
	repo := newFakeRepository()
	priorityActions := &fakeTargetPriorityActioner{}
	desired := desiredManagedPriorityForPlatform(upstream.PlatformNewAPI, nil, 0)
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100", Priority: &desired, Models: "gpt-4o"}},
		},
	}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: reader, priorityActions: priorityActions,
	}
	policy := Policy{ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, PriorityMode: PriorityModeMultiplier, ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}}}
	assignment := GroupPolicyAssignment{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID}
	pending := desired
	stored := PrioritySyncState{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", OriginalPriority: 7, LastAppliedPriority: 7, PendingPriority: &pending}

	service.syncMultiplierPriorities(context.Background(), []Policy{policy}, nil, []GroupPolicyAssignment{assignment}, nil, []PrioritySyncState{stored})
	updated := repo.priorityStates["user1|ws1|newapi:ws1:100"]
	if updated.Conflict || updated.PendingPriority != nil || updated.LastAppliedPriority != desired {
		t.Fatalf("pending priority write should be confirmed without conflict: %+v", updated)
	}
	if len(priorityActions.calls) != 0 {
		t.Fatalf("already-applied priority must not be written twice: %+v", priorityActions.calls)
	}
}

func TestMultiplierPrioritySync_DoesNotRestoreWhenInventoryIsIncomplete(t *testing.T) {
	repo := newFakeRepository()
	priorityActions := &fakeTargetPriorityActioner{}
	reader := fakePlatformGroupReader{
		groups:   []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)}},
		errByGrp: map[string]error{"g1": errors.New("temporary upstream failure")},
	}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: reader, priorityActions: priorityActions,
	}
	policy := Policy{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, PriorityMode: PriorityModeMultiplier,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	assignment := GroupPolicyAssignment{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID}
	stored := PrioritySyncState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100",
		OriginalPriority: 7, LastAppliedPriority: 40999,
	}
	repo.priorityStates["user1|ws1|"+stored.TargetID] = stored

	service.syncMultiplierPriorities(context.Background(), []Policy{policy}, nil, []GroupPolicyAssignment{assignment}, nil, []PrioritySyncState{stored})
	if len(priorityActions.calls) != 0 {
		t.Fatalf("incomplete inventory must not restore or rewrite priority: %+v", priorityActions.calls)
	}
	if _, exists := repo.priorityStates["user1|ws1|"+stored.TargetID]; !exists {
		t.Fatal("incomplete inventory must retain the priority checkpoint for the next scan")
	}
}

func TestMultiplierPrioritySync_MissingConflictedTargetIsNotOverwritten(t *testing.T) {
	repo := newFakeRepository()
	priorityActions := &fakeTargetPriorityActioner{}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: fakePlatformGroupReader{}, priorityActions: priorityActions,
	}
	stored := PrioritySyncState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100",
		OriginalPriority: 7, LastAppliedPriority: 40999, Conflict: true,
	}
	repo.priorityStates["user1|ws1|"+stored.TargetID] = stored

	service.syncMultiplierPriorities(context.Background(), nil, nil, nil, nil, []PrioritySyncState{stored})
	if len(priorityActions.calls) != 0 {
		t.Fatalf("missing target with a manual conflict must not be overwritten: %+v", priorityActions.calls)
	}
	if _, exists := repo.priorityStates["user1|ws1|"+stored.TargetID]; exists {
		t.Fatal("unmanaged conflicted target should release its stale checkpoint without a remote write")
	}
}

func TestDesiredManagedPriority_HealthAlwaysBeatsMultiplier(t *testing.T) {
	healthyExpensive := desiredManagedPriority([]ConnectionHealthState{{State: StateHealthy, CurrentWeight: 100}}, 20)
	degradedCheap := desiredManagedPriority([]ConnectionHealthState{{State: StateDegraded, CurrentWeight: 75}}, 0)
	if healthyExpensive <= degradedCheap {
		t.Fatalf("health tier must outrank price: healthy=%d degraded=%d", healthyExpensive, degradedCheap)
	}
	cheap := desiredManagedPriority([]ConnectionHealthState{{State: StateHealthy, CurrentWeight: 100}}, 0)
	expensive := desiredManagedPriority([]ConnectionHealthState{{State: StateHealthy, CurrentWeight: 100}}, 1)
	if cheap <= expensive {
		t.Fatalf("within same health tier lower multiplier must rank higher: cheap=%d expensive=%d", cheap, expensive)
	}
}

func TestDesiredManagedPriority_MissingModelIsUnconfigured(t *testing.T) {
	healthy := []ConnectionHealthState{{State: StateHealthy, CurrentWeight: 100}}
	score := desiredManagedPriorityForPlatformWithExpected(upstream.PlatformNewAPI, healthy, 0, 2)
	expected := 10000 + 999
	if score != expected {
		t.Fatalf("one healthy and one unprobed model must use the unconfigured tier: got %d want %d", score, expected)
	}
	suspended := []ConnectionHealthState{{State: StateSuspended, CurrentWeight: 0}}
	if got := desiredManagedPriorityForPlatformWithExpected(upstream.PlatformNewAPI, suspended, 0, 2); got != 1 {
		t.Fatalf("known suspended model must remain the lowest tier even with missing siblings: %d", got)
	}
}

func TestMultiplierPrioritySync_IgnoresExcludedGroupMultiplier(t *testing.T) {
	repo := newFakeRepository()
	priorityActions := &fakeTargetPriorityActioner{}
	priority := 7
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{
			{ID: "managed", Name: "managed", Multiplier: float64Ptr(2)},
			{ID: "excluded", Name: "excluded", Multiplier: float64Ptr(0.1)},
		},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"managed":  {{ID: "100", Priority: &priority, Models: "gpt-4o"}},
			"excluded": {{ID: "100", Priority: &priority, Models: "gpt-4o"}},
		},
	}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: reader, priorityActions: priorityActions,
	}
	policy := Policy{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, PriorityMode: PriorityModeMultiplier,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	assignments := []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "managed", AdminGroupName: "managed", PolicyID: "p1",
	}}
	exclusions := []GroupTargetExclusion{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "excluded", TargetID: "newapi:ws1:100",
	}}

	service.syncMultiplierPriorities(context.Background(), []Policy{policy}, nil, assignments, exclusions, nil)
	stored := repo.priorityStates["user1|ws1|newapi:ws1:100"]
	if stored.EffectiveMultiplier != 2 {
		t.Fatalf("excluded group multiplier must not leak into priority, got %+v", stored)
	}
}

func TestMultiplierPrioritySync_ExplicitTargetSurvivesGroupExclusion(t *testing.T) {
	repo := newFakeRepository()
	priorityActions := &fakeTargetPriorityActioner{}
	priority := 7
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "excluded", Name: "excluded", Multiplier: float64Ptr(0.1)}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"excluded": {{ID: "100", Priority: &priority, Models: "gpt-4o"}},
		},
	}
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: reader, priorityActions: priorityActions,
	}
	policy := Policy{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, PriorityMode: PriorityModeMultiplier,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	targetID := "newapi:ws1:100"
	exclusions := []GroupTargetExclusion{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "excluded", TargetID: targetID,
	}}
	targetAssignments := []PolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID, PolicyID: policy.ID,
	}}

	service.syncMultiplierPriorities(context.Background(), []Policy{policy}, targetAssignments, nil, exclusions, nil)
	stored := repo.priorityStates["user1|ws1|"+targetID]
	if stored.EffectiveMultiplier != 0.1 || len(priorityActions.calls) != 1 {
		t.Fatalf("explicit target multiplier should survive group exclusion, stored=%+v calls=%+v", stored, priorityActions.calls)
	}
}

func TestDesiredManagedPriority_UsesPlatformPriorityDirection(t *testing.T) {
	healthy := []ConnectionHealthState{{State: StateHealthy, CurrentWeight: 100}}
	newAPICheap := desiredManagedPriorityForPlatform(upstream.PlatformNewAPI, healthy, 0)
	newAPIExpensive := desiredManagedPriorityForPlatform(upstream.PlatformNewAPI, healthy, 1)
	if newAPICheap <= newAPIExpensive {
		t.Fatalf("NewAPI must use a larger priority for the lower multiplier: cheap=%d expensive=%d", newAPICheap, newAPIExpensive)
	}
	sub2APICheap := desiredManagedPriorityForPlatform(upstream.PlatformSub2API, healthy, 0)
	sub2APIExpensive := desiredManagedPriorityForPlatform(upstream.PlatformSub2API, healthy, 1)
	if sub2APICheap >= sub2APIExpensive {
		t.Fatalf("Sub2API must use a smaller priority for the lower multiplier: cheap=%d expensive=%d", sub2APICheap, sub2APIExpensive)
	}
}

func TestFilterToAssignedTargetEvents_SameNameGroupExclusionDoesNotHideOtherAssignment(t *testing.T) {
	repo := newFakeRepository()
	repo.groupAssignments = []GroupPolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "same", PolicyID: "p1"},
		{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g2", AdminGroupName: "same", PolicyID: "p1"},
	}
	targetID := "newapi:ws1:100"
	repo.groupExclusions = []GroupTargetExclusion{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", TargetID: targetID,
	}}
	service := &Service{repo: repo}
	events, err := service.filterToAssignedTargetEvents(context.Background(), "user1", "ws1", []ConnectionHealthEvent{{
		ConnectionID: targetID, OwnGroupName: "same",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("assignment from the non-excluded same-name group should retain the event, got %+v", events)
	}
}

func TestFilterToAssignedTargetEvents_DropsUnassignedAdminGroupEvent(t *testing.T) {
	repo := newFakeRepository()
	service := &Service{repo: repo}
	targetID := "newapi:ws1:100"
	events, err := service.filterToAssignedTargetEvents(context.Background(), "user1", "ws1", []ConnectionHealthEvent{{
		ConnectionID: targetID, AdminGroupID: "removed", OwnGroupName: "removed",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("event from an unassigned admin group must be filtered, got %+v", events)
	}
}

func TestFilterToAssignedTargetEvents_KeepsUnmanagedRestoreAudit(t *testing.T) {
	repo := newFakeRepository()
	service := &Service{repo: repo}
	targetID := "newapi:ws1:100"
	events, err := service.filterToAssignedTargetEvents(context.Background(), "user1", "ws1", []ConnectionHealthEvent{{
		ConnectionID: targetID, Result: "policy_unmanaged_restore",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("automatic restore must remain visible after the final policy is unbound: %+v", events)
	}
}

func TestFilterToAssignedTargetEvents_UsesPolicyForLegacyWrongGroupMetadata(t *testing.T) {
	repo := newFakeRepository()
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g2", AdminGroupName: "second", PolicyID: "p2",
	}}
	service := &Service{repo: repo}
	targetID := "newapi:ws1:100"
	events, err := service.filterToAssignedTargetEvents(context.Background(), "user1", "ws1", []ConnectionHealthEvent{{
		ConnectionID: targetID, PolicyID: "p2", AdminGroupID: "removed-g1", OwnGroupName: "first",
	}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("legacy event must follow its still-assigned policy when stored group metadata is stale: %+v", events)
	}
}

func float64Ptr(value float64) *float64 { return &value }
