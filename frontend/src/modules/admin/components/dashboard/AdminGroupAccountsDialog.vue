<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
import { Activity, Eye, Layers, ShieldAlert, ShieldCheck, ShieldQuestion, X, Zap } from 'lucide-vue-next'
import { Tooltip } from '@/components/ui/tooltip'
import { connectionHealthStateBadgeClass } from '../../composables/useConnectionHealth'
import type {
  AdminGroupAccount,
  AdminGroupHealth,
  ConnectionHealthState,
} from '../../types/connectionHealth'

const props = defineProps<{
  open: boolean
  group: AdminGroupHealth | null
}>()

const emit = defineEmits<{
  (event: 'close'): void
  // probe / view-events 只对可独立探活（probeAvailable）的账号/渠道触发。
  (event: 'probe', account: AdminGroupAccount): void
  (event: 'view-events', account: AdminGroupAccount): void
  // assign-policy 与 probeAvailable 完全解耦：无论是否可手动探活，都可以管理策略分配。
  (event: 'assign-policy', account: AdminGroupAccount): void
}>()

const { t } = useI18n()
const prefix = 'admin.connectionHealth'
const dialogPrefix = `${prefix}.accountsDialog`

// isNewAPI: new-api channel 才有 weight 概念。sub2api 账号没有 weight，展示 "-"，
// 不能把 priority 冒充成 weight。
const isNewAPI = computed(() => (props.group?.platform ?? '').toLowerCase().includes('new'))

const groupTypeLabel = (type: string): string => {
  switch (type) {
    case 'exclusive':
      return t(`${prefix}.groupTypes.exclusive`)
    case 'subscription':
      return t(`${prefix}.groupTypes.subscription`)
    default:
      return t(`${prefix}.groupTypes.public`)
  }
}

const stateLabel = (state: ConnectionHealthState | string): string => t(`${prefix}.stateLabels.${state}`)

// 不可探活原因 -> 文案。未知 reason 回退到通用「不可探活」。
const KNOWN_REASONS = new Set([
  'credential_unavailable',
  'secure_verification_required',
  'base_url_unavailable',
  'model_unavailable',
  'export_unavailable',
  'credentials_redacted',
])
const reasonLabel = (reason: string | undefined): string => {
  if (reason && KNOWN_REASONS.has(reason)) return t(`${prefix}.probeUnavailableReasons.${reason}`)
  return t(`${dialogPrefix}.unprobeable`)
}

// aggregateState 从账号的逐模型健康状态归纳出一个代表性状态用于行首徽标：
// 暂停 > 已禁用 > 降级/观察/恢复 > 健康。没有任何模型健康数据时返回空串。
const STATE_PRIORITY: ConnectionHealthState[] = ['suspended', 'disabled', 'degraded', 'observing', 'recovering', 'healthy']
const aggregateState = (account: AdminGroupAccount): ConnectionHealthState | '' => {
  if (!account.modelHealth || account.modelHealth.length === 0) return ''
  const present = new Set(account.modelHealth.map((m) => m.state))
  for (const s of STATE_PRIORITY) {
    if (present.has(s)) return s
  }
  return ''
}

const numberOrDash = (value: number | undefined | null): string =>
  value === undefined || value === null ? '-' : String(value)

