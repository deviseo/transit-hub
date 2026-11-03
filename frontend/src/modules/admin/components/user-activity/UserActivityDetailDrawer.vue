<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, Table2, X } from 'lucide-vue-next'
import ActivityTrendChart from './ActivityTrendChart.vue'
import PeriodComparisonCard from './PeriodComparisonCard.vue'
import { getUserActivityDetail } from '../../api/userActivity'
import { useUserActivityFormat } from '../../composables/useUserActivityFormat'
import type { UserActivityDetail, UserActivityPeriodKey } from '../../types/userActivity'

const props = defineProps<{
  siteUserId: string
  platform: string
  today: string
}>()

const emit = defineEmits<{ (event: 'close'): void }>()

const { t, te } = useI18n()
const { formatCount, formatUsd, formatUsdAxis, formatCompact, formatDay, formatShortDay, formatDateTime } = useUserActivityFormat()

const detail = ref<UserActivityDetail | null>(null)
const isLoading = ref(false)
const errorKey = ref('')
const showTable = ref(false)
let requestSequence = 0

const periodKeys: UserActivityPeriodKey[] = ['day', 'week', 'month']

// 趋势只画完整自然日（截至昨天），今天的数据在上方「今日」中单独展示，避免末端看起来像下跌。
const completeDays = computed(() => (detail.value?.daily ?? []).filter((point) => point.day < props.today))
const days = computed(() => completeDays.value.map((point) => point.day))
const requestValues = computed(() => completeDays.value.map((point) => point.requests))
const costValues = computed(() => completeDays.value.map((point) => point.cost))
const rechargeValues = computed(() => completeDays.value.map((point) => point.recharge))
const tableRows = computed(() => [...completeDays.value].reverse())

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.userActivity.errors.request'))

const lastUsedText = computed(() => {
  const user = detail.value?.user
  if (!user) return ''
  if (user.daysSinceLastUse != null) {
    return user.daysSinceLastUse === 0 ? t('admin.userActivity.lastUsed.today') : t('admin.userActivity.lastUsed.daysAgo', { count: user.daysSinceLastUse }, user.daysSinceLastUse)
  }
  return user.everUsed ? t('admin.userActivity.lastUsed.longAgo') : t('admin.userActivity.lastUsed.never')
})

const lifetimeLabel = computed(() => (props.platform === 'newapi' ? t('admin.userActivity.fields.totalUsed') : t('admin.userActivity.fields.totalRecharged')))
const lifetimeValue = computed(() => {
  const user = detail.value?.user
  if (!user) return null
  return props.platform === 'newapi' ? user.totalUsed : user.totalRecharged
})

const load = async () => {
  const sequence = ++requestSequence
  isLoading.value = true
  errorKey.value = ''
  try {
    const result = await getUserActivityDetail(props.siteUserId)
    if (sequence === requestSequence) detail.value = result
  } catch (error) {
    if (sequence === requestSequence) errorKey.value = error instanceof Error ? error.message : 'admin.userActivity.errors.request'
  } finally {
    if (sequence === requestSequence) isLoading.value = false
  }
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') emit('close')
}

watch(() => props.siteUserId, () => { void load() })

