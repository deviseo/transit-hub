package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"transithub/backend/internal/shared/authctx"
)

func newSMTPTestHandlerMux(svc *Service) *http.ServeMux {
	mux := http.NewServeMux()
	RegisterRoutes(mux, svc)
	return mux
}

func doSMTPRequest(mux *http.ServeMux, method, path, userID string, body any) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body != nil {
		payload, _ := json.Marshal(body)
		reader = bytes.NewReader(payload)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	if userID != "" {
		req = req.WithContext(authctx.WithUserID(req.Context(), userID))
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)
	return recorder
}

func decodeErrorMessage(t *testing.T, recorder *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode error response: %v (body=%s)", err, recorder.Body.String())
	}
	return payload.Message
}

func TestHandlerGetSMTPUnauthorizedWithoutUser(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	mux := newSMTPTestHandlerMux(svc)

	recorder := doSMTPRequest(mux, http.MethodGet, "/api/settings/smtp", "", nil)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestHandlerGetSMTPNoCurrentWorkspaceReturns409(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), nil, true)
	mux := newSMTPTestHandlerMux(svc)

	recorder := doSMTPRequest(mux, http.MethodGet, "/api/settings/smtp", "user-1", nil)
	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if decodeErrorMessage(t, recorder) != "admin.adminAccounts.errors.noCurrentAccount" {
		t.Fatalf("unexpected message: %s", recorder.Body.String())
	}
}

func TestHandlerGetSMTPReturnsEmptyDefaultsWhenNoRecord(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	mux := newSMTPTestHandlerMux(svc)

	recorder := doSMTPRequest(mux, http.MethodGet, "/api/settings/smtp", "user-1", nil)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var got SmtpSettings
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.PasswordConfigured || got.TLSMode != SmtpTLSModeStarttls || got.Host != "" {
		t.Fatalf("unexpected default settings: %+v", got)
	}
}

func TestHandlerSaveSMTPValidationBadJSONReturns400(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	mux := newSMTPTestHandlerMux(svc)

	req := httptest.NewRequest(http.MethodPut, "/api/settings/smtp", bytes.NewReader([]byte("{not-json")))
	req = req.WithContext(authctx.WithUserID(req.Context(), "user-1"))
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if decodeErrorMessage(t, recorder) != ErrSMTPValidation.Error() {
		t.Fatalf("unexpected message: %s", recorder.Body.String())
	}
}

func TestHandlerSaveSMTPInvalidTLSModeReturns400(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	mux := newSMTPTestHandlerMux(svc)

	payload := map[string]any{
		"host": "smtp.example.com", "port": 587, "username": "", "password": "",
		"fromEmail": "mailer@example.com", "fromName": "TransitHub", "tlsMode": "none",
	}
	recorder := doSMTPRequest(mux, http.MethodPut, "/api/settings/smtp", "user-1", payload)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if decodeErrorMessage(t, recorder) != ErrSMTPInvalidTLSMode.Error() {
		t.Fatalf("unexpected message: %s", recorder.Body.String())
	}
}

func TestHandlerSaveSMTPSuccessReturns200WithoutPassword(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	mux := newSMTPTestHandlerMux(svc)

	payload := map[string]any{
		"host": "smtp.example.com", "port": 587, "username": "mailer@example.com", "password": "secret-pw",
		"fromEmail": "mailer@example.com", "fromName": "TransitHub", "tlsMode": "starttls",
	}
	recorder := doSMTPRequest(mux, http.MethodPut, "/api/settings/smtp", "user-1", payload)
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte("secret-pw")) {
		t.Fatalf("response must never contain the plaintext password: %s", recorder.Body.String())
	}
	var got SmtpSettings
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.PasswordConfigured {
		t.Fatalf("expected passwordConfigured=true")
	}
}

func TestHandlerSaveSMTPEncryptionKeyUnavailableReturns503(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, false)
	mux := newSMTPTestHandlerMux(svc)

	payload := map[string]any{
		"host": "smtp.example.com", "port": 587, "username": "mailer@example.com", "password": "secret-pw",
		"fromEmail": "mailer@example.com", "fromName": "TransitHub", "tlsMode": "starttls",
	}
	recorder := doSMTPRequest(mux, http.MethodPut, "/api/settings/smtp", "user-1", payload)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if decodeErrorMessage(t, recorder) != ErrSMTPEncryptionKeyUnavailable.Error() {
		t.Fatalf("unexpected message: %s", recorder.Body.String())
	}
}

func TestHandlerTestEmailMissingConfigReturns400(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	mux := newSMTPTestHandlerMux(svc)

	recorder := doSMTPRequest(mux, http.MethodPost, "/api/settings/smtp/test-email", "user-1", map[string]any{"recipientEmail": "admin@example.com"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if decodeErrorMessage(t, recorder) != ErrSMTPMissingConfig.Error() {
		t.Fatalf("unexpected message: %s", recorder.Body.String())
	}
}

func TestHandlerTestEmailInvalidRecipientReturns400(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	mux := newSMTPTestHandlerMux(svc)

	recorder := doSMTPRequest(mux, http.MethodPost, "/api/settings/smtp/test-email", "user-1", map[string]any{"recipientEmail": "not-an-email"})
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if decodeErrorMessage(t, recorder) != ErrSMTPInvalidEmail.Error() {
		t.Fatalf("unexpected message: %s", recorder.Body.String())
	}
}

func TestHandlerTestEmailDecryptFailureReturns503(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, Username: "mailer@example.com",
		PasswordCiphertext: "v1:not-valid-base64!!", FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	mux := newSMTPTestHandlerMux(svc)

	recorder := doSMTPRequest(mux, http.MethodPost, "/api/settings/smtp/test-email", "user-1", map[string]any{"recipientEmail": "admin@example.com"})
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if decodeErrorMessage(t, recorder) != ErrSMTPDecryptFailed.Error() {
		t.Fatalf("unexpected message: %s", recorder.Body.String())
	}
}

func TestHandlerTestEmailSendFailureReturns502(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	svc.smtpSender = &fakeSMTPSender{err: errors.New("connection refused")}
	mux := newSMTPTestHandlerMux(svc)

	recorder := doSMTPRequest(mux, http.MethodPost, "/api/settings/smtp/test-email", "user-1", map[string]any{"recipientEmail": "admin@example.com"})
	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if decodeErrorMessage(t, recorder) != ErrSMTPSendFailed.Error() {
		t.Fatalf("unexpected message: %s", recorder.Body.String())
	}
}

func TestHandlerTestEmailSuccessReturns200(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	svc.smtpSender = &fakeSMTPSender{}
	mux := newSMTPTestHandlerMux(svc)

	recorder := doSMTPRequest(mux, http.MethodPost, "/api/settings/smtp/test-email", "user-1", map[string]any{"recipientEmail": "admin@example.com"})
	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var got TestSmtpEmailResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !got.Success || got.Message != "admin.settings.smtp.testEmailSuccess" {
		t.Fatalf("unexpected response: %+v", got)
	}
}
