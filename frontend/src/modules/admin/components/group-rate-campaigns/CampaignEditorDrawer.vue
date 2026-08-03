<script setup lang="ts">
import { ref, watch } from 'vue'
import { X, Megaphone, CircleHelp, Bell, BellOff, Copy, Check, Loader2, Eye } from 'lucide-vue-next'
import { Tooltip } from '@/components/ui/tooltip'
import { Button } from '@/components/ui/button'
import { getMySiteMappingOptions } from '../../api/mySites'
import { getNotificationChannelSettings } from '../../api/settings'
import { createGroupRateCampaign, previewGroupRateCampaign } from '../../api/groupRateCampaigns'
import type {
  CampaignDetail,
  CampaignEndMode,
  CampaignNotifyDefaults,
  CampaignPreviewItem,
  CampaignStartMode,
  CreateGroupRateCampaignRequest,
} from '../../types/groupRateCampaigns'
import type { MySiteMappingOwnGroupOption } from '../../types/mySites'

interface BotOption {
  id: string
  name: string
  channel: string
}

const props = defineProps<{
  open: boolean
  notifyDefaults: CampaignNotifyDefaults | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'created', campaign: CampaignDetail): void
}>()

import { t } from '@/locales'
const prefix = 'admin.groupRateCampaigns.editor'

const name = ref('')
const description = ref('')

// selectedGroups 是活动调价新口径的核心状态：只支持手动选择分组，
// 每个已选分组都要单独填写自己的固定活动倍率，不再有全局调价范围/调价方式。
const selectedGroups = ref<{ groupName: string; campaignMultiplier: number | null }[]>([])

const startMode = ref<CampaignStartMode>('now')
const startAt = ref('')
const endMode = ref<CampaignEndMode>('manual')
const endAt = ref('')

const notifyEnabled = ref(false)
const notifyBotIds = ref<string[]>([])
const notifyStartTemplate = ref('')
const notifyEndTemplate = ref('')
const copiedVar = ref<string | null>(null)
const copyError = ref(false)

const availableGroups = ref<MySiteMappingOwnGroupOption[]>([])
const availableBots = ref<BotOption[]>([])
const isLoadingOptions = ref(false)

const isPreviewing = ref(false)
const previewItems = ref<CampaignPreviewItem[]>([])
const previewTotal = ref<number | null>(null)
const previewErrorKey = ref<string | null>(null)

const isSubmitting = ref(false)
const validationError = ref<string | null>(null)

const templateVars = [
  { key: '{activityName}', labelKey: `${prefix}.notifyVarActivityName` },
  { key: '{totalCount}', labelKey: `${prefix}.notifyVarTotalCount` },
  { key: '{appliedCount}', labelKey: `${prefix}.notifyVarAppliedCount` },
  { key: '{failedCount}', labelKey: `${prefix}.notifyVarFailedCount` },
  { key: '{startTime}', labelKey: `${prefix}.notifyVarStartTime` },
  { key: '{endTime}', labelKey: `${prefix}.notifyVarEndTime` },
]

const copyVar = async (varKey: string) => {
  copyError.value = false
  try {
    await navigator.clipboard.writeText(varKey)
    copiedVar.value = varKey
    setTimeout(() => { copiedVar.value = null }, 1500)
  } catch (error) {
    copyError.value = true
    console.warn('Failed to copy notification template variable', error)
    setTimeout(() => { copyError.value = false }, 1500)
  }
}

const isGroupSelected = (groupName: string): boolean => (
  selectedGroups.value.some((g) => g.groupName === groupName)
)

const groupMultiplierInput = (groupName: string): number | null => {
  const found = selectedGroups.value.find((g) => g.groupName === groupName)
  return found ? found.campaignMultiplier : null
}

const toggleGroupSelection = (groupName: string) => {
  const idx = selectedGroups.value.findIndex((g) => g.groupName === groupName)
  if (idx >= 0) {
    selectedGroups.value.splice(idx, 1)
  } else {
    selectedGroups.value.push({ groupName, campaignMultiplier: null })
  }
}

const setGroupMultiplier = (groupName: string, rawValue: string) => {
  const found = selectedGroups.value.find((g) => g.groupName === groupName)
  if (!found) return
  const trimmed = rawValue.trim()
  if (trimmed === '') {
    found.campaignMultiplier = null
    return
  }
  const parsed = Number(trimmed)
  found.campaignMultiplier = Number.isFinite(parsed) ? parsed : null
}

