// 销售分析（GET /api/sales/summary）。金额均已按「我的站点售价」换算为人民币。

export type SalesChannel = 'payment' | 'redeem' | 'admin_add' | 'admin_deduct' | 'affiliate' | 'credit' | 'debit'

export interface SalesDayRange {
  from: string
  to: string
  days: number
}

export interface SalesCoverage {
  synced: boolean
  platform: string
  method: '' | 'records' | 'snapshot'
  rechargeFrom?: string
  rechargeTo?: string
  usageFrom?: string
  usageTo?: string
  rechargeComplete: boolean
  usageComplete: boolean
  previousComplete: boolean
  lastSyncedAt: string | null
}

export interface SalesPeriodTotals {
  credits: number
  deductions: number
  events: number
  payers: number
  newPayers: number | null
  repeatPayers: number
  consumption: number | null
  registered: number
  registeredPaid: number
  balanceStart: number | null
  balanceEnd: number | null
}

export interface SalesChannelTotal {
  channel: SalesChannel | string
  amount: number
  events: number
  users: number
  /** 整个渠道都不计入（amount 是重新计入时会加上的金额）。 */
  excluded?: boolean
  /** 这个渠道里单独选择不计入的明细（金额、条数）。 */
  excludedAmount?: number
  excludedEntries?: number
}

/** 区间内没有计入的部分（上面的数字已经去掉了）。 */
export interface SalesExclusions {
  channels: string[]
  entries: number
  amount: number
}

/** 一条入账明细：某个用户某天某个渠道的入账合计。 */
export interface SalesEntry {
  siteUserId: string
  email: string
  username: string
  day: string
  channel: string
  amount: number
  events: number
  /** 不计入的部分；整条都不计入时等于 amount，0 表示计入。 */
  excludedAmount: number
  excludedEvents: number
}

export interface SalesEntriesResponse {
  items: SalesEntry[]
  total: number
  page: number
  pageSize: number
  excludedChannels: string[]
}

export interface SalesEntriesQuery {
  from: string
  to: string
  channel?: string
  q?: string
  excluded?: boolean
  page?: number
}

export interface SalesEntryKey {
  siteUserId: string
  day: string
  channel: string
}

export interface SalesDailyPoint {
  day: string
  credits: number | null
  deductions: number | null
  channels: Record<string, number>
  consumption: number | null
}

export interface SalesDistributionBucket {
  min: number
  max: number | null
  users: number
  amount: number
}

export interface SalesPayerRow {
  siteUserId: string
  email: string
  username: string
  status: string
  credits: number
  events: number
  activeDays: number
  lastCreditDay: string
  consumption: number
  balance: number | null
  channels: SalesChannelTotal[]
}

export interface SalesSummary {
  range: SalesDayRange
  previousRange: SalesDayRange
  unitPrice: number
  coverage: SalesCoverage
  current: SalesPeriodTotals
  previous: SalesPeriodTotals | null
  channels: SalesChannelTotal[]
  daily: SalesDailyPoint[]
  concentration: { top1Share: number, top10Share: number }
  distribution: SalesDistributionBucket[]
  topPayers: SalesPayerRow[]
  exclusions: SalesExclusions
}

export type SalesRangeQuery = { days: number } | { from: string, to: string }
