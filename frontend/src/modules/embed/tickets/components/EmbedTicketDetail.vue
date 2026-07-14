<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, ArrowLeft, Loader2, Send } from 'lucide-vue-next'
import { getEmbedTicket, replyEmbedTicket } from '../api/tickets'
import EmbedAttachmentThumbnail from './EmbedAttachmentThumbnail.vue'
import type { EmbedTicketDetail, TicketEmbedTemplate } from '../types'

const props = withDefaults(defineProps<{
  ticketId: string
  template?: TicketEmbedTemplate
}>(), {
  template: 'default',
})

const emit = defineEmits<{
  (event: 'back'): void
  (event: 'updated'): void
}>()

const { t, locale } = useI18n()

const detail = ref<EmbedTicketDetail | null>(null)
const isLoading = ref(false)
const errorKey = ref<string | null>(null)
const replyBody = ref('')
const isReplying = ref(false)
const replyErrorKey = ref<string | null>(null)

const load = async () => {
  isLoading.value = true
  errorKey.value = null
  try {
    detail.value = await getEmbedTicket(props.ticketId)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'embed.tickets.errors.unknown'
  } finally {
    isLoading.value = false
  }
}

watch(() => props.ticketId, load, { immediate: true })

const submitReply = async () => {
  if (!replyBody.value.trim()) return
  isReplying.value = true
  replyErrorKey.value = null
  try {
    detail.value = await replyEmbedTicket(props.ticketId, replyBody.value.trim())
    replyBody.value = ''
    emit('updated')
  } catch (error) {
    replyErrorKey.value = error instanceof Error ? error.message : 'embed.tickets.errors.unknown'
  } finally {
    isReplying.value = false
  }
}

