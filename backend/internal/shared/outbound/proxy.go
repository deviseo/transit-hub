// Package outbound 管理 TransitHub 访问上游站点时使用的出站代理。
//
// 代理配置保存在系统设置中，可随时开启、关闭或更换，修改后立即对新请求生效，无需重启：
//   - 从未保存过配置时，沿用进程启动时的环境变量（HTTPS_PROXY / HTTP_PROXY / NO_PROXY），保持旧部署行为不变；
//   - 保存后由配置决定：关闭即直连，开启则走指定代理，bypass 列表中的域名不走代理。
//
// localhost 与回环地址永远直连（与 Go 标准库 NO_PROXY 语义一致）。
package outbound

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/net/http/httpproxy"
)

// 生效模式，供界面展示当前请求实际怎么出站。
const (
	ModeEnvironment = "environment" // 未保存配置：沿用启动环境变量
	ModeDirect      = "direct"      // 已关闭代理：全部直连
	ModeProxy       = "proxy"       // 已开启代理
)

// MaskedPassword 是代理密码在接口响应中的占位符。
const MaskedPassword = "***"

// ErrInvalidProxyURL 表示代理地址无法解析或协议不受支持。
var ErrInvalidProxyURL = errors.New("invalid proxy url")

// Settings 是保存在系统设置中的出站代理配置。
type Settings struct {
	Enabled  bool   `json:"enabled"`
	ProxyURL string `json:"proxyUrl"`
	// Bypass 是不走代理的地址列表，逗号分隔，语义同 NO_PROXY：
	// example.com 同时匹配其子域名，.example.com / *.example.com 只匹配子域名，也支持 IP 与 CIDR。
	Bypass string `json:"bypass"`
}

// Normalize 校验并规范化配置：未写协议的地址按 http 处理；开启代理时地址必填。
func Normalize(settings Settings) (Settings, error) {
	settings.ProxyURL = strings.TrimSpace(settings.ProxyURL)
	settings.Bypass = NormalizeBypass(settings.Bypass)
	if settings.ProxyURL != "" {
		normalized, err := normalizeProxyURL(settings.ProxyURL)
		if err != nil {
			return Settings{}, err
		}
		settings.ProxyURL = normalized
	}
	if settings.Enabled && settings.ProxyURL == "" {
		return Settings{}, ErrInvalidProxyURL
	}
	return settings, nil
}

func normalizeProxyURL(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.Hostname() == "" {
		return "", ErrInvalidProxyURL
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https", "socks5", "socks5h":
	default:
		return "", ErrInvalidProxyURL
	}
	if parsed.Path != "" && parsed.Path != "/" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", ErrInvalidProxyURL
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	parsed.Path = ""
	return parsed.String(), nil
}

// NormalizeBypass 把逗号、空白或换行分隔的列表整理成逗号分隔、去重后的形式。
func NormalizeBypass(raw string) string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\r' || r == '\t'
	})
	seen := make(map[string]struct{}, len(fields))
	entries := make([]string, 0, len(fields))
	for _, field := range fields {
		entry := strings.ToLower(strings.TrimSpace(field))
		if entry == "" {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		entries = append(entries, entry)
	}
	return strings.Join(entries, ",")
}

// MaskProxyURL 把代理地址中的密码替换为占位符，用于接口响应和日志，例如 http://user:***@host:port。
// 占位符直接拼接而不经 URL 编码，避免界面上显示成 %2A%2A%2A。
func MaskProxyURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User == nil {
		return strings.TrimSpace(raw)
	}
	if _, hasPassword := parsed.User.Password(); !hasPassword {
		return parsed.String()
	}
	username := url.User(parsed.User.Username()).String()
	parsed.User = url.User(parsed.User.Username())
	return strings.Replace(parsed.String(), "//"+username+"@", "//"+username+":"+MaskedPassword+"@", 1)
}

// HasPassword 判断代理地址是否带密码。
func HasPassword(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.User == nil {
		return false
	}
	_, hasPassword := parsed.User.Password()
	return hasPassword
}

// RestoreMaskedPassword 在用户提交的地址仍带着占位密码时，换回已保存地址中的真实密码。
// 只有协议、主机、用户名都一致时才替换，避免把旧密码误用到新的代理上。
func RestoreMaskedPassword(submitted string, stored string) string {
	submittedURL, err := url.Parse(strings.TrimSpace(submitted))
	if err != nil || submittedURL.User == nil {
		return submitted
	}
	password, hasPassword := submittedURL.User.Password()
	if !hasPassword || password != MaskedPassword {
		return submitted
	}
	storedURL, err := url.Parse(strings.TrimSpace(stored))
	if err != nil || storedURL.User == nil {
		return submitted
	}
	storedPassword, storedHasPassword := storedURL.User.Password()
	if !storedHasPassword || !strings.EqualFold(storedURL.Scheme, submittedURL.Scheme) ||
		!strings.EqualFold(storedURL.Host, submittedURL.Host) || storedURL.User.Username() != submittedURL.User.Username() {
		return submitted
	}
	submittedURL.User = url.UserPassword(submittedURL.User.Username(), storedPassword)
	return submittedURL.String()
}

