<script setup lang="ts">
import { computed, type Component } from 'vue'
import { ArrowDownRight, ArrowUpRight, Minus } from 'lucide-vue-next'
import type { DashboardColorToken } from '../../types/dashboard'
import { DELTA_TEXT_CLASSES, METRIC_ICON_CLASSES, type DeltaDirection } from '../../utils/dashboard'

const props = defineProps<{
  label: string
  value: string
  icon: Component
  color: DashboardColorToken
  deltaDirection: DeltaDirection
  deltaText: string
  deltaCaption: string
  clickable?: boolean
  negativeWhenUp?: boolean
}>()

const emit = defineEmits<{
  (event: 'click'): void
}>()

const iconClass = computed(() => METRIC_ICON_CLASSES[props.color])
const deltaClass = computed(() => {
  if (!props.negativeWhenUp || props.deltaDirection === 'flat') {
    return DELTA_TEXT_CLASSES[props.deltaDirection]
  }
  return props.deltaDirection === 'up'
    ? DELTA_TEXT_CLASSES.down
    : DELTA_TEXT_CLASSES.up
})
const deltaIcon = computed(() => {
  if (props.deltaDirection === 'up') return ArrowUpRight
  if (props.deltaDirection === 'down') return ArrowDownRight
  return Minus
})
</script>

<template>
  <component
    :is="clickable ? 'button' : 'div'"
    :type="clickable ? 'button' : undefined"
    class="min-h-[132px] w-full rounded-lg border border-border/60 bg-card p-4 text-left shadow-sm sm:min-h-[142px] sm:p-5"
    :class="{ 'cursor-pointer transition-[border-color,box-shadow,transform] hover:border-primary/30 hover:shadow-md focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background active:translate-y-px': clickable }"
    @click="clickable && emit('click')"
  >
    <div class="flex items-start justify-between">
      <div class="min-w-0">
        <p class="text-xs font-medium leading-5 text-muted-foreground sm:text-sm">{{ label }}</p>
        <p class="mt-2 break-words text-lg font-bold leading-tight tabular-nums text-foreground sm:text-xl xl:text-2xl">{{ value }}</p>
      </div>
      <div :class="['shrink-0 rounded-lg p-2 sm:p-2.5', iconClass]">
        <component :is="icon" class="h-4 w-4 sm:h-5 sm:w-5" aria-hidden="true" />
      </div>
    </div>
    <div class="mt-3 flex min-h-5 flex-wrap items-center gap-x-1.5 gap-y-0.5 text-xs sm:mt-4">
      <span v-if="deltaText" :class="['inline-flex items-center gap-0.5 font-semibold', deltaClass]">
        <component :is="deltaIcon" class="w-3.5 h-3.5" />
        {{ deltaText }}
      </span>
      <span class="text-muted-foreground">{{ deltaCaption }}</span>
    </div>
  </component>
</template>
