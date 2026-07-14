<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Crown, Medal, RefreshCw, Trophy, Users } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import type { LeaderboardPeriod, LeaderboardRow } from '../types'

const props = defineProps<{
  rows: LeaderboardRow[]
  loading: boolean
  errorKey: string | null
  period: LeaderboardPeriod
  updatedAt: Date | null
  i18nPrefix: 'admin.leaderboard' | 'embed.leaderboard'
}>()

const emit = defineEmits<{
  refresh: []
  'update:period': [period: LeaderboardPeriod]
}>()

const { t, locale } = useI18n()
const key = (suffix: string): string => `${props.i18nPrefix}.${suffix}`

const periods: LeaderboardPeriod[] = ['today', '7d', '30d']
const podiumRows = computed(() => props.rows.slice(0, 3))
const remainingRows = computed(() => props.rows.slice(3))

const numberFormatter = computed(() => new Intl.NumberFormat(locale.value, { notation: 'compact', maximumFractionDigits: 1 }))
const currencyFormatter = computed(() => new Intl.NumberFormat(locale.value, { style: 'currency', currency: 'USD', maximumFractionDigits: 2 }))

const formatNumber = (value: number): string => numberFormatter.value.format(value)
const formatCurrency = (value: number): string => currencyFormatter.value.format(value)
const initials = (row: LeaderboardRow): string => (row.email || row.userId || '?').slice(0, 1).toUpperCase()
const identity = (row: LeaderboardRow): string => row.email || t(key('anonymous'), { id: row.userId.slice(-6) })

const podiumOrderClass = (rank: number): string => {
  if (rank === 1) return 'md:order-2 md:-translate-y-7'
  if (rank === 2) return 'md:order-1'
  return 'md:order-3'
}

const medalClass = (rank: number): string => {
  if (rank === 1) return 'border-amber-400/70 bg-amber-400/10 text-amber-600 dark:text-amber-300'
  if (rank === 2) return 'border-slate-300 bg-slate-400/10 text-slate-600 dark:border-slate-500 dark:text-slate-300'
  return 'border-orange-400/60 bg-orange-400/10 text-orange-700 dark:text-orange-300'
}

const formatUpdatedAt = (): string => {
  if (!props.updatedAt) return ''
  return new Intl.DateTimeFormat(locale.value, {
    timeZone: 'Asia/Shanghai',
    hour: '2-digit',
    minute: '2-digit',
  }).format(props.updatedAt)
}
</script>

