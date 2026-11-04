<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { BarChart, LineChart } from 'echarts/charts'
import {
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
} from 'echarts/components'
import { init, use, type ECharts, type EChartsCoreOption } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { axisTooltipPosition, type TooltipSize } from '../../utils/chartTooltip'

use([
  BarChart,
  LineChart,
  DataZoomComponent,
  GridComponent,
  LegendComponent,
  TooltipComponent,
  CanvasRenderer,
])

const props = defineProps<{
  option: EChartsCoreOption
  accessibleLabel: string
}>()

const chartElement = ref<HTMLDivElement | null>(null)
let chart: ECharts | null = null
let resizeObserver: ResizeObserver | null = null

// 坐标轴 tooltip 放在悬停这一列的旁边（见 axisTooltipPosition），不压住这一列的柱子和折线点。
// 列中心按 X 轴（类目轴）换算成像素；列宽取相邻一列的距离。
const axisPosition = (point: number[], params: unknown, _dom: unknown, _rect: unknown, size: TooltipSize) => {
  const first = (Array.isArray(params) ? params[0] : params) as { dataIndex?: unknown } | undefined
  const index = first?.dataIndex
  let center: number | null = null
  let band: number | null = null
  if (chart && typeof index === 'number') {
    const instance = chart
    const pixelOf = (value: number): number => Number(instance.convertToPixel({ xAxisIndex: 0 }, value))
    const pixel = pixelOf(index)
    if (Number.isFinite(pixel)) {
      center = pixel
      const neighbor = pixelOf(index > 0 ? index - 1 : index + 1)
      if (Number.isFinite(neighbor) && neighbor !== pixel) band = Math.abs(neighbor - pixel)
    }
  }
  return axisTooltipPosition(point, size, center, band)
}

// 图表自己没有指定 position 的坐标轴 tooltip 才使用上面的默认位置。
const withTooltipPlacement = (option: EChartsCoreOption): EChartsCoreOption => {
  const tooltip = option.tooltip
  if (!tooltip || typeof tooltip !== 'object' || Array.isArray(tooltip)) return option
  const config = tooltip as Record<string, unknown>
  if (config.trigger !== 'axis' || config.position !== undefined) return option
  return { ...option, tooltip: { ...config, position: axisPosition } }
}

const render = () => {
  if (!chart) return
  chart.setOption(withTooltipPlacement(props.option), { notMerge: true, lazyUpdate: true })
}

onMounted(() => {
  if (!chartElement.value) return
  chart = init(chartElement.value, undefined, { renderer: 'canvas' })
  render()
  resizeObserver = new ResizeObserver(() => chart?.resize())
  resizeObserver.observe(chartElement.value)
})

watch(() => props.option, render, { deep: true })

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  chart?.dispose()
  chart = null
})
</script>

<template>
  <div
    ref="chartElement"
    class="h-full min-h-0 w-full"
    role="img"
    :aria-label="accessibleLabel"
  />
</template>
