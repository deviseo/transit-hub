<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { Activity, ArrowRight, X } from 'lucide-vue-next'
import { remoteActionLabelKey } from '../../composables/useConnectionHealth'
import ConnectionHealthLinkDetailCard from './ConnectionHealthLinkDetailCard.vue'
import type {
  ConnectionHealthEvent,
  ConnectionHealthState,
  AdminGroupHealth,
  EffectiveProbePolicySource,
  OwnGroupHealth,
} from '../../types/connectionHealth'

const props = defineProps<{
  open: boolean
  events: ConnectionHealthEvent[]
  groups: OwnGroupHealth[]
  adminGroups: AdminGroupHealth[]
  selectedConnectionId: string
  siteName: (id: string) => string
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'view-all'): void
}>()

import { t } from '@/locales'
const prefix = 'admin.connectionHealth'
const cardPrefix = `${prefix}.eventsDialog.card`

// 探活结果分类：用于近 60 次记录条着色和可用率计算分母。人工禁用/恢复不算一次探活结果，
// 不计入可用率分母，只在记录条里以中性色展示这个动作发生过。
const PROBE_RESULTS = new Set(['ok', 'slow_response', 'network_fluctuation', 'rate_limited', 'server_error', 'auth', 'model_not_found', 'invalid_response'])
const VALID_STATES = new Set<ConnectionHealthState>(['healthy', 'degraded', 'suspended', 'observing', 'recovering', 'disabled'])

interface StatusCard {
  key: string
  connectionId: string
  modelName: string
  upstreamSiteId: string
  upstreamGroupName: string
  accountName: string
  provider: string
  state: ConnectionHealthState | ''
  latestLatencyMs: number | null
  lastProbeAt: string | null
  nextProbeAt: string | null
  blockedReason: string
  lastFailureAt: string | null
  lastErrorKey: string
  lastErrorDetail: string
  elapsedSeconds: number | null
  effectiveIntervalSeconds: number
  effectivePolicySources: EffectiveProbePolicySource[]
  budgetPolicyId: string
  actionSource: string
  actionAt: string | null
  scheduleKnown: boolean
  availabilityPct: number | null
  records: ConnectionHealthEvent[]
  // 最近一次远端动作（newapi_channel_disabled / sub2api_account_status_inactive / unsupported /
  // skipped_independent_probe 等），空字符串表示这次探活没有触发远端动作。
  remoteAction: string
}

interface GroupBlock {
  key: string
  name: string
  cards: StatusCard[]
}

// connectionMeta 把当前分组健康主列表已有的「链路 -> 模型」健康数据（provider/当前状态/ownGroupId/
// 最近一次探活时间）按 connectionId 建索引。全局模式（未聚焦具体链路）按事件分组展示时，
// 用它关联出真实的当前状态和 provider，而不是从事件的 fromState/toState 猜测——
// 事件本身只记录状态迁移，不等于「当前」状态。
const connectionMeta = computed(() => {
  const map = new Map<string, { ownGroupId: string; models: Map<string, {
    providerFamily: string
    state: ConnectionHealthState
    lastProbeAt: string | null
    nextProbeAt: string | null
    blockedReason: string
    lastFailureAt: string | null
    lastErrorKey: string
    lastErrorDetail: string
    elapsedSeconds: number | null
    effectiveIntervalSeconds: number
    effectivePolicySources: EffectiveProbePolicySource[]
    budgetPolicyId: string
  }> }>()
  for (const group of props.groups) {
    for (const conn of group.connections) {
      const models = new Map<string, {
        providerFamily: string
        state: ConnectionHealthState
        lastProbeAt: string | null
        nextProbeAt: string | null
        blockedReason: string
        lastFailureAt: string | null
        lastErrorKey: string
        lastErrorDetail: string
        elapsedSeconds: number | null
        effectiveIntervalSeconds: number
        effectivePolicySources: EffectiveProbePolicySource[]
        budgetPolicyId: string
      }>()
      for (const model of conn.models) {
        models.set(model.modelName, {
          providerFamily: model.providerFamily,
          state: model.state,
          lastProbeAt: model.lastProbeAt,
          nextProbeAt: model.nextProbeAt ?? null,
          blockedReason: model.blockedReason ?? '',
          lastFailureAt: model.lastFailureAt,
          lastErrorKey: model.lastErrorKey,
          lastErrorDetail: model.lastErrorDetail,
          elapsedSeconds: model.elapsedSeconds ?? null,
          effectiveIntervalSeconds: model.effectiveIntervalSeconds ?? 0,
          effectivePolicySources: model.effectivePolicySources ?? [],
          budgetPolicyId: model.budgetPolicyId ?? '',
        })
      }
      map.set(conn.connectionId, { ownGroupId: group.ownGroupId, models })
    }
  }
  return map
})

