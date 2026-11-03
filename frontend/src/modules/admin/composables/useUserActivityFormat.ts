import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

export interface DeltaValue {
  // 相对变化（0.25 = +25%）；上期为 0 时为 null
  ratio: number | null
  // 上期为 0、本期大于 0：「新增」
  fromZero: boolean
}

// 本期相对上期的变化。上期缺失（本地没有数据）返回 null，由界面显示「—」。
export const computeDelta = (current: number, previous: number | null | undefined): DeltaValue | null => {
  if (previous == null) return null
  if (previous === 0) return { ratio: current === 0 ? 0 : null, fromZero: current > 0 }
  return { ratio: (current - previous) / previous, fromZero: false }
}

export function useUserActivityFormat() {
  const { t, locale } = useI18n()

  const integerFormatter = computed(() => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 0 }))
  const compactFormatter = computed(() => new Intl.NumberFormat(locale.value, { notation: 'compact', maximumFractionDigits: 1 }))
  const percentFormatter = computed(() => new Intl.NumberFormat(locale.value, { style: 'percent', maximumFractionDigits: 1 }))
  const dateFormatter = computed(() => new Intl.DateTimeFormat(locale.value, { year: 'numeric', month: '2-digit', day: '2-digit' }))
  const dateTimeFormatter = computed(() => new Intl.DateTimeFormat(locale.value, {
    year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit',
  }))

  const formatCount = (value: number | null | undefined): string => integerFormatter.value.format(value ?? 0)

  const formatCompact = (value: number): string => (Math.abs(value) >= 10_000 ? compactFormatter.value.format(value) : integerFormatter.value.format(value))

  // 金额统一为美元口径（sub2api actual_cost；new-api 额度按 quota_per_unit 换算）。
  const formatUsd = (value: number | null | undefined): string => {
    if (value == null) return t('admin.userActivity.common.placeholder')
    const absolute = Math.abs(value)
    if (absolute >= 10_000_000) return `$${compactFormatter.value.format(value)}`
    const digits = absolute > 0 && absolute < 0.01 ? 4 : 2
    return `$${new Intl.NumberFormat(locale.value, { minimumFractionDigits: digits, maximumFractionDigits: digits }).format(value)}`
  }

  const formatUsdAxis = (value: number): string => `$${formatCompact(value)}`

  // 带正负号的金额（用于「较昨日末」等变化值）。
  const formatSignedUsd = (value: number): string => {
    const text = formatUsd(Math.abs(value))
    if (value > 0) return `+${text}`
    if (value < 0) return `−${text}`
    return text
  }

  const formatPercent = (ratio: number): string => percentFormatter.value.format(ratio)

  const formatSignedPercent = (ratio: number): string => {
    const text = percentFormatter.value.format(Math.abs(ratio))
    if (ratio > 0) return `+${text}`
    if (ratio < 0) return `−${text}`
    return text
  }

  // day 为 YYYY-MM-DD（业务时区的自然日），按日期本身展示，不做时区换算。
  const formatDay = (day: string | null | undefined): string => {
    if (!day) return t('admin.userActivity.common.placeholder')
    const [year, month, date] = day.split('-').map(Number)
    if (!year || !month || !date) return day
    return dateFormatter.value.format(new Date(year, month - 1, date))
  }

  const formatShortDay = (day: string): string => day.slice(5)

  const formatRange = (from?: string, to?: string): string => {
    if (!from) return t('admin.userActivity.common.placeholder')
    if (!to || from === to) return formatDay(from)
    return `${formatDay(from)} ~ ${formatDay(to)}`
  }

  const formatDateTime = (value: string | null | undefined): string => {
    if (!value) return t('admin.userActivity.common.placeholder')
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return t('admin.userActivity.common.placeholder')
    return dateTimeFormatter.value.format(date)
  }

  const formatRelativeTime = (value: string | null | undefined): string => {
    if (!value) return t('admin.userActivity.common.placeholder')
    const date = new Date(value)
    if (Number.isNaN(date.getTime())) return t('admin.userActivity.common.placeholder')
    const minutes = Math.max(0, Math.round((Date.now() - date.getTime()) / 60_000))
    if (minutes < 1) return t('admin.userActivity.time.justNow')
    if (minutes < 60) return t('admin.userActivity.time.minutesAgo', { count: minutes })
    const hours = Math.round(minutes / 60)
    if (hours < 24) return t('admin.userActivity.time.hoursAgo', { count: hours })
    return t('admin.userActivity.time.daysAgo', { count: Math.round(hours / 24) })
  }

  return {
    formatCount,
    formatCompact,
    formatUsd,
    formatUsdAxis,
    formatSignedUsd,
    formatPercent,
    formatSignedPercent,
    formatDay,
    formatShortDay,
    formatRange,
    formatDateTime,
    formatRelativeTime,
  }
}
