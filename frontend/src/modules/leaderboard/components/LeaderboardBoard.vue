<script setup lang="ts">
import { computed, useId } from 'vue'
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

import { t, locale } from '@/locales'
const titleId = useId()
const key = (suffix: string): string => `${props.i18nPrefix}.${suffix}`

const periods: LeaderboardPeriod[] = ['today', '7d', '30d']
const podiumRows = computed(() => props.rows.slice(0, 3))

const numberFormatter = computed(() => new Intl.NumberFormat(locale, {
  notation: 'compact',
  maximumFractionDigits: 1,
}))
const currencyFormatter = computed(() => new Intl.NumberFormat(locale, {
  style: 'currency',
  currency: 'USD',
  maximumFractionDigits: 2,
}))

const formatNumber = (value: number): string => numberFormatter.value.format(value)
const formatCurrency = (value: number): string => currencyFormatter.value.format(value)
const initials = (row: LeaderboardRow): string => (row.email || row.userId || '?').slice(0, 1).toUpperCase()
const identity = (row: LeaderboardRow): string => row.email || t(key('anonymous'), { id: row.userId.slice(-6) })
const rankLabel = (rank: number): string => String(rank).padStart(2, '0')

const podiumPositionClass = (rank: number): string => {
  if (rank === 1) return 'col-span-2 md:col-span-1 md:col-start-2 md:row-start-1 md:min-h-[22rem]'
  if (rank === 2) return 'md:col-start-1 md:row-start-1 md:mt-10 md:min-h-[19.5rem]'
  return 'md:col-start-3 md:row-start-1 md:mt-10 md:min-h-[19.5rem]'
}

const podiumSurfaceClass = (rank: number): string => {
  if (rank === 1) {
    return 'border-amber-400/70 bg-amber-50 text-foreground shadow-lg shadow-amber-500/10 hover:-translate-y-1 dark:border-amber-300/40 dark:bg-amber-400/10'
  }
  if (rank === 2) {
    return 'border-zinc-300 bg-zinc-100/80 text-foreground shadow-sm hover:-translate-y-1 hover:border-zinc-400 dark:border-zinc-500 dark:bg-zinc-400/10 dark:hover:border-zinc-400'
  }
  return 'border-orange-300 bg-orange-50 text-foreground shadow-sm hover:-translate-y-1 hover:border-orange-400 dark:border-orange-400/40 dark:bg-orange-400/10 dark:hover:border-orange-300/60'
}

const podiumAccentClass = (rank: number): string => {
  if (rank === 1) return 'text-amber-600 dark:text-amber-300'
  if (rank === 2) return 'text-zinc-500 dark:text-zinc-300'
  return 'text-orange-600 dark:text-orange-300'
}

const podiumRankClass = (rank: number): string => {
  if (rank === 1) return 'text-amber-600/35 dark:text-amber-300/30'
  if (rank === 2) return 'text-zinc-500/35 dark:text-zinc-300/30'
  return 'text-orange-600/35 dark:text-orange-300/30'
}

const podiumIconClass = (rank: number): string => {
  if (rank === 1) return 'fill-amber-400/15 text-amber-600 dark:fill-amber-300/10 dark:text-amber-300'
  if (rank === 2) return 'fill-zinc-400/15 text-zinc-500 dark:fill-zinc-300/10 dark:text-zinc-300'
  return 'fill-orange-400/15 text-orange-600 dark:fill-orange-300/10 dark:text-orange-300'
}

const podiumAvatarClass = (rank: number): string => {
  if (rank === 1) {
    return 'h-20 w-20 bg-amber-100 text-2xl text-amber-800 ring-4 ring-amber-400/30 dark:bg-amber-300/15 dark:text-amber-200 dark:ring-amber-300/20'
  }
  if (rank === 2) {
    return 'h-16 w-16 bg-zinc-200 text-xl text-zinc-700 ring-4 ring-zinc-400/25 dark:bg-zinc-300/15 dark:text-zinc-200 dark:ring-zinc-300/15'
  }
  return 'h-16 w-16 bg-orange-100 text-xl text-orange-800 ring-4 ring-orange-400/25 dark:bg-orange-300/15 dark:text-orange-200 dark:ring-orange-300/15'
}

