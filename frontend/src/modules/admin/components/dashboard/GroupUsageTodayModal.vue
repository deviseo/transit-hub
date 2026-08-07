<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { AlertTriangle, ArrowDownWideNarrow, ArrowUpWideNarrow, Loader2, RefreshCw, TrendingUp, X } from 'lucide-vue-next'
import {
  getGroupUsageToday,
  type GroupUsageTodayItem,
  type GroupProfitQuality,
  type ProfitIssue,
} from '../../api/dashboardAdmin'
import { formatCny } from '../../utils/dashboard'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

import { t } from '@/locales'
const loading = ref(false)
const error = ref<string | null>(null)
const groups = ref<GroupUsageTodayItem[]>([])
const total = ref(0)
const quality = ref<GroupProfitQuality | null>(null)
const issues = ref<ProfitIssue[]>([])
// 默认按金额从高到低排序；toggle 后按金额从低到高，金额相同时用分组名排序，均不触发新的请求。
const sortAsc = ref(false)

const sortedGroups = computed(() => {
  return [...groups.value].sort((a, b) => {
    const diff = sortAsc.value ? a.todayAmount - b.todayAmount : b.todayAmount - a.todayAmount
    if (diff !== 0) return diff
    return a.groupName.localeCompare(b.groupName)
  })
})

const toggleSort = () => {
  sortAsc.value = !sortAsc.value
}

const qualityLabel = computed(() => {
  if (quality.value?.status === 'exact') return t('admin.dashboard.groupUsage.statusExact')
  if (quality.value?.status === 'partial') return t('admin.dashboard.groupUsage.statusPartial')
  return t('admin.dashboard.groupUsage.statusUnavailable')
})

const issueScope = (issue: ProfitIssue) => [
  issue.connectionId ? `连接 ${issue.connectionId}` : '',
  issue.accountId ? `账号 ${issue.accountId}` : '',
  issue.groupId ? `分组 ${issue.groupId}` : '',
  issue.siteId ? `站点 ${issue.siteId}` : '',
  issue.keyId ? `Key ${issue.keyId}` : '',
].filter(Boolean).join(' · ') || t('admin.dashboard.groupUsage.noDetail')

const issueMeta = (issue: ProfitIssue) => {
  const status = issue.httpStatus ? `HTTP ${issue.httpStatus}` : ''
  const retryable = issue.httpStatus != null || issue.retryable
    ? (issue.retryable
      ? t('admin.dashboard.groupUsage.retryable')
      : t('admin.dashboard.groupUsage.nonRetryable'))
    : ''
  return [status, retryable].filter(Boolean).join(' · ')
}

