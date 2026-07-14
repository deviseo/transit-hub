<script setup lang="ts">
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Inbox, Loader2, Plus, RefreshCw } from 'lucide-vue-next'
import EmbedTicketCreateModal from './EmbedTicketCreateModal.vue'
import type { EmbedTicketListItem, TicketEmbedTemplate } from '../types'

const props = withDefaults(defineProps<{
  tickets: EmbedTicketListItem[]
  isLoading: boolean
  errorKey: string | null
  template?: TicketEmbedTemplate
  maxImages?: number
  categoryOptions?: string[]
  priorityOptions?: string[]
}>(), {
  template: 'default',
  maxImages: 0,
  categoryOptions: () => [],
  priorityOptions: () => [],
})

const emit = defineEmits<{
  (event: 'select', id: string): void
  (event: 'refresh'): void
  (event: 'created', id: string): void
}>()

const { t, locale } = useI18n()

const isCreateModalOpen = ref(false)

const handleCreated = (id: string) => {
  emit('created', id)
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

// default 现在嵌套在 TicketEmbedPage.vue 的整块外壳卡片里（见 pageShellClass），
// 所以这里不再给 default 的分区/列表项套自己的 border+bg-card 小卡片，改用分隔线和更充分的
// padding，避免"卡片里套卡片"的拥挤感；minimal/support 的取值原样保留，不受这次调整影响。
const rootSpacingClass = computed(() => (props.template === 'default' ? 'space-y-6' : 'space-y-4'))

const titleClass = computed(() => (
  props.template === 'default' ? 'text-lg font-semibold text-foreground' : 'text-base font-semibold text-foreground'
))

const itemClass = computed(() => {
  if (props.template === 'minimal') {
    return 'flex w-full flex-col gap-2 border-b border-border/30 bg-transparent px-1 py-3 text-left transition-colors hover:bg-surface/20'
  }
  if (props.template === 'support') {
    return 'flex w-full flex-col gap-2 rounded-xl border border-l-4 border-border/50 border-l-primary/50 bg-card p-4 text-left transition-colors hover:border-primary/60 hover:bg-surface/30'
  }
  return 'flex w-full flex-col gap-2 px-1 py-4 text-left transition-colors hover:bg-surface/40 sm:px-2'
})

const listContainerClass = computed(() => {
  if (props.template === 'minimal') return ''
  if (props.template === 'support') return 'space-y-2'
  return 'divide-y divide-border/40'
})

const emptyCardClass = computed(() => {
  if (props.template === 'minimal') return 'flex flex-col items-center justify-center border-t border-border/30 px-6 py-12 text-center'
  if (props.template === 'support') return 'flex flex-col items-center justify-center rounded-xl border border-border/50 bg-card px-6 py-12 text-center'
  return 'flex flex-col items-center justify-center px-6 py-16 text-center sm:py-20'
})
</script>

<template>
  <div :class="rootSpacingClass">
    <!-- 顶部标题独占一行，操作按钮在标题下方靠左排布，不贴右上角——避免和 Sub2API 外层 iframe
         右上角的"新窗口打开"按钮重叠或挤在一起。 -->
    <div class="space-y-3">
      <h2 :class="titleClass">{{ t('embed.tickets.list.title') }}</h2>
      <div class="flex flex-wrap items-center gap-2">
        <button
          type="button"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg border border-border/50 px-3 text-sm text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground disabled:opacity-50"
          :disabled="props.isLoading"
          @click="emit('refresh')"
        >
          <Loader2 v-if="props.isLoading" class="h-3.5 w-3.5 animate-spin" />
          <RefreshCw v-else class="h-3.5 w-3.5" />
          {{ t('embed.tickets.list.refresh') }}
        </button>
        <button
          type="button"
          class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-primary px-3 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90"
          @click="isCreateModalOpen = true"
        >
          <Plus class="h-3.5 w-3.5" />
          {{ t('embed.tickets.list.create') }}
        </button>
      </div>
    </div>

    <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-xs text-warning">
      <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
      <span>{{ t(errorKey) }}</span>
    </div>

    <div v-if="isLoading && tickets.length === 0" class="flex items-center justify-center py-12 text-muted-foreground">
      <Loader2 class="mr-2 h-5 w-5 animate-spin" />
      {{ t('embed.tickets.list.loading') }}
    </div>

    <div v-else-if="tickets.length === 0" :class="emptyCardClass">
      <div class="flex h-12 w-12 items-center justify-center rounded-2xl border border-border/50 bg-surface-elevated text-muted-foreground">
        <Inbox class="h-5 w-5" />
      </div>
      <h3 class="mt-4 font-semibold text-foreground">{{ t('embed.tickets.list.emptyTitle') }}</h3>
      <p class="mt-2 max-w-sm text-sm text-muted-foreground">{{ t('embed.tickets.list.emptyDescription') }}</p>
    </div>

    <div v-else :class="listContainerClass">
      <button
        v-for="ticket in tickets"
        :key="ticket.id"
        type="button"
        :class="itemClass"
        @click="emit('select', ticket.id)"
      >
        <div class="flex items-start justify-between gap-3">
          <p class="text-sm font-medium text-foreground">{{ ticket.title }}</p>
          <span :class="['inline-flex shrink-0 rounded-md border px-2 py-0.5 text-xs font-semibold', statusBadgeClass(ticket.status)]">
            {{ t(`embed.tickets.status.${ticket.status}`) }}
          </span>
        </div>
        <div class="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
          <span class="inline-flex items-center rounded-md border border-border/50 bg-surface-elevated px-1.5 py-0.5">{{ ticket.category }}</span>
          <span class="inline-flex items-center rounded-md border border-border/50 bg-surface-elevated px-1.5 py-0.5">{{ ticket.priority }}</span>
        </div>
        <div class="flex items-center justify-between text-xs text-muted-foreground">
          <span class="truncate">{{ ticket.manualEmail }}</span>
          <span class="shrink-0">{{ formatDateTime(ticket.lastMessageAt) }}</span>
        </div>
      </button>
    </div>

    <EmbedTicketCreateModal
      :open="isCreateModalOpen"
      :max-images="maxImages"
      :category-options="categoryOptions"
      :priority-options="priorityOptions"
      @close="isCreateModalOpen = false"
      @created="handleCreated"
    />
  </div>
</template>
