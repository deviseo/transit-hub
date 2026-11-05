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

// Recommend 是各工具的推荐设置。JSON 键和 TransitLink 里工具的 ID 一致；旧版 TransitLink 不认识的键会忽略。
type Recommend struct {
	ClaudeCode      ToolRecommend `json:"claudeCode"`
	Codex           ToolRecommend `json:"codex"`
	GeminiCli       ToolRecommend `json:"geminiCli"`
	GrokBuild       ToolRecommend `json:"grokBuild"`
	OpenCode        ToolRecommend `json:"opencode"`
	DeepseekHarness ToolRecommend `json:"deepseekHarness"`
}

// fields 按固定顺序列出每个工具的推荐设置（存库、校验都按这个顺序）。
func (r *Recommend) fields() []*ToolRecommend {
	return []*ToolRecommend{&r.ClaudeCode, &r.Codex, &r.GeminiCli, &r.GrokBuild, &r.OpenCode, &r.DeepseekHarness}
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
	// MinClientVersion：TransitLink 的最低版本（例如 0.2.0），低于它的客户端提示更新；空表示不限制。
	MinClientVersion string
	// 工单：开启后 TransitLink 里出现「工单」；TicketCategories 是用户能选的问题分类。
	TicketsEnabled   bool
	TicketCategories []string
	UpdatedAt        *time.Time
}

// TicketSettings 是「客户端」页面里的工单设置。
type TicketSettings struct {
	Enabled    bool     `json:"enabled"`
	Categories []string `json:"categories"`
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
	// MinClientVersion 例如 "0.2.0" 或 "v0.2.0"；空表示不限制。
	MinClientVersion string         `json:"minClientVersion"`
	Tickets          TicketSettings `json:"tickets"`
}

// AdminView 是「客户端」页面看到的配置，带上当前站点的信息。
type AdminView struct {
	Enabled          bool           `json:"enabled"`
	Code             string         `json:"code"`
	ConfigPath       string         `json:"configPath"`
	HubURL           string         `json:"hubUrl"`
	SiteURL          string         `json:"siteUrl"`
	Platform         string         `json:"platform"`
	Brand            Brand          `json:"brand"`
	Recommend        Recommend      `json:"recommend"`
	Notice           string         `json:"notice"`
	NoticeUpdatedAt  *time.Time     `json:"noticeUpdatedAt"`
	Support          []SupportLink  `json:"support"`
	MinClientVersion string         `json:"minClientVersion"`
	Tickets          TicketSettings `json:"tickets"`
	UpdatedAt        *time.Time     `json:"updatedAt"`
	// LinkSameDomain：TransitHub 对外地址和用户访问的站点地址在同一个域名下。不在的话，用户第一次打开
	// 专属链接时 TransitLink 会提醒他确认链接是站长发的。还没填对外地址时为 true（不用提示）。
	LinkSameDomain bool `json:"linkSameDomain"`
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
	Client    PublicClient  `json:"client"`
	Tickets   PublicTickets `json:"tickets"`
}

// PublicTickets 告诉客户端有没有工单（分类在换取工单会话时返回）。
type PublicTickets struct {
	Enabled bool `json:"enabled"`
}

// PublicClient 是对客户端本身的要求。
type PublicClient struct {
	// MinVersion：低于这个版本的 TransitLink 提示用户更新；空表示不限制。
	MinVersion string `json:"minVersion"`
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
