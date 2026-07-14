import type {
  CreateEmbedSessionRequest,
  CreateEmbedSessionResponse,
  CreateEmbedTicketRequest,
  EmbedTicketDetail,
  EmbedTicketListResponse,
} from '../types'

const apiBaseUrl = import.meta.env.VITE_API_BASE_URL ?? '/api'

const endpoint = (path: string): string => `${apiBaseUrl.replace(/\/$/, '')}${path}`

// embed 会话 token 只保存在内存里（模块级变量），不落 localStorage：iframe 场景下会话本来就
// 是短期的，页面刷新会重新走一遍 session 初始化流程，没有必要持久化，也避免和后台登录态混淆。
let sessionToken: string | null = null

export const setEmbedSessionToken = (token: string | null): void => {
  sessionToken = token
}

type EmbedErrorPayload = {
  message?: string
}

// authHeaders 供 requestJson 和附件二进制下载共用同一份鉴权头拼装逻辑。
const authHeaders = (): HeadersInit => (sessionToken ? { Authorization: `Bearer ${sessionToken}` } : {})

const requestJson = async <T>(path: string, options: RequestInit = {}): Promise<T> => {
  // FormData 请求体不能手动设置 Content-Type：浏览器需要自己生成带 boundary 的
  // multipart/form-data 值，手动设置反而会丢失 boundary 导致后端解析失败。
  const isFormData = typeof FormData !== 'undefined' && options.body instanceof FormData
  let response: Response
  try {
    response = await fetch(endpoint(path), {
      ...options,
      headers: {
        Accept: 'application/json',
        ...(isFormData ? {} : { 'Content-Type': 'application/json' }),
        ...authHeaders(),
        ...(options.headers ?? {}),
      },
    })
  } catch (error) {
    throw new Error('embed.tickets.errors.network')
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) as T & EmbedErrorPayload : ({} as T & EmbedErrorPayload)

  if (!response.ok) {
    throw new Error(payload.message ?? 'embed.tickets.errors.request')
  }

  return payload
}

export const createEmbedSession = async (
  request: CreateEmbedSessionRequest,
): Promise<CreateEmbedSessionResponse> => (
  requestJson<CreateEmbedSessionResponse>('/embed/tickets/session', {
    method: 'POST',
    body: JSON.stringify(request),
  })
)

export const listEmbedTickets = async (): Promise<EmbedTicketListResponse> => (
  requestJson<EmbedTicketListResponse>('/embed/tickets')
)

// createEmbedTicket 没有图片时继续走旧的 JSON 请求体（兼容第一版接口行为）；
// 选了图片时改用 multipart/form-data 一次性提交 manualEmail/title/body/images。
export const createEmbedTicket = async (
  request: CreateEmbedTicketRequest,
  images: File[] = [],
): Promise<EmbedTicketDetail> => {
  if (images.length === 0) {
    return requestJson<EmbedTicketDetail>('/embed/tickets', {
      method: 'POST',
      body: JSON.stringify(request),
    })
  }

  const form = new FormData()
  form.set('manualEmail', request.manualEmail)
  form.set('title', request.title)
  form.set('body', request.body)
  form.set('category', request.category)
  form.set('priority', request.priority)
  for (const image of images) {
    form.append('images', image)
  }
  return requestJson<EmbedTicketDetail>('/embed/tickets', {
    method: 'POST',
    body: form,
  })
}

export const getEmbedTicket = async (id: string): Promise<EmbedTicketDetail> => (
  requestJson<EmbedTicketDetail>(`/embed/tickets/${encodeURIComponent(id)}`)
)

export const replyEmbedTicket = async (id: string, body: string): Promise<EmbedTicketDetail> => (
  requestJson<EmbedTicketDetail>(`/embed/tickets/${encodeURIComponent(id)}/messages`, {
    method: 'POST',
    body: JSON.stringify({ body }),
  })
)

// fetchEmbedAttachmentBlob 用当前 embed session token 拉取附件二进制内容。<img> 标签无法附带
// Authorization 请求头，所以图片必须先用 fetch 取回 blob，再由调用方通过 URL.createObjectURL
// 转成可以赋给 <img src> 的临时地址（并在不再需要时 revokeObjectURL 释放）。
export const fetchEmbedAttachmentBlob = async (id: string): Promise<Blob> => {
  let response: Response
  try {
    response = await fetch(endpoint(`/embed/tickets/attachments/${encodeURIComponent(id)}`), {
      headers: authHeaders(),
    })
  } catch (error) {
    throw new Error('embed.tickets.errors.network')
  }
  if (!response.ok) {
    throw new Error('embed.tickets.errors.attachmentLoadFailed')
  }
  return response.blob()
}
