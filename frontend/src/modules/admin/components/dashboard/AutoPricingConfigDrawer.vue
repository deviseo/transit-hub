<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { X, Zap, ZapOff, Calculator, CircleHelp, Bell, BellOff, Copy, Check, Loader2 } from 'lucide-vue-next'
import { Tooltip } from '@/components/ui/tooltip'
import type { MySiteMapping, MySiteGroupRef, AutoPricingSource, AutoPricingStrategy } from '../../types/mySites'

export interface BotOption {
  id: string
  name: string
  channel: string
}

const props = defineProps<{
  open: boolean
  mapping: MySiteMapping | null
  upstreamMultipliers: Map<string, number>
  upstreamLabels?: Map<string, string>
  availableBots: BotOption[]
  saving?: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'save', config: Partial<MySiteMapping>): void
}>()

const { t } = useI18n()

const enableAutoPricing = ref(false)
const autoPricingSource = ref<AutoPricingSource>('primary_upstream')
const primaryUpstreamSiteId = ref('')
const primaryUpstreamGroupName = ref('')
const autoPricingStrategy = ref<AutoPricingStrategy>('percentage')
const fixedIncrease = ref(0.1)
const percentageIncrease = ref(10)
const adjustThresholdPercent = ref(10)
const minMultiplier = ref<number | null>(null)
const maxMultiplier = ref<number | null>(null)
const enableAutoPricingNotify = ref(false)
const autoPricingNotifyBotIds = ref<string[]>([])
const autoPricingNotifyTemplate = ref('')
const copiedVar = ref<string | null>(null)
const validationError = ref<string | null>(null)

const prefix = 'admin.groupAssociations.autoPricingDrawer'

const upstreamTargets = computed<MySiteGroupRef[]>(() => props.mapping?.upstreamTargets ?? [])

const hasUpstreams = computed(() => upstreamTargets.value.length > 0)

const sourceOptions: { value: AutoPricingSource; labelKey: string }[] = [
  { value: 'primary_upstream', labelKey: `${prefix}.sourcePrimaryUpstream` },
  { value: 'lowest_upstream', labelKey: `${prefix}.sourceLowestUpstream` },
  { value: 'highest_upstream', labelKey: `${prefix}.sourceHighestUpstream` },
  { value: 'average_upstream', labelKey: `${prefix}.sourceAverageUpstream` },
]

const templateVars = [
  { key: '{ownGroup}', labelKey: `${prefix}.notify.varOwnGroup` },
  { key: '{upstreamSiteName}', labelKey: `${prefix}.notify.varUpstreamSiteName` },
  { key: '{upstreamGroupName}', labelKey: `${prefix}.notify.varUpstreamGroupName` },
  { key: '{oldReference}', labelKey: `${prefix}.notify.varOldReference` },
  { key: '{newReference}', labelKey: `${prefix}.notify.varNewReference` },
  { key: '{oldOwnMultiplier}', labelKey: `${prefix}.notify.varOldOwnMultiplier` },
  { key: '{newOwnMultiplier}', labelKey: `${prefix}.notify.varNewOwnMultiplier` },
  { key: '{strategy}', labelKey: `${prefix}.notify.varStrategy` },
  { key: '{fixedIncrease}', labelKey: `${prefix}.notify.varFixedIncrease` },
  { key: '{percentageIncrease}', labelKey: `${prefix}.notify.varPercentageIncrease` },
  { key: '{threshold}', labelKey: `${prefix}.notify.varThreshold` },
]

const copyVar = async (varKey: string) => {
  try {
    await navigator.clipboard.writeText(varKey)
    copiedVar.value = varKey
    setTimeout(() => { copiedVar.value = null }, 1500)
  } catch {}
}

const toggleBot = (botId: string) => {
  const idx = autoPricingNotifyBotIds.value.indexOf(botId)
  if (idx >= 0) {
    autoPricingNotifyBotIds.value.splice(idx, 1)
  } else {
    autoPricingNotifyBotIds.value.push(botId)
  }
}

