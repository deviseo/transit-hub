<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { AlertCircle, AlertTriangle, ArrowDownRight, ArrowUpRight, ChevronDown, ChevronRight, Database, Info, Loader2, RefreshCw, Table2, Wallet } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import DeltaBadge from '../components/user-activity/DeltaBadge.vue'
import ProfitTrendChart from '../components/profit/ProfitTrendChart.vue'
import SitePricingModal from '../components/dashboard/SitePricingModal.vue'
import { getProfitSummary, startProfitSync } from '../api/profit'
import { notifyCollectionStarted } from '../composables/useCollectionWatcher'
import { useUserActivityFormat } from '../composables/useUserActivityFormat'
import { formatCny } from '../utils/dashboard'
import type { ProfitInsight, ProfitPriceChange, ProfitRangeQuery, ProfitRouteRef, ProfitSummary, ProfitUnattributedRow, ProfitUpstreamRow } from '../types/profit'

// 利润分析：自己站点按路由、分组的营收 + 已登记上游站点的成本，按上游域名自动归因，看每个上游、每个分组的毛利，
// 以及上游续航、建议充值和调价影响。只读；数据由后台每小时采集完整自然日，首次需要手动开始采集。

type RangeMode = 7 | 30 | 90 | 'custom'

const { t, te } = useI18n()
const { formatCount, formatPercent, formatSignedPercent, formatDay, formatShortDay, formatRange, formatDateTime, formatRelativeTime } = useUserActivityFormat()

const presets: Array<7 | 30 | 90> = [7, 30, 90]
const LOW_RUNWAY_DAYS = 3
const POLL_INTERVAL = 5000

const businessToday = (): string => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
const shiftDay = (day: string, delta: number): string => {
  const [year, month, date] = day.split('-').map(Number)
  const shifted = new Date(Date.UTC(year, month - 1, date + delta))
  return shifted.toISOString().slice(0, 10)
}

// 利润只统计完整自然日：自定义区间最晚到昨天。
const today = businessToday()
const yesterday = shiftDay(today, -1)
const rangeMode = ref<RangeMode>(30)
const customFrom = ref(shiftDay(today, -30))
const customTo = ref(yesterday)
const summary = ref<ProfitSummary | null>(null)
const isLoading = ref(false)
const isStartingSync = ref(false)
const errorKey = ref<string | null>(null)
const showTable = ref(false)
const isPricingOpen = ref(false)
const expanded = ref<Record<string, boolean>>({})
let requestId = 0
let pollTimer: number | undefined

const placeholder = computed(() => t('admin.profit.common.placeholder'))
const errorText = (key: string): string => (te(key) ? t(key) : t('admin.profit.errors.request'))

const query = (): ProfitRangeQuery => (rangeMode.value === 'custom'
  ? { from: customFrom.value, to: customTo.value }
  : { days: rangeMode.value })

const sync = computed(() => summary.value?.sync)
const isSyncRunning = computed(() => sync.value?.status === 'running')
const synced = computed(() => summary.value?.synced === true)

const stopPolling = () => {
  if (pollTimer === undefined) return
  window.clearInterval(pollTimer)
  pollTimer = undefined
}

// 采集在后台进行：期间每 5 秒静默刷新，结束后停止。
const syncPolling = () => {
  if (!isSyncRunning.value) {
    stopPolling()
    return
  }
  if (pollTimer !== undefined) return
  pollTimer = window.setInterval(() => { void load({ quiet: true }) }, POLL_INTERVAL)
}

const load = async (options: { quiet?: boolean } = {}) => {
  const current = ++requestId
  if (!options.quiet) {
    isLoading.value = true
    errorKey.value = null
  }
  try {
    const response = await getProfitSummary(query())
    if (current !== requestId) return
    summary.value = response
    if (options.quiet) errorKey.value = null
  } catch (error) {
    if (current !== requestId) return
    errorKey.value = error instanceof Error ? error.message : 'admin.profit.errors.request'
  } finally {
    if (current === requestId) {
      isLoading.value = false
      syncPolling()
    }
  }
}

const startSync = async () => {
  if (isStartingSync.value || isSyncRunning.value) return
  isStartingSync.value = true
  errorKey.value = null
  try {
    const status = await startProfitSync()
    if (summary.value) summary.value = { ...summary.value, sync: status }
    else await load()
    syncPolling()
    notifyCollectionStarted()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.profit.errors.request'
  } finally {
    isStartingSync.value = false
  }
}

const selectPreset = (days: 7 | 30 | 90) => {
  if (rangeMode.value === days) return
  rangeMode.value = days
}

watch(rangeMode, (mode) => {
  if (mode !== 'custom') void load()
})

onMounted(() => { void load() })
onBeforeUnmount(stopPolling)

const customRangeValid = computed(() => Boolean(customFrom.value && customTo.value && customFrom.value <= customTo.value && customTo.value <= yesterday))
const applyCustomRange = () => {
  if (!customRangeValid.value) return
  rangeMode.value = 'custom'
  void load()
}

const onPricingSaved = () => { void load() }

const isAuthError = computed(() => errorKey.value === 'admin.profit.errors.authRequired' || sync.value?.errorKey === 'admin.profit.errors.authRequired')

