import type {
  EmailTemplate,
  NotificationChannelSettings,
  OutboundProxySettings,
  SaveEmailTemplatePayload,
  SaveOutboundProxyPayload,
  SaveSmtpSettingsPayload,
  SmtpSettings,
  StrategySettings,
  TestNotificationChannelPayload,
  TestNotificationChannelResponse,
  TestOutboundProxyPayload,
  TestOutboundProxyResponse,
  TestSmtpEmailPayload,
  TestSmtpEmailResponse,
  TestEmailTemplatePayload,
  TestEmailTemplateResponse,
} from '../types/settings'
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
    throw new Error('admin.settings.errors.network')
  }

  const text = await response.text()
  const payload = text ? JSON.parse(text) as T & AdminErrorPayload : ({} as T & AdminErrorPayload)

  if (!response.ok) {
    if (isUnauthorizedApiResponse(response.status, payload)) {
      handleAuthExpired()
      throw new Error(authUnauthorizedErrorKey)
    }
    throw new Error(payload.message ?? 'admin.settings.errors.request')
  }

  return payload
}

export const testNotificationChannel = async (
  payload: TestNotificationChannelPayload,
): Promise<TestNotificationChannelResponse> => (
  requestJson<TestNotificationChannelResponse>('/settings/notification-channels/test', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
)

export const getStrategySettings = async (): Promise<StrategySettings> => (
  requestJson<StrategySettings>('/settings/strategy')
)

export const saveStrategySettings = async (settings: StrategySettings): Promise<StrategySettings> => (
  requestJson<StrategySettings>('/settings/strategy', {
    method: 'PUT',
    body: JSON.stringify(settings),
  })
)

export const getNotificationChannelSettings = async (): Promise<NotificationChannelSettings> => (
  requestJson<NotificationChannelSettings>('/settings/notification-channels')
)

export const saveNotificationChannelSettings = async (
  settings: NotificationChannelSettings,
): Promise<NotificationChannelSettings> => (
  requestJson<NotificationChannelSettings>('/settings/notification-channels', {
    method: 'PUT',
    body: JSON.stringify(settings),
  })
)

export const getEmailTemplates = async (): Promise<EmailTemplate[]> => (
  requestJson<EmailTemplate[]>('/settings/email-templates')
)

export const createEmailTemplate = async (payload: SaveEmailTemplatePayload): Promise<EmailTemplate> => (
  requestJson<EmailTemplate>('/settings/email-templates', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
)

export const updateEmailTemplate = async (id: string, payload: SaveEmailTemplatePayload): Promise<EmailTemplate> => (
  requestJson<EmailTemplate>(`/settings/email-templates/${encodeURIComponent(id)}`, {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
)

export const deleteEmailTemplate = async (id: string): Promise<Record<string, never>> => (
  requestJson<Record<string, never>>(`/settings/email-templates/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
)

export const testEmailTemplate = async (
  id: string,
  payload: TestEmailTemplatePayload,
): Promise<TestEmailTemplateResponse> => (
  requestJson<TestEmailTemplateResponse>(`/settings/email-templates/${encodeURIComponent(id)}/test-email`, {
    method: 'POST',
    body: JSON.stringify(payload),
  })
)

export const getSmtpSettings = async (): Promise<SmtpSettings> => (
  requestJson<SmtpSettings>('/settings/smtp')
)

export const saveSmtpSettings = async (payload: SaveSmtpSettingsPayload): Promise<SmtpSettings> => (
  requestJson<SmtpSettings>('/settings/smtp', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
)

export const testSmtpEmail = async (payload: TestSmtpEmailPayload): Promise<TestSmtpEmailResponse> => (
  requestJson<TestSmtpEmailResponse>('/settings/smtp/test-email', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
)

// 出站代理是系统级设置（对整个 TransitHub 生效），接口在 /system 下，只有系统管理员可以修改。
export const getOutboundProxy = async (): Promise<OutboundProxySettings> => (
  requestJson<OutboundProxySettings>('/system/outbound-proxy')
)

export const saveOutboundProxy = async (payload: SaveOutboundProxyPayload): Promise<OutboundProxySettings> => (
  requestJson<OutboundProxySettings>('/system/outbound-proxy', {
    method: 'PUT',
    body: JSON.stringify(payload),
  })
)

export const testOutboundProxy = async (payload: TestOutboundProxyPayload): Promise<TestOutboundProxyResponse> => (
  requestJson<TestOutboundProxyResponse>('/system/outbound-proxy/test', {
    method: 'POST',
    body: JSON.stringify(payload),
  })
)
