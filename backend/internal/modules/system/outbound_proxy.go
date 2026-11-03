package system

import (
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/outbound"
)

// outboundProxyTestTimeout 是连接测试的超时时间。
const outboundProxyTestTimeout = 15 * time.Second

// OutboundProxyStore 是出站代理配置的持久化接口，由 *Repository 实现。
type OutboundProxyStore interface {
	GetOutboundProxy(ctx context.Context) (StoredOutboundProxy, bool, error)
	SaveOutboundProxy(ctx context.Context, settings outbound.Settings, updatedBy string) (time.Time, error)
}

// SystemAdminChecker 判断用户能否修改系统级设置（见 auth.Service.IsSystemAdmin）。
type SystemAdminChecker interface {
	IsSystemAdmin(ctx context.Context, userID string) (bool, error)
}

// SetOutboundProxy 注入出站代理所需的存储、生效控制器与系统管理员判断。
func (s *Service) SetOutboundProxy(store OutboundProxyStore, controller *outbound.Controller, admins SystemAdminChecker) {
	s.proxyStore = store
	s.proxyController = controller
	s.admins = admins
}

func (s *Service) outboundProxyReady() bool {
	return s.proxyStore != nil && s.proxyController != nil && s.admins != nil
}

// LoadOutboundProxy 在启动时应用已保存的出站代理配置；从未保存过时保持「沿用环境变量」。
func (s *Service) LoadOutboundProxy(ctx context.Context) error {
	if !s.outboundProxyReady() {
		return nil
	}
	stored, configured, err := s.proxyStore.GetOutboundProxy(ctx)
	if err != nil || !configured {
		return err
	}
	if err := s.proxyController.Apply(stored.Settings); err != nil {
		return err
	}
	logOutboundProxy("loaded", "", stored.Settings, s.proxyController.Mode())
	return nil
}

// GetOutboundProxy 返回当前出站代理配置（密码已脱敏）。所有登录用户可查看，只有系统管理员可修改。
func (s *Service) GetOutboundProxy(ctx context.Context, userID string) (OutboundProxyResponse, error) {
	if !s.outboundProxyReady() {
		return OutboundProxyResponse{}, requestError(ErrorProxyUnavailable)
	}
	canEdit, err := s.admins.IsSystemAdmin(ctx, userID)
	if err != nil {
		return OutboundProxyResponse{}, err
	}
	stored, configured, err := s.proxyStore.GetOutboundProxy(ctx)
	if err != nil {
		return OutboundProxyResponse{}, err
	}
	environment := outbound.EnvironmentSettings()
	response := OutboundProxyResponse{
		Configured: configured,
		Mode:       s.proxyController.Mode(),
		CanEdit:    canEdit,
		Environment: EnvironmentProxy{
			ProxyURL: outbound.MaskProxyURL(environment.ProxyURL),
			Bypass:   environment.Bypass,
		},
	}
	current := environment
	if configured {
		current = stored.Settings
		updatedAt := stored.UpdatedAt
		response.UpdatedAt = &updatedAt
	}
	response.Enabled = current.Enabled
	response.ProxyURL = outbound.MaskProxyURL(current.ProxyURL)
	response.Bypass = current.Bypass
	return response, nil
}

// SaveOutboundProxy 保存并立即应用出站代理配置：先落库再切换内存中的生效配置，落库失败时不改变当前行为。
func (s *Service) SaveOutboundProxy(ctx context.Context, userID string, req SaveOutboundProxyRequest) (OutboundProxyResponse, error) {
	if !s.outboundProxyReady() {
		return OutboundProxyResponse{}, requestError(ErrorProxyUnavailable)
	}
	if err := s.requireSystemAdmin(ctx, userID); err != nil {
		return OutboundProxyResponse{}, err
	}
	settings, err := s.proposedSettings(ctx, req.Enabled, req.ProxyURL, req.Bypass)
	if err != nil {
		return OutboundProxyResponse{}, err
	}
	if _, err := s.proxyStore.SaveOutboundProxy(ctx, settings, userID); err != nil {
		return OutboundProxyResponse{}, err
	}
	if err := s.proxyController.Apply(settings); err != nil {
		return OutboundProxyResponse{}, requestError(ErrorProxyInvalidURL)
	}
	logOutboundProxy("updated", userID, settings, s.proxyController.Mode())
	return s.GetOutboundProxy(ctx, userID)
}