const podiumTableAvatarClass = (rank: number): string => {
  if (rank === 1) return 'bg-amber-100 text-amber-700 dark:bg-amber-300/15 dark:text-amber-300'
  if (rank === 2) return 'bg-zinc-200 text-zinc-700 dark:bg-zinc-300/15 dark:text-zinc-300'
  return 'bg-orange-100 text-orange-700 dark:bg-orange-300/15 dark:text-orange-300'
}

const formatUpdatedAt = (): string => {
  if (!props.updatedAt) return ''
  return new Intl.DateTimeFormat(locale, {
    timeZone: 'Asia/Shanghai',
    hour: '2-digit',
    minute: '2-digit',
  }).format(props.updatedAt)
}
</script>

<template>
  <section class="w-full text-foreground" :aria-busy="loading" :aria-labelledby="titleId">
    <header class="mb-6 flex flex-col gap-5 sm:flex-row sm:items-end sm:justify-between">
      <div class="flex min-w-0 items-start gap-3.5">
        <span class="mt-0.5 flex h-10 w-10 shrink-0 items-center justify-center rounded-lg bg-primary text-primary-foreground shadow-sm">
          <Trophy class="h-5 w-5" aria-hidden="true" />
        </span>
        <div class="min-w-0">
          <p class="mb-1 text-xs font-medium text-primary">{{ t(key('eyebrow')) }}</p>
          <h1 :id="titleId" class="text-xl font-semibold leading-tight text-foreground sm:text-2xl">
            {{ t(key('title')) }}
          </h1>
          <p class="mt-1.5 max-w-2xl text-sm leading-6 text-muted-foreground">{{ t(key('subtitle')) }}</p>
        </div>
      </div>

      <div class="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:justify-end">
        <div
          role="group"
          class="grid h-9 min-w-0 flex-1 grid-cols-3 rounded-lg border border-border bg-surface p-1 sm:flex-none"
          :aria-label="t(key('period.label'))"
        >
          <button
            v-for="option in periods"
            :key="option"
            type="button"
            :aria-pressed="period === option"
            :class="[
              'h-7 min-w-16 whitespace-nowrap rounded-md px-2.5 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary focus-visible:ring-offset-1 focus-visible:ring-offset-background',
              period === option
                ? 'bg-card text-foreground shadow-sm ring-1 ring-border/70'
                : 'text-muted-foreground hover:text-foreground',
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

    <div
      v-if="errorKey"
      role="alert"
      class="flex min-h-40 items-start gap-4 rounded-lg border border-destructive/25 bg-card p-5 shadow-sm sm:items-center sm:p-6"
    >
      <span class="flex h-11 w-11 shrink-0 items-center justify-center rounded-lg bg-destructive/10 text-destructive">
        <AlertCircle class="h-5 w-5" aria-hidden="true" />
      </span>
      <div class="min-w-0">
        <h2 class="text-sm font-semibold text-foreground">{{ t(key('errorTitle')) }}</h2>
        <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ t(errorKey) }}</p>
      </div>
    </div>

    <template v-else-if="loading && rows.length === 0">
      <div class="grid grid-cols-2 gap-3 pt-4 md:grid-cols-3 md:gap-5 md:pt-6" aria-hidden="true">
        <div class="col-span-2 min-h-80 animate-pulse rounded-lg border border-amber-400/40 bg-amber-400/10 md:col-span-1 md:col-start-2 md:row-start-1 md:min-h-[22rem]" />
        <div class="min-h-72 animate-pulse rounded-lg border border-zinc-300 bg-zinc-400/10 md:col-start-1 md:row-start-1 md:mt-10 md:min-h-[19.5rem] dark:border-zinc-500" />
        <div class="min-h-72 animate-pulse rounded-lg border border-orange-400/40 bg-orange-400/10 md:col-start-3 md:row-start-1 md:mt-10 md:min-h-[19.5rem]" />
      </div>
      <div class="mt-6 overflow-hidden rounded-lg border border-border bg-card shadow-sm" aria-hidden="true">
        <div class="p-5">
          <div class="h-10 animate-pulse rounded-md bg-surface" />
          <div class="mt-4 space-y-2">
            <div v-for="index in 5" :key="index" class="h-12 animate-pulse rounded-md bg-surface/70" />
          </div>
        </div>
      </div>
    </template>

    <div
      v-else-if="rows.length === 0"
      class="flex min-h-64 flex-col items-center justify-center rounded-lg border border-border bg-card px-6 py-12 text-center shadow-sm"
    >
      <span class="flex h-12 w-12 items-center justify-center rounded-lg bg-surface text-muted-foreground">
        <Users class="h-5 w-5" aria-hidden="true" />
      </span>
      <h2 class="mt-4 text-base font-semibold">{{ t(key('emptyTitle')) }}</h2>
      <p class="mt-1.5 max-w-sm text-sm leading-6 text-muted-foreground">{{ t(key('emptyDescription')) }}</p>
    </div>

    <template v-else>
      <ol
        class="grid grid-cols-2 gap-3 pt-4 md:grid-cols-3 md:gap-5 md:pt-6"
        :aria-label="t(key('podiumLabel'))"
      >
        <li
          v-for="row in podiumRows"
          :key="row.userId || row.rank"
          :class="[
            'relative flex min-h-80 overflow-hidden rounded-lg border p-4 transition-[transform,border-color,box-shadow] duration-200 sm:p-5',
            podiumPositionClass(row.rank),
            podiumSurfaceClass(row.rank),
          ]"
        >
          <span
            :class="[
              'pointer-events-none absolute left-4 top-4 font-mono text-3xl font-semibold leading-none tabular-nums',
              podiumRankClass(row.rank),
            ]"
          >
            {{ rankLabel(row.rank) }}
          </span>

          <div class="flex min-w-0 flex-1 flex-col items-center text-center">
            <div class="flex min-h-0 w-full flex-1 flex-col items-center justify-center pt-8">
              <Crown
                v-if="row.rank === 1"
                :class="['mb-2 h-8 w-8 drop-shadow-sm', podiumIconClass(row.rank)]"
                :stroke-width="1.8"
                aria-hidden="true"
              />
              <Medal
                v-else
                :class="['mb-2 h-7 w-7 drop-shadow-sm', podiumIconClass(row.rank)]"
                :stroke-width="1.8"
                aria-hidden="true"
              />

              <span
                :class="[
                  'flex shrink-0 items-center justify-center rounded-full font-semibold shadow-sm',
                  podiumAvatarClass(row.rank),
                ]"
              >
                {{ initials(row) }}
              </span>

              <p class="mt-3 w-full truncate text-sm font-semibold" :title="identity(row)">{{ identity(row) }}</p>
              <div class="mt-5">
                <p class="text-xs font-medium text-muted-foreground">
                  {{ t(key('metrics.tokens')) }}
                </p>
                <p :class="['mt-1 font-mono font-semibold leading-none tracking-normal tabular-nums', row.rank === 1 ? 'text-4xl' : 'text-2xl', podiumAccentClass(row.rank)]">
                  {{ formatNumber(row.totalTokens) }}
                </p>
              </div>
            </div>

            <dl
              class="mt-5 grid w-full grid-cols-2 gap-2 border-t border-border/70 pt-4 text-center text-xs"
            >
              <div class="min-w-0">
                <dt class="text-muted-foreground">{{ t(key('metrics.requests')) }}</dt>
                <dd class="mt-1 whitespace-nowrap font-mono font-semibold tabular-nums">{{ formatNumber(row.requests) }}</dd>
              </div>
              <div class="min-w-0">
                <dt class="text-muted-foreground">{{ t(key('metrics.cost')) }}</dt>
                <dd class="mt-1 whitespace-nowrap font-mono font-semibold tabular-nums">{{ formatCurrency(row.actualCost) }}</dd>
              </div>
            </dl>
          </div>
        </li>
      </ol>

      <section class="mt-6 overflow-hidden rounded-lg border border-border bg-card shadow-sm" :aria-labelledby="`${titleId}-table`">
        <div class="flex flex-col gap-1 border-b border-border bg-surface/55 px-4 py-3.5 sm:flex-row sm:items-center sm:justify-between sm:px-5">
          <div>
            <h2 :id="`${titleId}-table`" class="text-sm font-semibold">{{ t(key('table.title')) }}</h2>
            <p class="mt-0.5 text-xs text-muted-foreground">{{ t(key('table.caption'), { count: rows.length }) }}</p>
          </div>
          <span v-if="updatedAt" class="text-xs text-muted-foreground">{{ t(key('updatedAt'), { time: formatUpdatedAt() }) }}</span>
        </div>

        <div class="hidden overflow-x-auto md:block">
          <table class="w-full min-w-[720px] text-left text-sm">
            <thead class="bg-card text-xs text-muted-foreground">
              <tr>
                <th class="w-20 px-5 py-3 font-medium">{{ t(key('table.rank')) }}</th>
                <th class="px-4 py-3 font-medium">{{ t(key('table.user')) }}</th>
                <th class="px-4 py-3 text-right font-medium">{{ t(key('metrics.tokens')) }}</th>
                <th class="px-4 py-3 text-right font-medium">{{ t(key('metrics.requests')) }}</th>
                <th class="px-5 py-3 text-right font-medium">{{ t(key('metrics.cost')) }}</th>
              </tr>
            </thead>
            <tbody>
              <tr
                v-for="row in rows"
                :key="row.userId || row.rank"
                class="odd:bg-surface/35 transition-colors hover:bg-primary/5"
              >
                <td :class="['px-5 py-3.5 font-mono font-semibold tabular-nums', row.rank <= 3 ? podiumAccentClass(row.rank) : 'text-muted-foreground']">
                  {{ rankLabel(row.rank) }}
                </td>
                <td class="px-4 py-3.5">
                  <div class="flex items-center gap-3">
                    <span :class="['flex h-8 w-8 shrink-0 items-center justify-center rounded-md text-xs font-semibold', row.rank <= 3 ? podiumTableAvatarClass(row.rank) : 'bg-surface text-foreground']">
                      {{ initials(row) }}
                    </span>
                    <span class="min-w-0 truncate font-medium text-foreground" :title="identity(row)">{{ identity(row) }}</span>
                  </div>
                </td>
                <td class="px-4 py-3.5 text-right font-mono font-semibold tabular-nums text-primary">{{ formatNumber(row.totalTokens) }}</td>
                <td class="px-4 py-3.5 text-right font-mono tabular-nums text-muted-foreground">{{ formatNumber(row.requests) }}</td>
                <td class="px-5 py-3.5 text-right font-mono font-medium tabular-nums text-foreground">{{ formatCurrency(row.actualCost) }}</td>
              </tr>
            </tbody>
          </table>
        </div>

        <ol class="md:hidden">
          <li v-for="row in rows" :key="row.userId || row.rank" class="odd:bg-surface/35 p-4">
            <div class="flex items-center gap-3">
              <span :class="['w-7 shrink-0 font-mono text-sm font-semibold tabular-nums', row.rank <= 3 ? podiumAccentClass(row.rank) : 'text-muted-foreground']">
                {{ rankLabel(row.rank) }}
              </span>
              <span :class="['flex h-9 w-9 shrink-0 items-center justify-center rounded-md text-xs font-semibold', row.rank <= 3 ? podiumTableAvatarClass(row.rank) : 'bg-surface text-foreground']">
                {{ initials(row) }}
              </span>
              <span class="min-w-0 flex-1 truncate text-sm font-medium" :title="identity(row)">{{ identity(row) }}</span>
              <span class="font-mono text-sm font-semibold tabular-nums text-primary">{{ formatNumber(row.totalTokens) }}</span>
            </div>
            <dl class="mt-3 grid grid-cols-2 gap-3 pl-10 text-xs">
              <div class="flex items-center justify-between gap-2">
                <dt class="text-muted-foreground">{{ t(key('metrics.requests')) }}</dt>
                <dd class="font-mono font-medium tabular-nums">{{ formatNumber(row.requests) }}</dd>
              </div>
              <div class="flex items-center justify-between gap-2">
                <dt class="text-muted-foreground">{{ t(key('metrics.cost')) }}</dt>
                <dd class="font-mono font-medium tabular-nums">{{ formatCurrency(row.actualCost) }}</dd>
              </div>
            </dl>
          </li>
        </ol>
      </section>
    </template>
  </section>
</template>
