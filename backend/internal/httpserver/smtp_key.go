package httpserver

import "transithub/backend/internal/modules/settings"

// configureSMTPEncryptionKey 是 server.New 实际调用的窄组装 seam：把 SMTP_ENCRYPTION_KEY 的
// 原始环境变量值安装到 settings service。返回值只暴露“是否显式配置并成功安装”这个布尔状态，
// 不暴露 key 材料，也不要求 settings.Service 提供包外可见的加密能力查询方法。
func configureSMTPEncryptionKey(settingsService *settings.Service, rawKey string) (bool, error) {
	if err := settingsService.SetSMTPEncryptionKey(rawKey); err != nil {
		return false, err
	}
	return rawKey != "", nil
}
