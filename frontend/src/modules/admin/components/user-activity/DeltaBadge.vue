<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowDownRight, ArrowUpRight } from 'lucide-vue-next'
import { computeDelta, useUserActivityFormat } from '../../composables/useUserActivityFormat'

// 环比 / 同比变化：箭头 + 正负号 + 百分比，颜色只是辅助（方向不只靠颜色表达）。
const props = withDefaults(defineProps<{
  current: number
  previous: number | null | undefined
  // 指标上升是否是好事（活跃、消费上升为好；流失上升为坏）。
  upIsGood?: boolean
  // 上期缺失时的说明（悬停提示）。
  missingHint?: string
}>(), {
  upIsGood: true,
  missingHint: '',
})

const { t } = useI18n()
const { formatSignedPercent } = useUserActivityFormat()

const delta = computed(() => computeDelta(props.current, props.previous))
// 两期都没有用量：没有可比较的变化，显示占位符。
const bothZero = computed(() => props.current === 0 && props.previous === 0)

const direction = computed<'up' | 'down' | 'flat'>(() => {
  const value = delta.value
  if (!value) return 'flat'
  if (value.fromZero) return 'up'
  if (value.ratio == null || value.ratio === 0) return 'flat'
  return value.ratio > 0 ? 'up' : 'down'
})

const toneClass = computed(() => {
  if (!delta.value || bothZero.value || direction.value === 'flat') return 'text-muted-foreground'
  const good = (direction.value === 'up') === props.upIsGood
  return good ? 'text-emerald-700 dark:text-emerald-400' : 'text-red-600 dark:text-red-400'
})

const text = computed(() => {
  const value = delta.value
  if (!value || bothZero.value) return t('admin.userActivity.common.placeholder')
  if (value.fromZero) return t('admin.userActivity.delta.fromZero')
  return formatSignedPercent(value.ratio ?? 0)
})

const title = computed(() => (delta.value ? '' : props.missingHint))
</script>

<template>
  <span class="inline-flex items-center gap-0.5 whitespace-nowrap text-xs font-medium tabular-nums" :class="toneClass" :title="title || undefined">
    <ArrowUpRight v-if="delta && !bothZero && direction === 'up'" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
    <ArrowDownRight v-else-if="delta && !bothZero && direction === 'down'" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
    <span>{{ text }}</span>
  </span>
</template>
