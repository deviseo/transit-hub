<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, MessageSquare, Send, X } from 'lucide-vue-next'
import { getTicket, replyTicket, updateTicketStatus } from '../../api/tickets'
import AdminAttachmentThumbnail from './AdminAttachmentThumbnail.vue'
import AdminAttachmentPreviewModal from './AdminAttachmentPreviewModal.vue'
import type { AdminTicketDetail, TicketStatus } from '../../types/tickets'

const props = defineProps<{
  open: boolean
  ticketId: string | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'updated'): void
}>()

const { t, locale } = useI18n()
const prefix = 'admin.tickets.detail'

const detail = ref<AdminTicketDetail | null>(null)
const isLoading = ref(false)
const errorKey = ref<string | null>(null)
const replyBody = ref('')
const isReplying = ref(false)
const replyErrorKey = ref<string | null>(null)
const isStatusUpdating = ref(false)
const previewImage = ref<{ url: string; name: string } | null>(null)

const statusOptions: TicketStatus[] = ['open', 'pending', 'replied', 'closed']

const openAttachmentPreview = (payload: { url: string; name: string }) => {
  previewImage.value = payload
}

const closeAttachmentPreview = () => {
  previewImage.value = null
}

const load = async () => {
  if (!props.ticketId) return
  isLoading.value = true
  errorKey.value = null
  try {
    detail.value = await getTicket(props.ticketId)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.tickets.errors.unknown'
  } finally {
    isLoading.value = false
  }
}

watch(() => [props.open, props.ticketId], ([isOpen]) => {
  // 抽屉关闭或切换到另一张工单时，一并关闭可能还打开着的图片预览层，避免展示一个即将被
  // AdminAttachmentThumbnail 卸载/revoke 掉的 object URL。
  previewImage.value = null
  if (isOpen) {
    detail.value = null
    replyBody.value = ''
    replyErrorKey.value = null
    void load()
  }
})

const submitReply = async () => {
  if (!props.ticketId || !replyBody.value.trim()) return
  isReplying.value = true
  replyErrorKey.value = null
  try {
    detail.value = await replyTicket(props.ticketId, replyBody.value.trim())
    replyBody.value = ''
    emit('updated')
  } catch (error) {
    replyErrorKey.value = error instanceof Error ? error.message : 'admin.tickets.errors.unknown'
  } finally {
    isReplying.value = false
  }
}

const handleStatusChange = async (event: Event) => {
  if (!props.ticketId) return
  const target = event.target as HTMLSelectElement
  const nextStatus = target.value as TicketStatus
  isStatusUpdating.value = true
  errorKey.value = null
  try {
    detail.value = await updateTicketStatus(props.ticketId, nextStatus)
    emit('updated')
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.tickets.errors.unknown'
    // 恢复下拉框到失败前的状态，避免界面显示和实际状态不一致。
    if (detail.value) target.value = detail.value.status
  } finally {
    isStatusUpdating.value = false
  }
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
      return 'bg-sky-500/10 text-sky-600 dark:text-sky-400'
    case 'pending':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
    case 'replied':
      return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
    case 'closed':
      return 'bg-surface-elevated text-muted-foreground'
    default:
      return 'bg-surface-elevated text-muted-foreground'
  }
}
</script>

