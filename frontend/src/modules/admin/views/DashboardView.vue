<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch, type Component } from 'vue'
import { useRouter } from 'vue-router'
import { useI18n } from 'vue-i18n'
import { Landmark, Layers, Loader2, Lock, PiggyBank, RefreshCw, ShieldCheck, ShoppingCart, TrendingUp, Wallet } from 'lucide-vue-next'
import StatCard from '../components/dashboard/StatCard.vue'
import MetricTrendCard from '../components/dashboard/MetricTrendCard.vue'
import AdminLoginModal from '../components/dashboard/AdminLoginModal.vue'
import BalanceFilterModal from '../components/dashboard/BalanceFilterModal.vue'
import GroupUsageTodayModal from '../components/dashboard/GroupUsageTodayModal.vue'
import UpstreamKeyUsageTodayModal from '../components/dashboard/UpstreamKeyUsageTodayModal.vue'
import UpstreamBalanceBreakdownModal from '../components/dashboard/UpstreamBalanceBreakdownModal.vue'
import DashboardLoadingModal from '../components/dashboard/DashboardLoadingModal.vue'
import type { LoadingStep } from '../components/dashboard/DashboardLoadingModal.vue'
import { useDashboardMetrics } from '../composables/useDashboardMetrics'
import { useDashboardAdmin } from '../composables/useDashboardAdmin'
import { getDashboardMetrics, getDashboardTrends } from '../api/dashboardAdmin'
import { computeDelta, formatCny, formatDateTime } from '../utils/dashboard'
import type { DashboardMetricKey, DashboardPeriod } from '../types/dashboard'
import type { DashboardAdminPlatform, Sub2apiAuthMethod } from '../types/dashboardAdmin'

const { t } = useI18n()
const router = useRouter()
const { metrics, loading: metricsLoading, error: metricsError, fetchMetrics, applyRawData } = useDashboardMetrics()

// 仪表盘 admin 登录门禁：进入页面即检查是否已登录 admin，未登录则弹窗。
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

// 登录弹窗预填：来自当前已知的非敏感状态字段，仅在“更新凭证”校验失败自动弹窗时使用。
const adminLoginInitialValue = computed(() => ({
  platform: (adminStatus.value.platform as DashboardAdminPlatform) || 'sub2api',
  siteUrl: adminStatus.value.baseUrl || '',
  authMethod: (adminStatus.value.authMethod as Sub2apiAuthMethod) || 'password',
  email: adminStatus.value.identity || '',
}))

const balanceFilterOpen = ref(false)
const openBalanceFilter = () => { balanceFilterOpen.value = true }
const closeBalanceFilter = () => { balanceFilterOpen.value = false }
const onBalanceFilterSaved = () => { void fetchMetrics() }

const groupCount = ref<number | null>(null)
// “我的分组”卡片不再打开弹窗，改为跳转到独立的分组关联页面。
const openGroupList = () => { router.push({ name: 'AdminGroupAssociations' }) }

// 今日营收分组明细弹窗：数据只在弹窗打开时按需请求，不参与 metrics 批量拉取。
const groupUsageTodayOpen = ref(false)
const openGroupUsageToday = () => { groupUsageTodayOpen.value = true }
const closeGroupUsageToday = () => { groupUsageTodayOpen.value = false }

// 今日成本（上游 key 消费）下钻弹窗：数据只在弹窗打开时按需请求。
const upstreamKeyUsageTodayOpen = ref(false)
const openUpstreamKeyUsageToday = () => { upstreamKeyUsageTodayOpen.value = true }
const closeUpstreamKeyUsageToday = () => { upstreamKeyUsageTodayOpen.value = false }

// 上游总余额下钻弹窗：数据只在弹窗打开时按需请求。
const upstreamBalanceBreakdownOpen = ref(false)
const openUpstreamBalanceBreakdown = () => { upstreamBalanceBreakdownOpen.value = true }
const closeUpstreamBalanceBreakdown = () => { upstreamBalanceBreakdownOpen.value = false }

