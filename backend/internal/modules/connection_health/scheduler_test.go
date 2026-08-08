package connection_health

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

func TestIsDue_NeverProbedIsDue(t *testing.T) {
	repo := newFakeRepository()
	svc := &Service{repo: repo}
	if !svc.isDue(context.Background(), "conn-1", "m1", Policy{ProbeIntervalSeconds: 60}, time.Now()) {
		t.Fatalf("expected never-probed target to be due")
	}
}

func TestIsDue_DisabledNeverDue(t *testing.T) {
	repo := newFakeRepository()
	repo.states["conn-1"] = map[string]ConnectionHealthState{
		"m1": {ConnectionID: "conn-1", ModelName: "m1", State: StateDisabled},
	}
	svc := &Service{repo: repo}
	if svc.isDue(context.Background(), "conn-1", "m1", Policy{ProbeIntervalSeconds: 60}, time.Now()) {
		t.Fatalf("disabled state must never be due for automatic probing")
	}
}

func TestRecordTargetCredentialUnavailable_PreservesLegacyRemoteAction(t *testing.T) {
	repo := newFakeRepository()
	svc := &Service{repo: repo}
	targetID := "sub2api:ws1:acc-1"
	repo.states[targetID] = map[string]ConnectionHealthState{
		"gpt-4o": {
			ConnectionID: targetID, ModelName: "gpt-4o", UserID: "user1", AdminAccountID: "ws1",
			State: StateSuspended, LastRemoteAction: RemoteActionSub2APIStatusInactive,
		},
	}
	target := AdminProbeTarget{TargetID: targetID, Platform: string(upstream.PlatformSub2API), AccountID: "acc-1"}
	spec := probeModelSpec{modelName: "gpt-4o", policy: Policy{ID: "p1"}}

	svc.recordTargetCredentialUnavailable(context.Background(), "user1", "ws1", target, []probeModelSpec{spec}, upstream.ReasonCredentialUnavailable)
	stored := repo.states[targetID]["gpt-4o"]
	if stored.LastRemoteAction != RemoteActionSub2APIStatusInactive {
		t.Fatalf("credential failure must preserve legacy ownership evidence, got %+v", stored)
	}
	if stored.LastProbeAt != nil || stored.UpdatedAt.IsZero() {
		t.Fatalf("credential resolution is not a real probe and must only update retry timing: %+v", stored)
	}
	decision := calculateEffectiveProbeDecision([]Policy{{ID: "p1", ProbeIntervalSeconds: 60}}, boolPointer(true), &stored, stored.UpdatedAt)
	if decision.NextProbeAt == nil || !decision.NextProbeAt.Equal(stored.UpdatedAt.Add(time.Minute)) {
		t.Fatalf("credential retry must still respect the probe interval: %+v", decision)
	}
}

func TestFinishTargetProbeBatch_SchedulingSkipPreservesLegacyRemoteAction(t *testing.T) {
	repo := newFakeRepository()
	platform := &fakePlatformActioner{}
	svc := &Service{repo: repo, dispatcher: newRemoteActionDispatcher(nil, nil, platform)}
	targetID := "sub2api:ws1:acc-1"
	schedulable := false
	state := ConnectionHealthState{
		ConnectionID: targetID, ModelName: "gpt-4o", UserID: "user1", AdminAccountID: "ws1",
		State: StateSuspended, CurrentWeight: 0, LastRemoteAction: RemoteActionSub2APIStatusInactive,
	}
	repo.states[targetID] = map[string]ConnectionHealthState{"gpt-4o": state}
	policy := Policy{ID: "p1", Enabled: true, AutoDegradeEnabled: true, AutoRemoteActionEnabled: true}
	spec := probeModelSpec{modelName: "gpt-4o", policy: policy}
	target := AdminProbeTarget{
		TargetID: targetID, Platform: string(upstream.PlatformSub2API), AccountID: "acc-1",
		AccountStatus: "active", Schedulable: &schedulable,
	}

	svc.finishTargetProbeBatch(context.Background(), "user1", "ws1", upstream.Session{Platform: upstream.PlatformSub2API}, target,
		[]probeModelSpec{spec}, []targetProbeResult{{state: &state, previousState: StateSuspended, outcome: ProbeOutcome{Result: ResultOK}, spec: spec}}, EventSourceScheduled)

	stored := repo.states[targetID]["gpt-4o"]
	if stored.LastRemoteAction != RemoteActionSub2APIStatusInactive {
		t.Fatalf("scheduling skip must preserve legacy ownership evidence, got %q", stored.LastRemoteAction)
	}
	if len(repo.events) != 1 || repo.events[0].RemoteAction != RemoteActionSkippedUpstreamScheduling {
		t.Fatalf("scheduling skip must remain visible in the event audit, got %+v", repo.events)
	}
	if len(platform.sub2APICalls) != 0 {
		t.Fatalf("scheduling-disabled target must not receive a status write, got %+v", platform.sub2APICalls)
	}
}

