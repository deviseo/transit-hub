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
  output?: string
}

class SystemApiError extends Error {
  readonly transient: boolean

  constructor(message: string, transient = false) {
    super(message)
    this.name = 'SystemApiError'
    this.transient = transient
  }
}

export const isTransientSystemApiError = (error: unknown): boolean => (
  error instanceof SystemApiError && error.transient
)

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
    throw new SystemApiError('admin.system.errors.network', true)
  }

  const text = await response.text()
  let payload = {} as T & AdminErrorPayload
  if (text) {
    try {
      payload = JSON.parse(text) as T & AdminErrorPayload
    } catch {
      throw new SystemApiError(
        response.ok ? 'admin.system.errors.request' : 'admin.system.errors.network',
        response.status === 502 || response.status === 503 || response.status === 504,
      )
    }
  }

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    const message = [payload.message, payload.output].filter(Boolean).join('\n') || 'admin.system.errors.request'
    throw new SystemApiError(message, response.status === 502 || response.status === 503 || response.status === 504)
  }

  return payload
}

export interface SystemVersionResponse {
  version: string
}

export const getSystemVersion = async (): Promise<SystemVersionResponse> => (
  requestJson<SystemVersionResponse>('/system/version')
)

export type SystemUpgradeState = 'idle' | 'starting' | 'running' | 'succeeded' | 'failed'

export interface SystemUpgradeStartResponse {
  state: 'starting'
  requestedAt: string
}

export interface SystemUpgradeStatusResponse {
  state: SystemUpgradeState
  startedAt?: string
  finishedAt?: string
  exitCode?: number
  output?: string
}

export const startSystemUpgrade = async (): Promise<SystemUpgradeStartResponse> => (
  requestJson<SystemUpgradeStartResponse>('/system/upgrade', { method: 'POST' })
)

export const getSystemUpgradeStatus = async (): Promise<SystemUpgradeStatusResponse> => (
  requestJson<SystemUpgradeStatusResponse>('/system/upgrade')
)

export type SystemRestartState = 'idle' | 'starting' | 'running' | 'succeeded' | 'failed'

export interface SystemRestartStartResponse {
  state: 'starting'
  requestedAt: string
}

export interface SystemRestartStatusResponse {
  state: SystemRestartState
  startedAt?: string
  finishedAt?: string
  exitCode?: number
  output?: string
}

export const startSystemRestart = async (): Promise<SystemRestartStartResponse> => (
  requestJson<SystemRestartStartResponse>('/system/restart', { method: 'POST' })
)

export const getSystemRestartStatus = async (): Promise<SystemRestartStatusResponse> => (
  requestJson<SystemRestartStatusResponse>('/system/restart')
)
