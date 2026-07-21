<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  ArrowDownUp,
  Check,
  ChevronLeft,
  ChevronRight,
  Gauge,
  Loader2,
  Radar,
  ShieldCheck,
  SlidersHorizontal,
  X,
} from 'lucide-vue-next'
import { connectionHealthMessageKey, useConnectionHealth } from '../../composables/useConnectionHealth'
import type {
  AdminGroupAccount,
  AdminGroupHealth,
  ConnectionHealthPolicy,
  ConnectionHealthPriorityMode,
  PolicyInput,
} from '../../types/connectionHealth'

type SetupMode = 'stable' | 'multiplier' | 'monitor' | 'existing'
type Phase = 'loading' | 'ready' | 'saving' | 'error'
type ModelSuggestionSource = 'common' | 'discovered' | 'none'

const props = defineProps<{
  open: boolean
  group: AdminGroupHealth | null
  policies: ConnectionHealthPolicy[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'saved'): void
}>()

const { t, te } = useI18n()
const prefix = 'admin.connectionHealth.setup'
const {
  createPolicyForSetup,
  loadAdminGroupPolicyConfiguration,
  saveAdminGroupPolicyConfiguration,
  updatePolicyForSetup,
} = useConnectionHealth()

const phase = ref<Phase>('loading')
const step = ref(1)
const mode = ref<SetupMode>('multiplier')
const selectedTargetIds = ref<Set<string>>(new Set())
const selectedPolicyIds = ref<Set<string>>(new Set())
const modelText = ref('')
const providerFamily = ref('openai')
const autoRemoteActionEnabled = ref(true)
const errorKey = ref('')
const modelsTouched = ref(false)
const modelSuggestionSource = ref<ModelSuggestionSource>('none')
let loadSequence = 0

type PendingLegacyPolicy = {
  policyId: string
  fingerprint: string
}

const pendingLegacyPolicyStorageKey = 'transithub.connection-health.pending-legacy-policy.v1'

const readPendingLegacyPolicies = (): Record<string, PendingLegacyPolicy> => {
  try {
    const raw = window.localStorage.getItem(pendingLegacyPolicyStorageKey)
    if (!raw) return {}
    const parsed = JSON.parse(raw) as Record<string, PendingLegacyPolicy>
    return parsed && typeof parsed === 'object' ? parsed : {}
  } catch {
    return {}
  }
}

const writePendingLegacyPolicies = (pending: Record<string, PendingLegacyPolicy>) => {
  try {
    if (Object.keys(pending).length === 0) window.localStorage.removeItem(pendingLegacyPolicyStorageKey)
    else window.localStorage.setItem(pendingLegacyPolicyStorageKey, JSON.stringify(pending))
  } catch {
    // localStorage 不可用时仍可完成当前请求，只失去跨刷新重试能力。
  }
}

const legacyPolicyScope = (group: AdminGroupHealth): string => [
  group.platform,
  group.id,
  group.accounts[0]?.targetId ?? '',
].join('|')

const pendingLegacyPolicy = (group: AdminGroupHealth): PendingLegacyPolicy | undefined => {
  const pending = readPendingLegacyPolicies()[legacyPolicyScope(group)]
  if (!pending || typeof pending.policyId !== 'string' || typeof pending.fingerprint !== 'string') return undefined
  return pending
}

const rememberPendingLegacyPolicy = (group: AdminGroupHealth, pendingPolicy: PendingLegacyPolicy) => {
  const pending = readPendingLegacyPolicies()
  pending[legacyPolicyScope(group)] = pendingPolicy
  writePendingLegacyPolicies(pending)
}

const clearPendingLegacyPolicy = (group: AdminGroupHealth) => {
  const pending = readPendingLegacyPolicies()
  delete pending[legacyPolicyScope(group)]
  writePendingLegacyPolicies(pending)
}

const providerOptions = ['openai', 'anthropic', 'gemini', 'custom']

const dedupeModels = (value: string): string[] => {
  const seen = new Set<string>()
  const result: string[] = []
  for (const item of value.split(/[\n,]/)) {
    const model = item.trim()
    if (!model || seen.has(model)) continue
    seen.add(model)
    result.push(model)
  }
  return result
}

const accountModels = (account: AdminGroupAccount): string[] => dedupeModels([
  account.models ?? '',
  ...(account.modelHealth ?? []).map((model) => model.modelName),
].join(','))