func TestIsDue_WithinCooldownIsNotDue(t *testing.T) {
	repo := newFakeRepository()
	future := time.Now().Add(1 * time.Minute)
	repo.states["conn-1"] = map[string]ConnectionHealthState{
		"m1": {ConnectionID: "conn-1", ModelName: "m1", State: StateSuspended, CooldownUntil: &future},
	}
	svc := &Service{repo: repo}
	if svc.isDue(context.Background(), "conn-1", "m1", Policy{ProbeIntervalSeconds: 60}, time.Now()) {
		t.Fatalf("expected target within cooldown to not be due")
	}
}

func TestIsDue_RespectsIntervalAndBackoff(t *testing.T) {
	repo := newFakeRepository()
	now := time.Now()
	recentProbe := now.Add(-10 * time.Second)
	repo.states["conn-1"] = map[string]ConnectionHealthState{
		"m1": {ConnectionID: "conn-1", ModelName: "m1", State: StateHealthy, LastProbeAt: &recentProbe},
	}
	svc := &Service{repo: repo}

	if svc.isDue(context.Background(), "conn-1", "m1", Policy{ProbeIntervalSeconds: 60}, now) {
		t.Fatalf("expected not due within interval")
	}

	repo.states["conn-1"] = map[string]ConnectionHealthState{
		"m1": {ConnectionID: "conn-1", ModelName: "m1", State: StateDegraded, LastProbeAt: &recentProbe, ConsecutiveFailures: 2},
	}
	if svc.isDue(context.Background(), "conn-1", "m1", Policy{ProbeIntervalSeconds: 60}, now) {
		t.Fatalf("expected backoff window to still be active 10s after failure")
	}

	longAgo := now.Add(-6 * time.Minute)
	repo.states["conn-1"] = map[string]ConnectionHealthState{
		"m1": {ConnectionID: "conn-1", ModelName: "m1", State: StateDegraded, LastProbeAt: &longAgo, ConsecutiveFailures: 2},
	}
	if !svc.isDue(context.Background(), "conn-1", "m1", Policy{ProbeIntervalSeconds: 60}, now) {
		t.Fatalf("expected due after backoff window elapses")
	}
}

