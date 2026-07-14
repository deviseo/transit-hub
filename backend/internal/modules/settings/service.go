package settings

import (
	"bytes"
	"context"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const testMessage = "Transit Hub notification channel test succeeded."

const minRefreshIntervalSeconds = 60

var (
	ErrInvalidNotificationChannel = errors.New("admin.settings.errors.invalidChannel")
	ErrMissingWebhook             = errors.New("admin.settings.errors.missingWebhook")
	ErrMissingTelegramConfig      = errors.New("admin.settings.errors.missingTelegramConfig")
	ErrSendNotificationFailed     = errors.New("admin.settings.errors.sendFailed")
)

// SMTP 错误 sentinel：handler 用 errors.Is 逐一映射到固定状态码，详见 handler.go writeSmtpError。
var (
	ErrSMTPValidation               = errors.New("admin.settings.smtp.errors.validation")
	ErrSMTPMissingConfig            = errors.New("admin.settings.smtp.errors.missingConfig")
	ErrSMTPInvalidTLSMode           = errors.New("admin.settings.smtp.errors.invalidTlsMode")
	ErrSMTPInvalidEmail             = errors.New("admin.settings.smtp.errors.invalidEmail")
	ErrSMTPEncryptionKeyUnavailable = errors.New("admin.settings.smtp.errors.encryptionKeyUnavailable")
	ErrSMTPDecryptFailed            = errors.New("admin.settings.smtp.errors.decryptFailed")
	ErrSMTPSendFailed               = errors.New("admin.settings.smtp.errors.sendFailed")
	ErrSMTPPersistence              = errors.New("admin.settings.smtp.errors.persistence")
)

type Service struct {
	client            *http.Client
	repository        *Repository
	accounts          AdminAccountResolver
	OnStrategyChanged func(StrategySettings)

	// smtpRepo 是 SMTP 存储层的窄接口，由 *Repository 结构性满足；测试可注入内存 fake。
	smtpRepo smtpRepository
	// smtpKeyGCM 为 nil 表示 SMTP_ENCRYPTION_KEY 未配置，此时禁止保存非空密码或解密已保存密码。
	smtpKeyGCM cipher.AEAD
	// smtpSender 默认使用生产实现；测试可注入 fake sender 以避免依赖真实外部 SMTP 服务。
	smtpSender smtpSender
	// emailTemplateRepo 是邮件模板存储层窄接口，测试通过内存 fake 覆盖 workspace 隔离和限制规则。
	emailTemplateRepo emailTemplateRepository
}

type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

func NewService(client *http.Client, repository *Repository) *Service {
	if client == nil {
		client = http.DefaultClient
	}
	return &Service{client: client, repository: repository, smtpRepo: repository, emailTemplateRepo: repository}
}

func (s *Service) SetAdminAccountResolver(accounts AdminAccountResolver) {
	s.accounts = accounts
}

func DefaultNotificationChannelSettings() NotificationChannelSettings {
	return NotificationChannelSettings{
		Dingtalk: []DingtalkChannelSettings{},
		Feishu:   []WebhookChannelSettings{},
		Telegram: []TelegramChannelSettings{},
	}
}

func DefaultStrategySettings() StrategySettings {
	return StrategySettings{RefreshInterval: minRefreshIntervalSeconds}
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	return s.repository.EnsureSchema(ctx)
}

func (s *Service) GetNotificationChannels(ctx context.Context, userID string) (NotificationChannelSettings, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return NotificationChannelSettings{}, err
	}
	settings, err := s.repository.GetNotificationChannels(ctx, userID, adminAccountID)
	if err != nil {
		return settings, err
	}
	normalized := normalizeNotificationChannelSettings(settings)
	// 如果 normalize 修正了 ID（空值或重复），自动回写到数据库以持久化唯一 ID。
	if needsPersist(settings, normalized) {
		_ = s.repository.SaveNotificationChannels(ctx, userID, adminAccountID, normalized)
	}
	return normalized, nil
}

func (s *Service) GetFirstStrategy(ctx context.Context) (StrategySettings, error) {
	return s.repository.GetFirstStrategy(ctx)
}

func (s *Service) GetStrategy(ctx context.Context, userID string) (StrategySettings, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return StrategySettings{}, err
	}
	return s.repository.GetStrategy(ctx, userID, adminAccountID)
}

func (s *Service) SaveStrategy(ctx context.Context, userID string, settings StrategySettings) (StrategySettings, error) {
	settings = normalizeStrategySettings(settings)
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return StrategySettings{}, err
	}
	if err := s.repository.SaveStrategy(ctx, userID, adminAccountID, settings); err != nil {
		return StrategySettings{}, err
	}
	if s.OnStrategyChanged != nil {
		s.OnStrategyChanged(settings)
	}
	return settings, nil
}

