<script setup lang="ts">
import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, User, X } from 'lucide-vue-next'
import { getSub2apiUserProfile } from '../../api/tickets'
import type { Sub2apiUserProfile } from '../../types/tickets'

const props = defineProps<{
  open: boolean
  ticketId: string | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t, locale } = useI18n()
const prefix = 'admin.tickets.sub2apiProfile'

const profile = ref<Sub2apiUserProfile | null>(null)
const isLoading = ref(false)
const errorKey = ref<string | null>(null)

const load = async () => {
  if (!props.ticketId) return
  isLoading.value = true
  errorKey.value = null
  profile.value = null
  try {
    profile.value = await getSub2apiUserProfile(props.ticketId)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.tickets.errors.unknown'
  } finally {
    isLoading.value = false
  }
}

watch(() => [props.open, props.ticketId], ([isOpen]) => {
  if (isOpen) void load()
})

const formatDateTime = (value: string | null | undefined): string => {
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

const formatAmount = (value: number | null | undefined): string => (
  typeof value === 'number' ? value.toFixed(2) : t('admin.tickets.common.placeholder')
)

// remoteUnavailableReason 是后端直接下发的 i18n key（如
// admin.tickets.sub2apiProfile.remoteUnavailable.noAdminSession），可以直接传给 t() 使用。
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
      <div v-if="open" class="fixed inset-0 z-[150] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="emit('close')" />

        <Transition
          enter-active-class="transition duration-200 ease-out"
          enter-from-class="opacity-0 scale-95"
          enter-to-class="opacity-100 scale-100"
          leave-active-class="transition duration-150 ease-in"
          leave-from-class="opacity-100 scale-100"
          leave-to-class="opacity-0 scale-95"
        >
          <div v-if="open" class="relative w-full max-w-md max-h-[85vh] overflow-y-auto rounded-2xl border border-border/60 bg-card shadow-2xl">
            <div class="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <div class="flex items-center gap-2.5">
                <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <User class="h-4 w-4" />
                </div>
                <h2 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h2>
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

            <div v-else-if="profile" class="space-y-5 px-5 py-5">
              <div
                v-if="profile.remoteUnavailableReason"
                class="flex items-start gap-2 rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-xs text-amber-700 dark:text-amber-400"
              >
                <AlertCircle class="mt-0.5 h-3.5 w-3.5 shrink-0" />
                <span>{{ t(profile.remoteUnavailableReason) }}</span>
              </div>

              <div class="space-y-3">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionIdentity`) }}</p>
                <div class="rounded-xl border border-border/40 bg-surface/30 p-4">
                  <div class="grid grid-cols-2 gap-3 text-xs">
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.userId`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ profile.sub2apiUserId || t('admin.tickets.common.placeholder') }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.email`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ profile.sub2apiEmail || t('admin.tickets.common.placeholder') }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.role`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ profile.sub2apiRole || t('admin.tickets.common.placeholder') }}</p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.srcHost`) }}</p>
                      <p class="mt-0.5 truncate text-foreground" :title="profile.sub2apiSrcHost">{{ profile.sub2apiSrcHost || t('admin.tickets.common.placeholder') }}</p>
                    </div>
                    <div v-if="profile.username">
                      <p class="text-muted-foreground">{{ t(`${prefix}.username`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ profile.username }}</p>
                    </div>
                    <div v-if="profile.status">
                      <p class="text-muted-foreground">{{ t(`${prefix}.status`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ profile.status }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <div class="space-y-3 border-t border-border/40 pt-5">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionBalance`) }}</p>
                <div class="rounded-xl border border-border/40 bg-surface/30 p-4">
                  <div class="grid grid-cols-2 gap-3 text-xs">
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.balance`) }}</p>
                      <p class="mt-0.5 text-foreground">
                        {{ profile.balanceAvailable ? formatAmount(profile.balance) : t(`${prefix}.unavailable`) }}
                      </p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.totalRecharged`) }}</p>
                      <p class="mt-0.5 text-foreground">
                        {{ profile.totalRechargedAvailable ? formatAmount(profile.totalRecharged) : t(`${prefix}.unavailable`) }}
                      </p>
                    </div>
                    <div>
                      <p class="text-muted-foreground">{{ t(`${prefix}.registeredAt`) }}</p>
                      <p class="mt-0.5 text-foreground">
                        {{ profile.registeredAtAvailable ? formatDateTime(profile.registeredAt) : t(`${prefix}.unavailable`) }}
                      </p>
                    </div>
                    <div v-if="profile.frozenBalance !== undefined">
                      <p class="text-muted-foreground">{{ t(`${prefix}.frozenBalance`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ formatAmount(profile.frozenBalance) }}</p>
                    </div>
                    <div v-if="profile.concurrency !== undefined">
                      <p class="text-muted-foreground">{{ t(`${prefix}.concurrency`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ profile.concurrency }}</p>
                    </div>
                    <div v-if="profile.rpmLimit !== undefined">
                      <p class="text-muted-foreground">{{ t(`${prefix}.rpmLimit`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ profile.rpmLimit }}</p>
                    </div>
                    <div v-if="profile.lastUsedAt">
                      <p class="text-muted-foreground">{{ t(`${prefix}.lastUsedAt`) }}</p>
                      <p class="mt-0.5 text-foreground">{{ formatDateTime(profile.lastUsedAt) }}</p>
                    </div>
                  </div>
                </div>
              </div>

              <div class="space-y-3 border-t border-border/40 pt-5">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionRechargeHistory`) }}</p>
                <div v-if="!profile.rechargeHistoryAvailable" class="rounded-xl border border-border/40 bg-surface/30 p-4 text-center text-xs text-muted-foreground">
                  {{ t(`${prefix}.unavailable`) }}
                </div>
                <div v-else-if="profile.rechargeHistory && profile.rechargeHistory.length > 0" class="space-y-2">
                  <div
                    v-for="item in profile.rechargeHistory"
                    :key="item.id"
                    class="flex items-center justify-between gap-3 rounded-lg border border-border/30 bg-surface/20 px-3 py-2 text-xs"
                  >
                    <div class="min-w-0">
                      <p class="truncate text-foreground">{{ item.type || t('admin.tickets.common.placeholder') }}</p>
                      <p class="text-muted-foreground">{{ formatDateTime(item.createdAt) }}</p>
                      <p v-if="item.note" class="mt-0.5 truncate text-muted-foreground" :title="item.note">{{ item.note }}</p>
                    </div>
                    <p class="shrink-0 font-medium text-foreground">{{ formatAmount(item.amount) }}</p>
                  </div>
                </div>
                <div v-else class="rounded-xl border border-border/40 bg-surface/30 p-4 text-center text-xs text-muted-foreground">
                  {{ t(`${prefix}.historyEmpty`) }}
                </div>
              </div>
            </div>

            <div v-else class="px-5 py-16 text-center text-sm text-muted-foreground">
              {{ t(`${prefix}.empty`) }}
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
