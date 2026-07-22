<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertTriangle, ArrowDownWideNarrow, ArrowUpWideNarrow, Loader2, RefreshCw, ShoppingCart, X } from 'lucide-vue-next'
import { getUpstreamKeyUsageToday, type UpstreamKeyUsageTodayItem } from '../../api/dashboardAdmin'
import { formatCny } from '../../utils/dashboard'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()

const loading = ref(false)
const error = ref<string | null>(null)
const keys = ref<UpstreamKeyUsageTodayItem[]>([])
const total = ref(0)
const failedSites = ref(0)
const totalSites = ref(0)
// 默认按金额从高到低排序；toggle 后按金额从低到高，金额相同时用 key 名排序，均不触发新的请求。
const sortAsc = ref(false)

const sortedKeys = computed(() => {
  return [...keys.value].sort((a, b) => {
    const diff = sortAsc.value ? a.todayAmount - b.todayAmount : b.todayAmount - a.todayAmount
    if (diff !== 0) return diff
    return a.keyName.localeCompare(b.keyName)
  })
})

const toggleSort = () => {
  sortAsc.value = !sortAsc.value
}

const platformLabel = (platform: string): string => t(`admin.upstream.modal.form.platforms.${platform}`)

const loadErrorKey = (err: unknown): string => {
  if (!(err instanceof Error)) return 'admin.dashboard.upstreamKeyUsage.loadError'
  if (err.message.startsWith('admin.dashboard.upstreamKeyUsage.')) return err.message
  return 'admin.dashboard.upstreamKeyUsage.loadError'
}

// 仅展示掩码后的 key 名称（后端已保证不会返回真实密钥/token），前端不做额外处理。
const loadData = async () => {
  loading.value = true
  error.value = null
  failedSites.value = 0
  totalSites.value = 0
  try {
    const response = await getUpstreamKeyUsageToday()
    keys.value = response.keys ?? []
    total.value = response.total ?? 0
    failedSites.value = response.failedSites ?? 0
    totalSites.value = response.totalSites ?? 0
  } catch (err) {
    error.value = loadErrorKey(err)
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
              <ShoppingCart class="h-5 w-5" />
            </div>
            <div>
              <h2 class="text-lg font-semibold text-foreground">{{ t('admin.dashboard.upstreamKeyUsage.title') }}</h2>
              <p class="text-sm text-muted-foreground">
                {{ t('admin.dashboard.upstreamKeyUsage.subtitle', { count: keys.length, total: formatCny(total) }) }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              :disabled="loading || !!error || keys.length === 0"
              class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground disabled:opacity-50"
              @click="toggleSort"
            >
              <ArrowUpWideNarrow v-if="sortAsc" class="h-3.5 w-3.5" />
              <ArrowDownWideNarrow v-else class="h-3.5 w-3.5" />
              {{ sortAsc ? t('admin.dashboard.upstreamKeyUsage.sort.asc') : t('admin.dashboard.upstreamKeyUsage.sort.desc') }}
            </button>
            <button
              type="button"
              class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
              :title="t('admin.dashboard.upstreamKeyUsage.close')"
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
              {{ t('admin.dashboard.upstreamKeyUsage.retry') }}
            </button>
          </div>

          <div
            v-else-if="failedSites > 0"
            class="mb-4 flex items-start gap-2 rounded-lg border border-warning/30 bg-warning/5 px-3 py-2.5 text-sm text-muted-foreground"
          >
            <AlertTriangle class="mt-0.5 h-4 w-4 shrink-0 text-warning" />
            <span>{{ t('admin.dashboard.upstreamKeyUsage.partialWarning', { failed: failedSites, total: totalSites }) }}</span>
          </div>

          <div
            v-if="!loading && !error && sortedKeys.length === 0"
            class="flex flex-col items-center justify-center gap-2 py-12 text-center"
          >
            <ShoppingCart class="h-8 w-8 text-muted-foreground/40" />
            <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.upstreamKeyUsage.empty') }}</p>
          </div>

          <div v-else-if="!loading && !error" class="max-h-[60vh] overflow-y-auto rounded-xl border border-border/60">
            <table class="w-full text-sm">
              <thead class="sticky top-0 z-10 bg-surface/90 backdrop-blur">
                <tr class="border-b border-border/60 text-left text-xs font-medium text-muted-foreground">
                  <th class="px-4 py-3">{{ t('admin.dashboard.upstreamKeyUsage.columns.siteName') }}</th>
                  <th class="px-4 py-3">{{ t('admin.dashboard.upstreamKeyUsage.columns.keyName') }}</th>
                  <th class="px-4 py-3">{{ t('admin.dashboard.upstreamKeyUsage.columns.groupName') }}</th>
                  <th class="px-4 py-3 text-right">{{ t('admin.dashboard.upstreamKeyUsage.columns.amount') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="item in sortedKeys"
                  :key="`${item.siteId}-${item.keyId}`"
                  class="border-b border-border/40 last:border-b-0"
                >
                  <td class="px-4 py-3 align-middle text-foreground">
                    <div class="font-medium">{{ item.siteName }}</div>
                    <div class="text-xs text-muted-foreground">{{ platformLabel(item.platform) }}</div>
                  </td>
                  <td class="px-4 py-3 align-middle font-medium text-foreground">{{ item.keyName }}</td>
                  <td class="px-4 py-3 align-middle text-muted-foreground">{{ item.groupName }}</td>
                  <td class="px-4 py-3 align-middle text-right text-foreground">{{ formatCny(item.todayAmount) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
