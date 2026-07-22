<script setup lang="ts">
import { computed, onMounted, ref, watch, type Component } from 'vue'
import { useMediaQuery } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import type { EChartsCoreOption } from 'echarts/core'
import {
  Activity,
  AlertTriangle,
  ArrowRight,
  CircleCheckBig,
  Clock3,
  Gauge,
  Landmark,
  Layers3,
  Loader2,
  Lock,
  PiggyBank,
  RefreshCw,
  ShieldCheck,
  ShoppingCart,
  TrendingUp,
  Wallet,
} from 'lucide-vue-next'
import AdminLoginModal from '../components/dashboard/AdminLoginModal.vue'
import BalanceFilterModal from '../components/dashboard/BalanceFilterModal.vue'
import DashboardEChart from '../components/dashboard/DashboardEChart.vue'
import GroupUsageTodayModal from '../components/dashboard/GroupUsageTodayModal.vue'
import StatCard from '../components/dashboard/StatCard.vue'
import UpstreamBalanceBreakdownModal from '../components/dashboard/UpstreamBalanceBreakdownModal.vue'
import UpstreamKeyUsageTodayModal from '../components/dashboard/UpstreamKeyUsageTodayModal.vue'
import {
  getDashboardMetrics,
  getDashboardTrends,
  getGroupUsageToday,
  getUpstreamBalanceBreakdown,
  type GroupUsageTodayResponse,
  type UpstreamBalanceBreakdownResponse,
} from '../api/dashboardAdmin'
import { getConnectionHealthStoredSummary } from '../api/connectionHealth'
import { useDashboardAdmin } from '../composables/useDashboardAdmin'
import { useDashboardChartTheme } from '../composables/useDashboardChartTheme'
import {
  getDashboardDataSnapshot,
  saveDashboardDataSnapshot,
  updateDashboardOperationalSnapshot,
  type DashboardDataSnapshot,
} from '../composables/useDashboardDataCache'
import { useDashboardMetrics } from '../composables/useDashboardMetrics'
import { useAdminAccounts } from '../composables/useAdminAccounts'
import type { ConnectionHealthStoredSummary } from '../types/connectionHealth'
import type { DashboardColorToken, DashboardMetricData, DashboardMetricKey, DashboardPeriod } from '../types/dashboard'
import type { DashboardAdminPlatform, Sub2apiAuthMethod } from '../types/dashboardAdmin'
import { computeDelta, formatCny, formatDateTime } from '../utils/dashboard'

const { t, locale } = useI18n()
const router = useRouter()
const { metrics, applyRawData } = useDashboardMetrics()
const { theme: chartTheme } = useDashboardChartTheme()
const isNarrowScreen = useMediaQuery('(max-width: 639px)')
const { currentAccount } = useAdminAccounts()
const workspaceID = computed(() => currentAccount.value?.id ?? '')

const {
  status: adminStatus,
  isModalOpen: adminModalOpen,
  isSubmitting: adminSubmitting,
  isRefreshingCredentials: adminRefreshingCredentials,
  errorKey: adminErrorKey,
  checkStatus: checkAdminStatus,
  submitLogin: submitAdminLogin,
  updateAdminCredentials,
  openModal: openAdminModal,
  closeModal: closeAdminModal,
} = useDashboardAdmin()

const adminIdentity = computed(() => adminStatus.value.identity || adminStatus.value.baseUrl || '')
const adminLoginInitialValue = computed(() => ({
  platform: (adminStatus.value.platform as DashboardAdminPlatform) || 'sub2api',
  siteUrl: adminStatus.value.baseUrl || '',
  authMethod: (adminStatus.value.authMethod as Sub2apiAuthMethod) || 'password',
  email: adminStatus.value.identity || '',
}))

const balanceFilterOpen = ref(false)
const groupUsageTodayOpen = ref(false)
const upstreamKeyUsageTodayOpen = ref(false)
const upstreamBalanceBreakdownOpen = ref(false)

const openBalanceFilter = () => { balanceFilterOpen.value = true }
const closeBalanceFilter = () => { balanceFilterOpen.value = false }
const onBalanceFilterSaved = () => { void loadAllData({ skipStatusCheck: true }) }
const openGroupUsageToday = () => { groupUsageTodayOpen.value = true }
const closeGroupUsageToday = () => { groupUsageTodayOpen.value = false }
const openUpstreamKeyUsageToday = () => { upstreamKeyUsageTodayOpen.value = true }
const closeUpstreamKeyUsageToday = () => { upstreamKeyUsageTodayOpen.value = false }
const openUpstreamBalanceBreakdown = () => { upstreamBalanceBreakdownOpen.value = true }
const closeUpstreamBalanceBreakdown = () => { upstreamBalanceBreakdownOpen.value = false }
const openGroupList = () => { void router.push({ name: 'AdminGroupAssociations' }) }

const handleMetricCardClick = (key: string) => {
  switch (key) {
    case 'todayProfit':
      openGroupUsageToday()
      break
    case 'todayPurchase':
      openUpstreamKeyUsageToday()
      break
    case 'upstreamBalance':
      openUpstreamBalanceBreakdown()
      break
    case 'siteBalance':
      openBalanceFilter()
      break
  }
}

