/** 工单状态：open 待回复、pending 用户追问待回复、replied 已回复、closed 已关闭。 */
export type TicketStatus = 'open' | 'pending' | 'replied' | 'closed'

export const ticketStatuses: TicketStatus[] = ['open', 'pending', 'replied', 'closed']

export interface AdminTicket {
  id: string
  title: string
  category: string
  status: TicketStatus
  /** 用户填的联系方式（可选）。 */
  contact: string
  /** 用户所在的平台：sub2api / newapi。 */
  platform: string
  siteUserId: string
  siteUsername: string
  siteEmail: string
  /** 用户的 TransitLink 版本和系统。 */
  clientInfo: string
  /** 最后一条消息是谁发的：customer / admin。 */
  lastAuthor: 'customer' | 'admin'
  lastMessageAt: string
  createdAt: string
  updatedAt: string
}

export interface TicketAttachment {
  id: string
  originalName: string
  contentType: string
  sizeBytes: number
  createdAt: string
}

export interface TicketMessage {
  id: string
  authorType: 'customer' | 'admin'
  authorName: string
  body: string
  createdAt: string
  attachments: TicketAttachment[]
}

export interface AdminTicketDetail extends AdminTicket {
  messages: TicketMessage[]
}

export interface TicketStatusCounts {
  open: number
  pending: number
  replied: number
  closed: number
}

export interface AdminTicketList {
  items: AdminTicket[]
  total: number
  page: number
  pageSize: number
  totalPages: number
  counts: TicketStatusCounts
}