func (s *Service) SaveNotificationChannels(ctx context.Context, userID string, settings NotificationChannelSettings) (NotificationChannelSettings, error) {
	settings = normalizeNotificationChannelSettings(settings)
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return NotificationChannelSettings{}, err
	}
	if err := s.repository.SaveNotificationChannels(ctx, userID, adminAccountID, settings); err != nil {
		return NotificationChannelSettings{}, err
	}
	return settings, nil
}

func (s *Service) currentAdminAccountID(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", errors.New("admin.adminAccounts.errors.noCurrentAccount")
	}
	return s.accounts.RequireCurrentID(ctx, userID)
}

// needsPersist 检测 normalize 前后的 bot ID 是否发生了变化（空值补全或重复去重）。
func needsPersist(before, after NotificationChannelSettings) bool {
	for i := range before.Dingtalk {
		if i < len(after.Dingtalk) && before.Dingtalk[i].ID != after.Dingtalk[i].ID {
			return true
		}
	}
	for i := range before.Feishu {
		if i < len(after.Feishu) && before.Feishu[i].ID != after.Feishu[i].ID {
			return true
		}
	}
	for i := range before.Telegram {
		if i < len(after.Telegram) && before.Telegram[i].ID != after.Telegram[i].ID {
			return true
		}
	}
	return false
}

// generateBotID 生成 16 字节的随机十六进制 ID，确保每个机器人拥有全局唯一标识。
func generateBotID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func normalizeNotificationChannelSettings(settings NotificationChannelSettings) NotificationChannelSettings {
	if settings.Dingtalk == nil {
		settings.Dingtalk = []DingtalkChannelSettings{}
	}
	if settings.Feishu == nil {
		settings.Feishu = []WebhookChannelSettings{}
	}
	if settings.Telegram == nil {
		settings.Telegram = []TelegramChannelSettings{}
	}

	// 收集所有 bot ID，确保全局唯一；空 ID 或重复 ID 均重新生成。
	seen := make(map[string]struct{})

	for index := range settings.Dingtalk {
		settings.Dingtalk[index].ID = strings.TrimSpace(settings.Dingtalk[index].ID)
		if settings.Dingtalk[index].ID == "" {
			settings.Dingtalk[index].ID = generateBotID()
		}
		if _, dup := seen[settings.Dingtalk[index].ID]; dup {
			settings.Dingtalk[index].ID = generateBotID()
		}
		seen[settings.Dingtalk[index].ID] = struct{}{}
		settings.Dingtalk[index].Name = strings.TrimSpace(settings.Dingtalk[index].Name)
		settings.Dingtalk[index].Webhook = strings.TrimSpace(settings.Dingtalk[index].Webhook)
		settings.Dingtalk[index].Secret = strings.TrimSpace(settings.Dingtalk[index].Secret)
	}
	for index := range settings.Feishu {
		settings.Feishu[index].ID = strings.TrimSpace(settings.Feishu[index].ID)
		if settings.Feishu[index].ID == "" {
			settings.Feishu[index].ID = generateBotID()
		}
		if _, dup := seen[settings.Feishu[index].ID]; dup {
			settings.Feishu[index].ID = generateBotID()
		}
		seen[settings.Feishu[index].ID] = struct{}{}
		settings.Feishu[index].Name = strings.TrimSpace(settings.Feishu[index].Name)
		settings.Feishu[index].Webhook = strings.TrimSpace(settings.Feishu[index].Webhook)
		settings.Feishu[index].Secret = strings.TrimSpace(settings.Feishu[index].Secret)
	}
	for index := range settings.Telegram {
		settings.Telegram[index].ID = strings.TrimSpace(settings.Telegram[index].ID)
		if settings.Telegram[index].ID == "" {
			settings.Telegram[index].ID = generateBotID()
		}
		if _, dup := seen[settings.Telegram[index].ID]; dup {
			settings.Telegram[index].ID = generateBotID()
		}
		seen[settings.Telegram[index].ID] = struct{}{}
		settings.Telegram[index].Name = strings.TrimSpace(settings.Telegram[index].Name)
		settings.Telegram[index].BotToken = strings.TrimSpace(settings.Telegram[index].BotToken)
		settings.Telegram[index].ChatID = strings.TrimSpace(settings.Telegram[index].ChatID)
		settings.Telegram[index].ProxyURL = strings.TrimSpace(settings.Telegram[index].ProxyURL)
	}
	return settings
}

func normalizeStrategySettings(settings StrategySettings) StrategySettings {
	if settings.RefreshInterval < minRefreshIntervalSeconds {
		settings.RefreshInterval = minRefreshIntervalSeconds
	}
	return settings
}

func (s *Service) TestNotification(ctx context.Context, dto TestNotificationRequest) error {
	switch dto.Channel {
	case NotificationChannelDingtalk:
		return s.sendDingtalk(ctx, strings.TrimSpace(dto.Webhook), strings.TrimSpace(dto.Secret), testMessage)
	case NotificationChannelFeishu:
		return s.sendFeishu(ctx, strings.TrimSpace(dto.Webhook), strings.TrimSpace(dto.Secret), testMessage)
	case NotificationChannelTelegram:
		return s.sendTelegram(ctx, strings.TrimSpace(dto.TelegramBotToken), strings.TrimSpace(dto.TelegramChatID), strings.TrimSpace(dto.TelegramProxyURL), testMessage)
	default:
		return ErrInvalidNotificationChannel
	}
}

