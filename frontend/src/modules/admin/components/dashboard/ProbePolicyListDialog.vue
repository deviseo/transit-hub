<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { Ban, CheckCircle2, Plus, Settings2, ShieldCheck, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Tooltip } from '@/components/ui/tooltip'
import type { ConnectionHealthPolicy } from '../../types/connectionHealth'

defineProps<{
  open: boolean
  policies: ConnectionHealthPolicy[]
}>()

const emit = defineEmits<{
  (event: 'close'): void
  (event: 'create'): void
  (event: 'edit', policy: ConnectionHealthPolicy): void
  (event: 'toggle', policy: ConnectionHealthPolicy): void
}>()

const { t } = useI18n()
const prefix = 'admin.connectionHealth.policies'
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
      <div v-if="open" class="fixed inset-0 z-[140] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="emit('close')" />

        <div class="relative flex h-[min(760px,calc(100vh-2rem))] w-full max-w-4xl flex-col overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl">
          <div class="flex shrink-0 items-center justify-between gap-3 border-b border-border/60 px-5 py-4">
            <div class="flex min-w-0 items-center gap-2.5">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <ShieldCheck class="h-4 w-4" />
              </div>
              <div class="min-w-0">
                <h3 class="text-sm font-semibold text-foreground">{{ t(`${prefix}.title`) }}</h3>
                <p class="truncate text-xs text-muted-foreground">{{ t(`${prefix}.subtitle`) }}</p>
              </div>
            </div>
            <div class="flex shrink-0 items-center gap-2">
              <Button size="sm" @click="emit('create')">
                <Plus class="h-4 w-4" />
                {{ t(`${prefix}.create`) }}
              </Button>
              <button
                type="button"
                class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
                @click="emit('close')"
              >
                <X class="h-4 w-4" />
              </button>
            </div>
          </div>

          <div class="flex-1 overflow-y-auto px-5 py-4">
            <div v-if="policies.length === 0" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
              <ShieldCheck class="h-8 w-8 text-muted-foreground/40" />
              <p class="text-sm text-muted-foreground">{{ t(`${prefix}.empty`) }}</p>
            </div>

            <ul v-else class="space-y-2">
              <li v-for="policy in policies" :key="policy.id" class="flex items-center justify-between gap-3 rounded-lg border border-border/40 px-3 py-2.5">
                <div class="min-w-0">
                  <div class="flex flex-wrap items-center gap-2">
                    <span class="text-sm font-medium text-foreground">{{ policy.name }}</span>
                    <span
                      class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                      :class="policy.enabled ? 'bg-green-500/10 text-green-600 dark:text-green-400' : 'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400'"
                    >
                      {{ policy.enabled ? t(`${prefix}.enabled`) : t(`${prefix}.disabled`) }}
                    </span>
                    <span v-if="policy.autoRemoteActionEnabled" class="inline-flex items-center rounded-full bg-amber-500/10 px-2 py-0.5 text-xs font-medium text-amber-600 dark:text-amber-400">
                      {{ t(`${prefix}.remoteActionOn`) }}
                    </span>
                  </div>
                  <p class="mt-0.5 text-xs text-muted-foreground">
                    {{ policy.ownGroupName || t(`${prefix}.allGroupsScope`) }}
                    · {{ t(`${prefix}.modelTargetCount`, { count: policy.modelTargets.filter(m => m.enabled).length }) }}
                    · {{ policy.probeIntervalSeconds }}s
                  </p>
                </div>
                <div class="flex shrink-0 items-center gap-1.5">
                  <Tooltip :text="policy.enabled ? t(`${prefix}.disable`) : t(`${prefix}.enable`)">
                    <button type="button" class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface-line hover:text-foreground" @click="emit('toggle', policy)">
                      <Ban v-if="policy.enabled" class="h-4 w-4" />
                      <CheckCircle2 v-else class="h-4 w-4" />
                    </button>
                  </Tooltip>
                  <Tooltip :text="t(`${prefix}.edit`)">
                    <button type="button" class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface-line hover:text-primary" @click="emit('edit', policy)">
                      <Settings2 class="h-4 w-4" />
                    </button>
                  </Tooltip>
                </div>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
