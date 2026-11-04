<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { Inbox, Loader2, RefreshCw } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { listTickets } from '../api/tickets'
import { getClientConfig } from '../api/clientConfig'
import { ticketStatuses, type AdminTicket, type AdminTicketList, type TicketStatus } from '../types/tickets'
import TicketDetailDrawer from '../components/tickets/TicketDetailDrawer.vue'

// 工单：TransitLink 用户提交的问题。默认先看待处理的（待回复和用户追问）。
const { t, locale } = useI18n()

type Filter = TicketStatus | ''
const filter = ref<Filter>('')
const page = ref(1)
const pageSize = 20
const data = ref<AdminTicketList | null>(null)
const loading = ref(false)
const error = ref('')
const openTicketId = ref<string | null>(null)
const ticketsEnabled = ref<boolean | null>(null)

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    data.value = await listTickets({ status: filter.value, page: page.value, pageSize })
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.tickets.errors.request'
  } finally {
    loading.value = false
  }
}

const choose = (next: Filter) => {
  filter.value = next
  page.value = 1
  void load()
}

const goTo = (next: number) => {
  page.value = next
  void load()
}

const counts = computed(() => data.value?.counts ?? { open: 0, pending: 0, replied: 0, closed: 0 })
const allCount = computed(() => counts.value.open + counts.value.pending + counts.value.replied + counts.value.closed)
const tabs = computed(() => [
  { value: '' as Filter, label: t('admin.tickets.tabs.all'), count: allCount.value },
  ...ticketStatuses.map((status) => ({ value: status as Filter, label: t(`admin.tickets.status.${status}`), count: counts.value[status] })),
])

const statusClass = (status: TicketStatus) => ({
  open: 'bg-sky-500/10 text-sky-700 dark:text-sky-300',
  pending: 'bg-amber-500/10 text-amber-700 dark:text-amber-300',
  replied: 'bg-emerald-500/10 text-emerald-700 dark:text-emerald-300',
  closed: 'bg-surface-elevated text-muted-foreground',
}[status])

const needsReply = (ticket: AdminTicket) => ticket.status === 'open' || ticket.status === 'pending'

const userName = (ticket: AdminTicket) => ticket.siteUsername || ticket.siteEmail || t('admin.tickets.unknownUser')

const formatTime = (value: string) => {
  const date = new Date(value)
  const sameDay = date.toDateString() === new Date().toDateString()
  return new Intl.DateTimeFormat(locale.value, sameDay ? { hour: '2-digit', minute: '2-digit' } : { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(date)
}

// 停留在页面上时每分钟刷新一次，新工单不用手动刷新。
let timer: number | undefined
onMounted(async () => {
  void load()
  timer = window.setInterval(() => { if (!openTicketId.value) void load() }, 60_000)
  try {
    const config = await getClientConfig()
    ticketsEnabled.value = config.enabled && config.tickets.enabled
  } catch {
    ticketsEnabled.value = null
  }
})
onBeforeUnmount(() => window.clearInterval(timer))
</script>

<template>
  <div class="mx-auto flex w-full max-w-6xl flex-col gap-6">
    <header class="flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-bold tracking-tight text-foreground">{{ t('admin.tickets.title') }}</h1>
        <p class="mt-1 max-w-2xl text-sm text-muted-foreground">{{ t('admin.tickets.subtitle') }}</p>
      </div>
      <Button variant="secondary" :disabled="loading" @click="load">
        <Loader2 v-if="loading" class="h-4 w-4 animate-spin" />
        <RefreshCw v-else class="h-4 w-4" />
        {{ t('admin.tickets.refresh') }}
      </Button>
    </header>

    <p v-if="ticketsEnabled === false" class="rounded-xl border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-800 dark:text-amber-300">
      {{ t('admin.tickets.disabledHint') }}
      <RouterLink to="/admin/client" class="font-medium underline underline-offset-2">{{ t('admin.tickets.openClientPage') }}</RouterLink>
    </p>

    <div class="flex flex-wrap gap-2" role="tablist" :aria-label="t('admin.tickets.title')">
      <button
        v-for="tab in tabs"
        :key="tab.value"
        type="button"
        role="tab"
        :aria-selected="filter === tab.value"
        class="inline-flex h-9 items-center gap-2 rounded-lg border px-3 text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        :class="filter === tab.value ? 'border-primary bg-primary/10 font-medium text-primary' : 'border-border/60 text-muted-foreground hover:bg-surface hover:text-foreground'"
        @click="choose(tab.value)"
      >
        {{ tab.label }}
        <span class="rounded-full bg-surface-elevated px-1.5 text-xs tabular-nums text-foreground">{{ tab.count }}</span>
      </button>
    </div>

    <p v-if="error" class="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive" role="alert">{{ t(error) }}</p>

    <section class="overflow-hidden rounded-2xl border border-border/50 bg-card shadow-sm">
      <p v-if="loading && !data" class="flex items-center gap-2 p-6 text-sm text-muted-foreground"><Loader2 class="h-4 w-4 animate-spin" />{{ t('admin.tickets.loading') }}</p>
      <div v-else-if="data && data.items.length === 0" class="flex flex-col items-center gap-2 p-10 text-center text-sm text-muted-foreground">
        <Inbox class="h-8 w-8" />
        {{ filter ? t('admin.tickets.emptyFiltered') : t('admin.tickets.empty') }}
      </div>
      <ul v-else-if="data" class="divide-y divide-border/50">
        <li v-for="ticket in data.items" :key="ticket.id">
          <button type="button" class="flex w-full items-center gap-4 px-5 py-3.5 text-left transition-colors hover:bg-surface/60 focus-visible:bg-surface/60 focus-visible:outline-none" @click="openTicketId = ticket.id">
            <span class="h-2 w-2 shrink-0 rounded-full" :class="needsReply(ticket) ? 'bg-primary' : 'bg-transparent'" aria-hidden="true"></span>
            <div class="min-w-0 flex-1">
              <p class="truncate text-sm font-medium text-foreground">{{ ticket.title }}</p>
              <p class="mt-0.5 truncate text-xs text-muted-foreground">
                {{ userName(ticket) }}<template v-if="ticket.siteUserId"> · ID {{ ticket.siteUserId }}</template> · {{ ticket.category }}
              </p>
            </div>
            <span class="shrink-0 rounded-full px-2 py-0.5 text-xs font-medium" :class="statusClass(ticket.status)">{{ t(`admin.tickets.status.${ticket.status}`) }}</span>
            <span class="w-32 shrink-0 whitespace-nowrap text-right text-xs tabular-nums text-muted-foreground">{{ formatTime(ticket.lastMessageAt) }}</span>
          </button>
        </li>
      </ul>
    </section>

    <div v-if="data && data.totalPages > 1" class="flex items-center justify-end gap-2 text-sm">
      <span class="text-muted-foreground">{{ t('admin.tickets.page', { page: data.page, pages: data.totalPages }) }}</span>
      <Button variant="secondary" size="sm" :disabled="loading || data.page <= 1" @click="goTo(data.page - 1)">{{ t('admin.tickets.previous') }}</Button>
      <Button variant="secondary" size="sm" :disabled="loading || data.page >= data.totalPages" @click="goTo(data.page + 1)">{{ t('admin.tickets.next') }}</Button>
    </div>

    <TicketDetailDrawer v-if="openTicketId" :ticket-id="openTicketId" @close="openTicketId = null; load()" @updated="load" />
  </div>
</template>
