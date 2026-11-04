import type { AdminTicketDetail, AdminTicketList, TicketStatus, TicketStatusCounts } from '../types/tickets'
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

const send = async (path: string, options: RequestInit = {}): Promise<Response> => {
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: { Accept: 'application/json', ...authHeaders(), ...(options.headers ?? {}) },
    })
  } catch {
    throw new Error('admin.tickets.errors.network')
  }
  if (!response.ok) {
    let payload: ErrorPayload = {}
    try {
      payload = await response.json() as ErrorPayload
    } catch {
      // 不是 JSON（例如反向代理的错误页）：用通用提示。
    }
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.tickets.errors.request')
  }
  return response
}

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  const response = await send(path, options)
  try {
    return await response.json() as T
  } catch {
    throw new Error('admin.tickets.errors.request')
  }
}

const jsonBody = (value: unknown): RequestInit => ({
  body: JSON.stringify(value),
  headers: { 'Content-Type': 'application/json' },
})

export interface TicketListQuery {
  status: TicketStatus | ''
  page: number
  pageSize: number
}

export const listTickets = async (query: TicketListQuery): Promise<AdminTicketList> => {
  const params = new URLSearchParams({ page: String(query.page), pageSize: String(query.pageSize) })
  if (query.status) params.set('status', query.status)
  return requestJson<AdminTicketList>(`/tickets?${params.toString()}`)
}

/** 各状态的工单数（菜单上的提醒用）。 */
export const getTicketCounts = async (): Promise<TicketStatusCounts> => requestJson<TicketStatusCounts>('/tickets/counts')

export const getTicket = async (id: string): Promise<AdminTicketDetail> =>
  requestJson<AdminTicketDetail>(`/tickets/${encodeURIComponent(id)}`)

export const replyTicket = async (id: string, body: string): Promise<AdminTicketDetail> =>
  requestJson<AdminTicketDetail>(`/tickets/${encodeURIComponent(id)}/messages`, { method: 'POST', ...jsonBody({ body }) })

export const setTicketStatus = async (id: string, status: TicketStatus): Promise<AdminTicketDetail> =>
  requestJson<AdminTicketDetail>(`/tickets/${encodeURIComponent(id)}/status`, { method: 'PUT', ...jsonBody({ status }) })

/** 下载附件图片（需要登录，不能直接用图片地址）。 */
export const fetchTicketAttachment = async (id: string): Promise<Blob> => {
  const response = await send(`/tickets/attachments/${encodeURIComponent(id)}`, { headers: { Accept: 'image/*' } })
  return response.blob()
}
