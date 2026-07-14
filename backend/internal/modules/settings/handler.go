package settings

import (
	"errors"
	"log"
	"net/http"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

// JSON escaping can expand a valid 100 KiB HTML body substantially (for example, quote-heavy markup).
// Keep a transport cap above the domain limit while still rejecting unbounded request bodies before decode.
const maxEmailTemplateRequestBytes = 512 * 1024

type Handler struct {
	service *Service
}

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/settings/strategy", handler.getStrategy)
	mux.HandleFunc("PUT /api/settings/strategy", handler.saveStrategy)
	mux.HandleFunc("GET /api/settings/notification-channels", handler.getNotificationChannels)
	mux.HandleFunc("PUT /api/settings/notification-channels", handler.saveNotificationChannels)
	mux.HandleFunc("POST /api/settings/notification-channels/test", handler.testNotificationChannel)
	mux.HandleFunc("GET /api/settings/smtp", handler.getSMTP)
	mux.HandleFunc("PUT /api/settings/smtp", handler.saveSMTP)
	mux.HandleFunc("POST /api/settings/smtp/test-email", handler.testSMTPEmail)
	mux.HandleFunc("GET /api/settings/email-templates", handler.listEmailTemplates)
	mux.HandleFunc("POST /api/settings/email-templates", handler.createEmailTemplate)
	mux.HandleFunc("GET /api/settings/email-templates/{id}", handler.getEmailTemplate)
	mux.HandleFunc("PUT /api/settings/email-templates/{id}", handler.updateEmailTemplate)
	mux.HandleFunc("DELETE /api/settings/email-templates/{id}", handler.deleteEmailTemplate)
	mux.HandleFunc("POST /api/settings/email-templates/{id}/test-email", handler.testEmailTemplate)
}

func (h *Handler) getStrategy(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	settings, err := h.service.GetStrategy(r.Context(), userID)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		log.Printf("get strategy settings: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "admin.settings.errors.request")
		return
	}
	httpjson.Write(w, http.StatusOK, settings)
}

func (h *Handler) saveStrategy(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto StrategySettings
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	settings, err := h.service.SaveStrategy(r.Context(), userID, dto)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		log.Printf("save strategy settings: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "admin.settings.errors.request")
		return
	}
	httpjson.Write(w, http.StatusOK, settings)
}

func (h *Handler) getNotificationChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	settings, err := h.service.GetNotificationChannels(r.Context(), userID)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		log.Printf("get notification channels: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "admin.settings.errors.request")
		return
	}
	httpjson.Write(w, http.StatusOK, settings)
}

func (h *Handler) saveNotificationChannels(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto NotificationChannelSettings
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	settings, err := h.service.SaveNotificationChannels(r.Context(), userID, dto)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		log.Printf("save notification channels: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "admin.settings.errors.request")
		return
	}
	httpjson.Write(w, http.StatusOK, settings)
}

func writeWorkspaceError(w http.ResponseWriter, err error) bool {
	if err != nil && err.Error() == "admin.adminAccounts.errors.noCurrentAccount" {
		httpjson.WriteError(w, http.StatusConflict, err.Error())
		return true
	}
	return false
}

func (h *Handler) testNotificationChannel(w http.ResponseWriter, r *http.Request) {
	if _, ok := authctx.UserID(r.Context()); !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}

	var dto TestNotificationRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := h.service.TestNotification(r.Context(), dto); err != nil {
		writeSettingsError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, TestNotificationResponse{Success: true, Message: "admin.settings.sections.channels.testConnectionSuccess"})
}

func writeSettingsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrInvalidNotificationChannel), errors.Is(err, ErrMissingWebhook), errors.Is(err, ErrMissingTelegramConfig):
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrSendNotificationFailed):
		log.Printf("send test notification: %v", err)
		httpjson.WriteError(w, http.StatusBadGateway, ErrSendNotificationFailed.Error())
	default:
		log.Printf("test notification: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "admin.settings.errors.request")
	}
}

func (h *Handler) getSMTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	settings, err := h.service.GetSMTPSettings(r.Context(), userID)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeSmtpError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, settings)
}

func (h *Handler) saveSMTP(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto SaveSmtpSettingsInput
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrSMTPValidation.Error())
		return
	}
	settings, err := h.service.SaveSMTPSettings(r.Context(), userID, dto)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeSmtpError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, settings)
}

func (h *Handler) testSMTPEmail(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto TestSmtpEmailRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrSMTPValidation.Error())
		return
	}
	if err := h.service.TestSMTPEmail(r.Context(), userID, dto.RecipientEmail); err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeSmtpError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, TestSmtpEmailResponse{Success: true, Message: "admin.settings.smtp.testEmailSuccess"})
}

