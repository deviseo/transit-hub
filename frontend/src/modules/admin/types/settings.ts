export type NotificationChannel = 'dingtalk' | 'feishu' | 'telegram'

export type TestNotificationChannelPayload = {
  channel: NotificationChannel
  webhook?: string
  secret?: string
  telegramBotToken?: string
  telegramChatId?: string
  telegramProxyUrl?: string
}

export type TestNotificationChannelResponse = {
  success: boolean
  message: string
}

export type NotificationChannelSettings = {
  dingtalk: DingtalkChannelSettings[]
  feishu: WebhookChannelSettings[]
  telegram: TelegramChannelSettings[]
}

export type StrategySettings = {
  enableRefreshInterval: boolean
  refreshInterval: number
  enableBalanceWarning: boolean
  defaultBalanceThreshold: number
  balanceNotifyBotIds: string[]
  balanceTemplate: string
  enableMultiplierAlert: boolean
  multiplierNotifyBotIds: string[]
  multiplierTemplate: string
}

export type DingtalkChannelSettings = {
  id: string
  name: string
  enabled: boolean
  webhook: string
  secret: string
}

export type WebhookChannelSettings = {
  id: string
  name: string
  enabled: boolean
  webhook: string
  secret: string
}

export type TelegramChannelSettings = {
  id: string
  name: string
  enabled: boolean
  botToken: string
  chatId: string
  proxyUrl: string
}

export type SmtpTlsMode = 'implicit' | 'starttls'

export type SmtpSettings = {
  host: string
  port: number
  username: string
  fromEmail: string
  fromName: string
  tlsMode: SmtpTlsMode
  passwordConfigured: boolean
  updatedAt: string | null
}

export type SaveSmtpSettingsPayload = {
  host: string
  port: number
  username: string
  password?: string
  fromEmail: string
  fromName: string
  tlsMode: SmtpTlsMode
}

export type TestSmtpEmailPayload = {
  recipientEmail: string
}

export type TestSmtpEmailResponse = {
  success: boolean
  message: string
}

export type EmailTemplate = {
  id: string
  name: string
  subject: string
  htmlBody: string
  isBuiltIn: boolean
  createdAt: string | null
  updatedAt: string | null
}

export type SaveEmailTemplatePayload = {
  name: string
  subject: string
  htmlBody: string
}

export type TestEmailTemplatePayload = {
  recipientEmail: string
}

export type TestEmailTemplateResponse = {
  success: boolean
  message: string
}
