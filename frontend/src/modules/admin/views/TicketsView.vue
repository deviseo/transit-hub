<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Inbox, Loader2, RefreshCw, Settings2 } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { useTickets } from '../composables/useTickets'
import TicketDetailDrawer from '../components/tickets/TicketDetailDrawer.vue'
import TicketEmbedConfigModal from '../components/tickets/TicketEmbedConfigModal.vue'
import Sub2apiUserProfileModal from '../components/tickets/Sub2apiUserProfileModal.vue'
import type { AdminTicketListItem } from '../types/tickets'

const { t, locale } = useI18n()

const {
  tickets,
  total,
  page,
  pageSize,
  totalPages,
  statusFilter,
  isLoading,
  errorKey,
  loadTickets,
  setStatusFilter,
  goToPage,
} = useTickets()

const detailTicketId = ref<string | null>(null)
const isDetailOpen = ref(false)
const isEmbedConfigOpen = ref(false)
const profileTicketId = ref<string | null>(null)
const isProfileOpen = ref(false)

const statusOptions = ['open', 'pending', 'replied', 'closed'] as const

const openDetail = (ticket: AdminTicketListItem) => {
  detailTicketId.value = ticket.id
  isDetailOpen.value = true
}

const closeDetail = () => {
  isDetailOpen.value = false
  detailTicketId.value = null
}

const openProfile = (ticket: AdminTicketListItem) => {
  profileTicketId.value = ticket.id
  isProfileOpen.value = true
}

const closeProfile = () => {
  isProfileOpen.value = false
  profileTicketId.value = null
}

const handleUpdated = async () => {
  await loadTickets()
}

const handleStatusChange = async (event: Event) => {
  const target = event.target as HTMLSelectElement
  await setStatusFilter(target.value)
}

