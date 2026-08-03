<script setup lang="ts">
import { computed } from 'vue'
import { Zap } from 'lucide-vue-next'
import {
  connectionHealthMessageKey,
  connectionHealthRecordColorClass,
  connectionHealthStateBadgeClass,
  formatConnectionHealthTime,
  remoteActionLabelKey,
} from '../../composables/useConnectionHealth'
import type { ConnectionHealthEvent, ConnectionHealthState, EffectiveProbePolicySource } from '../../types/connectionHealth'

// 链路详情健康卡片：链路详情弹窗（聚焦某条链路）和全局最近事件弹窗共用同一张卡片布局，
// 数据聚合和"下次探活"文案计算都由父组件完成，这里只负责纯展示，避免两处弹窗各自维护
// 一份几乎相同的卡片模板。
const props = defineProps<{
  siteLabel: string
  upstreamGroupName: string
  accountName: string
  modelName: string
  provider: string
  state: ConnectionHealthState | ''
  latestLatencyMs: number | null
  lastProbeAt: string | null
  lastFailureAt: string | null
  lastErrorKey: string
  lastErrorDetail: string
  elapsedSeconds: number | null
  effectiveIntervalSeconds: number
  effectivePolicySources: EffectiveProbePolicySource[]
  budgetPolicyId: string
  availabilityPct: number | null
  records: ConnectionHealthEvent[]
  nextProbeText: string
  actionSource: string
  actionAt: string | null
  // 最近一次探活触发的远端动作原始字符串，空串表示没有触发远端动作（不展示这一行）。
  remoteAction: string
}>()

import { t, te } from '@/locales'
const prefix = 'admin.connectionHealth'
const cardPrefix = `${prefix}.eventsDialog.card`
const readableMessage = (rawKey: string): string => t(connectionHealthMessageKey(rawKey, te))

const actionSourceLabel = computed(() => {
  if (props.actionSource === 'user_action') return t(`${cardPrefix}.actionSources.userAction`)
  if (props.actionSource === 'upstream_observed') return t(`${cardPrefix}.actionSources.upstreamObserved`)
  if (props.actionSource === 'health_probe') return t(`${cardPrefix}.actionSources.automatic`)
  return props.actionSource
})

const effectivePolicySourcesText = computed(() => props.effectivePolicySources.map((source) => t(`${cardPrefix}.decisionSource`, {
  name: source.policyName || source.policyId,
  state: source.continueAutoProbe ? t(`${cardPrefix}.sourceContinues`) : t(`${cardPrefix}.sourceStops`),
  interval: source.effectiveIntervalSeconds,
})).join('；'))

const budgetPolicyLabel = computed(() => props.effectivePolicySources.find((source) => source.policyId === props.budgetPolicyId)?.policyName || props.budgetPolicyId)
const slowResponseCount = computed(() => props.records.filter(record => record.result === 'slow_response').length)

const eventSourceLabel = (source?: string): string => {
  const key = `${cardPrefix}.eventSources.${source || 'legacy'}`
  return te(key) ? t(key) : (source || t(`${cardPrefix}.eventSources.legacy`))
}

const latestEventSource = computed(() => {
  const latest = props.records[props.records.length - 1]
  return latest ? eventSourceLabel(latest.source) : ''
})

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
          <p class="truncate text-sm font-medium text-foreground">{{ accountName || siteLabel }}</p>
          <p class="truncate text-xs text-muted-foreground">{{ upstreamGroupName }} · {{ modelName }}</p>
          <p v-if="accountName && siteLabel" class="truncate text-[11px] text-muted-foreground">{{ siteLabel }}</p>
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
        <p class="text-[11px] text-muted-foreground">{{ t(`${cardPrefix}.lastProbe`, { value: '' }) }}</p>
        <p class="mt-0.5 text-xs font-medium text-foreground">{{ formatConnectionHealthTime(lastProbeAt) }}</p>
      </div>
      <div class="rounded-lg border border-border/40 bg-background/60 px-3 py-2">
        <p class="text-[11px] text-muted-foreground">{{ t(`${cardPrefix}.lastFailure`, { value: '' }) }}</p>
        <p class="mt-0.5 text-xs font-medium text-foreground">{{ formatConnectionHealthTime(lastFailureAt) }}</p>
      </div>
    </div>

    <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
      <span>{{ t(`${cardPrefix}.latencyLabel`) }}：{{ latestLatencyMs != null ? `${latestLatencyMs}ms` : t(`${cardPrefix}.noData`) }}</span>
      <span v-if="elapsedSeconds != null">{{ t(`${cardPrefix}.elapsed`, { seconds: elapsedSeconds }) }}</span>
    </div>
    <p v-if="lastErrorKey" class="mt-2 text-xs text-destructive">{{ readableMessage(lastErrorKey) }}</p>
    <p v-if="lastErrorDetail" class="mt-1 break-words text-xs text-muted-foreground">{{ t(`${cardPrefix}.errorDetail`, { value: lastErrorDetail }) }}</p>

    <div class="mt-3">
      <p class="text-[11px] text-muted-foreground">{{ t(`${cardPrefix}.availabilityLabel`) }}</p>
      <p class="text-2xl font-bold text-foreground">
        {{ availabilityPct != null ? `${availabilityPct}%` : t(`${cardPrefix}.noData`) }}
      </p>
      <p v-if="slowResponseCount > 0" class="mt-1 text-xs text-amber-600 dark:text-amber-400">
        {{ t(`${cardPrefix}.slowResponseCount`, { count: slowResponseCount }) }}
      </p>
    </div>

    <p class="mt-3 text-xs font-medium text-foreground">{{ nextProbeText }}</p>
    <p v-if="effectiveIntervalSeconds > 0 || effectivePolicySourcesText" class="mt-1 text-xs text-muted-foreground">
      {{ t(`${cardPrefix}.effectiveDecision`, { interval: effectiveIntervalSeconds, sources: effectivePolicySourcesText || '-' }) }}
    </p>
    <p v-if="budgetPolicyId" class="mt-1 text-xs text-muted-foreground">{{ t(`${cardPrefix}.budgetPolicy`, { policy: budgetPolicyLabel }) }}</p>
    <p v-if="actionSource" class="mt-1 text-xs text-muted-foreground">{{ t(`${cardPrefix}.actionSource`, { source: actionSourceLabel, time: formatConnectionHealthTime(actionAt) }) }}</p>
    <p v-if="latestEventSource" class="mt-1 text-xs text-muted-foreground">{{ t(`${cardPrefix}.eventSource`, { source: latestEventSource }) }}</p>
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
            :title="`${formatConnectionHealthTime(record.createdAt)} · ${readableMessage(record.result)} · ${eventSourceLabel(record.source)}`"
          />
        </div>
        <span class="text-[10px] text-muted-foreground">{{ t(`${cardPrefix}.now`) }}</span>
      </div>
    </div>
  </div>
</template>
