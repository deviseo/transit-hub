<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  Activity,
  AlertTriangle,
  ArrowDownUp,
  ChevronDown,
  ChevronRight,
  ChevronUp,
  Clock3,
  Eye,
  Gauge,
  Radar,
  Settings2,
  ShieldCheck,
  ShieldQuestion,
  Power,
  X,
  Zap,
} from 'lucide-vue-next'
import { Tooltip } from '@/components/ui/tooltip'
import {
  connectionHealthMessageKey,
  connectionHealthStateBadgeClass,
  formatConnectionHealthTime,
  remoteActionLabelKey,
} from '../../composables/useConnectionHealth'
import type {
  AdminGroupAccount,
  AdminGroupHealth,
  ConnectionHealthState,
} from '../../types/connectionHealth'

const props = defineProps<{
  group: AdminGroupHealth
  hideUnmonitoredAccounts: boolean
  actionLoading: boolean
}>()

const emit = defineEmits<{
  (event: 'setup', group: AdminGroupHealth): void
  (event: 'probe', account: AdminGroupAccount): void
  (event: 'view-events', account: AdminGroupAccount): void
  (event: 'update:hide-unmonitored-accounts', value: boolean): void
  (event: 'set-schedulable', account: AdminGroupAccount): void
}>()

import { t, te } from '@/locales'
const prefix = 'admin.connectionHealth'
const detailPrefix = `${prefix}.groupDetail`
const expandedTargetId = ref('')

type GroupHealthFilter =
  | { kind: 'all' }
  | { kind: 'monitored' }
  | { kind: 'probeable' }
  | { kind: 'unprobeable' }
  | { kind: 'unconfigured' }
  | { kind: 'modelState'; state: ConnectionHealthState }
  | { kind: 'notProbed' }

type AccountSortField =
  | 'account'
  | 'health'
  | 'strategy'
  | 'priority'
  | 'effectiveMultiplier'
  | 'upstreamMultiplier'
  | 'latency'

type SortDirection = 'asc' | 'desc'
type StateBreakdownItem = {
  key: string
  count: number
  filter: GroupHealthFilter
  tone: string
}

const monitoringEnabled = (account: AdminGroupAccount): boolean =>
  account.hasEnabledProbePolicy ?? account.hasEnabledPolicy ?? account.assignedPolicies?.some((policy) => policy.enabled) ?? Boolean(account.hasAssignedPolicy)

const monitoredCount = computed(() => props.group.monitoredAccountCount ?? props.group.accounts.filter(monitoringEnabled).length)
const lastProbeAt = computed(() => props.group.healthSummary?.lastProbeAt ?? null)
const isNewAPI = computed(() => props.group.platform.toLowerCase().includes('new'))

const strictDegradedCount = computed(() => Math.max(
  0,
  (props.group.healthSummary.degradedModels ?? 0)
    - (props.group.healthSummary.observingModels ?? 0)
    - (props.group.healthSummary.recoveringModels ?? 0),
))

const modelStateFilter = (state: ConnectionHealthState): GroupHealthFilter => ({ kind: 'modelState', state })

const stateBreakdown = computed<StateBreakdownItem[]>(() => [
  { key: 'healthy', count: props.group.healthSummary.healthyModels ?? 0, filter: modelStateFilter('healthy'), tone: 'text-emerald-600 dark:text-emerald-400' },
  { key: 'degraded', count: strictDegradedCount.value, filter: modelStateFilter('degraded'), tone: 'text-amber-600 dark:text-amber-400' },
  { key: 'suspended', count: props.group.healthSummary.suspendedModels ?? 0, filter: modelStateFilter('suspended'), tone: 'text-red-600 dark:text-red-400' },
  { key: 'observing', count: props.group.healthSummary.observingModels ?? 0, filter: modelStateFilter('observing'), tone: 'text-blue-600 dark:text-blue-400' },
  { key: 'recovering', count: props.group.healthSummary.recoveringModels ?? 0, filter: modelStateFilter('recovering'), tone: 'text-cyan-600 dark:text-cyan-400' },
  { key: 'disabled', count: props.group.healthSummary.disabledModels ?? 0, filter: modelStateFilter('disabled'), tone: 'text-muted-foreground' },
  { key: 'notProbed', count: props.group.healthSummary.pendingModels ?? 0, filter: { kind: 'notProbed' }, tone: 'text-muted-foreground' },
  { key: 'unconfigured', count: props.group.healthSummary.unconfiguredModels ?? 0, filter: { kind: 'unconfigured' }, tone: 'text-muted-foreground' },
  { key: 'unprobeable', count: props.group.healthSummary.unprobeableAccounts ?? 0, filter: { kind: 'unprobeable' }, tone: 'text-amber-600 dark:text-amber-400' },
])

const readableMessage = (rawKey: string): string => t(connectionHealthMessageKey(rawKey, te))

const STATE_PRIORITY: ConnectionHealthState[] = ['suspended', 'disabled', 'degraded', 'observing', 'recovering', 'healthy']
const aggregateState = (account: AdminGroupAccount): ConnectionHealthState | '' => {
  const present = new Set((account.modelHealth ?? []).map((model) => model.state))
  return STATE_PRIORITY.find((state) => present.has(state)) ?? ''
}

const unprobedModels = (account: AdminGroupAccount) => account.unprobedModels ?? []
const isNotProbed = (account: AdminGroupAccount): boolean =>
  account.probeAvailable && account.probeModelsConfigured !== false && (unprobedModels(account).length > 0 || account.modelHealth.length === 0)

