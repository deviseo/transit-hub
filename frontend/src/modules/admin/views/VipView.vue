<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { AlertCircle, ArrowDownRight, ArrowUpRight, BellOff, Check, ClipboardCopy, Crown, Database, Loader2, NotebookPen, Plus, RefreshCw, Search, Settings2, UserMinus } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import UserActivityDetailDrawer from '../components/user-activity/UserActivityDetailDrawer.vue'
import VipSettingsDialog from '../components/vip/VipSettingsDialog.vue'
import VipWatchDialog from '../components/vip/VipWatchDialog.vue'
import { getVipSummary, unwatchVip } from '../api/vip'
import { useUserActivityFormat } from '../composables/useUserActivityFormat'
import { formatCny } from '../utils/dashboard'
import { vipDaysText, vipReminderText } from '../utils/vipReminder'
import type { VipRow, VipSettings, VipStatus, VipSummary } from '../types/vip'

// 大户关注：系统按近 7 天日均消耗、累计消耗推荐的大户，加上手动关注的；看余额、日均消耗和预计还能用几天。
// 快用完、用完了时后台用选好的机器人提醒站长；也可以复制一段话术发给用户。数据来自「用户分析」，只读。

type Filter = 'all' | 'low' | 'empty' | 'declining'

const { t, te } = useI18n()
const { formatDay, formatPercent, formatRelativeTime } = useUserActivityFormat()

const summary = ref<VipSummary | null>(null)
const isLoading = ref(false)
const errorKey = ref<string | null>(null)
const filter = ref<Filter>('all')
const search = ref('')
const detailUserId = ref<string | null>(null)
const watchDialog = ref<{ row?: VipRow } | null>(null)
const isSettingsOpen = ref(false)
const copiedId = ref('')
const removingId = ref('')
let requestId = 0

const today = new Intl.DateTimeFormat('en-CA', { timeZone: 'Asia/Shanghai', year: 'numeric', month: '2-digit', day: '2-digit' }).format(new Date())
const errorText = (key: string): string => (te(key) ? t(key) : t('admin.vip.errors.request'))

const load = async () => {
  const current = ++requestId
  isLoading.value = true
  errorKey.value = null
  try {
    const response = await getVipSummary()
    if (current !== requestId) return
    summary.value = response
  } catch (error) {
    if (current !== requestId) return
    errorKey.value = error instanceof Error ? error.message : 'admin.vip.errors.request'
  } finally {
    if (current === requestId) isLoading.value = false
  }
}

onMounted(() => { void load() })

const settings = computed(() => summary.value?.settings)
const needsBots = computed(() => Boolean(settings.value?.alertsEnabled && settings.value.botIds.length === 0))
const filters = computed(() => {
  const counts = summary.value?.counts ?? { all: 0, low: 0, empty: 0, declining: 0 }
  return [
    { value: 'all' as const, label: t('admin.vip.filters.all'), count: counts.all },
    { value: 'low' as const, label: t('admin.vip.filters.low'), count: counts.low },
    { value: 'empty' as const, label: t('admin.vip.filters.empty'), count: counts.empty },
    { value: 'declining' as const, label: t('admin.vip.filters.declining'), count: counts.declining },
  ]
})

const rows = computed(() => {
  const query = search.value.trim().toLowerCase()
  return (summary.value?.items ?? []).filter((row) => {
    if (filter.value !== 'all' && row.status !== filter.value) return false
    if (!query) return true
    return [row.email, row.username, row.siteUserId, row.source.note].some((value) => value.toLowerCase().includes(query))
  })
})

const name = (row: VipRow): string => row.email || row.username || `#${row.siteUserId}`

const statusTone: Record<VipStatus, string> = {
  normal: 'bg-surface text-muted-foreground',
  low: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
  empty: 'bg-destructive/10 text-destructive',
  declining: 'bg-amber-500/15 text-amber-700 dark:text-amber-300',
  disabled: 'bg-surface text-muted-foreground',
}

const daysLeftText = (row: VipRow): string => {
  if (row.status === 'empty') return t('admin.vip.status.empty')
  if (row.daysLeft == null) return t('admin.vip.common.placeholder')
  return vipDaysText(t, row.daysLeft)
}

