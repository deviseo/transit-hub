<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { X, ClipboardList, Loader2, Play, Square, Ban } from 'lucide-vue-next'
import { getGroupRateCampaign } from '../../api/groupRateCampaigns'
import type { CampaignDetail } from '../../types/groupRateCampaigns'

const props = defineProps<{
  open: boolean
  campaignId: string | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'start', id: string): void
  (event: 'end', id: string): void
  (event: 'cancel', id: string): void
}>()

import { t, locale } from '@/locales'
const prefix = 'admin.groupRateCampaigns.detail'

const detail = ref<CampaignDetail | null>(null)
const isLoading = ref(false)
const errorKey = ref<string | null>(null)
const isActionLoading = ref(false)

const load = async () => {
  if (!props.campaignId) return
  isLoading.value = true
  errorKey.value = null
  try {
    detail.value = await getGroupRateCampaign(props.campaignId)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.groupRateCampaigns.errors.unknown'
  } finally {
    isLoading.value = false
  }
}

watch(() => [props.open, props.campaignId], ([isOpen]) => {
  if (isOpen) {
    detail.value = null
    void load()
  }
})

const canStart = computed(() => detail.value?.status === 'draft' || detail.value?.status === 'scheduled')
const canEnd = computed(() => detail.value?.status === 'running' || detail.value?.status === 'partial')
const canCancel = computed(() => detail.value?.status === 'draft' || detail.value?.status === 'scheduled')

const handleStart = async () => {
  if (!detail.value) return
  isActionLoading.value = true
  try {
    emit('start', detail.value.id)
  } finally {
    isActionLoading.value = false
  }
}

const handleEnd = async () => {
  if (!detail.value) return
  if (!window.confirm(t(`${prefix}.confirmEnd`))) return
  isActionLoading.value = true
  try {
    emit('end', detail.value.id)
  } finally {
    isActionLoading.value = false
  }
}

const handleCancel = async () => {
  if (!detail.value) return
  if (!window.confirm(t(`${prefix}.confirmCancel`))) return
  isActionLoading.value = true
  try {
    emit('cancel', detail.value.id)
  } finally {
    isActionLoading.value = false
  }
}

const formatMultiplier = (value: number | null): string => (
  value === null ? '—' : `${Number(value.toFixed(4)).toString()}×`
)