func TestEffectiveProbeDecision_UnschedulableTargetUsesConfiguredInterval(t *testing.T) {
	now := time.Now()
	lastProbe := now.Add(-30 * time.Minute)
	state := &ConnectionHealthState{State: StateHealthy, LastProbeAt: &lastProbe}
	decision := calculateEffectiveProbeDecision([]Policy{{
		ID: "p1", Name: "slow while closed", ProbeIntervalSeconds: 60,
		ContinueProbeWhenUnschedulable: true, UnschedulableProbeIntervalMinutes: 60,
	}}, boolPointer(false), state, now)

	if !decision.ContinueAutoProbe || decision.EffectiveIntervalSeconds != 3600 {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if decision.NextProbeAt == nil || !decision.NextProbeAt.Equal(lastProbe.Add(time.Hour)) {
		t.Fatalf("unexpected next probe time: %+v", decision)
	}
	if decision.BlockedReason != ProbeBlockedInterval {
		t.Fatalf("blocked reason = %q, want %q", decision.BlockedReason, ProbeBlockedInterval)
	}
}

func TestEffectiveProbeDecision_UnschedulableTargetStopsOnlyWhenAllPoliciesStop(t *testing.T) {
	now := time.Now()
	stopped := false
	continued := true
	decision := calculateEffectiveProbeDecision([]Policy{
		{ID: "stop", ContinueProbeWhenUnschedulable: stopped, UnschedulableProbeIntervalMinutes: 60},
		{ID: "continue", ContinueProbeWhenUnschedulable: continued, UnschedulableProbeIntervalMinutes: 120},
	}, boolPointer(false), nil, now)
	if !decision.ContinueAutoProbe || decision.BudgetPolicyID != "continue" || len(decision.SourcePolicies) != 2 {
		t.Fatalf("one continuing policy must keep monitoring with all sources visible: %+v", decision)
	}

	decision = calculateEffectiveProbeDecision([]Policy{{
		ID: "stop", ContinueProbeWhenUnschedulable: false, UnschedulableProbeIntervalMinutes: 60,
	}}, boolPointer(false), nil, now)
	if decision.ContinueAutoProbe || decision.NextProbeAt != nil || decision.BlockedReason != ProbeBlockedUpstreamSchedulingDisabled {
		t.Fatalf("all policies stopping must block automatic probes: %+v", decision)
	}
}

func TestEffectiveProbeDecision_UnschedulableOverrideCannotShortenCooldownOrBackoff(t *testing.T) {
	now := time.Now()
	lastProbe := now.Add(-3 * time.Minute)
	cooldown := now.Add(8 * time.Minute)
	state := &ConnectionHealthState{
		State: StateSuspended, LastProbeAt: &lastProbe, ConsecutiveFailures: 3, CooldownUntil: &cooldown,
	}
	decision := calculateEffectiveProbeDecision([]Policy{{
		ID: "p1", ContinueProbeWhenUnschedulable: true, UnschedulableProbeIntervalMinutes: 1,
	}}, boolPointer(false), state, now)
	if decision.NextProbeAt == nil || !decision.NextProbeAt.Equal(cooldown) || decision.BlockedReason != ProbeBlockedCooldown {
		t.Fatalf("cooldown must remain authoritative: %+v", decision)
	}
}

func TestEffectiveProbeDecision_BudgetFallsBackToNextEligiblePolicy(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	lastProbe := now.Add(-2 * time.Minute)
	policies := []Policy{
		{ID: "fast", Name: "fast", ProbeIntervalSeconds: 60, DailyProbeBudget: 1},
		{ID: "slow", Name: "slow", ProbeIntervalSeconds: 300, DailyProbeBudget: 10},
	}
	decision := calculateEffectiveProbeDecisionWithBudgets(policies, boolPointer(true), &ConnectionHealthState{
		State: StateHealthy, LastProbeAt: &lastProbe,
	}, now, map[string]int{"fast": 1, "slow": 0})
	if decision.BudgetPolicyID != "slow" {
		t.Fatalf("budget owner = %q, want slow after fast policy budget is exhausted", decision.BudgetPolicyID)
	}
	wantNext := lastProbe.Add(300 * time.Second)
	if decision.NextProbeAt == nil || !decision.NextProbeAt.Equal(wantNext) || decision.BlockedReason != ProbeBlockedInterval {
		t.Fatalf("unexpected fallback decision: %+v", decision)
	}
	if len(decision.SourcePolicies) != 2 {
		t.Fatalf("all participating sources must remain visible: %+v", decision.SourcePolicies)
	}
}

func TestEffectiveProbeDecision_AllBudgetsExhaustedUsesNextBusinessDay(t *testing.T) {
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	policy := Policy{ID: "only", ProbeIntervalSeconds: 60, DailyProbeBudget: 1}
	decision := calculateEffectiveProbeDecisionWithBudgets([]Policy{policy}, boolPointer(true), nil, now, map[string]int{"only": 1})
	wantNext := probeBudgetDayStart(now).Add(24 * time.Hour).UTC()
	if decision.NextProbeAt == nil || !decision.NextProbeAt.Equal(wantNext) || decision.BlockedReason != ProbeBlockedDailyBudget {
		t.Fatalf("unexpected exhausted-budget decision: %+v", decision)
	}
}

func boolPointer(value bool) *bool { return &value }

func timePointer(value time.Time) *time.Time { return &value }

// schedulerReader 构造一个平台读取器：单分组，若干可探活 channel（带 base_url + models）。
func schedulerReader(accountIDs ...string) fakePlatformGroupReader {
	accounts := make([]upstream.AdminGroupAccountInfo, 0, len(accountIDs))
	for _, id := range accountIDs {
		accounts = append(accounts, upstream.AdminGroupAccountInfo{ID: id, Name: "ch-" + id, BaseURL: "https://up", Models: "gpt-4o"})
	}
	return fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": accounts},
	}
}

func sub2APISchedulerReader(schedulable *bool) fakePlatformGroupReader {
	return fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "default", Platform: string(upstream.PlatformSub2API)}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "1515", Name: "account-1515", Models: "gpt-4o", Schedulable: schedulable}},
		},
	}
}

