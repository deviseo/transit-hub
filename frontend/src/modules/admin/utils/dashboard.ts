// 仪表盘共用的展示工具：主题色类名映射、CNY 金额格式化、环比变化计算。
// 颜色类使用「字面量字符串」写法，确保 Tailwind JIT 能扫描到对应工具类。

import type { DashboardColorToken } from '../types/dashboard'

/** 指标图标底色 + 文字色。 */
export const METRIC_ICON_CLASSES: Record<DashboardColorToken, string> = {
  primary: 'bg-primary/10 text-primary',
  accent: 'bg-accent/10 text-accent',
  signal: 'bg-signal/10 text-signal',
  warning: 'bg-warning/10 text-warning',
}

/** 趋势卡标题前的小圆点颜色。 */
export const METRIC_DOT_CLASSES: Record<DashboardColorToken, string> = {
  primary: 'bg-primary',
  accent: 'bg-accent',
  signal: 'bg-signal',
  warning: 'bg-warning',
}

/** 环比变化方向。 */
export type DeltaDirection = 'up' | 'down' | 'flat'

/** 不同方向的文字色（红色做了暗黑模式适配）。 */
export const DELTA_TEXT_CLASSES: Record<DeltaDirection, string> = {
  up: 'text-signal',
  down: 'text-red-500 dark:text-red-400',
  flat: 'text-muted-foreground',
}

// 固定使用 en-US 千分位分组，只影响数字分隔符（无本地化文字），保证两种语言下表现一致。
const cnyFormatter = new Intl.NumberFormat('en-US', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

/** 格式化为人民币显示，空值返回占位符。 */
export function formatCny(value: number | null | undefined): string {
  if (value == null || !Number.isFinite(value)) return '¥—'
  return `¥${cnyFormatter.format(value)}`
}

/** 把毫秒时间戳格式化为可读时间；空值或非数字返回 null，由调用方回退「未知」文案。 */
export function formatDateTime(ms: number | null | undefined, locale = 'zh-CN'): string | null {
  if (ms == null || !Number.isFinite(ms)) return null
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(new Date(ms))
}

export interface DeltaResult {
  amount: number
  direction: DeltaDirection
  unavailable?: boolean // true 时不显示数值（非相邻、成本不完整等）
  reason?: 'non_adjacent' | 'partial_cost' | 'missing_data'
}

/** 计算序列最后一个点相对前一个点的变化（今日 vs 昨日）。
 * 新版本接收带日期和状态的点，校验日期相邻性后再计算环比。 */
export function computeDelta(
  values: { value: number | null; date?: string; status?: string }[] | number[],
): DeltaResult {
  if (!values || values.length < 2) return { amount: 0, direction: 'flat' }

  // 兼容旧版 number[] 调用。
  if (typeof values[0] === 'number') {
    const nums = values as number[]
    const amount = nums[nums.length - 1] - nums[nums.length - 2]
    const direction: DeltaDirection = amount > 0 ? 'up' : amount < 0 ? 'down' : 'flat'
    return { amount, direction }
  }

  const pts = values as { value: number | null; date?: string; status?: string }[]
  const last = pts[pts.length - 1]
  const prev = pts[pts.length - 2]

  // 任一点为 null（指标不可用）时不计算环比。
  if (last.value === null || prev.value === null) {
    return { amount: 0, direction: 'flat', unavailable: true, reason: 'missing_data' }
  }

  // 日期相邻性校验：date 差恰好 1 天才计算环比。
  if (last.date && prev.date) {
    const lastDate = new Date(last.date).getTime()
    const prevDate = new Date(prev.date).getTime()
    const diffDays = Math.round((lastDate - prevDate) / (1000 * 60 * 60 * 24))
    if (diffDays !== 1) {
      return { amount: 0, direction: 'flat', unavailable: true, reason: 'non_adjacent' }
    }
  }

  // 成本未完整时不显示环比。
  if (last.status === 'partial' || prev.status === 'partial') {
    return { amount: 0, direction: 'flat', unavailable: true, reason: 'partial_cost' }
  }

  // 昨日未结算。
  if (prev.status === 'missing' || prev.status === 'provisional') {
    return { amount: 0, direction: 'flat', unavailable: true, reason: 'missing_data' }
  }

  const amount = last.value - prev.value
  const direction: DeltaDirection = amount > 0 ? 'up' : amount < 0 ? 'down' : 'flat'
  return { amount, direction }
}
