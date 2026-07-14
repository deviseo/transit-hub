package settings

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transithub/backend/internal/shared/authctx"
)

func newEmailTemplateTestService() (*Service, *fakeEmailTemplateRepository, *fakeSMTPRepository, *fakeSMTPSender) {
	templateRepo := newFakeEmailTemplateRepository()
	smtpRepo := newFakeSMTPRepository()
	sender := &fakeSMTPSender{}
	svc := newTestEmailTemplateService(templateRepo, smtpRepo, &fakeAdminAccountResolver{id: "acct-1"})
	svc.smtpSender = sender
	return svc, templateRepo, smtpRepo, sender
}

func TestHandlerEmailTemplatesCRUDContract(t *testing.T) {
	svc, _, _, _ := newEmailTemplateTestService()
	mux := newSMTPTestHandlerMux(svc)

	list := doSMTPRequest(mux, http.MethodGet, "/api/settings/email-templates", "user-1", nil)
	if list.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", list.Code, list.Body.String())
	}
	var templates []EmailTemplate
	if err := json.Unmarshal(list.Body.Bytes(), &templates); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(templates) != 1 || templates[0].ID != builtInMarketingTemplateID || !templates[0].IsBuiltIn {
		t.Fatalf("unexpected list: %+v", templates)
	}

	create := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates", "user-1", validEmailTemplateInput())
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", create.Code, create.Body.String())
	}
	var created EmailTemplate
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" || created.Name != "活动" || created.IsBuiltIn {
		t.Fatalf("unexpected created template: %+v", created)
	}

	update := doSMTPRequest(mux, http.MethodPut, "/api/settings/email-templates/"+created.ID, "user-1", SaveEmailTemplateInput{Name: "更新", Subject: "更新标题", HTMLBody: "<p>updated</p>"})
	if update.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", update.Code, update.Body.String())
	}
	get := doSMTPRequest(mux, http.MethodGet, "/api/settings/email-templates/"+created.ID, "user-1", nil)
	if get.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", get.Code, get.Body.String())
	}
	deleteResponse := doSMTPRequest(mux, http.MethodDelete, "/api/settings/email-templates/"+created.ID, "user-1", nil)
	if deleteResponse.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", deleteResponse.Code, deleteResponse.Body.String())
	}
}

func TestHandlerEmailTemplatesErrors(t *testing.T) {
	svc, _, _, _ := newEmailTemplateTestService()
	mux := newSMTPTestHandlerMux(svc)

	unauthorized := doSMTPRequest(mux, http.MethodGet, "/api/settings/email-templates", "", nil)
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthorized.Code)
	}
	invalid := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates", "user-1", SaveEmailTemplateInput{Name: "", Subject: "ok", HTMLBody: "<p>ok</p>"})
	if invalid.Code != http.StatusBadRequest || decodeErrorMessage(t, invalid) != ErrEmailTemplateValidation.Error() {
		t.Fatalf("unexpected validation response: %d %s", invalid.Code, invalid.Body.String())
	}
	notFound := doSMTPRequest(mux, http.MethodGet, "/api/settings/email-templates/missing", "user-1", nil)
	if notFound.Code != http.StatusNotFound || decodeErrorMessage(t, notFound) != ErrEmailTemplateNotFound.Error() {
		t.Fatalf("unexpected not found response: %d %s", notFound.Code, notFound.Body.String())
	}
	protected := doSMTPRequest(mux, http.MethodDelete, "/api/settings/email-templates/"+builtInMarketingTemplateID, "user-1", nil)
	if protected.Code != http.StatusConflict || decodeErrorMessage(t, protected) != ErrEmailTemplateBuiltInProtected.Error() {
		t.Fatalf("unexpected protected response: %d %s", protected.Code, protected.Body.String())
	}
}

func TestHandlerEmailTemplateCreateUpdateCapsRequestBody(t *testing.T) {
	svc, _, _, _ := newEmailTemplateTestService()
	mux := newSMTPTestHandlerMux(svc)
	oversizedBody := []byte(`{"name":"活动","subject":"标题","htmlBody":"` + strings.Repeat("a", maxEmailTemplateRequestBytes) + `"}`)

	createReq := httptest.NewRequest(http.MethodPost, "/api/settings/email-templates", bytes.NewReader(oversizedBody))
	createReq = createReq.WithContext(authctx.WithUserID(createReq.Context(), "user-1"))
	create := httptest.NewRecorder()
	mux.ServeHTTP(create, createReq)
	if create.Code != http.StatusBadRequest || decodeErrorMessage(t, create) != ErrEmailTemplateValidation.Error() {
		t.Fatalf("unexpected oversized create response: %d %s", create.Code, create.Body.String())
	}

	createdResponse := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates", "user-1", validEmailTemplateInput())
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", createdResponse.Code, createdResponse.Body.String())
	}
	var created EmailTemplate
	if err := json.Unmarshal(createdResponse.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/settings/email-templates/"+created.ID, bytes.NewReader(oversizedBody))
	updateReq = updateReq.WithContext(authctx.WithUserID(updateReq.Context(), "user-1"))
	update := httptest.NewRecorder()
	mux.ServeHTTP(update, updateReq)
	if update.Code != http.StatusBadRequest || decodeErrorMessage(t, update) != ErrEmailTemplateValidation.Error() {
		t.Fatalf("unexpected oversized update response: %d %s", update.Code, update.Body.String())
	}
}

