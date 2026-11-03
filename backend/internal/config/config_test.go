package config

import "testing"

func TestSetEnvLineOverridesWhenKeyMarkedForOverride(t *testing.T) {
	t.Setenv("SOME_KEY", "online-value")

	setEnvLine("SOME_KEY=local-value", map[string]struct{}{"SOME_KEY": {}})

	if got := envOrDefault("SOME_KEY", ""); got != "local-value" {
		t.Fatalf("expected local value to override existing value, got %q", got)
	}
}

func TestSetEnvLinePreservesExistingValuesByDefault(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://online")

	setEnvLine("DATABASE_URL=postgres://local", nil)

	if got := envOrDefault("DATABASE_URL", ""); got != "postgres://online" {
		t.Fatalf("expected existing env value to be preserved, got %q", got)
	}
}

func TestLoadUsesHardcodedAppVersion(t *testing.T) {
	t.Setenv("APP_VERSION", "9.9.9")

	cfg := Load()

	if cfg.AppVersion != defaultAppVersion {
		t.Fatalf("expected hardcoded AppVersion %q, got %q", defaultAppVersion, cfg.AppVersion)
	}
}

// TestLoadDefaultsSMTPEncryptionKeyEmpty 校验未设置 SMTP_ENCRYPTION_KEY 时 Load() 正常返回空值，
// 保证现有部署在不配置该变量的情况下应用仍然可以启动（key 解析/校验由 settings 模块负责，不在这里做）。
func TestLoadDefaultsSMTPEncryptionKeyEmpty(t *testing.T) {
	t.Setenv("SMTP_ENCRYPTION_KEY", "")

	cfg := Load()

	if cfg.SMTPEncryptionKey != "" {
		t.Fatalf("expected empty SMTPEncryptionKey by default, got %q", cfg.SMTPEncryptionKey)
	}
}

// TestLoadReadsRawSMTPEncryptionKey 校验 Load() 原样读取非空环境变量，不做 base64/长度解析。
func TestLoadReadsRawSMTPEncryptionKey(t *testing.T) {
	t.Setenv("SMTP_ENCRYPTION_KEY", "not-a-valid-key-but-load-should-not-care")

	cfg := Load()

	if cfg.SMTPEncryptionKey != "not-a-valid-key-but-load-should-not-care" {
		t.Fatalf("expected raw SMTPEncryptionKey to be read as-is, got %q", cfg.SMTPEncryptionKey)
	}
}
