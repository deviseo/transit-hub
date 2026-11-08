import type { VipCandidate, VipSettings, VipSummary } from '../types/vip'
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
  return token ? { Authorization: `Bearer ${token}` } : {}
}

type ErrorPayload = {
  message?: string
}

// 出错时抛出 i18n key：后端返回的 message（admin.vip.errors.*），否则用通用的。
const requestJson = async <T>(path: string, init?: { method: string, body?: unknown }): Promise<T> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      method: init?.method ?? 'GET',
      headers: {
        Accept: 'application/json',
        ...(init?.body !== undefined ? { 'Content-Type': 'application/json' } : {}),
        ...authHeaders(),
      },
      body: init?.body !== undefined ? JSON.stringify(init.body) : undefined,
    })
  } catch {
    throw new Error('admin.vip.errors.network')
  }
  const text = await response.text()
  let payload: T & ErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & ErrorPayload : ({} as T & ErrorPayload)
  } catch {
    throw new Error('admin.vip.errors.request')
  }
  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.vip.errors.request')
  }
  return payload
}

export const getVipSummary = async (): Promise<VipSummary> => requestJson<VipSummary>('/vip/summary')

export const searchVipCandidates = async (query: string): Promise<VipCandidate[]> => {
  const params = new URLSearchParams({ q: query })
  const response = await requestJson<{ items: VipCandidate[] }>(`/vip/search?${params.toString()}`)
  return response.items
}

/** 手动关注（已经关注的更新备注）；之前移出过的也回到名单。 */
export const watchVip = async (siteUserId: string, note: string): Promise<void> => {
  await requestJson('/vip/watch', { method: 'POST', body: { siteUserId, note } })
}

/** 移出名单（系统也不再推荐他）。 */
export const unwatchVip = async (siteUserId: string): Promise<void> => {
  await requestJson('/vip/unwatch', { method: 'POST', body: { siteUserId } })
}

/** 让移出过的人重新参加推荐。 */
export const restoreDismissedVip = async (): Promise<number> => {
  const response = await requestJson<{ restored: number }>('/vip/dismissed/restore', { method: 'POST', body: {} })
  return response.restored
}

export const saveVipSettings = async (settings: VipSettings): Promise<VipSettings> =>
  requestJson<VipSettings>('/vip/settings', { method: 'PUT', body: settings })