// SendToBots 根据机器人 ID 列表向已配置的通知渠道发送消息。
// 从数据库加载用户的通知渠道配置，匹配 ID 后逐个发送。
// 发送失败仅记录日志，不中断流程（fire-and-forget）。
func (s *Service) SendToBots(ctx context.Context, userID string, botIDs []string, message string) {
	if len(botIDs) == 0 || message == "" {
		return
	}
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		log.Printf("[settings] 当前 admin workspace 缺失 user_id=%s err=%v", userID, err)
		return
	}
	channels, err := s.repository.GetNotificationChannels(ctx, userID, adminAccountID)
	if err != nil {
		log.Printf("[settings] 加载通知渠道配置失败 user_id=%s err=%v", userID, err)
		return
	}

	idSet := make(map[string]struct{}, len(botIDs))
	for _, id := range botIDs {
		idSet[id] = struct{}{}
	}

	for _, bot := range channels.Dingtalk {
		if _, ok := idSet[bot.ID]; ok {
			if err := s.sendDingtalk(ctx, bot.Webhook, bot.Secret, message); err != nil {
				log.Printf("[settings] 钉钉通知发送失败 bot=%s err=%v", bot.Name, err)
			}
		}
	}
	for _, bot := range channels.Feishu {
		if _, ok := idSet[bot.ID]; ok {
			if err := s.sendFeishu(ctx, bot.Webhook, bot.Secret, message); err != nil {
				log.Printf("[settings] 飞书通知发送失败 bot=%s err=%v", bot.Name, err)
			}
		}
	}
	for _, bot := range channels.Telegram {
		if _, ok := idSet[bot.ID]; ok {
			if err := s.sendTelegram(ctx, bot.BotToken, bot.ChatID, bot.ProxyURL, message); err != nil {
				log.Printf("[settings] Telegram 通知发送失败 bot=%s err=%v", bot.Name, err)
			}
		}
	}
}

func (s *Service) sendDingtalk(ctx context.Context, webhook string, secret string, message string) error {
	if webhook == "" {
		return ErrMissingWebhook
	}
	signedWebhook, err := dingtalkSignedWebhook(webhook, secret)
	if err != nil {
		return err
	}
	body := map[string]any{
		"msgtype": "text",
		"text": map[string]string{
			"content": message,
		},
	}
	return s.postJSON(ctx, signedWebhook, body)
}

func (s *Service) sendFeishu(ctx context.Context, webhook string, secret string, message string) error {
	if webhook == "" {
		return ErrMissingWebhook
	}
	timestamp := time.Now().Unix()
	body := map[string]any{
		"msg_type": "text",
		"content": map[string]string{
			"text": message,
		},
	}
	if secret != "" {
		body["timestamp"] = strconv.FormatInt(timestamp, 10)
		body["sign"] = feishuSign(timestamp, secret)
	}
	return s.postJSON(ctx, webhook, body)
}

func (s *Service) sendTelegram(ctx context.Context, botToken string, chatID string, proxyURL string, message string) error {
	if botToken == "" || chatID == "" {
		return ErrMissingTelegramConfig
	}
	endpoint := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", url.PathEscape(botToken))
	body := map[string]string{
		"chat_id": chatID,
		"text":    message,
	}
	return s.postJSONWithClient(ctx, s.telegramClient(proxyURL), endpoint, body)
}

func (s *Service) postJSON(ctx context.Context, endpoint string, payload any) error {
	return s.postJSONWithClient(ctx, s.client, endpoint, payload)
}

func (s *Service) postJSONWithClient(ctx context.Context, client *http.Client, endpoint string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSendNotificationFailed, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("%w: status=%d body=%s", ErrSendNotificationFailed, response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func (s *Service) telegramClient(proxyURL string) *http.Client {
	if proxyURL == "" {
		return s.client
	}
	parsedProxy, err := url.Parse(proxyURL)
	if err != nil {
		return s.client
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(parsedProxy)
	return &http.Client{Transport: transport, Timeout: s.client.Timeout}
}

func dingtalkSignedWebhook(webhook string, secret string) (string, error) {
	if secret == "" {
		return webhook, nil
	}
	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(timestamp + "\n" + secret))
	signature := url.QueryEscape(base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	separator := "?"
	if strings.Contains(webhook, "?") {
		separator = "&"
	}
	return webhook + separator + "timestamp=" + timestamp + "&sign=" + signature, nil
}

func feishuSign(timestamp int64, secret string) string {
	mac := hmac.New(sha256.New, []byte(strconv.FormatInt(timestamp, 10)+"\n"+secret))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
