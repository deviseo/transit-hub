import type { DashboardAdminLoginForm, DashboardAdminStatus } from '../types/dashboardAdmin'
import type { DashboardMetricKey } from '../types/dashboard'
import {
  authUnauthorizedErrorKey,
  getAccessToken,
  handleAuthExpired,
  isUnauthorizedApiResponse,
} from '@/modules/auth/api/auth'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'

const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

const authHeaders = (): HeadersInit => {
  const token = getAccessToken()
  if (!token) return {}
  return { Authorization: `Bearer ${token}` }
}

type AdminErrorPayload = {
  message?: string
}

// 与 mySites.ts 保持一致的请求封装：附带 TransitHub 鉴权头，并把后端返回的
// i18n 错误 key 透传为 Error.message，由调用方用 t() 渲染。
const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...authHeaders(),
        ...(options.headers ?? {}),
      },
    })
  } catch (error) {
    throw new Error('admin.dashboard.adminAuth.errors.network')
  }

  const text = await response.text()
  const payload = text ? (JSON.parse(text) as T & AdminErrorPayload) : ({} as T & AdminErrorPayload)

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.dashboard.adminAuth.errors.request')
  }

  return payload
}

/** 查询当前用户的仪表盘 admin 登录状态。 */
export const getDashboardAdminStatus = async (): Promise<DashboardAdminStatus> =>
  requestJson<DashboardAdminStatus>('/dashboard/admin/status')

/** 提交 admin 登录（三种 sub2api 方式之一）。 */
export const loginDashboardAdmin = async (form: DashboardAdminLoginForm): Promise<DashboardAdminStatus> =>
  requestJson<DashboardAdminStatus>('/dashboard/admin/login', {
    method: 'POST',
    body: JSON.stringify(form),
  })

/** 退出当前 admin 账户（仅清除仪表盘 admin 会话）。保留供旧调用方兼容，新代码不再使用。 */
export const logoutDashboardAdmin = async (): Promise<DashboardAdminStatus> =>
  requestJson<DashboardAdminStatus>('/dashboard/admin/logout', {
    method: 'POST',
  })

/** 主动刷新当前 admin session 并重新校验 admin 身份。 */
export const refreshDashboardAdminSession = async (): Promise<DashboardAdminStatus> =>
  requestJson<DashboardAdminStatus>('/dashboard/admin/refresh', {
    method: 'POST',
  })

// ─── 仪表盘指标数据 ────────────────────────────────────────

/** 五项核心指标的实时数据。
 * todayProfit/todayPurchase/netProfit 为 null 表示该指标不可用（非零）。 */
export interface DashboardMetricsResponse {
  date: string
  timezone: string
  todayProfit: number | null
  siteBalance: number
  todayPurchase: number | null
  netProfit: number | null
  upstreamBalance: number
  groupCount: number
  metricErrors?: Partial<Record<DashboardMetricKey, string>>
  costQuality?: CostQuality
}

/** 成本质量信息，描述缓存成本的完整性。 */
export interface CostQuality {
  businessDate: string
  confirmedCost: number   // 已确认站点成本之和（真实下限）
  complete: boolean       // 所有目标站点均成功采集时为 true
  expectedSites: number
  collectedSites: number
  failedSites: number
  observedAt?: string
  failures?: SiteCostFault[]
}

/** 单个站点成本采集失败记录。 */
export interface SiteCostFault {
  siteName: string
  reason: 'date_mismatch' | 'stale' | 'fetch_error'
}

/** 历史趋势单日数据点。nullable 字段为 null 表示该天数据未采集，前端显示断点。 */
export interface DashboardTrendPoint {
  date: string
  todayProfit: number | null
  siteBalance: number
  todayPurchase: number | null
  netProfit: number | null
  upstreamBalance: number
}

/** 历史趋势响应。 */
export interface DashboardTrendsResponse {
  points: DashboardTrendPoint[]
}

/** 站点用户余额筛选配置。 */
export interface BalanceFilterConfig {
  excludeAdmin: boolean
  excludeBalances: number[]
}

/** 获取当前用户的余额筛选配置。 */
export const getBalanceFilter = async (): Promise<BalanceFilterConfig> =>
  requestJson<BalanceFilterConfig>('/dashboard/balance-filter')

/** 保存余额筛选配置。 */
export const saveBalanceFilter = async (config: BalanceFilterConfig): Promise<BalanceFilterConfig> =>
  requestJson<BalanceFilterConfig>('/dashboard/balance-filter', {
    method: 'PUT',
    body: JSON.stringify(config),
  })

/** 管理员站点分组信息。 */
export interface AdminGroupItem {
  id: string
  name: string
  platform: string
  multiplier: string
}

/** 管理员站点分组列表响应。 */
export interface AdminGroupsResponse {
  count: number
  groups: AdminGroupItem[]
}

/** 获取管理员站点的分组列表。 */
export const getAdminGroups = async (): Promise<AdminGroupsResponse> =>
  requestJson<AdminGroupsResponse>('/dashboard/groups')

/** 获取五项核心指标的实时数据。 */
export const getDashboardMetrics = async (): Promise<DashboardMetricsResponse> =>
  requestJson<DashboardMetricsResponse>('/dashboard/metrics')

/** 获取历史趋势数据，days 支持 7（周）或 30（月）。 */
export const getDashboardTrends = async (days: number): Promise<DashboardTrendsResponse> =>
  requestJson<DashboardTrendsResponse>(`/dashboard/trends?days=${days}`)

