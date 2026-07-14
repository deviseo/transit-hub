<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  Activity,
  AlertTriangle,
  Ban,
  BookOpenText,
  CheckCircle2,
  ChevronRight,
  Gauge,
  Layers,
  Loader2,
  PauseCircle,
  RefreshCw,
  Settings2,
} from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import {
  connectionHealthStateBadgeClass,
  formatConnectionHealthTime,
  useConnectionHealth,
} from '../composables/useConnectionHealth'
import { listUpstreamSites } from '../api/upstream'
import PolicyConfigDrawer from '../components/dashboard/PolicyConfigDrawer.vue'
import PolicyRunFlowDialog from '../components/dashboard/PolicyRunFlowDialog.vue'
import type { OwnGroupOption } from '../components/dashboard/PolicyConfigDrawer.vue'
import ManualOneTimeProbeDialog from '../components/dashboard/ManualOneTimeProbeDialog.vue'
import type { ManualProbeTargetSummary } from '../components/dashboard/ManualOneTimeProbeDialog.vue'
import TargetPolicyAssignmentDialog from '../components/dashboard/TargetPolicyAssignmentDialog.vue'
import ProbePolicyListDialog from '../components/dashboard/ProbePolicyListDialog.vue'
import ConnectionHealthEventsDialog from '../components/dashboard/ConnectionHealthEventsDialog.vue'
import AdminGroupAccountsDialog from '../components/dashboard/AdminGroupAccountsDialog.vue'
import type {
  AdminGroupAccount,
  AdminGroupHealth,
  ConnectionHealthPolicy,
  PolicyInput,
} from '../types/connectionHealth'

const { t } = useI18n()
const {
  overview,
  groups,
  adminGroups,
  events,
  policies,
  isLoading,
  isActionLoading,
  errorKey,
  loadAll,
  loadEvents,
  loadPolicies,
  savePolicy,
} = useConnectionHealth()

const siteNameMap = ref<Map<string, string>>(new Map())
const selectedType = ref<string>('')
const selectedPlatform = ref<string>('')
const searchText = ref<string>('')
const selectedConnectionId = ref<string>('')

const loadSiteNames = async () => {
  try {
    const sites = await listUpstreamSites()
    siteNameMap.value = new Map(sites.map((site) => [site.id, site.name]))
  } catch {
    // 站点名称仅用于事件弹窗展示，拉取失败时退化为展示站点 ID，不阻塞主流程。
  }
}

onMounted(() => {
  void loadAll()
  void loadEvents()
  void loadSiteNames()
  void loadPolicies()
})

// 策略配置抽屉：新建/编辑复用同一个 PolicyConfigDrawer。
const policyDrawerOpen = ref(false)
const editingPolicy = ref<ConnectionHealthPolicy | null>(null)
const runFlowDialogOpen = ref(false)

// 策略生效范围下拉仍使用「我的分组」维度（策略语义未变），从旧的 groups 数据推导。
const ownGroupOptions = computed<OwnGroupOption[]>(() =>
  groups.value.map((g) => ({ id: g.ownGroupId, name: g.ownGroupName || g.ownGroupId }))
)

const openCreatePolicy = () => {
  editingPolicy.value = null
  policyDrawerOpen.value = true
}

const openEditPolicy = (policy: ConnectionHealthPolicy) => {
  editingPolicy.value = policy
  policyDrawerOpen.value = true
}

const handleSavePolicy = async (input: PolicyInput) => {
  const ok = await savePolicy(input)
  if (ok) {
    policyDrawerOpen.value = false
  }
}