const toggleBot = (botId: string) => {
  const idx = notifyBotIds.value.indexOf(botId)
  if (idx >= 0) {
    notifyBotIds.value.splice(idx, 1)
  } else {
    notifyBotIds.value.push(botId)
  }
}

const resetForm = () => {
  name.value = ''
  description.value = ''
  selectedGroups.value = []
  startMode.value = 'now'
  startAt.value = ''
  endMode.value = 'manual'
  endAt.value = ''
  notifyEnabled.value = props.notifyDefaults?.enabled ?? false
  notifyBotIds.value = [...(props.notifyDefaults?.botIds ?? [])]
  notifyStartTemplate.value = props.notifyDefaults?.startTemplate ?? ''
  notifyEndTemplate.value = props.notifyDefaults?.endTemplate ?? ''
  copiedVar.value = null
  copyError.value = false
  previewItems.value = []
  previewTotal.value = null
  previewErrorKey.value = null
  validationError.value = null
}

const loadOptions = async () => {
  isLoadingOptions.value = true
  try {
    const [mappingOptions, channelSettings] = await Promise.all([
      getMySiteMappingOptions().catch(() => ({ ownGroups: [], mappings: [] })),
      getNotificationChannelSettings().catch(() => ({ dingtalk: [], wecom: [], qq: [], feishu: [], telegram: [] })),
    ])
    availableGroups.value = mappingOptions.ownGroups

    const bots: BotOption[] = []
    for (const bot of channelSettings.dingtalk ?? []) {
      if (bot.enabled) bots.push({ id: bot.id, name: bot.name, channel: 'DingTalk' })
    }
    for (const bot of channelSettings.wecom ?? []) {
      if (bot.enabled) bots.push({ id: bot.id, name: bot.name, channel: 'WeCom' })
    }
    for (const bot of channelSettings.qq ?? []) {
      if (bot.enabled) bots.push({ id: bot.id, name: bot.name, channel: 'QQ' })
    }
    for (const bot of channelSettings.feishu ?? []) {
      if (bot.enabled) bots.push({ id: bot.id, name: bot.name, channel: 'Feishu' })
    }
    for (const bot of channelSettings.telegram ?? []) {
      if (bot.enabled) bots.push({ id: bot.id, name: bot.name, channel: 'Telegram' })
    }
    availableBots.value = bots
  } finally {
    isLoadingOptions.value = false
  }
}

watch(() => props.open, (isOpen) => {
  if (isOpen) {
    resetForm()
    void loadOptions()
  }
})

const buildRequest = (): CreateGroupRateCampaignRequest => ({
  name: name.value.trim(),
  description: description.value.trim(),
  selection: {
    mode: 'manual',
    types: [],
    groups: selectedGroups.value.map((g) => ({
      groupName: g.groupName,
      campaignMultiplier: g.campaignMultiplier,
    })),
    filter: { search: '', type: '', platform: '' },
  },
  // 顶层 adjustment 只为兼容后端历史结构固定发送，实际活动倍率完全由 selection.groups 逐分组决定。
  adjustment: {
    mode: 'set',
    value: 0,
  },
  schedule: {
    startMode: startMode.value,
    startAt: startMode.value === 'scheduled' && startAt.value ? new Date(startAt.value).toISOString() : null,
    endMode: endMode.value,
    endAt: endMode.value === 'scheduled' && endAt.value ? new Date(endAt.value).toISOString() : null,
  },
  notify: {
    enabled: notifyEnabled.value,
    botIds: notifyBotIds.value,
    startTemplate: notifyStartTemplate.value,
    endTemplate: notifyEndTemplate.value,
  },
})

const validate = (): boolean => {
  validationError.value = null

  if (!name.value.trim()) {
    validationError.value = t(`${prefix}.errors.nameRequired`)
    return false
  }

  if (selectedGroups.value.length === 0) {
    validationError.value = t(`${prefix}.errors.selectionEmpty`)
    return false
  }
  for (const g of selectedGroups.value) {
    if (g.campaignMultiplier === null || !Number.isFinite(g.campaignMultiplier) || g.campaignMultiplier < 0) {
      validationError.value = t(`${prefix}.errors.groupMultiplierInvalid`)
      return false
    }
  }

  if (startMode.value === 'scheduled' && !startAt.value) {
    validationError.value = t(`${prefix}.errors.scheduleInvalid`)
    return false
  }
  if (endMode.value === 'scheduled' && !endAt.value) {
    validationError.value = t(`${prefix}.errors.scheduleInvalid`)
    return false
  }

  if (notifyEnabled.value && notifyBotIds.value.length === 0) {
    validationError.value = t(`${prefix}.errors.notifyBotsRequired`)
    return false
  }

  return true
}

