package settings

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeSMTPRepository is an in-memory smtpRepository test double, keyed by (userID, adminAccountID),
// so SMTP service tests never touch a real database (matches this backend's testing convention).
type fakeSMTPRepository struct {
	rows map[string]smtpRow
}

func newFakeSMTPRepository() *fakeSMTPRepository {
	return &fakeSMTPRepository{rows: map[string]smtpRow{}}
}

func smtpRepoKey(userID, adminAccountID string) string {
	return userID + "|" + adminAccountID
}

func (f *fakeSMTPRepository) GetSMTPSettings(_ context.Context, userID string, adminAccountID string) (smtpRow, error) {
	row, ok := f.rows[smtpRepoKey(userID, adminAccountID)]
	if !ok {
		return defaultSMTPRow(), nil
	}
	return row, nil
}

func (f *fakeSMTPRepository) SaveSMTPSettings(_ context.Context, userID string, adminAccountID string, row smtpRow) error {
	f.rows[smtpRepoKey(userID, adminAccountID)] = row
	return nil
}

// fakeAdminAccountResolver is an AdminAccountResolver test double.
type fakeAdminAccountResolver struct {
	id  string
	err error
}

func (f *fakeAdminAccountResolver) RequireCurrentID(_ context.Context, _ string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.id, nil
}

// fakeSMTPSender is a smtpSender test double, avoiding any dependency on a real external SMTP service.
type fakeSMTPSender struct {
	err  error
	sent []smtpSendConfig
}

func (f *fakeSMTPSender) Send(_ context.Context, cfg smtpSendConfig) error {
	f.sent = append(f.sent, cfg)
	return f.err
}

func newTestSMTPService(t *testing.T, repo *fakeSMTPRepository, resolver AdminAccountResolver, gcmAvailable bool) *Service {
	t.Helper()
	svc := &Service{smtpRepo: repo, accounts: resolver}
	if gcmAvailable {
		key, err := ParseSMTPEncryptionKey(validTestKey())
		if err != nil {
			t.Fatalf("parse test key: %v", err)
		}
		svc.smtpKeyGCM = key
	}
	return svc
}

func validSaveInput() SaveSmtpSettingsInput {
	return SaveSmtpSettingsInput{
		Host:      "smtp.example.com",
		Port:      587,
		Username:  "mailer@example.com",
		Password:  "",
		FromEmail: "mailer@example.com",
		FromName:  "TransitHub",
		TLSMode:   SmtpTLSModeStarttls,
	}
}

func TestSaveSMTPSettingsValidationHostRequired(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.Host = "   "
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPValidation {
		t.Fatalf("expected ErrSMTPValidation, got %v", err)
	}
}

func TestSaveSMTPSettingsValidationPortRange(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.Port = 0
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPValidation {
		t.Fatalf("expected ErrSMTPValidation for port=0, got %v", err)
	}
	input.Port = 70000
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPValidation {
		t.Fatalf("expected ErrSMTPValidation for port=70000, got %v", err)
	}
}

func TestSaveSMTPSettingsValidationInvalidFromEmail(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.FromEmail = "not-an-email"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPInvalidEmail {
		t.Fatalf("expected ErrSMTPInvalidEmail, got %v", err)
	}
}

func TestSaveSMTPSettingsRejectsTLSModeNone(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.TLSMode = "none"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPInvalidTLSMode {
		t.Fatalf("expected ErrSMTPInvalidTLSMode for tlsMode=none, got %v", err)
	}
}

func TestSaveSMTPSettingsRejectsCRLFInjection(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.FromName = "Evil\r\nBcc: attacker@example.com"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPValidation {
		t.Fatalf("expected ErrSMTPValidation for CRLF injection, got %v", err)
	}
}

func TestSaveSMTPSettingsRejectsOrphanPasswordWithoutUsername(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.Username = ""
	input.Password = "some-password"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPValidation {
		t.Fatalf("expected ErrSMTPValidation for password without username, got %v", err)
	}
}

func TestSaveSMTPSettingsWithoutEncryptionKeyRejectsNonEmptyPassword(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, false)
	input := validSaveInput()
	input.Password = "some-password"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPEncryptionKeyUnavailable {
		t.Fatalf("expected ErrSMTPEncryptionKeyUnavailable, got %v", err)
	}
}

