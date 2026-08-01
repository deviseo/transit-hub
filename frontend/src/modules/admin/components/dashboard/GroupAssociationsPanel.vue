<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertCircle,
  ArrowDownUp,
  Calculator,
  Check,
  ChevronDown,
  ChevronUp,
  CircleOff,
  Layers,
  Link2,
  Loader2,
  Play,
  RefreshCw,
  Search,
  Settings2,
  Trash2,
  TriangleAlert,
  X,
  Zap,
} from 'lucide-vue-next'
import { getConnectionHealthAdminGroups } from '../../api/connectionHealth'
import { listAllGroupRates } from '../../api/groupRates'
import { getMySiteMappingOptions, listRealConnections, removeMySiteMapping, runAutoPricing, saveMySiteMapping } from '../../api/mySites'
import { listUpstreamSites } from '../../api/upstream'
import { getNotificationChannelSettings } from '../../api/settings'
import { useAdminAccounts } from '../../composables/useAdminAccounts'
import {
  createDefaultGroupAssociationsPreferences,
  mergeGroupAssociationsGroupOrder,
  readGroupAssociationsPreferences,
  type GroupAssociationsPreferences,
  writeGroupAssociationsPreferences,
} from '../../utils/groupAssociationsPreferences'
import type { AdminGroupHealth } from '../../types/connectionHealth'
import type { GroupRate } from '../../types/groupRates'
import type {
  AutoPricingRunResult,
  MySiteGroupRef,
  MySiteMapping,
  MySiteMappingOwnGroupOption,
  MySiteUpstreamTargetOption,
  RealConnection,
} from '../../types/mySites'
import type { UpstreamGroupInfo, UpstreamSiteResponse } from '../../types/upstream'
import AutoPricingConfigDrawer, { type BotOption } from './AutoPricingConfigDrawer.vue'
import GroupAssociationTargetsDrawer from './GroupAssociationTargetsDrawer.vue'

type AssociationFilter = 'all' | 'associated' | 'unassociated' | 'stale'
type ProfitCalculatorMode = 'group' | 'custom'
type AssociationSortKey = 'name' | 'sourceStatus' | 'healthStatus' | 'upstreamMultiplier' | 'delta' | 'effectiveCostMultiplier' | 'profitMargin'
type AssociationSortDirection = 'asc' | 'desc'
type SourceStatus = 'available' | 'notFound' | 'syncError' | 'unknown'
type HealthStatus = 'healthy' | 'partial' | 'autoStopped' | 'unmonitored' | 'unknown'

interface AssociationRow {
  id: string
  ownGroup: string
  ownGroupInfo: MySiteMappingOwnGroupOption | null
  mapping: MySiteMapping
  stale: boolean
  staleTargetCount: number
}

interface AssociationTargetRow {
  key: string
  siteId: string
  groupId: string
  groupName: string
  siteName: string
  platform: string
  included: boolean
  stale: boolean
  connections: RealConnection[]
  rate: GroupRate | null
  upstreamMultiplier: number | null
  delta: number | null
  effectiveCostMultiplier: number | null
  profitMargin: number | null
  sourceStatus: SourceStatus
  healthStatus: HealthStatus
}

const { t, locale } = useI18n()
const { currentAccount } = useAdminAccounts()
const loading = ref(true)
const error = ref<string | null>(null)
const ownGroups = ref<MySiteMappingOwnGroupOption[]>([])
const mappings = ref<MySiteMapping[]>([])
const upstreamSites = ref<UpstreamSiteResponse[]>([])
const realConnections = ref<RealConnection[]>([])
const groupRates = ref<GroupRate[]>([])
const healthGroups = ref<AdminGroupHealth[]>([])
const realConnectionsAvailable = ref(false)
const groupRatesAvailable = ref(false)
const healthDataAvailable = ref(false)
const upstreamSitesAvailable = ref(false)
const staleOwnGroupNames = ref<string[]>([])
const staleTargetRefs = ref<MySiteGroupRef[]>([])
const botOptions = ref<BotOption[]>([])
const search = ref('')
const filter = ref<AssociationFilter>('all')
const selectedOwnGroup = ref('')
const savingOwnGroup = ref<string | null>(null)
const savedOwnGroup = ref<string | null>(null)
const runningOwnGroup = ref<string | null>(null)
const targetsDrawerOpen = ref(false)
const pricingDrawerOpen = ref(false)
const cleanupDialogOpen = ref(false)
const profitCalculatorOpen = ref(false)
const profitCalculatorMode = ref<ProfitCalculatorMode>('group')
const simulatedRevenueInput = ref<string | number>('100')
const customUpstreamMultiplierInput = ref<string | number>('')
const customSaleMultiplierInput = ref<string | number>('')
const showExcluded = ref(false)
const groupManagerOpen = ref(false)
const associationSortKey = ref<AssociationSortKey>('name')
const associationSortDirection = ref<AssociationSortDirection>('asc')
const preferences = ref<GroupAssociationsPreferences>(createDefaultGroupAssociationsPreferences())
let loadedPreferenceScope = ''
let savedTimer: ReturnType<typeof setTimeout> | null = null

const preferenceScope = computed(() => currentAccount.value?.id ?? 'anonymous')

const updatePreferences = (updater: (current: GroupAssociationsPreferences) => GroupAssociationsPreferences) => {
  const next = updater(preferences.value)
  preferences.value = next
  writeGroupAssociationsPreferences(preferenceScope.value, next)
}

const loadPreferences = (scope: string) => {
  if (loadedPreferenceScope === scope) return
  preferences.value = readGroupAssociationsPreferences(scope)
  loadedPreferenceScope = scope
}

watch(preferenceScope, loadPreferences, { immediate: true })

const targetKey = (siteId: string, groupName: string): string => `${siteId}\u0000${groupName}`
// Older deployments may have persisted a missing/null upstreamTargets field.
// Keep rendering resilient even before the upgraded backend normalizes it.
const mappingTargets = (mapping: MySiteMapping): MySiteGroupRef[] => (
  Array.isArray(mapping.upstreamTargets) ? mapping.upstreamTargets : []
)
const staleOwnGroupSet = computed(() => new Set(staleOwnGroupNames.value))
const staleTargetSet = computed(() => new Set(staleTargetRefs.value.map(target => targetKey(target.siteId, target.groupName))))
const siteById = computed(() => new Map(upstreamSites.value.map(site => [site.id, site])))
const upstreamLabels = computed(() => new Map(upstreamSites.value.map(site => [site.id, site.name])))

const upstreamMultiplierMap = computed(() => {
  const values = new Map<string, number>()
  for (const site of upstreamSites.value) {
    for (const group of site.metrics?.groups ?? []) {
      if (group.multiplier != null) values.set(targetKey(site.id, group.name), group.multiplier)
    }
  }
  return values
})

const mappingByOwnGroup = computed(() => new Map(mappings.value.map(mapping => [mapping.ownGroup, mapping])))

const groupIdForRow = (row: AssociationRow): string => row.ownGroupInfo?.id ?? `stale:${row.ownGroup}`

const connectionBelongsToRow = (connection: RealConnection, row: AssociationRow): boolean => {
  if (row.ownGroupInfo && connection.ownGroupIds.includes(row.ownGroupInfo.id)) return true
  return connection.ownGroupNames?.includes(row.ownGroup) ?? false
}

const rows = computed<AssociationRow[]>(() => {
  const result: AssociationRow[] = []
  const seen = new Set<string>()
  for (const group of ownGroups.value) {
    seen.add(group.groupName)
    const mapping = mappingByOwnGroup.value.get(group.groupName) ?? { ownGroup: group.groupName, upstreamTargets: [] }
    result.push({
      id: group.id,
      ownGroup: group.groupName,
      ownGroupInfo: group,
      mapping,
      stale: staleOwnGroupSet.value.has(group.groupName),
      staleTargetCount: mappingTargets(mapping).filter(target => staleTargetSet.value.has(targetKey(target.siteId, target.groupName))).length,
    })
  }
  for (const mapping of mappings.value) {
    if (seen.has(mapping.ownGroup)) continue
    result.push({
      id: `stale:${mapping.ownGroup}`,
      ownGroup: mapping.ownGroup,
      ownGroupInfo: null,
      mapping,
      stale: true,
      staleTargetCount: mappingTargets(mapping).filter(target => staleTargetSet.value.has(targetKey(target.siteId, target.groupName))).length,
    })
  }
  return result.sort((first, second) => first.ownGroup.localeCompare(second.ownGroup))
})

const counts = computed(() => ({
  all: rows.value.length,
  associated: rows.value.filter(row => mappingTargets(row.mapping).length > 0).length,
  unassociated: rows.value.filter(row => mappingTargets(row.mapping).length === 0).length,
  stale: rows.value.filter(row => row.stale || row.staleTargetCount > 0).length,
}))

const orderedRows = computed(() => {
  const currentIds = rows.value.map(row => row.id)
  const hasStoredOrder = preferences.value.groupOrder.some(id => currentIds.includes(id))
  if (!hasStoredOrder) return rows.value
  const order = mergeGroupAssociationsGroupOrder(preferences.value.groupOrder, currentIds)
  const rowMap = new Map(rows.value.map(row => [row.id, row]))
  return order.flatMap(id => {
    const row = rowMap.get(id)
    return row ? [row] : []
  })
})

const filteredRows = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return orderedRows.value
    .filter(row => !preferences.value.hiddenGroupIds.includes(row.id))
    .filter(row => {
      const matchesMappingSearch = mappingTargets(row.mapping).some(target => {
        const siteName = siteById.value.get(target.siteId)?.name ?? target.siteId
        return target.groupName.toLocaleLowerCase().includes(query) || siteName.toLocaleLowerCase().includes(query)
      })
      const matchesConnectionSearch = realConnections.value.some(connection => (
        connectionBelongsToRow(connection, row)
        && (connection.upstreamGroupName.toLocaleLowerCase().includes(query)
          || (siteById.value.get(connection.upstreamSiteId)?.name ?? connection.upstreamSiteId).toLocaleLowerCase().includes(query))
      ))
      const matchesSearch = !query || row.ownGroup.toLocaleLowerCase().includes(query) || matchesMappingSearch || matchesConnectionSearch
      if (!matchesSearch) return false
      if (filter.value === 'associated') return mappingTargets(row.mapping).length > 0
      if (filter.value === 'unassociated') return mappingTargets(row.mapping).length === 0
      if (filter.value === 'stale') return row.stale || row.staleTargetCount > 0
      return true
    })
})

