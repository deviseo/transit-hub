import {
  authUnauthorizedErrorKey,
  getAccessToken,
  handleAuthExpired,
  isUnauthorizedApiResponse,
} from '@/modules/auth/api/auth'
import type { LeaderboardDateRange, LeaderboardEmbedConfig, LeaderboardResponse } from '../types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'
const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

type ErrorPayload = { message?: string }

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  const token = getAccessToken()
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
        ...(options.headers ?? {}),
      },
    })
  } catch {
    throw new Error('admin.leaderboard.errors.network')
  }

  const text = await response.text()
  let payload: T & ErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & ErrorPayload : ({} as T & ErrorPayload)
  } catch {
    throw new Error('admin.leaderboard.errors.unknown')
  }
  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.leaderboard.errors.unknown')
  }
  return payload
}

export const getLeaderboard = async (range: LeaderboardDateRange): Promise<LeaderboardResponse> => {
  const params = new URLSearchParams({ start_date: range.startDate, end_date: range.endDate })
  return requestJson<LeaderboardResponse>(`/leaderboard/data?${params.toString()}`)
}

export const getLeaderboardEmbedConfig = async (): Promise<LeaderboardEmbedConfig> => (
  requestJson<LeaderboardEmbedConfig>('/leaderboard/embed-config')
)

export const rotateLeaderboardEmbedToken = async (): Promise<LeaderboardEmbedConfig> => (
  requestJson<LeaderboardEmbedConfig>('/leaderboard/embed-config/rotate-token', { method: 'POST' })
)
