package settings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	builtInMarketingTemplateID = "marketing-default"
	maxEmailTemplatesPerSpace  = 50
	maxEmailTemplateNameLen    = 120
	maxEmailTemplateSubjectLen = 255
	maxEmailTemplateHTMLBytes  = 100 * 1024
)

var (
	ErrEmailTemplateValidation       = errors.New("admin.settings.emailTemplates.errors.validation")
	ErrEmailTemplateInvalidEmail     = errors.New("admin.settings.emailTemplates.errors.invalidEmail")
	ErrEmailTemplateNotFound         = errors.New("admin.settings.emailTemplates.errors.notFound")
	ErrEmailTemplateBuiltInProtected = errors.New("admin.settings.emailTemplates.errors.builtInProtected")
	ErrEmailTemplateLimitReached     = errors.New("admin.settings.emailTemplates.errors.limitReached")
	ErrEmailTemplatePersistence      = errors.New("admin.settings.emailTemplates.errors.persistence")
)

// emailTemplateRepository 是 Service 访问邮件模板存储的完整边界。生产由 *Repository 满足，
// 单元测试注入内存 fake，避免为了业务规则测试连接真实 Postgres。
type emailTemplateRepository interface {
	EnsureBuiltInEmailTemplate(ctx context.Context, userID string, adminAccountID string, template EmailTemplate) error
	ListEmailTemplates(ctx context.Context, userID string, adminAccountID string) ([]EmailTemplate, error)
	GetEmailTemplate(ctx context.Context, userID string, adminAccountID string, id string) (EmailTemplate, bool, error)
	CreateEmailTemplate(ctx context.Context, userID string, adminAccountID string, template EmailTemplate, limit int) (EmailTemplate, bool, error)
	UpdateEmailTemplate(ctx context.Context, userID string, adminAccountID string, id string, input SaveEmailTemplateInput) (EmailTemplate, bool, error)
	DeleteEmailTemplate(ctx context.Context, userID string, adminAccountID string, id string) (bool, error)
}

func defaultMarketingEmailTemplate() EmailTemplate {
	return EmailTemplate{
		ID:        builtInMarketingTemplateID,
		Name:      "营销活动",
		Subject:   "TransitHub 限时活动",
		HTMLBody:  builtInMarketingHTML,
		IsBuiltIn: true,
	}
}

const builtInMarketingHTML = `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>TransitHub 限时活动</title>
</head>
<body style="margin:0;background:#f4f7fb;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI','Microsoft YaHei',Arial,sans-serif;color:#172033;">
  <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="background:#f4f7fb;padding:32px 12px;">
    <tr>
      <td align="center">
        <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="max-width:640px;background:#ffffff;border-radius:8px;overflow:hidden;border:1px solid #e1e7f0;">
          <tr>
            <td style="background:#10233f;padding:32px;color:#ffffff;">
              <div style="font-size:14px;letter-spacing:0;color:#8bd3ff;margin-bottom:12px;">TransitHub 专属活动</div>
              <h1 style="margin:0;font-size:28px;line-height:1.35;font-weight:700;">让您的 API 服务运营更稳、更快、更省心</h1>
            </td>
          </tr>
          <tr>
            <td style="padding:32px;">
              <p style="margin:0 0 18px;font-size:16px;line-height:1.8;">您好，TransitHub 正在为团队用户提供限时优惠，帮助您集中管理上游站点、倍率策略、余额预警和通知渠道。</p>
              <table role="presentation" width="100%" cellspacing="0" cellpadding="0" style="margin:24px 0;background:#eef7ff;border-radius:8px;">
                <tr>
                  <td style="padding:22px;">
                    <div style="font-size:20px;font-weight:700;color:#10233f;margin-bottom:8px;">限时权益</div>
                    <div style="font-size:15px;line-height:1.8;color:#334155;">完成部署后即可体验统一站点监控、自动刷新、SMTP 邮件测试和多渠道告警能力。</div>
                  </td>
                </tr>
              </table>
              <p style="margin:0 0 24px;font-size:15px;line-height:1.8;color:#334155;">现在启用 TransitHub，把分散的运营动作收拢到一个清晰的工作台中。</p>
              <a href="#" style="display:inline-block;background:#1665d8;color:#ffffff;text-decoration:none;border-radius:6px;padding:12px 20px;font-size:15px;font-weight:700;">立即查看活动</a>
            </td>
          </tr>
          <tr>
            <td style="padding:20px 32px;background:#f8fafc;color:#64748b;font-size:12px;line-height:1.6;">这是一封 TransitHub 营销活动模板邮件。您可以在后台设置中编辑标题和正文。</td>
          </tr>
        </table>
      </td>
    </tr>
  </table>
</body>
</html>`

