<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Ban, CheckCircle2, ChevronDown, ChevronUp, Eye, FileText, History, Loader2, Mail, RefreshCw, Search, Send, Trash2, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { getEmailTemplates } from '../api/settings'
import {
  cancelMassEmailBatch,
  createMassEmailBatch,
  getMassEmailBatch,
  listMassEmailBatches,
  listMassEmailBatchItems,
  listMassEmailUsers,
} from '../api/massEmail'
import type { EmailTemplate } from '../types/settings'
import type {
  CreateMassEmailBatchRequest,
  MassEmailBatch,
  MassEmailBatchItem,
  MassEmailSelectionMode,
  MassEmailSortOrder,
  MassEmailUser,
  MassEmailUserSortBy,
} from '../types/massEmail'

const { t, te, locale } = useI18n()

const users = ref<MassEmailUser[]>([])
const totalUsers = ref(0)
const userPage = ref(1)
const pageSize = ref(20)
const totalUserPages = ref(1)
const statusFilter = ref('')
const roleFilter = ref('')
const searchDraft = ref('')
const searchFilter = ref('')
const sortBy = ref<MassEmailUserSortBy>('created_at')
const sortOrder = ref<MassEmailSortOrder>('desc')
const selectedIds = ref<Set<string>>(new Set())
const selectAllRef = ref<HTMLInputElement | null>(null)

const templates = ref<EmailTemplate[]>([])
const selectedTemplateId = ref('')
const batches = ref<MassEmailBatch[]>([])
const selectedBatch = ref<MassEmailBatch | null>(null)
const batchItems = ref<MassEmailBatchItem[]>([])
const batchItemPage = ref(1)
const batchItemTotalPages = ref(1)
const batchItemTotal = ref(0)

const isLoadingUsers = ref(false)
const isLoadingTemplates = ref(false)
const isLoadingBatches = ref(false)
const isLoadingItems = ref(false)
const isSubmitting = ref(false)
const isCancelling = ref(false)
const errorKey = ref('')
const successKey = ref('')

const confirmOpen = ref(false)
const confirmMode = ref<MassEmailSelectionMode>('selected')
const confirmUserIds = ref<string[]>([])
const confirmTitleKey = ref('')
const confirmDescriptionKey = ref('')
const confirmRequestId = ref('')
const isBatchListOpen = ref(false)
const isBatchDetailOpen = ref(false)
const isPreviewOpen = ref(false)

let pollTimer: number | undefined
let isPollingBatches = false
let usersRequestSequence = 0

const statusOptions = ['active', 'disabled', 'inactive', 'banned'] as const
const roleOptions = ['user', 'admin'] as const
const activeBatchStatuses = ['queued', 'running', 'cancelling']

