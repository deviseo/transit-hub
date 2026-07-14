export type EmbedTicketStatus = 'open' | 'pending' | 'replied' | 'closed'

export interface EmbedTicketListItem {
  id: string
  title: string
  status: EmbedTicketStatus
  manualEmail: string
  category: string
  priority: string
  lastMessageAt: string
  createdAt: string
  updatedAt: string
}

export interface EmbedTicketListResponse {
  items: EmbedTicketListItem[]
}

export interface EmbedTicketAttachment {
  id: string
  originalName: string
  contentType: string
  sizeBytes: number
  createdAt: string
}

export interface EmbedTicketMessage {
  id: string
  authorType: 'customer' | 'admin'
  authorName: string
  body: string
  createdAt: string
  attachments: EmbedTicketAttachment[]
}

export interface EmbedTicketDetail extends EmbedTicketListItem {
  messages: EmbedTicketMessage[]
}

export interface CreateEmbedSessionRequest {
  embedToken: string
  sub2apiToken: string
  urlUserId: string
  srcHost: string
  srcUrl: string
}

export type TicketEmbedTemplate = 'default' | 'minimal' | 'support'

export interface CreateEmbedSessionResponse {
  sessionToken: string
  template: TicketEmbedTemplate
  maxImagesPerTicket: number
  categoryOptions: string[]
  priorityOptions: string[]
}

export interface CreateEmbedTicketRequest {
  manualEmail: string
  title: string
  body: string
  category: string
  priority: string
}