const isSub2API = (account: AdminGroupAccount): boolean => account.targetId.toLowerCase().startsWith('sub2api:')
const schedulableLabel = (account: AdminGroupAccount): string => {
  if (!isSub2API(account)) return t(`${detailPrefix}.notApplicable`)
  if (account.schedulable == null) return t(`${detailPrefix}.schedulableUnknown`)
  return account.schedulable ? t(`${detailPrefix}.schedulableOn`) : t(`${detailPrefix}.schedulableOff`)
}

const upstreamStatusLabel = (account: AdminGroupAccount): string => {
  const status = account.status?.toLowerCase()
  if (status === 'active' || status === '1') return t(`${detailPrefix}.upstreamAccountActive`)
  if (status === 'inactive' || status === '0' || status === 'disabled') return t(`${detailPrefix}.upstreamAccountInactive`)
  return t(`${detailPrefix}.upstreamStatus`, { status: account.status || t(`${detailPrefix}.unknownUpstreamStatus`) })
}

const statusSourceLabel = (source: string | null | undefined): string =>
  t(`${detailPrefix}.statusSourceLabels.${source || 'unknown'}`)

const assignmentLabel = (account: AdminGroupAccount): string => {
  const policies = account.assignedPolicies ?? []
  if (policies.length === 0) return t(`${detailPrefix}.unmonitored`)
  if (policies.length === 1) return policies[0].policyName
  return t(`${detailPrefix}.policyCount`, { name: policies[0].policyName, count: policies.length - 1 })
}

const assignmentSourceLabel = (account: AdminGroupAccount): string =>
  t(`${detailPrefix}.assignmentSources.${account.policyAssignmentSource ?? 'none'}`)

const strategyStateLabel = (account: AdminGroupAccount): string => {
  if (account.hasAssignedPolicy && !account.hasEnabledPolicy) return t(`${detailPrefix}.strategyDisabled`)
  return assignmentSourceLabel(account)
}

const priorityStateLabel = (account: AdminGroupAccount): string => {
  if (!account.priorityManaged) return t(`${detailPrefix}.priorityUnmanaged`)
  if (account.hasEnabledProbePolicy && account.probeModelsConfigured === false) return t(`${detailPrefix}.probeModelsNotConfigured`)
  if (isNotProbed(account)) return t(`${detailPrefix}.priorityPendingProbe`)
  const state = aggregateState(account)
  if (state === 'suspended' || state === 'disabled') return t(`${detailPrefix}.healthSuspended`)
  return t(`${detailPrefix}.priorityManaged`)
}

const lastSchedulableActionLabel = (account: AdminGroupAccount): string => {
  const action = remoteActionLabelKey(account.lastSchedulableAction ?? '')
  if (!action) return ''
  const result = account.lastSchedulableActionResult === 'schedulable_user_action_succeeded'
    ? t(`${detailPrefix}.schedulableActionSucceeded`)
    : t(`${detailPrefix}.schedulableActionFailed`)
  const error = account.lastSchedulableActionErrorKey
    ? ` · ${t(connectionHealthMessageKey(account.lastSchedulableActionErrorKey, te))}`
    : ''
  return t(`${detailPrefix}.lastSchedulableAction`, {
    action: t(action.key, action.params),
    result,
    time: formatConnectionHealthTime(account.lastSchedulableActionAt ?? null),
    error,
  })
}

const blockedReasonLabel = (reason: string | null | undefined): string => {
  if (!reason) return ''
  const key = `${prefix}.probeBlockedReasons.${reason}`
  return te(key) ? t(key) : reason
}

const effectiveSourcesLabel = (accountModel: AdminGroupAccount['modelHealth'][number] | NonNullable<AdminGroupAccount['unprobedModels']>[number]): string =>
  (accountModel.effectivePolicySources ?? [])
    .map(source => t(`${detailPrefix}.models.policySource`, {
      name: source.policyName || source.policyId,
      interval: source.effectiveIntervalSeconds,
      state: source.continueAutoProbe ? t(`${detailPrefix}.models.policyContinues`) : t(`${detailPrefix}.models.policyStops`),
    }))
    .join('；')

const toggleModels = (targetId: string) => {
  expandedTargetId.value = expandedTargetId.value === targetId ? '' : targetId
}

const activeFilter = ref<GroupHealthFilter>({ kind: 'all' })
const sortField = ref<AccountSortField>('health')
const sortDirection = ref<SortDirection>('asc')
const customSortActive = ref(false)

const DEFAULT_SORT_DIRECTIONS: Record<AccountSortField, SortDirection> = {
  account: 'asc',
  health: 'asc',
  strategy: 'asc',
  priority: 'asc',
  effectiveMultiplier: 'asc',
  upstreamMultiplier: 'asc',
  latency: 'asc',
}

const HEALTH_SORT_RANK: Record<ConnectionHealthState, number> = {
  healthy: 0,
  observing: 1,
  recovering: 2,
  degraded: 3,
  suspended: 4,
  disabled: 5,
}

const filtersEqual = (first: GroupHealthFilter, second: GroupHealthFilter): boolean =>
  first.kind === second.kind
  && (first.kind !== 'modelState' || second.kind === 'modelState' && first.state === second.state)

const filterLabel = computed(() => {
  switch (activeFilter.value.kind) {
    case 'monitored':
      return t(`${detailPrefix}.metrics.monitored`)
    case 'probeable':
      return t(`${detailPrefix}.metrics.probeable`)
    case 'unprobeable':
      return t(`${detailPrefix}.statusBreakdown.unprobeable`)
    case 'notProbed':
      return t(`${detailPrefix}.statusBreakdown.notProbed`)
    case 'unconfigured':
      return t(`${detailPrefix}.statusBreakdown.unconfigured`)
    case 'modelState':
      return t(`${prefix}.stateLabels.${activeFilter.value.state}`)
    default:
      return t(`${detailPrefix}.filters.all`)
  }
})