func schedulableProbePolicy(id string, continueProbe bool, intervalMinutes int) Policy {
	return Policy{
		ID: id, UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60,
		ContinueProbeWhenUnschedulable: continueProbe, UnschedulableProbeIntervalMinutes: intervalMinutes,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
}

func TestCollectAdminProbeJobs_UnschedulableTargetCanStopAutomaticMonitoring(t *testing.T) {
	repo := newFakeRepository()
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}},
		platformGroups: sub2APISchedulerReader(boolPointer(false)),
	}
	policy := schedulableProbePolicy("stop", false, 60)
	assignments := []PolicyAssignment{{UserID: "user1", AdminAccountID: "ws1", TargetID: "sub2api:ws1:1515", PolicyID: policy.ID}}

	if jobs := service.collectAdminProbeJobs(context.Background(), []Policy{policy}, assignments); len(jobs) != 0 {
		t.Fatalf("unschedulable target with all policies stopped must not generate jobs: %+v", jobs)
	}
}

func TestRecheckAdminProbeSpecs_UsesFreshUnschedulableDecision(t *testing.T) {
	now := time.Now().UTC()
	targetID := "sub2api:ws1:1515"
	for _, tt := range []struct {
		name          string
		continueProbe bool
		lastProbeAt   *time.Time
	}{
		{name: "strategy stops monitoring", continueProbe: false},
		{name: "slow interval not reached", continueProbe: true, lastProbeAt: timePointer(now.Add(-30 * time.Minute))},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			if tt.lastProbeAt != nil {
				repo.states[targetID] = map[string]ConnectionHealthState{
					"gpt-4o": {ConnectionID: targetID, ModelName: "gpt-4o", State: StateHealthy, LastProbeAt: tt.lastProbeAt},
				}
			}
			service := &Service{repo: repo}
			policy := schedulableProbePolicy("p1", tt.continueProbe, 60)
			spec := probeModelSpec{modelName: "gpt-4o", policy: policy, policies: []Policy{policy}}
			target := AdminProbeTarget{TargetID: targetID, Schedulable: boolPointer(false)}

			if due := service.recheckAdminProbeSpecs(context.Background(), "user1", "ws1", target, []probeModelSpec{spec}, now); len(due) != 0 {
				t.Fatalf("fresh unschedulable decision must discard stale due specs: %+v", due)
			}
		})
	}
}

func TestRecheckAdminProbeSpecs_ReservesBudgetWithinBatch(t *testing.T) {
	now := time.Now().UTC()
	repo := newFakeRepository()
	service := &Service{repo: repo}
	fast := Policy{
		ID: "fast", Enabled: true, ProbeIntervalSeconds: 60, DailyProbeBudget: 1,
		ModelTargets: []ModelTarget{{ModelName: "model-a", Enabled: true}, {ModelName: "model-b", Enabled: true}},
	}
	slow := Policy{
		ID: "slow", Enabled: true, ProbeIntervalSeconds: 60, DailyProbeBudget: 10,
		ModelTargets: []ModelTarget{{ModelName: "model-a", Enabled: true}, {ModelName: "model-b", Enabled: true}},
	}
	specs := candidateModelSpecs([]string{"model-a", "model-b"}, []Policy{fast, slow})
	due := service.recheckAdminProbeSpecs(context.Background(), "user1", "ws1", AdminProbeTarget{
		TargetID: "newapi:ws1:100", Schedulable: boolPointer(true),
	}, specs, now)
	if len(due) != 2 {
		t.Fatalf("due specs = %+v, want two models", due)
	}
	if due[0].budgetPolicy.ID != fast.ID || due[1].budgetPolicy.ID != slow.ID {
		t.Fatalf("batch budget owners = %s, %s; want fast then slow", due[0].budgetPolicy.ID, due[1].budgetPolicy.ID)
	}
}

func TestCurrentScheduledProbeSpecsDropsStalePolicyAndUsesCurrentAssignments(t *testing.T) {
	repo := newFakeRepository()
	current := schedulableProbePolicy("current", true, 120)
	repo.policies = []Policy{current}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g2", AdminGroupName: "second", PolicyID: current.ID,
	}}
	service := &Service{repo: repo}
	queued := schedulableProbePolicy("deleted", true, 60)

	all, queuedNow, ok := service.currentScheduledProbeSpecs(context.Background(), "user1", "ws1", AdminProbeTarget{
		TargetID: "sub2api:ws1:1515", Models: []string{"gpt-4o"},
	}, []adminTargetMembership{{groupID: "g2", groupName: "second"}}, []probeModelSpec{{
		modelName: "gpt-4o", policy: queued, policies: []Policy{queued},
	}})
	if !ok || len(all) != 1 || len(queuedNow) != 1 {
		t.Fatalf("current specs not rebuilt: ok=%v all=%+v queued=%+v", ok, all, queuedNow)
	}
	if queuedNow[0].policy.ID != current.ID || queuedNow[0].policies[0].UnschedulableProbeIntervalMinutes != 120 {
		t.Fatalf("queued spec retained stale policy: %+v", queuedNow[0])
	}
	if got := targetForProbeSpec(AdminProbeTarget{AdminGroupID: "old"}, queuedNow[0]); got.AdminGroupID != "g2" {
		t.Fatalf("event group = %q, want current budget policy source g2", got.AdminGroupID)
	}
}