const copyReminder = async (row: VipRow) => {
  const current = summary.value
  if (!current) return
  await navigator.clipboard.writeText(vipReminderText(t, row, current.siteName, current.settings))
  copiedId.value = row.siteUserId
  window.setTimeout(() => {
    if (copiedId.value === row.siteUserId) copiedId.value = ''
  }, 1500)
}

const remove = async (row: VipRow) => {
  if (!window.confirm(t('admin.vip.actions.removeConfirm', { name: name(row) }))) return
  removingId.value = row.siteUserId
  try {
    await unwatchVip(row.siteUserId)
    await load()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.vip.errors.request'
  } finally {
    removingId.value = ''
  }
}

const onSettingsSaved = (next: VipSettings) => {
  if (summary.value) summary.value = { ...summary.value, settings: next }
  isSettingsOpen.value = false
  void load()
}

const onWatched = () => {
  watchDialog.value = null
  void load()
}
</script>

<template>
  <div class="flex flex-col gap-3">
    <section class="rounded-lg border border-border/50 bg-card p-4 shadow-sm">
      <div class="flex flex-col gap-3 xl:flex-row xl:items-start xl:justify-between">
        <div class="min-w-0">
          <h2 class="flex items-center gap-2 text-base font-semibold text-foreground">
            <Crown class="h-4 w-4 text-amber-500" />
            {{ t('admin.vip.title') }}
          </h2>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.vip.subtitle') }}</p>
          <p v-if="summary" class="mt-2 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
            <span v-if="summary.platform" class="rounded-md bg-surface px-2 py-0.5 font-medium text-foreground">{{ t(`admin.userActivity.platforms.${summary.platform}`) }}</span>
            <span>{{ summary.lastSyncedAt ? t('admin.vip.source.lastSynced', { time: formatRelativeTime(summary.lastSyncedAt) }) : t('admin.vip.source.neverSynced') }}</span>
            <span aria-hidden="true">·</span>
            <span>{{ t('admin.vip.source.basis', { price: summary.unitPrice }) }}</span>
          </p>
        </div>
        <div class="flex shrink-0 flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" class="h-8 rounded-lg" :disabled="!summary" @click="isSettingsOpen = true">
            <Settings2 class="h-4 w-4" />
            {{ t('admin.vip.actions.settings') }}
          </Button>
          <Button variant="secondary" size="sm" class="h-8 rounded-lg" :disabled="isLoading" @click="load">
            <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('admin.vip.actions.refresh') }}
          </Button>
          <Button size="sm" class="h-8 rounded-lg" :disabled="!summary?.synced" @click="watchDialog = {}">
            <Plus class="h-4 w-4" />
            {{ t('admin.vip.actions.add') }}
          </Button>
        </div>
      </div>
    </section>

    <div v-if="errorKey" class="flex items-start gap-3 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ errorText(errorKey) }}</span>
    </div>

    <div v-if="isLoading && !summary" class="flex items-center justify-center rounded-lg border border-border/50 bg-card py-16 text-muted-foreground">
      <Loader2 class="h-5 w-5 animate-spin" />
    </div>

    <section v-else-if="summary && !summary.synced" class="flex flex-col items-center rounded-lg border border-dashed border-border/70 bg-card px-6 py-12 text-center">
      <Database class="mb-3 h-10 w-10 text-muted-foreground/60" />
      <h2 class="text-base font-semibold text-foreground">{{ t('admin.vip.empty.title') }}</h2>
      <p class="mt-2 max-w-xl text-sm text-muted-foreground">{{ t('admin.vip.empty.description') }}</p>
      <RouterLink to="/admin/user-activity" class="mt-4 inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground hover:bg-primary/90">
        {{ t('admin.vip.actions.goUserActivity') }}
      </RouterLink>
    </section>

    <template v-else-if="summary">
      <div v-if="needsBots" class="flex flex-wrap items-center gap-3 rounded-lg border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-700 dark:text-amber-300" role="status">
        <BellOff class="h-4 w-4 shrink-0" />
        <span class="min-w-0 flex-1">{{ t('admin.vip.notice.noBots') }}</span>
        <Button variant="secondary" size="sm" class="h-7 rounded-lg" @click="isSettingsOpen = true">{{ t('admin.vip.actions.chooseBots') }}</Button>
      </div>

      <section class="rounded-lg border border-border/50 bg-card shadow-sm" :class="{ 'opacity-60 transition-opacity': isLoading }">
        <div class="flex flex-wrap items-center justify-between gap-3 border-b border-border/50 p-4">
          <div class="flex items-center gap-1 rounded-lg bg-surface p-1" role="group" :aria-label="t('admin.vip.filters.label')">
            <button
              v-for="item in filters"
              :key="item.value"
              type="button"
              class="rounded-md px-3 py-1 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="filter === item.value ? 'bg-card text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
              :aria-pressed="filter === item.value"
              @click="filter = item.value"
            >
              {{ item.label }}
              <span class="ml-1 tabular-nums" :class="item.value !== 'all' && item.count > 0 ? 'text-amber-600 dark:text-amber-400' : 'text-muted-foreground'">{{ item.count }}</span>
            </button>
          </div>
          <div class="relative w-full max-w-xs">
            <Search class="pointer-events-none absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted-foreground" />
            <label class="sr-only" for="vip-search">{{ t('admin.vip.search') }}</label>
            <input
              id="vip-search"
              v-model="search"
              type="search"
              :placeholder="t('admin.vip.search')"
              class="h-8 w-full rounded-lg border border-border/60 bg-surface pl-7 pr-2 text-xs text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
            >
          </div>
        </div>

        <div v-if="summary.items.length === 0" class="px-6 py-12 text-center text-sm text-muted-foreground">{{ t('admin.vip.list.emptyAll') }}</div>
        <div v-else-if="rows.length === 0" class="px-6 py-12 text-center text-sm text-muted-foreground">{{ t('admin.vip.list.emptyFilter') }}</div>
        <div v-else class="overflow-x-auto">
          <table class="w-full min-w-[1080px] text-left text-sm">
            <thead class="bg-surface-elevated/60 text-xs text-muted-foreground">
              <tr>
                <th class="px-4 py-2 font-medium">{{ t('admin.vip.list.user') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.vip.list.source') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.vip.list.balance') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.vip.list.daily') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.vip.list.daysLeft') }}</th>
                <th class="px-4 py-2 text-right font-medium">{{ t('admin.vip.list.trend') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.vip.list.lastUsed') }}</th>
                <th class="px-4 py-2 font-medium">{{ t('admin.vip.list.lastCredit') }}</th>
                <th class="px-4 py-2 text-right font-medium"><span class="sr-only">{{ t('admin.vip.list.actions') }}</span></th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50 tabular-nums">
              <tr v-for="row in rows" :key="row.siteUserId" class="transition-colors hover:bg-surface/40">
                <td class="max-w-[15rem] px-4 py-2.5">
                  <button
                    type="button"
                    class="block max-w-full truncate text-left font-medium text-foreground hover:text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                    :title="t('admin.vip.actions.viewDetail', { name: name(row) })"
                    @click="detailUserId = row.siteUserId"
                  >
                    {{ name(row) }}
                  </button>
                  <span v-if="row.source.note" class="block truncate text-xs text-muted-foreground">{{ row.source.note }}</span>
                  <span v-else-if="row.email && row.username" class="block truncate text-xs text-muted-foreground">{{ row.username }}</span>
                </td>
                <td class="px-4 py-2.5">
                  <span class="flex flex-wrap gap-1 whitespace-nowrap">
                    <span v-if="row.source.manual" class="rounded-md bg-primary/10 px-1.5 py-0.5 text-[11px] text-primary">{{ t('admin.vip.source.manual') }}</span>
                    <span v-if="row.source.dailyRank" class="rounded-md bg-surface px-1.5 py-0.5 text-[11px] text-muted-foreground">{{ t('admin.vip.source.dailyRank', { rank: row.source.dailyRank }) }}</span>
                    <span v-if="row.source.lifetimeRank" class="rounded-md bg-surface px-1.5 py-0.5 text-[11px] text-muted-foreground">{{ t('admin.vip.source.lifetimeRank', { rank: row.source.lifetimeRank }) }}</span>
                  </span>
                </td>
                <td class="px-4 py-2.5 text-right font-medium" :class="row.status === 'empty' ? 'text-destructive' : row.status === 'low' ? 'text-amber-700 dark:text-amber-300' : 'text-foreground'">
                  {{ row.balance == null ? t('admin.vip.common.placeholder') : formatCny(row.balance) }}
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-right text-muted-foreground">{{ row.dailyAverage > 0 ? t('admin.vip.list.perDay', { amount: formatCny(row.dailyAverage) }) : t('admin.vip.common.placeholder') }}</td>
                <td class="px-4 py-2.5">
                  <span class="flex items-center gap-1.5 whitespace-nowrap">
                    <span v-if="row.status !== 'empty'" class="text-foreground">{{ daysLeftText(row) }}</span>
                    <span v-if="row.status !== 'normal' && row.status !== 'declining'" class="rounded-md px-1.5 py-0.5 text-[11px] font-medium" :class="statusTone[row.status]">{{ t(`admin.vip.status.${row.status}`) }}</span>
                  </span>
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-right">
                  <span v-if="row.trend == null" class="text-muted-foreground">{{ t('admin.vip.common.placeholder') }}</span>
                  <span v-else-if="Math.abs(row.trend) < 0.005" class="text-muted-foreground">{{ t('admin.vip.list.flat') }}</span>
                  <span v-else class="inline-flex items-center justify-end gap-0.5" :class="row.status === 'declining' ? 'font-medium text-amber-700 dark:text-amber-300' : row.trend >= 0 ? 'text-emerald-600 dark:text-emerald-400' : 'text-muted-foreground'">
                    <ArrowUpRight v-if="row.trend >= 0" class="h-3.5 w-3.5" />
                    <ArrowDownRight v-else class="h-3.5 w-3.5" />
                    {{ formatPercent(Math.abs(row.trend)) }}
                  </span>
                  <span v-if="row.status === 'declining'" class="block text-[11px] text-amber-700 dark:text-amber-300">{{ t('admin.vip.status.declining') }}</span>
                </td>
                <td class="whitespace-nowrap px-4 py-2.5 text-muted-foreground">{{ row.lastUsedAt ? formatRelativeTime(row.lastUsedAt) : t('admin.vip.common.placeholder') }}</td>
                <td class="whitespace-nowrap px-4 py-2.5 text-muted-foreground">
                  <template v-if="row.lastCreditDay">
                    {{ formatDay(row.lastCreditDay) }}
                    <span class="block text-xs">{{ formatCny(row.lastCreditAmount ?? 0) }}</span>
                  </template>
                  <template v-else>{{ t('admin.vip.common.placeholder') }}</template>
                </td>
                <td class="whitespace-nowrap px-4 py-2.5">
                  <div class="flex items-center justify-end gap-1">
                    <Button variant="secondary" size="sm" class="h-7 whitespace-nowrap rounded-lg px-2.5 text-xs" @click="copyReminder(row)">
                      <Check v-if="copiedId === row.siteUserId" class="h-3.5 w-3.5 text-emerald-600" />
                      <ClipboardCopy v-else class="h-3.5 w-3.5" />
                      {{ copiedId === row.siteUserId ? t('admin.vip.actions.copied') : t('admin.vip.actions.copyReminder') }}
                    </Button>
                    <Button variant="ghost" size="sm" class="h-7 rounded-lg px-2" :title="t('admin.vip.actions.note')" :aria-label="t('admin.vip.actions.note')" @click="watchDialog = { row }">
                      <NotebookPen class="h-3.5 w-3.5" />
                    </Button>
                    <Button variant="ghost" size="sm" class="h-7 rounded-lg px-2" :title="t('admin.vip.actions.remove')" :aria-label="t('admin.vip.actions.remove')" :disabled="removingId === row.siteUserId" @click="remove(row)">
                      <UserMinus class="h-3.5 w-3.5" />
                    </Button>
                  </div>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </section>

      <p class="px-1 text-xs text-muted-foreground">{{ t('admin.vip.footnote', { days: settings?.alertDays ?? 2, balance: formatCny(settings?.alertBalance ?? 20) }) }}</p>
    </template>

    <VipWatchDialog v-if="watchDialog" :row="watchDialog.row" @close="watchDialog = null" @saved="onWatched" />
    <VipSettingsDialog
      v-if="isSettingsOpen && summary"
      :settings="summary.settings"
      :site-name="summary.siteName"
      :dismissed-count="summary.dismissedCount"
      @close="isSettingsOpen = false"
      @saved="onSettingsSaved"
      @restored="load"
    />
    <UserActivityDetailDrawer
      v-if="detailUserId"
      :site-user-id="detailUserId"
      :platform="summary?.platform ?? ''"
      :today="today"
      @close="detailUserId = null"
    />
  </div>
</template>