const applyFilter = (filter: GroupHealthFilter) => {
  activeFilter.value = filter
  expandedTargetId.value = ''
}

const clearFilter = () => applyFilter({ kind: 'all' })

const isFilterActive = (filter: GroupHealthFilter): boolean => filtersEqual(activeFilter.value, filter)

const matchesFilter = (account: AdminGroupAccount): boolean => {
  const filter = activeFilter.value
  switch (filter.kind) {
    case 'monitored':
      return monitoringEnabled(account)
    case 'probeable':
      return account.probeAvailable
    case 'unprobeable':
      return !account.probeAvailable
    case 'notProbed':
      return isNotProbed(account)
    case 'unconfigured':
      return Boolean(account.hasEnabledProbePolicy) && account.probeModelsConfigured === false
    case 'modelState':
      return account.modelHealth.some(model => model.state === filter.state)
    default:
      return true
  }
}

const shouldHideUnmonitored = (account: AdminGroupAccount): boolean =>
  props.hideUnmonitoredAccounts && !monitoringEnabled(account)

const accountLatency = (account: AdminGroupAccount): number | null => {
  const values = account.modelHealth
    .map(model => model.lastSuccessLatencyMs)
    .filter((value): value is number => value != null && Number.isFinite(value))
  return values.length > 0 ? Math.max(...values) : null
}

const accountHasSlowResponse = (account: AdminGroupAccount): boolean => (accountLatency(account) ?? 0) > 5000

const accountHealthRank = (account: AdminGroupAccount): number => {
  if (!account.probeAvailable) return 7
  const state = aggregateState(account)
  if (!state) return 6
  return HEALTH_SORT_RANK[state] ?? 6
}

const accountSortValue = (account: AdminGroupAccount, field: AccountSortField): string | number | null => {
  switch (field) {
    case 'account':
      return (account.name || account.id).toLocaleLowerCase()
    case 'health':
      return accountHealthRank(account)
    case 'strategy':
      return assignmentLabel(account).toLocaleLowerCase()
    case 'priority':
      return account.priority ?? null
    case 'effectiveMultiplier':
      return effectiveMultiplier(account)
    case 'upstreamMultiplier':
      return account.upstreamKeyGroupMultiplier ?? null
    case 'latency':
      return accountLatency(account)
  }
}

const compareSortValues = (
  first: string | number | null,
  second: string | number | null,
  direction: SortDirection,
): number => {
  if (first === null || first === undefined) return second === null || second === undefined ? 0 : 1
  if (second === null || second === undefined) return -1
  const rawDiff = typeof first === 'string' && typeof second === 'string'
    ? first.localeCompare(second)
    : Number(first) - Number(second)
  return direction === 'asc' ? rawDiff : -rawDiff
}

const compareAccountsByName = (first: AdminGroupAccount, second: AdminGroupAccount): number => {
  const nameDiff = (first.name || first.id).localeCompare(second.name || second.id)
  return nameDiff !== 0 ? nameDiff : first.id.localeCompare(second.id)
}

const filteredAccounts = computed(() => props.group.accounts.filter(account =>
  !shouldHideUnmonitored(account) && matchesFilter(account),
))

const sortedAccounts = computed(() => [...filteredAccounts.value].sort((first, second) => {
  if (!customSortActive.value) {
    const orderDiff = compareSortValues(first.productionSortOrder ?? null, second.productionSortOrder ?? null, 'asc')
    return orderDiff !== 0 ? orderDiff : first.targetId.localeCompare(second.targetId)
  }
  const valueDiff = compareSortValues(
    accountSortValue(first, sortField.value),
    accountSortValue(second, sortField.value),
    sortDirection.value,
  )
  return valueDiff !== 0 ? valueDiff : compareAccountsByName(first, second)
}))

const filteredModelHealth = (account: AdminGroupAccount) => {
  const filter = activeFilter.value
  if (filter.kind === 'notProbed' || filter.kind === 'unconfigured') return []
  if (filter.kind !== 'modelState') return account.modelHealth
  return account.modelHealth.filter(model => model.state === filter.state)
}

const filteredUnprobedModels = (account: AdminGroupAccount) => {
  if (activeFilter.value.kind === 'modelState') return []
  if (activeFilter.value.kind === 'notProbed') return unprobedModels(account)
  if (activeFilter.value.kind === 'unconfigured') return []
  return unprobedModels(account)
}

const toggleSort = (field: AccountSortField) => {
  customSortActive.value = true
  if (sortField.value === field) {
    sortDirection.value = sortDirection.value === 'asc' ? 'desc' : 'asc'
    return
  }
  sortField.value = field
  sortDirection.value = DEFAULT_SORT_DIRECTIONS[field]
}

const ariaSort = (field: AccountSortField): 'ascending' | 'descending' | 'none' => {
  if (!customSortActive.value || sortField.value !== field) return 'none'
  return sortDirection.value === 'asc' ? 'ascending' : 'descending'
}

watch(() => props.group.id, () => {
  activeFilter.value = { kind: 'all' }
  sortField.value = 'health'
  sortDirection.value = 'asc'
  customSortActive.value = false
  expandedTargetId.value = ''
})

const formatNumber = (value: number | null | undefined): string => value == null ? '-' : String(value)
const formatMultiplier = (value: number | null | undefined): string => value == null ? '-' : `${value}x`

