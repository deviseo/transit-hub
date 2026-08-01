<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Plus, Save, Loader2, CheckCircle2, MessageSquare, Send, Trash2, Timer, AlertTriangle, TrendingUp, Info, Mail, RefreshCw, ServerCog, Power, X } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import EmailTemplatesPanel from '../components/settings/EmailTemplatesPanel.vue'
import NotificationTemplateEditor from '../components/settings/NotificationTemplateEditor.vue'
import {
  getSystemRestartStatus,
  getSystemUpgradeStatus,
  isTransientSystemApiError,
  startSystemRestart,
  startSystemUpgrade,
} from '../api/system'
import type { SystemRestartStatusResponse, SystemUpgradeStatusResponse } from '../api/system'
import {
  getNotificationChannelSettings,
  getSmtpSettings,
  getStrategySettings,
  saveNotificationChannelSettings,
  saveSmtpSettings,
  saveStrategySettings,
  testNotificationChannel,
  testSmtpEmail,
} from '../api/settings'
import type { NotificationChannel, NotificationChannelSettings, NotificationTemplateFormat, SmtpSettings, SmtpTlsMode, StrategySettings, TestNotificationChannelPayload } from '../types/settings'

const { t } = useI18n()

// Tab Settings
const activeTab = ref<'strategy' | 'channels' | 'email' | 'system'>('strategy')

// Save loading states
const isSavingStrategy = ref(false)
const showSuccessStrategy = ref(false)
const isLoadingStrategy = ref(false)
const errorStrategy = ref('')

const isSavingChannels = ref(false)
const showSuccessChannels = ref(false)
const isLoadingChannels = ref(false)
const errorChannels = ref('')

// === Tab 1: Strategy ===
const enableRefreshInterval = ref(false)
const refreshInterval = ref('60')
const minimumRefreshInterval = 60

const enableBalanceWarning = ref(false)
const defaultBalanceThreshold = ref('10.00')
const balanceSelectedBots = ref<string[]>([])
const defaultBalanceTemplate = computed(() => t('admin.settings.sections.templates.balanceDefaultTemplate', {
  siteName: '{siteName}',
  balance: '{balance}',
  threshold: '{threshold}',
}))
const defaultMultiplierTemplate = computed(() => t('admin.settings.sections.templates.multiplierDefaultTemplate', {
  siteName: '{siteName}',
  groupName: '{groupName}',
  oldRate: '{oldRate}',
  newRate: '{newRate}',
  changeDirection: '{changeDirection}',
}))
const normalizeBuiltInTemplate = (template?: string) => (template?.trim() ?? '').replace(/>\s+</g, '><')
const legacyBalanceTemplates = new Set([
  '【余额预警】{siteName} 站点余额（CNY）已不足 {threshold} 元，当前余额为 {balance} 元。',
  '[Balance warning] {siteName} balance (CNY) is below {threshold}; current balance is {balance}.',
  '<div style="border-left:4px solid #f59e0b;background:rgba(245,158,11,0.12);padding:16px;border-radius:6px"><div style="font-size:18px;font-weight:700;color:#f59e0b">🔴 余额预警</div><p style="margin:10px 0 0">上游站点 <strong style="color:#3b82f6">{siteName}</strong> 的可用余额已低于预警阈值。</p><p style="margin:10px 0 0">💰 当前余额: <strong style="color:#ef4444">¥{balance}</strong><br>⚠️ 预警阈值: <strong>¥{threshold}</strong></p><p style="margin:10px 0 0">请及时检查并充值，避免服务中断。</p></div>',
  '<div style="border-left:4px solid #f59e0b;background:rgba(245,158,11,0.12);padding:16px;border-radius:6px"><div style="font-size:18px;font-weight:700;color:#f59e0b">🔴 Balance warning</div><p style="margin:10px 0 0">The available balance for <strong style="color:#3b82f6">{siteName}</strong> is below the warning threshold.</p><p style="margin:10px 0 0">💰 Current balance: <strong style="color:#ef4444">¥{balance}</strong><br>⚠️ Warning threshold: <strong>¥{threshold}</strong></p><p style="margin:10px 0 0">Please review and recharge the upstream account to avoid service interruption.</p></div>',
].map(normalizeBuiltInTemplate))
const legacyMultiplierTemplates = new Set([
  '【倍率变更】{siteName} 的 {groupName} 分组倍率已{changeDirection}：{oldRate}x -> {newRate}x。',
  '[Multiplier change] {siteName} / {groupName} {changeDirection}: {oldRate}x -> {newRate}x.',
  '<div style="border-left:4px solid #3b82f6;background:rgba(59,130,246,0.12);padding:16px;border-radius:6px"><div style="font-size:18px;font-weight:700;color:#3b82f6">🟠 倍率变更预警</div><p style="margin:10px 0 0">上游站点 <strong style="color:#3b82f6">{siteName}</strong> 的分组 <strong style="color:#8b5cf6">{groupName}</strong> 倍率已<strong style="color:#f59e0b">{changeDirection}</strong>。</p><p style="margin:10px 0 0">▫️ 原倍率: <strong>{oldRate}x</strong><br>🔸 新倍率: <strong style="color:#f59e0b">{newRate}x</strong></p><p style="margin:10px 0 0">🔎 请确认成本变化，并检查下游定价策略。</p></div>',
  '<div style="border-left:4px solid #3b82f6;background:rgba(59,130,246,0.12);padding:16px;border-radius:6px"><div style="font-size:18px;font-weight:700;color:#3b82f6">🟠 Multiplier change warning</div><p style="margin:10px 0 0">The <strong style="color:#8b5cf6">{groupName}</strong> multiplier on <strong style="color:#3b82f6">{siteName}</strong> has <strong style="color:#f59e0b">{changeDirection}</strong>.</p><p style="margin:10px 0 0">▫️ Previous rate: <strong>{oldRate}x</strong><br>🔸 New rate: <strong style="color:#f59e0b">{newRate}x</strong></p><p style="margin:10px 0 0">🔎 Review the cost change and confirm whether downstream pricing needs adjustment.</p></div>',
].map(normalizeBuiltInTemplate))
const shouldUseDefaultTemplate = (template: string | undefined, legacyTemplates: ReadonlySet<string>) => {
  const normalized = normalizeBuiltInTemplate(template)
  return normalized === '' || legacyTemplates.has(normalized)
}
const balanceTemplate = ref(defaultBalanceTemplate.value)
const balanceTemplateFormat = ref<NotificationTemplateFormat>('markdown')

const enableMultiplierAlert = ref(false)
const multiplierSelectedBots = ref<string[]>([])
const multiplierTemplate = ref(defaultMultiplierTemplate.value)
const multiplierTemplateFormat = ref<NotificationTemplateFormat>('markdown')

const normalizeTemplateFormat = (format?: string): NotificationTemplateFormat => (
  format === 'markdown' || format === 'html' ? format : 'text'
)

const balanceTemplateVariables = computed(() => [
  { token: '{siteName}', label: t('admin.settings.varSiteName') },
  { token: '{balance}', label: t('admin.settings.varBalance') },
  { token: '{threshold}', label: t('admin.settings.varThreshold') },
])
const balancePreviewValues = computed(() => ({
  '{siteName}': t('admin.settings.templateEditor.samples.siteName'),
  '{balance}': t('admin.settings.templateEditor.samples.balance'),
  '{threshold}': t('admin.settings.templateEditor.samples.threshold'),
}))
const multiplierTemplateVariables = computed(() => [
  { token: '{siteName}', label: t('admin.settings.varSiteName') },
  { token: '{groupName}', label: t('admin.settings.varGroupName') },
  { token: '{oldRate}', label: t('admin.settings.varOldRate') },
  { token: '{newRate}', label: t('admin.settings.varNewRate') },
  { token: '{changeDirection}', label: t('admin.settings.varChangeDirection') },
])
const multiplierPreviewValues = computed(() => ({
  '{siteName}': t('admin.settings.templateEditor.samples.siteName'),
  '{groupName}': t('admin.settings.templateEditor.samples.groupName'),
  '{oldRate}': t('admin.settings.templateEditor.samples.oldRate'),
  '{newRate}': t('admin.settings.templateEditor.samples.newRate'),
  '{changeDirection}': t('admin.settings.templateEditor.samples.changeDirection'),
}))
// === Tab 2: Channels ===
const activeChannelTab = ref<NotificationChannel>('dingtalk')

type WebhookBotForm = {
  id: string
  name: string
  webhook: string
  secret: string
}

type TelegramBotForm = {
  id: string
  name: string
  botToken: string
  chatId: string
  proxyUrl: string
}

type QQBotForm = {
  id: string
  name: string
  appId: string
  clientSecret: string
  userOpenId: string
  groupOpenId: string
}

const dingtalkBots = ref<WebhookBotForm[]>([])
const wecomBots = ref<WebhookBotForm[]>([])
const qqBots = ref<QQBotForm[]>([])
const feishuBots = ref<WebhookBotForm[]>([])
const telegramBots = ref<TelegramBotForm[]>([])

const addBot = (type: NotificationChannel) => {
  if (type === 'dingtalk') dingtalkBots.value.push({ id: uniqueId(), name: '', webhook: '', secret: '' })
  if (type === 'wecom') wecomBots.value.push({ id: uniqueId(), name: '', webhook: '', secret: '' })
  if (type === 'qq') qqBots.value.push({ id: uniqueId(), name: '', appId: '', clientSecret: '', userOpenId: '', groupOpenId: '' })
  if (type === 'feishu') feishuBots.value.push({ id: uniqueId(), name: '', webhook: '', secret: '' })
  if (type === 'telegram') telegramBots.value.push({ id: uniqueId(), name: '', botToken: '', chatId: '', proxyUrl: '' })
}

const removeBot = (type: NotificationChannel, index: number) => {
  if (type === 'dingtalk') dingtalkBots.value.splice(index, 1)
  if (type === 'wecom') wecomBots.value.splice(index, 1)
  if (type === 'qq') qqBots.value.splice(index, 1)
  if (type === 'feishu') feishuBots.value.splice(index, 1)
  if (type === 'telegram') telegramBots.value.splice(index, 1)
}

