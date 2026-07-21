<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { Loader2, ShieldCheck, ShieldQuestion, X } from 'lucide-vue-next'
import { connectionHealthMessageKey, useConnectionHealth } from '../../composables/useConnectionHealth'
import type { ConnectionHealthPolicy } from '../../types/connectionHealth'

const props = defineProps<{
  open: boolean
  targetId: string
  accountName: string
  policies: ConnectionHealthPolicy[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'saved'): void
}>()

const { t, te } = useI18n()
const prefix = 'admin.connectionHealth.policyAssignment'
const { loadTargetPolicyAssignments, saveTargetPolicyAssignments } = useConnectionHealth()

type Phase = 'loading' | 'ready' | 'saving' | 'error'

const phase = ref<Phase>('loading')
const selected = ref<Set<string>>(new Set())
const loadErrorKey = ref('')
const saveErrorKey = ref('')
const readableMessage = (rawKey: string): string => t(connectionHealthMessageKey(rawKey, te))

watch(
  () => [props.open, props.targetId],
  async ([isOpen]) => {
    if (!isOpen || !props.targetId) return
    phase.value = 'loading'
    selected.value = new Set()
    loadErrorKey.value = ''
    saveErrorKey.value = ''

    const outcome = await loadTargetPolicyAssignments(props.targetId)
    if ('errorKey' in outcome) {
      loadErrorKey.value = outcome.errorKey
      phase.value = 'error'
      return
    }
    selected.value = new Set(outcome.assignments.policyIds)
    phase.value = 'ready'
  },
)

const hasPolicies = computed(() => props.policies.length > 0)

const toggle = (policyId: string) => {
  if (phase.value === 'saving') return
  const next = new Set(selected.value)
  if (next.has(policyId)) {
    next.delete(policyId)
  } else {
    next.add(policyId)
  }
  selected.value = next
}

const save = async () => {
  if (phase.value === 'saving' || !props.targetId) return
  phase.value = 'saving'
  saveErrorKey.value = ''
  const outcome = await saveTargetPolicyAssignments(props.targetId, Array.from(selected.value))
  if ('errorKey' in outcome) {
    saveErrorKey.value = outcome.errorKey
    phase.value = 'ready'
    return
  }
  phase.value = 'ready'
  emit('saved')
}

const close = () => {
  if (phase.value === 'saving') return
  emit('close')
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
      <div v-if="open" class="fixed inset-0 z-[150] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="close" />

      <div role="dialog" aria-modal="true" :aria-label="t(`${prefix}.title`)" class="relative flex h-[min(640px,calc(100dvh-2rem))] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl">
          <div class="flex shrink-0 items-center justify-between gap-3 border-b border-border/60 px-5 py-4">
            <div class="flex min-w-0 items-center gap-2.5">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <ShieldCheck class="h-4 w-4" />
              </div>
              <div class="min-w-0">
                <h3 class="truncate text-sm font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h3>
                <p class="truncate text-xs text-muted-foreground">{{ accountName }} · {{ t(`${prefix}.subtitle`) }}</p>
              </div>
            </div>
            <button
              type="button"
              class="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
              @click="close"
            >
              <X class="h-4 w-4" />
            </button>
          </div>

          <div class="flex-1 overflow-y-auto px-5 py-4">
            <div v-if="phase === 'loading'" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
              <Loader2 class="h-6 w-6 animate-spin text-primary/60" />
            </div>

            <div v-else-if="loadErrorKey" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
              <ShieldQuestion class="h-8 w-8 text-red-500/70" />
              <p class="text-sm text-red-600 dark:text-red-400">{{ readableMessage(loadErrorKey) }}</p>
            </div>

            <template v-else>
              <div v-if="!hasPolicies" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
                <ShieldQuestion class="h-8 w-8 text-muted-foreground/40" />
                <p class="text-sm text-muted-foreground">{{ t(`${prefix}.empty`) }}</p>
              </div>

              <ul v-else class="space-y-2">
                <li
                  v-for="policy in policies"
                  :key="policy.id"
                  class="flex items-start gap-3 rounded-lg border border-border/40 px-3 py-2.5"
                >
                  <input
                    type="checkbox"
                    class="mt-0.5 h-4 w-4 shrink-0 rounded border-border/60"
                    :disabled="phase === 'saving'"
                    :checked="selected.has(policy.id)"
                    @change="toggle(policy.id)"
                  />
                  <div class="min-w-0 flex-1">
                    <div class="flex flex-wrap items-center gap-2">
                      <span class="text-sm font-medium text-foreground">{{ policy.name }}</span>
                      <span
                        class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                        :class="policy.enabled ? 'bg-green-500/10 text-green-600 dark:text-green-400' : 'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400'"
                      >
                        {{ policy.enabled ? t('admin.connectionHealth.policies.enabled') : t('admin.connectionHealth.policies.disabled') }}
                      </span>
                    </div>
                    <p class="mt-0.5 text-xs text-muted-foreground">
                      {{ t('admin.connectionHealth.policies.modelTargetCount', { count: policy.modelTargets.filter((m) => m.enabled).length }) }}
                      · {{ policy.probeIntervalSeconds }}s
                    </p>
                  </div>
                </li>
              </ul>

            <p v-if="saveErrorKey" class="mt-4 rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400">
              {{ readableMessage(saveErrorKey) }}
            </p>
            </template>
          </div>

          <div class="flex shrink-0 items-center justify-end gap-2 border-t border-border/60 px-5 py-4">
            <button type="button" class="rounded-lg px-3 py-1.5 text-sm text-muted-foreground hover:bg-surface-line" @click="close">
              {{ t(`${prefix}.cancel`) }}
            </button>
            <button
              type="button"
              class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="phase === 'loading' || phase === 'saving'"
              @click="save"
            >
              <Loader2 v-if="phase === 'saving'" class="h-4 w-4 animate-spin" />
              {{ t(`${prefix}.save`) }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
