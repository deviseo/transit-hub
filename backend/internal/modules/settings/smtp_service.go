package settings

import (
	"context"
	"fmt"
	"log"
	"net/mail"
	"strings"
)

const (
	smtpMaxHostLength     = 255
	smtpMaxUsernameLength = 255
	smtpMaxFromEmailLen   = 320
	smtpMaxFromNameLength = 120
)

// smtpRepository 是 Service 对 SMTP 存储层的全部依赖，由 *Repository 结构性满足；
// 测试可注入内存 fake，无需连接真实数据库。
type smtpRepository interface {
	GetSMTPSettings(ctx context.Context, userID string, adminAccountID string) (smtpRow, error)
	SaveSMTPSettings(ctx context.Context, userID string, adminAccountID string, row smtpRow) error
}

// SetSMTPEncryptionKey 解析并安装 SMTP_ENCRYPTION_KEY。
// 空值表示加密能力不可用（不返回错误，允许应用继续启动）；非空但非法的值返回错误，
// 调用方（httpserver.New）应 panic 使启动失败，因为显式配置错误应尽早暴露。
func (s *Service) SetSMTPEncryptionKey(raw string) error {
	gcm, err := ParseSMTPEncryptionKey(raw)
	if err != nil {
		return err
	}
	s.smtpKeyGCM = gcm
	return nil
}

// GetSMTPSettings 返回当前 workspace 的安全 SMTP 配置对象，永不包含密码明文或密文。
func (s *Service) GetSMTPSettings(ctx context.Context, userID string) (SmtpSettings, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return SmtpSettings{}, err
	}
	row, err := s.smtpRepo.GetSMTPSettings(ctx, userID, adminAccountID)
	if err != nil {
		// %w 保留底层数据库错误供内部日志诊断；对外仍只暴露 ErrSMTPPersistence 稳定 key
		// （handler 通过 errors.Is 匹配后只返回 ErrSMTPPersistence.Error()，不回传 err 原文）。
		return SmtpSettings{}, fmt.Errorf("%w: %v", ErrSMTPPersistence, err)
	}
	return smtpRowToSettings(row), nil
}

// parseStrictEmailAddress 校验 raw 是裸 addr-spec（不含 display-name），用于 SMTP envelope 地址
// （fromEmail / recipientEmail）。显示名只能来自独立的 fromName 字段，不允许通过 envelope 地址
// 夹带 "Alice <alice@example.com>" 这种 display-name 形式。
func parseStrictEmailAddress(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	parsed, err := mail.ParseAddress(normalized)
	if err != nil {
		return "", ErrSMTPInvalidEmail
	}
	if parsed.Name != "" || parsed.Address != normalized {
		return "", ErrSMTPInvalidEmail
	}
	return parsed.Address, nil
}

func smtpRowToSettings(row smtpRow) SmtpSettings {
	return SmtpSettings{
		Host:               row.Host,
		Port:               row.Port,
		Username:           row.Username,
		FromEmail:          row.FromEmail,
		FromName:           row.FromName,
		TLSMode:            SmtpTLSMode(row.TLSMode),
		PasswordConfigured: row.PasswordCiphertext != "",
		UpdatedAt:          row.UpdatedAt,
	}
}

