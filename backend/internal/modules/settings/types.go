package settings

import "time"

type NotificationChannel string

const (
	NotificationChannelDingtalk NotificationChannel = "dingtalk"
	NotificationChannelWecom    NotificationChannel = "wecom"
	NotificationChannelQQ       NotificationChannel = "qq"
	NotificationChannelFeishu   NotificationChannel = "feishu"
	NotificationChannelTelegram NotificationChannel = "telegram"
)

type TestNotificationRequest struct {
	Channel          NotificationChannel `json:"channel"`
	Webhook          string              `json:"webhook"`
	Secret           string              `json:"secret"`
	TelegramBotToken string              `json:"telegramBotToken"`
	TelegramChatID   string              `json:"telegramChatId"`
	TelegramProxyURL string              `json:"telegramProxyUrl"`
	QQAppID          string              `json:"qqAppId"`
	QQClientSecret   string              `json:"qqClientSecret"`
	QQUserOpenID     string              `json:"qqUserOpenId"`
	QQGroupOpenID    string              `json:"qqGroupOpenId,omitempty"`
}

type TestNotificationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type NotificationChannelSettings struct {
	Dingtalk []DingtalkChannelSettings `json:"dingtalk"`
	Wecom    []WebhookChannelSettings  `json:"wecom"`
	QQ       []QQChannelSettings       `json:"qq"`
	Feishu   []WebhookChannelSettings  `json:"feishu"`
	Telegram []TelegramChannelSettings `json:"telegram"`
}

type StrategySettings struct {
	EnableRefreshInterval      bool                       `json:"enableRefreshInterval"`
	RefreshInterval            int                        `json:"refreshInterval"`
	EnableBalanceWarning       bool                       `json:"enableBalanceWarning"`
	DefaultBalanceThreshold    float64                    `json:"defaultBalanceThreshold"`
	BalanceNotifyBotIDs        []string                   `json:"balanceNotifyBotIds"`
	BalanceTemplate            string                     `json:"balanceTemplate"`
	BalanceTemplateFormat      NotificationTemplateFormat `json:"balanceTemplateFormat,omitempty"`
	EnableMultiplierAlert      bool                       `json:"enableMultiplierAlert"`
	MultiplierNotifyBotIDs     []string                   `json:"multiplierNotifyBotIds"`
	MultiplierTemplate         string                     `json:"multiplierTemplate"`
	MultiplierTemplateFormat   NotificationTemplateFormat `json:"multiplierTemplateFormat,omitempty"`
	EnableAutoChangeMultiplier bool                       `json:"enableAutoChangeMultiplier"`
}

// NotificationTemplateFormat 描述预警模板的源格式。空值和未知值都会在保存及发送前
// 归一化为 text，从而让升级前已保存的纯文本模板保持完全相同的发送行为。
type NotificationTemplateFormat string

const (
	NotificationTemplateFormatText     NotificationTemplateFormat = "text"
	NotificationTemplateFormatMarkdown NotificationTemplateFormat = "markdown"
	NotificationTemplateFormatHTML     NotificationTemplateFormat = "html"
)

type DingtalkChannelSettings struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	Secret  string `json:"secret"`
}

type WebhookChannelSettings struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Webhook string `json:"webhook"`
	Secret  string `json:"secret"`
}

type TelegramChannelSettings struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	BotToken string `json:"botToken"`
	ChatID   string `json:"chatId"`
	ProxyURL string `json:"proxyUrl"`
}

// QQChannelSettings 保存 QQ 官方机器人的长期凭据和单聊用户 OpenID。
// GroupOpenID 仅用于兼容尚未发布的群通知配置草稿，不参与单聊发送；Access Token
// 由后端按需获取并仅缓存在内存中，不写入数据库或返回前端。
type QQChannelSettings struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Enabled      bool   `json:"enabled"`
	AppID        string `json:"appId"`
	ClientSecret string `json:"clientSecret"`
	UserOpenID   string `json:"userOpenId"`
	GroupOpenID  string `json:"groupOpenId,omitempty"`
}

// SmtpTLSMode 只允许 implicit（隐式 TLS，如 465 端口）或 starttls（如 587 端口）。
// 明确不支持 "none"：SMTP 发送必须使用 TLS 1.2+。
type SmtpTLSMode string

const (
	SmtpTLSModeImplicit SmtpTLSMode = "implicit"
	SmtpTLSModeStarttls SmtpTLSMode = "starttls"
)

// SmtpSettings 是 GET/PUT /api/settings/smtp 的安全响应对象，永不包含密码明文或密文。
type SmtpSettings struct {
	Host               string      `json:"host"`
	Port               int         `json:"port"`
	Username           string      `json:"username"`
	FromEmail          string      `json:"fromEmail"`
	FromName           string      `json:"fromName"`
	TLSMode            SmtpTLSMode `json:"tlsMode"`
	PasswordConfigured bool        `json:"passwordConfigured"`
	UpdatedAt          *time.Time  `json:"updatedAt"`
}

// SaveSmtpSettingsInput 是 PUT /api/settings/smtp 的请求体。
// Password 省略、空字符串或纯空白均表示保留已有密文，由 service 层 trim 后判断。
type SaveSmtpSettingsInput struct {
	Host      string      `json:"host"`
	Port      int         `json:"port"`
	Username  string      `json:"username"`
	Password  string      `json:"password"`
	FromEmail string      `json:"fromEmail"`
	FromName  string      `json:"fromName"`
	TLSMode   SmtpTLSMode `json:"tlsMode"`
}

// TestSmtpEmailRequest 是 POST /api/settings/smtp/test-email 的请求体。
// 只携带收件人；SMTP host/port/password 一律读取已保存配置，不允许从请求体传入。
type TestSmtpEmailRequest struct {
	RecipientEmail string `json:"recipientEmail"`
}

type TestSmtpEmailResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// EmailTemplate 是后台营销邮件模板的 HTTP DTO。模板按当前 workspace 隔离，handler 不接收
// user_id/admin_account_id，避免调用方越权指定其他 workspace。
type EmailTemplate struct {
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Subject   string     `json:"subject"`
	HTMLBody  string     `json:"htmlBody"`
	IsBuiltIn bool       `json:"isBuiltIn"`
	CreatedAt *time.Time `json:"createdAt"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

// EmailTemplateSnapshot 是给后台 worker 使用的模板快照 DTO。HTML 只在服务间传递和落库，
// mass_email 的 HTTP DTO 不会把它序列化给前端。
type EmailTemplateSnapshot struct {
	ID       string
	Name     string
	Subject  string
	HTMLBody string
}

type SaveEmailTemplateInput struct {
	Name     string `json:"name"`
	Subject  string `json:"subject"`
	HTMLBody string `json:"htmlBody"`
}

type TestEmailTemplateRequest struct {
	RecipientEmail string `json:"recipientEmail"`
}

type TestEmailTemplateResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}
