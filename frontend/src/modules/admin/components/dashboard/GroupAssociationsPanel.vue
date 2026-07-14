<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Layers, Loader2, Save, CheckCircle2, CircleHelp, Zap, Settings2, Play } from 'lucide-vue-next'
import { getMySiteMappingOptions, runAutoPricing, saveMySiteMappings } from '../../api/mySites'
import { listUpstreamSites } from '../../api/upstream'
import { getNotificationChannelSettings } from '../../api/settings'
import type { AutoPricingRunResult, MySiteMapping, MySiteMappingOwnGroupOption } from '../../types/mySites'
import AutoPricingConfigDrawer from './AutoPricingConfigDrawer.vue'
import type { BotOption } from './AutoPricingConfigDrawer.vue'

const { t, locale } = useI18n()

const loading = ref(false)
const error = ref<string | null>(null)
const ownGroups = ref<MySiteMappingOwnGroupOption[]>([])
const mappings = ref<MySiteMapping[]>([])
const upstreamMultiplierMap = ref<Map<string, number>>(new Map())
const botOptions = ref<BotOption[]>([])
const hoveredMappingIndex = ref<number | null>(null)
const hoveredTargetIndex = ref<number | null>(null)

const setHovered = (mi: number, ti: number) => { hoveredMappingIndex.value = mi; hoveredTargetIndex.value = ti }
const clearHovered = () => { hoveredMappingIndex.value = null; hoveredTargetIndex.value = null }

const tipTrigger = ref<HTMLElement | null>(null)
const tipVisible = ref(false)
const tipStyle = ref<Record<string, string>>({})
const tipPlacement = ref<'top' | 'bottom'>('top')
let tipTimer: ReturnType<typeof setTimeout> | null = null

const showTip = () => {
  if (tipTimer) clearTimeout(tipTimer)
  tipTimer = setTimeout(() => {
    if (!tipTrigger.value) return
    const rect = tipTrigger.value.getBoundingClientRect()
    const spaceAbove = rect.top
    const spaceBelow = window.innerHeight - rect.bottom
    const placement = spaceAbove >= 60 ? 'top' : 'bottom'
    tipPlacement.value = placement
    const left = Math.min(Math.max(rect.left + rect.width / 2, 160), window.innerWidth - 160)
    if (placement === 'top') {
      tipStyle.value = { left: `${left}px`, top: `${rect.top - 8}px`, transform: 'translate(-50%, -100%)' }
    } else {
      tipStyle.value = { left: `${left}px`, top: `${rect.bottom + 8}px`, transform: 'translate(-50%, 0)' }
    }
    tipVisible.value = true
  }, 150)
}

const hideTip = () => {
  if (tipTimer) clearTimeout(tipTimer)
  tipTimer = null
  tipVisible.value = false
}

onBeforeUnmount(() => { if (tipTimer) clearTimeout(tipTimer) })

const formatMultiplier = (value: number | null | undefined): string => {
  if (value == null || !Number.isFinite(value)) return '—'
  return `${Number(value.toFixed(4)).toString()}×`
}

const exclusiveLabel = (isExclusive: boolean): string => {
  return isExclusive ? t('admin.groupAssociations.exclusiveLabels.exclusive') : t('admin.groupAssociations.exclusiveLabels.public')
}

const statusLabel = (status: string): string => {
  const key = `admin.groupAssociations.statusLabels.${status}`
  const result = t(key)
  return result === key ? status : result
}