const usesMultiplierOnly = (account: AdminGroupAccount): boolean =>
  (account.assignedPolicies ?? []).some(policy => policy.enabled && policy.strategyMode === 'multiplier_only')

const effectiveMultiplier = (account: AdminGroupAccount): number | null => {
  if (account.effectiveMultiplier != null) return account.effectiveMultiplier
  if (usesMultiplierOnly(account)) return props.group.multiplier
  return null
}

const multiplierSourceLabel = (account: AdminGroupAccount): string => {
  if (usesMultiplierOnly(account)) return t(`${detailPrefix}.multiplierSources.adminGroup`)
  if (account.multiplierSource === 'upstream_key') return t(`${detailPrefix}.multiplierSources.upstreamKey`)
  if (account.multiplierSource === 'local_fallback') {
    if (account.multiplierResolutionStatus === 'conflict') return t(`${detailPrefix}.multiplierSources.conflictFallback`)
    return t(`${detailPrefix}.multiplierSources.missingFallback`)
  }
  if (account.multiplierResolutionStatus === 'unavailable') {
    return account.priorityManaged
      ? t(`${detailPrefix}.multiplierSources.unavailableHeld`)
      : t(`${detailPrefix}.multiplierSources.unavailableUnmanaged`)
  }
  return t(`${detailPrefix}.multiplierSources.fallbackRequired`)
}

const upstreamMultiplierStatusLabel = (account: AdminGroupAccount): string => {
  if (account.multiplierResolutionStatus === 'conflict') return t(`${detailPrefix}.upstreamMultiplierStatuses.conflict`)
  if (account.multiplierResolutionStatus === 'unavailable') return t(`${detailPrefix}.upstreamMultiplierStatuses.unavailable`)
  if (account.multiplierResolutionStatus === 'unassociated') return t(`${detailPrefix}.upstreamMultiplierStatuses.unassociated`)
  return t(`${detailPrefix}.upstreamMultiplierStatuses.missing`)
}
</script>

