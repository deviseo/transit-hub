<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'
import { AlertCircle, ArrowUpDown, Check, ChevronDown, History, KeyRound, Link2, Loader2, Megaphone, RefreshCw, Search, ServerCog, Sparkles, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { getMySiteMappingOptions, realConnect, realBind, listAdminResources, listUpstreamKeys, listRealConnections, realDisconnect } from '../api/mySites'
import { getDashboardAdminStatus } from '../api/dashboardAdmin'
import { useGroupRates } from '../composables/useGroupRates'
import type { GroupRate, GroupRateHistoryRow } from '../types/groupRates'
import type { AdminResourceOption, ConnectionCapabilities, MySiteMapping, MySiteMappingOwnGroupOption, RealConnection, UpstreamKeyItem } from '../types/mySites'
import { LEGACY_NEW_API_CHANNEL_SUGGESTIONS, NEW_API_CHANNEL_TYPES } from '../types/mySites'

const { t, locale } = useI18n()
const router = useRouter()

const {
  rates,
  history,
  total,
  page,
  pageSize,
  totalPages,
  types,
  platforms,
  typeFilter,
  platformFilter,
  statusFilter,
  sortMode,
  statusCounts,
  serverSupportsStatusFilters,
  isLoading,
  isHistoryLoading,
  isActionLoading,
  errorKey,
  historyErrorKey,
  loadRates,
  loadHistory,
  saveType,
  setSearch,
  setTypeFilter,
  setPlatformFilter,
  setStatusFilter,
  setSortMode,
  goToPage,
} = useGroupRates()

const selectedRate = ref<GroupRate | null>(null)
const isHistoryOpen = ref(false)
const editingRate = ref<GroupRate | null>(null)
const connectingRate = ref<GroupRate | null>(null)
const editTypeValue = ref('')
const connectOwnGroups = ref<string[]>([])
const connectMode = ref<'real' | 'bind'>('real')
const ownGroups = ref<MySiteMappingOwnGroupOption[]>([])
const mySiteMappings = ref<MySiteMapping[]>([])
const hasLoadedMappingOptions = ref(false)
const connectionCapabilities = ref<ConnectionCapabilities | null>(null)
const searchQuery = ref('')
const realConnectionsData = ref<RealConnection[]>([])
const disconnectingRate = ref<GroupRate | null>(null)
const disconnectMode = ref<'unlink' | 'full'>('unlink')
const disconnectRemovePricing = ref(true)
const isDisconnecting = ref(false)
const disconnectError = ref('')
const isAnyDialogOpen = computed(() => Boolean(isHistoryOpen.value || editingRate.value || connectingRate.value || disconnectingRate.value))
let previouslyFocusedElement: HTMLElement | null = null
let previousBodyOverflow = ''

const dialogFocusableSelector = [
  'button:not([disabled])',
  'a[href]',
  'input:not([disabled])',
  'select:not([disabled])',
  'textarea:not([disabled])',
  '[tabindex]:not([tabindex="-1"])',
].join(',')

const handleDialogKeydown = (event: KeyboardEvent) => {
  const dialog = document.querySelector<HTMLElement>('[data-group-rates-dialog]')
  if (!dialog) return
  if (event.key === 'Escape') {
    event.preventDefault()
    if (disconnectingRate.value && !isDisconnecting.value) closeDisconnect()
    else if (connectingRate.value && !isActionLoading.value) closeConnector()
    else if (editingRate.value && !isActionLoading.value) closeTypeEditor()
    else if (isHistoryOpen.value) closeHistory()
    return
  }
  if (event.key !== 'Tab') return
  const focusableElements = Array.from(dialog.querySelectorAll<HTMLElement>(dialogFocusableSelector))
  if (focusableElements.length === 0) return
  const first = focusableElements[0]
  const last = focusableElements[focusableElements.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}
const selectedGroupType = ref('')
const selectedChannelType = ref(0)
const adminPlatform = ref('')
const upstreamKeys = ref<UpstreamKeyItem[]>([])
const selectedKeyId = ref('')
const isLoadingKeys = ref(false)
const selectedAdminGroupId = ref('')
const adminResources = ref<AdminResourceOption[]>([])
const selectedAdminResourceId = ref('')
const isLoadingAdminResources = ref(false)
const addToPricingMapping = ref(true)
const connectOperationId = ref('')
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null

const editTypeOptions = computed(() => {
  const options = new Set(types.value)
  if (editingRate.value?.type) options.add(editingRate.value.type)
  return Array.from(options).sort((first, second) => first.localeCompare(second))
})
const mappedOwnGroupsForRate = (rate: GroupRate): string[] => (
  mySiteMappings.value
    .filter((mapping) => mapping.upstreamTargets.some((target) => target.siteId === rate.siteId && target.groupName === rate.groupName))
    .map((mapping) => mapping.ownGroup)
)

const firstMappedOwnGroupForRate = (rate: GroupRate): string => mappedOwnGroupsForRate(rate)[0] ?? ''

const filteredOwnGroups = computed(() => {
  // new-api admin 不按渠道类型筛选，直接显示全部自有分组
  if (isAdminNewAPI.value) return ownGroups.value
  const upstreamType = (connectingRate.value?.type || selectedGroupType.value).toLowerCase()
  if (upstreamType) {
    return ownGroups.value.filter(g => g.platform.toLowerCase() === upstreamType)
  }
  return ownGroups.value
})

const realConnectionForRate = (rate: GroupRate): RealConnection | undefined =>
  realConnectionsData.value.find(c => (
    c.upstreamSiteId === rate.siteId &&
    (c.upstreamGroupId === rate.groupId || ((!c.upstreamGroupId || !rate.groupId) && c.upstreamGroupName === rate.groupName))
  ))

const isRealConnected = (rate: GroupRate): boolean => !!realConnectionForRate(rate)
const isPricingMapped = (rate: GroupRate): boolean => rate.pricingMapped ?? mappedOwnGroupsForRate(rate).length > 0
const disconnectConnection = computed(() => disconnectingRate.value ? realConnectionForRate(disconnectingRate.value) : undefined)

const loadRealConnections = async () => {
  try {
    realConnectionsData.value = await listRealConnections()
  } catch {
    realConnectionsData.value = []
  }
}

const loadAdminPlatform = async () => {
  try {
    const status = await getDashboardAdminStatus()
    adminPlatform.value = status.platform ?? ''
  } catch {
    adminPlatform.value = ''
  }
}

const filteredRates = computed(() => {
  if (serverSupportsStatusFilters.value) return rates.value

  const filtered = rates.value.filter(rate => {
    const typeMatch = !typeFilter.value || rate.type === typeFilter.value
    const platformMatch = !platformFilter.value || rate.platform === platformFilter.value

    if (statusFilter.value === 'deleted') {
      return typeMatch && platformMatch && rate.deleted
    }

    if (rate.deleted) return false

    const mappedMatch = statusFilter.value === 'all' ||
      (statusFilter.value === 'mapped' && rate.mapped) ||
      (statusFilter.value === 'unmapped' && !rate.mapped)

    return typeMatch && platformMatch && mappedMatch
  })

  return [...filtered].sort((a, b) => {
    switch (sortMode.value) {
      case 'multiplierAsc':
        return (a.currentMultiplier ?? Infinity) - (b.currentMultiplier ?? Infinity)
      case 'multiplierDesc':
        return (b.currentMultiplier ?? -Infinity) - (a.currentMultiplier ?? -Infinity)
      case 'siteNameAsc':
        return a.siteName.localeCompare(b.siteName)
      case 'groupNameAsc':
        return a.groupName.localeCompare(b.groupName)
    }
  })
})
const canGoPrevious = computed(() => page.value > 1 && !isLoading.value)
const canGoNext = computed(() => page.value < totalPages.value && !isLoading.value)

watch(searchQuery, (value) => {
  if (searchDebounceTimer) clearTimeout(searchDebounceTimer)
  searchDebounceTimer = setTimeout(() => {
    void setSearch(value)
  }, 300)
})
const hasAnyRateData = computed(() => Object.values(statusCounts.value).some(count => count > 0))
const hasActiveRateFilters = computed(() => Boolean(
  searchQuery.value.trim() ||
  typeFilter.value ||
  platformFilter.value ||
  statusFilter.value !== 'all',
))

watch(isAnyDialogOpen, async (open) => {
  if (open) {
    previouslyFocusedElement = document.activeElement as HTMLElement | null
    previousBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    await nextTick()
    document.querySelector<HTMLElement>('[data-group-rates-dialog] button:not([disabled]), [data-group-rates-dialog] input:not([disabled])')?.focus()
    return
  }
  document.body.style.overflow = previousBodyOverflow
  previouslyFocusedElement?.focus()
  previouslyFocusedElement = null
})

onMounted(() => {
  document.addEventListener('keydown', handleDialogKeydown)
	void Promise.all([loadRates(), loadRealConnections(), loadAdminPlatform()])
})

onBeforeUnmount(() => {
  if (searchDebounceTimer) clearTimeout(searchDebounceTimer)
  document.removeEventListener('keydown', handleDialogKeydown)
  document.body.style.overflow = previousBodyOverflow
})

const isAdminNewAPI = computed(() => adminPlatform.value === 'newapi')
const needsGroupTypeSelection = computed(() => (
  connectMode.value === 'real' &&
  (connectionCapabilities.value?.requiresGroupType ?? (!connectingRate.value?.type && !isAdminNewAPI.value))
) && !connectingRate.value?.type)
const needsChannelTypeSelection = computed(() => connectMode.value === 'real' && (connectionCapabilities.value?.requiresChannelType ?? isAdminNewAPI.value))

// new-api admin：根据自有分组类型过滤可选的渠道类型
// 分组类型已知时只显示对应渠道，未知时显示全部
const filteredChannelTypes = computed(() => {
  const groupType = (connectingRate.value?.type || '').toLowerCase()
  const available = connectionCapabilities.value?.channelTypes?.length
    ? connectionCapabilities.value.channelTypes
    : NEW_API_CHANNEL_TYPES
  const suggestedId = connectionCapabilities.value?.suggestedChannelTypeByGroup?.[groupType]
    ?? LEGACY_NEW_API_CHANNEL_SUGGESTIONS[groupType]
  if (suggestedId) {
    return available.filter(channelType => channelType.id === suggestedId)
  }
  return available
})

const canSubmitConnect = computed(() => {
  if (!connectingRate.value) return false
  if (connectMode.value === 'bind') {
    return Boolean(selectedKeyId.value && selectedAdminGroupId.value && selectedAdminResourceId.value)
  }
  if (connectOwnGroups.value.length === 0) return false
  // sub2api admin：分组类型未知时必须手动选择
  if (needsGroupTypeSelection.value && !selectedGroupType.value) return false
  // new-api admin：必须选择渠道类型
  if (needsChannelTypeSelection.value && !selectedChannelType.value) return false
  return true
})

const handleTypeChange = async (event: Event) => {
  const target = event.target as HTMLSelectElement
  await setTypeFilter(target.value)
}

const formatMultiplier = (value: number | null): string => {
  if (value === null || !Number.isFinite(value)) return t('admin.groupRates.common.placeholder')
  return t('admin.groupRates.format.multiplier', { value: Number(value.toFixed(4)).toString() })
}

const formatDelta = (delta: number | null): string => {
  if (delta === null || !Number.isFinite(delta)) return t('admin.groupRates.common.placeholder')

  const sign = delta > 0 ? '+' : ''
  const deltaValue = `${sign}${Number(delta.toFixed(4)).toString()}`
  return t('admin.groupRates.format.deltaMultiplier', { value: deltaValue })
}

const formatDateTime = (value: string | null): string => {
  if (!value) return t('admin.groupRates.common.placeholder')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('admin.groupRates.common.placeholder')
  return new Intl.DateTimeFormat(locale.value, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const platformLabel = (platform: string | null): string => {
  if (!platform) return t('admin.groupRates.common.unknown')
  if (platform === 'newapi') return t('admin.groupRates.platforms.newapi')
  if (platform === 'sub2api') return t('admin.groupRates.platforms.sub2api')
  return platform
}

const typeLabel = (type: string | null): string => {
  if (!type) return t('admin.groupRates.common.unknown')
  return type
}

const platformClasses = (platform: string | null): string => {
  if (platform === 'newapi') return 'border-sky-400/30 bg-sky-500/10 text-sky-600 dark:text-sky-300'
  if (platform === 'sub2api') return 'border-violet-400/30 bg-violet-500/10 text-violet-600 dark:text-violet-300'
  return 'border-border/60 bg-surface-elevated text-muted-foreground'
}

const typeClasses = (type: string | null): string => {
  if (!type) return 'border-border/60 bg-surface-elevated text-muted-foreground'

  let hash = 0
  for (const char of type) {
    hash = (hash + char.charCodeAt(0)) % 4
  }

  return [
    'border-emerald-400/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300',
    'border-amber-400/30 bg-amber-500/10 text-amber-600 dark:text-amber-300',
    'border-rose-400/30 bg-rose-500/10 text-rose-600 dark:text-rose-300',
    'border-cyan-400/30 bg-cyan-500/10 text-cyan-600 dark:text-cyan-300',
  ][hash]
}

const deltaClasses = (delta: number | null): string => {
  if (delta === null || !Number.isFinite(delta)) return 'bg-surface-elevated text-muted-foreground border-border/50'
  if (delta > 0) return 'bg-red-500/10 text-red-500 border-red-500/20'
  if (delta < 0) return 'bg-emerald-500/10 text-emerald-500 border-emerald-500/20'
  return 'bg-primary/10 text-primary border-primary/20'
}

const historyActionLabel = (rate: GroupRate): string => (
  t('admin.groupRates.actions.viewHistoryForRate', {
    site: rate.siteName,
    group: rate.groupName,
    delta: formatDelta(rate.delta),
  })
)

const openHistory = async (rate: GroupRate) => {
  selectedRate.value = rate
  isHistoryOpen.value = true
  await loadHistory({
    siteId: rate.siteId,
    groupId: rate.groupId,
    groupName: rate.groupId || rate.groupName,
    platform: rate.platform,
  })
}

const closeHistory = () => {
  isHistoryOpen.value = false
  selectedRate.value = null
}

const openTypeEditor = (rate: GroupRate) => {
  editingRate.value = rate
  editTypeValue.value = rate.type ?? ''
}

const closeTypeEditor = () => {
  editingRate.value = null
  editTypeValue.value = ''
}

const openConnector = async (rate: GroupRate) => {
  connectingRate.value = rate
  connectOwnGroups.value = []
  connectMode.value = 'real'
  selectedGroupType.value = ''
  selectedChannelType.value = 0
  addToPricingMapping.value = true
  connectOperationId.value = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random().toString(36).slice(2)}`
  await loadMySiteMappingData()
}

const isActiveResourceStatus = (status: string): boolean => ['1', 'active', 'enabled'].includes(status.toLowerCase())

const resourceStatusLabel = (status: string): string => (
  isActiveResourceStatus(status)
    ? t('admin.groupRates.connect.resourceActive')
    : t('admin.groupRates.connect.resourceInactive')
)

const adminResourceTypeLabel = (resource: AdminResourceOption): string => {
  if (adminPlatform.value === 'newapi') {
    const channelType = NEW_API_CHANNEL_TYPES.find(item => item.id === Number(resource.type))
    if (channelType) return channelType.name
  }
  return resource.platform || resource.type || t('admin.groupRates.common.unknown')
}

const closeConnector = () => {
  connectingRate.value = null
  connectOwnGroups.value = []
  connectMode.value = 'real'
  realConnectError.value = ''
  selectedGroupType.value = ''
  selectedChannelType.value = 0
  upstreamKeys.value = []
  selectedKeyId.value = ''
  isLoadingKeys.value = false
  selectedAdminGroupId.value = ''
  adminResources.value = []
  selectedAdminResourceId.value = ''
  isLoadingAdminResources.value = false
  addToPricingMapping.value = true
  connectOperationId.value = ''
}

const setConnectMode = async (mode: 'real' | 'bind') => {
  connectMode.value = mode
  connectOwnGroups.value = []
  selectedGroupType.value = ''
  selectedChannelType.value = 0
  selectedKeyId.value = ''
  selectedAdminGroupId.value = ''
  selectedAdminResourceId.value = ''
  adminResources.value = []
  realConnectError.value = ''
  if (mode === 'bind' && connectingRate.value) {
    await loadUpstreamKeys(connectingRate.value)
  }
}

const loadAdminResourcesForGroup = async (groupId: string) => {
  selectedAdminGroupId.value = groupId
  selectedAdminResourceId.value = ''
  adminResources.value = []
  if (!groupId) return
  isLoadingAdminResources.value = true
  try {
    adminResources.value = await listAdminResources(groupId)
  } catch {
    adminResources.value = []
    realConnectError.value = t('admin.groupRates.connect.adminResourcesFailed')
  } finally {
    isLoadingAdminResources.value = false
  }
}

const handleAdminGroupChange = (event: Event) => {
  void loadAdminResourcesForGroup((event.target as HTMLSelectElement).value)
}

const submitTypeEditor = async () => {
  if (!editingRate.value) return
  await saveType(editingRate.value, editTypeValue.value.trim())
  closeTypeEditor()
}

const loadMySiteMappingData = async (force = false) => {
  if (hasLoadedMappingOptions.value && !force) return
  isActionLoading.value = true
  try {
    const options = await getMySiteMappingOptions()
    ownGroups.value = options.ownGroups
    mySiteMappings.value = options.mappings ?? []
    connectionCapabilities.value = options.connectionCapabilities ?? null
    hasLoadedMappingOptions.value = true
  } finally {
    isActionLoading.value = false
  }
}

const toggleOwnGroup = (groupId: string) => {
  const index = connectOwnGroups.value.indexOf(groupId)
  if (index === -1) {
    connectOwnGroups.value = [...connectOwnGroups.value, groupId]
  } else {
    connectOwnGroups.value = connectOwnGroups.value.filter(id => id !== groupId)
  }
}

const submitConnector = async () => {
  if (!connectingRate.value || !canSubmitConnect.value) return

  if (connectMode.value === 'bind') {
    await submitBind()
  } else {
    await submitRealConnect()
  }
}

const realConnectError = ref('')

const refreshAfterMutation = async () => {
  try {
    await Promise.all([loadRates(), loadRealConnections(), loadMySiteMappingData(true)])
  } catch {
    errorKey.value = 'admin.groupRates.errors.refreshFailed'
  }
}

const handlePlatformChange = async (event: Event) => {
  const target = event.target as HTMLSelectElement
  await setPlatformFilter(target.value)
}

const handleStatusChange = async (status: 'all' | 'mapped' | 'unmapped' | 'deleted') => {
  if (status === statusFilter.value || isLoading.value) return
  await setStatusFilter(status)
}

const handleSortChange = async (event: Event) => {
  const target = event.target as HTMLSelectElement
  await setSortMode(target.value as 'multiplierAsc' | 'multiplierDesc' | 'siteNameAsc' | 'groupNameAsc')
}

const submitRealConnect = async () => {
  if (!connectingRate.value || connectOwnGroups.value.length === 0) return
  realConnectError.value = ''
  isActionLoading.value = true
  const payload = {
    upstreamSiteId: connectingRate.value.siteId,
    upstreamGroupId: connectingRate.value.groupId ?? '',
    upstreamGroupName: connectingRate.value.groupName,
    groupType: selectedGroupType.value,
    channelType: selectedChannelType.value || undefined,
    ownGroupIds: connectOwnGroups.value,
    addToPricingMapping: addToPricingMapping.value,
    operationId: connectOperationId.value,
  }
  try {
    await realConnect(payload)
    closeConnector()
  } catch {
    realConnectError.value = t('admin.groupRates.connect.realFailed')
    isActionLoading.value = false
    return
  }

  await refreshAfterMutation()
  isActionLoading.value = false
}

const loadUpstreamKeys = async (rate: GroupRate) => {
  isLoadingKeys.value = true
  try {
    upstreamKeys.value = await listUpstreamKeys(rate.siteId, rate.groupId ?? '', rate.groupName)
  } catch {
    upstreamKeys.value = []
  } finally {
    isLoadingKeys.value = false
  }
}

const submitBind = async () => {
  if (!connectingRate.value || !selectedKeyId.value || !selectedAdminGroupId.value || !selectedAdminResourceId.value) return
  const selectedKey = upstreamKeys.value.find(k => k.id === selectedKeyId.value)
  if (!selectedKey) return
  realConnectError.value = ''
  isActionLoading.value = true
  try {
    await realBind({
      upstreamSiteId: connectingRate.value.siteId,
      upstreamGroupId: connectingRate.value.groupId ?? '',
      upstreamGroupName: connectingRate.value.groupName,
      upstreamKeyId: selectedKey.id,
      upstreamKey: selectedKey.key,
      ownGroupIds: [selectedAdminGroupId.value],
      groupType: selectedGroupType.value,
      adminGroupId: selectedAdminGroupId.value,
      adminResourceId: selectedAdminResourceId.value,
      addToPricingMapping: addToPricingMapping.value,
      operationId: connectOperationId.value,
    })
    closeConnector()
  } catch {
    realConnectError.value = t('admin.groupRates.connect.bindFailed')
    isActionLoading.value = false
    return
  }

  await refreshAfterMutation()
  isActionLoading.value = false
}

const openDisconnect = (rate: GroupRate) => {
  disconnectingRate.value = rate
  disconnectMode.value = 'unlink'
  disconnectRemovePricing.value = realConnectionForRate(rate)?.pricingMappingEnabled ?? Boolean(rate.pricingMapped)
  disconnectError.value = ''
}

const closeDisconnect = () => {
  disconnectingRate.value = null
  disconnectMode.value = 'unlink'
  disconnectRemovePricing.value = true
  disconnectError.value = ''
}

const submitDisconnect = async () => {
  if (!disconnectingRate.value) return
  const conn = realConnectionForRate(disconnectingRate.value)
  if (!conn) return

  isDisconnecting.value = true
  disconnectError.value = ''
  try {
    await realDisconnect({
      connectionId: conn.id,
      mode: disconnectMode.value,
      removePricingMapping: disconnectRemovePricing.value,
    })
    closeDisconnect()
  } catch {
    disconnectError.value = t('admin.groupRates.disconnect.failed')
    isDisconnecting.value = false
    return
  }

  await refreshAfterMutation()
  isDisconnecting.value = false
}

const historyTitle = computed(() => {
  if (!selectedRate.value) return t('admin.groupRates.history.title')
  return t('admin.groupRates.history.titleWithGroup', {
    site: selectedRate.value.siteName,
    group: selectedRate.value.groupName,
  })
})

const editTypeTitle = computed(() => {
  if (!editingRate.value) return t('admin.groupRates.edit.title')
  return t('admin.groupRates.edit.titleWithGroup', {
    site: editingRate.value.siteName,
    group: editingRate.value.groupName,
  })
})

const historyRowKey = (row: GroupRateHistoryRow, index: number): string => (
  `${row.siteId}-${row.groupId || row.groupName}-${row.platform ?? 'all'}-${row.createdAt ?? index}`
)

</script>

<template>
  <div class="flex min-h-[calc(100dvh-8rem)] flex-col space-y-6 lg:h-[calc(100dvh-8rem)]">
    <div class="flex max-w-full w-fit shrink-0 items-center gap-1 overflow-x-auto rounded-lg border border-border/50 bg-surface p-1" role="tablist" :aria-label="t('admin.menu.groupRates')">
      <button
        v-for="tab in (['all', 'mapped', 'unmapped', 'deleted'] as const)"
        :key="tab"
        type="button"
        role="tab"
        :aria-selected="statusFilter === tab"
        aria-controls="group-rates-panel"
        :class="[
          'shrink-0 rounded-md px-4 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary',
          statusFilter === tab
            ? 'bg-primary text-primary-foreground shadow-sm'
            : 'text-muted-foreground hover:text-foreground hover:bg-surface-elevated'
        ]"
        @click="handleStatusChange(tab)"
      >
        <span>{{ t(`admin.groupRates.tabs.${tab}`) }}</span>
        <span
          class="ml-2 rounded bg-background/60 px-1.5 py-0.5 text-[11px] tabular-nums"
          :class="statusFilter === tab ? 'text-primary-foreground' : 'text-muted-foreground'"
        >
          {{ statusCounts[tab] }}
        </span>
      </button>
    </div>

    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 shrink-0">
      <div class="flex w-full flex-1 flex-col gap-3 sm:flex-row sm:flex-wrap lg:flex-nowrap">
        <div class="relative w-full sm:w-80 max-w-sm">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <input
            v-model="searchQuery"
            name="groupRateSearch"
            type="text"
            :placeholder="t('admin.groupRates.filters.searchPlaceholder')"
            :aria-label="t('admin.groupRates.filters.searchPlaceholder')"
            autocomplete="off"
            spellcheck="false"
            class="h-10 w-full rounded-lg border border-border/50 bg-surface pl-10 pr-4 text-sm text-foreground outline-none transition-[color,background-color,border-color,box-shadow] placeholder:text-muted-foreground focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/30"
          />
        </div>

        <div class="relative w-full sm:w-48 sm:shrink-0">
          <select
            v-model="typeFilter"
            class="h-10 w-full appearance-none rounded-lg border border-border/50 bg-surface px-3 pr-8 text-sm text-foreground outline-none transition-[color,background-color,border-color,box-shadow] focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/30"
            @change="handleTypeChange"
          >
            <option value="">{{ t('admin.groupRates.common.allTypes') }}</option>
            <option v-for="type in types" :key="type" :value="type">{{ typeLabel(type) }}</option>
          </select>
          <ChevronDown class="pointer-events-none absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
        </div>

        <div class="relative w-full sm:w-44 sm:shrink-0">
          <select
            v-model="platformFilter"
            class="h-10 w-full appearance-none rounded-lg border border-border/50 bg-surface px-3 pr-8 text-sm text-foreground outline-none transition-[color,background-color,border-color,box-shadow] focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/30"
            @change="handlePlatformChange"
          >
            <option value="">{{ t('admin.groupRates.common.allPlatforms') }}</option>
            <option v-for="platform in platforms" :key="platform" :value="platform">{{ platformLabel(platform) }}</option>
          </select>
          <ChevronDown class="pointer-events-none absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
        </div>

        <div class="relative w-full sm:w-52 sm:shrink-0">
          <ArrowUpDown class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground pointer-events-none" />
          <select
            v-model="sortMode"
            class="h-10 w-full appearance-none rounded-lg border border-border/50 bg-surface pl-9 pr-8 text-sm text-foreground outline-none transition-[color,background-color,border-color,box-shadow] focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/30"
            @change="handleSortChange"
          >
            <option value="multiplierAsc">{{ t('admin.groupRates.sort.multiplierAsc') }}</option>
            <option value="multiplierDesc">{{ t('admin.groupRates.sort.multiplierDesc') }}</option>
            <option value="siteNameAsc">{{ t('admin.groupRates.sort.siteNameAsc') }}</option>
            <option value="groupNameAsc">{{ t('admin.groupRates.sort.groupNameAsc') }}</option>
          </select>
          <ChevronDown class="pointer-events-none absolute right-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
        </div>
      </div>

      <div class="grid w-full shrink-0 grid-cols-1 gap-2 sm:w-auto sm:grid-cols-2">
        <Button variant="secondary" class="h-10 gap-2" @click="router.push('/admin/group-rate-campaigns?action=create')">
          <Megaphone class="h-4 w-4" />
          {{ t('admin.groupRates.actions.createCampaign') }}
        </Button>
        <Button class="h-10 gap-2 shadow-sm" :disabled="isLoading" @click="loadRates">
          <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" />
          <RefreshCw v-else class="h-4 w-4" />
          {{ t('admin.groupRates.actions.refresh') }}
        </Button>
      </div>
    </div>

    <div v-if="errorKey" class="flex items-start gap-3 rounded-2xl border border-warning/20 bg-warning/10 p-4 text-sm text-warning shrink-0">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ t(errorKey) }}</span>
    </div>

    <div id="group-rates-panel" class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-border/50 bg-card shadow-sm" role="tabpanel">
      <div v-if="isLoading" class="flex flex-1 items-center justify-center text-muted-foreground">
        <Loader2 class="mr-2 h-5 w-5 animate-spin" />
        {{ t('admin.groupRates.status.loading') }}
      </div>

      <div v-else-if="filteredRates.length === 0" class="flex flex-1 flex-col items-center justify-center px-6 text-center">
        <div class="flex h-12 w-12 items-center justify-center rounded-2xl border border-border/50 bg-surface-elevated text-muted-foreground">
          <History class="h-5 w-5" />
        </div>
        <h3 class="mt-4 font-semibold text-foreground">{{ t(hasActiveRateFilters || hasAnyRateData ? 'admin.groupRates.empty.filteredTitle' : 'admin.groupRates.empty.title') }}</h3>
        <p class="mt-2 max-w-sm text-sm text-muted-foreground">{{ t(hasActiveRateFilters || hasAnyRateData ? 'admin.groupRates.empty.filteredDescription' : 'admin.groupRates.empty.description') }}</p>
      </div>

      <div v-else class="flex-1 overflow-auto">
        <table class="w-full min-w-[980px] text-left text-sm relative">
          <thead class="sticky top-0 z-10 border-b border-border/50 bg-surface-elevated/90 backdrop-blur-sm">
            <tr>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.siteName') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.groupName') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.type') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.platform') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.effectiveMultiplier') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.delta') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.updatedAt') }}</th>
              <th class="px-6 py-3 text-right font-medium text-muted-foreground">{{ t('admin.groupRates.fields.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/50">
            <tr v-for="rate in filteredRates" :key="`${rate.siteId}-${rate.groupName}-${rate.platform ?? 'all'}`" class="transition-colors hover:bg-surface/30">
              <td class="px-4 py-2.5">
                <div class="font-medium text-foreground">{{ rate.siteName }}</div>
              </td>
              <td class="px-4 py-2.5">
                <div class="flex items-center gap-1.5">
                  <span class="font-medium text-foreground">{{ rate.groupName }}</span>
                  <span v-if="rate.deleted" class="inline-flex rounded-md border border-red-500/20 bg-red-500/10 px-1.5 py-0.5 text-[10px] font-semibold text-red-500">{{ t('admin.groupRates.status.deleted') }}</span>
                  <span v-else-if="isRealConnected(rate)" class="inline-flex rounded-md border border-emerald-500/20 bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-600 dark:text-emerald-300">{{ t('admin.groupRates.status.mapped') }}</span>
                  <span v-if="!rate.deleted && isPricingMapped(rate)" class="inline-flex rounded-md border border-amber-500/20 bg-amber-500/10 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700 dark:text-amber-300">{{ t('admin.groupRates.status.pricingMapped') }}</span>
                </div>
              </td>
              <td class="px-4 py-2.5">
                <span :class="['inline-flex rounded-md border px-2 py-1 text-xs font-semibold uppercase tracking-wider', typeClasses(rate.type)]">
                  {{ typeLabel(rate.type) }}
                </span>
              </td>
              <td class="px-4 py-2.5">
                <span :class="['inline-flex rounded-md border px-2 py-1 text-xs font-semibold uppercase tracking-wider', platformClasses(rate.platform)]">
                  {{ platformLabel(rate.platform) }}
                </span>
              </td>
              <td class="px-4 py-2.5 tabular-nums">
                <div class="font-semibold text-foreground">{{ formatMultiplier(rate.currentMultiplier) }}</div>
                <div v-if="rate.upstreamMultiplier != null" class="mt-0.5 text-[11px] text-muted-foreground">
                  {{ t('admin.groupRates.fields.multiplierFormula', {
                    upstream: formatMultiplier(rate.upstreamMultiplier),
                    recharge: Number((rate.rechargeRate ?? 1).toFixed(4)).toString(),
                  }) }}
                </div>
              </td>
              <td class="px-4 py-2.5">
                <button
                  type="button"
                  :class="[
                    'inline-flex rounded-md border px-2.5 py-1 text-xs font-semibold transition-all hover:-translate-y-px hover:shadow-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-background',
                    deltaClasses(rate.delta),
                  ]"
                  :title="historyActionLabel(rate)"
                  :aria-label="historyActionLabel(rate)"
                  @click="openHistory(rate)"
                >
                  {{ formatDelta(rate.delta) }}
                </button>
              </td>
              <td class="px-4 py-2.5 text-muted-foreground tabular-nums">{{ formatDateTime(rate.updatedAt) }}</td>
              <td class="px-4 py-2.5 text-right">
                <div v-if="!rate.deleted" class="flex justify-end gap-2">
                  <Button
                    v-if="isRealConnected(rate)"
                    variant="destructive"
                    size="sm"
                    class="gap-1.5"
                    :disabled="isActionLoading || isDisconnecting"
                    @click="openDisconnect(rate)"
                  >
                    <X class="h-3.5 w-3.5" />
                    {{ t('admin.groupRates.disconnect.action') }}
                  </Button>
                  <Button
                    v-else
                    variant="secondary"
                    size="sm"
                    class="gap-1.5 text-primary hover:text-primary"
                    :disabled="isActionLoading"
                    @click="openConnector(rate)"
                  >
                    <Link2 class="h-3.5 w-3.5" />
                    {{ t('admin.groupRates.actions.connect') }}
                  </Button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="flex flex-col gap-3 border-t border-border/50 bg-surface-elevated/30 px-4 py-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span>{{ t('admin.groupRates.pagination.total', { total }) }}</span>
          <span>{{ t('admin.groupRates.pagination.pageSize', { pageSize }) }}</span>
          <span>{{ t('admin.groupRates.pagination.currentPage', { page, totalPages }) }}</span>
        </div>

        <div class="flex items-center gap-2">
          <Button variant="secondary" size="sm" :disabled="!canGoPrevious" @click="goToPage(page - 1)">
            {{ t('admin.groupRates.pagination.previous') }}
          </Button>
          <Button variant="secondary" size="sm" :disabled="!canGoNext" @click="goToPage(page + 1)">
            {{ t('admin.groupRates.pagination.next') }}
          </Button>
        </div>
      </div>
    </div>

    <div v-if="isHistoryOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4 backdrop-blur-sm">
      <div data-group-rates-dialog role="dialog" aria-modal="true" aria-labelledby="group-rate-history-title" tabindex="-1" class="max-h-[calc(100dvh-2rem)] w-full max-w-4xl overflow-hidden overscroll-contain rounded-xl border border-border/50 bg-card shadow-xl">
        <div class="flex items-start justify-between gap-4 border-b border-border/50 p-6">
          <div>
            <h2 id="group-rate-history-title" class="text-xl font-semibold text-foreground">{{ historyTitle }}</h2>
            <p v-if="selectedRate" class="mt-2 text-sm text-muted-foreground">
              {{ t('admin.groupRates.history.subtitle', { platform: platformLabel(selectedRate.platform) }) }}
            </p>
          </div>
          <button class="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-surface-line hover:text-foreground" @click="closeHistory">
            <X class="h-5 w-5" />
            <span class="sr-only">{{ t('admin.groupRates.actions.closeHistory') }}</span>
          </button>
        </div>

        <div v-if="historyErrorKey" class="m-6 flex items-start gap-3 rounded-xl border border-warning/20 bg-warning/10 p-4 text-sm text-warning">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ t(historyErrorKey) }}</span>
        </div>

        <div v-if="isHistoryLoading" class="flex min-h-[260px] items-center justify-center text-muted-foreground">
          <Loader2 class="mr-2 h-5 w-5 animate-spin" />
          {{ t('admin.groupRates.history.loading') }}
        </div>

        <div v-else-if="history.length === 0" class="flex min-h-[260px] flex-col items-center justify-center px-6 text-center">
          <History class="h-8 w-8 text-muted-foreground" />
          <h3 class="mt-4 font-semibold text-foreground">{{ t('admin.groupRates.history.emptyTitle') }}</h3>
          <p class="mt-2 text-sm text-muted-foreground">{{ t('admin.groupRates.history.emptyDescription') }}</p>
        </div>

        <div v-else class="max-h-[60vh] overflow-auto">
          <table class="w-full min-w-[680px] text-left text-sm">
            <thead class="sticky top-0 border-b border-border/50 bg-surface-elevated">
              <tr>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.siteName') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.groupName') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.type') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.platform') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.history.multiplier') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.history.delta') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.history.createdAt') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50">
              <tr v-for="(row, index) in history" :key="historyRowKey(row, index)" class="transition-colors hover:bg-surface/30">
                <td class="px-6 py-4 font-medium text-foreground">{{ row.siteName }}</td>
                <td class="px-6 py-4 text-foreground">
                  <div class="flex items-center gap-1.5">
                    <span>{{ row.groupName }}</span>
                    <span v-if="row.deleted" class="inline-flex rounded-md border border-red-500/20 bg-red-500/10 px-1.5 py-0.5 text-[10px] font-semibold text-red-500">{{ t('admin.groupRates.status.deleted') }}</span>
                  </div>
                </td>
                <td class="px-6 py-4 text-muted-foreground">{{ typeLabel(row.type) }}</td>
                <td class="px-6 py-4 text-muted-foreground">{{ platformLabel(row.platform) }}</td>
                <td class="px-6 py-4">
                  <span class="font-semibold text-foreground">{{ formatMultiplier(row.currentMultiplier ?? row.multiplier) }}</span>
                  <span v-if="row.currentMultiplier !== null && row.currentMultiplier !== row.multiplier" class="ml-1 text-[10px] text-muted-foreground">{{ formatMultiplier(row.multiplier) }}</span>
                </td>
                <td class="px-6 py-4">
                  <span
                    class="inline-flex items-center rounded-md border px-2 py-0.5 text-xs font-semibold"
                    :class="deltaClasses(row.delta)"
                  >
                    {{ formatDelta(row.delta) }}
                  </span>
                </td>
                <td class="px-6 py-4 text-muted-foreground">{{ formatDateTime(row.createdAt) }}</td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </div>

    <div v-if="editingRate" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4 backdrop-blur-sm">
      <div data-group-rates-dialog role="dialog" aria-modal="true" aria-labelledby="group-rate-edit-title" tabindex="-1" class="max-h-[calc(100dvh-2rem)] w-full max-w-md overflow-y-auto overscroll-contain rounded-xl border border-border/50 bg-card shadow-xl">
        <div class="flex items-start justify-between gap-4 border-b border-border/50 p-6">
          <div>
            <h2 id="group-rate-edit-title" class="text-xl font-semibold text-foreground">{{ editTypeTitle }}</h2>
            <p class="mt-2 text-sm text-muted-foreground">{{ t('admin.groupRates.edit.description') }}</p>
          </div>
          <button class="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-surface-line hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.groupRates.actions.cancel')" :disabled="isActionLoading" @click="closeTypeEditor">
            <X class="h-5 w-5" />
            <span class="sr-only">{{ t('admin.groupRates.actions.closeEdit') }}</span>
          </button>
        </div>

        <form class="space-y-5 p-6" @submit.prevent="submitTypeEditor">
          <label class="block space-y-2">
            <span class="text-sm font-medium text-foreground">{{ t('admin.groupRates.edit.typeLabel') }}</span>
            <select
              v-model="editTypeValue"
              class="h-11 w-full rounded-xl border border-border/70 bg-surface px-4 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
              :disabled="isActionLoading"
            >
              <option value="">{{ t('admin.groupRates.edit.typePlaceholder') }}</option>
              <option v-for="type in editTypeOptions" :key="type" :value="type">{{ typeLabel(type) }}</option>
            </select>
          </label>

          <div class="flex justify-end gap-2">
            <Button type="button" variant="secondary" :disabled="isActionLoading" @click="closeTypeEditor">
              {{ t('admin.groupRates.actions.cancel') }}
            </Button>
            <Button type="submit" class="gap-2" :disabled="isActionLoading">
              <Loader2 v-if="isActionLoading" class="h-4 w-4 animate-spin" />
              {{ t('admin.groupRates.actions.saveType') }}
            </Button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="connectingRate" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4 backdrop-blur-sm">
      <div data-group-rates-dialog role="dialog" aria-modal="true" aria-labelledby="group-rate-connect-title" tabindex="-1" class="max-h-[calc(100dvh-2rem)] w-full max-w-2xl overflow-y-auto overscroll-contain rounded-lg border border-border/60 bg-card shadow-xl">
        <div class="flex items-start justify-between gap-4 border-b border-border/50 p-6">
          <div>
            <h2 id="group-rate-connect-title" class="text-xl font-semibold text-foreground">
              {{ t('admin.groupRates.connect.titleWithGroup', { site: connectingRate.siteName, group: connectingRate.groupName }) }}
            </h2>
            <p class="mt-2 text-sm text-muted-foreground">
              {{ connectMode === 'bind' ? t('admin.groupRates.connect.bindDescription') : t('admin.groupRates.connect.realDescription') }}
            </p>
          </div>
          <button class="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-surface-line hover:text-foreground" :disabled="isActionLoading" @click="closeConnector">
            <X class="h-5 w-5" />
            <span class="sr-only">{{ t('admin.groupRates.actions.closeConnect') }}</span>
          </button>
        </div>

        <form class="space-y-5 p-6" @submit.prevent="submitConnector">
          <div class="grid gap-3 sm:grid-cols-2">
            <button
              type="button"
              :class="[
                'flex min-h-24 items-start gap-3 rounded-lg border p-4 text-left transition-colors',
                connectMode === 'real'
                  ? 'border-primary bg-primary/5 text-foreground'
                  : 'border-border/60 bg-surface text-muted-foreground hover:border-primary/40 hover:text-foreground'
              ]"
              @click="setConnectMode('real')"
            >
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                <Sparkles class="h-4 w-4" />
              </span>
              <span class="min-w-0">
                <span class="flex items-center gap-2 text-sm font-semibold">
                  {{ t('admin.groupRates.connect.modeReal') }}
                  <Check v-if="connectMode === 'real'" class="h-4 w-4 text-primary" />
                </span>
                <span class="mt-1 block text-xs leading-5 text-muted-foreground">{{ t('admin.groupRates.connect.realDescription') }}</span>
              </span>
            </button>
            <button
              type="button"
              :class="[
                'flex min-h-24 items-start gap-3 rounded-lg border p-4 text-left transition-colors',
                connectMode === 'bind'
                  ? 'border-primary bg-primary/5 text-foreground'
                  : 'border-border/60 bg-surface text-muted-foreground hover:border-primary/40 hover:text-foreground'
              ]"
              @click="setConnectMode('bind')"
            >
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-primary/10 text-primary">
                <Link2 class="h-4 w-4" />
              </span>
              <span class="min-w-0">
                <span class="flex items-center gap-2 text-sm font-semibold">
                  {{ t('admin.groupRates.connect.modeBind') }}
                  <Check v-if="connectMode === 'bind'" class="h-4 w-4 text-primary" />
                </span>
                <span class="mt-1 block text-xs leading-5 text-muted-foreground">{{ t('admin.groupRates.connect.bindDescription') }}</span>
              </span>
            </button>
          </div>

          <div class="rounded-xl border border-border/50 bg-surface/50 p-4 space-y-3">
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-muted-foreground">{{ t('admin.groupRates.connect.upstreamSiteLabel') }}</span>
              <span class="text-sm font-medium text-foreground">{{ connectingRate?.siteName }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-muted-foreground">{{ t('admin.groupRates.connect.upstreamGroupNameLabel') }}</span>
              <span class="text-sm font-medium text-foreground">{{ connectingRate?.groupName }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-muted-foreground">{{ t('admin.groupRates.connect.upstreamMultiplierLabel') }}</span>
              <span class="text-sm font-semibold text-primary">{{ formatMultiplier(connectingRate?.currentMultiplier ?? null) }}</span>
            </div>
            <div class="flex items-center justify-between">
              <span class="text-xs font-medium text-muted-foreground">{{ t('admin.groupRates.connect.upstreamPlatformLabel') }}</span>
              <span :class="['inline-flex rounded-md border px-2 py-0.5 text-xs font-semibold uppercase tracking-wider', platformClasses(connectingRate?.platform ?? null)]">
                {{ platformLabel(connectingRate?.platform ?? null) }}
              </span>
            </div>
          </div>

          <!-- sub2api admin：分组类型选择（仅在无法自动检测时显示） -->
          <div v-if="needsGroupTypeSelection" class="space-y-2">
            <span class="text-sm font-medium text-foreground">{{ t('admin.groupRates.connect.groupTypeLabel') }}</span>
            <div class="relative">
              <select
                v-model="selectedGroupType"
                class="h-10 w-full rounded-xl border border-border/50 bg-surface px-3 pr-8 text-sm text-foreground outline-none appearance-none transition-all focus:border-primary focus:ring-1 focus:ring-primary"
                :disabled="isActionLoading"
              >
                <option value="">{{ t('admin.groupRates.connect.groupTypePlaceholder') }}</option>
                <option value="openai">{{ t('admin.groupRates.connect.groupTypeOpenai') }}</option>
                <option value="anthropic">{{ t('admin.groupRates.connect.groupTypeAnthropic') }}</option>
                <option value="gemini">{{ t('admin.groupRates.connect.groupTypeGemini') }}</option>
                <option value="antigravity">{{ t('admin.groupRates.connect.groupTypeAntigravity') }}</option>
              </select>
              <div class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">
                <ChevronDown class="h-3.5 w-3.5" />
              </div>
            </div>
          </div>

          <!-- new-api admin：渠道类型选择 -->
          <div v-if="needsChannelTypeSelection" class="space-y-2">
            <span class="text-sm font-medium text-foreground">{{ t('admin.groupRates.connect.channelTypeLabel') }}</span>
            <div class="relative">
              <select
                v-model.number="selectedChannelType"
                class="h-10 w-full rounded-xl border border-border/50 bg-surface px-3 pr-8 text-sm text-foreground outline-none appearance-none transition-all focus:border-primary focus:ring-1 focus:ring-primary"
                :disabled="isActionLoading"
              >
                <option :value="0">{{ t('admin.groupRates.connect.channelTypePlaceholder') }}</option>
                <option v-for="ct in filteredChannelTypes" :key="ct.id" :value="ct.id">{{ ct.name }}</option>
              </select>
              <div class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground">
                <ChevronDown class="h-3.5 w-3.5" />
              </div>
            </div>
          </div>

          <div v-if="connectMode === 'bind'" class="space-y-2">
            <span class="text-sm font-medium text-foreground">{{ t('admin.groupRates.connect.bindSelectKey') }}</span>
            <div v-if="isLoadingKeys" class="flex items-center justify-center py-6 text-muted-foreground">
              <Loader2 class="mr-2 h-4 w-4 animate-spin" />
              {{ t('admin.groupRates.connect.bindKeysLoading') }}
            </div>
            <div v-else-if="upstreamKeys.length === 0" class="px-4 py-6 text-center text-sm text-muted-foreground">
              {{ t('admin.groupRates.connect.bindKeysEmpty') }}
            </div>
            <div v-else class="max-h-48 overflow-auto rounded-xl border border-border/50 bg-surface divide-y divide-border/30">
              <label
                v-for="keyItem in upstreamKeys"
                :key="keyItem.id"
                :class="[
                  'flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors',
                  selectedKeyId === keyItem.id ? 'bg-primary/5' : 'hover:bg-surface-elevated'
                ]"
              >
                <input
                  type="radio"
                  :value="keyItem.id"
                  :checked="selectedKeyId === keyItem.id"
                  class="h-4 w-4 border-border text-primary focus:ring-primary"
                  :disabled="isActionLoading"
                  @change="selectedKeyId = keyItem.id"
                />
                <div class="flex-1 min-w-0">
                  <div class="text-sm font-medium text-foreground truncate">{{ keyItem.name }}</div>
                  <div v-if="keyItem.keyPreview" class="text-xs text-muted-foreground font-mono truncate">{{ keyItem.keyPreview }}</div>
                </div>
                <span v-if="keyItem.groupName" class="inline-flex rounded-md border border-border/50 bg-surface-elevated px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground shrink-0">
                  {{ keyItem.groupName }}
                </span>
                <span
                  :class="[
                    'inline-flex rounded-md border px-1.5 py-0.5 text-[10px] font-semibold shrink-0',
                    isActiveResourceStatus(keyItem.status)
                      ? 'border-emerald-400/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'
                      : 'border-border/60 bg-surface-elevated text-muted-foreground'
                  ]"
                >
                  {{ resourceStatusLabel(keyItem.status) }}
                </span>
              </label>
            </div>
          </div>

          <div v-if="connectMode === 'bind'" class="space-y-2">
            <label for="existing-admin-group" class="flex items-center gap-2 text-sm font-medium text-foreground">
              <ServerCog class="h-4 w-4 text-primary" />
              {{ t('admin.groupRates.connect.bindSelectAdminGroup') }}
            </label>
            <div class="relative">
              <select
                id="existing-admin-group"
                :value="selectedAdminGroupId"
                class="h-11 w-full appearance-none rounded-lg border border-border/60 bg-surface px-3 pr-9 text-sm text-foreground outline-none transition focus:border-primary focus:ring-1 focus:ring-primary"
                :disabled="isActionLoading || isLoadingAdminResources"
                @change="handleAdminGroupChange"
              >
                <option value="">{{ t('admin.groupRates.connect.bindAdminGroupPlaceholder') }}</option>
                <option v-for="group in ownGroups" :key="group.id" :value="group.id">
                  {{ group.groupName }} · {{ formatMultiplier(group.multiplier) }}
                </option>
              </select>
              <ChevronDown class="pointer-events-none absolute right-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            </div>
          </div>

          <div v-if="connectMode === 'bind' && selectedAdminGroupId" class="space-y-2">
            <span class="flex items-center gap-2 text-sm font-medium text-foreground">
              <ServerCog class="h-4 w-4 text-primary" />
              {{ t('admin.groupRates.connect.bindSelectAdminResource') }}
            </span>
            <div v-if="isLoadingAdminResources" class="flex items-center justify-center py-6 text-muted-foreground">
              <Loader2 class="mr-2 h-4 w-4 animate-spin" />
              {{ t('admin.groupRates.connect.adminResourcesLoading') }}
            </div>
            <div v-else-if="adminResources.length === 0" class="rounded-lg border border-dashed border-border/70 px-4 py-6 text-center text-sm text-muted-foreground">
              {{ t('admin.groupRates.connect.adminResourcesEmpty') }}
            </div>
            <div v-else class="max-h-48 divide-y divide-border/30 overflow-auto rounded-lg border border-border/60 bg-surface">
              <label
                v-for="resource in adminResources"
                :key="resource.id"
                class="flex cursor-pointer items-center gap-3 px-4 py-3 transition-colors hover:bg-surface-elevated"
                :class="selectedAdminResourceId === resource.id ? 'bg-primary/5' : ''"
              >
                <input
                  v-model="selectedAdminResourceId"
                  type="radio"
                  :value="resource.id"
                  class="h-4 w-4 border-border text-primary focus:ring-primary"
                  :disabled="isActionLoading"
                />
                <div class="min-w-0 flex-1">
                  <div class="truncate text-sm font-medium text-foreground">{{ resource.name }}</div>
                  <div class="mt-0.5 truncate text-xs text-muted-foreground">{{ adminResourceTypeLabel(resource) }}</div>
                </div>
                <span :class="['rounded-md border px-2 py-0.5 text-[10px] font-semibold', isActiveResourceStatus(resource.status) ? 'border-emerald-500/20 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300' : 'border-border/60 bg-surface-elevated text-muted-foreground']">
                  {{ resourceStatusLabel(resource.status) }}
                </span>
              </label>
            </div>
          </div>

          <div v-if="connectMode === 'real'" class="space-y-2">
            <span class="text-sm font-medium text-foreground">{{ t('admin.groupRates.connect.ownGroupLabel') }}</span>
            <div class="max-h-48 overflow-auto rounded-xl border border-border/50 bg-surface divide-y divide-border/30">
              <label
                v-for="group in filteredOwnGroups"
                :key="group.id"
                class="flex items-center gap-3 px-4 py-2.5 cursor-pointer transition-colors hover:bg-surface-elevated"
              >
                <input
                  type="checkbox"
                  :checked="connectOwnGroups.includes(group.id)"
                  class="h-4 w-4 rounded border-border text-primary focus:ring-primary"
                  :disabled="isActionLoading"
                  @change="toggleOwnGroup(group.id)"
                />
                <span class="text-sm text-foreground">{{ group.groupName }}</span>
                <span v-if="group.platform" class="inline-flex rounded-md border border-border/50 bg-surface-elevated px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-muted-foreground">
                  {{ group.platform }}
                </span>
                <span class="ml-auto text-xs text-muted-foreground">{{ formatMultiplier(group.multiplier) }}</span>
              </label>
              <div v-if="filteredOwnGroups.length === 0" class="px-4 py-3 text-sm text-muted-foreground">
                {{ t('admin.groupRates.connect.ownGroupPlaceholder') }}
              </div>
            </div>
          </div>

          <label class="flex cursor-pointer items-start gap-3 rounded-lg border border-border/60 bg-surface p-4 transition-colors hover:border-primary/40">
            <input
              v-model="addToPricingMapping"
              type="checkbox"
              class="mt-0.5 h-4 w-4 rounded border-border text-primary focus:ring-primary"
              :disabled="isActionLoading"
            />
            <span>
              <span class="flex items-center gap-2 text-sm font-medium text-foreground">
                <KeyRound class="h-4 w-4 text-primary" />
                {{ t('admin.groupRates.connect.addToPricingMapping') }}
              </span>
              <span class="mt-1 block text-xs leading-5 text-muted-foreground">{{ t('admin.groupRates.connect.addToPricingMappingHint') }}</span>
            </span>
          </label>

          <div v-if="realConnectError" class="flex items-start gap-3 rounded-xl border border-warning/20 bg-warning/10 p-3 text-sm text-warning">
            <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
            <span>{{ realConnectError }}</span>
          </div>

          <div class="flex justify-end gap-2">
            <Button type="button" variant="secondary" :disabled="isActionLoading" @click="closeConnector">
              {{ t('admin.groupRates.actions.cancel') }}
            </Button>
            <Button type="submit" class="gap-2" :disabled="isActionLoading || !canSubmitConnect">
              <Loader2 v-if="isActionLoading" class="h-4 w-4 animate-spin" />
              {{ t(connectMode === 'real' ? 'admin.groupRates.connect.submitManaged' : 'admin.groupRates.connect.submitExisting') }}
            </Button>
          </div>
        </form>
      </div>
    </div>

    <div v-if="disconnectingRate" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-4 backdrop-blur-sm">
      <div data-group-rates-dialog role="dialog" aria-modal="true" aria-labelledby="group-rate-disconnect-title" tabindex="-1" class="max-h-[calc(100dvh-2rem)] w-full max-w-md overflow-y-auto overscroll-contain rounded-xl border border-border/50 bg-card shadow-xl">
        <div class="flex items-start justify-between gap-4 border-b border-border/50 p-6">
          <div>
            <h2 id="group-rate-disconnect-title" class="text-xl font-semibold text-foreground">{{ t('admin.groupRates.disconnect.title') }}</h2>
            <p class="mt-2 text-sm text-muted-foreground">
              {{ t('admin.groupRates.disconnect.description', { site: disconnectingRate.siteName, group: disconnectingRate.groupName }) }}
            </p>
          </div>
          <button class="rounded-lg p-2 text-muted-foreground transition-colors hover:bg-surface-line hover:text-foreground" :disabled="isDisconnecting" @click="closeDisconnect">
            <X class="h-5 w-5" />
          </button>
        </div>

        <div v-if="disconnectError" class="mx-6 mt-6 flex items-start gap-3 rounded-xl border border-warning/20 bg-warning/10 p-3 text-sm text-warning">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ disconnectError }}</span>
        </div>

        <div class="space-y-4 p-6">
          <div class="space-y-3">
            <label
              v-if="disconnectConnection?.canDeleteRemote !== false"
              class="flex cursor-pointer items-start gap-3 rounded-xl border p-4 transition-colors"
              :class="disconnectMode === 'unlink'
                ? 'border-primary bg-primary/5'
                : 'border-border/50 bg-surface hover:bg-surface-elevated'"
            >
              <input
                v-model="disconnectMode"
                type="radio"
                value="unlink"
                class="mt-0.5 h-4 w-4 border-border text-primary focus:ring-primary"
                :disabled="isDisconnecting"
              />
              <div>
                <span class="text-sm font-medium text-foreground">{{ t('admin.groupRates.disconnect.unlinkOnly') }}</span>
                <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.groupRates.disconnect.unlinkOnlyHint') }}</p>
              </div>
            </label>

            <label
              class="flex cursor-pointer items-start gap-3 rounded-xl border p-4 transition-colors"
              :class="disconnectMode === 'full'
                ? 'border-red-500/50 bg-red-500/5'
                : 'border-border/50 bg-surface hover:bg-surface-elevated'"
            >
              <input
                v-model="disconnectMode"
                type="radio"
                value="full"
                class="mt-0.5 h-4 w-4 border-border text-red-500 focus:ring-red-500"
                :disabled="isDisconnecting"
              />
              <div>
                <span class="text-sm font-medium text-red-600 dark:text-red-400">{{ t('admin.groupRates.disconnect.deleteAll') }}</span>
                <p class="mt-1 text-xs text-red-500/70">{{ t('admin.groupRates.disconnect.deleteAllHint') }}</p>
              </div>
            </label>
          </div>

          <label
            v-if="disconnectConnection?.pricingMappingEnabled || disconnectingRate?.pricingMapped"
            class="flex cursor-pointer items-start gap-3 rounded-lg border border-border/60 bg-surface p-4"
          >
            <input
              v-model="disconnectRemovePricing"
              type="checkbox"
              class="mt-0.5 h-4 w-4 rounded border-border text-primary focus:ring-primary"
              :disabled="isDisconnecting"
            />
            <span>
              <span class="text-sm font-medium text-foreground">{{ t('admin.groupRates.disconnect.removePricingMapping') }}</span>
              <span class="mt-1 block text-xs text-muted-foreground">{{ t('admin.groupRates.disconnect.removePricingMappingHint') }}</span>
            </span>
          </label>

          <div class="flex justify-end gap-2">
            <Button variant="secondary" :disabled="isDisconnecting" @click="closeDisconnect">
              {{ t('admin.groupRates.actions.cancel') }}
            </Button>
            <Button
              :variant="disconnectMode === 'full' ? 'destructive' : 'default'"
              class="gap-2"
              :disabled="isDisconnecting"
              @click="submitDisconnect"
            >
              <Loader2 v-if="isDisconnecting" class="h-4 w-4 animate-spin" />
              {{ t('admin.groupRates.disconnect.confirm') }}
            </Button>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
