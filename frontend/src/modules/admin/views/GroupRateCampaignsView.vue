<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { AlertCircle, Loader2, Megaphone, Plus, RefreshCw } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { useGroupRateCampaigns } from '../composables/useGroupRateCampaigns'
import CampaignEditorDrawer from '../components/group-rate-campaigns/CampaignEditorDrawer.vue'
import CampaignDetailDrawer from '../components/group-rate-campaigns/CampaignDetailDrawer.vue'
import type { CampaignDetail, CampaignListItem } from '../types/groupRateCampaigns'

import { t, locale } from '@/locales'
const route = useRoute()
const router = useRouter()

const {
  campaigns,
  total,
  page,
  pageSize,
  totalPages,
  statusFilter,
  notifyDefaults,
  isLoading,
  errorKey,
  loadCampaigns,
  setStatusFilter,
  goToPage,
  startCampaign,
  endCampaign,
  cancelCampaign,
} = useGroupRateCampaigns()

const isEditorOpen = ref(false)
const detailCampaignId = ref<string | null>(null)
const isDetailOpen = ref(false)

const statusOptions = ['draft', 'scheduled', 'running', 'ending', 'ended', 'partial', 'failed', 'cancelled'] as const

const openEditor = () => {
  isEditorOpen.value = true
}

const closeEditor = () => {
  isEditorOpen.value = false
}

const handleCreated = async (_detail: CampaignDetail) => {
  isEditorOpen.value = false
  await loadCampaigns()
}

const openDetail = (campaign: CampaignListItem) => {
  detailCampaignId.value = campaign.id
  isDetailOpen.value = true
}

const closeDetail = () => {
  isDetailOpen.value = false
  detailCampaignId.value = null
}

const handleStart = async (id: string) => {
  await startCampaign(id)
  closeDetail()
}

const handleEnd = async (id: string) => {
  await endCampaign(id)
  closeDetail()
}

const handleCancel = async (id: string) => {
  await cancelCampaign(id)
  closeDetail()
}

const handleStatusChange = async (event: Event) => {
  const target = event.target as HTMLSelectElement
  await setStatusFilter(target.value)
}