/** 单个分组的今日营收、成本与利润。todayAmount 保留为营收兼容字段。 */
export interface GroupUsageTodayItem {
  groupName: string
  todayAmount: number
  todayRevenue: number
  todayCost?: number | null
  todayProfit?: number | null
}

/** 分组今日营收明细，以及在成本口径完整时实时派生的利润明细。 */
export interface GroupUsageTodayResponse {
  date: string
  total: number
  totalRevenue: number
  totalCost?: number | null
  totalProfit?: number | null
  profitAvailable: boolean
  profitUnavailableReason?: string
  groups: GroupUsageTodayItem[]
}

/** 获取当前工作区「我的站点」所有分组今日营收及实时利润。首页运营区和弹窗按需调用。 */
export const getGroupUsageToday = async (): Promise<GroupUsageTodayResponse> =>
  requestJson<GroupUsageTodayResponse>('/dashboard/group-usage-today')

/** 单个 key 的今日消费明细（「今日成本」下钻）。TodayAmount 已乘以站点 rechargeRate，RawAmount 为上游平台原始金额。 */
export interface UpstreamKeyUsageTodayItem {
  siteId: string
  siteName: string
  platform: string
  keyId: string
  keyName: string
  groupName: string
  todayAmount: number
  rawAmount: number
  rechargeRate: number
}

/** 「今日成本」下钻响应：当前工作区所有上游站点中，今天有消费的 key 列表。 */
export interface UpstreamKeyUsageTodayResponse {
  date: string
  total: number
  keys: UpstreamKeyUsageTodayItem[]
  failedSites?: number
  totalSites?: number
}

/** 获取当前工作区所有上游站点中，今天有消费的 key 明细。仅在弹窗打开时按需调用。 */
export const getUpstreamKeyUsageToday = async (): Promise<UpstreamKeyUsageTodayResponse> =>
  requestJson<UpstreamKeyUsageTodayResponse>('/dashboard/upstream-key-usage-today')

/** 单个上游站点的余额明细（「上游总余额」下钻）。balance/rawBalance 为 null 表示该站点余额未知。 */
export interface UpstreamBalanceBreakdownItem {
  siteId: string
  siteName: string
  platform: string
  balance: number | null
  rawBalance: number | null
  rechargeRate: number
  lastSyncedAt: number | null
  status: string
}

/** 「上游总余额」下钻响应：当前工作区所有上游站点的缓存余额列表。 */
export interface UpstreamBalanceBreakdownResponse {
  total: number
  sites: UpstreamBalanceBreakdownItem[]
}

/** 获取当前工作区所有上游站点的缓存余额明细（不触发外部平台请求）。仅在弹窗打开时按需调用。 */
export const getUpstreamBalanceBreakdown = async (): Promise<UpstreamBalanceBreakdownResponse> =>
  requestJson<UpstreamBalanceBreakdownResponse>('/dashboard/upstream-balance-breakdown')

// ─── 每日明细 API ────────────────────────────────────────

/** 单个站点成本明细（expand=true 时返回）。 */
export interface SiteCostDetail {
  siteId: string
  siteName: string
  platform: string
  rawCost?: number | null
  rechargeRate: number
  adjustedCost?: number | null
  status: string
  source: string
  errorReason?: string
  observedAt: string
}

/** 单日结算数据。settlementStatus 为 "missing" 表示该日期无记录。 */
export interface DailyStatItem {
  date: string
  settlementStatus: 'missing' | 'provisional' | 'partial' | 'final'
  snapshotSource?: string
  todayProfit?: number | null
  confirmedCost?: number | null
  netProfitCeiling?: number | null
  marginCeiling?: number | null
  costExpectedCount?: number | null
  costCollectedCount?: number | null
  finalizedAt?: string | null
  siteCosts?: SiteCostDetail[]
  siteCostsLoadError?: boolean  // expand=true 但站点明细查询失败时为 true
}

/** 每日明细响应。 */
export interface DailyStatsResponse {
  items: DailyStatItem[]
}

/** 获取指定日期范围内每天的结算状态。缺失日期返回 missing 占位。 */
export const getDailyStats = async (
  from: string,
  to: string,
  options?: { page?: number; pageSize?: number; expand?: boolean },
): Promise<DailyStatsResponse> => {
  const params = new URLSearchParams({ from, to })
  if (options?.page) params.set('page', String(options.page))
  if (options?.pageSize) params.set('pageSize', String(options.pageSize))
  if (options?.expand) params.set('expand', 'true')
  return requestJson<DailyStatsResponse>(`/dashboard/daily-stats?${params.toString()}`)
}

// ─── 受控回填 API ────────────────────────────────────────

/** 回填请求。dryRun 默认 true（安全优先）。 */
export interface BackfillRequest {
  from: string
  to: string
  dryRun?: boolean
  force?: boolean
}

/** 单天回填结果。 */
export interface BackfillDayResult {
  date: string
  status: 'updated' | 'skipped' | 'failed'
  reason?: string
  oldProfit?: number | null
  newProfit?: number | null
  oldCost?: number | null
  newCost?: number | null
}

/** 回填响应。 */
export interface BackfillResponse {
  dryRun: boolean
  results: BackfillDayResult[]
}

/** 执行受控回填。dryRun=true（默认）时只返回预览，不写库。 */
export const postBackfill = async (req: BackfillRequest): Promise<BackfillResponse> =>
  requestJson<BackfillResponse>('/dashboard/backfill', {
    method: 'POST',
    body: JSON.stringify({ dryRun: true, force: false, ...req }),
  })