const togglePolicyEnabled = async (policy: ConnectionHealthPolicy) => {
  await savePolicy({
    id: policy.id,
    name: policy.name,
    enabled: !policy.enabled,
    ownGroupId: policy.ownGroupId,
    ownGroupName: policy.ownGroupName,
    probeIntervalSeconds: policy.probeIntervalSeconds,
    failureThreshold: policy.failureThreshold,
    successThreshold: policy.successThreshold,
    cooldownSeconds: policy.cooldownSeconds,
    observationSeconds: policy.observationSeconds,
    recoveryStepPercent: policy.recoveryStepPercent,
    dailyProbeBudget: policy.dailyProbeBudget,
    autoDegradeEnabled: policy.autoDegradeEnabled,
    autoRemoteActionEnabled: policy.autoRemoteActionEnabled,
    modelTargets: policy.modelTargets.map((m) => ({
      id: m.id, modelName: m.modelName, providerFamily: m.providerFamily,
      enabled: m.enabled, probePrompt: m.probePrompt, maxProbeTokens: m.maxProbeTokens,
    })),
  })
}

const siteName = (siteId: string): string => siteNameMap.value.get(siteId) ?? siteId

// 主列表筛选：按平台 / 类型 / 名称搜索，仅前端展示过滤，不改变后端聚合语义。
const platformOptions = computed(() => {
  const set = new Set<string>()
  for (const g of adminGroups.value) {
    if (g.platform) set.add(g.platform)
  }
  return Array.from(set)
})

const typeOptions = ['public', 'exclusive', 'subscription']

const groupTypeLabel = (type: string): string => {
  switch (type) {
    case 'exclusive':
      return t('admin.connectionHealth.groupTypes.exclusive')
    case 'subscription':
      return t('admin.connectionHealth.groupTypes.subscription')
    default:
      return t('admin.connectionHealth.groupTypes.public')
  }
}

const groupTypeBadgeClass = (type: string): string => {
  switch (type) {
    case 'exclusive':
      return 'bg-violet-500/10 text-violet-600 dark:text-violet-400'
    case 'subscription':
      return 'bg-blue-500/10 text-blue-600 dark:text-blue-400'
    default:
      return 'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400'
  }
}

const groupStatusLabel = (status: string): string => {
  switch (status) {
    case 'active':
    case '1':
      return t('admin.connectionHealth.groupStatusLabels.active')
    case 'inactive':
    case '0':
      return t('admin.connectionHealth.groupStatusLabels.inactive')
    default:
      return t('admin.connectionHealth.accountsDialog.unknownStatus')
  }
}

const filteredAdminGroups = computed(() => {
  const keyword = searchText.value.trim().toLowerCase()
  return adminGroups.value.filter((g) => {
    if (selectedType.value && g.type !== selectedType.value) return false
    if (selectedPlatform.value && g.platform !== selectedPlatform.value) return false
    if (keyword && !g.name.toLowerCase().includes(keyword)) return false
    return true
  })
})

const stateLabel = (state: string): string => t(`admin.connectionHealth.stateLabels.${state}`)
const stateBadgeClass = connectionHealthStateBadgeClass
const formatTime = formatConnectionHealthTime

// 顶部两个独立弹窗：策略列表、探活事件。语义保持不变。
const policyListDialogOpen = ref(false)
const eventsDialogOpen = ref(false)

const openPolicyListDialog = () => {
  policyListDialogOpen.value = true
}

const openEventsDialog = async () => {
  selectedConnectionId.value = ''
  await loadEvents()
  eventsDialogOpen.value = true
}

const showAllEvents = async () => {
  selectedConnectionId.value = ''
  await loadEvents()
}

// selectTargetEvents：查看某个探活目标的事件详情。后端 Events 接口按同一 query 参数同时支持
// 旧 connectionId 与新 targetId（targetId 内嵌 workspace，做归属校验），故直接复用。
const selectTargetEvents = async (targetId: string) => {
  selectedConnectionId.value = targetId
  await loadEvents(targetId)
  eventsDialogOpen.value = true
}

// 账号/渠道弹窗状态。
const accountsDialogOpen = ref(false)
const selectedAdminGroup = ref<AdminGroupHealth | null>(null)

