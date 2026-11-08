<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { AlertCircle, BadgeDollarSign, Database, Info, ListFilter, Loader2, RefreshCw, Table2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import DeltaBadge from '../components/user-activity/DeltaBadge.vue'
import SalesEntriesDrawer from '../components/sales/SalesEntriesDrawer.vue'
import SalesTrendChart from '../components/sales/SalesTrendChart.vue'
import SitePricingModal from '../components/dashboard/SitePricingModal.vue'
import UserActivityDetailDrawer from '../components/user-activity/UserActivityDetailDrawer.vue'
import { getSalesSummary, setSalesExcludedChannels } from '../api/sales'
import { useUserActivityFormat } from '../composables/useUserActivityFormat'
import { formatCny } from '../utils/dashboard'
import type { SalesChannelTotal, SalesDistributionBucket, SalesRangeQuery, SalesSummary } from '../types/sales'

// 销售分析：入账结构、付费用户、转化与复购、收入集中度和入账排行。数据来自「用户分析」同步到本地的数据，只读；
// 金额按「我的站点售价」换算为人民币，与仪表盘营收口径一致。

type RangeMode = 7 | 30 | 90 | 'custom'

const { t, te } = useI18n()
const { formatCount, formatPercent, formatDay, formatShortDay, formatRange, formatRelativeTime } = useUserActivityFormat()

const presets: Array<7 | 30 | 90> = [7, 30, 90]
// 渠道的固定顺序（也是图表颜色槽位的顺序），颜色跟着渠道走，不随金额排名变化。
const CHANNEL_ORDER = ['payment', 'credit', 'redeem', 'admin_add', 'affiliate']
// 可以选择不计入的渠道（正向入账；扣减本来就不计入入账合计）。
const COUNTABLE = new Set(CHANNEL_ORDER)

const businessToday = (): string => new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
const shiftDay = (day: string, delta: number): string => {
  const [year, month, date] = day.split('-').map(Number)
  const shifted = new Date(Date.UTC(year, month - 1, date + delta))
  return shifted.toISOString().slice(0, 10)
}

const today = businessToday()
const rangeMode = ref<RangeMode>(30)
const customFrom = ref(shiftDay(today, -30))
const customTo = ref(shiftDay(today, -1))
const summary = ref<SalesSummary | null>(null)
const isLoading = ref(false)
const errorKey = ref<string | null>(null)
const showTable = ref(false)
const isPricingOpen = ref(false)
const detailUserId = ref<string | null>(null)
// 入账明细抽屉：打开时的筛选。
const entries = ref<{ channel?: string, excludedOnly?: boolean } | null>(null)
const savingChannel = ref('')
const channelError = ref<string | null>(null)
let requestId = 0

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.sales.errors.request'))

const query = (): SalesRangeQuery => (rangeMode.value === 'custom'
  ? { from: customFrom.value, to: customTo.value }
  : { days: rangeMode.value })

const load = async () => {
  const current = ++requestId
  isLoading.value = true
  errorKey.value = null
  try {
    const response = await getSalesSummary(query())
    if (current !== requestId) return
    summary.value = response
  } catch (error) {
    if (current !== requestId) return
    errorKey.value = error instanceof Error ? error.message : 'admin.sales.errors.request'
  } finally {
    if (current === requestId) isLoading.value = false
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

const formatMoney = (value: number | null | undefined): string => formatCny(value ?? null)
const formatMoneyAxis = (value: number): string => {
  const absolute = Math.abs(value)
  if (absolute >= 10_000) return `¥${new Intl.NumberFormat('zh-CN', { notation: 'compact', maximumFractionDigits: 1 }).format(value)}`
  return `¥${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(value)}`
}
const formatSignedMoney = (value: number): string => {
  const text = formatCny(Math.abs(value))
  if (value > 0) return `+${text}`
  if (value < 0) return `−${text}`
  return text
}

const coverage = computed(() => summary.value?.coverage)
const synced = computed(() => coverage.value?.synced === true)
const current = computed(() => summary.value?.current)
const previous = computed(() => summary.value?.previous ?? null)

const channelLabel = (channel: string): string => {
  const key = `admin.userActivity.money.channels.${channel}`
  return te(key) ? t(key) : channel
}

const arppu = (totals: { credits: number, payers: number } | null | undefined): number | null => {
  if (!totals) return null
  return totals.payers > 0 ? totals.credits / totals.payers : 0
}

const balanceChange = computed(() => {
  const totals = current.value
  if (!totals || totals.balanceStart == null || totals.balanceEnd == null) return null
  return totals.balanceEnd - totals.balanceStart
})

const creditToConsumption = computed(() => {
  const totals = current.value
  if (!totals || totals.consumption == null || totals.consumption <= 0) return null
  return totals.credits / totals.consumption
})

const conversionRate = computed(() => {
  const totals = current.value
  if (!totals || totals.registered === 0) return null
  return totals.registeredPaid / totals.registered
})

const repeatRate = computed(() => {
  const totals = current.value
  if (!totals || totals.payers === 0) return null
  return totals.repeatPayers / totals.payers
})

const daily = computed(() => summary.value?.daily ?? [])
const chartDays = computed(() => daily.value.map((point) => point.day))
const chartChannels = computed(() => {
  const present = new Set<string>()
  for (const item of summary.value?.channels ?? []) {
    if (item.amount > 0) present.add(item.channel)
  }
  for (const point of daily.value) {
    for (const [channel, amount] of Object.entries(point.channels)) {
      if (amount > 0) present.add(channel)
    }
  }
  return [...CHANNEL_ORDER.filter((channel) => present.has(channel)), ...[...present].filter((channel) => !CHANNEL_ORDER.includes(channel)).sort()]
})
const chartSeries = computed(() => {
  const series: Record<string, Array<number | null>> = {}
  for (const channel of chartChannels.value) {
    series[channel] = daily.value.map((point) => (point.credits == null ? null : (point.channels[channel] ?? 0)))
  }
  return series
})
const chartConsumption = computed(() => daily.value.map((point) => point.consumption))

// 明细全部不计入（金额变成 0）的渠道也留着，方便恢复。
const positiveChannels = computed(() => (summary.value?.channels ?? []).filter((item) => item.amount > 0 || (item.excludedAmount ?? 0) > 0))
const negativeChannels = computed(() => (summary.value?.channels ?? []).filter((item) => item.amount < 0))
const channelShare = (item: SalesChannelTotal): number => {
  const total = current.value?.credits ?? 0
  return total > 0 && item.amount > 0 && !item.excluded ? item.amount / total : 0
}
const exclusions = computed(() => summary.value?.exclusions ?? { channels: [], entries: 0, amount: 0 })
const hasExclusions = computed(() => exclusions.value.amount > 0 || exclusions.value.channels.length > 0)

// 整个渠道计入或不计入：保存后重新算。
const toggleChannel = async (channel: string, counted: boolean) => {
  const next = new Set(exclusions.value.channels)
  if (counted) next.delete(channel)
  else next.add(channel)
  savingChannel.value = channel
  channelError.value = null
  try {
    await setSalesExcludedChannels([...next])
    await load()
  } catch (error) {
    channelError.value = error instanceof Error ? error.message : 'admin.sales.errors.request'
  } finally {
    savingChannel.value = ''
  }
}

const openEntries = (channel?: string) => {
  entries.value = channel ? { channel } : { excludedOnly: exclusions.value.entries > 0 }
}

const distribution = computed(() => summary.value?.distribution ?? [])
const maxBucketUsers = computed(() => Math.max(1, ...distribution.value.map((bucket) => bucket.users)))
// 分档边界是整数元，不显示小数。
const formatWholeYuan = (value: number): string => `¥${new Intl.NumberFormat('zh-CN', { maximumFractionDigits: 0 }).format(value)}`
const bucketLabel = (bucket: SalesDistributionBucket): string => (bucket.max == null
  ? t('admin.sales.conversion.bucketOpen', { min: formatWholeYuan(bucket.min) })
  : t('admin.sales.conversion.bucket', { min: formatWholeYuan(bucket.min), max: formatWholeYuan(bucket.max) }))

const rangeLabel = computed(() => (summary.value ? formatRange(summary.value.range.from, summary.value.range.to) : ''))
const previousRangeLabel = computed(() => (summary.value ? formatRange(summary.value.previousRange.from, summary.value.previousRange.to) : ''))

const coverageNotice = computed(() => {
  const cov = coverage.value
  if (!cov?.synced) return null
  if (cov.method === 'snapshot') return t('admin.sales.coverage.snapshot', { from: formatDay(cov.rechargeFrom) })
  if (!cov.rechargeComplete) return t('admin.sales.coverage.partial', { from: formatDay(cov.rechargeFrom) })
  return null
})

const payerName = (row: { email: string, username: string, siteUserId: string }): string => row.email || row.username || `#${row.siteUserId}`

const customRangeValid = computed(() => Boolean(customFrom.value && customTo.value && customFrom.value <= customTo.value && customTo.value <= today))
const applyCustomRange = () => {
  if (!customRangeValid.value) return
  rangeMode.value = 'custom'
  void load()
}

const onPricingSaved = () => { void load() }
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- 标题与区间 -->
    <section class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
      <div class="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
        <div class="min-w-0">
          <h2 class="text-base font-semibold text-foreground">{{ t('admin.sales.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.sales.subtitle') }}</p>
          <p class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <span v-if="coverage?.platform" class="rounded-md bg-surface px-2 py-0.5 font-medium text-foreground">{{ t(`admin.userActivity.platforms.${coverage.platform}`) }}</span>
            <span>{{ coverage?.lastSyncedAt ? t('admin.sales.source.lastSynced', { time: formatRelativeTime(coverage.lastSyncedAt) }) : t('admin.sales.source.neverSynced') }}</span>
            <span aria-hidden="true">·</span>
            <span>{{ t('admin.sales.source.basis', { price: summary?.unitPrice ?? 1 }) }}</span>
            <button type="button" class="font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="isPricingOpen = true">
              {{ t('admin.sales.actions.changePrice') }}
            </button>
          </p>
        </div>
        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <div class="flex items-center gap-1 rounded-lg bg-surface p-1" role="group" :aria-label="t('admin.sales.range.label')">
            <button
              v-for="days in presets"
              :key="days"
              type="button"
              class="rounded-md px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="rangeMode === days ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
              :aria-pressed="rangeMode === days"
              @click="selectPreset(days)"
            >
              {{ t('admin.sales.range.lastDays', { days }) }}
            </button>
          </div>
          <div class="flex items-center gap-1.5">
            <label class="sr-only" for="sales-range-from">{{ t('admin.sales.range.from') }}</label>
            <input id="sales-range-from" v-model="customFrom" type="date" :max="today" class="h-8 rounded-lg border border-border/60 bg-surface px-2 text-xs text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary">
            <span class="text-xs text-muted-foreground">~</span>
            <label class="sr-only" for="sales-range-to">{{ t('admin.sales.range.to') }}</label>
            <input id="sales-range-to" v-model="customTo" type="date" :max="today" class="h-8 rounded-lg border border-border/60 bg-surface px-2 text-xs text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary">
            <Button variant="secondary" size="sm" class="h-8 rounded-lg" :class="rangeMode === 'custom' ? 'ring-1 ring-primary' : ''" :disabled="!customRangeValid || isLoading" @click="applyCustomRange">
              {{ t('admin.sales.range.apply') }}
            </Button>
          </div>
          <Button variant="secondary" size="sm" class="h-8 rounded-lg" :disabled="isLoading" @click="load">
            <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('admin.sales.actions.refresh') }}
          </Button>
        </div>
      </div>
      <p v-if="summary && synced" class="mt-3 text-xs text-muted-foreground">
        {{ t('admin.sales.range.current', { range: rangeLabel }) }}
        <span aria-hidden="true">·</span>
        {{ t('admin.sales.range.previous', { range: previousRangeLabel }) }}
      </p>
    </section>

    <div v-if="errorKey" class="flex items-start gap-3 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ errorText(errorKey) }}</span>
    </div>

    <div v-if="isLoading && !summary" class="flex items-center justify-center rounded-lg border border-border/50 bg-card py-16 text-muted-foreground">
      <Loader2 class="h-5 w-5 animate-spin" />
    </div>

    <!-- 尚未同步 -->
    <section v-else-if="summary && !synced" class="flex flex-col items-center rounded-lg border border-dashed border-border/70 bg-card px-6 py-12 text-center">
      <Database class="mb-3 h-10 w-10 text-muted-foreground/60" />
      <h2 class="text-base font-semibold text-foreground">{{ t('admin.sales.empty.title') }}</h2>
      <p class="mt-2 max-w-xl text-sm text-muted-foreground">{{ t('admin.sales.empty.description') }}</p>
      <RouterLink to="/admin/user-activity" class="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
        {{ t('admin.sales.actions.goUserActivity') }}
      </RouterLink>
    </section>

    <template v-else-if="summary && current">
      <div v-if="coverageNotice" class="flex items-start gap-3 rounded-lg border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300" role="status">
        <Info class="mt-0.5 h-4 w-4 shrink-0" />
        <span>{{ coverageNotice }}</span>
      </div>

      <!-- 核心指标 -->
      <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5" :class="{ 'opacity-60 transition-opacity': isLoading }">
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.sales.kpi.credits') }}</p>
          <p class="mt-1 flex flex-wrap items-baseline gap-2">
            <span class="text-2xl font-semibold text-foreground">{{ formatMoney(current.credits) }}</span>
            <DeltaBadge :current="current.credits" :previous="previous?.credits" :missing-hint="t('admin.sales.coverage.previousMissing')" />
          </p>
          <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.sales.kpi.creditsHint', { count: formatCount(current.events), deductions: formatMoney(current.deductions) }) }}</p>
          <p v-if="hasExclusions" class="mt-1 text-xs text-muted-foreground">
            {{ t('admin.sales.kpi.excluded', { amount: formatMoney(exclusions.amount) }) }}
            <button type="button" class="font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="openEntries()">
              {{ t('admin.sales.actions.manageExcluded') }}
            </button>
          </p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.sales.kpi.payers') }}</p>
          <p class="mt-1 flex flex-wrap items-baseline gap-2">
            <span class="text-2xl font-semibold text-foreground">{{ formatCount(current.payers) }}</span>
            <DeltaBadge :current="current.payers" :previous="previous?.payers" :missing-hint="t('admin.sales.coverage.previousMissing')" />
          </p>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ t('admin.sales.kpi.payersHint', { newPayers: current.newPayers == null ? t('admin.sales.common.placeholder') : formatCount(current.newPayers), repeat: formatCount(current.repeatPayers) }) }}
          </p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.sales.kpi.arppu') }}</p>
          <p class="mt-1 flex flex-wrap items-baseline gap-2">
            <span class="text-2xl font-semibold text-foreground">{{ formatMoney(arppu(current)) }}</span>
            <DeltaBadge :current="arppu(current) ?? 0" :previous="previous ? arppu(previous) : null" :missing-hint="t('admin.sales.coverage.previousMissing')" />
          </p>
          <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.sales.kpi.arppuHint') }}</p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.sales.kpi.consumption') }}</p>
          <p class="mt-1 flex flex-wrap items-baseline gap-2">
            <span class="text-2xl font-semibold text-foreground">{{ formatMoney(current.consumption) }}</span>
            <DeltaBadge v-if="current.consumption != null" :current="current.consumption" :previous="previous?.consumption" :missing-hint="t('admin.sales.coverage.previousMissing')" />
          </p>
          <p class="mt-1 text-xs text-muted-foreground">
            <template v-if="current.consumption == null">{{ t('admin.sales.coverage.usagePartial', { range: formatRange(coverage?.usageFrom, coverage?.usageTo) }) }}</template>
            <template v-else-if="creditToConsumption != null">{{ t('admin.sales.kpi.consumptionHint', { ratio: creditToConsumption.toFixed(2) }) }}</template>
            <template v-else>{{ t('admin.sales.kpi.consumptionNone') }}</template>
          </p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.sales.kpi.balanceChange') }}</p>
          <p class="mt-1 text-2xl font-semibold text-foreground">{{ balanceChange == null ? t('admin.sales.common.placeholder') : formatSignedMoney(balanceChange) }}</p>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ current.balanceEnd != null ? t('admin.sales.kpi.balanceChangeHint', { value: formatMoney(current.balanceEnd) }) : t('admin.sales.kpi.balanceUnknown') }}
          </p>
        </article>
      </section>

      <!-- 每日入账与消耗 -->
      <section class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <h3 class="text-sm font-semibold text-foreground">{{ t('admin.sales.trend.title') }}</h3>
            <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.sales.trend.subtitle') }}</p>
          </div>
          <button
            type="button"
            class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :aria-pressed="showTable"
            @click="showTable = !showTable"
          >
            <Table2 class="h-3.5 w-3.5" />
            {{ showTable ? t('admin.sales.actions.showChart') : t('admin.sales.actions.showTable') }}
          </button>
        </div>
        <div v-if="!showTable" class="mt-3 h-72" :class="{ 'opacity-60 transition-opacity': isLoading }">
          <SalesTrendChart
            :days="chartDays"
            :channels="chartChannels"
            :series="chartSeries"
            :consumption="chartConsumption"
            :channel-label="channelLabel"
            :consumption-label="t('admin.sales.trend.consumption')"
            :total-label="t('admin.sales.trend.total')"
            :no-data-label="t('admin.sales.trend.noData')"
            :accessible-label="t('admin.sales.trend.aria', { range: rangeLabel })"
            :format-value="(value) => formatMoney(value)"
            :format-axis="formatMoneyAxis"
            :format-day="formatDay"
            :format-tick="formatShortDay"
          />
        </div>
        <div v-else class="mt-3 max-h-96 overflow-auto rounded-lg border border-border/50">
          <table class="w-full min-w-[640px] text-left text-sm">
            <thead class="sticky top-0 bg-surface-elevated text-xs text-muted-foreground">
              <tr>
                <th class="px-3 py-2 font-medium">{{ t('admin.sales.trend.day') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.sales.trend.total') }}</th>
                <th v-for="channel in chartChannels" :key="channel" class="px-3 py-2 text-right font-medium">{{ channelLabel(channel) }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.sales.trend.deductions') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.sales.trend.consumption') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50 tabular-nums">
              <tr v-for="point in daily" :key="point.day">
                <td class="px-3 py-2 text-foreground">{{ formatDay(point.day) }}</td>
                <td class="px-3 py-2 text-right font-medium text-foreground">{{ point.credits == null ? t('admin.sales.trend.noData') : formatMoney(point.credits) }}</td>
                <td v-for="channel in chartChannels" :key="channel" class="px-3 py-2 text-right text-muted-foreground">
                  {{ point.credits == null ? t('admin.sales.common.placeholder') : formatMoney(point.channels[channel] ?? 0) }}
                </td>
                <td class="px-3 py-2 text-right text-muted-foreground">{{ point.deductions == null ? t('admin.sales.common.placeholder') : formatMoney(point.deductions) }}</td>
                <td class="px-3 py-2 text-right text-muted-foreground">{{ point.consumption == null ? t('admin.sales.trend.noData') : formatMoney(point.consumption) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <section class="grid gap-3 xl:grid-cols-2">
        <!-- 渠道构成 -->
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <div class="flex items-start justify-between gap-3">
            <div class="min-w-0 flex-1">
              <h3 class="text-sm font-semibold text-foreground">{{ t('admin.sales.channels.title') }}</h3>
              <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.sales.channels.subtitle') }}</p>
            </div>
            <button
              type="button"
              class="inline-flex shrink-0 items-center gap-1.5 rounded-lg border border-border/60 px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              @click="openEntries()"
            >
              <ListFilter class="h-3.5 w-3.5" />
              {{ t('admin.sales.actions.entries') }}
            </button>
          </div>
          <p v-if="channelError" class="mt-2 text-xs text-warning" role="alert">{{ errorText(channelError) }}</p>
          <div v-if="positiveChannels.length === 0 && negativeChannels.length === 0" class="py-10 text-center text-sm text-muted-foreground">{{ t('admin.sales.channels.empty') }}</div>
          <table v-else class="mt-3 w-full text-left text-sm">
            <thead class="text-xs text-muted-foreground">
              <tr>
                <th class="w-8 py-2 pr-1 font-medium"><span class="sr-only">{{ t('admin.sales.channels.counted') }}</span></th>
                <th class="py-2 pr-2 font-medium">{{ t('admin.sales.channels.channel') }}</th>
                <th class="py-2 pr-2 text-right font-medium">{{ t('admin.sales.channels.amount') }}</th>
                <th class="w-1/4 py-2 pr-2 font-medium">{{ t('admin.sales.channels.share') }}</th>
                <th class="py-2 pr-2 text-right font-medium">{{ t('admin.sales.channels.events') }}</th>
                <th class="py-2 pr-2 text-right font-medium">{{ t('admin.sales.channels.users') }}</th>
                <th class="py-2 text-right font-medium">{{ t('admin.sales.channels.perEvent') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50 tabular-nums">
              <tr v-for="item in [...positiveChannels, ...negativeChannels]" :key="item.channel" :class="{ 'opacity-60': item.excluded }">
                <td class="py-2 pr-1">
                  <input
                    v-if="item.amount >= 0 && COUNTABLE.has(item.channel)"
                    type="checkbox"
                    class="h-3.5 w-3.5 accent-primary"
                    :checked="!item.excluded"
                    :disabled="savingChannel !== '' || isLoading"
                    :aria-label="t('admin.sales.channels.countThis', { channel: channelLabel(item.channel) })"
                    :title="t('admin.sales.channels.countThis', { channel: channelLabel(item.channel) })"
                    @change="toggleChannel(item.channel, ($event.target as HTMLInputElement).checked)"
                  >
                </td>
                <td class="py-2 pr-2 text-foreground">
                  <button
                    v-if="item.amount >= 0 && COUNTABLE.has(item.channel)"
                    type="button"
                    class="text-left hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                    :title="t('admin.sales.channels.viewEntries', { channel: channelLabel(item.channel) })"
                    @click="openEntries(item.channel)"
                  >
                    {{ channelLabel(item.channel) }}
                  </button>
                  <span v-else>{{ channelLabel(item.channel) }}</span>
                </td>
                <td class="py-2 pr-2 text-right font-medium" :class="item.amount < 0 || item.excluded ? 'text-muted-foreground' : 'text-foreground'">
                  {{ item.amount < 0 ? formatSignedMoney(item.amount) : formatMoney(item.amount) }}
                  <span v-if="(item.excludedAmount ?? 0) > 0 && !item.excluded" class="block text-[11px] font-normal text-muted-foreground">
                    {{ t('admin.sales.channels.excludedPart', { amount: formatMoney(item.excludedAmount) }) }}
                  </span>
                </td>
                <td class="py-2 pr-2">
                  <span v-if="item.excluded" class="text-xs text-muted-foreground">{{ t('admin.sales.channels.excludedChannel') }}</span>
                  <div v-else-if="item.amount > 0" class="flex items-center gap-2">
                    <span class="h-1.5 flex-1 overflow-hidden rounded-full bg-primary/15" aria-hidden="true">
                      <span class="block h-full rounded-full bg-primary" :style="{ width: `${Math.min(100, channelShare(item) * 100)}%` }" />
                    </span>
                    <span class="w-12 text-right text-xs text-muted-foreground">{{ formatPercent(channelShare(item)) }}</span>
                  </div>
                  <span v-else class="text-xs text-muted-foreground">{{ t('admin.sales.channels.notCounted') }}</span>
                </td>
                <td class="py-2 pr-2 text-right text-muted-foreground">{{ formatCount(item.events) }}</td>
                <td class="py-2 pr-2 text-right text-muted-foreground">{{ formatCount(item.users) }}</td>
                <td class="py-2 text-right text-muted-foreground">{{ item.events > 0 ? formatMoney(Math.abs(item.amount) / item.events) : t('admin.sales.common.placeholder') }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="coverage?.method === 'records'" class="mt-3 text-xs text-muted-foreground">{{ t('admin.sales.channels.recordsNote') }}</p>
        </article>

        <!-- 转化、复购与集中度 -->
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <h3 class="text-sm font-semibold text-foreground">{{ t('admin.sales.conversion.title') }}</h3>
          <div class="mt-3 grid gap-3 sm:grid-cols-3">
            <div class="rounded-lg bg-surface p-3">
              <p class="text-xs text-muted-foreground">{{ t('admin.sales.conversion.registered') }}</p>
              <p class="mt-1 text-lg font-semibold text-foreground">{{ conversionRate == null ? t('admin.sales.common.placeholder') : formatPercent(conversionRate) }}</p>
              <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.sales.conversion.registeredHint', { paid: formatCount(current.registeredPaid), registered: formatCount(current.registered) }) }}</p>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <p class="text-xs text-muted-foreground">{{ t('admin.sales.conversion.repeat') }}</p>
              <p class="mt-1 text-lg font-semibold text-foreground">{{ repeatRate == null ? t('admin.sales.common.placeholder') : formatPercent(repeatRate) }}</p>
              <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.sales.conversion.repeatHint', { repeat: formatCount(current.repeatPayers), payers: formatCount(current.payers) }) }}</p>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <p class="text-xs text-muted-foreground">{{ t('admin.sales.conversion.concentration') }}</p>
              <p class="mt-1 text-lg font-semibold text-foreground">{{ current.payers > 0 ? formatPercent(summary.concentration.top10Share) : t('admin.sales.common.placeholder') }}</p>
              <p class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.sales.conversion.concentrationHint', { top1: formatPercent(summary.concentration.top1Share) }) }}</p>
            </div>
          </div>
          <h4 class="mt-4 text-xs font-medium text-muted-foreground">{{ t('admin.sales.conversion.distribution') }}</h4>
          <ul class="mt-2 space-y-1.5">
            <li v-for="bucket in distribution" :key="bucket.min" class="grid grid-cols-[7.5rem_1fr_6.5rem] items-center gap-2 text-xs">
              <span class="truncate text-muted-foreground">{{ bucketLabel(bucket) }}</span>
              <span class="h-2 overflow-hidden rounded-full bg-primary/10" aria-hidden="true">
                <span class="block h-full rounded-full bg-primary" :style="{ width: `${(bucket.users / maxBucketUsers) * 100}%` }" />
              </span>
              <span class="text-right tabular-nums text-foreground">{{ t('admin.sales.conversion.bucketUsers', { count: formatCount(bucket.users) }) }}</span>
            </li>
          </ul>
        </article>
      </section>

      <!-- 入账排行 -->
      <section class="rounded-lg border border-border/50 bg-card shadow-sm">
        <div class="flex flex-wrap items-baseline justify-between gap-2 border-b border-border/50 p-4">
          <h3 class="text-sm font-semibold text-foreground">{{ t('admin.sales.payers.title') }}</h3>
          <p class="text-xs text-muted-foreground">{{ t('admin.sales.payers.subtitle') }}</p>
        </div>
        <div v-if="summary.topPayers.length === 0" class="py-10 text-center text-sm text-muted-foreground">{{ t('admin.sales.payers.empty') }}</div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[900px] text-left text-sm">
            <thead class="bg-surface-elevated/60 text-xs text-muted-foreground">
              <tr>
                <th class="px-4 py-2 font-medium">#</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.sales.payers.user') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.sales.payers.credits') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.sales.payers.events') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.sales.payers.channels') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.sales.payers.consumption') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.sales.payers.balance') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.sales.payers.lastCredit') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50 tabular-nums">
              <tr v-for="(row, index) in summary.topPayers" :key="row.siteUserId" class="transition-colors hover:bg-surface/40">
                <td class="px-4 py-2 text-muted-foreground">{{ index + 1 }}</td>
                <td class="max-w-[16rem] px-4 py-2">
                  <button
                    type="button"
                    class="block max-w-full truncate text-left font-medium text-foreground hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                    :title="t('admin.sales.actions.viewDetail', { name: payerName(row) })"
                    @click="detailUserId = row.siteUserId"
                  >
                    {{ payerName(row) }}
                  </button>
                  <span v-if="row.email && row.username" class="block truncate text-xs text-muted-foreground">{{ row.username }}</span>
                </td>
                <td class="px-4 py-2 text-right font-semibold text-foreground">{{ formatMoney(row.credits) }}</td>
                <td class="px-4 py-2 text-right text-muted-foreground">{{ t('admin.sales.payers.eventsValue', { events: formatCount(row.events), days: formatCount(row.activeDays) }) }}</td>
                <td class="px-4 py-2">
                  <span class="flex flex-wrap gap-1">
                    <span v-for="mix in row.channels" :key="mix.channel" class="rounded-md bg-surface px-1.5 py-0.5 text-[11px] text-muted-foreground">
                      {{ channelLabel(mix.channel) }} {{ formatMoney(mix.amount) }}
                    </span>
                  </span>
                </td>
                <td class="px-4 py-2 text-right text-muted-foreground">{{ formatMoney(row.consumption) }}</td>
                <td class="px-4 py-2 text-right text-muted-foreground">{{ formatMoney(row.balance) }}</td>
                <td class="px-4 py-2 text-muted-foreground">{{ formatDay(row.lastCreditDay) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <p class="flex items-start gap-2 px-1 text-xs text-muted-foreground">
        <BadgeDollarSign class="mt-0.5 h-3.5 w-3.5 shrink-0" />
        <span>{{ t('admin.sales.footnote') }}</span>
      </p>
    </template>

    <SitePricingModal :open="isPricingOpen" @close="isPricingOpen = false" @saved="onPricingSaved" />
    <SalesEntriesDrawer
      v-if="entries && summary"
      :from="summary.range.from"
      :to="summary.range.to"
      :channel="entries.channel"
      :excluded-only="entries.excludedOnly"
      @changed="load"
      @close="entries = null"
    />
    <UserActivityDetailDrawer
      v-if="detailUserId"
      :site-user-id="detailUserId"
      :platform="coverage?.platform ?? ''"
      :today="today"
      @close="detailUserId = null"
    />
  </div>
</template>
