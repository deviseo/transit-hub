<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import DeltaBadge from './DeltaBadge.vue'
import { useUserActivityFormat } from '../../composables/useUserActivityFormat'
import type { UserActivityPeriodComparison, UserActivityPeriodKey, UserActivityPeriodMetrics } from '../../types/userActivity'

// 日 / 周 / 月对比卡：本期数值 + 环比（vs 上期）+ 同比（vs 去年同期）。
const props = withDefaults(defineProps<{
  periodKey: UserActivityPeriodKey
  comparison?: UserActivityPeriodComparison
  // 单个用户的详情不显示活跃人数、新增注册与留存。
  scope?: 'site' | 'user'
}>(), {
  scope: 'site',
})

const { t } = useI18n()
const { formatCount, formatUsd, formatRange } = useUserActivityFormat()

type MetricKey = 'activeUsers' | 'requests' | 'cost' | 'newUsers'

const metrics = computed<MetricKey[]>(() => (props.scope === 'site' ? ['activeUsers', 'requests', 'cost', 'newUsers'] : ['requests', 'cost']))

const valueOf = (period: UserActivityPeriodMetrics | null | undefined, key: MetricKey): number | null => (period ? period[key] : null)

const display = (key: MetricKey, value: number | null): string => {
  if (value == null) return t('admin.userActivity.common.placeholder')
  return key === 'cost' ? formatUsd(value) : formatCount(value)
}
</script>

<template>
  <article class="min-w-0 rounded-lg border border-border/50 bg-card p-4 shadow-sm">
    <header class="flex flex-wrap items-start justify-between gap-2">
      <div class="min-w-0">
        <h3 class="whitespace-nowrap text-sm font-semibold text-foreground">{{ t(`admin.userActivity.periods.${periodKey}.title`) }}</h3>
        <p class="mt-0.5 truncate text-xs text-muted-foreground">
          {{ t(`admin.userActivity.periods.${periodKey}.description`) }}
        </p>
      </div>
      <span class="shrink-0 rounded-md bg-surface px-2 py-0.5 text-xs text-muted-foreground tabular-nums">
        {{ comparison ? formatRange(comparison.current.from, comparison.current.to) : t('admin.userActivity.common.placeholder') }}
      </span>
    </header>

    <table class="mt-3 w-full text-sm">
      <thead>
        <tr class="text-xs text-muted-foreground">
          <th class="whitespace-nowrap pb-1.5 text-left font-normal">{{ t('admin.userActivity.periods.metric') }}</th>
          <th class="whitespace-nowrap pb-1.5 pl-2 text-right font-normal">{{ t('admin.userActivity.periods.current') }}</th>
          <th class="whitespace-nowrap pb-1.5 pl-2 text-right font-normal">{{ t('admin.userActivity.periods.sequential') }}</th>
          <th class="whitespace-nowrap pb-1.5 pl-2 text-right font-normal">{{ t('admin.userActivity.periods.yearOverYear') }}</th>
        </tr>
      </thead>
      <tbody class="divide-y divide-border/40">
        <tr v-for="key in metrics" :key="key">
          <td class="py-1.5 text-muted-foreground">{{ t(`admin.userActivity.metrics.${key}`) }}</td>
          <td class="py-1.5 text-right font-semibold tabular-nums text-foreground">{{ display(key, valueOf(comparison?.current, key)) }}</td>
          <td class="py-1.5 text-right">
            <DeltaBadge
              :current="valueOf(comparison?.current, key) ?? 0"
              :previous="valueOf(comparison?.previous, key)"
              :missing-hint="t('admin.userActivity.periods.previousMissing')"
            />
          </td>
          <td class="py-1.5 text-right">
            <DeltaBadge
              :current="valueOf(comparison?.current, key) ?? 0"
              :previous="valueOf(comparison?.lastYear, key)"
              :missing-hint="t('admin.userActivity.periods.lastYearMissing')"
            />
          </td>
        </tr>
      </tbody>
    </table>

    <p v-if="scope === 'site' && comparison?.retention" class="mt-3 flex flex-wrap gap-x-3 gap-y-1 border-t border-border/40 pt-2 text-xs text-muted-foreground">
      <span>{{ t('admin.userActivity.retention.retained', { count: formatCount(comparison.retention.retained) }) }}</span>
      <span>{{ t('admin.userActivity.retention.churned', { count: formatCount(comparison.retention.churned) }) }}</span>
      <span>{{ t('admin.userActivity.retention.gained', { count: formatCount(comparison.retention.gained) }) }}</span>
    </p>
    <p v-else-if="comparison && !comparison.previous" class="mt-3 border-t border-border/40 pt-2 text-xs text-muted-foreground">
      {{ t('admin.userActivity.periods.previousMissing') }}
    </p>
  </article>
</template>
