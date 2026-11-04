package client_config

import "time"

// 错误 key 直接作为 i18n key 返回给前端。
const (
	ErrorNoCurrentAccount = "admin.adminAccounts.errors.noCurrentAccount"
	ErrorInvalid          = "admin.client.errors.invalid"
	ErrorLogoTooLarge     = "admin.client.errors.logoTooLarge"
	ErrorLogoType         = "admin.client.errors.logoType"
	ErrorRequest          = "admin.client.errors.request"
	ErrorNotFound         = "client.errors.notFound"
)

const (
	// 上传的 logo 存成 data URL，解码后最大 256 KB。
	maxLogoBytes    = 256 * 1024
	maxSupportLinks = 8
	codeLength      = 10
	// PublicPathPrefix 是 TransitLink 读取配置的公开接口前缀（不需要 TransitHub 登录）。
	PublicPathPrefix = "/api/client/v1/sites/"
)

// 允许的客服方式（客户端按类型显示图标）。
var supportTypes = map[string]bool{
	"telegram": true, "qq": true, "wechat": true, "discord": true, "email": true, "url": true, "other": true,
}

// 允许上传的 logo 格式。不收 SVG：它能带脚本，直接打开时会在 TransitHub 的域名下运行。
var logoTypes = map[string]bool{
	"image/png": true, "image/jpeg": true, "image/webp": true, "image/gif": true,
}

// Workspace 是数据隔离单元（TransitHub 用户 + 当前 admin 工作区）。
type Workspace struct {
	UserID         string
	AdminAccountID string
}

type Brand struct {
	Name    string `json:"name"`
	Logo    string `json:"logo"`
	Color   string `json:"color"`
	Welcome string `json:"welcome"`
}

// ToolRecommend 是给某个工具推荐的分组（new-api 是分组名，sub2api 是分组 ID）和模型（空表示不指定）。
type ToolRecommend struct {
	Group string `json:"group"`
	Model string `json:"model"`
}

type Recommend struct {
	ClaudeCode ToolRecommend `json:"claudeCode"`
	Codex      ToolRecommend `json:"codex"`
}

type SupportLink struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// Config 是数据库里的一行。
type Config struct {
	Workspace
	Code            string
	Enabled         bool
	HubURL          string
	SiteURL         string
	Brand           Brand
	Recommend       Recommend
	Notice          string
	NoticeUpdatedAt *time.Time
	Support         []SupportLink
	UpdatedAt       *time.Time
}

// UpdateRequest 是保存配置的请求。
type UpdateRequest struct {
	Enabled   bool          `json:"enabled"`
	HubURL    string        `json:"hubUrl"`
	SiteURL   string        `json:"siteUrl"`
	Brand     Brand         `json:"brand"`
	Recommend Recommend     `json:"recommend"`
	Notice    string        `json:"notice"`
	Support   []SupportLink `json:"support"`
}

// AdminView 是「客户端」页面看到的配置，带上当前站点的信息。
type AdminView struct {
	Enabled         bool          `json:"enabled"`
	Code            string        `json:"code"`
	ConfigPath      string        `json:"configPath"`
	HubURL          string        `json:"hubUrl"`
	SiteURL         string        `json:"siteUrl"`
	DefaultSiteURL  string        `json:"defaultSiteUrl"`
	SiteName        string        `json:"siteName"`
	Platform        string        `json:"platform"`
	Brand           Brand         `json:"brand"`
	Recommend       Recommend     `json:"recommend"`
	Notice          string        `json:"notice"`
	NoticeUpdatedAt *time.Time    `json:"noticeUpdatedAt"`
	Support         []SupportLink `json:"support"`
	UpdatedAt       *time.Time    `json:"updatedAt"`
}

// Group 是推荐分组下拉框里的一项（站点自己的用户分组）。
type Group struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Platform   *string  `json:"platform"`
	Multiplier *float64 `json:"multiplier"`
}

// PublicConfig 是 TransitLink 读到的配置（公开接口）。
type PublicConfig struct {
	Version   int           `json:"version"`
	Site      PublicSite    `json:"site"`
	Brand     PublicBrand   `json:"brand"`
	Recommend Recommend     `json:"recommend"`
	Notice    PublicNotice  `json:"notice"`
	Support   []SupportLink `json:"support"`
}

type PublicSite struct {
	Name     string `json:"name"`
	BaseURL  string `json:"baseUrl"`
	Platform string `json:"platform"`
}

type PublicBrand struct {
	Name    string `json:"name"`
	LogoURL string `json:"logoUrl"`
	Color   string `json:"color"`
	Welcome string `json:"welcome"`
}

type PublicNotice struct {
	Content   string     `json:"content"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

type requestError string

func (e requestError) Error() string { return string(e) }

// IsRequestError 判断是不是可以直接返回给前端的请求错误（i18n key）。
func IsRequestError(err error) bool {
	_, ok := err.(requestError)
	return ok
}
