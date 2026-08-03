package connection_health

import (
	"context"
	"sync"
)

type probeConcurrencyLimiter struct {
	global          chan struct{}
	perWorkspaceCap int
	mu              sync.Mutex
	workspaces      map[string]chan struct{}
}

func newProbeConcurrencyLimiter(globalLimit int, perWorkspaceLimit int) *probeConcurrencyLimiter {
	return &probeConcurrencyLimiter{
		global: make(chan struct{}, globalLimit), perWorkspaceCap: perWorkspaceLimit,
		workspaces: make(map[string]chan struct{}),
	}
}

func (l *probeConcurrencyLimiter) acquire(ctx context.Context, workspaceKey string, wait bool) (func(), bool) {
	if !acquireProbeSlot(ctx, l.global, wait) {
		return nil, false
	}
	l.mu.Lock()
	workspace := l.workspaces[workspaceKey]
	if workspace == nil {
		workspace = make(chan struct{}, l.perWorkspaceCap)
		l.workspaces[workspaceKey] = workspace
	}
	l.mu.Unlock()
	if !acquireProbeSlot(ctx, workspace, wait) {
		<-l.global
		return nil, false
	}
	return func() {
		<-workspace
		<-l.global
	}, true
}

func acquireProbeSlot(ctx context.Context, slot chan struct{}, wait bool) bool {
	if !wait {
		select {
		case slot <- struct{}{}:
			return true
		default:
			return false
		}
	}
	select {
	case slot <- struct{}{}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (s *Service) sharedProbeLimiter() *probeConcurrencyLimiter {
	s.probeLimiterMu.Lock()
	defer s.probeLimiterMu.Unlock()
	if s.probeLimiter == nil {
		s.probeLimiter = newProbeConcurrencyLimiter(globalProbeConcurrency, perSiteProbeConcurrency)
	}
	return s.probeLimiter
}