<template>
  <section class="w-full text-foreground" :aria-busy="loading">
    <header class="mb-8 flex flex-col gap-4 border-b border-border/70 pb-5 sm:flex-row sm:items-end sm:justify-between">
      <div>
        <div class="mb-2 flex items-center gap-2 text-primary">
          <Trophy class="h-5 w-5" aria-hidden="true" />
          <span class="text-xs font-semibold uppercase">{{ t(key('eyebrow')) }}</span>
        </div>
        <h1 class="text-2xl font-semibold text-foreground sm:text-3xl">{{ t(key('title')) }}</h1>
        <p class="mt-1 text-sm text-muted-foreground">{{ t(key('subtitle')) }}</p>
      </div>

      <div class="flex flex-wrap items-center gap-2">
        <div class="inline-flex h-9 items-center rounded-lg border border-border/70 bg-surface p-1" :aria-label="t(key('period.label'))">
          <button
            v-for="option in periods"
            :key="option"
            type="button"
            :aria-pressed="period === option"
            :class="[
              'h-7 rounded-md px-3 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary',
              period === option ? 'bg-primary text-primary-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground',
            ]"
            @click="emit('update:period', option)"
          >
            {{ t(key(`period.${option}`)) }}
          </button>
        </div>
        <slot name="actions" />
        <Button
          variant="secondary"
          size="sm"
          :disabled="loading"
          :title="t(key('refresh'))"
          :aria-label="t(key('refresh'))"
          @click="emit('refresh')"
        >
          <RefreshCw :class="['h-4 w-4', loading ? 'animate-spin' : '']" aria-hidden="true" />
        </Button>
      </div>
    </header>

    <div v-if="errorKey" class="mb-6 flex items-start gap-3 rounded-lg border border-destructive/30 bg-destructive/5 p-4 text-sm text-destructive">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
      <div>
        <p class="font-medium">{{ t(key('errorTitle')) }}</p>
        <p class="mt-1 text-foreground/75">{{ t(errorKey) }}</p>
      </div>
    </div>

    <template v-if="loading && rows.length === 0">
      <div class="mb-10 grid gap-4 md:grid-cols-3 md:items-end">
        <div v-for="index in 3" :key="index" class="h-64 animate-pulse rounded-lg border border-border/60 bg-surface" />
      </div>
      <div class="h-72 animate-pulse rounded-lg border border-border/60 bg-surface" />
    </template>

    <div v-else-if="rows.length === 0" class="flex min-h-80 flex-col items-center justify-center rounded-lg border border-dashed border-border bg-surface/50 px-6 text-center">
      <Users class="h-10 w-10 text-muted-foreground" aria-hidden="true" />
      <h2 class="mt-4 text-base font-semibold">{{ t(key('emptyTitle')) }}</h2>
      <p class="mt-1 max-w-sm text-sm text-muted-foreground">{{ t(key('emptyDescription')) }}</p>
    </div>

    <template v-else>
      <ol class="mb-10 grid gap-4 pt-7 md:grid-cols-3 md:items-end" :aria-label="t(key('podiumLabel'))">
        <li
          v-for="row in podiumRows"
          :key="row.userId || row.rank"
          :class="['relative min-h-64 rounded-lg border bg-card p-5 shadow-sm transition-transform', medalClass(row.rank), podiumOrderClass(row.rank), row.rank === 1 ? 'md:min-h-72' : '']"
        >
          <div class="flex items-start justify-between">
            <span class="text-4xl font-semibold tabular-nums opacity-25">0{{ row.rank }}</span>
            <Crown v-if="row.rank === 1" class="h-7 w-7" aria-hidden="true" />
            <Medal v-else class="h-7 w-7" aria-hidden="true" />
          </div>
          <div class="mt-5 flex flex-col items-center text-center">
            <div class="flex h-16 w-16 items-center justify-center rounded-full border-2 border-current bg-background text-xl font-semibold shadow-sm">
              {{ initials(row) }}
            </div>
            <p class="mt-3 max-w-full truncate text-sm font-semibold text-foreground" :title="identity(row)">{{ identity(row) }}</p>
          </div>
          <dl class="mt-5 grid grid-cols-3 gap-2 border-t border-current/15 pt-4 text-center">
            <div>
              <dt class="text-[11px] text-muted-foreground">{{ t(key('metrics.tokens')) }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatNumber(row.totalTokens) }}</dd>
            </div>
            <div>
              <dt class="text-[11px] text-muted-foreground">{{ t(key('metrics.requests')) }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatNumber(row.requests) }}</dd>
            </div>
            <div>
              <dt class="text-[11px] text-muted-foreground">{{ t(key('metrics.cost')) }}</dt>
              <dd class="mt-1 text-sm font-semibold text-foreground">{{ formatCurrency(row.actualCost) }}</dd>
            </div>
          </dl>
        </li>
      </ol>

      <div class="overflow-hidden rounded-lg border border-border/70 bg-card shadow-sm">
        <div class="flex items-center justify-between gap-3 border-b border-border/70 px-4 py-3 sm:px-5">
          <div>
            <h2 class="text-sm font-semibold">{{ t(key('table.title')) }}</h2>
            <p class="mt-0.5 text-xs text-muted-foreground">{{ t(key('table.caption'), { count: rows.length }) }}</p>
          </div>
          <span v-if="updatedAt" class="text-xs text-muted-foreground">{{ t(key('updatedAt'), { time: formatUpdatedAt() }) }}</span>
        </div>

        <div class="hidden overflow-x-auto md:block">
          <table class="w-full min-w-[720px] text-left text-sm">
            <thead class="bg-surface text-xs text-muted-foreground">
              <tr>
                <th class="w-20 px-5 py-3 font-medium">{{ t(key('table.rank')) }}</th>
                <th class="px-4 py-3 font-medium">{{ t(key('table.user')) }}</th>
                <th class="px-4 py-3 text-right font-medium">{{ t(key('metrics.tokens')) }}</th>
                <th class="px-4 py-3 text-right font-medium">{{ t(key('metrics.requests')) }}</th>
                <th class="px-5 py-3 text-right font-medium">{{ t(key('metrics.cost')) }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/60">
              <tr v-for="row in remainingRows" :key="row.userId || row.rank" class="transition-colors hover:bg-surface/70">
                <td class="px-5 py-3.5 font-semibold tabular-nums text-muted-foreground">{{ String(row.rank).padStart(2, '0') }}</td>
                <td class="px-4 py-3.5">
                  <div class="flex items-center gap-3">
                    <span class="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">{{ initials(row) }}</span>
                    <span class="font-medium text-foreground">{{ identity(row) }}</span>
                  </div>
                </td>
                <td class="px-4 py-3.5 text-right font-semibold text-primary">{{ formatNumber(row.totalTokens) }}</td>
                <td class="px-4 py-3.5 text-right tabular-nums text-muted-foreground">{{ formatNumber(row.requests) }}</td>
                <td class="px-5 py-3.5 text-right font-semibold tabular-nums">{{ formatCurrency(row.actualCost) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <ol class="divide-y divide-border/60 md:hidden" start="4">
          <li v-for="row in remainingRows" :key="row.userId || row.rank" class="p-4">
            <div class="flex items-center gap-3">
              <span class="w-7 shrink-0 text-sm font-semibold tabular-nums text-muted-foreground">{{ String(row.rank).padStart(2, '0') }}</span>
              <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-xs font-semibold text-primary">{{ initials(row) }}</span>
              <span class="min-w-0 flex-1 truncate text-sm font-medium">{{ identity(row) }}</span>
            </div>
            <dl class="mt-3 grid grid-cols-3 gap-2 pl-10 text-xs">
              <div><dt class="text-muted-foreground">{{ t(key('metrics.tokens')) }}</dt><dd class="mt-1 font-semibold text-primary">{{ formatNumber(row.totalTokens) }}</dd></div>
              <div><dt class="text-muted-foreground">{{ t(key('metrics.requests')) }}</dt><dd class="mt-1 font-semibold">{{ formatNumber(row.requests) }}</dd></div>
              <div><dt class="text-muted-foreground">{{ t(key('metrics.cost')) }}</dt><dd class="mt-1 font-semibold">{{ formatCurrency(row.actualCost) }}</dd></div>
            </dl>
          </li>
        </ol>
      </div>
    </template>
  </section>
</template>
