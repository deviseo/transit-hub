<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Search, Plus, CheckCircle2, XCircle, X, Loader2, AlertCircle, Trash2, Edit2, LayoutGrid, List, RefreshCw, Settings2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tooltip } from '@/components/ui/tooltip'
import { getStrategySettings } from '../api/settings'
import { useUpstreamSites } from '../composables/useUpstreamSites'
import SiteSettingsModal from '../components/upstream/SiteSettingsModal.vue'
import type { UpstreamGroupInfo, UpstreamMetricValue, UpstreamSite, UpstreamSiteForm, UpstreamStatus } from '../types/upstream'

const { t } = useI18n()

const searchQuery = ref('')
const isAddModalOpen = ref(false)
const { sites: upstreamSites, isAdding, isRefreshing, addErrorKey, connectedCount, siteSyncStates, syncingSiteIds, addSite, updateSite, deleteSite, streamRefreshSites, refreshSingleSite } = useUpstreamSites()
const deletingSiteId = ref<string | null>(null)
const deleteErrorKey = ref<string | null>(null)
const editingSiteId = ref<string | null>(null)
const refreshIntervalSeconds = ref<number | null>(null)
const remainingSeconds = ref(0)
let countdownTimer: ReturnType<typeof window.setInterval> | null = null
const nextRefreshAtStorageKey = 'transit-hub:upstream-next-refresh-at'

const viewMode = ref<'card' | 'list'>('card')

const countdownDisplay = computed(() => {
  if (!refreshIntervalSeconds.value) return t('admin.upstream.refresh.disabled')
  return t('admin.upstream.refresh.countdown', { seconds: remainingSeconds.value })
})

const readNextRefreshAt = (): number | null => {
  const value = Number.parseInt(window.localStorage.getItem(nextRefreshAtStorageKey) ?? '', 10)
  if (!Number.isFinite(value) || value <= Date.now()) return null
  return value
}

const writeNextRefreshAt = (timestamp: number) => {
  window.localStorage.setItem(nextRefreshAtStorageKey, String(timestamp))
}

const updateRemainingSeconds = () => {
  const nextRefreshAt = readNextRefreshAt()
  remainingSeconds.value = nextRefreshAt ? Math.max(Math.ceil((nextRefreshAt - Date.now()) / 1000), 0) : 0
}

const scheduleNextRefresh = () => {
  if (!refreshIntervalSeconds.value) return
  writeNextRefreshAt(Date.now() + refreshIntervalSeconds.value * 1000)
  updateRemainingSeconds()
}

const runRefresh = async () => {
  if (isRefreshing.value) return
  await streamRefreshSites()
  scheduleNextRefresh()
}

const startCountdown = (seconds: number) => {
  refreshIntervalSeconds.value = seconds
  const nextRefreshAt = readNextRefreshAt()
  if (!nextRefreshAt || nextRefreshAt > Date.now() + seconds * 1000) scheduleNextRefresh()
  updateRemainingSeconds()
  countdownTimer = window.setInterval(() => {
    if (!refreshIntervalSeconds.value || isRefreshing.value) return
    updateRemainingSeconds()
    if (remainingSeconds.value <= 0) void runRefresh()
  }, 1000)
}

const stopCountdown = () => {
  if (countdownTimer) window.clearInterval(countdownTimer)
  countdownTimer = null
}

const loadRefreshSettings = async () => {
  try {
    const settings = await getStrategySettings()
    if (!settings.enableRefreshInterval) return
    startCountdown(Math.max(settings.refreshInterval, 60))
  } catch (error) {
    refreshIntervalSeconds.value = null
  }
}

const createEmptyForm = (): UpstreamSiteForm => ({
  name: '',
  siteUrl: '',
  platform: 'auto',
  authMode: 'password',
  account: '',
  password: '',
  accessToken: '',
  refreshToken: '',
  tokenType: 'Bearer',
  rechargeRate: 1,
  remark: '',
})

const newSiteForm = ref<UpstreamSiteForm>(createEmptyForm())

const handleAddSite = async () => {
  const success = editingSiteId.value
    ? await updateSite(editingSiteId.value, newSiteForm.value)
    : await addSite(newSiteForm.value)
  if (!success) return
  isAddModalOpen.value = false
  newSiteForm.value = createEmptyForm()
  editingSiteId.value = null
}

const handleEditSite = (site: UpstreamSite) => {
  editingSiteId.value = site.id
  newSiteForm.value = {
    name: site.name,
    siteUrl: site.baseUrl,
    platform: site.platform,
    authMode: 'password',
    account: site.account,
    password: '',
    accessToken: '',
    refreshToken: '',
    tokenType: 'Bearer',
    rechargeRate: site.rechargeRate > 0 ? site.rechargeRate : 1,
    remark: site.remark,
  }
  isAddModalOpen.value = true
}

const closeSiteModal = () => {
  isAddModalOpen.value = false
  editingSiteId.value = null
  newSiteForm.value = createEmptyForm()
}

const requestDeleteSite = (id: string) => {
  deletingSiteId.value = id
  deleteErrorKey.value = null
}

const cancelDeleteSite = () => {
  deletingSiteId.value = null
  deleteErrorKey.value = null
}

const confirmDeleteSite = async () => {
  if (!deletingSiteId.value) return
  try {
    await deleteSite(deletingSiteId.value)
    cancelDeleteSite()
  } catch (error) {
    deleteErrorKey.value = error instanceof Error ? error.message : 'admin.upstream.errors.unknown'
  }
}

const filteredSites = computed(() => {
  if (!searchQuery.value) return upstreamSites.value
  return upstreamSites.value.filter(site =>
    site.name.toLowerCase().includes(searchQuery.value.toLowerCase())
    || site.baseUrl.toLowerCase().includes(searchQuery.value.toLowerCase())
  )
})