const syncDescription = computed(() => {
  const state = sync.value
  if (!state || !state.status) return t('admin.profit.sync.never')
  const parts: string[] = []
  if (state.lastSyncedAt) parts.push(t('admin.profit.sync.lastSynced', { time: formatRelativeTime(state.lastSyncedAt) }))
  if (state.syncedFrom) parts.push(t('admin.profit.sync.coverage', { range: formatRange(state.syncedFrom, state.syncedTo) }))
  parts.push(t('admin.profit.sync.routes', { count: formatCount(state.routeCount) }, state.routeCount))
  if (state.revenueSource && te(`admin.profit.sources.${state.revenueSource}`)) parts.push(t(`admin.profit.sources.${state.revenueSource}`))
  return parts.join(' · ')
})

// 金额：负数用真正的减号放在 ¥ 前面。
const formatMoney = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return placeholder.value
  return value < 0 ? `−${formatCny(-value)}` : formatCny(value)
}
const formatSignedMoney = (value: number): string => {
  if (value > 0) return `+${formatCny(value)}`
  if (value < 0) return `−${formatCny(-value)}`
  return formatCny(0)
}
const formatMoneyAxis = (value: number): string => {
  const absolute = Math.abs(value)
  const sign = value < 0 ? '−' : ''
  if (absolute >= 10_000) return `${sign}¥${new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 }).format(absolute)}`
  return `${sign}¥${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(absolute)}`
}
const formatMargin = (value: number | null | undefined): string => (value == null ? placeholder.value : formatPercent(value))
const formatRunway = (days: number | null | undefined): string => {
  if (days == null) return placeholder.value
  if (days >= 999) return t('admin.profit.upstreams.runwayLong')
  return t('admin.profit.upstreams.runwayDays', { days: days < 10 ? days.toFixed(1) : String(Math.round(days)) })
}
const formatRate = (value: number): string => `${Number(value.toFixed(4))}x`
const profitClass = (value: number): string => (value < 0 ? 'text-red-600 dark:text-red-400' : 'text-foreground')

const totals = computed(() => summary.value?.totals)
const previous = computed(() => summary.value?.previous ?? null)
const profitDelta = computed(() => (totals.value && previous.value ? totals.value.profit - previous.value.profit : null))

const rangeLabel = computed(() => (summary.value ? formatRange(summary.value.range.from, summary.value.range.to) : ''))
const previousRangeLabel = computed(() => (summary.value ? formatRange(summary.value.previousRange.from, summary.value.previousRange.to) : ''))

const coverageNotice = computed(() => {
  const current = summary.value
  if (!current?.synced || current.complete) return null
  return t('admin.profit.coverage.partial', {
    range: formatRange(current.sync.syncedFrom, current.sync.syncedTo),
    complete: formatDay(current.sync.completeThrough),
  })
})

// 提示
const insightText = (insight: ProfitInsight): string => {
  const subject = insight.subject || placeholder.value
  const value = insight.value ?? null
  const extra = insight.extra ?? null
  switch (insight.kind) {
    case 'negativeMargin':
      return t('admin.profit.insights.negativeMargin', { subject, value: formatMoney(value == null ? null : Math.abs(value)) })
    case 'unmatchedCost':
      return t('admin.profit.insights.unmatchedCost', { subject, value: formatMoney(value) })
    case 'unregistered':
      return t('admin.profit.insights.unregistered', { subject, value: formatMoney(value) })
    case 'lowRunway':
      return t('admin.profit.insights.lowRunway', { subject, days: formatRunway(value), topUp: formatMoney(extra) })
    case 'costSpike':
      return t('admin.profit.insights.costSpike', { subject, ratio: value == null ? placeholder.value : formatPercent(value), baseline: extra == null ? placeholder.value : formatPercent(extra) })
    case 'noRate':
      return t('admin.profit.insights.noRate', { subject })
    case 'costFailed':
      return t('admin.profit.insights.costFailed', { subject, count: formatCount(value ?? 0) }, value ?? 0)
    case 'priceIncrease':
      return value == null
        ? t('admin.profit.insights.priceIncreaseUnknown', { subject, change: extra == null ? placeholder.value : formatSignedPercent(extra) })
        : t('admin.profit.insights.priceIncrease', { subject, change: extra == null ? placeholder.value : formatSignedPercent(extra), value: formatMoney(value) })
    default:
      return subject
  }
}
const insightTone = (severity: ProfitInsight['severity']): string => {
  if (severity === 'critical') return 'text-red-600 dark:text-red-400'
  if (severity === 'warning') return 'text-amber-600 dark:text-amber-400'
  return 'text-muted-foreground'
}

// 每日序列
const daily = computed(() => summary.value?.daily ?? [])
const chartDays = computed(() => daily.value.map((point) => point.day))
const chartRevenue = computed(() => daily.value.map((point) => point.revenue))
const chartCost = computed(() => daily.value.map((point) => point.cost))
const chartProfit = computed(() => daily.value.map((point) => point.profit))
const dailyMargin = (revenue: number | null, profit: number | null): string => (revenue != null && profit != null && revenue > 0 ? formatPercent(profit / revenue) : placeholder.value)

