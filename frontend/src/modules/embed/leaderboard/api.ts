import type {
  CreateLeaderboardEmbedSessionRequest,
  CreateLeaderboardEmbedSessionResponse,
  LeaderboardDateRange,
  LeaderboardResponse,
} from '@/modules/leaderboard/types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'
const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`
let sessionToken: string | null = null

type ErrorPayload = { message?: string }

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...(sessionToken ? { Authorization: `Bearer ${sessionToken}` } : {}),
        ...(options.headers ?? {}),
      },
    })
  } catch {
    throw new Error('embed.leaderboard.errors.network')
  }
  const text = await response.text()
  let payload: T & ErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & ErrorPayload : ({} as T & ErrorPayload)
  } catch {
    throw new Error('embed.leaderboard.errors.request')
  }
  if (!response.ok) throw new Error(payload.message ?? 'embed.leaderboard.errors.request')
  return payload
}

export const createLeaderboardEmbedSession = async (
  request: CreateLeaderboardEmbedSessionRequest,
): Promise<CreateLeaderboardEmbedSessionResponse> => {
  const response = await requestJson<CreateLeaderboardEmbedSessionResponse>('/embed/leaderboard/session', {
    method: 'POST',
    body: JSON.stringify(request),
  })
  sessionToken = response.sessionToken
  return response
}

export const getEmbedLeaderboard = async (range: LeaderboardDateRange): Promise<LeaderboardResponse> => {
  const params = new URLSearchParams({ start_date: range.startDate, end_date: range.endDate })
  return requestJson<LeaderboardResponse>(`/embed/leaderboard?${params.toString()}`)
}