// EnvironmentSettings 返回进程环境变量中的代理配置（HTTPS_PROXY 优先，其次 HTTP_PROXY / ALL_PROXY）。
// 仅用于在未保存配置时展示和预填表单。
func EnvironmentSettings() Settings {
	proxyURL := firstEnv("HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy")
	return Settings{
		Enabled:  proxyURL != "",
		ProxyURL: proxyURL,
		Bypass:   NormalizeBypass(firstEnv("NO_PROXY", "no_proxy")),
	}
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}

// state 是某一时刻生效的代理配置，整体替换以保证并发读取一致。
type state struct {
	mode     string
	settings Settings
	proxy    func(*url.URL) (*url.URL, error)
}

// Controller 持有当前生效的出站代理配置，并为共享的 http.Transport 提供 Proxy 函数。
// 配置更新后立即对新请求生效，并关闭空闲连接，避免复用按旧配置建立的连接。
type Controller struct {
	current    atomic.Pointer[state]
	mu         sync.Mutex
	transports []*http.Transport
	// environmentProxy 是未保存配置时使用的代理函数，默认读取进程环境变量；测试可替换。
	environmentProxy func(*http.Request) (*url.URL, error)
}

// NewController 创建控制器，初始为「沿用环境变量」模式。
func NewController() *Controller {
	controller := &Controller{environmentProxy: http.ProxyFromEnvironment}
	controller.current.Store(&state{mode: ModeEnvironment})
	return controller
}

// Apply 应用已保存的配置（会先校验并规范化）。
func (c *Controller) Apply(settings Settings) error {
	normalized, err := Normalize(settings)
	if err != nil {
		return err
	}
	next := &state{mode: ModeDirect, settings: normalized}
	if normalized.Enabled {
		next.mode = ModeProxy
		next.proxy = (&httpproxy.Config{
			HTTPProxy:  normalized.ProxyURL,
			HTTPSProxy: normalized.ProxyURL,
			NoProxy:    normalized.Bypass,
		}).ProxyFunc()
	}
	c.current.Store(next)
	c.closeIdleConnections()
	return nil
}

// Mode 返回当前生效模式。
func (c *Controller) Mode() string {
	return c.current.Load().mode
}

// Proxy 实现 http.Transport.Proxy：按当前配置返回本次请求应使用的代理，nil 表示直连。
func (c *Controller) Proxy(req *http.Request) (*url.URL, error) {
	current := c.current.Load()
	switch current.mode {
	case ModeEnvironment:
		return c.environmentProxy(req)
	case ModeProxy:
		return current.proxy(req.URL)
	default:
		return nil, nil
	}
}

// NewTransport 返回一个使用本控制器代理配置的 Transport，配置变化时其空闲连接会被关闭。
func (c *Controller) NewTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = c.Proxy
	c.mu.Lock()
	c.transports = append(c.transports, transport)
	c.mu.Unlock()
	return transport
}

// NewClient 返回使用本控制器代理配置的 http.Client。
func (c *Controller) NewClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: c.NewTransport()}
}

func (c *Controller) closeIdleConnections() {
	c.mu.Lock()
	transports := append([]*http.Transport(nil), c.transports...)
	c.mu.Unlock()
	for _, transport := range transports {
		transport.CloseIdleConnections()
	}
}

// 连接测试失败原因分类，取值同时是前端 i18n key 的后缀。
const (
	FailureDNS     = "dns"
	FailureProxy   = "proxy"
	FailureTLS     = "tls"
	FailureTimeout = "timeout"
	FailureRefused = "refused"
	FailureNetwork = "network"
)

// ClassifyError 把出站请求的错误归类为用户能理解的原因，不暴露原始报文。
func ClassifyError(err error) string {
	if err == nil {
		return ""
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Op == "proxyconnect" {
		return FailureProxy
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return FailureTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return FailureTimeout
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return FailureDNS
	}
	var recordErr tls.RecordHeaderError
	var certErr *tls.CertificateVerificationError
	var unknownAuthority x509.UnknownAuthorityError
	var hostnameErr x509.HostnameError
	if errors.As(err, &recordErr) || errors.As(err, &certErr) || errors.As(err, &unknownAuthority) || errors.As(err, &hostnameErr) {
		return FailureTLS
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return FailureRefused
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "proxyconnect"):
		return FailureProxy
	case strings.Contains(message, "tls:") || strings.Contains(message, "x509:") || strings.Contains(message, "TLS handshake"):
		return FailureTLS
	}
	return FailureNetwork
}

// DescribeRoute 返回某个目标地址在给定配置下的出站方式，用于测试结果展示："proxy" 或 "direct"。
func (c *Controller) DescribeRoute(target *url.URL) (string, error) {
	proxyURL, err := c.Proxy(&http.Request{URL: target, Header: http.Header{}})
	if err != nil {
		return "", fmt.Errorf("resolve proxy: %w", err)
	}
	if proxyURL == nil {
		return ModeDirect, nil
	}
	return ModeProxy, nil
}
