<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  AlertCircle,
  Check,
  ChevronRight,
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
  Zap,
} from 'lucide-vue-next'
import { getMySiteMappingOptions, removeMySiteMapping, runAutoPricing, saveMySiteMapping } from '../../api/mySites'
import { listUpstreamSites } from '../../api/upstream'
import { getNotificationChannelSettings } from '../../api/settings'
import type {
  AutoPricingRunResult,
  MySiteGroupRef,
  MySiteMapping,
  MySiteMappingOwnGroupOption,
  MySiteUpstreamTargetOption,
} from '../../types/mySites'
import type { UpstreamSiteResponse } from '../../types/upstream'
import AutoPricingConfigDrawer, { type BotOption } from './AutoPricingConfigDrawer.vue'
import GroupAssociationTargetsDrawer from './GroupAssociationTargetsDrawer.vue'

type AssociationFilter = 'all' | 'associated' | 'unassociated' | 'stale'

interface AssociationRow {
  ownGroup: string
  ownGroupInfo: MySiteMappingOwnGroupOption | null
  mapping: MySiteMapping
  stale: boolean
  staleTargetCount: number
}

const { t, locale } = useI18n()
const loading = ref(true)
const error = ref<string | null>(null)
const ownGroups = ref<MySiteMappingOwnGroupOption[]>([])
const mappings = ref<MySiteMapping[]>([])
const upstreamSites = ref<UpstreamSiteResponse[]>([])
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
let savedTimer: ReturnType<typeof setTimeout> | null = null

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

