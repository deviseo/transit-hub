<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, ArrowUpDown, ChevronDown, History, Loader2, RefreshCw, Search, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { useGroupRates } from '../composables/useGroupRates'
import type { GroupRate, GroupRateHistoryRow, GroupRateSort, GroupRateStatusFilter } from '../types/groupRates'

// 分组倍率：只读列表。展示上游站点各分组的实际倍率（上游倍率 × 充值倍率）及其涨跌，点击涨跌可查看倍率历史。

const { t, locale } = useI18n()

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
  isLoading,
  isHistoryLoading,
  errorKey,
  historyErrorKey,
  loadRates,
  loadHistory,
  setSearch,
  setTypeFilter,
  setPlatformFilter,
  setStatusFilter,
  setSortMode,
  goToPage,
} = useGroupRates()

const selectedRate = ref<GroupRate | null>(null)
const isHistoryOpen = ref(false)
const searchQuery = ref('')
let previouslyFocusedElement: HTMLElement | null = null
let previousBodyOverflow = ''
let searchDebounceTimer: ReturnType<typeof setTimeout> | null = null

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
    closeHistory()
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

watch(isHistoryOpen, async (open) => {
  if (open) {
    previouslyFocusedElement = document.activeElement as HTMLElement | null
    previousBodyOverflow = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    await nextTick()
    document.querySelector<HTMLElement>('[data-group-rates-dialog] button:not([disabled])')?.focus()
    return
  }
  document.body.style.overflow = previousBodyOverflow
  previouslyFocusedElement?.focus()
  previouslyFocusedElement = null
})

onMounted(() => {
  document.addEventListener('keydown', handleDialogKeydown)
  void loadRates()
})

onBeforeUnmount(() => {
  if (searchDebounceTimer) clearTimeout(searchDebounceTimer)
  document.removeEventListener('keydown', handleDialogKeydown)
  document.body.style.overflow = previousBodyOverflow
})

const handleTypeChange = async (event: Event) => {
  await setTypeFilter((event.target as HTMLSelectElement).value)
}

const handlePlatformChange = async (event: Event) => {
  await setPlatformFilter((event.target as HTMLSelectElement).value)
}

const handleStatusChange = async (status: GroupRateStatusFilter) => {
  if (status === statusFilter.value || isLoading.value) return
  await setStatusFilter(status)
}

const handleSortChange = async (event: Event) => {
  await setSortMode((event.target as HTMLSelectElement).value as GroupRateSort)
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

const historyTitle = computed(() => {
  if (!selectedRate.value) return t('admin.groupRates.history.title')
  return t('admin.groupRates.history.titleWithGroup', {
    site: selectedRate.value.siteName,
    group: selectedRate.value.groupName,
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
        v-for="tab in (['all', 'deleted'] as const)"
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

      <div class="w-full shrink-0 sm:w-auto">
        <Button class="h-10 w-full gap-2 shadow-sm sm:w-auto" :disabled="isLoading" @click="loadRates">
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

      <div v-else-if="rates.length === 0" class="flex flex-1 flex-col items-center justify-center px-6 text-center">
        <div class="flex h-12 w-12 items-center justify-center rounded-2xl border border-border/50 bg-surface-elevated text-muted-foreground">
          <History class="h-5 w-5" />
        </div>
        <h3 class="mt-4 font-semibold text-foreground">{{ t(hasActiveRateFilters || hasAnyRateData ? 'admin.groupRates.empty.filteredTitle' : 'admin.groupRates.empty.title') }}</h3>
        <p class="mt-2 max-w-sm text-sm text-muted-foreground">{{ t(hasActiveRateFilters || hasAnyRateData ? 'admin.groupRates.empty.filteredDescription' : 'admin.groupRates.empty.description') }}</p>
      </div>

      <div v-else class="flex-1 overflow-auto">
        <table class="w-full min-w-[880px] text-left text-sm relative">
          <thead class="sticky top-0 z-10 border-b border-border/50 bg-surface-elevated/90 backdrop-blur-sm">
            <tr>
              <th class="px-4 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.siteName') }}</th>
              <th class="px-4 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.groupName') }}</th>
              <th class="px-4 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.type') }}</th>
              <th class="px-4 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.platform') }}</th>
              <th class="px-4 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.effectiveMultiplier') }}</th>
              <th class="px-4 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.delta') }}</th>
              <th class="px-4 py-3 font-medium text-muted-foreground">{{ t('admin.groupRates.fields.updatedAt') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/50">
            <tr v-for="rate in rates" :key="`${rate.siteId}-${rate.groupName}-${rate.platform ?? 'all'}`" class="transition-colors hover:bg-surface/30">
              <td class="px-4 py-2.5">
                <div class="font-medium text-foreground">{{ rate.siteName }}</div>
              </td>
              <td class="px-4 py-2.5">
                <div class="flex items-center gap-1.5">
                  <span class="font-medium text-foreground">{{ rate.groupName }}</span>
                  <span v-if="rate.deleted" class="inline-flex rounded-md border border-red-500/20 bg-red-500/10 px-1.5 py-0.5 text-[10px] font-semibold text-red-500">{{ t('admin.groupRates.status.deleted') }}</span>
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
  </div>
</template>