// 可点击下钻的指标卡片集合，与各自的打开逻辑一一对应。
const clickableMetricKeys = new Set<DashboardMetricKey>(['todayProfit', 'todayPurchase', 'upstreamBalance', 'siteBalance'])

const handleMetricCardClick = (key: DashboardMetricKey) => {
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

// ─── 步骤式加载弹窗 ────────────────────────────────────
const initialLoadDone = ref(false)
const loadingModalOpen = ref(false)

const STEP_KEYS = ['auth', 'data', 'done'] as const
type StepKey = typeof STEP_KEYS[number]

const stepStatuses = reactive<Record<StepKey, LoadingStep['status']>>({
  auth: 'pending',
  data: 'pending',
  done: 'pending',
})

const loadingSteps = computed<LoadingStep[]>(() =>
  STEP_KEYS.map(key => ({
    key,
    labelKey: `admin.dashboard.loadingModal.steps.${key}`,
    status: stepStatuses[key],
  })),
)

const setStep = (key: StepKey, status: LoadingStep['status']) => {
  stepStatuses[key] = status
}

const resetSteps = () => {
  for (const key of STEP_KEYS) {
    stepStatuses[key] = 'pending'
  }
}

const loadAllData = async () => {
  resetSteps()
  loadingModalOpen.value = true

  // 1. 验证管理员身份
  setStep('auth', 'active')
  await checkAdminStatus()
  if (!adminStatus.value.authenticated) {
    setStep('auth', 'error')
    loadingModalOpen.value = false
    return
  }
  setStep('auth', 'done')

  // 2. 并行加载实时指标 + 历史趋势（分组数量已合入 metrics 响应）
  setStep('data', 'active')
  let liveData, trendsData
  try {
    ;[liveData, trendsData] = await Promise.all([
      getDashboardMetrics(),
      getDashboardTrends(30),
    ])
    groupCount.value = liveData.groupCount ?? null
    setStep('data', 'done')
  } catch {
    setStep('data', 'error')
    loadingModalOpen.value = false
    return
  }

  // 3. 整理数据完成
  setStep('done', 'active')
  applyRawData(liveData, trendsData)
  setStep('done', 'done')

  initialLoadDone.value = true

  setTimeout(() => {
    loadingModalOpen.value = false
  }, 400)
}

// 顶部状态条在邮箱后展示登录凭证的过期时间（临期自动刷新）；取不到时显示「未知」。
const adminExpiry = computed(
  () => formatDateTime(adminStatus.value.expiresAt) ?? t('admin.dashboard.adminAuth.timeUnknown'),
)

onMounted(() => {
  void loadAllData()
})

// admin 登录成功后自动拉取指标数据和分组数。
// 仅在非加载中时触发，避免 loadAllData 内部的 checkAdminStatus 改变 authenticated 时重复调用。
watch(() => adminStatus.value.authenticated, (authenticated) => {
  if (authenticated && metrics.value.length === 0 && !loadingModalOpen.value) {
    void loadAllData()
  }
})

// 指标的图标与文案 key（颜色由数据层提供，图标/标题属于展示层）。
const METRIC_META: Record<DashboardMetricKey, { icon: Component; labelKey: string }> = {
  todayProfit: { icon: TrendingUp, labelKey: 'admin.dashboard.metrics.todayProfit' },
  siteBalance: { icon: Wallet, labelKey: 'admin.dashboard.metrics.siteBalance' },
  todayPurchase: { icon: ShoppingCart, labelKey: 'admin.dashboard.metrics.todayPurchase' },
  netProfit: { icon: PiggyBank, labelKey: 'admin.dashboard.metrics.netProfit' },
  upstreamBalance: { icon: Landmark, labelKey: 'admin.dashboard.metrics.upstreamBalance' },
}

const period = ref<DashboardPeriod>('week')
const periods: DashboardPeriod[] = ['week', 'month']

const deltaCaption = computed(() => t('admin.dashboard.delta.vsPrev'))

// 顶部统计卡片：当前值 + 今日相对昨日的环比（始终基于月序列，保持稳定）。
const cards = computed(() =>
  metrics.value.map((metric) => {
    const meta = METRIC_META[metric.key]
    const delta = computeDelta(metric.series.month.map((point) => point.value))
    return {
      key: metric.key,
      label: t(meta.labelKey),
      icon: meta.icon,
      color: metric.color,
      value: formatCny(metric.current),
      deltaDirection: delta.direction,
      deltaText: formatCny(Math.abs(delta.amount)),
      clickable: clickableMetricKeys.has(metric.key),
    }
  }),
)

// 趋势图卡片：随周/月切换展示连续序列，环比基于当前周期序列。
const charts = computed(() =>
  metrics.value.map((metric) => {
    const meta = METRIC_META[metric.key]
    const series = metric.series[period.value]
    const values = series.map((point) => point.value)
    const labels = series.map((point) => point.label)
    const delta = computeDelta(values)
    const metricName = t(meta.labelKey)
    return {
      key: metric.key,
      title: t('admin.dashboard.charts.trendTitle', { metric: metricName }),
      color: metric.color,
      value: formatCny(metric.current),
      values,
      labels,
      deltaDirection: delta.direction,
      deltaText: formatCny(Math.abs(delta.amount)),
    }
  }),
)
</script>

<template>
  <div class="space-y-8">
    <!-- 当前 admin 账户状态条：已登录显示身份+退出，未登录显示提示+登录入口 -->
    <div
      v-if="adminStatus.authenticated"
      class="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-border/50 bg-card px-5 py-3 shadow-sm"
    >
      <div class="flex flex-wrap items-center gap-x-2 gap-y-1 text-sm">
        <span class="inline-flex h-2 w-2 rounded-full bg-signal"></span>
        <span class="text-muted-foreground">{{ t('admin.dashboard.adminAuth.loggedInAs', { identity: adminIdentity }) }}</span>
        <span class="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground">
          <span class="text-foreground/70">{{ t('admin.dashboard.adminAuth.expiresAt') }}</span>
          <span class="font-medium text-foreground/90">{{ adminExpiry }}</span>
        </span>
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:border-primary/40 hover:text-primary disabled:cursor-not-allowed disabled:opacity-60"
        :disabled="adminRefreshingCredentials"
        @click="updateAdminCredentials"
      >
        <RefreshCw class="h-4 w-4" :class="{ 'animate-spin': adminRefreshingCredentials }" />
        {{ adminRefreshingCredentials ? t('admin.dashboard.adminAuth.updatingCredentials') : t('admin.dashboard.adminAuth.updateCredentials') }}
      </button>
    </div>
    <div
      v-else-if="!adminModalOpen && !adminStatus.authenticated"
      class="flex flex-wrap items-center justify-between gap-3 rounded-2xl border border-warning/30 bg-warning/5 px-5 py-3 shadow-sm"
    >
      <div class="flex items-center gap-2 text-sm">
        <span class="inline-flex h-2 w-2 rounded-full bg-warning"></span>
        <span class="text-muted-foreground">{{ t('admin.dashboard.adminAuth.notLoggedIn') }}</span>
      </div>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        @click="openAdminModal"
      >
        <ShieldCheck class="h-4 w-4" />
        {{ t('admin.dashboard.adminAuth.login') }}
      </button>
    </div>

    <!-- 数据门禁：仅在已登录 admin 时展示指标与趋势，未登录则显示提示登录的空状态 -->
    <template v-if="adminStatus.authenticated">

    <!-- 加载失败提示（弹窗关闭后仍无数据时显示） -->
    <div
      v-if="!loadingModalOpen && metrics.length === 0"
      class="flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed border-red-300/40 bg-red-50/5 px-6 py-16 text-center dark:border-red-500/20 dark:bg-red-950/10"
    >
      <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.loadError') }}</p>
      <button
        type="button"
        class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        @click="loadAllData"
      >
        <RefreshCw class="h-4 w-4" />
        {{ t('admin.dashboard.retry') }}
      </button>
    </div>

    <!-- 核心指标卡片 -->
    <template v-else-if="metrics.length > 0">
    <section class="grid gap-6 md:grid-cols-2 xl:grid-cols-6">
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
        @click="handleMetricCardClick(card.key)"
      />
      <StatCard
        :label="t('admin.dashboard.metrics.groupCount')"
        :value="groupCount != null ? String(groupCount) : '—'"
        :icon="Layers"
        color="accent"
        delta-direction="flat"
        delta-text=""
        :delta-caption="t('admin.dashboard.metrics.groupCountCaption')"
        :clickable="true"
        @click="openGroupList()"
      />
    </section>

    <!-- 趋势统计 -->
    <section class="space-y-5">
      <div class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <h2 class="text-lg font-semibold text-foreground">{{ t('admin.dashboard.charts.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.dashboard.charts.subtitle') }}</p>
        </div>
        <div
          class="inline-flex items-center rounded-xl border border-border/60 bg-surface/40 p-1"
          role="group"
          :aria-label="t('admin.dashboard.period.label')"
        >
          <button
            v-for="item in periods"
            :key="item"
            type="button"
            class="px-4 py-1.5 text-sm font-medium rounded-lg transition-colors"
            :class="period === item
              ? 'bg-primary text-primary-foreground shadow-sm'
              : 'text-muted-foreground hover:text-foreground'"
            @click="period = item"
          >
            {{ t(`admin.dashboard.period.${item}`) }}
          </button>
        </div>
      </div>

      <div class="grid gap-6 md:grid-cols-2 xl:grid-cols-3">
        <MetricTrendCard
          v-for="chart in charts"
          :key="chart.key"
          :title="chart.title"
          :value="chart.value"
          :color="chart.color"
          :values="chart.values"
          :labels="chart.labels"
          :delta-direction="chart.deltaDirection"
          :delta-text="chart.deltaText"
          :delta-caption="deltaCaption"
        />
      </div>
    </section>
    </template>
    </template>

    <!-- 未登录 admin 时的数据门禁空状态：不展示任何统计数据，引导先登录 -->
    <div
      v-else-if="!adminModalOpen"
      class="flex flex-col items-center justify-center gap-4 rounded-2xl border border-dashed border-border/60 bg-card/40 px-6 py-16 text-center"
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
        class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
        @click="openAdminModal"
      >
        <ShieldCheck class="h-4 w-4" />
        {{ t('admin.dashboard.adminAuth.login') }}
      </button>
    </div>

    <!-- admin 登录弹窗：未登录时进入仪表盘自动打开；更新凭证校验失败时也会自动打开并预填非敏感字段 -->
    <AdminLoginModal
      :open="adminModalOpen"
      :submitting="adminSubmitting"
      :error-key="adminErrorKey"
      :initial-value="adminLoginInitialValue"
      @submit="submitAdminLogin"
      @close="closeAdminModal"
    />

    <!-- 站点用户余额筛选配置弹窗 -->
    <BalanceFilterModal
      :open="balanceFilterOpen"
      @close="closeBalanceFilter"
      @saved="onBalanceFilterSaved"
    />

    <!-- 今日营收分组明细弹窗 -->
    <GroupUsageTodayModal
      :open="groupUsageTodayOpen"
      @close="closeGroupUsageToday"
    />

    <!-- 今日成本（上游 key 消费）下钻弹窗 -->
    <UpstreamKeyUsageTodayModal
      :open="upstreamKeyUsageTodayOpen"
      @close="closeUpstreamKeyUsageToday"
    />

    <!-- 上游总余额下钻弹窗 -->
    <UpstreamBalanceBreakdownModal
      :open="upstreamBalanceBreakdownOpen"
      @close="closeUpstreamBalanceBreakdown"
    />

    <!-- 初始数据加载进度弹窗 -->
    <DashboardLoadingModal
      :open="loadingModalOpen"
      :steps="loadingSteps"
    />
  </div>
</template>