const groupCount = ref<number | null>(null)
const groupUsage = ref<GroupUsageTodayResponse | null>(null)
const balanceBreakdown = ref<UpstreamBalanceBreakdownResponse | null>(null)
const healthSummary = ref<ConnectionHealthStoredSummary | null>(null)
const operationalLoading = ref(false)
const operationalLoadError = ref(false)
const groupUsageLoadError = ref(false)
const balanceLoadError = ref(false)
const healthLoadError = ref(false)
const initialLoading = ref(true)
const isRefreshingData = ref(false)
const refreshDataFailed = ref(false)
const lastUpdatedAt = ref<number | null>(null)

const hydrateSnapshot = (snapshot: DashboardDataSnapshot | null): boolean => {
  if (!snapshot) return false
  adminStatus.value = snapshot.adminStatus
  groupCount.value = snapshot.live.groupCount ?? null
  groupUsage.value = snapshot.groupUsage ?? null
  balanceBreakdown.value = snapshot.balanceBreakdown ?? null
  healthSummary.value = snapshot.healthSummary ?? null
  applyRawData(snapshot.live, snapshot.trends)
  lastUpdatedAt.value = snapshot.updatedAt
  initialLoading.value = false
  return true
}

const initialSnapshot = getDashboardDataSnapshot(workspaceID.value)
hydrateSnapshot(initialSnapshot)

// 次要面板失败不阻断五项核心指标。三个请求都只读，其中健康摘要和余额明细只读本地缓存。
const loadOperationalData = async () => {
  operationalLoading.value = true
  operationalLoadError.value = false
  const [groupResult, balanceResult, healthResult] = await Promise.allSettled([
    getGroupUsageToday(),
    getUpstreamBalanceBreakdown(),
    getConnectionHealthStoredSummary(),
  ])
  groupUsageLoadError.value = groupResult.status === 'rejected'
  balanceLoadError.value = balanceResult.status === 'rejected'
  healthLoadError.value = healthResult.status === 'rejected'
  if (groupResult.status === 'fulfilled') groupUsage.value = groupResult.value
  if (balanceResult.status === 'fulfilled') balanceBreakdown.value = balanceResult.value
  if (healthResult.status === 'fulfilled') healthSummary.value = healthResult.value
  operationalLoadError.value = groupUsageLoadError.value || balanceLoadError.value || healthLoadError.value
  operationalLoading.value = false

  const key = workspaceID.value
  if (key) {
    updateDashboardOperationalSnapshot(key, {
      ...(groupResult.status === 'fulfilled' ? { groupUsage: groupResult.value } : {}),
      ...(balanceResult.status === 'fulfilled' ? { balanceBreakdown: balanceResult.value } : {}),
      ...(healthResult.status === 'fulfilled' ? { healthSummary: healthResult.value } : {}),
    })
  }
}

const loadAllData = async (options: { skipStatusCheck?: boolean } = {}) => {
  if (isRefreshingData.value) return
  isRefreshingData.value = true
  refreshDataFailed.value = false

  if (!options.skipStatusCheck) {
    await checkAdminStatus({ preserveAuthenticatedOnError: metrics.value.length > 0 })
  }
  if (!adminStatus.value.authenticated) {
    initialLoading.value = false
    isRefreshingData.value = false
    return
  }

  try {
    const [liveData, trendsData] = await Promise.all([
      getDashboardMetrics(),
      getDashboardTrends(30),
    ])
    groupCount.value = liveData.groupCount ?? null
    applyRawData(liveData, trendsData)
    const updatedAt = Date.now()
    lastUpdatedAt.value = updatedAt
    const key = workspaceID.value
    if (key) {
      const previous = getDashboardDataSnapshot(key)
      saveDashboardDataSnapshot(key, {
        adminStatus: { ...adminStatus.value },
        live: liveData,
        trends: trendsData,
        updatedAt,
        groupUsage: previous?.groupUsage,
        balanceBreakdown: previous?.balanceBreakdown,
        healthSummary: previous?.healthSummary,
      })
    }
    void loadOperationalData()
  } catch {
    refreshDataFailed.value = true
  } finally {
    initialLoading.value = false
    isRefreshingData.value = false
  }
}

const adminExpiry = computed(
  () => formatDateTime(adminStatus.value.expiresAt, locale.value) ?? t('admin.dashboard.adminAuth.timeUnknown'),
)