// 仅在弹窗打开（open 从 false 变为 true）或用户点击重试时才发起请求，
// 不在挂载时、排序切换时或关闭时请求。
const loadData = async () => {
  loading.value = true
  error.value = null
  quality.value = null
  issues.value = []
  try {
    const response = await getGroupUsageToday()
    groups.value = response.groups ?? []
    total.value = response.total ?? 0
    quality.value = response.quality ?? null
    issues.value = response.issues ?? []
  } catch (err) {
    error.value = err instanceof Error ? err.message : 'admin.dashboard.groupUsage.loadError'
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
        class="relative w-full max-w-2xl overflow-hidden rounded-[2rem] border border-border/60 bg-card text-card-foreground shadow-2xl shadow-primary/10 animate-in fade-in zoom-in-95 duration-200"
      >
        <div class="absolute left-0 right-0 top-0 h-1 bg-gradient-to-r from-primary via-accent to-primary" />

        <div class="flex items-start justify-between gap-4 px-6 pt-6">
          <div class="flex items-center gap-3">
            <div class="flex h-11 w-11 items-center justify-center rounded-full bg-primary/10 text-primary">
              <TrendingUp class="h-5 w-5" />
            </div>
            <div>
              <h2 class="text-lg font-semibold text-foreground">{{ t('admin.dashboard.groupUsage.title') }}</h2>
              <p class="text-sm text-muted-foreground">
                {{ t('admin.dashboard.groupUsage.subtitle', { count: groups.length, total: formatCny(total) }) }}
              </p>
            </div>
          </div>
          <div class="flex items-center gap-2">
            <button
              type="button"
              :disabled="loading || !!error || groups.length === 0"
              class="inline-flex items-center gap-1.5 rounded-lg border border-border/60 px-3 py-1.5 text-sm font-medium text-muted-foreground transition-colors hover:border-primary/40 hover:text-foreground disabled:opacity-50"
              @click="toggleSort"
            >
              <ArrowUpWideNarrow v-if="sortAsc" class="h-3.5 w-3.5" />
              <ArrowDownWideNarrow v-else class="h-3.5 w-3.5" />
              {{ sortAsc ? t('admin.dashboard.groupUsage.sort.asc') : t('admin.dashboard.groupUsage.sort.desc') }}
            </button>
            <button
              type="button"
              class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
              :title="t('admin.dashboard.groupUsage.close')"
              @click="emit('close')"
            >
              <X class="h-5 w-5" />
            </button>
          </div>
        </div>

        <div class="px-6 py-6">
          <div v-if="quality" class="mb-4 space-y-3 rounded-lg border border-warning/30 bg-warning/5 px-3 py-3 text-xs">
            <div class="flex flex-wrap items-center justify-between gap-2">
              <span class="font-medium text-foreground">
                {{ t('admin.dashboard.groupUsage.quality', {
                  status: qualityLabel,
                  resolved: quality.resolvedConnections,
                  expected: quality.expectedConnections,
                }) }}
              </span>
              <span v-if="issues.length" class="text-warning">
                {{ t('admin.dashboard.groupUsage.issuesTitle', { count: issues.length }) }}
              </span>
            </div>
            <ul v-if="issues.length" class="space-y-2 border-t border-warning/20 pt-2 text-muted-foreground">
              <li v-for="issue in issues" :key="`${issue.code}-${issue.connectionId ?? issue.groupId ?? issue.keyId ?? 'global'}`" class="space-y-0.5">
                <div class="flex flex-wrap items-center gap-x-2 gap-y-1">
                  <AlertTriangle class="h-3.5 w-3.5 text-warning" />
                  <span class="font-medium text-foreground">{{ issue.code }}</span>
                  <span>{{ issueMeta(issue) }}</span>
                </div>
                <div>{{ issueScope(issue) }}</div>
                <div v-if="issue.detail">{{ issue.detail }}</div>
              </li>
            </ul>
          </div>

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
              {{ t('admin.dashboard.groupUsage.retry') }}
            </button>
          </div>

          <div
            v-else-if="sortedGroups.length === 0"
            class="flex flex-col items-center justify-center gap-2 py-12 text-center"
          >
            <TrendingUp class="h-8 w-8 text-muted-foreground/40" />
            <p class="text-sm text-muted-foreground">{{ t('admin.dashboard.groupUsage.empty') }}</p>
          </div>

          <div v-else class="max-h-[60vh] overflow-y-auto rounded-xl border border-border/60">
            <table class="w-full text-sm">
              <thead class="sticky top-0 z-10 bg-surface/90 backdrop-blur">
                <tr class="border-b border-border/60 text-left text-xs font-medium text-muted-foreground">
                  <th class="px-4 py-3">{{ t('admin.dashboard.groupUsage.columns.groupName') }}</th>
                  <th class="px-4 py-3 text-right">{{ t('admin.dashboard.groupUsage.columns.amount') }}</th>
                </tr>
              </thead>
              <tbody>
                <tr
                  v-for="group in sortedGroups"
                  :key="group.groupName"
                  class="border-b border-border/40 last:border-b-0"
                >
                  <td class="px-4 py-3 align-middle font-medium text-foreground">{{ group.groupName }}</td>
                  <td class="px-4 py-3 align-middle text-right text-foreground">{{ formatCny(group.todayAmount) }}</td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
