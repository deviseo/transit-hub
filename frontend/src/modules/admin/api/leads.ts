import type { MassEmailBatch } from '../types/massEmail'
import type {
  Lead,
  LeadImportResult,
  LeadListQuery,
  LeadListResponse,
  LeadSelection,
  LeadSendPreview,
  LeadSendRequest,
  LeadSettings,
  LeadSourceCount,
} from '../types/leads'
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

// 出错时抛出 i18n key：后端返回的 message（admin.leads.errors.* 或群发的 admin.massEmail.errors.*），否则用通用的。
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
    throw new Error('admin.leads.errors.network')
  }
  const text = await response.text()
  let payload: T & { message?: string }
  try {
    payload = text ? JSON.parse(text) as T & { message?: string } : ({} as T & { message?: string })
  } catch {
    throw new Error('admin.leads.errors.request')
  }
  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.leads.errors.request')
  }
  return payload
}

export const listLeads = async (query: LeadListQuery): Promise<LeadListResponse> => {
  const params = new URLSearchParams({ page: String(query.page), page_size: String(query.pageSize) })
  if (query.source) params.set('source', query.source)
  if (query.status) params.set('status', query.status)
  if (query.search) params.set('search', query.search)
  const payload = await requestJson<Partial<LeadListResponse>>(`/leads?${params.toString()}`)
  return {
    items: payload.items ?? [],
    total: payload.total ?? 0,
    page: payload.page ?? query.page,
    pageSize: payload.pageSize ?? query.pageSize,
    pages: payload.pages ?? 0,
    counts: payload.counts ?? { all: 0, notSent: 0, sent: 0, unsubscribed: 0, bounced: 0 },
  }
}

export const listLeadSources = async (): Promise<LeadSourceCount[]> => {
  const payload = await requestJson<{ items?: LeadSourceCount[] }>('/leads/sources')
  return payload.items ?? []
}

export const addLead = (lead: { email: string; source: string; note: string }): Promise<Lead> => (
  requestJson<Lead>('/leads', { method: 'POST', body: JSON.stringify(lead) })
)

export const updateLead = (id: string, lead: { source: string; note: string }): Promise<Lead> => (
  requestJson<Lead>(`/leads/${encodeURIComponent(id)}`, { method: 'PATCH', body: JSON.stringify(lead) })
)

export const importLeads = (text: string, source: string): Promise<LeadImportResult> => (
  requestJson<LeadImportResult>('/leads/import', { method: 'POST', body: JSON.stringify({ text, source }) })
)

export const deleteLeads = async (ids: string[]): Promise<number> => {
  const payload = await requestJson<{ deleted?: number }>('/leads/delete', { method: 'POST', body: JSON.stringify({ ids }) })
  return payload.deleted ?? 0
}

export const getLeadSettings = (): Promise<LeadSettings> => requestJson<LeadSettings>('/leads/settings')

export const saveLeadSettings = (dailyLimit: number): Promise<LeadSettings> => (
  requestJson<LeadSettings>('/leads/settings', { method: 'PUT', body: JSON.stringify({ dailyLimit }) })
)

export const previewLeadSend = (selection: LeadSelection): Promise<LeadSendPreview> => (
  requestJson<LeadSendPreview>('/leads/send/preview', { method: 'POST', body: JSON.stringify(selection) })
)

export const sendToLeads = (request: LeadSendRequest): Promise<MassEmailBatch> => (
  requestJson<MassEmailBatch>('/leads/send', { method: 'POST', body: JSON.stringify(request) })
)
