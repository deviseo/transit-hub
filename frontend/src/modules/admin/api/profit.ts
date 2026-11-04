import type { ProfitRangeQuery, ProfitSummary, ProfitSyncStatus } from '../types/profit'
import {
  authUnauthorizedErrorKey,
  getAccessToken,
  handleAuthExpired,
  isUnauthorizedApiResponse,
} from '@/modules/auth/api/auth'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'

const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

const authHeaders = (): HeadersInit => {
  const token = getAccessToken()
  if (!token) return {}
  return { Authorization: `Bearer ${token}` }
}

type ErrorPayload = {
  message?: string
}

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: { Accept: 'application/json', ...authHeaders(), ...(options.headers ?? {}) },
    })
  } catch {
    throw new Error('admin.profit.errors.network')
  }

  const text = await response.text()
  let payload: T & ErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & ErrorPayload : ({} as T & ErrorPayload)
  } catch {
    throw new Error('admin.profit.errors.request')
  }

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.profit.errors.request')
  }
  return payload
}

export const getProfitSummary = async (query: ProfitRangeQuery): Promise<ProfitSummary> => {
  const params = new URLSearchParams()
  if ('days' in query) params.set('days', String(query.days))
  else {
    params.set('from', query.from)
    params.set('to', query.to)
  }
  return requestJson<ProfitSummary>(`/profit/summary?${params.toString()}`)
}

export const startProfitSync = async (): Promise<ProfitSyncStatus> => (
  requestJson<ProfitSyncStatus>('/profit/sync', { method: 'POST' })
)