func TestCollectAdminProbeJobs_UnschedulableTargetUsesMinuteOverride(t *testing.T) {
	repo := newFakeRepository()
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}},
		platformGroups: sub2APISchedulerReader(boolPointer(false)),
	}
	policy := schedulableProbePolicy("slow", true, 60)
	assignments := []PolicyAssignment{{UserID: "user1", AdminAccountID: "ws1", TargetID: "sub2api:ws1:1515", PolicyID: policy.ID}}
	lastProbe := time.Now().Add(-30 * time.Minute)
	repo.states["sub2api:ws1:1515"] = map[string]ConnectionHealthState{
		"gpt-4o": {ConnectionID: "sub2api:ws1:1515", ModelName: "gpt-4o", State: StateHealthy, LastProbeAt: &lastProbe},
	}
	if jobs := service.collectAdminProbeJobs(context.Background(), []Policy{policy}, assignments); len(jobs) != 0 {
		t.Fatalf("60 minute override must suppress a probe after only 30 minutes: %+v", jobs)
	}

	lastProbe = time.Now().Add(-61 * time.Minute)
	repo.states["sub2api:ws1:1515"]["gpt-4o"] = ConnectionHealthState{
		ConnectionID: "sub2api:ws1:1515", ModelName: "gpt-4o", State: StateHealthy, LastProbeAt: &lastProbe,
	}
	jobs := service.collectAdminProbeJobs(context.Background(), []Policy{policy}, assignments)
	if len(jobs) != 1 || len(jobs[0].dueSpecs) != 1 || jobs[0].dueSpecs[0].budgetPolicy.ID != policy.ID {
		t.Fatalf("expected one due model attributed to slow policy budget: %+v", jobs)
	}
}

func TestCollectAdminProbeJobs_MultipleUnschedulablePoliciesKeepAllSources(t *testing.T) {
	repo := newFakeRepository()
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}},
		platformGroups: sub2APISchedulerReader(boolPointer(false)),
	}
	stop := schedulableProbePolicy("stop", false, 60)
	keep := schedulableProbePolicy("keep", true, 120)
	assignments := []PolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "sub2api:ws1:1515", PolicyID: stop.ID},
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "sub2api:ws1:1515", PolicyID: keep.ID},
	}
	jobs := service.collectAdminProbeJobs(context.Background(), []Policy{stop, keep}, assignments)
	if len(jobs) != 1 || len(jobs[0].dueSpecs) != 1 {
		t.Fatalf("one continuing policy must keep automatic monitoring: %+v", jobs)
	}
	spec := jobs[0].dueSpecs[0]
	if len(spec.policies) != 2 || spec.budgetPolicy.ID != keep.ID {
		t.Fatalf("all sources and continuing policy budget owner must be retained: %+v", spec)
	}
}

func TestCollectAdminProbeJobs_Sub2APIFallsBackWhenFastPolicyBudgetIsExhausted(t *testing.T) {
	repo := newFakeRepository()
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}},
		platformGroups: sub2APISchedulerReader(boolPointer(true)),
	}
	fast := Policy{
		ID: "fast", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		ProbeIntervalSeconds: 60, DailyProbeBudget: 1,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	slow := Policy{
		ID: "slow", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		ProbeIntervalSeconds: 300, DailyProbeBudget: 10,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	assignments := []PolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "sub2api:ws1:1515", PolicyID: fast.ID},
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "sub2api:ws1:1515", PolicyID: slow.ID},
	}
	lastProbe := time.Now().Add(-10 * time.Minute)
	repo.states["sub2api:ws1:1515"] = map[string]ConnectionHealthState{
		"gpt-4o": {ConnectionID: "sub2api:ws1:1515", ModelName: "gpt-4o", State: StateHealthy, LastProbeAt: &lastProbe},
	}
	repo.events = []ConnectionHealthEvent{{
		UserID: "user1", AdminAccountID: "ws1", PolicyID: fast.ID, Result: string(ResultOK), CreatedAt: time.Now(),
	}}

	jobs := service.collectAdminProbeJobs(context.Background(), []Policy{fast, slow}, assignments)
	if len(jobs) != 1 || len(jobs[0].dueSpecs) != 1 {
		t.Fatalf("the slower policy should keep the due probe after the fast budget is exhausted: %+v", jobs)
	}
	if jobs[0].dueSpecs[0].budgetPolicy.ID != slow.ID || len(jobs[0].dueSpecs[0].policies) != 2 {
		t.Fatalf("unexpected fallback budget attribution or policy sources: %+v", jobs[0].dueSpecs[0])
	}
}

