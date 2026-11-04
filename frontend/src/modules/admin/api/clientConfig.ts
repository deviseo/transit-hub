import type { ClientConfig, ClientConfigUpdate, ClientGroup } from '../types/clientConfig'
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
    throw new Error('admin.client.errors.network')
  }

  const text = await response.text()
  let payload: T & ErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & ErrorPayload : ({} as T & ErrorPayload)
  } catch {
    throw new Error('admin.client.errors.request')
  }

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.client.errors.request')
  }
  return payload
}

export const getClientConfig = async (): Promise<ClientConfig> => requestJson<ClientConfig>('/client-config')

export const saveClientConfig = async (config: ClientConfigUpdate): Promise<ClientConfig> => (
  requestJson<ClientConfig>('/client-config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  })
)

export const resetClientConfigCode = async (): Promise<ClientConfig> => (
  requestJson<ClientConfig>('/client-config/reset-code', { method: 'POST' })
)

export const getClientGroups = async (): Promise<ClientGroup[]> => requestJson<ClientGroup[]>('/client-config/groups')