const openAccountsDialog = (group: AdminGroupHealth) => {
  selectedAdminGroup.value = group
  accountsDialogOpen.value = true
}

// 手动一次性探活弹窗状态：从账号弹窗某个账号/渠道触发，弹窗自己负责拉模型列表 + 发起测试 +
// 展示结果，本视图只需要传一份不含凭据的账号摘要。手动探活不落库、不影响探活状态徽标，
// 因此这里不再需要 loadAll/事件跳转之类的副作用。
const probeDialogOpen = ref(false)
const probeDialogTarget = ref<ManualProbeTargetSummary | null>(null)

// resyncSelectedGroup 在主列表数据刷新后，把账号弹窗持有的分组快照重新指向刷新后的同 id 分组，
// 避免弹窗展示过期的探活状态。
const resyncSelectedGroup = () => {
  if (!selectedAdminGroup.value) return
  const fresh = adminGroups.value.find((g) => g.id === selectedAdminGroup.value?.id)
  if (fresh) selectedAdminGroup.value = fresh
}

// onProbeAccount：账号弹窗里点击某个账号/渠道的「手动探活」。不再依赖策略候选池，
// 只把账号摘要（不含 base_url/key）交给弹窗，模型列表和探活结果都由弹窗自己向后端请求。
const onProbeAccount = (account: AdminGroupAccount) => {
  if (!account.probeAvailable) return
  probeDialogTarget.value = {
    targetId: account.targetId,
    accountName: account.name || account.id,
    platform: selectedAdminGroup.value?.platform || '',
    type: account.type,
    status: account.status,
    groupName: selectedAdminGroup.value?.name || '',
  }
  probeDialogOpen.value = true
}

const onViewEventsAccount = (account: AdminGroupAccount) => {
  if (!account.targetId) return
  void selectTargetEvents(account.targetId)
}

const closeProbeDialog = () => {
  probeDialogOpen.value = false
}

// 策略分配弹窗状态：与手动探活弹窗互相独立，分配保存后静默刷新主列表，让账号弹窗里的
// 分配状态、事件弹窗能否展示策略事件都随之更新。
const policyAssignmentDialogOpen = ref(false)
const policyAssignmentTargetId = ref('')
const policyAssignmentAccountName = ref('')

const onAssignPolicy = (account: AdminGroupAccount) => {
  policyAssignmentTargetId.value = account.targetId
  policyAssignmentAccountName.value = account.name || account.id
  policyAssignmentDialogOpen.value = true
}

const closePolicyAssignmentDialog = () => {
  policyAssignmentDialogOpen.value = false
}

const onPolicyAssignmentSaved = async () => {
  policyAssignmentDialogOpen.value = false
  await loadAll({ silent: true })
  resyncSelectedGroup()
}
</script>

