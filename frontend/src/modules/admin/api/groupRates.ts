import type { GroupRateHistoryQuery, GroupRateHistoryRow, GroupRatesQuery, PaginatedGroupRatesResponse, UpdateGroupRateTypeRequest } from '../types/groupRates'
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

type AdminErrorPayload = {
  message?: string
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
  } catch (error) {
    throw new Error('admin.groupRates.errors.network')
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) as T & AdminErrorPayload : ({} as T & AdminErrorPayload)

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }

    throw new Error('admin.groupRates.errors.request')
  }

  return payload
}

export const listGroupRates = async (query: GroupRatesQuery): Promise<PaginatedGroupRatesResponse> => {
  const params = new URLSearchParams({
    page: query.page.toString(),
    search: query.search.trim(),
    type: query.type,
    platform: query.platform,
    status: query.status,
    sort: query.sort,
  })

  return requestJson<PaginatedGroupRatesResponse>(`/group-rates?${params.toString()}`)
}

export const listGroupRateHistory = async (query: GroupRateHistoryQuery): Promise<GroupRateHistoryRow[]> => {
  const params = new URLSearchParams({
    siteId: query.siteId,
    groupName: query.groupId || query.groupName,
  })

  if (query.platform) params.set('platform', query.platform)

  return requestJson<GroupRateHistoryRow[]>(`/group-rates/history?${params.toString()}`)
}

export const updateGroupRateType = async (request: UpdateGroupRateTypeRequest): Promise<void> => {
  await requestJson<{ success: boolean }>('/group-rates/type', {
    method: 'PATCH',
    body: JSON.stringify(request),
  })
}