type AdminTargetModelMeta = {
  providerFamily: string
  configured: boolean
  state: ConnectionHealthState | ''
  lastProbeAt: string | null
  lastLatencyMs: number | null
  lastRemoteAction: string
  nextProbeAt: string | null
  blockedReason: string
  lastFailureAt: string | null
  lastErrorKey: string
  lastErrorDetail: string
  elapsedSeconds: number | null
  effectiveIntervalSeconds: number
  effectivePolicySources: EffectiveProbePolicySource[]
  budgetPolicyId: string
}

type AdminTargetMeta = {
  groupNames: string[]
  accountName: string
  platform: string
  models: Map<string, AdminTargetModelMeta>
  policyIds: string[]
}

// 新版 admin target 不一定存在 real_connection；从 admin 分组主列表补齐当前模型元数据。
const adminTargetMeta = computed(() => {
  const map = new Map<string, AdminTargetMeta>()
  for (const group of props.adminGroups) {
    for (const account of group.accounts) {
      const models = new Map<string, AdminTargetModelMeta>()
      for (const model of account.modelHealth ?? []) {
        models.set(model.modelName, {
          providerFamily: model.providerFamily || account.platform || 'custom',
          configured: model.configured,
          state: model.configured ? model.state : '',
          lastProbeAt: model.lastProbeAt,
          lastLatencyMs: model.lastLatencyMs,
          lastRemoteAction: model.lastRemoteAction ?? '',
          nextProbeAt: model.nextProbeAt ?? null,
          blockedReason: model.blockedReason ?? '',
          lastFailureAt: model.lastFailureAt ?? null,
          lastErrorKey: model.lastErrorKey ?? '',
          lastErrorDetail: model.lastErrorDetail ?? '',
          elapsedSeconds: model.elapsedSeconds ?? null,
          effectiveIntervalSeconds: model.effectiveIntervalSeconds ?? 0,
          effectivePolicySources: model.effectivePolicySources ?? [],
          budgetPolicyId: model.budgetPolicyId ?? '',
        })
      }
      for (const model of account.unprobedModels ?? []) {
        if (models.has(model.modelName)) continue
        models.set(model.modelName, {
          providerFamily: model.providerFamily || account.platform || 'custom',
          configured: true,
          state: '',
          lastProbeAt: null,
          lastLatencyMs: null,
          lastRemoteAction: '',
          nextProbeAt: model.nextProbeAt ?? null,
          blockedReason: model.blockedReason ?? '',
          lastFailureAt: null,
          lastErrorKey: '',
          lastErrorDetail: '',
          elapsedSeconds: null,
          effectiveIntervalSeconds: model.effectiveIntervalSeconds ?? 0,
          effectivePolicySources: model.effectivePolicySources ?? [],
          budgetPolicyId: model.budgetPolicyId ?? '',
        })
      }
      const existing = map.get(account.targetId)
      if (existing) {
        for (const [modelName, model] of models) existing.models.set(modelName, model)
        if (group.name && !existing.groupNames.includes(group.name)) existing.groupNames.push(group.name)
        existing.policyIds = Array.from(new Set([
          ...existing.policyIds,
          ...(account.assignedPolicyIds ?? []),
          ...(group.assignedPolicyIds ?? []),
        ]))
        continue
      }
      map.set(account.targetId, {
        groupNames: group.name ? [group.name] : [],
        accountName: account.name || account.id,
        platform: account.targetId.toLowerCase().startsWith('sub2api:') ? 'Sub2API' : account.targetId.toLowerCase().startsWith('newapi:') ? 'NewAPI' : 'custom',
        models,
        policyIds: [...(account.assignedPolicyIds ?? []), ...(group.assignedPolicyIds ?? [])],
      })
    }
  }
  return map
})

const findConnectionContext = (connectionId: string) => {
  for (const group of props.groups) {
    const conn = group.connections.find((c) => c.connectionId === connectionId)
    if (conn) return { group, conn }
  }
  return null
}

