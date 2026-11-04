<script setup lang="ts">
import { computed } from 'vue'
import type { EChartsCoreOption } from 'echarts/core'
import DashboardEChart from '../dashboard/DashboardEChart.vue'
import { useDashboardChartTheme } from '../../composables/useDashboardChartTheme'
import { columnHighlightPointer } from '../../utils/chartTooltip'

// 每日入账（按渠道堆叠柱）+ 用户消耗（折线）。两者都是人民币金额，共用一个 Y 轴（不用双轴）。
// 渠道颜色按实体固定分配（不随排名或筛选变化），取自经过 CVD 校验的分类色板（亮 / 暗两套）；
// 亮色下浅色槽对比度不足 3:1，因此必须同时提供图例与表格视图（由父组件提供切换）。
// 值为 null 的日期表示本地没有数据：柱子缺失、折线断开，提示框显示「无数据」，不当作 0。
const props = defineProps<{
  days: string[]
  channels: string[]
  series: Record<string, Array<number | null>>
  consumption: Array<number | null>
  channelLabel: (channel: string) => string
  consumptionLabel: string
  totalLabel: string
  noDataLabel: string
  accessibleLabel: string
  formatValue: (value: number) => string
  formatAxis: (value: number) => string
  formatDay: (day: string) => string
  formatTick: (day: string) => string
}>()

const { theme } = useDashboardChartTheme()

const CHANNEL_SLOTS: Record<string, number> = { payment: 0, credit: 0, redeem: 1, admin_add: 2, affiliate: 3 }
const PALETTE_LIGHT = ['#2a78d6', '#eb6834', '#1baf7a', '#eda100']
const PALETTE_DARK = ['#3987e5', '#d95926', '#199e70', '#c98500']

const isDark = computed(() => {
  void theme.value
  return typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
})

const channelColor = (channel: string): string => {
  const palette = isDark.value ? PALETTE_DARK : PALETTE_LIGHT
  return palette[CHANNEL_SLOTS[channel] ?? 3]
}

const legend = computed(() => [
  ...props.channels.map((channel) => ({ key: channel, label: props.channelLabel(channel), color: channelColor(channel), kind: 'bar' as const })),
  { key: '__consumption', label: props.consumptionLabel, color: theme.value.foreground, kind: 'line' as const },
])

const escapeHtml = (value: string): string => value.replace(/[&<>"']/g, (character) => (
  { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[character] ?? character
))

const option = computed<EChartsCoreOption>(() => {
  const colors = theme.value
  const swatch = (color: string, line = false) => line
    ? `<span style="display:inline-block;width:10px;height:2px;margin-right:6px;vertical-align:middle;background:${color}"></span>`
    : `<span style="display:inline-block;width:8px;height:8px;margin-right:6px;border-radius:2px;vertical-align:middle;background:${color}"></span>`
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
      axisPointer: columnHighlightPointer(colors.border),
      formatter: (params: unknown) => {
        const first = (Array.isArray(params) ? params[0] : params) as { dataIndex?: number } | undefined
        const index = first?.dataIndex ?? 0
        const rows: string[] = []
        let total: number | null = null
        for (const channel of props.channels) {
          const value = props.series[channel]?.[index]
          if (value == null) continue
          total = (total ?? 0) + value
          if (value === 0) continue
          rows.push(`<div style="display:flex;align-items:center;justify-content:space-between;gap:16px"><span style="display:inline-flex;align-items:center;flex:1">${swatch(channelColor(channel))}${escapeHtml(props.channelLabel(channel))}</span><span style="font-weight:600">${escapeHtml(props.formatValue(value))}</span></div>`)
        }
        const consumption = props.consumption[index]
        const totalText = escapeHtml(total == null ? props.noDataLabel : props.formatValue(total))
        const consumptionText = escapeHtml(consumption == null ? props.noDataLabel : props.formatValue(consumption))
        return `<div style="font-size:12px;color:${colors.muted}">${escapeHtml(props.formatDay(props.days[index] ?? ''))}</div>`
          + `<div style="margin-top:4px;display:flex;justify-content:space-between;gap:16px;font-weight:600"><span>${escapeHtml(props.totalLabel)}</span><span>${totalText}</span></div>`
          + rows.join('')
          + `<div style="margin-top:4px;display:flex;align-items:center;justify-content:space-between;gap:16px"><span style="display:inline-flex;align-items:center;flex:1">${swatch(colors.foreground, true)}${escapeHtml(props.consumptionLabel)}</span><span style="font-weight:600">${consumptionText}</span></div>`
      },
    },
    xAxis: {
      type: 'category',
      data: props.days.map((day) => props.formatTick(day)),
      axisLine: { lineStyle: { color: colors.border } },
      axisTick: { show: false },
      axisLabel: { color: colors.muted, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      min: 0,
      axisLabel: { color: colors.muted, formatter: (value: number) => props.formatAxis(value) },
      splitLine: { lineStyle: { color: colors.border, type: 'solid', width: 1 } },
    },
    series: [
      ...props.channels.map((channel) => ({
        name: props.channelLabel(channel),
        type: 'bar',
        stack: 'credits',
        barMaxWidth: 18,
        data: props.series[channel] ?? [],
        itemStyle: { color: channelColor(channel), borderColor: colors.card, borderWidth: 1 },
        emphasis: { disabled: true },
      })),
      {
        name: props.consumptionLabel,
        type: 'line',
        data: props.consumption,
        showSymbol: false,
        symbol: 'circle',
        symbolSize: 8,
        z: 3,
        lineStyle: { width: 2, color: colors.foreground, cap: 'round', join: 'round' },
        itemStyle: { color: colors.foreground, borderColor: colors.card, borderWidth: 2 },
        emphasis: { scale: false },
      },
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
