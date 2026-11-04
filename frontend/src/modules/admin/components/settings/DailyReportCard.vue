<script setup lang="ts">
import { ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { CalendarClock, CheckCircle2, Eye, Loader2, MessageSquare, Send } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { previewDailyReport, sendDailyReportTest } from '../../api/settings'

// 经营日报设置卡片：开关、发送时间（北京时间）、接收机器人，以及预览 / 测试发送。
// 开关、时间和机器人随「保存」按钮一起保存在策略设置里；预览和测试发送不需要先保存。
const enabled = defineModel<boolean>('enabled', { required: true })
const time = defineModel<string>('time', { required: true })
const selectedBots = defineModel<string[]>('selectedBots', { required: true })

defineProps<{
  allBots: Array<{ id: string, name: string }>
  hasBots: boolean
}>()

const { t, te } = useI18n()

const previewText = ref('')
const isPreviewing = ref(false)
const isSending = ref(false)
const sentCount = ref<number | null>(null)
const errorKey = ref('')

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.settings.dailyReport.errors.request'))

const toggleBot = (botId: string) => {
  const next = [...selectedBots.value]
  const index = next.indexOf(botId)
  if (index >= 0) next.splice(index, 1)
  else next.push(botId)
  selectedBots.value = next
}

const preview = async () => {
  if (isPreviewing.value) return
  isPreviewing.value = true
  errorKey.value = ''
  try {
    previewText.value = (await previewDailyReport()).text
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.settings.dailyReport.errors.request'
  } finally {
    isPreviewing.value = false
  }
}

const sendTest = async () => {
  if (isSending.value) return
  isSending.value = true
  errorKey.value = ''
  sentCount.value = null
  try {
    const response = await sendDailyReportTest(selectedBots.value)
    previewText.value = response.report.text
    sentCount.value = response.sent
    setTimeout(() => { sentCount.value = null }, 4000)
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.settings.dailyReport.errors.request'
  } finally {
    isSending.value = false
  }
}
</script>

<template>
  <div class="rounded-2xl border border-border/50 bg-card shadow-sm overflow-hidden">
    <div class="p-5 flex items-start justify-between gap-4">
      <div class="flex items-start gap-3">
        <div class="p-2 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 rounded-xl shrink-0 mt-0.5">
          <CalendarClock class="w-5 h-5" />
        </div>
        <div>
          <h4 class="text-sm font-semibold text-foreground">{{ t('admin.settings.dailyReport.title') }}</h4>
          <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.dailyReport.help') }}</p>
        </div>
      </div>
      <label class="relative inline-flex items-center cursor-pointer shrink-0 mt-1">
        <input v-model="enabled" type="checkbox" class="sr-only peer" :aria-label="t('admin.settings.dailyReport.title')">
        <div class="peer h-6 w-11 rounded-full bg-surface-elevated peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-border after:bg-white after:transition-transform after:content-[''] peer-checked:after:translate-x-full peer-checked:after:border-white"></div>
      </label>
    </div>
    <div v-if="enabled" class="px-5 pb-5 pt-0">
      <div class="space-y-4 animate-in slide-in-from-top-2 fade-in duration-200 sm:pl-12">
        <div class="grid gap-1.5">
          <label for="daily-report-time" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.dailyReport.time') }}</label>
          <div class="flex flex-wrap items-center gap-2">
            <input
              id="daily-report-time"
              v-model="time"
              type="time"
              step="60"
              class="h-9 w-32 rounded-lg border border-border/60 bg-surface px-3 text-sm text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary"
            >
            <span class="text-xs text-muted-foreground">{{ t('admin.settings.dailyReport.timeHelp') }}</span>
          </div>
        </div>

        <div class="grid gap-1.5">
          <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.notifyBots') }} <span class="text-destructive">*</span></label>
          <div class="flex flex-wrap gap-2">
            <button
              v-for="bot in allBots"
              :key="'report-' + bot.id"
              type="button"
              :aria-pressed="selectedBots.includes(bot.id)"
              class="flex select-none items-center gap-2 rounded-lg border px-3 py-1.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="selectedBots.includes(bot.id) ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 hover:bg-surface/50'"
              @click="toggleBot(bot.id)"
            >
              <MessageSquare class="w-3.5 h-3.5" />
              <span class="text-sm">{{ bot.name || t('admin.settings.unnamedBot') }}</span>
            </button>
            <div v-if="!hasBots" class="text-sm text-muted-foreground italic py-1">
              {{ t('admin.settings.noBotsConfigured') }}
            </div>
          </div>
          <p v-if="selectedBots.length === 0 && hasBots" class="text-xs text-destructive mt-0.5">{{ t('admin.settings.mustSelectBot') }}</p>
        </div>

        <div class="flex flex-wrap items-center gap-2">
          <Button variant="secondary" size="sm" class="rounded-lg" :disabled="isPreviewing" @click="preview">
            <Loader2 v-if="isPreviewing" class="h-4 w-4 animate-spin" />
            <Eye v-else class="h-4 w-4" />
            {{ t('admin.settings.dailyReport.preview') }}
          </Button>
          <Button variant="secondary" size="sm" class="rounded-lg" :disabled="isSending || selectedBots.length === 0" @click="sendTest">
            <Loader2 v-if="isSending" class="h-4 w-4 animate-spin" />
            <Send v-else class="h-4 w-4" />
            {{ t('admin.settings.dailyReport.sendTest') }}
          </Button>
          <span v-if="sentCount != null" class="inline-flex items-center gap-1 text-xs text-emerald-700 dark:text-emerald-400" role="status">
            <CheckCircle2 class="h-3.5 w-3.5" />
            {{ t('admin.settings.dailyReport.sent', { count: sentCount }) }}
          </span>
          <span v-if="errorKey" class="text-xs text-destructive" role="alert">{{ errorText(errorKey) }}</span>
        </div>
        <p class="text-xs text-muted-foreground">{{ t('admin.settings.dailyReport.saveHint') }}</p>

        <div v-if="previewText" class="rounded-lg border border-border/50 bg-surface p-3">
          <p class="mb-2 text-xs font-medium text-muted-foreground">{{ t('admin.settings.dailyReport.previewTitle') }}</p>
          <pre class="whitespace-pre-wrap break-words font-sans text-sm leading-6 text-foreground">{{ previewText }}</pre>
        </div>
      </div>
    </div>
  </div>
</template>