const saveStrategy = async () => {
  if (isSavingStrategy.value) return
  isSavingStrategy.value = true
  errorStrategy.value = ''
  showSuccessStrategy.value = false
  try {
    applyStrategySettings(await saveStrategySettings(currentStrategySettings()))
    showSuccessStrategy.value = true
    setTimeout(() => { showSuccessStrategy.value = false }, 3000)
  } catch (error) {
    errorStrategy.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isSavingStrategy.value = false
  }
}

const applyStrategySettings = (settings: StrategySettings) => {
  enableRefreshInterval.value = settings.enableRefreshInterval
  refreshInterval.value = String(Math.max(settings.refreshInterval, minimumRefreshInterval))
  enableBalanceWarning.value = settings.enableBalanceWarning
  defaultBalanceThreshold.value = String(settings.defaultBalanceThreshold || 10)
  balanceSelectedBots.value = settings.balanceNotifyBotIds ?? []
  const usesDefaultBalanceTemplate = shouldUseDefaultTemplate(settings.balanceTemplate, legacyBalanceTemplates)
  balanceTemplate.value = usesDefaultBalanceTemplate ? defaultBalanceTemplate.value : settings.balanceTemplate
  balanceTemplateFormat.value = usesDefaultBalanceTemplate ? 'markdown' : normalizeTemplateFormat(settings.balanceTemplateFormat)
  enableMultiplierAlert.value = settings.enableMultiplierAlert
  multiplierSelectedBots.value = settings.multiplierNotifyBotIds ?? []
  const usesDefaultMultiplierTemplate = shouldUseDefaultTemplate(settings.multiplierTemplate, legacyMultiplierTemplates)
  multiplierTemplate.value = usesDefaultMultiplierTemplate ? defaultMultiplierTemplate.value : settings.multiplierTemplate
  multiplierTemplateFormat.value = usesDefaultMultiplierTemplate ? 'markdown' : normalizeTemplateFormat(settings.multiplierTemplateFormat)
}

const currentStrategySettings = (): StrategySettings => ({
  enableRefreshInterval: enableRefreshInterval.value,
  refreshInterval: Math.max(Number.parseInt(refreshInterval.value, 10) || minimumRefreshInterval, minimumRefreshInterval),
  enableBalanceWarning: enableBalanceWarning.value,
  defaultBalanceThreshold: Number.parseFloat(defaultBalanceThreshold.value) || 10,
  balanceNotifyBotIds: balanceSelectedBots.value,
  balanceTemplate: balanceTemplate.value.trim(),
  balanceTemplateFormat: balanceTemplateFormat.value,
  enableMultiplierAlert: enableMultiplierAlert.value,
  multiplierNotifyBotIds: multiplierSelectedBots.value,
  multiplierTemplate: multiplierTemplate.value.trim(),
  multiplierTemplateFormat: multiplierTemplateFormat.value,
})

const loadStrategy = async () => {
  if (isLoadingStrategy.value) return
  isLoadingStrategy.value = true
  errorStrategy.value = ''
  try {
    applyStrategySettings(await getStrategySettings())
  } catch (error) {
    errorStrategy.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isLoadingStrategy.value = false
  }
}

const saveChannels = async () => {
  if (isSavingChannels.value) return
  isSavingChannels.value = true
  errorChannels.value = ''
  showSuccessChannels.value = false
  try {
    applyNotificationChannelSettings(await saveNotificationChannelSettings(currentNotificationChannelSettings()))
    showSuccessChannels.value = true
    setTimeout(() => { showSuccessChannels.value = false }, 3000)
  } catch (error) {
    errorChannels.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isSavingChannels.value = false
  }
}

const testingBotId = ref<string | null>(null)
const successBotId = ref<string | null>(null)
const errorBotId = ref<string | null>(null)
const errorBotMessage = ref('')

const uniqueId = () => `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`

const applyNotificationChannelSettings = (settings: NotificationChannelSettings) => {
  dingtalkBots.value = (settings.dingtalk ?? []).map(bot => ({
    id: bot.id || uniqueId(),
    name: bot.name,
    webhook: bot.webhook,
    secret: bot.secret,
  }))
  wecomBots.value = (settings.wecom ?? []).map(bot => ({
    id: bot.id || uniqueId(),
    name: bot.name,
    webhook: bot.webhook,
    secret: '',
  }))
  qqBots.value = (settings.qq ?? []).map(bot => ({
    id: bot.id || uniqueId(),
    name: bot.name,
    appId: bot.appId,
    clientSecret: bot.clientSecret,
    userOpenId: bot.userOpenId ?? '',
    groupOpenId: bot.groupOpenId ?? '',
  }))
  feishuBots.value = (settings.feishu ?? []).map(bot => ({
    id: bot.id || uniqueId(),
    name: bot.name,
    webhook: bot.webhook,
    secret: bot.secret,
  }))
  telegramBots.value = (settings.telegram ?? []).map(bot => ({
    id: bot.id || uniqueId(),
    name: bot.name,
    botToken: bot.botToken,
    chatId: bot.chatId,
    proxyUrl: bot.proxyUrl,
  }))
}

const currentNotificationChannelSettings = (): NotificationChannelSettings => ({
  dingtalk: dingtalkBots.value.map(bot => ({
    id: bot.id,
    name: bot.name.trim(),
    enabled: true,
    webhook: bot.webhook.trim(),
    secret: bot.secret.trim(),
  })),
  wecom: wecomBots.value.map(bot => ({
    id: bot.id,
    name: bot.name.trim(),
    enabled: true,
    webhook: bot.webhook.trim(),
    secret: '',
  })),
  qq: qqBots.value.map(bot => ({
    id: bot.id,
    name: bot.name.trim(),
    enabled: true,
    appId: bot.appId.trim(),
    clientSecret: bot.clientSecret.trim(),
    userOpenId: bot.userOpenId.trim(),
    groupOpenId: bot.groupOpenId.trim(),
  })),
  feishu: feishuBots.value.map(bot => ({
    id: bot.id,
    name: bot.name.trim(),
    enabled: true,
    webhook: bot.webhook.trim(),
    secret: bot.secret.trim(),
  })),
  telegram: telegramBots.value.map(bot => ({
    id: bot.id,
    name: bot.name.trim(),
    enabled: true,
    botToken: bot.botToken.trim(),
    chatId: bot.chatId.trim(),
    proxyUrl: bot.proxyUrl.trim(),
  })),
})

const loadChannels = async () => {
  if (isLoadingChannels.value) return
  isLoadingChannels.value = true
  errorChannels.value = ''
  try {
    applyNotificationChannelSettings(await getNotificationChannelSettings())
  } catch (error) {
    errorChannels.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isLoadingChannels.value = false
  }
}

const testBot = async (channel: NotificationChannel, id: string) => {
  if (testingBotId.value) return
  const payload = testPayload(channel, id)
  if (!payload) return
  testingBotId.value = id
  errorBotId.value = null
  errorBotMessage.value = ''
  try {
    await testNotificationChannel(payload)
    successBotId.value = id
    setTimeout(() => { successBotId.value = null }, 2000)
  } catch (error) {
    errorBotId.value = id
    errorBotMessage.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    testingBotId.value = null
  }
}

const testPayload = (channel: NotificationChannel, id: string): TestNotificationChannelPayload | null => {
  if (channel === 'qq') {
    const bot = qqBots.value.find(item => item.id === id)
    if (!bot) return null
    return {
      channel,
      qqAppId: bot.appId.trim(),
      qqClientSecret: bot.clientSecret.trim(),
      qqUserOpenId: bot.userOpenId.trim(),
    }
  }
  if (channel === 'telegram') {
    const bot = telegramBots.value.find(item => item.id === id)
    if (!bot) return null
    return { channel, telegramBotToken: bot.botToken.trim(), telegramChatId: bot.chatId.trim(), telegramProxyUrl: bot.proxyUrl.trim() }
  }
  const bot = channel === 'dingtalk'
    ? dingtalkBots.value.find(item => item.id === id)
    : channel === 'wecom'
      ? wecomBots.value.find(item => item.id === id)
      : feishuBots.value.find(item => item.id === id)
  if (!bot) return null
  return { channel, webhook: bot.webhook.trim(), secret: channel === 'wecom' ? '' : bot.secret.trim() }
}

const allBots = computed(() => [...dingtalkBots.value, ...wecomBots.value, ...qqBots.value, ...feishuBots.value, ...telegramBots.value])
const hasBots = computed(() => allBots.value.length > 0)

const toggleBalanceBot = (botId: string) => {
  const idx = balanceSelectedBots.value.indexOf(botId)
  if (idx >= 0) balanceSelectedBots.value.splice(idx, 1)
  else balanceSelectedBots.value.push(botId)
}
const toggleMultiplierBot = (botId: string) => {
  const idx = multiplierSelectedBots.value.indexOf(botId)
  if (idx >= 0) multiplierSelectedBots.value.splice(idx, 1)
  else multiplierSelectedBots.value.push(botId)
}

// === Tab 3: Email (SMTP) ===
const isSavingSmtp = ref(false)
const showSuccessSmtp = ref(false)
const isLoadingSmtp = ref(false)
const errorSmtp = ref('')

const isTestingSmtp = ref(false)
const errorSmtpTest = ref('')
const successSmtpTest = ref(false)

const smtpHost = ref('')
const smtpPort = ref('587')
const smtpUsername = ref('')
const smtpPassword = ref('')
const smtpFromEmail = ref('')
const smtpFromName = ref('')
const smtpTlsMode = ref<SmtpTlsMode>('starttls')
const smtpTestRecipient = ref('')
const smtpPasswordConfigured = ref(false)

type SmtpBaseline = {
  host: string
  port: number
  username: string
  fromEmail: string
  fromName: string
  tlsMode: SmtpTlsMode
}

const smtpBaseline = ref<SmtpBaseline | null>(null)

