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
  AdminResourceOption,
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

const normalizeMappings = (value: unknown): MySiteMapping[] => {
  if (!Array.isArray(value)) return []
  return value.flatMap((entry) => {
    if (entry == null || typeof entry !== 'object') return []
    const mapping = entry as MySiteMapping
    if (typeof mapping.ownGroup !== 'string' || !mapping.ownGroup.trim()) return []
    const upstreamTargets = Array.isArray(mapping.upstreamTargets)
      ? mapping.upstreamTargets.filter(target => (
          target != null &&
          typeof target.siteId === 'string' &&
          typeof target.groupName === 'string'
        ))
      : []
    return [{ ...mapping, upstreamTargets }]
  })
}

const normalizeStatus = (status: MySiteStatus): MySiteStatus => ({
  ...status,
  ...(Object.prototype.hasOwnProperty.call(status, 'mappings')
    ? { mappings: normalizeMappings(status.mappings) }
    : {}),
})

const normalizeMappingOptions = (response: MySiteMappingOptionsResponse): MySiteMappingOptionsResponse => ({
  ...response,
  ownGroups: Array.isArray(response.ownGroups) ? response.ownGroups : [],
  mappings: normalizeMappings(response.mappings),
  staleOwnGroups: Array.isArray(response.staleOwnGroups) ? response.staleOwnGroups : [],
  staleTargets: Array.isArray(response.staleTargets) ? response.staleTargets : [],
})

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
  let payload = {} as T & AdminErrorPayload
  if (text) {
    try {
      payload = JSON.parse(text) as T & AdminErrorPayload
    } catch {
      payload = {} as T & AdminErrorPayload
    }
  }

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }

    throw new Error(payload.message ?? 'admin.mySites.errors.request')
  }

  return payload
}

export const getMySiteMappingOptions = async (): Promise<MySiteMappingOptionsResponse> => (
  normalizeMappingOptions(await requestJson<MySiteMappingOptionsResponse>('/my-sites/mapping-options'))
)

export const saveMySiteMappings = async (mappings: MySiteMapping[]): Promise<MySiteStatus> => (
  normalizeStatus(await requestJson<MySiteStatus>('/my-sites/mappings', {
    method: 'PUT',
    body: JSON.stringify({ mappings }),
  }))
)

export const realConnect = async (req: RealConnectRequest): Promise<RealConnectResponse> => (
  requestJson<RealConnectResponse>('/my-sites/real-connect', {
    method: 'POST',
    body: JSON.stringify(req),
  })
)

export const listRealConnections = async (): Promise<RealConnection[]> =>
  requestJson<RealConnection[]>('/my-sites/real-connections')

export const listUpstreamKeys = async (siteId: string, groupId: string, groupName: string): Promise<UpstreamKeyItem[]> => {
  const params = new URLSearchParams({ siteId, groupId, groupName })
  const items = await requestJson<UpstreamKeyItem[]>(`/my-sites/upstream-keys?${params.toString()}`)
  return Array.isArray(items)
    ? items.map(item => ({
        ...item,
        // Older backends returned the full key. Keep it only as an internal
        // compatibility fallback; the UI renders the non-secret preview.
        keyPreview: item.keyPreview || (item.key ? `${item.key.slice(0, 6)}...${item.key.slice(-4)}` : ''),
      }))
    : []
}

export const listAdminResources = async (groupId: string): Promise<AdminResourceOption[]> => {
  const items = await requestJson<AdminResourceOption[]>(`/my-sites/admin-resources?groupId=${encodeURIComponent(groupId)}`)
  return Array.isArray(items) ? items : []
}

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

export const runAutoPricing = async (req: RunAutoPricingRequest): Promise<RunAutoPricingResponse> => {
  const response = await requestJson<RunAutoPricingResponse>('/my-sites/auto-pricing/run', {
    method: 'POST',
    body: JSON.stringify(req),
  })
  return {
    ...response,
    mapping: normalizeMappings([response.mapping])[0] ?? response.mapping,
  }
}

// New backends update one mapping atomically. A generic method-not-supported
// response falls back to the legacy full-array PUT so rolling deployments remain usable.
export const saveMySiteMapping = async (mapping: MySiteMapping, currentMappings: MySiteMapping[]): Promise<MySiteStatus> => {
  try {
    return normalizeStatus(await requestJson<MySiteStatus>('/my-sites/mappings', {
      method: 'PATCH',
      body: JSON.stringify({ mapping }),
    }))
  } catch (error) {
    if (!(error instanceof Error) || error.message !== 'admin.mySites.errors.request') throw error
    const nextMappings = currentMappings.some(item => item.ownGroup === mapping.ownGroup)
      ? currentMappings.map(item => item.ownGroup === mapping.ownGroup ? mapping : item)
      : [...currentMappings, mapping]
    return saveMySiteMappings(nextMappings)
  }
}

export const removeMySiteMapping = async (ownGroup: string, currentMappings: MySiteMapping[]): Promise<MySiteStatus> => {
  try {
    return normalizeStatus(await requestJson<MySiteStatus>(`/my-sites/mappings/${encodeURIComponent(ownGroup)}`, { method: 'DELETE' }))
  } catch (error) {
    if (!(error instanceof Error) || error.message !== 'admin.mySites.errors.request') throw error
    return saveMySiteMappings(currentMappings.filter(item => item.ownGroup !== ownGroup))
  }
}