const emptyGroupMessage = computed(() => {
  if (rows.value.length === 0 || search.value.trim() || filter.value !== 'all') {
    return t('admin.groupAssociations.empty')
  }
  return t('admin.groupAssociations.groupDisplay.noVisibleGroups')
})

const selectedRow = computed(() => rows.value.find(row => row.ownGroup === selectedOwnGroup.value) ?? null)
const selectedMapping = computed<MySiteMapping | null>(() => selectedRow.value?.mapping ?? null)

watch(filteredRows, (nextRows) => {
  if (nextRows.some(row => row.ownGroup === selectedOwnGroup.value)) return
  selectedOwnGroup.value = nextRows[0]?.ownGroup ?? ''
}, { immediate: true })


const targetOptions = computed<MySiteUpstreamTargetOption[]>(() => {
  const options = new Map<string, MySiteUpstreamTargetOption>()
  for (const site of upstreamSites.value) {
    for (const group of site.metrics?.groups ?? []) {
      const key = targetKey(site.id, group.name)
      options.set(key, {
        siteId: site.id,
        siteName: site.name,
        groupName: group.name,
        platform: site.platform,
        multiplier: group.multiplier ?? null,
        multiplierMode: group.multiplierMode,
        stale: staleTargetSet.value.has(key),
      })
    }
  }
  for (const mapping of mappings.value) {
    for (const target of mappingTargets(mapping)) {
      const key = targetKey(target.siteId, target.groupName)
      if (options.has(key)) continue
      const site = siteById.value.get(target.siteId)
      options.set(key, {
        ...target,
        siteName: site?.name ?? target.siteId,
        platform: site?.platform ?? '',
        multiplier: null,
        stale: true,
      })
    }
  }
  return [...options.values()].sort((first, second) => (
    first.siteName.localeCompare(second.siteName) || first.groupName.localeCompare(second.groupName)
  ))
})

const selectedTargets = computed(() => selectedMapping.value?.upstreamTargets ?? [])

const calculateEffectiveCostMultiplier = (
  multiplier: number | null | undefined,
  rechargeRate: number | null | undefined,
): number | null => {
  if (
    multiplier == null
    || rechargeRate == null
    || !Number.isFinite(multiplier)
    || !Number.isFinite(rechargeRate)
    || multiplier < 0
    || rechargeRate <= 0
  ) return null
  return multiplier * rechargeRate
}

const calculateProfitMargin = (
  saleMultiplier: number | null | undefined,
  costMultiplier: number | null,
): number | null => {
  if (
    saleMultiplier == null
    || costMultiplier == null
    || !Number.isFinite(saleMultiplier)
    || saleMultiplier <= 0
  ) return null
  return (saleMultiplier - costMultiplier) / saleMultiplier
}

const selectedTargetDetails = computed(() => selectedTargets.value.map(target => {
  const option = targetOptions.value.find(item => item.siteId === target.siteId && item.groupName === target.groupName)
  const detail = option ?? {
    ...target,
    siteName: target.siteId,
    platform: '',
    multiplier: null,
    multiplierMode: undefined,
    stale: true,
  }
  const rechargeRate = siteById.value.get(target.siteId)?.rechargeRate
  const effectiveCostMultiplier = detail.stale
    ? null
    : calculateEffectiveCostMultiplier(detail.multiplier, rechargeRate)
  return {
    ...detail,
    effectiveCostMultiplier,
    profitMargin: calculateProfitMargin(selectedRow.value?.ownGroupInfo?.multiplier, effectiveCostMultiplier),
  }
}))

const upstreamGroupInfoFor = (siteId: string, groupId: string, groupName: string): UpstreamGroupInfo | null => {
  const site = siteById.value.get(siteId)
  if (!site) return null
  return site.metrics?.groups.find(group => (
    (groupId && group.id === groupId) || group.name === groupName
  )) ?? null
}

const groupRateKey = (siteId: string, groupName: string, platform: string | null | undefined): string => (
  `${targetKey(siteId, groupName)}\u0000${platform ?? ''}`
)

const groupRateMap = computed(() => {
  const result = new Map<string, GroupRate>()
  for (const rate of groupRates.value) {
    result.set(groupRateKey(rate.siteId, rate.groupName, rate.platform), rate)
  }
  return result
})

const sourceStatusFor = (siteId: string, groupId: string, groupName: string): SourceStatus => {
  const site = siteById.value.get(siteId)
  if (!site) return 'unknown'
  if (site.status === 'error') return 'syncError'
  if (site.status !== 'connected') return 'unknown'
  return upstreamGroupInfoFor(siteId, groupId, groupName) ? 'available' : 'notFound'
}

const healthGroupFor = (row: AssociationRow): AdminGroupHealth | null => {
  if (!healthDataAvailable.value) return null
  return healthGroups.value.find(group => (
    (row.ownGroupInfo != null && group.id === row.ownGroupInfo.id) || group.name === row.ownGroup
  )) ?? null
}

const healthAccountManaged = (account: AdminGroupHealth['accounts'][number], group: AdminGroupHealth): boolean => {
  const accountEnabledPolicy = account.hasEnabledProbePolicy ?? account.hasEnabledPolicy
  if (accountEnabledPolicy != null) return accountEnabledPolicy
  if (account.assignedPolicies?.some(policy => policy.enabled)) return true
  if (account.assignedPolicies?.length) return false

  const groupEnabledPolicy = group.hasEnabledProbePolicy ?? group.hasEnabledPolicy
  if (groupEnabledPolicy != null) return groupEnabledPolicy
  if (group.assignedPolicies?.some(policy => policy.enabled)) return true
  if (group.assignedPolicies?.length) return false

  return Boolean(account.hasAssignedPolicy || group.hasAssignedPolicy)
}

const healthAccountHasAutoStopEvidence = (
  account: AdminGroupHealth['accounts'][number],
  group: AdminGroupHealth,
): boolean => {
  const hasAutoRemoteAction = [
    ...(account.assignedPolicies ?? []),
    ...(group.assignedPolicies ?? []),
  ].some(policy => policy.enabled && policy.autoRemoteActionEnabled === true)
  if (!hasAutoRemoteAction) return false

  const currentStatus = account.status.toLowerCase()
  if (!['inactive', 'disabled', 'suspended'].includes(currentStatus)) return false

  const autoDisableActions = new Set([
    'newapi_channel_disabled',
    'sub2api_account_status_inactive',
  ])
  return account.modelHealth.some(model => (
    autoDisableActions.has(model.lastRemoteAction)
    && (model.state === 'disabled' || model.state === 'suspended')
  ))
}

const healthAccountAvailable = (account: AdminGroupHealth['accounts'][number]): boolean => {
  const status = account.status.toLowerCase()
  if (['disabled', 'inactive', 'suspended'].includes(status)) return false
  if (!account.probeAvailable) return false
  return !account.modelHealth.some(model => model.state === 'disabled' || model.state === 'suspended')
}

const healthStatusFor = (connections: RealConnection[], row: AssociationRow): HealthStatus => {
  const group = healthGroupFor(row)
  if (!group || group.accountsError) return 'unknown'
  const connectionAccountIds = new Set(connections.map(connection => connection.adminAccountId).filter(Boolean))
  const accounts = group.accounts.filter(account => connectionAccountIds.has(account.id))
  if (accounts.length === 0) return 'unknown'

  const statuses = accounts.map(account => {
    if (!healthAccountManaged(account, group)) return 'unmonitored'
    if (healthAccountHasAutoStopEvidence(account, group)) return 'autoStopped'
    return healthAccountAvailable(account) ? 'healthy' : 'partial'
  })
  if (statuses.every(status => status === 'unmonitored')) return 'unmonitored'
  if (statuses.every(status => status === 'autoStopped')) return 'autoStopped'
  if (statuses.some(status => status !== 'healthy')) return 'partial'
  return 'healthy'
}

const associationRows = computed<AssociationTargetRow[]>(() => {
  const row = selectedRow.value
  if (!row) return []

  const mappingTargetSet = new Set(mappingTargets(row.mapping).map(target => targetKey(target.siteId, target.groupName)))
  const entries = new Map<string, {
    siteId: string
    groupId: string
    groupName: string
    included: boolean
    connections: RealConnection[]
  }>()

  for (const target of mappingTargets(row.mapping)) {
    const key = targetKey(target.siteId, target.groupName)
    entries.set(key, {
      siteId: target.siteId,
      groupId: '',
      groupName: target.groupName,
      included: true,
      connections: [],
    })
  }

  for (const connection of realConnections.value) {
    if (!connectionBelongsToRow(connection, row)) continue
    const key = targetKey(connection.upstreamSiteId, connection.upstreamGroupName)
    const current = entries.get(key) ?? {
      siteId: connection.upstreamSiteId,
      groupId: connection.upstreamGroupId,
      groupName: connection.upstreamGroupName,
      included: mappingTargetSet.has(key),
      connections: [],
    }
    if (!current.groupId) current.groupId = connection.upstreamGroupId
    if (!current.connections.some(item => item.id === connection.id)) current.connections.push(connection)
    entries.set(key, current)
  }

  return Array.from(entries.entries()).map(([key, entry]) => {
    const site = siteById.value.get(entry.siteId)
    const platform = site?.platform ?? entry.connections[0]?.upstreamPlatform ?? ''
    const groupInfo = upstreamGroupInfoFor(entry.siteId, entry.groupId, entry.groupName)
    const rate = groupRateMap.value.get(groupRateKey(entry.siteId, entry.groupName, platform))
      ?? groupRateMap.value.get(groupRateKey(entry.siteId, entry.groupName, ''))
      ?? null
    const upstreamMultiplier = rate?.upstreamMultiplier ?? groupInfo?.multiplier ?? null
    const effectiveCostMultiplier = rate?.currentMultiplier
      ?? calculateEffectiveCostMultiplier(upstreamMultiplier, site?.rechargeRate)
    return {
      key,
      siteId: entry.siteId,
      groupId: entry.groupId,
      groupName: entry.groupName,
      siteName: rate?.siteName ?? site?.name ?? entry.siteId,
      platform: rate?.platform ?? platform,
      included: entry.included,
      stale: staleTargetSet.value.has(key) || (entry.included && entry.connections.length === 0) || Boolean(rate?.deleted),
      connections: entry.connections,
      rate,
      upstreamMultiplier,
      delta: rate?.delta ?? null,
      effectiveCostMultiplier,
      profitMargin: calculateProfitMargin(row.ownGroupInfo?.multiplier, effectiveCostMultiplier),
      sourceStatus: sourceStatusFor(entry.siteId, entry.groupId, entry.groupName),
      healthStatus: healthStatusFor(entry.connections, row),
    }
  })
})