// 按上游：站点的余额、日均成本、建议充值汇总到上游一行（已删除的站点不计）；续航只按同时有余额和日均成本的站点计算。
const upstreams = computed(() => summary.value?.upstreams ?? [])
const upstreamCells = (row: ProfitUpstreamRow) => {
  let balance: number | null = null
  let topUp: number | null = null
  let runwayBalance = 0
  let runwayCost = 0
  for (const site of row.sites) {
    if (site.deleted) continue
    if (site.balance != null) balance = (balance ?? 0) + site.balance
    if (site.suggestedTopUp != null) topUp = (topUp ?? 0) + site.suggestedTopUp
    if (site.balance != null && site.avgDailyCost != null && site.avgDailyCost > 0) {
      runwayBalance += site.balance
      runwayCost += site.avgDailyCost
    }
  }
  return { balance, topUp, runway: runwayCost > 0 ? runwayBalance / runwayCost : null }
}
const isLowRunway = (days: number | null | undefined): boolean => days != null && days < LOW_RUNWAY_DAYS
const siteNames = (row: ProfitUpstreamRow): string => row.sites.map((site) => site.name || site.id).join('、')
const toggleUpstream = (key: string) => {
  expanded.value = { ...expanded.value, [key]: !expanded.value[key] }
}
const platformLabel = (platform: string): string => (platform && te(`admin.userActivity.platforms.${platform}`) ? t(`admin.userActivity.platforms.${platform}`) : placeholder.value)
const routeName = (route: ProfitRouteRef): string => route.name || (route.id ? `#${route.id}` : t('admin.profit.unattributed.unknownRoute'))
// 路由状态：sub2api 账号为 active / inactive / error，new-api 渠道为 1 启用、2 手动禁用、3 自动禁用。
const routeStatus = (route: ProfitRouteRef): string | null => {
  if (route.deleted) return t('admin.profit.upstreams.routeDeleted')
  const status = route.status.toLowerCase()
  if (status === '' || status === 'active' || status === '1' || status === 'enabled') return null
  if (status === 'error') return t('admin.profit.upstreams.routeError')
  return t('admin.profit.upstreams.routeDisabled')
}

