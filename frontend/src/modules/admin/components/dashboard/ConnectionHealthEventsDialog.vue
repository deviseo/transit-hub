<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Activity, ArrowRight, X } from 'lucide-vue-next'
import { matchingProbeIntervalSeconds, remoteActionLabelKey } from '../../composables/useConnectionHealth'
import ConnectionHealthLinkDetailCard from './ConnectionHealthLinkDetailCard.vue'
import type {
  ConnectionHealthEvent,
  ConnectionHealthPolicy,
  ConnectionHealthState,
  OwnGroupHealth,
} from '../../types/connectionHealth'

const props = defineProps<{
  open: boolean
  events: ConnectionHealthEvent[]
  groups: OwnGroupHealth[]
  policies: ConnectionHealthPolicy[]
  selectedConnectionId: string
  siteName: (id: string) => string
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'view-all'): void
}>()

const { t } = useI18n()
const prefix = 'admin.connectionHealth'
const cardPrefix = `${prefix}.eventsDialog.card`

// 探活结果分类：用于近 60 次记录条着色和可用率计算分母。人工禁用/恢复不算一次探活结果，
// 不计入可用率分母，只在记录条里以中性色展示这个动作发生过。
const PROBE_RESULTS = new Set(['ok', 'network_fluctuation', 'rate_limited', 'server_error', 'auth', 'model_not_found', 'invalid_response', 'unsupported'])
const VALID_STATES = new Set<ConnectionHealthState>(['healthy', 'degraded', 'suspended', 'observing', 'recovering', 'disabled'])

interface StatusCard {
  key: string
  connectionId: string
  modelName: string
  upstreamSiteId: string
  upstreamGroupName: string
  provider: string
  state: ConnectionHealthState | ''
  latestLatencyMs: number | null
  lastProbeAt: string | null
  intervalSeconds: number | null
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
  const map = new Map<string, { ownGroupId: string; models: Map<string, { providerFamily: string; state: ConnectionHealthState; lastProbeAt: string | null }> }>()
  for (const group of props.groups) {
    for (const conn of group.connections) {
      const models = new Map<string, { providerFamily: string; state: ConnectionHealthState; lastProbeAt: string | null }>()
      for (const model of conn.models) {
        models.set(model.modelName, { providerFamily: model.providerFamily, state: model.state, lastProbeAt: model.lastProbeAt })
      }
      map.set(conn.connectionId, { ownGroupId: group.ownGroupId, models })
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
  const okCount = probeRecords.filter((r) => r.result === 'ok').length
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
    if (!eventsByModel.has(ev.modelName)) eventsByModel.set(ev.modelName, [])
    eventsByModel.get(ev.modelName)!.push(ev)
  }

  if (ctx) {
    return ctx.conn.models.map((model) => {
      const eventsDesc = eventsByModel.get(model.modelName) ?? []
      const { records, availabilityPct } = buildRecords(eventsDesc)
      return {
        key: `${connectionId}::${model.modelName}`,
        connectionId,
        modelName: model.modelName,
        upstreamSiteId: ctx.conn.upstreamSiteId,
        upstreamGroupName: ctx.conn.upstreamGroupName,
        provider: model.providerFamily,
        state: model.state,
        latestLatencyMs: eventsDesc[0]?.latencyMs ?? model.lastLatencyMs,
        lastProbeAt: model.lastProbeAt,
        intervalSeconds: matchingProbeIntervalSeconds(ctx.group.ownGroupId, model.modelName, props.policies),
        availabilityPct,
        records,
        remoteAction: eventsDesc[0]?.remoteAction ?? model.lastRemoteAction ?? '',
      }
    })
  }

  // 兜底：groups 数据还没跟上时（极少见的时序问题），退化为纯粹从事件推导，
  // 保证弹窗至少不是空的；此时无法解析 ownGroupId，探活间隔只能显示「未配置策略」。
  return Array.from(eventsByModel.entries()).map(([modelName, eventsDesc]) => {
    const latest = eventsDesc[0]
    const { records, availabilityPct } = buildRecords(eventsDesc)
    const state = VALID_STATES.has(latest.toState as ConnectionHealthState) ? (latest.toState as ConnectionHealthState) : ''
    return {
      key: `${connectionId}::${modelName}`,
      connectionId,
      modelName,
      upstreamSiteId: latest.upstreamSiteId,
      upstreamGroupName: latest.upstreamGroupName,
      provider: 'custom',
      state,
      latestLatencyMs: latest.latencyMs,
      lastProbeAt: latest.createdAt,
      intervalSeconds: null,
      availabilityPct,
      records,
      remoteAction: latest.remoteAction ?? '',
    }
  })
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
      const modelMeta = meta?.models.get(latest.modelName)
      const state: ConnectionHealthState | '' = modelMeta?.state
        ?? (VALID_STATES.has(latest.toState as ConnectionHealthState) ? (latest.toState as ConnectionHealthState) : '')
      const { records, availabilityPct } = buildRecords(eventsDesc)

      return {
        key: cardKey,
        connectionId: latest.connectionId,
        modelName: latest.modelName,
        upstreamSiteId: latest.upstreamSiteId,
        upstreamGroupName: latest.upstreamGroupName,
        provider: modelMeta?.providerFamily ?? 'custom',
        state,
        latestLatencyMs: latest.latencyMs,
        lastProbeAt: modelMeta?.lastProbeAt ?? latest.createdAt,
        intervalSeconds: matchingProbeIntervalSeconds(meta?.ownGroupId ?? '', latest.modelName, props.policies),
        availabilityPct,
        records,
        remoteAction: latest.remoteAction ?? '',
      }
    })

    return { key: groupKey, name: bucket.name, cards }
  })
})

