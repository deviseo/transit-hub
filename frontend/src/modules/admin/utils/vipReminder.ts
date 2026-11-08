import type { VipRow } from '../types/vip'
import { formatCny } from './dashboard'

type Translate = (key: string, params?: Record<string, unknown>) => string

/** 预计可用天数的文字：不到 1 天、约 1.5 天、约 12 天（和提醒消息一致，往小取）。 */
export const vipDaysText = (t: Translate, days: number): string => {
  if (days < 1) return t('admin.vip.days.underOne')
  // 一位小数，整数时不带「.0」：约 1.5 天、约 3 天。
  if (days < 10) return t('admin.vip.days.about', { days: String(Math.floor(days * 10) / 10) })
  return t('admin.vip.days.about', { days: Math.floor(days) })
}

/** 用户在话术里的称呼：用户名，其次邮箱。 */
export const vipUserName = (row: Pick<VipRow, 'username' | 'email' | 'siteUserId'>): string => row.username || row.email || `#${row.siteUserId}`

/**
 * 「复制提醒」的话术：快用完和用完了各一段，可以在提醒设置里改。变量：{name} 称呼、{site} 站点名、
 * {balance} 余额、{days} 预计还能用多久。
 */
export const vipReminderText = (
  t: Translate,
  row: VipRow,
  site: string,
  templates: { reminderText: string, emptyReminderText: string },
): string => {
  const empty = row.status === 'empty' || (row.balance != null && row.balance <= 0)
  const values: Record<string, string> = {
    name: vipUserName(row),
    site,
    balance: formatCny(row.balance ?? 0),
    days: row.daysLeft != null ? vipDaysText(t, row.daysLeft) : t('admin.vip.days.unknown'),
  }
  const custom = (empty ? templates.emptyReminderText : templates.reminderText).trim()
  // 默认话术在语言包里，由 vue-i18n 填变量；自己写的话术在这里替换。
  if (!custom) return t(empty ? 'admin.vip.reminder.defaultEmpty' : 'admin.vip.reminder.defaultLow', values)
  return custom.replace(/\{(name|site|balance|days)\}/g, (_, key: string) => values[key])
}

/** 话术输入框的占位：默认话术，变量原样显示。 */
export const vipReminderPlaceholder = (t: Translate, empty: boolean): string =>
  t(empty ? 'admin.vip.reminder.defaultEmpty' : 'admin.vip.reminder.defaultLow', { name: '{name}', site: '{site}', balance: '{balance}', days: '{days}' })