onMounted(() => {
  void load()
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="fixed inset-0 z-50 flex justify-end bg-background/70 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="user-activity-detail-title" @click.self="emit('close')">
    <aside class="flex h-full w-full max-w-3xl flex-col border-l border-border/60 bg-card shadow-xl">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <div class="min-w-0">
          <h2 id="user-activity-detail-title" class="truncate text-lg font-semibold text-foreground">
            {{ detail?.user.email || detail?.user.username || siteUserId }}
          </h2>
          <p class="mt-1 truncate text-sm text-muted-foreground">
            {{ t('admin.userActivity.detail.identity', { id: siteUserId, username: detail?.user.username || t('admin.userActivity.common.placeholder') }) }}
          </p>
        </div>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.userActivity.actions.close')" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
        <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
        <div v-if="isLoading && !detail" class="flex items-center justify-center py-16 text-sm text-muted-foreground" role="status">
          <Loader2 class="mr-2 h-4 w-4 animate-spin" />
          {{ t('admin.userActivity.status.loading') }}
        </div>

        <template v-if="detail">
          <dl class="grid grid-cols-2 gap-2 sm:grid-cols-4" :class="{ 'opacity-60': isLoading }">
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.lastUsed') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ lastUsedText }}</dd>
              <dd v-if="detail.user.lastUsedAt" class="mt-0.5 text-xs text-muted-foreground tabular-nums">{{ formatDateTime(detail.user.lastUsedAt) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.balance') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatUsd(detail.user.balance) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ lifetimeLabel }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatUsd(lifetimeValue) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.createdAt') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground tabular-nums">{{ formatDateTime(detail.user.createdAt) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.today') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatUsd(detail.user.costToday) }}</dd>
              <dd class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.userActivity.fields.requestsCount', { count: formatCount(detail.user.requestsToday) }, detail.user.requestsToday) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.cost7d') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatUsd(detail.user.cost7d) }}</dd>
              <dd class="mt-0.5 text-xs text-muted-foreground">{{ t('admin.userActivity.fields.requestsCount', { count: formatCount(detail.user.requests7d) }, detail.user.requests7d) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.cost30d') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatUsd(detail.user.cost30d) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.activeDays30d') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ t('admin.userActivity.fields.daysValue', { count: detail.user.activeDays30d }, detail.user.activeDays30d) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.rechargeToday') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatUsd(detail.user.rechargeToday) }}</dd>
            </div>
            <div class="rounded-lg bg-surface p-3">
              <dt class="text-xs text-muted-foreground">{{ t('admin.userActivity.fields.recharge30d') }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatUsd(detail.user.recharge30d) }}</dd>
            </div>
          </dl>

          <div class="grid gap-3">
            <PeriodComparisonCard v-for="key in periodKeys" :key="key" :period-key="key" :comparison="detail.periods[key]" scope="user" />
          </div>

          <section class="rounded-lg border border-border/50 p-4">
            <div class="flex items-center justify-between gap-3">
              <div>
                <h3 class="text-sm font-semibold text-foreground">{{ t('admin.userActivity.detail.trendTitle') }}</h3>
                <p class="text-xs text-muted-foreground">{{ t('admin.userActivity.trend.completeDays') }}</p>
              </div>
              <button
                type="button"
                class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 px-2.5 py-1.5 text-xs font-medium text-foreground hover:bg-surface focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                :aria-pressed="showTable"
                @click="showTable = !showTable"
              >
                <Table2 class="h-3.5 w-3.5" />
                {{ showTable ? t('admin.userActivity.trend.showChart') : t('admin.userActivity.trend.showTable') }}
              </button>
            </div>
            <div v-if="!showTable" class="mt-3 grid gap-4 md:grid-cols-2">
              <div>
                <p class="text-xs font-medium text-muted-foreground">{{ t('admin.userActivity.metrics.requests') }}</p>
                <div class="mt-1 h-44">
                  <ActivityTrendChart
                    :days="days"
                    :values="requestValues"
                    integer
                    :accessible-label="t('admin.userActivity.detail.requestsChartAria')"
                    :format-value="(value) => formatCount(value)"
                    :format-axis="formatCompact"
                    :format-day="formatDay"
                    :format-tick="formatShortDay"
                  />
                </div>
              </div>
              <div>
                <p class="text-xs font-medium text-muted-foreground">{{ t('admin.userActivity.metrics.cost') }}</p>
                <div class="mt-1 h-44">
                  <ActivityTrendChart
                    :days="days"
                    :values="costValues"
                    :accessible-label="t('admin.userActivity.detail.costChartAria')"
                    :format-value="(value) => formatUsd(value)"
                    :format-axis="formatUsdAxis"
                    :format-day="formatDay"
                    :format-tick="formatShortDay"
                  />
                </div>
              </div>
              <div>
                <p class="text-xs font-medium text-muted-foreground">{{ t('admin.userActivity.metrics.recharge') }}</p>
                <div class="mt-1 h-44">
                  <ActivityTrendChart
                    :days="days"
                    :values="rechargeValues"
                    :accessible-label="t('admin.userActivity.detail.rechargeChartAria')"
                    :format-value="(value) => formatUsd(value)"
                    :format-axis="formatUsdAxis"
                    :format-day="formatDay"
                    :format-tick="formatShortDay"
                    :no-data-label="t('admin.userActivity.trend.noData')"
                  />
                </div>
              </div>
            </div>
            <div v-else class="mt-3 max-h-80 overflow-auto rounded-lg border border-border/50">
              <table class="w-full text-left text-sm">
                <thead class="sticky top-0 bg-surface text-xs text-muted-foreground">
                  <tr>
                    <th class="px-3 py-2 font-medium">{{ t('admin.userActivity.trend.day') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.requests') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.tokens') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.cost') }}</th>
                    <th class="px-3 py-2 text-right font-medium">{{ t('admin.userActivity.metrics.recharge') }}</th>
                  </tr>
                </thead>
                <tbody class="divide-y divide-border/40 tabular-nums">
                  <tr v-for="point in tableRows" :key="point.day">
                    <td class="px-3 py-1.5 text-muted-foreground">{{ formatDay(point.day) }}</td>
                    <td class="px-3 py-1.5 text-right text-foreground">{{ formatCount(point.requests) }}</td>
                    <td class="px-3 py-1.5 text-right text-foreground">{{ formatCount(point.tokens) }}</td>
                    <td class="px-3 py-1.5 text-right text-foreground">{{ formatUsd(point.cost) }}</td>
                    <td class="px-3 py-1.5 text-right text-foreground">{{ point.recharge == null ? t('admin.userActivity.common.placeholder') : formatUsd(point.recharge) }}</td>
                  </tr>
                </tbody>
              </table>
            </div>
          </section>
        </template>
      </div>
    </aside>
  </div>
</template>