const selectedConnectionMeta = computed(() => {
  if (!props.selectedConnectionId) return null
  const ctx = findConnectionContext(props.selectedConnectionId)
  if (!ctx) return null
  return {
    siteLabel: props.siteName(ctx.conn.upstreamSiteId),
    upstreamGroupName: ctx.conn.upstreamGroupName,
    ownGroupName: ctx.group.ownGroupName || ctx.group.ownGroupId,
  }
})

// nowMs 每秒滚动一次，仅在弹窗打开时计时，用来把「探活间隔 + 上次探活时间」换算成
// 动态的"下次探活"倒计时（而不是弹窗打开瞬间的静态快照）。
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

// 下次探活倒计时：disabled 链路不会自动探活，直接说明原因；没有探活过、或者没有命中
// 任何启用策略时都不编造时间，分别给出明确文案；否则用「上次探活时间 + 策略探活间隔」
// 算出下次时间，到期但调度还没跑到时展示"已到期"而不是负数或 0s。
const nextProbeLabel = (card: StatusCard): string => {
  if (card.state === 'disabled') return t(`${cardPrefix}.nextProbeDisabled`)
  if (!card.lastProbeAt) return t(`${cardPrefix}.nextProbeNeverProbed`)
  const lastAt = new Date(card.lastProbeAt).getTime()
  if (Number.isNaN(lastAt)) return t(`${cardPrefix}.nextProbeNeverProbed`)
  if (!card.intervalSeconds) return t(`${cardPrefix}.nextProbeNoPolicy`)
  const remainingMs = card.intervalSeconds * 1000 - (nowMs.value - lastAt)
  if (remainingMs > 0) return t(`${cardPrefix}.nextProbeIn`, { seconds: Math.ceil(remainingMs / 1000) })
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

        <div class="relative flex h-[min(760px,calc(100vh-2rem))] w-full max-w-6xl flex-col overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl">
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
                  :upstream-group-name="card.upstreamGroupName"
                  :model-name="card.modelName"
                  :provider="card.provider"
                  :state="card.state"
                  :latest-latency-ms="card.latestLatencyMs"
                  :availability-pct="card.availabilityPct"
                  :records="card.records"
                  :next-probe-text="nextProbeLabel(card)"
                  :remote-action="card.remoteAction"
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
                      :upstream-group-name="card.upstreamGroupName"
                      :model-name="card.modelName"
                      :provider="card.provider"
                      :state="card.state"
                      :latest-latency-ms="card.latestLatencyMs"
                      :availability-pct="card.availabilityPct"
                      :records="card.records"
                      :next-probe-text="nextProbeLabel(card)"
                      :remote-action="card.remoteAction"
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
