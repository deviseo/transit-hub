import type { DashboardAdminLoginForm, DashboardAdminStatus } from '../types/dashboardAdmin'
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

/** 五项核心指标的实时数据。 */
export interface DashboardMetricsResponse {
  todayProfit: number
  siteBalance: number
  todayPurchase: number
  netProfit: number
  upstreamBalance: number
  groupCount: number
}

/** 历史趋势单日数据点。 */
export interface DashboardTrendPoint {
  date: string
  todayProfit: number
  siteBalance: number
  todayPurchase: number
  netProfit: number
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

/** 单个分组的今日使用额度。 */
export interface GroupUsageTodayItem {
  groupName: string
  todayAmount: number
}

/** 分组今日用量明细响应。 */
export interface GroupUsageTodayResponse {
  date: string
  total: number
  groups: GroupUsageTodayItem[]
}

/** 获取当前工作区「我的站点」所有分组今日的使用额度明细。仅在弹窗打开时按需调用。 */
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
