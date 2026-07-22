import type {
  DashboardMetricsResponse,
  DashboardTrendsResponse,
  GroupUsageTodayResponse,
  UpstreamBalanceBreakdownResponse,
} from '../api/dashboardAdmin'
import type { ConnectionHealthStoredSummary } from '../types/connectionHealth'
import type { DashboardAdminStatus } from '../types/dashboardAdmin'

export interface DashboardDataSnapshot {
  adminStatus: DashboardAdminStatus
  live: DashboardMetricsResponse
  trends: DashboardTrendsResponse
  updatedAt: number
  groupUsage?: GroupUsageTodayResponse
  balanceBreakdown?: UpstreamBalanceBreakdownResponse
  healthSummary?: ConnectionHealthStoredSummary
}

// 仪表盘数据包含营收和余额，只保存在当前 JavaScript 进程内，不写入浏览器持久化存储。
// Vue 路由切换不会清空模块，因此返回仪表盘时可以立即恢复；页面关闭或刷新后自然失效。
const snapshots = new Map<string, DashboardDataSnapshot>()

export const getDashboardDataSnapshot = (workspaceID: string): DashboardDataSnapshot | null => {
  if (!workspaceID) return null
  return snapshots.get(workspaceID) ?? null
}

export const saveDashboardDataSnapshot = (workspaceID: string, snapshot: DashboardDataSnapshot) => {
  if (!workspaceID) return
  snapshots.set(workspaceID, snapshot)
}

export const updateDashboardOperationalSnapshot = (
  workspaceID: string,
  update: Partial<Pick<DashboardDataSnapshot, 'groupUsage' | 'balanceBreakdown' | 'healthSummary'>>,
) => {
  const current = getDashboardDataSnapshot(workspaceID)
  if (!current) return
  snapshots.set(workspaceID, { ...current, ...update })
}
