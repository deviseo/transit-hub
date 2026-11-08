<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, ChevronLeft, ChevronRight, Loader2, RotateCcw, Search, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { excludeSalesEntries, getSalesEntries, restoreSalesEntries } from '../../api/sales'
import { useUserActivityFormat } from '../../composables/useUserActivityFormat'
import { formatCny } from '../../utils/dashboard'
import type { SalesEntriesResponse, SalesEntry } from '../../types/sales'

// 入账明细：某个用户某天某个渠道一行。点「不计入」后这笔入账不算进销售分析（只影响 TransitHub 的统计，
// 不改站点上的数据），随时可以恢复。

const props = defineProps<{
  from: string
  to: string
  /** 打开时先筛选这个渠道（空为全部）。 */
  channel?: string
  /** 打开时只看不计入的。 */
  excludedOnly?: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
  /** 有明细排除或恢复了：页面上的数字要重新算。 */
  (event: 'changed'): void
}>()

const { t, te } = useI18n()
const { formatCount, formatDay, formatRange } = useUserActivityFormat()

// 可以选择不计入的渠道（正向入账），顺序和页面上的渠道一致。
const CHANNELS = ['payment', 'credit', 'redeem', 'admin_add', 'affiliate']

const channel = ref(props.channel ?? '')
const search = ref('')
const excludedOnly = ref(props.excludedOnly ?? false)
const page = ref(1)
const result = ref<SalesEntriesResponse | null>(null)
const isLoading = ref(false)
const errorKey = ref('')
const busyKey = ref('')
let requestSequence = 0
let searchTimer: number | undefined

const channelLabel = (value: string): string => {
  const key = `admin.userActivity.money.channels.${value}`
  return te(key) ? t(key) : value
}
const errorText = (key: string): string => (te(key) ? t(key) : t('admin.sales.errors.request'))
const rowKey = (row: SalesEntry): string => `${row.siteUserId}|${row.day}|${row.channel}`
const userName = (row: SalesEntry): string => row.email || row.username || `#${row.siteUserId}`
const fullyExcluded = (row: SalesEntry): boolean => row.excludedAmount > 0 && row.excludedAmount >= row.amount - 0.005
const partlyExcluded = (row: SalesEntry): boolean => row.excludedAmount > 0 && !fullyExcluded(row)

const load = async () => {
  const sequence = ++requestSequence
  isLoading.value = true
  errorKey.value = ''
  try {
    const response = await getSalesEntries({
      from: props.from,
      to: props.to,
      channel: channel.value,
      q: search.value.trim(),
      excluded: excludedOnly.value,
      page: page.value,
    })
    if (sequence === requestSequence) result.value = response
  } catch (error) {
    if (sequence === requestSequence) errorKey.value = error instanceof Error ? error.message : 'admin.sales.errors.request'
  } finally {
    if (sequence === requestSequence) isLoading.value = false
  }
}

const pages = computed(() => (result.value ? Math.max(1, Math.ceil(result.value.total / result.value.pageSize)) : 1))
const channelExcluded = computed(() => Boolean(channel.value && result.value?.excludedChannels.includes(channel.value)))

watch([channel, excludedOnly], () => {
  page.value = 1
  void load()
})
watch(search, () => {
  window.clearTimeout(searchTimer)
  searchTimer = window.setTimeout(() => {
    page.value = 1
    void load()
  }, 300)
})
watch(page, () => { void load() })

const toggle = async (row: SalesEntry, exclude: boolean) => {
  const key = rowKey(row)
  busyKey.value = key
  errorKey.value = ''
  try {
    const entry = { siteUserId: row.siteUserId, day: row.day, channel: row.channel }
    await (exclude ? excludeSalesEntries([entry]) : restoreSalesEntries([entry]))
    emit('changed')
    await load()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.sales.errors.request'
  } finally {
    busyKey.value = ''
  }
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') emit('close')
}

