import type { SalesRangeQuery, SalesSummary } from '../types/sales'
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

const requestJson = async <T>(path: string): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      headers: { Accept: 'application/json', ...authHeaders() },
    })
  } catch {
    throw new Error('admin.sales.errors.network')
  }

  const text = await response.text()
  let payload: T & ErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & ErrorPayload : ({} as T & ErrorPayload)
  } catch {
    throw new Error('admin.sales.errors.request')
  }

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.sales.errors.request')
  }
  return payload
}

export const getSalesSummary = async (query: SalesRangeQuery): Promise<SalesSummary> => {
  const params = new URLSearchParams()
  if ('days' in query) params.set('days', String(query.days))
  else {
    params.set('from', query.from)
    params.set('to', query.to)
  }
  return requestJson<SalesSummary>(`/sales/summary?${params.toString()}`)
}
