<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ImageOff, Loader2 } from 'lucide-vue-next'
import { fetchEmbedAttachmentBlob } from '../api/tickets'
import type { EmbedTicketAttachment } from '../types'

const props = defineProps<{
  attachment: EmbedTicketAttachment
}>()

const { t } = useI18n()

const objectUrl = ref<string | null>(null)
const isLoading = ref(true)
const hasError = ref(false)

const revoke = () => {
  if (objectUrl.value) {
    URL.revokeObjectURL(objectUrl.value)
    objectUrl.value = null
  }
}

// <img> 标签没办法带 Authorization 请求头，所以图片必须先用带鉴权的 fetch 取回 blob，
// 再转成临时的 object URL 赋给 <img src>；组件卸载或附件切换时释放，避免内存泄漏。
const load = async () => {
  isLoading.value = true
  hasError.value = false
  revoke()
  try {
    const blob = await fetchEmbedAttachmentBlob(props.attachment.id)
    objectUrl.value = URL.createObjectURL(blob)
  } catch (error) {
    hasError.value = true
  } finally {
    isLoading.value = false
  }
}

watch(() => props.attachment.id, load, { immediate: true })
onBeforeUnmount(revoke)

// 点击缩略图在新标签页打开已经取回的图片内容（本身就是走鉴权接口拿到的 blob），
// 不是重新发起一次未鉴权的公开请求。
const openFullSize = () => {
  if (objectUrl.value) window.open(objectUrl.value, '_blank', 'noopener,noreferrer')
}
</script>

<template>
  <button
    type="button"
    class="flex h-16 w-16 shrink-0 items-center justify-center overflow-hidden rounded-lg border border-border/50 bg-surface-elevated transition-colors hover:border-primary/50 disabled:cursor-default disabled:hover:border-border/50"
    :disabled="isLoading || hasError"
    :title="attachment.originalName"
    @click="openFullSize"
  >
    <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin text-muted-foreground" />
    <ImageOff v-else-if="hasError" class="h-4 w-4 text-muted-foreground" :aria-label="t('embed.tickets.attachments.loadFailed')" />
    <img v-else :src="objectUrl!" :alt="attachment.originalName" class="h-full w-full object-cover" />
  </button>
</template>
