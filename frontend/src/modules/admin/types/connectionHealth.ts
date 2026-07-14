// 与后端 backend/internal/modules/connection_health 的 JSON 响应字段一一对应。
// 所有类型都不含 upstream key 明文字段。

export type ConnectionHealthState =
  | 'healthy'
  | 'degraded'
  | 'suspended'
  | 'observing'
  | 'recovering'
  | 'disabled'

export interface ModelHealth {
  modelName: string
  providerFamily: string
  configured: boolean
  state: ConnectionHealthState
  currentWeight: number
  consecutiveFailures: number
  consecutiveSuccesses: number
  lastProbeAt: string | null
  lastSuccessAt: string | null
  lastFailureAt: string | null
  lastLatencyMs: number | null
  lastErrorKey: string
  lastErrorDetail: string
  lastRemoteAction: string
  updatedAt: string | null
}

export interface ConnectionHealth {
  connectionId: string
  upstreamSiteId: string
  upstreamGroupId: string
  upstreamGroupName: string
  upstreamKeyId: string
  groupType: string
  models: ModelHealth[]
}

export interface OwnGroupHealth {
  ownGroupId: string
  ownGroupName: string
  hasConnections: boolean
  connections: ConnectionHealth[]
}

// AdminGroupHealth 系列类型对应后端 GET /api/connection-health/admin-groups 的响应：
// 「当前 admin workspace 下的 admin 全量分组 -> 分组下账号/渠道（独立探活目标）-> 独立探活状态叠加」。
// 探活体系已改为独立目标：账号/渠道本身就是探活目标，不再依赖 real_connections 对接链路。
// 所有字段都不含 upstream key / token / cookie / credentials 明文。
export type AdminGroupType = 'public' | 'exclusive' | 'subscription'

// AdminProbeUnavailableReason 是账号/渠道不可探活的原因枚举（后端脱敏后透出），
// 前端据此展示明确文案，绝不携带任何密钥/上游报文。
export type AdminProbeUnavailableReason =
  | 'credential_unavailable'
  | 'secure_verification_required'
  | 'base_url_unavailable'
  | 'model_unavailable'
  | 'export_unavailable'
  | 'credentials_redacted'

export interface AdminGroupHealthSummary {
  totalAccounts: number
  probeableAccounts: number
  unprobeableAccounts: number
  healthyModels: number
  degradedModels: number
  suspendedModels: number
  disabledModels: number
  unconfiguredModels: number
  lastProbeAt: string | null
}

// TargetPolicyAssignmentSummary 是某个账号/渠道已分配策略的展示摘要，不含任何敏感字段。
export interface TargetPolicyAssignmentSummary {
  policyId: string
  policyName: string
  enabled: boolean
}

// TargetPolicyAssignments 是策略分配管理接口 GET/PUT 的响应体。
export interface TargetPolicyAssignments {
  policyIds: string[]
  policies: TargetPolicyAssignmentSummary[]
}

export interface AdminGroupAccount {
  id: string
  name: string
  platform: string
  type: string
  status: string
  schedulable?: boolean
  priority?: number
  concurrency?: number
  rateMultiplier?: number
  loadFactor?: number
  weight?: number
  models?: string
  groupIds?: string[]
  // 独立探活字段：targetId 是稳定探活目标 ID，手动探活/事件按 targetId 走。
  targetId: string
  probeAvailable: boolean
  probeUnavailableReason?: AdminProbeUnavailableReason | string
  modelHealth: ModelHealth[]
  // 策略分配字段：与 probeAvailable 完全解耦——未分配策略仍可手动一次性探活，只是不会被
  // 调度器自动探活。旧后端响应不带这些字段时前端按「未分配」兜底展示，不强制要求存在。
  assignedPolicyIds?: string[]
  assignedPolicies?: TargetPolicyAssignmentSummary[]
  hasAssignedPolicy?: boolean
}

export interface AdminGroupHealth {
  id: string
  name: string
  platform: string
  status: string
  type: AdminGroupType | string
  isExclusive: boolean
  subscriptionType: string
  multiplier: number | null
  multiplierDisplay: string
  accountCount: number
  healthSummary: AdminGroupHealthSummary
  // accountsError 非空（i18n key）表示该分组账号列表加载失败，其余分组不受影响。
  accountsError?: string
  accounts: AdminGroupAccount[]
}

export interface ConnectionHealthEvent {
  id: string
  connectionId: string
  modelName: string
  ownGroupName: string
  upstreamSiteId: string
  upstreamGroupName: string
  result: string
  fromState: string
  toState: string
  latencyMs: number | null
  errorKey: string
  remoteAction: string
  createdAt: string
}

export interface ConnectionHealthOverview {
  totalConnections: number
  healthy: number
  degraded: number
  suspended: number
  observing: number
  recovering: number
  disabled: number
  unconfigured: number
  recentEvents: ConnectionHealthEvent[]
}

export interface ModelTargetInput {
  id?: string
  modelName: string
  providerFamily: string
  enabled: boolean
  probePrompt?: string
  maxProbeTokens?: number
}

export interface ConnectionHealthModelTarget extends Required<Pick<ModelTargetInput, 'modelName' | 'providerFamily' | 'enabled'>> {
  id: string
  policyId: string
  probePrompt: string
  maxProbeTokens: number
  createdAt: string
  updatedAt: string
}

export interface ConnectionHealthPolicy {
  id: string
  name: string
  enabled: boolean
  ownGroupId: string
  ownGroupName: string
  modelPattern: string
  probeMode: string
  probeIntervalSeconds: number
  failureThreshold: number
  successThreshold: number
  cooldownSeconds: number
  observationSeconds: number
  recoveryStepPercent: number
  autoDegradeEnabled: boolean
  autoRemoteActionEnabled: boolean
  dailyProbeBudget: number
  createdAt: string
  updatedAt: string
  modelTargets: ConnectionHealthModelTarget[]
}

// ProbeModelCandidate 是手动探活模型选择弹窗里的一行候选模型：来自当前 workspace 启用探活策略
// 下的启用 modelTargets，按连接所属的 own group 匹配（见 composables/useConnectionHealth 的
// matchingProbeCandidates）。不包含任何 upstream key / token 等敏感字段。
export interface ProbeModelCandidate {
  modelName: string
  providerFamily: string
  policyId: string
  policyName: string
  autoRemoteActionEnabled: boolean
  maxProbeTokens: number
}

// ManualProbeModelOption 是手动一次性探活弹窗展示的模型候选，来自后端 server-only 现查
// 上游 /v1/models 的结果，只含安全字段，不含 base_url/key/credentials。
export interface ManualProbeModelOption {
  id: string
  name: string
  ownedBy?: string
  providerFamily?: string
}

// ManualProbeResult 是手动一次性探活单个模型的 transient 结果：只用于弹窗内展示，
// 不对应任何落库的状态/事件记录。
export interface ManualProbeResult {
  modelName: string
  result: string
  healthy: boolean
  latencyMs: number | null
  errorKey: string
  errorDetail: string
  probedAt: string
}

export interface PolicyInput {
  id?: string
  name: string
  enabled: boolean
  ownGroupId: string
  ownGroupName: string
  modelPattern?: string
  probeIntervalSeconds?: number
  failureThreshold?: number
  successThreshold?: number
  cooldownSeconds?: number
  observationSeconds?: number
  recoveryStepPercent?: number
  autoDegradeEnabled: boolean
  autoRemoteActionEnabled: boolean
  dailyProbeBudget?: number
  modelTargets: ModelTargetInput[]
}
