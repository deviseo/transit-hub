import type {
  UserActivityDetail,
  UserActivityFilter,
  UserActivityListQuery,
  UserActivityListResponse,
  UserActivityRecipientPreview,
  UserActivitySummary,
  UserActivitySync,
} from '../types/userActivity'
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
      headers: {
        Accept: 'application/json',
        'Content-Type': 'application/json',
        ...authHeaders(),
        ...(options.headers ?? {}),
      },
    })
  } catch {
    throw new Error('admin.userActivity.errors.network')
  }

  const text = await response.text()
  let payload: T & ErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & ErrorPayload : ({} as T & ErrorPayload)
  } catch {
    throw new Error('admin.userActivity.errors.request')
  }

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.userActivity.errors.request')
  }
  return payload
}

const filterParams = (filter: UserActivityFilter): URLSearchParams => {
  const params = new URLSearchParams({ segment: filter.segment })
  if (filter.segment === 'inactive' && filter.inactiveDays) params.set('inactiveDays', String(filter.inactiveDays))
  if (filter.search) params.set('search', filter.search)
  if (filter.status) params.set('status', filter.status)
  if (filter.role) params.set('role', filter.role)
  return params
}

export const getUserActivitySummary = async (days: number): Promise<UserActivitySummary> => (
  requestJson<UserActivitySummary>(`/user-activity/summary?days=${encodeURIComponent(String(days))}`)
)

export const triggerUserActivitySync = async (): Promise<UserActivitySync> => (
  requestJson<UserActivitySync>('/user-activity/sync', { method: 'POST' })
)

export const listUserActivityUsers = async (query: UserActivityListQuery): Promise<UserActivityListResponse> => {
  const params = filterParams(query)
  params.set('sort', query.sort)
  params.set('order', query.order)
  params.set('page', String(query.page))
  params.set('pageSize', String(query.pageSize))
  const payload = await requestJson<UserActivityListResponse>(`/user-activity/users?${params.toString()}`)
  return { ...payload, items: payload.items ?? [] }
}

export const getUserActivityDetail = async (siteUserId: string): Promise<UserActivityDetail> => (
  requestJson<UserActivityDetail>(`/user-activity/users/${encodeURIComponent(siteUserId)}`)
)

export const previewUserActivityRecipients = async (filter: UserActivityFilter): Promise<UserActivityRecipientPreview> => (
  requestJson<UserActivityRecipientPreview>(`/user-activity/recipients/preview?${filterParams(filter).toString()}`)
)

// 退订链接使用的 TransitHub API 公开地址（例如 https://th.example.com/api）。
export const defaultUnsubscribeBaseUrl = (): string => {
  try {
    return new URL(apiBaseUrl, window.location.origin).toString().replace(/\/$/, '')
  } catch {
    return ''
  }
}

// 本机或内网地址：收件人在外网点击退订链接时无法访问。
export const isLikelyPrivateUrl = (value: string): boolean => {
  try {
    const host = new URL(value).hostname.replace(/^\[|\]$/g, '').toLowerCase()
    if (host === 'localhost' || host.endsWith('.localhost') || host.endsWith('.local') || host === '::1' || host === '0.0.0.0') return true
    if (/^127\./.test(host) || /^10\./.test(host) || /^192\.168\./.test(host) || /^169\.254\./.test(host)) return true
    const match = /^172\.(\d+)\./.exec(host)
    if (match && Number(match[1]) >= 16 && Number(match[1]) <= 31) return true
    return /^f[cd][0-9a-f]{2}:/.test(host) || /^fe80:/.test(host)
  } catch {
    return true
  }
}
