<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'

// 站内放大预览工单图片：复用缩略图已经取到的临时地址，不重新下载。
defineProps<{ url: string; name: string }>()
const emit = defineEmits<{ close: [] }>()
const { t } = useI18n()

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') emit('close')
}
onMounted(() => document.addEventListener('keydown', onKeydown))
onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))
</script>

<template>
  <div class="fixed inset-0 z-[60] flex items-center justify-center bg-black/70 p-4" role="dialog" aria-modal="true" :aria-label="name" @click.self="emit('close')">
    <button type="button" class="absolute right-4 top-4 rounded-full bg-black/50 p-2 text-white hover:bg-black/70 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white" :aria-label="t('admin.tickets.detail.closePreview')" @click="emit('close')">
      <X class="h-5 w-5" />
    </button>
    <img :src="url" :alt="name" class="max-h-[90vh] max-w-[90vw] rounded-lg object-contain shadow-2xl" />
  </div>
</template>