const runPreview = async () => {
  if (!validate()) return
  isPreviewing.value = true
  previewErrorKey.value = null
  try {
    const response = await previewGroupRateCampaign(buildRequest())
    previewItems.value = response.items
    previewTotal.value = response.total
  } catch (error) {
    previewErrorKey.value = error instanceof Error ? error.message : 'admin.groupRateCampaigns.errors.unknown'
  } finally {
    isPreviewing.value = false
  }
}

const handleSubmit = async () => {
  if (!validate()) return
  isSubmitting.value = true
  validationError.value = null
  try {
    const detail = await createGroupRateCampaign(buildRequest())
    emit('created', detail)
  } catch (error) {
    validationError.value = error instanceof Error ? t(error.message) : t('admin.groupRateCampaigns.errors.unknown')
  } finally {
    isSubmitting.value = false
  }
}

const formatMultiplier = (value: number): string => `${Number(value.toFixed(4)).toString()}×`
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
            :aria-label="t(`${prefix}.titleCreate`)"
            class="absolute bottom-0 right-0 top-0 w-full max-w-lg overflow-y-auto overscroll-contain border-l border-border/60 bg-card shadow-2xl"
          >
            <div class="sticky top-0 z-10 flex items-center justify-between gap-3 border-b border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <div class="flex items-center gap-2.5">
                <div class="flex h-8 w-8 items-center justify-center rounded-lg bg-primary/10 text-primary">
                  <Megaphone class="h-4 w-4" />
                </div>
                <h3 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.titleCreate`) }}</h3>
              </div>
              <button
                type="button"
                class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                @click="emit('close')"
              >
                <X class="h-4 w-4" />
              </button>
            </div>

            <div class="space-y-5 px-5 py-5">
              <!-- 活动信息 -->
              <div class="space-y-3">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionInfo`) }}</p>
                <div class="space-y-1.5">
                  <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.nameLabel`) }}</label>
                  <input
                    v-model="name"
                    type="text"
                    :placeholder="t(`${prefix}.namePlaceholder`)"
                    class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
                <div class="space-y-1.5">
                  <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.descriptionLabel`) }}</label>
                  <textarea
                    v-model="description"
                    :placeholder="t(`${prefix}.descriptionPlaceholder`)"
                    rows="2"
                    class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground/50 focus:outline-none focus:ring-1 focus:ring-primary resize-y"
                  />
                </div>
              </div>

              <!-- 选择分组：手动选择 + 每个分组单独固定活动倍率 -->
              <div class="space-y-3 border-t border-border/40 pt-5">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionSelection`) }}</p>
                <p class="text-xs text-muted-foreground">{{ t(`${prefix}.selectionHint`) }}</p>

                <div v-if="availableGroups.length === 0" class="rounded-lg border border-border/40 bg-surface/30 p-3 text-xs text-muted-foreground">
                  {{ t(`${prefix}.groupsEmpty`) }}
                </div>
                <div v-else class="space-y-1.5 max-h-72 overflow-y-auto rounded-lg border border-border/40 p-2">
                  <div
                    v-for="group in availableGroups"
                    :key="group.groupName"
                    class="flex flex-wrap items-center gap-2.5 rounded-md px-2.5 py-2 text-sm transition-colors"
                    :class="isGroupSelected(group.groupName) ? 'bg-primary/5' : 'hover:bg-surface/50'"
                  >
                    <label class="flex flex-1 min-w-0 items-center gap-2.5 cursor-pointer">
                      <input
                        type="checkbox"
                        :checked="isGroupSelected(group.groupName)"
                        class="h-3.5 w-3.5 shrink-0 rounded border-border text-primary focus:ring-primary/40"
                        @change="toggleGroupSelection(group.groupName)"
                      />
                      <span class="flex-1 min-w-0 truncate text-foreground">{{ group.groupName }}</span>
                    </label>
                    <span class="shrink-0 text-xs text-muted-foreground">
                      {{ t('admin.groupRateCampaigns.detail.itemOriginal') }}: {{ formatMultiplier(group.multiplier) }}
                    </span>
                    <input
                      v-if="isGroupSelected(group.groupName)"
                      :value="groupMultiplierInput(group.groupName)"
                      type="number"
                      step="0.01"
                      :placeholder="t(`${prefix}.groupMultiplierPlaceholder`)"
                      class="w-28 shrink-0 rounded-md border border-border/50 bg-background px-2 py-1 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                      @input="setGroupMultiplier(group.groupName, ($event.target as HTMLInputElement).value)"
                    />
                  </div>
                </div>
              </div>

              <!-- 时间计划 -->
              <div class="space-y-3 border-t border-border/40 pt-5">
                <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.sectionSchedule`) }}</p>
                <div class="space-y-1.5">
                  <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.startModeLabel`) }}</label>
                  <div class="flex gap-2">
                    <button
                      v-for="mode in (['now', 'scheduled', 'draft'] as const)"
                      :key="mode"
                      type="button"
                      class="flex-1 rounded-lg border px-3 py-2 text-sm font-medium transition-colors"
                      :class="startMode === mode ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 text-muted-foreground hover:bg-surface/50'"
                      @click="startMode = mode"
                    >
                      {{ t(`${prefix}.start${mode.charAt(0).toUpperCase()}${mode.slice(1)}`) }}
                    </button>
                  </div>
                  <input
                    v-if="startMode === 'scheduled'"
                    v-model="startAt"
                    type="datetime-local"
                    class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
                <div class="space-y-1.5">
                  <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.endModeLabel`) }}</label>
                  <div class="flex gap-2">
                    <button
                      v-for="mode in (['manual', 'scheduled'] as const)"
                      :key="mode"
                      type="button"
                      class="flex-1 rounded-lg border px-3 py-2 text-sm font-medium transition-colors"
                      :class="endMode === mode ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 text-muted-foreground hover:bg-surface/50'"
                      @click="endMode = mode"
                    >
                      {{ t(`${prefix}.end${mode.charAt(0).toUpperCase()}${mode.slice(1)}`) }}
                    </button>
                  </div>
                  <input
                    v-if="endMode === 'scheduled'"
                    v-model="endAt"
                    type="datetime-local"
                    class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
                  />
                </div>
              </div>

              <!-- 通知 -->
              <div class="space-y-4 border-t border-border/40 pt-5">
                <div class="flex items-center justify-between rounded-lg border border-border/40 bg-surface/30 px-4 py-3">
                  <div class="flex items-center gap-2">
                    <Bell v-if="notifyEnabled" class="h-4 w-4 text-primary" />
                    <BellOff v-else class="h-4 w-4 text-muted-foreground" />
                    <span class="text-sm font-medium text-foreground">{{ t(`${prefix}.sectionNotify`) }}</span>
                  </div>
                  <label class="relative inline-flex items-center cursor-pointer">
                    <input type="checkbox" v-model="notifyEnabled" class="sr-only peer">
                    <div class="w-9 h-5 bg-surface-elevated rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-border after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-primary"></div>
                  </label>
                </div>

                <template v-if="notifyEnabled">
                  <div class="space-y-1.5">
                    <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.notifyBotSelectLabel`) }}</label>
                    <div v-if="availableBots.length === 0" class="rounded-lg border border-amber-500/30 bg-amber-500/5 p-3 text-xs text-amber-700 dark:text-amber-400">
                      {{ t(`${prefix}.notifyNoBots`) }}
                    </div>
                    <div v-else class="space-y-1.5 max-h-40 overflow-y-auto rounded-lg border border-border/40 p-2">
                      <label
                        v-for="bot in availableBots"
                        :key="bot.id"
                        class="flex items-center gap-2.5 rounded-md px-2.5 py-2 text-sm cursor-pointer transition-colors hover:bg-surface/50"
                        :class="notifyBotIds.includes(bot.id) ? 'bg-primary/5' : ''"
                      >
                        <input
                          type="checkbox"
                          :checked="notifyBotIds.includes(bot.id)"
                          @change="toggleBot(bot.id)"
                          class="h-3.5 w-3.5 rounded border-border text-primary focus:ring-primary/40"
                        />
                        <span class="flex-1 text-foreground">{{ bot.name }}</span>
                        <span class="rounded-full bg-surface-elevated px-1.5 py-0.5 text-[10px] text-muted-foreground">{{ bot.channel }}</span>
                      </label>
                    </div>
                  </div>

                  <div class="space-y-1.5">
                    <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.notifyStartTemplateLabel`) }}</label>
                    <textarea
                      v-model="notifyStartTemplate"
                      rows="2"
                      class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary resize-y"
                    />
                  </div>
                  <div class="space-y-1.5">
                    <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.notifyEndTemplateLabel`) }}</label>
                    <textarea
                      v-model="notifyEndTemplate"
                      rows="2"
                      class="w-full rounded-lg border border-border/50 bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary resize-y"
                    />
                  </div>

                  <div class="space-y-1.5">
                    <div class="inline-flex items-center gap-1.5">
                      <label class="text-xs font-medium text-muted-foreground">{{ t(`${prefix}.notifyVariablesTitle`) }}</label>
                      <Tooltip :text="t(`${prefix}.notifyVariablesTitle`)" wide>
                        <button type="button" class="inline-flex h-4 w-4 items-center justify-center rounded-full text-muted-foreground/70 hover:text-foreground">
                          <CircleHelp class="h-3.5 w-3.5" />
                        </button>
                      </Tooltip>
                    </div>
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
                    <p v-if="copyError" class="text-xs text-destructive">{{ t(`${prefix}.copyVarFailed`) }}</p>
                  </div>
                </template>
              </div>

              <!-- 预览 -->
              <div class="space-y-3 border-t border-border/40 pt-5">
                <div class="flex items-center justify-between">
                  <p class="text-xs font-semibold uppercase tracking-wide text-muted-foreground">{{ t(`${prefix}.previewTitle`) }}</p>
                  <Button variant="secondary" size="sm" class="gap-1.5" :disabled="isPreviewing" @click="runPreview">
                    <Loader2 v-if="isPreviewing" class="h-3.5 w-3.5 animate-spin" />
                    <Eye v-else class="h-3.5 w-3.5" />
                    {{ t('admin.groupRateCampaigns.actions.preview') }}
                  </Button>
                </div>
                <div v-if="previewErrorKey" class="rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-sm text-red-600 dark:text-red-400">
                  {{ t(previewErrorKey) }}
                </div>
                <div v-else-if="previewItems.length === 0" class="rounded-lg border border-border/40 bg-surface/30 p-3 text-xs text-muted-foreground">
                  {{ t(`${prefix}.previewEmpty`) }}
                </div>
                <div v-else class="space-y-2">
                  <p class="text-xs text-muted-foreground">{{ t(`${prefix}.previewTotal`, { total: previewTotal ?? previewItems.length }) }}</p>
                  <div class="max-h-48 overflow-y-auto rounded-lg border border-border/40">
                    <table class="w-full text-left text-xs">
                      <thead class="sticky top-0 bg-surface-elevated">
                        <tr>
                          <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.previewGroupName`) }}</th>
                          <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.previewOriginal`) }}</th>
                          <th class="px-3 py-2 font-medium text-muted-foreground">{{ t(`${prefix}.previewCampaign`) }}</th>
                        </tr>
                      </thead>
                      <tbody class="divide-y divide-border/30">
                        <tr v-for="item in previewItems" :key="item.groupId || item.groupName">
                          <td class="px-3 py-2 text-foreground">{{ item.groupName }}</td>
                          <td class="px-3 py-2 text-muted-foreground">{{ formatMultiplier(item.originalMultiplier) }}</td>
                          <td class="px-3 py-2 font-semibold text-primary">{{ formatMultiplier(item.campaignMultiplier) }}</td>
                        </tr>
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>

              <div v-if="validationError" class="rounded-lg border border-red-500/30 bg-red-500/5 p-3 text-sm text-red-600 dark:text-red-400">
                {{ validationError }}
              </div>
            </div>

            <div class="sticky bottom-0 flex items-center justify-end gap-2 border-t border-border/60 bg-card/95 backdrop-blur px-5 py-4">
              <button
                type="button"
                class="rounded-lg border border-border/50 px-4 py-2 text-sm font-medium text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                @click="emit('close')"
              >
                {{ t('admin.groupRateCampaigns.actions.cancelEdit') }}
              </button>
              <button
                type="button"
                class="inline-flex items-center gap-1.5 rounded-lg bg-primary px-4 py-2 text-sm font-medium text-primary-foreground transition-colors hover:bg-primary/90 disabled:opacity-50"
                :disabled="isSubmitting"
                @click="handleSubmit"
              >
                <Loader2 v-if="isSubmitting" class="h-3.5 w-3.5 animate-spin" />
                {{ t('admin.groupRateCampaigns.actions.confirmCreate') }}
              </button>
            </div>
          </div>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>