func (s *Service) ListEmailTemplates(ctx context.Context, userID string) ([]EmailTemplate, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureBuiltInEmailTemplate(ctx, userID, adminAccountID); err != nil {
		return nil, err
	}
	templates, err := s.emailTemplateRepo.ListEmailTemplates(ctx, userID, adminAccountID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	return templates, nil
}

func (s *Service) GetEmailTemplate(ctx context.Context, userID string, id string) (EmailTemplate, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return EmailTemplate{}, err
	}
	if err := s.ensureBuiltInEmailTemplate(ctx, userID, adminAccountID); err != nil {
		return EmailTemplate{}, err
	}
	template, ok, err := s.emailTemplateRepo.GetEmailTemplate(ctx, userID, adminAccountID, strings.TrimSpace(id))
	if err != nil {
		return EmailTemplate{}, fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	if !ok {
		return EmailTemplate{}, ErrEmailTemplateNotFound
	}
	return template, nil
}

// SnapshotEmailTemplateForWorkspace 按显式 workspace 读取模板，用于异步 worker 在用户切换当前
// workspace 后仍能发送创建批次时选定的模板。返回值不直接作为 HTTP 响应使用。
func (s *Service) SnapshotEmailTemplateForWorkspace(ctx context.Context, userID string, adminAccountID string, id string) (EmailTemplateSnapshot, error) {
	if err := s.ensureBuiltInEmailTemplate(ctx, userID, adminAccountID); err != nil {
		return EmailTemplateSnapshot{}, err
	}
	template, ok, err := s.emailTemplateRepo.GetEmailTemplate(ctx, userID, adminAccountID, strings.TrimSpace(id))
	if err != nil {
		return EmailTemplateSnapshot{}, fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	if !ok {
		return EmailTemplateSnapshot{}, ErrEmailTemplateNotFound
	}
	return EmailTemplateSnapshot{ID: template.ID, Name: template.Name, Subject: template.Subject, HTMLBody: template.HTMLBody}, nil
}

func (s *Service) CreateEmailTemplate(ctx context.Context, userID string, input SaveEmailTemplateInput) (EmailTemplate, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return EmailTemplate{}, err
	}
	if err := s.ensureBuiltInEmailTemplate(ctx, userID, adminAccountID); err != nil {
		return EmailTemplate{}, err
	}
	normalized, err := normalizeEmailTemplateInput(input)
	if err != nil {
		return EmailTemplate{}, err
	}
	id, err := generateEmailTemplateID()
	if err != nil {
		return EmailTemplate{}, fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	template := EmailTemplate{ID: id, Name: normalized.Name, Subject: normalized.Subject, HTMLBody: normalized.HTMLBody}
	created, ok, err := s.emailTemplateRepo.CreateEmailTemplate(ctx, userID, adminAccountID, template, maxEmailTemplatesPerSpace)
	if err != nil {
		return EmailTemplate{}, fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	if !ok {
		return EmailTemplate{}, ErrEmailTemplateLimitReached
	}
	return created, nil
}

func (s *Service) UpdateEmailTemplate(ctx context.Context, userID string, id string, input SaveEmailTemplateInput) (EmailTemplate, error) {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return EmailTemplate{}, err
	}
	if err := s.ensureBuiltInEmailTemplate(ctx, userID, adminAccountID); err != nil {
		return EmailTemplate{}, err
	}
	normalized, err := normalizeEmailTemplateInput(input)
	if err != nil {
		return EmailTemplate{}, err
	}
	template, ok, err := s.emailTemplateRepo.UpdateEmailTemplate(ctx, userID, adminAccountID, strings.TrimSpace(id), normalized)
	if err != nil {
		return EmailTemplate{}, fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	if !ok {
		return EmailTemplate{}, ErrEmailTemplateNotFound
	}
	return template, nil
}

func (s *Service) DeleteEmailTemplate(ctx context.Context, userID string, id string) error {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.ensureBuiltInEmailTemplate(ctx, userID, adminAccountID); err != nil {
		return err
	}
	id = strings.TrimSpace(id)
	template, ok, err := s.emailTemplateRepo.GetEmailTemplate(ctx, userID, adminAccountID, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	if !ok {
		return ErrEmailTemplateNotFound
	}
	if template.IsBuiltIn {
		return ErrEmailTemplateBuiltInProtected
	}
	deleted, err := s.emailTemplateRepo.DeleteEmailTemplate(ctx, userID, adminAccountID, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	if !deleted {
		return ErrEmailTemplateNotFound
	}
	return nil
}

func (s *Service) TestEmailTemplate(ctx context.Context, userID string, id string, recipientEmail string) error {
	adminAccountID, err := s.currentAdminAccountID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.ensureBuiltInEmailTemplate(ctx, userID, adminAccountID); err != nil {
		return err
	}
	template, ok, err := s.emailTemplateRepo.GetEmailTemplate(ctx, userID, adminAccountID, strings.TrimSpace(id))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	if !ok {
		return ErrEmailTemplateNotFound
	}
	if err := s.sendSavedSMTPEmail(ctx, userID, adminAccountID, recipientEmail, template.Subject, template.HTMLBody); err != nil {
		if errors.Is(err, ErrSMTPInvalidEmail) {
			return ErrEmailTemplateInvalidEmail
		}
		return err
	}
	return nil
}

func (s *Service) ensureBuiltInEmailTemplate(ctx context.Context, userID string, adminAccountID string) error {
	if err := s.emailTemplateRepo.EnsureBuiltInEmailTemplate(ctx, userID, adminAccountID, defaultMarketingEmailTemplate()); err != nil {
		return fmt.Errorf("%w: %v", ErrEmailTemplatePersistence, err)
	}
	return nil
}

func normalizeEmailTemplateInput(input SaveEmailTemplateInput) (SaveEmailTemplateInput, error) {
	normalized := SaveEmailTemplateInput{
		Name:     strings.TrimSpace(input.Name),
		Subject:  strings.TrimSpace(input.Subject),
		HTMLBody: input.HTMLBody,
	}
	if normalized.Name == "" || utf8.RuneCountInString(normalized.Name) > maxEmailTemplateNameLen {
		return SaveEmailTemplateInput{}, ErrEmailTemplateValidation
	}
	if normalized.Subject == "" || utf8.RuneCountInString(normalized.Subject) > maxEmailTemplateSubjectLen || strings.ContainsAny(normalized.Subject, "\r\n") {
		return SaveEmailTemplateInput{}, ErrEmailTemplateValidation
	}
	if strings.TrimSpace(normalized.HTMLBody) == "" || len([]byte(normalized.HTMLBody)) > maxEmailTemplateHTMLBytes {
		return SaveEmailTemplateInput{}, ErrEmailTemplateValidation
	}
	return normalized, nil
}

func generateEmailTemplateID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