// assignedPolicyLabel 汇总账号已分配策略的展示文案：优先展示第一个策略名 + 剩余数量，
// 都没有分配时展示明确的「未分配策略，不会自动探活」提示。
const assignedPolicyLabel = (account: AdminGroupAccount): string => {
  const assigned = account.assignedPolicies ?? []
  if (assigned.length === 0) return t(`${dialogPrefix}.unassignedPolicy`)
  if (assigned.length === 1) return assigned[0].policyName
  return t(`${dialogPrefix}.assignedPolicyCount`, { name: assigned[0].policyName, count: assigned.length - 1 })
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
      <div v-if="open && group" class="fixed inset-0 z-[130] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/60 backdrop-blur-sm" @click="emit('close')" />

        <div class="relative flex h-[min(720px,calc(100vh-2rem))] w-full max-w-5xl flex-col overflow-hidden rounded-2xl border border-border/60 bg-card shadow-2xl">
          <!-- 头部：分组上下文 -->
          <div class="flex shrink-0 items-start justify-between gap-3 border-b border-border/60 px-5 py-4">
            <div class="flex min-w-0 items-center gap-2.5">
              <div class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg bg-primary/10 text-primary">
                <Layers class="h-4 w-4" />
              </div>
              <div class="min-w-0">
                <h3 class="truncate text-sm font-semibold text-foreground">{{ group.name }}</h3>
                <div class="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-muted-foreground">
                  <span>{{ group.platform || t(`${dialogPrefix}.unknownPlatform`) }}</span>
                  <span>·</span>
                  <span>{{ groupTypeLabel(group.type) }}</span>
                  <span>·</span>
                  <span>{{ t(`${dialogPrefix}.multiplier`) }} {{ group.multiplierDisplay || '-' }}</span>
                  <span>·</span>
                  <span
                    class="inline-flex items-center rounded-full px-2 py-0.5 font-medium"
                    :class="group.status === 'active' || group.status === '1'
                      ? 'bg-green-500/10 text-green-600 dark:text-green-400'
                      : 'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400'"
                  >
                    {{ group.status || t(`${dialogPrefix}.unknownStatus`) }}
                  </span>
                </div>
              </div>
            </div>
            <button
              type="button"
              class="shrink-0 rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground"
              @click="emit('close')"
            >
              <X class="h-4 w-4" />
            </button>
          </div>

          <!-- 账号/渠道列表 -->
          <div class="flex-1 overflow-y-auto px-5 py-4">
            <p v-if="group.accountsError" class="rounded-lg bg-red-500/10 px-4 py-3 text-sm text-red-600 dark:text-red-400">
              {{ t(group.accountsError) }}
            </p>
            <div v-else-if="group.accounts.length === 0" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
              <Activity class="h-8 w-8 text-muted-foreground/40" />
              <p class="text-sm text-muted-foreground">{{ t(`${dialogPrefix}.empty`) }}</p>
            </div>
            <table v-else class="w-full text-xs">
              <thead>
                <tr class="text-left text-muted-foreground">
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.name`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.platform`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.type`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.status`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.priority`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.concurrency`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.weight`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.models`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.probeStatus`) }}</th>
                  <th class="py-1.5 pr-3 font-medium">{{ t(`${dialogPrefix}.columns.policyAssignment`) }}</th>
                  <th class="py-1.5 pr-0 text-right font-medium">{{ t(`${dialogPrefix}.columns.actions`) }}</th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="account in group.accounts" :key="account.id" class="border-t border-border/20 align-top">
                  <td class="py-2 pr-3 text-foreground">{{ account.name || account.id }}</td>
                  <td class="py-2 pr-3 text-muted-foreground">{{ account.platform || '-' }}</td>
                  <td class="py-2 pr-3 text-muted-foreground">{{ account.type || '-' }}</td>
                  <td class="py-2 pr-3 text-muted-foreground">{{ account.status || '-' }}</td>
                  <td class="py-2 pr-3 text-muted-foreground">{{ numberOrDash(account.priority) }}</td>
                  <td class="py-2 pr-3 text-muted-foreground">{{ numberOrDash(account.concurrency) }}</td>
                  <!-- sub2api 没有 weight，展示 "-"，不把 priority 假装成 weight。 -->
                  <td class="py-2 pr-3 text-muted-foreground">{{ isNewAPI ? numberOrDash(account.weight) : '-' }}</td>
                  <td class="max-w-[14rem] py-2 pr-3 text-muted-foreground">
                    <span v-if="account.models" class="line-clamp-2 break-words">{{ account.models }}</span>
                    <span v-else>-</span>
                  </td>
                  <td class="py-2 pr-3">
                    <!-- 不可探活：展示明确原因，不展示会失败的探活按钮。 -->
                    <Tooltip v-if="!account.probeAvailable" :text="reasonLabel(account.probeUnavailableReason)" wide>
                      <span class="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 font-medium text-amber-600 dark:text-amber-400">
                        <ShieldAlert class="h-3 w-3" />
                        {{ reasonLabel(account.probeUnavailableReason) }}
                      </span>
                    </Tooltip>
                    <span
                      v-else-if="aggregateState(account) === ''"
                      class="inline-flex items-center rounded-full bg-zinc-500/10 px-2 py-0.5 font-medium text-zinc-500 dark:text-zinc-400"
                    >
                      {{ t(`${prefix}.notProbed`) }}
                    </span>
                    <span
                      v-else
                      class="inline-flex items-center rounded-full px-2 py-0.5 font-medium"
                      :class="connectionHealthStateBadgeClass(aggregateState(account))"
                    >
                      {{ stateLabel(aggregateState(account)) }}
                      <span v-if="account.modelHealth.length > 1" class="ml-1 opacity-70">×{{ account.modelHealth.length }}</span>
                    </span>
                  </td>
                  <td class="py-2 pr-3">
                    <Tooltip v-if="account.hasAssignedPolicy" :text="(account.assignedPolicies ?? []).map((p) => p.policyName).join('、')" wide>
                      <span class="inline-flex items-center gap-1 rounded-full bg-green-500/10 px-2 py-0.5 font-medium text-green-600 dark:text-green-400">
                        <ShieldCheck class="h-3 w-3" />
                        {{ assignedPolicyLabel(account) }}
                      </span>
                    </Tooltip>
                    <Tooltip v-else :text="t(`${dialogPrefix}.unassignedPolicyHint`)" wide>
                      <span class="inline-flex items-center gap-1 rounded-full bg-zinc-500/10 px-2 py-0.5 font-medium text-zinc-500 dark:text-zinc-400">
                        <ShieldQuestion class="h-3 w-3" />
                        {{ t(`${dialogPrefix}.unassignedPolicy`) }}
                      </span>
                    </Tooltip>
                  </td>
                  <td class="py-2 pr-0">
                    <div class="flex items-center justify-end gap-1">
                      <template v-if="account.probeAvailable">
                        <Tooltip :text="t(`${prefix}.actions.probe`)">
                          <button
                            type="button"
                            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface-line hover:text-primary"
                            @click="emit('probe', account)"
                          >
                            <Zap class="h-4 w-4" />
                          </button>
                        </Tooltip>
                        <Tooltip :text="t(`${prefix}.actions.viewEvents`)">
                          <button
                            type="button"
                            class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface-line hover:text-foreground"
                            @click="emit('view-events', account)"
                          >
                            <Eye class="h-4 w-4" />
                          </button>
                        </Tooltip>
                      </template>
                      <!-- 不可探活目标不显示手动探活/查看事件按钮，但分配策略入口始终可用，与
                           probeAvailable 完全解耦。 -->
                      <span v-else class="text-[11px] text-muted-foreground">{{ t(`${dialogPrefix}.unprobeable`) }}</span>
                      <Tooltip :text="t(`${dialogPrefix}.assignPolicy`)">
                        <button
                          type="button"
                          class="rounded-md p-1.5 text-muted-foreground transition-colors hover:bg-surface-line hover:text-foreground"
                          @click="emit('assign-policy', account)"
                        >
                          <ShieldCheck class="h-4 w-4" />
                        </button>
                      </Tooltip>
                    </div>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>
