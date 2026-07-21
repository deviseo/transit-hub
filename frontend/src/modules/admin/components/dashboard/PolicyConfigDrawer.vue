<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { ArrowDownUp, BookOpenText, X, ShieldCheck, Plus, Trash2 } from 'lucide-vue-next'
import { HelpTooltip } from '@/components/ui/tooltip'
import PolicyRunFlowDialog from './PolicyRunFlowDialog.vue'
import type { ConnectionHealthPolicy, ConnectionHealthPriorityMode, ModelTargetInput, PolicyInput } from '../../types/connectionHealth'

export interface OwnGroupOption {
  id: string
  name: string
}

const props = defineProps<{
  open: boolean
  policy: ConnectionHealthPolicy | null
  ownGroupOptions: OwnGroupOption[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', input: PolicyInput): void
}>()

const { t } = useI18n()
const prefix = 'admin.connectionHealth.policyDrawer'

const providerOptions = ['gemini', 'anthropic', 'openai', 'custom']

// 保守默认值：60s 探活间隔、1 个探活 token、每日预算有限、远端动作默认关闭需要用户显式打开。
const DEFAULTS = {
  probeIntervalSeconds: 60,
  failureThreshold: 3,
  successThreshold: 2,
  cooldownSeconds: 300,
  observationSeconds: 300,
  recoveryStepPercent: 25,
  dailyProbeBudget: 1000,
  maxProbeTokens: 1,
}

const name = ref('')
const enabled = ref(true)
const ownGroupId = ref('')
const probeIntervalSeconds = ref(DEFAULTS.probeIntervalSeconds)
const failureThreshold = ref(DEFAULTS.failureThreshold)
const successThreshold = ref(DEFAULTS.successThreshold)
const cooldownSeconds = ref(DEFAULTS.cooldownSeconds)
const observationSeconds = ref(DEFAULTS.observationSeconds)
const recoveryStepPercent = ref(DEFAULTS.recoveryStepPercent)
const dailyProbeBudget = ref(DEFAULTS.dailyProbeBudget)
const autoDegradeEnabled = ref(true)
const autoRemoteActionEnabled = ref(false)
const priorityMode = ref<ConnectionHealthPriorityMode>('none')
const modelTargets = ref<ModelTargetInput[]>([])
const validationError = ref<string | null>(null)
const runFlowOpen = ref(false)

// 单策略单 provider：策略级选择，下方所有模型目标共用同一个 provider。
// policyProvider 为空字符串是一个专门状态，只在"编辑一个历史遗留的混用 provider 策略"
// 时出现，用来强制要求用户显式选择一个 provider 之后才允许保存。
const policyProvider = ref<string>('openai')
const providerMismatch = ref(false)

const isEditing = computed(() => !!props.policy)

const resetForm = () => {
  const p = props.policy
  name.value = p?.name ?? ''
  enabled.value = p?.enabled ?? true
  ownGroupId.value = p?.ownGroupId ?? ''
  probeIntervalSeconds.value = p?.probeIntervalSeconds ?? DEFAULTS.probeIntervalSeconds
  failureThreshold.value = p?.failureThreshold ?? DEFAULTS.failureThreshold
  successThreshold.value = p?.successThreshold ?? DEFAULTS.successThreshold
  cooldownSeconds.value = p?.cooldownSeconds ?? DEFAULTS.cooldownSeconds
  observationSeconds.value = p?.observationSeconds ?? DEFAULTS.observationSeconds
  recoveryStepPercent.value = p?.recoveryStepPercent ?? DEFAULTS.recoveryStepPercent
  dailyProbeBudget.value = p?.dailyProbeBudget ?? DEFAULTS.dailyProbeBudget
  autoDegradeEnabled.value = p?.autoDegradeEnabled ?? true
  autoRemoteActionEnabled.value = autoDegradeEnabled.value && (p?.autoRemoteActionEnabled ?? false)
  priorityMode.value = p?.priorityMode === 'multiplier' ? 'multiplier' : 'none'

  // 已有模型目标全部同一个 provider 时直接复用该 provider 初始化——必须从"唯一值"取，
  // 不能从 modelTargets[0] 取，否则历史数据里第一条 provider 恰好为空、后面几条其实
  // 是同一个非 openai provider 时，会被误判成默认 openai 并在保存时把旧数据静默改错；
  // 没有任何有效 provider（新建策略）时才使用默认值 openai；混用多个 provider 时不静默
  // 选一个、也不丢数据——留空强制用户显式选择，同时打开 providerMismatch 警告，
  // 保存前的校验会因为 provider 为空而拦下。
  const existingProviders = Array.from(new Set((p?.modelTargets ?? []).map(m => m.providerFamily).filter(Boolean)))
  if (existingProviders.length > 1) {
    providerMismatch.value = true
    policyProvider.value = ''
  } else {
    providerMismatch.value = false
    policyProvider.value = existingProviders[0] ?? 'openai'
  }

  modelTargets.value = p?.modelTargets?.length
    ? p.modelTargets.map(m => ({
        id: m.id,
        modelName: m.modelName,
        providerFamily: m.providerFamily,
        enabled: m.enabled,
        probePrompt: m.probePrompt,
        maxProbeTokens: m.maxProbeTokens,
      }))
    : [{ modelName: '', providerFamily: policyProvider.value, enabled: true, probePrompt: '', maxProbeTokens: DEFAULTS.maxProbeTokens }]
  validationError.value = null
}

watch(() => props.open, (isOpen) => { if (isOpen) resetForm() })

// 切换策略级 provider 时同步更新表单内所有模型目标的 providerFamily，保持实时一致；
// 混用警告状态下 policyProvider 初始为空，此时不覆盖已有目标，等用户真正选择后再统一。
watch(policyProvider, (val) => {
  if (!val) return
  modelTargets.value.forEach((m) => { m.providerFamily = val })
})

watch(autoDegradeEnabled, (enabled) => {
  if (!enabled) autoRemoteActionEnabled.value = false
})

const addModelTarget = () => {
  modelTargets.value.push({ modelName: '', providerFamily: policyProvider.value, enabled: true, probePrompt: '', maxProbeTokens: DEFAULTS.maxProbeTokens })
}

const removeModelTarget = (index: number) => {
  modelTargets.value.splice(index, 1)
}

const handleSave = () => {
  validationError.value = null
  if (!name.value.trim()) {
    validationError.value = t(`${prefix}.errors.nameRequired`)
    return
  }
  if (!policyProvider.value) {
    validationError.value = t(`${prefix}.errors.providerRequired`)
    return
  }
  const targets = modelTargets.value
    .filter(m => m.modelName.trim() !== '')
    // 保存出去的目标 provider 必须和策略级 provider 一致：不管表单内部状态如何，
    // 落盘前统一按 policyProvider 重新盖章，从根上避免出现混用 provider 的脏数据。
    .map(m => ({ ...m, providerFamily: policyProvider.value }))
  if (targets.length === 0) {
    validationError.value = t(`${prefix}.errors.modelTargetRequired`)
    return
  }

  const ownGroup = props.ownGroupOptions.find(g => g.id === ownGroupId.value)
  const input: PolicyInput = {
    id: props.policy?.id,
    name: name.value.trim(),
    enabled: enabled.value,
    ownGroupId: ownGroupId.value,
    ownGroupName: ownGroup?.name ?? '',
    probeIntervalSeconds: probeIntervalSeconds.value,
    failureThreshold: failureThreshold.value,
    successThreshold: successThreshold.value,
    cooldownSeconds: cooldownSeconds.value,
    observationSeconds: observationSeconds.value,
    recoveryStepPercent: recoveryStepPercent.value,
    dailyProbeBudget: dailyProbeBudget.value,
    autoDegradeEnabled: autoDegradeEnabled.value,
    autoRemoteActionEnabled: autoRemoteActionEnabled.value,
    priorityMode: priorityMode.value,
    modelTargets: targets,
  }
  emit('save', input)
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
            :aria-label="isEditing ? t(`${prefix}.editTitle`) : t(`${prefix}.createTitle`)"
            class="absolute bottom-0 right-0 top-0 w-full max-w-lg overflow-y-auto overscroll-contain border-l border-border/60 bg-card shadow-2xl"
          >
            <div class="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <div class="flex items-center gap-2.5">
                <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <ShieldCheck class="h-4 w-4" />
                </div>
                <h3 class="text-sm font-semibold text-foreground">
                  {{ isEditing ? t(`${prefix}.editTitle`) : t(`${prefix}.createTitle`) }}
                </h3>
              </div>
              <div class="flex shrink-0 items-center gap-1">
                <button
                  type="button"
                  class="inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                  @click="runFlowOpen = true"
                >
                  <BookOpenText class="h-3.5 w-3.5" />
                  {{ t(`${prefix}.runFlow.buttonLabel`) }}
                </button>
                <button
                  type="button"
                  class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                  @click="emit('close')"
                >
                  <X class="h-4 w-4" />
                </button>
              </div>
            </div>

            <div class="space-y-5 px-5 py-5">
              <div v-if="validationError" class="rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-sm text-red-600 dark:text-red-400">
                {{ validationError }}
              </div>

              <!-- 基础信息 -->
              <div class="space-y-3">
                <div class="space-y-1.5">
                  <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.nameLabel`) }}</label>
                  <input
                    v-model="name"
                    type="text"
                    :placeholder="t(`${prefix}.namePlaceholder`)"
                    class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground"
                  />
                </div>

                <div class="flex items-center justify-between rounded-lg border border-border/40 bg-surface/30 px-4 py-3">
                  <div class="text-sm text-foreground">{{ t(`${prefix}.enabledLabel`) }}</div>
                  <label class="relative inline-flex cursor-pointer items-center">
                    <input v-model="enabled" type="checkbox" class="peer sr-only" />
                    <div class="w-9 h-5 bg-surface-elevated rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-primary"></div>
                  </label>
                </div>

                <div class="space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.ownGroupLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.ownGroup`)" />
                  </label>
                  <select v-model="ownGroupId" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground">
                    <option value="">{{ t(`${prefix}.ownGroupAllOption`) }}</option>
                    <option v-for="g in ownGroupOptions" :key="g.id" :value="g.id">{{ g.name }}</option>
                  </select>
                </div>
              </div>

              <!-- 模型探活目标 -->
              <div class="space-y-2 border-t border-border/40 pt-4">
                <div class="flex items-center justify-between">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.modelTargetsLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.modelTargets`)" />
                  </label>
                  <button type="button" class="inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline" @click="addModelTarget">
                    <Plus class="h-3 w-3" />{{ t(`${prefix}.addModelTarget`) }}
                  </button>
                </div>

                <!-- 策略级 provider：一个策略只能选一个 provider，下方所有模型目标都跟随这个选择。 -->
                <div class="space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.providerLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.provider`)" />
                  </label>
                  <select v-model="policyProvider" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground">
                    <option v-if="!policyProvider" value="" disabled>{{ t(`${prefix}.providerPlaceholder`) }}</option>
                    <option v-for="p in providerOptions" :key="p" :value="p">{{ t(`admin.connectionHealth.providerLabels.${p}`) }}</option>
                  </select>
                  <p v-if="providerMismatch" class="rounded-lg border border-amber-500/30 bg-amber-500/5 p-2.5 text-xs text-amber-700 dark:text-amber-400">
                    {{ t(`${prefix}.providerMismatchWarning`) }}
                  </p>
                </div>

                <div v-for="(target, index) in modelTargets" :key="index" class="rounded-lg border border-border/40 p-3 space-y-2">
                  <div class="flex items-center gap-2">
                    <input
                      v-model="target.modelName"
                      type="text"
                      :placeholder="t(`${prefix}.modelNamePlaceholder`)"
                      class="h-8 flex-1 rounded-md border border-border/60 bg-background px-2 text-xs text-foreground"
                    />
                    <button type="button" class="rounded-md p-1.5 text-muted-foreground hover:bg-surface-line hover:text-red-500" @click="removeModelTarget(index)">
                      <Trash2 class="h-3.5 w-3.5" />
                    </button>
                  </div>
                  <div class="flex items-center gap-3">
                    <label class="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
                      <input v-model="target.enabled" type="checkbox" class="h-3.5 w-3.5 rounded border-border/60" />
                      {{ t(`${prefix}.modelEnabledLabel`) }}
                    </label>
                    <label class="inline-flex items-center gap-1.5 text-xs text-muted-foreground">
                      {{ t(`${prefix}.maxProbeTokensLabel`) }}
                      <input v-model.number="target.maxProbeTokens" type="number" min="1" class="h-7 w-16 rounded-md border border-border/60 bg-background px-1.5 text-xs text-foreground" />
                    </label>
                  </div>
                  <input
                    v-model="target.probePrompt"
                    type="text"
                    :placeholder="t(`${prefix}.probePromptPlaceholder`)"
                    class="h-8 w-full rounded-md border border-border/60 bg-background px-2 text-xs text-foreground"
                  />
                </div>
              </div>

              <!-- 阈值配置 -->
              <div class="grid grid-cols-2 gap-3 border-t border-border/40 pt-4">
                <div class="space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.probeIntervalLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.probeInterval`)" />
                  </label>
                  <input v-model.number="probeIntervalSeconds" type="number" min="1" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground" />
                </div>
                <div class="space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.dailyBudgetLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.dailyBudget`)" />
                  </label>
                  <input v-model.number="dailyProbeBudget" type="number" min="1" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground" />
                </div>
                <div class="space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.failureThresholdLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.failureThreshold`)" />
                  </label>
                  <input v-model.number="failureThreshold" type="number" min="1" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground" />
                </div>
                <div class="space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.successThresholdLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.successThreshold`)" />
                  </label>
                  <input v-model.number="successThreshold" type="number" min="1" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground" />
                </div>
                <div class="space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.cooldownLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.cooldown`)" />
                  </label>
                  <input v-model.number="cooldownSeconds" type="number" min="1" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground" />
                </div>
                <div class="space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.observationLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.observation`)" />
                  </label>
                  <input v-model.number="observationSeconds" type="number" min="1" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground" />
                </div>
                <div class="col-span-2 space-y-1.5">
                  <label class="flex items-center gap-1 text-xs font-medium text-muted-foreground">
                    {{ t(`${prefix}.recoveryStepLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.recoveryStep`)" />
                  </label>
                  <input v-model.number="recoveryStepPercent" type="number" min="1" max="100" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground" />
                </div>
              </div>

              <!-- 自动化开关：默认保守，远端动作必须让用户明确可见并主动打开 -->
              <div class="space-y-3 border-t border-border/40 pt-4">
                <div class="space-y-2 rounded-lg border border-border/40 bg-surface/30 px-4 py-3">
                  <div class="flex items-center gap-1 text-sm text-foreground">
                    <ArrowDownUp class="h-4 w-4 text-primary" />
                    {{ t(`${prefix}.priorityModeLabel`) }}
                    <HelpTooltip :text="t(`${prefix}.tooltips.priorityMode`)" />
                  </div>
                  <div class="grid grid-cols-2 gap-1 rounded-lg bg-surface p-1" role="radiogroup" :aria-label="t(`${prefix}.priorityModeLabel`)">
                    <button
                      v-for="mode in (['none', 'multiplier'] as const)"
                      :key="mode"
                      type="button"
                      role="radio"
                      :aria-checked="priorityMode === mode"
                      class="rounded-md px-3 py-2 text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                      :class="priorityMode === mode ? 'bg-background text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground'"
                      @click="priorityMode = mode"
                    >
                      {{ t(`${prefix}.priorityModes.${mode}`) }}
                    </button>
                  </div>
                  <p class="text-xs leading-5 text-muted-foreground">{{ t(`${prefix}.priorityModeHelp`) }}</p>
                </div>
                <div class="flex items-center justify-between rounded-lg border border-border/40 bg-surface/30 px-4 py-3">
                  <div>
                    <div class="flex items-center gap-1 text-sm text-foreground">
                      {{ t(`${prefix}.autoDegradeLabel`) }}
                      <HelpTooltip :text="t(`${prefix}.tooltips.autoDegrade`)" />
                    </div>
                    <div class="text-xs text-muted-foreground">{{ t(`${prefix}.autoDegradeHelp`) }}</div>
                  </div>
                  <label class="relative inline-flex cursor-pointer items-center shrink-0">
                    <input v-model="autoDegradeEnabled" type="checkbox" class="peer sr-only" />
                    <div class="w-9 h-5 bg-surface-elevated rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-primary"></div>
                  </label>
                </div>
                <div class="flex items-center justify-between rounded-lg border border-amber-500/30 bg-amber-500/5 px-4 py-3">
                  <div>
                    <div class="flex items-center gap-1 text-sm text-foreground">
                      {{ t(`${prefix}.autoRemoteActionLabel`) }}
                      <HelpTooltip :text="t(`${prefix}.tooltips.autoRemoteAction`)" />
                    </div>
                    <div class="text-xs text-amber-700 dark:text-amber-400">{{ t(`${prefix}.autoRemoteActionHelp`) }}</div>
                  </div>
                  <label class="relative inline-flex cursor-pointer items-center shrink-0">
                    <input v-model="autoRemoteActionEnabled" type="checkbox" class="peer sr-only" :disabled="!autoDegradeEnabled" />
                    <div class="w-9 h-5 bg-surface-elevated rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white peer-disabled:cursor-not-allowed peer-disabled:opacity-50 after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-primary"></div>
                  </label>
                </div>
              </div>

              <div class="flex items-center justify-end gap-2 border-t border-border/40 pt-4">
                <button type="button" class="rounded-lg px-3 py-1.5 text-sm text-muted-foreground hover:bg-surface-line" @click="emit('close')">
                  {{ t(`${prefix}.cancel`) }}
                </button>
                <button type="button" class="rounded-lg bg-primary px-4 py-1.5 text-sm font-medium text-primary-foreground hover:bg-primary/90" @click="handleSave">
                  {{ t(`${prefix}.save`) }}
                </button>
              </div>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>

  <PolicyRunFlowDialog :open="runFlowOpen" @close="runFlowOpen = false" />
</template>