func TestCollectAdminProbeJobs_NewAPIPreservesSinglePolicyBudgetBehavior(t *testing.T) {
	repo := newFakeRepository()
	service := &Service{
		repo: repo, mySites: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: schedulerReader("100"),
	}
	fast := Policy{
		ID: "fast", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		ProbeIntervalSeconds: 60, DailyProbeBudget: 1,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	slow := Policy{
		ID: "slow", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		ProbeIntervalSeconds: 300, DailyProbeBudget: 10,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}
	assignments := []PolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: fast.ID},
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: slow.ID},
	}
	lastProbe := time.Now().Add(-10 * time.Minute)
	repo.states["newapi:ws1:100"] = map[string]ConnectionHealthState{
		"gpt-4o": {ConnectionID: "newapi:ws1:100", ModelName: "gpt-4o", State: StateHealthy, LastProbeAt: &lastProbe},
	}
	repo.events = []ConnectionHealthEvent{{
		UserID: "user1", AdminAccountID: "ws1", PolicyID: fast.ID, Result: string(ResultOK), CreatedAt: time.Now(),
	}}

	jobs := service.collectAdminProbeJobs(context.Background(), []Policy{fast, slow}, assignments)
	if len(jobs) != 0 {
		t.Fatalf("P version must not add multi-policy budget fallback to NewAPI: %+v", jobs)
	}
}

// TestCollectAdminProbeJobs_GeneratesDueTargets 验证独立探活调度：为可探活、到期（从未探活）的
// 目标模型生成任务，禁用的模型目标不生成任务。
func TestCollectAdminProbeJobs_GeneratesDueTargets(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := &Service{repo: repo, mySites: mySites, platformGroups: schedulerReader("100")}

	policies := []Policy{{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60,
		ModelTargets: []ModelTarget{
			{ModelName: "gpt-4o", Enabled: true},
			{ModelName: "disabled-model", Enabled: false},
		},
	}}
	assignments := []PolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: "p1"},
	}
	jobs := svc.collectAdminProbeJobs(context.Background(), policies, assignments)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 target job, got %d", len(jobs))
	}
	if jobs[0].target.TargetID != "newapi:ws1:100" {
		t.Fatalf("unexpected targetId: %q", jobs[0].target.TargetID)
	}
	if len(jobs[0].dueSpecs) != 1 || jobs[0].dueSpecs[0].modelName != "gpt-4o" {
		t.Fatalf("expected only enabled gpt-4o due, got %+v", jobs[0].dueSpecs)
	}
}

// TestCollectAdminProbeJobs_UnassignedTargetNeverScheduled 验证核心新语义：即使 workspace 有启用
// 策略且模型能匹配，没有显式分配关系的 target 也绝不会自动探活。
func TestCollectAdminProbeJobs_UnassignedTargetNeverScheduled(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := &Service{repo: repo, mySites: mySites, platformGroups: schedulerReader("100")}

	policies := []Policy{{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}},
	}}
	jobs := svc.collectAdminProbeJobs(context.Background(), policies, nil)
	if len(jobs) != 0 {
		t.Fatalf("expected no jobs without any policy assignment, got %d", len(jobs))
	}
}

// TestCollectAdminProbeJobs_MultiplierOnlyNeverScheduled guards the central safety contract:
// even legacy or malformed rows that still contain model targets cannot enter probe scheduling.
func TestCollectAdminProbeJobs_MultiplierOnlyNeverScheduled(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := &Service{repo: repo, mySites: mySites, platformGroups: schedulerReader("100")}
	policies := []Policy{{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		StrategyMode: StrategyModeMultiplierOnly, PriorityMode: PriorityModeMultiplier,
		ModelTargets: []ModelTarget{{ModelName: "legacy-model", Enabled: true}},
	}}
	assignments := []PolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: "p1",
	}}

	jobs := svc.collectAdminProbeJobs(context.Background(), policies, assignments)
	if len(jobs) != 0 {
		t.Fatalf("multiplier-only policy must never generate probe jobs: %+v", jobs)
	}
}

