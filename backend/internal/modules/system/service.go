package system

import (
	"transithub/backend/internal/config"
	"transithub/backend/internal/shared/outbound"
)

// Service 提供系统版本信息、版本检查（只读，对比 GitHub 上最新的正式版）与系统级设置（目前是出站代理）。
type Service struct {
	cfg config.Config

	updates updateChecker
	// credentialsEncrypted：保存的登录凭据是否加密（检查更新时一并提示部署问题）。
	credentialsEncrypted bool

	proxyStore      OutboundProxyStore
	proxyController *outbound.Controller
	admins          SystemAdminChecker
}

// NewService 创建系统服务
func NewService(cfg config.Config) *Service {
	return &Service{cfg: cfg}
}

// Version 返回当前系统版本信息
func (s *Service) Version() VersionResponse {
	return VersionResponse{
		Version: s.cfg.AppVersion,
	}
}

// SetCredentialsEncrypted 记录保存的登录凭据是否加密（启动时取得密钥后设置）。
func (s *Service) SetCredentialsEncrypted(encrypted bool) {
	s.credentialsEncrypted = encrypted
}
