package client_config

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// SiteInfo 是工作区对应的站点（站长自己的 new-api / sub2api）。
// 不带工作区名称：它默认是站点管理员的登录账号，不能出现在对外的配置里。
type SiteInfo struct {
	Platform string
	BaseURL  string
}

// Sites 提供当前工作区和站点信息（由 admin_accounts 实现）。
type Sites interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
	Site(ctx context.Context, userID string, adminAccountID string) (SiteInfo, error)
}

// GroupSource 读取站点自己的用户分组（用工作区的 admin 会话）。
type GroupSource interface {
	Groups(ctx context.Context, userID string, adminAccountID string) ([]Group, error)
}

// Store 是 Repository 的接口，方便测试替换。
type Store interface {
	Get(ctx context.Context, ws Workspace) (*Config, error)
	GetByCode(ctx context.Context, code string) (*Config, error)
	Save(ctx context.Context, config Config) error
}

type Service struct {
	store   Store
	sites   Sites
	groups  GroupSource
	now     func() time.Time
	newCode func() string
}

func NewService(store Store, sites Sites, groups GroupSource) *Service {
	return &Service{store: store, sites: sites, groups: groups, now: time.Now, newCode: randomCode}
}

// CurrentWorkspace 返回用户当前选中的工作区。
func (s *Service) CurrentWorkspace(ctx context.Context, userID string) (Workspace, error) {
	id, err := s.sites.RequireCurrentID(ctx, userID)
	if err != nil || strings.TrimSpace(id) == "" {
		return Workspace{}, requestError(ErrorNoCurrentAccount)
	}
	return Workspace{UserID: userID, AdminAccountID: id}, nil
}

// Get 返回「客户端」页面的配置；还没保存过时返回默认值（未开启、没有链接代码）。
func (s *Service) Get(ctx context.Context, ws Workspace) (AdminView, error) {
	config, err := s.store.Get(ctx, ws)
	if err != nil {
		return AdminView{}, err
	}
	if config == nil {
		config = &Config{Workspace: ws}
	}
	return s.view(ctx, *config)
}

// Update 校验并保存配置。第一次保存时生成专属链接的代码；公告内容变了时更新公告时间。
func (s *Service) Update(ctx context.Context, ws Workspace, req UpdateRequest) (AdminView, error) {
	next, err := normalize(req)
	if err != nil {
		return AdminView{}, err
	}
	current, err := s.store.Get(ctx, ws)
	if err != nil {
		return AdminView{}, err
	}
	next.Workspace = ws
	if current != nil {
		next.Code = current.Code
		next.NoticeUpdatedAt = current.NoticeUpdatedAt
	}
	if next.Code == "" {
		next.Code = s.newCode()
	}
	if current == nil || current.Notice != next.Notice {
		if next.Notice == "" {
			next.NoticeUpdatedAt = nil
		} else {
			now := s.now()
			next.NoticeUpdatedAt = &now
		}
	}
	if err := s.store.Save(ctx, next); err != nil {
		return AdminView{}, err
	}
	return s.Get(ctx, ws)
}

// ResetCode 换一个专属链接代码：旧链接立刻失效（已经接入的用户不受影响，下次读配置用新地址）。
func (s *Service) ResetCode(ctx context.Context, ws Workspace) (AdminView, error) {
	current, err := s.store.Get(ctx, ws)
	if err != nil {
		return AdminView{}, err
	}
	if current == nil {
		current = &Config{Workspace: ws}
	}
	current.Code = s.newCode()
	if err := s.store.Save(ctx, *current); err != nil {
		return AdminView{}, err
	}
	return s.Get(ctx, ws)
}

// Groups 返回站点自己的用户分组，给推荐分组的下拉框用。
func (s *Service) Groups(ctx context.Context, ws Workspace) ([]Group, error) {
	return s.groups.Groups(ctx, ws.UserID, ws.AdminAccountID)
}

func (s *Service) view(ctx context.Context, config Config) (AdminView, error) {
	site, err := s.sites.Site(ctx, config.UserID, config.AdminAccountID)
	if err != nil {
		return AdminView{}, err
	}
	view := AdminView{
		Enabled:          config.Enabled,
		Code:             config.Code,
		HubURL:           config.HubURL,
		SiteURL:          config.SiteURL,
		Platform:         site.Platform,
		Brand:            config.Brand,
		Recommend:        config.Recommend,
		Notice:           config.Notice,
		NoticeUpdatedAt:  config.NoticeUpdatedAt,
		Support:          config.Support,
		MinClientVersion: config.MinClientVersion,
		Tickets:          TicketSettings{Enabled: config.TicketsEnabled, Categories: categoriesOrDefault(config.TicketCategories)},
		UpdatedAt:        config.UpdatedAt,
	}
	siteURL := config.SiteURL
	if siteURL == "" {
		siteURL = site.BaseURL
	}
	view.LinkSameDomain = config.HubURL == "" || sameOwner(siteURL, config.HubURL)
	if view.Support == nil {
		view.Support = []SupportLink{}
	}
	if config.Code != "" {
		view.ConfigPath = PublicPathPrefix + config.Code
	}
	return view, nil
}