func TestSaveSMTPSettingsPersistsAndEncryptsPassword(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.Password = "super-secret"
	result, err := svc.SaveSMTPSettings(context.Background(), "user-1", input)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if !result.PasswordConfigured {
		t.Fatalf("expected passwordConfigured=true after saving non-empty password")
	}
	row := repo.rows[smtpRepoKey("user-1", "acct-1")]
	if row.PasswordCiphertext == "" {
		t.Fatalf("expected password_ciphertext to be persisted")
	}
	if row.PasswordCiphertext == input.Password {
		t.Fatalf("password_ciphertext must not equal plaintext")
	}
}

// TestSaveSMTPSettingsEmptyPasswordPreservesExistingCiphertext 校验 PUT 省略/空密码时保留已有密文，
// 而不是清空——这是规格明确要求的语义（一期没有清空密码功能）。
func TestSaveSMTPSettingsEmptyPasswordPreservesExistingCiphertext(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)

	first := validSaveInput()
	first.Password = "initial-secret"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", first); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	originalCiphertext := repo.rows[smtpRepoKey("user-1", "acct-1")].PasswordCiphertext

	second := validSaveInput()
	second.Host = "smtp2.example.com"
	second.Password = "   " // whitespace-only must be treated as omitted
	result, err := svc.SaveSMTPSettings(context.Background(), "user-1", second)
	if err != nil {
		t.Fatalf("second save: %v", err)
	}
	if !result.PasswordConfigured {
		t.Fatalf("expected passwordConfigured to remain true")
	}
	if repo.rows[smtpRepoKey("user-1", "acct-1")].PasswordCiphertext != originalCiphertext {
		t.Fatalf("expected existing ciphertext to be preserved on empty password PUT")
	}
	if repo.rows[smtpRepoKey("user-1", "acct-1")].Host != "smtp2.example.com" {
		t.Fatalf("expected other fields to still be updated")
	}
}

func TestGetSMTPSettingsNeverExposesPassword(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.Password = "super-secret"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := svc.GetSMTPSettings(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.PasswordConfigured {
		t.Fatalf("expected passwordConfigured=true")
	}
	// SmtpSettings 类型本身没有 password 字段，这里额外确认 host/from 字段可见但密码从不出现。
	if got.Host != input.Host {
		t.Fatalf("expected host to round-trip")
	}
}

func TestGetSMTPSettingsNoRecordReturnsEmptyDefaults(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	got, err := svc.GetSMTPSettings(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.PasswordConfigured {
		t.Fatalf("expected passwordConfigured=false for no record")
	}
	if got.TLSMode != SmtpTLSModeStarttls {
		t.Fatalf("expected default tlsMode=starttls, got %v", got.TLSMode)
	}
	if got.Port != 587 {
		t.Fatalf("expected default port=587 for no record, got %d", got.Port)
	}
	if got.Host != "" || got.UpdatedAt != nil {
		t.Fatalf("expected empty defaults for no record, got %+v", got)
	}
}

// TestSMTPSettingsWorkspaceIsolation 校验不同 (user_id, admin_account_id) 的配置互不可见。
func TestSMTPSettingsWorkspaceIsolation(t *testing.T) {
	repo := newFakeSMTPRepository()
	svcAccount1 := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	svcAccount2 := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-2"}, true)

	inputA := validSaveInput()
	inputA.Host = "acct1.example.com"
	if _, err := svcAccount1.SaveSMTPSettings(context.Background(), "user-1", inputA); err != nil {
		t.Fatalf("save account 1: %v", err)
	}

	got2, err := svcAccount2.GetSMTPSettings(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get account 2: %v", err)
	}
	if got2.Host != "" {
		t.Fatalf("expected account 2 to see no config, got host=%s", got2.Host)
	}

	got1, err := svcAccount1.GetSMTPSettings(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("get account 1: %v", err)
	}
	if got1.Host != "acct1.example.com" {
		t.Fatalf("expected account 1 to see its own config, got host=%s", got1.Host)
	}
}

func TestSMTPSettingsMissingWorkspaceReturnsWorkspaceError(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), nil, true)
	_, err := svc.GetSMTPSettings(context.Background(), "user-1")
	if err == nil || err.Error() != "admin.adminAccounts.errors.noCurrentAccount" {
		t.Fatalf("expected noCurrentAccount workspace error, got %v", err)
	}
}

