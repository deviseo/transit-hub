<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertTriangle, CheckCircle2, Loader2, ShieldAlert, X, XCircle, Zap } from 'lucide-vue-next'
import {
  connectionHealthMessageKey,
  connectionHealthRecordColorClass,
  formatConnectionHealthTime,
  useConnectionHealth,
} from '../../composables/useConnectionHealth'
import type { ManualProbeModelOption, ManualProbeResult } from '../../types/connectionHealth'

export interface ManualProbeTargetSummary {
  targetId: string
  accountName: string
  platform: string
  type: string
  status: string
  groupName: string
}

const props = defineProps<{
  open: boolean
  target: ManualProbeTargetSummary | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t, te } = useI18n()
const prefix = 'admin.connectionHealth.manualProbeDialog'
const { discoverModels, runManualProbeOnce } = useConnectionHealth()

type Phase = 'loading' | 'ready' | 'testing' | 'error'

const phase = ref<Phase>('loading')
const models = ref<ManualProbeModelOption[]>([])
const selected = ref<Set<string>>(new Set())
const results = ref<ManualProbeResult[]>([])
const loadErrorKey = ref('')
const testErrorKey = ref('')
let loadSequence = 0

const defaultSelection = (options: ManualProbeModelOption[]): Set<string> =>
  new Set(options.length > 0 ? [options[0].id] : [])

const readableMessage = (rawKey: string): string => t(connectionHealthMessageKey(rawKey, te))

// 弹窗每次打开都是全新的一次性会话：重置全部状态并立即拉模型列表，不复用上一次打开的结果。
watch(
  () => [props.open, props.target?.targetId],
  async ([isOpen]) => {
    if (!isOpen || !props.target) {
      loadSequence++
      return
    }
    const targetId = props.target.targetId
    const sequence = ++loadSequence
    phase.value = 'loading'
    models.value = []
    selected.value = new Set()
    results.value = []
    loadErrorKey.value = ''
    testErrorKey.value = ''

    const outcome = await discoverModels(targetId)
    if (sequence !== loadSequence || !props.open || props.target?.targetId !== targetId) return
    if ('errorKey' in outcome) {
      loadErrorKey.value = outcome.errorKey
      phase.value = 'error'
      return
    }
    models.value = outcome.models
    // 手动一次性探活默认只选择首个模型，避免首次操作误触发大批量探活；用户仍可自行多选。
    selected.value = defaultSelection(outcome.models)
    phase.value = 'ready'
  },
)

const hasModels = computed(() => models.value.length > 0)
const canStartTest = computed(() => hasModels.value && selected.value.size > 0 && phase.value !== 'testing')

const toggle = (modelId: string) => {
  if (phase.value === 'testing') return
  const next = new Set(selected.value)
  if (next.has(modelId)) {
    next.delete(modelId)
  } else {
    next.add(modelId)
  }
  selected.value = next
}

const retryLoad = async () => {
  if (!props.target) return
  const targetId = props.target.targetId
  const sequence = ++loadSequence
  phase.value = 'loading'
  loadErrorKey.value = ''
  const outcome = await discoverModels(targetId)
  if (sequence !== loadSequence || !props.open || props.target?.targetId !== targetId) return
  if ('errorKey' in outcome) {
    loadErrorKey.value = outcome.errorKey
    phase.value = 'error'
    return
  }
  models.value = outcome.models
  selected.value = defaultSelection(outcome.models)
  phase.value = 'ready'
}

const startTest = async () => {
  if (!canStartTest.value || !props.target) return
  phase.value = 'testing'
  testErrorKey.value = ''
  const outcome = await runManualProbeOnce(props.target.targetId, Array.from(selected.value))
  if ('errorKey' in outcome) {
    testErrorKey.value = outcome.errorKey
    phase.value = 'ready'
    return
  }
  results.value = outcome.results
  phase.value = 'ready'
}

const resultLabel = (result: string): string => readableMessage(result)