// Public 返回给 TransitLink 的配置。没开启、代码不对或站点已删除时都按不存在处理。
func (s *Service) Public(ctx context.Context, code string) (PublicConfig, error) {
	config, site, err := s.publicConfig(ctx, code)
	if err != nil {
		return PublicConfig{}, err
	}
	baseURL := config.SiteURL
	if baseURL == "" {
		baseURL = site.BaseURL
	}
	logo := config.Brand.Logo
	if strings.HasPrefix(logo, "data:") {
		// 上传的图片单独提供，配置里给相对地址（客户端按配置地址解析）。
		logo = PublicPathPrefix + config.Code + "/logo"
	}
	support := config.Support
	if support == nil {
		support = []SupportLink{}
	}
	// 站点名称只用对外地址的域名；没设品牌名称时留空，TransitLink 会显示站点自己的名称。
	return PublicConfig{
		Version:   1,
		Site:      PublicSite{Name: siteHost(baseURL), BaseURL: baseURL, Platform: site.Platform},
		Brand:     PublicBrand{Name: config.Brand.Name, LogoURL: logo, Color: config.Brand.Color, Welcome: config.Brand.Welcome},
		Recommend: config.Recommend,
		Notice:    PublicNotice{Content: config.Notice, UpdatedAt: config.NoticeUpdatedAt},
		Support:   support,
		Client:    PublicClient{MinVersion: config.MinClientVersion},
		Tickets:   PublicTickets{Enabled: config.TicketsEnabled},
	}, nil
}

// siteHost 返回站点地址的域名（带端口），例如 api.example.com。
func siteHost(baseURL string) string {
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Host != "" {
		return parsed.Host
	}
	return baseURL
}

// Logo 返回上传的 logo 图片（类型和内容）。
func (s *Service) Logo(ctx context.Context, code string) (string, []byte, error) {
	config, _, err := s.publicConfig(ctx, code)
	if err != nil {
		return "", nil, err
	}
	contentType, data, ok := decodeLogo(config.Brand.Logo)
	if !ok {
		return "", nil, requestError(ErrorNotFound)
	}
	return contentType, data, nil
}

func (s *Service) publicConfig(ctx context.Context, code string) (*Config, SiteInfo, error) {
	code = strings.TrimSpace(code)
	if !validCode(code) {
		return nil, SiteInfo{}, requestError(ErrorNotFound)
	}
	config, err := s.store.GetByCode(ctx, code)
	if err != nil {
		return nil, SiteInfo{}, err
	}
	if config == nil || !config.Enabled {
		return nil, SiteInfo{}, requestError(ErrorNotFound)
	}
	site, err := s.sites.Site(ctx, config.UserID, config.AdminAccountID)
	if err != nil {
		return nil, SiteInfo{}, requestError(ErrorNotFound)
	}
	return config, site, nil
}

var colorPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// normalize 去掉首尾空白、校验长度和格式，返回要保存的配置。
func normalize(req UpdateRequest) (Config, error) {
	invalid := requestError(ErrorInvalid)
	trimmed := func(value string, limit int) (string, bool) {
		value = strings.TrimSpace(value)
		return value, utf8.RuneCountInString(value) <= limit
	}
	config := Config{Enabled: req.Enabled}
	var ok bool
	var err error
	if config.HubURL, err = normalizeURL(req.HubURL, true); err != nil {
		return Config{}, invalid
	}
	if config.SiteURL, err = normalizeURL(req.SiteURL, false); err != nil {
		return Config{}, invalid
	}
	if config.Brand.Name, ok = trimmed(req.Brand.Name, 40); !ok {
		return Config{}, invalid
	}
	if config.Brand.Welcome, ok = trimmed(req.Brand.Welcome, 200); !ok {
		return Config{}, invalid
	}
	color := strings.TrimSpace(req.Brand.Color)
	if color != "" && !colorPattern.MatchString(color) {
		return Config{}, invalid
	}
	config.Brand.Color = strings.ToLower(color)
	if config.Brand.Logo, err = normalizeLogo(req.Brand.Logo); err != nil {
		return Config{}, err
	}
	for _, field := range []struct {
		value  string
		target *string
	}{
		{req.Recommend.ClaudeCode.Group, &config.Recommend.ClaudeCode.Group},
		{req.Recommend.ClaudeCode.Model, &config.Recommend.ClaudeCode.Model},
		{req.Recommend.Codex.Group, &config.Recommend.Codex.Group},
		{req.Recommend.Codex.Model, &config.Recommend.Codex.Model},
	} {
		if *field.target, ok = trimmed(field.value, 100); !ok {
			return Config{}, invalid
		}
	}
	if config.Notice, ok = trimmed(req.Notice, 2000); !ok {
		return Config{}, invalid
	}
	config.Support = []SupportLink{}
	for _, link := range req.Support {
		label, labelOK := trimmed(link.Label, 30)
		value, valueOK := trimmed(link.Value, 300)
		kind := strings.TrimSpace(link.Type)
		if label == "" && value == "" {
			continue
		}
		if !labelOK || !valueOK || value == "" || !supportTypes[kind] {
			return Config{}, invalid
		}
		config.Support = append(config.Support, SupportLink{Type: kind, Label: label, Value: value})
	}
	if len(config.Support) > maxSupportLinks {
		return Config{}, invalid
	}
	version := strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(req.MinClientVersion), "v"), "V")
	if version != "" && !clientVersionPattern.MatchString(version) {
		return Config{}, invalid
	}
	config.MinClientVersion = version
	config.TicketsEnabled = req.Tickets.Enabled
	if config.TicketCategories, ok = normalizeCategories(req.Tickets.Categories); !ok {
		return Config{}, invalid
	}
	return config, nil
}