const formatDateTime = (value: string | null): string => {
  if (!value) return '—'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

const statusBadgeClass = (status: string): string => {
  switch (status) {
    case 'running':
      return 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400'
    case 'scheduled':
      return 'bg-blue-500/10 text-blue-600 dark:text-blue-400'
    case 'ended':
      return 'bg-surface-elevated text-muted-foreground'
    case 'partial':
    case 'ending':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
    case 'failed':
    case 'cancelled':
      return 'bg-red-500/10 text-red-600 dark:text-red-400'
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
            role="dialog"
            aria-modal="true"
            :aria-label="t(`${prefix}.title`)"
            class="absolute bottom-0 right-0 top-0 w-full max-w-xl overflow-y-auto overscroll-contain border-l border-border/60 bg-card shadow-2xl"
          >
            <div class="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <div class="flex items-center gap-2.5">
                <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <ClipboardList class="h-4 w-4" />
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

            <div v-else-if="errorKey" class="px-5 py-5">
              <div class="rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-sm text-red-600 dark:text-red-400">
                {{ t(errorKey) }}
              </div>
            </div>

            <div v-else-if="detail" class="space-y-5 px-5 py-5">
              <div class="space-y-3">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionConfig`) }}</p>
                <div class="rounded-xl border border-border/40 bg-surface/30 p-4 space-y-3">
                  <div class="flex items-start justify-between gap-3">
                    <div>
                      <p class="text-sm font-semibold text-foreground">{{ detail.name }}</p>
                      <p v-if="detail.description" class="mt-1 text-xs text-muted-foreground">{{ detail.description }}</p>
                    </div>
                    <span class="shrink-0 rounded-full px-2.5 py-1 text-xs font-medium" :class="statusBadgeClass(detail.status)">
                      {{ t(`admin.groupRateCampaigns.status.${detail.status}`) }}
                    </span>
                  </div>
                  <div class="grid grid-cols-2 gap-3 text-xs">
                    <div>
                      <p class="text-muted-foreground">{{ t(`admin.groupRateCampaigns.fields.startAt`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ formatDateTime(detail.startedAt ?? detail.startAt) }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`admin.groupRateCampaigns.fields.endAt`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ formatDateTime(detail.endedAt ?? detail.endAt) }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`admin.groupRateCampaigns.fields.createdBy`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ detail.createdBy }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`admin.groupRateCampaigns.fields.summary`) }}</p>
                      <p class="mt-0.5 text-foreground">
                        {{ t('admin.groupRateCampaigns.format.summary', { applied: detail.summary.applied, total: detail.summary.total }) }}
                      </p>
                    </div>
                  </div>
                </div>
              </div>

              <div class="space-y-3 border-t border-border/40 pt-5">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionItems`) }}</p>
                <div class="max-h-96 overflow-y-auto rounded-lg border border-border/40">
                  <table class="w-full text-left text-xs">
                    <thead class="sticky top-0 bg-surface-elevated">
                      <tr>
                        <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.itemGroupName`) }}</th>
                        <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.itemOriginal`) }}</th>
                        <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.itemCampaign`) }}</th>
                        <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.itemRestored`) }}</th>
                        <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.itemApplyStatus`) }}</th>
                        <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.itemRestoreStatus`) }}</th>
                      </tr>
                    </thead>
                    <tbody class="divide-y divide-border/30">
                      <tr v-for="item in detail.items" :key="item.groupId || item.groupName">
                        <td class="px-3 py-2 text-foreground">{{ item.groupName }}</td>
                        <td class="px-3 py-2 text-muted-foreground">{{ formatMultiplier(item.originalMultiplier) }}</td>
                        <td class="px-3 py-2 font-semibold text-primary">{{ formatMultiplier(item.campaignMultiplier) }}</td>
                        <td class="px-3 py-2 text-muted-foreground">{{ formatMultiplier(item.restoredMultiplier) }}</td>
                        <td class="px-3 py-2">
                          <span :title="item.applyReason || t(`${prefix}.noReason`)">{{ item.applyStatus }}</span>
                        </td>
                        <td class="px-3 py-2">
                          <span :title="item.restoreReason || t(`${prefix}.noReason`)">{{ item.restoreStatus }}</span>
                        </td>
                      </tr>
                    </tbody>
                  </table>
                </div>
              </div>
            </div>

            <div v-if="detail" class="sticky bottom-0 flex items-center justify-end gap-2 border-t border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <button
                v-if="canCancel"
                type="button"
                class="inline-flex items-center gap-1.5 rounded-lg border border-red-500/30 px-4 py-2 text-sm font-medium text-red-600 transition-colors hover:bg-red-500/10 disabled:opacity-50 dark:text-red-400"
                :disabled="isActionLoading"
                @click="handleCancel"
              >
                <Ban class="h-3.5 w-3.5" />
                {{ t('admin.groupRateCampaigns.actions.cancel') }}
              </button>
              <button
                v-if="canEnd"
                type="button"
                class="inline-flex items-center gap-1.5 rounded-lg border border-border/50 px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground disabled:opacity-50"
                :disabled="isActionLoading"
                @click="handleEnd"
              >
                <Square class="h-3.5 w-3.5" />
                {{ t('admin.groupRateCampaigns.actions.end') }}
              </button>
              <button
                v-if="canStart"
                type="button"
                class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
                :disabled="isActionLoading"
                @click="handleStart"
              >
                <Play class="h-3.5 w-3.5" />
                {{ t('admin.groupRateCampaigns.actions.start') }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