const compareNullableNumbers = (first: number | null, second: number | null): number => {
  const firstMissing = first == null || !Number.isFinite(first)
  const secondMissing = second == null || !Number.isFinite(second)
  if (firstMissing && secondMissing) return 0
  if (firstMissing) return 1
  if (secondMissing) return -1
  return first - second
}

const sourceStatusOrder: Record<SourceStatus, number> = {
  available: 0,
  syncError: 1,
  notFound: 2,
  unknown: 3,
}

const healthStatusOrder: Record<HealthStatus, number> = {
  healthy: 0,
  partial: 1,
  autoStopped: 2,
  unmonitored: 3,
  unknown: 4,
}

const compareAssociationRows = (first: AssociationTargetRow, second: AssociationTargetRow): number => {
  let comparison = 0
  let firstMissing = false
  let secondMissing = false
  switch (associationSortKey.value) {
    case 'name':
      comparison = first.siteName.localeCompare(second.siteName) || first.groupName.localeCompare(second.groupName)
      break
    case 'sourceStatus':
      firstMissing = first.sourceStatus === 'unknown'
      secondMissing = second.sourceStatus === 'unknown'
      comparison = sourceStatusOrder[first.sourceStatus] - sourceStatusOrder[second.sourceStatus]
      break
    case 'healthStatus':
      firstMissing = first.healthStatus === 'unknown'
      secondMissing = second.healthStatus === 'unknown'
      comparison = healthStatusOrder[first.healthStatus] - healthStatusOrder[second.healthStatus]
      break
    case 'upstreamMultiplier':
      firstMissing = first.upstreamMultiplier == null || !Number.isFinite(first.upstreamMultiplier)
      secondMissing = second.upstreamMultiplier == null || !Number.isFinite(second.upstreamMultiplier)
      comparison = compareNullableNumbers(first.upstreamMultiplier, second.upstreamMultiplier)
      break
    case 'delta':
      firstMissing = first.delta == null || !Number.isFinite(first.delta)
      secondMissing = second.delta == null || !Number.isFinite(second.delta)
      comparison = compareNullableNumbers(first.delta, second.delta)
      break
    case 'effectiveCostMultiplier':
      firstMissing = first.effectiveCostMultiplier == null || !Number.isFinite(first.effectiveCostMultiplier)
      secondMissing = second.effectiveCostMultiplier == null || !Number.isFinite(second.effectiveCostMultiplier)
      comparison = compareNullableNumbers(first.effectiveCostMultiplier, second.effectiveCostMultiplier)
      break
    case 'profitMargin':
      firstMissing = first.profitMargin == null || !Number.isFinite(first.profitMargin)
      secondMissing = second.profitMargin == null || !Number.isFinite(second.profitMargin)
      comparison = compareNullableNumbers(first.profitMargin, second.profitMargin)
      break
  }
  if (firstMissing !== secondMissing) return firstMissing ? 1 : -1
  if (comparison === 0) {
    comparison = first.siteName.localeCompare(second.siteName)
      || first.groupName.localeCompare(second.groupName)
      || first.key.localeCompare(second.key)
  }
  return associationSortDirection.value === 'asc' ? comparison : -comparison
}

const sortedAssociationRows = (rowsToSort: AssociationTargetRow[]): AssociationTargetRow[] => [...rowsToSort].sort(compareAssociationRows)
const includedAssociationRows = computed(() => sortedAssociationRows(associationRows.value.filter(row => row.included)))
const excludedAssociationRows = computed(() => sortedAssociationRows(associationRows.value.filter(row => !row.included)))
const visibleAssociationRows = computed(() => (
  showExcluded.value
    ? [...includedAssociationRows.value, ...excludedAssociationRows.value]
    : includedAssociationRows.value
))

const toggleAssociationSort = (key: AssociationSortKey) => {
  if (associationSortKey.value === key) {
    associationSortDirection.value = associationSortDirection.value === 'asc' ? 'desc' : 'asc'
    return
  }
  associationSortKey.value = key
  associationSortDirection.value = 'asc'
}

const associationSortAria = (key: AssociationSortKey): 'ascending' | 'descending' | 'none' => (
  associationSortKey.value !== key ? 'none' : associationSortDirection.value === 'asc' ? 'ascending' : 'descending'
)

const formatRecentDelta = (value: number | null): string => {
  if (value == null || !Number.isFinite(value) || value === 0) return t('admin.groupAssociations.common.placeholder')
  return `${value > 0 ? '+' : ''}${value.toFixed(2)}x`
}

const sourceStatusLabel = (status: SourceStatus): string => t(`admin.groupAssociations.statuses.source.${status}`)
const healthStatusLabel = (status: HealthStatus): string => t(`admin.groupAssociations.statuses.health.${status}`)

const sourceStatusClass = (status: SourceStatus): string => {
  if (status === 'available') return 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400'
  if (status === 'syncError') return 'bg-warning/10 text-warning'
  if (status === 'notFound') return 'bg-surface-elevated text-muted-foreground'
  return 'bg-surface text-muted-foreground'
}

const healthStatusClass = (status: HealthStatus): string => {
  if (status === 'healthy') return 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-400'
  if (status === 'partial') return 'bg-warning/10 text-warning'
  if (status === 'autoStopped') return 'bg-destructive/10 text-destructive'
  if (status === 'unmonitored') return 'bg-surface-elevated text-muted-foreground'
  return 'bg-surface text-muted-foreground'
}

const profitMarginRange = computed(() => {
  const margins = selectedTargetDetails.value
    .map(target => target.profitMargin)
    .filter((margin): margin is number => margin != null && Number.isFinite(margin))
  if (margins.length === 0) return null
  return {
    minimum: Math.min(...margins),
    maximum: Math.max(...margins),
  }
})

const parseNumericInput = (input: string | number): number | null => {
  const normalized = String(input).trim()
  if (!normalized) return null
  const value = Number(normalized)
  return Number.isFinite(value) ? value : null
}

const simulatedRevenue = computed(() => {
  const value = parseNumericInput(simulatedRevenueInput.value)
  return value != null && value >= 0 ? value : null
})

const customUpstreamMultiplier = computed(() => {
  const value = parseNumericInput(customUpstreamMultiplierInput.value)
  return value != null && value >= 0 ? value : null
})

const customSaleMultiplier = computed(() => {
  const value = parseNumericInput(customSaleMultiplierInput.value)
  return value != null && value > 0 ? value : null
})

const hasCustomUpstreamMultiplierInput = computed(() => String(customUpstreamMultiplierInput.value).trim().length > 0)
const hasCustomSaleMultiplierInput = computed(() => String(customSaleMultiplierInput.value).trim().length > 0)

const customProfitSimulation = computed(() => {
  const revenue = simulatedRevenue.value
  const margin = calculateProfitMargin(customSaleMultiplier.value, customUpstreamMultiplier.value)
  if (revenue == null || margin == null) return null
  const estimatedProfit = revenue * margin
  return {
    margin,
    estimatedCost: revenue - estimatedProfit,
    estimatedProfit,
  }
})

const profitSimulationRows = computed(() => selectedTargetDetails.value.map(target => {
  const revenue = simulatedRevenue.value
  const margin = target.profitMargin
  if (revenue == null || margin == null || !Number.isFinite(margin)) {
    return { ...target, estimatedCost: null, estimatedProfit: null }
  }
  const estimatedProfit = revenue * margin
  return {
    ...target,
    estimatedCost: revenue - estimatedProfit,
    estimatedProfit,
  }
}))

const simulatedProfitRange = computed(() => {
  const profits = profitSimulationRows.value
    .map(target => target.estimatedProfit)
    .filter((profit): profit is number => profit != null && Number.isFinite(profit))
  if (profits.length === 0) return null
  return {
    minimum: Math.min(...profits),
    maximum: Math.max(...profits),
  }
})

const autoPricingStatus = computed<'notConfigured' | 'enabled' | 'savedDisabled'>(() => {
  const mapping = selectedMapping.value
  if (!mapping || (mapping.autoPricingSource == null && mapping.autoPricingStrategy == null && !mapping.enableAutoPricing)) return 'notConfigured'
  return mapping.enableAutoPricing ? 'enabled' : 'savedDisabled'
})

const formatMultiplier = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return t('admin.groupAssociations.common.placeholder')
  return t('admin.groupAssociations.common.multiplier', { value: Number(value.toFixed(4)).toString() })
}

const formatTargetMultiplier = (target: MySiteUpstreamTargetOption): string => {
  if (target.multiplierMode === 'auto') return t('admin.groupAssociations.targetsDrawer.autoMultiplier')
  return formatMultiplier(target.multiplier)
}

const profitMarginFormatter = computed(() => new Intl.NumberFormat(locale.value, {
  style: 'percent',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
}))

const currencyFormatter = computed(() => new Intl.NumberFormat(locale.value, {
  style: 'currency',
  currency: 'CNY',
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
}))

const formatProfitMargin = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return t('admin.groupAssociations.common.placeholder')
  return profitMarginFormatter.value.format(value)
}

const formatProfitMarginRange = (): string => {
  const range = profitMarginRange.value
  if (!range) return t('admin.groupAssociations.common.placeholder')
  if (Math.abs(range.maximum - range.minimum) < 0.00005) return formatProfitMargin(range.minimum)
  return t('admin.groupAssociations.metrics.marginRange', {
    minimum: formatProfitMargin(range.minimum),
    maximum: formatProfitMargin(range.maximum),
  })
}

const profitMarginClass = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return 'text-muted-foreground'
  if (value < 0) return 'text-destructive'
  if (value === 0) return 'text-warning'
  return 'text-emerald-600 dark:text-emerald-400'
}

const formatCurrency = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return t('admin.groupAssociations.common.placeholder')
  return currencyFormatter.value.format(value)
}