// TestOutboundProxy 用表单里尚未保存的配置访问目标地址，验证能否连通，不影响当前生效配置。
// 只允许系统管理员调用，避免普通用户借此让服务端访问任意地址。
func (s *Service) TestOutboundProxy(ctx context.Context, userID string, req TestOutboundProxyRequest) (TestOutboundProxyResponse, error) {
	if !s.outboundProxyReady() {
		return TestOutboundProxyResponse{}, requestError(ErrorProxyUnavailable)
	}
	if err := s.requireSystemAdmin(ctx, userID); err != nil {
		return TestOutboundProxyResponse{}, err
	}
	target, err := parseTestTarget(req.TargetURL)
	if err != nil {
		return TestOutboundProxyResponse{}, err
	}
	settings, err := s.proposedSettings(ctx, req.Enabled, req.ProxyURL, req.Bypass)
	if err != nil {
		return TestOutboundProxyResponse{}, err
	}

	controller := outbound.NewController()
	if err := controller.Apply(settings); err != nil {
		return TestOutboundProxyResponse{}, requestError(ErrorProxyInvalidURL)
	}
	route, err := controller.DescribeRoute(target)
	if err != nil {
		return TestOutboundProxyResponse{}, requestError(ErrorProxyInvalidURL)
	}
	transport := controller.NewTransport()
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout:   outboundProxyTestTimeout,
		Transport: transport,
		// 只验证连通性：重定向本身已经说明连上了目标站点，不再跟随到其他地址。
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return TestOutboundProxyResponse{}, requestError(ErrorProxyInvalidTarget)
	}
	request.Header.Set("Accept", "*/*")
	request.Header.Set("User-Agent", upstream.BrowserUserAgent)

	startedAt := time.Now()
	response, err := client.Do(request)
	duration := time.Since(startedAt).Milliseconds()
	if err != nil {
		failure := outbound.ClassifyError(err)
		if route == outbound.ModeProxy && failure == outbound.FailureNetwork {
			// 经代理时连接被中途断开（例如 TLS 握手阶段被关闭），多半是代理规则或节点连不上目标站点。
			failure = outbound.FailureProxy
		}
		log.Printf("[outbound-proxy] test failed host=%s route=%s failure=%s err=%v", target.Host, route, failure, err)
		return TestOutboundProxyResponse{OK: false, Route: route, DurationMs: duration, Failure: failure}, nil
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	return TestOutboundProxyResponse{OK: true, Route: route, StatusCode: response.StatusCode, DurationMs: duration}, nil
}

func (s *Service) requireSystemAdmin(ctx context.Context, userID string) error {
	allowed, err := s.admins.IsSystemAdmin(ctx, userID)
	if err != nil {
		return err
	}
	if !allowed {
		return requestError(ErrorProxyForbidden)
	}
	return nil
}

// proposedSettings 规范化表单提交的配置；代理地址里保留密码占位符时，换回已保存（或环境变量中）的真实密码。
func (s *Service) proposedSettings(ctx context.Context, enabled bool, proxyURL string, bypass string) (outbound.Settings, error) {
	previous := outbound.EnvironmentSettings().ProxyURL
	if stored, configured, err := s.proxyStore.GetOutboundProxy(ctx); err != nil {
		return outbound.Settings{}, err
	} else if configured {
		previous = stored.Settings.ProxyURL
	}
	settings, err := outbound.Normalize(outbound.Settings{
		Enabled:  enabled,
		ProxyURL: outbound.RestoreMaskedPassword(proxyURL, previous),
		Bypass:   bypass,
	})
	if err != nil {
		return outbound.Settings{}, requestError(ErrorProxyInvalidURL)
	}
	return settings, nil
}

func parseTestTarget(raw string) (*url.URL, error) {
	raw = strings.TrimSpace(raw)
	if raw != "" && !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	target, err := url.Parse(raw)
	if err != nil || (target.Scheme != "http" && target.Scheme != "https") || target.Hostname() == "" || target.User != nil {
		return nil, requestError(ErrorProxyInvalidTarget)
	}
	return target, nil
}

func logOutboundProxy(action string, userID string, settings outbound.Settings, mode string) {
	log.Printf("[outbound-proxy] %s user_id=%s mode=%s proxy=%s bypass=%q", action, userID, mode, outbound.MaskProxyURL(settings.ProxyURL), settings.Bypass)
}
