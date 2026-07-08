<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ShieldAlert, X, Zap } from 'lucide-vue-next'
import type { ProbeModelCandidate } from '../../types/connectionHealth'

const props = defineProps<{
  open: boolean
  siteName: string
  upstreamGroupName: string
  ownGroupName: string
  candidates: ProbeModelCandidate[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'confirm', models: string[]): void
}>()

const { t } = useI18n()
const prefix = 'admin.connectionHealth.probeDialog'

const selected = ref<Set<string>>(new Set())

// 弹窗每次打开时默认全选当前匹配到的候选模型，但用户必须能看到具体选中了哪些模型
// （不是隐式全量探活），可以取消勾选后再确认。
watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      selected.value = new Set(props.candidates.map((c) => c.modelName))
    }
  },
)

const toggle = (modelName: string) => {
  const next = new Set(selected.value)
  if (next.has(modelName)) {
    next.delete(modelName)
  } else {
    next.add(modelName)
  }
  selected.value = next
}

const hasCandidates = computed(() => props.candidates.length > 0)
const canConfirm = computed(() => hasCandidates.value && selected.value.size > 0)

const handleConfirm = () => {
  if (!canConfirm.value) return
  emit('confirm', Array.from(selected.value))
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
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="emit('close')" />

        <div class="relative w-full max-w-lg overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl">
          <div class="flex items-center justify-between gap-3 border-b border-border/60 px-5 py-4">
            <div class="flex min-w-0 items-center gap-2.5">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Zap class="h-4 w-4" />
              </div>
              <div class="min-w-0">
                <h3 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h3>
                <p class="truncate text-xs text-muted-foreground">{{ siteName }} · {{ upstreamGroupName }} · {{ ownGroupName }}</p>
              </div>
            </div>
            <button
              type="button"
              class="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
              @click="emit('close')"
            >
              <X class="h-4 w-4" />
            </button>
          </div>

          <div class="max-h-[60vh] overflow-y-auto px-5 py-4">
            <div v-if="!hasCandidates" class="flex flex-col items-center justify-center gap-2 py-10 text-center">
              <ShieldAlert class="h-8 w-8 text-muted-foreground/40" />
              <p class="text-sm text-muted-foreground">{{ t(`${prefix}.emptyTitle`) }}</p>
              <p class="text-xs text-muted-foreground">{{ t(`${prefix}.emptyHint`) }}</p>
            </div>
            <ul v-else class="space-y-2">
              <li
                v-for="candidate in candidates"
                :key="candidate.modelName"
                class="flex items-center gap-3 rounded-lg border border-border/40 px-3 py-2.5"
              >
                <input
                  type="checkbox"
                  class="h-4 w-4 shrink-0 rounded border-border/60"
                  :checked="selected.has(candidate.modelName)"
                  @change="toggle(candidate.modelName)"
                />
                <div class="min-w-0 flex-1">
                  <div class="flex flex-wrap items-center gap-2 text-sm text-foreground">
                    <span class="font-medium">{{ candidate.modelName }}</span>
                    <span class="inline-flex items-center rounded-full bg-surface-elevated px-2 py-0.5 text-xs text-muted-foreground">
                      {{ t(`admin.connectionHealth.providerLabels.${candidate.providerFamily}`) }}
                    </span>
                    <span
                      v-if="candidate.autoRemoteActionEnabled"
                      class="inline-flex items-center rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-600 dark:text-amber-400"
                    >
                      {{ t(`${prefix}.remoteActionOn`) }}
                    </span>
                  </div>
                  <p class="mt-0.5 text-xs text-muted-foreground">
                    {{ t(`${prefix}.fromPolicy`, { name: candidate.policyName }) }}
                    · {{ t(`${prefix}.maxTokens`, { count: candidate.maxProbeTokens }) }}
                  </p>
                </div>
              </li>
            </ul>
          </div>

          <div class="flex items-center justify-end gap-2 border-t border-border/60 px-5 py-4">
            <button type="button" class="rounded-lg px-3 py-1.5 text-sm text-muted-foreground hover:bg-surface-line" @click="emit('close')">
              {{ t(`${prefix}.cancel`) }}
            </button>
            <button
              type="button"
              class="rounded-lg bg-primary px-4 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
              :disabled="!canConfirm"
              @click="handleConfirm"
            >
              {{ t(`${prefix}.confirm`) }}
            </button>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
