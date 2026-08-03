<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ArrowDownWideNarrow, ArrowUpWideNarrow, CheckCircle2, Landmark, Loader2, RefreshCw, X } from 'lucide-vue-next'
import { getUpstreamBalanceBreakdown, type UpstreamBalanceBreakdownItem } from '../../api/dashboardAdmin'
import { formatCny, formatDateTime } from '../../utils/dashboard'
import type { UpstreamStatus } from '../../types/upstream'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

import { t } from '@/locales'
const loading = ref(false)
const error = ref<string | null>(null)
const sites = ref<UpstreamBalanceBreakdownItem[]>([])
const total = ref(0)
// 默认按余额从高到低排序（未知余额始终排在最后）；toggle 后按余额从低到高，
// 余额相同时用站点名排序，均不触发新的请求。
const sortAsc = ref(false)

const sortedSites = computed(() => {
  return [...sites.value].sort((a, b) => {
    if (a.balance == null || b.balance == null) {
      if (a.balance == null && b.balance == null) return a.siteName.localeCompare(b.siteName)
      return a.balance == null ? 1 : -1
    }
    const diff = sortAsc.value ? a.balance - b.balance : b.balance - a.balance
    if (diff !== 0) return diff
    return a.siteName.localeCompare(b.siteName)
  })
})

const toggleSort = () => {
  sortAsc.value = !sortAsc.value
}

const platformLabel = (platform: string): string => t(`admin.upstream.modal.form.platforms.${platform}`)
const statusLabel = (status: string): string => t(`admin.upstream.status.${status}`)

const statusClasses: Record<UpstreamStatus, string> = {
  connecting: 'bg-primary/10 text-primary border-primary/20',
  syncing: 'bg-warning/10 text-warning border-warning/20',
  connected: 'bg-signal/10 text-signal border-signal/20',
  error: 'bg-warning/10 text-warning border-warning/20',
}

const statusClass = (status: string): string => statusClasses[status as UpstreamStatus] ?? 'bg-muted text-muted-foreground border-border/40'

const loadData = async () => {
  loading.value = true
  error.value = null
  try {
    const response = await getUpstreamBalanceBreakdown()
    sites.value = response.sites ?? []
    total.value = response.total ?? 0
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'admin.dashboard.upstreamBalanceBreakdown.loadError'
  } finally {
    loading.value = false
  }
}

watch(() => props.open, (isOpen) => {
  if (isOpen) {
    void loadData()
  }
})
</script>

<template>
  <Teleport defer to="body">
    <div v-if="open" class="fixed inset-0 z-[100] flex items-center justify-center p-4">
      <div class="absolute inset-0 bg-background/80 backdrop-blur-sm" @click="emit('close')"></div>

      <div
        role="dialog"
        aria-modal="true"
        class="relative w-full max-w-3xl overflow-hidden rounded-[2rem] border border-border/60 bg-card text-card-foreground shadow-2xl shadow-primary/10 animate-in fade-in zoom-in-95 duration-200"
      >
        <div class="absolute left-0 right-0 top-0 h-1 bg-gradient-to-r from-primary via-accent to-primary" />

        <div class="flex items-start justify-between gap-4 px-6 pt-6">
          <div class="flex items-center gap-3">
            <div class="flex h-11 w-11 items-center justify-center rounded-full bg-primary/10 text-primary">
              <Landmark class="h-5 w-5" />
            </div>
            <div>
              <h2 class="text-lg font-semibold text-foreground">{{ t('admin.dashboard.upstreamBalanceBreakdown.title') }}</h2>
              <p class="text-sm text-muted-foreground">
                {{ t('admin.dashboard.upstreamBalanceBreakdown.subtitle', { count: sites.length, total: formatCny(total) }) }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              :disabled="loading || !!error || sites.length === 0"
              class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground disabled:opacity-50"
              @click="toggleSort"
            >
              <ArrowUpWideNarrow v-if="sortAsc" class="h-3.5 w-3.5" />
              <ArrowDownWideNarrow v-else class="h-3.5 w-3.5" />
              {{ sortAsc ? t('admin.dashboard.upstreamBalanceBreakdown.sort.asc') : t('admin.dashboard.upstreamBalanceBreakdown.sort.desc') }}
            </button>
            <button
              type="button"
              class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
              :title="t('admin.dashboard.upstreamBalanceBreakdown.close')"
              @click="emit('close')"
            >
              <X class="h-5 w-5" />
            </button>
          </div>
        </div>

        <div class="px-6 py-6">
          <div v-if="loading" class="flex items-center justify-center py-12">
            <Loader2 class="h-6 w-6 animate-spin text-primary/60" />
          </div>

          <div
            v-else-if="error"
            class="flex flex-col items-center justify-center gap-3 py-12 text-center"
          >
            <p class="text-sm text-muted-foreground">{{ t(error) }}</p>
            <button
              type="button"
              class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
              @click="loadData"
            >
              <RefreshCw class="h-4 w-4" />
              {{ t('admin.dashboard.upstreamBalanceBreakdown.retry') }}
            </button>
          </div>

          <div
            v-else-if="sortedSites.length === 0"
            class="flex flex-col items-center justify-center gap-2 py-12 text-center"
          >
            <Landmark class="h-8 w-8 text-muted-foreground/40" />
            <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.upstreamBalanceBreakdown.empty') }}</p>
          </div>

          <div v-else class="max-h-[60vh] overflow-y-auto rounded-xl border border-border/60">
            <table class="w-full text-sm">
              <thead class="sticky top-0 z-10 bg-surface/90 backdrop-blur">
                <tr class="border-b border-border/60 text-left text-xs font-medium text-muted-foreground">
                  <th class="px-4 py-3">{{ t('admin.dashboard.upstreamBalanceBreakdown.columns.siteName') }}</th>
                  <th class="px-4 py-3">{{ t('admin.dashboard.upstreamBalanceBreakdown.columns.status') }}</th>
                  <th class="px-4 py-3">{{ t('admin.dashboard.upstreamBalanceBreakdown.columns.lastSyncedAt') }}</th>
                  <th class="px-4 py-3 text-right">{{ t('admin.dashboard.upstreamBalanceBreakdown.columns.balance') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="site in sortedSites"
                  :key="site.siteId"
                  class="border-b border-border/40 last:border-b-0"
                >
                  <td class="px-4 py-3 align-middle text-foreground">
                    <div class="font-medium">{{ site.siteName }}</div>
                    <div class="text-xs text-muted-foreground">{{ platformLabel(site.platform) }}</div>
                  </td>
                  <td class="px-4 py-3 align-middle">
                    <span
                      class="inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium"
                      :class="statusClass(site.status)"
                    >
                      <CheckCircle2 v-if="site.status === 'connected'" class="h-3 w-3" />
                      {{ statusLabel(site.status) }}
                    </span>
                  </td>
                  <td class="px-4 py-3 align-middle text-muted-foreground">
                    {{ formatDateTime(site.lastSyncedAt) ?? t('admin.dashboard.upstreamBalanceBreakdown.neverSynced') }}
                  </td>
                  <td class="px-4 py-3 align-middle text-right text-foreground">
                    {{ site.balance != null ? formatCny(site.balance) : t('admin.dashboard.upstreamBalanceBreakdown.unknownBalance') }}
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
