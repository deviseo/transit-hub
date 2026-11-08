package httpserver

import (
	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/shared/secretbox"
)

// SMTP 密码用的是哪一把密钥（只用于启动日志，不含任何密钥内容）。
const (
	smtpKeyNone    = ""
	smtpKeyEnv     = "SMTP_ENCRYPTION_KEY"
	smtpKeyDerived = "a key derived from the credential key"
)

// smtpKeyPurpose 是从凭据密钥派生 SMTP 密钥时的用途名；改了它，以前保存的 SMTP 密码就解不开了。
const smtpKeyPurpose = "smtp-password"

// configureSMTPEncryptionKey 是 server.New 实际调用的窄组装 seam：安装 SMTP 密码的加密密钥，返回用的是哪一把。
//   - 配了 SMTP_ENCRYPTION_KEY：用它，以前保存的密码照旧能解开；凭据密钥派生的那把只用来解开用它加密过的密码。
//   - 没配：用凭据密钥（CREDENTIAL_ENCRYPTION_KEY 或数据目录里的 credential.key）派生的密钥，开箱即用，
//     不需要再单独配置。
//   - 两者都没有（数据目录不是持久卷、也没配任何密钥）：返回 smtpKeyNone，不能保存 SMTP 密码。
//
// 显式配置了非法的 SMTP_ENCRYPTION_KEY（非 base64 或不是 32 字节）返回错误，调用方让启动失败；错误里不带密钥原文。
func configureSMTPEncryptionKey(settingsService *settings.Service, rawKey string, credentials *secretbox.Box) (string, error) {
	if err := settingsService.SetSMTPEncryptionKey(rawKey); err != nil {
		return smtpKeyNone, err
	}
	source := smtpKeyNone
	if rawKey != "" {
		source = smtpKeyEnv
	}
	if credentials == nil {
		return source, nil
	}
	derived, err := credentials.DeriveKey(smtpKeyPurpose)
	if err != nil {
		return source, err
	}
	if err := settingsService.SetDerivedSMTPKey(derived); err != nil {
		return source, err
	}
	if source == smtpKeyNone {
		source = smtpKeyDerived
	}
	return source, nil
}
