import type { SalesEntriesQuery, SalesEntriesResponse, SalesEntryKey, SalesRangeQuery, SalesSummary } from '../types/sales'
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

const requestJson = async <T>(path: string, init?: { method: string, body: unknown }): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      method: init?.method ?? 'GET',
      headers: {
        Accept: 'application/json',
        ...(init ? { 'Content-Type': 'application/json' } : {}),
        ...authHeaders(),
      },
      body: init ? JSON.stringify(init.body) : undefined,
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

/** 入账明细（某个用户某天某个渠道一行），可以按渠道、用户筛选，或者只看不计入的。 */
export const getSalesEntries = async (query: SalesEntriesQuery): Promise<SalesEntriesResponse> => {
  const params = new URLSearchParams({ from: query.from, to: query.to })
  if (query.channel) params.set('channel', query.channel)
  if (query.q) params.set('q', query.q)
  if (query.excluded) params.set('excluded', '1')
  if (query.page) params.set('page', String(query.page))
  return requestJson<SalesEntriesResponse>(`/sales/entries?${params.toString()}`)
}

/** 这些明细不计入销售分析（只影响 TransitHub 的统计，不改站点数据）。 */
export const excludeSalesEntries = async (entries: SalesEntryKey[]): Promise<{ changed: number }> =>
  requestJson<{ changed: number }>('/sales/exclusions', { method: 'POST', body: { entries } })

/** 把这些明细重新计入。 */
export const restoreSalesEntries = async (entries: SalesEntryKey[]): Promise<{ changed: number }> =>
  requestJson<{ changed: number }>('/sales/exclusions/delete', { method: 'POST', body: { entries } })

/** 设置整个不计入的渠道。 */
export const setSalesExcludedChannels = async (excludedChannels: string[]): Promise<{ excludedChannels: string[] }> =>
  requestJson<{ excludedChannels: string[] }>('/sales/settings', { method: 'PUT', body: { excludedChannels } })