const formatDateTime = (value: string): string => {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
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

// default 现在嵌套在 TicketEmbedPage.vue 的整块外壳卡片里（见 pageShellClass），
// 所以这里不再给 default 的各个分区套自己的 border+bg-card 小卡片，改用分隔线和更充分的
// padding/间距，避免"卡片里套卡片"的拥挤感；minimal/support 的取值原样保留，不受这次调整影响。
const rootSpacingClass = computed(() => (props.template === 'default' ? 'space-y-6' : 'space-y-4'))

const titleClass = computed(() => (
  props.template === 'default' ? 'text-base font-semibold text-foreground' : 'text-sm font-semibold text-foreground'
))

const cardClass = computed(() => {
  if (props.template === 'minimal') return 'border-b border-border/40 pb-4'
  if (props.template === 'support') return 'rounded-xl border border-border/60 bg-card p-4 shadow-sm'
  return 'space-y-1 border-b border-border/40 pb-6'
})

const messagesWrapperClass = computed(() => {
  if (props.template === 'minimal') return 'space-y-4 border-b border-border/40 pb-4'
  if (props.template === 'support') return 'space-y-3 rounded-xl border border-border/60 bg-card p-4 shadow-sm'
  return 'space-y-5 border-b border-border/40 pb-6'
})

// support 模板用对话气泡：用户消息靠右、主色调气泡；客服消息靠左、中性气泡。
// default/minimal 保持原有的上下堆叠布局，只是外层卡片密度和消息间距不同。
const messageRowClass = (authorType: string): string => {
  if (props.template === 'support') {
    return authorType === 'admin' ? 'flex flex-col items-start' : 'flex flex-col items-end'
  }
  return props.template === 'default' ? 'space-y-1.5' : 'space-y-1'
}

const messageBubbleClass = (authorType: string): string => {
  if (props.template === 'support') {
    return authorType === 'admin'
      ? 'max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-tl-sm bg-surface-elevated p-3 text-sm text-foreground'
      : 'max-w-[85%] whitespace-pre-wrap rounded-2xl rounded-tr-sm bg-primary/15 p-3 text-sm text-foreground'
  }
  const padding = props.template === 'default' ? 'p-4' : 'p-3'
  return `whitespace-pre-wrap rounded-lg bg-surface/50 ${padding} text-sm leading-relaxed text-foreground`
}

const replyCardClass = computed(() => {
  if (props.template === 'minimal') return 'space-y-2'
  if (props.template === 'support') return 'space-y-2 rounded-xl border border-border/50 bg-card p-4'
  return 'space-y-3'
})

const replyRows = computed(() => (props.template === 'default' ? 4 : 3))

const closedNoticeClass = computed(() => (
  props.template === 'default'
    ? 'rounded-xl border border-border/50 bg-surface-elevated/50 p-5 text-center text-sm text-muted-foreground'
    : 'rounded-xl border border-border/50 bg-surface-elevated/50 p-4 text-center text-sm text-muted-foreground'
))
</script>

<template>
  <div :class="rootSpacingClass">
    <button
      type="button"
      class="inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
      @click="emit('back')"
    >
      <ArrowLeft class="h-3.5 w-3.5" />
      {{ t('embed.tickets.detail.back') }}
    </button>

    <div v-if="isLoading" class="flex items-center justify-center py-12 text-muted-foreground">
      <Loader2 class="mr-2 h-5 w-5 animate-spin" />
      {{ t('embed.tickets.detail.loading') }}
    </div>

    <div v-else-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-xs text-warning">
      <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
      <span>{{ t(errorKey) }}</span>
    </div>

    <template v-else-if="detail">
      <div :class="cardClass">
        <div class="flex items-start justify-between gap-3">
          <p :class="titleClass">{{ detail.title }}</p>
          <span :class="['inline-flex shrink-0 rounded-md border px-2 py-0.5 text-xs font-semibold', statusBadgeClass(detail.status)]">
            {{ t(`embed.tickets.status.${detail.status}`) }}
          </span>
        </div>
        <div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground" :class="template === 'default' ? 'mt-1' : 'mt-1'">
          <span class="inline-flex items-center rounded-md border border-border/50 bg-surface-elevated px-1.5 py-0.5">{{ detail.category }}</span>
          <span class="inline-flex items-center rounded-md border border-border/50 bg-surface-elevated px-1.5 py-0.5">{{ detail.priority }}</span>
          <span>{{ detail.manualEmail }}</span>
        </div>
      </div>

      <div :class="messagesWrapperClass">
        <div v-for="message in detail.messages" :key="message.id" :class="messageRowClass(message.authorType)">
          <div class="flex items-center gap-2 text-xs text-muted-foreground" :class="template === 'support' ? 'justify-between w-full max-w-[85%]' : 'justify-between'">
            <span class="font-medium text-foreground">
              {{ message.authorType === 'admin' ? t('embed.tickets.detail.support') : t('embed.tickets.detail.you') }}
            </span>
            <span>{{ formatDateTime(message.createdAt) }}</span>
          </div>
          <p :class="messageBubbleClass(message.authorType)">{{ message.body }}</p>
          <div v-if="message.attachments.length > 0" class="flex flex-wrap gap-2">
            <EmbedAttachmentThumbnail
              v-for="attachment in message.attachments"
              :key="attachment.id"
              :attachment="attachment"
            />
          </div>
        </div>
      </div>

      <div v-if="detail.status !== 'closed'" :class="replyCardClass">
        <textarea
          v-model="replyBody"
          :rows="replyRows"
          class="w-full rounded-lg border border-border/50 bg-surface px-3 py-2 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
          :placeholder="t('embed.tickets.detail.replyPlaceholder')"
        />
        <div v-if="replyErrorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-xs text-warning">
          <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
          <span>{{ t(replyErrorKey) }}</span>
        </div>
        <div class="flex justify-end">
          <button
            type="button"
            class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
            :disabled="isReplying || !replyBody.trim()"
            @click="submitReply"
          >
            <Loader2 v-if="isReplying" class="h-3.5 w-3.5 animate-spin" />
            <Send v-else class="h-3.5 w-3.5" />
            {{ t('embed.tickets.detail.send') }}
          </button>
        </div>
      </div>
      <div v-else :class="closedNoticeClass">
        {{ t('embed.tickets.detail.closedNotice') }}
      </div>
    </template>
  </div>
</template>
