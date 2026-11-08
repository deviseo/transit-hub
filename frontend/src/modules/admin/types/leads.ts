/** 线索状态（后端按发送记录和退订名单算出来）。 */
export type LeadStatus = 'not_sent' | 'sent' | 'unsubscribed' | 'bounced'

export const LEAD_STATUSES: LeadStatus[] = ['not_sent', 'sent', 'unsubscribed', 'bounced']

/** 筛选「没填来源」的线索时用的来源值。 */
export const LEAD_NO_SOURCE = '__none__'

export interface Lead {
  id: string
  email: string
  source: string
  note: string
  status: LeadStatus
  sentCount: number
  lastSentAt?: string
  createdAt: string
}

export interface LeadStatusCounts {
  all: number
  notSent: number
  sent: number
  unsubscribed: number
  bounced: number
}

export interface LeadListQuery {
  page: number
  pageSize: number
  source: string
  status: '' | LeadStatus
  search: string
}

export interface LeadListResponse {
  items: Lead[]
  total: number
  page: number
  pageSize: number
  pages: number
  counts: LeadStatusCounts
}

export interface LeadSourceCount {
  source: string
  count: number
}

export interface LeadImportResult {
  added: number
  duplicates: number
  invalid: number
  suppressed: number
}

export interface LeadSettings {
  dailyLimit: number
  sentLast24h: number
  remaining: number
}

/** 要发给哪些线索：勾选的，或当前筛选条件下的全部（已退订、退信的总是跳过）。 */
export type LeadSelection =
  | { mode: 'selected'; ids: string[] }
  | { mode: 'filter'; source: string; status: '' | LeadStatus; search: string }

export interface LeadSendPreview {
  sendable: number
  skipped: number
  dailyLimit: number
  remaining: number
}

export interface LeadSendRequest {
  templateId: string
  selection: LeadSelection
  publicBaseUrl: string
  adLabel: boolean
  requestId: string
}