const buildRecords = (eventsDesc: ConnectionHealthEvent[]) => {
  // 近 60 次记录条：取最新 60 条后反转为「从过去到现在」排列，条数不足 60 就按实际数量渲染。
  const records = eventsDesc.slice(0, 60).slice().reverse()
  const probeRecords = records.filter((r) => PROBE_RESULTS.has(r.result))
  const okCount = probeRecords.filter((r) => r.result === 'ok' || r.result === 'slow_response').length
  const availabilityPct = probeRecords.length > 0 ? Math.round((okCount / probeRecords.length) * 100) : null
  return { records, availabilityPct }
}

// 链路详情模式：以该链路已配置的模型列表（ConnectionHealth.models）为准逐个建卡，
// 而不是只从事件反推——这样从未探活过的模型也能展示一张「尚未探活」的卡片，
// 而不是因为没有事件就完全不出现。
const buildFocusedCards = (connectionId: string): StatusCard[] => {
  const ctx = findConnectionContext(connectionId)
  const eventsByModel = new Map<string, ConnectionHealthEvent[]>()
  for (const ev of props.events) {
    if (ev.connectionId !== connectionId) continue
    if (ev.modelName === '*' || ev.modelName === '') continue
    if (!eventsByModel.has(ev.modelName)) eventsByModel.set(ev.modelName, [])
    eventsByModel.get(ev.modelName)!.push(ev)
  }

  const actionEvents = props.events.filter((ev) => ev.connectionId === connectionId && (ev.modelName === '*' || ev.modelName === ''))
  const buildActionCard = (event: ConnectionHealthEvent, provider: string, upstreamGroupName: string, upstreamSiteId: string, accountName = ''): StatusCard => ({
    key: `${connectionId}::action::${event.id}`,
    connectionId,
    modelName: t(`${prefix}.eventsDialog.accountAction`),
    upstreamSiteId,
    upstreamGroupName,
    accountName,
    provider,
    state: '',
    latestLatencyMs: null,
    lastProbeAt: null,
    nextProbeAt: null,
    blockedReason: '',
    lastFailureAt: null,
    lastErrorKey: event.errorKey ?? '',
    lastErrorDetail: event.errorDetail ?? '',
    elapsedSeconds: null,
    effectiveIntervalSeconds: 0,
    effectivePolicySources: [],
    budgetPolicyId: '',
    actionSource: event.actionSource ?? (event.remoteAction ? 'user_action' : ''),
    actionAt: event.createdAt,
    scheduleKnown: false,
    availabilityPct: null,
    records: [event],
    remoteAction: event.remoteAction ?? '',
  })

  if (ctx) {
    const modelCards = ctx.conn.models.map((model) => {
      const eventsDesc = eventsByModel.get(model.modelName) ?? []
      const latestAction = eventsDesc.find((event) => Boolean(event.remoteAction))
      const { records, availabilityPct } = buildRecords(eventsDesc)
      return {
        key: `${connectionId}::${model.modelName}`,
        connectionId,
        modelName: model.modelName,
        upstreamSiteId: ctx.conn.upstreamSiteId,
        upstreamGroupName: ctx.conn.upstreamGroupName,
        accountName: '',
        provider: model.providerFamily,
        state: model.state,
        latestLatencyMs: eventsDesc[0]?.latencyMs ?? model.lastLatencyMs,
        lastProbeAt: model.lastProbeAt,
        nextProbeAt: model.nextProbeAt ?? null,
        blockedReason: model.blockedReason ?? '',
        lastFailureAt: model.lastFailureAt,
        lastErrorKey: model.lastErrorKey,
        lastErrorDetail: model.lastErrorDetail,
        elapsedSeconds: model.elapsedSeconds ?? null,
        effectiveIntervalSeconds: model.effectiveIntervalSeconds ?? 0,
        effectivePolicySources: model.effectivePolicySources ?? [],
        budgetPolicyId: model.budgetPolicyId ?? '',
        actionSource: latestAction?.actionSource ?? '',
        actionAt: latestAction?.createdAt ?? null,
        scheduleKnown: model.nextProbeAt != null || Boolean(model.blockedReason) || model.state === 'disabled',
        availabilityPct,
        records,
        remoteAction: latestAction?.remoteAction ?? '',
      }
    })
    return [...actionEvents.map((event) => buildActionCard(event, ctx.conn.models[0]?.providerFamily ?? 'custom', ctx.conn.upstreamGroupName, ctx.conn.upstreamSiteId)), ...modelCards]
  }

  const adminMeta = adminTargetMeta.value.get(connectionId)
  if (adminMeta) {
    const groupName = adminMeta.groupNames.join('、')
    const modelCards = Array.from(adminMeta.models.entries()).map(([modelName, modelMeta]) => {
      const eventsDesc = eventsByModel.get(modelName) ?? []
      const latestAction = eventsDesc.find((event) => Boolean(event.remoteAction))
      const { records, availabilityPct } = buildRecords(eventsDesc)
      return {
        key: `${connectionId}::${modelName}`,
        connectionId,
        modelName,
        upstreamSiteId: adminMeta.platform,
        upstreamGroupName: groupName,
        accountName: adminMeta.accountName,
        provider: modelMeta.providerFamily,
        state: modelMeta.state,
        latestLatencyMs: eventsDesc[0]?.latencyMs ?? modelMeta.lastLatencyMs,
        lastProbeAt: modelMeta.lastProbeAt,
        nextProbeAt: modelMeta.nextProbeAt,
        blockedReason: modelMeta.blockedReason,
        lastFailureAt: modelMeta.lastFailureAt,
        lastErrorKey: modelMeta.lastErrorKey,
        lastErrorDetail: modelMeta.lastErrorDetail,
        elapsedSeconds: modelMeta.elapsedSeconds,
        effectiveIntervalSeconds: modelMeta.effectiveIntervalSeconds,
        effectivePolicySources: modelMeta.effectivePolicySources,
        budgetPolicyId: modelMeta.budgetPolicyId,
        actionSource: latestAction?.actionSource ?? '',
        actionAt: latestAction?.createdAt ?? null,
        scheduleKnown: true,
        availabilityPct,
        records,
        remoteAction: latestAction?.remoteAction ?? '',
      }
    })
    const actionProvider = adminMeta.models.values().next().value?.providerFamily ?? 'custom'
    return [...actionEvents.map((event) => buildActionCard(event, actionProvider, groupName, adminMeta.platform, adminMeta.accountName)), ...modelCards]
  }

  // 兜底：groups 数据还没跟上时（极少见的时序问题），退化为纯粹从事件推导，
  // 保证弹窗至少不是空的；此时无法解析 ownGroupId，探活间隔只能显示「未配置策略」。
  const modelCards = Array.from(eventsByModel.entries()).map(([modelName, eventsDesc]) => {
    const latest = eventsDesc[0]
    const latestAction = eventsDesc.find((event) => Boolean(event.remoteAction))
    const { records, availabilityPct } = buildRecords(eventsDesc)
    const state: ConnectionHealthState | '' = VALID_STATES.has(latest.toState as ConnectionHealthState) ? (latest.toState as ConnectionHealthState) : ''
    return {
      key: `${connectionId}::${modelName}`,
      connectionId,
      modelName,
      upstreamSiteId: latest.upstreamSiteId,
      upstreamGroupName: latest.upstreamGroupName,
      accountName: '',
      provider: 'custom',
      state,
      latestLatencyMs: latest.latencyMs,
      lastProbeAt: latest.createdAt,
      nextProbeAt: null,
      blockedReason: '',
      lastFailureAt: null,
      lastErrorKey: latest.errorKey,
      lastErrorDetail: latest.errorDetail ?? '',
      elapsedSeconds: null,
      effectiveIntervalSeconds: 0,
      effectivePolicySources: [],
      budgetPolicyId: '',
      actionSource: latestAction?.actionSource ?? '',
      actionAt: latestAction?.createdAt ?? null,
      scheduleKnown: false,
      availabilityPct,
      records,
      remoteAction: latestAction?.remoteAction ?? '',
    }
  })
  return [...actionEvents.map((event) => buildActionCard(event, 'custom', event.upstreamGroupName, event.upstreamSiteId)), ...modelCards]
}

