import {
  authUnauthorizedErrorKey,
  getAccessToken,
  handleAuthExpired,
  isUnauthorizedApiResponse,
} from '@/modules/auth/api/auth'
import type {
  LotteryAuditResponse,
  LotteryCampaign,
  LotteryCampaignRequest,
  LotteryCampaignsResponse,
  LotteryEmbedConfig,
  LotteryEntriesResponse,
  LotteryOkResponse,
  LotterySubscriptionGroupsResponse,
} from '../types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'
const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

type AdminErrorPayload = { message?: string }

const authHeaders = (): HeadersInit => {
  const token = getAccessToken()
  if (!token) return {}
  return { Authorization: `Bearer ${token}` }
}

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...authHeaders(),
        ...(options.headers ?? {}),
      },
    })
  } catch {
    throw new Error('admin.lottery.errors.network')
  }

  const text = await response.text()
  let payload: T & AdminErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & AdminErrorPayload : ({} as T & AdminErrorPayload)
  } catch {
    throw new Error('admin.lottery.errors.request')
  }

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.lottery.errors.request')
  }

  return payload
}

export const getLotteryEmbedConfig = async (): Promise<LotteryEmbedConfig> => (
  requestJson<LotteryEmbedConfig>('/lottery/embed-config')
)

export const rotateLotteryEmbedToken = async (): Promise<LotteryEmbedConfig> => (
  requestJson<LotteryEmbedConfig>('/lottery/embed-config/rotate-token', { method: 'POST' })
)

export const listLotteryCampaigns = async (): Promise<LotteryCampaignsResponse> => (
  requestJson<LotteryCampaignsResponse>('/lottery/campaigns')
)

export const listLotterySubscriptionGroups = async (): Promise<LotterySubscriptionGroupsResponse> => (
  requestJson<LotterySubscriptionGroupsResponse>('/lottery/subscription-groups')
)

export const createLotteryCampaign = async (request: LotteryCampaignRequest): Promise<LotteryCampaign> => (
  requestJson<LotteryCampaign>('/lottery/campaigns', { method: 'POST', body: JSON.stringify(request) })
)

export const getLotteryCampaign = async (id: string): Promise<LotteryCampaign> => (
  requestJson<LotteryCampaign>(`/lottery/campaigns/${encodeURIComponent(id)}`)
)

export const updateLotteryCampaign = async (id: string, request: LotteryCampaignRequest): Promise<LotteryCampaign> => (
  requestJson<LotteryCampaign>(`/lottery/campaigns/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(request) })
)

export const publishLotteryCampaign = async (id: string): Promise<LotteryCampaign> => (
  requestJson<LotteryCampaign>(`/lottery/campaigns/${encodeURIComponent(id)}/publish`, { method: 'POST' })
)

export const closeLotteryCampaign = async (id: string): Promise<LotteryCampaign> => (
  requestJson<LotteryCampaign>(`/lottery/campaigns/${encodeURIComponent(id)}/close`, { method: 'POST' })
)

export const drawLotteryCampaign = async (id: string): Promise<LotteryCampaign> => (
  requestJson<LotteryCampaign>(`/lottery/campaigns/${encodeURIComponent(id)}/draw`, { method: 'POST' })
)

export const cancelLotteryCampaign = async (id: string): Promise<LotteryCampaign> => (
  requestJson<LotteryCampaign>(`/lottery/campaigns/${encodeURIComponent(id)}/cancel`, { method: 'POST' })
)

export const listLotteryEntries = async (id: string): Promise<LotteryEntriesResponse> => (
  requestJson<LotteryEntriesResponse>(`/lottery/campaigns/${encodeURIComponent(id)}/entries`)
)

export const listLotteryAudit = async (id: string): Promise<LotteryAuditResponse> => (
  requestJson<LotteryAuditResponse>(`/lottery/campaigns/${encodeURIComponent(id)}/audit`)
)

export const retryLotteryReward = async (id: string): Promise<LotteryOkResponse> => (
  requestJson<LotteryOkResponse>(`/lottery/reward-jobs/${encodeURIComponent(id)}/retry`, { method: 'POST' })
)

export const completeManualLotteryReward = async (id: string): Promise<LotteryOkResponse> => (
  requestJson<LotteryOkResponse>(`/lottery/reward-jobs/${encodeURIComponent(id)}/complete`, { method: 'POST' })
)