// smtpPortNumber 严格解析端口原始字符串：必须是纯十进制数字（不含符号、小数点、空白），
// 且落在 1-65535 范围内，否则视为非法（null）。刻意不用 `Number.parseInt(...) || 587` 之类的
// 兜底——那会把空字符串、非法字符串和 0 都静默救回 587，掩盖真实的表单校验缺口。
// 允许形如 "0587" 的前导零输入：按十进制解析为 587，行为确定且在此注明。
const smtpPortNumber = computed<number | null>(() => {
  const raw = smtpPort.value.trim()
  if (!/^\d+$/.test(raw)) return null
  const parsed = Number.parseInt(raw, 10)
  if (parsed < 1 || parsed > 65535) return null
  return parsed
})

const smtpPortInvalid = computed(() => smtpPortNumber.value === null)

const currentSmtpBaseline = (): SmtpBaseline => ({
  host: smtpHost.value.trim(),
  // 非法端口没有合法数值可用于比较；用 -1 这个越界哨兵值确保它永远不等于 baseline 的合法端口，
  // 从而让 smtpDirty 恒为 true（非法输入必须阻止“视为未变更”）。
  port: smtpPortNumber.value ?? -1,
  username: smtpUsername.value.trim(),
  fromEmail: smtpFromEmail.value.trim(),
  fromName: smtpFromName.value.trim(),
  tlsMode: smtpTlsMode.value,
})

const smtpDirty = computed(() => {
  if (smtpPassword.value.trim() !== '') return true
  if (smtpPortInvalid.value) return true
  if (!smtpBaseline.value) return true
  const current = currentSmtpBaseline()
  return (
    current.host !== smtpBaseline.value.host ||
    current.port !== smtpBaseline.value.port ||
    current.username !== smtpBaseline.value.username ||
    current.fromEmail !== smtpBaseline.value.fromEmail ||
    current.fromName !== smtpBaseline.value.fromName ||
    current.tlsMode !== smtpBaseline.value.tlsMode
  )
})

const applySmtpSettings = (settings: SmtpSettings) => {
  smtpHost.value = settings.host
  // 后端 no-record 默认值和已保存记录都保证 port 是合法整数；这里只展示后端返回的值，
  // 不再用 `|| 587` 掩盖 API 合同缺口。
  smtpPort.value = String(settings.port)
  smtpUsername.value = settings.username
  smtpFromEmail.value = settings.fromEmail
  smtpFromName.value = settings.fromName
  smtpTlsMode.value = settings.tlsMode
  smtpPasswordConfigured.value = settings.passwordConfigured
  smtpPassword.value = ''
  smtpBaseline.value = {
    host: settings.host,
    port: settings.port,
    username: settings.username,
    fromEmail: settings.fromEmail,
    fromName: settings.fromName,
    tlsMode: settings.tlsMode,
  }
}

const loadSmtp = async () => {
  if (isLoadingSmtp.value) return
  isLoadingSmtp.value = true
  errorSmtp.value = ''
  try {
    applySmtpSettings(await getSmtpSettings())
  } catch (error) {
    errorSmtp.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isLoadingSmtp.value = false
  }
}

const saveSmtp = async () => {
  if (isSavingSmtp.value) return
  if (smtpPortInvalid.value) {
    // 防御性兜底：正常情况下保存按钮已经因为 smtpPortInvalid 被禁用，这里避免任何
    // 程序化调用绕过 UI 禁用状态直接把非法端口发给后端。
    errorSmtp.value = 'admin.settings.smtp.errors.invalidPort'
    return
  }
  isSavingSmtp.value = true
  errorSmtp.value = ''
  showSuccessSmtp.value = false
  try {
    // 密码字节保真：非空白密码必须原样发送 smtpPassword.value，不能 trim/归一化，
    // 否则合法密码里的前导/尾随空格会被后端加密前的处理悄悄破坏。
    const rawPassword = smtpPassword.value
    const payload = {
      host: smtpHost.value.trim(),
      port: smtpPortNumber.value as number,
      username: smtpUsername.value.trim(),
      ...(rawPassword.trim() !== '' ? { password: rawPassword } : {}),
      fromEmail: smtpFromEmail.value.trim(),
      fromName: smtpFromName.value.trim(),
      tlsMode: smtpTlsMode.value,
    }
    applySmtpSettings(await saveSmtpSettings(payload))
    showSuccessSmtp.value = true
    setTimeout(() => { showSuccessSmtp.value = false }, 3000)
  } catch (error) {
    errorSmtp.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isSavingSmtp.value = false
  }
}

const testSmtp = async () => {
  if (isTestingSmtp.value || smtpDirty.value) return
  isTestingSmtp.value = true
  errorSmtpTest.value = ''
  successSmtpTest.value = false
  try {
    await testSmtpEmail({ recipientEmail: smtpTestRecipient.value.trim() })
    successSmtpTest.value = true
    setTimeout(() => { successSmtpTest.value = false }, 3000)
  } catch (error) {
    errorSmtpTest.value = error instanceof Error ? error.message : 'admin.settings.errors.unknown'
  } finally {
    isTestingSmtp.value = false
  }
}

// === Tab 4: System upgrade ===
const upgradeStatus = ref<SystemUpgradeStatusResponse>({ state: 'idle' })
const isStartingUpgrade = ref(false)
const upgradeResult = ref<{ state: 'succeeded' | 'failed'; message: string; output: string } | null>(null)
const isUpgradeBusy = computed(() => (
  isStartingUpgrade.value || upgradeStatus.value.state === 'starting' || upgradeStatus.value.state === 'running'
))
const upgradeStatusKey = computed(() => `admin.settings.upgrade.statuses.${upgradeStatus.value.state}`)

let upgradePollTimer: number | undefined
let upgradePollDeadline = 0
let currentUpgradeRequestedAt = ''

const clearUpgradePoll = () => {
  if (upgradePollTimer !== undefined) {
    window.clearTimeout(upgradePollTimer)
    upgradePollTimer = undefined
  }
}

const localizeSystemError = (error: unknown): string => {
  const message = error instanceof Error ? error.message : 'admin.system.errors.request'
  return message.startsWith('admin.') ? t(message) : message
}

const scheduleUpgradePoll = (delay = 2000) => {
  clearUpgradePoll()
  upgradePollTimer = window.setTimeout(() => { void pollUpgradeStatus() }, delay)
}

const statusIsOlderThanRequest = (status: SystemUpgradeStatusResponse): boolean => {
  if (!currentUpgradeRequestedAt || status.state === 'starting' || status.state === 'running') return false
  if (status.state === 'idle' || !status.startedAt) return true
  const requestedAt = Date.parse(currentUpgradeRequestedAt)
  const startedAt = Date.parse(status.startedAt)
  return Number.isFinite(requestedAt) && Number.isFinite(startedAt) && startedAt < requestedAt
}

const showUpgradeFailure = (message: string, output = '') => {
  clearUpgradePoll()
  upgradeResult.value = { state: 'failed', message, output }
}

async function pollUpgradeStatus() {
  try {
    const status = await getSystemUpgradeStatus()
    if (statusIsOlderThanRequest(status)) {
      if (Date.now() < upgradePollDeadline) {
        scheduleUpgradePoll()
      } else {
        showUpgradeFailure(t('admin.settings.upgrade.timeout'))
      }
      return
    }

    upgradeStatus.value = status
    if (status.state === 'starting' || status.state === 'running') {
      scheduleUpgradePoll()
      return
    }
    if (status.state === 'succeeded') {
      clearUpgradePoll()
      upgradeResult.value = {
        state: 'succeeded',
        message: t('admin.settings.upgrade.successMessage'),
        output: '',
      }
      currentUpgradeRequestedAt = ''
      return
    }
    if (status.state === 'failed') {
      showUpgradeFailure(t('admin.settings.upgrade.failedMessage'), status.output ?? '')
      currentUpgradeRequestedAt = ''
    }
  } catch (error) {
    if (isTransientSystemApiError(error) && Date.now() < upgradePollDeadline) {
      scheduleUpgradePoll()
      return
    }
    showUpgradeFailure(localizeSystemError(error))
  }
}

const beginUpgradePolling = () => {
  upgradePollDeadline = Date.now() + 20 * 60 * 1000
  scheduleUpgradePoll(500)
}

const startUpgrade = async () => {
  if (isUpgradeBusy.value || isRestartBusy.value) return
  isStartingUpgrade.value = true
  upgradeResult.value = null
  try {
    const response = await startSystemUpgrade()
    currentUpgradeRequestedAt = response.requestedAt
    upgradeStatus.value = { state: response.state }
    beginUpgradePolling()
  } catch (error) {
    showUpgradeFailure(localizeSystemError(error))
  } finally {
    isStartingUpgrade.value = false
  }
}

const restoreUpgradeStatus = async () => {
  try {
    const status = await getSystemUpgradeStatus()
    upgradeStatus.value = status
    if (status.state === 'starting' || status.state === 'running') {
      beginUpgradePolling()
    }
  } catch {
    upgradeStatus.value = { state: 'idle' }
  }
}

const closeUpgradeResult = () => {
  if (upgradeResult.value?.state === 'succeeded') {
    window.location.reload()
    return
  }
  upgradeResult.value = null
}

// === Manual backend service restart ===
const restartStatus = ref<SystemRestartStatusResponse>({ state: 'idle' })
const isStartingRestart = ref(false)
const isRestartConfirmOpen = ref(false)
const restartResult = ref<{ state: 'succeeded' | 'failed'; message: string; output: string } | null>(null)
const isRestartBusy = computed(() => (
  isStartingRestart.value || restartStatus.value.state === 'starting' || restartStatus.value.state === 'running'
))
const restartStatusKey = computed(() => `admin.settings.restart.statuses.${restartStatus.value.state}`)

let restartPollTimer: number | undefined
let restartPollDeadline = 0
let currentRestartRequestedAt = ''

const clearRestartPoll = () => {
  if (restartPollTimer !== undefined) {
    window.clearTimeout(restartPollTimer)
    restartPollTimer = undefined
  }
}

const scheduleRestartPoll = (delay = 2000) => {
  clearRestartPoll()
  restartPollTimer = window.setTimeout(() => { void pollRestartStatus() }, delay)
}