// writeSmtpError 把 SMTP sentinel 错误映射为规格固定的状态码与 message key。
// 日志只记录失败类别，不记录 username、password、密文或 recipientEmail 完整值；
// SMTP 发送/解密相关的敏感上下文已经在 service 层按 host/port/tlsMode/workspace 粒度记录。
func writeSmtpError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrSMTPValidation):
		httpjson.WriteError(w, http.StatusBadRequest, ErrSMTPValidation.Error())
	case errors.Is(err, ErrSMTPMissingConfig):
		httpjson.WriteError(w, http.StatusBadRequest, ErrSMTPMissingConfig.Error())
	case errors.Is(err, ErrSMTPInvalidTLSMode):
		httpjson.WriteError(w, http.StatusBadRequest, ErrSMTPInvalidTLSMode.Error())
	case errors.Is(err, ErrSMTPInvalidEmail):
		httpjson.WriteError(w, http.StatusBadRequest, ErrSMTPInvalidEmail.Error())
	case errors.Is(err, ErrSMTPEncryptionKeyUnavailable):
		httpjson.WriteError(w, http.StatusServiceUnavailable, ErrSMTPEncryptionKeyUnavailable.Error())
	case errors.Is(err, ErrSMTPDecryptFailed):
		httpjson.WriteError(w, http.StatusServiceUnavailable, ErrSMTPDecryptFailed.Error())
	case errors.Is(err, ErrSMTPSendFailed):
		httpjson.WriteError(w, http.StatusBadGateway, ErrSMTPSendFailed.Error())
	case errors.Is(err, ErrSMTPPersistence):
		log.Printf("smtp persistence error: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, ErrSMTPPersistence.Error())
	default:
		log.Printf("smtp settings unexpected error: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "admin.settings.errors.request")
	}
}

func (h *Handler) listEmailTemplates(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	templates, err := h.service.ListEmailTemplates(r.Context(), userID)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeEmailTemplateError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, templates)
}

func (h *Handler) getEmailTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	template, err := h.service.GetEmailTemplate(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeEmailTemplateError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, template)
}

func (h *Handler) createEmailTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto SaveEmailTemplateInput
	r.Body = http.MaxBytesReader(w, r.Body, maxEmailTemplateRequestBytes)
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrEmailTemplateValidation.Error())
		return
	}
	template, err := h.service.CreateEmailTemplate(r.Context(), userID, dto)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeEmailTemplateError(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, template)
}

func (h *Handler) updateEmailTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto SaveEmailTemplateInput
	r.Body = http.MaxBytesReader(w, r.Body, maxEmailTemplateRequestBytes)
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrEmailTemplateValidation.Error())
		return
	}
	template, err := h.service.UpdateEmailTemplate(r.Context(), userID, r.PathValue("id"), dto)
	if err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeEmailTemplateError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, template)
}

func (h *Handler) deleteEmailTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if err := h.service.DeleteEmailTemplate(r.Context(), userID, r.PathValue("id")); err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeEmailTemplateError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) testEmailTemplate(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto TestEmailTemplateRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrEmailTemplateValidation.Error())
		return
	}
	if err := h.service.TestEmailTemplate(r.Context(), userID, r.PathValue("id"), dto.RecipientEmail); err != nil {
		if writeWorkspaceError(w, err) {
			return
		}
		writeEmailTemplateError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, TestEmailTemplateResponse{Success: true, Message: "admin.settings.emailTemplates.testEmailSuccess"})
}

// writeEmailTemplateError keeps the email-template API on stable message keys while reusing SMTP
// sentinel errors for saved-SMTP configuration, decryption, and send failures.
func writeEmailTemplateError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrEmailTemplateValidation):
		httpjson.WriteError(w, http.StatusBadRequest, ErrEmailTemplateValidation.Error())
	case errors.Is(err, ErrEmailTemplateInvalidEmail):
		httpjson.WriteError(w, http.StatusBadRequest, ErrEmailTemplateInvalidEmail.Error())
	case errors.Is(err, ErrEmailTemplateNotFound):
		httpjson.WriteError(w, http.StatusNotFound, ErrEmailTemplateNotFound.Error())
	case errors.Is(err, ErrEmailTemplateBuiltInProtected):
		httpjson.WriteError(w, http.StatusConflict, ErrEmailTemplateBuiltInProtected.Error())
	case errors.Is(err, ErrEmailTemplateLimitReached):
		httpjson.WriteError(w, http.StatusConflict, ErrEmailTemplateLimitReached.Error())
	case errors.Is(err, ErrSMTPMissingConfig):
		httpjson.WriteError(w, http.StatusBadRequest, ErrSMTPMissingConfig.Error())
	case errors.Is(err, ErrSMTPEncryptionKeyUnavailable):
		httpjson.WriteError(w, http.StatusServiceUnavailable, ErrSMTPEncryptionKeyUnavailable.Error())
	case errors.Is(err, ErrSMTPDecryptFailed):
		httpjson.WriteError(w, http.StatusServiceUnavailable, ErrSMTPDecryptFailed.Error())
	case errors.Is(err, ErrSMTPSendFailed):
		httpjson.WriteError(w, http.StatusBadGateway, ErrSMTPSendFailed.Error())
	case errors.Is(err, ErrEmailTemplatePersistence):
		log.Printf("email template persistence error: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, ErrEmailTemplatePersistence.Error())
	default:
		log.Printf("email template unexpected error: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "admin.settings.errors.request")
	}
}
