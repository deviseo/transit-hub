<script setup lang="ts">
import { computed } from 'vue'
import { Check, Loader2 } from 'lucide-vue-next'

export interface LoadingStep {
  key: string
  labelKey: string
  status: 'pending' | 'active' | 'done' | 'error'
}

const props = defineProps<{
  open: boolean
  steps: LoadingStep[]
}>()

import { t } from '@/locales'
const progress = computed(() => {
  if (props.steps.length === 0) return 0
  const done = props.steps.filter(s => s.status === 'done').length
  return Math.round((done / props.steps.length) * 100)
})
</script>

<template>
  <Teleport defer to="body">
    <div v-if="open" class="fixed inset-0 z-[100] flex items-center justify-center p-4">
      <div class="absolute inset-0 bg-background/80 backdrop-blur-sm"></div>

      <div
        role="dialog"
        aria-modal="true"
        class="relative w-full max-w-sm overflow-hidden rounded-[2rem] border border-border/60 bg-card text-card-foreground shadow-2xl shadow-primary/10 animate-in fade-in zoom-in-95 duration-200"
      >
        <div class="absolute left-0 right-0 top-0 h-1 bg-muted">
          <div
            class="h-full bg-gradient-to-r from-primary to-accent transition-all duration-500 ease-out"
            :style="{ width: `${progress}%` }"
          />
        </div>

        <div class="px-6 pt-7 pb-6 space-y-5">
          <!-- Title -->
          <div class="text-center space-y-1">
            <h2 class="text-base font-semibold text-foreground">
              {{ t('admin.dashboard.loadingModal.title') }}
            </h2>
            <p class="text-xs text-muted-foreground">
              {{ t('admin.dashboard.loadingModal.progress', { progress }) }}
            </p>
          </div>

          <!-- Steps -->
          <div class="space-y-2.5">
            <div
              v-for="step in steps"
              :key="step.key"
              class="flex items-center gap-3 rounded-xl px-3 py-2 transition-colors"
              :class="{
                'bg-primary/5': step.status === 'active',
                'bg-signal/5': step.status === 'done',
                'bg-red-500/5': step.status === 'error',
              }"
            >
              <!-- Icon -->
              <div class="flex h-6 w-6 shrink-0 items-center justify-center">
                <Loader2
                  v-if="step.status === 'active'"
                  class="h-4 w-4 animate-spin text-primary"
                />
                <Check
                  v-else-if="step.status === 'done'"
                  class="h-4 w-4 text-signal"
                />
                <span
                  v-else-if="step.status === 'error'"
                  class="text-xs text-red-500 font-bold"
                >!</span>
                <span
                  v-else
                  class="h-1.5 w-1.5 rounded-full bg-muted-foreground/30"
                />
              </div>

              <!-- Label -->
              <span
                class="text-sm transition-colors"
                :class="{
                  'text-primary font-medium': step.status === 'active',
                  'text-signal/80': step.status === 'done',
                  'text-red-500': step.status === 'error',
                  'text-muted-foreground/50': step.status === 'pending',
                }"
              >
                {{ t(step.labelKey) }}
              </span>
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