func TestSMTPSettingsWorkspaceResolverErrorPropagates(t *testing.T) {
	resolverErr := errors.New("admin.adminAccounts.errors.noCurrentAccount")
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{err: resolverErr}, true)
	_, err := svc.SaveSMTPSettings(context.Background(), "user-1", validSaveInput())
	if err != resolverErr {
		t.Fatalf("expected resolver error to propagate, got %v", err)
	}
}

func TestTestSMTPEmailMissingConfigWhenNoRecord(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	err := svc.TestSMTPEmail(context.Background(), "user-1", "admin@example.com")
	if err != ErrSMTPMissingConfig {
		t.Fatalf("expected ErrSMTPMissingConfig, got %v", err)
	}
}

func TestTestSMTPEmailMissingConfigWhenUsernameWithoutPassword(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, Username: "mailer@example.com",
		FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	err := svc.TestSMTPEmail(context.Background(), "user-1", "admin@example.com")
	if err != ErrSMTPMissingConfig {
		t.Fatalf("expected ErrSMTPMissingConfig for username without saved password, got %v", err)
	}
}

func TestTestSMTPEmailMissingConfigWhenPasswordWithoutUsername(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587,
		PasswordCiphertext: "v1:legacy-ciphertext", FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	sender := &fakeSMTPSender{}
	svc.smtpSender = sender

	err := svc.TestSMTPEmail(context.Background(), "user-1", "admin@example.com")
	if err != ErrSMTPMissingConfig {
		t.Fatalf("expected ErrSMTPMissingConfig for legacy password without username, got %v", err)
	}
	if len(sender.sent) != 0 {
		t.Fatalf("expected sender not to be called, got %d sends", len(sender.sent))
	}
}

func TestTestSMTPEmailInvalidRecipientEmail(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	err := svc.TestSMTPEmail(context.Background(), "user-1", "not-an-email")
	if err != ErrSMTPInvalidEmail {
		t.Fatalf("expected ErrSMTPInvalidEmail, got %v", err)
	}
}

func TestTestSMTPEmailEncryptionKeyUnavailableWhenPasswordSavedButKeyMissing(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, Username: "mailer@example.com",
		PasswordCiphertext: "v1:doesnotmatter", FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, false)
	err := svc.TestSMTPEmail(context.Background(), "user-1", "admin@example.com")
	if err != ErrSMTPEncryptionKeyUnavailable {
		t.Fatalf("expected ErrSMTPEncryptionKeyUnavailable, got %v", err)
	}
}

func TestTestSMTPEmailDecryptFailureNotReportedAsAuthFailure(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, Username: "mailer@example.com",
		PasswordCiphertext: "v1:corrupted-not-base64!!", FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	err := svc.TestSMTPEmail(context.Background(), "user-1", "admin@example.com")
	if err != ErrSMTPDecryptFailed {
		t.Fatalf("expected ErrSMTPDecryptFailed, got %v", err)
	}
}

func TestTestSMTPEmailSendSuccessUsesSavedConfigNotRequestBody(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", FromName: "TransitHub",
		TLSMode: string(SmtpTLSModeStarttls),
	}
	sender := &fakeSMTPSender{}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	svc.smtpSender = sender

	if err := svc.TestSMTPEmail(context.Background(), "user-1", "recipient@example.com"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected exactly one send attempt, got %d", len(sender.sent))
	}
	sent := sender.sent[0]
	if sent.Host != "smtp.example.com" || sent.FromEmail != "mailer@example.com" || sent.RecipientEmail != "recipient@example.com" {
		t.Fatalf("expected sender to receive saved config, got %+v", sent)
	}
}

func TestTestSMTPEmailSendFailureMapsToSendFailed(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	svc.smtpSender = &fakeSMTPSender{err: errors.New("connection refused")}

	err := svc.TestSMTPEmail(context.Background(), "user-1", "recipient@example.com")
	if err != ErrSMTPSendFailed {
		t.Fatalf("expected ErrSMTPSendFailed, got %v", err)
	}
}

// --- Remediation: password byte-fidelity (docs/development-docs/claude-code-smtp-phase1-remediation-task.md #1) ---