const close = () => {
  if (phase.value === 'testing') return
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
      <div v-if="open && target" class="fixed inset-0 z-[150] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="close" />

      <div role="dialog" aria-modal="true" :aria-label="t(`${prefix}.title`)" class="relative flex h-[min(760px,calc(100dvh-2rem))] w-full max-w-6xl flex-col overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl">
          <!-- 头部：账号/channel 摘要 -->
          <div class="flex shrink-0 items-center justify-between gap-3 border-b border-border/60 px-5 py-4">
            <div class="flex min-w-0 items-center gap-2.5">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Zap class="h-4 w-4" />
              </div>
              <div class="min-w-0">
                <h3 class="truncate text-sm font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h3>
                <p class="truncate text-xs text-muted-foreground">
                  {{ target.accountName }} · {{ target.platform || '-' }} · {{ target.type || '-' }} · {{ target.status || '-' }} · {{ target.groupName }}
                </p>
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

          <!-- 内容区：模型选择 + 结果展示，内部独立滚动 -->
          <div class="flex-1 overflow-y-auto px-5 py-4">
            <div v-if="phase === 'loading'" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
              <Loader2 class="h-6 w-6 animate-spin text-primary/60" />
              <p class="text-sm text-muted-foreground">{{ t(`${prefix}.loadingModels`) }}</p>
            </div>

            <div v-else-if="phase === 'error'" class="flex flex-col items-center justify-center gap-3 py-16 text-center">
              <ShieldAlert class="h-8 w-8 text-red-500/70" />
              <p class="text-sm text-red-600 dark:text-red-400">{{ readableMessage(loadErrorKey) }}</p>
              <button
                type="button"
                class="rounded-lg border border-border/60 px-3 py-1.5 text-xs font-medium text-foreground transition-colors hover:bg-surface-line"
                @click="retryLoad"
              >
                {{ t(`${prefix}.retryLoad`) }}
              </button>
            </div>

            <template v-else>
              <div v-if="!hasModels" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
                <ShieldAlert class="h-8 w-8 text-muted-foreground/40" />
                <p class="text-sm text-muted-foreground">{{ t(`${prefix}.empty`) }}</p>
              </div>

              <template v-else>
                <p class="mb-3 text-xs text-muted-foreground">{{ t(`${prefix}.selectHint`) }}</p>
                <div class="grid grid-cols-1 gap-2.5 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
                  <label
                    v-for="model in models"
                    :key="model.id"
                    class="flex cursor-pointer items-start gap-2 rounded-lg border border-border/40 px-3 py-2.5 transition-colors"
                    :class="selected.has(model.id) ? 'border-primary/50 bg-primary/5' : 'hover:bg-surface-line/40'"
                  >
                    <input
                      type="checkbox"
                      class="mt-0.5 h-4 w-4 shrink-0 rounded border-border/60"
                      :disabled="phase === 'testing'"
                      :checked="selected.has(model.id)"
                      @change="toggle(model.id)"
                    />
                    <div class="min-w-0 flex-1">
                      <p class="truncate text-sm font-medium text-foreground">{{ model.name }}</p>
                      <p v-if="model.ownedBy" class="truncate text-xs text-muted-foreground">{{ model.ownedBy }}</p>
                    </div>
                  </label>
                </div>

                <p v-if="testErrorKey" class="mt-4 rounded-lg bg-red-500/10 px-3 py-2 text-xs text-red-600 dark:text-red-400">
                  {{ readableMessage(testErrorKey) }}
                </p>

                <div class="mt-5">
                  <h4 class="mb-2 text-xs font-semibold text-foreground">{{ t(`${prefix}.resultTitle`) }}</h4>
                  <div v-if="results.length === 0" class="rounded-lg border border-dashed border-border/50 px-3 py-6 text-center text-xs text-muted-foreground">
                    {{ t(`${prefix}.resultEmpty`) }}
                  </div>
                  <ul v-else class="space-y-2">
                    <li
                      v-for="result in results"
                      :key="result.modelName"
                      class="flex flex-wrap items-center justify-between gap-2 rounded-lg border border-border/40 px-3 py-2.5"
                    >
                      <div class="flex min-w-0 items-center gap-2">
                        <CheckCircle2 v-if="result.healthy" class="h-4 w-4 shrink-0 text-green-500" />
                        <XCircle v-else class="h-4 w-4 shrink-0 text-red-500" />
                        <span class="truncate text-sm font-medium text-foreground">{{ result.modelName }}</span>
                        <span class="inline-flex shrink-0 items-center gap-1 rounded-full bg-surface-elevated px-2 py-0.5 text-xs text-muted-foreground">
                          <span class="h-1.5 w-1.5 rounded-full" :class="connectionHealthRecordColorClass(result.result)" />
                          {{ resultLabel(result.result) }}
                        </span>
                      </div>
                      <div class="flex shrink-0 items-center gap-3 text-xs text-muted-foreground">
                        <span v-if="result.latencyMs !== null">{{ t(`${prefix}.latency`, { ms: result.latencyMs }) }}</span>
                        <span>{{ formatConnectionHealthTime(result.probedAt) }}</span>
                      </div>
                      <p v-if="!result.healthy && result.errorDetail" class="w-full truncate text-xs text-red-500/80">
                        {{ result.errorDetail }}
                      </p>
                    </li>
                  </ul>
                </div>
              </template>
            </template>
          </div>

          <!-- 底部操作栏 -->
          <div class="flex shrink-0 items-center justify-between gap-3 border-t border-border/60 px-5 py-4">
            <p v-if="hasModels" class="flex items-center gap-1 text-xs text-muted-foreground">
              <AlertTriangle v-if="selected.size === 0" class="h-3.5 w-3.5" />
              {{ t(`${prefix}.selectedCount`, { count: selected.size }) }}
            </p>
            <div v-else />
            <div class="flex items-center gap-2">
              <button type="button" class="rounded-lg px-3 py-1.5 text-sm text-muted-foreground hover:bg-surface-line" @click="close">
                {{ t(`${prefix}.close`) }}
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-1.5 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:cursor-not-allowed disabled:opacity-50"
                :disabled="!canStartTest"
                @click="startTest"
              >
                <Loader2 v-if="phase === 'testing'" class="h-4 w-4 animate-spin" />
                {{ phase === 'testing' ? t(`${prefix}.testing`) : t(`${prefix}.startTest`) }}
              </button>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