// 分组向导只使用主列表已经返回的账号模型字段，不额外请求上游。优先取所有已选目标的模型
// 交集并限制为 3 个；无法确认完整交集时回退到少量已发现模型，避免默认创建过多探活目标。
const suggestedModels = (group: AdminGroupHealth, targetIds: Set<string>): { models: string[]; source: ModelSuggestionSource } => {
  const accounts = group.accounts.filter((account) => targetIds.has(account.targetId))
  const inventories = accounts.map(accountModels)
  const allHaveInventory = accounts.length > 0 && inventories.every((models) => models.length > 0)

  if (allHaveInventory) {
    const remaining = inventories.slice(1).map((models) => new Set(models))
    const common = inventories[0].filter((model) => remaining.every((inventory) => inventory.has(model)))
    if (common.length > 0) return { models: common.slice(0, 3), source: 'common' }
  }

  const discovered = dedupeModels(inventories.flat().join(','))
  if (discovered.length > 0) return { models: discovered.slice(0, 3), source: 'discovered' }
  return { models: [], source: 'none' }
}

const applyModelSuggestion = (group: AdminGroupHealth) => {
  const suggestion = suggestedModels(group, selectedTargetIds.value)
  modelText.value = suggestion.models.join('\n')
  modelSuggestionSource.value = suggestion.source
}

const reset = async () => {
  const group = props.group
  if (!group) return
  const sequence = ++loadSequence
  phase.value = 'loading'
  step.value = 1
  errorKey.value = ''
  selectedTargetIds.value = new Set(group.accounts.map((account) => account.targetId))
  selectedPolicyIds.value = new Set()
  modelsTouched.value = false
  applyModelSuggestion(group)
  const detectedProvider = group.accounts.find((account) => providerOptions.includes(account.platform))?.platform
  providerFamily.value = detectedProvider ?? (providerOptions.includes(group.platform) ? group.platform : 'openai')
  mode.value = 'multiplier'
  autoRemoteActionEnabled.value = true

  const outcome = await loadAdminGroupPolicyConfiguration(group.id)
  if (sequence !== loadSequence || !props.open || props.group?.id !== group.id) return
  if ('errorKey' in outcome) {
    errorKey.value = outcome.errorKey
    phase.value = 'error'
    return
  }
  const configuration = outcome.configuration
  selectedPolicyIds.value = new Set(configuration.policyIds)
  const excluded = new Set(configuration.excludedTargetIds)
  selectedTargetIds.value = new Set(group.accounts.filter((account) => !excluded.has(account.targetId)).map((account) => account.targetId))
  if (configuration.policyIds.length > 0) {
    mode.value = 'existing'
  }
  phase.value = 'ready'
}

watch(
  () => [props.open, props.group?.id],
  ([isOpen]) => {
    if (isOpen && props.group) void reset()
    else loadSequence++
  },
)

watch(mode, (nextMode) => {
  if (nextMode === 'monitor') autoRemoteActionEnabled.value = false
  if (nextMode === 'stable' || nextMode === 'multiplier') autoRemoteActionEnabled.value = true
})

const selectedCount = computed(() => selectedTargetIds.value.size)
const excludedCount = computed(() => Math.max(0, (props.group?.accounts.length ?? 0) - selectedCount.value))
const models = computed(() => dedupeModels(modelText.value))
const readableMessage = (rawKey: string): string => t(connectionHealthMessageKey(rawKey, te))
const effectivePriorityMode = computed<ConnectionHealthPriorityMode>(() => mode.value === 'multiplier' ? 'multiplier' : 'none')
const selectedExistingPolicies = computed(() => props.policies.filter((policy) => selectedPolicyIds.value.has(policy.id)))

const toggleTarget = (targetId: string) => {
  const next = new Set(selectedTargetIds.value)
  if (next.has(targetId)) next.delete(targetId)
  else next.add(targetId)
  selectedTargetIds.value = next
}

const togglePolicy = (policyId: string) => {
  const next = new Set(selectedPolicyIds.value)
  if (next.has(policyId)) next.delete(policyId)
  else next.add(policyId)
  selectedPolicyIds.value = next
}

const canContinue = computed(() => {
  if (phase.value !== 'ready') return false
  if (step.value === 1) return selectedCount.value > 0
  if (step.value === 2 && mode.value === 'existing') return selectedPolicyIds.value.size > 0
  if (step.value === 2) return models.value.length > 0
  return true
})

const next = () => {
  errorKey.value = ''
  if (!canContinue.value || step.value >= 3) return
  if (step.value === 1 && props.group && !modelsTouched.value) applyModelSuggestion(props.group)
  step.value++
}