func TestSaveSMTPSettingsPreservesPasswordLeadingAndTrailingSpaces(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.Username = "mailer@example.com"
	input.Password = "  secret with spaces  "
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != nil {
		t.Fatalf("save: %v", err)
	}
	row := repo.rows[smtpRepoKey("user-1", "acct-1")]
	decrypted, err := decryptSMTPPassword(svc.smtpKeyGCM, "user-1", "acct-1", row.PasswordCiphertext)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if decrypted != "  secret with spaces  " {
		t.Fatalf("expected password bytes preserved exactly including leading/trailing spaces, got %q", decrypted)
	}
}

func TestSaveSMTPSettingsWhitespaceOnlyPasswordPreservesExistingCiphertextExactly(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)

	first := validSaveInput()
	first.Username = "mailer@example.com"
	first.Password = "original-secret"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", first); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	originalCiphertext := repo.rows[smtpRepoKey("user-1", "acct-1")].PasswordCiphertext

	second := validSaveInput()
	second.Username = "mailer@example.com"
	second.Password = "   "
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", second); err != nil {
		t.Fatalf("second save: %v", err)
	}
	if repo.rows[smtpRepoKey("user-1", "acct-1")].PasswordCiphertext != originalCiphertext {
		t.Fatalf("expected whitespace-only password input to preserve the existing ciphertext untouched")
	}
}

// --- Remediation: auth-degrade state must be rejected, not silently kept (#2) ---

func TestSaveSMTPSettingsRejectsAuthDegradeWhenClearingUsernameWithExistingCiphertext(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)

	initial := validSaveInput()
	initial.Username = "mailer@example.com"
	initial.Password = "initial-secret"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", initial); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	before := repo.rows[smtpRepoKey("user-1", "acct-1")].PasswordCiphertext

	degrade := validSaveInput()
	degrade.Username = ""
	degrade.Password = ""
	_, err := svc.SaveSMTPSettings(context.Background(), "user-1", degrade)
	if err != ErrSMTPValidation {
		t.Fatalf("expected ErrSMTPValidation when clearing username with an existing ciphertext, got %v", err)
	}
	after := repo.rows[smtpRepoKey("user-1", "acct-1")].PasswordCiphertext
	if after != before {
		t.Fatalf("expected existing ciphertext to remain untouched after a rejected auth-degrade save")
	}
}

func TestSaveSMTPSettingsAllowsUnauthenticatedWhenNoExistingCiphertextAndUsernameBlank(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.Username = ""
	input.Password = ""
	result, err := svc.SaveSMTPSettings(context.Background(), "user-1", input)
	if err != nil {
		t.Fatalf("expected unauthenticated SMTP save to succeed when there is no existing ciphertext, got %v", err)
	}
	if result.PasswordConfigured {
		t.Fatalf("expected passwordConfigured=false for unauthenticated SMTP")
	}
}

func TestSaveSMTPSettingsKeepsUsernameAndPreservesCiphertextWhenPasswordBlank(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)

	initial := validSaveInput()
	initial.Username = "mailer@example.com"
	initial.Password = "initial-secret"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", initial); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	before := repo.rows[smtpRepoKey("user-1", "acct-1")].PasswordCiphertext

	keepUsername := validSaveInput()
	keepUsername.Username = "mailer@example.com"
	keepUsername.Password = ""
	result, err := svc.SaveSMTPSettings(context.Background(), "user-1", keepUsername)
	if err != nil {
		t.Fatalf("expected keeping username with blank password to succeed, got %v", err)
	}
	if !result.PasswordConfigured {
		t.Fatalf("expected passwordConfigured=true to remain")
	}
	if repo.rows[smtpRepoKey("user-1", "acct-1")].PasswordCiphertext != before {
		t.Fatalf("expected ciphertext to remain preserved")
	}
}

// --- Remediation: envelope addresses must be bare addr-spec, not display-name form (#3) ---