// DefaultTicketCategories 是工单的默认问题分类（和迁移 000026 的默认值一致）。
var DefaultTicketCategories = []string{"通用问题", "余额/计费", "接口调用", "账号/登录"}

// normalizeCategories 去掉空白和重复的分类；最多 20 个、每个最多 40 个字；一个都没有时用默认分类。
func normalizeCategories(values []string) ([]string, bool) {
	categories := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		if utf8.RuneCountInString(value) > 40 {
			return nil, false
		}
		seen[value] = true
		categories = append(categories, value)
	}
	if len(categories) > 20 {
		return nil, false
	}
	if len(categories) == 0 {
		categories = append([]string(nil), DefaultTicketCategories...)
	}
	return categories, true
}

// ByCode 按专属链接代码读取配置（工单入口用）；没有时返回 nil。
func (s *Service) ByCode(ctx context.Context, code string) (*Config, error) {
	return s.store.GetByCode(ctx, code)
}

// ForWorkspace 读取工作区的配置（工单用）；没有保存过时返回 nil。
func (s *Service) ForWorkspace(ctx context.Context, ws Workspace) (*Config, error) {
	return s.store.Get(ctx, ws)
}

func categoriesOrDefault(categories []string) []string {
	if len(categories) == 0 {
		return append([]string(nil), DefaultTicketCategories...)
	}
	return categories
}

// clientVersionPattern：TransitLink 的版本号，例如 0.2.0。
var clientVersionPattern = regexp.MustCompile(`^[0-9]{1,4}\.[0-9]{1,4}\.[0-9]{1,4}$`)

// normalizeURL 校验 http(s) 地址。keepPath 为真时保留路径（TransitHub 可能部署在子路径下），
// 否则只留协议、主机和端口（站点地址）。
func normalizeURL(value string, keepPath bool) (string, error) {
	value = strings.TrimRight(strings.TrimSpace(value), "/")
	if value == "" {
		return "", nil
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || len(value) > 300 {
		return "", requestError(ErrorInvalid)
	}
	result := parsed.Scheme + "://" + parsed.Host
	if keepPath {
		result += strings.TrimRight(parsed.EscapedPath(), "/")
	}
	return result, nil
}

// normalizeLogo 接受空、https 图片地址，或上传后转成的 data URL（png、jpeg、webp、gif，最大 256 KB）。
func normalizeLogo(value string) (string, error) {
	value = strings.TrimSpace(value)
	switch {
	case value == "":
		return "", nil
	case strings.HasPrefix(value, "data:"):
		if _, data, ok := decodeLogo(value); !ok {
			if len(data) > maxLogoBytes {
				return "", requestError(ErrorLogoTooLarge)
			}
			return "", requestError(ErrorLogoType)
		}
		return value, nil
	default:
		parsed, err := url.Parse(value)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" || len(value) > 500 {
			return "", requestError(ErrorInvalid)
		}
		return value, nil
	}
}

// decodeLogo 解析 `data:<类型>;base64,<内容>`。类型不允许或超过大小时 ok 为假（超过大小时仍返回内容，便于区分错误）。
func decodeLogo(value string) (string, []byte, bool) {
	header, payload, found := strings.Cut(strings.TrimPrefix(value, "data:"), ",")
	if !strings.HasPrefix(value, "data:") || !found {
		return "", nil, false
	}
	contentType, encoding, _ := strings.Cut(header, ";")
	if encoding != "base64" || !logoTypes[strings.ToLower(contentType)] {
		return "", nil, false
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, false
	}
	if len(data) > maxLogoBytes {
		return "", data, false
	}
	return strings.ToLower(contentType), data, true
}

const codeAlphabet = "abcdefghijkmnpqrstuvwxyz23456789"

// randomCode 生成专属链接的代码：10 位小写字母和数字（去掉了容易看错的 0、o、1、l）。
func randomCode() string {
	buffer := make([]byte, codeLength)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	for i, value := range buffer {
		buffer[i] = codeAlphabet[int(value)%len(codeAlphabet)]
	}
	return string(buffer)
}

func validCode(code string) bool {
	if len(code) != codeLength {
		return false
	}
	for _, char := range code {
		if !strings.ContainsRune(codeAlphabet, char) {
			return false
		}
	}
	return true
}