const statusClasses: Record<UpstreamStatus, string> = {
  connecting: 'bg-primary/10 text-primary border-primary/20',
  syncing: 'bg-warning/10 text-warning border-warning/20',
  connected: 'bg-signal/10 text-signal border-signal/20',
  error: 'bg-warning/10 text-warning border-warning/20',
}

const statusLabel = (status: UpstreamStatus): string => t(`admin.upstream.status.${status}`)

const deletingSite = computed(() => upstreamSites.value.find((site) => site.id === deletingSiteId.value) ?? null)

// Groups Modal Logic
const isGroupsModalOpen = ref(false)
const selectedSiteForGroups = ref<UpstreamSite | null>(null)

const openGroupsModal = (site: UpstreamSite) => {
  selectedSiteForGroups.value = site
  isGroupsModalOpen.value = true
}

const closeGroupsModal = () => {
  isGroupsModalOpen.value = false
  selectedSiteForGroups.value = null
}

const isSiteSettingsOpen = ref(false)
const selectedSiteForSettings = ref<UpstreamSite | null>(null)

const openSiteSettings = (site: UpstreamSite) => {
  selectedSiteForSettings.value = site
  isSiteSettingsOpen.value = true
}

const closeSiteSettings = () => {
  isSiteSettingsOpen.value = false
  selectedSiteForSettings.value = null
}

const onSiteSettingsSaved = (siteId: string, settings: { balanceThreshold: number | null }) => {
  const site = upstreamSites.value.find(s => s.id === siteId)
  if (site) {
    site.settings = settings
  }
}

const groupedGroups = computed<Record<string, UpstreamGroupInfo[]>>(() => {
  if (!selectedSiteForGroups.value) return {}
  const groups = selectedSiteForGroups.value.metrics.groups
  return groups.reduce<Record<string, UpstreamGroupInfo[]>>((acc, group) => {
    const platform = group.platform ?? t('admin.upstream.fields.unknownPlatform')
    if (!acc[platform]) acc[platform] = []
    acc[platform].push(group)
    return acc
  }, {})
})

const cnyMetricDisplay = (site: UpstreamSite, metric: UpstreamMetricValue): string | null => {
  if (metric.value === null || !Number.isFinite(metric.value) || site.rechargeRate <= 0 || !Number.isFinite(site.rechargeRate)) return null
  return t('admin.upstream.currency.cnyValue', { amount: (metric.value * site.rechargeRate).toFixed(2) })
}

const usdMetricDisplay = (metric: UpstreamMetricValue): string => {
  if (metric.display.toUpperCase().includes('USD')) return metric.display
  return t('admin.upstream.currency.usdValue', { amount: metric.display })
}

const lastUpdatedDisplay = (site: UpstreamSite): string => {
  if (!site.lastSyncedAt) return t('admin.upstream.fields.notSynced')
  return new Date(site.lastSyncedAt).toLocaleString()
}

onMounted(() => {
  void loadRefreshSettings()
})

onBeforeUnmount(() => {
  stopCountdown()
})
</script>

