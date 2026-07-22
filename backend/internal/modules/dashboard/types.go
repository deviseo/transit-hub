package dashboard

import "transithub/backend/internal/modules/upstream"

// 仪表盘 admin 登录支持的平台。本轮先实现 sub2api，new-api 预留。
const (
	PlatformSub2API = "sub2api"
	PlatformNewAPI  = "newapi"
)

// 管理员登录方式：
//   - password: 管理员邮箱 + 密码登录，换取 access/refresh token。
//   - token: 兼容旧 Sub2API access/refresh token 登录。
//   - admin_key: Sub2API Admin API Key，或 new-api 管理员/Root 系统访问令牌。
const (
	AuthMethodPassword = "password"
	AuthMethodToken    = "token"
	AuthMethodAdminKey = "admin_key"
)

// 前端用于国际化的错误 key，统一挂在 admin.dashboard.adminAuth.errors 命名空间下。
const (
	ErrorRequest                     = "admin.dashboard.adminAuth.errors.request"
	ErrorMissingCredentials          = "admin.dashboard.adminAuth.errors.missingCredentials"
	ErrorInvalidURL                  = "admin.dashboard.adminAuth.errors.invalidUrl"
	ErrorAdminOnly                   = "admin.dashboard.adminAuth.errors.adminOnly"
	ErrorNetwork                     = "admin.dashboard.adminAuth.errors.network"
	ErrorPlatformUnsupported         = "admin.dashboard.adminAuth.errors.platformUnsupported"
	ErrorUnknown                     = "admin.dashboard.adminAuth.errors.unknown"
	ErrorUpstreamKeyUsageUnavailable = "admin.dashboard.upstreamKeyUsage.errors.unavailable"
)

// AdminSession 是持久化到 Redis 的仪表盘 admin 会话。
// 复用 upstream.Session 承载 token 细节，避免重复实现 sub2api 的令牌结构。
type AdminSession struct {
	Platform        string           `json:"platform"`
	BaseURL         string           `json:"baseUrl"`
	AuthMethod      string           `json:"authMethod"`
	Identity        string           `json:"identity"` // 展示用标识：邮箱或登录方式名
	Session         upstream.Session `json:"session"`
	CreatedAt       int64            `json:"createdAt"`       // 毫秒时间戳
	LastRefreshedAt int64            `json:"lastRefreshedAt"` // 主 RT 最近一次刷新的毫秒时间戳
}

// LoginRequest 是登录弹窗提交的请求体，覆盖三种 sub2api 登录方式所需字段。
type LoginRequest struct {
	Platform     string `json:"platform"`
	SiteURL      string `json:"siteUrl"`
	AuthMethod   string `json:"authMethod"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	AdminKey     string `json:"adminKey"`
	UserID       string `json:"userId"`
}

// StatusResponse 描述当前用户的仪表盘 admin 登录状态，供前端决定是否弹窗。
// ExpiresAt 是登录凭证（access token）的过期毫秒时间戳，临期会自动刷新；nil 表示未知。
type StatusResponse struct {
	Authenticated bool   `json:"authenticated"`
	Platform      string `json:"platform"`
	BaseURL       string `json:"baseUrl"`
	AuthMethod    string `json:"authMethod"`
	Identity      string `json:"identity"`
	ExpiresAt     *int64 `json:"expiresAt"`
}

// requestError 让 service 返回可直接作为 i18n key 的业务错误，由 handler 映射成 HTTP 状态码。
type requestError string

func (e requestError) Error() string { return string(e) }
