import { ref } from 'vue'
import type {
  AdminGroupHealth,
  ConnectionHealthEvent,
  ConnectionHealthOverview,
  ConnectionHealthPolicy,
  ConnectionHealthState,
  ManualProbeModelOption,
  ManualProbeResult,
  ModelHealth,
  OwnGroupHealth,
  PolicyInput,
  ProbeModelCandidate,
  TargetPolicyAssignments,
} from '../types/connectionHealth'
import type { AdminGroupAccount } from '../types/connectionHealth'
import {
  createConnectionHealthPolicy,
  disableConnection,
  discoverTargetModels,
  getConnectionHealthAdminGroups,
  getConnectionHealthEvents,
  getConnectionHealthGroups,
  getConnectionHealthOverview,
  getTargetPolicyAssignments,
  listConnectionHealthPolicies,
  manualProbeOnce,
  probeConnection,
  probeTarget,
  restoreConnection,
  setTargetPolicyAssignments,
  updateConnectionHealthPolicy,
} from '../api/connectionHealth'

const overview = ref<ConnectionHealthOverview | null>(null)
const groups = ref<OwnGroupHealth[]>([])
// adminGroups 是新的主列表数据源：当前 admin workspace 下的 admin 全量分组。
// 与旧的 groups（我的分组链路）并存，供改造后的 ConnectionHealthView 主列表使用。
const adminGroups = ref<AdminGroupHealth[]>([])
const events = ref<ConnectionHealthEvent[]>([])
const policies = ref<ConnectionHealthPolicy[]>([])
const isLoading = ref(false)
const isActionLoading = ref(false)
const errorKey = ref('')