<template>
  <div class="space-y-6">
    <div class="flex flex-wrap items-start justify-between gap-3">
      <div class="min-w-0">
        <h1 class="text-xl font-semibold text-foreground">{{ t('admin.connectionHealth.title') }}</h1>
        <p class="mt-1 text-sm text-muted-foreground">{{ t('admin.connectionHealth.adminSubtitle') }}</p>
      </div>
      <div class="flex flex-wrap items-center gap-2">
        <Button variant="secondary" size="sm" @click="runFlowDialogOpen = true">
          <BookOpenText class="h-4 w-4" />
          {{ t('admin.connectionHealth.topActions.runFlow') }}
        </Button>
        <Button variant="secondary" size="sm" @click="openPolicyListDialog">
          <Settings2 class="h-4 w-4" />
          {{ t('admin.connectionHealth.topActions.policies') }}
        </Button>
        <Button variant="secondary" size="sm" @click="openEventsDialog">
          <Activity class="h-4 w-4" />
          {{ t('admin.connectionHealth.topActions.events') }}
        </Button>
        <Button variant="secondary" size="sm" :disabled="isLoading" @click="loadAll">
          <Loader2 v-if="isLoading" class="h-4 w-4 animate-spin" />
          <RefreshCw v-else class="h-4 w-4" />
          {{ t('admin.connectionHealth.refresh') }}
        </Button>
      </div>
    </div>

    <!-- 顶部汇总卡片（探活链路维度，语义不变） -->
    <div class="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
      <div class="rounded-2xl border border-border/50 bg-card p-4 shadow-sm">
        <p class="text-xs font-medium text-muted-foreground">{{ t('admin.connectionHealth.summary.total') }}</p>
        <p class="mt-1 text-2xl font-bold text-foreground">{{ overview?.totalConnections ?? 0 }}</p>
      </div>
      <div class="rounded-2xl border border-border/50 bg-card p-4 shadow-sm">
        <p class="flex items-center gap-1 text-xs font-medium text-green-600 dark:text-green-400"><CheckCircle2 class="h-3.5 w-3.5" />{{ t('admin.connectionHealth.stateLabels.healthy') }}</p>
        <p class="mt-1 text-2xl font-bold text-foreground">{{ overview?.healthy ?? 0 }}</p>
      </div>
      <div class="rounded-2xl border border-border/50 bg-card p-4 shadow-sm">
        <p class="flex items-center gap-1 text-xs font-medium text-amber-600 dark:text-amber-400"><AlertTriangle class="h-3.5 w-3.5" />{{ t('admin.connectionHealth.stateLabels.degraded') }}</p>
        <p class="mt-1 text-2xl font-bold text-foreground">{{ overview?.degraded ?? 0 }}</p>
      </div>
      <div class="rounded-2xl border border-border/50 bg-card p-4 shadow-sm">
        <p class="flex items-center gap-1 text-xs font-medium text-red-600 dark:text-red-400"><Ban class="h-3.5 w-3.5" />{{ t('admin.connectionHealth.stateLabels.suspended') }}</p>
        <p class="mt-1 text-2xl font-bold text-foreground">{{ overview?.suspended ?? 0 }}</p>
      </div>
      <div class="rounded-2xl border border-border/50 bg-card p-4 shadow-sm">
        <p class="flex items-center gap-1 text-xs font-medium text-zinc-500 dark:text-zinc-400"><PauseCircle class="h-3.5 w-3.5" />{{ t('admin.connectionHealth.stateLabels.disabled') }}</p>
        <p class="mt-1 text-2xl font-bold text-foreground">{{ overview?.disabled ?? 0 }}</p>
      </div>
      <div class="rounded-2xl border border-border/50 bg-card p-4 shadow-sm">
        <p class="flex items-center gap-1 text-xs font-medium text-muted-foreground"><Gauge class="h-3.5 w-3.5" />{{ t('admin.connectionHealth.summary.unconfigured') }}</p>
        <p class="mt-1 text-2xl font-bold text-foreground">{{ overview?.unconfigured ?? 0 }}</p>
      </div>
    </div>

    <p v-if="errorKey" class="text-sm text-red-500">{{ t(errorKey) }}</p>

    <!-- 筛选栏 -->
    <div class="flex flex-wrap items-center gap-3 rounded-2xl border border-border/50 bg-card p-4 shadow-sm">
      <input
        v-model="searchText"
        type="text"
        :placeholder="t('admin.connectionHealth.filters.searchGroup')"
        class="h-9 min-w-[12rem] flex-1 rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground"
      />
      <select v-model="selectedPlatform" class="h-9 rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground">
        <option value="">{{ t('admin.connectionHealth.filters.allPlatforms') }}</option>
        <option v-for="p in platformOptions" :key="p" :value="p">{{ p }}</option>
      </select>
      <select v-model="selectedType" class="h-9 rounded-lg border border-border/60 bg-background px-3 text-sm text-foreground">
        <option value="">{{ t('admin.connectionHealth.filters.allTypes') }}</option>
        <option v-for="ty in typeOptions" :key="ty" :value="ty">{{ groupTypeLabel(ty) }}</option>
      </select>
    </div>

    <!-- 主表：admin 全量分组 -->
    <div class="rounded-2xl border border-border/50 bg-card shadow-sm">
      <div v-if="isLoading" class="flex items-center justify-center py-16">
        <Loader2 class="h-6 w-6 animate-spin text-primary/60" />
      </div>
      <div v-else-if="filteredAdminGroups.length === 0" class="flex flex-col items-center justify-center gap-2 py-16 text-center">
        <Layers class="h-8 w-8 text-muted-foreground/40" />
        <p class="text-sm text-muted-foreground">{{ t('admin.connectionHealth.adminEmpty') }}</p>
      </div>
      <div v-else class="overflow-x-auto">
        <table class="w-full text-sm">
          <thead>
            <tr class="border-b border-border/40 text-left text-xs text-muted-foreground">
              <th class="px-4 py-3 font-medium">{{ t('admin.connectionHealth.adminColumns.name') }}</th>
              <th class="px-4 py-3 font-medium">{{ t('admin.connectionHealth.adminColumns.platform') }}</th>
              <th class="px-4 py-3 font-medium">{{ t('admin.connectionHealth.adminColumns.type') }}</th>
              <th class="px-4 py-3 font-medium">{{ t('admin.connectionHealth.adminColumns.multiplier') }}</th>
              <th class="px-4 py-3 font-medium">{{ t('admin.connectionHealth.adminColumns.accounts') }}</th>
              <th class="px-4 py-3 font-medium">{{ t('admin.connectionHealth.adminColumns.status') }}</th>
              <th class="px-4 py-3 font-medium">{{ t('admin.connectionHealth.adminColumns.probeOverview') }}</th>
              <th class="px-4 py-3 text-right font-medium">{{ t('admin.connectionHealth.adminColumns.detail') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr
              v-for="group in filteredAdminGroups"
              :key="group.id"
              class="border-b border-border/20 transition-colors hover:bg-surface-line/40"
            >
              <td class="px-4 py-3 font-medium text-foreground">{{ group.name }}</td>
              <td class="px-4 py-3 text-muted-foreground">{{ group.platform || '-' }}</td>
              <td class="px-4 py-3">
                <span class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium" :class="groupTypeBadgeClass(group.type)">
                  {{ groupTypeLabel(group.type) }}
                </span>
              </td>
              <td class="px-4 py-3 text-foreground">{{ group.multiplierDisplay || '-' }}</td>
              <td class="px-4 py-3">
                <button
                  type="button"
                  class="inline-flex items-center gap-1 rounded-md px-2 py-0.5 text-sm font-medium text-primary transition-colors hover:bg-primary/10"
                  @click="openAccountsDialog(group)"
                >
                  {{ group.accountCount }}
                  <span class="text-xs text-muted-foreground">{{ t('admin.connectionHealth.adminColumns.accountsUnit') }}</span>
                </button>
              </td>
              <td class="px-4 py-3">
                <span
                  class="inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium"
                  :class="group.status === 'active' || group.status === '1'
                    ? 'bg-green-500/10 text-green-600 dark:text-green-400'
                    : 'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400'"
                >
                  {{ groupStatusLabel(group.status) }}
                </span>
              </td>
              <td class="px-4 py-3">
                <!-- 分组账号列表加载失败：只在该行提示，不影响其它分组。 -->
                <span v-if="group.accountsError" class="text-xs text-red-500">{{ t(group.accountsError) }}</span>
                <!-- 没有任何 real_connection 匹配：展示未对接，不显示误导性可用率。 -->
                <span
                  v-else-if="group.healthSummary.probeableAccounts === 0"
                  class="inline-flex items-center rounded-full bg-zinc-500/10 px-2 py-0.5 text-xs font-medium text-zinc-500 dark:text-zinc-400"
                >
                  {{ t('admin.connectionHealth.adminOverview.noneProbeable') }}
                </span>
                <div v-else class="flex flex-wrap items-center gap-1.5 text-xs">
                  <span class="text-muted-foreground">
                    {{ t('admin.connectionHealth.adminOverview.probeable', { probeable: group.healthSummary.probeableAccounts, total: group.healthSummary.totalAccounts }) }}
                  </span>
                  <span v-if="group.healthSummary.healthyModels > 0" class="inline-flex items-center rounded-full px-1.5 py-0.5 font-medium" :class="stateBadgeClass('healthy')">{{ group.healthSummary.healthyModels }}</span>
                  <span v-if="group.healthSummary.degradedModels > 0" class="inline-flex items-center rounded-full px-1.5 py-0.5 font-medium" :class="stateBadgeClass('degraded')">{{ group.healthSummary.degradedModels }}</span>
                  <span v-if="group.healthSummary.suspendedModels > 0" class="inline-flex items-center rounded-full px-1.5 py-0.5 font-medium" :class="stateBadgeClass('suspended')">{{ group.healthSummary.suspendedModels }}</span>
                  <span v-if="group.healthSummary.disabledModels > 0" class="inline-flex items-center rounded-full px-1.5 py-0.5 font-medium" :class="stateBadgeClass('disabled')">{{ group.healthSummary.disabledModels }}</span>
                  <span
                    v-if="group.healthSummary.unconfiguredModels > 0"
                    class="inline-flex items-center rounded-full bg-zinc-500/10 px-1.5 py-0.5 font-medium text-zinc-500 dark:text-zinc-400"
                  >
                    {{ t('admin.connectionHealth.adminOverview.noProbe', { count: group.healthSummary.unconfiguredModels }) }}
                  </span>
                </div>
              </td>
              <td class="px-4 py-3 text-right">
                <button
                  type="button"
                  class="inline-flex items-center gap-0.5 rounded-md px-2 py-1 text-xs font-medium text-primary transition-colors hover:bg-primary/10"
                  @click="openAccountsDialog(group)"
                >
                  {{ t('admin.connectionHealth.adminColumns.detail') }}
                  <ChevronRight class="h-3.5 w-3.5" />
                </button>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>

    <PolicyConfigDrawer
      :open="policyDrawerOpen"
      :policy="editingPolicy"
      :own-group-options="ownGroupOptions"
      @close="policyDrawerOpen = false"
      @save="handleSavePolicy"
    />

    <PolicyRunFlowDialog
      :open="runFlowDialogOpen"
      @close="runFlowDialogOpen = false"
    />

    <AdminGroupAccountsDialog
      :open="accountsDialogOpen"
      :group="selectedAdminGroup"
      @close="accountsDialogOpen = false"
      @probe="onProbeAccount"
      @view-events="onViewEventsAccount"
      @assign-policy="onAssignPolicy"
    />

    <ManualOneTimeProbeDialog
      :open="probeDialogOpen"
      :target="probeDialogTarget"
      @close="closeProbeDialog"
    />

    <TargetPolicyAssignmentDialog
      :open="policyAssignmentDialogOpen"
      :target-id="policyAssignmentTargetId"
      :account-name="policyAssignmentAccountName"
      :policies="policies"
      @close="closePolicyAssignmentDialog"
      @saved="onPolicyAssignmentSaved"
    />

    <ProbePolicyListDialog
      :open="policyListDialogOpen"
      :policies="policies"
      @close="policyListDialogOpen = false"
      @create="openCreatePolicy"
      @edit="openEditPolicy"
      @toggle="togglePolicyEnabled"
    />

    <ConnectionHealthEventsDialog
      :open="eventsDialogOpen"
      :events="events"
      :groups="groups"
      :policies="policies"
      :selected-connection-id="selectedConnectionId"
      :site-name="siteName"
      @close="eventsDialogOpen = false"
      @view-all="showAllEvents"
    />
  </div>
</template>
