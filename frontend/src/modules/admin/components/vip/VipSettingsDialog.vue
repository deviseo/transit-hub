<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { AlertCircle, Loader2, MessageSquare, RotateCcw, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { restoreDismissedVip, saveVipSettings } from '../../api/vip'
import { getNotificationChannelSettings } from '../../api/settings'
import { vipReminderPlaceholder, vipReminderText } from '../../utils/vipReminder'
import type { VipRow, VipSettings } from '../../types/vip'

// 大户关注的设置：推荐前几名、什么情况算快用完、用哪些机器人提醒站长、「复制提醒」的话术。
const props = defineProps<{
  settings: VipSettings
  siteName: string
  dismissedCount: number
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'saved', settings: VipSettings): void
  (event: 'restored'): void
}>()

const { t, te } = useI18n()

const form = ref<VipSettings>({ ...props.settings, botIds: [...props.settings.botIds] })
const bots = ref<Array<{ id: string, name: string }>>([])
const botsLoaded = ref(false)
const isSaving = ref(false)
const isRestoring = ref(false)
const restoredCount = ref<number | null>(null)
const errorKey = ref('')

const errorText = (key: string): string => (te(key) ? t(key) : t('admin.vip.errors.request'))

onMounted(async () => {
  document.addEventListener('keydown', onKeydown)
  try {
    const channels = await getNotificationChannelSettings()
    bots.value = [...channels.dingtalk, ...channels.wecom, ...channels.qq, ...channels.feishu, ...channels.telegram]
      .map((bot) => ({ id: bot.id, name: bot.name }))
  } catch {
    bots.value = []
  } finally {
    botsLoaded.value = true
  }
})

const toggleBot = (id: string) => {
  const next = [...form.value.botIds]
  const index = next.indexOf(id)
  if (index >= 0) next.splice(index, 1)
  else next.push(id)
  form.value.botIds = next
}

// 预览话术：一个示例大户（快用完、用完了各一个）。
const sample = (status: 'low' | 'empty'): VipRow => ({
  siteUserId: '1',
  email: 'alice@example.com',
  username: 'alice',
  status,
  source: { manual: false, note: '' },
  balance: status === 'low' ? 35 : 0,
  dailyAverage: 60,
  previousAverage: 55,
  trend: 0.09,
  daysLeft: status === 'low' ? 35 / 60 : 0,
  lifetime: 3000,
  lastUsedAt: null,
})
const previewLow = computed(() => vipReminderText(t, sample('low'), props.siteName, form.value))
const previewEmpty = computed(() => vipReminderText(t, sample('empty'), props.siteName, form.value))

const valid = computed(() => {
  const value = form.value
  const inRange = (number: number, max: number) => Number.isFinite(number) && number >= 0 && number <= max
  return Number.isInteger(value.dailyTop) && inRange(value.dailyTop, 100)
    && Number.isInteger(value.lifetimeTop) && inRange(value.lifetimeTop, 100)
    && inRange(value.alertDays, 30) && inRange(value.alertBalance, 1_000_000)
})

const save = async () => {
  if (!valid.value || isSaving.value) return
  isSaving.value = true
  errorKey.value = ''
  try {
    emit('saved', await saveVipSettings(form.value))
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.vip.errors.request'
  } finally {
    isSaving.value = false
  }
}

const restore = async () => {
  isRestoring.value = true
  errorKey.value = ''
  try {
    restoredCount.value = await restoreDismissedVip()
    emit('restored')
  } catch (error) {
    errorKey.value = error instanceof Error ? error.message : 'admin.vip.errors.request'
  } finally {
    isRestoring.value = false
  }
}

const close = () => {
  if (!isSaving.value) emit('close')
}

const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') close()
}

onBeforeUnmount(() => document.removeEventListener('keydown', onKeydown))

const numberInput = 'h-9 w-24 rounded-lg border border-border/60 bg-surface px-2 text-right text-sm tabular-nums text-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary'
const textArea = 'w-full resize-none rounded-lg border border-border/60 bg-surface px-3 py-2 text-sm text-foreground outline-none placeholder:text-muted-foreground focus:border-primary focus:ring-1 focus:ring-primary'
</script>