// TestCollectAdminProbeJobs_AssignmentToDisabledPolicyIgnored 验证分配指向的策略如果已被禁用，
// 该分配不生效（因为 policies 只包含 ListEnabledPolicies 的结果，policyByID 查不到）。
func TestCollectAdminProbeJobs_AssignmentToDisabledPolicyIgnored(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := &Service{repo: repo, mySites: mySites, platformGroups: schedulerReader("100")}

	// 模拟调度器视角：runSchedulerTick 只会把 ListEnabledPolicies 的结果传进来，
	// 一条被禁用的策略永远不会出现在 policies 参数里，即使它有分配记录。
	policies := []Policy{}
	assignments := []PolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: "disabled-policy"},
	}
	jobs := svc.collectAdminProbeJobs(context.Background(), policies, assignments)
	if len(jobs) != 0 {
		t.Fatalf("expected assignment to a disabled/nonexistent policy to be ignored, got %d jobs", len(jobs))
	}
}

// TestCollectAdminProbeJobs_OnlyUsesAssignedPolicies 验证 workspace 下其它启用策略，如果没有
// 分配给某个 target，就不会影响该 target 的候选模型计算——即使那条策略的模型池能匹配上。
func TestCollectAdminProbeJobs_OnlyUsesAssignedPolicies(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := &Service{repo: repo, mySites: mySites, platformGroups: schedulerReader("100")}

	policies := []Policy{
		{ID: "assigned", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60,
			ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}}},
		{ID: "not-assigned", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60,
			ModelTargets: []ModelTarget{{ModelName: "gpt-4o-mini", Enabled: true}}},
	}
	assignments := []PolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: "assigned"},
	}
	jobs := svc.collectAdminProbeJobs(context.Background(), policies, assignments)
	if len(jobs) != 1 {
		t.Fatalf("expected 1 target job, got %d", len(jobs))
	}
	for _, spec := range jobs[0].dueSpecs {
		if spec.modelName == "gpt-4o-mini" {
			t.Fatalf("model from unassigned policy must not be scheduled: %+v", jobs[0].dueSpecs)
		}
	}
	if len(jobs[0].dueSpecs) != 1 || jobs[0].dueSpecs[0].modelName != "gpt-4o" {
		t.Fatalf("expected only gpt-4o (from assigned policy) due, got %+v", jobs[0].dueSpecs)
	}
}

// TestCollectAdminProbeJobs_SkipsUnavailableTargets 验证不可探活目标（new-api 缺 base_url）不排期。
func TestCollectAdminProbeJobs_SkipsUnavailableTargets(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", Models: "gpt-4o"}}}, // 无 base_url
	}
	svc := &Service{repo: repo, mySites: mySites, platformGroups: reader}

	policies := []Policy{{ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60, ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}}}}
	assignments := []PolicyAssignment{{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: "p1"}}
	jobs := svc.collectAdminProbeJobs(context.Background(), policies, assignments)
	if len(jobs) != 0 {
		t.Fatalf("expected unavailable target to be skipped, got %d jobs", len(jobs))
	}
}

// TestCollectAdminProbeJobs_CapsAtMaxJobsPerTick 验证单轮到期模型任务总数受 maxJobsPerTick 限制。
func TestCollectAdminProbeJobs_CapsAtMaxJobsPerTick(t *testing.T) {
	repo := newFakeRepository()
	ids := make([]string, 0, maxJobsPerTick+50)
	for i := range maxJobsPerTick + 50 {
		ids = append(ids, fmt.Sprintf("%d", i))
	}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := &Service{repo: repo, mySites: mySites, platformGroups: schedulerReader(ids...)}

	policies := []Policy{{ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60, ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}}}}
	assignments := make([]PolicyAssignment, 0, len(ids))
	for _, id := range ids {
		assignments = append(assignments, PolicyAssignment{UserID: "user1", AdminAccountID: "ws1", TargetID: buildTargetID("newapi", "ws1", id), PolicyID: "p1"})
	}
	jobs := svc.collectAdminProbeJobs(context.Background(), policies, assignments)
	total := 0
	for _, j := range jobs {
		total += len(j.dueSpecs)
	}
	if total != maxJobsPerTick {
		t.Fatalf("expected due model tasks capped at %d, got %d", maxJobsPerTick, total)
	}
}