func TestSaveSMTPSettingsRejectsDisplayNameFromEmail(t *testing.T) {
	svc := newTestSMTPService(t, newFakeSMTPRepository(), &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.FromEmail = "Alice <alice@example.com>"
	if _, err := svc.SaveSMTPSettings(context.Background(), "user-1", input); err != ErrSMTPInvalidEmail {
		t.Fatalf("expected ErrSMTPInvalidEmail for display-name fromEmail, got %v", err)
	}
}

func TestSaveSMTPSettingsAcceptsBareFromEmailAndStoresBareAddress(t *testing.T) {
	repo := newFakeSMTPRepository()
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	input := validSaveInput()
	input.FromEmail = " alice@example.com "
	result, err := svc.SaveSMTPSettings(context.Background(), "user-1", input)
	if err != nil {
		t.Fatalf("expected bare address (with surrounding whitespace) to be accepted, got %v", err)
	}
	if result.FromEmail != "alice@example.com" {
		t.Fatalf("expected stored/returned fromEmail to be the bare trimmed address, got %q", result.FromEmail)
	}
}

func TestTestSMTPEmailRejectsDisplayNameRecipient(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", TLSMode: string(SmtpTLSModeStarttls),
	}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	err := svc.TestSMTPEmail(context.Background(), "user-1", "Bob <bob@example.com>")
	if err != ErrSMTPInvalidEmail {
		t.Fatalf("expected ErrSMTPInvalidEmail for display-name recipient, got %v", err)
	}
}

func TestTestSMTPEmailUsesBareEnvelopeRecipientAddress(t *testing.T) {
	repo := newFakeSMTPRepository()
	repo.rows[smtpRepoKey("user-1", "acct-1")] = smtpRow{
		Host: "smtp.example.com", Port: 587, FromEmail: "mailer@example.com", FromName: "TransitHub", TLSMode: string(SmtpTLSModeStarttls),
	}
	sender := &fakeSMTPSender{}
	svc := newTestSMTPService(t, repo, &fakeAdminAccountResolver{id: "acct-1"}, true)
	svc.smtpSender = sender
	if err := svc.TestSMTPEmail(context.Background(), "user-1", " recipient@example.com "); err != nil {
		t.Fatalf("send: %v", err)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("expected exactly one send attempt")
	}
	if sender.sent[0].RecipientEmail != "recipient@example.com" {
		t.Fatalf("expected bare trimmed recipient address, got %q", sender.sent[0].RecipientEmail)
	}
	if sender.sent[0].FromEmail != "mailer@example.com" {
		t.Fatalf("expected bare fromEmail envelope address, got %q", sender.sent[0].FromEmail)
	}
}

// --- Remediation: persistence errors preserve cause internally, public response stays stable (#6) ---

// erroringSMTPRepository is a smtpRepository test double whose Get/Save calls fail with a
// caller-supplied cause, used to assert that the service wraps repository errors with %w
// around ErrSMTPPersistence (so errors.Is still matches) while retaining the underlying cause
// for internal diagnostics.
type erroringSMTPRepository struct {
	getErr  error
	saveErr error
}

func (e *erroringSMTPRepository) GetSMTPSettings(_ context.Context, _ string, _ string) (smtpRow, error) {
	if e.getErr != nil {
		return smtpRow{}, e.getErr
	}
	return defaultSMTPRow(), nil
}

func (e *erroringSMTPRepository) SaveSMTPSettings(_ context.Context, _ string, _ string, _ smtpRow) error {
	return e.saveErr
}

func TestGetSMTPSettingsWrapsPersistenceErrorPreservingCause(t *testing.T) {
	cause := errors.New("dial tcp: connection refused")
	svc := &Service{smtpRepo: &erroringSMTPRepository{getErr: cause}, accounts: &fakeAdminAccountResolver{id: "acct-1"}}
	_, err := svc.GetSMTPSettings(context.Background(), "user-1")
	if !errors.Is(err, ErrSMTPPersistence) {
		t.Fatalf("expected errors.Is(err, ErrSMTPPersistence) to be true, got %v", err)
	}
	if !strings.Contains(err.Error(), cause.Error()) {
		t.Fatalf("expected wrapped error to retain the underlying cause for internal diagnostics, got %v", err)
	}
}

func TestSaveSMTPSettingsWrapsPersistenceErrorPreservingCause(t *testing.T) {
	cause := errors.New("unique_violation: duplicate key")
	svc := &Service{smtpRepo: &erroringSMTPRepository{saveErr: cause}, accounts: &fakeAdminAccountResolver{id: "acct-1"}}
	_, err := svc.SaveSMTPSettings(context.Background(), "user-1", validSaveInput())
	if !errors.Is(err, ErrSMTPPersistence) {
		t.Fatalf("expected errors.Is(err, ErrSMTPPersistence) to be true, got %v", err)
	}
	if !strings.Contains(err.Error(), cause.Error()) {
		t.Fatalf("expected wrapped error to retain the underlying cause, got %v", err)
	}
}
