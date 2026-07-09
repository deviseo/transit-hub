<script setup lang="ts">
import { onBeforeUnmount, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { X } from 'lucide-vue-next'

const props = defineProps<{
  open: boolean
  url: string | null
  name: string | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()

// Esc 关闭预览；只在预览打开时响应，避免和页面上其它 Esc 行为冲突。
const handleKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape' && props.open) emit('close')
}

onMounted(() => window.addEventListener('keydown', handleKeydown))
onBeforeUnmount(() => window.removeEventListener('keydown', handleKeydown))
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
      <div v-if="open && url" class="fixed inset-0 z-[200] flex items-center justify-center p-4 sm:p-8">
        <div class="absolute inset-0 bg-background/80 backdrop-blur-sm" @click="emit('close')" />

        <div class="relative flex max-h-full max-w-full flex-col items-center gap-3">
          <button
            type="button"
            class="absolute -top-3 -right-3 z-10 flex h-8 w-8 items-center justify-center rounded-full border border-border/60 bg-card text-muted-foreground shadow-lg transition-colors hover:text-foreground"
            :aria-label="t('admin.tickets.detail.closePreview')"
            :title="t('admin.tickets.detail.closePreview')"
            @click="emit('close')"
          >
            <X class="h-4 w-4" />
          </button>
          <img
            :src="url"
            :alt="name ?? t('admin.tickets.detail.previewImage')"
            class="max-h-[80vh] max-w-full rounded-lg object-contain shadow-2xl sm:max-h-[85vh]"
          />
          <p v-if="name" class="max-w-full truncate rounded-full bg-background/70 px-3 py-1 text-xs text-foreground backdrop-blur">
            {{ name }}
          </p>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