export function useConnectionHealth() {
  const loadOverview = async () => {
    try {
      overview.value = await getConnectionHealthOverview()
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
    }
  }

  // silent=true 时跳过 isLoading 切换：用于手动探活等已经在链路级别自带 loading 反馈的
  // 场景下后台刷新主列表数据，避免主列表出现整页 loading 空白/重绘。
  const loadGroups = async (opts: { silent?: boolean } = {}) => {
    if (!opts.silent) isLoading.value = true
    errorKey.value = ''
    try {
      groups.value = await getConnectionHealthGroups()
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
    } finally {
      if (!opts.silent) isLoading.value = false
    }
  }

  // loadAdminGroups 载入新的主列表数据源（admin 全量分组）。silent 语义同 loadGroups。
  const loadAdminGroups = async (opts: { silent?: boolean } = {}) => {
    if (!opts.silent) isLoading.value = true
    errorKey.value = ''
    try {
      adminGroups.value = await getConnectionHealthAdminGroups()
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
    } finally {
      if (!opts.silent) isLoading.value = false
    }
  }

  // loadAll 同时刷新概览、新的 admin 分组主列表，以及旧的我的分组链路数据。
  // 旧的 groups 仍需加载：探活事件弹窗按 connectionId 关联链路上下文、手动探活候选模型按
  // 链路所属 own group 匹配策略，都依赖这份数据；主列表展示已切换到 adminGroups。
  const loadAll = async (opts: { silent?: boolean } = {}) => {
    await Promise.all([loadOverview(), loadAdminGroups(opts), loadGroups({ silent: true })])
  }

  const loadEvents = async (connectionId?: string) => {
    try {
      events.value = await getConnectionHealthEvents(connectionId)
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
    }
  }

  const loadPolicies = async () => {
    try {
      policies.value = await listConnectionHealthPolicies()
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
    }
  }

  const savePolicy = async (input: PolicyInput) => {
    errorKey.value = ''
    try {
      if (input.id) {
        await updateConnectionHealthPolicy(input.id, input)
      } else {
        await createConnectionHealthPolicy(input)
      }
      await loadPolicies()
      return true
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
      return false
    }
  }

  // manualProbe 触发一次手动探活。返回值区分三种情况供调用方展示不同反馈：
  // - null：请求失败（含"所选模型未匹配当前链路策略"等业务错误），errorKey 已同步设置。
  // - []：探活接口成功执行但没有产出任何结果（例如探活过程中逐个模型请求异常），
  //   调用方应提示"探活完成但结果为空"，不能等同于"没有匹配模型"。
  // - ModelHealth[]（非空）：正常探活结果。
  // 故意不在这里 loadAll()：手动探活是高频的链路级操作，如果每次都强制刷新并 loading 整个
  // 主列表，用户会感觉"整页刷新"。数据刷新交给调用方（ConnectionHealthView）按需做：
  // 刷新当前链路事件 + 用 silent 选项后台刷新 groups/overview。
  const manualProbe = async (connectionId: string, models?: string[]): Promise<ModelHealth[] | null> => {
    isActionLoading.value = true
    errorKey.value = ''
    try {
      return await probeConnection(connectionId, models)
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
      return null
    } finally {
      isActionLoading.value = false
    }
  }

  // manualProbeTarget 触发一次独立目标手动探活（targetId 维度）。返回值语义同 manualProbe：
  // null=失败（errorKey 已设置，含 credential_unavailable 等结构化不可探活错误）、
  // []=执行成功但无结果、非空=正常结果。
  const manualProbeTarget = async (targetId: string, models?: string[]): Promise<ModelHealth[] | null> => {
    isActionLoading.value = true
    errorKey.value = ''
    try {
      return await probeTarget(targetId, models)
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
      return null
    } finally {
      isActionLoading.value = false
    }
  }

  // discoverModels / manualProbeOnce 服务于新的手动一次性探活弹窗。这两个动作是弹窗自身的
  // 一次性交互，不影响主列表的探活状态徽标，因此不复用 isActionLoading/errorKey 这两个
  // 面向主列表操作的共享状态——弹窗组件自己持有 loading/error 展示，避免多个弹窗实例之间
  // 或与主列表操作互相污染 loading/错误提示。
  const discoverModels = async (targetId: string): Promise<{ models: ManualProbeModelOption[] } | { errorKey: string }> => {
    try {
      return { models: await discoverTargetModels(targetId) }
    } catch (err) {
      return { errorKey: err instanceof Error ? err.message : 'admin.connectionHealth.errors.request' }
    }
  }

  const runManualProbeOnce = async (targetId: string, models: string[]): Promise<{ results: ManualProbeResult[] } | { errorKey: string }> => {
    try {
      return { results: await manualProbeOnce(targetId, models) }
    } catch (err) {
      return { errorKey: err instanceof Error ? err.message : 'admin.connectionHealth.errors.request' }
    }
  }

  const loadTargetPolicyAssignments = async (targetId: string): Promise<{ assignments: TargetPolicyAssignments } | { errorKey: string }> => {
    try {
      return { assignments: await getTargetPolicyAssignments(targetId) }
    } catch (err) {
      return { errorKey: err instanceof Error ? err.message : 'admin.connectionHealth.errors.request' }
    }
  }

  const saveTargetPolicyAssignments = async (targetId: string, policyIds: string[]): Promise<{ assignments: TargetPolicyAssignments } | { errorKey: string }> => {
    try {
      return { assignments: await setTargetPolicyAssignments(targetId, policyIds) }
    } catch (err) {
      return { errorKey: err instanceof Error ? err.message : 'admin.connectionHealth.errors.request' }
    }
  }

  const disable = async (connectionId: string) => {
    isActionLoading.value = true
    errorKey.value = ''
    try {
      await disableConnection(connectionId)
      await loadAll()
      return true
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
      return false
    } finally {
      isActionLoading.value = false
    }
  }

  const restore = async (connectionId: string) => {
    isActionLoading.value = true
    errorKey.value = ''
    try {
      await restoreConnection(connectionId)
      await loadAll()
      return true
    } catch (err) {
      errorKey.value = err instanceof Error ? err.message : 'admin.connectionHealth.errors.request'
      return false
    } finally {
      isActionLoading.value = false
    }
  }

  return {
    overview,
    groups,
    adminGroups,
    events,
    policies,
    isLoading,
    isActionLoading,
    errorKey,
    loadAll,
    loadOverview,
    loadGroups,
    loadAdminGroups,
    loadEvents,
    loadPolicies,
    savePolicy,
    manualProbe,
    manualProbeTarget,
    discoverModels,
    runManualProbeOnce,
    loadTargetPolicyAssignments,
    saveTargetPolicyAssignments,
    disable,
    restore,
  }
}

