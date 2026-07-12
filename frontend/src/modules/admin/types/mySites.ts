export interface MySiteGroupRef {
  siteId: string
  groupName: string
}

export type AutoPricingSource = 'primary_upstream' | 'lowest_upstream' | 'highest_upstream' | 'average_upstream'
export type AutoPricingStrategy = 'fixed' | 'percentage'
export type AutoPricingRunStatus = 'applied' | 'skipped' | 'threshold_exceeded' | 'failed'
export type AutoPricingRunTrigger = 'after_sync' | 'manual'

export interface AutoPricingRunResult {
  status?: AutoPricingRunStatus
  reason?: string
  trigger?: AutoPricingRunTrigger | string
  ranAt?: string
  oldReference?: number | null
  newReference?: number | null
  targetMultiplier?: number | null
  oldOwnMultiplier?: number | null
  newOwnMultiplier?: number | null
}

export interface MySiteMapping {
  ownGroup: string
  upstreamTargets: MySiteGroupRef[]
  enableAutoPricing?: boolean
  autoPricingSource?: AutoPricingSource
  primaryUpstreamSiteId?: string
  primaryUpstreamGroupName?: string
  autoPricingStrategy?: AutoPricingStrategy
  fixedIncrease?: number
  percentageIncrease?: number
  adjustThresholdPercent?: number
  minMultiplier?: number | null
  maxMultiplier?: number | null
  enableAutoPricingNotify?: boolean
  autoPricingNotifyBotIds?: string[]
  autoPricingNotifyTemplate?: string
  lastAutoPricingRun?: AutoPricingRunResult | null
}

export interface RunAutoPricingRequest {
  ownGroup: string
}

export interface RunAutoPricingResponse {
  result: AutoPricingRunResult
  mapping: MySiteMapping
}

export interface MySiteStatus {
  authenticated: boolean
  baseUrl?: string
  email?: string
  mappings?: MySiteMapping[]
}

export interface MySiteMappingOptionsResponse {
  ownGroups: MySiteMappingOwnGroupOption[]
  mappings: MySiteMapping[]
}

export interface MySiteMappingOwnGroupOption {
  id: string
  siteName: string
  groupName: string
  multiplier: number
  platform: string
  status: string
  isExclusive: boolean
  subscriptionType: string
}

export interface RealConnectRequest {
  upstreamSiteId: string
  upstreamGroupId: string
  upstreamGroupName: string
  groupType: string
  channelType?: number
  ownGroupIds: string[]
}

export interface NewAPIChannelType {
  id: number
  name: string
}

export const NEW_API_CHANNEL_TYPES: NewAPIChannelType[] = [
  { id: 1, name: 'OpenAI' },
  { id: 2, name: 'Midjourney' },
  { id: 3, name: 'Azure' },
  { id: 4, name: 'Ollama' },
  { id: 5, name: 'MidjourneyPlus' },
  { id: 6, name: 'OpenAIMax' },
  { id: 7, name: 'OhMyGPT' },
  { id: 8, name: 'Custom' },
  { id: 9, name: 'AILS' },
  { id: 10, name: 'AIProxy' },
  { id: 11, name: 'PaLM' },
  { id: 12, name: 'API2GPT' },
  { id: 13, name: 'AIGC2D' },
  { id: 14, name: 'Anthropic' },
  { id: 15, name: 'Baidu' },
  { id: 16, name: 'Zhipu' },
  { id: 17, name: 'Ali' },
  { id: 18, name: 'Xunfei' },
  { id: 19, name: '360' },
  { id: 20, name: 'OpenRouter' },
  { id: 21, name: 'AIProxyLibrary' },
  { id: 22, name: 'FastGPT' },
  { id: 23, name: 'Tencent' },
  { id: 24, name: 'Gemini' },
  { id: 25, name: 'Moonshot' },
  { id: 26, name: 'ZhipuV4' },
  { id: 27, name: 'Perplexity' },
  { id: 31, name: 'LingYiWanWu' },
  { id: 33, name: 'AWS' },
  { id: 34, name: 'Cohere' },
  { id: 35, name: 'MiniMax' },
  { id: 36, name: 'SunoAPI' },
  { id: 37, name: 'Dify' },
  { id: 38, name: 'Jina' },
  { id: 39, name: 'Cloudflare' },
  { id: 40, name: 'SiliconFlow' },
  { id: 41, name: 'VertexAI' },
  { id: 42, name: 'Mistral' },
  { id: 43, name: 'DeepSeek' },
  { id: 44, name: 'MokaAI' },
  { id: 45, name: 'VolcEngine' },
  { id: 46, name: 'BaiduV2' },
  { id: 47, name: 'Xinference' },
  { id: 48, name: 'xAI' },
  { id: 49, name: 'Coze' },
  { id: 50, name: 'Kling' },
  { id: 51, name: 'Jimeng' },
  { id: 52, name: 'Vidu' },
  { id: 53, name: 'Submodel' },
  { id: 54, name: 'DoubaoVideo' },
  { id: 55, name: 'Sora' },
  { id: 56, name: 'Replicate' },
  { id: 57, name: 'Codex' },
]

export interface RealConnection {
  id: string
  upstreamSiteId: string
  upstreamGroupId: string
  upstreamGroupName: string
  upstreamKeyId: string
  upstreamKey: string
  adminAccountId: string
  adminAccountName: string
  ownGroupIds: string[]
  groupType: string
  createdAt: string
}

export interface RealBindRequest {
  upstreamSiteId: string
  upstreamGroupId: string
  upstreamGroupName: string
  upstreamKeyId: string
  upstreamKey: string
  ownGroupIds: string[]
  groupType: string
}

export interface UpstreamKeyItem {
  id: string
  key: string
  name: string
  groupId: string
  groupName: string
  status: string
}

export interface RealConnectResponse {
  connection: RealConnection
}

export interface RealDisconnectRequest {
  connectionId: string
  mode: 'unlink' | 'full'
}