const focusedCards = computed<StatusCard[]>(() =>
  props.selectedConnectionId ? buildFocusedCards(props.selectedConnectionId) : [],
)

// 全局模式（顶部"探活事件"入口，未聚焦具体链路）：仍按 ownGroupName 分组展示最近事件，
// 但只保留分组名作为轻量标题，不再展示「x 条链路 · x 条事件」这类汇总。
const globalGroups = computed<GroupBlock[]>(() => {
  const groupOrder: string[] = []
  const groupMap = new Map<string, { name: string; cardOrder: string[]; cards: Map<string, ConnectionHealthEvent[]> }>()

  for (const ev of props.events) {
    const groupKey = ev.ownGroupName || ev.connectionId
    if (!groupMap.has(groupKey)) {
      groupMap.set(groupKey, { name: ev.ownGroupName || ev.connectionId, cardOrder: [], cards: new Map() })
      groupOrder.push(groupKey)
    }
    const bucket = groupMap.get(groupKey)!
    const cardKey = `${ev.connectionId}::${ev.modelName}`
    if (!bucket.cards.has(cardKey)) {
      bucket.cards.set(cardKey, [])
      bucket.cardOrder.push(cardKey)
    }
    bucket.cards.get(cardKey)!.push(ev)
  }

  return groupOrder.map((groupKey) => {
    const bucket = groupMap.get(groupKey)!
    const cards: StatusCard[] = bucket.cardOrder.map((cardKey) => {
      const eventsDesc = bucket.cards.get(cardKey)!
      const latest = eventsDesc[0]
      const meta = connectionMeta.value.get(latest.connectionId)
      const legacyModelMeta = meta?.models.get(latest.modelName)
      const adminMeta = adminTargetMeta.value.get(latest.connectionId)
      const adminModelMeta = adminMeta?.models.get(latest.modelName)
      const failureMeta = adminModelMeta ?? legacyModelMeta
      const latestAction = eventsDesc.find((event) => Boolean(event.remoteAction))
      const state: ConnectionHealthState | '' = legacyModelMeta?.state ?? adminModelMeta?.state
        ?? (VALID_STATES.has(latest.toState as ConnectionHealthState) ? (latest.toState as ConnectionHealthState) : '')
      const { records, availabilityPct } = buildRecords(eventsDesc)

      return {
        key: cardKey,
        connectionId: latest.connectionId,
        modelName: latest.modelName === '*' || latest.modelName === ''
          ? t(`${prefix}.eventsDialog.accountAction`)
          : latest.modelName,
        upstreamSiteId: latest.upstreamSiteId || adminMeta?.platform || '',
        upstreamGroupName: adminMeta?.groupNames.join('、') || latest.upstreamGroupName,
        accountName: adminMeta?.accountName ?? '',
        provider: legacyModelMeta?.providerFamily ?? adminModelMeta?.providerFamily ?? 'custom',
        state,
        latestLatencyMs: latest.latencyMs ?? adminModelMeta?.lastLatencyMs ?? null,
        lastProbeAt: legacyModelMeta?.lastProbeAt ?? adminModelMeta?.lastProbeAt ?? null,
        nextProbeAt: legacyModelMeta?.nextProbeAt ?? adminModelMeta?.nextProbeAt ?? null,
        blockedReason: legacyModelMeta?.blockedReason ?? adminModelMeta?.blockedReason ?? '',
        lastFailureAt: failureMeta?.lastFailureAt ?? null,
        lastErrorKey: failureMeta?.lastErrorKey ?? latest.errorKey ?? '',
        lastErrorDetail: failureMeta?.lastErrorDetail ?? latest.errorDetail ?? '',
        elapsedSeconds: failureMeta?.elapsedSeconds ?? null,
        effectiveIntervalSeconds: legacyModelMeta?.effectiveIntervalSeconds ?? adminModelMeta?.effectiveIntervalSeconds ?? 0,
        effectivePolicySources: legacyModelMeta?.effectivePolicySources ?? adminModelMeta?.effectivePolicySources ?? [],
        budgetPolicyId: legacyModelMeta?.budgetPolicyId ?? adminModelMeta?.budgetPolicyId ?? '',
        actionSource: latestAction?.actionSource ?? '',
        actionAt: latestAction?.createdAt ?? null,
        scheduleKnown: adminModelMeta != null || legacyModelMeta?.nextProbeAt != null || Boolean(legacyModelMeta?.blockedReason) || legacyModelMeta?.state === 'disabled',
        availabilityPct,
        records,
        remoteAction: latestAction?.remoteAction ?? '',
      }
    })

    return { key: groupKey, name: bucket.name, cards }
  })
})