const rows = computed<AssociationRow[]>(() => {
  const result: AssociationRow[] = []
  const seen = new Set<string>()
  for (const group of ownGroups.value) {
    seen.add(group.groupName)
    const mapping = mappingByOwnGroup.value.get(group.groupName) ?? { ownGroup: group.groupName, upstreamTargets: [] }
    result.push({
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

const filteredRows = computed(() => {
  const query = search.value.trim().toLocaleLowerCase()
  return rows.value.filter(row => {
    const matchesSearch = !query || row.ownGroup.toLocaleLowerCase().includes(query) || mappingTargets(row.mapping).some(target => {
      const siteName = siteById.value.get(target.siteId)?.name ?? target.siteId
      return target.groupName.toLocaleLowerCase().includes(query) || siteName.toLocaleLowerCase().includes(query)
    })
    if (!matchesSearch) return false
    if (filter.value === 'associated') return mappingTargets(row.mapping).length > 0
    if (filter.value === 'unassociated') return mappingTargets(row.mapping).length === 0
    if (filter.value === 'stale') return row.stale || row.staleTargetCount > 0
    return true
  })
})

const selectedRow = computed(() => rows.value.find(row => row.ownGroup === selectedOwnGroup.value) ?? null)
const selectedMapping = computed<MySiteMapping | null>(() => selectedRow.value?.mapping ?? null)

watch(rows, (nextRows) => {
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
const selectedTargetDetails = computed(() => selectedTargets.value.map(target => {
  const option = targetOptions.value.find(item => item.siteId === target.siteId && item.groupName === target.groupName)
  return option ?? {
    ...target,
    siteName: target.siteId,
    platform: '',
    multiplier: null,
    multiplierMode: undefined,
    stale: true,
  }
}))

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

const formatRunTime = (value: string | undefined): string => {
  if (!value) return t('admin.groupAssociations.lastRun.never')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('admin.groupAssociations.lastRun.never')
  return new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

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
  try {
    const [mappingResponse, sites, channelSettings] = await Promise.all([
      getMySiteMappingOptions(),
      listUpstreamSites().catch(() => []),
      getNotificationChannelSettings().catch(() => ({ dingtalk: [], feishu: [], telegram: [] })),
    ])
    ownGroups.value = mappingResponse.ownGroups ?? []
    mappings.value = mappingResponse.mappings ?? []
    staleOwnGroupNames.value = mappingResponse.staleOwnGroups ?? []
    staleTargetRefs.value = mappingResponse.staleTargets ?? []
    upstreamSites.value = sites
    botOptions.value = [
      ...(channelSettings.dingtalk ?? []).filter(bot => bot.enabled).map(bot => ({ id: bot.id, name: bot.name, channel: 'DingTalk' })),
      ...(channelSettings.feishu ?? []).filter(bot => bot.enabled).map(bot => ({ id: bot.id, name: bot.name, channel: 'Feishu' })),
      ...(channelSettings.telegram ?? []).filter(bot => bot.enabled).map(bot => ({ id: bot.id, name: bot.name, channel: 'Telegram' })),
    ]
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.groupAssociations.loadError'
  } finally {
    loading.value = false
  }
}

onMounted(() => { void loadData() })
onBeforeUnmount(() => { if (savedTimer) clearTimeout(savedTimer) })
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
        </div>

        <nav class="max-h-[24rem] flex-1 overflow-y-auto p-2 lg:max-h-[calc(100dvh-22rem)]" :aria-label="t('admin.groupAssociations.listAria')">
          <div v-if="filteredRows.length === 0" class="flex min-h-44 flex-col items-center justify-center px-5 text-center">
            <CircleOff class="h-7 w-7 text-muted-foreground/50" />
            <p class="mt-3 text-sm font-medium text-foreground">{{ t('admin.groupAssociations.empty') }}</p>
          </div>
          <button
            v-for="row in filteredRows"
            v-else
            :key="row.ownGroup"
            type="button"
            class="mb-1 flex w-full items-center gap-3 rounded-lg px-3 py-3 text-left transition-colors last:mb-0 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :class="selectedOwnGroup === row.ownGroup ? 'bg-primary/[0.08] text-foreground' : 'text-muted-foreground hover:bg-surface/60 hover:text-foreground'"
            @click="selectedOwnGroup = row.ownGroup"
          >
            <span class="min-w-0 flex-1">
              <span class="flex items-center gap-2">
                <span class="truncate text-sm font-medium">{{ row.ownGroup }}</span>
                <TriangleAlert v-if="row.stale || row.staleTargetCount" class="h-3.5 w-3.5 shrink-0 text-warning" />
              </span>
              <span class="mt-1 block text-xs text-muted-foreground">
                {{ t('admin.groupAssociations.targetCount', { count: mappingTargets(row.mapping).length }) }}
              </span>
            </span>
            <ChevronRight class="h-4 w-4 shrink-0 opacity-60" />
          </button>
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
                {{ t('admin.groupAssociations.detailSubtitle', { count: selectedTargets.length }) }}
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

          <dl class="grid border-b border-border/50 sm:grid-cols-3">
            <div class="border-b border-border/50 px-5 py-4 sm:border-b-0 sm:border-r">
              <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.metrics.ownMultiplier') }}</dt>
              <dd class="mt-1 text-lg font-semibold tabular-nums text-foreground">{{ formatMultiplier(selectedRow.ownGroupInfo?.multiplier) }}</dd>
            </div>
            <div class="border-b border-border/50 px-5 py-4 sm:border-b-0 sm:border-r">
              <dt class="text-xs font-medium text-muted-foreground">{{ t('admin.groupAssociations.metrics.targets') }}</dt>
              <dd class="mt-1 text-lg font-semibold tabular-nums text-foreground">{{ selectedTargets.length }}</dd>
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
              <div class="flex items-center justify-between gap-3">
                <div>
                  <h3 class="text-sm font-semibold text-foreground">{{ t('admin.groupAssociations.sections.targets') }}</h3>
                  <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.groupAssociations.sections.targetsSummary', { count: selectedTargets.length }) }}</p>
                </div>
                <button type="button" class="text-xs font-medium text-primary hover:underline" @click="targetsDrawerOpen = true">
                  {{ t('admin.groupAssociations.actions.manage') }}
                </button>
              </div>

              <div v-if="selectedTargetDetails.length === 0" class="mt-3 flex min-h-28 flex-col items-center justify-center rounded-lg border border-dashed border-border/70 px-5 text-center">
                <Link2 class="h-6 w-6 text-muted-foreground/45" />
                <p class="mt-2 text-sm font-medium text-foreground">{{ t('admin.groupAssociations.noTargets.title') }}</p>
                <p class="mt-1 text-xs text-muted-foreground">{{ t('admin.groupAssociations.noTargets.description') }}</p>
              </div>

              <div v-else class="mt-3 divide-y divide-border/50 rounded-lg border border-border/60">
                <div v-for="target in selectedTargetDetails" :key="targetKey(target.siteId, target.groupName)" class="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center">
                  <div class="min-w-0 flex-1">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="truncate text-sm font-medium text-foreground">{{ target.groupName }}</span>
                      <span v-if="target.stale" class="rounded border border-warning/30 bg-warning/10 px-1.5 py-0.5 text-[10px] font-medium text-warning">
                        {{ t('admin.groupAssociations.staleTarget') }}
                      </span>
                    </div>
                    <div class="mt-1 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                      <span>{{ target.siteName }}</span>
                      <span v-if="target.platform" class="rounded bg-surface-elevated px-1.5 py-0.5">{{ target.platform }}</span>
                    </div>
                  </div>
                  <div class="shrink-0 text-left sm:text-right">
                    <div class="text-sm font-semibold tabular-nums text-foreground">{{ formatTargetMultiplier(target) }}</div>
                    <div class="text-[11px] text-muted-foreground">{{ t('admin.groupAssociations.metrics.effectiveUpstream') }}</div>
                  </div>
                </div>
              </div>
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
