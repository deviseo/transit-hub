import type {
  CreateMassEmailBatchRequest,
  MassEmailBatch,
  MassEmailUsersQuery,
  PaginatedMassEmailBatchItemsResponse,
  PaginatedMassEmailBatchesResponse,
  PaginatedMassEmailUsersResponse,
} from '../types/massEmail'
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
    throw new Error('admin.massEmail.errors.network')
  }

  const text = await response.text()
  let payload: T & AdminErrorPayload
  try {
    payload = text ? JSON.parse(text) as T & AdminErrorPayload : ({} as T & AdminErrorPayload)
  } catch {
    throw new Error('admin.massEmail.errors.request')
  }

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }

    throw new Error(payload.message ?? 'admin.massEmail.errors.request')
  }

  return payload
}

type PaginatedWire<T> = {
  items?: T[]
  total?: number
  page?: number
  pageSize?: number
  page_size?: number
  pages?: number
  totalPages?: number
  total_pages?: number
}

const normalizePaginated = <T>(payload: PaginatedWire<T>): { items: T[]; total: number; page: number; pageSize: number; totalPages: number } => ({
  items: payload.items ?? [],
  total: payload.total ?? 0,
  page: payload.page ?? 1,
  pageSize: payload.pageSize ?? payload.page_size ?? 20,
  totalPages: payload.pages ?? payload.totalPages ?? payload.total_pages ?? 1,
})

export const listMassEmailUsers = async (
  query: MassEmailUsersQuery,
): Promise<PaginatedMassEmailUsersResponse> => {
  const params = new URLSearchParams({
    page: query.page.toString(),
    page_size: query.pageSize.toString(),
    sort_by: query.sortBy,
    sort_order: query.sortOrder,
    timezone: query.timezone,
  })
  if (query.status) params.set('status', query.status)
  if (query.role) params.set('role', query.role)
  if (query.search) params.set('search', query.search)

  const payload = await requestJson<PaginatedWire<PaginatedMassEmailUsersResponse['items'][number]>>(`/mass-email/users?${params.toString()}`)
  return normalizePaginated(payload)
}

export const createMassEmailBatch = async (payload: CreateMassEmailBatchRequest): Promise<MassEmailBatch> => (
  requestJson<MassEmailBatch>('/mass-email/batches', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
)

export const listMassEmailBatches = async (page = 1, pageSize = 10): Promise<PaginatedMassEmailBatchesResponse> => {
  const params = new URLSearchParams({
    page: page.toString(),
    page_size: pageSize.toString(),
  })
  const payload = await requestJson<PaginatedWire<MassEmailBatch>>(`/mass-email/batches?${params.toString()}`)
  return normalizePaginated(payload)
}

export const getMassEmailBatch = async (id: string): Promise<MassEmailBatch> => (
  requestJson<MassEmailBatch>(`/mass-email/batches/${encodeURIComponent(id)}`)
)

export const listMassEmailBatchItems = async (
  id: string,
  page = 1,
  pageSize = 20,
): Promise<PaginatedMassEmailBatchItemsResponse> => {
  const params = new URLSearchParams({
    page: page.toString(),
    page_size: pageSize.toString(),
  })
  const payload = await requestJson<PaginatedWire<PaginatedMassEmailBatchItemsResponse['items'][number]>>(
    `/mass-email/batches/${encodeURIComponent(id)}/items?${params.toString()}`,
  )
  return normalizePaginated(payload)
}

export const cancelMassEmailBatch = async (id: string): Promise<MassEmailBatch> => (
  requestJson<MassEmailBatch>(`/mass-email/batches/${encodeURIComponent(id)}/cancel`, {
    method: 'POST',
  })
)