<template>
  <section class="min-w-0" :aria-label="group.name">
    <header class="flex flex-col gap-4 border-b border-border/50 px-5 py-5 lg:flex-row lg:items-start lg:justify-between">
      <div class="min-w-0">
        <div class="flex flex-wrap items-center gap-2">
          <h2 class="truncate text-xl font-semibold text-foreground">{{ group.name }}</h2>
          <span class="rounded-md bg-surface px-2 py-0.5 text-xs text-muted-foreground">{{ group.platform || '-' }}</span>
          <span
            class="rounded-md px-2 py-0.5 text-xs font-medium"
            :class="group.status === 'active' || group.status === '1'
              ? 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
              : 'bg-muted text-muted-foreground'"
          >
            {{ t(`${prefix}.groupStatusLabels.${group.status === 'active' || group.status === '1' ? 'active' : 'inactive'}`) }}
          </span>
          <span v-if="group.priorityMode === 'multiplier'" class="inline-flex items-center gap-1 rounded-md bg-primary/10 px-2 py-0.5 text-xs font-medium text-primary">
            <ArrowDownUp class="h-3 w-3" />
            {{ t(`${detailPrefix}.multiplierPriority`) }}
          </span>
        </div>
        <p class="mt-1 text-sm text-muted-foreground">
          {{ t(`${detailPrefix}.subtitle`, { monitored: monitoredCount, total: group.accountCount }) }}
        </p>
      </div>
      <div class="flex flex-wrap items-center gap-3">
        <label class="inline-flex h-9 shrink-0 cursor-pointer items-center gap-2 rounded-lg border border-border/60 bg-background px-3 text-xs text-foreground">
          <input
            type="checkbox"
            class="h-4 w-4 rounded border-border accent-primary"
            :checked="hideUnmonitoredAccounts"
            @change="emit('update:hide-unmonitored-accounts', ($event.target as HTMLInputElement).checked)"
          >
          {{ t(`${detailPrefix}.filters.hideUnmonitored`) }}
        </label>
        <button
          type="button"
          class="inline-flex h-9 shrink-0 items-center justify-center gap-2 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          @click="emit('setup', group)"
        >
          <Settings2 class="h-4 w-4" />
          {{ t(`${detailPrefix}.${group.hasAssignedPolicy ? 'manageMonitoring' : 'enableMonitoring'}`) }}
        </button>
      </div>
    </header>

    <div class="grid border-b border-border/50 sm:grid-cols-2 xl:grid-cols-4">
      <button
        type="button"
        class="border-b border-border/50 px-5 py-4 text-left transition-colors hover:bg-surface/35 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary sm:border-r xl:border-b-0"
        :class="isFilterActive({ kind: 'all' }) ? 'bg-primary/[0.05]' : ''"
        :aria-pressed="isFilterActive({ kind: 'all' })"
        @click="applyFilter({ kind: 'all' })"
      >
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground"><Radar class="h-3.5 w-3.5" />{{ t(`${detailPrefix}.metrics.accounts`) }}</span>
        <span class="mt-1 block text-xl font-semibold tabular-nums text-foreground">{{ group.accountCount }}</span>
      </button>
      <button
        type="button"
        class="border-b border-border/50 px-5 py-4 text-left transition-colors hover:bg-surface/35 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary xl:border-b-0 xl:border-r"
        :class="isFilterActive({ kind: 'monitored' }) ? 'bg-primary/[0.05]' : ''"
        :aria-pressed="isFilterActive({ kind: 'monitored' })"
        @click="applyFilter({ kind: 'monitored' })"
      >
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground"><ShieldCheck class="h-3.5 w-3.5" />{{ t(`${detailPrefix}.metrics.monitored`) }}</span>
        <span class="mt-1 block text-xl font-semibold tabular-nums text-foreground">{{ monitoredCount }}</span>
      </button>
      <button
        type="button"
        class="border-b border-border/50 px-5 py-4 text-left transition-colors hover:bg-surface/35 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary sm:border-b-0 sm:border-r"
        :class="isFilterActive({ kind: 'probeable' }) ? 'bg-primary/[0.05]' : ''"
        :aria-pressed="isFilterActive({ kind: 'probeable' })"
        @click="applyFilter({ kind: 'probeable' })"
      >
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground"><Gauge class="h-3.5 w-3.5" />{{ t(`${detailPrefix}.metrics.probeable`) }}</span>
        <span class="mt-1 block text-xl font-semibold tabular-nums text-foreground">{{ group.healthSummary.probeableAccounts }}</span>
      </button>
      <div class="px-5 py-4">
        <span class="flex items-center gap-1.5 text-xs font-medium text-muted-foreground"><Clock3 class="h-3.5 w-3.5" />{{ t(`${detailPrefix}.metrics.lastProbe`) }}</span>
        <span class="mt-1 block text-sm font-medium text-foreground">{{ formatConnectionHealthTime(lastProbeAt) }}</span>
      </div>
    </div>

    <div v-if="(group.priorityConflictCount ?? 0) > 0" class="flex items-start gap-2 border-b border-amber-500/25 bg-amber-500/[0.07] px-5 py-3 text-sm text-amber-700 dark:text-amber-400">
      <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0" />
      <div class="min-w-0">
        <p>{{ t(`${detailPrefix}.priorityConflict`, { count: group.priorityConflictCount ?? 0 }) }}</p>
        <ul v-if="group.priorityConflicts?.length" class="mt-1 space-y-0.5 text-xs">
          <li v-for="conflict in group.priorityConflicts" :key="conflict.targetId">
            {{ t(`${detailPrefix}.priorityConflictTarget`, {
              target: conflict.accountName || conflict.targetId,
              current: conflict.currentPriority ?? '-',
              expected: conflict.expectedPriority ?? '-',
              time: formatConnectionHealthTime(conflict.conflictAt ?? null),
            }) }}
          </li>
        </ul>
      </div>
    </div>

    <section class="border-b border-border/50 bg-surface/20 px-5 py-4" :aria-label="t(`${detailPrefix}.statusBreakdown.title`)">
      <div class="flex flex-col gap-1 sm:flex-row sm:items-baseline sm:justify-between sm:gap-4">
        <h3 class="text-xs font-semibold text-foreground">{{ t(`${detailPrefix}.statusBreakdown.title`) }}</h3>
        <p class="text-xs leading-5 text-muted-foreground">{{ t(`${detailPrefix}.statusBreakdown.hint`) }}</p>
      </div>
      <div class="mt-3 grid grid-cols-2 divide-x divide-y divide-border/50 overflow-hidden rounded-lg border border-border/50 bg-background sm:grid-cols-4 xl:grid-cols-8 xl:divide-y-0">
        <button
          v-for="item in stateBreakdown"
          :key="item.key"
          type="button"
          class="min-w-0 px-3 py-2.5 text-left transition-colors hover:bg-surface/45 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-inset focus-visible:ring-primary"
          :class="isFilterActive(item.filter) ? 'bg-primary/[0.06]' : ''"
          :aria-pressed="isFilterActive(item.filter)"
          @click="applyFilter(item.filter)"
        >
          <span class="block truncate text-[11px] font-medium" :class="item.tone">{{ t(`${detailPrefix}.statusBreakdown.${item.key}`) }}</span>
          <span class="mt-0.5 block text-lg font-semibold tabular-nums text-foreground">{{ item.count }}</span>
        </button>
      </div>
    </section>

    <div class="px-5 py-5">
      <p class="mb-3 text-xs text-muted-foreground">
        {{ customSortActive ? t(`${detailPrefix}.temporarySortHint`) : t(`${detailPrefix}.productionSortHint`) }}
      </p>
      <div v-if="activeFilter.kind !== 'all'" class="mb-3 flex items-center justify-between gap-3 rounded-lg border border-primary/20 bg-primary/[0.06] px-3 py-2 text-xs text-primary">
        <span>{{ t(`${detailPrefix}.filters.active`, { label: filterLabel }) }}</span>
        <button
          type="button"
          class="inline-flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-primary transition-colors hover:bg-primary/10"
          @click="clearFilter"
        >
          <X class="h-3.5 w-3.5" />
          {{ t(`${detailPrefix}.filters.clear`) }}
        </button>
      </div>
      <div v-if="group.accountsError" class="rounded-lg bg-destructive/10 px-4 py-3 text-sm text-destructive">
        {{ readableMessage(group.accountsError) }}
      </div>
      <div v-else-if="group.accounts.length === 0" class="flex min-h-64 flex-col items-center justify-center text-center">
        <Activity class="h-8 w-8 text-muted-foreground/40" />
        <p class="mt-3 text-sm text-muted-foreground">{{ t(`${detailPrefix}.empty`) }}</p>
      </div>
      <div v-else class="overflow-x-auto rounded-lg border border-border/60">
        <table class="w-full min-w-[72rem] text-sm">
          <thead class="bg-surface/60 text-left text-xs text-muted-foreground">
            <tr>
              <th class="w-10 px-3 py-2.5 font-medium"><span class="sr-only">{{ t(`${detailPrefix}.columns.expand`) }}</span></th>
              <th class="px-3 py-2.5 font-medium" :aria-sort="ariaSort('account')">
                <button type="button" class="inline-flex items-center gap-1.5 text-left hover:text-foreground" @click="toggleSort('account')">
                  {{ t(`${detailPrefix}.columns.account`) }}
                  <ChevronUp v-if="customSortActive && sortField === 'account' && sortDirection === 'asc'" class="h-3.5 w-3.5" />
                  <ChevronDown v-else-if="customSortActive && sortField === 'account'" class="h-3.5 w-3.5" />
                  <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-50" />
                </button>
              </th>
              <th class="px-3 py-2.5 font-medium" :aria-sort="ariaSort('health')">
                <button type="button" class="inline-flex items-center gap-1.5 text-left hover:text-foreground" @click="toggleSort('health')">
                  {{ t(`${detailPrefix}.columns.health`) }}
                  <ChevronUp v-if="customSortActive && sortField === 'health' && sortDirection === 'asc'" class="h-3.5 w-3.5" />
                  <ChevronDown v-else-if="customSortActive && sortField === 'health'" class="h-3.5 w-3.5" />
                  <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-50" />
                </button>
              </th>
              <th class="px-3 py-2.5 font-medium" :aria-sort="ariaSort('strategy')">
                <button type="button" class="inline-flex items-center gap-1.5 text-left hover:text-foreground" @click="toggleSort('strategy')">
                  {{ t(`${detailPrefix}.columns.strategy`) }}
                  <ChevronUp v-if="customSortActive && sortField === 'strategy' && sortDirection === 'asc'" class="h-3.5 w-3.5" />
                  <ChevronDown v-else-if="customSortActive && sortField === 'strategy'" class="h-3.5 w-3.5" />
                  <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-50" />
                </button>
              </th>
              <th class="px-3 py-2.5 font-medium" :aria-sort="ariaSort('priority')">
                <button type="button" class="inline-flex items-center gap-1.5 text-left hover:text-foreground" @click="toggleSort('priority')">
                  {{ t(`${detailPrefix}.columns.priority`) }}
                  <ChevronUp v-if="customSortActive && sortField === 'priority' && sortDirection === 'asc'" class="h-3.5 w-3.5" />
                  <ChevronDown v-else-if="customSortActive && sortField === 'priority'" class="h-3.5 w-3.5" />
                  <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-50" />
                </button>
              </th>
              <th class="px-3 py-2.5 font-medium" :aria-sort="ariaSort('effectiveMultiplier')">
                <button type="button" class="inline-flex items-center gap-1.5 text-left hover:text-foreground" @click="toggleSort('effectiveMultiplier')">
                  {{ t(`${detailPrefix}.columns.strategyMultiplier`) }}
                  <ChevronUp v-if="customSortActive && sortField === 'effectiveMultiplier' && sortDirection === 'asc'" class="h-3.5 w-3.5" />
                  <ChevronDown v-else-if="customSortActive && sortField === 'effectiveMultiplier'" class="h-3.5 w-3.5" />
                  <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-50" />
                </button>
              </th>
              <th class="px-3 py-2.5 font-medium" :aria-sort="ariaSort('upstreamMultiplier')">
                <button type="button" class="inline-flex items-center gap-1.5 text-left hover:text-foreground" @click="toggleSort('upstreamMultiplier')">
                  {{ t(`${detailPrefix}.columns.upstreamMultiplier`) }}
                  <ChevronUp v-if="customSortActive && sortField === 'upstreamMultiplier' && sortDirection === 'asc'" class="h-3.5 w-3.5" />
                  <ChevronDown v-else-if="customSortActive && sortField === 'upstreamMultiplier'" class="h-3.5 w-3.5" />
                  <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-50" />
                </button>
              </th>
              <th class="px-3 py-2.5 font-medium" :aria-sort="ariaSort('latency')">
                <button type="button" class="inline-flex items-center gap-1.5 text-left hover:text-foreground" @click="toggleSort('latency')">
                  {{ t(`${detailPrefix}.columns.latency`) }}
                  <ChevronUp v-if="customSortActive && sortField === 'latency' && sortDirection === 'asc'" class="h-3.5 w-3.5" />
                  <ChevronDown v-else-if="customSortActive && sortField === 'latency'" class="h-3.5 w-3.5" />
                  <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-50" />
                </button>
              </th>
              <th class="px-3 py-2.5 text-right font-medium">{{ t(`${detailPrefix}.columns.actions`) }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="sortedAccounts.length === 0">
              <td colspan="9" class="px-4 py-12 text-center text-sm text-muted-foreground">
                {{ t(`${detailPrefix}.filters.noMatches`) }}
              </td>
            </tr>
            <template v-else v-for="account in sortedAccounts" :key="account.targetId">
              <tr class="border-t border-border/40 transition-colors hover:bg-surface/35">
                <td class="px-3 py-3">
                  <button
                    type="button"
                    class="rounded p-1 text-muted-foreground hover:bg-surface hover:text-foreground"
                    :aria-label="t(`${detailPrefix}.columns.expand`)"
                    @click="toggleModels(account.targetId)"
                  >
                    <ChevronDown v-if="expandedTargetId === account.targetId" class="h-4 w-4" />
                    <ChevronRight v-else class="h-4 w-4" />
                  </button>
                </td>
                <td class="px-3 py-3">
                  <div class="max-w-56">
                    <p class="truncate font-medium text-foreground">{{ account.name || account.id }}</p>
                    <p class="mt-0.5 truncate text-xs text-muted-foreground">
                      {{ account.platform || account.type || '-' }} · {{ upstreamStatusLabel(account) }} · {{ schedulableLabel(account) }}
                    </p>
                    <p class="mt-0.5 truncate text-[11px] text-muted-foreground">
                      {{ t(`${detailPrefix}.statusSources`, { upstream: statusSourceLabel(account.upstreamStatusSource), health: statusSourceLabel(account.healthStatusSource), schedulable: statusSourceLabel(account.schedulableSource) }) }}
                    </p>
                    <p v-if="account.schedulableChangedAt" class="mt-0.5 truncate text-[11px] text-muted-foreground">{{ t(`${detailPrefix}.schedulableChangedAt`, { time: formatConnectionHealthTime(account.schedulableChangedAt) }) }}</p>
                    <p v-if="account.lastSchedulableAction" class="mt-0.5 max-w-72 truncate text-[11px] text-muted-foreground" :title="lastSchedulableActionLabel(account)">{{ lastSchedulableActionLabel(account) }}</p>
                  </div>
                </td>
                <td class="px-3 py-3">
                  <div class="flex flex-col items-start gap-1">
                    <span v-if="!account.hasEnabledProbePolicy" class="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
                      <ShieldQuestion class="h-3 w-3" />{{ t(`${detailPrefix}.notApplicable`) }}
                    </span>
                    <span v-else-if="!account.probeAvailable" class="inline-flex items-center gap-1 rounded-md bg-amber-500/10 px-2 py-1 text-xs font-medium text-amber-600 dark:text-amber-400">
                      <AlertTriangle class="h-3 w-3" />{{ t(`${detailPrefix}.unprobeable`) }}
                    </span>
                    <span v-else-if="account.probeModelsConfigured === false" class="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
                      <Settings2 class="h-3 w-3" />{{ t(`${prefix}.notConfigured`) }}
                    </span>
                    <span v-else-if="!aggregateState(account)" class="inline-flex items-center gap-1 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground">
                      <ShieldQuestion class="h-3 w-3" />{{ t(`${prefix}.notProbed`) }}
                    </span>
                    <span v-else class="inline-flex items-center rounded-md px-2 py-1 text-xs font-medium" :class="connectionHealthStateBadgeClass(aggregateState(account))">
                      {{ t(`${prefix}.stateLabels.${aggregateState(account)}`) }}
                      <span v-if="account.modelHealth.length > 1" class="ml-1 opacity-70">×{{ account.modelHealth.length }}</span>
                    </span>
                    <span v-if="unprobedModels(account).length > 0 && aggregateState(account)" class="text-[11px] text-muted-foreground">
                      {{ t(`${prefix}.notProbed`) }} ×{{ unprobedModels(account).length }}
                    </span>
                  </div>
                </td>
                <td class="px-3 py-3">
                  <Tooltip :text="assignmentLabel(account)" wide>
                    <div class="max-w-52">
                      <p class="truncate text-xs font-medium" :class="monitoringEnabled(account) ? 'text-foreground' : 'text-muted-foreground'">{{ assignmentLabel(account) }}</p>
                      <p class="mt-0.5 text-[11px] text-muted-foreground">{{ strategyStateLabel(account) }}</p>
                    </div>
                  </Tooltip>
                </td>
                <td class="px-3 py-3 tabular-nums text-foreground">
                  <div class="flex items-center gap-1.5">
                    <span>{{ formatNumber(account.priority) }}</span>
                    <span v-if="account.priorityConflict" class="text-[11px] text-amber-600 dark:text-amber-400" :title="t(`${detailPrefix}.priorityConflictShort`)">{{ account.priorityConflictValue ?? account.priority }} → {{ account.priorityExpected ?? '-' }}</span>
                    <ArrowDownUp v-else-if="account.priorityManaged" class="h-3.5 w-3.5 text-primary" />
                  </div>
                  <p class="mt-0.5 text-[11px] text-muted-foreground">{{ priorityStateLabel(account) }}</p>
                  <p v-if="account.priorityCapacityLimited" class="mt-0.5 text-[11px] text-amber-600 dark:text-amber-400">
                    {{ t(`${detailPrefix}.priorityCapacityLimited`) }}
                  </p>
                </td>
                <td class="px-3 py-3 tabular-nums text-foreground">
                  <span>{{ formatMultiplier(effectiveMultiplier(account)) }}</span>
                  <span class="mt-0.5 block max-w-40 text-[11px] leading-4 text-muted-foreground">
                    {{ multiplierSourceLabel(account) }}
                  </span>
                </td>
                <td class="px-3 py-3 tabular-nums text-foreground">
                  <span v-if="account.upstreamKeyGroupMultiplier == null" class="text-xs text-muted-foreground">
                    {{ upstreamMultiplierStatusLabel(account) }}
                  </span>
                  <span v-else>{{ formatMultiplier(account.upstreamKeyGroupMultiplier) }}</span>
                  <span v-if="account.upstreamKeyGroupName" class="mt-0.5 block max-w-32 truncate text-[11px] text-muted-foreground">
                    {{ account.upstreamKeyGroupName }}
                  </span>
                </td>
                <td class="px-3 py-3 tabular-nums">
                  <span v-if="aggregateState(account) === 'suspended'" class="text-destructive">-</span>
                  <span v-else-if="accountLatency(account) == null" class="text-muted-foreground">-</span>
                  <span v-else :class="accountHasSlowResponse(account) ? 'text-amber-600 dark:text-amber-400' : 'text-foreground'">
                    {{ accountLatency(account) }} ms
                  </span>
                  <span v-if="accountHasSlowResponse(account)" class="mt-0.5 block text-[11px] text-amber-600 dark:text-amber-400">
                    {{ t(`${detailPrefix}.slowResponse`) }}
                  </span>
                </td>
                <td class="px-3 py-3">
                  <div class="flex items-center justify-end gap-1">
                    <Tooltip :text="t(`${prefix}.actions.probe`)">
                      <button
                        type="button"
                        class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface hover:text-primary disabled:opacity-35"
                        :aria-label="t(`${prefix}.actions.probe`)"
                        :disabled="!account.probeAvailable"
                        @click="emit('probe', account)"
                      >
                        <Zap class="h-4 w-4" />
                      </button>
                    </Tooltip>
                    <Tooltip v-if="isSub2API(account) && account.schedulable != null" :text="account.schedulable ? t(`${detailPrefix}.actions.disableScheduling`) : t(`${detailPrefix}.actions.enableScheduling`)" wide>
                      <button
                        type="button"
                        class="rounded-md p-1.5 transition-colors disabled:cursor-not-allowed disabled:opacity-35"
                        :class="account.schedulable
                          ? 'text-emerald-600 hover:bg-emerald-500/10 hover:text-emerald-700 dark:text-emerald-400 dark:hover:text-emerald-300'
                          : 'text-muted-foreground hover:bg-surface hover:text-foreground'"
                        :aria-label="account.schedulable ? t(`${detailPrefix}.actions.disableScheduling`) : t(`${detailPrefix}.actions.enableScheduling`)"
                        :disabled="actionLoading"
                        @click="emit('set-schedulable', account)"
                      >
                        <Power class="h-4 w-4" />
                      </button>
                    </Tooltip>
                    <Tooltip :text="t(`${prefix}.actions.viewEvents`)">
                      <button
                        type="button"
                        class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface hover:text-foreground"
                        :aria-label="t(`${prefix}.actions.viewEvents`)"
                        @click="emit('view-events', account)"
                      >
                        <Eye class="h-4 w-4" />
                      </button>
                    </Tooltip>
                  </div>
                </td>
              </tr>
              <tr v-if="expandedTargetId === account.targetId" class="border-t border-border/40 bg-surface/25">
                <td colspan="9" class="px-12 py-4">
                  <div v-if="filteredModelHealth(account).length === 0 && filteredUnprobedModels(account).length === 0" class="text-xs text-muted-foreground">{{ t(`${detailPrefix}.models.empty`) }}</div>
                  <div v-else class="grid gap-2 lg:grid-cols-2">
                    <div v-for="model in filteredModelHealth(account)" :key="model.modelName" class="rounded-lg border border-border/50 bg-background px-3 py-2.5">
                      <div class="flex items-center justify-between gap-3">
                        <span class="truncate text-sm font-medium text-foreground">{{ model.modelName }}</span>
                        <span class="rounded-md px-2 py-0.5 text-xs font-medium" :class="model.configured ? connectionHealthStateBadgeClass(model.state) : 'bg-muted text-muted-foreground'">{{ model.configured ? t(`${prefix}.stateLabels.${model.state}`) : t(`${prefix}.notConfigured`) }}</span>
                      </div>
                      <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                        <span :class="model.state === 'suspended' ? 'text-destructive' : ''">{{ t(`${detailPrefix}.models.latency`, { value: !model.configured || model.state === 'suspended' ? '-' : (model.lastLatencyMs ?? '-') }) }}</span>
                        <span>{{ t(`${detailPrefix}.models.lastProbe`, { value: formatConnectionHealthTime(model.lastProbeAt) }) }}</span>
                        <span>{{ t(`${detailPrefix}.models.weight`, { value: model.currentWeight }) }}</span>
                        <span v-if="model.nextProbeAt">{{ t(`${detailPrefix}.models.nextProbe`, { value: formatConnectionHealthTime(model.nextProbeAt) }) }}</span>
                        <span v-if="model.blockedReason">{{ blockedReasonLabel(model.blockedReason) }}</span>
                      </div>
                      <p v-if="model.lastErrorKey" class="mt-2 truncate text-xs text-destructive">{{ readableMessage(model.lastErrorKey) }}</p>
                      <p v-if="effectiveSourcesLabel(model)" class="mt-1 text-xs text-muted-foreground">{{ effectiveSourcesLabel(model) }}</p>
                    </div>
                    <div v-for="model in filteredUnprobedModels(account)" :key="`unprobed:${model.modelName}`" class="rounded-lg border border-border/50 bg-background px-3 py-2.5">
                      <div class="flex items-center justify-between gap-3">
                        <span class="truncate text-sm font-medium text-foreground">{{ model.modelName }}</span>
                        <span class="rounded-md bg-muted px-2 py-0.5 text-xs text-muted-foreground">{{ account.probeModelsConfigured === false ? t(`${prefix}.notConfigured`) : t(`${prefix}.notProbed`) }}</span>
                      </div>
                      <div class="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-muted-foreground">
                        <span>{{ t(`${detailPrefix}.models.latency`, { value: '-' }) }}</span>
                        <span>{{ t(`${detailPrefix}.models.lastProbe`, { value: formatConnectionHealthTime(null) }) }}</span>
                        <span>{{ t(`${detailPrefix}.models.weight`, { value: '-' }) }}</span>
                        <span v-if="model.nextProbeAt">{{ t(`${detailPrefix}.models.nextProbe`, { value: formatConnectionHealthTime(model.nextProbeAt) }) }}</span>
                        <span v-if="model.blockedReason">{{ blockedReasonLabel(model.blockedReason) }}</span>
                      </div>
                      <p v-if="effectiveSourcesLabel(model)" class="mt-1 text-xs text-muted-foreground">{{ effectiveSourcesLabel(model) }}</p>
                    </div>
                  </div>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </div>
  </section>
</template>
