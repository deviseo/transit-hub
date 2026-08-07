import type { ConnectionHealthPolicy, ConnectionHealthStrategyMode } from '../types/connectionHealth'

// 兼容尚未返回 strategyMode 的旧后端：新版前端通过旧后端保存的仅倍率策略会表现为
// multiplier + 无模型目标 + 关闭自动降级，按该稳定特征恢复其真实模式。
export const resolveConnectionHealthStrategyMode = (
  policy: Pick<ConnectionHealthPolicy, 'strategyMode' | 'priorityMode' | 'autoDegradeEnabled' | 'modelTargets'>,
): ConnectionHealthStrategyMode => {
  if (policy.strategyMode === 'multiplier_only') return 'multiplier_only'
  if (
    !policy.strategyMode
    && policy.priorityMode === 'multiplier'
    && !policy.autoDegradeEnabled
    && policy.modelTargets.length === 0
  ) {
    return 'multiplier_only'
  }
  return 'health_probe'
}

export const preservePriorityMaxPendingAge = (
  existingSeconds: number | undefined,
  minWriteIntervalSeconds: number,
  defaultSeconds = 300,
): number => Math.max(existingSeconds ?? defaultSeconds, minWriteIntervalSeconds)