// SaveSMTPSettings 校验并保存 SMTP 配置。密码省略/空/纯空白时保留已有密文；
// 非空密码要求加密 key 可用，否则返回 ErrSMTPEncryptionKeyUnavailable。
//
// 密码字节保真：input.Password 只用 TrimSpace 结果判断“是否空白/保留已有密码”，
// 真正参与加密的必须是 input.Password 原始值，不能加密 trim 后的字符串，
// 否则合法密码中的前导/尾随空格会被悄悄破坏。
func (s *Service) SaveSMTPSettings(ctx context.Context, userID string, input SaveSmtpSettingsInput) (SmtpSettings, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return SmtpSettings{}, err
	}

	host := strings.TrimSpace(input.Host)
	username := strings.TrimSpace(input.Username)
	fromEmail := strings.TrimSpace(input.FromEmail)
	fromName := strings.TrimSpace(input.FromName)

	rawPassword := input.Password
	passwordIsBlank := strings.TrimSpace(rawPassword) == ""

	if host == "" || len(host) > smtpMaxHostLength {
		return SmtpSettings{}, ErrSMTPValidation
	}
	if strings.ContainsAny(host, "\r\n") || strings.ContainsAny(username, "\r\n") ||
		strings.ContainsAny(rawPassword, "\r\n") || strings.ContainsAny(fromEmail, "\r\n") ||
		strings.ContainsAny(fromName, "\r\n") {
		return SmtpSettings{}, ErrSMTPValidation
	}
	if input.Port < 1 || input.Port > 65535 {
		return SmtpSettings{}, ErrSMTPValidation
	}
	if len(username) > smtpMaxUsernameLength {
		return SmtpSettings{}, ErrSMTPValidation
	}
	if len(fromName) > smtpMaxFromNameLength {
		return SmtpSettings{}, ErrSMTPValidation
	}
	if input.TLSMode != SmtpTLSModeImplicit && input.TLSMode != SmtpTLSModeStarttls {
		return SmtpSettings{}, ErrSMTPInvalidTLSMode
	}
	if fromEmail == "" || len(fromEmail) > smtpMaxFromEmailLen {
		return SmtpSettings{}, ErrSMTPInvalidEmail
	}
	normalizedFromEmail, err := parseStrictEmailAddress(fromEmail)
	if err != nil {
		return SmtpSettings{}, err
	}
	fromEmail = normalizedFromEmail
	// username 为空但 password 非空：一期不保存无法使用的孤立密码。
	if username == "" && !passwordIsBlank {
		return SmtpSettings{}, ErrSMTPValidation
	}

	existing, err := s.smtpRepo.GetSMTPSettings(ctx, userID, adminAccountID)
	if err != nil {
		return SmtpSettings{}, fmt.Errorf("%w: %v", ErrSMTPPersistence, err)
	}

	// 认证降级拒绝：一期没有安全的"确认清除密码"设计。如果已经保存过密文，
	// 而本次保存又要把 username 清空且不提供新密码，会让数据库停留在
	// "有密文但运行时因 username 为空而跳过 AUTH" 的隐式降级状态——直接拒绝，
	// 不静默清空 password_ciphertext，也不悄悄保留一个已经用不上的密文。
	if existing.PasswordCiphertext != "" && username == "" && passwordIsBlank {
		return SmtpSettings{}, ErrSMTPValidation
	}

	ciphertext := existing.PasswordCiphertext
	if !passwordIsBlank {
		if s.smtpKeyGCM == nil {
			return SmtpSettings{}, ErrSMTPEncryptionKeyUnavailable
		}
		encrypted, err := encryptSMTPPassword(s.smtpKeyGCM, userID, adminAccountID, rawPassword)
		if err != nil {
			return SmtpSettings{}, fmt.Errorf("%w: %v", ErrSMTPPersistence, err)
		}
		ciphertext = encrypted
	}

	row := smtpRow{
		Host:               host,
		Port:               input.Port,
		Username:           username,
		PasswordCiphertext: ciphertext,
		FromEmail:          fromEmail,
		FromName:           fromName,
		TLSMode:            string(input.TLSMode),
	}
	if err := s.smtpRepo.SaveSMTPSettings(ctx, userID, adminAccountID, row); err != nil {
		return SmtpSettings{}, fmt.Errorf("%w: %v", ErrSMTPPersistence, err)
	}

	saved, err := s.smtpRepo.GetSMTPSettings(ctx, userID, adminAccountID)
	if err != nil {
		return SmtpSettings{}, fmt.Errorf("%w: %v", ErrSMTPPersistence, err)
	}
	return smtpRowToSettings(saved), nil
}

// TestSMTPEmail 使用已保存的 SMTP 配置实际发送一封固定的静态 HTML 测试邮件。
// SMTP host/port/password 只来自数据库，不接受请求体传入。
func (s *Service) TestSMTPEmail(ctx context.Context, userID string, recipientEmail string) error {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return err
	}
	return s.sendSavedSMTPEmail(ctx, userID, adminAccountID, recipientEmail, smtpTestSubject, smtpTestHTMLBody)
}

