<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Zap } from 'lucide-vue-next'
import {
  connectionHealthRecordColorClass,
  connectionHealthStateBadgeClass,
  formatConnectionHealthTime,
  remoteActionLabelKey,
} from '../../composables/useConnectionHealth'
import type { ConnectionHealthEvent, ConnectionHealthState } from '../../types/connectionHealth'

// 链路详情健康卡片：链路详情弹窗（聚焦某条链路）和全局最近事件弹窗共用同一张卡片布局，
// 数据聚合和"下次探活"文案计算都由父组件完成，这里只负责纯展示，避免两处弹窗各自维护
// 一份几乎相同的卡片模板。
const props = defineProps<{
  siteLabel: string
  upstreamGroupName: string
  modelName: string
  provider: string
  state: ConnectionHealthState | ''
  latestLatencyMs: number | null
  availabilityPct: number | null
  records: ConnectionHealthEvent[]
  nextProbeText: string
  // 最近一次探活触发的远端动作原始字符串，空串表示没有触发远端动作（不展示这一行）。
  remoteAction: string
}>()

const { t } = useI18n()
const prefix = 'admin.connectionHealth'
const cardPrefix = `${prefix}.eventsDialog.card`

// remoteActionText 为空字符串时模板不渲染这一行；失败/unsupported 也照常展示，不隐藏。
const remoteActionText = computed(() => {
  const mapped = remoteActionLabelKey(props.remoteAction)
  if (!mapped) return ''
  return t(mapped.key, mapped.params ?? {})
})
</script>

<template>
  <div class="rounded-xl border border-border/50 bg-surface/40 p-4">
    <div class="flex items-start justify-between gap-2">
      <div class="flex min-w-0 items-center gap-2">
        <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
          <Zap class="h-4 w-4" />
        </div>
        <div class="min-w-0">
          <p class="truncate text-sm font-medium text-foreground">{{ siteLabel }}</p>
          <p class="truncate text-xs text-muted-foreground">{{ upstreamGroupName }} · {{ modelName }}</p>
        </div>
      </div>
      <div class="flex shrink-0 flex-col items-end gap-1">
        <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" :class="connectionHealthStateBadgeClass(state)">
          {{ state ? t(`${prefix}.stateLabels.${state}`) : '—' }}
        </span>
        <span class="inline-flex items-center rounded-full bg-surface-elevated px-2 py-0.5 text-[11px] text-muted-foreground">
          {{ t(`${prefix}.providerLabels.${provider}`) }}
        </span>
      </div>
    </div>

    <div class="mt-3 grid grid-cols-2 gap-2">
      <div class="rounded-lg border border-border/40 bg-background/60 px-3 py-2">
        <p class="text-[11px] text-muted-foreground">{{ t(`${cardPrefix}.latencyLabel`) }}</p>
        <p class="mt-0.5 text-sm font-semibold text-foreground">
          {{ latestLatencyMs != null ? `${latestLatencyMs}ms` : t(`${cardPrefix}.noData`) }}
        </p>
      </div>
      <div class="rounded-lg border border-border/40 bg-background/60 px-3 py-2">
        <p class="text-[11px] text-muted-foreground">{{ t(`${cardPrefix}.pingLabel`) }}</p>
        <p class="mt-0.5 text-sm font-semibold text-foreground">{{ t(`${cardPrefix}.noData`) }}</p>
      </div>
    </div>

    <div class="mt-3">
      <p class="text-[11px] text-muted-foreground">{{ t(`${cardPrefix}.availabilityLabel`) }}</p>
      <p class="text-2xl font-bold text-foreground">
        {{ availabilityPct != null ? `${availabilityPct}%` : t(`${cardPrefix}.noData`) }}
      </p>
    </div>

    <p class="mt-3 text-xs font-medium text-foreground">{{ nextProbeText }}</p>
    <p v-if="remoteActionText" class="mt-1 text-xs text-muted-foreground">{{ t(`${cardPrefix}.remoteActionLine`, { label: remoteActionText }) }}</p>

    <div class="mt-3">
      <div class="mb-1 flex items-center justify-between text-[10px] text-muted-foreground">
        <span>{{ t(`${cardPrefix}.recentRecordsLabel`) }} ({{ records.length }})</span>
      </div>
      <div class="flex items-center gap-2">
        <span class="text-[10px] text-muted-foreground">{{ t(`${cardPrefix}.past`) }}</span>
        <div class="flex h-6 flex-1 items-stretch gap-[2px] overflow-hidden rounded-md bg-surface-elevated/40 p-1">
          <span
            v-for="record in records"
            :key="record.id"
            class="min-w-[2px] flex-1 rounded-[1px]"
            :class="connectionHealthRecordColorClass(record.result)"
            :title="`${formatConnectionHealthTime(record.createdAt)} · ${t(`${prefix}.errorKeys.${record.result}`)}`"
          />
        </div>
        <span class="text-[10px] text-muted-foreground">{{ t(`${cardPrefix}.now`) }}</span>
      </div>
    </div>
  </div>
</template>
