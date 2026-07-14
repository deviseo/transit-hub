package httpserver

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"transithub/backend/internal/modules/settings"
)

// newSeamTestSettingsService 构造一个不依赖真实数据库/Redis 的 settings.Service，专用于
// SMTP_ENCRYPTION_KEY 组装 seam 测试。configureSMTPEncryptionKey 只调用
// settingsService.SetSMTPEncryptionKey，不触碰 repository，所以底层 *pgxpool.Pool 可以是 nil。
func newSeamTestSettingsService() *settings.Service {
	return settings.NewService(nil, settings.NewRepository(nil))
}

func TestConfigureSMTPEncryptionKeyEmptySucceedsAndReportsUnavailable(t *testing.T) {
	svc := newSeamTestSettingsService()
	configured, err := configureSMTPEncryptionKey(svc, "")
	if err != nil {
		t.Fatalf("expected empty SMTP_ENCRYPTION_KEY to construct successfully, got %v", err)
	}
	if configured {
		t.Fatalf("expected empty SMTP_ENCRYPTION_KEY to report unavailable encryption")
	}
}

func TestConfigureSMTPEncryptionKeyValidReportsConfigured(t *testing.T) {
	svc := newSeamTestSettingsService()
	validKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x07}, 32))
	configured, err := configureSMTPEncryptionKey(svc, validKey)
	if err != nil {
		t.Fatalf("expected valid SMTP_ENCRYPTION_KEY to construct successfully, got %v", err)
	}
	if !configured {
		t.Fatalf("expected valid SMTP_ENCRYPTION_KEY to report configured encryption")
	}
}

func TestConfigureSMTPEncryptionKeyInvalidFailsWithSanitizedError(t *testing.T) {
	svc := newSeamTestSettingsService()
	badKey := "not-a-valid-base64-key!!!"
	configured, err := configureSMTPEncryptionKey(svc, badKey)
	if err == nil {
		t.Fatalf("expected explicit invalid SMTP_ENCRYPTION_KEY to fail construction")
	}
	if strings.Contains(err.Error(), badKey) {
		t.Fatalf("error must not leak the raw key content, got %v", err)
	}
	if configured {
		t.Fatalf("expected invalid SMTP_ENCRYPTION_KEY to report unavailable encryption")
	}
}

// TestConfigureSMTPEncryptionKeyInvalidLengthFailsWithSanitizedError 覆盖“合法 base64 但字节数不对”
// 这一类非法 key，同样必须失败且不泄露原始输入。
func TestConfigureSMTPEncryptionKeyInvalidLengthFailsWithSanitizedError(t *testing.T) {
	svc := newSeamTestSettingsService()
	shortKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x01}, 16))
	configured, err := configureSMTPEncryptionKey(svc, shortKey)
	if err == nil {
		t.Fatalf("expected a non-32-byte key to fail construction")
	}
	if strings.Contains(err.Error(), shortKey) {
		t.Fatalf("error must not leak the raw key content, got %v", err)
	}
	if configured {
		t.Fatalf("expected invalid SMTP_ENCRYPTION_KEY to report unavailable encryption")
	}
}

func TestConfigureSMTPEncryptionKeyWhitespaceOnlyFailsWithSanitizedError(t *testing.T) {
	svc := newSeamTestSettingsService()
	rawKey := "   	  "
	configured, err := configureSMTPEncryptionKey(svc, rawKey)
	if err == nil {
		t.Fatalf("expected whitespace-only SMTP_ENCRYPTION_KEY to fail construction")
	}
	if strings.Contains(err.Error(), rawKey) {
		t.Fatalf("error must not leak the raw key content, got %v", err)
	}
	if configured {
		t.Fatalf("expected whitespace-only SMTP_ENCRYPTION_KEY to report unavailable encryption")
	}
}
