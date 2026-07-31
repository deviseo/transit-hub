package system

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"transithub/backend/internal/config"
)

var ErrUpgradeInProgress = errors.New("系统升级正在执行")

type UpgradeExecutor interface {
	Start(ctx context.Context) error
	Status(ctx context.Context) (UpgradeStatusResponse, error)
}

// Service 提供系统版本信息查询能力。
// 开源版不再包含商业授权校验和自动更新逻辑。
type Service struct {
	cfg             config.Config
	upgradeExecutor UpgradeExecutor
	upgradeMu       sync.Mutex
	lastRequestedAt time.Time
}

// NewService 创建系统服务
func NewService(cfg config.Config) *Service {
	return newServiceWithUpgradeExecutor(cfg, newSystemdUpgradeExecutor())
}

func newServiceWithUpgradeExecutor(cfg config.Config, executor UpgradeExecutor) *Service {
	return &Service{cfg: cfg, upgradeExecutor: executor}
}

// StartUpgrade 启动固定的 systemd 升级单元。
func (s *Service) StartUpgrade(ctx context.Context) (UpgradeStartResponse, error) {
	s.upgradeMu.Lock()
	defer s.upgradeMu.Unlock()

	status, err := s.upgradeExecutor.Status(ctx)
	if err != nil {
		return UpgradeStartResponse{}, fmt.Errorf("读取升级状态失败：%w", err)
	}
	now := time.Now().UTC()
	if status.State == UpgradeStateStarting || status.State == UpgradeStateRunning || s.startRequestPending(status, now) {
		return UpgradeStartResponse{}, ErrUpgradeInProgress
	}
	if err := s.upgradeExecutor.Start(ctx); err != nil {
		return UpgradeStartResponse{}, fmt.Errorf("启动系统升级失败：%w", err)
	}

	s.lastRequestedAt = now
	return UpgradeStartResponse{
		State:       UpgradeStateStarting,
		RequestedAt: now.Format(time.RFC3339Nano),
	}, nil
}

func (s *Service) startRequestPending(status UpgradeStatusResponse, now time.Time) bool {
	if s.lastRequestedAt.IsZero() {
		return false
	}
	if startedAt, err := time.Parse(time.RFC3339Nano, status.StartedAt); err == nil && !startedAt.Before(s.lastRequestedAt) {
		s.lastRequestedAt = time.Time{}
		return false
	}
	if now.Sub(s.lastRequestedAt) < 30*time.Second {
		return true
	}
	s.lastRequestedAt = time.Time{}
	return false
}

// UpgradeStatus 读取独立执行器写入的固定状态。
func (s *Service) UpgradeStatus(ctx context.Context) (UpgradeStatusResponse, error) {
	status, err := s.upgradeExecutor.Status(ctx)
	if err != nil {
		return UpgradeStatusResponse{}, fmt.Errorf("读取升级状态失败：%w", err)
	}
	return status, nil
}

// Version 返回当前系统版本信息
func (s *Service) Version() VersionResponse {
	return VersionResponse{
		Version: s.cfg.AppVersion,
	}
}