const selectedConnectionMeta = computed(() => {
  if (!props.selectedConnectionId) return null
  const ctx = findConnectionContext(props.selectedConnectionId)
  if (ctx) {
    return {
      siteLabel: props.siteName(ctx.conn.upstreamSiteId),
      upstreamGroupName: ctx.conn.upstreamGroupName,
      ownGroupName: ctx.group.ownGroupName || ctx.group.ownGroupId,
    }
  }
  const adminMeta = adminTargetMeta.value.get(props.selectedConnectionId)
  return adminMeta ? { siteLabel: adminMeta.platform, upstreamGroupName: adminMeta.groupNames.join('、'), ownGroupName: adminMeta.accountName } : null
})

// nowMs 只把后端给出的权威 nextProbeAt 渲染为动态倒计时，不参与调度时间计算。
const nowMs = ref(Date.now())
let tickTimer: ReturnType<typeof window.setInterval> | null = null

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      nowMs.value = Date.now()
      if (!tickTimer) tickTimer = window.setInterval(() => { nowMs.value = Date.now() }, 1000)
    } else if (tickTimer) {
      window.clearInterval(tickTimer)
      tickTimer = null
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  if (tickTimer) window.clearInterval(tickTimer)
})

// 下次探活只消费后端返回的 nextProbeAt / blockedReason，不在浏览器重建调度规则。
const nextProbeLabel = (card: StatusCard): string => {
  if (card.actionSource && !card.lastProbeAt && !card.nextProbeAt) return t(`${cardPrefix}.nextProbeActionOnly`)
  if (!card.scheduleKnown) return t(`${cardPrefix}.nextProbeUnknown`)
  if (card.state === 'disabled') return t(`${cardPrefix}.nextProbeDisabled`)
  if (!card.nextProbeAt) {
    if (card.blockedReason === 'upstream_scheduling_disabled') return t(`${cardPrefix}.nextProbeBlocked`, { reason: t(`${prefix}.groupDetail.schedulableOff`) })
    if (card.blockedReason === 'health_disabled') return t(`${cardPrefix}.nextProbeBlocked`, { reason: t(`${prefix}.groupDetail.healthSuspended`) })
    if (card.blockedReason) return t(`${cardPrefix}.nextProbeBlocked`, { reason: t(`${prefix}.probeBlockedReasons.${card.blockedReason}`) })
    return card.lastProbeAt ? t(`${cardPrefix}.nextProbeDue`) : t(`${cardPrefix}.nextProbeNeverProbed`)
  }
  const nextAt = new Date(card.nextProbeAt).getTime()
  if (Number.isNaN(nextAt)) return t(`${cardPrefix}.nextProbeNeverProbed`)
  const remainingMs = nextAt - nowMs.value
  if (remainingMs > 0) {
    const countdown = t(`${cardPrefix}.nextProbeIn`, { seconds: Math.ceil(remainingMs / 1000) })
    if (!card.blockedReason) return countdown
    return t(`${cardPrefix}.nextProbeWaiting`, {
      countdown,
      reason: t(`${prefix}.probeBlockedReasons.${card.blockedReason}`),
    })
  }
  return t(`${cardPrefix}.nextProbeDue`)
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div v-if="open" class="fixed inset-0 z-[140] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="emit('close')" />

      <div role="dialog" aria-modal="true" :aria-label="t(`${prefix}.events.title`)" class="relative flex h-[min(760px,calc(100dvh-2rem))] w-full max-w-6xl flex-col overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl">
          <div class="flex shrink-0 items-center justify-between gap-3 border-b border-border/60 px-5 py-4">
            <div class="flex min-w-0 items-center gap-2.5">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Activity class="h-4 w-4" />
              </div>
              <div class="min-w-0">
                <h3 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.events.title`) }}</h3>
                <p class="truncate text-xs text-muted-foreground">
                  {{ selectedConnectionId ? t(`${prefix}.eventsDialog.subtitle`) : t(`${prefix}.eventsDialog.globalSubtitle`) }}
                </p>
              </div>
            </div>
            <button
              type="button"
              class="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
              @click="emit('close')"
            >
              <X class="h-4 w-4" />
            </button>
          </div>

          <div class="flex-1 overflow-y-auto px-5 py-4">
            <!-- 链路详情模式：聚焦当前链路，头部横幅展示站点/上游分组/我的分组，提供"查看全部"退回全局。 -->
            <template v-if="selectedConnectionId">
              <div
                v-if="selectedConnectionMeta"
                class="mb-4 flex flex-wrap items-center justify-between gap-2 rounded-lg border border-primary/30 bg-primary/5 px-4 py-2.5"
              >
                <p class="text-xs text-foreground">
                  <span class="font-medium">{{ t(`${prefix}.eventsDialog.viewingConnection`) }}</span>
                  <span class="text-muted-foreground"> · {{ selectedConnectionMeta.siteLabel }} · {{ selectedConnectionMeta.upstreamGroupName }} · {{ selectedConnectionMeta.ownGroupName }}</span>
                </p>
                <button type="button" class="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline" @click="emit('view-all')">
                  {{ t(`${prefix}.events.showAll`) }}
                  <ArrowRight class="h-3 w-3" />
                </button>
              </div>

              <div v-if="focusedCards.length === 0" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
                <Activity class="h-8 w-8 text-muted-foreground/40" />
                <p class="text-sm text-muted-foreground">{{ t(`${prefix}.events.emptyForConnection`) }}</p>
              </div>
              <div v-else class="grid grid-cols-1 gap-3 md:grid-cols-2">
                <ConnectionHealthLinkDetailCard
                  v-for="card in focusedCards"
                  :key="card.key"
                  :site-label="siteName(card.upstreamSiteId)"
                  :account-name="card.accountName"
                  :upstream-group-name="card.upstreamGroupName"
                  :model-name="card.modelName"
                  :provider="card.provider"
                  :state="card.state"
                  :latest-latency-ms="card.latestLatencyMs"
                  :last-probe-at="card.lastProbeAt"
                  :last-failure-at="card.lastFailureAt"
                  :last-error-key="card.lastErrorKey"
                  :last-error-detail="card.lastErrorDetail"
                  :elapsed-seconds="card.elapsedSeconds"
                  :effective-interval-seconds="card.effectiveIntervalSeconds"
                  :effective-policy-sources="card.effectivePolicySources"
                  :budget-policy-id="card.budgetPolicyId"
                  :availability-pct="card.availabilityPct"
                  :records="card.records"
                  :next-probe-text="nextProbeLabel(card)"
                  :remote-action="card.remoteAction"
                  :action-source="card.actionSource"
                  :action-at="card.actionAt"
                />
              </div>
            </template>

            <!-- 全局模式：顶部"探活事件"入口进入，按分组展示最近事件，分组名只作轻量标题。 -->
            <template v-else>
              <div v-if="events.length === 0" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
                <Activity class="h-8 w-8 text-muted-foreground/40" />
                <p class="text-sm text-muted-foreground">{{ t(`${prefix}.events.empty`) }}</p>
              </div>

              <div v-else class="space-y-6">
                <div v-for="group in globalGroups" :key="group.key">
                  <h4 class="mb-2.5 text-sm font-semibold text-foreground">{{ group.name }}</h4>

                  <div class="grid grid-cols-1 gap-3 md:grid-cols-2">
                    <ConnectionHealthLinkDetailCard
                      v-for="card in group.cards"
                      :key="card.key"
                      :site-label="siteName(card.upstreamSiteId)"
                      :account-name="card.accountName"
                      :upstream-group-name="card.upstreamGroupName"
                      :model-name="card.modelName"
                      :provider="card.provider"
                      :state="card.state"
                      :latest-latency-ms="card.latestLatencyMs"
                      :last-probe-at="card.lastProbeAt"
                      :last-failure-at="card.lastFailureAt"
                      :last-error-key="card.lastErrorKey"
                      :last-error-detail="card.lastErrorDetail"
                      :elapsed-seconds="card.elapsedSeconds"
                      :effective-interval-seconds="card.effectiveIntervalSeconds"
                      :effective-policy-sources="card.effectivePolicySources"
                      :budget-policy-id="card.budgetPolicyId"
                      :availability-pct="card.availabilityPct"
                      :records="card.records"
                      :next-probe-text="nextProbeLabel(card)"
                      :remote-action="card.remoteAction"
                      :action-source="card.actionSource"
                      :action-at="card.actionAt"
                    />
                  </div>
                </div>
              </div>
            </template>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
