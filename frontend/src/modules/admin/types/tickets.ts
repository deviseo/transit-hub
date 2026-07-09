export type TicketStatus = 'open' | 'pending' | 'replied' | 'closed'

export interface AdminTicketListItem {
  id: string
  title: string
  status: TicketStatus
  manualEmail: string
  category: string
  priority: string
  sub2apiUserId: string
  sub2apiEmail: string
  sub2apiRole: string
  sub2apiSrcHost: string
  lastMessageAt: string
  createdAt: string
}

export interface TicketAttachment {
  id: string
  originalName: string
  contentType: string
  sizeBytes: number
  createdAt: string
}

export interface TicketMessageView {
  id: string
  authorType: 'customer' | 'admin'
  authorName: string
  body: string
  createdAt: string
  attachments: TicketAttachment[]
}

export interface AdminTicketDetail extends AdminTicketListItem {
  sub2apiSrcUrl: string
  messages: TicketMessageView[]
}

export interface AdminTicketListResponse {
  items: AdminTicketListItem[]
  total: number
  page: number
  pageSize: number
  totalPages: number
}

export interface TicketsQuery {
  page: number
  pageSize: number
  status: string
}

export type TicketEmbedTemplate = 'default' | 'minimal' | 'support'

export interface TicketEmbedConfig {
  // enabled/allowedSrcHost 保留用于兼容旧响应形状，第二阶段起 UI 不再展示也不再允许修改；
  // 后端恒返回 enabled=true、allowedSrcHost=''。
  enabled: boolean
  embedToken: string
  allowedSrcHost: string
  embedUrl: string
  template: TicketEmbedTemplate
  maxImagesPerTicket: number
  categoryOptions: string[]
  priorityOptions: string[]
}

export interface UpdateTicketEmbedConfigRequest {
  template: TicketEmbedTemplate
  maxImagesPerTicket?: number
  categoryOptions?: string[]
  priorityOptions?: string[]
}

// Sub2apiRechargeHistoryItem 是充值/余额变动历史中的单条记录。
export interface Sub2apiRechargeHistoryItem {
  id: string
  type: string
  amount?: number
  note?: string
  createdAt?: string
}

// Sub2apiUserProfile 是后台"Sub2API 用户资料"只读弹窗的数据。身份快照字段
// （sub2apiUserId/Email/Role/SrcHost/SrcUrl）始终可用；余额/总充值/注册时间/充值记录尽量通过
// 当前 workspace 的 Sub2API admin 会话实时查询获得，*Available 为 false 时前端必须展示
// "暂不可用"（并可结合 remoteUnavailableReason 展示具体原因），不能当作错误处理。
export interface Sub2apiUserProfile {
  sub2apiUserId: string
  sub2apiEmail: string
  sub2apiRole: string
  sub2apiSrcHost: string
  sub2apiSrcUrl: string

  balanceAvailable: boolean
  balance?: number

  totalRechargedAvailable: boolean
  totalRecharged?: number

  registeredAtAvailable: boolean
  registeredAt?: string

  rechargeHistoryAvailable: boolean
  rechargeHistory?: Sub2apiRechargeHistoryItem[]

  // 以下字段后端第一版不存在，均为可选，来自 Sub2API admin 用户详情的实时查询结果。
  username?: string
  status?: string
  concurrency?: number
  rpmLimit?: number
  frozenBalance?: number
  lastUsedAt?: string

  // remoteUnavailableReason 是实时字段不可用时的 i18n key，全部实时字段都成功获取时为空。
  remoteUnavailableReason?: string
}
