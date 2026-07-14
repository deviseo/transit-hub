package settings

import (
	"strings"
	"testing"
)

// TestDefaultSMTPRowHasPort587AndStarttls 校验无记录时 Repository.GetSMTPSettings 使用的默认值
// 契约：port 587、tlsMode starttls。GetSMTPSettings 本身需要真实 Postgres 才能执行（本项目约定
// 测试不连接真实数据库），因此针对它实际使用的具名默认值构造函数 defaultSMTPRow 做单元断言，
// 保证前端不再需要用 `|| 587` 掩盖 API 合同缺口。
func TestDefaultSMTPRowHasPort587AndStarttls(t *testing.T) {
	row := defaultSMTPRow()
	if row.Port != 587 {
		t.Fatalf("expected default port 587, got %d", row.Port)
	}
	if row.TLSMode != string(SmtpTLSModeStarttls) {
		t.Fatalf("expected default tlsMode starttls, got %q", row.TLSMode)
	}
	if row.PasswordCiphertext != "" || row.Host != "" || row.FromEmail != "" {
		t.Fatalf("expected all other default fields to be empty, got %+v", row)
	}
}

func TestDefaultMarketingEmailTemplateContract(t *testing.T) {
	template := defaultMarketingEmailTemplate()
	if template.ID != builtInMarketingTemplateID {
		t.Fatalf("expected stable built-in id %q, got %q", builtInMarketingTemplateID, template.ID)
	}
	if template.Name != "营销活动" || template.Subject != "TransitHub 限时活动" || !template.IsBuiltIn {
		t.Fatalf("unexpected built-in metadata: %+v", template)
	}
	if template.HTMLBody == "" || strings.Contains(template.HTMLBody, "<script") || strings.Contains(template.HTMLBody, "http://") || strings.Contains(template.HTMLBody, "https://") {
		t.Fatalf("built-in html must be self-contained and script-free")
	}
}

func TestCreateEmailTemplateSQLAppliesCustomLimitAtomically(t *testing.T) {
	if !strings.Contains(createEmailTemplateSQL, "INSERT INTO email_templates") || !strings.Contains(createEmailTemplateSQL, "WHERE (") {
		t.Fatalf("create sql must insert through a conditional statement")
	}
	if !strings.Contains(createEmailTemplateSQL, "SELECT count(*)") || !strings.Contains(createEmailTemplateSQL, "is_builtin = false") || !strings.Contains(createEmailTemplateSQL, ") < $7") {
		t.Fatalf("create sql must apply the custom-template limit inside the insert statement: %s", createEmailTemplateSQL)
	}
	if !strings.Contains(createEmailTemplateSQL, "pg_advisory_xact_lock") {
		t.Fatalf("create sql must serialize concurrent workspace creates across processes: %s", createEmailTemplateSQL)
	}
}