<template>
  <div class="fixed inset-0 z-50 flex items-center justify-center bg-background/80 p-3 backdrop-blur-sm" role="dialog" aria-modal="true" aria-labelledby="vip-settings-title">
    <form class="flex max-h-[calc(100dvh-2rem)] w-full max-w-2xl flex-col rounded-lg border border-border/60 bg-card shadow-xl" @submit.prevent="save">
      <div class="flex shrink-0 items-start justify-between gap-4 border-b border-border/50 p-4">
        <h2 id="vip-settings-title" class="text-lg font-semibold text-foreground">{{ t('admin.vip.settings.title') }}</h2>
        <button type="button" class="rounded-lg p-2 text-muted-foreground outline-none hover:bg-surface focus-visible:ring-2 focus-visible:ring-primary" :aria-label="t('admin.vip.actions.close')" :disabled="isSaving" @click="close">
          <X class="h-4 w-4" />
        </button>
      </div>

      <div class="min-h-0 flex-1 space-y-5 overflow-y-auto p-4 text-sm">
        <section class="space-y-2">
          <h3 class="font-semibold text-foreground">{{ t('admin.vip.settings.recommendTitle') }}</h3>
          <div class="flex flex-wrap gap-x-6 gap-y-2">
            <label class="flex items-center gap-2 text-foreground">
              {{ t('admin.vip.settings.dailyTop') }}
              <input v-model.number="form.dailyTop" type="number" min="0" max="100" step="1" :class="numberInput">
            </label>
            <label class="flex items-center gap-2 text-foreground">
              {{ t('admin.vip.settings.lifetimeTop') }}
              <input v-model.number="form.lifetimeTop" type="number" min="0" max="100" step="1" :class="numberInput">
            </label>
          </div>
          <p class="text-xs text-muted-foreground">{{ t('admin.vip.settings.recommendHint') }}</p>
          <div v-if="dismissedCount > 0 && restoredCount == null" class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
            {{ t('admin.vip.settings.dismissed', { count: dismissedCount }) }}
            <Button type="button" variant="ghost" size="sm" class="h-7 rounded-lg px-2 text-xs" :disabled="isRestoring" @click="restore">
              <Loader2 v-if="isRestoring" class="h-3.5 w-3.5 animate-spin" />
              <RotateCcw v-else class="h-3.5 w-3.5" />
              {{ t('admin.vip.settings.restoreDismissed') }}
            </Button>
          </div>
          <p v-if="restoredCount != null" class="text-xs text-emerald-600 dark:text-emerald-400">{{ t('admin.vip.settings.restored', { count: restoredCount }) }}</p>
        </section>

        <section class="space-y-2 border-t border-border/50 pt-4">
          <h3 class="font-semibold text-foreground">{{ t('admin.vip.settings.lowTitle') }}</h3>
          <div class="flex flex-wrap gap-x-6 gap-y-2">
            <label class="flex items-center gap-2 text-foreground">
              {{ t('admin.vip.settings.alertDays') }}
              <input v-model.number="form.alertDays" type="number" min="0" max="30" step="0.5" :class="numberInput">
            </label>
            <label class="flex items-center gap-2 text-foreground">
              {{ t('admin.vip.settings.alertBalance') }}
              <input v-model.number="form.alertBalance" type="number" min="0" step="1" :class="numberInput">
            </label>
          </div>
          <p class="text-xs text-muted-foreground">{{ t('admin.vip.settings.lowHint') }}</p>
        </section>

        <section class="space-y-2 border-t border-border/50 pt-4">
          <label class="flex items-center gap-2 font-semibold text-foreground">
            <input v-model="form.alertsEnabled" type="checkbox" class="h-4 w-4 accent-primary">
            {{ t('admin.vip.settings.alertsEnabled') }}
          </label>
          <p class="text-xs text-muted-foreground">{{ t('admin.vip.settings.alertsHint') }}</p>
          <div class="flex flex-wrap gap-2" :class="{ 'pointer-events-none opacity-50': !form.alertsEnabled }">
            <button
              v-for="bot in bots"
              :key="bot.id"
              type="button"
              :aria-pressed="form.botIds.includes(bot.id)"
              class="flex select-none items-center gap-2 rounded-lg border px-3 py-1.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :class="form.botIds.includes(bot.id) ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 hover:bg-surface/50'"
              @click="toggleBot(bot.id)"
            >
              <MessageSquare class="h-3.5 w-3.5" />
              <span>{{ bot.name || t('admin.settings.unnamedBot') }}</span>
            </button>
            <p v-if="botsLoaded && bots.length === 0" class="py-1 text-sm italic text-muted-foreground">{{ t('admin.vip.settings.noBots') }}</p>
          </div>
        </section>

        <section class="space-y-2 border-t border-border/50 pt-4">
          <h3 class="font-semibold text-foreground">{{ t('admin.vip.settings.reminderTitle') }}</h3>
          <p class="text-xs text-muted-foreground">{{ t('admin.vip.settings.reminderHint') }}</p>
          <label class="block space-y-1">
            <span class="text-xs font-medium text-muted-foreground">{{ t('admin.vip.settings.reminderLow') }}</span>
            <textarea v-model="form.reminderText" rows="2" maxlength="500" :class="textArea" :placeholder="vipReminderPlaceholder(t, false)" />
            <span class="block rounded-md bg-surface px-2 py-1 text-xs text-muted-foreground">{{ previewLow }}</span>
          </label>
          <label class="block space-y-1">
            <span class="text-xs font-medium text-muted-foreground">{{ t('admin.vip.settings.reminderEmpty') }}</span>
            <textarea v-model="form.emptyReminderText" rows="2" maxlength="500" :class="textArea" :placeholder="vipReminderPlaceholder(t, true)" />
            <span class="block rounded-md bg-surface px-2 py-1 text-xs text-muted-foreground">{{ previewEmpty }}</span>
          </label>
        </section>

        <div v-if="errorKey" class="flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/10 p-3 text-sm text-warning" role="alert">
          <AlertCircle class="mt-0.5 h-4 w-4 shrink-0" />
          <span>{{ errorText(errorKey) }}</span>
        </div>
      </div>

      <div class="flex shrink-0 flex-col-reverse gap-2 border-t border-border/50 p-4 sm:flex-row sm:justify-end">
        <Button type="button" variant="secondary" class="rounded-lg" :disabled="isSaving" @click="close">{{ t('admin.vip.actions.cancel') }}</Button>
        <Button type="submit" class="rounded-lg" :disabled="!valid || isSaving">
          <Loader2 v-if="isSaving" class="h-4 w-4 animate-spin" />
          {{ t('admin.vip.actions.save') }}
        </Button>
      </div>
    </form>
  </div>
</template>
