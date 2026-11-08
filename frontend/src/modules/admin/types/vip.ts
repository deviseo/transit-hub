// 大户关注（GET /api/vip/summary）。金额都已按「我的站点售价」换算成人民币。

export type VipStatus = 'normal' | 'low' | 'empty' | 'declining' | 'disabled'

export interface VipSettings {
  /** 推荐近 7 天日均消耗、累计消耗前几名（0 表示不按这一项推荐）。 */
  dailyTop: number
  lifetimeTop: number
  /** 预计几天内用完、或者余额低于多少元算快用完（0 表示不按这一项判断）。 */
  alertDays: number
  alertBalance: number
  /** 快用完、用完了时用这些机器人提醒站长。 */
  alertsEnabled: boolean
  botIds: string[]
  /** 「复制提醒」的话术，空表示用默认的。 */
  reminderText: string
  emptyReminderText: string
}

export interface VipSource {
  manual: boolean
  note: string
  dailyRank?: number
  lifetimeRank?: number
}

export interface VipRow {
  siteUserId: string
  email: string
  username: string
  status: VipStatus
  source: VipSource
  balance: number | null
  /** 近 7 天、再往前 7 天的日均消耗。 */
  dailyAverage: number
  previousAverage: number
  /** 近 7 天比前 7 天的变化（0.12 表示多了 12%）。 */
  trend: number | null
  /** 余额 ÷ 近 7 天日均消耗。 */
  daysLeft: number | null
  lifetime: number
  lastUsedAt: string | null
  lastCreditDay?: string
  lastCreditAmount?: number
}

export interface VipSummary {
  synced: boolean
  platform: string
  lastSyncedAt: string | null
  unitPrice: number
  siteName: string
  settings: VipSettings
  counts: { all: number, low: number, empty: number, declining: number }
  items: VipRow[]
  dismissedCount: number
}

export interface VipCandidate {
  siteUserId: string
  email: string
  username: string
  balance: number | null
  dailyAverage: number
  watched: boolean
  dismissed: boolean
}
