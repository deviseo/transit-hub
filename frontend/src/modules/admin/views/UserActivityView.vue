<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { AlertCircle, ArrowDown, ArrowUp, ArrowUpDown, CheckCircle2, Database, Info, Loader2, Mail, MailX, RefreshCw, Search, Table2, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import ActivityTrendChart from '../components/user-activity/ActivityTrendChart.vue'
import DeltaBadge from '../components/user-activity/DeltaBadge.vue'
import PeriodComparisonCard from '../components/user-activity/PeriodComparisonCard.vue'
import SegmentEmailDialog from '../components/user-activity/SegmentEmailDialog.vue'
import SuppressionListDialog from '../components/user-activity/SuppressionListDialog.vue'
import UserActivityDetailDrawer from '../components/user-activity/UserActivityDetailDrawer.vue'
import { getUserActivitySummary, listUserActivityUsers, triggerUserActivitySync } from '../api/userActivity'
import { useUserActivityFormat } from '../composables/useUserActivityFormat'
import type { MassEmailBatch } from '../types/massEmail'
import type {
  UserActivityFilter,
  UserActivityPeriodKey,
  UserActivityRoleFilter,
  UserActivityRow,
  UserActivitySegment,
  UserActivitySortKey,
  UserActivityStatusFilter,
  UserActivitySummary,
} from '../types/userActivity'

const { t, te } = useI18n()
const {
  formatCount, formatCompact, formatUsd, formatUsdAxis, formatSignedUsd, formatPercent, formatDay, formatShortDay, formatRange, formatDateTime, formatRelativeTime,
} = useUserActivityFormat()

const summary = ref<UserActivitySummary | null>(null)
const users = ref<UserActivityRow[]>([])
const totalUsers = ref(0)
const page = ref(1)
const pages = ref(1)
const pageSize = 20

const segment = ref<UserActivitySegment>('all')
const inactiveDays = ref(3)
const statusFilter = ref<UserActivityStatusFilter>('active')
const roleFilter = ref<UserActivityRoleFilter>('')
const searchDraft = ref('')
const search = ref('')
const sortKey = ref<UserActivitySortKey>('lastUsedAt')
const sortOrder = ref<'asc' | 'desc'>('desc')
const trendDays = ref(30)
const showTrendTable = ref(false)

const isLoadingSummary = ref(false)
const isLoadingUsers = ref(false)
const isStartingSync = ref(false)
const errorKey = ref('')
const listErrorKey = ref('')
const createdBatch = ref<MassEmailBatch | null>(null)
const isEmailDialogOpen = ref(false)
const isSuppressionsOpen = ref(false)
const detailUserId = ref<string | null>(null)
const listSection = ref<HTMLElement | null>(null)

let pollTimer: number | undefined
let usersSequence = 0
let summarySequence = 0

const periodKeys: UserActivityPeriodKey[] = ['day', 'week', 'month']
const trendRanges = [30, 60, 90] as const
const segmentGroups: Array<{ labelKey: string; segments: UserActivitySegment[] }> = [
  { labelKey: 'admin.userActivity.segmentGroups.basic', segments: ['all'] },
  { labelKey: 'admin.userActivity.segmentGroups.active', segments: ['active_1d', 'active_7d', 'active_30d'] },
  { labelKey: 'admin.userActivity.segmentGroups.inactive', segments: ['inactive_2d', 'inactive_7d', 'inactive_14d', 'inactive_30d', 'inactive_60d', 'inactive_90d', 'inactive', 'no_usage'] },
  { labelKey: 'admin.userActivity.segmentGroups.change', segments: ['churned_7d', 'declining', 'growing'] },
  { labelKey: 'admin.userActivity.segmentGroups.money', segments: ['recharged_today', 'low_balance'] },
]
const bucketSegments: Record<number, UserActivitySegment> = {
  2: 'inactive_2d', 7: 'inactive_7d', 14: 'inactive_14d', 30: 'inactive_30d', 60: 'inactive_60d', 90: 'inactive_90d',
}

const sync = computed(() => summary.value?.sync)
const platform = computed(() => sync.value?.platform ?? '')
const isSyncRunning = computed(() => sync.value?.status === 'running')
const hasData = computed(() => Boolean(summary.value?.hasData))
const today = computed(() => summary.value?.today.from ?? '')
const eligibleUsers = computed(() => Math.max(0, (summary.value?.totalUsers ?? 0) - (summary.value?.disabled ?? 0)))
const lifetimeSortKey = computed<UserActivitySortKey>(() => (platform.value === 'newapi' ? 'totalUsed' : 'totalRecharged'))
const lifetimeLabel = computed(() => (platform.value === 'newapi' ? t('admin.userActivity.fields.totalUsed') : t('admin.userActivity.fields.totalRecharged')))

interface SortColumn {
  key: UserActivitySortKey
  label: string
  align: 'left' | 'right'
}

const sortColumns = computed<SortColumn[]>(() => [
  { key: 'lastUsedAt', label: t('admin.userActivity.fields.lastUsed'), align: 'left' },
  { key: 'costToday', label: t('admin.userActivity.fields.costToday'), align: 'right' },
  { key: 'cost7d', label: t('admin.userActivity.fields.cost7d'), align: 'right' },
  { key: 'change7d', label: t('admin.userActivity.fields.change7d'), align: 'right' },
  { key: 'cost30d', label: t('admin.userActivity.fields.cost30d'), align: 'right' },
  { key: 'activeDays30d', label: t('admin.userActivity.fields.activeDays30d'), align: 'right' },
  { key: 'rechargeToday', label: t('admin.userActivity.fields.rechargeToday'), align: 'right' },
  { key: 'recharge30d', label: t('admin.userActivity.fields.recharge30d'), align: 'right' },
  { key: 'balance', label: t('admin.userActivity.fields.balance'), align: 'right' },
  { key: lifetimeSortKey.value, label: lifetimeLabel.value, align: 'right' },
  { key: 'createdAt', label: t('admin.userActivity.fields.createdAt'), align: 'left' },
])

// 趋势只画完整自然日（截至昨天）；今天的数据在「今日」卡片中单独展示。
const completeDaily = computed(() => (summary.value?.daily ?? []).filter((point) => !today.value || point.day < today.value).slice(-trendDays.value))
const trendDaysList = computed(() => completeDaily.value.map((point) => point.day))
const activeUserValues = computed(() => completeDaily.value.map((point) => point.activeUsers))
const costValues = computed(() => completeDaily.value.map((point) => point.cost))
const rechargeValues = computed(() => completeDaily.value.map((point) => point.recharge))

const money = computed(() => summary.value?.money)
const hasRechargeData = computed(() => Boolean(money.value?.rechargeFrom))
// 渠道明细：入账在前（按金额从大到小），扣减在后。
const rechargeChannels = computed(() => {
  const channels = [...(money.value?.todayChannels ?? [])]
  return channels.sort((a, b) => (Math.sign(b.amount) - Math.sign(a.amount)) || (Math.abs(b.amount) - Math.abs(a.amount)))
})
const balanceChange = computed(() => {
  const value = money.value
  if (!value || value.balanceYesterday == null) return null
  return value.totalBalance - value.balanceYesterday
})
const moneyMethodHint = computed(() => {
  const value = money.value
  if (!value || !value.method) return ''
  if (value.method === 'snapshot') {
    return t('admin.userActivity.money.methodSnapshot', { time: value.rechargeSince ? formatDateTime(value.rechargeSince) : t('admin.userActivity.common.placeholder') })
  }
  return t('admin.userActivity.money.methodRecords')
})
const channelLabel = (channel: string): string => (te(`admin.userActivity.money.channels.${channel}`) ? t(`admin.userActivity.money.channels.${channel}`) : channel)
const latestComplete = computed(() => completeDaily.value[completeDaily.value.length - 1])
const trendTableRows = computed(() => [...completeDaily.value].reverse())

const currentFilter = computed<UserActivityFilter>(() => ({
  segment: segment.value,
  inactiveDays: segment.value === 'inactive' ? inactiveDays.value : undefined,
  search: search.value,
  status: statusFilter.value,
  role: roleFilter.value,
}))

const filterSummary = computed(() => {
  const parts = [segment.value === 'inactive'
    ? t('admin.userActivity.segments.inactiveCustom', { days: inactiveDays.value })
    : t(`admin.userActivity.segments.${segment.value}`)]
  parts.push(statusFilter.value ? t(`admin.userActivity.userStatus.${statusFilter.value}`) : t('admin.userActivity.filters.allStatuses'))
  parts.push(roleFilter.value ? t(`admin.userActivity.roles.${roleFilter.value}`) : t('admin.userActivity.filters.allRoles'))
  if (search.value) parts.push(t('admin.userActivity.filters.searchSummary', { search: search.value }))
  return parts.join(' · ')
})

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.userActivity.errors.request'))
const isAuthError = computed(() => errorKey.value === 'admin.userActivity.errors.authRequired' || sync.value?.errorKey === 'admin.userActivity.errors.authRequired')

const syncDescription = computed(() => {
  const state = sync.value
  if (!state || !state.status) return t('admin.userActivity.sync.never')
  const parts: string[] = []
  if (state.lastSyncedAt) parts.push(t('admin.userActivity.sync.lastSynced', { time: formatRelativeTime(state.lastSyncedAt) }))
  if (state.syncedFrom) parts.push(t('admin.userActivity.sync.coverage', { range: formatRange(state.syncedFrom, state.syncedTo) }))
  parts.push(t('admin.userActivity.sync.users', { count: formatCount(state.userCount) }))
  return parts.join(' · ')
})

const lastUsedText = (row: UserActivityRow): string => {
  if (row.daysSinceLastUse != null) {
    return row.daysSinceLastUse === 0 ? t('admin.userActivity.lastUsed.today') : t('admin.userActivity.lastUsed.daysAgo', { count: row.daysSinceLastUse }, row.daysSinceLastUse)
  }
  return row.everUsed ? t('admin.userActivity.lastUsed.longAgo') : t('admin.userActivity.lastUsed.never')
}

const lifetimeValue = (row: UserActivityRow): number | null => (platform.value === 'newapi' ? row.totalUsed : row.totalRecharged)

const loadSummary = async () => {
  const sequence = ++summarySequence
  isLoadingSummary.value = true
  try {
    // 多取一天：最后一天是今天（进行中），图表只用完整自然日。
    const result = await getUserActivitySummary(trendRanges[trendRanges.length - 1] + 1)
    if (sequence !== summarySequence) return
    summary.value = result
    if (errorKey.value && errorKey.value !== 'admin.userActivity.errors.authRequired') errorKey.value = ''
  } catch (error) {
    if (sequence === summarySequence) errorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
  } finally {
    if (sequence === summarySequence) isLoadingSummary.value = false
  }
}

const loadUsers = async () => {
  const sequence = ++usersSequence
  isLoadingUsers.value = true
  listErrorKey.value = ''
  try {
    const result = await listUserActivityUsers({
      ...currentFilter.value,
      sort: sortKey.value,
      order: sortOrder.value,
      page: page.value,
      pageSize,
    })
    if (sequence !== usersSequence) return
    users.value = result.items
    totalUsers.value = result.total
    pages.value = Math.max(1, result.pages)
  } catch (error) {
    if (sequence === usersSequence) listErrorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
  } finally {
    if (sequence === usersSequence) isLoadingUsers.value = false
  }
}

const refreshAll = async () => {
  await loadSummary()
  if (hasData.value) await loadUsers()
  syncPolling()
}

const startSync = async () => {
  if (isStartingSync.value) return
  isStartingSync.value = true
  errorKey.value = ''
  try {
    const status = await triggerUserActivitySync()
    if (summary.value) summary.value = { ...summary.value, sync: status }
    else await loadSummary()
    syncPolling()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
  } finally {
    isStartingSync.value = false
  }
}

const stopPolling = () => {
  if (pollTimer === undefined) return
  window.clearInterval(pollTimer)
  pollTimer = undefined
}

// 同步在后台进行：期间每 4 秒刷新概览，结束后刷新用户列表。
const syncPolling = () => {
  if (!isSyncRunning.value) {
    stopPolling()
    return
  }
  if (pollTimer !== undefined) return
  pollTimer = window.setInterval(async () => {
    const hadData = hasData.value
    await loadSummary()
    if (!isSyncRunning.value) {
      stopPolling()
      await loadUsers()
    } else if (!hadData && hasData.value) {
      await loadUsers()
    }
  }, 4000)
}

const applyFilters = () => {
  page.value = 1
  void loadUsers()
}

const applySearch = () => {
  search.value = searchDraft.value.trim()
  applyFilters()
}

const clearSearch = () => {
  searchDraft.value = ''
  if (!search.value) return
  search.value = ''
  applyFilters()
}

const selectSegment = (value: UserActivitySegment) => {
  segment.value = value
  statusFilter.value = 'active'
  applyFilters()
  listSection.value?.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

const toggleSort = (key: UserActivitySortKey) => {
  if (sortKey.value === key) {
    sortOrder.value = sortOrder.value === 'desc' ? 'asc' : 'desc'
  } else {
    sortKey.value = key
    sortOrder.value = 'desc'
  }
  applyFilters()
}

const ariaSort = (key: UserActivitySortKey): 'ascending' | 'descending' | 'none' => {
  if (sortKey.value !== key) return 'none'
  return sortOrder.value === 'asc' ? 'ascending' : 'descending'
}

const changePage = (target: number) => {
  if (target < 1 || target > pages.value) return
  page.value = target
  void loadUsers()
}

const onBatchCreated = (batch: MassEmailBatch) => {
  isEmailDialogOpen.value = false
  createdBatch.value = batch
}

const onSuppressionsChanged = () => {
  void loadSummary()
}

onMounted(() => {
  void refreshAll()
})

onBeforeUnmount(stopPolling)
</script>

<template>
  <div class="flex flex-col gap-3">
    <!-- 同步状态 -->
    <section class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
        <div class="min-w-0">
          <h2 class="text-base font-semibold text-foreground">{{ t('admin.userActivity.sync.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.userActivity.subtitle') }}</p>
          <p class="mt-2 flex flex-wrap items-center gap-2 text-xs text-muted-foreground" aria-live="polite">
            <span v-if="platform" class="rounded-md bg-surface px-2 py-0.5 font-medium text-foreground">{{ t(`admin.userActivity.platforms.${platform}`) }}</span>
            <span v-if="isSyncRunning" class="inline-flex items-center gap-1 text-primary">
              <Loader2 class="h-3.5 w-3.5 animate-spin" />
              {{ hasData ? t('admin.userActivity.sync.runningBackfill') : t('admin.userActivity.sync.running') }}
            </span>
            <span>{{ syncDescription }}</span>
          </p>
        </div>
        <div class="flex shrink-0 flex-wrap gap-2">
          <Button variant="secondary" class="rounded-lg" :disabled="isLoadingSummary || isLoadingUsers" @click="refreshAll">
            <Loader2 v-if="isLoadingSummary || isLoadingUsers" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('admin.userActivity.actions.refresh') }}
          </Button>
          <Button class="rounded-lg" :disabled="isStartingSync || isSyncRunning" @click="startSync">
            <Loader2 v-if="isStartingSync || isSyncRunning" class="h-4 w-4 animate-spin" />
            <Database v-else class="h-4 w-4" />
            {{ hasData ? t('admin.userActivity.actions.syncNow') : t('admin.userActivity.actions.startSync') }}
          </Button>
        </div>
      </div>
    </section>

    <div v-if="errorKey" class="flex items-start gap-3 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>
        {{ errorText(errorKey) }}
        <RouterLink v-if="isAuthError" to="/admin" class="ml-1 font-medium underline underline-offset-2">{{ t('admin.userActivity.actions.goDashboard') }}</RouterLink>
      </span>
    </div>
    <div v-else-if="sync?.status === 'failed' && sync.errorKey" class="flex items-start gap-3 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>
        {{ t('admin.userActivity.sync.failed', { reason: errorText(sync.errorKey) }) }}
        <RouterLink v-if="isAuthError" to="/admin" class="ml-1 font-medium underline underline-offset-2">{{ t('admin.userActivity.actions.goDashboard') }}</RouterLink>
      </span>
    </div>
    <div v-if="sync?.warningKey && sync.status !== 'failed'" class="flex items-start gap-3 rounded-lg border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300" role="status">
      <Info class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ errorText(sync.warningKey) }}</span>
    </div>
    <div v-if="createdBatch" class="flex items-start gap-3 rounded-lg border border-emerald-400/20 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-300" role="status">
      <CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0" />
      <span class="min-w-0 flex-1">
        {{ t('admin.userActivity.email.created', { count: formatCount(createdBatch.recipientCount), skipped: formatCount(createdBatch.skippedCount) }) }}
        <RouterLink to="/admin/mass-email" class="ml-1 font-medium underline underline-offset-2">{{ t('admin.userActivity.email.viewProgress') }}</RouterLink>
      </span>
      <button type="button" class="rounded p-0.5 hover:bg-emerald-500/10" :aria-label="t('admin.userActivity.actions.close')" @click="createdBatch = null">
        <X class="h-4 w-4" />
      </button>
    </div>

    <!-- 首次使用 -->
    <section v-if="summary && !hasData" class="flex flex-col items-center rounded-lg border border-dashed border-border/70 bg-card px-6 py-12 text-center">
      <Database class="mb-3 h-10 w-10 text-muted-foreground/60" />
      <h2 class="text-base font-semibold text-foreground">
        {{ isSyncRunning ? t('admin.userActivity.empty.runningTitle') : t('admin.userActivity.empty.title') }}
      </h2>
      <p class="mt-2 max-w-xl text-sm text-muted-foreground">
        {{ isSyncRunning ? t('admin.userActivity.empty.runningDescription') : t('admin.userActivity.empty.description') }}
      </p>
      <Button v-if="!isSyncRunning" class="mt-4 rounded-lg" :disabled="isStartingSync" @click="startSync">
        <Loader2 v-if="isStartingSync" class="h-4 w-4 animate-spin" />
        <Database v-else class="h-4 w-4" />
        {{ t('admin.userActivity.actions.startSync') }}
      </Button>
    </section>
    <div v-else-if="!summary && isLoadingSummary" class="flex items-center justify-center rounded-lg border border-border/50 bg-card py-16 text-sm text-muted-foreground" role="status">
      <Loader2 class="mr-2 h-4 w-4 animate-spin" />
      {{ t('admin.userActivity.status.loading') }}
    </div>

    <template v-if="summary && hasData">
      <!-- 资金概览 -->
      <section v-if="money" class="rounded-lg border border-border/50 bg-card p-4 shadow-sm" :class="{ 'opacity-60 transition-opacity': isLoadingSummary }">
        <div class="flex flex-wrap items-baseline justify-between gap-2">
          <h2 class="text-sm font-semibold text-foreground">{{ t('admin.userActivity.money.title') }}</h2>
          <p class="text-xs text-muted-foreground">{{ moneyMethodHint }}</p>
        </div>
        <div class="mt-3 grid gap-3 sm:grid-cols-2 xl:grid-cols-5">
          <article class="rounded-lg bg-surface p-3 sm:col-span-2">
            <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.money.todayRecharge') }}</p>
            <p class="mt-1 flex flex-wrap items-baseline gap-x-3 gap-y-1">
              <span class="text-2xl font-semibold text-foreground">{{ hasRechargeData ? formatUsd(money.todayRecharge) : t('admin.userActivity.common.placeholder') }}</span>
              <span v-if="hasRechargeData && money.todayDeduction > 0" class="text-xs text-muted-foreground">
                {{ t('admin.userActivity.money.deduction', { value: formatUsd(money.todayDeduction) }) }}
              </span>
              <span v-if="money.yesterdayRecharge != null" class="text-xs text-muted-foreground">
                {{ t('admin.userActivity.money.yesterdayFull', { value: formatUsd(money.yesterdayRecharge) }) }}
              </span>
            </p>
            <ul v-if="hasRechargeData && rechargeChannels.length" class="mt-2 grid gap-x-6 gap-y-1 text-xs sm:grid-cols-2">
              <li v-for="item in rechargeChannels" :key="item.channel" class="flex items-baseline justify-between gap-2">
                <span class="text-muted-foreground">{{ channelLabel(item.channel) }}</span>
                <span class="whitespace-nowrap tabular-nums text-foreground">
                  {{ item.amount < 0 ? formatSignedUsd(item.amount) : formatUsd(item.amount) }}
                  <span class="text-muted-foreground">· {{ t('admin.userActivity.money.events', { count: formatCount(item.events) }, item.events) }}</span>
                </span>
              </li>
            </ul>
            <p v-else-if="hasRechargeData" class="mt-2 text-xs text-muted-foreground">{{ t('admin.userActivity.money.noRechargeToday') }}</p>
            <p v-else class="mt-2 text-xs text-muted-foreground">{{ t('admin.userActivity.money.noRechargeData') }}</p>
            <p v-if="hasRechargeData && !money.rechargeUpToDate" class="mt-1 text-xs text-amber-700 dark:text-amber-300">{{ t('admin.userActivity.money.notUpToDate') }}</p>
          </article>
          <article class="rounded-lg bg-surface p-3">
            <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.money.todayCost') }}</p>
            <p class="mt-1 text-2xl font-semibold text-foreground">{{ formatUsd(money.todayCost) }}</p>
            <p class="mt-1 text-xs text-muted-foreground">
              {{ money.yesterdayCost != null ? t('admin.userActivity.money.yesterdayFull', { value: formatUsd(money.yesterdayCost) }) : t('admin.userActivity.common.placeholder') }}
            </p>
          </article>
          <article class="rounded-lg bg-surface p-3">
            <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.money.totalBalance') }}</p>
            <p class="mt-1 text-2xl font-semibold text-foreground">{{ formatUsd(money.totalBalance) }}</p>
            <p class="mt-1 text-xs text-muted-foreground">
              <template v-if="balanceChange != null">{{ t('admin.userActivity.money.balanceChange', { value: formatSignedUsd(balanceChange) }) }} · </template>
              {{ t('admin.userActivity.money.usersWithBalance', { count: formatCount(money.usersWithBalance) }) }}
            </p>
            <p v-if="money.lifetimeRecharged != null" class="mt-0.5 text-xs text-muted-foreground">
              {{ t('admin.userActivity.money.lifetimeRecharged', { value: formatUsd(money.lifetimeRecharged) }) }}
            </p>
          </article>
          <div class="grid gap-3">
            <button
              type="button"
              class="rounded-lg bg-surface p-3 text-left transition-colors hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :disabled="!hasRechargeData"
              @click="selectSegment('recharged_today')"
            >
              <span class="block text-xs text-muted-foreground">{{ t('admin.userActivity.money.rechargeUsers') }}</span>
              <span class="mt-0.5 block text-lg font-semibold text-foreground">{{ hasRechargeData ? formatCount(money.todayRechargeUsers) : t('admin.userActivity.common.placeholder') }}</span>
            </button>
            <button
              type="button"
              class="rounded-lg bg-surface p-3 text-left transition-colors hover:bg-primary/5 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              @click="selectSegment('low_balance')"
            >
              <span class="block text-xs text-muted-foreground">{{ t('admin.userActivity.money.lowBalance') }}</span>
              <span class="mt-0.5 block text-lg font-semibold text-foreground">{{ formatCount(money.lowBalanceActive) }}</span>
            </button>
          </div>
        </div>
      </section>

      <!-- 概览 -->
      <section class="grid gap-3 sm:grid-cols-2 xl:grid-cols-5" :class="{ 'opacity-60 transition-opacity': isLoadingSummary }">
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.kpi.totalUsers') }}</p>
          <p class="mt-1 text-2xl font-semibold text-foreground">{{ formatCount(summary.totalUsers) }}</p>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ t('admin.userActivity.kpi.totalUsersHint', { withEmail: formatCount(summary.withEmail), disabled: formatCount(summary.disabled) }) }}
          </p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.kpi.activeToday') }}</p>
          <p class="mt-1 text-2xl font-semibold text-foreground">{{ formatCount(summary.today.activeUsers) }}</p>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ t('admin.userActivity.kpi.todayHint', { requests: formatCount(summary.today.requests), cost: formatUsd(summary.today.cost) }) }}
          </p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.kpi.active7d') }}</p>
          <p class="mt-1 flex items-baseline gap-2">
            <span class="text-2xl font-semibold text-foreground">{{ formatCount(summary.periods.week?.current.activeUsers) }}</span>
            <DeltaBadge :current="summary.periods.week?.current.activeUsers ?? 0" :previous="summary.periods.week?.previous?.activeUsers" :missing-hint="t('admin.userActivity.periods.previousMissing')" />
          </p>
          <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.userActivity.kpi.vsPrevious7d') }}</p>
        </article>
        <article class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
          <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.kpi.active30d') }}</p>
          <p class="mt-1 flex items-baseline gap-2">
            <span class="text-2xl font-semibold text-foreground">{{ formatCount(summary.periods.month?.current.activeUsers) }}</span>
            <DeltaBadge :current="summary.periods.month?.current.activeUsers ?? 0" :previous="summary.periods.month?.previous?.activeUsers" :missing-hint="t('admin.userActivity.periods.previousMissing')" />
          </p>
          <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.userActivity.kpi.vsPrevious30d') }}</p>
        </article>
        <button
          type="button"
          class="rounded-lg border border-border/50 bg-card p-4 text-left shadow-sm transition-colors hover:border-primary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          @click="selectSegment('no_usage')"
        >
          <span class="block text-xs text-muted-foreground">{{ t('admin.userActivity.kpi.noUsage') }}</span>
          <span class="mt-1 block text-2xl font-semibold text-foreground">{{ formatCount(summary.noUsage) }}</span>
          <span class="mt-1 block text-xs text-muted-foreground">{{ t('admin.userActivity.kpi.noUsageHint') }}</span>
        </button>
      </section>

      <!-- 未活跃分布 -->
      <section class="rounded-lg border border-border/50 bg-card p-4 shadow-sm" :class="{ 'opacity-60 transition-opacity': isLoadingSummary }">
        <div class="flex flex-wrap items-baseline justify-between gap-2">
          <h2 class="text-sm font-semibold text-foreground">{{ t('admin.userActivity.inactive.title') }}</h2>
          <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.inactive.hint', { count: formatCount(eligibleUsers) }) }}</p>
        </div>
        <div class="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3 xl:grid-cols-6">
          <button
            v-for="bucket in summary.inactive"
            :key="bucket.days"
            type="button"
            class="rounded-lg border p-3 text-left transition-colors hover:border-primary/40 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :class="segment === bucketSegments[bucket.days] ? 'border-primary/60 bg-primary/5' : 'border-border/50 bg-surface'"
            :aria-pressed="segment === bucketSegments[bucket.days]"
            @click="selectSegment(bucketSegments[bucket.days])"
          >
            <span class="block text-xs text-muted-foreground">{{ t('admin.userActivity.inactive.bucket', { days: bucket.days }) }}</span>
            <span class="mt-1 flex items-baseline gap-1.5">
              <span class="text-lg font-semibold text-foreground">{{ formatCount(bucket.count) }}</span>
              <span class="text-xs text-muted-foreground">{{ eligibleUsers ? formatPercent(bucket.count / eligibleUsers) : '' }}</span>
            </span>
            <span class="mt-2 block h-1 overflow-hidden rounded-full bg-primary/15" aria-hidden="true">
              <span class="block h-full rounded-full bg-primary" :style="{ width: `${eligibleUsers ? Math.min(100, (bucket.count / eligibleUsers) * 100) : 0}%` }" />
            </span>
          </button>
        </div>
      </section>

      <!-- 日 / 周 / 月对比 -->
      <section class="grid gap-3 lg:grid-cols-3" :class="{ 'opacity-60 transition-opacity': isLoadingSummary }">
        <PeriodComparisonCard v-for="key in periodKeys" :key="key" :period-key="key" :comparison="summary.periods[key]" />
      </section>

      <!-- 每日趋势 -->
      <section class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-1 rounded-lg bg-surface p-1" role="group" :aria-label="t('admin.userActivity.trend.range')">
            <button
              v-for="range in trendRanges"
              :key="range"
              type="button"
              class="rounded-md px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="trendDays === range ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
              :aria-pressed="trendDays === range"
              @click="trendDays = range"
            >
              {{ t('admin.userActivity.trend.lastDays', { days: range }) }}
            </button>
          </div>
          <div class="flex items-center gap-3">
            <span class="text-xs text-muted-foreground">{{ t('admin.userActivity.trend.completeDays') }}</span>
            <button
              type="button"
              class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :aria-pressed="showTrendTable"
              @click="showTrendTable = !showTrendTable"
            >
              <Table2 class="h-3.5 w-3.5" />
              {{ showTrendTable ? t('admin.userActivity.trend.showChart') : t('admin.userActivity.trend.showTable') }}
            </button>
          </div>
        </div>

        <div v-if="!showTrendTable" class="mt-4 grid gap-4 lg:grid-cols-2 2xl:grid-cols-3" :class="{ 'opacity-60 transition-opacity': isLoadingSummary }">
          <div class="min-w-0">
            <div class="flex items-baseline justify-between gap-2">
              <h3 class="text-sm font-semibold text-foreground">{{ t('admin.userActivity.trend.activeUsersTitle') }}</h3>
              <span v-if="latestComplete" class="text-xs text-muted-foreground">
                {{ t('admin.userActivity.trend.latest', { day: formatShortDay(latestComplete.day), value: formatCount(latestComplete.activeUsers) }) }}
              </span>
            </div>
            <div class="mt-2 h-56">
              <ActivityTrendChart
                :days="trendDaysList"
                :values="activeUserValues"
                integer
                :accessible-label="t('admin.userActivity.trend.activeUsersAria', { days: trendDays })"
                :format-value="(value) => t('admin.userActivity.trend.usersValue', { count: formatCount(value) })"
                :format-axis="formatCompact"
                :format-day="formatDay"
                :format-tick="formatShortDay"
              />
            </div>
          </div>
          <div class="min-w-0">
            <div class="flex items-baseline justify-between gap-2">
              <h3 class="text-sm font-semibold text-foreground">{{ t('admin.userActivity.trend.costTitle') }}</h3>
              <span v-if="latestComplete" class="text-xs text-muted-foreground">
                {{ t('admin.userActivity.trend.latest', { day: formatShortDay(latestComplete.day), value: formatUsd(latestComplete.cost) }) }}
              </span>
            </div>
            <div class="mt-2 h-56">
              <ActivityTrendChart
                :days="trendDaysList"
                :values="costValues"
                :accessible-label="t('admin.userActivity.trend.costAria', { days: trendDays })"
                :format-value="(value) => formatUsd(value)"
                :format-axis="formatUsdAxis"
                :format-day="formatDay"
                :format-tick="formatShortDay"
              />
            </div>
          </div>
          <div class="min-w-0 lg:col-span-2 2xl:col-span-1">
            <div class="flex items-baseline justify-between gap-2">
              <h3 class="text-sm font-semibold text-foreground">{{ t('admin.userActivity.trend.rechargeTitle') }}</h3>
              <span v-if="latestComplete" class="text-xs text-muted-foreground">
                {{ t('admin.userActivity.trend.latest', { day: formatShortDay(latestComplete.day), value: latestComplete.recharge == null ? t('admin.userActivity.common.placeholder') : formatUsd(latestComplete.recharge) }) }}
              </span>
            </div>
            <div class="mt-2 h-56">
              <ActivityTrendChart
                :days="trendDaysList"
                :values="rechargeValues"
                :accessible-label="t('admin.userActivity.trend.rechargeAria', { days: trendDays })"
                :format-value="(value) => formatUsd(value)"
                :format-axis="formatUsdAxis"
                :format-day="formatDay"
                :format-tick="formatShortDay"
                :no-data-label="t('admin.userActivity.trend.noData')"
              />
            </div>
          </div>
        </div>
        <div v-else class="mt-4 max-h-96 overflow-auto rounded-lg border border-border/50">
          <table class="w-full text-left text-sm">
            <thead class="sticky top-0 bg-surface text-xs text-muted-foreground">
              <tr>
                <th class="px-3 py-2 font-medium">{{ t('admin.userActivity.trend.day') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.activeUsers') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.requests') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.tokens') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.cost') }}</th>
                <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.recharge') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/40 tabular-nums">
              <tr v-for="point in trendTableRows" :key="point.day">
                <td class="px-3 py-1.5 text-muted-foreground">{{ formatDay(point.day) }}</td>
                <td class="px-3 py-1.5 text-right text-foreground">{{ formatCount(point.activeUsers) }}</td>
                <td class="px-3 py-1.5 text-right text-foreground">{{ formatCount(point.requests) }}</td>
                <td class="px-3 py-1.5 text-right text-foreground">{{ formatCount(point.tokens) }}</td>
                <td class="px-3 py-1.5 text-right text-foreground">{{ formatUsd(point.cost) }}</td>
                <td class="px-3 py-1.5 text-right text-foreground">{{ point.recharge == null ? t('admin.userActivity.common.placeholder') : formatUsd(point.recharge) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <!-- 用户列表 -->
      <section ref="listSection" class="scroll-mt-4 rounded-lg border border-border/50 bg-card shadow-sm">
        <div class="flex flex-col gap-3 border-b border-border/50 p-4">
          <div class="flex flex-wrap items-center justify-between gap-2">
            <div>
              <h2 class="text-base font-semibold text-foreground">{{ t('admin.userActivity.users.title') }}</h2>
              <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.users.subtitle') }}</p>
            </div>
            <div class="flex flex-wrap gap-2">
              <Button variant="secondary" class="rounded-lg" @click="isSuppressionsOpen = true">
                <MailX class="h-4 w-4" />
                {{ t('admin.userActivity.actions.suppressions', { count: formatCount(summary.suppressions) }) }}
              </Button>
              <Button class="rounded-lg" :disabled="totalUsers === 0 || isLoadingUsers" @click="isEmailDialogOpen = true">
                <Mail class="h-4 w-4" />
                {{ t('admin.userActivity.actions.sendEmail') }}
              </Button>
            </div>
          </div>

          <div class="flex flex-wrap items-end gap-2">
            <label class="flex flex-col gap-1 text-xs font-medium text-foreground">
              <span>{{ t('admin.userActivity.filters.segment') }}</span>
              <select v-model="segment" class="h-9 w-52 rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary" @change="applyFilters">
                <optgroup v-for="group in segmentGroups" :key="group.labelKey" :label="t(group.labelKey)">
                  <option v-for="value in group.segments" :key="value" :value="value">{{ t(`admin.userActivity.segments.${value}`) }}</option>
                </optgroup>
              </select>
            </label>
            <label v-if="segment === 'inactive'" class="space-y-1 text-xs font-medium text-foreground">
              <span>{{ t('admin.userActivity.filters.inactiveDays') }}</span>
              <input
                v-model.number="inactiveDays"
                type="number"
                min="1"
                max="3650"
                class="h-9 w-24 rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                @change="applyFilters"
              >
            </label>
            <label class="flex flex-col gap-1 text-xs font-medium text-foreground">
              <span>{{ t('admin.userActivity.filters.status') }}</span>
              <select v-model="statusFilter" class="h-9 w-36 rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary" @change="applyFilters">
                <option value="">{{ t('admin.userActivity.filters.allStatuses') }}</option>
                <option value="active">{{ t('admin.userActivity.userStatus.active') }}</option>
                <option value="disabled">{{ t('admin.userActivity.userStatus.disabled') }}</option>
              </select>
            </label>
            <label class="flex flex-col gap-1 text-xs font-medium text-foreground">
              <span>{{ t('admin.userActivity.filters.role') }}</span>
              <select v-model="roleFilter" class="h-9 w-32 rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary" @change="applyFilters">
                <option value="">{{ t('admin.userActivity.filters.allRoles') }}</option>
                <option value="user">{{ t('admin.userActivity.roles.user') }}</option>
                <option value="admin">{{ t('admin.userActivity.roles.admin') }}</option>
              </select>
            </label>
            <form class="flex min-w-[16rem] flex-1 items-end gap-2" @submit.prevent="applySearch">
              <label class="flex min-w-0 flex-1 flex-col gap-1 text-xs font-medium text-foreground">
                <span>{{ t('admin.userActivity.filters.search') }}</span>
                <span class="relative block">
                  <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                  <input
                    v-model="searchDraft"
                    type="search"
                    class="h-9 w-full rounded-lg border border-border/60 bg-surface py-2 pl-9 pr-9 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
                    :placeholder="t('admin.userActivity.filters.searchPlaceholder')"
                  >
                  <button
                    v-if="searchDraft || search"
                    type="button"
                    class="absolute right-1 top-1/2 inline-flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground hover:bg-muted hover:text-foreground"
                    :aria-label="t('admin.userActivity.actions.clearSearch')"
                    @click="clearSearch"
                  >
                    <X class="h-4 w-4" />
                  </button>
                </span>
              </label>
              <Button type="submit" variant="secondary" size="sm" class="rounded-lg">{{ t('admin.userActivity.actions.search') }}</Button>
            </form>
          </div>
          <p class="text-xs text-muted-foreground" aria-live="polite">
            {{ t('admin.userActivity.users.matched', { count: formatCount(totalUsers) }) }} · {{ filterSummary }}
          </p>
        </div>

        <div v-if="listErrorKey" class="m-4 flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(listErrorKey) }}</span>
        </div>

        <div class="overflow-x-auto" :class="{ 'opacity-60 transition-opacity': isLoadingUsers }">
          <table class="w-full min-w-[1320px] text-left text-sm">
            <thead class="bg-surface text-xs text-muted-foreground">
              <tr>
                <th class="px-4 py-2.5 font-medium">{{ t('admin.userActivity.fields.user') }}</th>
                <th class="px-3 py-2.5 font-medium">{{ t('admin.userActivity.fields.status') }}</th>
                <th v-for="column in sortColumns" :key="column.key" class="px-3 py-2.5 font-medium" :class="column.align === 'right' ? 'text-right' : ''" :aria-sort="ariaSort(column.key)">
                  <button type="button" class="inline-flex items-center gap-1 whitespace-nowrap hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleSort(column.key)">
                    {{ column.label }}
                    <ArrowDown v-if="sortKey === column.key && sortOrder === 'desc'" class="h-3 w-3" />
                    <ArrowUp v-else-if="sortKey === column.key" class="h-3 w-3" />
                    <ArrowUpDown v-else class="h-3 w-3 opacity-40" />
                  </button>
                </th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/40">
              <tr v-if="users.length === 0">
                <td colspan="13" class="px-4 py-12 text-center text-sm text-muted-foreground">
                  {{ isLoadingUsers ? t('admin.userActivity.status.loading') : t('admin.userActivity.users.empty') }}
                </td>
              </tr>
              <tr v-for="row in users" :key="row.siteUserId" class="hover:bg-surface/70">
                <td class="max-w-[16rem] px-4 py-2.5">
                  <button type="button" class="block max-w-full truncate text-left font-medium text-foreground hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="detailUserId = row.siteUserId">
                    {{ row.email || row.username || row.siteUserId }}
                  </button>
                  <span class="block truncate text-xs text-muted-foreground">{{ row.username || t('admin.userActivity.common.placeholder') }} · #{{ row.siteUserId }}<template v-if="row.group"> · {{ row.group }}</template></span>
                </td>
                <td class="px-3 py-2.5">
                  <span class="whitespace-nowrap text-xs" :class="row.status === 'disabled' ? 'text-red-600 dark:text-red-400' : 'text-muted-foreground'">
                    {{ te(`admin.userActivity.userStatus.${row.status}`) ? t(`admin.userActivity.userStatus.${row.status}`) : row.status }}
                  </span>
                  <span v-if="row.role === 'admin' || row.role === 'root'" class="ml-1 rounded bg-surface px-1.5 py-0.5 text-[11px] text-muted-foreground">{{ t(`admin.userActivity.roles.${row.role}`) }}</span>
                </td>
                <td class="whitespace-nowrap px-3 py-2.5">
                  <span class="block text-foreground">{{ lastUsedText(row) }}</span>
                  <span v-if="row.lastUsedAt" class="block text-xs text-muted-foreground tabular-nums">{{ formatDateTime(row.lastUsedAt) }}</span>
                </td>
                <td class="px-3 py-2.5 text-right tabular-nums text-foreground">{{ formatUsd(row.costToday) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-foreground">
                  {{ formatUsd(row.cost7d) }}
                  <span class="block whitespace-nowrap text-xs text-muted-foreground">{{ t('admin.userActivity.fields.previousValue', { value: formatUsd(row.costPrev7d) }) }}</span>
                </td>
                <td class="px-3 py-2.5 text-right">
                  <DeltaBadge :current="row.cost7d" :previous="row.costPrev7d" />
                </td>
                <td class="px-3 py-2.5 text-right tabular-nums text-foreground">{{ formatUsd(row.cost30d) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-foreground">{{ formatCount(row.activeDays30d) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums" :class="row.rechargeToday > 0 ? 'font-medium text-foreground' : 'text-muted-foreground'">{{ formatUsd(row.rechargeToday) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-foreground">{{ formatUsd(row.recharge30d) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-foreground">{{ formatUsd(row.balance) }}</td>
                <td class="px-3 py-2.5 text-right tabular-nums text-foreground">{{ formatUsd(lifetimeValue(row)) }}</td>
                <td class="whitespace-nowrap px-3 py-2.5 text-xs text-muted-foreground tabular-nums">{{ formatDateTime(row.createdAt) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="flex flex-wrap items-center justify-between gap-2 border-t border-border/50 px-4 py-3 text-sm text-muted-foreground">
          <span>{{ t('admin.userActivity.pagination.total', { total: formatCount(totalUsers) }) }}</span>
          <div class="flex items-center gap-2">
            <Button variant="secondary" size="sm" class="rounded-lg" :disabled="page <= 1 || isLoadingUsers" @click="changePage(page - 1)">{{ t('admin.userActivity.pagination.previous') }}</Button>
            <span class="tabular-nums">{{ page }} / {{ pages }}</span>
            <Button variant="secondary" size="sm" class="rounded-lg" :disabled="page >= pages || isLoadingUsers" @click="changePage(page + 1)">{{ t('admin.userActivity.pagination.next') }}</Button>
          </div>
        </div>
      </section>
    </template>

    <SegmentEmailDialog
      v-if="isEmailDialogOpen"
      :filter="currentFilter"
      :filter-summary="filterSummary"
      @close="isEmailDialogOpen = false"
      @created="onBatchCreated"
    />
    <SuppressionListDialog v-if="isSuppressionsOpen" @close="isSuppressionsOpen = false" @changed="onSuppressionsChanged" />
    <UserActivityDetailDrawer
      v-if="detailUserId"
      :site-user-id="detailUserId"
      :platform="platform"
      :today="today"
      @close="detailUserId = null"
    />
  </div>
</template>
