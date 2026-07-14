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

// TestLoadDefaultsTicketUploadDir 校验未设置 TICKET_UPLOAD_DIR 时使用预期默认值，
// 保证线上旧部署（没有显式配置这个环境变量）不会因为升级而意外改变附件存储位置。
// 用空字符串模拟"未设置"：envOrDefault 对空字符串和真正未设置的行为完全一致（见 envOrDefault
// 实现，两者 os.Getenv 都返回空串），且 t.Setenv 会在测试结束后自动恢复，不污染其它测试。
func TestLoadDefaultsTicketUploadDir(t *testing.T) {
	t.Setenv("TICKET_UPLOAD_DIR", "")

	cfg := Load()

	if cfg.TicketUploadDir != "data/ticket-uploads" {
		t.Fatalf("expected default TicketUploadDir %q, got %q", "data/ticket-uploads", cfg.TicketUploadDir)
	}
}

// TestLoadOverridesTicketUploadDir 校验环境变量可以覆盖默认存储目录，
// 对应生产部署中 TICKET_UPLOAD_DIR 指向持久化 volume 挂载路径的场景。
func TestLoadOverridesTicketUploadDir(t *testing.T) {
	t.Setenv("TICKET_UPLOAD_DIR", "/app/data/ticket-uploads")

	cfg := Load()

	if cfg.TicketUploadDir != "/app/data/ticket-uploads" {
		t.Fatalf("expected overridden TicketUploadDir %q, got %q", "/app/data/ticket-uploads", cfg.TicketUploadDir)
	}
}

func TestLoadUsesHardcodedAppVersion(t *testing.T) {
	t.Setenv("APP_VERSION", "9.9.9")

	cfg := Load()

	if cfg.AppVersion != defaultAppVersion {
		t.Fatalf("expected hardcoded AppVersion %q, got %q", defaultAppVersion, cfg.AppVersion)
	}
}

func TestLoadDefaultsLotteryPrivateTargetsToDisabled(t *testing.T) {
	t.Setenv("LOTTERY_ALLOW_PRIVATE_SUB2API_TARGETS", "")

	cfg := Load()

	if cfg.LotteryAllowPrivateSub2APITargets {
		t.Fatal("expected private Sub2API lottery targets to be disabled by default")
	}
}

func TestLoadCanEnableLotteryPrivateTargetsForLocalDebugging(t *testing.T) {
	t.Setenv("LOTTERY_ALLOW_PRIVATE_SUB2API_TARGETS", "true")

	cfg := Load()

	if !cfg.LotteryAllowPrivateSub2APITargets {
		t.Fatal("expected private Sub2API lottery targets to be enabled")
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