const primaryUpstreamKey = computed({
  get: () => primaryUpstreamSiteId.value && primaryUpstreamGroupName.value
    ? `${primaryUpstreamSiteId.value}::${primaryUpstreamGroupName.value}`
    : '',
  set: (val: string) => {
    if (!val) {
      primaryUpstreamSiteId.value = ''
      primaryUpstreamGroupName.value = ''
      return
    }
    const idx = val.indexOf('::')
    if (idx >= 0) {
      primaryUpstreamSiteId.value = val.slice(0, idx)
      primaryUpstreamGroupName.value = val.slice(idx + 2)
    }
  }
})

const getUpstreamMultiplier = (siteId: string, groupName: string): number | null => {
  return props.upstreamMultipliers.get(`${siteId}::${groupName}`) ?? null
}

const getUpstreamLabel = (siteId: string): string => props.upstreamLabels?.get(siteId) ?? siteId

const referenceMultiplier = computed<number | null>(() => {
  const targets = upstreamTargets.value
  if (targets.length === 0) return null

  if (autoPricingSource.value === 'primary_upstream') {
    if (!primaryUpstreamSiteId.value || !primaryUpstreamGroupName.value) return null
    return getUpstreamMultiplier(primaryUpstreamSiteId.value, primaryUpstreamGroupName.value)
  }

  const multipliers = targets
    .map(t => getUpstreamMultiplier(t.siteId, t.groupName))
    .filter((m): m is number => m != null)
  if (multipliers.length === 0) return null

  switch (autoPricingSource.value) {
    case 'lowest_upstream':
      return Math.min(...multipliers)
    case 'highest_upstream':
      return Math.max(...multipliers)
    case 'average_upstream':
      return multipliers.reduce((a, b) => a + b, 0) / multipliers.length
    default:
      return null
  }
})

const estimatedMultiplier = computed(() => {
  if (!enableAutoPricing.value) return null
  const ref = referenceMultiplier.value
  if (ref == null) return null
  let next: number
  if (autoPricingStrategy.value === 'fixed') {
    next = ref + fixedIncrease.value
  } else {
    next = ref * (1 + percentageIncrease.value / 100)
  }
  if (minMultiplier.value != null && next < minMultiplier.value) next = minMultiplier.value
  if (maxMultiplier.value != null && next > maxMultiplier.value) next = maxMultiplier.value
  return next
})

const resetForm = () => {
  const m = props.mapping
  enableAutoPricing.value = m?.enableAutoPricing ?? false
  autoPricingSource.value = m?.autoPricingSource ?? 'primary_upstream'
  primaryUpstreamSiteId.value = m?.primaryUpstreamSiteId ?? ''
  primaryUpstreamGroupName.value = m?.primaryUpstreamGroupName ?? ''
  autoPricingStrategy.value = m?.autoPricingStrategy ?? 'percentage'
  fixedIncrease.value = m?.fixedIncrease ?? 0.1
  percentageIncrease.value = m?.percentageIncrease ?? 10
  adjustThresholdPercent.value = m?.adjustThresholdPercent ?? 10
  minMultiplier.value = m?.minMultiplier ?? null
  maxMultiplier.value = m?.maxMultiplier ?? null
  enableAutoPricingNotify.value = m?.enableAutoPricingNotify ?? false
  autoPricingNotifyBotIds.value = [...(m?.autoPricingNotifyBotIds ?? [])]
  autoPricingNotifyTemplate.value = m?.autoPricingNotifyTemplate ?? ''
  copiedVar.value = null
  validationError.value = null
}

watch(() => props.open, (isOpen) => {
  if (isOpen) resetForm()
})

const validate = (): boolean => {
  validationError.value = null

  if (enableAutoPricing.value && autoPricingSource.value === 'primary_upstream') {
    if (!primaryUpstreamSiteId.value || !primaryUpstreamGroupName.value) {
      validationError.value = t(`${prefix}.errors.primaryRequired`)
      return false
    }
  }

  if (fixedIncrease.value < 0 || percentageIncrease.value < 0) {
    validationError.value = t(`${prefix}.errors.increaseNonNegative`)
    return false
  }

  if (adjustThresholdPercent.value < 0) {
    validationError.value = t(`${prefix}.errors.thresholdNonNegative`)
    return false
  }

  if (minMultiplier.value != null && minMultiplier.value < 0) {
    validationError.value = t(`${prefix}.errors.multiplierNonNegative`)
    return false
  }
  if (maxMultiplier.value != null && maxMultiplier.value < 0) {
    validationError.value = t(`${prefix}.errors.multiplierNonNegative`)
    return false
  }

  if (minMultiplier.value != null && maxMultiplier.value != null && minMultiplier.value > maxMultiplier.value) {
    validationError.value = t(`${prefix}.errors.minGreaterThanMax`)
    return false
  }

  if (enableAutoPricingNotify.value && autoPricingNotifyBotIds.value.length === 0) {
    validationError.value = t(`${prefix}.errors.notifyBotsRequired`)
    return false
  }

  return true
}

