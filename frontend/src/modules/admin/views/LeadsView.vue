<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { AlertCircle, CheckCircle2, Contact, Loader2, Pencil, Plus, RefreshCw, Search, Send, Settings2, Trash2, Upload, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import LeadFormDialog from '../components/leads/LeadFormDialog.vue'
import LeadImportDialog from '../components/leads/LeadImportDialog.vue'
import LeadSendDialog from '../components/leads/LeadSendDialog.vue'
import { deleteLeads, getLeadSettings, listLeadSources, listLeads, saveLeadSettings } from '../api/leads'
import { LEAD_NO_SOURCE, type Lead, type LeadImportResult, type LeadSelection, type LeadSettings, type LeadSourceCount, type LeadStatus, type LeadStatusCounts } from '../types/leads'
import type { MassEmailBatch } from '../types/massEmail'

// 线索：站长自己收集的潜在用户邮箱。单个添加（带来源）或批量导入，按来源、状态筛选，再批量发邮件。
// 邮件走群发邮件的发送队列；已退订、退信的线索不会再收到邮件。
const { t, te, locale } = useI18n()

const pageSize = 20
const leads = ref<Lead[]>([])
const counts = ref<LeadStatusCounts>({ all: 0, notSent: 0, sent: 0, unsubscribed: 0, bounced: 0 })
const total = ref(0)
const pages = ref(0)
const page = ref(1)
const statusFilter = ref<'' | LeadStatus>('')
const sourceFilter = ref('')
const searchDraft = ref('')
const searchFilter = ref('')
const sources = ref<LeadSourceCount[]>([])
const settings = ref<LeadSettings | null>(null)
const selectedIds = ref(new Set<string>())
const isLoading = ref(false)
const errorKey = ref('')
const notice = ref('')
const createdBatch = ref<MassEmailBatch | null>(null)

const formLead = ref<Lead | 'new' | null>(null)
const isImportOpen = ref(false)
const sendTarget = ref<{ selection: LeadSelection; summary: string } | null>(null)
const deleteTarget = ref<string[] | null>(null)
const isDeleting = ref(false)
const isLimitOpen = ref(false)
const limitDraft = ref(200)
const isSavingLimit = ref(false)
const limitErrorKey = ref('')

const statusTabs = computed<{ value: '' | LeadStatus; count: number }[]>(() => [
  { value: '', count: counts.value.all },
  { value: 'not_sent', count: counts.value.notSent },
  { value: 'sent', count: counts.value.sent },
  { value: 'unsubscribed', count: counts.value.unsubscribed },
  { value: 'bounced', count: counts.value.bounced },
])
const sourceNames = computed(() => sources.value.map((item) => item.source).filter(Boolean))
const selectedCount = computed(() => selectedIds.value.size)
const pageIds = computed(() => leads.value.map((lead) => lead.id))
const isPageChecked = computed(() => pageIds.value.length > 0 && pageIds.value.every((id) => selectedIds.value.has(id)))
const totalPages = computed(() => Math.max(pages.value, 1))

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.leads.errors.request'))

const dateFormatter = computed(() => new Intl.DateTimeFormat(locale.value, { dateStyle: 'medium', timeStyle: 'short' }))
const formatDate = (value?: string): string => {
  if (!value) return '—'
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? '—' : dateFormatter.value.format(date)
}

const statusClass = (status: LeadStatus): string => ({
  not_sent: 'bg-surface text-muted-foreground',
  sent: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
  unsubscribed: 'bg-amber-500/10 text-amber-700 dark:text-amber-300',
  bounced: 'bg-red-500/10 text-red-700 dark:text-red-300',
}[status])

const loadLeads = async () => {
  isLoading.value = true
  errorKey.value = ''
  try {
    const response = await listLeads({ page: page.value, pageSize, source: sourceFilter.value, status: statusFilter.value, search: searchFilter.value })
    leads.value = response.items
    counts.value = response.counts
    total.value = response.total
    pages.value = response.pages
    if (page.value > 1 && response.items.length === 0 && response.total > 0) {
      page.value = Math.max(response.pages, 1)
      await loadLeads()
    }
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.leads.errors.request'
  } finally {
    isLoading.value = false
  }
}

const loadSources = async () => {
  try {
    sources.value = await listLeadSources()
  } catch {
    // 来源列表只用于筛选和输入提示，读不到不影响主列表。
  }
}

const loadSettings = async () => {
  try {
    settings.value = await getLeadSettings()
  } catch {
    settings.value = null
  }
}

const refreshAll = async () => {
  await Promise.all([loadLeads(), loadSources(), loadSettings()])
}

const resetToFirstPage = () => {
  page.value = 1
  selectedIds.value = new Set()
  void loadLeads()
}

watch([statusFilter, sourceFilter], resetToFirstPage)

const applySearch = () => {
  searchFilter.value = searchDraft.value.trim()
  resetToFirstPage()
}

const clearSearch = () => {
  searchDraft.value = ''
  if (searchFilter.value) {
    searchFilter.value = ''
    resetToFirstPage()
  }
}

const changePage = (target: number) => {
  if (target < 1 || target > totalPages.value) return
  page.value = target
  void loadLeads()
}

const toggleLead = (id: string) => {
  const next = new Set(selectedIds.value)
  if (next.has(id)) next.delete(id)
  else next.add(id)
  selectedIds.value = next
}

const togglePage = () => {
  const next = new Set(selectedIds.value)
  if (isPageChecked.value) pageIds.value.forEach((id) => next.delete(id))
  else pageIds.value.forEach((id) => next.add(id))
  selectedIds.value = next
}

const clearSelection = () => {
  selectedIds.value = new Set()
}

const sourceLabel = (source: string): string => {
  if (source === LEAD_NO_SOURCE) return t('admin.leads.filters.noSource')
  return source || t('admin.leads.filters.allSources')
}

const openSend = () => {
  if (selectedCount.value > 0) {
    sendTarget.value = {
      selection: { mode: 'selected', ids: Array.from(selectedIds.value) },
      summary: t('admin.leads.send.summarySelected', { count: selectedCount.value }),
    }
    return
  }
  sendTarget.value = {
    selection: { mode: 'filter', source: sourceFilter.value, status: statusFilter.value, search: searchFilter.value },
    summary: t('admin.leads.send.summaryFilter', {
      source: sourceLabel(sourceFilter.value),
      status: t(`admin.leads.status.${statusFilter.value || 'all'}`),
      search: searchFilter.value || t('admin.leads.filters.noSearch'),
    }),
  }
}

const onBatchCreated = (batch: MassEmailBatch) => {
  sendTarget.value = null
  createdBatch.value = batch
  notice.value = ''
  clearSelection()
  void loadSettings()
}

const onSaved = () => {
  const wasNew = formLead.value === 'new'
  formLead.value = null
  notice.value = t(wasNew ? 'admin.leads.notices.added' : 'admin.leads.notices.updated')
  void refreshAll()
}

const onImported = (result: LeadImportResult) => {
  notice.value = t('admin.leads.notices.imported', { ...result })
  void refreshAll()
}

const confirmDelete = async () => {
  if (!deleteTarget.value) return
  isDeleting.value = true
  try {
    const deleted = await deleteLeads(deleteTarget.value)
    const removed = new Set(deleteTarget.value)
    selectedIds.value = new Set(Array.from(selectedIds.value).filter((id) => !removed.has(id)))
    deleteTarget.value = null
    notice.value = t('admin.leads.notices.deleted', { count: deleted })
    await refreshAll()
  } catch (error) {
    deleteTarget.value = null
    errorKey.value = error instanceof Error ? error.message : 'admin.leads.errors.request'
  } finally {
    isDeleting.value = false
  }
}

const openLimit = () => {
  limitDraft.value = settings.value?.dailyLimit ?? 200
  limitErrorKey.value = ''
  isLimitOpen.value = true
}

const saveLimit = async () => {
  isSavingLimit.value = true
  limitErrorKey.value = ''
  try {
    settings.value = await saveLeadSettings(Math.round(Number(limitDraft.value)))
    isLimitOpen.value = false
  } catch (error) {
    limitErrorKey.value = error instanceof Error ? error.message : 'admin.leads.errors.request'
  } finally {
    isSavingLimit.value = false
  }
}

onMounted(() => {
  void refreshAll()
})
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-3 overflow-hidden">
    <section class="shrink-0 rounded-lg border border-border/50 bg-card p-4 shadow-sm">
      <div class="flex flex-col gap-3 lg:flex-row lg:items-start lg:justify-between">
        <div class="min-w-0">
          <h1 class="text-lg font-semibold text-foreground">{{ t('admin.leads.title') }}</h1>
          <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.leads.description') }}</p>
          <p v-if="settings" class="mt-2 flex flex-wrap items-center gap-x-2 text-xs text-muted-foreground">
            <span>{{ t('admin.leads.quota', { sent: settings.sentLast24h, limit: settings.dailyLimit }) }}</span>
            <button type="button" class="inline-flex items-center gap-1 font-medium text-primary outline-none hover:underline focus-visible:ring-2 focus-visible:ring-primary" @click="openLimit">
              <Settings2 class="h-3.5 w-3.5" />
              {{ t('admin.leads.actions.editLimit') }}
            </button>
          </p>
        </div>
        <div class="flex flex-wrap gap-2 lg:shrink-0 lg:flex-nowrap">
          <Button variant="secondary" class="rounded-lg" @click="formLead = 'new'">
            <Plus class="h-4 w-4" />
            {{ t('admin.leads.actions.add') }}
          </Button>
          <Button variant="secondary" class="rounded-lg" @click="isImportOpen = true">
            <Upload class="h-4 w-4" />
            {{ t('admin.leads.actions.import') }}
          </Button>
          <Button class="rounded-lg" :disabled="counts.all === 0" @click="openSend">
            <Send class="h-4 w-4" />
            {{ selectedCount > 0 ? t('admin.leads.actions.sendSelected', { count: selectedCount }) : t('admin.leads.actions.sendFiltered') }}
          </Button>
        </div>
      </div>
    </section>

    <div v-if="errorKey" class="flex shrink-0 items-start gap-3 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
      <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
      <span>{{ errorText(errorKey) }}</span>
    </div>
    <div v-if="createdBatch" class="flex shrink-0 items-start gap-3 rounded-lg border border-emerald-400/20 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-300" role="status">
      <CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0" />
      <span class="min-w-0 flex-1">
        {{ t('admin.leads.notices.queued', { count: createdBatch.recipientCount, skipped: createdBatch.skippedCount }) }}
        <RouterLink to="/admin/mass-email" class="ml-1 font-medium underline underline-offset-2">{{ t('admin.leads.notices.viewProgress') }}</RouterLink>
      </span>
      <button type="button" class="rounded p-0.5 hover:bg-emerald-500/10" :aria-label="t('admin.leads.actions.close')" @click="createdBatch = null">
        <X class="h-4 w-4" />
      </button>
    </div>
    <div v-else-if="notice" class="flex shrink-0 items-start gap-3 rounded-lg border border-emerald-400/20 bg-emerald-500/10 p-3 text-sm text-emerald-700 dark:text-emerald-300" role="status">
      <CheckCircle2 class="mt-0.5 h-4 w-4 shrink-0" />
      <span class="min-w-0 flex-1">{{ notice }}</span>
      <button type="button" class="rounded p-0.5 hover:bg-emerald-500/10" :aria-label="t('admin.leads.actions.close')" @click="notice = ''">
        <X class="h-4 w-4" />
      </button>
    </div>

    <section class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-lg border border-border/50 bg-card shadow-sm">
      <div class="flex shrink-0 flex-col gap-3 border-b border-border/50 p-3">
        <div class="flex flex-wrap gap-1" role="tablist" :aria-label="t('admin.leads.fields.status')">
          <button
            v-for="tab in statusTabs"
            :key="tab.value || 'all'"
            type="button"
            role="tab"
            :aria-selected="statusFilter === tab.value"
            class="inline-flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-sm outline-none transition focus-visible:ring-2 focus-visible:ring-primary"
            :class="statusFilter === tab.value ? 'bg-primary/10 font-medium text-primary' : 'text-muted-foreground hover:bg-surface hover:text-foreground'"
            @click="statusFilter = tab.value"
          >
            {{ t(`admin.leads.status.${tab.value || 'all'}`) }}
            <span class="text-xs opacity-80">{{ tab.count }}</span>
          </button>
        </div>
        <div class="flex flex-col gap-2 sm:flex-row sm:items-center">
          <select v-model="sourceFilter" class="h-9 rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary sm:w-52" :aria-label="t('admin.leads.fields.source')">
            <option value="">{{ t('admin.leads.filters.allSources') }}</option>
            <option :value="LEAD_NO_SOURCE">{{ t('admin.leads.filters.noSource') }}</option>
            <option v-for="item in sources.filter((entry) => entry.source)" :key="item.source" :value="item.source">{{ item.source }}（{{ item.count }}）</option>
          </select>
          <form class="relative min-w-0 flex-1 sm:max-w-sm" @submit.prevent="applySearch">
            <Search class="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
            <input
              v-model="searchDraft"
              type="search"
              class="h-9 w-full rounded-lg border border-border/60 bg-surface py-2 pl-9 pr-9 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary"
              :placeholder="t('admin.leads.filters.searchPlaceholder')"
              :aria-label="t('admin.leads.filters.search')"
            >
            <button v-if="searchDraft || searchFilter" type="button" class="absolute right-2 top-1/2 -translate-y-1/2 rounded p-1 text-muted-foreground hover:text-foreground" :aria-label="t('admin.leads.filters.clearSearch')" @click="clearSearch">
              <X class="h-3.5 w-3.5" />
            </button>
          </form>
          <Button variant="ghost" size="sm" class="rounded-lg sm:ml-auto" :disabled="isLoading" @click="refreshAll">
            <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" />
            <RefreshCw v-else class="h-4 w-4" />
            {{ t('admin.leads.actions.refresh') }}
          </Button>
        </div>
        <div v-if="selectedCount > 0" class="flex flex-wrap items-center gap-2 rounded-lg bg-primary/5 px-3 py-2 text-sm">
          <span class="font-medium text-foreground">{{ t('admin.leads.selection.count', { count: selectedCount }) }}</span>
          <Button size="sm" class="rounded-lg" @click="openSend">
            <Send class="h-4 w-4" />
            {{ t('admin.leads.actions.sendSelected', { count: selectedCount }) }}
          </Button>
          <Button variant="secondary" size="sm" class="rounded-lg" @click="deleteTarget = Array.from(selectedIds)">
            <Trash2 class="h-4 w-4" />
            {{ t('admin.leads.actions.deleteSelected') }}
          </Button>
          <Button variant="ghost" size="sm" class="rounded-lg" @click="clearSelection">{{ t('admin.leads.actions.clearSelection') }}</Button>
        </div>
      </div>

      <div v-if="isLoading && leads.length === 0" class="flex flex-1 items-center justify-center py-16 text-muted-foreground" role="status">
        <Loader2 class="mr-2 h-5 w-5 animate-spin" />
        {{ t('admin.leads.loading') }}
      </div>
      <div v-else-if="leads.length === 0" class="flex flex-1 flex-col items-center justify-center px-4 py-16 text-center text-muted-foreground">
        <Contact class="mb-3 h-10 w-10 opacity-40" />
        <p class="font-medium text-foreground">{{ t(counts.all === 0 && !sourceFilter && !searchFilter ? 'admin.leads.empty.title' : 'admin.leads.empty.filteredTitle') }}</p>
        <p class="mt-1 max-w-md text-sm">{{ t(counts.all === 0 && !sourceFilter && !searchFilter ? 'admin.leads.empty.description' : 'admin.leads.empty.filteredDescription') }}</p>
      </div>
      <div v-else class="min-h-0 flex-1 overflow-auto">
        <table class="w-full min-w-[820px] text-left text-sm">
          <thead class="sticky top-0 z-10 bg-surface text-xs text-muted-foreground">
            <tr>
              <th class="w-12 px-4 py-3">
                <input type="checkbox" class="h-4 w-4 rounded border-border bg-surface text-primary focus:ring-2 focus:ring-primary" :checked="isPageChecked" :aria-label="t('admin.leads.selection.selectPage')" @change="togglePage">
              </th>
              <th class="px-4 py-3 font-semibold">{{ t('admin.leads.fields.email') }}</th>
              <th class="px-4 py-3 font-semibold">{{ t('admin.leads.fields.source') }}</th>
              <th class="px-4 py-3 font-semibold">{{ t('admin.leads.fields.status') }}</th>
              <th class="px-4 py-3 font-semibold">{{ t('admin.leads.fields.sentCount') }}</th>
              <th class="px-4 py-3 font-semibold">{{ t('admin.leads.fields.lastSentAt') }}</th>
              <th class="px-4 py-3 font-semibold">{{ t('admin.leads.fields.createdAt') }}</th>
              <th class="px-4 py-3 text-right font-semibold">{{ t('admin.leads.fields.actions') }}</th>
            </tr>
          </thead>
          <tbody class="divide-y divide-border/50">
            <tr v-for="lead in leads" :key="lead.id" class="hover:bg-surface/70">
              <td class="px-4 py-3">
                <input type="checkbox" class="h-4 w-4 rounded border-border bg-surface text-primary focus:ring-2 focus:ring-primary" :checked="selectedIds.has(lead.id)" :aria-label="t('admin.leads.selection.selectLead', { email: lead.email })" @change="toggleLead(lead.id)">
              </td>
              <td class="max-w-[18rem] px-4 py-3">
                <div class="truncate font-medium text-foreground" :title="lead.email">{{ lead.email }}</div>
                <div v-if="lead.note" class="truncate text-xs text-muted-foreground" :title="lead.note">{{ lead.note }}</div>
              </td>
              <td class="max-w-[10rem] px-4 py-3">
                <span v-if="lead.source" class="block truncate text-foreground" :title="lead.source">{{ lead.source }}</span>
                <span v-else class="text-muted-foreground">—</span>
              </td>
              <td class="px-4 py-3">
                <span class="inline-flex rounded-md px-2 py-0.5 text-xs font-medium" :class="statusClass(lead.status)">{{ t(`admin.leads.status.${lead.status}`) }}</span>
              </td>
              <td class="px-4 py-3 text-muted-foreground">{{ lead.sentCount }}</td>
              <td class="px-4 py-3 text-muted-foreground">{{ formatDate(lead.lastSentAt) }}</td>
              <td class="px-4 py-3 text-muted-foreground">{{ formatDate(lead.createdAt) }}</td>
              <td class="px-4 py-3">
                <div class="flex justify-end gap-1">
                  <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface hover:text-foreground focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.leads.actions.edit')" :title="t('admin.leads.actions.edit')" @click="formLead = lead">
                    <Pencil class="h-4 w-4" />
                  </button>
                  <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-red-500/10 hover:text-red-600 focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.leads.actions.delete')" :title="t('admin.leads.actions.delete')" @click="deleteTarget = [lead.id]">
                    <Trash2 class="h-4 w-4" />
                  </button>
                </div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div v-if="total > 0" class="flex shrink-0 flex-wrap items-center justify-between gap-2 border-t border-border/50 px-3 py-2 text-sm text-muted-foreground">
        <span>{{ t('admin.leads.pagination.total', { total }) }}</span>
        <div class="flex items-center gap-2">
          <Button variant="secondary" size="sm" class="rounded-lg" :disabled="page <= 1 || isLoading" @click="changePage(page - 1)">{{ t('admin.leads.pagination.previous') }}</Button>
          <span>{{ t('admin.leads.pagination.page', { page, totalPages }) }}</span>
          <Button variant="secondary" size="sm" class="rounded-lg" :disabled="page >= totalPages || isLoading" @click="changePage(page + 1)">{{ t('admin.leads.pagination.next') }}</Button>
        </div>
      </div>
    </section>

    <LeadFormDialog
      v-if="formLead"
      :lead="formLead === 'new' ? null : formLead"
      :sources="sourceNames"
      @close="formLead = null"
      @saved="onSaved"
    />
    <LeadImportDialog
      v-if="isImportOpen"
      :sources="sourceNames"
      @close="isImportOpen = false"
      @imported="onImported"
    />
    <LeadSendDialog
      v-if="sendTarget"
      :selection="sendTarget.selection"
      :summary="sendTarget.summary"
      @close="sendTarget = null"
      @created="onBatchCreated"
    />

    <div v-if="deleteTarget" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="lead-delete-title">
      <div class="w-full max-w-sm rounded-lg border border-border/60 bg-card p-4 shadow-xl">
        <h2 id="lead-delete-title" class="text-base font-semibold text-foreground">{{ t('admin.leads.delete.title', { count: deleteTarget.length }) }}</h2>
        <p class="mt-2 text-sm text-muted-foreground">{{ t('admin.leads.delete.description') }}</p>
        <div class="mt-4 flex justify-end gap-2">
          <Button variant="secondary" class="rounded-lg" :disabled="isDeleting" @click="deleteTarget = null">{{ t('admin.leads.actions.cancel') }}</Button>
          <Button class="rounded-lg bg-red-600 text-white hover:bg-red-600/90" :disabled="isDeleting" @click="confirmDelete">
            <Loader2 v-if="isDeleting" class="h-4 w-4 animate-spin" />
            {{ t('admin.leads.actions.delete') }}
          </Button>
        </div>
      </div>
    </div>

    <div v-if="isLimitOpen" class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="lead-limit-title">
      <form class="w-full max-w-sm rounded-lg border border-border/60 bg-card p-4 shadow-xl" @submit.prevent="saveLimit">
        <h2 id="lead-limit-title" class="text-base font-semibold text-foreground">{{ t('admin.leads.limit.title') }}</h2>
        <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.leads.limit.description') }}</p>
        <label class="mt-3 block space-y-1 text-sm font-medium text-foreground">
          <span>{{ t('admin.leads.limit.label') }}</span>
          <input v-model.number="limitDraft" type="number" min="1" max="10000" step="1" class="h-10 w-full rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary">
        </label>
        <p v-if="limitErrorKey" class="mt-2 text-sm text-warning" role="alert">{{ errorText(limitErrorKey) }}</p>
        <div class="mt-4 flex justify-end gap-2">
          <Button type="button" variant="secondary" class="rounded-lg" :disabled="isSavingLimit" @click="isLimitOpen = false">{{ t('admin.leads.actions.cancel') }}</Button>
          <Button type="submit" class="rounded-lg" :disabled="isSavingLimit">
            <Loader2 v-if="isSavingLimit" class="h-4 w-4 animate-spin" />
            {{ t('admin.leads.actions.save') }}
          </Button>
        </div>
      </form>
    </div>
  </div>
</template>