const previous = () => {
  errorKey.value = ''
  if (step.value > 1) step.value--
}

const createQuickPolicyInput = (): PolicyInput => ({
  name: t(`${prefix}.generatedPolicyName`, { group: props.group?.name ?? '' }),
  enabled: true,
  ownGroupId: '',
  ownGroupName: '',
  probeIntervalSeconds: 60,
  failureThreshold: 3,
  successThreshold: 2,
  cooldownSeconds: 300,
  observationSeconds: 300,
  recoveryStepPercent: 25,
  dailyProbeBudget: 1000,
  autoDegradeEnabled: true,
  autoRemoteActionEnabled: autoRemoteActionEnabled.value,
  priorityMode: effectivePriorityMode.value,
  modelTargets: models.value.map((modelName) => ({
    modelName,
    providerFamily: providerFamily.value,
    enabled: true,
    probePrompt: '',
    maxProbeTokens: 1,
  })),
})

const bindLegacyQuickPolicy = async (
  group: AdminGroupHealth,
  quickPolicy: PolicyInput,
  excludedTargetIds: string[],
): Promise<string | null> => {
  const fingerprint = JSON.stringify(quickPolicy)
  const pending = pendingLegacyPolicy(group)
  let policyId = pending?.policyId ?? ''
  let reusedPendingPolicy = Boolean(pending)

  if (pending && pending.fingerprint !== fingerprint) {
    const updated = await updatePolicyForSetup(pending.policyId, quickPolicy)
    if ('errorKey' in updated) {
      if (updated.errorKey !== 'admin.connectionHealth.errors.notFound') return updated.errorKey
      clearPendingLegacyPolicy(group)
      policyId = ''
      reusedPendingPolicy = false
    } else {
      policyId = updated.policy.id
      rememberPendingLegacyPolicy(group, { policyId, fingerprint })
    }
  }

  if (!policyId) {
    const created = await createPolicyForSetup(quickPolicy)
    if ('errorKey' in created) return created.errorKey
    policyId = created.policy.id
    reusedPendingPolicy = false
    // 必须在绑定前保存：即使绑定请求失败或页面刷新，下次也会复用这一条策略。
    rememberPendingLegacyPolicy(group, { policyId, fingerprint })
  }

  let fallback = await saveAdminGroupPolicyConfiguration(group.id, {
    policyIds: [policyId],
    excludedTargetIds,
  })
  if ('errorKey' in fallback && fallback.errorKey === 'admin.connectionHealth.errors.policyNotFound' && reusedPendingPolicy) {
    // 缓存策略可能已被用户从其它页面删除。只在确认 policy 不存在时重建一次，
    // 网络或上游错误绝不触发新建，避免把瞬时失败放大为重复策略。
    clearPendingLegacyPolicy(group)
    const recreated = await createPolicyForSetup(quickPolicy)
    if ('errorKey' in recreated) return recreated.errorKey
    policyId = recreated.policy.id
    rememberPendingLegacyPolicy(group, { policyId, fingerprint })
    fallback = await saveAdminGroupPolicyConfiguration(group.id, {
      policyIds: [policyId],
      excludedTargetIds,
    })
  }
  if ('errorKey' in fallback) return fallback.errorKey
  clearPendingLegacyPolicy(group)
  return null
}