const mappingRows = computed(() => {
  const mappingIndex = new Map(mappings.value.map(m => [m.ownGroup, m]))
  const seen = new Set<string>()

  const rows: { index: number; ownGroup: string; ownMultiplier: number | null; platform: string; status: string; isExclusive: boolean; subscriptionType: string; upstreamTargets: { siteId: string; groupName: string; label: string; multiplier: number | null; targetIndex: number }[] }[] = []

  for (const group of ownGroups.value) {
    seen.add(group.groupName)
    const mapping = mappingIndex.get(group.groupName)
    rows.push({
      index: rows.length + 1,
      ownGroup: group.groupName,
      ownMultiplier: group.multiplier ?? null,
      platform: group.platform ?? '',
      status: group.status ?? '',
      isExclusive: group.isExclusive ?? false,
      subscriptionType: group.subscriptionType ?? '',
      upstreamTargets: mapping
        ? mapping.upstreamTargets.map((target, targetIndex) => ({
            ...target, label: target.groupName,
            multiplier: upstreamMultiplierMap.value.get(`${target.siteId}::${target.groupName}`) ?? null,
            targetIndex,
          }))
        : []
    })
  }

  for (const mapping of mappings.value) {
    if (seen.has(mapping.ownGroup)) continue
    rows.push({
      index: rows.length + 1,
      ownGroup: mapping.ownGroup,
      ownMultiplier: null,
      platform: '',
      status: '',
      isExclusive: false,
      subscriptionType: '',
      upstreamTargets: mapping.upstreamTargets.map((target, targetIndex) => ({
        ...target, label: target.groupName,
        multiplier: upstreamMultiplierMap.value.get(`${target.siteId}::${target.groupName}`) ?? null,
        targetIndex,
      }))
    })
  }

  return rows
})

const associatedCount = computed(() => mappingRows.value.filter(row => row.upstreamTargets.length > 0).length)
const unassociatedCount = computed(() => mappingRows.value.filter(row => row.upstreamTargets.length === 0).length)
const totalMappings = computed(() => mappingRows.value.length)

const isSaving = ref(false)
const showSaveSuccess = ref(false)
const savingDrawer = ref(false)
const runningOwnGroup = ref<string | null>(null)

const drawerOpen = ref(false)
const drawerOwnGroup = ref<string | null>(null)

const drawerMapping = computed<MySiteMapping | null>(() => {
  if (!drawerOwnGroup.value) return null
  return mappings.value.find(m => m.ownGroup === drawerOwnGroup.value) ?? {
    ownGroup: drawerOwnGroup.value,
    upstreamTargets: [],
  }
})

const openDrawer = (ownGroup: string) => {
  drawerOwnGroup.value = ownGroup
  drawerOpen.value = true
}

const upsertMapping = (nextMapping: MySiteMapping) => {
  const index = mappings.value.findIndex(m => m.ownGroup === nextMapping.ownGroup)
  if (index >= 0) {
    mappings.value.splice(index, 1, nextMapping)
  } else {
    mappings.value.push(nextMapping)
  }
}

const onDrawerSave = async (config: Partial<MySiteMapping>) => {
  if (!drawerOwnGroup.value) return
  if (savingDrawer.value) return

  savingDrawer.value = true
  error.value = null
  try {
    const existing = mappings.value.find(m => m.ownGroup === drawerOwnGroup.value) ?? {
      ownGroup: drawerOwnGroup.value,
      upstreamTargets: [],
    }
    const nextMapping = { ...existing, ...config, ownGroup: drawerOwnGroup.value }
    const nextMappings = mappings.value.some(m => m.ownGroup === drawerOwnGroup.value)
      ? mappings.value.map(m => m.ownGroup === drawerOwnGroup.value ? nextMapping : m)
      : [...mappings.value, nextMapping]

    const status = await saveMySiteMappings(nextMappings)
    mappings.value = status.mappings ?? nextMappings
    drawerOpen.value = false
    showSaveSuccess.value = true
    setTimeout(() => { showSaveSuccess.value = false }, 3000)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'admin.groupAssociations.saveError'
  } finally {
    savingDrawer.value = false
  }
}

const getAutoPricingStatus = (ownGroup: string): 'not_configured' | 'enabled' | 'saved_disabled' => {
  const m = mappings.value.find(m => m.ownGroup === ownGroup)
  if (!m || (m.autoPricingSource == null && m.autoPricingStrategy == null && !m.enableAutoPricing)) return 'not_configured'
  if (m.enableAutoPricing) return 'enabled'
  return 'saved_disabled'
}

const saveMappingsData = async () => {
  if (isSaving.value) return
  isSaving.value = true
  error.value = null
  try {
    const status = await saveMySiteMappings(mappings.value)
    mappings.value = status.mappings ?? mappings.value
    showSaveSuccess.value = true
    setTimeout(() => { showSaveSuccess.value = false }, 3000)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'admin.groupAssociations.saveError'
  } finally {
    isSaving.value = false
  }
}

