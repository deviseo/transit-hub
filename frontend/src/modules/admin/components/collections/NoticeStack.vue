<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { RouterLink } from 'vue-router'
import { AlertCircle, CheckCircle2, X } from 'lucide-vue-next'
import { useNotices } from '../../composables/useNotices'

// 页面右下角的提醒（后台采集完成 / 失败等）。
const { t } = useI18n()
const { notices, dismissNotice } = useNotices()
</script>

<template>
  <div class="pointer-events-none fixed bottom-4 right-4 z-[80] flex w-[calc(100vw-2rem)] max-w-sm flex-col gap-2" role="status" aria-live="polite">
    <TransitionGroup name="notice">
      <div
        v-for="notice in notices"
        :key="notice.id"
        class="pointer-events-auto flex items-start gap-3 rounded-lg border border-border/60 bg-card p-3 shadow-lg"
      >
        <CheckCircle2 v-if="notice.tone === 'success'" class="mt-0.5 h-4 w-4 shrink-0 text-emerald-600 dark:text-emerald-400" aria-hidden="true" />
        <AlertCircle v-else class="mt-0.5 h-4 w-4 shrink-0 text-red-600 dark:text-red-400" aria-hidden="true" />
        <div class="min-w-0 flex-1">
          <p class="text-sm font-medium text-foreground">{{ notice.title }}</p>
          <p v-if="notice.message" class="mt-0.5 text-xs text-muted-foreground">{{ notice.message }}</p>
          <div v-if="notice.actions.length > 0" class="mt-1.5 flex flex-wrap gap-x-3 gap-y-1">
            <RouterLink
              v-for="action in notice.actions"
              :key="action.to"
              :to="action.to"
              class="text-xs font-medium text-primary hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              @click="dismissNotice(notice.id)"
            >
              {{ action.label }}
            </RouterLink>
          </div>
        </div>
        <button
          type="button"
          class="rounded p-0.5 text-muted-foreground transition-colors hover:bg-surface hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
          :aria-label="t('admin.collections.dismiss')"
          @click="dismissNotice(notice.id)"
        >
          <X class="h-4 w-4" />
        </button>
      </div>
    </TransitionGroup>
  </div>
</template>

<style scoped>
.notice-enter-active,
.notice-leave-active {
  transition: opacity 0.2s ease, transform 0.2s ease;
}

.notice-enter-from,
.notice-leave-to {
  opacity: 0;
  transform: translateY(8px);
}
</style>
