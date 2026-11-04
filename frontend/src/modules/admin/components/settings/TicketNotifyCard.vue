<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { LifeBuoy, MessageSquare } from 'lucide-vue-next'

// 工单通知：TransitLink 用户新建工单或追问时，推送给这里选的机器人（不选就不推送）。
// 随「保存」按钮一起保存在策略设置里。
const selectedBots = defineModel<string[]>('selectedBots', { required: true })

defineProps<{
  allBots: Array<{ id: string, name: string }>
  hasBots: boolean
}>()

const { t } = useI18n()

const toggleBot = (botId: string) => {
  const next = [...selectedBots.value]
  const index = next.indexOf(botId)
  if (index >= 0) next.splice(index, 1)
  else next.push(botId)
  selectedBots.value = next
}
</script>

<template>
  <div class="rounded-2xl border border-border/50 bg-card shadow-sm overflow-hidden">
    <div class="p-5 flex items-start gap-3">
      <div class="p-2 bg-sky-500/10 text-sky-600 dark:text-sky-400 rounded-xl shrink-0 mt-0.5">
        <LifeBuoy class="w-5 h-5" />
      </div>
      <div class="min-w-0 flex-1 space-y-3">
        <div>
          <h4 class="text-sm font-semibold text-foreground">{{ t('admin.settings.ticketNotify.title') }}</h4>
          <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.ticketNotify.help') }}</p>
        </div>
        <div class="flex flex-wrap gap-2">
          <button
            v-for="bot in allBots"
            :key="'ticket-' + bot.id"
            type="button"
            :aria-pressed="selectedBots.includes(bot.id)"
            class="flex select-none items-center gap-2 rounded-lg border px-3 py-1.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            :class="selectedBots.includes(bot.id) ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 hover:bg-surface/50'"
            @click="toggleBot(bot.id)"
          >
            <MessageSquare class="w-3.5 h-3.5" />
            <span class="text-sm">{{ bot.name || t('admin.settings.unnamedBot') }}</span>
          </button>
          <div v-if="!hasBots" class="text-sm text-muted-foreground italic py-1">{{ t('admin.settings.noBotsConfigured') }}</div>
        </div>
      </div>
    </div>
  </div>
</template>
