export type UserActivitySyncStatus = '' | 'running' | 'success' | 'failed'

export type UserActivitySegment =
  | 'all'
  | 'active_1d'
  | 'active_7d'
  | 'active_30d'
  | 'inactive_2d'
  | 'inactive_7d'
  | 'inactive_14d'
  | 'inactive_30d'
  | 'inactive_60d'
  | 'inactive_90d'
  | 'no_usage'
  | 'churned_7d'
  | 'declining'
  | 'growing'
  | 'inactive'
  | 'recharged_today'
  | 'low_balance'

export type UserActivityStatusFilter = '' | 'active' | 'disabled'

export type UserActivityRoleFilter = '' | 'user' | 'admin'

export type UserActivitySortKey =
  | 'lastUsedAt'
  | 'costToday'
  | 'cost7d'
  | 'cost30d'
  | 'requests7d'
  | 'change7d'
  | 'activeDays30d'
  | 'balance'
  | 'totalRecharged'
  | 'totalUsed'
  | 'createdAt'
  | 'rechargeToday'
  | 'recharge30d'

export type UserActivityPeriodKey = 'day' | 'week' | 'month'

export interface UserActivitySync {
  platform: string
  status: UserActivitySyncStatus
  errorKey?: string
  syncedFrom?: string
  syncedTo?: string
  userCount: number
  lastStartedAt?: string
  lastSyncedAt?: string
  historyLimitDays: number
  warningKey?: string
}

export interface UserActivityPeriodMetrics {
  from: string
  to: string
  activeUsers: number
  newUsers: number
  requests: number
  tokens: number
  cost: number
}

export interface UserActivityRetention {
  retained: number
  churned: number
  gained: number
}

export interface UserActivityPeriodComparison {
  days: number
  current: UserActivityPeriodMetrics
  previous: UserActivityPeriodMetrics | null
  lastYear: UserActivityPeriodMetrics | null
  retention?: UserActivityRetention
}

export interface UserActivityInactiveBucket {
  days: number
  count: number
}

export interface UserActivityDailyPoint {
  day: string
  activeUsers: number
  requests: number
  tokens: number
  cost: number
  // 入账（美元）；null 表示本地入账数据没有覆盖这一天
  recharge: number | null
}

export type UserActivityRechargeChannel = 'payment' | 'redeem' | 'admin_add' | 'admin_deduct' | 'affiliate' | 'credit' | 'debit'

export interface UserActivityChannelAmount {
  channel: UserActivityRechargeChannel | string
  amount: number
  events: number
  users: number
}

export interface UserActivityMoney {
  // records：sub2api 按充值记录统计；snapshot：new-api 按余额快照差值统计
  method: 'records' | 'snapshot' | ''
  rechargeSince?: string
  rechargeFrom?: string
  rechargeUpToDate: boolean
  todayRecharge: number
  todayDeduction: number
  todayRechargeUsers: number
  todayChannels: UserActivityChannelAmount[]
  yesterdayRecharge: number | null
  todayCost: number
  yesterdayCost: number | null
  totalBalance: number
  usersWithBalance: number
  balanceYesterday: number | null
  lifetimeRecharged: number | null
  lowBalanceActive: number
}

export interface UserActivitySummary {
  sync: UserActivitySync
  hasData: boolean
  totalUsers: number
  disabled: number
  withEmail: number
  noUsage: number
  today: UserActivityPeriodMetrics
  money: UserActivityMoney
  inactive: UserActivityInactiveBucket[]
  periods: Partial<Record<UserActivityPeriodKey, UserActivityPeriodComparison>>
  daily: UserActivityDailyPoint[]
  timezone: string
  suppressions: number
}

export interface UserActivityFilter {
  segment: UserActivitySegment
  inactiveDays?: number
  search?: string
  status?: UserActivityStatusFilter
  role?: UserActivityRoleFilter
}

export interface UserActivityListQuery extends UserActivityFilter {
  sort: UserActivitySortKey
  order: 'asc' | 'desc'
  page: number
  pageSize: number
}

export interface UserActivityRow {
  siteUserId: string
  email: string
  username: string
  role: string
  status: string
  group: string
  balance: number | null
  totalRecharged: number | null
  totalUsed: number | null
  everUsed: boolean
  createdAt: string | null
  lastUsedAt: string | null
  daysSinceLastUse: number | null
  requestsToday: number
  costToday: number
  requests7d: number
  requestsPrev7d: number
  cost7d: number
  costPrev7d: number
  cost30d: number
  costPrev30d: number
  activeDays30d: number
  rechargeToday: number
  recharge30d: number
}

export interface UserActivityListResponse {
  items: UserActivityRow[]
  total: number
  page: number
  pageSize: number
  pages: number
}

export interface UserActivityDetail {
  user: UserActivityRow
  daily: UserActivityDailyPoint[]
  periods: Partial<Record<UserActivityPeriodKey, UserActivityPeriodComparison>>
}

export interface UserActivityRecipientPreview {
  matched: number
  withEmail: number
  suppressed: number
  sendable: number
}
