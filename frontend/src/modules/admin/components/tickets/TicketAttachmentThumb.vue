<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ImageOff, Loader2 } from 'lucide-vue-next'
import { fetchTicketAttachment } from '../../api/tickets'
import type { TicketAttachment } from '../../types/tickets'

const props = defineProps<{
  attachment: TicketAttachment
}>()

const emit = defineEmits<{
  (event: 'preview', payload: { url: string; name: string }): void
}>()

const { t } = useI18n()

const thumbUrl = ref<string | null>(null)
const fullUrl = ref<string | null>(null)
const isLoading = ref(true)
const isOpening = ref(false)
const hasError = ref(false)

const revoke = () => {
  for (const url of [thumbUrl.value, fullUrl.value]) {
    if (url) URL.revokeObjectURL(url)
  }
  thumbUrl.value = null
  fullUrl.value = null
}

// <img> 标签没办法带 Authorization 请求头，所以图片必须先用带鉴权的 fetch 取回 blob，
// 再转成临时的 object URL 赋给 <img src>；组件卸载或附件切换时释放，避免内存泄漏。
// 列表里只下载缩略图（几十 KB），点开时才下载原图。
const load = async () => {
  isLoading.value = true
  hasError.value = false
  revoke()
  try {
    thumbUrl.value = URL.createObjectURL(await fetchTicketAttachment(props.attachment.id, true))
  } catch {
    hasError.value = true
  } finally {
    isLoading.value = false
  }
}

watch(() => props.attachment.id, load, { immediate: true })
onBeforeUnmount(revoke)

// 点击后在站内弹出放大预览层（由 TicketDetailDrawer 统一管理）。第一次点开时下载原图，之后复用。
const openFullSize = async () => {
  if (isOpening.value) return
  if (!fullUrl.value) {
    isOpening.value = true
    try {
      fullUrl.value = URL.createObjectURL(await fetchTicketAttachment(props.attachment.id))
    } catch {
      hasError.value = true
      return
    } finally {
      isOpening.value = false
    }
  }
  emit('preview', { url: fullUrl.value, name: props.attachment.originalName })
}
</script>

<template>
  <button
    type="button"
    class="relative flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-border/50 bg-surface-elevated transition-colors hover:border-primary/50 disabled:cursor-default disabled:hover:border-border/50"
    :disabled="isLoading || hasError"
    :title="attachment.originalName"
    :aria-busy="isOpening"
    @click="openFullSize"
  >
    <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin text-muted-foreground" />
    <ImageOff v-else-if="hasError" class="h-4 w-4 text-muted-foreground" :aria-label="t('admin.tickets.detail.attachmentLoadFailed')" />
    <img v-else :src="thumbUrl!" :alt="attachment.originalName" class="h-full w-full object-cover" />
    <span v-if="isOpening" class="absolute inset-0 flex items-center justify-center bg-background/60">
      <Loader2 class="h-4 w-4 animate-spin text-foreground" />
    </span>
  </button>
</template>