const lastUpdatedLabel = computed(() => {
  if (lastUpdatedAt.value == null) return t('admin.dashboard.dataStatus.waiting')
  return new Intl.DateTimeFormat(locale.value, {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(new Date(lastUpdatedAt.value))
})

onMounted(() => { void loadAllData() })

watch(() => adminStatus.value.authenticated, (authenticated) => {
  if (authenticated && !isRefreshingData.value) {
    if (metrics.value.length === 0) initialLoading.value = true
    void loadAllData({ skipStatusCheck: true })
  }
})

watch(workspaceID, (next, previous) => {
  if (!next || next === previous || isRefreshingData.value) return
  groupUsage.value = null
  balanceBreakdown.value = null
  healthSummary.value = null
  metrics.value = []
  const restored = hydrateSnapshot(getDashboardDataSnapshot(next))
  if (!restored) initialLoading.value = true
  void loadAllData()
})

const METRIC_META: Record<DashboardMetricKey, { icon: Component; labelKey: string; color: DashboardColorToken }> = {
  todayProfit: { icon: TrendingUp, labelKey: 'admin.dashboard.metrics.todayProfit', color: 'primary' },
  siteBalance: { icon: Wallet, labelKey: 'admin.dashboard.metrics.siteBalance', color: 'accent' },
  todayPurchase: { icon: ShoppingCart, labelKey: 'admin.dashboard.metrics.todayPurchase', color: 'warning' },
  netProfit: { icon: PiggyBank, labelKey: 'admin.dashboard.metrics.netProfit', color: 'signal' },
  upstreamBalance: { icon: Landmark, labelKey: 'admin.dashboard.metrics.upstreamBalance', color: 'primary' },
}

const metricMap = computed(() => new Map(metrics.value.map(metric => [metric.key, metric])))
const metric = (key: DashboardMetricKey): DashboardMetricData | undefined => metricMap.value.get(key)
const deltaCaption = computed(() => t('admin.dashboard.delta.vsPrev'))

const percentFormatter = computed(() => new Intl.NumberFormat(locale.value, {
  style: 'percent',
  maximumFractionDigits: 1,
}))
const numberFormatter = computed(() => new Intl.NumberFormat(locale.value, { maximumFractionDigits: 1 }))

const profitMargin = computed(() => {
  const revenue = metric('todayProfit')?.current ?? 0
  const profit = metric('netProfit')?.current ?? 0
  return revenue > 0 ? (profit / revenue) * 100 : 0
})

const marginSeries = computed(() => {
  const revenue = metric('todayProfit')?.series.month ?? []
  const profit = metric('netProfit')?.series.month ?? []
  return revenue.map((point, index) => point.value > 0 ? ((profit[index]?.value ?? 0) / point.value) * 100 : 0)
})

interface DashboardCoreCard {
  key: string
  label: string
  icon: Component
  color: DashboardColorToken
  value: string
  deltaDirection: ReturnType<typeof computeDelta>['direction']
  deltaText: string
  clickable: boolean
  negativeWhenUp: boolean
}

const cards = computed<DashboardCoreCard[]>(() => {
  const result: DashboardCoreCard[] = (['todayProfit', 'todayPurchase', 'netProfit'] as DashboardMetricKey[]).flatMap((key) => {
    const current = metric(key)
    if (!current) return []
    const delta = computeDelta(current.series.month.map(point => point.value))
    return [{
      key,
      label: t(METRIC_META[key].labelKey),
      icon: METRIC_META[key].icon,
      color: METRIC_META[key].color,
      value: formatCny(current.current),
      deltaDirection: delta.direction,
      deltaText: formatCny(Math.abs(delta.amount)),
      clickable: key === 'todayProfit' || key === 'todayPurchase',
      negativeWhenUp: key === 'todayPurchase',
    }]
  })
  const marginDelta = computeDelta(marginSeries.value)
  result.push({
    key: 'profitMargin',
    label: t('admin.dashboard.metrics.profitMargin'),
    icon: Gauge,
    color: 'accent',
    value: percentFormatter.value.format(profitMargin.value / 100),
    deltaDirection: marginDelta.direction,
    deltaText: t('admin.dashboard.delta.percentagePoints', { value: numberFormatter.value.format(Math.abs(marginDelta.amount)) }),
    clickable: false,
    negativeWhenUp: false,
  })
  return result
})

const period = ref<DashboardPeriod>('week')
const periods: DashboardPeriod[] = ['week', 'month']
const selectedSeries = (key: DashboardMetricKey) => metric(key)?.series[period.value] ?? []
const sumSeries = (key: DashboardMetricKey) => selectedSeries(key).reduce((total, point) => total + point.value, 0)

const periodTotals = computed(() => ({
  revenue: sumSeries('todayProfit'),
  cost: sumSeries('todayPurchase'),
  profit: sumSeries('netProfit'),
}))

const compactCurrency = (value: number) => {
  const absolute = Math.abs(value)
  if (absolute >= 1_000_000) return `¥${numberFormatter.value.format(value / 1_000_000)}M`
  if (absolute >= 1_000) return `¥${numberFormatter.value.format(value / 1_000)}K`
  return `¥${numberFormatter.value.format(value)}`
}

const performanceChartOption = computed<EChartsCoreOption>(() => {
  const revenue = selectedSeries('todayProfit')
  const cost = selectedSeries('todayPurchase')
  const profit = selectedSeries('netProfit')
  const theme = chartTheme.value
  const commonSeries = (name: string, data: number[], color: string) => ({
    name,
    data,
    itemStyle: { color },
    tooltip: { valueFormatter: (value: number) => formatCny(value) },
  })
  return {
    animationDuration: 350,
    textStyle: { color: theme.muted, fontFamily: 'Segoe UI Variable, Segoe UI, sans-serif' },
    tooltip: {
      trigger: 'axis',
      confine: true,
      backgroundColor: theme.card,
      borderColor: theme.border,
      textStyle: { color: theme.foreground },
      axisPointer: { type: 'shadow', shadowStyle: { color: theme.border } },
    },
    legend: {
      top: 0,
      right: 0,
      itemWidth: 10,
      itemHeight: 10,
      textStyle: { color: theme.muted },
    },
    grid: { top: 42, right: 8, bottom: 8, left: 8, containLabel: true },
    xAxis: {
      type: 'category',
      data: revenue.map(point => point.label),
      axisLine: { lineStyle: { color: theme.border } },
      axisTick: { show: false },
      axisLabel: { color: theme.muted, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      axisLabel: { color: theme.muted, formatter: (value: number) => compactCurrency(value) },
      splitLine: { lineStyle: { color: theme.border, type: 'dashed' } },
    },
    dataZoom: period.value === 'month' ? [{ type: 'inside', start: 0, end: 100 }] : [],
    series: [
      { ...commonSeries(t('admin.dashboard.metrics.todayProfit'), revenue.map(point => point.value), theme.primary), type: 'bar', barMaxWidth: 18, itemStyle: { color: theme.primary, borderRadius: [3, 3, 0, 0] } },
      { ...commonSeries(t('admin.dashboard.metrics.todayPurchase'), cost.map(point => point.value), theme.warning), type: 'bar', barMaxWidth: 18, itemStyle: { color: theme.warning, borderRadius: [3, 3, 0, 0] } },
      { ...commonSeries(t('admin.dashboard.metrics.netProfit'), profit.map(point => point.value), theme.signal), type: 'line', smooth: 0.22, symbol: 'circle', symbolSize: 5, lineStyle: { width: 2.5, color: theme.signal }, itemStyle: { color: theme.signal } },
    ],
  }
})

const sortedGroups = computed(() => [...(groupUsage.value?.groups ?? [])].sort((a, b) => b.todayAmount - a.todayAmount))
const topGroups = computed(() => sortedGroups.value.slice(0, 6))
const groupConcentration = computed(() => {
  const total = groupUsage.value?.total ?? 0
  if (total <= 0) return 0
  return sortedGroups.value.slice(0, 3).reduce((sum, item) => sum + item.todayAmount, 0) / total
})

const escapeTooltipHtml = (value: string) => {
  const entities: Record<string, string> = {
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
    "'": '&#39;',
  }
  return value.replace(/[&<>"']/g, character => entities[character] ?? character)
}

const groupTooltipContent = (params: unknown, mutedColor: string, foregroundColor: string) => {
  const point = (Array.isArray(params) ? params[0] : params) as { name?: unknown; value?: unknown } | undefined
  if (!point) return ''
  const name = escapeTooltipHtml(String(point.name ?? ''))
  const amount = Number(point.value)
  const value = escapeTooltipHtml(formatCny(Number.isFinite(amount) ? amount : 0))
  const label = escapeTooltipHtml(t('admin.dashboard.groups.amount'))
  return `<div style="margin-bottom:6px;color:${foregroundColor};font-weight:600">${name}</div>`
    + `<div style="display:flex;min-width:160px;justify-content:space-between;gap:20px;color:${mutedColor}">`
    + `<span>${label}</span><strong style="color:${foregroundColor}">${value}</strong></div>`
}

const groupChartOption = computed<EChartsCoreOption>(() => {
  const theme = chartTheme.value
  return {
    animationDuration: 350,
    textStyle: { color: theme.muted, fontFamily: 'Segoe UI Variable, Segoe UI, sans-serif' },
    tooltip: {
      trigger: 'item',
      confine: true,
      backgroundColor: theme.card,
      borderColor: theme.border,
      textStyle: { color: theme.foreground },
      formatter: (params: unknown) => groupTooltipContent(params, theme.muted, theme.foreground),
    },
    grid: { top: 4, right: 14, bottom: 4, left: 8, containLabel: true },
    xAxis: {
      type: 'value',
      splitNumber: isNarrowScreen.value ? 3 : 5,
      axisLabel: { color: theme.muted, hideOverlap: true, formatter: (value: number) => compactCurrency(value) },
      splitLine: { lineStyle: { color: theme.border, type: 'dashed' } },
    },
    yAxis: {
      type: 'category',
      inverse: true,
      data: topGroups.value.map(item => item.groupName),
      axisLine: { show: false },
      axisTick: { show: false },
      axisLabel: {
        color: theme.muted,
        width: 112,
        overflow: 'truncate',
      },
    },
    series: [{
      name: t('admin.dashboard.groups.amount'),
      type: 'bar',
      data: topGroups.value.map(item => item.todayAmount),
      barMaxWidth: 20,
      itemStyle: { color: theme.primary, borderRadius: [0, 4, 4, 0] },
      emphasis: { itemStyle: { color: theme.primary, opacity: 0.88 } },
    }],
  }
})

const siteBalance = computed(() => metric('siteBalance')?.current ?? 0)
const upstreamBalance = computed(() => metric('upstreamBalance')?.current ?? 0)
const coverageRatio = computed(() => siteBalance.value > 0 ? (upstreamBalance.value / siteBalance.value) * 100 : null)
const averageDailyCost = computed(() => {
  const values = selectedSeries('todayPurchase').map(point => point.value)
  return values.length > 0 ? values.reduce((sum, value) => sum + value, 0) / values.length : 0
})
const runwayDays = computed(() => averageDailyCost.value > 0 ? upstreamBalance.value / averageDailyCost.value : null)
const coverageWidth = computed(() => `${Math.min(coverageRatio.value ?? 0, 100)}%`)
const coverageTone = computed(() => {
  if (coverageRatio.value == null) return 'bg-muted-foreground'
  if (coverageRatio.value >= 100) return 'bg-signal'
  if (coverageRatio.value >= 40) return 'bg-warning'
  return 'bg-destructive'
})

const upstreamIssueCount = computed(() => (balanceBreakdown.value?.sites ?? []).filter(
  site => site.balance == null || site.status === 'error',
).length)
const healthRiskCount = computed(() => (healthSummary.value?.attentionTargets ?? 0) + (healthSummary.value?.suspendedTargets ?? 0))
const attentionDataUnavailable = computed(() => healthLoadError.value || balanceLoadError.value)

const attentionItems = computed(() => {
  const items: Array<{
    key: string
    icon: Component
    title: string
    description: string
    count: number
    tone: string
    routeName: string
  }> = []
  if (healthRiskCount.value > 0) {
    items.push({
      key: 'health',
      icon: Activity,
      title: t('admin.dashboard.attention.healthTitle'),
      description: t('admin.dashboard.attention.healthDescription', {
        attention: healthSummary.value?.attentionTargets ?? 0,
        suspended: healthSummary.value?.suspendedTargets ?? 0,
      }),
      count: healthRiskCount.value,
      tone: 'text-warning bg-warning/10',
      routeName: 'AdminConnectionHealth',
    })
  }
  if ((healthSummary.value?.recentFailureEvents ?? 0) > 0) {
    items.push({
      key: 'events',
      icon: AlertTriangle,
      title: t('admin.dashboard.attention.failuresTitle'),
      description: t('admin.dashboard.attention.failuresDescription'),
      count: healthSummary.value?.recentFailureEvents ?? 0,
      tone: 'text-destructive bg-destructive/10',
      routeName: 'AdminConnectionHealth',
    })
  }
  if (upstreamIssueCount.value > 0) {
    items.push({
      key: 'upstream',
      icon: Landmark,
      title: t('admin.dashboard.attention.upstreamTitle'),
      description: t('admin.dashboard.attention.upstreamDescription'),
      count: upstreamIssueCount.value,
      tone: 'text-warning bg-warning/10',
      routeName: 'AdminUpstream',
    })
  }
  return items
})

const lastProbeLabel = computed(() => {
  const value = healthSummary.value?.lastProbeAt
  if (!value) return t('admin.dashboard.attention.neverProbed')
  return formatDateTime(Date.parse(value), locale.value) ?? t('admin.dashboard.attention.neverProbed')
})
</script>

<template>
  <div class="space-y-6">
    <div
      v-if="adminStatus.authenticated"
      class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-border/60 bg-card px-4 py-2.5 shadow-sm"
    >
      <div class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm">
        <span class="inline-flex h-2 w-2 shrink-0 rounded-full bg-signal" />
        <span class="truncate text-muted-foreground">{{ t('admin.dashboard.adminAuth.loggedInAs', { identity: adminIdentity }) }}</span>
        <span class="text-xs text-muted-foreground">
          {{ t('admin.dashboard.adminAuth.expiresAt') }} {{ adminExpiry }}
        </span>
        <span
          v-if="metrics.length > 0"
          class="inline-flex items-center gap-1 text-xs"
          :class="refreshDataFailed ? 'text-destructive' : 'text-muted-foreground'"
        >
          <RefreshCw v-if="isRefreshingData" class="h-3 w-3 animate-spin" />
          {{ isRefreshingData
            ? t('admin.dashboard.dataStatus.refreshing')
            : refreshDataFailed
              ? t('admin.dashboard.dataStatus.failed')
              : t('admin.dashboard.dataStatus.updatedAt', { time: lastUpdatedLabel }) }}
        </span>
      </div>
      <div class="flex items-center gap-1">
        <button
          type="button"
          class="inline-flex h-8 w-8 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="isRefreshingData"
          :title="t('admin.dashboard.dataStatus.refresh')"
          :aria-label="t('admin.dashboard.dataStatus.refresh')"
          @click="loadAllData()"
        >
          <RefreshCw class="h-3.5 w-3.5" :class="{ 'animate-spin': isRefreshingData }" />
        </button>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-xs font-medium text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="adminRefreshingCredentials"
          @click="updateAdminCredentials"
        >
          <ShieldCheck class="h-3.5 w-3.5" />
          {{ adminRefreshingCredentials ? t('admin.dashboard.adminAuth.updatingCredentials') : t('admin.dashboard.adminAuth.updateCredentials') }}
        </button>
      </div>
    </div>

    <div
      v-else-if="!initialLoading && !adminModalOpen"
      class="flex flex-wrap items-center justify-between gap-3 rounded-lg border border-warning/30 bg-warning/5 px-4 py-2.5"
    >
      <div class="flex items-center gap-2 text-sm text-muted-foreground">
        <span class="inline-flex h-2 w-2 rounded-full bg-warning" />
        {{ t('admin.dashboard.adminAuth.notLoggedIn') }}
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        @click="openAdminModal"
      >
        <ShieldCheck class="h-4 w-4" />
        {{ t('admin.dashboard.adminAuth.login') }}
      </button>
    </div>

    <section
      v-if="initialLoading"
      class="space-y-4"
      role="status"
      :aria-label="t('admin.dashboard.loading')"
    >
      <div class="grid grid-cols-2 gap-3 xl:grid-cols-4">
        <div
          v-for="item in 4"
          :key="item"
          class="min-h-[132px] animate-pulse rounded-lg border border-border/60 bg-card p-4 sm:min-h-[142px] sm:p-5"
        >
          <div class="flex items-start justify-between gap-4">
            <div class="flex-1 space-y-3">
              <div class="h-3 w-20 rounded bg-muted" />
              <div class="h-7 w-28 max-w-full rounded bg-muted" />
            </div>
            <div class="h-9 w-9 rounded-lg bg-muted" />
          </div>
          <div class="mt-5 h-3 w-32 max-w-full rounded bg-muted" />
        </div>
      </div>
      <div class="grid gap-4 xl:grid-cols-12">
        <div class="h-[430px] animate-pulse rounded-lg border border-border/60 bg-card p-5 xl:col-span-8">
          <div class="h-4 w-28 rounded bg-muted" />
          <div class="mt-4 h-[350px] rounded bg-muted/60" />
        </div>
        <div class="h-[430px] animate-pulse rounded-lg border border-border/60 bg-card p-5 xl:col-span-4">
          <div class="h-4 w-24 rounded bg-muted" />
          <div class="mt-6 space-y-5">
            <div class="h-12 rounded bg-muted/60" />
            <div class="h-12 rounded bg-muted/60" />
            <div class="h-24 rounded bg-muted/60" />
          </div>
        </div>
      </div>
    </section>

    <template v-else-if="adminStatus.authenticated">
      <div
        v-if="metrics.length === 0"
        class="flex flex-col items-center justify-center gap-4 rounded-lg border border-dashed border-destructive/30 bg-destructive/5 px-6 py-16 text-center"
      >
        <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.loadError') }}</p>
        <button
          type="button"
          class="inline-flex items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          @click="loadAllData()"
        >
          <RefreshCw class="h-4 w-4" />
          {{ t('admin.dashboard.retry') }}
        </button>
      </div>

      <template v-else-if="metrics.length > 0">
        <section class="grid grid-cols-2 gap-3 xl:grid-cols-4">
          <StatCard
            v-for="card in cards"
            :key="card.key"
            :label="card.label"
            :value="card.value"
            :icon="card.icon"
            :color="card.color"
            :delta-direction="card.deltaDirection"
            :delta-text="card.deltaText"
            :delta-caption="deltaCaption"
            :clickable="card.clickable"
            :negative-when-up="card.negativeWhenUp"
            @click="handleMetricCardClick(card.key)"
          />
        </section>

        <section class="grid gap-4 xl:grid-cols-12">
          <article class="min-w-0 rounded-lg border border-border/60 bg-card p-4 shadow-sm sm:p-5 xl:col-span-8">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div>
                <h2 class="text-base font-semibold text-foreground">{{ t('admin.dashboard.performance.title') }}</h2>
                <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.dashboard.performance.subtitle') }}</p>
              </div>
              <div
                class="inline-flex items-center rounded-lg border border-border/60 bg-surface/50 p-1"
                role="group"
                :aria-label="t('admin.dashboard.period.label')"
              >
                <button
                  v-for="item in periods"
                  :key="item"
                  type="button"
                  :aria-pressed="period === item"
                  class="rounded-md px-3 py-1.5 text-xs font-medium transition-colors sm:text-sm"
                  :class="period === item ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
                  @click="period = item"
                >
                  {{ t(`admin.dashboard.period.${item}`) }}
                </button>
              </div>
            </div>

            <dl class="mt-5 grid grid-cols-3 divide-x divide-border/60 border-y border-border/60 py-3">
              <div class="min-w-0 px-2 first:pl-0 sm:px-4">
                <dt class="truncate text-xs text-muted-foreground">{{ t('admin.dashboard.performance.periodRevenue') }}</dt>
                <dd class="mt-1 truncate text-sm font-semibold tabular-nums text-foreground sm:text-base">{{ formatCny(periodTotals.revenue) }}</dd>
              </div>
              <div class="min-w-0 px-2 sm:px-4">
                <dt class="truncate text-xs text-muted-foreground">{{ t('admin.dashboard.performance.periodCost') }}</dt>
                <dd class="mt-1 truncate text-sm font-semibold tabular-nums text-foreground sm:text-base">{{ formatCny(periodTotals.cost) }}</dd>
              </div>
              <div class="min-w-0 px-2 pr-0 sm:px-4 sm:pr-0">
                <dt class="truncate text-xs text-muted-foreground">{{ t('admin.dashboard.performance.periodProfit') }}</dt>
                <dd class="mt-1 truncate text-sm font-semibold tabular-nums text-signal sm:text-base">{{ formatCny(periodTotals.profit) }}</dd>
              </div>
            </dl>

            <div class="mt-4 h-[270px] sm:h-[310px]">
              <DashboardEChart
                :option="performanceChartOption"
                :accessible-label="t('admin.dashboard.performance.chartAria')"
              />
            </div>
          </article>

          <aside class="min-w-0 rounded-lg border border-border/60 bg-card p-4 shadow-sm sm:p-5 xl:col-span-4">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h2 class="text-base font-semibold text-foreground">{{ t('admin.dashboard.capital.title') }}</h2>
                <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.dashboard.capital.subtitle') }}</p>
              </div>
              <Landmark class="h-5 w-5 shrink-0 text-primary" />
            </div>

            <div class="mt-6 divide-y divide-border/60 border-y border-border/60">
              <button type="button" class="flex w-full items-center justify-between gap-4 py-4 text-left" @click="openBalanceFilter">
                <dt class="text-sm text-muted-foreground">{{ t('admin.dashboard.capital.siteBalance') }}</dt>
                <dd class="font-semibold tabular-nums text-foreground">{{ formatCny(siteBalance) }}</dd>
              </button>
              <button type="button" class="flex w-full items-center justify-between gap-4 py-4 text-left" @click="openUpstreamBalanceBreakdown">
                <dt class="text-sm text-muted-foreground">{{ t('admin.dashboard.capital.upstreamBalance') }}</dt>
                <dd class="font-semibold tabular-nums text-foreground">{{ formatCny(upstreamBalance) }}</dd>
              </button>
            </div>

            <div class="mt-6">
              <div class="flex items-end justify-between gap-3">
                <span class="text-sm text-muted-foreground">{{ t('admin.dashboard.capital.coverage') }}</span>
                <span class="text-xl font-semibold tabular-nums text-foreground">
                  {{ coverageRatio == null ? t('admin.dashboard.common.unavailable') : percentFormatter.format(coverageRatio / 100) }}
                </span>
              </div>
              <div class="mt-2 h-2 overflow-hidden rounded-full bg-muted">
                <div class="h-full rounded-full transition-[width]" :class="coverageTone" :style="{ width: coverageWidth }" />
              </div>
              <p class="mt-2 text-xs leading-5 text-muted-foreground">{{ t('admin.dashboard.capital.coverageHint') }}</p>
            </div>

            <div class="mt-6 flex items-center justify-between border-t border-border/60 pt-5">
              <div>
                <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.capital.runway') }}</p>
                <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.dashboard.capital.runwayHint') }}</p>
              </div>
              <div class="flex items-center gap-2 text-foreground">
                <Clock3 class="h-4 w-4 text-muted-foreground" />
                <span class="text-xl font-semibold tabular-nums">
                  {{ runwayDays == null ? t('admin.dashboard.common.unavailable') : t('admin.dashboard.capital.runwayValue', { value: numberFormatter.format(runwayDays) }) }}
                </span>
              </div>
            </div>
          </aside>
        </section>

        <section class="grid gap-4 xl:grid-cols-12">
          <article class="min-w-0 rounded-lg border border-border/60 bg-card p-4 shadow-sm sm:p-5 xl:col-span-7">
            <div class="flex flex-wrap items-start justify-between gap-4">
              <div>
                <h2 class="text-base font-semibold text-foreground">{{ t('admin.dashboard.groups.title') }}</h2>
                <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.dashboard.groups.subtitle') }}</p>
              </div>
              <button
                type="button"
                class="inline-flex items-center gap-1.5 rounded-md px-2.5 py-1.5 text-sm font-medium text-primary transition-colors hover:bg-primary/10"
                @click="openGroupList"
              >
                <Layers3 class="h-4 w-4" />
                {{ t('admin.dashboard.groups.total', { count: groupCount ?? 0 }) }}
              </button>
            </div>

            <div v-if="operationalLoading && !groupUsage" class="flex h-[280px] items-center justify-center text-muted-foreground">
              <Loader2 class="h-5 w-5 animate-spin" />
            </div>
            <div v-else-if="groupUsageLoadError && !groupUsage" class="flex h-[280px] flex-col items-center justify-center gap-3 text-sm text-muted-foreground">
              <AlertTriangle class="h-5 w-5 text-warning" />
              <span>{{ t('admin.dashboard.groups.loadError') }}</span>
              <button type="button" class="text-sm font-medium text-primary hover:underline" @click="loadOperationalData">
                {{ t('admin.dashboard.retry') }}
              </button>
            </div>
            <div v-else-if="topGroups.length > 0" class="mt-4 h-[280px]">
              <DashboardEChart :option="groupChartOption" :accessible-label="t('admin.dashboard.groups.chartAria')" />
            </div>
            <div v-else class="flex h-[280px] items-center justify-center text-sm text-muted-foreground">
              {{ t('admin.dashboard.groups.empty') }}
            </div>

            <div class="flex flex-wrap items-center justify-between gap-2 border-t border-border/60 pt-4 text-sm">
              <span class="text-muted-foreground">{{ t('admin.dashboard.groups.topThreeShare') }}</span>
              <span class="font-semibold tabular-nums text-foreground">{{ percentFormatter.format(groupConcentration) }}</span>
            </div>
          </article>

          <aside class="min-w-0 self-start rounded-lg border border-border/60 bg-card p-4 shadow-sm sm:p-5 xl:col-span-5">
            <div class="flex items-start justify-between gap-3">
              <div>
                <h2 class="text-base font-semibold text-foreground">{{ t('admin.dashboard.attention.title') }}</h2>
                <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.dashboard.attention.subtitle') }}</p>
              </div>
              <button
                type="button"
                class="rounded-md p-2 text-muted-foreground transition-colors hover:bg-muted hover:text-foreground disabled:opacity-50"
                :disabled="operationalLoading"
                :title="t('admin.dashboard.attention.refresh')"
                :aria-label="t('admin.dashboard.attention.refresh')"
                @click="loadOperationalData"
              >
                <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': operationalLoading }" />
              </button>
            </div>

            <div v-if="operationalLoading && !healthSummary && !balanceBreakdown" class="flex h-[260px] items-center justify-center text-muted-foreground">
              <Loader2 class="h-5 w-5 animate-spin" />
            </div>
            <div v-else-if="attentionItems.length > 0" class="mt-5 divide-y divide-border/60 border-y border-border/60">
              <button
                v-for="item in attentionItems"
                :key="item.key"
                type="button"
                class="flex w-full items-center gap-3 py-4 text-left transition-colors hover:bg-muted/40"
                @click="router.push({ name: item.routeName })"
              >
                <span :class="['flex h-9 w-9 shrink-0 items-center justify-center rounded-lg', item.tone]">
                  <component :is="item.icon" class="h-4 w-4" />
                </span>
                <span class="min-w-0 flex-1">
                  <span class="flex items-center gap-2">
                    <span class="truncate text-sm font-medium text-foreground">{{ item.title }}</span>
                    <span class="rounded-full bg-muted px-2 py-0.5 text-xs font-semibold tabular-nums text-foreground">{{ item.count }}</span>
                  </span>
                  <span class="mt-1 block text-xs leading-5 text-muted-foreground">{{ item.description }}</span>
                </span>
                <ArrowRight class="h-4 w-4 shrink-0 text-muted-foreground" />
              </button>
            </div>
            <div v-else-if="attentionDataUnavailable" class="flex min-h-[260px] flex-col items-center justify-center text-center">
              <span class="flex h-12 w-12 items-center justify-center rounded-full bg-warning/10 text-warning">
                <AlertTriangle class="h-6 w-6" />
              </span>
              <h3 class="mt-4 text-sm font-semibold text-foreground">{{ t('admin.dashboard.attention.unavailableTitle') }}</h3>
              <p class="mt-1 max-w-xs text-sm leading-6 text-muted-foreground">{{ t('admin.dashboard.attention.unavailableDescription') }}</p>
              <button type="button" class="mt-3 text-sm font-medium text-primary hover:underline" @click="loadOperationalData">
                {{ t('admin.dashboard.retry') }}
              </button>
            </div>
            <div v-else class="flex min-h-[260px] flex-col items-center justify-center text-center">
              <span class="flex h-12 w-12 items-center justify-center rounded-full bg-signal/10 text-signal">
                <CircleCheckBig class="h-6 w-6" />
              </span>
              <h3 class="mt-4 text-sm font-semibold text-foreground">{{ t('admin.dashboard.attention.allClearTitle') }}</h3>
              <p class="mt-1 max-w-xs text-sm leading-6 text-muted-foreground">{{ t('admin.dashboard.attention.allClearDescription') }}</p>
            </div>

            <div class="mt-4 flex flex-wrap items-center justify-between gap-2 border-t border-border/60 pt-4 text-xs text-muted-foreground">
              <span>{{ t('admin.dashboard.attention.lastProbe', { time: lastProbeLabel }) }}</span>
              <span v-if="operationalLoadError" class="text-destructive">{{ t('admin.dashboard.attention.partialLoadError') }}</span>
            </div>
          </aside>
        </section>
      </template>
    </template>

    <div
      v-else-if="!adminModalOpen"
      class="flex flex-col items-center justify-center gap-4 rounded-lg border border-dashed border-border/60 bg-card/40 px-6 py-16 text-center"
    >
      <div class="flex h-14 w-14 items-center justify-center rounded-full bg-muted text-muted-foreground">
        <Lock class="h-6 w-6" />
      </div>
      <div class="space-y-1.5">
        <h2 class="text-lg font-semibold text-foreground">{{ t('admin.dashboard.adminAuth.dataLocked.title') }}</h2>
        <p class="max-w-md text-sm text-muted-foreground">{{ t('admin.dashboard.adminAuth.dataLocked.description') }}</p>
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        @click="openAdminModal"
      >
        <ShieldCheck class="h-4 w-4" />
        {{ t('admin.dashboard.adminAuth.login') }}
      </button>
    </div>

    <AdminLoginModal
      :open="adminModalOpen"
      :submitting="adminSubmitting"
      :error-key="adminErrorKey"
      :initial-value="adminLoginInitialValue"
      @submit="submitAdminLogin"
      @close="closeAdminModal"
    />
    <BalanceFilterModal :open="balanceFilterOpen" @close="closeBalanceFilter" @saved="onBalanceFilterSaved" />
    <GroupUsageTodayModal :open="groupUsageTodayOpen" @close="closeGroupUsageToday" />
    <UpstreamKeyUsageTodayModal :open="upstreamKeyUsageTodayOpen" @close="closeUpstreamKeyUsageToday" />
    <UpstreamBalanceBreakdownModal :open="upstreamBalanceBreakdownOpen" @close="closeUpstreamBalanceBreakdown" />
  </div>
</template>