const restartStatusIsOlderThanRequest = (status: SystemRestartStatusResponse): boolean => {
  if (!currentRestartRequestedAt || status.state === 'starting' || status.state === 'running') return false
  if (status.state === 'idle' || !status.startedAt) return true
  const requestedAt = Date.parse(currentRestartRequestedAt)
  const startedAt = Date.parse(status.startedAt)
  return Number.isFinite(requestedAt) && Number.isFinite(startedAt) && startedAt < requestedAt
}

const showRestartFailure = (message: string, output = '') => {
  clearRestartPoll()
  restartResult.value = { state: 'failed', message, output }
}

const showRestartClientFailure = (message: string) => {
  restartStatus.value = { state: 'idle' }
  currentRestartRequestedAt = ''
  showRestartFailure(message)
}

async function pollRestartStatus() {
  try {
    const status = await getSystemRestartStatus()
    if (restartStatusIsOlderThanRequest(status)) {
      if (Date.now() < restartPollDeadline) {
        scheduleRestartPoll()
      } else {
        showRestartClientFailure(t('admin.settings.restart.timeout'))
      }
      return
    }

    restartStatus.value = status
    if (status.state === 'starting' || status.state === 'running') {
      scheduleRestartPoll()
      return
    }
    if (status.state === 'succeeded') {
      clearRestartPoll()
      restartResult.value = {
        state: 'succeeded',
        message: t('admin.settings.restart.successMessage'),
        output: '',
      }
      currentRestartRequestedAt = ''
      return
    }
    if (status.state === 'failed') {
      showRestartFailure(t('admin.settings.restart.failedMessage'), status.output ?? '')
      currentRestartRequestedAt = ''
    }
  } catch (error) {
    if (isTransientSystemApiError(error) && Date.now() < restartPollDeadline) {
      scheduleRestartPoll()
      return
    }
    showRestartClientFailure(localizeSystemError(error))
  }
}

const beginRestartPolling = () => {
  restartPollDeadline = Date.now() + 90 * 1000
  scheduleRestartPoll(500)
}

const openRestartConfirm = () => {
  if (isRestartBusy.value || isUpgradeBusy.value) return
  isRestartConfirmOpen.value = true
}

const closeRestartConfirm = () => {
  if (isStartingRestart.value) return
  isRestartConfirmOpen.value = false
}

const startRestart = async () => {
  if (isRestartBusy.value || isUpgradeBusy.value) return
  isRestartConfirmOpen.value = false
  isStartingRestart.value = true
  restartResult.value = null
  currentRestartRequestedAt = new Date().toISOString()
  restartStatus.value = { state: 'starting' }
  beginRestartPolling()
  try {
    const response = await startSystemRestart()
    currentRestartRequestedAt = response.requestedAt
    restartStatus.value = { state: response.state }
  } catch (error) {
    if (!isTransientSystemApiError(error)) {
      showRestartClientFailure(localizeSystemError(error))
    }
  } finally {
    isStartingRestart.value = false
  }
}

const restoreRestartStatus = async () => {
  try {
    const status = await getSystemRestartStatus()
    restartStatus.value = status
    if (status.state === 'starting' || status.state === 'running') {
      beginRestartPolling()
    }
  } catch {
    restartStatus.value = { state: 'idle' }
  }
}

const closeRestartResult = () => {
  if (restartResult.value?.state === 'succeeded') {
    window.location.reload()
    return
  }
  restartResult.value = null
}

onMounted(async () => {
  await loadChannels()
  void loadStrategy()
  void loadSmtp()
  void restoreUpgradeStatus()
  void restoreRestartStatus()
})

onBeforeUnmount(() => {
  clearUpgradePoll()
  clearRestartPoll()
})
</script>

