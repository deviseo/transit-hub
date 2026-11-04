<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2, Send, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { getTicket, replyTicket, setTicketStatus } from '../../api/tickets'
import { ticketStatuses, type AdminTicketDetail, type TicketStatus } from '../../types/tickets'
import TicketAttachmentThumb from './TicketAttachmentThumb.vue'
import TicketImagePreview from './TicketImagePreview.vue'

// 工单详情：用户信息、来往消息（含图片）、回复和修改状态。回复后工单变成「已回复」。
const props = defineProps<{ ticketId: string }>()
const emit = defineEmits<{ close: []; updated: [] }>()
const { t, locale } = useI18n()

const ticket = ref<AdminTicketDetail | null>(null)
const loading = ref(false)
const error = ref('')
const reply = ref('')
const sending = ref(false)
const changingStatus = ref(false)
const preview = ref<{ url: string; name: string } | null>(null)
const thread = ref<HTMLElement | null>(null)

const scrollToEnd = async () => {
  await nextTick()
  thread.value?.scrollTo({ top: thread.value.scrollHeight })
}

const load = async () => {
  loading.value = true
  error.value = ''
  try {
    ticket.value = await getTicket(props.ticketId)
    await scrollToEnd()
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.tickets.errors.request'
  } finally {
    loading.value = false
  }
}

const send = async () => {
  const body = reply.value.trim()
  if (!body || sending.value || !ticket.value) return
  sending.value = true
  error.value = ''
  try {
    ticket.value = await replyTicket(ticket.value.id, body)
    reply.value = ''
    emit('updated')
    await scrollToEnd()
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.tickets.errors.request'
  } finally {
    sending.value = false
  }
}

const changeStatus = async (event: Event) => {
  const status = (event.target as HTMLSelectElement).value as TicketStatus
  if (!ticket.value || status === ticket.value.status) return
  changingStatus.value = true
  error.value = ''
  try {
    ticket.value = await setTicketStatus(ticket.value.id, status)
    emit('updated')
  } catch (caught) {
    error.value = caught instanceof Error ? caught.message : 'admin.tickets.errors.request'
    ;(event.target as HTMLSelectElement).value = ticket.value.status
  } finally {
    changingStatus.value = false
  }
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && !preview.value) emit('close')
}

const onReplyKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Enter' && (event.metaKey || event.ctrlKey)) {
    event.preventDefault()
    void send()
  }
}

const formatTime = (value: string) =>
  new Intl.DateTimeFormat(locale.value, { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value))

const userLine = computed(() => {
  if (!ticket.value) return ''
  const name = ticket.value.siteUsername || ticket.value.siteEmail || t('admin.tickets.unknownUser')
  return ticket.value.siteUserId ? t('admin.tickets.detail.userWithId', { name, id: ticket.value.siteUserId }) : name
})