const formatRunTime = (value: string | undefined): string => {
  if (!value) return t('admin.groupAssociations.lastRun.never')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('admin.groupAssociations.lastRun.never')
  return new Intl.DateTimeFormat(locale.value, {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const runStatusKey = (status: AutoPricingRunResult['status']): string => {
  if (status === 'applied') return 'applied'
  if (status === 'skipped') return 'skipped'
  if (status === 'threshold_exceeded') return 'thresholdExceeded'
  if (status === 'failed') return 'failed'
  return 'unknown'
}

const runTriggerLabel = (trigger: AutoPricingRunResult['trigger']): string => {
  if (trigger === 'manual') return t('admin.groupAssociations.lastRun.triggerManual')
  if (trigger === 'after_sync') return t('admin.groupAssociations.lastRun.triggerAfterSync')
  return t('admin.groupAssociations.lastRun.triggerUnknown')
}

const runReasonLabel = (reason: string | undefined): string => {
  if (!reason) return t('admin.groupAssociations.lastRun.reasonUnknown')
  const key = `admin.groupAssociations.lastRun.reasons.${reason}`
  const label = t(key)
  return label === key ? t('admin.groupAssociations.lastRun.reasonUnknown') : label
}

const getLastRun = (ownGroup: string): AutoPricingRunResult | null => (
  mappings.value.find(m => m.ownGroup === ownGroup)?.lastAutoPricingRun ?? null
)

const runNow = async (ownGroup: string) => {
  const mapping = mappings.value.find(m => m.ownGroup === ownGroup)
  if (!mapping?.enableAutoPricing || mapping.upstreamTargets.length === 0 || runningOwnGroup.value) return

  runningOwnGroup.value = ownGroup
  error.value = null
  try {
    const response = await runAutoPricing({ ownGroup })
    upsertMapping(response.mapping)
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'admin.groupAssociations.runError'
  } finally {
    runningOwnGroup.value = null
  }
}

const loadData = async () => {
  loading.value = true
  error.value = null
  try {
    const [mappingRes, sites, channelSettings] = await Promise.all([
      getMySiteMappingOptions(),
      listUpstreamSites().catch(() => []),
      getNotificationChannelSettings().catch(() => ({ dingtalk: [], feishu: [], telegram: [] })),
    ])
    ownGroups.value = mappingRes.ownGroups ?? []
    mappings.value = mappingRes.mappings ?? []

    const mMap = new Map<string, number>()
    for (const site of sites) {
      if (!site.metrics?.groups) continue
      for (const g of site.metrics.groups) {
        if (g.multiplier != null) {
          mMap.set(`${site.id}::${g.name}`, g.multiplier)
        }
      }
    }
    upstreamMultiplierMap.value = mMap

    const bots: BotOption[] = []
    for (const bot of channelSettings.dingtalk ?? []) {
      if (bot.enabled) bots.push({ id: bot.id, name: bot.name, channel: 'DingTalk' })
    }
    for (const bot of channelSettings.feishu ?? []) {
      if (bot.enabled) bots.push({ id: bot.id, name: bot.name, channel: 'Feishu' })
    }
    for (const bot of channelSettings.telegram ?? []) {
      if (bot.enabled) bots.push({ id: bot.id, name: bot.name, channel: 'Telegram' })
    }
    botOptions.value = bots
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'admin.groupAssociations.loadError'
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  void loadData()
})
</script>

<template>
  <div class="rounded-2xl border border-border/60 bg-card text-card-foreground shadow-sm">
    <div class="flex items-start justify-between gap-4 px-6 pt-6">
      <div class="flex items-center gap-3">
        <div class="flex h-11 w-11 items-center justify-center rounded-full bg-primary/10 text-primary">
          <Layers class="h-5 w-5" />
        </div>
        <div>
          <h2 class="text-lg font-semibold text-foreground">{{ t('admin.groupAssociations.title') }}</h2>
          <p class="text-sm text-muted-foreground">
            {{ t('admin.groupAssociations.subtitle', { count: totalMappings, associated: associatedCount, unassociated: unassociatedCount }) }}
          </p>
        </div>
      </div>
      <button
        type="button"
        :disabled="isSaving"
        class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm font-medium transition-colors bg-primary text-primary-foreground hover:bg-primary/90 disabled:opacity-50"
        @click="saveMappingsData"
      >
        <Loader2 v-if="isSaving" class="h-3.5 w-3.5 animate-spin" />
        <CheckCircle2 v-else-if="showSaveSuccess" class="h-3.5 w-3.5" />
        <Save v-else class="h-3.5 w-3.5" />
        {{ showSaveSuccess ? t('admin.groupAssociations.saveSuccess') : (isSaving ? t('admin.groupAssociations.saving') : t('admin.groupAssociations.save')) }}
      </button>
    </div>

    <div class="px-6 py-6">
      <div v-if="loading" class="flex items-center justify-center py-12">
        <Loader2 class="h-6 w-6 animate-spin text-primary/60" />
      </div>

      <div
        v-else-if="error"
        class="flex flex-col items-center justify-center gap-2 py-12 text-center"
      >
        <p class="text-sm text-muted-foreground">{{ t(error) }}</p>
      </div>

      <div
        v-else-if="mappingRows.length === 0"
        class="flex flex-col items-center justify-center gap-2 py-12 text-center"
      >
        <Layers class="h-8 w-8 text-muted-foreground/40" />
        <p class="text-sm text-muted-foreground">{{ t('admin.groupAssociations.empty') }}</p>
      </div>

      <div v-else class="max-h-[70vh] overflow-y-auto rounded-xl border border-border/60">
        <table class="w-full text-sm">
          <thead class="sticky top-0 z-10 bg-surface/90 backdrop-blur">
            <tr class="border-b border-border/60 text-left text-xs font-medium text-muted-foreground">
              <th class="px-4 py-3">{{ t('admin.groupAssociations.columns.index') }}</th>
              <th class="px-4 py-3">{{ t('admin.groupAssociations.columns.ownGroup') }}</th>
              <th class="px-4 py-3">{{ t('admin.groupAssociations.columns.platform') }}</th>
              <th class="px-4 py-3 min-w-[6rem]">{{ t('admin.groupAssociations.columns.groupType') }}</th>
              <th class="px-4 py-3 min-w-[5rem]">{{ t('admin.groupAssociations.columns.status') }}</th>
              <th class="px-4 py-3 min-w-[6rem]">{{ t('admin.groupAssociations.columns.ownMultiplier') }}</th>
              <th class="px-4 py-3">{{ t('admin.groupAssociations.columns.upstreamGroup') }}</th>
              <th class="px-4 py-3 min-w-[6rem]">{{ t('admin.groupAssociations.columns.upstreamMultiplier') }}</th>
              <th class="px-4 py-3 min-w-[12rem]">
                <span class="inline-flex items-center gap-1">
                  {{ t('admin.groupAssociations.columns.autoPricing') }}
                  <span ref="tipTrigger" @mouseenter="showTip" @mouseleave="hideTip">
                    <CircleHelp class="w-3.5 h-3.5 text-muted-foreground/60 cursor-help" />
                  </span>
                </span>
              </th>
            </tr>
          </thead>
          <tbody
            v-for="(mapping, mappingIndex) in mappingRows"
            :key="mapping.ownGroup"
            class="border-b border-border/40 last:border-b-0"
            :class="{ 'bg-emerald-500/[0.03] dark:bg-emerald-500/[0.04]': getAutoPricingStatus(mapping.ownGroup) === 'enabled' }"
          >
            <!-- 没有对接分组的行：展示自有分组，对接列显示 — -->
            <tr
              v-if="mapping.upstreamTargets.length === 0"
              @mouseenter="setHovered(mappingIndex, -1)"
              @mouseleave="clearHovered"
            >
              <td class="px-4 py-3 align-middle text-muted-foreground border-r border-border/20 transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                {{ mapping.index }}
              </td>
              <td class="px-4 py-3 align-middle font-medium text-foreground border-r border-border/20 transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                {{ mapping.ownGroup }}
              </td>
              <td class="px-4 py-3 align-middle text-foreground border-r border-border/20 transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                {{ mapping.platform || '—' }}
              </td>
              <td class="px-4 py-3 align-middle border-r border-border/20 transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" :class="mapping.isExclusive ? 'bg-orange-500/10 text-orange-600 dark:text-orange-400' : 'bg-green-500/10 text-green-600 dark:text-green-400'">
                  {{ exclusiveLabel(mapping.isExclusive) }}
                </span>
              </td>
              <td class="px-4 py-3 align-middle border-r border-border/20 transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" :class="mapping.status === 'active' ? 'bg-green-500/10 text-green-600 dark:text-green-400' : 'bg-red-500/10 text-red-600 dark:text-red-400'">
                  {{ statusLabel(mapping.status) }}
                </span>
              </td>
              <td class="px-4 py-3 align-middle text-foreground border-r border-border/20 transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                {{ formatMultiplier(mapping.ownMultiplier) }}
              </td>
              <td class="px-4 py-3 align-middle text-muted-foreground transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                {{ t('admin.groupAssociations.unassociatedLabel') }}
              </td>
              <td class="px-4 py-3 align-middle text-muted-foreground transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                {{ t('admin.groupAssociations.unassociatedMultiplier') }}
              </td>
              <td class="px-4 py-3 align-middle transition-colors" :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}">
                <div class="flex flex-col gap-2">
                  <div class="flex flex-wrap items-center gap-2">
                  <span
                    class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
                    :class="{
                      'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400': getAutoPricingStatus(mapping.ownGroup) === 'enabled',
                      'bg-amber-500/10 text-amber-600 dark:text-amber-400': getAutoPricingStatus(mapping.ownGroup) === 'saved_disabled',
                      'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400': getAutoPricingStatus(mapping.ownGroup) === 'not_configured'
                    }"
                  >
                    <Zap v-if="getAutoPricingStatus(mapping.ownGroup) === 'enabled'" class="h-3 w-3" />
                    {{ t(`admin.groupAssociations.autoPricingStatus.${
                      getAutoPricingStatus(mapping.ownGroup) === 'enabled' ? 'enabled'
                      : getAutoPricingStatus(mapping.ownGroup) === 'saved_disabled' ? 'savedDisabled'
                      : 'notConfigured'
                    }`) }}
                  </span>
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-primary transition-colors hover:bg-primary/10"
                    @click="openDrawer(mapping.ownGroup)"
                  >
                    <Settings2 class="h-3 w-3" />
                    {{ getAutoPricingStatus(mapping.ownGroup) === 'not_configured' ? t('admin.groupAssociations.autoPricingActions.configure') : t('admin.groupAssociations.autoPricingActions.edit') }}
                  </button>
                  <button
                    v-if="getAutoPricingStatus(mapping.ownGroup) === 'enabled' && mapping.upstreamTargets.length > 0"
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-primary transition-colors hover:bg-primary/10 disabled:cursor-not-allowed disabled:opacity-50"
                    :disabled="runningOwnGroup !== null"
                    :aria-busy="runningOwnGroup === mapping.ownGroup"
                    :aria-label="t('admin.groupAssociations.autoPricingActions.runNowFor', { group: mapping.ownGroup })"
                    @click="runNow(mapping.ownGroup)"
                  >
                    <Loader2 v-if="runningOwnGroup === mapping.ownGroup" class="h-3 w-3 animate-spin" />
                    <Play v-else class="h-3 w-3" />
                    {{ t('admin.groupAssociations.autoPricingActions.runNow') }}
                  </button>
                  </div>
                  <div class="space-y-0.5 text-xs text-muted-foreground">
                    <div>
                      {{ t('admin.groupAssociations.lastRun.summary', {
                        status: t(`admin.groupAssociations.lastRun.status.${runStatusKey(getLastRun(mapping.ownGroup)?.status)}`),
                        trigger: runTriggerLabel(getLastRun(mapping.ownGroup)?.trigger),
                        time: formatRunTime(getLastRun(mapping.ownGroup)?.ranAt),
                      }) }}
                    </div>
                    <div v-if="getLastRun(mapping.ownGroup)?.reason" class="truncate max-w-[16rem]" :title="runReasonLabel(getLastRun(mapping.ownGroup)?.reason)">
                      {{ t('admin.groupAssociations.lastRun.reason', { reason: runReasonLabel(getLastRun(mapping.ownGroup)?.reason) }) }}
                    </div>
                  </div>
                </div>
              </td>
            </tr>
            <!-- 有对接分组的行：rowspan 模式 -->
            <tr
              v-for="target in mapping.upstreamTargets"
              v-else
              :key="`${mapping.ownGroup}-${target.siteId}-${target.groupName}`"
              class="last:border-b-0"
              :class="{'border-b border-border/20': target.targetIndex !== mapping.upstreamTargets.length - 1}"
              @mouseenter="setHovered(mappingIndex, target.targetIndex)"
              @mouseleave="clearHovered"
            >
              <td
                v-if="target.targetIndex === 0"
                :rowspan="mapping.upstreamTargets.length"
                class="px-4 py-3 align-middle text-muted-foreground border-r border-border/20 transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}"
              >
                {{ mapping.index }}
              </td>
              <td
                v-if="target.targetIndex === 0"
                :rowspan="mapping.upstreamTargets.length"
                class="px-4 py-3 align-middle font-medium text-foreground border-r border-border/20 transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}"
              >
                {{ mapping.ownGroup }}
              </td>
              <td
                v-if="target.targetIndex === 0"
                :rowspan="mapping.upstreamTargets.length"
                class="px-4 py-3 align-middle text-foreground border-r border-border/20 transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}"
              >
                {{ mapping.platform || '—' }}
              </td>
              <td
                v-if="target.targetIndex === 0"
                :rowspan="mapping.upstreamTargets.length"
                class="px-4 py-3 align-middle border-r border-border/20 transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}"
              >
                <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" :class="mapping.isExclusive ? 'bg-orange-500/10 text-orange-600 dark:text-orange-400' : 'bg-green-500/10 text-green-600 dark:text-green-400'">
                  {{ exclusiveLabel(mapping.isExclusive) }}
                </span>
              </td>
              <td
                v-if="target.targetIndex === 0"
                :rowspan="mapping.upstreamTargets.length"
                class="px-4 py-3 align-middle border-r border-border/20 transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}"
              >
                <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" :class="mapping.status === 'active' ? 'bg-green-500/10 text-green-600 dark:text-green-400' : 'bg-red-500/10 text-red-600 dark:text-red-400'">
                  {{ statusLabel(mapping.status) }}
                </span>
              </td>
              <td
                v-if="target.targetIndex === 0"
                :rowspan="mapping.upstreamTargets.length"
                class="px-4 py-3 align-middle text-foreground border-r border-border/20 transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}"
              >
                {{ formatMultiplier(mapping.ownMultiplier) }}
              </td>
              <td
                class="px-4 py-3 align-middle text-foreground transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex && hoveredTargetIndex === target.targetIndex}"
              >
                {{ target.label }}
              </td>
              <td
                class="px-4 py-3 align-middle text-foreground transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex && hoveredTargetIndex === target.targetIndex}"
              >
                {{ formatMultiplier(target.multiplier) }}
              </td>
              <td
                v-if="target.targetIndex === 0"
                :rowspan="mapping.upstreamTargets.length"
                class="px-4 py-3 align-middle transition-colors"
                :class="{'bg-primary/5': hoveredMappingIndex === mappingIndex}"
              >
                <div class="flex flex-col gap-2">
                  <div class="flex flex-wrap items-center gap-2">
                  <span
                    class="inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium"
                    :class="{
                      'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400': getAutoPricingStatus(mapping.ownGroup) === 'enabled',
                      'bg-amber-500/10 text-amber-600 dark:text-amber-400': getAutoPricingStatus(mapping.ownGroup) === 'saved_disabled',
                      'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400': getAutoPricingStatus(mapping.ownGroup) === 'not_configured'
                    }"
                  >
                    <Zap v-if="getAutoPricingStatus(mapping.ownGroup) === 'enabled'" class="h-3 w-3" />
                    {{ t(`admin.groupAssociations.autoPricingStatus.${
                      getAutoPricingStatus(mapping.ownGroup) === 'enabled' ? 'enabled'
                      : getAutoPricingStatus(mapping.ownGroup) === 'saved_disabled' ? 'savedDisabled'
                      : 'notConfigured'
                    }`) }}
                  </span>
                  <button
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-primary transition-colors hover:bg-primary/10"
                    @click="openDrawer(mapping.ownGroup)"
                  >
                    <Settings2 class="h-3 w-3" />
                    {{ getAutoPricingStatus(mapping.ownGroup) === 'not_configured' ? t('admin.groupAssociations.autoPricingActions.configure') : t('admin.groupAssociations.autoPricingActions.edit') }}
                  </button>
                  <button
                    v-if="getAutoPricingStatus(mapping.ownGroup) === 'enabled' && mapping.upstreamTargets.length > 0"
                    type="button"
                    class="inline-flex items-center gap-1 rounded-md px-2 py-1 text-xs font-medium text-primary transition-colors hover:bg-primary/10 disabled:cursor-not-allowed disabled:opacity-50"
                    :disabled="runningOwnGroup !== null"
                    :aria-busy="runningOwnGroup === mapping.ownGroup"
                    :aria-label="t('admin.groupAssociations.autoPricingActions.runNowFor', { group: mapping.ownGroup })"
                    @click="runNow(mapping.ownGroup)"
                  >
                    <Loader2 v-if="runningOwnGroup === mapping.ownGroup" class="h-3 w-3 animate-spin" />
                    <Play v-else class="h-3 w-3" />
                    {{ t('admin.groupAssociations.autoPricingActions.runNow') }}
                  </button>
                  </div>
                  <div class="space-y-0.5 text-xs text-muted-foreground">
                    <div>
                      {{ t('admin.groupAssociations.lastRun.summary', {
                        status: t(`admin.groupAssociations.lastRun.status.${runStatusKey(getLastRun(mapping.ownGroup)?.status)}`),
                        trigger: runTriggerLabel(getLastRun(mapping.ownGroup)?.trigger),
                        time: formatRunTime(getLastRun(mapping.ownGroup)?.ranAt),
                      }) }}
                    </div>
                    <div v-if="getLastRun(mapping.ownGroup)?.reason" class="truncate max-w-[16rem]" :title="runReasonLabel(getLastRun(mapping.ownGroup)?.reason)">
                      {{ t('admin.groupAssociations.lastRun.reason', { reason: runReasonLabel(getLastRun(mapping.ownGroup)?.reason) }) }}
                    </div>
                  </div>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <AutoPricingConfigDrawer
    :open="drawerOpen"
    :mapping="drawerMapping"
    :upstream-multipliers="upstreamMultiplierMap"
    :available-bots="botOptions"
    :saving="savingDrawer"
    @close="drawerOpen = false"
    @save="onDrawerSave"
  />

  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-150 ease-out"
      enter-from-class="opacity-0 scale-95"
      enter-to-class="opacity-100 scale-100"
      leave-active-class="transition duration-100 ease-in"
      leave-from-class="opacity-100 scale-100"
      leave-to-class="opacity-0 scale-95"
    >
      <div
        v-if="tipVisible"
        class="fixed z-[200] px-3 py-2 rounded-lg bg-zinc-900 dark:bg-zinc-800 text-white text-xs font-normal leading-relaxed whitespace-nowrap shadow-xl pointer-events-none"
        :style="tipStyle"
      >
        {{ t('admin.groupAssociations.autoPricingTip') }}
        <span
          v-if="tipPlacement === 'top'"
          class="absolute top-full left-1/2 -translate-x-1/2 -mt-px border-4 border-transparent border-t-zinc-900 dark:border-t-zinc-800"
        />
        <span
          v-else
          class="absolute bottom-full left-1/2 -translate-x-1/2 -mb-px border-4 border-transparent border-b-zinc-900 dark:border-b-zinc-800"
        />
      </div>
    </Transition>
  </Teleport>
</template>
