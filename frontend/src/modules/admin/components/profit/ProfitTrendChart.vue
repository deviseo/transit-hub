<script setup lang="ts">
import { computed } from 'vue'
import type { EChartsCoreOption } from 'echarts/core'
import DashboardEChart from '../dashboard/DashboardEChart.vue'
import { useDashboardChartTheme } from '../../composables/useDashboardChartTheme'
import { columnHighlightPointer } from '../../utils/chartTooltip'

// 每日营收、成本（折线）与毛利（柱，可为负）。三者都是人民币，共用一个 Y 轴（不用双轴）。
// 颜色按指标固定分配，取自经过 CVD 校验的分类色板（亮 / 暗两套）；亮色下部分颜色对比度不足 3:1，
// 因此同时提供图例与表格视图（由父组件提供切换）。值为 null 的日期表示还没有采集，柱子缺失、折线断开。
const props = defineProps<{
  days: string[]
  revenue: Array<number | null>
  cost: Array<number | null>
  profit: Array<number | null>
  revenueLabel: string
  costLabel: string
  profitLabel: string
  marginLabel: string
  noDataLabel: string
  accessibleLabel: string
  formatValue: (value: number) => string
  formatAxis: (value: number) => string
  formatPercent: (ratio: number) => string
  formatDay: (day: string) => string
  formatTick: (day: string) => string
}>()

const { theme } = useDashboardChartTheme()

const PALETTE_LIGHT = { revenue: '#2a78d6', cost: '#eb6834', profit: '#1baf7a' }
const PALETTE_DARK = { revenue: '#3987e5', cost: '#d95926', profit: '#199e70' }

const isDark = computed(() => {
  void theme.value
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
})

const colors = computed(() => (isDark.value ? PALETTE_DARK : PALETTE_LIGHT))

const legend = computed(() => [
  { key: 'revenue', label: props.revenueLabel, color: colors.value.revenue, kind: 'line' as const },
  { key: 'cost', label: props.costLabel, color: colors.value.cost, kind: 'line' as const },
  { key: 'profit', label: props.profitLabel, color: colors.value.profit, kind: 'bar' as const },
])

const escapeHtml = (value: string): string => value.replace(/[&<>"']/g, (character) => (
  { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[character] ?? character
))

const option = computed<EChartsCoreOption>(() => {
  const surface = theme.value
  const palette = colors.value
  const swatch = (color: string, line: boolean) => line
    ? `<span style="display:inline-block;width:10px;height:2px;margin-right:6px;vertical-align:middle;background:${color}"></span>`
    : `<span style="display:inline-block;width:8px;height:8px;margin-right:6px;border-radius:2px;vertical-align:middle;background:${color}"></span>`
  const row = (color: string, line: boolean, label: string, value: string) => (
    `<div style="display:flex;align-items:center;justify-content:space-between;gap:16px"><span style="display:inline-flex;align-items:center;flex:1">${swatch(color, line)}${escapeHtml(label)}</span><span style="font-weight:600">${escapeHtml(value)}</span></div>`
  )
  const lineSeries = (name: string, data: Array<number | null>, color: string) => ({
    name,
    type: 'line',
    data,
    showSymbol: false,
    symbol: 'circle',
    symbolSize: 8,
    z: 3,
    lineStyle: { width: 2, color, cap: 'round', join: 'round' },
    itemStyle: { color, borderColor: surface.card, borderWidth: 2 },
    emphasis: { scale: false },
  })
  return {
    animationDuration: 300,
    textStyle: { color: surface.muted, fontFamily: 'inherit' },
    grid: { top: 12, right: 12, bottom: 4, left: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: surface.card,
      borderColor: surface.border,
      textStyle: { color: surface.foreground },
      axisPointer: columnHighlightPointer(surface.border),
      formatter: (params: unknown) => {
        const first = (Array.isArray(params) ? params[0] : params) as { dataIndex?: number } | undefined
        const index = first?.dataIndex ?? 0
        const header = `<div style="font-size:12px;color:${surface.muted}">${escapeHtml(props.formatDay(props.days[index] ?? ''))}</div>`
        const revenue = props.revenue[index]
        const cost = props.cost[index]
        const profit = props.profit[index]
        if (revenue == null || cost == null || profit == null) {
          return `${header}<div style="margin-top:4px">${escapeHtml(props.noDataLabel)}</div>`
        }
        const margin = revenue > 0 ? props.formatPercent(profit / revenue) : '—'
        return header
          + `<div style="margin-top:4px">${row(palette.revenue, true, props.revenueLabel, props.formatValue(revenue))}</div>`
          + row(palette.cost, true, props.costLabel, props.formatValue(cost))
          + row(palette.profit, false, props.profitLabel, props.formatValue(profit))
          + `<div style="display:flex;justify-content:space-between;gap:16px;color:${surface.muted}"><span>${escapeHtml(props.marginLabel)}</span><span>${escapeHtml(margin)}</span></div>`
      },
    },
    xAxis: {
      type: 'category',
      data: props.days.map((day) => props.formatTick(day)),
      axisLine: { lineStyle: { color: surface.border } },
      axisTick: { show: false },
      axisLabel: { color: surface.muted, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: surface.muted, formatter: (value: number) => props.formatAxis(value) },
      splitLine: { lineStyle: { color: surface.border, type: 'solid', width: 1 } },
    },
    series: [
      {
        name: props.profitLabel,
        type: 'bar',
        barMaxWidth: 14,
        // 圆角只在远离零线的一端：正值在上、负值在下。
        data: props.profit.map((value) => (value == null
          ? null
          : { value, itemStyle: { borderRadius: value >= 0 ? [3, 3, 0, 0] : [0, 0, 3, 3] } })),
        itemStyle: { color: palette.profit },
        emphasis: { disabled: true },
      },
      lineSeries(props.revenueLabel, props.revenue, palette.revenue),
      lineSeries(props.costLabel, props.cost, palette.cost),
    ],
  }
})
</script>

<template>
  <div class="flex h-full flex-col">
    <ul class="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground" :aria-label="accessibleLabel">
      <li v-for="item in legend" :key="item.key" class="inline-flex items-center gap-1.5">
        <span v-if="item.kind === 'bar'" class="h-2.5 w-2.5 rounded-sm" :style="{ backgroundColor: item.color }" aria-hidden="true" />
        <span v-else class="h-0.5 w-3 rounded-full" :style="{ backgroundColor: item.color }" aria-hidden="true" />
        <span>{{ item.label }}</span>
      </li>
    </ul>
    <div class="mt-2 min-h-0 flex-1">
      <DashboardEChart :option="option" :accessible-label="accessibleLabel" />
    </div>
  </div>
</template>
