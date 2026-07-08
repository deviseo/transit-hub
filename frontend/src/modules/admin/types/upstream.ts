export type UpstreamPlatform = 'auto' | 'newapi' | 'sub2api'

export type ResolvedUpstreamPlatform = Exclude<UpstreamPlatform, 'auto'>

export type UpstreamStatus = 'connecting' | 'syncing' | 'connected' | 'error'

export type UpstreamAuthMode = 'password' | 'token'

export interface UpstreamSiteForm {
  name: string
  siteUrl: string
  platform: UpstreamPlatform
  authMode: UpstreamAuthMode
  account: string
  password: string
  accessToken: string
  refreshToken: string
  tokenType: string
  rechargeRate: number
  remark: string
}

export interface UpstreamMetricValue {
  value: number | null
  display: string
}

export interface UpstreamGroupInfo {
  id: string
  name: string
  platform: string | null
  multiplier: number | null
  multiplierDisplay: string
  // 以下字段为 sub2api 专属倍率展示新增的可选字段：旧后端/旧缓存数据没有这些字段时，
  // 前端保持原来的单倍率展示。multiplier/multiplierDisplay 始终是最终生效倍率。
  defaultMultiplier?: number | null
  defaultMultiplierDisplay?: string
  dedicatedMultiplier?: number | null
  dedicatedMultiplierDisplay?: string
  hasDedicatedMultiplier?: boolean
}

export interface UpstreamMetrics {
  balance: UpstreamMetricValue
  todayConsume: UpstreamMetricValue
  historyRecharge: UpstreamMetricValue
  group: UpstreamGroupInfo
  groups: UpstreamGroupInfo[]
}

export interface NewApiSession {
  platform: 'newapi'
  baseUrl: string
}

export interface Sub2ApiSession {
  platform: 'sub2api'
  baseUrl: string
  accessToken: string
  refreshToken: string | null
  tokenType: string
  expiresAt: number | null
}

export type UpstreamSession = NewApiSession | Sub2ApiSession

export interface SiteSettings {
  balanceThreshold: number | null
}

export interface UpstreamSite {
  id: string
  name: string
  baseUrl: string
  platform: ResolvedUpstreamPlatform
  requestedPlatform: UpstreamPlatform
  account: string
  rechargeRate: number
  remark: string
  logo: string
  logoBg: string
  status: UpstreamStatus
  errorKey: string | null
  metrics: UpstreamMetrics
  settings: SiteSettings
  session?: UpstreamSession | null
  lastSyncedAt: number | null
}

export type UpstreamSiteResponse = Omit<UpstreamSite, 'session' | 'logo' | 'logoBg'>

export interface UpstreamLoginResult {
  platform: ResolvedUpstreamPlatform
  baseUrl: string
  session: UpstreamSession
  metrics: UpstreamMetrics
}

export interface SyncStreamEvent {
  event: 'syncing' | 'done' | 'error' | 'complete'
  siteId: string
  site?: UpstreamSiteResponse
  errorKey?: string
}

export type SiteSyncPhase = 'idle' | 'syncing' | 'done' | 'error'

export interface SiteSyncState {
  phase: SiteSyncPhase
  errorKey?: string
}
