<script setup lang="ts">
import { computed } from 'vue'
import type { EChartsCoreOption } from 'echarts/core'
import DashboardEChart from '../dashboard/DashboardEChart.vue'
import { useDashboardChartTheme } from '../../composables/useDashboardChartTheme'

// 单序列趋势折线：一个图只画一个指标（不同量纲分成两张图，不用双 Y 轴）；
// 2px 线 + 10% 面积、实线细网格、十字准线 tooltip（数值在前、日期在后）。
// values 中的 null 表示这一天没有数据：线条在此断开，tooltip 显示「无数据」，不当作 0。
const props = withDefaults(defineProps<{
  days: string[]
  values: Array<number | null>
  accessibleLabel: string
  formatValue: (value: number) => string
  formatAxis: (value: number) => string
  formatDay: (day: string) => string
  formatTick: (day: string) => string
  integer?: boolean
  noDataLabel?: string
}>(), {
  integer: false,
  noDataLabel: '—',
})

const { theme } = useDashboardChartTheme()

const escapeHtml = (value: string): string => value.replace(/[&<>"']/g, (character) => (
  { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[character] ?? character
))

const option = computed<EChartsCoreOption>(() => {
  const colors = theme.value
  return {
    animationDuration: 300,
    textStyle: { color: colors.muted, fontFamily: 'inherit' },
    grid: { top: 12, right: 12, bottom: 4, left: 4, containLabel: true },
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: colors.card,
      borderColor: colors.border,
      textStyle: { color: colors.foreground },
      axisPointer: { type: 'line', lineStyle: { color: colors.muted, width: 1, type: 'solid' } },
      formatter: (params: unknown) => {
        const point = (Array.isArray(params) ? params[0] : params) as { dataIndex?: number } | undefined
        const index = point?.dataIndex ?? 0
        const raw = props.values[index]
        const value = escapeHtml(raw == null ? props.noDataLabel : props.formatValue(raw))
        const day = escapeHtml(props.formatDay(props.days[index] ?? ''))
        return `<div style="font-weight:600;color:${colors.foreground}">${value}</div>`
          + `<div style="margin-top:2px;font-size:12px;color:${colors.muted}">${day}</div>`
      },
    },
    xAxis: {
      type: 'category',
      boundaryGap: false,
      data: props.days.map((day) => props.formatTick(day)),
      axisLine: { lineStyle: { color: colors.border } },
      axisTick: { show: false },
      axisLabel: { color: colors.muted, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      min: 0,
      minInterval: props.integer ? 1 : undefined,
      axisLabel: { color: colors.muted, formatter: (value: number) => props.formatAxis(value) },
      splitLine: { lineStyle: { color: colors.border, type: 'solid', width: 1 } },
    },
    series: [{
      type: 'line',
      data: props.values,
      showSymbol: false,
      symbol: 'circle',
      symbolSize: 8,
      lineStyle: { width: 2, color: colors.primary, cap: 'round', join: 'round' },
      itemStyle: { color: colors.primary, borderColor: colors.card, borderWidth: 2 },
      areaStyle: { color: colors.primary, opacity: 0.1 },
      emphasis: { scale: false },
    }],
  }
})
</script>

<template>
  <DashboardEChart :option="option" :accessible-label="accessibleLabel" />
</template>