const selectedTemplate = computed(() => templates.value.find((template) => template.id === selectedTemplateId.value) ?? null)
const currentPageIds = computed(() => users.value.map((user) => user.id))
const currentPageSelectedCount = computed(() => currentPageIds.value.filter((id) => selectedIds.value.has(id)).length)
const isCurrentPageChecked = computed(() => currentPageIds.value.length > 0 && currentPageSelectedCount.value === currentPageIds.value.length)
const isCurrentPageIndeterminate = computed(() => currentPageSelectedCount.value > 0 && currentPageSelectedCount.value < currentPageIds.value.length)
const selectedCount = computed(() => selectedIds.value.size)
const activeBatches = computed(() => batches.value.filter((batch) => activeBatchStatuses.includes(batch.status)))
const hasActiveBatchState = computed(() => (
  activeBatches.value.length > 0 || Boolean(selectedBatch.value && activeBatchStatuses.includes(selectedBatch.value.status))
))
const confirmRecipientCount = computed(() => (confirmMode.value === 'all' ? totalUsers.value : confirmUserIds.value.length))
const timezone = computed(() => Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC')
const previewDocument = computed(() => {
  const htmlBody = selectedTemplate.value?.htmlBody ?? ''
  const policy = "default-src 'none'; style-src 'unsafe-inline'; img-src data: cid:; font-src data:; form-action 'none'; frame-src 'none'; connect-src 'none'"
  const meta = `<meta http-equiv="Content-Security-Policy" content="${policy}"><meta name="referrer" content="no-referrer">`
  if (/<head(?:\s[^>]*)?>/i.test(htmlBody)) {
    return htmlBody.replace(/<head(?:\s[^>]*)?>/i, (head) => `${head}${meta}`)
  }
  return `<!doctype html><html><head>${meta}</head><body>${htmlBody}</body></html>`
})

const formatDateTime = (value?: string | null): string => {
  if (!value) return t('admin.massEmail.common.placeholder')
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return t('admin.massEmail.common.placeholder')
  return new Intl.DateTimeFormat(locale.value, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

const terminalBatchCount = (batch: MassEmailBatch): number => (
  batch.sentCount + batch.failedCount + batch.uncertainCount + batch.cancelledCount
)

const batchProgress = (batch: MassEmailBatch): number => {
  if (!batch.recipientCount) return 0
  return Math.round((terminalBatchCount(batch) / batch.recipientCount) * 100)
}

const batchStatusClass = (status: string): string => {
  switch (status) {
    case 'completed':
      return 'border-emerald-400/30 bg-emerald-500/10 text-emerald-600 dark:text-emerald-300'
    case 'queued':
    case 'running':
      return 'border-sky-400/30 bg-sky-500/10 text-sky-600 dark:text-sky-300'
    case 'cancelling':
    case 'completed_with_errors':
      return 'border-amber-400/30 bg-amber-500/10 text-amber-600 dark:text-amber-300'
    case 'failed':
    case 'cancelled':
      return 'border-rose-400/30 bg-rose-500/10 text-rose-600 dark:text-rose-300'
    default:
      return 'border-border/60 bg-surface-elevated text-muted-foreground'
  }
}

const itemStatusClass = (status: string): string => {
  switch (status) {
    case 'sent':
      return 'text-emerald-600 dark:text-emerald-300'
    case 'sending':
      return 'text-sky-600 dark:text-sky-300'
    case 'failed':
    case 'cancelled':
      return 'text-rose-600 dark:text-rose-300'
    case 'uncertain':
      return 'text-amber-600 dark:text-amber-300'
    default:
      return 'text-muted-foreground'
  }
}

const itemErrorText = (item: MassEmailBatchItem): string => {
  if (!item.errorKey) return formatDateTime(item.finishedAt || item.sentAt || item.claimedAt || item.updatedAt)
  return te(item.errorKey) ? t(item.errorKey) : t('admin.massEmail.errors.itemGeneric')
}

const loadUsers = async () => {
  const requestSequence = ++usersRequestSequence
  isLoadingUsers.value = true
  errorKey.value = ''
  try {
    const response = await listMassEmailUsers({
      page: userPage.value,
      pageSize: pageSize.value,
      status: statusFilter.value,
      role: roleFilter.value,
      search: searchFilter.value,
      sortBy: sortBy.value,
      sortOrder: sortOrder.value,
      timezone: timezone.value,
    })
    if (requestSequence === usersRequestSequence) {
      users.value = response.items
      totalUsers.value = response.total
      userPage.value = response.page
      pageSize.value = response.pageSize
      totalUserPages.value = Math.max(response.totalPages, 1)
    }
  } catch (error) {
    if (requestSequence === usersRequestSequence) {
      errorKey.value = error instanceof Error ? error.message : 'admin.massEmail.errors.unknown'
    }
  } finally {
    if (requestSequence === usersRequestSequence) isLoadingUsers.value = false
  }
}

const loadTemplates = async () => {
  isLoadingTemplates.value = true
  try {
    templates.value = await getEmailTemplates()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.massEmail.errors.templates'
  } finally {
    isLoadingTemplates.value = false
  }
}

const ensureTemplatesLoaded = async () => {
  if (templates.value.length > 0 || isLoadingTemplates.value) return
  await loadTemplates()
}

const loadBatches = async () => {
  isLoadingBatches.value = true
  try {
    const response = await listMassEmailBatches(1, 10)
    batches.value = response.items
    if (selectedBatch.value) {
      const updated = response.items.find((batch) => batch.id === selectedBatch.value?.id)
      if (updated) selectedBatch.value = updated
    }
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.massEmail.errors.unknown'
  } finally {
    isLoadingBatches.value = false
  }
}

const loadBatchItems = async () => {
  if (!selectedBatch.value) return
  isLoadingItems.value = true
  try {
    const response = await listMassEmailBatchItems(selectedBatch.value.id, batchItemPage.value, 20)
    batchItems.value = response.items
    batchItemTotal.value = response.total
    batchItemPage.value = response.page
    batchItemTotalPages.value = Math.max(response.totalPages, 1)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.massEmail.errors.unknown'
  } finally {
    isLoadingItems.value = false
  }
}

const loadBatchDetail = async (batch: MassEmailBatch, openModal = true) => {
  errorKey.value = ''
  try {
    const detail = await getMassEmailBatch(batch.id)
    selectedBatch.value = detail
    batchItemPage.value = 1
    if (openModal) isBatchDetailOpen.value = true
    if (isBatchDetailOpen.value) await loadBatchItems()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.massEmail.errors.unknown'
  } finally {
    startPollingIfNeeded()
  }
}

const refreshAll = async () => {
  await Promise.all([loadUsers(), loadBatches()])
  startPollingIfNeeded()
}

const setFilters = async () => {
  userPage.value = 1
  await loadUsers()
}

const applySearch = async () => {
  searchFilter.value = searchDraft.value.trim()
  userPage.value = 1
  await loadUsers()
}

const clearSearch = async () => {
  searchDraft.value = ''
  searchFilter.value = ''
  userPage.value = 1
  await loadUsers()
}

const changePage = async (nextPage: number) => {
  if (nextPage < 1 || nextPage > totalUserPages.value || isLoadingUsers.value) return
  userPage.value = nextPage
  await loadUsers()
}

const toggleSort = async (field: MassEmailUserSortBy) => {
  if (sortBy.value === field) {
    sortOrder.value = sortOrder.value === 'asc' ? 'desc' : 'asc'
  } else {
    sortBy.value = field
    sortOrder.value = 'asc'
  }
  await loadUsers()
}

const toggleUser = (id: string) => {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

const toggleCurrentPage = () => {
  const next = new Set(selectedIds.value)
  if (isCurrentPageChecked.value) {
    currentPageIds.value.forEach((id) => next.delete(id))
  } else {
    currentPageIds.value.forEach((id) => next.add(id))
  }
  selectedIds.value = next
}

const clearSelection = () => {
  selectedIds.value = new Set()
}

const openSendConfirm = async (mode: MassEmailSelectionMode, userIds: string[]) => {
  if (isSubmitting.value) return
  confirmMode.value = mode
  confirmUserIds.value = userIds
  confirmTitleKey.value = mode === 'all' ? 'admin.massEmail.confirm.allTitle' : 'admin.massEmail.confirm.selectedTitle'
  confirmDescriptionKey.value = mode === 'all' ? 'admin.massEmail.confirm.allDescription' : 'admin.massEmail.confirm.selectedDescription'
  confirmRequestId.value = crypto.randomUUID()
  selectedTemplateId.value = ''
  confirmOpen.value = true
  await ensureTemplatesLoaded()
}

const closeConfirm = () => {
  if (isSubmitting.value) return
  confirmOpen.value = false
  confirmRequestId.value = ''
  selectedTemplateId.value = ''
}

const closePreview = () => {
  isPreviewOpen.value = false
}

const openPreview = () => {
  if (!selectedTemplate.value) return
  isPreviewOpen.value = true
}

const openBatchList = async () => {
  isBatchListOpen.value = true
  await loadBatches()
  startPollingIfNeeded()
}

const closeBatchList = () => {
  isBatchListOpen.value = false
}

const closeBatchDetail = () => {
  isBatchDetailOpen.value = false
}

const selectBatchFromList = async (batch: MassEmailBatch) => {
  isBatchListOpen.value = false
  await loadBatchDetail(batch, true)
}

const confirmSend = async () => {
  if (isSubmitting.value || !selectedTemplateId.value || !confirmRequestId.value) return
  const payload: CreateMassEmailBatchRequest = {
    templateId: selectedTemplateId.value,
    selectionMode: confirmMode.value,
    filters: {
      status: statusFilter.value,
      role: roleFilter.value,
      search: searchFilter.value,
    },
    requestId: confirmRequestId.value,
  }
  if (confirmMode.value === 'selected') {
    payload.userIds = confirmUserIds.value
  }

  isSubmitting.value = true
  errorKey.value = ''
  successKey.value = ''
  try {
    const batch = await createMassEmailBatch(payload)
    successKey.value = 'admin.massEmail.success.created'
    confirmOpen.value = false
    confirmRequestId.value = ''
    selectedTemplateId.value = ''
    await loadBatches()
    selectedBatch.value = batch
    batchItemPage.value = 1
    if (isBatchDetailOpen.value) await loadBatchItems()
    startPollingIfNeeded()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.massEmail.errors.unknown'
  } finally {
    isSubmitting.value = false
  }
}

const cancelBatch = async (batch: MassEmailBatch) => {
  if (isCancelling.value) return
  isCancelling.value = true
  try {
    const updated = await cancelMassEmailBatch(batch.id)
    selectedBatch.value = updated
    successKey.value = 'admin.massEmail.success.cancelled'
    await loadBatches()
    startPollingIfNeeded()
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.massEmail.errors.unknown'
  } finally {
    isCancelling.value = false
  }
}

const stopPolling = () => {
  if (pollTimer === undefined) return
  window.clearInterval(pollTimer)
  pollTimer = undefined
}

const startPollingIfNeeded = () => {
  if (!hasActiveBatchState.value) {
    stopPolling()
    return
  }
  if (pollTimer !== undefined) return
  pollTimer = window.setInterval(() => {
    void pollActiveBatches()
  }, 2000)
}

const pollActiveBatches = async () => {
  if (isPollingBatches) return
  if (!hasActiveBatchState.value) {
    stopPolling()
    return
  }
  const selectedWasActive = Boolean(selectedBatch.value && activeBatchStatuses.includes(selectedBatch.value.status))
  isPollingBatches = true
  try {
    await loadBatches()
    const selectedIsActive = Boolean(selectedBatch.value && activeBatchStatuses.includes(selectedBatch.value.status))
    if (selectedBatch.value && isBatchDetailOpen.value && (selectedIsActive || (selectedWasActive && !selectedIsActive))) {
      await loadBatchItems()
    }
  } finally {
    isPollingBatches = false
    if (hasActiveBatchState.value) startPollingIfNeeded()
    else stopPolling()
  }
}

const closeTopModal = () => {
  if (isSubmitting.value) return
  if (isPreviewOpen.value) {
    closePreview()
    return
  }
  if (isBatchDetailOpen.value) {
    closeBatchDetail()
    return
  }
  if (isBatchListOpen.value) {
    closeBatchList()
    return
  }
  if (confirmOpen.value) closeConfirm()
}

const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') closeTopModal()
}

watch([isCurrentPageChecked, isCurrentPageIndeterminate], async () => {
  await nextTick()
  if (selectAllRef.value) selectAllRef.value.indeterminate = isCurrentPageIndeterminate.value
})

onMounted(() => {
  void refreshAll()
  document.addEventListener('keydown', handleKeydown)
})

onBeforeUnmount(() => {
  stopPolling()
  document.removeEventListener('keydown', handleKeydown)
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-3 overflow-hidden">
    <section class="shrink-0 rounded-lg border border-border/50 bg-card p-3 shadow-sm">
      <div class="flex flex-col gap-3 2xl:flex-row 2xl:items-end 2xl:justify-between">
        <div class="grid flex-1 gap-2 sm:grid-cols-2 lg:grid-cols-[minmax(14rem,1.5fr)_minmax(8rem,1fr)_minmax(8rem,1fr)_auto]">
          <form class="space-y-1 sm:col-span-2 lg:col-span-1" @submit.prevent="applySearch">
            <label for="mass-email-user-search" class="block text-sm font-medium text-foreground">
              {{ t('admin.massEmail.filters.search') }}
            </label>
            <div class="flex gap-2">
              <div class="relative min-w-0 flex-1">
                <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
                <input
                  id="mass-email-user-search"
                  v-model="searchDraft"
                  type="search"
                  class="h-10 w-full rounded-lg border border-border/60 bg-surface py-2 pl-9 pr-9 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
                  :placeholder="t('admin.massEmail.filters.searchPlaceholder')"
                  :aria-label="t('admin.massEmail.filters.search')"
                >
                <button
                  v-if="searchDraft || searchFilter"
                  type="button"
                  class="absolute right-1 top-1/2 inline-flex h-8 w-8 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                  :aria-label="t('admin.massEmail.actions.clearSearch')"
                  :title="t('admin.massEmail.actions.clearSearch')"
                  @click="clearSearch"
                >
                  <X class="h-4 w-4" />
                </button>
              </div>
              <Button type="submit" variant="secondary" class="h-10 shrink-0 rounded-lg px-3" :disabled="isLoadingUsers">
                <Loader2 v-if="isLoadingUsers" class="h-4 w-4 animate-spin" />
                <Search v-else class="h-4 w-4" />
                <span>{{ t('admin.massEmail.actions.search') }}</span>
              </Button>
            </div>
          </form>
          <label class="space-y-1 text-sm font-medium text-foreground">
            <span>{{ t('admin.massEmail.filters.status') }}</span>
            <select v-model="statusFilter" class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary" @change="setFilters">
              <option value="">{{ t('admin.massEmail.filters.allStatuses') }}</option>
              <option v-for="status in statusOptions" :key="status" :value="status">
                {{ t(`admin.massEmail.userStatus.${status}`) }}
              </option>
            </select>
          </label>
          <label class="space-y-1 text-sm font-medium text-foreground">
            <span>{{ t('admin.massEmail.filters.role') }}</span>
            <select v-model="roleFilter" class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary" @change="setFilters">
              <option value="">{{ t('admin.massEmail.filters.allRoles') }}</option>
              <option v-for="role in roleOptions" :key="role" :value="role">
                {{ t(`admin.massEmail.roles.${role}`) }}
              </option>
            </select>
          </label>
          <div class="flex flex-wrap items-end gap-2 sm:col-span-2 lg:col-span-1">
            <Button variant="secondary" class="h-10 rounded-lg px-3" :disabled="isLoadingUsers || isLoadingBatches" @click="refreshAll">
              <Loader2 v-if="isLoadingUsers || isLoadingBatches" class="h-4 w-4 animate-spin" />
              <RefreshCw v-else class="h-4 w-4" />
              <span>{{ t('admin.massEmail.actions.refresh') }}</span>
            </Button>
            <Button variant="secondary" class="h-10 rounded-lg px-3" @click="openBatchList">
              <History class="h-4 w-4" />
              <span>{{ t('admin.massEmail.actions.openBatches') }}</span>
            </Button>
          </div>
        </div>
        <div class="grid gap-2 sm:grid-cols-[minmax(9rem,1.3fr)_minmax(9rem,1.3fr)_minmax(8rem,1fr)_auto] 2xl:w-[42rem]">
          <Button class="h-10 rounded-lg" :disabled="selectedCount === 0 || isSubmitting" @click="openSendConfirm('selected', Array.from(selectedIds))">
            <Send class="h-4 w-4" />
            <span>{{ t('admin.massEmail.actions.sendSelected') }}</span>
          </Button>
          <Button variant="secondary" class="h-10 rounded-lg" :disabled="users.length === 0 || isSubmitting" @click="openSendConfirm('selected', currentPageIds)">
            <Mail class="h-4 w-4" />
            <span>{{ t('admin.massEmail.actions.sendPage') }}</span>
          </Button>
          <Button variant="secondary" class="h-10 rounded-lg" :disabled="totalUsers === 0 || isSubmitting" @click="openSendConfirm('all', [])">
            <CheckCircle2 class="h-4 w-4" />
            <span>{{ t('admin.massEmail.actions.sendFilter') }}</span>
          </Button>
          <Button variant="ghost" class="h-10 rounded-lg" :disabled="selectedCount === 0" @click="clearSelection">
            <Trash2 class="h-4 w-4" />
            <span>{{ t('admin.massEmail.actions.clearSelection') }}</span>
          </Button>
        </div>
      </div>
      <div class="mt-2 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs text-muted-foreground" aria-live="polite">
        <span>{{ t('admin.massEmail.selection.count', { count: selectedCount }) }}</span>
        <span>{{ t('admin.massEmail.pagination.total', { total: totalUsers }) }}</span>
        <span>{{ t('admin.massEmail.pagination.currentPage', { page: userPage, totalPages: totalUserPages }) }}</span>
        <span>{{ t('admin.massEmail.batches.active', { count: activeBatches.length }) }}</span>
      </div>
    </section>

    <div v-if="errorKey" class="shrink-0 flex items-start gap-3 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ t(errorKey) }}</span>
    </div>
    <div v-if="successKey" class="shrink-0 flex items-start gap-3 rounded-lg border border-emerald-400/20 bg-emerald-500/10 p-3 text-sm text-emerald-600 dark:text-emerald-300" role="status">
      <CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ t(successKey) }}</span>
    </div>

    <section class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border/50 bg-card shadow-sm">
      <div class="flex shrink-0 flex-col gap-2 border-b border-border/50 p-3 sm:flex-row sm:items-center sm:justify-between">
        <div class="min-w-0">
          <h2 class="truncate text-base font-semibold text-foreground">{{ t('admin.massEmail.users.title') }}</h2>
          <p class="text-sm text-muted-foreground">{{ t('admin.massEmail.pagination.pageSize', { pageSize }) }}</p>
        </div>
        <div class="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
          <Button variant="secondary" size="sm" class="rounded-lg" :disabled="userPage <= 1 || isLoadingUsers" @click="changePage(userPage - 1)">
            {{ t('admin.massEmail.pagination.previous') }}
          </Button>
          <span>{{ t('admin.massEmail.pagination.currentPage', { page: userPage, totalPages: totalUserPages }) }}</span>
          <Button variant="secondary" size="sm" class="rounded-lg" :disabled="userPage >= totalUserPages || isLoadingUsers" @click="changePage(userPage + 1)">
            {{ t('admin.massEmail.pagination.next') }}
          </Button>
        </div>
      </div>

      <div v-if="isLoadingUsers" class="flex flex-1 items-center justify-center py-16 text-muted-foreground" role="status">
        <Loader2 class="mr-2 h-5 w-5 animate-spin" />
        {{ t('admin.massEmail.status.loadingUsers') }}
      </div>
      <div v-else-if="users.length === 0" class="flex flex-1 flex-col items-center justify-center py-16 text-center text-muted-foreground">
        <Mail class="mb-3 h-10 w-10 opacity-40" />
        <p class="font-medium text-foreground">{{ t('admin.massEmail.empty.usersTitle') }}</p>
        <p class="text-sm">{{ t('admin.massEmail.empty.usersDescription') }}</p>
      </div>
      <div v-else class="min-h-0 flex-1 overflow-auto">
        <table class="w-full min-w-[760px] text-left text-sm">
          <thead class="sticky top-0 z-10 bg-surface text-xs uppercase text-muted-foreground">
            <tr>
              <th class="w-12 px-4 py-3">
                <input ref="selectAllRef" type="checkbox" class="h-4 w-4 rounded border-border bg-surface text-primary focus:ring-2 focus:ring-primary" :checked="isCurrentPageChecked" :aria-label="t('admin.massEmail.selection.selectPage')" @change="toggleCurrentPage" />
              </th>
              <th class="px-4 py-3">
                <button type="button" class="inline-flex items-center gap-1 font-semibold outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleSort('email')">
                  {{ t('admin.massEmail.fields.email') }}
                  <ChevronUp v-if="sortBy === 'email' && sortOrder === 'asc'" class="h-3 w-3" />
                  <ChevronDown v-else class="h-3 w-3" />
                </button>
              </th>
              <th class="px-4 py-3">{{ t('admin.massEmail.fields.role') }}</th>
              <th class="px-4 py-3">{{ t('admin.massEmail.fields.status') }}</th>
              <th class="px-4 py-3">
                <button type="button" class="inline-flex items-center gap-1 font-semibold outline-none focus-visible:ring-2 focus-visible:ring-primary" @click="toggleSort('created_at')">
                  {{ t('admin.massEmail.fields.createdAt') }}
                  <ChevronUp v-if="sortBy === 'created_at' && sortOrder === 'asc'" class="h-3 w-3" />
                  <ChevronDown v-else class="h-3 w-3" />
                </button>
              </th>
              <th class="px-4 py-3 text-right">{{ t('admin.massEmail.fields.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/50">
            <tr v-for="user in users" :key="user.id" class="hover:bg-surface/70">
              <td class="px-4 py-3">
                <input type="checkbox" class="h-4 w-4 rounded border-border bg-surface text-primary focus:ring-2 focus:ring-primary" :checked="selectedIds.has(user.id)" :aria-label="t('admin.massEmail.selection.selectUser', { email: user.email })" @change="toggleUser(user.id)" />
              </td>
              <td class="px-4 py-3">
                <div class="font-medium text-foreground">{{ user.email }}</div>
                <div class="text-xs text-muted-foreground">{{ user.name || user.username || user.id }}</div>
              </td>
              <td class="px-4 py-3 text-muted-foreground">{{ t(`admin.massEmail.roles.${user.role}`) }}</td>
              <td class="px-4 py-3 text-muted-foreground">{{ t(`admin.massEmail.userStatus.${user.status}`) }}</td>
              <td class="px-4 py-3 text-muted-foreground">{{ formatDateTime(user.createdAt) }}</td>
              <td class="px-4 py-3 text-right">
                <Button size="sm" class="rounded-lg" :disabled="isSubmitting" @click="openSendConfirm('selected', [user.id])">
                  <Send class="h-4 w-4" />
                  {{ t('admin.massEmail.actions.sendRow') }}
                </Button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>

    <div v-if="confirmOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" :aria-labelledby="'mass-email-confirm-title'">
      <div class="flex max-h-[calc(100vh-2rem)] w-full max-w-xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
        <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
          <div class="min-w-0">
            <h2 id="mass-email-confirm-title" class="truncate text-lg font-semibold text-foreground">{{ t(confirmTitleKey) }}</h2>
            <p class="mt-1 text-sm text-muted-foreground">{{ t(confirmDescriptionKey, { count: confirmRecipientCount }) }}</p>
          </div>
          <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.massEmail.actions.closeConfirm')" :disabled="isSubmitting" @click="closeConfirm">
            <X class="h-4 w-4" />
          </button>
        </div>
        <div class="min-h-0 flex-1 space-y-3 overflow-y-auto p-4">
          <div class="rounded-lg bg-surface p-3 text-sm text-muted-foreground">
            <p>{{ t('admin.massEmail.confirm.recipients', { count: confirmRecipientCount }) }}</p>
            <p>{{ t('admin.massEmail.confirm.filters', { status: statusFilter ? t(`admin.massEmail.userStatus.${statusFilter}`) : t('admin.massEmail.filters.allStatuses'), role: roleFilter ? t(`admin.massEmail.roles.${roleFilter}`) : t('admin.massEmail.filters.allRoles'), search: searchFilter || t('admin.massEmail.filters.noSearch') }) }}</p>
          </div>
          <label class="space-y-1 text-sm font-medium text-foreground">
            <span>{{ t('admin.massEmail.template.label') }}</span>
            <select v-model="selectedTemplateId" class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary" :disabled="isLoadingTemplates">
              <option value="">{{ t('admin.massEmail.template.placeholder') }}</option>
              <option v-for="template in templates" :key="template.id" :value="template.id">
                {{ template.name }}
              </option>
            </select>
          </label>
          <div class="rounded-lg border border-border/50 bg-surface p-3 text-sm text-muted-foreground">
            <span class="font-medium text-foreground">{{ selectedTemplate?.subject || t('admin.massEmail.template.noSubject') }}</span>
          </div>
        </div>
        <div class="flex shrink-0 flex-col-reverse gap-2 border-t border-border/50 p-4 sm:flex-row sm:justify-end">
          <Button variant="secondary" class="rounded-lg" :disabled="isSubmitting" @click="closeConfirm">{{ t('admin.massEmail.confirm.cancel') }}</Button>
          <Button variant="secondary" class="rounded-lg" :disabled="!selectedTemplate" @click="openPreview">
            <Eye class="h-4 w-4" />
            {{ t('admin.massEmail.actions.previewTemplate') }}
          </Button>
          <Button class="rounded-lg" :disabled="isSubmitting || !selectedTemplateId" @click="confirmSend">
            <Loader2 v-if="isSubmitting" class="h-4 w-4 animate-spin" />
            <Send v-else class="h-4 w-4" />
            {{ t('admin.massEmail.confirm.submit') }}
          </Button>
        </div>
      </div>
    </div>

    <div v-if="isPreviewOpen" class="fixed inset-0 z-[60] flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" :aria-labelledby="'mass-email-preview-title'">
      <div class="flex max-h-[calc(100vh-2rem)] w-full max-w-4xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
        <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
          <div class="min-w-0">
            <h2 id="mass-email-preview-title" class="truncate text-lg font-semibold text-foreground">{{ t('admin.massEmail.preview.title') }}</h2>
            <p class="truncate text-sm text-muted-foreground">{{ selectedTemplate?.subject || t('admin.massEmail.template.noSubject') }}</p>
          </div>
          <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.massEmail.preview.close')" @click="closePreview">
            <X class="h-4 w-4" />
          </button>
        </div>
        <div class="min-h-0 flex-1 overflow-auto p-4">
          <iframe :srcdoc="previewDocument" sandbox="" referrerpolicy="no-referrer" :title="t('admin.massEmail.preview.iframeTitle')" class="h-[70vh] min-h-[22rem] w-full bg-white" />
        </div>
      </div>
    </div>

    <div v-if="isBatchListOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" :aria-labelledby="'mass-email-batches-title'">
      <div class="flex max-h-[calc(100vh-2rem)] w-full max-w-3xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
        <div class="flex shrink-0 items-center justify-between gap-4 border-b border-border/50 p-4">
          <h2 id="mass-email-batches-title" class="text-lg font-semibold text-foreground">{{ t('admin.massEmail.batches.title') }}</h2>
          <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.massEmail.batches.close')" @click="closeBatchList">
            <X class="h-4 w-4" />
          </button>
        </div>
        <div class="min-h-0 flex-1 overflow-y-auto">
          <div v-if="batches.length === 0" class="py-10 text-center text-sm text-muted-foreground">{{ t('admin.massEmail.empty.batches') }}</div>
          <div v-else class="divide-y divide-border/50">
            <div v-for="batch in batches" :key="batch.id" class="p-4 transition hover:bg-surface/70">
              <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="truncate font-medium text-foreground">{{ batch.templateName || batch.templateSubject || batch.templateId }}</span>
                    <span class="rounded-full border px-2 py-0.5 text-xs font-medium" :class="batchStatusClass(batch.status)">{{ t(`admin.massEmail.batchStatus.${batch.status}`) }}</span>
                  </div>
                  <p class="mt-1 text-xs text-muted-foreground">{{ formatDateTime(batch.createdAt) }}</p>
                </div>
                <div class="text-sm text-muted-foreground">{{ t('admin.massEmail.batches.progress', { done: terminalBatchCount(batch), total: batch.recipientCount, percent: batchProgress(batch) }) }}</div>
              </div>
              <div class="mt-3 h-2 overflow-hidden rounded-full bg-surface-line">
                <div class="h-full bg-primary transition-all" :style="{ width: `${batchProgress(batch)}%` }" />
              </div>
              <div class="mt-3 flex justify-end border-t border-border/50 pt-3">
                <Button variant="secondary" size="sm" class="rounded-lg" @click="selectBatchFromList(batch)">
                  <FileText class="h-4 w-4" />
                  <span>{{ t('admin.massEmail.actions.openBatchDetail') }}</span>
                </Button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <div v-if="isBatchDetailOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" :aria-labelledby="'mass-email-detail-title'">
      <div class="flex max-h-[calc(100vh-2rem)] w-full max-w-3xl flex-col rounded-lg border border-border/60 bg-card shadow-xl">
        <div class="flex shrink-0 items-center justify-between gap-4 border-b border-border/50 p-4">
          <h2 id="mass-email-detail-title" class="text-lg font-semibold text-foreground">{{ t('admin.massEmail.detail.title') }}</h2>
          <div class="flex items-center gap-2">
            <Button v-if="selectedBatch && activeBatchStatuses.includes(selectedBatch.status)" variant="destructive" size="sm" class="rounded-lg" :disabled="isCancelling" @click="cancelBatch(selectedBatch)">
              <Ban class="h-4 w-4" />
              {{ t('admin.massEmail.actions.cancelBatch') }}
            </Button>
            <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.massEmail.detail.close')" @click="closeBatchDetail">
              <X class="h-4 w-4" />
            </button>
          </div>
        </div>
        <div v-if="!selectedBatch" class="py-10 text-center text-sm text-muted-foreground">{{ t('admin.massEmail.empty.detail') }}</div>
        <div v-else class="flex min-h-0 flex-1 flex-col gap-3 overflow-hidden p-4">
          <div class="grid shrink-0 grid-cols-2 gap-2 text-sm sm:grid-cols-4">
            <div class="rounded-lg bg-surface p-3"><p class="text-xs text-muted-foreground">{{ t('admin.massEmail.summary.sent') }}</p><p class="font-semibold text-foreground">{{ selectedBatch.sentCount }}</p></div>
            <div class="rounded-lg bg-surface p-3"><p class="text-xs text-muted-foreground">{{ t('admin.massEmail.summary.failed') }}</p><p class="font-semibold text-foreground">{{ selectedBatch.failedCount }}</p></div>
            <div class="rounded-lg bg-surface p-3"><p class="text-xs text-muted-foreground">{{ t('admin.massEmail.summary.uncertain') }}</p><p class="font-semibold text-foreground">{{ selectedBatch.uncertainCount }}</p></div>
            <div class="rounded-lg bg-surface p-3"><p class="text-xs text-muted-foreground">{{ t('admin.massEmail.summary.cancelled') }}</p><p class="font-semibold text-foreground">{{ selectedBatch.cancelledCount }}</p></div>
          </div>
          <div class="flex shrink-0 flex-col gap-2 text-sm text-muted-foreground sm:flex-row sm:items-center sm:justify-between">
            <span>{{ t('admin.massEmail.detail.recipients', { total: batchItemTotal }) }}</span>
            <div class="flex flex-wrap items-center gap-2">
              <Button variant="secondary" size="sm" class="rounded-lg" :disabled="batchItemPage <= 1 || isLoadingItems" @click="batchItemPage--; loadBatchItems()">{{ t('admin.massEmail.pagination.previous') }}</Button>
              <span>{{ t('admin.massEmail.pagination.currentPage', { page: batchItemPage, totalPages: batchItemTotalPages }) }}</span>
              <Button variant="secondary" size="sm" class="rounded-lg" :disabled="batchItemPage >= batchItemTotalPages || isLoadingItems" @click="batchItemPage++; loadBatchItems()">{{ t('admin.massEmail.pagination.next') }}</Button>
            </div>
          </div>
          <div v-if="isLoadingItems" class="flex flex-1 items-center justify-center py-8 text-sm text-muted-foreground"><Loader2 class="mr-2 h-4 w-4 animate-spin" />{{ t('admin.massEmail.status.loadingItems') }}</div>
          <div v-else class="min-h-0 flex-1 overflow-y-auto divide-y divide-border/50 rounded-lg border border-border/50">
            <div v-for="item in batchItems" :key="item.id" class="p-3 text-sm">
              <div class="flex items-center justify-between gap-3">
                <span class="min-w-0 truncate font-medium text-foreground">{{ item.recipientEmail }}</span>
                <span class="shrink-0 font-medium" :class="itemStatusClass(item.status)">{{ t(`admin.massEmail.itemStatus.${item.status}`) }}</span>
              </div>
              <p class="mt-1 text-xs text-muted-foreground">{{ item.username || item.upstreamUserId || item.id }}</p>
              <p class="mt-1 text-xs text-muted-foreground">{{ itemErrorText(item) }}</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>