const formatDateTime = (value: string | null): string => {
  if (!value) return t('admin.groupRateCampaigns.common.placeholder')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('admin.groupRateCampaigns.common.placeholder')
  return new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const statusBadgeClass = (status: string): string => {
  switch (status) {
    case 'running':
      return 'border-emerald-400/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'
    case 'scheduled':
      return 'border-sky-400/30 bg-sky-500/10 text-sky-600 dark:text-sky-300'
    case 'ended':
      return 'border-border/60 bg-surface-elevated text-muted-foreground'
    case 'partial':
    case 'ending':
      return 'border-amber-400/30 bg-amber-500/10 text-amber-600 dark:text-amber-300'
    case 'failed':
    case 'cancelled':
      return 'border-rose-400/30 bg-rose-500/10 text-rose-600 dark:text-rose-300'
    default:
      return 'border-border/60 bg-surface-elevated text-muted-foreground'
  }
}

const canGoPrevious = () => page.value > 1 && !isLoading.value
const canGoNext = () => page.value < totalPages.value && !isLoading.value

onMounted(() => {
  if (route.query.action === 'create') {
    openEditor()
    router.replace({ path: route.path })
  }
})
</script>

<template>
  <div class="flex min-h-[calc(100dvh-8rem)] flex-col space-y-6 lg:h-[calc(100dvh-8rem)]">
    <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 shrink-0">
      <div class="flex items-center gap-3 w-full sm:w-auto flex-1">
        <div class="relative w-full sm:w-48">
          <select
            :value="statusFilter"
            class="h-10 w-full rounded-xl border border-border/50 bg-surface px-3 pr-8 text-sm text-foreground outline-none appearance-none transition-all focus:border-primary focus:ring-1 focus:ring-primary"
            @change="handleStatusChange"
          >
            <option value="">{{ t('admin.groupRateCampaigns.tabs.all') }}</option>
            <option v-for="status in statusOptions" :key="status" :value="status">
              {{ t(`admin.groupRateCampaigns.status.${status}`) }}
            </option>
          </select>
          <div class="absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none text-muted-foreground">
            <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>
          </div>
        </div>
      </div>

      <div class="flex items-center gap-2 shrink-0">
        <Button variant="secondary" class="h-10 rounded-xl gap-2" :disabled="isLoading" @click="loadCampaigns">
          <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" />
          <RefreshCw v-else class="h-4 w-4" />
          {{ t('admin.groupRateCampaigns.actions.refresh') }}
        </Button>
        <Button class="h-10 rounded-xl gap-2 bg-primary text-primary-foreground hover:bg-primary/90 shadow-sm" @click="openEditor">
          <Plus class="h-4 w-4" />
          {{ t('admin.groupRateCampaigns.actions.create') }}
        </Button>
      </div>
    </div>

    <div v-if="errorKey" class="flex items-start gap-3 rounded-2xl border border-warning/20 bg-warning/10 p-4 text-sm text-warning shrink-0">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ t(errorKey) }}</span>
    </div>

    <div class="flex-1 min-h-0 overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm flex flex-col">
      <div v-if="isLoading" class="flex flex-1 items-center justify-center text-muted-foreground">
        <Loader2 class="mr-2 h-5 w-5 animate-spin" />
        {{ t('admin.groupRateCampaigns.status.loading') }}
      </div>

      <div v-else-if="campaigns.length === 0" class="flex flex-1 flex-col items-center justify-center px-6 text-center">
        <div class="flex h-12 w-12 items-center justify-center rounded-2xl border border-border/50 bg-surface-elevated text-muted-foreground">
          <Megaphone class="h-5 w-5" />
        </div>
        <h3 class="mt-4 font-semibold text-foreground">{{ t('admin.groupRateCampaigns.empty.title') }}</h3>
        <p class="mt-2 max-w-sm text-sm text-muted-foreground">{{ t('admin.groupRateCampaigns.empty.description') }}</p>
      </div>

      <div v-else class="flex-1 overflow-auto">
        <table class="w-full min-w-[980px] text-left text-sm relative">
          <thead class="sticky top-0 z-10 border-b border-border/50 bg-surface-elevated/90 backdrop-blur-sm">
            <tr>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRateCampaigns.fields.name') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRateCampaigns.fields.status') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRateCampaigns.fields.startAt') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRateCampaigns.fields.endAt') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRateCampaigns.fields.summary') }}</th>
              <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.groupRateCampaigns.fields.createdBy') }}</th>
              <th class="px-6 py-3 text-right font-medium text-muted-foreground">{{ t('admin.groupRateCampaigns.fields.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/50">
            <tr v-for="campaign in campaigns" :key="campaign.id" class="transition-colors hover:bg-surface/30">
              <td class="px-6 py-2.5 font-medium text-foreground">{{ campaign.name }}</td>
              <td class="px-6 py-2.5">
                <span :class="['inline-flex rounded-md border px-2 py-1 text-xs font-semibold', statusBadgeClass(campaign.status)]">
                  {{ t(`admin.groupRateCampaigns.status.${campaign.status}`) }}
                </span>
              </td>
              <td class="px-6 py-2.5 text-muted-foreground">{{ formatDateTime(campaign.startedAt ?? campaign.startAt) }}</td>
              <td class="px-6 py-2.5 text-muted-foreground">{{ formatDateTime(campaign.endedAt ?? campaign.endAt) }}</td>
              <td class="px-6 py-2.5 text-muted-foreground">
                {{ t('admin.groupRateCampaigns.format.summary', { applied: campaign.summary.applied, total: campaign.summary.total }) }}
              </td>
              <td class="px-6 py-2.5 text-muted-foreground">{{ campaign.createdBy }}</td>
              <td class="px-6 py-2.5 text-right">
                <Button variant="secondary" size="sm" @click="openDetail(campaign)">
                  {{ t('admin.groupRateCampaigns.actions.viewDetail') }}
                </Button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="flex flex-col gap-3 border-t border-border/50 bg-surface-elevated/30 px-4 py-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
          <span>{{ t('admin.groupRateCampaigns.pagination.total', { total }) }}</span>
          <span>{{ t('admin.groupRateCampaigns.pagination.pageSize', { pageSize }) }}</span>
          <span>{{ t('admin.groupRateCampaigns.pagination.currentPage', { page, totalPages }) }}</span>
        </div>

        <div class="flex items-center gap-2">
          <Button variant="secondary" size="sm" :disabled="!canGoPrevious()" @click="goToPage(page - 1)">
            {{ t('admin.groupRateCampaigns.pagination.previous') }}
          </Button>
          <Button variant="secondary" size="sm" :disabled="!canGoNext()" @click="goToPage(page + 1)">
            {{ t('admin.groupRateCampaigns.pagination.next') }}
          </Button>
        </div>
      </div>
    </div>

    <CampaignEditorDrawer
      :open="isEditorOpen"
      :notify-defaults="notifyDefaults"
      @close="closeEditor"
      @created="handleCreated"
    />

    <CampaignDetailDrawer
      :open="isDetailOpen"
      :campaign-id="detailCampaignId"
      @close="closeDetail"
      @start="handleStart"
      @end="handleEnd"
      @cancel="handleCancel"
    />
  </div>
</template>
