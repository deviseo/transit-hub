package system

import "time"

// VersionResponse GET /api/system/version 响应。
// 开源版只展示版本号，不再包含授权状态或更新开关字段。
type VersionResponse struct {
	Version string `json:"version"`
}

// 出站代理相关错误 key（前端 i18n）。
const (
	ErrorProxyForbidden     = "admin.settings.proxy.errors.forbidden"
	ErrorProxyInvalidURL    = "admin.settings.proxy.errors.invalidUrl"
	ErrorProxyInvalidTarget = "admin.settings.proxy.errors.invalidTarget"
	ErrorProxyRequest       = "admin.settings.proxy.errors.request"
	ErrorProxyUnavailable   = "admin.settings.proxy.errors.unavailable"
)

// OutboundProxyResponse 是 GET/PUT /api/system/outbound-proxy 的响应。
// ProxyURL 中的密码已替换为占位符；Configured=false 表示从未保存过，此时沿用启动环境变量，
// Enabled/ProxyURL/Bypass 用环境变量预填，方便直接在此基础上修改保存。
type OutboundProxyResponse struct {
	Configured  bool             `json:"configured"`
	Enabled     bool             `json:"enabled"`
	ProxyURL    string           `json:"proxyUrl"`
	Bypass      string           `json:"bypass"`
	Mode        string           `json:"mode"`
	CanEdit     bool             `json:"canEdit"`
	Environment EnvironmentProxy `json:"environment"`
	UpdatedAt   *time.Time       `json:"updatedAt,omitempty"`
}

// EnvironmentProxy 是进程启动环境变量中的代理（已脱敏），仅供展示。
type EnvironmentProxy struct {
	ProxyURL string `json:"proxyUrl"`
	Bypass   string `json:"bypass"`
}

// SaveOutboundProxyRequest 是 PUT /api/system/outbound-proxy 的请求体。
// ProxyURL 里保留密码占位符时沿用已保存的密码。
type SaveOutboundProxyRequest struct {
	Enabled  bool   `json:"enabled"`
	ProxyURL string `json:"proxyUrl"`
	Bypass   string `json:"bypass"`
}

// TestOutboundProxyRequest 是 POST /api/system/outbound-proxy/test 的请求体：用表单里尚未保存的配置
// 访问 TargetURL，验证能否连通。
type TestOutboundProxyRequest struct {
	Enabled   bool   `json:"enabled"`
	ProxyURL  string `json:"proxyUrl"`
	Bypass    string `json:"bypass"`
	TargetURL string `json:"targetUrl"`
}

// TestOutboundProxyResponse 是连接测试结果。只要拿到 HTTP 响应（任意状态码）即视为连通。
// Failure 取值见 outbound.Failure*，用于前端展示失败原因。
type TestOutboundProxyResponse struct {
	OK         bool   `json:"ok"`
	Route      string `json:"route"`
	StatusCode int    `json:"statusCode,omitempty"`
	DurationMs int64  `json:"durationMs"`
	Failure    string `json:"failure,omitempty"`
}

// requestError 让 service 返回可直接作为 i18n key 的业务错误，由 handler 映射成 HTTP 状态码。
type requestError string

func (e requestError) Error() string { return string(e) }