const formatSimulatedProfitRange = (): string => {
  const range = simulatedProfitRange.value
  if (!range) return t('admin.groupAssociations.common.placeholder')
  if (Math.abs(range.maximum - range.minimum) < 0.005) return formatCurrency(range.minimum)
  return t('admin.groupAssociations.profitCalculator.amountRange', {
    minimum: formatCurrency(range.minimum),
    maximum: formatCurrency(range.maximum),
  })
}

const closeProfitCalculator = () => {
  profitCalculatorOpen.value = false
}

const openProfitCalculator = (mode: ProfitCalculatorMode) => {
  profitCalculatorMode.value = mode
  profitCalculatorOpen.value = true
}

const handleWindowKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && profitCalculatorOpen.value) closeProfitCalculator()
}

const formatRunTime = (value: string | undefined): string => {
  if (!value) return t('admin.groupAssociations.lastRun.never')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('admin.groupAssociations.lastRun.never')
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

const moveGroup = (groupId: string, offset: -1 | 1) => {
  const ids = orderedRows.value.map(row => row.id)
  const index = ids.indexOf(groupId)
  const targetIndex = index + offset
  if (index < 0 || targetIndex < 0 || targetIndex >= ids.length) return
  const nextOrder = [...ids]
  ;[nextOrder[index], nextOrder[targetIndex]] = [nextOrder[targetIndex], nextOrder[index]]
  updatePreferences(current => ({ ...current, groupOrder: nextOrder }))
}

const toggleGroupVisibility = (groupId: string) => {
  updatePreferences((current) => {
    const hidden = new Set(current.hiddenGroupIds)
    if (hidden.has(groupId)) hidden.delete(groupId)
    else hidden.add(groupId)
    return { ...current, hiddenGroupIds: Array.from(hidden) }
  })
}

const dataWarningKeys = computed(() => [
  ...(!realConnectionsAvailable.value ? ['admin.groupAssociations.dataWarnings.realConnections'] : []),
  ...(!groupRatesAvailable.value ? ['admin.groupAssociations.dataWarnings.groupRates'] : []),
  ...(!healthDataAvailable.value ? ['admin.groupAssociations.dataWarnings.health'] : []),
  ...(!upstreamSitesAvailable.value ? ['admin.groupAssociations.dataWarnings.upstreamSites'] : []),
])

const runStatusKey = (status: string | undefined): string => {
  if (status === 'applied') return 'applied'
  if (status === 'skipped') return 'skipped'
  if (status === 'threshold_exceeded') return 'thresholdExceeded'
  if (status === 'failed') return 'failed'
  return 'unknown'
}

const runTriggerLabel = (trigger: string | undefined): string => {
  if (trigger === 'manual') return t('admin.groupAssociations.lastRun.triggerManual')
  if (trigger === 'after_sync') return t('admin.groupAssociations.lastRun.triggerAfterSync')
  return t('admin.groupAssociations.lastRun.triggerUnknown')
}

const runReasonLabel = (run: AutoPricingRunResult | null | undefined): string => {
  if (!run?.reason) return t('admin.groupAssociations.lastRun.reasonUnknown')
  const key = `admin.groupAssociations.lastRun.reasons.${run.reason}`
  const translated = t(key)
  return translated === key ? t('admin.groupAssociations.lastRun.reasonUnknown') : translated
}

const setSaved = (ownGroup: string) => {
  savedOwnGroup.value = ownGroup
  if (savedTimer) clearTimeout(savedTimer)
  savedTimer = setTimeout(() => { savedOwnGroup.value = null }, 2200)
}

const replaceMappings = (nextMappings: MySiteMapping[] | undefined, fallback: MySiteMapping) => {
  if (nextMappings) {
    mappings.value = nextMappings
    return
  }
  const index = mappings.value.findIndex(mapping => mapping.ownGroup === fallback.ownGroup)
  if (index >= 0) mappings.value.splice(index, 1, fallback)
  else mappings.value.push(fallback)
}

const saveMapping = async (mapping: MySiteMapping) => {
  savingOwnGroup.value = mapping.ownGroup
  error.value = null
  try {
    const status = await saveMySiteMapping(mapping, mappings.value)
    replaceMappings(status.mappings, mapping)
    setSaved(mapping.ownGroup)
    return true
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.groupAssociations.saveError'
    return false
  } finally {
    savingOwnGroup.value = null
  }
}

const saveTargets = async (targets: MySiteGroupRef[]) => {
  const current = selectedMapping.value
  if (!current) return
  if (current.enableAutoPricing && current.autoPricingSource === 'primary_upstream') {
    const keepsPrimary = targets.some(target => (
      target.siteId === current.primaryUpstreamSiteId && target.groupName === current.primaryUpstreamGroupName
    ))
    if (!keepsPrimary) {
      error.value = 'admin.groupAssociations.errors.primaryTargetRequired'
      return
    }
  }
  if (await saveMapping({ ...current, upstreamTargets: targets })) targetsDrawerOpen.value = false
}

const savePricing = async (config: Partial<MySiteMapping>) => {
  const current = selectedMapping.value
  if (!current) return
  if (await saveMapping({ ...current, ...config, ownGroup: current.ownGroup })) pricingDrawerOpen.value = false
}

const runNow = async () => {
  const mapping = selectedMapping.value
  if (!mapping?.enableAutoPricing || runningOwnGroup.value) return
  runningOwnGroup.value = mapping.ownGroup
  error.value = null
  try {
    const response = await runAutoPricing({ ownGroup: mapping.ownGroup })
    replaceMappings(undefined, response.mapping)
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.groupAssociations.runError'
  } finally {
    runningOwnGroup.value = null
  }
}

const cleanupMapping = async () => {
  const row = selectedRow.value
  if (!row) return
  savingOwnGroup.value = row.ownGroup
  error.value = null
  try {
    const status = await removeMySiteMapping(row.ownGroup, mappings.value)
    mappings.value = status.mappings ?? mappings.value.filter(mapping => mapping.ownGroup !== row.ownGroup)
    staleOwnGroupNames.value = staleOwnGroupNames.value.filter(name => name !== row.ownGroup)
    cleanupDialogOpen.value = false
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.groupAssociations.saveError'
  } finally {
    savingOwnGroup.value = null
  }
}

const loadData = async () => {
  loading.value = true
  error.value = null
  realConnectionsAvailable.value = false
  groupRatesAvailable.value = false
  healthDataAvailable.value = false
  upstreamSitesAvailable.value = false
  try {
    const [mappingResponse, sitesResult, channelSettings, connectionsResult, ratesResult, healthResult] = await Promise.all([
      getMySiteMappingOptions(),
      listUpstreamSites()
        .then(items => ({ items, available: true }))
        .catch(() => ({ items: [] as UpstreamSiteResponse[], available: false })),
      getNotificationChannelSettings().catch(() => ({ dingtalk: [], wecom: [], qq: [], feishu: [], telegram: [] })),
      listRealConnections()
        .then(items => ({ items, available: true }))
        .catch(() => ({ items: [] as RealConnection[], available: false })),
      listAllGroupRates({ search: '', type: '', platform: '', status: 'all', sort: 'siteNameAsc' })
        .then(items => ({ items, available: true }))
        .catch(() => ({ items: [] as GroupRate[], available: false })),
      getConnectionHealthAdminGroups()
        .then(items => ({ items, available: true }))
        .catch(() => ({ items: [] as AdminGroupHealth[], available: false })),
    ])
    ownGroups.value = mappingResponse.ownGroups ?? []
    mappings.value = mappingResponse.mappings ?? []
    staleOwnGroupNames.value = mappingResponse.staleOwnGroups ?? []
    staleTargetRefs.value = mappingResponse.staleTargets ?? []
    upstreamSites.value = sitesResult.items
    realConnections.value = connectionsResult.items
    groupRates.value = ratesResult.items
    healthGroups.value = healthResult.items
    upstreamSitesAvailable.value = sitesResult.available
    realConnectionsAvailable.value = connectionsResult.available
    groupRatesAvailable.value = ratesResult.available
    healthDataAvailable.value = healthResult.available
    botOptions.value = [
      ...(channelSettings.dingtalk ?? []).filter(bot => bot.enabled).map(bot => ({ id: bot.id, name: bot.name, channel: 'DingTalk' })),
      ...(channelSettings.wecom ?? []).filter(bot => bot.enabled).map(bot => ({ id: bot.id, name: bot.name, channel: 'WeCom' })),
      ...(channelSettings.qq ?? []).filter(bot => bot.enabled).map(bot => ({ id: bot.id, name: bot.name, channel: 'QQ' })),
      ...(channelSettings.feishu ?? []).filter(bot => bot.enabled).map(bot => ({ id: bot.id, name: bot.name, channel: 'Feishu' })),
      ...(channelSettings.telegram ?? []).filter(bot => bot.enabled).map(bot => ({ id: bot.id, name: bot.name, channel: 'Telegram' })),
    ]
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.groupAssociations.loadError'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadData()
  window.addEventListener('keydown', handleWindowKeydown)
})
onBeforeUnmount(() => {
  if (savedTimer) clearTimeout(savedTimer)
  window.removeEventListener('keydown', handleWindowKeydown)
})
</script>

<template>
  <section class="overflow-hidden rounded-lg border border-border/60 bg-card text-card-foreground shadow-sm">
    <header class="flex flex-col gap-4 border-b border-border/60 px-5 py-5 sm:flex-row sm:items-center sm:justify-between">
      <div class="min-w-0">
        <div class="flex items-center gap-2.5">
          <Layers class="h-5 w-5 text-primary" />
          <h2 class="text-lg font-semibold text-foreground">{{ t('admin.groupAssociations.title') }}</h2>
        </div>
        <p class="mt-1 text-sm text-muted-foreground">
          {{ t('admin.groupAssociations.subtitle', { count: counts.all, associated: counts.associated, unassociated: counts.unassociated }) }}
        </p>
      </div>
      <div class="flex flex-col gap-2 sm:flex-row">
        <button
          type="button"
          class="inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-primary/30 bg-primary/10 px-3 text-sm font-medium text-primary transition-colors hover:bg-primary/15 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          aria-haspopup="dialog"
          @click="openProfitCalculator('custom')"
        >
          <Calculator class="h-4 w-4" />
          {{ t('admin.groupAssociations.actions.customProfitCalculator') }}
        </button>
        <button
          type="button"
          class="inline-flex h-9 items-center justify-center gap-2 rounded-lg border border-border/60 bg-background px-3 text-sm font-medium text-foreground transition-colors hover:bg-surface-elevated focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-50"
          :disabled="loading"
          @click="loadData"
        >
          <Loader2 v-if="loading" class="h-4 w-4 animate-spin" />
          <RefreshCw v-else class="h-4 w-4" />
          {{ t('admin.groupAssociations.actions.refresh') }}
        </button>
      </div>
    </header>

    <div v-if="error" class="flex items-center justify-between gap-4 border-b border-warning/25 bg-warning/10 px-5 py-3 text-sm text-warning">
      <span class="flex min-w-0 items-center gap-2">
        <AlertCircle class="h-4 w-4 shrink-0" />
        <span class="truncate">{{ t(error) }}</span>
      </span>
      <button type="button" class="shrink-0 font-medium underline underline-offset-4" @click="loadData">
        {{ t('admin.groupAssociations.actions.retry') }}
      </button>
    </div>

    <div v-if="!error && dataWarningKeys.length > 0" class="flex items-start gap-2 border-b border-warning/25 bg-warning/10 px-5 py-3 text-sm text-warning">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <div class="min-w-0 space-y-1">
        <p v-for="warningKey in dataWarningKeys" :key="warningKey">{{ t(warningKey) }}</p>
      </div>
    </div>

    <div v-if="loading" class="grid min-h-[32rem] lg:grid-cols-[19rem_minmax(0,1fr)]">
      <div class="space-y-3 border-r border-border/50 p-4">
        <div class="h-10 animate-pulse rounded-lg bg-surface-elevated" />
        <div v-for="index in 6" :key="index" class="h-16 animate-pulse rounded-lg bg-surface/70" />
      </div>
      <div class="space-y-5 p-6">
        <div class="h-8 w-48 animate-pulse rounded bg-surface-elevated" />
        <div class="h-20 animate-pulse rounded-lg bg-surface/70" />
        <div class="h-44 animate-pulse rounded-lg bg-surface/70" />
      </div>
    </div>

    <div v-else class="grid min-h-[32rem] lg:grid-cols-[19rem_minmax(0,1fr)]">
      <aside class="flex min-h-0 flex-col border-b border-border/50 lg:border-b-0 lg:border-r">
        <div class="space-y-3 border-b border-border/50 p-4">
          <div class="relative">
            <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              v-model="search"
              type="search"
              :placeholder="t('admin.groupAssociations.filters.searchPlaceholder')"
              :aria-label="t('admin.groupAssociations.filters.searchLabel')"
              class="h-10 w-full rounded-lg border border-border/60 bg-background pl-9 pr-3 text-sm text-foreground outline-none placeholder:text-muted-foreground focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/25"
            >
          </div>
          <div class="grid grid-cols-4 gap-1 rounded-lg bg-surface p-1" role="tablist">
            <button
              v-for="option in (['all', 'associated', 'unassociated', 'stale'] as const)"
              :key="option"
              type="button"
              class="min-w-0 rounded-md px-1.5 py-1.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="filter === option ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
              :title="t(`admin.groupAssociations.filters.${option}`)"
              @click="filter = option"
            >
              <span class="block truncate">{{ t(`admin.groupAssociations.filters.${option}`) }}</span>
              <span class="mt-0.5 block tabular-nums text-[10px] opacity-70">{{ counts[option] }}</span>
            </button>
          </div>
          <button
            type="button"
            class="inline-flex h-9 w-full items-center justify-center gap-2 rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground transition-colors hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :aria-expanded="groupManagerOpen"
            @click="groupManagerOpen = !groupManagerOpen"
          >
            <Settings2 class="h-4 w-4" />
            {{ t('admin.groupAssociations.groupDisplay.manage') }}
          </button>
          <div v-if="groupManagerOpen" class="space-y-2 rounded-lg border border-border/60 bg-background p-2">
            <div class="flex items-center justify-between gap-2 px-1 text-xs text-muted-foreground">
              <span>{{ t('admin.groupAssociations.groupDisplay.title') }}</span>
              <span>{{ orderedRows.length }}</span>
            </div>
            <div v-if="orderedRows.length === 0" class="px-1 py-3 text-xs text-muted-foreground">
              {{ t('admin.groupAssociations.groupDisplay.empty') }}
            </div>
            <div
              v-for="(row, index) in orderedRows"
              :key="`manage:${row.id}`"
              class="flex items-center gap-2 rounded-md px-1 py-1.5 hover:bg-surface/60"
            >
              <label class="flex min-w-0 flex-1 cursor-pointer items-center gap-2">
                <input
                  type="checkbox"
                  :checked="!preferences.hiddenGroupIds.includes(row.id)"
                  class="h-4 w-4 rounded border-border accent-primary"
                  @change="toggleGroupVisibility(row.id)"
                >
                <span class="min-w-0 truncate text-xs text-foreground">{{ row.ownGroup }}</span>
              </label>
              <span class="shrink-0 text-[11px] text-muted-foreground">{{ mappingTargets(row.mapping).length }}</span>
              <button
                type="button"
                class="rounded p-1 text-muted-foreground transition-colors hover:bg-surface hover:text-foreground disabled:opacity-30"
                :aria-label="t('admin.groupAssociations.groupDisplay.moveUp')"
                :disabled="index === 0"
                @click="moveGroup(row.id, -1)"
              >
                <ChevronUp class="h-4 w-4" />
              </button>
              <button
                type="button"
                class="rounded p-1 text-muted-foreground transition-colors hover:bg-surface hover:text-foreground disabled:opacity-30"
                :aria-label="t('admin.groupAssociations.groupDisplay.moveDown')"
                :disabled="index === orderedRows.length - 1"
                @click="moveGroup(row.id, 1)"
              >
                <ChevronDown class="h-4 w-4" />
              </button>
            </div>
          </div>
        </div>

        <nav class="max-h-[28rem] flex-1 overflow-y-auto p-2 lg:max-h-[calc(100dvh-20rem)]" :aria-label="t('admin.groupAssociations.listAria')">
          <div v-if="filteredRows.length === 0" class="flex min-h-48 flex-col items-center justify-center px-5 text-center">
            <CircleOff class="h-8 w-8 text-muted-foreground/40" />
            <p class="mt-3 text-sm text-muted-foreground">{{ emptyGroupMessage }}</p>
          </div>
          <template v-else>
            <button
              v-for="row in filteredRows"
              :key="row.id"
              type="button"
              class="mb-1 flex w-full items-start gap-3 rounded-lg px-3 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="selectedOwnGroup === row.ownGroup ? 'bg-primary/[0.08]' : 'hover:bg-surface/60'"
              @click="selectedOwnGroup = row.ownGroup"
            >
              <span
                class="mt-1.5 h-2 w-2 shrink-0 rounded-full"
                :class="mappingTargets(row.mapping).length > 0 ? 'bg-emerald-500' : 'bg-muted-foreground/35'"
              />
              <span class="min-w-0 flex-1">
                <span class="flex items-center justify-between gap-2">
                  <span class="truncate text-sm font-medium text-foreground">{{ row.ownGroup }}</span>
                  <TriangleAlert v-if="row.stale || row.staleTargetCount" class="h-3.5 w-3.5 shrink-0 text-warning" />
                </span>
                <span class="mt-1 flex items-center justify-between gap-2 text-xs text-muted-foreground">
                  <span>{{ t('admin.groupAssociations.targetCount', { count: mappingTargets(row.mapping).length }) }}</span>
                  <span>{{ formatMultiplier(row.ownGroupInfo?.multiplier) }}</span>
                </span>
              </span>
            </button>
          </template>
        </nav>
      </aside>

      <section class="min-w-0" :aria-label="t('admin.groupAssociations.detailsLabel')">
        <div v-if="!selectedRow" class="flex min-h-[28rem] flex-col items-center justify-center px-6 text-center">
          <Layers class="h-9 w-9 text-muted-foreground/40" />
          <p class="mt-3 text-sm text-muted-foreground">{{ t('admin.groupAssociations.empty') }}</p>
        </div>

        <template v-else>
          <div v-if="selectedRow.stale" class="flex flex-col gap-3 border-b border-warning/25 bg-warning/10 px-5 py-4 sm:flex-row sm:items-center sm:justify-between">
            <div class="flex min-w-0 gap-2.5 text-sm text-warning">
              <TriangleAlert class="mt-0.5 h-4 w-4 shrink-0" />
              <span>{{ t('admin.groupAssociations.staleOwnGroup') }}</span>
            </div>
            <button
              type="button"
              class="inline-flex shrink-0 items-center gap-1.5 self-start rounded-md border border-warning/30 px-2.5 py-1.5 text-xs font-medium text-warning transition-colors hover:bg-warning/10 disabled:opacity-50 sm:self-auto"
              :disabled="savingOwnGroup === selectedRow.ownGroup"
              @click="cleanupDialogOpen = true"
            >
              <Trash2 class="h-3.5 w-3.5" />
              {{ t('admin.groupAssociations.actions.cleanup') }}
            </button>
          </div>

          <header class="flex flex-col gap-4 border-b border-border/50 px-5 py-5 sm:flex-row sm:items-start sm:justify-between">
            <div class="min-w-0">
              <div class="flex flex-wrap items-center gap-2">
                <h2 class="truncate text-xl font-semibold text-foreground">{{ selectedRow.ownGroup }}</h2>
                <span v-if="selectedRow.ownGroupInfo" class="rounded border border-border/60 bg-surface px-2 py-0.5 text-xs text-muted-foreground">
                  {{ selectedRow.ownGroupInfo.platform || t('admin.groupAssociations.common.unknown') }}
                </span>
                <span
                  v-if="savedOwnGroup === selectedRow.ownGroup"
                  class="inline-flex items-center gap-1 text-xs font-medium text-emerald-600 dark:text-emerald-400"
                >
                  <Check class="h-3.5 w-3.5" />
                  {{ t('admin.groupAssociations.saveSuccess') }}
                </span>
              </div>
              <p class="mt-1 text-sm text-muted-foreground">
                {{ t('admin.groupAssociations.connectionSummary', { included: includedAssociationRows.length, excluded: excludedAssociationRows.length }) }}
              </p>
            </div>
            <button
              type="button"
              class="inline-flex h-9 items-center justify-center gap-2 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-50"
              :disabled="savingOwnGroup === selectedRow.ownGroup"
              @click="targetsDrawerOpen = true"
            >
              <Link2 class="h-4 w-4" />
              {{ t('admin.groupAssociations.actions.editTargets') }}
            </button>
          </header>

          <dl class="grid border-b border-border/50 sm:grid-cols-4">
            <div class="border-b border-border/50 px-5 py-4 sm:border-b-0 sm:border-r">
              <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.metrics.ownMultiplier') }}</dt>
              <dd class="mt-1 text-lg font-semibold tabular-nums text-foreground">{{ formatMultiplier(selectedRow.ownGroupInfo?.multiplier) }}</dd>
            </div>
            <div class="border-b border-border/50 px-5 py-4 sm:border-b-0 sm:border-r">
              <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.metrics.targets') }}</dt>
              <dd class="mt-1 text-lg font-semibold tabular-nums text-foreground">{{ selectedTargets.length }}</dd>
            </div>
            <div class="border-b border-border/50 px-5 py-4 sm:border-b-0 sm:border-r">
              <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.metrics.budgetMargin') }}</dt>
              <dd class="mt-1">
                <button
                  type="button"
                  class="group inline-flex max-w-full items-center gap-1.5 text-left text-lg font-semibold tabular-nums transition-colors hover:underline focus-visible:rounded-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  :class="profitMarginClass(profitMarginRange?.minimum)"
                  :aria-label="t('admin.groupAssociations.actions.openProfitCalculator')"
                  aria-haspopup="dialog"
                  @click="openProfitCalculator('group')"
                >
                  <span class="truncate">{{ formatProfitMarginRange() }}</span>
                  <Calculator class="h-4 w-4 shrink-0 opacity-70 transition-opacity group-hover:opacity-100" />
                </button>
              </dd>
            </div>
            <div class="px-5 py-4">
              <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.metrics.autoPricing') }}</dt>
              <dd class="mt-1 inline-flex items-center gap-1.5 text-sm font-semibold" :class="autoPricingStatus === 'enabled' ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground'">
                <Zap class="h-4 w-4" />
                {{ t(`admin.groupAssociations.autoPricingStatus.${autoPricingStatus}`) }}
              </dd>
            </div>
          </dl>

          <div class="space-y-7 px-5 py-6">
            <section>
              <div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                <div>
                  <h3 class="text-sm font-semibold text-foreground">{{ t('admin.groupAssociations.sections.targets') }}</h3>
                  <p class="mt-1 text-xs text-muted-foreground">
                    {{ t('admin.groupAssociations.connectionSummary', { included: includedAssociationRows.length, excluded: excludedAssociationRows.length }) }}
                  </p>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <button
                    v-if="selectedTargets.length > 0"
                    type="button"
                    class="text-xs font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                    @click="targetsDrawerOpen = true"
                  >
                    {{ t('admin.groupAssociations.actions.manage') }}
                  </button>
                  <button
                    v-if="excludedAssociationRows.length > 0"
                    type="button"
                    class="inline-flex items-center gap-1.5 rounded-md border border-emerald-500/25 bg-emerald-500/5 px-2.5 py-1.5 text-xs font-medium text-emerald-700 transition-colors hover:bg-emerald-500/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary dark:text-emerald-400"
                    @click="showExcluded = !showExcluded"
                  >
                    <ChevronUp v-if="showExcluded" class="h-3.5 w-3.5" />
                    <ChevronDown v-else class="h-3.5 w-3.5" />
                    {{ t(showExcluded ? 'admin.groupAssociations.associationTable.hideExcluded' : 'admin.groupAssociations.associationTable.showExcluded', { count: excludedAssociationRows.length }) }}
                  </button>
                </div>
              </div>

              <div v-if="associationRows.length === 0" class="mt-3 flex min-h-28 flex-col items-center justify-center rounded-lg border border-dashed border-border/70 px-5 text-center">
                <Link2 class="h-6 w-6 text-muted-foreground/45" />
                <p class="mt-2 text-sm font-medium text-foreground">{{ t(selectedTargets.length > 0 ? 'admin.groupAssociations.noTargets.title' : 'admin.groupAssociations.noConnections.title') }}</p>
                <p class="mt-1 text-xs text-muted-foreground">{{ t(selectedTargets.length > 0 ? 'admin.groupAssociations.noTargets.description' : 'admin.groupAssociations.noConnections.description') }}</p>
              </div>

              <template v-else>
                <div class="mt-3 hidden overflow-x-auto rounded-lg border border-border/60 md:block">
                  <table class="w-full min-w-[760px] table-fixed text-left">
                    <thead class="border-b border-border/60 bg-surface/50 text-xs text-muted-foreground">
                      <tr>
                        <th class="w-[28%] px-4 py-3 font-medium" scope="col" :aria-sort="associationSortAria('name')">
                          <button type="button" class="inline-flex items-center gap-1.5 font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleAssociationSort('name')">
                            {{ t('admin.groupAssociations.associationTable.columns.name') }}
                            <ChevronUp v-if="associationSortKey === 'name' && associationSortDirection === 'asc'" class="h-3.5 w-3.5" />
                            <ChevronDown v-else-if="associationSortKey === 'name'" class="h-3.5 w-3.5" />
                            <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-60" />
                          </button>
                        </th>
                        <th class="w-[14%] px-4 py-3 font-medium" scope="col" :aria-sort="associationSortAria('sourceStatus')">
                          <button type="button" class="inline-flex items-center gap-1.5 font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleAssociationSort('sourceStatus')">
                            {{ t('admin.groupAssociations.associationTable.columns.sourceStatus') }}
                            <ChevronUp v-if="associationSortKey === 'sourceStatus' && associationSortDirection === 'asc'" class="h-3.5 w-3.5" />
                            <ChevronDown v-else-if="associationSortKey === 'sourceStatus'" class="h-3.5 w-3.5" />
                            <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-60" />
                          </button>
                        </th>
                        <th class="w-[16%] px-4 py-3 font-medium" scope="col" :aria-sort="associationSortAria('healthStatus')">
                          <button type="button" class="inline-flex items-center gap-1.5 font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleAssociationSort('healthStatus')">
                            {{ t('admin.groupAssociations.associationTable.columns.healthStatus') }}
                            <ChevronUp v-if="associationSortKey === 'healthStatus' && associationSortDirection === 'asc'" class="h-3.5 w-3.5" />
                            <ChevronDown v-else-if="associationSortKey === 'healthStatus'" class="h-3.5 w-3.5" />
                            <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-60" />
                          </button>
                        </th>
                        <th class="w-[16%] px-4 py-3 font-medium" scope="col" :aria-sort="associationSortAria('upstreamMultiplier')">
                          <button type="button" class="inline-flex items-center gap-1.5 font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleAssociationSort('upstreamMultiplier')">
                            {{ t('admin.groupAssociations.associationTable.columns.upstreamMultiplier') }}
                            <ChevronUp v-if="associationSortKey === 'upstreamMultiplier' && associationSortDirection === 'asc'" class="h-3.5 w-3.5" />
                            <ChevronDown v-else-if="associationSortKey === 'upstreamMultiplier'" class="h-3.5 w-3.5" />
                            <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-60" />
                          </button>
                        </th>
                        <th class="w-[13%] px-4 py-3 font-medium" scope="col" :aria-sort="associationSortAria('effectiveCostMultiplier')">
                          <button type="button" class="inline-flex items-center gap-1.5 font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleAssociationSort('effectiveCostMultiplier')">
                            {{ t('admin.groupAssociations.associationTable.columns.effectiveCost') }}
                            <ChevronUp v-if="associationSortKey === 'effectiveCostMultiplier' && associationSortDirection === 'asc'" class="h-3.5 w-3.5" />
                            <ChevronDown v-else-if="associationSortKey === 'effectiveCostMultiplier'" class="h-3.5 w-3.5" />
                            <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-60" />
                          </button>
                        </th>
                        <th class="w-[13%] px-4 py-3 font-medium" scope="col" :aria-sort="associationSortAria('profitMargin')">
                          <button type="button" class="inline-flex items-center gap-1.5 font-medium hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleAssociationSort('profitMargin')">
                            {{ t('admin.groupAssociations.associationTable.columns.profitMargin') }}
                            <ChevronUp v-if="associationSortKey === 'profitMargin' && associationSortDirection === 'asc'" class="h-3.5 w-3.5" />
                            <ChevronDown v-else-if="associationSortKey === 'profitMargin'" class="h-3.5 w-3.5" />
                            <ArrowDownUp v-else class="h-3.5 w-3.5 opacity-60" />
                          </button>
                        </th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-border/50">
                      <tr
                        v-for="target in visibleAssociationRows"
                        :key="target.key"
                        class="align-top transition-colors"
                        :class="target.included ? '' : 'bg-emerald-500/[0.045]'"
                      >
                        <td class="px-4 py-3">
                          <div class="min-w-0">
                            <div class="flex flex-wrap items-center gap-2">
                              <span class="truncate text-sm font-medium text-foreground">{{ target.groupName }}</span>
                              <span v-if="!target.included" class="rounded border border-emerald-500/25 bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-400">
                                {{ t('admin.groupAssociations.associationTable.excludedLabel') }}
                              </span>
                              <span v-if="target.stale" class="rounded border border-warning/30 bg-warning/10 px-1.5 py-0.5 text-[10px] font-medium text-warning">
                                {{ t('admin.groupAssociations.staleTarget') }}
                              </span>
                            </div>
                            <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                              <span>{{ target.siteName }}</span>
                              <span v-if="target.platform" class="rounded bg-surface-elevated px-1.5 py-0.5">{{ target.platform }}</span>
                            </div>
                          </div>
                        </td>
                        <td class="px-4 py-3">
                          <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="sourceStatusClass(target.sourceStatus)">{{ sourceStatusLabel(target.sourceStatus) }}</span>
                        </td>
                        <td class="px-4 py-3">
                          <span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="healthStatusClass(target.healthStatus)">{{ healthStatusLabel(target.healthStatus) }}</span>
                        </td>
                        <td class="px-4 py-3 text-sm tabular-nums text-foreground">
                          <div class="font-semibold">{{ formatMultiplier(target.upstreamMultiplier) }}</div>
                          <div class="mt-1 text-xs text-muted-foreground">{{ t('admin.groupAssociations.associationTable.recentChange') }} {{ formatRecentDelta(target.delta) }}</div>
                        </td>
                        <td class="px-4 py-3 text-sm font-semibold tabular-nums text-foreground">{{ formatMultiplier(target.effectiveCostMultiplier) }}</td>
                        <td class="px-4 py-3 text-sm font-semibold tabular-nums" :class="profitMarginClass(target.profitMargin)">{{ formatProfitMargin(target.profitMargin) }}</td>
                      </tr>
                    </tbody>
                  </table>
                </div>

                <div class="mt-3 space-y-2 md:hidden">
                  <div
                    v-for="target in visibleAssociationRows"
                    :key="target.key"
                    class="rounded-lg border border-border/60 p-4"
                    :class="target.included ? '' : 'border-emerald-500/25 bg-emerald-500/[0.045]'"
                  >
                    <div class="flex items-start justify-between gap-3">
                      <div class="min-w-0">
                        <div class="flex flex-wrap items-center gap-2">
                          <span class="truncate text-sm font-medium text-foreground">{{ target.groupName }}</span>
                          <span v-if="!target.included" class="rounded border border-emerald-500/25 bg-emerald-500/10 px-1.5 py-0.5 text-[10px] font-medium text-emerald-700 dark:text-emerald-400">{{ t('admin.groupAssociations.associationTable.excludedLabel') }}</span>
                        </div>
                        <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                          <span>{{ target.siteName }}</span>
                          <span v-if="target.platform" class="rounded bg-surface-elevated px-1.5 py-0.5">{{ target.platform }}</span>
                        </div>
                      </div>
                      <span v-if="target.stale" class="shrink-0 rounded border border-warning/30 bg-warning/10 px-1.5 py-0.5 text-[10px] font-medium text-warning">{{ t('admin.groupAssociations.staleTarget') }}</span>
                    </div>
                    <div class="mt-4 grid grid-cols-2 gap-3 border-t border-border/50 pt-3">
                      <div>
                        <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.associationTable.columns.sourceStatus') }}</div>
                        <div class="mt-1"><span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="sourceStatusClass(target.sourceStatus)">{{ sourceStatusLabel(target.sourceStatus) }}</span></div>
                      </div>
                      <div>
                        <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.associationTable.columns.healthStatus') }}</div>
                        <div class="mt-1"><span class="inline-flex rounded-md px-2 py-1 text-xs font-medium" :class="healthStatusClass(target.healthStatus)">{{ healthStatusLabel(target.healthStatus) }}</span></div>
                      </div>
                      <div>
                        <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.associationTable.columns.upstreamMultiplier') }}</div>
                        <div class="mt-1 text-sm font-semibold tabular-nums text-foreground">{{ formatMultiplier(target.upstreamMultiplier) }}</div>
                        <div class="mt-0.5 text-xs text-muted-foreground">{{ formatRecentDelta(target.delta) }}</div>
                      </div>
                      <div>
                        <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.associationTable.columns.effectiveCost') }}</div>
                        <div class="mt-1 text-sm font-semibold tabular-nums text-foreground">{{ formatMultiplier(target.effectiveCostMultiplier) }}</div>
                      </div>
                      <div class="col-span-2">
                        <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.associationTable.columns.profitMargin') }}</div>
                        <div class="mt-1 text-sm font-semibold tabular-nums" :class="profitMarginClass(target.profitMargin)">{{ formatProfitMargin(target.profitMargin) }}</div>
                      </div>
                    </div>
                  </div>
                </div>
              </template>
            </section>

            <section class="border-t border-border/50 pt-6">
              <div class="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
                <div class="min-w-0">
                  <div class="flex items-center gap-2">
                    <Zap class="h-4 w-4 text-primary" />
                    <h3 class="text-sm font-semibold text-foreground">{{ t('admin.groupAssociations.sections.autoPricing') }}</h3>
                  </div>
                  <p class="mt-1 text-xs text-muted-foreground">
                    {{ t(`admin.groupAssociations.autoPricingStatus.${autoPricingStatus}`) }}
                  </p>
                  <div class="mt-3 text-xs text-muted-foreground">
                    {{ t('admin.groupAssociations.lastRun.summary', {
                      status: t(`admin.groupAssociations.lastRun.status.${runStatusKey(selectedMapping?.lastAutoPricingRun?.status)}`),
                      trigger: runTriggerLabel(selectedMapping?.lastAutoPricingRun?.trigger),
                      time: formatRunTime(selectedMapping?.lastAutoPricingRun?.ranAt),
                    }) }}
                  </div>
                  <p v-if="selectedMapping?.lastAutoPricingRun?.reason" class="mt-1 text-xs text-muted-foreground">
                    {{ t('admin.groupAssociations.lastRun.reason', { reason: runReasonLabel(selectedMapping.lastAutoPricingRun) }) }}
                  </p>
                </div>
                <div class="flex shrink-0 flex-wrap items-center gap-2">
                  <button
                    v-if="selectedMapping?.enableAutoPricing && selectedTargets.length > 0"
                    type="button"
                    class="inline-flex h-9 items-center gap-2 rounded-lg border border-border/60 px-3 text-sm font-medium text-foreground transition-colors hover:bg-surface-elevated disabled:opacity-50"
                    :disabled="runningOwnGroup !== null"
                    @click="runNow"
                  >
                    <Loader2 v-if="runningOwnGroup === selectedRow.ownGroup" class="h-4 w-4 animate-spin" />
                    <Play v-else class="h-4 w-4" />
                    {{ t('admin.groupAssociations.autoPricingActions.runNow') }}
                  </button>
                  <button
                    type="button"
                    class="inline-flex h-9 items-center gap-2 rounded-lg border border-border/60 px-3 text-sm font-medium text-foreground transition-colors hover:bg-surface-elevated disabled:opacity-50"
                    :disabled="selectedTargets.length === 0"
                    @click="pricingDrawerOpen = true"
                  >
                    <Settings2 class="h-4 w-4" />
                    {{ t(autoPricingStatus === 'notConfigured' ? 'admin.groupAssociations.autoPricingActions.configure' : 'admin.groupAssociations.autoPricingActions.edit') }}
                  </button>
                </div>
              </div>
            </section>
          </div>
        </template>
      </section>
    </div>

    <GroupAssociationTargetsDrawer
      :open="targetsDrawerOpen"
      :own-group="selectedRow?.ownGroup ?? ''"
      :options="targetOptions"
      :selected="selectedTargets"
      :saving="savingOwnGroup === selectedRow?.ownGroup"
      @close="targetsDrawerOpen = false"
      @save="saveTargets"
    />

    <AutoPricingConfigDrawer
      :open="pricingDrawerOpen"
      :mapping="selectedMapping"
      :upstream-multipliers="upstreamMultiplierMap"
      :upstream-labels="upstreamLabels"
      :available-bots="botOptions"
      :saving="savingOwnGroup === selectedRow?.ownGroup"
      @close="pricingDrawerOpen = false"
      @save="savePricing"
    />

    <Teleport to="body">
      <div
        v-if="profitCalculatorOpen"
        class="fixed inset-0 z-[180] flex items-center justify-center bg-background/80 p-4 backdrop-blur-sm"
        @click.self="closeProfitCalculator"
      >
        <div
          role="dialog"
          aria-modal="true"
          aria-labelledby="profit-calculator-title"
          class="flex max-h-[min(44rem,calc(100dvh-2rem))] w-full max-w-3xl flex-col overflow-hidden rounded-lg border border-border/60 bg-card text-card-foreground shadow-xl"
        >
          <header class="flex items-start justify-between gap-4 border-b border-border/60 px-5 py-4">
            <div class="flex min-w-0 items-start gap-3">
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Calculator class="h-4 w-4" />
              </span>
              <div class="min-w-0">
                <h2 id="profit-calculator-title" class="truncate text-base font-semibold text-foreground">
                  {{ profitCalculatorMode === 'custom'
                    ? t('admin.groupAssociations.profitCalculator.customTitle')
                    : t('admin.groupAssociations.profitCalculator.titleWithGroup', { group: selectedRow?.ownGroup ?? '' }) }}
                </h2>
              </div>
            </div>
            <button
              type="button"
              class="inline-flex h-9 w-9 shrink-0 items-center justify-center rounded-lg text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :aria-label="t('admin.groupAssociations.profitCalculator.close')"
              :title="t('admin.groupAssociations.profitCalculator.close')"
              @click="closeProfitCalculator"
            >
              <X class="h-4 w-4" />
            </button>
          </header>

          <div class="border-b border-border/60 px-5 py-3">
            <div class="grid grid-cols-2 gap-1 rounded-lg bg-surface p-1" role="tablist" :aria-label="t('admin.groupAssociations.profitCalculator.modeLabel')">
              <button
                type="button"
                role="tab"
                class="rounded-md px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:cursor-not-allowed disabled:opacity-50"
                :class="profitCalculatorMode === 'group' ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
                :aria-selected="profitCalculatorMode === 'group'"
                :disabled="!selectedRow"
                @click="profitCalculatorMode = 'group'"
              >
                {{ t('admin.groupAssociations.profitCalculator.groupMode') }}
              </button>
              <button
                type="button"
                role="tab"
                class="rounded-md px-3 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                :class="profitCalculatorMode === 'custom' ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
                :aria-selected="profitCalculatorMode === 'custom'"
                @click="profitCalculatorMode = 'custom'"
              >
                {{ t('admin.groupAssociations.profitCalculator.customMode') }}
              </button>
            </div>
          </div>

          <div class="overflow-y-auto">
            <template v-if="profitCalculatorMode === 'group' && selectedRow">
            <section class="border-b border-border/60 px-5 py-5">
              <label for="simulated-revenue" class="text-xs font-medium text-muted-foreground">
                {{ t('admin.groupAssociations.profitCalculator.revenueLabel') }}
              </label>
              <div class="relative mt-2 max-w-sm">
                <span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
                <input
                  id="simulated-revenue"
                  v-model="simulatedRevenueInput"
                  type="number"
                  min="0"
                  step="0.01"
                  inputmode="decimal"
                  :placeholder="t('admin.groupAssociations.profitCalculator.revenuePlaceholder')"
                  class="h-11 w-full rounded-lg border bg-background pl-8 pr-3 text-base font-semibold tabular-nums text-foreground outline-none placeholder:text-sm placeholder:font-normal placeholder:text-muted-foreground focus-visible:ring-2"
                  :class="simulatedRevenue == null ? 'border-destructive focus-visible:border-destructive focus-visible:ring-destructive/20' : 'border-border/60 focus-visible:border-primary focus-visible:ring-primary/25'"
                >
              </div>
              <p v-if="simulatedRevenue == null" class="mt-2 text-xs text-destructive">
                {{ t('admin.groupAssociations.profitCalculator.invalidRevenue') }}
              </p>
            </section>

            <dl class="grid border-b border-border/60 sm:grid-cols-2">
              <div class="border-b border-border/60 px-5 py-4 sm:border-b-0 sm:border-r">
                <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.profitCalculator.ownMultiplier') }}</dt>
                <dd class="mt-1 text-lg font-semibold tabular-nums text-foreground">{{ formatMultiplier(selectedRow.ownGroupInfo?.multiplier) }}</dd>
              </div>
              <div class="px-5 py-4">
                <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.profitCalculator.profitRange') }}</dt>
                <dd class="mt-1 text-lg font-semibold tabular-nums" :class="profitMarginClass(simulatedProfitRange?.minimum)">
                  {{ formatSimulatedProfitRange() }}
                </dd>
              </div>
            </dl>

            <section class="px-5 py-5">
              <div v-if="profitSimulationRows.length === 0" class="flex min-h-36 flex-col items-center justify-center border-y border-dashed border-border/70 px-5 text-center">
                <Calculator class="h-6 w-6 text-muted-foreground/45" />
                <p class="mt-2 text-sm font-medium text-foreground">{{ t('admin.groupAssociations.profitCalculator.noTargetsTitle') }}</p>
                <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.groupAssociations.profitCalculator.noTargetsDescription') }}</p>
              </div>

              <div v-else class="divide-y divide-border/50 border-y border-border/60">
                <div
                  v-for="target in profitSimulationRows"
                  :key="targetKey(target.siteId, target.groupName)"
                  class="grid grid-cols-2 gap-x-4 gap-y-3 py-4 sm:grid-cols-[minmax(0,1fr)_repeat(3,minmax(7rem,auto))] sm:items-center"
                >
                  <div class="col-span-2 min-w-0 sm:col-span-1">
                    <div class="truncate text-sm font-medium text-foreground">{{ target.groupName }}</div>
                    <div class="mt-1 truncate text-xs text-muted-foreground">{{ target.siteName }}</div>
                  </div>
                  <div>
                    <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.profitCalculator.estimatedCost') }}</div>
                    <div class="mt-1 text-sm font-semibold tabular-nums text-foreground">{{ formatCurrency(target.estimatedCost) }}</div>
                  </div>
                  <div>
                    <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.metrics.budgetMargin') }}</div>
                    <div class="mt-1 text-sm font-semibold tabular-nums" :class="profitMarginClass(target.profitMargin)">
                      {{ formatProfitMargin(target.profitMargin) }}
                    </div>
                  </div>
                  <div class="col-span-2 sm:col-span-1 sm:text-right">
                    <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.profitCalculator.estimatedProfit') }}</div>
                    <div class="mt-1 text-base font-semibold tabular-nums" :class="profitMarginClass(target.estimatedProfit)">
                      {{ formatCurrency(target.estimatedProfit) }}
                    </div>
                  </div>
                </div>
              </div>
            </section>
            </template>

            <template v-else>
              <section class="border-b border-border/60 px-5 py-5">
                <div class="grid gap-4 md:grid-cols-3">
                  <div>
                    <label for="custom-simulated-revenue" class="text-xs font-medium text-muted-foreground">
                      {{ t('admin.groupAssociations.profitCalculator.revenueLabel') }}
                    </label>
                    <div class="relative mt-2">
                      <span class="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">¥</span>
                      <input
                        id="custom-simulated-revenue"
                        v-model="simulatedRevenueInput"
                        type="number"
                        min="0"
                        step="0.01"
                        inputmode="decimal"
                        :placeholder="t('admin.groupAssociations.profitCalculator.revenuePlaceholder')"
                        :aria-invalid="simulatedRevenue == null"
                        class="h-11 w-full rounded-lg border bg-background pl-8 pr-3 text-base font-semibold tabular-nums text-foreground outline-none placeholder:text-sm placeholder:font-normal placeholder:text-muted-foreground focus-visible:ring-2"
                        :class="simulatedRevenue == null ? 'border-destructive focus-visible:border-destructive focus-visible:ring-destructive/20' : 'border-border/60 focus-visible:border-primary focus-visible:ring-primary/25'"
                      >
                    </div>
                    <p v-if="simulatedRevenue == null" class="mt-2 text-xs text-destructive">
                      {{ t('admin.groupAssociations.profitCalculator.invalidRevenue') }}
                    </p>
                  </div>

                  <div>
                    <label for="custom-upstream-multiplier" class="text-xs font-medium text-muted-foreground">
                      {{ t('admin.groupAssociations.profitCalculator.upstreamCostMultiplier') }}
                    </label>
                    <div class="relative mt-2">
                      <input
                        id="custom-upstream-multiplier"
                        v-model="customUpstreamMultiplierInput"
                        type="number"
                        min="0"
                        step="0.0001"
                        inputmode="decimal"
                        :placeholder="t('admin.groupAssociations.profitCalculator.multiplierPlaceholder')"
                        :aria-invalid="hasCustomUpstreamMultiplierInput && customUpstreamMultiplier == null"
                        class="h-11 w-full rounded-lg border bg-background pl-3 pr-8 text-base font-semibold tabular-nums text-foreground outline-none placeholder:text-sm placeholder:font-normal placeholder:text-muted-foreground focus-visible:ring-2"
                        :class="hasCustomUpstreamMultiplierInput && customUpstreamMultiplier == null ? 'border-destructive focus-visible:border-destructive focus-visible:ring-destructive/20' : 'border-border/60 focus-visible:border-primary focus-visible:ring-primary/25'"
                      >
                      <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">x</span>
                    </div>
                    <p v-if="hasCustomUpstreamMultiplierInput && customUpstreamMultiplier == null" class="mt-2 text-xs text-destructive">
                      {{ t('admin.groupAssociations.profitCalculator.invalidUpstreamMultiplier') }}
                    </p>
                  </div>

                  <div>
                    <label for="custom-sale-multiplier" class="text-xs font-medium text-muted-foreground">
                      {{ t('admin.groupAssociations.profitCalculator.saleMultiplier') }}
                    </label>
                    <div class="relative mt-2">
                      <input
                        id="custom-sale-multiplier"
                        v-model="customSaleMultiplierInput"
                        type="number"
                        min="0.0001"
                        step="0.0001"
                        inputmode="decimal"
                        :placeholder="t('admin.groupAssociations.profitCalculator.multiplierPlaceholder')"
                        :aria-invalid="hasCustomSaleMultiplierInput && customSaleMultiplier == null"
                        class="h-11 w-full rounded-lg border bg-background pl-3 pr-8 text-base font-semibold tabular-nums text-foreground outline-none placeholder:text-sm placeholder:font-normal placeholder:text-muted-foreground focus-visible:ring-2"
                        :class="hasCustomSaleMultiplierInput && customSaleMultiplier == null ? 'border-destructive focus-visible:border-destructive focus-visible:ring-destructive/20' : 'border-border/60 focus-visible:border-primary focus-visible:ring-primary/25'"
                      >
                      <span class="pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">x</span>
                    </div>
                    <p v-if="hasCustomSaleMultiplierInput && customSaleMultiplier == null" class="mt-2 text-xs text-destructive">
                      {{ t('admin.groupAssociations.profitCalculator.invalidSaleMultiplier') }}
                    </p>
                  </div>
                </div>
              </section>

              <dl class="grid sm:grid-cols-3" aria-live="polite">
                <div class="border-b border-border/60 px-5 py-5 sm:border-b-0 sm:border-r">
                  <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.profitCalculator.estimatedCost') }}</dt>
                  <dd class="mt-1 text-lg font-semibold tabular-nums text-foreground">
                    {{ formatCurrency(customProfitSimulation?.estimatedCost) }}
                  </dd>
                </div>
                <div class="border-b border-border/60 px-5 py-5 sm:border-b-0 sm:border-r">
                  <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.profitCalculator.profitMargin') }}</dt>
                  <dd class="mt-1 text-lg font-semibold tabular-nums" :class="profitMarginClass(customProfitSimulation?.margin)">
                    {{ formatProfitMargin(customProfitSimulation?.margin) }}
                  </dd>
                </div>
                <div class="px-5 py-5">
                  <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.profitCalculator.estimatedProfit') }}</dt>
                  <dd class="mt-1 text-lg font-semibold tabular-nums" :class="profitMarginClass(customProfitSimulation?.estimatedProfit)">
                    {{ formatCurrency(customProfitSimulation?.estimatedProfit) }}
                  </dd>
                </div>
              </dl>
            </template>
          </div>
        </div>
      </div>
    </Teleport>

    <Teleport to="body">
      <div v-if="cleanupDialogOpen && selectedRow" class="fixed inset-0 z-[170] flex items-center justify-center bg-background/75 p-4 backdrop-blur-sm">
        <div role="alertdialog" aria-modal="true" class="w-full max-w-md rounded-lg border border-border/60 bg-card p-5 shadow-xl">
          <div class="flex items-start gap-3">
            <div class="flex h-9 w-9 shrink-0 items-center justify-center rounded-lg bg-warning/10 text-warning">
              <TriangleAlert class="h-4 w-4" />
            </div>
            <div>
              <h2 class="text-base font-semibold text-foreground">{{ t('admin.groupAssociations.cleanup.title') }}</h2>
              <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ t('admin.groupAssociations.cleanup.description', { group: selectedRow.ownGroup }) }}</p>
            </div>
          </div>
          <div class="mt-5 flex justify-end gap-2">
            <button type="button" class="rounded-lg border border-border/60 px-3 py-2 text-sm font-medium text-muted-foreground hover:bg-surface-elevated" @click="cleanupDialogOpen = false">
              {{ t('admin.groupAssociations.cleanup.cancel') }}
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-2 rounded-lg bg-destructive px-3 py-2 text-sm font-medium text-destructive-foreground hover:bg-destructive/90 disabled:opacity-50"
              :disabled="savingOwnGroup === selectedRow.ownGroup"
              @click="cleanupMapping"
            >
              <Loader2 v-if="savingOwnGroup === selectedRow.ownGroup" class="h-4 w-4 animate-spin" />
              <Trash2 v-else class="h-4 w-4" />
              {{ t('admin.groupAssociations.cleanup.confirm') }}
            </button>
          </div>
        </div>
      </div>
    </Teleport>
  </section>
</template>
