import type { LotteryCampaign, LotteryCampaignsResponse, LotteryEntry } from '@/modules/lottery/types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'
const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

let sessionToken: string | null = null

export interface CreateLotteryEmbedSessionRequest {
  embedToken: string
  sub2apiToken: string
  srcHost: string
  srcUrl: string
  userId: string
}

export interface CreateLotteryEmbedSessionResponse {
  sessionToken: string
}

type EmbedErrorPayload = { message?: string }

const authHeaders = (): HeadersInit => (sessionToken ? { Authorization: `Bearer ${sessionToken}` } : {})

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
    throw new Error('embed.lottery.errors.network')
  }

  const text = await response.text()
  let payload: T & EmbedErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & EmbedErrorPayload : ({} as T & EmbedErrorPayload)
  } catch {
    throw new Error('embed.lottery.errors.request')
  }

  if (!response.ok) {
    throw new Error(payload.message ?? 'embed.lottery.errors.request')
  }

  return payload
}

export const createLotteryEmbedSession = async (
  request: CreateLotteryEmbedSessionRequest,
): Promise<CreateLotteryEmbedSessionResponse> => {
  const response = await requestJson<CreateLotteryEmbedSessionResponse>('/embed/lottery/session', {
    method: 'POST',
    body: JSON.stringify(request),
  })
  sessionToken = response.sessionToken
  return response
}

export const listEmbedLotteryCampaigns = async (): Promise<LotteryCampaignsResponse> => (
  requestJson<LotteryCampaignsResponse>('/embed/lottery/campaigns')
)

export const getEmbedLotteryCampaign = async (id: string): Promise<LotteryCampaign> => (
  requestJson<LotteryCampaign>(`/embed/lottery/campaigns/${encodeURIComponent(id)}`)
)

export const enterEmbedLotteryCampaign = async (id: string): Promise<LotteryEntry> => (
  requestJson<LotteryEntry>(`/embed/lottery/campaigns/${encodeURIComponent(id)}/entries`, { method: 'POST' })
)

export const withdrawEmbedLotteryEntry = async (id: string): Promise<void> => {
  await requestJson<Record<string, boolean>>(`/embed/lottery/campaigns/${encodeURIComponent(id)}/entries`, {
    method: 'DELETE',
  })
}