// adminTargetProbeCandidates 推导一个独立探活目标（账号/渠道）可手动探活的候选模型：
// 与后端 candidateModelSpecs 语义一致——策略池取当前 workspace 全部启用策略下的启用 modelTargets；
// 目标自带模型列表（account.models）非空时取「目标模型 ∩ 策略池」，否则用整个策略池。
// 保证前端展示的候选与后端实际会探活的模型一致，避免用户勾选到后端会拒绝的模型。
export function adminTargetProbeCandidates(
  account: AdminGroupAccount,
  policies: ConnectionHealthPolicy[],
): ProbeModelCandidate[] {
  const pool = new Map<string, ProbeModelCandidate>()
  for (const policy of policies) {
    if (!policy.enabled) continue
    for (const target of policy.modelTargets) {
      if (!target.enabled) continue
      if (pool.has(target.modelName)) continue
      pool.set(target.modelName, {
        modelName: target.modelName,
        providerFamily: target.providerFamily,
        policyId: policy.id,
        policyName: policy.name,
        autoRemoteActionEnabled: policy.autoRemoteActionEnabled,
        maxProbeTokens: target.maxProbeTokens,
      })
    }
  }

  const targetModels = (account.models ?? '')
    .split(',')
    .map((m) => m.trim())
    .filter((m) => m.length > 0)

  if (targetModels.length === 0) {
    return Array.from(pool.values())
  }
  const allowed = new Set(targetModels)
  return Array.from(pool.values()).filter((c) => allowed.has(c.modelName))
}

// matchingProbeCandidates 推导某条对接链路可手动探活的候选模型：按该链路所属的 own group
// （ownGroupId）匹配当前 workspace 下已启用策略的已启用 modelTargets——策略 ownGroupId 为空
// 表示匹配全部已对接分组链路。选择"前端基于 policies + groups 推导"而不是让后端在 Groups
// 响应里额外装载匹配摘要，是因为 policies 本身已经是前端已有的、职责单一的数据源，无需为了
// 这一个交互改动后端聚合接口的返回形态（更稳定、改动面更小）。
// 同一模型名可能被多条匹配策略重复收录（例如全局策略 + 该分组的专属策略都配置了同一模型），
// 这里按 modelName 去重，保留先匹配到的策略元信息，避免探活弹窗里出现重复行。
export function matchingProbeCandidates(ownGroupId: string, policies: ConnectionHealthPolicy[]): ProbeModelCandidate[] {
  const seen = new Set<string>()
  const candidates: ProbeModelCandidate[] = []
  for (const policy of policies) {
    if (!policy.enabled) continue
    if (policy.ownGroupId && policy.ownGroupId !== ownGroupId) continue
    for (const target of policy.modelTargets) {
      if (!target.enabled) continue
      if (seen.has(target.modelName)) continue
      seen.add(target.modelName)
      candidates.push({
        modelName: target.modelName,
        providerFamily: target.providerFamily,
        policyId: policy.id,
        policyName: policy.name,
        autoRemoteActionEnabled: policy.autoRemoteActionEnabled,
        maxProbeTokens: target.maxProbeTokens,
      })
    }
  }
  return candidates
}

// formatConnectionHealthTime 是分组健康页面/弹窗共用的时间展示格式化：非法或缺失日期统一
// 展示为 em dash，避免每个组件各自处理无效日期分支。
export function formatConnectionHealthTime(iso: string | null): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  return date.toLocaleString()
}

// connectionHealthStateBadgeClass 是分组健康状态徽标的颜色映射，主列表和事件弹窗状态卡片
// 共用同一套配色，避免两处各自维护一份容易产生视觉不一致。
export function connectionHealthStateBadgeClass(state: ConnectionHealthState | string): string {
  switch (state) {
    case 'healthy':
      return 'bg-green-500/10 text-green-600 dark:text-green-400'
    case 'degraded':
      return 'bg-amber-500/10 text-amber-600 dark:text-amber-400'
    case 'suspended':
      return 'bg-red-500/10 text-red-600 dark:text-red-400'
    case 'observing':
      return 'bg-blue-500/10 text-blue-600 dark:text-blue-400'
    case 'recovering':
      return 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400'
    case 'disabled':
      return 'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400'
    default:
      return 'bg-zinc-500/10 text-zinc-500 dark:text-zinc-400'
  }
}