<template>
  <div class="space-y-6">
    <!-- Top Action Bar -->
    <div class="flex flex-col sm:flex-row items-start sm:items-center justify-between gap-4">
      <div class="flex flex-col gap-3 w-full sm:w-auto">
        <div class="relative w-full sm:w-80">
          <Search class="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-muted-foreground" />
          <input
            v-model="searchQuery"
            type="text"
            :placeholder="t('admin.upstream.searchPlaceholder')"
            class="w-full h-10 pl-10 pr-4 rounded-xl bg-surface border border-border/50 focus:border-primary focus:ring-1 focus:ring-primary outline-none transition-all text-sm text-foreground placeholder:text-muted-foreground"
          />
        </div>
        <p class="text-xs text-muted-foreground">
          {{ t('admin.upstream.summary', { connected: connectedCount, total: upstreamSites.length }) }}
        </p>
      </div>

      <div class="flex items-center gap-2 w-full sm:w-auto">
        <div class="flex items-center bg-surface border border-border/50 rounded-xl p-1 shrink-0">
          <button
            @click="viewMode = 'list'"
            :class="{'bg-card shadow-sm text-foreground': viewMode === 'list', 'text-muted-foreground hover:text-foreground': viewMode !== 'list'}"
            class="p-1.5 rounded-lg transition-all"
            :title="t('admin.upstream.viewMode.list')"
          >
            <List class="w-4 h-4" />
          </button>
          <button
            @click="viewMode = 'card'"
            :class="{'bg-card shadow-sm text-foreground': viewMode === 'card', 'text-muted-foreground hover:text-foreground': viewMode !== 'card'}"
            class="p-1.5 rounded-lg transition-all"
            :title="t('admin.upstream.viewMode.card')"
          >
            <LayoutGrid class="w-4 h-4" />
          </button>
        </div>
        <div class="hidden md:flex h-10 items-center rounded-xl border border-border/50 bg-surface px-3 text-xs text-muted-foreground whitespace-nowrap">
          {{ countdownDisplay }}
        </div>
        <Button :disabled="isRefreshing" @click="runRefresh" variant="secondary" class="w-full sm:w-auto h-10 rounded-xl px-4 gap-2">
          <Loader2 v-if="isRefreshing" class="w-4 h-4 animate-spin" />
          <RefreshCw v-else class="w-4 h-4" />
          {{ isRefreshing ? t('admin.upstream.refresh.refreshing') : t('admin.upstream.refresh.action') }}
        </Button>
        <Button @click="isAddModalOpen = true" class="w-full sm:w-auto h-10 rounded-xl px-4 gap-2 bg-primary text-primary-foreground hover:bg-primary/90 shadow-sm">
          <Plus class="w-4 h-4" />
          {{ t('admin.upstream.addSite') }}
        </Button>
      </div>
    </div>

    <!-- Cards Grid -->
    <div v-if="viewMode === 'card'" class="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 2xl:grid-cols-4 gap-6">
      <div
        v-for="site in filteredSites"
        :key="site.id"
        class="group relative bg-card border border-border/60 rounded-2xl p-5 hover:border-primary/50 transition-colors shadow-sm hover:shadow-md"
      >
        <!-- Sync Progress Overlay -->
        <div
          v-if="siteSyncStates.get(site.id)?.phase && siteSyncStates.get(site.id)?.phase !== 'idle'"
          class="absolute inset-0 z-10 flex flex-col items-center justify-center rounded-2xl backdrop-blur-sm transition-all"
          :class="{
            'bg-background/60': siteSyncStates.get(site.id)?.phase === 'syncing',
            'bg-signal/10 dark:bg-signal/5': siteSyncStates.get(site.id)?.phase === 'done',
            'bg-destructive/10 dark:bg-destructive/5': siteSyncStates.get(site.id)?.phase === 'error',
          }"
        >
          <template v-if="siteSyncStates.get(site.id)?.phase === 'syncing'">
            <Loader2 class="h-6 w-6 animate-spin text-primary" />
            <span class="mt-2 text-sm font-medium text-foreground">{{ t('admin.upstream.syncStream.syncing') }}</span>
          </template>
          <template v-else-if="siteSyncStates.get(site.id)?.phase === 'done'">
            <CheckCircle2 class="h-6 w-6 text-signal" />
            <span class="mt-2 text-sm font-medium text-signal">{{ t('admin.upstream.syncStream.done') }}</span>
          </template>
          <template v-else-if="siteSyncStates.get(site.id)?.phase === 'error'">
            <XCircle class="h-6 w-6 text-destructive" />
            <span class="mt-2 text-sm font-medium text-destructive">{{ t('admin.upstream.syncStream.error') }}</span>
          </template>
        </div>

        <!-- Card Header -->
        <div class="flex flex-col gap-4 mb-5 border-b border-border/40 pb-4">
          <div class="flex items-start justify-between gap-2">
            <div class="flex items-center gap-3 min-w-0">
              <div :class="['w-10 h-10 rounded-xl flex items-center justify-center font-bold text-lg shrink-0', site.logoBg]">
                {{ site.logo }}
              </div>
              <div class="flex flex-col min-w-0">
                <a :href="site.baseUrl" target="_blank" rel="noopener noreferrer" class="font-semibold text-lg text-foreground hover:text-primary transition-colors cursor-pointer truncate" :title="site.name">
                  {{ site.name }}
                </a>
                <span class="px-2 py-0.5 mt-1 rounded-md bg-primary/10 text-primary border border-primary/20 text-[10px] font-bold uppercase tracking-wider w-fit">
                  {{ t(`admin.upstream.modal.form.platforms.${site.platform}`) }}
                </span>
              </div>
            </div>

            <div
              class="flex items-center gap-1.5 px-2 py-1 rounded-md text-[11px] font-medium border shrink-0"
              :class="statusClasses[site.status]"
            >
              <Loader2 v-if="site.status === 'connecting' || site.status === 'syncing'" class="w-3 h-3 animate-spin" />
              <CheckCircle2 v-else-if="site.status === 'connected'" class="w-3 h-3" />
              <XCircle v-else class="w-3 h-3" />
              {{ statusLabel(site.status) }}
            </div>
          </div>
        </div>

        <!-- Card Body (Stats) -->
        <div class="space-y-4">
          <div class="grid grid-cols-3 gap-3">
            <div class="flex flex-col items-center justify-center p-3 rounded-xl bg-surface/50 border border-border/40">
              <span class="text-xs text-muted-foreground mb-1">{{ t('admin.upstream.fields.balance') }}</span>
              <span v-if="cnyMetricDisplay(site, site.metrics.balance)" class="font-bold text-primary text-sm text-center">
                {{ cnyMetricDisplay(site, site.metrics.balance) }}
              </span>
              <span :class="[cnyMetricDisplay(site, site.metrics.balance) ? 'text-[10px] font-medium text-primary/70 mt-0.5' : 'font-bold text-primary text-sm', 'text-center']">
                {{ usdMetricDisplay(site.metrics.balance) }}
              </span>
            </div>
            <div class="flex flex-col items-center justify-center p-3 rounded-xl bg-surface/50 border border-border/40">
              <span class="text-xs text-muted-foreground mb-1">{{ t('admin.upstream.fields.todayConsume') }}</span>
              <span v-if="cnyMetricDisplay(site, site.metrics.todayConsume)" :class="['font-bold text-sm text-center', site.metrics.todayConsume.value && site.metrics.todayConsume.value > 0 ? 'text-orange-500' : 'text-foreground']">
                {{ cnyMetricDisplay(site, site.metrics.todayConsume) }}
              </span>
              <span :class="[cnyMetricDisplay(site, site.metrics.todayConsume) ? 'text-[10px] font-medium mt-0.5' : 'font-bold text-sm', site.metrics.todayConsume.value && site.metrics.todayConsume.value > 0 ? (cnyMetricDisplay(site, site.metrics.todayConsume) ? 'text-orange-500/70' : 'text-orange-500') : (cnyMetricDisplay(site, site.metrics.todayConsume) ? 'text-muted-foreground' : 'text-foreground'), 'text-center']">
                {{ usdMetricDisplay(site.metrics.todayConsume) }}
              </span>
            </div>
            <div class="flex flex-col items-center justify-center p-3 rounded-xl bg-surface/50 border border-border/40">
              <span class="text-xs text-muted-foreground mb-1">{{ t('admin.upstream.fields.historyRecharge') }}</span>
              <span v-if="cnyMetricDisplay(site, site.metrics.historyRecharge)" class="font-bold text-foreground text-sm text-center">
                {{ cnyMetricDisplay(site, site.metrics.historyRecharge) }}
              </span>
              <span :class="[cnyMetricDisplay(site, site.metrics.historyRecharge) ? 'text-[10px] font-medium text-muted-foreground mt-0.5' : 'font-bold text-foreground text-sm', 'text-center']">
                {{ usdMetricDisplay(site.metrics.historyRecharge) }}
              </span>
            </div>
          </div>

          <Button
            v-if="site.metrics.groups.length > 0"
            variant="secondary"
            class="w-full h-9 text-xs font-medium bg-surface hover:bg-surface-elevated border-border/50 border"
            @click="openGroupsModal(site)"
          >
            {{ t('admin.upstream.fields.viewAvailableGroups') }}
          </Button>

          <!-- Card Actions (Edit/Delete) -->
          <div class="flex items-center justify-between gap-3 pt-4 mt-2 border-t border-border/40">
            <div class="min-w-0 text-left text-[11px] leading-5 text-muted-foreground">
              <span class="block truncate">{{ t('admin.upstream.fields.lastUpdated') }}</span>
              <span class="block truncate font-medium text-foreground/80">{{ lastUpdatedDisplay(site) }}</span>
            </div>
            <div class="flex shrink-0 items-center justify-end gap-2">
              <Tooltip :text="syncingSiteIds.has(site.id) ? t('admin.upstream.action.syncing') : t('admin.upstream.action.sync')">
                <button
                  type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border/60 text-muted-foreground transition-colors hover:border-primary/60 hover:bg-primary/10 hover:text-primary"
                  :disabled="syncingSiteIds.has(site.id)"
                  @click="refreshSingleSite(site.id)"
                >
                  <Loader2 v-if="syncingSiteIds.has(site.id)" class="h-4 w-4 animate-spin" />
                  <RefreshCw v-else class="h-4 w-4" />
                </button>
              </Tooltip>
              <Tooltip :text="t('admin.upstream.action.settings')">
                <button
                  type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border/60 text-muted-foreground transition-colors hover:border-primary/60 hover:bg-primary/10 hover:text-primary"
                  @click="openSiteSettings(site)"
                >
                  <Settings2 class="h-4 w-4" />
                </button>
              </Tooltip>
              <Tooltip :text="t('admin.upstream.action.edit')">
                <button
                  type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border/60 text-muted-foreground transition-colors hover:border-primary/60 hover:bg-primary/10 hover:text-primary"
                  @click="handleEditSite(site)"
                >
                  <Edit2 class="h-4 w-4" />
                </button>
              </Tooltip>
              <Tooltip :text="t('admin.upstream.delete.action')">
                <button
                  type="button"
                  class="inline-flex h-8 w-8 items-center justify-center rounded-lg border border-border/60 text-muted-foreground transition-colors hover:border-red-400/60 hover:bg-red-500/10 hover:text-red-400"
                  @click="requestDeleteSite(site.id)"
                >
                  <Trash2 class="h-4 w-4" />
                </button>
              </Tooltip>
            </div>
          </div>
        </div>

        <div v-if="site.errorKey" class="mt-4 flex items-start gap-2 rounded-xl border border-warning/20 bg-warning/10 px-3 py-2 text-xs text-warning">
          <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{{ t(site.errorKey) }}</span>
        </div>
      </div>
    </div>

    <!-- Table (List) View -->
    <div v-if="viewMode === 'list'" class="rounded-2xl border border-border/60 bg-card overflow-hidden shadow-sm">
      <div class="overflow-x-auto">
        <table class="w-full text-sm text-left">
          <thead class="bg-surface/50 text-muted-foreground border-b border-border/40">
            <tr>
              <th class="px-6 py-4 font-medium">{{ t('admin.upstream.fields.siteName') }}</th>
              <th class="px-6 py-4 font-medium">{{ t('admin.upstream.fields.platform') }}</th>
              <th class="px-6 py-4 font-medium">{{ t('admin.upstream.status.connected') }}</th>
              <th class="px-6 py-4 font-medium">{{ t('admin.upstream.fields.balance') }}</th>
              <th class="px-6 py-4 font-medium">{{ t('admin.upstream.fields.todayConsume') }}</th>
              <th class="px-6 py-4 font-medium">{{ t('admin.upstream.fields.historyRecharge') }}</th>
              <th class="px-6 py-4 font-medium text-right">{{ t('admin.upstream.action.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/40">
            <tr v-for="site in filteredSites" :key="site.id" class="hover:bg-surface/30 transition-colors">
              <td class="px-6 py-4">
                <div class="flex items-center gap-3">
                  <div :class="['w-8 h-8 rounded-lg flex items-center justify-center font-bold text-sm shrink-0', site.logoBg]">
                    {{ site.logo }}
                  </div>
                  <a :href="site.baseUrl" target="_blank" rel="noopener noreferrer" class="font-medium text-foreground hover:text-primary transition-colors truncate max-w-[150px] inline-block">
                    {{ site.name }}
                  </a>
                </div>
              </td>
              <td class="px-6 py-4">
                <span class="px-2 py-1 rounded-md bg-primary/10 text-primary border border-primary/20 text-xs font-semibold uppercase tracking-wider">
                  {{ t(`admin.upstream.modal.form.platforms.${site.platform}`) }}
                </span>
              </td>
              <td class="px-6 py-4">
                <div
                  v-if="siteSyncStates.get(site.id)?.phase && siteSyncStates.get(site.id)?.phase !== 'idle'"
                  class="inline-flex items-center gap-1.5 text-xs font-medium"
                  :class="{
                    'text-primary': siteSyncStates.get(site.id)?.phase === 'syncing',
                    'text-signal': siteSyncStates.get(site.id)?.phase === 'done',
                    'text-destructive': siteSyncStates.get(site.id)?.phase === 'error',
                  }"
                >
                  <Loader2 v-if="siteSyncStates.get(site.id)?.phase === 'syncing'" class="w-3.5 h-3.5 animate-spin" />
                  <CheckCircle2 v-else-if="siteSyncStates.get(site.id)?.phase === 'done'" class="w-3.5 h-3.5" />
                  <XCircle v-else class="w-3.5 h-3.5" />
                  <template v-if="siteSyncStates.get(site.id)?.phase === 'syncing'">{{ t('admin.upstream.syncStream.syncing') }}</template>
                  <template v-else-if="siteSyncStates.get(site.id)?.phase === 'done'">{{ t('admin.upstream.syncStream.done') }}</template>
                  <template v-else>{{ t('admin.upstream.syncStream.error') }}</template>
                </div>
                <div
                  v-else
                  class="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium border"
                  :class="statusClasses[site.status]"
                >
                  <Loader2 v-if="site.status === 'connecting' || site.status === 'syncing'" class="w-3.5 h-3.5 animate-spin" />
                  <CheckCircle2 v-else-if="site.status === 'connected'" class="w-3.5 h-3.5" />
                  <XCircle v-else class="w-3.5 h-3.5" />
                  {{ statusLabel(site.status) }}
                </div>
              </td>
              <td class="px-6 py-4">
                <div class="flex flex-col gap-0.5">
                  <span v-if="cnyMetricDisplay(site, site.metrics.balance)" class="font-medium text-primary">
                    {{ cnyMetricDisplay(site, site.metrics.balance) }}
                  </span>
                  <span :class="[cnyMetricDisplay(site, site.metrics.balance) ? 'text-xs font-medium text-primary/70' : 'font-medium text-primary']">
                    {{ usdMetricDisplay(site.metrics.balance) }}
                  </span>
                </div>
              </td>
              <td class="px-6 py-4">
                <div class="flex flex-col gap-0.5">
                  <span v-if="cnyMetricDisplay(site, site.metrics.todayConsume)" :class="['font-medium', site.metrics.todayConsume.value && site.metrics.todayConsume.value > 0 ? 'text-orange-500' : 'text-muted-foreground']">
                    {{ cnyMetricDisplay(site, site.metrics.todayConsume) }}
                  </span>
                  <span :class="[cnyMetricDisplay(site, site.metrics.todayConsume) ? 'text-xs font-medium' : 'font-medium', site.metrics.todayConsume.value && site.metrics.todayConsume.value > 0 ? (cnyMetricDisplay(site, site.metrics.todayConsume) ? 'text-orange-500/70' : 'text-orange-500') : 'text-muted-foreground']">
                    {{ usdMetricDisplay(site.metrics.todayConsume) }}
                  </span>
                </div>
              </td>
              <td class="px-6 py-4">
                <div class="flex flex-col gap-0.5">
                  <span v-if="cnyMetricDisplay(site, site.metrics.historyRecharge)" class="font-medium text-muted-foreground">
                    {{ cnyMetricDisplay(site, site.metrics.historyRecharge) }}
                  </span>
                  <span :class="[cnyMetricDisplay(site, site.metrics.historyRecharge) ? 'text-xs font-medium text-muted-foreground' : 'text-muted-foreground']">
                    {{ usdMetricDisplay(site.metrics.historyRecharge) }}
                  </span>
                </div>
              </td>
              <td class="px-6 py-4 text-right">
                <div class="flex items-center justify-end gap-2">
                  <Button
                    v-if="site.metrics.groups.length > 0"
                    variant="ghost"
                    class="h-8 px-2 text-xs text-primary hover:text-primary hover:bg-primary/10"
                    @click="openGroupsModal(site)"
                  >
                    {{ t('admin.upstream.fields.availableGroups') }}
                  </Button>
                  <Tooltip :text="syncingSiteIds.has(site.id) ? t('admin.upstream.action.syncing') : t('admin.upstream.action.sync')">
                    <button
                      class="p-1.5 rounded-md text-muted-foreground hover:bg-primary/10 hover:text-primary transition-colors"
                      :disabled="syncingSiteIds.has(site.id)"
                      @click="refreshSingleSite(site.id)"
                    >
                      <Loader2 v-if="syncingSiteIds.has(site.id)" class="w-4 h-4 animate-spin" />
                      <RefreshCw v-else class="w-4 h-4" />
                    </button>
                  </Tooltip>
                  <Tooltip :text="t('admin.upstream.siteSettings.title')">
                    <button
                      class="p-1.5 rounded-md text-muted-foreground hover:bg-primary/10 hover:text-primary transition-colors"
                      @click="openSiteSettings(site)"
                    >
                      <Settings2 class="w-4 h-4" />
                    </button>
                  </Tooltip>
                  <Tooltip :text="t('admin.upstream.action.edit')">
                    <button
                      class="p-1.5 rounded-md text-muted-foreground hover:bg-primary/10 hover:text-primary transition-colors"
                      @click="handleEditSite(site)"
                    >
                      <Edit2 class="w-4 h-4" />
                    </button>
                  </Tooltip>
                  <Tooltip :text="t('admin.upstream.delete.action')">
                    <button
                      class="p-1.5 rounded-md text-muted-foreground hover:bg-red-500/10 hover:text-red-400 transition-colors"
                      @click="requestDeleteSite(site.id)"
                    >
                      <Trash2 class="w-4 h-4" />
                    </button>
                  </Tooltip>
                </div>
              </td>
            </tr>
            <tr v-if="filteredSites.length === 0">
              <td colspan="7" class="px-6 py-12 text-center text-muted-foreground">
                {{ t('admin.upstream.empty.description') }}
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <!-- Empty State -->
    <div v-if="filteredSites.length === 0" class="flex flex-col items-center justify-center py-12 text-center border border-dashed border-border/60 rounded-2xl bg-surface/30">
      <div class="w-12 h-12 rounded-full bg-muted/50 flex items-center justify-center mb-4">
        <Search class="w-6 h-6 text-muted-foreground" />
      </div>
      <p class="text-foreground font-medium">{{ t('admin.upstream.empty.title') }}</p>
      <p class="text-sm text-muted-foreground mt-1">{{ t('admin.upstream.empty.description') }}</p>
    </div>

    <!-- Delete Confirm Modal -->
    <div v-if="deletingSite" class="fixed inset-0 z-50 flex items-center justify-center p-4">
      <div class="absolute inset-0 bg-background/80 backdrop-blur-sm" @click="cancelDeleteSite" />
      <div class="relative w-full max-w-md overflow-hidden rounded-2xl border border-border/70 bg-card p-6 shadow-2xl">
        <div class="absolute inset-x-0 top-0 h-1 bg-gradient-to-r from-red-500 via-warning to-red-500" />
        <div class="flex items-start gap-4">
          <div class="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-red-500/30 bg-red-500/10 text-red-400">
            <Trash2 class="h-5 w-5" />
          </div>
          <div class="min-w-0 flex-1">
            <h3 class="text-lg font-semibold text-foreground">{{ t('admin.upstream.delete.title') }}</h3>
            <p class="mt-2 text-sm leading-6 text-muted-foreground">
              {{ t('admin.upstream.delete.description', { name: deletingSite.name }) }}
            </p>
          </div>
        </div>

        <div v-if="deleteErrorKey" class="mt-5 flex items-start gap-2 rounded-xl border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ t(deleteErrorKey) }}</span>
        </div>

        <div class="mt-6 flex flex-col-reverse gap-3 sm:flex-row sm:justify-end">
          <Button type="button" variant="secondary" @click="cancelDeleteSite">
            {{ t('admin.upstream.delete.cancel') }}
          </Button>
          <Button type="button" class="bg-red-500 text-white hover:bg-red-400" @click="confirmDeleteSite">
            {{ t('admin.upstream.delete.confirm') }}
          </Button>
        </div>
      </div>
    </div>

    <!-- Groups Modal -->
    <Teleport defer to="body">
      <div v-if="isGroupsModalOpen" class="fixed inset-0 z-[100] flex items-center justify-center p-4 sm:p-0">
        <!-- Backdrop -->
        <div
          class="absolute inset-0 bg-background/80 backdrop-blur-sm"
          @click="closeGroupsModal"
        ></div>

        <!-- Modal Content -->
        <div class="relative bg-card border border-border/60 rounded-[2rem] shadow-2xl w-full max-w-2xl overflow-hidden animate-in fade-in zoom-in-95 duration-200">
          <div class="absolute top-0 left-0 right-0 h-1 bg-gradient-to-r from-primary via-accent to-primary" />

          <div class="flex items-center justify-between px-6 py-5 border-b border-border/40">
            <h3 class="text-lg font-semibold text-foreground">
              {{ t('admin.upstream.fields.availableGroups') }}
              <span class="text-muted-foreground ml-2 text-sm font-medium">{{ selectedSiteForGroups?.name }}</span>
            </h3>
            <button @click="closeGroupsModal" class="text-muted-foreground hover:text-foreground transition-colors p-1 rounded-md hover:bg-surface-elevated">
              <X class="w-5 h-5" />
            </button>
          </div>

          <div class="p-6 max-h-[60vh] overflow-y-auto space-y-6">
            <div v-for="(groups, platform) in groupedGroups" :key="platform" class="space-y-3">
              <h4 class="text-sm font-semibold text-muted-foreground uppercase tracking-wider flex items-center gap-2">
                <div class="w-1.5 h-1.5 rounded-full bg-primary"></div>
                {{ platform }}
              </h4>
              <div class="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-3">
                <button
                  v-for="group in groups"
                  :key="group.name"
                  class="flex flex-col items-center justify-center p-3 rounded-xl border border-border/60 bg-surface/50 hover:bg-surface hover:border-primary/50 transition-colors text-center group"
                >
                  <span class="text-sm font-medium text-foreground truncate w-full group-hover:text-primary transition-colors">{{ group.name }}</span>
                  <span
                    v-if="group.multiplier !== null && selectedSiteForGroups && selectedSiteForGroups.rechargeRate > 0"
                    class="mt-2 text-xs font-semibold text-primary px-2 py-0.5 rounded-md bg-primary/10 border border-primary/20"
                  >
                    {{ (group.multiplier * selectedSiteForGroups.rechargeRate).toFixed(2) }}
                  </span>
                  <template v-if="group.hasDedicatedMultiplier">
                    <Tooltip :text="t('admin.upstream.fields.dedicatedMultiplierTooltip')" wide>
                      <span class="text-[10px] text-muted-foreground mt-1">
                        {{ group.defaultMultiplierDisplay }} -&gt; {{ group.dedicatedMultiplierDisplay }}
                      </span>
                    </Tooltip>
                    <span class="mt-1 text-[9px] font-semibold text-accent px-1.5 py-0.5 rounded bg-accent/10 border border-accent/20">
                      {{ t('admin.upstream.fields.dedicatedMultiplierBadge') }}
                    </span>
                  </template>
                  <span v-else class="text-[10px] text-muted-foreground mt-1">
                    {{ group.multiplierDisplay }}
                  </span>
                </button>
              </div>
            </div>
          </div>

          <div class="p-4 border-t border-border/40 flex justify-end">
             <Button variant="ghost" @click="closeGroupsModal">{{ t('admin.upstream.fields.closeGroupsModal') }}</Button>
          </div>
        </div>
      </div>
    </Teleport>

    <!-- Add Site Modal -->
    <Teleport defer to="body">
      <div v-if="isAddModalOpen" class="fixed inset-0 z-[100] flex items-center justify-center p-4 sm:p-0">
        <!-- Backdrop -->
        <div
          class="absolute inset-0 bg-background/80 backdrop-blur-sm"
          @click="closeSiteModal"
        ></div>

        <!-- Modal Content -->
        <div class="relative bg-card border border-border/60 rounded-[2rem] shadow-2xl w-full max-w-2xl overflow-hidden animate-in fade-in zoom-in-95 duration-200">
          <div class="absolute top-0 left-0 right-0 h-1 bg-gradient-to-r from-primary via-accent to-primary" />

          <div class="flex items-center justify-between px-6 py-5 border-b border-border/40">
            <h3 class="text-lg font-semibold text-foreground">
              {{ t(editingSiteId ? 'admin.upstream.modal.editTitle' : 'admin.upstream.modal.title') }}
            </h3>
            <button @click="closeSiteModal" class="text-muted-foreground hover:text-foreground transition-colors p-1 rounded-md hover:bg-surface-elevated">
              <X class="w-5 h-5" />
            </button>
          </div>

          <form @submit.prevent="handleAddSite" class="p-6">
            <div v-if="addErrorKey" class="flex items-start gap-2 rounded-xl border border-warning/20 bg-warning/10 px-3 py-2 text-sm text-warning mb-5">
              <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
              <span>{{ t(addErrorKey) }}</span>
            </div>

            <div class="grid grid-cols-1 sm:grid-cols-2 gap-5">
              <!-- Site Name -->
              <div class="space-y-2">
                <label class="text-sm font-medium text-foreground flex items-center gap-1">
                  <span class="text-red-500">*</span>
                  {{ t('admin.upstream.modal.form.siteName') }}
                </label>
                <Input
                  v-model="newSiteForm.name"
                  :placeholder="t('admin.upstream.modal.form.siteNamePlaceholder')"
                  :disabled="isAdding"
                  required
                  class="bg-surface border-border/50 focus:border-primary h-10"
                />
              </div>

              <!-- Platform Select -->
              <div class="space-y-2">
                <label class="text-sm font-medium text-foreground flex items-center gap-1">
                  <span class="text-red-500">*</span>
                  {{ t('admin.upstream.modal.form.platform') }}
                </label>
                <div class="relative">
                  <select
                    v-model="newSiteForm.platform"
                    :disabled="isAdding"
                    class="w-full h-10 px-3 rounded-xl bg-surface border border-border/50 focus:border-primary focus:ring-1 focus:ring-primary outline-none appearance-none text-sm text-foreground transition-all"
                  >
                    <option value="auto">{{ t('admin.upstream.modal.form.platforms.auto') }}</option>
                    <option value="sub2api">{{ t('admin.upstream.modal.form.platforms.sub2api') }}</option>
                    <option value="newapi">{{ t('admin.upstream.modal.form.platforms.newapi') }}</option>
                  </select>
                  <!-- Custom arrow since we removed appearance -->
                  <div class="absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none text-muted-foreground">
                    <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>
                  </div>
                </div>
              </div>

              <!-- Site URL -->
              <div class="space-y-2 sm:col-span-2">
                <label class="text-sm font-medium text-foreground flex items-center gap-1">
                  <span class="text-red-500">*</span>
                  {{ t('admin.upstream.modal.form.siteUrl') }}
                </label>
                <Input
                  v-model="newSiteForm.siteUrl"
                  type="url"
                  :placeholder="t('admin.upstream.modal.form.siteUrlPlaceholder')"
                  :disabled="isAdding"
                  required
                  class="bg-surface border-border/50 focus:border-primary h-10"
                />
              </div>

              <!-- Auth Mode -->
              <div class="space-y-2 sm:col-span-2">
                <label class="text-sm font-medium text-foreground flex items-center gap-1">
                  <span class="text-red-500">*</span>
                  {{ t('admin.upstream.modal.form.authMode') }}
                </label>
                <div class="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  <label class="flex cursor-pointer items-start gap-3 rounded-xl border border-border/50 bg-surface p-3 text-sm transition-colors hover:border-primary/50">
                    <input v-model="newSiteForm.authMode" type="radio" value="password" :disabled="isAdding" class="mt-1" />
                    <span class="space-y-1">
                      <span class="block font-medium text-foreground">{{ t('admin.upstream.modal.form.authModes.password') }}</span>
                      <span class="block text-xs leading-5 text-muted-foreground">{{ t('admin.upstream.modal.form.authModes.passwordHelp') }}</span>
                    </span>
                  </label>
                  <label class="flex cursor-pointer items-start gap-3 rounded-xl border border-border/50 bg-surface p-3 text-sm transition-colors hover:border-primary/50">
                    <input v-model="newSiteForm.authMode" type="radio" value="token" :disabled="isAdding" class="mt-1" />
                    <span class="space-y-1">
                      <span class="block font-medium text-foreground">{{ t('admin.upstream.modal.form.authModes.token') }}</span>
                      <span class="block text-xs leading-5 text-muted-foreground">{{ t('admin.upstream.modal.form.authModes.tokenHelp') }}</span>
                    </span>
                  </label>
                </div>
              </div>

              <!-- Account -->
              <div v-if="newSiteForm.authMode === 'password'" class="space-y-2">
                <label class="text-sm font-medium text-foreground flex items-center gap-1">
                  <span class="text-red-500">*</span>
                  {{ t('admin.upstream.modal.form.account') }}
                </label>
                <Input
                  v-model="newSiteForm.account"
                  :placeholder="t('admin.upstream.modal.form.accountPlaceholder')"
                  :disabled="isAdding"
                  required
                  class="bg-surface border-border/50 focus:border-primary h-10"
                />
              </div>

              <!-- Password -->
              <div v-if="newSiteForm.authMode === 'password'" class="space-y-2">
                <label class="text-sm font-medium text-foreground flex items-center gap-1">
                  <span v-if="!editingSiteId" class="text-red-500">*</span>
                  {{ t('admin.upstream.modal.form.password') }}
                </label>
                <Input
                  v-model="newSiteForm.password"
                  type="password"
                  :placeholder="t(editingSiteId ? 'admin.upstream.modal.form.passwordEditPlaceholder' : 'admin.upstream.modal.form.passwordPlaceholder')"
                  :disabled="isAdding"
                  :required="!editingSiteId"
                  class="bg-surface border-border/50 focus:border-primary h-10"
                />
                <p v-if="editingSiteId" class="text-xs leading-5 text-muted-foreground">
                  {{ t('admin.upstream.modal.form.passwordEditHelp') }}
                </p>
              </div>

              <template v-else>
                <div class="space-y-2 sm:col-span-2">
                  <label class="text-sm font-medium text-foreground flex items-center gap-1">
                    {{ t('admin.upstream.modal.form.accessToken') }}
                  </label>
                  <Input
                    v-model="newSiteForm.accessToken"
                    :placeholder="t('admin.upstream.modal.form.accessTokenPlaceholder')"
                    :disabled="isAdding"
                    class="bg-surface border-border/50 focus:border-primary h-10"
                  />
                </div>
                <div class="space-y-2">
                  <label class="text-sm font-medium text-foreground flex items-center gap-1">
                    {{ t('admin.upstream.modal.form.refreshToken') }}
                  </label>
                  <Input
                    v-model="newSiteForm.refreshToken"
                    :placeholder="t('admin.upstream.modal.form.refreshTokenPlaceholder')"
                    :disabled="isAdding"
                    class="bg-surface border-border/50 focus:border-primary h-10"
                  />
                </div>
                <div class="space-y-2">
                  <label class="text-sm font-medium text-foreground flex items-center gap-1">
                    {{ t('admin.upstream.modal.form.tokenType') }}
                  </label>
                  <Input
                    v-model="newSiteForm.tokenType"
                    :placeholder="t('admin.upstream.modal.form.tokenTypePlaceholder')"
                    :disabled="isAdding"
                    class="bg-surface border-border/50 focus:border-primary h-10"
                  />
                  <p class="text-xs leading-5 text-muted-foreground">
                    {{ t('admin.upstream.modal.form.tokenHelp') }}
                  </p>
                </div>
              </template>

              <!-- Recharge Rate -->
              <div class="space-y-2">
                <label class="text-sm font-medium text-foreground flex items-center gap-1">
                  <span class="text-red-500">*</span>
                  {{ t('admin.upstream.modal.form.rechargeRate') }}
                </label>
                <input
                  v-model.number="newSiteForm.rechargeRate"
                  type="number"
                  min="0.000001"
                  step="0.000001"
                  :placeholder="t('admin.upstream.modal.form.rechargeRatePlaceholder')"
                  :disabled="isAdding"
                  required
                  class="w-full h-10 px-3 rounded-xl bg-surface border border-border/50 focus:border-primary focus:ring-1 focus:ring-primary outline-none text-sm text-foreground placeholder:text-muted-foreground transition-all disabled:cursor-not-allowed disabled:opacity-50"
                />
                <p class="text-xs text-muted-foreground">
                  {{ t('admin.upstream.modal.form.rechargeRateHelp') }}
                </p>
              </div>

              <!-- Remark -->
              <div class="space-y-2">
                <label class="text-sm font-medium text-foreground ml-2.5">
                  {{ t('admin.upstream.modal.form.remark') }}
                </label>
                <Input
                  v-model="newSiteForm.remark"
                  :placeholder="t('admin.upstream.modal.form.remarkPlaceholder')"
                  :disabled="isAdding"
                  class="bg-surface border-border/50 focus:border-primary h-10"
                />
              </div>
            </div>

            <!-- Actions -->
            <div class="flex items-center justify-end gap-3 pt-4 border-t border-border/40 mt-6">
              <Button type="button" variant="ghost" :disabled="isAdding" @click="isAddModalOpen = false" class="hover:bg-surface-line">
                {{ t('admin.upstream.modal.cancel') }}
              </Button>
              <Button type="submit" :disabled="isAdding" class="bg-primary text-primary-foreground hover:bg-primary/90">
                <Loader2 v-if="isAdding" class="h-4 w-4 animate-spin" />
              {{ isAdding ? t('admin.upstream.modal.submitting') : t(editingSiteId ? 'admin.upstream.modal.updateSubmit' : 'admin.upstream.modal.submit') }}
            </Button>
            </div>
          </form>
        </div>
      </div>
    </Teleport>

    <SiteSettingsModal
      :open="isSiteSettingsOpen"
      :site="selectedSiteForSettings"
      @close="closeSiteSettings"
      @saved="onSiteSettingsSaved"
    />
  </div>
</template>
