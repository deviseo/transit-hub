package connection_health

import "time"

// TransitionInput 是状态机做一次决策所需的全部输入：探活前的状态快照 + 本次探活结果 + 所属策略阈值。
// 不依赖任何 IO，纯函数，便于单测覆盖全部分支。
type TransitionInput struct {
	Current              State
	CurrentWeight        int
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	ObservingUntil       *time.Time
	Now                  time.Time
	Result               ResultKey
	Policy               Policy
}

// TransitionOutput 是状态机决策的结果：新状态 + 新权重 + 计数器 + 是否需要触发远端降级/恢复动作。
type TransitionOutput struct {
	NextState            State
	Weight               int
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	CooldownUntil        *time.Time
	ObservingUntil       *time.Time
	TriggerRemoteDegrade bool
	TriggerRemoteRestore bool
}

// isHardFailure 分类：5xx、认证失败、模型不存在，无需累计失败次数即可直接暂停。
func isHardFailure(result ResultKey) bool {
	switch result {
	case ResultServerError, ResultAuth, ResultModelNotFound:
		return true
	default:
		return false
	}
}

// isSoftFailure 分类：网络波动、限流、响应无法解析，先降级观察，达到阈值才暂停。
func isSoftFailure(result ResultKey) bool {
	switch result {
	case ResultNetworkFluctuation, ResultRateLimited, ResultInvalidResponse:
		return true
	default:
		return false
	}
}

// Transition 是健康状态机的核心决策函数。disabled 只能人工进出，探活结果不会自动改变它。
func Transition(in TransitionInput) TransitionOutput {
	if in.Current == StateDisabled {
		return TransitionOutput{
			NextState:            StateDisabled,
			Weight:               0,
			ConsecutiveFailures:  in.ConsecutiveFailures,
			ConsecutiveSuccesses: in.ConsecutiveSuccesses,
			ObservingUntil:       in.ObservingUntil,
		}
	}

	step := stepPercent(in.Policy)

	switch {
	case in.Result == ResultOK:
		return transitionOnSuccess(in, step)
	case isHardFailure(in.Result):
		return transitionOnHardFailure(in)
	case isSoftFailure(in.Result):
		return transitionOnSoftFailure(in, step)
	default:
		// unsupported 等非探活结果不驱动状态机，原样保持。
		return TransitionOutput{
			NextState:            in.Current,
			Weight:               in.CurrentWeight,
			ConsecutiveFailures:  in.ConsecutiveFailures,
			ConsecutiveSuccesses: in.ConsecutiveSuccesses,
			ObservingUntil:       in.ObservingUntil,
		}
	}
}

func stepPercent(p Policy) int {
	if p.RecoveryStepPercent <= 0 {
		return 25
	}
	return p.RecoveryStepPercent
}

func successThreshold(p Policy) int {
	if p.SuccessThreshold <= 0 {
		return 2
	}
	return p.SuccessThreshold
}

func failureThreshold(p Policy) int {
	if p.FailureThreshold <= 0 {
		return 3
	}
	return p.FailureThreshold
}

func transitionOnSuccess(in TransitionInput, step int) TransitionOutput {
	out := TransitionOutput{
		ConsecutiveFailures:  0,
		ConsecutiveSuccesses: in.ConsecutiveSuccesses + 1,
	}

	switch in.Current {
	case StateHealthy:
		out.NextState = StateHealthy
		out.Weight = 100

	case StateDegraded:
		weight := minInt(100, in.CurrentWeight+step)
		if weight >= 100 {
			out.NextState = StateHealthy
			out.Weight = 100
		} else {
			out.NextState = StateDegraded
			out.Weight = weight
		}

	case StateSuspended:
		// 冷却后探活成功：进入 observing，权重从 0 起步观察，不立即恢复调用。
		observingUntil := in.Now.Add(observationWindow(in.Policy))
		out.NextState = StateObserving
		out.Weight = 0
		out.ObservingUntil = &observingUntil
		out.ConsecutiveSuccesses = 1

	case StateObserving:
		out.ObservingUntil = in.ObservingUntil
		// 观察期和连续成功阈值必须同时满足。旧数据可能没有 observing_until，
		// 此时只按成功阈值判断，保持升级前已进入 observing 的状态可继续恢复。
		observationFinished := in.ObservingUntil == nil || !in.Now.Before(*in.ObservingUntil)
		if observationFinished && out.ConsecutiveSuccesses >= successThreshold(in.Policy) {
			out.NextState = StateRecovering
			out.Weight = minInt(100, step)
			out.TriggerRemoteRestore = true
		} else {
			out.NextState = StateObserving
			out.Weight = in.CurrentWeight
		}

	case StateRecovering:
		weight := minInt(100, in.CurrentWeight+step)
		if weight >= 100 {
			out.NextState = StateHealthy
			out.Weight = 100
		} else {
			out.NextState = StateRecovering
			out.Weight = weight
		}
		out.TriggerRemoteRestore = true

	default:
		out.NextState = StateHealthy
		out.Weight = 100
	}

	return out
}

func transitionOnSoftFailure(in TransitionInput, step int) TransitionOutput {
	out := TransitionOutput{
		ConsecutiveSuccesses: 0,
		ConsecutiveFailures:  in.ConsecutiveFailures + 1,
	}

	switch in.Current {
	case StateHealthy:
		out.NextState = StateDegraded
		out.Weight = maxInt(0, 100-step)

	case StateDegraded, StateObserving, StateRecovering:
		if out.ConsecutiveFailures >= failureThreshold(in.Policy) {
			cooldownUntil := in.Now.Add(cooldownWindow(in.Policy))
			out.NextState = StateSuspended
			out.Weight = 0
			out.CooldownUntil = &cooldownUntil
			out.TriggerRemoteDegrade = true
		} else {
			out.NextState = StateDegraded
			out.Weight = maxInt(0, in.CurrentWeight-step)
		}

	case StateSuspended:
		cooldownUntil := in.Now.Add(cooldownWindow(in.Policy))
		out.NextState = StateSuspended
		out.Weight = 0
		out.CooldownUntil = &cooldownUntil

	default:
		out.NextState = StateDegraded
		out.Weight = maxInt(0, 100-step)
	}

	return out
}

func transitionOnHardFailure(in TransitionInput) TransitionOutput {
	cooldownUntil := in.Now.Add(cooldownWindow(in.Policy))
	return TransitionOutput{
		NextState:            StateSuspended,
		Weight:               0,
		ConsecutiveFailures:  in.ConsecutiveFailures + 1,
		ConsecutiveSuccesses: 0,
		CooldownUntil:        &cooldownUntil,
		TriggerRemoteDegrade: in.Current != StateSuspended,
	}
}

func observationWindow(p Policy) time.Duration {
	if p.ObservationSeconds <= 0 {
		return 300 * time.Second
	}
	return time.Duration(p.ObservationSeconds) * time.Second
}

func cooldownWindow(p Policy) time.Duration {
	if p.CooldownSeconds <= 0 {
		return 300 * time.Second
	}
	return time.Duration(p.CooldownSeconds) * time.Second
}

// ProbeBackoff 按连续失败次数返回下一次探活前的退避时长：2、5、10 分钟，超过后维持 10 分钟。
func ProbeBackoff(consecutiveFailures int) time.Duration {
	switch {
	case consecutiveFailures <= 0:
		return 0
	case consecutiveFailures == 1:
		return 2 * time.Minute
	case consecutiveFailures == 2:
		return 5 * time.Minute
	default:
		return 10 * time.Minute
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
