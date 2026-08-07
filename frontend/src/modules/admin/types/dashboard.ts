// 仪表盘统计相关类型定义。
// 这些指标语义与「我的站点」概览保持一致，后续会由后端统计接口提供真实数据。

/** 趋势图统计周期：周（最近 7 天）/ 月（最近 30 天）。 */
export type DashboardPeriod = 'week' | 'month'

/** 仪表盘的五个核心指标。 */
export type DashboardMetricKey =
  | 'todayProfit' // 今日营收
  | 'siteBalance' // 站点用户总余额
  | 'todayPurchase' // 今日成本
  | 'netProfit' // 今日净利润
  | 'upstreamBalance' // 上游总余额

/** 可用的主题色 token，对应 globals.css / tailwind.config 中的 CSS 变量。 */
export type DashboardColorToken = 'primary' | 'accent' | 'signal' | 'warning'

/** 单个趋势数据点。 */
export interface TrendPoint {
  /** X 轴标签，使用与语言无关的日期格式，如 "6/27"。 */
  label: string
  /** 指标数值；null 表示该天指标不可用，图表渲染时显示断点而非伪零。 */
  value: number | null
  date?: string  // 用于 computeDelta 日期相邻校验
  quality?: 'exact' | 'confirmed' | 'ceiling' | 'unavailable'
  status?: string
  expected?: number | null
  collected?: number | null
}

/** 同一指标在不同周期下的连续序列。 */
export interface MetricSeries {
  week: TrendPoint[]
  month: TrendPoint[]
}

/** 一个仪表盘指标的完整数据（当前值 + 连续趋势序列）。 */
export interface DashboardMetricData {
  key: DashboardMetricKey
  color: DashboardColorToken
  /** 当前值（”今天”的数值），nil 表示该指标不可用（非零）。 */
  current: number | null
  /** 局部拉取失败原因；存在时 current 为 null 或显式的 0 占位。 */
  error?: string
  series: MetricSeries
}
