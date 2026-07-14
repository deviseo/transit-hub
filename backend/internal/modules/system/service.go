package system

import (
	"transithub/backend/internal/config"
)

// Service 提供系统版本信息查询能力。
// 开源版不再包含商业授权校验和自动更新逻辑。
type Service struct {
	cfg config.Config
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
