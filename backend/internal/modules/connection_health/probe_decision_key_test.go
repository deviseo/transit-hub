package connection_health

import (
	"context"
	"testing"
	"time"
)

func TestEffectiveProbeDecisionForSpec_ChangedRequestContractIsDueImmediately(t *testing.T) {
	now := time.Now().UTC()
	lastProbe := now.Add(-5 * time.Second)
	policy := Policy{ID: "p1", Enabled: true, ProbeIntervalSeconds: 60}
	target := AdminProbeTarget{TargetID: "newapi:ws1:100", Platform: "newapi", ProviderFamily: ProviderOpenAI, Schedulable: boolPointer(true)}
	oldSpec := probeModelSpec{modelName: "gpt-4o", providerFamily: ProviderOpenAI, maxProbeTokens: 1, probePrompt: "old", policy: policy, policies: []Policy{policy}}
	newSpec := oldSpec
	newSpec.probePrompt = "new"
	repo := newFakeRepository()
	repo.states[target.TargetID] = map[string]ConnectionHealthState{
		"gpt-4o": {
			ConnectionID: target.TargetID, ModelName: "gpt-4o", State: StateHealthy,
			LastProbeAt: &lastProbe, LastProbeDecisionKey: probeDecisionKey(target, oldSpec),
		},
	}
	svc := &Service{repo: repo}

	decision, ok := svc.effectiveProbeDecisionForSpec(context.Background(), target, newSpec, newSpec.policies, now, nil)
	if !ok || decision.NextProbeAt == nil || decision.NextProbeAt.After(now) {
		t.Fatalf("changed request contract must be immediately due: %+v ok=%v", decision, ok)
	}
	unchanged, ok := svc.effectiveProbeDecisionForSpec(context.Background(), target, oldSpec, oldSpec.policies, now, nil)
	if !ok || unchanged.NextProbeAt == nil || !unchanged.NextProbeAt.Equal(lastProbe.Add(time.Minute)) {
		t.Fatalf("same request contract must reuse the recent result: %+v ok=%v", unchanged, ok)
	}
}

func TestEffectiveProbeDecisionForSpec_ChangedRequestContractPreservesCooldownAndBackoff(t *testing.T) {
	now := time.Now().UTC()
	lastProbe := now.Add(-30 * time.Second)
	cooldown := now.Add(4 * time.Minute)
	policy := Policy{ID: "p1", Enabled: true, ProbeIntervalSeconds: 60}
	target := AdminProbeTarget{TargetID: "newapi:ws1:100", Platform: "newapi", ProviderFamily: ProviderOpenAI, Schedulable: boolPointer(true)}
	oldSpec := probeModelSpec{modelName: "gpt-4o", probePrompt: "old", policy: policy, policies: []Policy{policy}}
	newSpec := oldSpec
	newSpec.probePrompt = "new"

	tests := []struct {
		name       string
		state      State
		failures   int
		cooldown   *time.Time
		wantNext   time.Time
		wantReason string
	}{
		{name: "cooldown", state: StateSuspended, cooldown: &cooldown, wantNext: cooldown, wantReason: ProbeBlockedCooldown},
		{name: "failure backoff", state: StateDegraded, failures: 1, wantNext: lastProbe.Add(2 * time.Minute), wantReason: ProbeBlockedFailureBackoff},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepository()
			repo.states[target.TargetID] = map[string]ConnectionHealthState{
				"gpt-4o": {
					ConnectionID: target.TargetID, ModelName: "gpt-4o", State: tt.state,
					LastProbeAt: &lastProbe, ConsecutiveFailures: tt.failures, CooldownUntil: tt.cooldown,
					LastProbeDecisionKey: probeDecisionKey(target, oldSpec),
				},
			}
			svc := &Service{repo: repo}
			decision, ok := svc.effectiveProbeDecisionForSpec(context.Background(), target, newSpec, newSpec.policies, now, nil)
			if !ok || decision.NextProbeAt == nil || !decision.NextProbeAt.Equal(tt.wantNext) || decision.BlockedReason != tt.wantReason {
				t.Fatalf("changed request contract must preserve %s: %+v ok=%v", tt.wantReason, decision, ok)
			}
		})
	}
}

func TestProbeDecisionKey_IsIndependentOfPolicyInputOrder(t *testing.T) {
	target := AdminProbeTarget{TargetID: "newapi:ws1:100", Platform: "newapi"}
	first := Policy{ID: "a", Enabled: true, ProbeIntervalSeconds: 60}
	second := Policy{ID: "b", Enabled: true, ProbeIntervalSeconds: 120}
	spec := probeModelSpec{modelName: "gpt-4o", policy: first, policies: []Policy{first, second}}
	reordered := spec
	reordered.policies = []Policy{second, first}
	if probeDecisionKey(target, spec) != probeDecisionKey(target, reordered) {
		t.Fatal("policy enumeration order must not change the decision key")
	}
}

func TestModelHealthForSpecs_UsesSameDecisionKeyAsScheduler(t *testing.T) {
	now := time.Now().UTC()
	lastProbe := now.Add(-5 * time.Second)
	policy := Policy{ID: "p1", Enabled: true, ProbeIntervalSeconds: 60}
	target := AdminProbeTarget{TargetID: "newapi:ws1:100", Platform: "newapi", ProviderFamily: ProviderOpenAI, Schedulable: boolPointer(true)}
	oldSpec := probeModelSpec{modelName: "gpt-4o", probePrompt: "old", policy: policy, policies: []Policy{policy}}
	newSpec := oldSpec
	newSpec.probePrompt = "new"
	state := ConnectionHealthState{
		ConnectionID: target.TargetID, ModelName: "gpt-4o", State: StateHealthy, UpdatedAt: lastProbe,
		LastProbeAt: &lastProbe, LastProbeDecisionKey: probeDecisionKey(target, oldSpec),
	}

	models, _ := modelHealthForSpecs(map[string]ConnectionHealthState{"gpt-4o": state}, []probeModelSpec{newSpec}, target, now, nil, true)
	if len(models) != 1 || models[0].NextProbeAt == nil || models[0].NextProbeAt.After(now) {
		t.Fatalf("page decision must show the changed request contract as due: %+v", models)
	}
}
