package connection_health

import (
	"testing"
	"time"
)

func testPolicy() Policy {
	return Policy{
		FailureThreshold:    3,
		SuccessThreshold:    2,
		CooldownSeconds:     300,
		ObservationSeconds:  300,
		RecoveryStepPercent: 25,
	}
}

func TestTransition_SoftFailureDegradesGradually(t *testing.T) {
	policy := testPolicy()
	now := time.Now()

	// 健康状态下第一次网络波动：进入 degraded，权重下降，不直接暂停。
	out := Transition(TransitionInput{
		Current:       StateHealthy,
		CurrentWeight: 100,
		Now:           now,
		Result:        ResultNetworkFluctuation,
		Policy:        policy,
	})
	if out.NextState != StateDegraded {
		t.Fatalf("expected degraded, got %s", out.NextState)
	}
	if out.Weight != 75 {
		t.Fatalf("expected weight 75, got %d", out.Weight)
	}
	if out.TriggerRemoteDegrade {
		t.Fatalf("first soft failure should not trigger remote degrade")
	}
}

func TestTransition_SoftFailureSuspendsAtThreshold(t *testing.T) {
	policy := testPolicy()
	now := time.Now()

	out := Transition(TransitionInput{
		Current:             StateDegraded,
		CurrentWeight:       50,
		ConsecutiveFailures: 2, // 第三次即达到 failureThreshold=3
		Now:                 now,
		Result:              ResultRateLimited,
		Policy:              policy,
	})
	if out.NextState != StateSuspended {
		t.Fatalf("expected suspended at failure threshold, got %s", out.NextState)
	}
	if out.Weight != 0 {
		t.Fatalf("expected weight 0 when suspended, got %d", out.Weight)
	}
	if !out.TriggerRemoteDegrade {
		t.Fatalf("expected remote degrade to trigger on entering suspended")
	}
	if out.CooldownUntil == nil || !out.CooldownUntil.After(now) {
		t.Fatalf("expected cooldown_until to be set in the future")
	}
}

func TestTransition_HardFailureSuspendsImmediately(t *testing.T) {
	policy := testPolicy()
	now := time.Now()

	out := Transition(TransitionInput{
		Current:       StateHealthy,
		CurrentWeight: 100,
		Now:           now,
		Result:        ResultServerError,
		Policy:        policy,
	})
	if out.NextState != StateSuspended {
		t.Fatalf("expected immediate suspend on 5xx, got %s", out.NextState)
	}
	if !out.TriggerRemoteDegrade {
		t.Fatalf("expected remote degrade to trigger")
	}

	// 认证失败同样直接暂停。
	out2 := Transition(TransitionInput{
		Current:       StateDegraded,
		CurrentWeight: 60,
		Now:           now,
		Result:        ResultAuth,
		Policy:        policy,
	})
	if out2.NextState != StateSuspended {
		t.Fatalf("expected immediate suspend on auth failure, got %s", out2.NextState)
	}
}

func TestTransition_SuspendedSuccessEntersObserving(t *testing.T) {
	policy := testPolicy()
	now := time.Now()

	out := Transition(TransitionInput{
		Current:       StateSuspended,
		CurrentWeight: 0,
		Now:           now,
		Result:        ResultOK,
		Policy:        policy,
	})
	if out.NextState != StateObserving {
		t.Fatalf("expected observing after cooldown success, got %s", out.NextState)
	}
	if out.ObservingUntil == nil || !out.ObservingUntil.After(now) {
		t.Fatalf("expected observing_until set in the future")
	}
	if out.TriggerRemoteDegrade || out.TriggerRemoteRestore {
		t.Fatalf("entering observing should not itself trigger remote actions")
	}
}

func TestTransition_ObservingThenRecoveringThenHealthy(t *testing.T) {
	policy := testPolicy()
	now := time.Now()
	observingUntil := now.Add(5 * time.Minute)

	// 观察期第一次成功：未达到 successThreshold=2，继续观察。
	out1 := Transition(TransitionInput{
		Current:              StateObserving,
		CurrentWeight:        0,
		ConsecutiveSuccesses: 0,
		ObservingUntil:       &observingUntil,
		Now:                  now,
		Result:               ResultOK,
		Policy:               policy,
	})
	if out1.NextState != StateObserving {
		t.Fatalf("expected still observing after 1 success, got %s", out1.NextState)
	}

	// 观察期第二次连续成功：达到阈值，进入 recovering 并按 step 恢复权重。
	out2 := Transition(TransitionInput{
		Current:              StateObserving,
		CurrentWeight:        0,
		ConsecutiveSuccesses: out1.ConsecutiveSuccesses,
		ObservingUntil:       &observingUntil,
		Now:                  now,
		Result:               ResultOK,
		Policy:               policy,
	})
	if out2.NextState != StateRecovering {
		t.Fatalf("expected recovering after reaching success threshold, got %s", out2.NextState)
	}
	if out2.Weight != 25 {
		t.Fatalf("expected weight to start at recovery step 25, got %d", out2.Weight)
	}
	if !out2.TriggerRemoteRestore {
		t.Fatalf("expected remote restore to trigger on entering recovering")
	}

	// recovering 阶段继续成功，权重逐步恢复直至 100 -> healthy。
	weight := out2.Weight
	state := out2.NextState
	for i := 0; i < 10 && state != StateHealthy; i++ {
		next := Transition(TransitionInput{
			Current:       state,
			CurrentWeight: weight,
			Now:           now,
			Result:        ResultOK,
			Policy:        policy,
		})
		weight = next.Weight
		state = next.NextState
	}
	if state != StateHealthy || weight != 100 {
		t.Fatalf("expected full recovery to healthy/100, got state=%s weight=%d", state, weight)
	}
}

func TestTransition_DisabledOnlyExitsManually(t *testing.T) {
	policy := testPolicy()
	now := time.Now()

	out := Transition(TransitionInput{
		Current:       StateDisabled,
		CurrentWeight: 0,
		Now:           now,
		Result:        ResultOK,
		Policy:        policy,
	})
	if out.NextState != StateDisabled {
		t.Fatalf("disabled state must not change automatically from probe results, got %s", out.NextState)
	}

	out2 := Transition(TransitionInput{
		Current:       StateDisabled,
		CurrentWeight: 0,
		Now:           now,
		Result:        ResultServerError,
		Policy:        policy,
	})
	if out2.NextState != StateDisabled {
		t.Fatalf("disabled state must not change automatically even on hard failure, got %s", out2.NextState)
	}
}

func TestProbeBackoff(t *testing.T) {
	cases := []struct {
		failures int
		want     time.Duration
	}{
		{0, 0},
		{1, 2 * time.Minute},
		{2, 5 * time.Minute},
		{3, 10 * time.Minute},
		{9, 10 * time.Minute},
	}
	for _, c := range cases {
		if got := ProbeBackoff(c.failures); got != c.want {
			t.Fatalf("ProbeBackoff(%d) = %s, want %s", c.failures, got, c.want)
		}
	}
}
