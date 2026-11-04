// 利润分析（GET /api/profit/summary）。金额均为人民币：营收按「我的站点售价」换算，成本按各上游的充值倍率换算。

export interface ProfitDayRange {
  from: string
  to: string
  days: number
}

export interface ProfitSyncStatus {
  platform: string
  status: '' | 'running' | 'success' | 'failed' | 'interrupted'
  errorKey?: string
  warningKey?: string
  revenueSource?: '' | 'sub2api_groups' | 'newapi_flow' | 'newapi_log_stat'
  routeCount: number
  syncedFrom?: string
  syncedTo?: string
  completeThrough?: string
  lastStartedAt: string | null
  lastSyncedAt: string | null
}

export interface ProfitTotals {
  revenue: number
  cost: number
  profit: number
  margin: number | null
  attributedRevenue: number
  unattributedRevenue: number
  attributedShare: number | null
  unmatchedCost: number
  failedSiteDays: number
}

export interface ProfitDailyPoint {
  day: string
  revenue: number | null
  cost: number | null
  profit: number | null
}

export interface ProfitRouteRef {
  id: string
  name: string
  groups: string[]
  status: string
  deleted: boolean
  revenue: number
}

export interface ProfitUpstreamSite {
  id: string
  name: string
  platform: string
  rechargeRate: number
  cost: number
  failedDays: number
  noRate: boolean
  deleted: boolean
  balance: number | null
  avgDailyCost: number | null
  runwayDays: number | null
  suggestedTopUp: number | null
}

export interface ProfitUpstreamRow {
  key: string
  sites: ProfitUpstreamSite[]
  routes: ProfitRouteRef[]
  revenue: number
  cost: number
  profit: number
  margin: number | null
}

export interface ProfitGroupRow {
  group: string
  revenue: number
  attributedRevenue: number
  unattributedRevenue: number
  cost: number
  profit: number
  margin: number | null
}

export interface ProfitUnattributedRow {
  key: string
  category: 'official' | 'unregistered' | 'unknown'
  routes: ProfitRouteRef[]
  revenue: number
}

export interface ProfitInsight {
  kind: 'negativeMargin' | 'unmatchedCost' | 'unregistered' | 'lowRunway' | 'costSpike' | 'noRate' | 'costFailed' | 'priceIncrease'
  severity: 'critical' | 'warning' | 'info'
  subject: string
  value?: number
  extra?: number
}

// 上游分组调价及对每天成本的估算（人民币）：日均成本按该分组近 windowDays 天在上游的消费 × 充值倍率，
// 每天影响 = 日均成本 × (新倍率 ÷ 旧倍率 − 1)，为负表示每天节省。
export interface ProfitPriceChange {
  id: number
  siteId: string
  siteName: string
  upstreamKey: string
  groupName: string
  oldMultiplier: number
  newMultiplier: number
  windowDays: number
  estimateStatus: 'ok' | 'no_usage' | 'no_rate' | 'failed' | ''
  avgDailyCost: number | null
  dailyImpact: number | null
  detectedAt: string
}

export interface ProfitSummary {
  range: ProfitDayRange
  previousRange: ProfitDayRange
  unitPrice: number
  sync: ProfitSyncStatus
  synced: boolean
  complete: boolean
  totals: ProfitTotals
  previous: ProfitTotals | null
  daily: ProfitDailyPoint[]
  upstreams: ProfitUpstreamRow[]
  groups: ProfitGroupRow[]
  unattributed: ProfitUnattributedRow[]
  insights: ProfitInsight[]
  priceChanges: ProfitPriceChange[]
}

export type ProfitRangeQuery = { days: number } | { from: string, to: string }