// connectionHealthRecordColorClass 是事件弹窗"近 60 次记录条"的着色规则：ok 绿色，网络类
// 波动/限流/响应解析失败琥珀色，服务端/鉴权/模型类错误红色，人工禁用/恢复中性蓝色，
// 不支持场景灰色。主视图和链路详情卡片共用同一份映射，避免颜色定义分散、后续不一致。
const RECORD_COLOR_CLASS: Record<string, string> = {
  ok: 'bg-green-500',
  network_fluctuation: 'bg-amber-500',
  rate_limited: 'bg-amber-500',
  invalid_response: 'bg-amber-500',
  server_error: 'bg-red-500',
  auth: 'bg-red-500',
  model_not_found: 'bg-red-500',
  manual_disable: 'bg-blue-500',
  manual_restore: 'bg-blue-500',
  unsupported: 'bg-zinc-400',
}

export function connectionHealthRecordColorClass(result: string): string {
  return RECORD_COLOR_CLASS[result] ?? 'bg-zinc-400'
}

// matchingProbeIntervalSeconds 推导某条链路上某个模型当前生效的探活间隔：匹配规则与
// matchingProbeCandidates 一致（按 ownGroupId 精确匹配优先于全局策略），用于事件弹窗状态卡片
// 展示"下次刷新倒计时"。找不到匹配的启用策略时返回 null，调用方应回退展示绝对时间而不是
// 编造一个刷新间隔。
export function matchingProbeIntervalSeconds(
  ownGroupId: string,
  modelName: string,
  policies: ConnectionHealthPolicy[],
): number | null {
  let matched: ConnectionHealthPolicy | null = null
  for (const policy of policies) {
    if (!policy.enabled) continue
    if (policy.ownGroupId && policy.ownGroupId !== ownGroupId) continue
    const hasModel = policy.modelTargets.some((target) => target.enabled && target.modelName === modelName)
    if (!hasModel) continue
    // 分组专属策略优先于全局策略（ownGroupId 为空表示全局）。
    if (!matched || (policy.ownGroupId && !matched.ownGroupId)) matched = policy
  }
  return matched ? matched.probeIntervalSeconds : null
}

// remoteActionLabelKey 把后端记录的 remoteAction 原始字符串（见 backend connection_health/
// actions.go 的 RemoteAction* 常量）映射为 i18n key + 插值参数，供事件弹窗展示"这次探活触发
// 的远端动作是什么"。返回 null 表示这次探活没有触发任何远端动作（remoteAction 为空），
// 调用方此时不应渲染这一行，避免每张卡片都显示一行空文案。
// 刻意做成不依赖 useI18n() 的纯函数（返回 key 而不是已翻译文案），因为这个模块内的其它
// 展示型工具函数（connectionHealthStateBadgeClass 等）都是同样的纯函数风格，调用方在
// 组件模板里自己执行 t(key, params)。
export function remoteActionLabelKey(remoteAction: string): { key: string; params?: Record<string, number | string> } | null {
  if (!remoteAction) return null
  const prefix = 'admin.connectionHealth.remoteActions'
  switch (remoteAction) {
    case 'unsupported':
      return { key: `${prefix}.unsupported` }
    case 'skipped_independent_probe':
      return { key: `${prefix}.skippedIndependentProbe` }
    case 'sub2api_account_status_inactive':
      return { key: `${prefix}.sub2apiInactive` }
    case 'sub2api_account_status_active':
      return { key: `${prefix}.sub2apiActive` }
    case 'sub2api_account_status_inactive_failed':
      return { key: `${prefix}.sub2apiInactiveFailed` }
    case 'sub2api_account_status_active_failed':
      return { key: `${prefix}.sub2apiActiveFailed` }
    case 'newapi_channel_disabled':
      return { key: `${prefix}.newapiDisabled` }
  }
  const weightMatch = /^newapi_channel_weight_(\d+)$/.exec(remoteAction)
  if (weightMatch) {
    return { key: `${prefix}.newapiWeight`, params: { weight: Number(weightMatch[1]) } }
  }
  // 未识别的取值（理论上不应出现，但不要因为一个新常量没有及时加 i18n 映射就隐藏信息）
  // 原样透出，让用户至少能看到后端记录的原始动作字符串。
  return { key: `${prefix}.other`, params: { action: remoteAction } }
}