// 按分组、未归因、调价
const groups = computed(() => summary.value?.groups ?? [])
const groupName = (name: string): string => name || t('admin.profit.groups.none')
const unattributed = computed(() => summary.value?.unattributed ?? [])
const unattributedAddress = (row: ProfitUnattributedRow): string => row.key || placeholder.value
const unattributedRoutes = (row: ProfitUnattributedRow): string => (row.routes.length === 0 ? placeholder.value : row.routes.map(routeName).join('、'))
const priceChanges = computed(() => summary.value?.priceChanges ?? [])
const multiplierChange = (change: ProfitPriceChange): number | null => (change.oldMultiplier > 0 ? change.newMultiplier / change.oldMultiplier - 1 : null)
const impactText = (change: ProfitPriceChange): string => {
  if (change.estimateStatus === 'ok' && change.dailyImpact != null) return formatSignedMoney(change.dailyImpact)
  if (change.estimateStatus === 'no_usage') return t('admin.profit.priceChanges.noUsage')
  if (change.estimateStatus === 'no_rate') return t('admin.profit.priceChanges.noRate')
  return t('admin.profit.priceChanges.failed')
}
const impactClass = (change: ProfitPriceChange): string => {
  if (change.estimateStatus !== 'ok' || change.dailyImpact == null || change.dailyImpact === 0) return 'text-muted-foreground'
  return change.dailyImpact > 0 ? 'text-red-600 dark:text-red-400' : 'text-emerald-700 dark:text-emerald-400'
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- 标题、采集状态与区间 -->
    <section class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div class="min-w-0">
          <h2 class="text-base font-semibold text-foreground">{{ t('admin.profit.title') }}</h2>
          <p class="mt-1 max-w-4xl text-sm text-muted-foreground">{{ t('admin.profit.subtitle') }}</p>
        </div>
        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" class="h-8 rounded-lg" :disabled="isLoading" @click="load()">
            <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('admin.profit.actions.refresh') }}
          </Button>
          <Button v-if="synced" size="sm" class="h-8 rounded-lg" :disabled="isStartingSync || isSyncRunning" @click="startSync">
            <Loader2 v-if="isStartingSync || isSyncRunning" class="h-4 w-4 animate-spin" />
            <Database v-else class="h-4 w-4" />
            {{ t('admin.profit.actions.sync') }}
          </Button>
        </div>
      </div>
      <p class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground" aria-live="polite">
        <span v-if="sync?.platform" class="rounded-md bg-surface px-2 py-0.5 font-medium text-foreground">{{ platformLabel(sync.platform) }}</span>
        <span v-if="isSyncRunning" class="inline-flex items-center gap-1 text-primary">
          <Loader2 class="h-3.5 w-3.5 animate-spin" />
          {{ t('admin.profit.sync.running') }}
        </span>
        <span>{{ syncDescription }}</span>
        <span aria-hidden="true">·</span>
        <span>{{ t('admin.profit.source.basis', { price: summary?.unitPrice ?? 1 }) }}</span>
        <button type="button" class="font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="isPricingOpen = true">
          {{ t('admin.profit.actions.changePrice') }}
        </button>
      </p>
      <div v-if="summary && synced" class="mt-3 flex flex-col gap-2 border-t border-border/50 pt-3 xl:flex-row xl:items-center xl:justify-between">
        <p class="min-w-0 flex-1 text-xs text-muted-foreground">
          {{ t('admin.profit.range.current', { range: rangeLabel }) }}
          <span aria-hidden="true">·</span>
          {{ t('admin.profit.range.previous', { range: previousRangeLabel }) }}
        </p>
        <div class="flex flex-wrap items-center gap-2 xl:shrink-0 xl:flex-nowrap">
          <div class="flex items-center gap-1 rounded-lg bg-surface p-1" role="group" :aria-label="t('admin.profit.range.label')">
            <button
              v-for="days in presets"
              :key="days"
              type="button"
              class="rounded-md px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="rangeMode === days ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
              :aria-pressed="rangeMode === days"
              @click="selectPreset(days)"
            >
              {{ t('admin.profit.range.lastDays', { days }) }}
            </button>
          </div>
          <div class="flex items-center gap-1.5">
            <label class="sr-only" for="profit-range-from">{{ t('admin.profit.range.from') }}</label>
            <input id="profit-range-from" v-model="customFrom" type="date" :max="yesterday" class="h-8 rounded-lg border border-border/60 bg-surface px-2 text-xs text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary">
            <span class="text-xs text-muted-foreground">~</span>
            <label class="sr-only" for="profit-range-to">{{ t('admin.profit.range.to') }}</label>
            <input id="profit-range-to" v-model="customTo" type="date" :max="yesterday" class="h-8 rounded-lg border border-border/60 bg-surface px-2 text-xs text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary">
            <Button variant="secondary" size="sm" class="h-8 rounded-lg" :class="rangeMode === 'custom' ? 'ring-1 ring-primary' : ''" :disabled="!customRangeValid || isLoading" @click="applyCustomRange">
              {{ t('admin.profit.range.apply') }}
            </Button>
          </div>
        </div>
      </div>
    </section>

    <div v-if="errorKey" class="flex items-start gap-3 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>
        {{ errorText(errorKey) }}
        <RouterLink v-if="isAuthError" to="/admin" class="ml-1 font-medium underline underline-offset-2">{{ t('admin.profit.actions.goDashboard') }}</RouterLink>
      </span>
    </div>
    <div v-else-if="sync?.status === 'failed' && sync.errorKey" class="flex items-start gap-3 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>
        {{ t('admin.profit.sync.failed', { reason: errorText(sync.errorKey) }) }}
        <RouterLink v-if="isAuthError" to="/admin" class="ml-1 font-medium underline underline-offset-2">{{ t('admin.profit.actions.goDashboard') }}</RouterLink>
      </span>
    </div>
    <div v-else-if="sync?.status === 'interrupted'" class="flex items-start gap-3 rounded-lg border border-primary/20 bg-primary/5 p-3 text-sm text-foreground" role="status">
      <Info class="mt-0.5 h-4 w-4 shrink-0 text-primary" />
      <span>{{ t('admin.profit.errors.interrupted') }}</span>
    </div>
    <div v-if="sync?.warningKey && sync.status === 'success'" class="flex items-start gap-3 rounded-lg border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300" role="status">
      <Info class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ errorText(sync.warningKey) }}</span>
    </div>

    <div v-if="isLoading && !summary" class="flex items-center justify-center rounded-lg border border-border/50 bg-card py-16 text-muted-foreground">
      <Loader2 class="h-5 w-5 animate-spin" />
    </div>

    <!-- 首次采集 -->
    <section v-else-if="summary && !synced" class="flex flex-col items-center rounded-lg border border-dashed border-border/70 bg-card px-6 py-12 text-center">
      <Database class="mb-3 h-10 w-10 text-muted-foreground/60" />
      <h2 class="text-base font-semibold text-foreground">{{ isSyncRunning ? t('admin.profit.empty.runningTitle') : t('admin.profit.empty.title') }}</h2>
      <p class="mt-2 max-w-2xl text-sm text-muted-foreground">{{ isSyncRunning ? t('admin.profit.empty.runningDescription') : t('admin.profit.empty.description') }}</p>
      <p v-if="!isSyncRunning" class="mt-2 max-w-2xl text-xs text-muted-foreground">{{ t('admin.profit.empty.requirements') }}</p>
      <Button v-if="!isSyncRunning" class="mt-4 rounded-lg" :disabled="isStartingSync" @click="startSync">
        <Loader2 v-if="isStartingSync" class="h-4 w-4 animate-spin" />
        <Database v-else class="h-4 w-4" />
        {{ t('admin.profit.actions.startSync') }}
      </Button>
    </section>

    <template v-else-if="summary && totals">
      <div v-if="coverageNotice" class="flex items-start gap-3 rounded-lg border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300" role="status">
        <Info class="mt-0.5 h-4 w-4 shrink-0" />
        <span>{{ coverageNotice }}</span>
      </div>

      <!-- 核心指标 -->
      <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5" :class="{ 'opacity-60 transition-opacity': isLoading }">
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.profit.kpi.revenue') }}</p>
          <p class="mt-1 flex flex-wrap items-baseline gap-2">
            <span class="text-2xl font-semibold text-foreground">{{ formatMoney(totals.revenue) }}</span>
            <DeltaBadge :current="totals.revenue" :previous="previous?.revenue" :missing-hint="t('admin.profit.coverage.previousMissing')" />
          </p>
          <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.profit.kpi.revenueHint') }}</p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.profit.kpi.cost') }}</p>
          <p class="mt-1 flex flex-wrap items-baseline gap-2">
            <span class="text-2xl font-semibold text-foreground">{{ formatMoney(totals.cost) }}</span>
            <DeltaBadge :current="totals.cost" :previous="previous?.cost" :up-is-good="false" :missing-hint="t('admin.profit.coverage.previousMissing')" />
          </p>
          <p class="mt-1 text-xs" :class="totals.failedSiteDays > 0 ? 'text-amber-700 dark:text-amber-300' : 'text-muted-foreground'">
            {{ totals.failedSiteDays > 0 ? t('admin.profit.kpi.costFailedHint', { count: formatCount(totals.failedSiteDays) }, totals.failedSiteDays) : t('admin.profit.kpi.costHint') }}
          </p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.profit.kpi.profit') }}</p>
          <p class="mt-1 flex flex-wrap items-baseline gap-2">
            <span class="text-2xl font-semibold" :class="profitClass(totals.profit)">{{ formatMoney(totals.profit) }}</span>
            <span
              v-if="profitDelta != null"
              class="inline-flex items-center gap-0.5 whitespace-nowrap text-xs font-medium tabular-nums"
              :class="profitDelta > 0 ? 'text-emerald-700 dark:text-emerald-400' : profitDelta < 0 ? 'text-red-600 dark:text-red-400' : 'text-muted-foreground'"
            >
              <ArrowUpRight v-if="profitDelta > 0" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
              <ArrowDownRight v-else-if="profitDelta < 0" class="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
              {{ t('admin.profit.kpi.profitDelta', { value: formatSignedMoney(profitDelta) }) }}
            </span>
            <span v-else class="text-xs text-muted-foreground" :title="t('admin.profit.coverage.previousMissing')">{{ placeholder }}</span>
          </p>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ t('admin.profit.kpi.profitHint', { margin: formatMargin(totals.margin) }) }}
            <template v-if="previous">{{ t('admin.profit.kpi.previousMargin', { margin: formatMargin(previous.margin) }) }}</template>
          </p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.profit.kpi.attributed') }}</p>
          <p class="mt-1 text-2xl font-semibold text-foreground">{{ formatMargin(totals.attributedShare) }}</p>
          <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.profit.kpi.attributedHint', { value: formatMoney(totals.unattributedRevenue) }) }}</p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.profit.kpi.unmatched') }}</p>
          <p class="mt-1 text-2xl font-semibold" :class="totals.unmatchedCost > 0 ? 'text-amber-700 dark:text-amber-300' : 'text-foreground'">{{ formatMoney(totals.unmatchedCost) }}</p>
          <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.profit.kpi.unmatchedHint') }}</p>
        </article>
      </section>

      <!-- 需要关注 -->
      <section v-if="summary.insights.length > 0" class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
        <h3 class="text-sm font-semibold text-foreground">{{ t('admin.profit.insights.title') }}</h3>
        <ul class="mt-2 space-y-1.5">
          <li v-for="(insight, index) in summary.insights" :key="`${insight.kind}-${insight.subject}-${index}`" class="flex items-start gap-2 text-sm">
            <AlertTriangle v-if="insight.severity === 'critical'" class="mt-0.5 h-4 w-4 shrink-0" :class="insightTone(insight.severity)" aria-hidden="true" />
            <AlertCircle v-else-if="insight.severity === 'warning'" class="mt-0.5 h-4 w-4 shrink-0" :class="insightTone(insight.severity)" aria-hidden="true" />
            <Info v-else class="mt-0.5 h-4 w-4 shrink-0" :class="insightTone(insight.severity)" aria-hidden="true" />
            <span class="min-w-0 text-foreground">
              <span class="mr-1 text-xs font-medium" :class="insightTone(insight.severity)">{{ t(`admin.profit.insights.severity.${insight.severity}`) }}</span>
              {{ insightText(insight) }}
            </span>
          </li>
        </ul>
      </section>

      <!-- 每日营收、成本与毛利 -->
      <section class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold text-foreground">{{ t('admin.profit.trend.title') }}</h3>
            <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.profit.trend.subtitle') }}</p>
          </div>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :aria-pressed="showTable"
            @click="showTable = !showTable"
          >
            <Table2 class="h-3.5 w-3.5" />
            {{ showTable ? t('admin.profit.actions.showChart') : t('admin.profit.actions.showTable') }}
          </button>
        </div>
        <div v-if="!showTable" class="mt-3 h-72" :class="{ 'opacity-60 transition-opacity': isLoading }">
          <ProfitTrendChart
            :days="chartDays"
            :revenue="chartRevenue"
            :cost="chartCost"
            :profit="chartProfit"
            :revenue-label="t('admin.profit.trend.revenue')"
            :cost-label="t('admin.profit.trend.cost')"
            :profit-label="t('admin.profit.trend.profit')"
            :margin-label="t('admin.profit.trend.margin')"
            :no-data-label="t('admin.profit.trend.noData')"
            :accessible-label="t('admin.profit.trend.aria', { range: rangeLabel })"
            :format-value="(value) => formatMoney(value)"
            :format-axis="formatMoneyAxis"
            :format-percent="formatPercent"
            :format-day="formatDay"
            :format-tick="formatShortDay"
          />
        </div>
        <div v-else class="mt-3 max-h-96 overflow-auto rounded-lg border border-border/50">
          <table class="w-full min-w-[560px] text-left text-sm">
            <thead class="sticky top-0 bg-surface-elevated text-xs text-muted-foreground">
              <tr>
                <th class="px-3 py-2 font-medium">{{ t('admin.profit.trend.day') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.profit.trend.revenue') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.profit.trend.cost') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.profit.trend.profit') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.profit.trend.margin') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50 tabular-nums">
              <tr v-for="point in daily" :key="point.day">
                <td class="px-3 py-2 text-foreground">{{ formatDay(point.day) }}</td>
                <template v-if="point.revenue != null && point.cost != null && point.profit != null">
                  <td class="px-3 py-2 text-right text-foreground">{{ formatMoney(point.revenue) }}</td>
                  <td class="px-3 py-2 text-right text-muted-foreground">{{ formatMoney(point.cost) }}</td>
                  <td class="px-3 py-2 text-right font-medium" :class="profitClass(point.profit)">{{ formatMoney(point.profit) }}</td>
                  <td class="px-3 py-2 text-right text-muted-foreground">{{ dailyMargin(point.revenue, point.profit) }}</td>
                </template>
                <td v-else colspan="4" class="px-3 py-2 text-right text-muted-foreground">{{ t('admin.profit.trend.noData') }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- 按上游 -->
      <section class="rounded-lg border border-border/50 bg-card shadow-sm">
        <div class="flex flex-wrap items-baseline justify-between gap-2 border-b border-border/50 p-4">
          <h3 class="text-sm font-semibold text-foreground">{{ t('admin.profit.upstreams.title') }}</h3>
          <p class="text-xs text-muted-foreground">{{ t('admin.profit.upstreams.subtitle') }}</p>
        </div>
        <div v-if="upstreams.length === 0" class="py-10 text-center text-sm text-muted-foreground">
          {{ t('admin.profit.upstreams.empty') }}
          <RouterLink to="/admin/upstream" class="ml-1 font-medium text-primary hover:underline">{{ t('admin.profit.actions.goUpstream') }}</RouterLink>
        </div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[980px] text-left text-sm">
            <thead class="bg-surface-elevated/60 text-xs text-muted-foreground">
              <tr>
                <th class="px-4 py-2 font-medium">{{ t('admin.profit.upstreams.upstream') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.profit.upstreams.revenue') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.profit.upstreams.cost') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.profit.upstreams.profit') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.profit.upstreams.margin') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.profit.upstreams.balance') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.profit.upstreams.runway') }}</th>
                <th class="px-4 py-2 text-right font-medium" :title="t('admin.profit.upstreams.topUpHint')">{{ t('admin.profit.upstreams.topUp') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50 tabular-nums">
              <template v-for="row in upstreams" :key="row.key">
                <tr class="transition-colors hover:bg-surface/40">
                  <td class="max-w-[22rem] px-4 py-2">
                    <button
                      type="button"
                      class="flex max-w-full items-start gap-1.5 text-left focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                      :aria-expanded="Boolean(expanded[row.key])"
                      :aria-label="expanded[row.key] ? t('admin.profit.actions.collapse', { name: row.key }) : t('admin.profit.actions.expand', { name: row.key })"
                      @click="toggleUpstream(row.key)"
                    >
                      <ChevronDown v-if="expanded[row.key]" class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                      <ChevronRight v-else class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                      <span class="min-w-0">
                        <span class="block truncate font-medium text-foreground">{{ row.key }}</span>
                        <span class="block truncate text-xs text-muted-foreground">
                          {{ siteNames(row) }}
                          <span aria-hidden="true">·</span>
                          {{ row.routes.length > 0 ? t('admin.profit.upstreams.routes', { count: formatCount(row.routes.length) }, row.routes.length) : t('admin.profit.upstreams.noRoutes') }}
                        </span>
                      </span>
                    </button>
                  </td>
                  <td class="px-4 py-2 text-right text-foreground">{{ formatMoney(row.revenue) }}</td>
                  <td class="px-4 py-2 text-right text-muted-foreground">{{ formatMoney(row.cost) }}</td>
                  <td class="px-4 py-2 text-right font-semibold" :class="row.revenue > 0 ? profitClass(row.profit) : 'text-muted-foreground'">{{ row.revenue > 0 ? formatMoney(row.profit) : placeholder }}</td>
                  <td class="px-4 py-2 text-right text-muted-foreground">{{ formatMargin(row.margin) }}</td>
                  <td class="px-4 py-2 text-right text-muted-foreground">{{ formatMoney(upstreamCells(row).balance) }}</td>
                  <td class="px-4 py-2 text-right">
                    <span class="inline-flex items-center gap-1" :class="isLowRunway(upstreamCells(row).runway) ? 'font-medium text-red-600 dark:text-red-400' : 'text-muted-foreground'">
                      <AlertTriangle v-if="isLowRunway(upstreamCells(row).runway)" class="h-3.5 w-3.5" aria-hidden="true" />
                      {{ formatRunway(upstreamCells(row).runway) }}
                    </span>
                  </td>
                  <td class="px-4 py-2 text-right" :class="(upstreamCells(row).topUp ?? 0) > 0 ? 'font-medium text-foreground' : 'text-muted-foreground'">{{ formatMoney(upstreamCells(row).topUp) }}</td>
                </tr>
                <tr v-if="expanded[row.key]" class="bg-surface/30">
                  <td colspan="8" class="px-4 py-3">
                    <div class="grid gap-4 xl:grid-cols-2">
                      <div>
                        <h4 class="text-xs font-medium text-muted-foreground">{{ t('admin.profit.upstreams.sites') }}</h4>
                        <table class="mt-1.5 w-full text-left text-xs">
                          <thead class="text-muted-foreground">
                            <tr>
                              <th class="py-1 pr-2 font-medium">{{ t('admin.profit.upstreams.site') }}</th>
                              <th class="py-1 pr-2 text-right font-medium">{{ t('admin.profit.upstreams.rechargeRate') }}</th>
                              <th class="py-1 pr-2 text-right font-medium">{{ t('admin.profit.upstreams.rangeCost') }}</th>
                              <th class="py-1 pr-2 text-right font-medium">{{ t('admin.profit.upstreams.balance') }}</th>
                              <th class="py-1 pr-2 text-right font-medium">{{ t('admin.profit.upstreams.avgDailyCost') }}</th>
                              <th class="py-1 pr-2 text-right font-medium">{{ t('admin.profit.upstreams.runway') }}</th>
                              <th class="py-1 text-right font-medium">{{ t('admin.profit.upstreams.topUp') }}</th>
                            </tr>
                          </thead>
                          <tbody class="divide-y divide-border/40">
                            <tr v-for="site in row.sites" :key="site.id">
                              <td class="py-1.5 pr-2">
                                <span class="font-medium text-foreground">{{ site.name || site.id }}</span>
                                <span class="ml-1 text-muted-foreground">{{ platformLabel(site.platform) }}</span>
                                <span class="mt-0.5 flex flex-wrap gap-1">
                                  <span v-if="site.deleted" class="rounded bg-surface px-1.5 py-0.5 text-[11px] text-muted-foreground">{{ t('admin.profit.upstreams.deleted') }}</span>
                                  <span v-if="site.noRate && !site.deleted" class="rounded bg-amber-500/10 px-1.5 py-0.5 text-[11px] text-amber-700 dark:text-amber-300">{{ t('admin.profit.upstreams.noRate') }}</span>
                                  <span v-if="site.failedDays > 0" class="rounded bg-amber-500/10 px-1.5 py-0.5 text-[11px] text-amber-700 dark:text-amber-300">{{ t('admin.profit.upstreams.failedDays', { count: formatCount(site.failedDays) }, site.failedDays) }}</span>
                                </span>
                              </td>
                              <td class="py-1.5 pr-2 text-right text-muted-foreground">{{ site.rechargeRate > 0 ? site.rechargeRate : placeholder }}</td>
                              <td class="py-1.5 pr-2 text-right text-foreground">{{ formatMoney(site.cost) }}</td>
                              <td class="py-1.5 pr-2 text-right text-muted-foreground">{{ formatMoney(site.balance) }}</td>
                              <td class="py-1.5 pr-2 text-right text-muted-foreground">{{ formatMoney(site.avgDailyCost) }}</td>
                              <td class="py-1.5 pr-2 text-right" :class="isLowRunway(site.runwayDays) ? 'font-medium text-red-600 dark:text-red-400' : 'text-muted-foreground'">{{ formatRunway(site.runwayDays) }}</td>
                              <td class="py-1.5 text-right text-muted-foreground">{{ formatMoney(site.suggestedTopUp) }}</td>
                            </tr>
                          </tbody>
                        </table>
                      </div>
                      <div>
                        <h4 class="text-xs font-medium text-muted-foreground">{{ t('admin.profit.upstreams.routeList') }}</h4>
                        <p v-if="row.routes.length === 0" class="mt-1.5 text-xs text-muted-foreground">{{ t('admin.profit.upstreams.noRoutesHint') }}</p>
                        <table v-else class="mt-1.5 w-full text-left text-xs">
                          <thead class="text-muted-foreground">
                            <tr>
                              <th class="py-1 pr-2 font-medium">{{ t('admin.profit.upstreams.route') }}</th>
                              <th class="py-1 pr-2 font-medium">{{ t('admin.profit.upstreams.groups') }}</th>
                              <th class="py-1 text-right font-medium">{{ t('admin.profit.upstreams.routeRevenue') }}</th>
                            </tr>
                          </thead>
                          <tbody class="divide-y divide-border/40">
                            <tr v-for="route in row.routes" :key="route.id">
                              <td class="py-1.5 pr-2">
                                <span class="font-medium text-foreground">{{ routeName(route) }}</span>
                                <span v-if="routeStatus(route)" class="ml-1 rounded bg-surface px-1.5 py-0.5 text-[11px] text-muted-foreground">{{ routeStatus(route) }}</span>
                              </td>
                              <td class="py-1.5 pr-2">
                                <span class="flex flex-wrap gap-1">
                                  <span v-for="group in route.groups" :key="group" class="rounded bg-surface px-1.5 py-0.5 text-[11px] text-muted-foreground">{{ group }}</span>
                                  <span v-if="route.groups.length === 0" class="text-muted-foreground">{{ placeholder }}</span>
                                </span>
                              </td>
                              <td class="py-1.5 text-right text-foreground">{{ formatMoney(route.revenue) }}</td>
                            </tr>
                          </tbody>
                        </table>
                      </div>
                    </div>
                  </td>
                </tr>
              </template>
            </tbody>
          </table>
        </div>
      </section>

      <section class="grid gap-3">
        <!-- 按分组 -->
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <h3 class="text-sm font-semibold text-foreground">{{ t('admin.profit.groups.title') }}</h3>
          <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.profit.groups.subtitle') }}</p>
          <div v-if="groups.length === 0" class="py-10 text-center text-sm text-muted-foreground">{{ t('admin.profit.groups.empty') }}</div>
          <div v-else class="mt-3 overflow-x-auto">
            <table class="w-full min-w-[560px] text-left text-sm">
              <thead class="text-xs text-muted-foreground">
                <tr>
                  <th class="py-2 pr-2 font-medium">{{ t('admin.profit.groups.group') }}</th>
                  <th class="py-2 pr-2 text-right font-medium">{{ t('admin.profit.groups.revenue') }}</th>
                  <th class="py-2 pr-2 text-right font-medium">{{ t('admin.profit.groups.cost') }}</th>
                  <th class="py-2 pr-2 text-right font-medium">{{ t('admin.profit.groups.profit') }}</th>
                  <th class="py-2 pr-2 text-right font-medium">{{ t('admin.profit.groups.margin') }}</th>
                  <th class="py-2 text-right font-medium">{{ t('admin.profit.groups.unattributed') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border/50 tabular-nums">
                <tr v-for="row in groups" :key="row.group">
                  <td class="py-2 pr-2 text-foreground">{{ groupName(row.group) }}</td>
                  <td class="py-2 pr-2 text-right text-foreground">{{ formatMoney(row.revenue) }}</td>
                  <td class="py-2 pr-2 text-right text-muted-foreground">{{ formatMoney(row.cost) }}</td>
                  <td class="py-2 pr-2 text-right font-medium" :class="row.attributedRevenue > 0 ? profitClass(row.profit) : 'text-muted-foreground'">{{ row.attributedRevenue > 0 ? formatMoney(row.profit) : placeholder }}</td>
                  <td class="py-2 pr-2 text-right text-muted-foreground">{{ formatMargin(row.margin) }}</td>
                  <td class="py-2 text-right text-muted-foreground">{{ row.unattributedRevenue > 0 ? formatMoney(row.unattributedRevenue) : placeholder }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </article>

        <!-- 未归因营收 -->
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <h3 class="text-sm font-semibold text-foreground">{{ t('admin.profit.unattributed.title') }}</h3>
          <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.profit.unattributed.subtitle') }}</p>
          <div v-if="unattributed.length === 0" class="py-10 text-center text-sm text-muted-foreground">{{ t('admin.profit.unattributed.empty') }}</div>
          <div v-else class="mt-3 overflow-x-auto">
            <table class="w-full min-w-[480px] text-left text-sm">
              <thead class="text-xs text-muted-foreground">
                <tr>
                  <th class="py-2 pr-2 font-medium">{{ t('admin.profit.unattributed.category') }}</th>
                  <th class="py-2 pr-2 font-medium">{{ t('admin.profit.unattributed.address') }}</th>
                  <th class="py-2 pr-2 font-medium">{{ t('admin.profit.unattributed.routes') }}</th>
                  <th class="py-2 text-right font-medium">{{ t('admin.profit.unattributed.revenue') }}</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-border/50 tabular-nums">
                <tr v-for="row in unattributed" :key="`${row.category}|${row.key}`">
                  <td class="py-2 pr-2 text-foreground">{{ t(`admin.profit.unattributed.categories.${row.category}`) }}</td>
                  <td class="py-2 pr-2 text-muted-foreground">
                    {{ unattributedAddress(row) }}
                    <RouterLink v-if="row.category === 'unregistered'" to="/admin/upstream" class="ml-1 text-xs font-medium text-primary hover:underline">{{ t('admin.profit.actions.goUpstream') }}</RouterLink>
                  </td>
                  <td class="max-w-[14rem] truncate py-2 pr-2 text-muted-foreground" :title="unattributedRoutes(row)">{{ unattributedRoutes(row) }}</td>
                  <td class="py-2 text-right font-medium text-foreground">{{ formatMoney(row.revenue) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </article>
      </section>

      <!-- 上游调价 -->
      <section class="rounded-lg border border-border/50 bg-card shadow-sm">
        <div class="flex flex-wrap items-baseline justify-between gap-2 border-b border-border/50 p-4">
          <h3 class="text-sm font-semibold text-foreground">{{ t('admin.profit.priceChanges.title') }}</h3>
          <p class="text-xs text-muted-foreground">{{ t('admin.profit.priceChanges.subtitle') }}</p>
        </div>
        <div v-if="priceChanges.length === 0" class="py-10 text-center text-sm text-muted-foreground">{{ t('admin.profit.priceChanges.empty') }}</div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[820px] text-left text-sm">
            <thead class="bg-surface-elevated/60 text-xs text-muted-foreground">
              <tr>
                <th class="px-4 py-2 font-medium">{{ t('admin.profit.priceChanges.detectedAt') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.profit.priceChanges.site') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.profit.priceChanges.group') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.profit.priceChanges.multiplier') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.profit.priceChanges.avgDailyCost') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.profit.priceChanges.impact') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50 tabular-nums">
              <tr v-for="change in priceChanges" :key="change.id">
                <td class="px-4 py-2 text-muted-foreground">{{ formatDateTime(change.detectedAt) }}</td>
                <td class="px-4 py-2">
                  <span class="font-medium text-foreground">{{ change.siteName || change.siteId }}</span>
                  <span v-if="change.upstreamKey" class="block text-xs text-muted-foreground">{{ change.upstreamKey }}</span>
                </td>
                <td class="px-4 py-2 text-foreground">{{ change.groupName || placeholder }}</td>
                <td class="px-4 py-2">
                  <span class="inline-flex items-center gap-1 text-foreground">
                    {{ formatRate(change.oldMultiplier) }} → {{ formatRate(change.newMultiplier) }}
                    <span
                      v-if="multiplierChange(change) != null"
                      class="inline-flex items-center text-xs font-medium"
                      :class="(multiplierChange(change) ?? 0) > 0 ? 'text-red-600 dark:text-red-400' : 'text-emerald-700 dark:text-emerald-400'"
                    >
                      <ArrowUpRight v-if="(multiplierChange(change) ?? 0) > 0" class="h-3.5 w-3.5" aria-hidden="true" />
                      <ArrowDownRight v-else class="h-3.5 w-3.5" aria-hidden="true" />
                      {{ formatSignedPercent(multiplierChange(change) ?? 0) }}
                    </span>
                  </span>
                </td>
                <td class="px-4 py-2 text-right text-muted-foreground">{{ change.estimateStatus === 'ok' || change.estimateStatus === 'no_usage' ? formatMoney(change.avgDailyCost) : placeholder }}</td>
                <td class="px-4 py-2 text-right font-medium" :class="impactClass(change)">{{ impactText(change) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <p class="flex items-start gap-2 px-1 text-xs text-muted-foreground">
        <Wallet class="mt-0.5 h-3.5 w-3.5 shrink-0" />
        <span>{{ t('admin.profit.footnote') }}</span>
      </p>
    </template>

    <SitePricingModal :open="isPricingOpen" @close="isPricingOpen = false" @saved="onPricingSaved" />
  </div>
</template>
