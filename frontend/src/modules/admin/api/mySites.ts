import type {
  RunAutoPricingRequest,
  RunAutoPricingResponse,
  MySiteMapping,
  MySiteMappingOptionsResponse,
  MySiteStatus,
  RealBindRequest,
  RealConnectRequest,
  RealConnectResponse,
  RealConnection,
  RealDisconnectRequest,
  UpstreamKeyItem,
} from '../types/mySites'
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
    throw new Error('admin.mySites.errors.network')
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) as T & AdminErrorPayload : ({} as T & AdminErrorPayload)

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }

    throw new Error(payload.message ?? 'admin.mySites.errors.request')
  }

  return payload
}

export const getMySiteMappingOptions = async (): Promise<MySiteMappingOptionsResponse> => requestJson<MySiteMappingOptionsResponse>('/my-sites/mapping-options')

export const saveMySiteMappings = async (mappings: MySiteMapping[]): Promise<MySiteStatus> => (
  requestJson<MySiteStatus>('/my-sites/mappings', {
    method: 'PUT',
    body: JSON.stringify({ mappings }),
  })
)

export const realConnect = async (req: RealConnectRequest): Promise<RealConnectResponse> => (
  requestJson<RealConnectResponse>('/my-sites/real-connect', {
    method: 'POST',
    body: JSON.stringify(req),
  })
)

export const listRealConnections = async (): Promise<RealConnection[]> =>
  requestJson<RealConnection[]>('/my-sites/real-connections')

export const listUpstreamKeys = async (siteId: string): Promise<UpstreamKeyItem[]> =>
  requestJson<UpstreamKeyItem[]>(`/my-sites/upstream-keys?siteId=${encodeURIComponent(siteId)}`)

export const realBind = async (req: RealBindRequest): Promise<RealConnectResponse> => (
  requestJson<RealConnectResponse>('/my-sites/real-bind', {
    method: 'POST',
    body: JSON.stringify(req),
  })
)

export const realDisconnect = async (req: RealDisconnectRequest): Promise<void> => {
  await requestJson<{ ok: boolean }>('/my-sites/real-disconnect', {
    method: 'POST',
    body: JSON.stringify(req),
  })
}

export const runAutoPricing = async (req: RunAutoPricingRequest): Promise<RunAutoPricingResponse> => (
  requestJson<RunAutoPricingResponse>('/my-sites/auto-pricing/run', {
    method: 'POST',
    body: JSON.stringify(req),
  })
)