<template>
  <div class="mx-auto max-w-6xl space-y-6 pb-12">
    <!-- Tab bar -->
    <div class="sticky top-0 z-10 -mx-3 mb-8 flex justify-start border-b border-border/40 bg-background/90 px-3 py-4 backdrop-blur-xl sm:-mx-6 sm:justify-center sm:px-6">
      <div class="inline-flex max-w-full overflow-x-auto rounded-lg border border-border/50 bg-surface-elevated p-1 shadow-sm" role="tablist" :aria-label="t('admin.menu.settings')">
        <button
          id="settings-tab-strategy"
          type="button"
          role="tab"
          :aria-selected="activeTab === 'strategy'"
          aria-controls="settings-panel-strategy"
          @click="activeTab = 'strategy'"
          class="relative whitespace-nowrap rounded-md px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:px-6"
          :class="activeTab === 'strategy' ? 'text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground hover:bg-surface/50'"
        >
          <div v-if="activeTab === 'strategy'" class="absolute inset-0 -z-10 rounded-md border border-border/50 bg-background shadow-sm"></div>
          <div class="flex items-center gap-2">
            <Timer class="w-4 h-4" />
            {{ t('admin.settings.tabs.strategy') }}
          </div>
        </button>
        <button
          id="settings-tab-channels"
          type="button"
          role="tab"
          :aria-selected="activeTab === 'channels'"
          aria-controls="settings-panel-channels"
          @click="activeTab = 'channels'"
          class="relative whitespace-nowrap rounded-md px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:px-6"
          :class="activeTab === 'channels' ? 'text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground hover:bg-surface/50'"
        >
          <div v-if="activeTab === 'channels'" class="absolute inset-0 -z-10 rounded-md border border-border/50 bg-background shadow-sm"></div>
          <div class="flex items-center gap-2">
            <MessageSquare class="w-4 h-4" />
            {{ t('admin.settings.tabs.channels') }}
          </div>
        </button>
        <button
          id="settings-tab-email"
          type="button"
          role="tab"
          :aria-selected="activeTab === 'email'"
          aria-controls="settings-panel-email"
          @click="activeTab = 'email'"
          class="relative whitespace-nowrap rounded-md px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:px-6"
          :class="activeTab === 'email' ? 'text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground hover:bg-surface/50'"
        >
          <div v-if="activeTab === 'email'" class="absolute inset-0 -z-10 rounded-md border border-border/50 bg-background shadow-sm"></div>
          <div class="flex items-center gap-2">
            <Mail class="w-4 h-4" />
            {{ t('admin.settings.tabs.email') }}
          </div>
        </button>
        <button
          id="settings-tab-system"
          type="button"
          role="tab"
          :aria-selected="activeTab === 'system'"
          aria-controls="settings-panel-system"
          @click="activeTab = 'system'"
          class="relative whitespace-nowrap rounded-md px-4 py-2 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary sm:px-6"
          :class="activeTab === 'system' ? 'text-foreground shadow-sm' : 'text-muted-foreground hover:text-foreground hover:bg-surface/50'"
        >
          <div v-if="activeTab === 'system'" class="absolute inset-0 -z-10 rounded-md border border-border/50 bg-background shadow-sm"></div>
          <div class="flex items-center gap-2">
            <ServerCog class="w-4 h-4" />
            {{ t('admin.settings.tabs.system') }}
          </div>
        </button>
      </div>
    </div>

    <div class="relative">
      <transition
        mode="out-in"
        enter-active-class="transition-[opacity,transform] duration-200 ease-out"
        enter-from-class="opacity-0 translate-y-2"
        enter-to-class="opacity-100 translate-y-0"
        leave-active-class="transition-[opacity,transform] duration-150 ease-in"
        leave-from-class="opacity-100 translate-y-0"
        leave-to-class="opacity-0 -translate-y-2"
      >
        <!-- ============================================ -->
        <!-- Strategy Tab                                 -->
        <!-- ============================================ -->
        <div v-if="activeTab === 'strategy'" id="settings-panel-strategy" class="space-y-5" role="tabpanel" aria-labelledby="settings-tab-strategy">
          <!-- Strategy Header + Save button -->
          <div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
            <div>
              <h3 class="text-lg font-semibold text-foreground">{{ t('admin.settings.tabs.strategy') }}</h3>
              <p class="text-sm text-muted-foreground mt-0.5">{{ t('admin.settings.strategyDescription') }}</p>
            </div>
            <Button :disabled="isSavingStrategy" @click="saveStrategy" class="min-w-[120px]">
              <Loader2 v-if="isSavingStrategy" class="h-4 w-4 animate-spin mr-2" />
              <CheckCircle2 v-else-if="showSuccessStrategy" class="h-4 w-4 mr-2 text-green-400" />
              <Save v-else class="h-4 w-4 mr-2" />
              {{ showSuccessStrategy ? t('admin.settings.saveSuccess') : (isSavingStrategy ? t('admin.settings.saving') : t('admin.settings.save')) }}
            </Button>
          </div>

          <p v-if="errorStrategy" class="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">
            {{ t(errorStrategy) }}
          </p>

          <!-- Card 1: Data Refresh -->
          <div class="rounded-2xl border border-border/50 bg-card shadow-sm overflow-hidden">
            <div class="p-5 flex items-start justify-between gap-4">
              <div class="flex items-start gap-3">
                <div class="p-2 bg-blue-500/10 text-blue-500 rounded-xl shrink-0 mt-0.5">
                  <Timer class="w-5 h-5" />
                </div>
                <div>
                  <h4 class="text-sm font-semibold text-foreground">{{ t('admin.settings.sections.basic.refreshInterval') }}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.sections.basic.refreshIntervalHelp') }}</p>
                </div>
              </div>
              <label class="relative inline-flex items-center cursor-pointer shrink-0 mt-1">
                <input type="checkbox" v-model="enableRefreshInterval" class="sr-only peer" :aria-label="t('admin.settings.sections.basic.refreshInterval')">
                <div class="peer h-6 w-11 rounded-full bg-surface-elevated peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-border after:bg-white after:transition-transform after:content-[''] peer-checked:after:translate-x-full peer-checked:after:border-white"></div>
              </label>
            </div>
            <div v-if="enableRefreshInterval" class="px-5 pb-5 pt-0">
              <div class="animate-in slide-in-from-top-2 fade-in duration-200 sm:pl-12">
                <div class="flex items-center gap-3 max-w-xs">
                  <Input type="number" v-model="refreshInterval" :min="minimumRefreshInterval" step="10" class="w-24" />
                  <span class="text-sm text-muted-foreground whitespace-nowrap">{{ t('admin.settings.sections.basic.seconds') }}</span>
                </div>
              </div>
            </div>
          </div>

          <!-- Card 2: Balance Warning -->
          <div class="rounded-2xl border border-border/50 bg-card shadow-sm overflow-hidden">
            <div class="p-5 flex items-start justify-between gap-4">
              <div class="flex items-start gap-3">
                <div class="p-2 bg-amber-500/10 text-amber-500 rounded-xl shrink-0 mt-0.5">
                  <AlertTriangle class="w-5 h-5" />
                </div>
                <div>
                  <h4 class="text-sm font-semibold text-foreground">{{ t('admin.settings.sections.thresholds.balanceWarning') }}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.sections.thresholds.balanceWarningHelp') }}</p>
                  <div v-if="!enableRefreshInterval" class="flex items-center gap-1.5 mt-1.5">
                    <Info class="w-3.5 h-3.5 text-blue-500 shrink-0" />
                    <span class="text-xs text-blue-500">{{ t('admin.settings.requiresRefresh') }}</span>
                  </div>
                </div>
              </div>
              <label class="relative inline-flex items-center cursor-pointer shrink-0 mt-1">
                <input type="checkbox" v-model="enableBalanceWarning" class="sr-only peer" :aria-label="t('admin.settings.sections.thresholds.balanceWarning')">
                <div class="peer h-6 w-11 rounded-full bg-surface-elevated peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-border after:bg-white after:transition-transform after:content-[''] peer-checked:after:translate-x-full peer-checked:after:border-white"></div>
              </label>
            </div>
            <div v-if="enableBalanceWarning" class="px-5 pb-5 pt-0">
              <div class="space-y-4 animate-in slide-in-from-top-2 fade-in duration-200 sm:pl-12">
                <!-- Threshold amount -->
                <div class="grid gap-1.5">
                  <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.balanceWarningAmount') }}</label>
                  <div class="relative max-w-xs">
                    <span class="absolute left-3 top-1/2 -translate-y-1/2 text-sm font-medium text-muted-foreground">¥</span>
                    <Input type="number" v-model="defaultBalanceThreshold" min="0" step="0.01" class="pl-8" />
                  </div>
                </div>

                <!-- Bot selector -->
                <div class="grid gap-1.5">
                  <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.notifyBots') }} <span class="text-destructive">*</span></label>
                  <div class="flex flex-wrap gap-2">
                    <button v-for="bot in allBots" :key="'bal-' + bot.id" type="button" :aria-pressed="balanceSelectedBots.includes(bot.id)" @click="toggleBalanceBot(bot.id)" class="flex select-none items-center gap-2 rounded-lg border px-3 py-1.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" :class="balanceSelectedBots.includes(bot.id) ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 hover:bg-surface/50'">
                      <MessageSquare class="w-3.5 h-3.5" />
                      <span class="text-sm">{{ bot.name || t('admin.settings.unnamedBot') }}</span>
                    </button>
                    <div v-if="!hasBots" class="text-sm text-muted-foreground italic py-1">
                      {{ t('admin.settings.noBotsConfigured') }}
                    </div>
                  </div>
                  <p v-if="balanceSelectedBots.length === 0 && hasBots" class="text-xs text-destructive mt-0.5">{{ t('admin.settings.mustSelectBot') }}</p>
                </div>

                <NotificationTemplateEditor
                  v-model="balanceTemplate"
                  v-model:format="balanceTemplateFormat"
                  :variables="balanceTemplateVariables"
                  :preview-values="balancePreviewValues"
                  :placeholder="t('admin.settings.sections.templates.balanceTemplatePlaceholder', { siteName: '{siteName}', balance: '{balance}', threshold: '{threshold}' })"
                />
              </div>
            </div>
          </div>

          <!-- Card 3: Multiplier Change Alert -->
          <div class="rounded-2xl border border-border/50 bg-card shadow-sm overflow-hidden">
            <div class="p-5 flex items-start justify-between gap-4">
              <div class="flex items-start gap-3">
                <div class="p-2 bg-purple-500/10 text-purple-500 rounded-xl shrink-0 mt-0.5">
                  <TrendingUp class="w-5 h-5" />
                </div>
                <div>
                  <h4 class="text-sm font-semibold text-foreground">{{ t('admin.settings.sections.thresholds.multiplierChangeWarning') }}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.sections.thresholds.multiplierChangeWarningHelp') }}</p>
                  <div v-if="!enableRefreshInterval" class="flex items-center gap-1.5 mt-1.5">
                    <Info class="w-3.5 h-3.5 text-blue-500 shrink-0" />
                    <span class="text-xs text-blue-500">{{ t('admin.settings.requiresRefresh') }}</span>
                  </div>
                </div>
              </div>
              <label class="relative inline-flex items-center cursor-pointer shrink-0 mt-1">
                <input type="checkbox" v-model="enableMultiplierAlert" class="sr-only peer" :aria-label="t('admin.settings.sections.thresholds.multiplierChangeWarning')">
                <div class="peer h-6 w-11 rounded-full bg-surface-elevated peer-checked:bg-primary peer-focus-visible:ring-2 peer-focus-visible:ring-primary peer-focus-visible:ring-offset-2 peer-focus-visible:ring-offset-background after:absolute after:left-[2px] after:top-[2px] after:h-5 after:w-5 after:rounded-full after:border after:border-border after:bg-white after:transition-transform after:content-[''] peer-checked:after:translate-x-full peer-checked:after:border-white"></div>
              </label>
            </div>
            <div v-if="enableMultiplierAlert" class="px-5 pb-5 pt-0">
              <div class="space-y-4 animate-in slide-in-from-top-2 fade-in duration-200 sm:pl-12">
                <!-- Bot selector -->
                <div class="grid gap-1.5">
                  <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.notifyBots') }} <span class="text-destructive">*</span></label>
                  <div class="flex flex-wrap gap-2">
                    <button v-for="bot in allBots" :key="'mul-' + bot.id" type="button" :aria-pressed="multiplierSelectedBots.includes(bot.id)" @click="toggleMultiplierBot(bot.id)" class="flex select-none items-center gap-2 rounded-lg border px-3 py-1.5 transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary" :class="multiplierSelectedBots.includes(bot.id) ? 'border-primary bg-primary/10 text-primary' : 'border-border/50 bg-surface/30 hover:bg-surface/50'">
                      <MessageSquare class="w-3.5 h-3.5" />
                      <span class="text-sm">{{ bot.name || t('admin.settings.unnamedBot') }}</span>
                    </button>
                    <div v-if="!hasBots" class="text-sm text-muted-foreground italic py-1">
                      {{ t('admin.settings.noBotsConfigured') }}
                    </div>
                  </div>
                  <p v-if="multiplierSelectedBots.length === 0 && hasBots" class="text-xs text-destructive mt-0.5">{{ t('admin.settings.mustSelectBot') }}</p>
                </div>

                <NotificationTemplateEditor
                  v-model="multiplierTemplate"
                  v-model:format="multiplierTemplateFormat"
                  :variables="multiplierTemplateVariables"
                  :preview-values="multiplierPreviewValues"
                  :placeholder="t('admin.settings.sections.templates.multiplierTemplatePlaceholder', { siteName: '{siteName}', groupName: '{groupName}', oldRate: '{oldRate}', newRate: '{newRate}' })"
                />

              </div>
            </div>
          </div>

        </div>

        <!-- ============================================ -->
        <!-- Channels Tab                                 -->
        <!-- ============================================ -->
        <section v-else-if="activeTab === 'channels'" id="settings-panel-channels" class="w-full overflow-hidden rounded-xl border border-border/50 bg-card shadow-sm" role="tabpanel" aria-labelledby="settings-tab-channels">
          <div class="p-6 border-b border-border/50 bg-surface/30 flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="p-2 bg-blue-500/10 text-blue-500 rounded-xl">
                <MessageSquare class="w-5 h-5" />
              </div>
              <div>
                <h3 class="text-lg font-semibold text-foreground">{{ t('admin.settings.sections.channels.title') }}</h3>
                <p class="text-sm text-muted-foreground">{{ t('admin.settings.sections.channels.description') }}</p>
              </div>
            </div>
            <Button :disabled="isSavingChannels || isLoadingChannels" @click="saveChannels" class="min-w-[120px]">
              <Loader2 v-if="isSavingChannels" class="h-4 w-4 animate-spin mr-2" />
              <CheckCircle2 v-else-if="showSuccessChannels" class="h-4 w-4 mr-2 text-green-400" />
              <Save v-else class="h-4 w-4 mr-2" />
              {{ showSuccessChannels ? t('admin.settings.saveSuccess') : (isSavingChannels ? t('admin.settings.saving') : t('admin.settings.save')) }}
            </Button>
          </div>
          <div class="p-6 space-y-6">
            <p v-if="isLoadingChannels" class="text-sm text-muted-foreground">{{ t('admin.settings.sections.channels.loading') }}</p>
            <p v-if="errorChannels" class="text-sm text-destructive">{{ t(errorChannels) }}</p>

            <!-- Channels Sub-tabs -->
            <div class="flex overflow-x-auto border-b border-border/30">
              <button
                @click="activeChannelTab = 'dingtalk'"
                class="shrink-0 px-6 py-3 text-sm font-medium transition-colors border-b-2"
                :class="activeChannelTab === 'dingtalk' ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
              >
                {{ t('admin.settings.sections.channels.dingtalk') }} ({{ dingtalkBots.length }})
              </button>
              <button
                @click="activeChannelTab = 'wecom'"
                class="shrink-0 px-6 py-3 text-sm font-medium transition-colors border-b-2"
                :class="activeChannelTab === 'wecom' ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
              >
                {{ t('admin.settings.sections.channels.wecom') }} ({{ wecomBots.length }})
              </button>
              <button
                @click="activeChannelTab = 'qq'"
                class="shrink-0 px-6 py-3 text-sm font-medium transition-colors border-b-2"
                :class="activeChannelTab === 'qq' ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
              >
                {{ t('admin.settings.sections.channels.qq') }} ({{ qqBots.length }})
              </button>
              <button
                @click="activeChannelTab = 'feishu'"
                class="shrink-0 px-6 py-3 text-sm font-medium transition-colors border-b-2"
                :class="activeChannelTab === 'feishu' ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
              >
                {{ t('admin.settings.sections.channels.feishu') }} ({{ feishuBots.length }})
              </button>
              <button
                @click="activeChannelTab = 'telegram'"
                class="shrink-0 px-6 py-3 text-sm font-medium transition-colors border-b-2"
                :class="activeChannelTab === 'telegram' ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'"
              >
                {{ t('admin.settings.sections.channels.telegram') }} ({{ telegramBots.length }})
              </button>
            </div>

            <!-- Dingtalk -->
            <div v-if="activeChannelTab === 'dingtalk'" class="space-y-4">
              <div class="flex items-center justify-between">
                <div>
                  <h4 class="font-medium text-foreground">{{ t('admin.settings.sections.channels.dingtalk') }}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.sections.channels.dingtalkHelp') }}</p>
                </div>
                <Button variant="secondary" size="sm" class="h-8" @click="addBot('dingtalk')">
                  <Plus class="h-3 w-3 mr-1.5" /> {{ t('admin.settings.addDingtalkBot') }}
                </Button>
              </div>
              <div class="grid md:grid-cols-2 gap-4">
                <div v-for="(bot, idx) in dingtalkBots" :key="bot.id" class="p-4 rounded-xl border border-border/50 bg-surface/20 relative group">
                  <Button variant="ghost" size="sm" class="absolute top-2 right-2 h-8 w-8 p-0 opacity-0 group-hover:opacity-100 transition-opacity text-red-500 hover:bg-red-500/10" @click="removeBot('dingtalk', idx)">
                    <Trash2 class="h-4 w-4" />
                  </Button>
                  <div class="grid gap-4 pr-10">
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.botNameLabel') }}</label>
                      <Input v-model="bot.name" :placeholder="t('admin.settings.botNameDingtalkPlaceholder')" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.webhookUrl') }}</label>
                      <Input type="url" v-model="bot.webhook" placeholder="https://oapi.dingtalk.com/robot/send?access_token=..." class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.secret') }}</label>
                      <Input type="password" v-model="bot.secret" placeholder="SEC..." class="h-8 text-sm" />
                    </div>
                    <div>
                      <Button variant="secondary" size="sm" class="h-8" :disabled="testingBotId === bot.id || !bot.webhook" @click="testBot('dingtalk', bot.id)">
                        <Loader2 v-if="testingBotId === bot.id" class="h-3 w-3 animate-spin mr-1.5" />
                        <CheckCircle2 v-else-if="successBotId === bot.id" class="h-3 w-3 mr-1.5 text-green-500" />
                        <Send v-else class="h-3 w-3 mr-1.5 text-muted-foreground" />
                        <span :class="{ 'text-green-500': successBotId === bot.id }">{{ successBotId === bot.id ? t('admin.settings.sections.channels.testConnectionSuccess') : t('admin.settings.sections.channels.testConnection') }}</span>
                      </Button>
                      <p v-if="errorBotId === bot.id" class="mt-2 text-xs text-destructive">{{ t(errorBotMessage) }}</p>
                    </div>
                  </div>
                </div>
              </div>
              <div v-if="dingtalkBots.length === 0" class="text-center py-6 text-sm text-muted-foreground border border-dashed border-border/50 rounded-xl">
                {{ t('admin.settings.emptyDingtalk') }}
              </div>
            </div>

            <!-- WeCom -->
            <div v-if="activeChannelTab === 'wecom'" class="space-y-4">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <h4 class="font-medium text-foreground">{{ t('admin.settings.sections.channels.wecom') }}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.sections.channels.wecomHelp') }}</p>
                </div>
                <Button variant="secondary" size="sm" class="h-8 shrink-0" @click="addBot('wecom')">
                  <Plus class="h-3 w-3 mr-1.5" /> {{ t('admin.settings.addWecomBot') }}
                </Button>
              </div>
              <div class="grid md:grid-cols-2 gap-4">
                <div v-for="(bot, idx) in wecomBots" :key="bot.id" class="p-4 rounded-xl border border-border/50 bg-surface/20 relative group">
                  <Button variant="ghost" size="sm" class="absolute top-2 right-2 h-8 w-8 p-0 opacity-0 group-hover:opacity-100 transition-opacity text-red-500 hover:bg-red-500/10" @click="removeBot('wecom', idx)">
                    <Trash2 class="h-4 w-4" />
                  </Button>
                  <div class="grid gap-4 pr-10">
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.botNameLabel') }}</label>
                      <Input v-model="bot.name" :placeholder="t('admin.settings.botNameWecomPlaceholder')" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.webhookUrl') }}</label>
                      <Input type="url" v-model="bot.webhook" placeholder="https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=..." class="h-8 text-sm" />
                    </div>
                    <div>
                      <Button variant="secondary" size="sm" class="h-8" :disabled="testingBotId === bot.id || !bot.webhook" @click="testBot('wecom', bot.id)">
                        <Loader2 v-if="testingBotId === bot.id" class="h-3 w-3 animate-spin mr-1.5" />
                        <CheckCircle2 v-else-if="successBotId === bot.id" class="h-3 w-3 mr-1.5 text-green-500" />
                        <Send v-else class="h-3 w-3 mr-1.5 text-muted-foreground" />
                        <span :class="{ 'text-green-500': successBotId === bot.id }">{{ successBotId === bot.id ? t('admin.settings.sections.channels.testConnectionSuccess') : t('admin.settings.sections.channels.testConnection') }}</span>
                      </Button>
                      <p v-if="errorBotId === bot.id" class="mt-2 text-xs text-destructive">{{ t(errorBotMessage) }}</p>
                    </div>
                  </div>
                </div>
              </div>
              <div v-if="wecomBots.length === 0" class="text-center py-6 text-sm text-muted-foreground border border-dashed border-border/50 rounded-xl">
                {{ t('admin.settings.emptyWecom') }}
              </div>
            </div>

            <!-- QQ -->
            <div v-if="activeChannelTab === 'qq'" class="space-y-4">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <h4 class="font-medium text-foreground">{{ t('admin.settings.sections.channels.qq') }}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.sections.channels.qqHelp') }}</p>
                </div>
                <Button variant="secondary" size="sm" class="h-8 shrink-0" @click="addBot('qq')">
                  <Plus class="h-3 w-3 mr-1.5" /> {{ t('admin.settings.addQQBot') }}
                </Button>
              </div>
              <div class="grid md:grid-cols-2 gap-4">
                <div v-for="(bot, idx) in qqBots" :key="bot.id" class="p-4 rounded-xl border border-border/50 bg-surface/20 relative group">
                  <Button variant="ghost" size="sm" class="absolute top-2 right-2 h-8 w-8 p-0 opacity-0 group-hover:opacity-100 transition-opacity text-red-500 hover:bg-red-500/10" @click="removeBot('qq', idx)">
                    <Trash2 class="h-4 w-4" />
                  </Button>
                  <div class="grid gap-4 pr-10">
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.botNameLabel') }}</label>
                      <Input v-model="bot.name" :placeholder="t('admin.settings.botNameQQPlaceholder')" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.appId') }}</label>
                      <Input v-model="bot.appId" :placeholder="t('admin.settings.sections.channels.appIdPlaceholder')" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.appSecret') }}</label>
                      <Input type="password" v-model="bot.clientSecret" :placeholder="t('admin.settings.sections.channels.appSecretPlaceholder')" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.userOpenId') }}</label>
                      <Input v-model="bot.userOpenId" :placeholder="t('admin.settings.sections.channels.userOpenIdPlaceholder')" class="h-8 text-sm" />
                      <p class="text-xs text-muted-foreground">{{ t('admin.settings.sections.channels.userOpenIdHelp') }}</p>
                    </div>
                    <div>
                      <Button variant="secondary" size="sm" class="h-8" :disabled="testingBotId === bot.id || !bot.appId || !bot.clientSecret || !bot.userOpenId" @click="testBot('qq', bot.id)">
                        <Loader2 v-if="testingBotId === bot.id" class="h-3 w-3 animate-spin mr-1.5" />
                        <CheckCircle2 v-else-if="successBotId === bot.id" class="h-3 w-3 mr-1.5 text-green-500" />
                        <Send v-else class="h-3 w-3 mr-1.5 text-muted-foreground" />
                        <span :class="{ 'text-green-500': successBotId === bot.id }">{{ successBotId === bot.id ? t('admin.settings.sections.channels.testConnectionSuccess') : t('admin.settings.sections.channels.testConnection') }}</span>
                      </Button>
                      <p v-if="errorBotId === bot.id" class="mt-2 text-xs text-destructive">{{ t(errorBotMessage) }}</p>
                    </div>
                  </div>
                </div>
              </div>
              <div v-if="qqBots.length === 0" class="text-center py-6 text-sm text-muted-foreground border border-dashed border-border/50 rounded-xl">
                {{ t('admin.settings.emptyQQ') }}
              </div>
            </div>

            <!-- Feishu -->
            <div v-if="activeChannelTab === 'feishu'" class="space-y-4">
              <div class="flex items-center justify-between">
                <div>
                  <h4 class="font-medium text-foreground">{{ t('admin.settings.sections.channels.feishu') }}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.sections.channels.feishuHelp') }}</p>
                </div>
                <Button variant="secondary" size="sm" class="h-8" @click="addBot('feishu')">
                  <Plus class="h-3 w-3 mr-1.5" /> {{ t('admin.settings.addFeishuBot') }}
                </Button>
              </div>
              <div class="grid md:grid-cols-2 gap-4">
                <div v-for="(bot, idx) in feishuBots" :key="bot.id" class="p-4 rounded-xl border border-border/50 bg-surface/20 relative group">
                  <Button variant="ghost" size="sm" class="absolute top-2 right-2 h-8 w-8 p-0 opacity-0 group-hover:opacity-100 transition-opacity text-red-500 hover:bg-red-500/10" @click="removeBot('feishu', idx)">
                    <Trash2 class="h-4 w-4" />
                  </Button>
                  <div class="grid gap-4 pr-10">
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.botNameLabel') }}</label>
                      <Input v-model="bot.name" :placeholder="t('admin.settings.botNameFeishuPlaceholder')" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.webhookUrl') }}</label>
                      <Input type="url" v-model="bot.webhook" placeholder="https://open.feishu.cn/open-apis/bot/v2/hook/..." class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.secret') }}</label>
                      <Input type="password" v-model="bot.secret" placeholder="..." class="h-8 text-sm" />
                    </div>
                    <div>
                      <Button variant="secondary" size="sm" class="h-8" :disabled="testingBotId === bot.id || !bot.webhook" @click="testBot('feishu', bot.id)">
                        <Loader2 v-if="testingBotId === bot.id" class="h-3 w-3 animate-spin mr-1.5" />
                        <CheckCircle2 v-else-if="successBotId === bot.id" class="h-3 w-3 mr-1.5 text-green-500" />
                        <Send v-else class="h-3 w-3 mr-1.5 text-muted-foreground" />
                        <span :class="{ 'text-green-500': successBotId === bot.id }">{{ successBotId === bot.id ? t('admin.settings.sections.channels.testConnectionSuccess') : t('admin.settings.sections.channels.testConnection') }}</span>
                      </Button>
                      <p v-if="errorBotId === bot.id" class="mt-2 text-xs text-destructive">{{ t(errorBotMessage) }}</p>
                    </div>
                  </div>
                </div>
              </div>
              <div v-if="feishuBots.length === 0" class="text-center py-6 text-sm text-muted-foreground border border-dashed border-border/50 rounded-xl">
                {{ t('admin.settings.emptyFeishu') }}
              </div>
            </div>

            <!-- Telegram -->
            <div v-if="activeChannelTab === 'telegram'" class="space-y-4">
              <div class="flex items-center justify-between">
                <div>
                  <h4 class="font-medium text-foreground">{{ t('admin.settings.sections.channels.telegram') }}</h4>
                  <p class="text-xs text-muted-foreground mt-0.5">{{ t('admin.settings.sections.channels.telegramHelp') }}</p>
                </div>
                <Button variant="secondary" size="sm" class="h-8" @click="addBot('telegram')">
                  <Plus class="h-3 w-3 mr-1.5" /> {{ t('admin.settings.addTelegramBot') }}
                </Button>
              </div>
              <div class="grid md:grid-cols-2 gap-4">
                <div v-for="(bot, idx) in telegramBots" :key="bot.id" class="p-4 rounded-xl border border-border/50 bg-surface/20 relative group">
                  <Button variant="ghost" size="sm" class="absolute top-2 right-2 h-8 w-8 p-0 opacity-0 group-hover:opacity-100 transition-opacity text-red-500 hover:bg-red-500/10" @click="removeBot('telegram', idx)">
                    <Trash2 class="h-4 w-4" />
                  </Button>
                  <div class="grid gap-4 pr-10">
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.botNameLabel') }}</label>
                      <Input v-model="bot.name" :placeholder="t('admin.settings.botNameTelegramPlaceholder')" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.botToken') }}</label>
                      <Input type="password" v-model="bot.botToken" placeholder="123456789:ABCdefGHIjklMNOpqrsTUVwxyz" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.chatId') }}</label>
                      <Input type="text" v-model="bot.chatId" placeholder="-1001234567890" class="h-8 text-sm" />
                    </div>
                    <div class="grid gap-2">
                      <label class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.sections.channels.proxyUrl') }}</label>
                      <Input type="url" v-model="bot.proxyUrl" :placeholder="t('admin.settings.sections.channels.proxyUrlPlaceholder')" class="h-8 text-sm" />
                      <p class="text-xs text-muted-foreground">{{ t('admin.settings.sections.channels.proxyUrlHelp') }}</p>
                    </div>
                    <div>
                      <Button variant="secondary" size="sm" class="h-8" :disabled="testingBotId === bot.id || !bot.botToken || !bot.chatId" @click="testBot('telegram', bot.id)">
                        <Loader2 v-if="testingBotId === bot.id" class="h-3 w-3 animate-spin mr-1.5" />
                        <CheckCircle2 v-else-if="successBotId === bot.id" class="h-3 w-3 mr-1.5 text-green-500" />
                        <Send v-else class="h-3 w-3 mr-1.5 text-muted-foreground" />
                        <span :class="{ 'text-green-500': successBotId === bot.id }">{{ successBotId === bot.id ? t('admin.settings.sections.channels.testConnectionSuccess') : t('admin.settings.sections.channels.testConnection') }}</span>
                      </Button>
                      <p v-if="errorBotId === bot.id" class="mt-2 text-xs text-destructive">{{ t(errorBotMessage) }}</p>
                    </div>
                  </div>
                </div>
              </div>
              <div v-if="telegramBots.length === 0" class="text-center py-6 text-sm text-muted-foreground border border-dashed border-border/50 rounded-xl">
                {{ t('admin.settings.emptyTelegram') }}
              </div>
            </div>

          </div>
        </section>

        <!-- ============================================ -->
        <!-- Email (SMTP) Tab                              -->
        <!-- ============================================ -->
        <div v-else-if="activeTab === 'email'" id="settings-panel-email" class="space-y-6" role="tabpanel" aria-labelledby="settings-tab-email">
        <section class="rounded-2xl border border-border/50 bg-card shadow-sm overflow-hidden w-full">
          <div class="p-6 border-b border-border/50 bg-surface/30 flex items-center justify-between">
            <div class="flex items-center gap-3">
              <div class="p-2 bg-blue-500/10 text-blue-500 rounded-xl">
                <Mail class="w-5 h-5" />
              </div>
              <div>
                <h3 class="text-lg font-semibold text-foreground">{{ t('admin.settings.smtp.title') }}</h3>
                <p class="text-sm text-muted-foreground">{{ t('admin.settings.smtp.description') }}</p>
              </div>
            </div>
            <Button :disabled="isSavingSmtp || isLoadingSmtp || smtpPortInvalid" @click="saveSmtp" class="min-w-[120px]">
              <Loader2 v-if="isSavingSmtp" class="h-4 w-4 animate-spin mr-2" />
              <CheckCircle2 v-else-if="showSuccessSmtp" class="h-4 w-4 mr-2 text-green-400" />
              <Save v-else class="h-4 w-4 mr-2" />
              {{ showSuccessSmtp ? t('admin.settings.smtp.saveSuccess') : (isSavingSmtp ? t('admin.settings.saving') : t('admin.settings.save')) }}
            </Button>
          </div>
          <div class="p-6 space-y-5">
            <p v-if="isLoadingSmtp" class="text-sm text-muted-foreground">{{ t('admin.settings.sections.channels.loading') }}</p>
            <p v-if="errorSmtp" class="rounded-xl border border-destructive/30 bg-destructive/10 px-4 py-3 text-sm text-destructive">{{ t(errorSmtp) }}</p>

            <div class="grid md:grid-cols-2 gap-4">
              <div class="grid gap-2">
                <label for="smtp-host" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.smtp.host') }}</label>
                <Input id="smtp-host" v-model="smtpHost" placeholder="smtp.example.com" class="h-9 text-sm" />
              </div>
              <div class="grid gap-2">
                <label for="smtp-port" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.smtp.port') }}</label>
                <Input
                  id="smtp-port"
                  type="number"
                  v-model="smtpPort"
                  placeholder="587"
                  class="h-9 text-sm"
                  :class="{ 'border-destructive focus:border-destructive': smtpPortInvalid }"
                />
                <p v-if="smtpPortInvalid" class="text-xs text-destructive">{{ t('admin.settings.smtp.errors.invalidPort') }}</p>
              </div>
              <div class="grid gap-2">
                <label for="smtp-tls-mode" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.smtp.tlsMode') }}</label>
                <select
                  id="smtp-tls-mode"
                  v-model="smtpTlsMode"
                  class="flex h-9 w-full rounded-lg border border-input bg-background px-3 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  <option value="starttls">{{ t('admin.settings.smtp.tlsStarttls') }}</option>
                  <option value="implicit">{{ t('admin.settings.smtp.tlsImplicit') }}</option>
                </select>
              </div>
              <div class="grid gap-2">
                <label for="smtp-username" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.smtp.username') }}</label>
                <Input id="smtp-username" v-model="smtpUsername" class="h-9 text-sm" />
              </div>
              <div class="grid gap-2">
                <label for="smtp-password" class="text-xs font-medium text-muted-foreground flex items-center gap-2">
                  {{ t('admin.settings.smtp.password') }}
                  <span
                    class="text-[11px] font-normal px-1.5 py-0.5 rounded"
                    :class="smtpPasswordConfigured ? 'bg-green-500/10 text-green-500' : 'bg-surface-elevated text-muted-foreground'"
                  >
                    {{ smtpPasswordConfigured ? t('admin.settings.smtp.passwordConfigured') : t('admin.settings.smtp.passwordNotConfigured') }}
                  </span>
                </label>
                <Input
                  id="smtp-password"
                  type="password"
                  v-model="smtpPassword"
                  :placeholder="smtpPasswordConfigured ? t('admin.settings.smtp.passwordKeepPlaceholder') : t('admin.settings.smtp.passwordNewPlaceholder')"
                  class="h-9 text-sm"
                />
              </div>
              <div class="grid gap-2">
                <label for="smtp-from-email" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.smtp.fromEmail') }}</label>
                <Input id="smtp-from-email" type="email" v-model="smtpFromEmail" class="h-9 text-sm" />
              </div>
              <div class="grid gap-2">
                <label for="smtp-from-name" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.smtp.fromName') }}</label>
                <Input id="smtp-from-name" v-model="smtpFromName" class="h-9 text-sm" />
              </div>
            </div>

            <div class="border-t border-border/40 pt-5 space-y-3">
              <div class="grid gap-2 max-w-sm">
                <label for="smtp-test-recipient" class="text-xs font-medium text-muted-foreground">{{ t('admin.settings.smtp.testRecipient') }}</label>
                <Input id="smtp-test-recipient" type="email" v-model="smtpTestRecipient" class="h-9 text-sm" />
              </div>
              <div class="flex items-center gap-3">
                <Button
                  variant="secondary"
                  :disabled="isTestingSmtp || smtpDirty || !smtpTestRecipient.trim()"
                  @click="testSmtp"
                >
                  <Loader2 v-if="isTestingSmtp" class="h-4 w-4 animate-spin mr-2" />
                  <CheckCircle2 v-else-if="successSmtpTest" class="h-4 w-4 mr-2 text-green-400" />
                  <Send v-else class="h-4 w-4 mr-2" />
                  {{ successSmtpTest ? t('admin.settings.smtp.testEmailSuccess') : t('admin.settings.smtp.testEmail') }}
                </Button>
                <p v-if="smtpDirty" class="text-xs text-amber-500 flex items-center gap-1.5">
                  <Info class="w-3.5 h-3.5" />
                  {{ t('admin.settings.smtp.dirtyBeforeTest') }}
                </p>
              </div>
              <p v-if="errorSmtpTest" class="text-xs text-destructive">{{ t(errorSmtpTest) }}</p>
            </div>
          </div>
        </section>
        <EmailTemplatesPanel />
        </div>

        <!-- ============================================ -->
        <!-- System Upgrade Tab                           -->
        <!-- ============================================ -->
        <div v-else id="settings-panel-system" class="space-y-4" role="tabpanel" aria-labelledby="settings-tab-system">
          <section class="w-full overflow-hidden rounded-xl border border-border/50 bg-card shadow-sm">
            <div class="flex flex-col gap-5 p-6 sm:flex-row sm:items-center sm:justify-between">
              <div class="flex min-w-0 items-center gap-3">
                <div class="rounded-lg bg-blue-500/10 p-2 text-blue-500">
                  <ServerCog class="h-5 w-5" />
                </div>
                <div class="min-w-0">
                  <h3 class="text-lg font-semibold text-foreground">{{ t('admin.settings.upgrade.title') }}</h3>
                  <div class="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
                    <span
                      class="h-2 w-2 rounded-full"
                      :class="isUpgradeBusy ? 'animate-pulse bg-blue-500' : (upgradeStatus.state === 'failed' ? 'bg-destructive' : (upgradeStatus.state === 'succeeded' ? 'bg-green-500' : 'bg-muted-foreground/50'))"
                    ></span>
                    <span>{{ t('admin.settings.upgrade.statusLabel') }}：{{ t(upgradeStatusKey) }}</span>
                  </div>
                </div>
              </div>
              <Button class="min-w-[156px]" :disabled="isUpgradeBusy || isRestartBusy" @click="startUpgrade">
                <Loader2 v-if="isUpgradeBusy" class="mr-2 h-4 w-4 animate-spin" />
                <RefreshCw v-else class="mr-2 h-4 w-4" />
                {{ isUpgradeBusy ? t('admin.settings.upgrade.running') : t('admin.settings.upgrade.action') }}
              </Button>
            </div>
          </section>

          <section class="w-full overflow-hidden rounded-xl border border-border/50 bg-card shadow-sm">
            <div class="flex flex-col gap-5 p-6 sm:flex-row sm:items-center sm:justify-between">
              <div class="flex min-w-0 items-center gap-3">
                <div class="rounded-lg bg-blue-500/10 p-2 text-blue-500">
                  <Power class="h-5 w-5" />
                </div>
                <div class="min-w-0">
                  <h3 class="text-lg font-semibold text-foreground">{{ t('admin.settings.restart.title') }}</h3>
                  <div class="mt-1 flex items-center gap-2 text-sm text-muted-foreground">
                    <span
                      class="h-2 w-2 rounded-full"
                      :class="isRestartBusy ? 'animate-pulse bg-blue-500' : (restartStatus.state === 'failed' ? 'bg-destructive' : (restartStatus.state === 'succeeded' ? 'bg-green-500' : 'bg-muted-foreground/50'))"
                    ></span>
                    <span>{{ t('admin.settings.restart.statusLabel') }}：{{ t(restartStatusKey) }}</span>
                  </div>
                </div>
              </div>
              <Button class="min-w-[156px]" :disabled="isRestartBusy || isUpgradeBusy" @click="openRestartConfirm">
                <Loader2 v-if="isRestartBusy" class="mr-2 h-4 w-4 animate-spin" />
                <Power v-else class="mr-2 h-4 w-4" />
                {{ isRestartBusy ? t('admin.settings.restart.running') : t('admin.settings.restart.action') }}
              </Button>
            </div>
          </section>
        </div>
      </transition>
    </div>

    <Teleport to="body">
      <div v-if="isRestartConfirmOpen" class="fixed inset-0 z-[180] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/80 backdrop-blur-sm"></div>
        <div
          class="relative z-10 w-full max-w-lg overflow-hidden rounded-xl border border-border/60 bg-card shadow-2xl"
          role="alertdialog"
          aria-modal="true"
          aria-labelledby="restart-confirm-title"
        >
          <div class="flex items-start gap-3 border-b border-border/50 px-5 py-4">
            <div class="rounded-lg bg-destructive/10 p-2 text-destructive">
              <AlertTriangle class="h-5 w-5" />
            </div>
            <h3 id="restart-confirm-title" class="pt-1 text-base font-semibold text-foreground">
              {{ t('admin.settings.restart.confirmTitle') }}
            </h3>
          </div>
          <div class="space-y-5 px-5 py-5">
            <p class="text-sm leading-6 text-muted-foreground">{{ t('admin.settings.restart.confirmMessage') }}</p>
            <div class="flex flex-col-reverse gap-2 sm:flex-row sm:justify-end">
              <Button variant="secondary" :disabled="isStartingRestart" @click="closeRestartConfirm">
                {{ t('admin.settings.restart.cancel') }}
              </Button>
              <Button :disabled="isStartingRestart" @click="startRestart">
                <Loader2 v-if="isStartingRestart" class="mr-2 h-4 w-4 animate-spin" />
                <Power v-else class="mr-2 h-4 w-4" />
                {{ t('admin.settings.restart.confirm') }}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <Teleport to="body">
      <div v-if="upgradeResult" class="fixed inset-0 z-[180] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/80 backdrop-blur-sm"></div>
        <div class="relative z-10 w-full max-w-lg overflow-hidden rounded-xl border border-border/60 bg-card shadow-2xl" role="dialog" aria-modal="true">
          <div class="flex items-start justify-between gap-4 border-b border-border/50 px-5 py-4">
            <div class="flex min-w-0 items-center gap-3">
              <div
                class="rounded-lg p-2"
                :class="upgradeResult.state === 'succeeded' ? 'bg-green-500/10 text-green-500' : 'bg-destructive/10 text-destructive'"
              >
                <CheckCircle2 v-if="upgradeResult.state === 'succeeded'" class="h-5 w-5" />
                <AlertTriangle v-else class="h-5 w-5" />
              </div>
              <h3 class="text-base font-semibold text-foreground">
                {{ upgradeResult.state === 'succeeded' ? t('admin.settings.upgrade.successTitle') : t('admin.settings.upgrade.failedTitle') }}
              </h3>
            </div>
            <button
              type="button"
              class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :aria-label="t('admin.settings.upgrade.close')"
              @click="closeUpgradeResult"
            >
              <X class="h-5 w-5" />
            </button>
          </div>
          <div class="space-y-4 px-5 py-5">
            <p class="text-sm leading-6 text-muted-foreground">{{ upgradeResult.message }}</p>
            <pre v-if="upgradeResult.output" class="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-destructive/20 bg-destructive/5 p-3 text-xs leading-5 text-foreground">{{ upgradeResult.output }}</pre>
            <div class="flex justify-end">
              <Button @click="closeUpgradeResult">
                {{ upgradeResult.state === 'succeeded' ? t('admin.settings.upgrade.reload') : t('admin.settings.upgrade.close') }}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>

    <Teleport to="body">
      <div v-if="restartResult" class="fixed inset-0 z-[180] flex items-center justify-center p-4">
        <div class="absolute inset-0 bg-background/80 backdrop-blur-sm"></div>
        <div class="relative z-10 w-full max-w-lg overflow-hidden rounded-xl border border-border/60 bg-card shadow-2xl" role="dialog" aria-modal="true">
          <div class="flex items-start justify-between gap-4 border-b border-border/50 px-5 py-4">
            <div class="flex min-w-0 items-center gap-3">
              <div
                class="rounded-lg p-2"
                :class="restartResult.state === 'succeeded' ? 'bg-green-500/10 text-green-500' : 'bg-destructive/10 text-destructive'"
              >
                <CheckCircle2 v-if="restartResult.state === 'succeeded'" class="h-5 w-5" />
                <AlertTriangle v-else class="h-5 w-5" />
              </div>
              <h3 class="text-base font-semibold text-foreground">
                {{ restartResult.state === 'succeeded' ? t('admin.settings.restart.successTitle') : t('admin.settings.restart.failedTitle') }}
              </h3>
            </div>
            <button
              type="button"
              class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-surface-elevated hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
              :aria-label="t('admin.settings.restart.close')"
              @click="closeRestartResult"
            >
              <X class="h-5 w-5" />
            </button>
          </div>
          <div class="space-y-4 px-5 py-5">
            <p class="text-sm leading-6 text-muted-foreground">{{ restartResult.message }}</p>
            <pre v-if="restartResult.output" class="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-lg border border-destructive/20 bg-destructive/5 p-3 text-xs leading-5 text-foreground">{{ restartResult.output }}</pre>
            <div class="flex justify-end">
              <Button @click="closeRestartResult">
                {{ restartResult.state === 'succeeded' ? t('admin.settings.restart.reload') : t('admin.settings.restart.close') }}
              </Button>
            </div>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>