watch(() => props.ticketId, () => { void load() })
onMounted(() => {
  void load()
  document.addEventListener('keydown', onKeydown)
})
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="fixed inset-0 z-50 flex justify-end bg-background/60 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="ticket-detail-title" @click.self="emit('close')">
    <div class="flex h-full w-full max-w-2xl flex-col border-l border-border/60 bg-card shadow-2xl">
      <div class="flex shrink-0 items-start gap-3 border-b border-border/50 p-4">
        <div class="min-w-0 flex-1">
          <h2 id="ticket-detail-title" class="truncate text-lg font-semibold text-foreground">{{ ticket?.title ?? t('admin.tickets.detail.loading') }}</h2>
          <p v-if="ticket" class="mt-0.5 text-xs text-muted-foreground">{{ ticket.category }} · {{ formatTime(ticket.createdAt) }}</p>
        </div>
        <select
          v-if="ticket"
          :value="ticket.status"
          :disabled="changingStatus"
          :aria-label="t('admin.tickets.detail.status')"
          class="h-9 rounded-lg border border-border/60 bg-surface px-2 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
          @change="changeStatus"
        >
          <option v-for="status in ticketStatuses" :key="status" :value="status">{{ t(`admin.tickets.status.${status}`) }}</option>
        </select>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.tickets.detail.close')" @click="emit('close')">
          <X class="h-4 w-4" />
        </button>
      </div>

      <p v-if="error" class="mx-4 mt-3 rounded-lg border border-destructive/30 bg-destructive/10 px-3 py-2 text-sm text-destructive" role="alert">{{ t(error) }}</p>
      <p v-if="loading && !ticket" class="flex items-center gap-2 p-4 text-sm text-muted-foreground"><Loader2 class="h-4 w-4 animate-spin" />{{ t('admin.tickets.detail.loading') }}</p>

      <template v-if="ticket">
        <dl class="grid shrink-0 grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 border-b border-border/50 px-4 py-3 text-sm">
          <dt class="text-muted-foreground">{{ t('admin.tickets.detail.user') }}</dt>
          <dd class="min-w-0 truncate text-foreground">{{ userLine }} <span class="text-xs text-muted-foreground">{{ ticket.platform }}</span></dd>
          <template v-if="ticket.siteEmail && ticket.siteEmail !== ticket.siteUsername">
            <dt class="text-muted-foreground">{{ t('admin.tickets.detail.email') }}</dt>
            <dd class="min-w-0 truncate text-foreground">{{ ticket.siteEmail }}</dd>
          </template>
          <template v-if="ticket.contact">
            <dt class="text-muted-foreground">{{ t('admin.tickets.detail.contact') }}</dt>
            <dd class="min-w-0 truncate text-foreground">{{ ticket.contact }}</dd>
          </template>
          <template v-if="ticket.clientInfo">
            <dt class="text-muted-foreground">{{ t('admin.tickets.detail.client') }}</dt>
            <dd class="min-w-0 truncate text-foreground">{{ ticket.clientInfo }}</dd>
          </template>
        </dl>

        <div ref="thread" class="min-h-0 flex-1 space-y-4 overflow-y-auto p-4">
          <div v-for="message in ticket.messages" :key="message.id" class="flex" :class="message.authorType === 'admin' ? 'justify-end' : 'justify-start'">
            <div class="max-w-[85%] space-y-1.5">
              <p class="text-xs text-muted-foreground" :class="message.authorType === 'admin' ? 'text-right' : ''">
                {{ message.authorType === 'admin' ? t('admin.tickets.detail.you') : (message.authorName || t('admin.tickets.unknownUser')) }} · {{ formatTime(message.createdAt) }}
              </p>
              <div
                v-if="message.body"
                class="whitespace-pre-wrap break-words rounded-2xl px-3.5 py-2.5 text-sm"
                :class="message.authorType === 'admin' ? 'rounded-tr-sm bg-primary text-primary-foreground' : 'rounded-tl-sm bg-surface text-foreground'"
              >{{ message.body }}</div>
              <div v-if="message.attachments.length" class="flex flex-wrap gap-2" :class="message.authorType === 'admin' ? 'justify-end' : ''">
                <TicketAttachmentThumb v-for="attachment in message.attachments" :key="attachment.id" :attachment="attachment" @preview="preview = $event" />
              </div>
            </div>
          </div>
        </div>

        <div class="shrink-0 border-t border-border/50 p-3">
          <p v-if="ticket.status === 'closed'" class="mb-2 text-xs text-muted-foreground">{{ t('admin.tickets.detail.closedNote') }}</p>
          <div class="flex items-end gap-2">
            <textarea
              v-model="reply"
              rows="3"
              maxlength="5000"
              :placeholder="t('admin.tickets.detail.replyPlaceholder')"
              :aria-label="t('admin.tickets.detail.reply')"
              class="min-h-[4.5rem] flex-1 resize-y rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
              @keydown="onReplyKeydown"
            />
            <Button :disabled="!reply.trim() || sending" @click="send">
              <Loader2 v-if="sending" class="h-4 w-4 animate-spin" />
              <Send v-else class="h-4 w-4" />
              {{ t('admin.tickets.detail.send') }}
            </Button>
          </div>
        </div>
      </template>
    </div>
    <TicketImagePreview v-if="preview" :url="preview.url" :name="preview.name" @close="preview = null" />
  </div>
</template>