func TestHandlerEmailTemplateAllowsEscapeHeavyBodyWithinDomainLimit(t *testing.T) {
	svc, _, _, _ := newEmailTemplateTestService()
	mux := newSMTPTestHandlerMux(svc)
	input := SaveEmailTemplateInput{
		Name:     "Escape-heavy",
		Subject:  "Valid body",
		HTMLBody: strings.Repeat(`\`, 90*1024),
	}
	body, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	if len(body) <= 128*1024 || len(body) >= maxEmailTemplateRequestBytes {
		t.Fatalf("test body must exceed the old transport cap but fit the new cap: %d", len(body))
	}

	req := httptest.NewRequest(http.MethodPost, "/api/settings/email-templates", bytes.NewReader(body))
	req = req.WithContext(authctx.WithUserID(req.Context(), "user-1"))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201 for valid quote-heavy HTML, got %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestHandlerEmailTemplateTestEmailContract(t *testing.T) {
	svc, _, smtpRepo, sender := newEmailTemplateTestService()
	mux := newSMTPTestHandlerMux(svc)
	create := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates", "user-1", SaveEmailTemplateInput{Name: "活动", Subject: "已保存标题", HTMLBody: "<p>saved body</p>"})
	if create.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", create.Code, create.Body.String())
	}
	var created EmailTemplate
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	smtpRepo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls)}

	body := []byte(`{"recipientEmail":"recipient@example.com","subject":"unsaved"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings/email-templates/"+created.ID+"/test-email", bytes.NewReader(body))
	req = req.WithContext(authctx.WithUserID(req.Context(), "user-1"))
	bad := httptest.NewRecorder()
	mux.ServeHTTP(bad, req)
	if bad.Code != http.StatusBadRequest || decodeErrorMessage(t, bad) != ErrEmailTemplateValidation.Error() {
		t.Fatalf("unexpected override response: %d %s", bad.Code, bad.Body.String())
	}

	success := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates/"+created.ID+"/test-email", "user-1", TestEmailTemplateRequest{RecipientEmail: "recipient@example.com"})
	if success.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", success.Code, success.Body.String())
	}
	if len(sender.sent) != 1 || sender.sent[0].Subject != "已保存标题" || sender.sent[0].HTMLBody != "<p>saved body</p>" {
		t.Fatalf("expected saved content send, got %+v", sender.sent)
	}
}

func TestHandlerEmailTemplateSMTPErrors(t *testing.T) {
	svc, _, smtpRepo, sender := newEmailTemplateTestService()
	mux := newSMTPTestHandlerMux(svc)
	create := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates", "user-1", validEmailTemplateInput())
	var created EmailTemplate
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}

	missing := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates/"+created.ID+"/test-email", "user-1", TestEmailTemplateRequest{RecipientEmail: "recipient@example.com"})
	if missing.Code != http.StatusBadRequest || decodeErrorMessage(t, missing) != ErrSMTPMissingConfig.Error() {
		t.Fatalf("unexpected missing config response: %d %s", missing.Code, missing.Body.String())
	}
	smtpRepo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls)}
	invalidEmail := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates/"+created.ID+"/test-email", "user-1", TestEmailTemplateRequest{RecipientEmail: "bad\nemail@example.com"})
	if invalidEmail.Code != http.StatusBadRequest || decodeErrorMessage(t, invalidEmail) != ErrEmailTemplateInvalidEmail.Error() {
		t.Fatalf("unexpected invalid email response: %d %s", invalidEmail.Code, invalidEmail.Body.String())
	}
	sender.err = ErrSMTPSendFailed
	sendFailed := doSMTPRequest(mux, http.MethodPost, "/api/settings/email-templates/"+created.ID+"/test-email", "user-1", TestEmailTemplateRequest{RecipientEmail: "recipient@example.com"})
	if sendFailed.Code != http.StatusBadGateway || decodeErrorMessage(t, sendFailed) != ErrSMTPSendFailed.Error() {
		t.Fatalf("unexpected send failed response: %d %s", sendFailed.Code, sendFailed.Body.String())
	}
}