// ValidateSMTPReadyForWorkspace 只检查指定 workspace 的已保存 SMTP 配置是否足以发送邮件，
// 不解密密码、不发测试邮件，避免创建批次时触发外部副作用。
func (s *Service) ValidateSMTPReadyForWorkspace(ctx context.Context, userID string, adminAccountID string) error {
	row, err := s.smtpRepo.GetSMTPSettings(ctx, userID, adminAccountID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSMTPPersistence, err)
	}
	if row.Host == "" || row.Port == 0 || row.FromEmail == "" ||
		(row.TLSMode != string(SmtpTLSModeImplicit) && row.TLSMode != string(SmtpTLSModeStarttls)) {
		return ErrSMTPMissingConfig
	}
	if row.Username != "" && row.PasswordCiphertext == "" {
		return ErrSMTPMissingConfig
	}
	if row.Username == "" && row.PasswordCiphertext != "" {
		return ErrSMTPMissingConfig
	}
	if row.PasswordCiphertext != "" && s.smtpKeyGCM == nil {
		return ErrSMTPEncryptionKeyUnavailable
	}
	return nil
}

// SendSavedSMTPEmailForWorkspace 让异步 worker 使用创建批次时的 workspace SMTP 配置发送快照内容。
// SMTP 密码仍只在 settings.Service 内部解密，调用方不能读取或持久化明文凭据。
func (s *Service) SendSavedSMTPEmailForWorkspace(ctx context.Context, userID string, adminAccountID string, recipientEmail string, subject string, htmlBody string) error {
	return s.sendSavedSMTPEmail(ctx, userID, adminAccountID, recipientEmail, subject, htmlBody)
}

// sendSavedSMTPEmail 是 SMTP 测试和模板测试共用的发送路径。调用方只提供收件人和已保存的
// subject/html，SMTP 连接参数和密码始终来自数据库，避免接口携带未保存的临时 SMTP 配置。
func (s *Service) sendSavedSMTPEmail(ctx context.Context, userID string, adminAccountID string, recipientEmail string, subject string, htmlBody string) error {
	recipientEmail = strings.TrimSpace(recipientEmail)
	if recipientEmail == "" || strings.ContainsAny(recipientEmail, "\r\n") {
		return ErrSMTPInvalidEmail
	}
	if subject == "" || strings.ContainsAny(subject, "\r\n") || htmlBody == "" {
		return ErrSMTPValidation
	}
	normalizedRecipient, err := parseStrictEmailAddress(recipientEmail)
	if err != nil {
		return err
	}
	recipientEmail = normalizedRecipient

	row, err := s.smtpRepo.GetSMTPSettings(ctx, userID, adminAccountID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSMTPPersistence, err)
	}

	if row.Host == "" || row.Port == 0 || row.FromEmail == "" ||
		(row.TLSMode != string(SmtpTLSModeImplicit) && row.TLSMode != string(SmtpTLSModeStarttls)) {
		return ErrSMTPMissingConfig
	}
	if row.Username != "" && row.PasswordCiphertext == "" {
		return ErrSMTPMissingConfig
	}
	if row.Username == "" && row.PasswordCiphertext != "" {
		return ErrSMTPMissingConfig
	}

	password := ""
	if row.PasswordCiphertext != "" {
		if s.smtpKeyGCM == nil {
			return ErrSMTPEncryptionKeyUnavailable
		}
		decrypted, err := decryptSMTPPassword(s.smtpKeyGCM, userID, adminAccountID, row.PasswordCiphertext)
		if err != nil {
			return ErrSMTPDecryptFailed
		}
		password = decrypted
	}

	sender := s.smtpSender
	if sender == nil {
		sender = newNetSMTPSender()
	}

	cfg := smtpSendConfig{
		Host:           row.Host,
		Port:           row.Port,
		Username:       row.Username,
		Password:       password,
		TLSMode:        SmtpTLSMode(row.TLSMode),
		FromEmail:      row.FromEmail,
		FromName:       row.FromName,
		RecipientEmail: recipientEmail,
		Subject:        subject,
		HTMLBody:       htmlBody,
	}
	if err := sender.Send(ctx, cfg); err != nil {
		// 只记录失败类别、host、port、tlsMode 和 workspace id；底层错误文本可能来自
		// SMTP 服务端响应，不写入日志，避免意外泄漏认证相关细节。
		log.Printf("[settings] smtp test email send failed user_id=%s admin_account_id=%s host=%s port=%d tlsMode=%s",
			userID, adminAccountID, row.Host, row.Port, row.TLSMode)
		return ErrSMTPSendFailed
	}
	return nil
}