onMounted(() => {
  void load()
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  window.clearTimeout(searchTimer)
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <div class="fixed inset-0 z-50 flex justify-end bg-background/70 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="sales-entries-title" @click.self="emit('close')">
    <aside class="flex h-full w-full max-w-3xl flex-col border-l border-border/60 bg-card shadow-xl">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <div class="min-w-0">
          <h2 id="sales-entries-title" class="text-lg font-semibold text-foreground">{{ t('admin.sales.entries.title') }}</h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.sales.entries.subtitle', { range: formatRange(from, to) }) }}</p>
        </div>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.sales.entries.close')" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="flex shrink-0 flex-wrap items-center gap-2 border-b border-border/50 px-4 py-3">
        <label class="sr-only" for="sales-entries-channel">{{ t('admin.sales.entries.channel') }}</label>
        <select
          id="sales-entries-channel"
          v-model="channel"
          class="h-8 rounded-lg border border-border/60 bg-surface px-2 text-xs text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
        >
          <option value="">{{ t('admin.sales.entries.allChannels') }}</option>
          <option v-for="item in CHANNELS" :key="item" :value="item">{{ channelLabel(item) }}</option>
        </select>
        <div class="relative min-w-[12rem] flex-1">
          <Search class="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
          <label class="sr-only" for="sales-entries-search">{{ t('admin.sales.entries.search') }}</label>
          <input
            id="sales-entries-search"
            v-model="search"
            type="search"
            :placeholder="t('admin.sales.entries.search')"
            class="h-8 w-full rounded-lg border border-border/60 bg-surface pl-7 pr-2 text-xs text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
          >
        </div>
        <label class="inline-flex items-center gap-1.5 text-xs text-foreground">
          <input v-model="excludedOnly" type="checkbox" class="h-3.5 w-3.5 accent-primary">
          {{ t('admin.sales.entries.excludedOnly') }}
        </label>
      </div>

      <div class="min-h-0 flex-1 overflow-y-auto">
        <div v-if="channelExcluded" class="m-4 rounded-lg border border-border/60 bg-surface p-3 text-xs text-muted-foreground" role="status">
          {{ t('admin.sales.entries.channelExcluded', { channel: channelLabel(channel) }) }}
        </div>
        <div v-if="errorKey" class="m-4 flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
        <div v-if="isLoading && !result" class="flex items-center justify-center py-16 text-muted-foreground" role="status">
          <Loader2 class="h-5 w-5 animate-spin" />
        </div>
        <p v-else-if="result && result.items.length === 0" class="py-16 text-center text-sm text-muted-foreground">
          {{ excludedOnly ? t('admin.sales.entries.emptyExcluded') : t('admin.sales.entries.empty') }}
        </p>
        <table v-else-if="result" class="w-full text-left text-sm" :class="{ 'opacity-60 transition-opacity': isLoading }">
          <thead class="sticky top-0 bg-surface-elevated text-xs text-muted-foreground">
            <tr>
              <th class="px-4 py-2 font-medium">{{ t('admin.sales.entries.day') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('admin.sales.entries.user') }}</th>
              <th class="px-4 py-2 font-medium">{{ t('admin.sales.entries.channel') }}</th>
              <th class="px-4 py-2 text-right font-medium">{{ t('admin.sales.entries.amount') }}</th>
              <th class="px-4 py-2 text-right font-medium"><span class="sr-only">{{ t('admin.sales.entries.action') }}</span></th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/50 tabular-nums">
            <tr v-for="row in result.items" :key="rowKey(row)" :class="fullyExcluded(row) ? 'bg-surface/50' : ''">
              <td class="whitespace-nowrap px-4 py-2 text-muted-foreground">{{ formatDay(row.day) }}</td>
              <td class="max-w-[14rem] px-4 py-2">
                <span class="block truncate text-foreground" :class="{ 'text-muted-foreground': fullyExcluded(row) }">{{ userName(row) }}</span>
                <span v-if="row.email && row.username" class="block truncate text-xs text-muted-foreground">{{ row.username }}</span>
              </td>
              <td class="whitespace-nowrap px-4 py-2 text-muted-foreground">{{ channelLabel(row.channel) }}</td>
              <td class="whitespace-nowrap px-4 py-2 text-right">
                <span class="font-medium" :class="fullyExcluded(row) ? 'text-muted-foreground line-through' : 'text-foreground'">{{ formatCny(row.amount) }}</span>
                <span class="block text-xs text-muted-foreground">
                  <template v-if="partlyExcluded(row)">{{ t('admin.sales.entries.partly', { amount: formatCny(row.excludedAmount) }) }}</template>
                  <template v-else>{{ t('admin.sales.entries.events', { count: formatCount(row.events) }) }}</template>
                </span>
              </td>
              <td class="whitespace-nowrap px-4 py-2 text-right">
                <div class="flex items-center justify-end gap-1.5">
                  <span v-if="fullyExcluded(row)" class="rounded-md bg-muted px-1.5 py-0.5 text-[11px] text-muted-foreground">{{ t('admin.sales.entries.notCounted') }}</span>
                  <Button
                    v-if="!fullyExcluded(row)"
                    variant="secondary"
                    size="sm"
                    class="h-7 rounded-lg px-2.5 text-xs"
                    :disabled="busyKey === rowKey(row)"
                    @click="toggle(row, true)"
                  >
                    <Loader2 v-if="busyKey === rowKey(row)" class="h-3.5 w-3.5 animate-spin" />
                    {{ partlyExcluded(row) ? t('admin.sales.entries.excludeRest') : t('admin.sales.entries.exclude') }}
                  </Button>
                  <Button
                    v-if="row.excludedAmount > 0"
                    variant="ghost"
                    size="sm"
                    class="h-7 rounded-lg px-2.5 text-xs"
                    :disabled="busyKey === rowKey(row)"
                    @click="toggle(row, false)"
                  >
                    <RotateCcw class="h-3.5 w-3.5" />
                    {{ t('admin.sales.entries.restore') }}
                  </Button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="result && result.total > 0" class="flex shrink-0 items-center justify-between gap-3 border-t border-border/50 px-4 py-3 text-xs text-muted-foreground">
        <span>{{ t('admin.sales.entries.pageInfo', { page: result.page, pages, total: formatCount(result.total) }) }}</span>
        <div class="flex gap-1">
          <Button variant="secondary" size="sm" class="h-7 rounded-lg px-2" :disabled="page <= 1 || isLoading" :aria-label="t('admin.sales.entries.previous')" @click="page -= 1">
            <ChevronLeft class="h-3.5 w-3.5" />
          </Button>
          <Button variant="secondary" size="sm" class="h-7 rounded-lg px-2" :disabled="page >= pages || isLoading" :aria-label="t('admin.sales.entries.next')" @click="page += 1">
            <ChevronRight class="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
    </aside>
  </div>
</template>