const save = async () => {
  const group = props.group
  if (!group || phase.value === 'saving') return
  phase.value = 'saving'
  errorKey.value = ''
  const policyIds = mode.value === 'existing' ? Array.from(selectedPolicyIds.value) : []

  const excludedTargetIds = group.accounts
    .filter((account) => !selectedTargetIds.value.has(account.targetId))
    .map((account) => account.targetId)
  const quickPolicy = mode.value === 'existing' ? null : createQuickPolicyInput()

  // 上一次旧后端兼容绑定若只完成了策略创建，直接继续绑定该策略；不再先尝试创建新的 quickPolicy。
  if (quickPolicy && pendingLegacyPolicy(group)) {
    const fallbackError = await bindLegacyQuickPolicy(group, quickPolicy, excludedTargetIds)
    if (fallbackError) {
      errorKey.value = fallbackError
      phase.value = 'ready'
      return
    }
    phase.value = 'ready'
    emit('saved')
    return
  }

  const outcome = await saveAdminGroupPolicyConfiguration(group.id, {
    policyIds,
    excludedTargetIds,
    ...(quickPolicy ? { quickPolicy } : {}),
  })
  if ('errorKey' in outcome) {
    // 旧后端的配置 DTO 不认识 quickPolicy，严格 JSON 解码会返回 request 错误。
    // 仅对这个兼容信号走旧的「先创建策略、再绑定分组」路径，真实业务/网络错误仍直接返回。
    if (quickPolicy && outcome.errorKey === 'admin.connectionHealth.errors.request') {
      const fallbackError = await bindLegacyQuickPolicy(group, quickPolicy, excludedTargetIds)
      if (fallbackError) {
        errorKey.value = fallbackError
        phase.value = 'ready'
        return
      }
      phase.value = 'ready'
      emit('saved')
      return
    }
    errorKey.value = outcome.errorKey
    phase.value = 'ready'
    return
  }
  clearPendingLegacyPolicy(group)
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
      <div v-if="open && group" class="fixed inset-0 z-[150]">
        <div class="absolute inset-0 bg-background/65 backdrop-blur-sm" @click="close" />
        <aside
          role="dialog"
          aria-modal="true"
          :aria-label="t(`${prefix}.title`)"
          class="absolute bottom-0 right-0 top-0 flex w-full max-w-2xl flex-col border-l border-border/60 bg-card shadow-2xl"
        >
          <header class="flex shrink-0 items-start justify-between gap-4 border-b border-border/60 px-6 py-5">
            <div class="min-w-0">
              <div class="flex items-center gap-2">
                <Radar class="h-4 w-4 text-primary" />
                <h2 class="truncate text-base font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h2>
              </div>
              <p class="mt-1 truncate text-sm text-muted-foreground">{{ group.name }} · {{ group.platform || '-' }}</p>
            </div>
            <button type="button" class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface hover:text-foreground" @click="close">
              <X class="h-4 w-4" />
            </button>
          </header>

          <div class="shrink-0 border-b border-border/50 px-6 py-4">
            <ol class="grid grid-cols-3 gap-2" :aria-label="t(`${prefix}.stepsLabel`)">
              <li v-for="index in 3" :key="index" class="flex items-center gap-2">
                <span
                  class="flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-semibold"
                  :class="step >= index ? 'bg-primary text-primary-foreground' : 'bg-surface text-muted-foreground'"
                >
                  <Check v-if="step > index" class="h-3.5 w-3.5" />
                  <span v-else>{{ index }}</span>
                </span>
                <span class="truncate text-xs" :class="step === index ? 'font-medium text-foreground' : 'text-muted-foreground'">
                  {{ t(`${prefix}.steps.${index}`) }}
                </span>
              </li>
            </ol>
          </div>

          <div class="flex-1 overflow-y-auto px-6 py-5">
            <div v-if="phase === 'loading'" class="flex min-h-80 items-center justify-center">
              <Loader2 class="h-6 w-6 animate-spin text-primary" />
            </div>

            <div v-else-if="phase === 'error'" class="flex min-h-80 flex-col items-center justify-center gap-4 text-center">
              <p class="max-w-md text-sm text-destructive">{{ readableMessage(errorKey) }}</p>
              <button
                type="button"
                class="inline-flex h-9 items-center rounded-lg border border-border bg-background px-4 text-sm font-medium text-foreground transition-colors hover:bg-surface"
                @click="reset"
              >
                {{ t(`${prefix}.retry`) }}
              </button>
            </div>

            <template v-else>
              <section v-if="step === 1" class="space-y-4">
                <div>
                  <h3 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.scope.title`) }}</h3>
                  <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ t(`${prefix}.scope.description`) }}</p>
                </div>
                <div class="divide-y divide-border/50 rounded-lg border border-border/60">
                  <label
                    v-for="account in group.accounts"
                    :key="account.targetId"
                    class="flex cursor-pointer items-start gap-3 px-4 py-3 transition-colors hover:bg-surface/60"
                  >
                    <input
                      type="checkbox"
                      class="mt-0.5 h-4 w-4 rounded border-border"
                      :checked="selectedTargetIds.has(account.targetId)"
                      @change="toggleTarget(account.targetId)"
                    >
                    <span class="min-w-0 flex-1">
                      <span class="flex flex-wrap items-center gap-2">
                        <span class="truncate text-sm font-medium text-foreground">{{ account.name || account.id }}</span>
                        <span class="text-xs text-muted-foreground">{{ account.status || '-' }}</span>
                      </span>
                      <span class="mt-1 block truncate text-xs text-muted-foreground">{{ account.models || t(`${prefix}.scope.modelsUnknown`) }}</span>
                    </span>
                    <span class="shrink-0 text-xs" :class="account.probeAvailable ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400'">
                      {{ t(`${prefix}.scope.${account.probeAvailable ? 'probeable' : 'pending'}`) }}
                    </span>
                  </label>
                </div>
                <p class="text-xs text-muted-foreground">{{ t(`${prefix}.scope.futureHint`) }}</p>
              </section>

              <section v-else-if="step === 2" class="space-y-5">
                <div>
                  <h3 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.strategy.title`) }}</h3>
                  <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ t(`${prefix}.strategy.description`) }}</p>
                </div>
                <div class="grid gap-2 sm:grid-cols-2">
                  <button
                    v-for="option in (['multiplier', 'stable', 'monitor'] as const)"
                    :key="option"
                    type="button"
                    class="flex min-h-24 items-start gap-3 rounded-lg border px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                    :class="mode === option ? 'border-primary bg-primary/[0.06]' : 'border-border/60 hover:bg-surface/60'"
                    @click="mode = option"
                  >
                    <ArrowDownUp v-if="option === 'multiplier'" class="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                    <ShieldCheck v-else-if="option === 'stable'" class="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                    <Gauge v-else class="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                    <span>
                      <span class="block text-sm font-medium text-foreground">{{ t(`${prefix}.strategy.options.${option}.title`) }}</span>
                      <span class="mt-1 block text-xs leading-5 text-muted-foreground">{{ t(`${prefix}.strategy.options.${option}.description`) }}</span>
                    </span>
                  </button>
                  <button
                    v-if="policies.length > 0"
                    type="button"
                    class="flex min-h-24 items-start gap-3 rounded-lg border px-4 py-3 text-left transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
                    :class="mode === 'existing' ? 'border-primary bg-primary/[0.06]' : 'border-border/60 hover:bg-surface/60'"
                    @click="mode = 'existing'"
                  >
                    <SlidersHorizontal class="mt-0.5 h-4 w-4 shrink-0 text-primary" />
                    <span>
                      <span class="block text-sm font-medium text-foreground">{{ t(`${prefix}.strategy.options.existing.title`) }}</span>
                      <span class="mt-1 block text-xs leading-5 text-muted-foreground">{{ t(`${prefix}.strategy.options.existing.description`) }}</span>
                    </span>
                  </button>
                </div>

                <div v-if="mode === 'existing'" class="space-y-2 rounded-lg border border-border/60 p-3">
                  <label v-for="policy in policies" :key="policy.id" class="flex cursor-pointer items-center gap-3 rounded-md px-2 py-2 hover:bg-surface/60">
                    <input type="checkbox" class="h-4 w-4 rounded border-border" :checked="selectedPolicyIds.has(policy.id)" @change="togglePolicy(policy.id)">
                    <span class="min-w-0 flex-1 truncate text-sm text-foreground">{{ policy.name }}</span>
                    <span class="text-xs text-muted-foreground">{{ t(`admin.connectionHealth.policies.${policy.enabled ? 'enabled' : 'disabled'}`) }}</span>
                  </label>
                </div>

                <template v-else>
                  <div class="space-y-2">
                    <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.strategy.modelsLabel`) }}</label>
                    <textarea
                      v-model="modelText"
                      rows="5"
                      :placeholder="t(`${prefix}.strategy.modelsPlaceholder`)"
                      class="w-full resize-y rounded-lg border border-border/60 bg-background px-3 py-2 text-sm text-foreground outline-none focus-visible:border-primary focus-visible:ring-2 focus-visible:ring-primary/20"
                      @input="modelsTouched = true"
                    />
                    <p class="text-xs text-muted-foreground">{{ t(`${prefix}.strategy.modelsDetected`, { count: models.length }) }}</p>
                    <p v-if="modelSuggestionSource !== 'none' && !modelsTouched" class="text-xs leading-5 text-primary">
                      {{ t(`${prefix}.strategy.modelSuggestions.${modelSuggestionSource}`, { count: models.length }) }}
                    </p>
                  </div>
                  <div class="grid gap-3 sm:grid-cols-2">
                    <label class="space-y-1.5">
                      <span class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.strategy.providerLabel`) }}</span>
                      <select v-model="providerFamily" class="h-9 w-full rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground">
                        <option v-for="provider in providerOptions" :key="provider" :value="provider">{{ t(`admin.connectionHealth.providerLabels.${provider}`) }}</option>
                      </select>
                    </label>
                    <label class="flex items-center justify-between gap-3 rounded-lg border border-border/60 px-3 py-2">
                      <span>
                        <span class="block text-xs font-medium text-foreground">{{ t(`${prefix}.strategy.remoteActionLabel`) }}</span>
                        <span class="mt-0.5 block text-xs text-muted-foreground">{{ t(`${prefix}.strategy.remoteActionHelp`) }}</span>
                      </span>
                      <input v-model="autoRemoteActionEnabled" type="checkbox" class="h-4 w-4 rounded border-border" :disabled="mode === 'monitor'">
                    </label>
                  </div>
                </template>
              </section>

              <section v-else class="space-y-5">
                <div>
                  <h3 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.confirm.title`) }}</h3>
                  <p class="mt-1 text-sm leading-6 text-muted-foreground">{{ t(`${prefix}.confirm.description`) }}</p>
                </div>
                <dl class="divide-y divide-border/50 rounded-lg border border-border/60">
                  <div class="flex items-center justify-between gap-4 px-4 py-3">
                    <dt class="text-sm text-muted-foreground">{{ t(`${prefix}.confirm.scope`) }}</dt>
                    <dd class="text-right text-sm font-medium text-foreground">{{ t(`${prefix}.confirm.scopeValue`, { selected: selectedCount, excluded: excludedCount }) }}</dd>
                  </div>
                  <div class="flex items-center justify-between gap-4 px-4 py-3">
                    <dt class="text-sm text-muted-foreground">{{ t(`${prefix}.confirm.strategy`) }}</dt>
                    <dd class="text-right text-sm font-medium text-foreground">
                      {{ mode === 'existing' ? selectedExistingPolicies.map((policy) => policy.name).join(', ') : t(`${prefix}.strategy.options.${mode}.title`) }}
                    </dd>
                  </div>
                  <div class="flex items-center justify-between gap-4 px-4 py-3">
                    <dt class="text-sm text-muted-foreground">{{ t(`${prefix}.confirm.models`) }}</dt>
                    <dd class="text-right text-sm font-medium text-foreground">{{ mode === 'existing' ? t(`${prefix}.confirm.fromPolicy`) : models.length }}</dd>
                  </div>
                  <div class="flex items-center justify-between gap-4 px-4 py-3">
                    <dt class="text-sm text-muted-foreground">{{ t(`${prefix}.confirm.remoteAction`) }}</dt>
                    <dd class="text-right text-sm font-medium" :class="autoRemoteActionEnabled && mode !== 'existing' ? 'text-amber-600 dark:text-amber-400' : 'text-foreground'">
                      {{ mode === 'existing' ? t(`${prefix}.confirm.fromPolicy`) : t(`${prefix}.confirm.${autoRemoteActionEnabled ? 'enabled' : 'disabled'}`) }}
                    </dd>
                  </div>
                </dl>
                <div v-if="mode === 'multiplier'" class="rounded-lg border border-primary/25 bg-primary/[0.05] px-4 py-3 text-xs leading-5 text-muted-foreground">
                  {{ t(`${prefix}.confirm.multiplierRule`) }}
                </div>
              </section>

              <p v-if="errorKey" class="mt-5 rounded-lg bg-destructive/10 px-3 py-2 text-sm text-destructive">{{ readableMessage(errorKey) }}</p>
            </template>
          </div>

          <footer class="flex shrink-0 items-center justify-between gap-3 border-t border-border/60 px-6 py-4">
            <button
              type="button"
              class="inline-flex h-9 items-center gap-1.5 rounded-lg px-3 text-sm font-medium text-muted-foreground transition-colors hover:bg-surface disabled:opacity-40"
              :disabled="step === 1 || phase !== 'ready'"
              @click="previous"
            >
              <ChevronLeft class="h-4 w-4" />
              {{ t(`${prefix}.back`) }}
            </button>
            <button
              v-if="step < 3"
              type="button"
              class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-40"
              :disabled="!canContinue"
              @click="next"
            >
              {{ t(`${prefix}.next`) }}
              <ChevronRight class="h-4 w-4" />
            </button>
            <button
              v-else
              type="button"
              class="inline-flex h-9 items-center gap-1.5 rounded-lg bg-primary px-4 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-40"
              :disabled="phase === 'saving'"
              @click="save"
            >
              <Loader2 v-if="phase === 'saving'" class="h-4 w-4 animate-spin" />
              <ShieldCheck v-else class="h-4 w-4" />
              {{ t(`${prefix}.save`) }}
            </button>
          </footer>
        </aside>
      </div>
    </Transition>
  </Teleport>
</template>
