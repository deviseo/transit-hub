package httpserver

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/shared/secretbox"
)

// newSeamTestSettingsService 构造一个不依赖真实数据库/Redis 的 settings.Service，专用于
// SMTP_ENCRYPTION_KEY 组装 seam 测试。configureSMTPEncryptionKey 只调用
// settingsService.SetSMTPEncryptionKey，不触碰 repository，所以底层 *pgxpool.Pool 可以是 nil。
func newSeamTestSettingsService() *settings.Service {
	return settings.NewService(nil, settings.NewRepository(nil))
}

func TestConfigureSMTPEncryptionKeyEmptySucceedsAndReportsUnavailable(t *testing.T) {
	svc := newSeamTestSettingsService()
	source, err := configureSMTPEncryptionKey(svc, "", nil)
	if err != nil {
		t.Fatalf("expected empty SMTP_ENCRYPTION_KEY to construct successfully, got %v", err)
	}
	if source != smtpKeyNone {
		t.Fatalf("expected empty SMTP_ENCRYPTION_KEY without a credential key to report unavailable encryption, got %q", source)
	}
}

func TestConfigureSMTPEncryptionKeyValidReportsConfigured(t *testing.T) {
	svc := newSeamTestSettingsService()
	validKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x07}, 32))
	source, err := configureSMTPEncryptionKey(svc, validKey, nil)
	if err != nil {
		t.Fatalf("expected valid SMTP_ENCRYPTION_KEY to construct successfully, got %v", err)
	}
	if source != smtpKeyEnv {
		t.Fatalf("expected valid SMTP_ENCRYPTION_KEY to be used, got %q", source)
	}
}

func TestConfigureSMTPEncryptionKeyInvalidFailsWithSanitizedError(t *testing.T) {
	svc := newSeamTestSettingsService()
	badKey := "not-a-valid-base64-key!!!"
	source, err := configureSMTPEncryptionKey(svc, badKey, nil)
	if err == nil {
		t.Fatalf("expected explicit invalid SMTP_ENCRYPTION_KEY to fail construction")
	}
	if strings.Contains(err.Error(), badKey) {
		t.Fatalf("error must not leak the raw key content, got %v", err)
	}
	if source != smtpKeyNone {
		t.Fatalf("expected invalid SMTP_ENCRYPTION_KEY to report unavailable encryption, got %q", source)
	}
}

// TestConfigureSMTPEncryptionKeyInvalidLengthFailsWithSanitizedError 覆盖“合法 base64 但字节数不对”
// 这一类非法 key，同样必须失败且不泄露原始输入。
func TestConfigureSMTPEncryptionKeyInvalidLengthFailsWithSanitizedError(t *testing.T) {
	svc := newSeamTestSettingsService()
	shortKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 16))
	source, err := configureSMTPEncryptionKey(svc, shortKey, nil)
	if err == nil {
		t.Fatalf("expected a non-32-byte key to fail construction")
	}
	if strings.Contains(err.Error(), shortKey) {
		t.Fatalf("error must not leak the raw key content, got %v", err)
	}
	if source != smtpKeyNone {
		t.Fatalf("expected invalid SMTP_ENCRYPTION_KEY to report unavailable encryption, got %q", source)
	}
}

func TestConfigureSMTPEncryptionKeyWhitespaceOnlyFailsWithSanitizedError(t *testing.T) {
	svc := newSeamTestSettingsService()
	rawKey := "   	  "
	source, err := configureSMTPEncryptionKey(svc, rawKey, nil)
	if err == nil {
		t.Fatalf("expected whitespace-only SMTP_ENCRYPTION_KEY to fail construction")
	}
	if strings.Contains(err.Error(), rawKey) {
		t.Fatalf("error must not leak the raw key content, got %v", err)
	}
	if source != smtpKeyNone {
		t.Fatalf("expected whitespace-only SMTP_ENCRYPTION_KEY to report unavailable encryption, got %q", source)
	}
}

// 没配 SMTP_ENCRYPTION_KEY 时，用凭据密钥派生的密钥：保存 SMTP 密码开箱即用。
func TestConfigureSMTPEncryptionKeyFallsBackToTheCredentialKey(t *testing.T) {
	box, err := secretbox.New(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	source, err := configureSMTPEncryptionKey(newSeamTestSettingsService(), "", box)
	if err != nil || source != smtpKeyDerived {
		t.Fatalf("expected the derived key, got %q %v", source, err)
	}
	// 配了 SMTP_ENCRYPTION_KEY 时仍以它为准（派生的只用来解密以前的密码）。
	validKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x07}, 32))
	source, err = configureSMTPEncryptionKey(newSeamTestSettingsService(), validKey, box)
	if err != nil || source != smtpKeyEnv {
		t.Fatalf("SMTP_ENCRYPTION_KEY must win, got %q %v", source, err)
	}
	// 非法的 SMTP_ENCRYPTION_KEY 即使有凭据密钥也要启动失败。
	if _, err := configureSMTPEncryptionKey(newSeamTestSettingsService(), "not-base64!!", box); err == nil {
		t.Fatal("an invalid SMTP_ENCRYPTION_KEY must still fail startup")
	}
}