<template>
  <Teleport to="body">
    <Transition
      enter-active-class="transition duration-200 ease-out"
      enter-from-class="opacity-0"
      enter-to-class="opacity-100"
      leave-active-class="transition duration-150 ease-in"
      leave-from-class="opacity-100"
      leave-to-class="opacity-0"
    >
      <div v-if="open" class="fixed inset-0 z-[150]">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="emit('close')" />

        <Transition
          enter-active-class="transition duration-250 ease-out"
          enter-from-class="translate-x-full"
          enter-to-class="translate-x-0"
          leave-active-class="transition duration-200 ease-in"
          leave-from-class="translate-x-0"
          leave-to-class="translate-x-full"
        >
          <div
            v-if="open"
            class="absolute right-0 top-0 bottom-0 w-full max-w-xl overflow-y-auto border-l border-border/60 bg-card shadow-2xl"
          >
            <div class="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <div class="flex items-center gap-2.5">
                <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <MessageSquare class="h-4 w-4" />
                </div>
                <h3 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h3>
              </div>
              <button
                type="button"
                class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                @click="emit('close')"
              >
                <X class="h-4 w-4" />
              </button>
            </div>

            <div v-if="isLoading" class="flex items-center justify-center py-16">
              <Loader2 class="h-6 w-6 animate-spin text-muted-foreground" />
            </div>

            <div v-else-if="errorKey && !detail" class="px-5 py-5">
              <div class="rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-sm text-red-600 dark:text-red-400">
                {{ t(errorKey) }}
              </div>
            </div>

            <div v-else-if="detail" class="space-y-5 px-5 py-5">
              <div class="space-y-3">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionTicket`) }}</p>
                <div class="rounded-xl border border-border/40 bg-surface/30 p-4 space-y-3">
                  <div class="flex items-start justify-between gap-3">
                    <p class="text-sm font-semibold text-foreground">{{ detail.title }}</p>
                    <div class="flex shrink-0 items-center gap-2">
                      <span class="rounded-full px-2.5 py-1 text-xs font-medium" :class="statusBadgeClass(detail.status)">
                        {{ t(`admin.tickets.status.${detail.status}`) }}
                      </span>
                      <select
                        :value="detail.status"
                        class="h-7 rounded-md border border-border/50 bg-surface px-1.5 text-xs text-foreground outline-none focus:border-primary"
                        :disabled="isStatusUpdating"
                        @change="handleStatusChange"
                      >
                        <option v-for="status in statusOptions" :key="status" :value="status">
                          {{ t(`admin.tickets.status.${status}`) }}
                        </option>
                      </select>
                    </div>
                  </div>
                  <div class="grid grid-cols-2 gap-3 text-xs">
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.category`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ detail.category }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.priority`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ detail.priority }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.manualEmail`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ detail.manualEmail }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.lastMessageAt`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ formatDateTime(detail.lastMessageAt) }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.sub2apiUserId`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ detail.sub2apiUserId || t('admin.tickets.common.placeholder') }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.sub2apiEmail`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ detail.sub2apiEmail || t('admin.tickets.common.placeholder') }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.sub2apiRole`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ detail.sub2apiRole || t('admin.tickets.common.placeholder') }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.sub2apiSrcHost`) }}</p>
                      <p class="mt-0.5 truncate text-foreground" :title="detail.sub2apiSrcHost">{{ detail.sub2apiSrcHost || t('admin.tickets.common.placeholder') }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <div class="space-y-3 border-t border-border/40 pt-5">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionMessages`) }}</p>
                <div class="max-h-80 space-y-3 overflow-y-auto rounded-lg border border-border/40 p-3">
                  <div v-for="message in detail.messages" :key="message.id" class="space-y-1">
                    <div class="flex items-center justify-between text-xs text-muted-foreground">
                      <span class="font-medium text-foreground">
                        {{ message.authorType === 'admin' ? t(`${prefix}.authorAdmin`) : (message.authorName || t(`${prefix}.authorCustomer`)) }}
                      </span>
                      <span>{{ formatDateTime(message.createdAt) }}</span>
                    </div>
                    <p class="whitespace-pre-wrap rounded-lg bg-surface/50 p-3 text-sm text-foreground">{{ message.body }}</p>
                    <div v-if="message.attachments.length > 0" class="flex flex-wrap gap-2">
                      <AdminAttachmentThumbnail
                        v-for="attachment in message.attachments"
                        :key="attachment.id"
                        :attachment="attachment"
                        @preview="openAttachmentPreview"
                      />
                    </div>
                  </div>
                </div>
              </div>

              <div class="space-y-2 border-t border-border/40 pt-5">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionReply`) }}</p>
                <textarea
                  v-model="replyBody"
                  rows="3"
                  class="w-full rounded-lg border border-border/50 bg-surface px-3 py-2 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
                  :placeholder="t(`${prefix}.replyPlaceholder`)"
                />
                <div v-if="replyErrorKey" class="flex items-start gap-2 rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-xs text-red-600 dark:text-red-400">
                  <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span>{{ t(replyErrorKey) }}</span>
                </div>
                <div class="flex justify-end">
                  <button
                    type="button"
                    class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
                    :disabled="isReplying || !replyBody.trim()"
                    @click="submitReply"
                  >
                    <Loader2 v-if="isReplying" class="h-3.5 w-3.5 animate-spin" />
                    <Send v-else class="h-3.5 w-3.5" />
                    {{ t(`${prefix}.send`) }}
                  </button>
                </div>
              </div>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>

  <AdminAttachmentPreviewModal
    :open="!!previewImage"
    :url="previewImage?.url ?? null"
    :name="previewImage?.name ?? null"
    @close="closeAttachmentPreview"
  />
</template>
