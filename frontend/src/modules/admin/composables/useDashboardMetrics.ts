// 仪表盘指标数据来源。
//
// 从后端 /api/dashboard/metrics 获取实时指标，
// 从 /api/dashboard/trends 获取历史快照，两者组合后驱动统计卡片与趋势图。

import { ref } from 'vue'
import type {
  DashboardColorToken,
  DashboardMetricData,
  DashboardMetricKey,
  TrendPoint,
} from '../types/dashboard'
import {
  getDashboardMetrics,
  getDashboardTrends,
  type DashboardMetricsResponse,
  type DashboardTrendPoint,
  type DashboardTrendsResponse,
} from '../api/dashboardAdmin'

const METRIC_CONFIGS: { key: DashboardMetricKey; color: DashboardColorToken }[] = [
  { key: 'todayProfit', color: 'primary' },
  { key: 'siteBalance', color: 'accent' },
  { key: 'todayPurchase', color: 'warning' },
  { key: 'netProfit', color: 'signal' },
  { key: 'upstreamBalance', color: 'primary' },
]

function dateLabel(dateStr: string | undefined): string {
  if (!dateStr) return ''
  const [, month, day] = dateStr.split('-').map(Number)
  if (!month || !day) return dateStr
  return `${month}/${day}`
}

function buildMetricData(
  key: DashboardMetricKey,
  color: DashboardColorToken,
  live: DashboardMetricsResponse,
  trendPoints: DashboardTrendPoint[],
): DashboardMetricData {
  const current = live[key]
  const pointsByDate = new Map<string, DashboardTrendPoint>()
  for (const point of trendPoints) {
    if (point.date) pointsByDate.set(point.date, point)
  }
  if (live.date) {
    pointsByDate.set(live.date, {
      date: live.date,
      todayProfit: live.todayProfit,
      siteBalance: live.siteBalance,
      todayPurchase: live.todayPurchase,
      netProfit: live.netProfit,
      upstreamBalance: live.upstreamBalance,
    })
  }

  const monthPoints: TrendPoint[] = Array.from(pointsByDate.values())
    .sort((a, b) => a.date.localeCompare(b.date))
    .map((p) => ({
      label: dateLabel(p.date),
      value: p[key],
    }))

  const week = monthPoints.slice(-7)
  const month = monthPoints.slice(-30)

  return { key, color, current, error: live.metricErrors?.[key], series: { week, month } }
}

export function useDashboardMetrics() {
  const metrics = ref<DashboardMetricData[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  const fetchMetrics = async () => {
    loading.value = true
    error.value = null
    try {
      const [live, trends] = await Promise.all([
        getDashboardMetrics(),
        getDashboardTrends(30),
      ])

      metrics.value = METRIC_CONFIGS.map(({ key, color }) =>
        buildMetricData(key, color, live, trends.points),
      )
    } catch (err) {
      error.value = err instanceof Error ? err.message : 'admin.dashboard.loadError'
    } finally {
      loading.value = false
    }
  }

  const applyRawData = (live: DashboardMetricsResponse, trends: DashboardTrendsResponse) => {
    metrics.value = METRIC_CONFIGS.map(({ key, color }) =>
      buildMetricData(key, color, live, trends.points),
    )
  }

  return { metrics, loading, error, fetchMetrics, applyRawData }
}