const handleSave = () => {
  if (!validate()) return
  emit('save', {
    enableAutoPricing: enableAutoPricing.value,
    autoPricingSource: autoPricingSource.value,
    primaryUpstreamSiteId: primaryUpstreamSiteId.value || undefined,
    primaryUpstreamGroupName: primaryUpstreamGroupName.value || undefined,
    autoPricingStrategy: autoPricingStrategy.value,
    fixedIncrease: fixedIncrease.value,
    percentageIncrease: percentageIncrease.value,
    adjustThresholdPercent: adjustThresholdPercent.value,
    minMultiplier: minMultiplier.value,
    maxMultiplier: maxMultiplier.value,
    enableAutoPricingNotify: enableAutoPricingNotify.value,
    autoPricingNotifyBotIds: autoPricingNotifyBotIds.value,
    autoPricingNotifyTemplate: autoPricingNotifyTemplate.value,
  })
}

const parseNumberInput = (value: string): number | null => {
  if (value === '' || value === '-') return null
  const num = Number.parseFloat(value)
  return Number.isFinite(num) ? num : null
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
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="saving ? undefined : emit('close')" />

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
            :aria-label="mapping?.ownGroup ? t(`${prefix}.titleWithGroup`, { group: mapping.ownGroup }) : t(`${prefix}.title`)"
            class="absolute bottom-0 right-0 top-0 w-full max-w-md overflow-y-auto overscroll-contain border-l border-border/60 bg-card shadow-2xl"
          >
            <!-- Header -->
            <div class="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <div class="flex items-center gap-2.5">
                <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <Calculator class="h-4 w-4" />
                </div>
                <h3 class="text-sm font-semibold text-foreground">
                  {{ mapping?.ownGroup ? t(`${prefix}.titleWithGroup`, { group: mapping.ownGroup }) : t(`${prefix}.title`) }}
                </h3>
              </div>
              <button
                type="button"
                class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground disabled:opacity-50"
                :disabled="saving"
                @click="emit('close')"
              >
                <X class="h-4 w-4" />
              </button>
            </div>

            <!-- Body -->
            <div class="space-y-5 px-5 py-5">
              <!-- No upstreams warning -->
              <div v-if="!hasUpstreams" class="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-sm text-amber-700 dark:text-amber-400">
                {{ t(`${prefix}.noUpstreams`) }}
              </div>

              <template v-else>
                <!-- Enable toggle -->
                <div class="flex items-center justify-between rounded-lg border border-border/40 bg-surface/30 px-4 py-3">
                  <div class="flex items-center gap-2">
                    <Zap v-if="enableAutoPricing" class="h-4 w-4 text-primary" />
                    <ZapOff v-else class="h-4 w-4 text-muted-foreground" />
                    <span class="text-sm font-medium text-foreground">{{ t(`${prefix}.enableLabel`) }}</span>
                  </div>
                  <label class="relative inline-flex items-center cursor-pointer">
                    <input type="checkbox" v-model="enableAutoPricing" class="sr-only peer">
                    <div class="w-9 h-5 bg-surface-elevated rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-primary"></div>
                  </label>
                </div>

                <template v-if="enableAutoPricing">
                  <!-- Pricing Source -->
                  <div class="space-y-1.5">
                    <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.sourceLabel`) }}</label>
                    <select
                      v-model="autoPricingSource"
                      class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                    >
                      <option v-for="opt in sourceOptions" :key="opt.value" :value="opt.value">
                        {{ t(opt.labelKey) }}
                      </option>
                    </select>
                  </div>

                  <!-- Primary Upstream selector -->
                  <div v-if="autoPricingSource === 'primary_upstream'" class="space-y-1.5">
                    <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.primaryUpstreamLabel`) }}</label>
                    <select
                      v-model="primaryUpstreamKey"
                      class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                    >
                      <option value="" disabled>{{ t(`${prefix}.primaryUpstreamPlaceholder`) }}</option>
                      <option
                        v-for="target in upstreamTargets"
                        :key="`${target.siteId}::${target.groupName}`"
                        :value="`${target.siteId}::${target.groupName}`"
                      >
                        {{ target.groupName }} · {{ getUpstreamLabel(target.siteId) }}{{ getUpstreamMultiplier(target.siteId, target.groupName) != null ? ` · ${Number(getUpstreamMultiplier(target.siteId, target.groupName)!.toFixed(4))}×` : '' }}
                      </option>
                    </select>
                  </div>

                  <!-- Strategy -->
                  <div class="space-y-1.5">
                    <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.strategyLabel`) }}</label>
                    <div class="flex gap-2">
                      <button
                        type="button"
                        class="flex-1 rounded-lg border px-3 py-2 text-sm font-medium transition-colors"
                        :class="autoPricingStrategy === 'fixed' ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 text-muted-foreground hover:bg-surface/50'"
                        @click="autoPricingStrategy = 'fixed'"
                      >
                        {{ t(`${prefix}.strategyFixed`) }}
                      </button>
                      <button
                        type="button"
                        class="flex-1 rounded-lg border px-3 py-2 text-sm font-medium transition-colors"
                        :class="autoPricingStrategy === 'percentage' ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 text-muted-foreground hover:bg-surface/50'"
                        @click="autoPricingStrategy = 'percentage'"
                      >
                        {{ t(`${prefix}.strategyPercentage`) }}
                      </button>
                    </div>
                  </div>

                  <!-- Fixed / Percentage value -->
                  <div class="space-y-1.5">
                    <label class="text-xs font-medium text-muted-foreground">
                      {{ autoPricingStrategy === 'fixed' ? t(`${prefix}.fixedIncreaseLabel`) : t(`${prefix}.percentageIncreaseLabel`) }}
                    </label>
                    <div class="relative">
                      <input
                        type="number"
                        :value="autoPricingStrategy === 'fixed' ? fixedIncrease : percentageIncrease"
                        @input="autoPricingStrategy === 'fixed' ? (fixedIncrease = Number.parseFloat(($event.target as HTMLInputElement).value) || 0) : (percentageIncrease = Number.parseFloat(($event.target as HTMLInputElement).value) || 0)"
                        :step="autoPricingStrategy === 'fixed' ? '0.01' : '1'"
                        min="0"
                        class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 pr-8 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                      />
                      <span class="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">
                        {{ autoPricingStrategy === 'fixed' ? '×' : '%' }}
                      </span>
                    </div>
                  </div>

                  <!-- Threshold -->
                  <div class="space-y-1.5">
                    <div class="inline-flex items-center gap-1.5">
                      <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.thresholdLabel`) }}</label>
                      <Tooltip :text="t(`${prefix}.tips.threshold`)" wide>
                        <button
                          type="button"
                          class="inline-flex h-4 w-4 items-center justify-center rounded-full text-muted-foreground/70 transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
                          :aria-label="t(`${prefix}.tips.thresholdAria`)"
                        >
                          <CircleHelp class="h-3.5 w-3.5" />
                        </button>
                      </Tooltip>
                    </div>
                    <div class="relative">
                      <input
                        type="number"
                        v-model.number="adjustThresholdPercent"
                        step="1"
                        min="0"
                        class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 pr-8 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                      />
                      <span class="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-muted-foreground">%</span>
                    </div>
                    <p class="text-xs text-muted-foreground/80">{{ t(`${prefix}.thresholdHelp`) }}</p>
                  </div>

                  <!-- Min / Max multiplier -->
                  <div class="grid grid-cols-2 gap-3">
                    <div class="space-y-1.5">
                      <div class="inline-flex items-center gap-1.5">
                        <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.minMultiplierLabel`) }}</label>
                        <Tooltip :text="t(`${prefix}.tips.minMultiplier`)" wide>
                          <button
                            type="button"
                            class="inline-flex h-4 w-4 items-center justify-center rounded-full text-muted-foreground/70 transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
                            :aria-label="t(`${prefix}.tips.minMultiplierAria`)"
                          >
                            <CircleHelp class="h-3.5 w-3.5" />
                          </button>
                        </Tooltip>
                      </div>
                      <input
                        type="number"
                        :value="minMultiplier ?? ''"
                        @input="minMultiplier = parseNumberInput(($event.target as HTMLInputElement).value)"
                        step="0.01"
                        min="0"
                        class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                      />
                    </div>
                    <div class="space-y-1.5">
                      <div class="inline-flex items-center gap-1.5">
                        <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.maxMultiplierLabel`) }}</label>
                        <Tooltip :text="t(`${prefix}.tips.maxMultiplier`)" wide>
                          <button
                            type="button"
                            class="inline-flex h-4 w-4 items-center justify-center rounded-full text-muted-foreground/70 transition-colors hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40"
                            :aria-label="t(`${prefix}.tips.maxMultiplierAria`)"
                          >
                            <CircleHelp class="h-3.5 w-3.5" />
                          </button>
                        </Tooltip>
                      </div>
                      <input
                        type="number"
                        :value="maxMultiplier ?? ''"
                        @input="maxMultiplier = parseNumberInput(($event.target as HTMLInputElement).value)"
                        step="0.01"
                        min="0"
                        class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                      />
                    </div>
                  </div>

                  <!-- Guidance: recommended defaults & example -->
                  <div class="rounded-lg border border-border/40 bg-surface/30 px-4 py-3 text-xs text-muted-foreground">
                    <p class="font-medium text-foreground">{{ t(`${prefix}.guidance.title`) }}</p>
                    <ul class="mt-2 space-y-1 list-disc list-inside">
                      <li>{{ t(`${prefix}.guidance.minMultiplier`) }}</li>
                      <li>{{ t(`${prefix}.guidance.maxMultiplier`) }}</li>
                      <li>{{ t(`${prefix}.guidance.threshold`) }}</li>
                    </ul>
                    <div class="mt-3 border-t border-border/40 pt-3">
                      <p class="font-medium text-foreground">{{ t(`${prefix}.guidance.exampleTitle`) }}</p>
                      <ul class="mt-2 space-y-1 list-disc list-inside">
                        <li>{{ t(`${prefix}.guidance.exampleOld`) }}</li>
                        <li>{{ t(`${prefix}.guidance.exampleNew`) }}</li>
                        <li>{{ t(`${prefix}.guidance.exampleThreshold`) }}</li>
                        <li>{{ t(`${prefix}.guidance.exampleMarkup`) }}</li>
                        <li>{{ t(`${prefix}.guidance.exampleMin`) }}</li>
                        <li>{{ t(`${prefix}.guidance.exampleMax`) }}</li>
                      </ul>
                      <p class="mt-2 text-muted-foreground/80">{{ t(`${prefix}.guidance.exampleResult`) }}</p>
                    </div>
                  </div>

                  <!-- Estimated multiplier preview -->
                  <div v-if="referenceMultiplier != null && estimatedMultiplier != null" class="rounded-lg border border-primary/20 bg-primary/5 px-4 py-3">
                    <div class="flex items-center justify-between">
                      <span class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.estimatedMultiplier`) }}</span>
                      <span class="text-sm font-semibold text-primary">
                        {{ Number(estimatedMultiplier.toFixed(4)).toString() }}×
                      </span>
                    </div>
                    <p class="mt-1 text-[10px] text-muted-foreground/70">
                      {{ autoPricingStrategy === 'fixed'
                        ? `${Number(referenceMultiplier.toFixed(4))} + ${fixedIncrease}`
                        : `${Number(referenceMultiplier.toFixed(4))} × (1 + ${percentageIncrease}%)`
                      }}
                    </p>
                  </div>
                  <div v-else-if="referenceMultiplier == null && enableAutoPricing" class="rounded-lg border border-amber-500/20 bg-amber-500/5 px-4 py-3">
                    <span class="text-xs text-amber-600 dark:text-amber-400">{{ t(`${prefix}.noMultiplierData`) }}</span>
                  </div>
                  <!-- Auto-pricing success notification -->
                  <div class="mt-2 border-t border-border/40 pt-5 space-y-4">
                    <div class="flex items-center justify-between rounded-lg border border-border/40 bg-surface/30 px-4 py-3">
                      <div class="flex items-center gap-2">
                        <Bell v-if="enableAutoPricingNotify" class="h-4 w-4 text-primary" />
                        <BellOff v-else class="h-4 w-4 text-muted-foreground" />
                        <div>
                          <span class="text-sm font-medium text-foreground">{{ t(`${prefix}.notify.sectionTitle`) }}</span>
                          <p class="text-xs text-muted-foreground/80">{{ t(`${prefix}.notify.enableHelp`) }}</p>
                        </div>
                      </div>
                      <label class="relative inline-flex items-center cursor-pointer">
                        <input type="checkbox" v-model="enableAutoPricingNotify" class="sr-only peer">
                        <div class="w-9 h-5 bg-surface-elevated rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-primary"></div>
                      </label>
                    </div>

                    <template v-if="enableAutoPricingNotify">
                      <!-- Bot selector -->
                      <div class="space-y-1.5">
                        <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.notify.botSelectLabel`) }}</label>
                        <div v-if="availableBots.length === 0" class="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-xs text-amber-700 dark:text-amber-400">
                          {{ t(`${prefix}.notify.noBots`) }}
                        </div>
                        <div v-else class="space-y-1.5 max-h-40 overflow-y-auto rounded-lg border border-border/40 p-2">
                          <label
                            v-for="bot in availableBots"
                            :key="bot.id"
                            class="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm cursor-pointer transition-colors hover:bg-surface/50"
                            :class="autoPricingNotifyBotIds.includes(bot.id) ? 'bg-primary/5' : ''"
                          >
                            <input
                              type="checkbox"
                              :checked="autoPricingNotifyBotIds.includes(bot.id)"
                              @change="toggleBot(bot.id)"
                              class="h-3.5 w-3.5 rounded border-border text-primary focus:ring-primary/40"
                            />
                            <span class="flex-1 text-foreground">{{ bot.name }}</span>
                            <span class="rounded-full bg-surface-elevated px-1.5 py-0.5 text-[10px] text-muted-foreground">{{ bot.channel }}</span>
                          </label>
                        </div>
                      </div>

                      <!-- Template -->
                      <div class="space-y-1.5">
                        <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.notify.templateLabel`) }}</label>
                        <textarea
                          v-model="autoPricingNotifyTemplate"
                          :placeholder="t(`${prefix}.notify.defaultTemplate`)"
                          rows="3"
                          class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-primary resize-y"
                        />
                        <p class="text-xs text-muted-foreground/80">{{ t(`${prefix}.notify.templateHelp`) }}</p>
                      </div>

                      <!-- Variable chips -->
                      <div class="space-y-1.5">
                        <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.notify.variablesTitle`) }}</label>
                        <div class="flex flex-wrap gap-1.5">
                          <button
                            v-for="v in templateVars"
                            :key="v.key"
                            type="button"
                            class="inline-flex items-center gap-1 rounded-md border border-border/40 bg-surface/30 px-2 py-1 text-xs font-mono text-foreground transition-colors hover:bg-primary/10 hover:border-primary/30"
                            :title="t(v.labelKey)"
                            @click="copyVar(v.key)"
                          >
                            <Copy v-if="copiedVar !== v.key" class="h-3 w-3 text-muted-foreground" />
                            <Check v-else class="h-3 w-3 text-emerald-500" />
                            {{ v.key }}
                          </button>
                        </div>
                      </div>
                    </template>
                  </div>
                </template>

                <!-- Validation error -->
                <div v-if="validationError" class="rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-sm text-red-600 dark:text-red-400">
                  {{ validationError }}
                </div>
              </template>
            </div>

            <!-- Footer -->
            <div v-if="hasUpstreams" class="sticky bottom-0 flex items-center justify-end gap-2 border-t border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <button
                type="button"
                class="rounded-lg border border-border/50 px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground disabled:opacity-50"
                :disabled="saving"
                @click="emit('close')"
              >
                {{ t(`${prefix}.cancel`) }}
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-2 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
                :disabled="saving"
                @click="handleSave"
              >
                <Loader2 v-if="saving" class="h-4 w-4 animate-spin" />
                <Check v-else class="h-4 w-4" />
                {{ saving ? t('admin.groupAssociations.saving') : t(`${prefix}.save`) }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