const formatDateTime = (value: string | null): string => {
  if (!value) return t('admin.tickets.common.placeholder')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('admin.tickets.common.placeholder')
  return new Intl.DateTimeFormat(locale.value, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const statusBadgeClass = (status: string): string => {
  switch (status) {
    case 'open':
      return 'border-sky-400/30 bg-sky-500/10 text-sky-600 dark:text-sky-300'
    case 'pending':
      return 'border-amber-400/30 bg-amber-500/10 text-amber-600 dark:text-amber-300'
    case 'replied':
      return 'border-emerald-400/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'
    case 'closed':
      return 'border-border/60 bg-surface-elevated text-muted-foreground'
    default:
      return 'border-border/60 bg-surface-elevated text-muted-foreground'
  }
}

const canGoPrevious = () => page.value > 1 && !isLoading.value
const canGoNext = () => page.value < totalPages.value && !isLoading.value
</script>

<template>
  <div class="flex flex-col gap-6">
    <div class="flex flex-col space-y-4">
      <div class="flex flex-col sm:flex-row sm:items-center justify-between gap-4 shrink-0">
        <div class="flex items-center gap-3 w-full sm:w-auto flex-1">
          <div class="relative w-full sm:w-48">
            <select
              :value="statusFilter"
              class="h-10 w-full rounded-xl border border-border/50 bg-surface px-3 pr-8 text-sm text-foreground outline-none appearance-none transition-all focus:border-primary focus:ring-1 focus:ring-primary"
              @change="handleStatusChange"
            >
              <option value="">{{ t('admin.tickets.tabs.all') }}</option>
              <option v-for="status in statusOptions" :key="status" :value="status">
                {{ t(`admin.tickets.status.${status}`) }}
              </option>
            </select>
            <div class="absolute right-3 top-1/2 -translate-y-1/2 pointer-events-none text-muted-foreground">
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m6 9 6 6 6-6"/></svg>
            </div>
          </div>
        </div>

        <div class="flex items-center gap-2 shrink-0">
          <Button variant="secondary" class="h-10 rounded-xl gap-2" @click="isEmbedConfigOpen = true">
            <Settings2 class="h-4 w-4" />
            {{ t('admin.tickets.actions.embedSettings') }}
          </Button>
          <Button variant="secondary" class="h-10 rounded-xl gap-2" :disabled="isLoading" @click="loadTickets">
            <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('admin.tickets.actions.refresh') }}
          </Button>
        </div>
      </div>

      <div v-if="errorKey" class="flex items-start gap-3 rounded-2xl border border-warning/20 bg-warning/10 p-4 text-sm text-warning shrink-0">
        <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
        <span>{{ t(errorKey) }}</span>
      </div>

      <div class="min-h-0 overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm flex flex-col">
        <div v-if="isLoading" class="flex items-center justify-center py-16 text-muted-foreground">
          <Loader2 class="mr-2 h-5 w-5 animate-spin" />
          {{ t('admin.tickets.status.loading') }}
        </div>

        <div v-else-if="tickets.length === 0" class="flex flex-col items-center justify-center px-6 py-16 text-center">
          <div class="flex h-12 w-12 items-center justify-center rounded-2xl border border-border/50 bg-surface-elevated text-muted-foreground">
            <Inbox class="h-5 w-5" />
          </div>
          <h3 class="mt-4 font-semibold text-foreground">{{ t('admin.tickets.empty.title') }}</h3>
          <p class="mt-2 max-w-sm text-sm text-muted-foreground">{{ t('admin.tickets.empty.description') }}</p>
        </div>

        <div v-else class="flex-1 overflow-auto">
          <table class="w-full min-w-[980px] text-left text-sm relative">
            <thead class="sticky top-0 z-10 border-b border-border/50 bg-surface-elevated/90 backdrop-blur-sm">
              <tr>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.tickets.fields.title') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.tickets.fields.status') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.tickets.fields.category') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.tickets.fields.priority') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.tickets.fields.manualEmail') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.tickets.fields.sub2apiUser') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.tickets.fields.sub2apiSrcHost') }}</th>
                <th class="px-6 py-3 font-medium text-muted-foreground">{{ t('admin.tickets.fields.lastMessageAt') }}</th>
                <th class="px-6 py-3 text-right font-medium text-muted-foreground">{{ t('admin.tickets.fields.actions') }}</th>
              </tr>
            </thead>
            <tbody class="divide-y divide-border/50">
              <tr v-for="ticket in tickets" :key="ticket.id" class="transition-colors hover:bg-surface/30">
                <td class="px-6 py-2.5 font-medium text-foreground">{{ ticket.title }}</td>
                <td class="px-6 py-2.5">
                  <span :class="['inline-flex rounded-md border px-2 py-1 text-xs font-semibold', statusBadgeClass(ticket.status)]">
                    {{ t(`admin.tickets.status.${ticket.status}`) }}
                  </span>
                </td>
                <td class="px-6 py-2.5">
                  <span class="inline-flex rounded-md border border-border/60 bg-surface-elevated px-2 py-1 text-xs text-foreground">{{ ticket.category }}</span>
                </td>
                <td class="px-6 py-2.5">
                  <span class="inline-flex rounded-md border border-border/60 bg-surface-elevated px-2 py-1 text-xs text-foreground">{{ ticket.priority }}</span>
                </td>
                <td class="px-6 py-2.5 text-muted-foreground">{{ ticket.manualEmail }}</td>
                <td class="px-6 py-2.5 text-muted-foreground">
                  <button
                    v-if="ticket.sub2apiUserId"
                    type="button"
                    class="flex flex-col text-left underline decoration-dotted underline-offset-2 transition-colors hover:text-foreground"
                    @click="openProfile(ticket)"
                  >
                    <span>{{ ticket.sub2apiEmail || ticket.sub2apiUserId }}</span>
                    <span v-if="ticket.sub2apiRole" class="text-xs">{{ ticket.sub2apiRole }}</span>
                  </button>
                  <span v-else>{{ t('admin.tickets.common.placeholder') }}</span>
                </td>
                <td class="px-6 py-2.5 text-muted-foreground truncate max-w-[220px]" :title="ticket.sub2apiSrcHost">{{ ticket.sub2apiSrcHost || t('admin.tickets.common.placeholder') }}</td>
                <td class="px-6 py-2.5 text-muted-foreground">{{ formatDateTime(ticket.lastMessageAt) }}</td>
                <td class="px-6 py-2.5 text-right">
                  <Button variant="secondary" size="sm" @click="openDetail(ticket)">
                    {{ t('admin.tickets.actions.viewDetail') }}
                  </Button>
                </td>
              </tr>
            </tbody>
          </table>
        </div>

        <div class="flex flex-col gap-3 border-t border-border/50 bg-surface-elevated/30 px-4 py-4 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
          <div class="flex flex-wrap items-center gap-x-4 gap-y-1">
            <span>{{ t('admin.tickets.pagination.total', { total }) }}</span>
            <span>{{ t('admin.tickets.pagination.pageSize', { pageSize }) }}</span>
            <span>{{ t('admin.tickets.pagination.currentPage', { page, totalPages }) }}</span>
          </div>

          <div class="flex items-center gap-2">
            <Button variant="secondary" size="sm" :disabled="!canGoPrevious()" @click="goToPage(page - 1)">
              {{ t('admin.tickets.pagination.previous') }}
            </Button>
            <Button variant="secondary" size="sm" :disabled="!canGoNext()" @click="goToPage(page + 1)">
              {{ t('admin.tickets.pagination.next') }}
            </Button>
          </div>
        </div>
      </div>
    </div>

    <TicketDetailDrawer
      :open="isDetailOpen"
      :ticket-id="detailTicketId"
      @close="closeDetail"
      @updated="handleUpdated"
    />

    <TicketEmbedConfigModal :open="isEmbedConfigOpen" @close="isEmbedConfigOpen = false" />

    <Sub2apiUserProfileModal :open="isProfileOpen" :ticket-id="profileTicketId" @close="closeProfile" />
  </div>
</template>
