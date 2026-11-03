export type NotificationChannel = 'dingtalk' | 'wecom' | 'qq' | 'feishu' | 'telegram'
export type NotificationTemplateFormat = 'text' | 'markdown' | 'html'

export type TestNotificationChannelPayload = {
  channel: NotificationChannel
  webhook?: string
  secret?: string
  telegramBotToken?: string
  telegramChatId?: string
  telegramProxyUrl?: string
  qqAppId?: string
  qqClientSecret?: string
  qqUserOpenId?: string
  qqGroupOpenId?: string
}

export type TestNotificationChannelResponse = {
  success: boolean
  message: string
}

export type NotificationChannelSettings = {
  dingtalk: DingtalkChannelSettings[]
  wecom: WebhookChannelSettings[]
  qq: QQChannelSettings[]
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
  balanceTemplateFormat?: NotificationTemplateFormat
  enableMultiplierAlert: boolean
  multiplierNotifyBotIds: string[]
  multiplierTemplate: string
  multiplierTemplateFormat?: NotificationTemplateFormat
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

export type QQChannelSettings = {
  id: string
  name: string
  enabled: boolean
  appId: string
  clientSecret: string
  userOpenId: string
  groupOpenId?: string
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

// 出站代理生效方式：沿用启动环境变量（从未保存过配置）/ 直连 / 使用代理。
export type OutboundProxyMode = 'environment' | 'direct' | 'proxy'

export type OutboundProxySettings = {
  configured: boolean
  enabled: boolean
  // 密码已脱敏为 ***，原样提交时后端沿用已保存的密码
  proxyUrl: string
  bypass: string
  mode: OutboundProxyMode
  canEdit: boolean
  environment: {
    proxyUrl: string
    bypass: string
  }
  updatedAt?: string
}

export type SaveOutboundProxyPayload = {
  enabled: boolean
  proxyUrl: string
  bypass: string
}

export type TestOutboundProxyPayload = SaveOutboundProxyPayload & {
  targetUrl: string
}

export type OutboundProxyFailure = 'dns' | 'proxy' | 'tls' | 'timeout' | 'refused' | 'network'

export type TestOutboundProxyResponse = {
  ok: boolean
  route: 'proxy' | 'direct'
  statusCode?: number
  durationMs: number
  failure?: OutboundProxyFailure
}
