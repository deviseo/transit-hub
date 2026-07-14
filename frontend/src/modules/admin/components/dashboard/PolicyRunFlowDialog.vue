<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { BookOpenText, X } from 'lucide-vue-next'

const props = defineProps<{
  open: boolean
}>()

const emit = defineEmits<{
  (event: 'close'): void
}>()

const { t } = useI18n()
const prefix = 'admin.connectionHealth.policyDrawer.runFlow'
const titleId = 'connection-health-policy-run-flow-title'
const descriptionId = 'connection-health-policy-run-flow-description'

// 基础 dialog 可访问性：打开时挂载 Esc 关闭监听，关闭/卸载时立即移除，避免全局 keydown 泄漏。
const onKeydown = (event: KeyboardEvent) => {
  if (event.key === 'Escape') emit('close')
}

watch(
  () => props.open,
  (isOpen) => {
    if (isOpen) {
      window.addEventListener('keydown', onKeydown)
    } else {
      window.removeEventListener('keydown', onKeydown)
    }
  },
  { immediate: true },
)

onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown)
})

// 运行流程的 10 个步骤纯粹是静态说明文案，用 tm() 取出 steps 对象再按 rt() 渲染，
// 这样新增/调整某一步时只需要改 locale，不用同步改这里的模板结构。
const stepKeys = [
  'policyScope',
  'modelProvider',
  'schedulerCadence',
  'dueCheck',
  'budget',
  'stateTransition',
  'cooldownObservation',
  'autoDegradeVsRemoteAction',
  'manualProbe',
  'nextProbeCopy',
] as const
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
      <div v-if="open" class="fixed inset-0 z-[160] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="emit('close')" />

        <div
          role="dialog"
          aria-modal="true"
          :aria-labelledby="titleId"
          :aria-describedby="descriptionId"
          class="relative flex h-[min(680px,calc(100vh-2rem))] w-full max-w-2xl flex-col overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl"
        >
          <div class="flex shrink-0 items-center justify-between gap-3 border-b border-border/60 px-5 py-4">
            <div class="flex min-w-0 items-center gap-2.5">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <BookOpenText class="h-4 w-4" />
              </div>
              <div class="min-w-0">
                <h3 :id="titleId" class="text-sm font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h3>
                <p :id="descriptionId" class="truncate text-xs text-muted-foreground">{{ t(`${prefix}.subtitle`) }}</p>
              </div>
            </div>
            <button
              type="button"
              :aria-label="t(`${prefix}.close`)"
              class="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
              @click="emit('close')"
            >
              <X class="h-4 w-4" />
            </button>
          </div>

          <div class="flex-1 space-y-4 overflow-y-auto px-5 py-4">
            <div v-for="stepKey in stepKeys" :key="stepKey" class="rounded-lg border border-border/40 bg-surface/30 p-3.5">
              <h4 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.steps.${stepKey}.title`) }}</h4>
              <p class="mt-1.5 text-xs leading-relaxed text-muted-foreground">{{ t(`${prefix}.steps.${stepKey}.description`) }}</p>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
