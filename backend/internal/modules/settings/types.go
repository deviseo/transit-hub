package settings

import "time"

type NotificationChannel string

const (
	NotificationChannelDingtalk NotificationChannel = "dingtalk"
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
}

type TestNotificationResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type NotificationChannelSettings struct {
	Dingtalk []DingtalkChannelSettings `json:"dingtalk"`
	Feishu   []WebhookChannelSettings  `json:"feishu"`
	Telegram []TelegramChannelSettings `json:"telegram"`
}

type StrategySettings struct {
	EnableRefreshInterval      bool     `json:"enableRefreshInterval"`
	RefreshInterval            int      `json:"refreshInterval"`
	EnableBalanceWarning       bool     `json:"enableBalanceWarning"`
	DefaultBalanceThreshold    float64  `json:"defaultBalanceThreshold"`
	BalanceNotifyBotIDs        []string `json:"balanceNotifyBotIds"`
	BalanceTemplate            string   `json:"balanceTemplate"`
	EnableMultiplierAlert      bool     `json:"enableMultiplierAlert"`
	MultiplierNotifyBotIDs     []string `json:"multiplierNotifyBotIds"`
	MultiplierTemplate         string   `json:"multiplierTemplate"`
	EnableAutoChangeMultiplier bool     `json:"enableAutoChangeMultiplier"`
}

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
