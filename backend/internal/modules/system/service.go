package system

import (
	"transithub/backend/internal/config"
	"transithub/backend/internal/shared/outbound"
)

// Service 提供系统版本信息与系统级设置（目前是出站代理）。
// 开源版不再包含商业授权校验和自动更新逻辑。
type Service struct {
	cfg config.Config

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
