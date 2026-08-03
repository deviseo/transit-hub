package connection_health

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

type probeDecisionPolicyKey struct {
	ID                                string
	Enabled                           bool
	ProbeMode                         string
	ProbeIntervalSeconds              int
	ContinueProbeWhenUnschedulable    bool
	UnschedulableProbeIntervalMinutes int
	FailureThreshold                  int
	SuccessThreshold                  int
	CooldownSeconds                   int
	ObservationSeconds                int
	RecoveryStepPercent               int
	AutoDegradeEnabled                bool
	AutoRemoteActionEnabled           bool
	PriorityMode                      string
	StrategyMode                      string
	DailyProbeBudget                  int
}

type probeDecisionKeyInput struct {
	TargetID       string
	Platform       string
	ModelName      string
	ProviderFamily string
	MaxProbeTokens int
	ProbePrompt    string
	Schedulable    int
	SelectedPolicy string
	Policies       []probeDecisionPolicyKey
}

func probeDecisionKey(target AdminProbeTarget, spec probeModelSpec) string {
	providerFamily := spec.providerFamily
	if providerFamily == "" {
		providerFamily = target.ProviderFamily
	}
	maxProbeTokens := spec.maxProbeTokens
	if maxProbeTokens <= 0 {
		maxProbeTokens = 1
	}
	probePrompt := strings.TrimSpace(spec.probePrompt)
	if probePrompt == "" {
		probePrompt = defaultProbePrompt
	}
	schedulable := -1
	if target.Schedulable != nil {
		schedulable = 0
		if *target.Schedulable {
			schedulable = 1
		}
	}
	policies := spec.policies
	if len(policies) == 0 {
		policies = []Policy{spec.policy}
	}
	policyKeys := make([]probeDecisionPolicyKey, 0, len(policies))
	for _, policy := range policies {
		policyKeys = append(policyKeys, probeDecisionPolicyKey{
			ID: policy.ID, Enabled: policy.Enabled, ProbeMode: policy.ProbeMode,
			ProbeIntervalSeconds:              policy.ProbeIntervalSeconds,
			ContinueProbeWhenUnschedulable:    policy.ContinueProbeWhenUnschedulable,
			UnschedulableProbeIntervalMinutes: policy.UnschedulableProbeIntervalMinutes,
			FailureThreshold:                  policy.FailureThreshold, SuccessThreshold: policy.SuccessThreshold,
			CooldownSeconds: policy.CooldownSeconds, ObservationSeconds: policy.ObservationSeconds,
			RecoveryStepPercent: policy.RecoveryStepPercent, AutoDegradeEnabled: policy.AutoDegradeEnabled,
			AutoRemoteActionEnabled: policy.AutoRemoteActionEnabled, PriorityMode: policy.PriorityMode,
			StrategyMode: policy.StrategyMode, DailyProbeBudget: policy.DailyProbeBudget,
		})
	}
	sort.Slice(policyKeys, func(i, j int) bool { return policyKeys[i].ID < policyKeys[j].ID })
	payload, _ := json.Marshal(probeDecisionKeyInput{
		TargetID: target.TargetID, Platform: target.Platform, ModelName: spec.modelName,
		ProviderFamily: providerFamily, MaxProbeTokens: maxProbeTokens, ProbePrompt: probePrompt,
		Schedulable: schedulable, SelectedPolicy: spec.policy.ID,
		Policies: policyKeys,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func probeDecisionCanReuseInterval(state *ConnectionHealthState, decisionKey string) bool {
	return state == nil || state.LastProbeDecisionKey == "" || state.LastProbeDecisionKey == decisionKey
}