// TestCollectAdminProbeJobs_MultiWorkspaceIsolation 验证多 workspace 隔离：每个 workspace 的策略
// 只为自己 workspace 生成目标（targetId 内嵌各自 adminAccountID）。
func TestCollectAdminProbeJobs_MultiWorkspaceIsolation(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := &Service{repo: repo, mySites: mySites, platformGroups: schedulerReader("100")}

	policies := []Policy{
		{ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ProbeIntervalSeconds: 60, ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}}},
		{ID: "p2", UserID: "user1", AdminAccountID: "ws2", Enabled: true, ProbeIntervalSeconds: 60, ModelTargets: []ModelTarget{{ModelName: "gpt-4o", Enabled: true}}},
	}
	assignments := []PolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", TargetID: buildTargetID("newapi", "ws1", "100"), PolicyID: "p1"},
		{UserID: "user1", AdminAccountID: "ws2", TargetID: buildTargetID("newapi", "ws2", "100"), PolicyID: "p2"},
	}
	jobs := svc.collectAdminProbeJobs(context.Background(), policies, assignments)
	if len(jobs) != 2 {
		t.Fatalf("expected one job per workspace (2 total), got %d", len(jobs))
	}
	seen := map[string]bool{}
	for _, j := range jobs {
		seen[j.adminAccountID] = true
		if j.target.TargetID != buildTargetID("newapi", j.adminAccountID, "100") {
			t.Fatalf("target %q does not embed its workspace %q", j.target.TargetID, j.adminAccountID)
		}
	}
	if !seen["ws1"] || !seen["ws2"] {
		t.Fatalf("expected both workspaces scheduled, got %+v", seen)
	}
}

func TestRunSchedulerTick_DoesNotWritePriorityWhenOneTargetHealthChangesWithoutReordering(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	repo := newFakeRepository()
	policy := Policy{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true,
		ProbeIntervalSeconds: 60, DailyProbeBudget: 10, AutoDegradeEnabled: true,
		PriorityMode: PriorityModeMultiplier, StrategyMode: StrategyModeHealthProbe,
		ModelTargets: []ModelTarget{{ModelName: "gpt-4o", ProviderFamily: ProviderOpenAI, Enabled: true, MaxProbeTokens: 1}},
	}
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: policy.ID,
	}}
	fallback := 0.4
	repo.groupSortSettings["user1|ws1|g1"] = GroupProbeSortSetting{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", FallbackMultiplier: &fallback,
	}
	initialPriority := desiredHealthPriorityForPlatform(upstream.PlatformNewAPI, 3, 0)
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100", Name: "channel", Priority: &initialPriority, BaseURL: server.URL, Models: "gpt-4o"}},
		},
		credByAccount: map[string]upstream.ProbeCredential{
			"100": {BaseURL: server.URL, Key: "secret"},
		},
	}
	priorityActions := &fakeTargetPriorityActioner{}
	mySites := fakeAdminGroupKeyReader{
		fakeMySitesReader: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		keysBySite:        map[string][]upstream.Sub2APIKeyItem{},
	}
	service := &Service{
		repo: repo, mySites: mySites, sites: fakeSiteLookup{site: &upstream.Site{}},
		platformGroups: reader, priorityActions: priorityActions, dispatcher: noopRemoteActionRunner{},
		probeRunner: NewRealProbeRunner(),
	}
	inventory, err := service.loadAdminInventory(context.Background(), "user1", "ws1", make(adminInventoryCache))
	if err != nil {
		t.Fatalf("prime inventory snapshot: %v", err)
	}
	service.putAdminInventorySnapshot(context.Background(), "user1", "ws1", inventory, time.Now().UTC(), time.Minute)

	service.runSchedulerTick(context.Background())

	stored := repo.states["newapi:ws1:100"]["gpt-4o"]
	if stored.State != StateSuspended {
		t.Fatalf("probe state = %q, want suspended", stored.State)
	}
	if len(priorityActions.calls) != 0 {
		t.Fatalf("a health-only change without a global order change must not write priority: calls=%+v priority_states=%+v states=%+v", priorityActions.calls, repo.priorityStates, repo.states)
	}
	if repo.priorityLeaseCount["user1|ws1"] != 1 {
		t.Fatalf("probe tick must only lease the local post-probe evaluation: %+v", repo.priorityLeaseCount)
	}
}
