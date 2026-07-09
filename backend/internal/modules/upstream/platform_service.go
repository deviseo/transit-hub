package upstream

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// jsonUnmarshalString 将 JSON 字符串解析到目标结构。
func jsonUnmarshalString(s string, v any) error {
	return json.Unmarshal([]byte(s), v)
}

// jsonMarshal 将值序列化为 JSON 字节。
func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

const refreshSkewMS int64 = 60_000

type PlatformService struct {
	httpClient *HTTPClient
}

func NewPlatformService(httpClient *HTTPClient) *PlatformService {
	return &PlatformService{httpClient: httpClient}
}

func (s *PlatformService) NormalizeURL(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	// 仅允许 http/https，且主机名必须是合法 IP 或域名，拦截明显写错的站点地址。
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !isValidHost(parsed.Hostname()) {
		return "", newRequestError(ErrorInvalidURL, "")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (s *PlatformService) Login(baseURL string, platform Platform, account string, password string) (LoginResult, error) {
	normalizedURL, err := s.NormalizeURL(baseURL)
	if err != nil {
		return LoginResult{}, err
	}
	switch platform {
	case PlatformNewAPI:
		return s.loginNewAPI(normalizedURL, account, password)
	case PlatformSub2API:
		return s.loginSub2API(normalizedURL, account, password)
	default:
		result, err := s.loginNewAPI(normalizedURL, account, password)
		if err == nil {
			return result, nil
		}
		return s.loginSub2API(normalizedURL, account, password)
	}
}

func (s *PlatformService) LoginWithToken(baseURL string, platform Platform, account string, accessToken string, refreshToken string, tokenType string) (LoginResult, error) {
	if platform == PlatformNewAPI {
		return LoginResult{}, newRequestError(ErrorAuth, PlatformSub2API)
	}
	normalizedURL, err := s.NormalizeURL(baseURL)
	if err != nil {
		return LoginResult{}, err
	}
	accessToken = strings.TrimSpace(accessToken)
	refreshToken = strings.TrimSpace(refreshToken)
	tokenType = strings.TrimSpace(tokenType)
	if tokenType == "" {
		tokenType = "Bearer"
	}
	session := Session{Platform: PlatformSub2API, BaseURL: normalizedURL, AccessToken: accessToken, RefreshToken: refreshToken, TokenType: tokenType}
	if session.RefreshToken != "" {
		refreshedSession, err := s.refreshSub2APISession(session)
		if err != nil {
			return LoginResult{}, err
		}
		session = refreshedSession
	}
	if strings.TrimSpace(session.AccessToken) == "" {
		return LoginResult{}, newRequestError(ErrorAuth, PlatformSub2API)
	}
	metrics, err := s.fetchSub2APIMetrics(session)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Platform: PlatformSub2API, Session: session, Metrics: metrics}, nil
}

func (s *PlatformService) RefreshSession(session Session) (Session, error) {
	if session.Platform == PlatformNewAPI {
		return session, nil
	}
	now := time.Now().UnixMilli()
	if session.RefreshToken == "" || (session.ExpiresAt != nil && *session.ExpiresAt-now > refreshSkewMS) {
		return session, nil
	}
	return s.refreshSub2APISession(session)
}

func (s *PlatformService) refreshSub2APISession(session Session) (Session, error) {
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/auth/refresh", requestOptions{
		Method: http.MethodPost,
		Body: map[string]string{
			"refresh_token": session.RefreshToken,
		},
	})
	if err != nil {
		return Session{}, err
	}
	data := dataRecord(response.Payload)
	accessToken := firstString(data, []string{"access_token", "accessToken"})
	if accessToken == nil {
		return Session{}, newRequestError(ErrorAuth, PlatformSub2API)
	}
	refreshToken := session.RefreshToken
	if value := firstString(data, []string{"refresh_token", "refreshToken"}); value != nil {
		refreshToken = *value
	}
	tokenType := session.TokenType
	if value := firstString(data, []string{"token_type", "tokenType"}); value != nil {
		tokenType = *value
	}
	expiresAt := session.ExpiresAt
	if expiresIn := firstNumber(data, []string{"expires_in", "expiresIn"}); expiresIn != nil {
		next := time.Now().UnixMilli() + int64(*expiresIn*1000)
		expiresAt = &next
	}
	return Session{Platform: PlatformSub2API, BaseURL: session.BaseURL, AccessToken: *accessToken, RefreshToken: refreshToken, TokenType: tokenType, ExpiresAt: expiresAt}, nil
}

func (s *PlatformService) FetchMetrics(session Session) (Metrics, error) {
	if session.Platform == PlatformNewAPI {
		return s.fetchNewAPIMetrics(session, nil)
	}
	return s.fetchSub2APIMetrics(session)
}

// LoginAdmin 平台中性的 admin 登录：按平台分发到 sub2api 或 new-api 的 admin 登录流程。
// 登录成功后内部已完成 admin 角色校验。
func (s *PlatformService) LoginAdmin(baseURL string, platform Platform, account string, password string) (Session, error) {
	switch platform {
	case PlatformNewAPI:
		return s.LoginNewAPIAdmin(baseURL, account, password)
	default:
		return s.LoginSub2APIAdmin(baseURL, account, password)
	}
}

// VerifyAdmin 平台中性的 admin 校验：按 session.Platform 分发到对应平台的校验逻辑。
func (s *PlatformService) VerifyAdmin(session Session) error {
	switch session.Platform {
	case PlatformNewAPI:
		return s.VerifyNewAPIAdmin(session)
	default:
		return s.VerifySub2APIAdmin(session)
	}
}

func (s *PlatformService) LoginSub2APIAdmin(baseURL string, email string, password string) (Session, error) {
	normalizedURL, err := s.NormalizeURL(baseURL)
	if err != nil {
		return Session{}, err
	}
	log.Printf("my-sites sub2api admin login start base_url=%s email=%s", normalizedURL, email)
	result, err := s.loginSub2API(normalizedURL, email, password)
	if err != nil {
		log.Printf("my-sites sub2api admin login failed base_url=%s email=%s err=%v", normalizedURL, email, err)
		return Session{}, err
	}
	log.Printf("my-sites sub2api admin login token received base_url=%s email=%s token_type=%s expires_at_set=%t refresh_token_set=%t", normalizedURL, email, result.Session.TokenType, result.Session.ExpiresAt != nil, result.Session.RefreshToken != "")
	if err := s.VerifySub2APIAdmin(result.Session); err != nil {
		log.Printf("my-sites sub2api admin verify failed base_url=%s email=%s err=%v", normalizedURL, email, err)
		return Session{}, err
	}
	log.Printf("my-sites sub2api admin verify passed base_url=%s email=%s", normalizedURL, email)
	return result.Session, nil
}

// LoginNewAPIAdmin 登录 new-api admin 并校验 role >= 10。
// 登录成功后同时拉取 /api/status 获取 quota 换算配置。
func (s *PlatformService) LoginNewAPIAdmin(baseURL string, username string, password string) (Session, error) {
	normalizedURL, err := s.NormalizeURL(baseURL)
	if err != nil {
		return Session{}, err
	}
	log.Printf("new-api admin login start base_url=%s username=%s", normalizedURL, username)
	result, err := s.loginNewAPI(normalizedURL, username, password)
	if err != nil {
		log.Printf("new-api admin login failed base_url=%s username=%s err=%v", normalizedURL, username, err)
		return Session{}, err
	}
	if err := s.VerifyNewAPIAdmin(result.Session); err != nil {
		log.Printf("new-api admin verify failed base_url=%s username=%s err=%v", normalizedURL, username, err)
		return Session{}, err
	}
	// 拉取 quota 换算配置
	quotaPerUnit := s.fetchNewAPIQuotaPerUnit(result.Session)
	result.Session.QuotaPerUnit = quotaPerUnit
	log.Printf("new-api admin login+verify passed base_url=%s username=%s quota_per_unit=%.0f", normalizedURL, username, quotaPerUnit)
	return result.Session, nil
}

// VerifyNewAPIAdmin 调用 /api/user/self 校验 new-api 用户是否为 admin（role >= 10）。
func (s *PlatformService) VerifyNewAPIAdmin(session Session) error {
	if session.Platform != PlatformNewAPI || strings.TrimSpace(session.Cookie) == "" {
		return newRequestError(ErrorAuth, PlatformNewAPI)
	}
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/user/self", cookieOptions)
	if err != nil {
		return err
	}
	selfData := dataRecord(response.Payload)
	role := firstNumber(selfData, []string{"role"})
	if role == nil || *role < 10 {
		log.Printf("new-api admin verify role rejected base_url=%s role=%v", session.BaseURL, role)
		return newRequestError(ErrorAuth, PlatformNewAPI)
	}
	return nil
}

// fetchNewAPIQuotaPerUnit 调用 /api/status 获取 new-api 的 quota_per_unit 配置。
// 失败时返回默认值 500000。
func (s *PlatformService) fetchNewAPIQuotaPerUnit(session Session) float64 {
	const defaultQuotaPerUnit = 500000
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/status", cookieOptions)
	if err != nil {
		log.Printf("new-api /api/status fetch failed base_url=%s err=%v, using default quota_per_unit", session.BaseURL, err)
		return defaultQuotaPerUnit
	}
	data := dataRecord(response.Payload)
	if qpu := firstNumber(data, []string{"quota_per_unit", "quotaPerUnit"}); qpu != nil && *qpu > 0 {
		return *qpu
	}
	return defaultQuotaPerUnit
}

func (s *PlatformService) VerifySub2APIAdmin(session Session) error {
	if session.Platform != PlatformSub2API {
		log.Printf("my-sites sub2api admin verify skipped invalid_platform=%s base_url=%s", session.Platform, session.BaseURL)
		return newRequestError(ErrorAuth, PlatformSub2API)
	}
	requestURL := session.BaseURL + "/api/v1/auth/me"
	log.Printf("my-sites sub2api admin verify request url=%s token_type=%s access_token_set=%t", requestURL, session.TokenType, session.AccessToken != "")
	response, err := s.httpClient.requestJSON(requestURL, requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType})
	if err != nil {
		log.Printf("my-sites sub2api admin verify request failed url=%s err=%v", requestURL, err)
		return err
	}
	record := dataRecord(response.Payload)
	role := firstString(record, []string{"role"})
	log.Printf("my-sites sub2api admin verify response url=%s payload_type=%T data_keys=%s parsed_role=%s", requestURL, response.Payload, recordKeys(record), stringValue(role))
	if role == nil || !strings.EqualFold(*role, "admin") {
		log.Printf("my-sites sub2api admin verify role rejected url=%s parsed_role=%s", requestURL, stringValue(role))
		return newRequestError(ErrorAuth, PlatformSub2API)
	}
	return nil
}

func recordKeys(record map[string]any) string {
	if len(record) == 0 {
		return ""
	}
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	return strings.Join(keys, ",")
}

func stringValue(value *string) string {
	if value == nil {
		return "<nil>"
	}
	return *value
}

func (s *PlatformService) FetchSub2APIGroupDailyStats(session Session, groups ...[]GroupInfo) ([]GroupDailyStat, error) {
	stats, err := s.fetchSub2APIGroupUsageSummaryStats(session)
	if err == nil {
		return stats, nil
	}
	stats, err = s.fetchSub2APIKeyGroupDailyStats(session)
	if err == nil {
		return stats, nil
	}
	if err := s.VerifySub2APIAdmin(session); err != nil {
		return nil, err
	}
	today := time.Now().Format("2006-01-02")
	statsURL := session.BaseURL + "/api/v1/admin/dashboard/groups?start_date=" + today + "&end_date=" + today
	response, err := s.httpClient.requestJSON(statsURL, requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType})
	if err != nil {
		return nil, err
	}
	items := dataArray(response.Payload)
	if len(items) == 0 {
		if groups, ok := dataRecord(response.Payload)["groups"].([]any); ok {
			items = groups
		}
	}
	stats = make([]GroupDailyStat, 0, len(items))
	for _, item := range items {
		name := firstString(item, []string{"group_name", "groupName", "name"})
		if name == nil || strings.TrimSpace(*name) == "" {
			continue
		}
		stats = append(stats, GroupDailyStat{GroupName: *name, TodayActualCost: sub2APIGroupDailyCost(item)})
	}
	return stats, nil
}

// FetchSub2APIAdminUsageStats 调用管理员的 sub2api 站点 /api/v1/admin/usage/stats 接口，
// 查询指定日期范围内的总实际消费（即站点的盈利额度）。
// startDate 和 endDate 格式为 "2006-01-02"，查询当天数据时两者传同一天即可。
func (s *PlatformService) FetchSub2APIAdminUsageStats(session Session, startDate, endDate string) (float64, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return 0, newRequestError(ErrorAuth, PlatformSub2API)
	}
	statsURL := session.BaseURL + "/api/v1/admin/usage/stats?start_date=" + startDate + "&end_date=" + endDate
	response, err := s.httpClient.requestJSON(statsURL, requestOptions{
		AccessToken: session.AccessToken,
		TokenType:   session.TokenType,
	})
	if err != nil {
		return 0, err
	}
	data := dataRecord(response.Payload)
	cost := firstNumber(data, []string{"total_actual_cost", "totalActualCost", "total_cost", "totalCost"})
	if cost == nil {
		return 0, nil
	}
	return *cost, nil
}

// FetchSub2APIAdminSiteBalance 使用默认过滤规则（排除 admin 角色）统计站点用户总余额。
// my_sites 模块调用此方法时无需关心自定义筛选条件。
func (s *PlatformService) FetchSub2APIAdminSiteBalance(session Session) (AdminSiteBalance, error) {
	return s.FetchSub2APIAdminSiteBalanceFiltered(session, BalanceFilter{ExcludeAdmin: true})
}

// FetchSub2APIAdminSiteBalanceFiltered 按自定义过滤规则统计站点用户总余额。
// 先获取第 1 页以确定总数，然后并发获取剩余页面（并发上限 5），
// 对每个用户按 filter 条件判断是否跳过，最后汇总余额。
func (s *PlatformService) FetchSub2APIAdminSiteBalanceFiltered(session Session, filter BalanceFilter) (AdminSiteBalance, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return AdminSiteBalance{}, newRequestError(ErrorAuth, PlatformSub2API)
	}

	const pageSize = 100
	const maxConcurrency = 5
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}

	// 第 1 页顺序获取，确定总用户数。
	firstURL := session.BaseURL + "/api/v1/admin/users?page=1&page_size=" + strconvInt(pageSize)
	firstResponse, err := s.httpClient.requestJSON(firstURL, authOptions)
	if err != nil {
		return AdminSiteBalance{}, err
	}
	firstUsers := dataArray(firstResponse.Payload)
	if len(firstUsers) == 0 {
		return AdminSiteBalance{Balance: 0}, nil
	}
	totalBalance := sumFilteredBalances(firstUsers, filter)

	// 判断是否只有一页。
	total, hasTotal := paginationTotal(firstResponse.Payload)
	if (!hasTotal && len(firstUsers) < pageSize) || (hasTotal && pageSize >= total) {
		return AdminSiteBalance{Balance: totalBalance}, nil
	}

	// 计算剩余页数，并发获取。
	totalPages := (total + pageSize - 1) / pageSize
	if !hasTotal {
		totalPages = 1000
	}

	sem := make(chan struct{}, maxConcurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var fetchErr error

	for page := 2; page <= totalPages; page++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(page int) {
			defer wg.Done()
			defer func() { <-sem }()

			pageURL := session.BaseURL + "/api/v1/admin/users?page=" + strconvInt(int64(page)) + "&page_size=" + strconvInt(pageSize)
			response, err := s.httpClient.requestJSON(pageURL, authOptions)
			if err != nil {
				mu.Lock()
				if fetchErr == nil {
					fetchErr = err
				}
				mu.Unlock()
				return
			}
			users := dataArray(response.Payload)
			if len(users) == 0 {
				return
			}
			pageBalance := sumFilteredBalances(users, filter)
			mu.Lock()
			totalBalance += pageBalance
			mu.Unlock()
		}(page)
	}
	wg.Wait()

	if fetchErr != nil {
		return AdminSiteBalance{}, fetchErr
	}
	return AdminSiteBalance{Balance: totalBalance}, nil
}

// sumFilteredBalances 对一页用户数据按过滤条件计算余额小计。
func sumFilteredBalances(users []any, filter BalanceFilter) float64 {
	var sum float64
	for _, user := range users {
		if filter.ExcludeAdmin {
			role := firstString(user, []string{"role"})
			if role != nil && strings.EqualFold(strings.TrimSpace(*role), "admin") {
				continue
			}
		}
		balance := firstNumber(user, []string{"balance"})
		if balance == nil {
			continue
		}
		if balanceExcluded(*balance, filter.ExcludeBalances) {
			continue
		}
		sum += *balance
	}
	return sum
}

// FetchSub2APIAdminGroups 获取管理员站点的所有分组列表。
// 复用 fetchSub2APIAvailableGroupsWithRates，与 fetchSub2APIMetrics 共用同一套
// "默认倍率 + 专属倍率覆盖" 解析逻辑，避免两个入口分别维护、其中一个遗漏修复。
func (s *PlatformService) FetchSub2APIAdminGroups(session Session) ([]GroupInfo, error) {
	return s.fetchSub2APIAvailableGroupsWithRates(session)
}

// fetchSub2APIAvailableGroupsWithRates 获取 sub2api 用户可见分组列表，并按 sub2api 文档规则
// 合并专属倍率：
//  1. 先以 /api/v1/groups/available 的 rate_multiplier 作为每个分组的默认倍率。
//  2. 再拉取 /api/v1/groups/rates，按相同分组 ID 覆盖默认倍率。
//  3. /groups/rates 缺失某个 ID 时保留默认倍率；出现 available 不包含的未知 ID 时不新增分组
//     （缺少 name、platform 等基础展示字段）。
//
// Multiplier/MultiplierDisplay 始终表示最终生效倍率，供现有业务计算直接使用；
// DefaultMultiplier/DedicatedMultiplier/HasDedicatedMultiplier 是新增的向后兼容字段，
// 仅供前端展示"默认倍率 -> 专属倍率"提示。
//
// /groups/rates 请求失败（含旧版 sub2api 没有该接口的 404）时不影响返回结果，只回退到
// 默认倍率并记录日志，因为 available 已经提供了完整的基础分组列表；调用方（登录、同步）
// 因此不会因为这一个可选接口失败而整体失败。
func (s *PlatformService) fetchSub2APIAvailableGroupsWithRates(session Session) ([]GroupInfo, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}

	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/groups/available", authOptions)
	if err != nil {
		return nil, err
	}

	rateOverrides := map[string]float64{}
	ratesResponse, ratesErr := s.httpClient.requestJSON(session.BaseURL+"/api/v1/groups/rates", authOptions)
	if ratesErr != nil {
		log.Printf("[sub2api-groups] /api/v1/groups/rates 拉取失败，回退默认倍率 base_url=%s err=%v", session.BaseURL, ratesErr)
	} else {
		rateOverrides = sub2APIGroupRateOverrides(ratesResponse.Payload)
	}

	groups := make([]GroupInfo, 0)
	for _, item := range dataArray(response.Payload) {
		id := groupID(item)
		name := defaultDisplay
		if value := firstString(item, []string{"name"}); value != nil {
			name = *value
		}
		if name == defaultDisplay {
			continue
		}
		platform := firstString(item, []string{"platform"})
		defaultRate := firstNumber(item, []string{"rate_multiplier"})

		group := GroupInfo{
			ID:                       id,
			Name:                     name,
			Platform:                 platform,
			Multiplier:               defaultRate,
			MultiplierDisplay:        multiplier(defaultRate),
			DefaultMultiplier:        defaultRate,
			DefaultMultiplierDisplay: multiplier(defaultRate),
		}

		if dedicatedRate, ok := rateOverrides[id]; ok && id != "" {
			rate := dedicatedRate
			group.Multiplier = &rate
			group.MultiplierDisplay = multiplier(&rate)
			group.DedicatedMultiplier = &rate
			group.DedicatedMultiplierDisplay = multiplier(&rate)
			group.HasDedicatedMultiplier = true
		}

		groups = append(groups, group)
	}
	return groups, nil
}

// sub2APIGroupRateOverrides 将 /api/v1/groups/rates 的响应解析为 "分组 ID -> 专属倍率" 映射。
// 兼容 sub2api 上游包装层可能出现的几种形态：
//   - {"data": {"1": 0.8, "2": 1.2}}                       对象 map，key 为分组 ID
//   - {"data": [{"group_id": 1, "rate_multiplier": 0.8}]}  对象数组，包在 data 里
//   - [{"groupId": "1", "rateMultiplier": "0.8"}]          未包装的对象数组
//
// 缺少 ID 或倍率不是有效数字的条目会被静默忽略，不影响其余覆盖项按 ID 生效。
func sub2APIGroupRateOverrides(payload any) map[string]float64 {
	overrides := map[string]float64{}

	var dataValue any = payload
	if record, ok := payload.(map[string]any); ok {
		if data, exists := record["data"]; exists {
			dataValue = data
		}
	}

	switch typed := dataValue.(type) {
	case map[string]any:
		for id, value := range typed {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if rate := readNumber(value); rate != nil {
				overrides[id] = *rate
			}
		}
	case []any:
		for _, item := range typed {
			id := groupID(item)
			if id == "" {
				continue
			}
			rate := firstNumber(item, []string{"rate_multiplier", "rateMultiplier", "multiplier", "rate"})
			if rate == nil {
				continue
			}
			overrides[id] = *rate
		}
	}
	return overrides
}

// AdminGroupInfo 是 /api/v1/admin/groups 返回的完整分组信息，
// 包含 status、is_exclusive、subscription_type 等管理端专有字段。
type AdminGroupInfo struct {
	ID                string
	Name              string
	Platform          string
	Status            string // active / inactive
	IsExclusive       bool   // true = 专属分组
	SubscriptionType  string // standard / subscription
	Multiplier        *float64
	MultiplierDisplay string
}

// FetchSub2APIAdminAllGroups 通过 /api/v1/admin/groups 获取管理端全量分组列表，
// 包括专属分组、已禁用分组等 /api/v1/groups/available 不返回的条目。
func (s *PlatformService) FetchSub2APIAdminAllGroups(session Session) ([]AdminGroupInfo, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/admin/groups", authOptions)
	if err != nil {
		return nil, err
	}
	groups := make([]AdminGroupInfo, 0)
	for _, item := range dataArray(response.Payload) {
		id := groupID(item)
		name := defaultDisplay
		if value := firstString(item, []string{"name"}); value != nil {
			name = *value
		}
		if name == defaultDisplay {
			continue
		}
		platform := ""
		if value := firstString(item, []string{"platform"}); value != nil {
			platform = *value
		}
		// status 为字符串，如 active / inactive
		status := ""
		if value := firstString(item, []string{"status"}); value != nil {
			status = *value
		}
		// is_exclusive 标识专属分组
		isExclusive := false
		if record, ok := item.(map[string]any); ok {
			if v, ok := record["is_exclusive"].(bool); ok {
				isExclusive = v
			}
		}
		// subscription_type 区分普通分组(standard)和订阅分组(subscription)
		subscriptionType := ""
		if value := firstString(item, []string{"subscription_type"}); value != nil {
			subscriptionType = *value
		}
		rate := firstNumber(item, []string{"rate_multiplier"})
		groups = append(groups, AdminGroupInfo{
			ID:                id,
			Name:              name,
			Platform:          platform,
			Status:            status,
			IsExclusive:       isExclusive,
			SubscriptionType:  subscriptionType,
			Multiplier:        rate,
			MultiplierDisplay: multiplier(rate),
		})
	}
	return groups, nil
}

// balanceExcluded 检查余额值是否在排除列表中（使用 epsilon 比较避免浮点精度问题）。
func balanceExcluded(balance float64, excludes []float64) bool {
	const epsilon = 1e-9
	for _, v := range excludes {
		if math.Abs(balance-v) < epsilon {
			return true
		}
	}
	return false
}

func (s *PlatformService) fetchSub2APIGroupUsageSummaryStats(session Session) ([]GroupDailyStat, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}
	groupsResponse, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/groups/available", authOptions)
	if err != nil {
		return nil, err
	}
	groupNames := map[int64]string{}
	for _, item := range dataArray(groupsResponse.Payload) {
		id := firstNumber(item, []string{"id"})
		name := firstString(item, []string{"name"})
		if id == nil || name == nil || strings.TrimSpace(*name) == "" {
			continue
		}
		groupNames[int64(*id)] = strings.TrimSpace(*name)
	}
	if len(groupNames) == 0 {
		return nil, newRequestError(ErrorInvalidResponse, PlatformSub2API)
	}
	usageResponse, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/admin/groups/usage-summary", authOptions)
	if err != nil {
		return nil, err
	}
	items := dataArray(usageResponse.Payload)
	if len(items) == 0 {
		return nil, newRequestError(ErrorInvalidResponse, PlatformSub2API)
	}
	stats := make([]GroupDailyStat, 0, len(items))
	for _, item := range items {
		groupID := firstNumber(item, []string{"group_id", "groupId"})
		if groupID == nil {
			continue
		}
		name := groupNames[int64(*groupID)]
		if name == "" {
			continue
		}
		stats = append(stats, GroupDailyStat{GroupName: name, TodayActualCost: sub2APIUsageSummaryCost(item)})
	}
	return stats, nil
}

func sub2APIUsageSummaryCost(item any) float64 {
	cost := firstNumber(item, []string{"today_cost", "todayCost", "today_actual_cost", "todayActualCost"})
	if cost == nil {
		return 0
	}
	return *cost
}

func (s *PlatformService) fetchSub2APIKeyGroupDailyStats(session Session) ([]GroupDailyStat, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}
	keysResponse, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/keys", authOptions)
	if err != nil {
		return nil, err
	}
	keys := dataArray(keysResponse.Payload)
	if len(keys) == 0 {
		return nil, newRequestError(ErrorInvalidResponse, PlatformSub2API)
	}
	today := time.Now().Format("2006-01-02")
	totals := map[string]float64{}
	for _, item := range keys {
		keyID := firstNumber(item, []string{"id"})
		groupName := sub2APIKeyGroupName(item)
		if keyID == nil || strings.TrimSpace(groupName) == "" || groupName == defaultDisplay {
			continue
		}
		// Sub2API exposes reliable per-key daily usage through /usage/stats. Each
		// key carries its group object from /keys, so summing every key's actual cost
		// by group gives a group-level total even when the admin dashboard endpoint is
		// unavailable for ordinary upstream tokens.
		statsURL := session.BaseURL + "/api/v1/usage/stats?start_date=" + today + "&end_date=" + today + "&api_key_id=" + strconvInt(int64(*keyID)) + "&timezone=Asia%2FShanghai"
		statsResponse, err := s.httpClient.requestJSON(statsURL, authOptions)
		if err != nil {
			return nil, err
		}
		totals[groupName] += sub2APIUsageStatsCost(dataRecord(statsResponse.Payload))
	}
	stats := make([]GroupDailyStat, 0, len(totals))
	for groupName, total := range totals {
		stats = append(stats, GroupDailyStat{GroupName: groupName, TodayActualCost: total})
	}
	return stats, nil
}

func sub2APIKeyGroupName(item any) string {
	record, ok := item.(map[string]any)
	if !ok {
		return ""
	}
	if group, ok := record["group"].(map[string]any); ok {
		if name := firstString(group, []string{"name"}); name != nil {
			return strings.TrimSpace(*name)
		}
	}
	if name := firstString(item, []string{"group_name", "groupName"}); name != nil {
		return strings.TrimSpace(*name)
	}
	return ""
}

func sub2APIUsageStatsCost(item any) float64 {
	cost := firstNumber(item, []string{"total_actual_cost", "totalActualCost", "actual_cost", "actualCost", "cost"})
	if cost == nil {
		return 0
	}
	return *cost
}

func sub2APIGroupDailyCost(item any) float64 {
	cost := firstNumber(item, []string{"today_actual_cost", "todayActualCost", "actual_cost", "actualCost", "cost", "today_cost", "todayCost", "usage", "used"})
	if cost == nil {
		return 0
	}
	return *cost
}

func (s *PlatformService) FetchNewAPIGroupDailyStats(session Session, groups []GroupInfo) ([]GroupDailyStat, error) {
	if session.Platform != PlatformNewAPI || strings.TrimSpace(session.Cookie) == "" {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}
	stats := make([]GroupDailyStat, 0, len(groups))
	for _, group := range groups {
		name := strings.TrimSpace(group.Name)
		if name == "" || name == defaultDisplay {
			continue
		}
		statURL := session.BaseURL + "/api/log/self/stat?type=2&start_timestamp=" + strconvInt(todayStart()) + "&end_timestamp=" + strconvInt(todayEnd()) + "&group=" + url.QueryEscape(name)
		payload, err := s.httpClient.requestJSON(statURL, cookieOptions)
		if err != nil {
			return nil, err
		}
		stats = append(stats, GroupDailyStat{GroupName: name, TodayActualCost: quotaToUSDValueWithUnit(firstNumber(dataRecord(payload.Payload), []string{"quota"}), session.QuotaPerUnit)})
	}
	return stats, nil
}

// FetchKeyUsageToday 按平台分发获取上游站点今天各 key 的消费统计。
// 返回值为上游平台原始金额，未乘以站点 rechargeRate；由 upstream.Service 层完成 CNY 换算和跨站点聚合。
func (s *PlatformService) FetchKeyUsageToday(session Session, groups []GroupInfo) ([]KeyUsageTodayStat, error) {
	switch session.Platform {
	case PlatformSub2API:
		return s.fetchSub2APIKeyUsageToday(session)
	case PlatformNewAPI:
		return s.fetchNewAPIKeyUsageToday(session, groups)
	default:
		return nil, newRequestError(ErrorNotFound, "")
	}
}

// sub2APIKeyRecord 是分页拉取 /api/v1/keys 后缓存的 key 基本信息，供后续并发查询 usage stats 使用。
type sub2APIKeyRecord struct {
	id        string
	name      string
	groupName string
}

// fetchSub2APIKeyUsageToday 分页拉取 sub2api 站点全部 key（不能只取第一页），
// 再并发查询每个 key 的今日 usage stats（并发上限 maxKeyConcurrency），只保留消费 > 0 的 key。
func (s *PlatformService) fetchSub2APIKeyUsageToday(session Session) ([]KeyUsageTodayStat, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}

	const pageSize = 100
	const maxPages = 100 // 安全上限，防止上游分页字段异常导致死循环
	records := make([]sub2APIKeyRecord, 0)
	for page := 1; page <= maxPages; page++ {
		pageURL := session.BaseURL + "/api/v1/keys?page=" + strconvInt(int64(page)) + "&page_size=" + strconvInt(pageSize) + "&sort_by=created_at&sort_order=desc"
		response, err := s.httpClient.requestJSON(pageURL, authOptions)
		if err != nil {
			return nil, err
		}
		items := dataArray(response.Payload)
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id := groupID2(record)
			if id == "" {
				continue
			}
			records = append(records, sub2APIKeyRecord{
				id:        id,
				name:      safeString(record, "name"),
				groupName: sub2APIKeyGroupName(record),
			})
		}
		total, hasTotal := paginationTotal(response.Payload)
		if hasTotal && page*pageSize >= total {
			break
		}
		if !hasTotal && len(items) < pageSize {
			break
		}
	}
	if len(records) == 0 {
		return nil, nil
	}

	today := time.Now().Format("2006-01-02")
	const maxKeyConcurrency = 4
	sem := make(chan struct{}, maxKeyConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	stats := make([]KeyUsageTodayStat, 0, len(records))
	var firstErr error

	for _, record := range records {
		wg.Add(1)
		sem <- struct{}{}
		go func(record sub2APIKeyRecord) {
			defer wg.Done()
			defer func() { <-sem }()

			statsURL := session.BaseURL + "/api/v1/usage/stats?start_date=" + today + "&end_date=" + today + "&api_key_id=" + record.id + "&timezone=Asia%2FShanghai"
			response, err := s.httpClient.requestJSON(statsURL, authOptions)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			cost := sub2APIUsageStatsCost(dataRecord(response.Payload))
			if cost <= 0 {
				return
			}
			groupName := record.groupName
			mu.Lock()
			stats = append(stats, KeyUsageTodayStat{
				KeyID:       record.id,
				KeyName:     record.name,
				GroupName:   groupName,
				TodayAmount: cost,
			})
			mu.Unlock()
		}(record)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return stats, nil
}

// fetchNewAPIKeyUsageToday 分页拉取 new-api 站点全部 token（不能只取第一页）。
// token 记录自带分组字段时直接按 token_name+group 查询；否则仅按 token_name 查询自助统计接口
// （沿用已验证的 self 统计能力，不做 token×全部分组的穷举以控制并发/请求量）。
// 并发上限 maxKeyConcurrency，只保留今日 quota 换算金额 > 0 的 token。
func (s *PlatformService) fetchNewAPIKeyUsageToday(session Session, groups []GroupInfo) ([]KeyUsageTodayStat, error) {
	if session.Platform != PlatformNewAPI || strings.TrimSpace(session.Cookie) == "" {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}

	const pageSize = 100
	const maxPages = 100
	type tokenRecord struct {
		id        string
		name      string
		groupName string
	}
	records := make([]tokenRecord, 0)
	for page := 1; page <= maxPages; page++ {
		pageURL := session.BaseURL + "/api/token/?p=" + strconvInt(int64(page)) + "&page_size=" + strconvInt(pageSize)
		response, err := s.httpClient.requestJSON(pageURL, cookieOptions)
		if err != nil {
			return nil, err
		}
		items := dataArray(response.Payload)
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id := groupID2(record)
			if id == "" || safeString(record, "name") == "" {
				continue
			}
			groupName := ""
			if g := firstString(record, []string{"group"}); g != nil {
				groupName = strings.TrimSpace(*g)
			}
			records = append(records, tokenRecord{
				id:        id,
				name:      safeString(record, "name"),
				groupName: groupName,
			})
		}
		total, hasTotal := paginationTotal(response.Payload)
		if hasTotal && page*pageSize >= total {
			break
		}
		if !hasTotal && len(items) < pageSize {
			break
		}
	}
	if len(records) == 0 {
		return nil, nil
	}

	const maxKeyConcurrency = 4
	sem := make(chan struct{}, maxKeyConcurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	stats := make([]KeyUsageTodayStat, 0, len(records))
	var firstErr error

	for _, record := range records {
		wg.Add(1)
		sem <- struct{}{}
		go func(record tokenRecord) {
			defer wg.Done()
			defer func() { <-sem }()

			statURL := session.BaseURL + "/api/log/self/stat?type=2&start_timestamp=" + strconvInt(todayStart()) + "&end_timestamp=" + strconvInt(todayEnd()) + "&token_name=" + url.QueryEscape(record.name)
			if record.groupName != "" {
				statURL += "&group=" + url.QueryEscape(record.groupName)
			}
			response, err := s.httpClient.requestJSON(statURL, cookieOptions)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
				return
			}
			amount := quotaToUSDValueWithUnit(firstNumber(dataRecord(response.Payload), []string{"quota"}), session.QuotaPerUnit)
			if amount <= 0 {
				return
			}
			groupName := record.groupName
			if groupName == "" {
				groupName = "Ungrouped"
			}
			mu.Lock()
			stats = append(stats, KeyUsageTodayStat{
				KeyID:       record.id,
				KeyName:     record.name,
				GroupName:   groupName,
				TodayAmount: amount,
			})
			mu.Unlock()
		}(record)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return stats, nil
}

func (s *PlatformService) loginNewAPI(baseURL string, username string, password string) (LoginResult, error) {
	response, err := s.httpClient.requestJSON(baseURL+"/api/user/login", requestOptions{
		Method: http.MethodPost,
		Body:   map[string]string{"username": username, "password": password},
	})
	if err != nil {
		return LoginResult{}, err
	}
	if record, ok := response.Payload.(map[string]any); ok {
		if success, ok := record["success"].(bool); ok && !success {
			return LoginResult{}, newRequestError(ErrorAuth, PlatformNewAPI)
		}
	}
	loginData := dataRecord(response.Payload)
	userID := newAPIUserID(loginData)
	session := Session{Platform: PlatformNewAPI, BaseURL: baseURL, Cookie: cookieHeader(response.Header), UserID: userID}
	if strings.TrimSpace(session.Cookie) == "" {
		return LoginResult{}, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	if strings.TrimSpace(session.UserID) == "" {
		return LoginResult{}, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	session.QuotaPerUnit = s.fetchNewAPIQuotaPerUnit(session)
	metrics, err := s.fetchNewAPIMetrics(session, loginData)
	if err != nil {
		log.Printf("new-api metrics fetch failed base_url=%s err=%v", baseURL, err)
		metrics = defaultMetrics()
	}
	return LoginResult{Platform: PlatformNewAPI, Session: session, Metrics: metrics}, nil
}

func (s *PlatformService) loginSub2API(baseURL string, email string, password string) (LoginResult, error) {
	response, err := s.httpClient.requestJSON(baseURL+"/api/v1/auth/login", requestOptions{
		Method: http.MethodPost,
		Body:   map[string]string{"email": email, "password": password},
	})
	if err != nil {
		return LoginResult{}, err
	}
	data := dataRecord(response.Payload)
	accessToken := firstString(data, []string{"access_token", "accessToken"})
	if accessToken == nil {
		return LoginResult{}, newRequestError(ErrorAuth, PlatformSub2API)
	}
	refreshToken := ""
	if value := firstString(data, []string{"refresh_token", "refreshToken"}); value != nil {
		refreshToken = *value
	}
	tokenType := "Bearer"
	if value := firstString(data, []string{"token_type", "tokenType"}); value != nil {
		tokenType = *value
	}
	var expiresAt *int64
	if expiresIn := firstNumber(data, []string{"expires_in", "expiresIn"}); expiresIn != nil {
		next := time.Now().UnixMilli() + int64(*expiresIn*1000)
		expiresAt = &next
	}
	session := Session{Platform: PlatformSub2API, BaseURL: baseURL, AccessToken: *accessToken, RefreshToken: refreshToken, TokenType: tokenType, ExpiresAt: expiresAt}
	metrics, err := s.fetchSub2APIMetrics(session)
	if err != nil {
		return LoginResult{}, err
	}
	return LoginResult{Platform: PlatformSub2API, Session: session, Metrics: metrics}, nil
}

func (s *PlatformService) fetchNewAPIMetrics(session Session, loginData map[string]any) (Metrics, error) {
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}
	self, err := s.httpClient.requestJSON(session.BaseURL+"/api/user/self", cookieOptions)
	if err != nil {
		return Metrics{}, err
	}
	statURL := session.BaseURL + "/api/log/self/stat?type=2&start_timestamp=" + strconvInt(todayStart()) + "&end_timestamp=" + strconvInt(todayEnd())
	stat, err := s.httpClient.requestJSON(statURL, cookieOptions)
	if err != nil {
		log.Printf("new-api stat request failed base_url=%s err=%v", session.BaseURL, err)
		stat = jsonResponse{Payload: map[string]any{}}
	}
	groupsPayload, err := s.httpClient.requestJSON(session.BaseURL+"/api/user/self/groups", cookieOptions)
	if err != nil {
		groupsPayload, err = s.httpClient.requestJSON(session.BaseURL+"/api/user/groups", cookieOptions)
		if err != nil {
			log.Printf("new-api groups request failed base_url=%s err=%v", session.BaseURL, err)
			groupsPayload = jsonResponse{Payload: map[string]any{}}
		}
	}
	pricingPayload, err := s.httpClient.requestJSON(session.BaseURL+"/api/pricing", cookieOptions)
	if err != nil {
		log.Printf("new-api pricing request failed base_url=%s err=%v", session.BaseURL, err)
		pricingPayload = jsonResponse{Payload: map[string]any{}}
	}

	defaults := defaultMetrics()
	selfData := dataRecord(self.Payload)
	groupName := defaults.Group.Name
	if value := firstString(selfData, []string{"group"}); value != nil {
		groupName = *value
	} else if value := firstString(loginData, []string{"group"}); value != nil {
		groupName = *value
	}
	groups := newAPIGroups(groupsPayload.Payload, pricingPayload.Payload)
	groupRatio := newAPIGroupRatio(groupName, groups)
	quota := firstNumber(selfData, []string{"quota", "balance"})
	usedQuota := firstNumber(selfData, []string{"used_quota", "usedQuota", "used"})
	estimatedGranted := quota
	if quota != nil && usedQuota != nil {
		next := *quota + *usedQuota
		estimatedGranted = &next
	}
	group := GroupInfo{ID: groupName, Name: groupName, Platform: nil, Multiplier: groupRatio, MultiplierDisplay: multiplier(groupRatio)}
	if groupRatio == nil && len(groups) > 0 {
		group = groups[0]
	}
	qpu := session.QuotaPerUnit
	return Metrics{
		Balance:         metric(quotaToUSDWithUnit(quota, qpu)),
		TodayConsume:    metric(quotaToUSDWithUnit(firstNumber(dataRecord(stat.Payload), []string{"quota", "used_quota", "usedQuota"}), qpu)),
		HistoryRecharge: metric(quotaToUSDWithUnit(estimatedGranted, qpu)),
		Group:           group,
		Groups:          groups,
	}, nil
}

func (s *PlatformService) fetchSub2APIMetrics(session Session) (Metrics, error) {
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}
	log.Printf("[sub2api-metrics] 开始拉取指标 base_url=%s", session.BaseURL)
	me, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/auth/me", authOptions)
	if err != nil {
		log.Printf("[sub2api-metrics] /api/v1/auth/me 失败 base_url=%s err=%v", session.BaseURL, err)
		return Metrics{}, err
	}
	stats, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/usage/dashboard/stats", authOptions)
	if err != nil {
		log.Printf("[sub2api-metrics] /api/v1/usage/dashboard/stats 失败 base_url=%s err=%v", session.BaseURL, err)
		return Metrics{}, err
	}
	groups, err := s.fetchSub2APIAvailableGroupsWithRates(session)
	if err != nil {
		log.Printf("[sub2api-metrics] 分组列表拉取失败 base_url=%s err=%v", session.BaseURL, err)
		return Metrics{}, err
	}

	meData := dataRecord(me.Payload)
	statsData := dataRecord(stats.Payload)
	balance := firstNumber(meData, []string{"balance"})
	totalRecharged := firstNumber(meData, []string{"total_recharged"})
	if totalRecharged == nil || *totalRecharged == 0 {
		if totalActualCost := firstNumber(statsData, []string{"total_actual_cost"}); totalActualCost != nil && balance != nil {
			fallbackTotal := *totalActualCost + *balance
			totalRecharged = &fallbackTotal
		}
	}

	firstGroup := defaultMetrics().Group
	if len(groups) > 0 {
		firstGroup = groups[0]
	}
	return Metrics{
		Balance:         metric(balance),
		TodayConsume:    metric(firstNumber(statsData, []string{"today_actual_cost"})),
		HistoryRecharge: metric(totalRecharged),
		Group:           firstGroup,
		Groups:          groups,
	}, nil
}

func cookieHeader(headers http.Header) string {
	cookies := headers.Values("Set-Cookie")
	if len(cookies) == 0 {
		return ""
	}
	parts := make([]string, 0, len(cookies))
	for _, cookie := range cookies {
		parts = append(parts, strings.Split(cookie, ";")[0])
	}
	return strings.Join(parts, "; ")
}

func newAPIUserID(loginData map[string]any) string {
	if value := firstNumber(loginData, []string{"id", "user_id", "userId"}); value != nil {
		return strconv.FormatInt(int64(*value), 10)
	}
	return ""
}

func todayStart() int64 {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return start.Unix()
}

func todayEnd() int64 {
	now := time.Now()
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), now.Location())
	return end.Unix()
}

// ListSub2APIKeys 获取上游 Sub2API 站点的 API Key 列表。
// 按创建时间倒序返回，最多 100 条。每条包含 ID、Key 值、名称、所属分组等信息。
func (s *PlatformService) ListSub2APIKeys(session Session) ([]Sub2APIKeyItem, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	response, err := s.httpClient.requestJSON(
		session.BaseURL+"/api/v1/keys?page=1&page_size=100&sort_by=created_at&sort_order=desc",
		requestOptions{
			AccessToken: session.AccessToken,
			TokenType:   session.TokenType,
		},
	)
	if err != nil {
		return nil, err
	}

	items := dataArray(response.Payload)
	keys := make([]Sub2APIKeyItem, 0, len(items))
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		keyItem := Sub2APIKeyItem{
			ID:   groupID2(record),
			Name: safeString(record, "name"),
		}
		if k := firstString(record, []string{"key", "token", "api_key"}); k != nil {
			keyItem.Key = *k
		}
		if s := firstString(record, []string{"status"}); s != nil {
			keyItem.Status = *s
		}
		if gid := firstNumber(record, []string{"group_id"}); gid != nil {
			keyItem.GroupID = strconv.FormatInt(int64(*gid), 10)
		}
		if group, ok := record["group"].(map[string]any); ok {
			keyItem.GroupName = safeString(group, "name")
		}
		keys = append(keys, keyItem)
	}
	return keys, nil
}

// CreateSub2APIKey 在上游 Sub2API 站点创建一个 API Key。
// name 为 key 名称，groupID 为关联分组的数字 ID。
// 返回新建 key 的 ID（字符串）和 key 值；失败时返回 error。
func (s *PlatformService) CreateSub2APIKey(session Session, name string, groupID int) (string, string, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return "", "", newRequestError(ErrorAuth, PlatformSub2API)
	}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/keys", requestOptions{
		AccessToken: session.AccessToken,
		TokenType:   session.TokenType,
		Method:      http.MethodPost,
		Body: map[string]any{
			"name":     name,
			"group_id": groupID,
		},
	})
	if err != nil {
		return "", "", err
	}
	data := dataRecord(response.Payload)
	keyID := groupID2(data)
	key := firstString(data, []string{"key", "token", "api_key", "apiKey"})
	if key == nil || *key == "" {
		return "", "", newRequestError(ErrorInvalidResponse, PlatformSub2API)
	}
	return keyID, *key, nil
}

// groupID2 从响应记录中提取 ID 并转为字符串，复用 groupID 的逻辑。
func groupID2(record map[string]any) string {
	if id := firstNumber(record, []string{"id"}); id != nil {
		return strconv.FormatInt(int64(*id), 10)
	}
	if id := firstString(record, []string{"id"}); id != nil {
		return *id
	}
	return ""
}

// CreateSub2APIAdminAccount 在 admin 站点通过 /api/v1/admin/accounts 创建转发账号。
// payload 为完整的创建参数（platform、type、credentials、extra 等），由调用方组装。
// 返回新建账号的 ID（字符串）；失败时返回 error。
func (s *PlatformService) CreateSub2APIAdminAccount(session Session, payload map[string]any) (string, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return "", newRequestError(ErrorAuth, PlatformSub2API)
	}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/admin/accounts", requestOptions{
		AccessToken: session.AccessToken,
		TokenType:   session.TokenType,
		Method:      http.MethodPost,
		Body:        payload,
	})
	if err != nil {
		return "", err
	}
	data := dataRecord(response.Payload)
	accountID := groupID2(data)
	return accountID, nil
}

// DeleteSub2APIKey 删除上游 Sub2API 站点的指定 API Key。
func (s *PlatformService) DeleteSub2APIKey(session Session, keyID string) error {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return newRequestError(ErrorAuth, PlatformSub2API)
	}
	_, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/keys/"+keyID, requestOptions{
		AccessToken: session.AccessToken,
		TokenType:   session.TokenType,
		Method:      http.MethodDelete,
	})
	return err
}

// DeleteSub2APIAdminAccount 删除 admin 站点的指定转发账号。
func (s *PlatformService) DeleteSub2APIAdminAccount(session Session, accountID string) error {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return newRequestError(ErrorAuth, PlatformSub2API)
	}
	_, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/admin/accounts/"+accountID, requestOptions{
		AccessToken: session.AccessToken,
		TokenType:   session.TokenType,
		Method:      http.MethodDelete,
	})
	return err
}

// FetchAdminUsageStats 平台中性的管理员今日消费查询。
// sub2api 返回 actual_cost，new-api 返回 quota 并按 quotaPerUnit 换算为 USD。
func (s *PlatformService) FetchAdminUsageStats(session Session, startDate, endDate string) (float64, error) {
	switch session.Platform {
	case PlatformNewAPI:
		return s.fetchNewAPIAdminUsageStats(session, startDate, endDate)
	default:
		return s.FetchSub2APIAdminUsageStats(session, startDate, endDate)
	}
}

// fetchNewAPIAdminUsageStats 调用 new-api /api/log/self/stat 获取指定日期范围内的总消费 quota。
func (s *PlatformService) fetchNewAPIAdminUsageStats(session Session, startDate, endDate string) (float64, error) {
	if !session.IsAuthenticated() {
		return 0, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return 0, err
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return 0, err
	}
	startTS := start.Unix()
	endTS := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, 0, end.Location()).Unix()
	statURL := session.BaseURL + "/api/log/self/stat?type=2&start_timestamp=" + strconvInt(startTS) + "&end_timestamp=" + strconvInt(endTS)
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}
	response, err := s.httpClient.requestJSON(statURL, cookieOptions)
	if err != nil {
		return 0, err
	}
	data := dataRecord(response.Payload)
	quota := firstNumber(data, []string{"quota"})
	return quotaToUSDValueWithUnit(quota, session.QuotaPerUnit), nil
}

// FetchAdminSiteBalanceFiltered 平台中性的站点用户总余额统计。
func (s *PlatformService) FetchAdminSiteBalanceFiltered(session Session, filter BalanceFilter) (AdminSiteBalance, error) {
	switch session.Platform {
	case PlatformNewAPI:
		return s.fetchNewAPIAdminSiteBalanceFiltered(session, filter)
	default:
		return s.FetchSub2APIAdminSiteBalanceFiltered(session, filter)
	}
}

// fetchNewAPIAdminSiteBalanceFiltered 通过 new-api /api/user/ 分页获取用户列表，
// 排除 admin/root（role >= 10），汇总 quota 并按 quotaPerUnit 换算。
func (s *PlatformService) fetchNewAPIAdminSiteBalanceFiltered(session Session, filter BalanceFilter) (AdminSiteBalance, error) {
	if !session.IsAuthenticated() {
		return AdminSiteBalance{}, newRequestError(ErrorAuth, PlatformNewAPI)
	}

	const pageSize = 100
	const maxConcurrency = 5
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}

	firstURL := session.BaseURL + "/api/user/?p=1&page_size=" + strconvInt(int64(pageSize))
	firstResponse, err := s.httpClient.requestJSON(firstURL, cookieOptions)
	if err != nil {
		return AdminSiteBalance{}, err
	}

	firstData := dataRecord(firstResponse.Payload)
	firstItems := newAPIDataItems(firstResponse.Payload)
	if len(firstItems) == 0 {
		return AdminSiteBalance{Balance: 0}, nil
	}
	totalBalance := sumNewAPIFilteredQuotas(firstItems, filter, session.QuotaPerUnit)

	total := 0
	if t := firstNumber(firstData, []string{"total", "count"}); t != nil {
		total = int(*t)
	}
	if total <= pageSize {
		return AdminSiteBalance{Balance: totalBalance}, nil
	}

	totalPages := (total + pageSize - 1) / pageSize
	sem := make(chan struct{}, maxConcurrency)
	var mu sync.Mutex
	var wg sync.WaitGroup
	var fetchErr error

	for page := 2; page <= totalPages; page++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(page int) {
			defer wg.Done()
			defer func() { <-sem }()
			pageURL := session.BaseURL + "/api/user/?p=" + strconvInt(int64(page)) + "&page_size=" + strconvInt(int64(pageSize))
			response, err := s.httpClient.requestJSON(pageURL, cookieOptions)
			if err != nil {
				mu.Lock()
				if fetchErr == nil {
					fetchErr = err
				}
				mu.Unlock()
				return
			}
			items := newAPIDataItems(response.Payload)
			if len(items) == 0 {
				return
			}
			pageBalance := sumNewAPIFilteredQuotas(items, filter, session.QuotaPerUnit)
			mu.Lock()
			totalBalance += pageBalance
			mu.Unlock()
		}(page)
	}
	wg.Wait()
	if fetchErr != nil {
		return AdminSiteBalance{}, fetchErr
	}
	return AdminSiteBalance{Balance: totalBalance}, nil
}

// newAPIDataItems 提取 new-api 分页响应中的 data.items 数组。
func newAPIDataItems(payload any) []any {
	record, ok := payload.(map[string]any)
	if !ok {
		return nil
	}
	data, ok := record["data"].(map[string]any)
	if !ok {
		return nil
	}
	items, ok := data["items"].([]any)
	if !ok {
		return nil
	}
	return items
}

// sumNewAPIFilteredQuotas 对 new-api 用户列表按过滤条件汇总 quota。
// new-api role >= 10 为 admin/root，排除时使用数字比较而非字符串。
func sumNewAPIFilteredQuotas(users []any, filter BalanceFilter, quotaPerUnit float64) float64 {
	var sum float64
	for _, user := range users {
		if filter.ExcludeAdmin {
			role := firstNumber(user, []string{"role"})
			if role != nil && *role >= 10 {
				continue
			}
		}
		quota := firstNumber(user, []string{"quota"})
		if quota == nil {
			continue
		}
		usdBalance := quotaToUSDValueWithUnit(quota, quotaPerUnit)
		if balanceExcluded(usdBalance, filter.ExcludeBalances) {
			continue
		}
		sum += usdBalance
	}
	return sum
}

// FetchAdminGroups 平台中性的分组列表获取（用户可见分组）。
func (s *PlatformService) FetchAdminGroups(session Session) ([]GroupInfo, error) {
	switch session.Platform {
	case PlatformNewAPI:
		return s.fetchNewAPIAdminGroups(session)
	default:
		return s.FetchSub2APIAdminGroups(session)
	}
}

// fetchNewAPIAdminGroups 获取 new-api 的分组列表：合并 /api/user/self/groups 和 /api/pricing 数据。
func (s *PlatformService) fetchNewAPIAdminGroups(session Session) ([]GroupInfo, error) {
	if !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}
	groupsPayload, err := s.httpClient.requestJSON(session.BaseURL+"/api/user/self/groups", cookieOptions)
	if err != nil {
		groupsPayload, err = s.httpClient.requestJSON(session.BaseURL+"/api/user/groups", cookieOptions)
		if err != nil {
			return nil, err
		}
	}
	pricingPayload, err := s.httpClient.requestJSON(session.BaseURL+"/api/pricing", cookieOptions)
	if err != nil {
		log.Printf("new-api pricing request failed base_url=%s err=%v", session.BaseURL, err)
		pricingPayload = jsonResponse{Payload: map[string]any{}}
	}
	return newAPIGroups(groupsPayload.Payload, pricingPayload.Payload), nil
}

// FetchAdminGroupDailyStats 平台中性的分组今日用量统计。
// sub2api 复用 FetchSub2APIGroupDailyStats 的多级降级策略，new-api 复用 FetchNewAPIGroupDailyStats 的逐组查询。
func (s *PlatformService) FetchAdminGroupDailyStats(session Session, groups []GroupInfo) ([]GroupDailyStat, error) {
	switch session.Platform {
	case PlatformNewAPI:
		return s.FetchNewAPIGroupDailyStats(session, groups)
	default:
		return s.FetchSub2APIGroupDailyStats(session, groups)
	}
}

// FetchAdminAllGroups 平台中性的全量分组列表获取（包含管理端专有字段）。
func (s *PlatformService) FetchAdminAllGroups(session Session) ([]AdminGroupInfo, error) {
	switch session.Platform {
	case PlatformNewAPI:
		return s.fetchNewAPIAdminAllGroups(session)
	default:
		return s.FetchSub2APIAdminAllGroups(session)
	}
}

// fetchNewAPIAdminAllGroups 获取 new-api 全量分组列表。
// new-api 的 /api/group/ 返回纯分组名数组，结合 /api/user/self/groups 获取 ratio。
func (s *PlatformService) fetchNewAPIAdminAllGroups(session Session) ([]AdminGroupInfo, error) {
	if !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}
	// 获取分组名列表
	groupListPayload, err := s.httpClient.requestJSON(session.BaseURL+"/api/group/", cookieOptions)
	if err != nil {
		return nil, err
	}
	// 获取分组倍率
	groupsPayload, err := s.httpClient.requestJSON(session.BaseURL+"/api/user/self/groups", cookieOptions)
	if err != nil {
		groupsPayload = jsonResponse{Payload: map[string]any{}}
	}
	pricingPayload, err := s.httpClient.requestJSON(session.BaseURL+"/api/pricing", cookieOptions)
	if err != nil {
		pricingPayload = jsonResponse{Payload: map[string]any{}}
	}

	groupInfos := newAPIGroups(groupsPayload.Payload, pricingPayload.Payload)
	ratioMap := make(map[string]*float64, len(groupInfos))
	platformMap := make(map[string]*string, len(groupInfos))
	for _, g := range groupInfos {
		ratioMap[g.Name] = g.Multiplier
		platformMap[g.Name] = g.Platform
	}

	// new-api /api/group/ 返回 data: ["default", "vip", ...]
	var groupNames []string
	groupListData := dataRecord(groupListPayload.Payload)
	if arr, ok := groupListData["data"]; ok {
		if names, ok := arr.([]any); ok {
			for _, name := range names {
				if s, ok := name.(string); ok && strings.TrimSpace(s) != "" {
					groupNames = append(groupNames, s)
				}
			}
		}
	}
	// 回退：如果 /api/group/ 返回的不是直接数组，则使用 groups info
	if len(groupNames) == 0 {
		if rawData, ok := groupListPayload.Payload.(map[string]any); ok {
			if arr, ok := rawData["data"].([]any); ok {
				for _, name := range arr {
					if s, ok := name.(string); ok && strings.TrimSpace(s) != "" {
						groupNames = append(groupNames, s)
					}
				}
			}
		}
	}
	if len(groupNames) == 0 {
		for _, g := range groupInfos {
			groupNames = append(groupNames, g.Name)
		}
	}

	groups := make([]AdminGroupInfo, 0, len(groupNames))
	for _, name := range groupNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		rate := ratioMap[name]
		platform := ""
		if p := platformMap[name]; p != nil {
			platform = *p
		}
		groups = append(groups, AdminGroupInfo{
			ID:                name,
			Name:              name,
			Platform:          platform,
			Status:            "active",
			Multiplier:        rate,
			MultiplierDisplay: multiplier(rate),
		})
	}
	return groups, nil
}

func strconvInt(value int64) string {
	return strconv.FormatInt(value, 10)
}

// CreateNewAPIToken 在上游 new-api 站点创建 Token，回查 Token ID，获取完整 Key。
// new-api 创建 token 不返回 ID 和完整 key，因此：
//  1. 使用唯一 name 创建 token；
//  2. 按 name 搜索回查 token ID；
//  3. 调用 /api/token/:id/key 获取完整 key。
func (s *PlatformService) CreateNewAPIToken(session Session, name string, group string) (string, string, error) {
	if session.Platform != PlatformNewAPI {
		return "", "", newRequestError(ErrorAuth, PlatformNewAPI)
	}

	// 步骤 1：创建 token
	_, err := s.httpClient.requestJSON(session.BaseURL+"/api/token/", requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
		Method: http.MethodPost,
		Body: map[string]any{
			"name":                 name,
			"remain_quota":         0,
			"unlimited_quota":      true,
			"expired_time":         -1,
			"model_limits_enabled": false,
			"model_limits":         "",
			"allow_ips":            "",
			"group":                group,
			"cross_group_retry":    false,
		},
	})
	if err != nil {
		return "", "", err
	}

	// 步骤 2：按 name 搜索回查 token ID
	tokenID, err := s.searchNewAPITokenByName(session, name)
	if err != nil {
		return "", "", err
	}

	// 步骤 3：获取完整 key
	fullKey, err := s.FetchNewAPITokenKey(session, tokenID)
	if err != nil {
		return "", "", err
	}

	return tokenID, fullKey, nil
}

// searchNewAPITokenByName 通过分页查询 token 列表，按 name 精确匹配回查 token ID。
func (s *PlatformService) searchNewAPITokenByName(session Session, name string) (string, error) {
	for page := 1; page <= 10; page++ {
		endpoint := session.BaseURL + "/api/token/?p=" + strconv.Itoa(page) + "&page_size=100"
		response, err := s.httpClient.requestJSON(endpoint, requestOptions{
			Cookie: session.Cookie,
			UserID: session.UserID,
		})
		if err != nil {
			return "", err
		}
		items := dataArray(response.Payload)
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if safeString(record, "name") == name {
				id := groupID2(record)
				if id != "" {
					return id, nil
				}
			}
		}
		total, ok := paginationTotal(response.Payload)
		if ok && page*100 >= total {
			break
		}
	}
	return "", newRequestError(ErrorInvalidResponse, PlatformNewAPI)
}

// FetchNewAPITokenKey 调用 /api/token/:id/key 获取完整的 token key。
func (s *PlatformService) FetchNewAPITokenKey(session Session, tokenID string) (string, error) {
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/token/"+tokenID+"/key", requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
		Method: http.MethodPost,
	})
	if err != nil {
		return "", err
	}
	data := dataRecord(response.Payload)
	if key := firstString(data, []string{"key"}); key != nil && *key != "" {
		return *key, nil
	}
	return "", newRequestError(ErrorInvalidResponse, PlatformNewAPI)
}

// ListNewAPITokens 列出上游 new-api 站点的 token 列表。
// 返回与 Sub2APIKeyItem 相同的结构，便于前端统一使用。
func (s *PlatformService) ListNewAPITokens(session Session) ([]Sub2APIKeyItem, error) {
	if session.Platform != PlatformNewAPI {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/token/?p=1&page_size=100", requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
	})
	if err != nil {
		return nil, err
	}
	items := dataArray(response.Payload)
	tokens := make([]Sub2APIKeyItem, 0, len(items))
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		token := Sub2APIKeyItem{
			ID:   groupID2(record),
			Name: safeString(record, "name"),
		}
		if k := firstString(record, []string{"key"}); k != nil {
			token.Key = *k
		}
		if s := firstString(record, []string{"status"}); s != nil {
			token.Status = *s
		} else if statusNum := firstNumber(record, []string{"status"}); statusNum != nil {
			token.Status = strconv.FormatInt(int64(*statusNum), 10)
		}
		if g := firstString(record, []string{"group"}); g != nil {
			token.GroupID = *g
			token.GroupName = *g
		}
		tokens = append(tokens, token)
	}
	return tokens, nil
}

// DeleteNewAPIToken 删除上游 new-api 站点的指定 token。
func (s *PlatformService) DeleteNewAPIToken(session Session, tokenID string) error {
	if session.Platform != PlatformNewAPI {
		return newRequestError(ErrorAuth, PlatformNewAPI)
	}
	_, err := s.httpClient.requestJSON(session.BaseURL+"/api/token/"+tokenID, requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
		Method: http.MethodDelete,
	})
	return err
}

// CreateNewAPIChannel 在 admin new-api 站点创建 channel，回查 channel ID。
// new-api 创建 channel 不返回 ID，因此按唯一 name 搜索回查。
func (s *PlatformService) CreateNewAPIChannel(session Session, name string, baseURL string, key string, channelType int, groupIDs []string) (string, error) {
	if session.Platform != PlatformNewAPI {
		return "", newRequestError(ErrorAuth, PlatformNewAPI)
	}

	groupStr := strings.Join(groupIDs, ",")

	// 步骤 1：创建 channel
	_, err := s.httpClient.requestJSON(session.BaseURL+"/api/channel/", requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
		Method: http.MethodPost,
		Body: map[string]any{
			"mode":                            "single",
			"multi_key_mode":                  "",
			"batch_add_set_key_prefix_2_name": false,
			"channel": map[string]any{
				"type":            channelType,
				"key":             key,
				"name":            name,
				"base_url":        baseURL,
				"models":          "",
				"group":           groupStr,
				"status":          1,
				"weight":          0,
				"priority":        0,
				"auto_ban":        1,
				"model_mapping":   "",
				"tag":             "",
				"setting":         "",
				"param_override":  "",
				"header_override": "",
			},
		},
	})
	if err != nil {
		return "", err
	}

	// 步骤 2：按 name 搜索回查 channel ID
	channelID, err := s.searchNewAPIChannelByName(session, name)
	if err != nil {
		return "", err
	}
	return channelID, nil
}

// searchNewAPIChannelByName 通过分页查询 channel 列表，按 name 精确匹配回查 channel ID。
func (s *PlatformService) searchNewAPIChannelByName(session Session, name string) (string, error) {
	for page := 1; page <= 10; page++ {
		endpoint := session.BaseURL + "/api/channel/?p=" + strconv.Itoa(page) + "&page_size=100"
		response, err := s.httpClient.requestJSON(endpoint, requestOptions{
			Cookie: session.Cookie,
			UserID: session.UserID,
		})
		if err != nil {
			return "", err
		}
		items := dataArray(response.Payload)
		if len(items) == 0 {
			break
		}
		for _, item := range items {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if safeString(record, "name") == name {
				id := groupID2(record)
				if id != "" {
					return id, nil
				}
			}
		}
		total, ok := paginationTotal(response.Payload)
		if ok && page*100 >= total {
			break
		}
	}
	return "", newRequestError(ErrorInvalidResponse, PlatformNewAPI)
}

// DeleteNewAPIChannel 删除 admin new-api 站点的指定 channel。
func (s *PlatformService) DeleteNewAPIChannel(session Session, channelID string) error {
	if session.Platform != PlatformNewAPI {
		return newRequestError(ErrorAuth, PlatformNewAPI)
	}
	_, err := s.httpClient.requestJSON(session.BaseURL+"/api/channel/"+channelID, requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
		Method: http.MethodDelete,
	})
	return err
}

// UpdateAdminGroupMultiplier 平台中性地更新 admin 站点指定分组的倍率。
// sub2api 使用 GET+PUT /api/v1/admin/groups/{id}，new-api 使用 GET+PUT /api/option/ 修改 GroupRatio。
func (s *PlatformService) UpdateAdminGroupMultiplier(session Session, group AdminGroupInfo, multiplier float64) error {
	switch session.Platform {
	case PlatformNewAPI:
		return s.updateNewAPIGroupRatio(session, group.Name, multiplier)
	default:
		return s.updateSub2APIAdminGroupMultiplier(session, group.ID, multiplier)
	}
}

// updateSub2APIAdminGroupMultiplier 通过 GET+PUT /api/v1/admin/groups/{id} 更新 sub2api 分组倍率。
// 先 GET 原始详情，复制必要字段，仅替换 rate_multiplier 后 PUT 回去，避免覆盖其他字段。
func (s *PlatformService) updateSub2APIAdminGroupMultiplier(session Session, groupID string, multiplier float64) error {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return newRequestError(ErrorAuth, PlatformSub2API)
	}
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}

	// GET 分组详情
	getURL := session.BaseURL + "/api/v1/admin/groups/" + groupID
	response, err := s.httpClient.requestJSON(getURL, authOptions)
	if err != nil {
		return err
	}
	data := dataRecord(response.Payload)

	// 从原始响应中复制必要字段，仅替换 rate_multiplier
	payload := map[string]any{
		"rate_multiplier": multiplier,
	}
	for _, key := range []string{"name", "description", "platform", "is_exclusive", "status", "subscription_type"} {
		if v, ok := data[key]; ok {
			payload[key] = v
		}
	}

	// PUT 更新
	_, err = s.httpClient.requestJSON(getURL, requestOptions{
		AccessToken: session.AccessToken,
		TokenType:   session.TokenType,
		Method:      http.MethodPut,
		Body:        payload,
	})
	return err
}

// updateNewAPIGroupRatio 通过 GET+PUT /api/option/ 更新 new-api 的 GroupRatio。
// GroupRatio 是一个 JSON 字符串形式的 map[string]float64，修改指定分组的倍率后整体 PUT 回去。
// 需要 RootAuth 权限，权限不足时返回错误（调用方应记录并跳过）。
func (s *PlatformService) updateNewAPIGroupRatio(session Session, groupName string, multiplier float64) error {
	if !session.IsAuthenticated() {
		return newRequestError(ErrorAuth, PlatformNewAPI)
	}
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}

	// GET 当前 options
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/option/", cookieOptions)
	if err != nil {
		return err
	}
	data := dataRecord(response.Payload)

	// 找到 GroupRatio 的当前值并解析
	ratioMap := map[string]float64{}
	if ratioStr := firstString(data, []string{"GroupRatio"}); ratioStr != nil && *ratioStr != "" {
		if jsonErr := jsonUnmarshalString(*ratioStr, &ratioMap); jsonErr != nil {
			log.Printf("[auto-pricing] new-api GroupRatio parse failed base_url=%s err=%v", session.BaseURL, jsonErr)
			return jsonErr
		}
	}

	// 更新目标分组的倍率
	ratioMap[groupName] = multiplier

	// 序列化为 JSON 字符串
	ratioBytes, err := jsonMarshal(ratioMap)
	if err != nil {
		return err
	}

	// PUT 更新 GroupRatio，需要 root option 权限（RootAuth）
	_, err = s.httpClient.requestJSON(session.BaseURL+"/api/option/", requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
		Method: http.MethodPut,
		Body: map[string]any{
			"key":   "GroupRatio",
			"value": string(ratioBytes),
		},
	})
	if err != nil {
		if reqErr, ok := err.(*RequestError); ok && (reqErr.MessageKey == ErrorAuth || reqErr.MessageKey == ErrorRequest) {
			return fmt.Errorf("new-api requires root option permission to update GroupRatio: %w", err)
		}
		return err
	}
	return nil
}

// UpdateNewAPIChannelWeightStatus 更新 admin new-api 站点指定 channel 的权重和启用状态。
// 供 connection_health 模块的自动降级/恢复动作使用：降级时 weight=0/status=2（禁用），
// 恢复时按策略的 recovery_step_percent 逐步调高 weight 并在权重恢复到位后把 status 设回 1。
// 采用与 updateSub2APIAdminGroupMultiplier 一致的 GET+PUT-merge 手法：先 GET 单个 channel
// 详情复制原始字段，仅替换 weight/status 后整体 PUT 回去，避免覆盖 key/base_url/group 等字段。
// 若 GET 失败（接口不存在、权限不足或 channel 已被删除），直接把错误透传给调用方，
// 调用方应记录为 remote_action=unsupported 并停止后续动作，不做任何猜测性的 PUT 请求。
func (s *PlatformService) UpdateNewAPIChannelWeightStatus(session Session, channelID string, weight int, status int) error {
	if session.Platform != PlatformNewAPI {
		return newRequestError(ErrorAuth, PlatformNewAPI)
	}
	cookieOptions := requestOptions{Cookie: session.Cookie, UserID: session.UserID}

	getURL := session.BaseURL + "/api/channel/" + channelID
	response, err := s.httpClient.requestJSON(getURL, cookieOptions)
	if err != nil {
		return err
	}
	data := dataRecord(response.Payload)
	if len(data) == 0 {
		return newRequestError(ErrorInvalidResponse, PlatformNewAPI)
	}

	channel := map[string]any{
		"weight": weight,
		"status": status,
	}
	for _, key := range []string{"id", "type", "key", "name", "base_url", "models", "group", "priority", "auto_ban", "model_mapping", "tag", "setting", "param_override", "header_override"} {
		if v, ok := data[key]; ok {
			channel[key] = v
		}
	}
	if _, ok := channel["id"]; !ok {
		// GET 响应缺少 id 字段时兜底：new-api channel id 是数值型，按数字解析后回填，
		// 避免把字符串 channelID 直接序列化成 JSON 字符串导致后端 ShouldBindJSON 绑定失败。
		if idNum, err := strconv.ParseInt(channelID, 10, 64); err == nil {
			channel["id"] = idNum
		} else {
			channel["id"] = channelID
		}
	}

	// new-api 的 UpdateChannel 用 ShouldBindJSON(&PatchChannel) 直接绑定请求体，
	// 字段必须在 JSON 顶层，不能像 CreateNewAPIChannel 那样包一层 "channel"。
	_, err = s.httpClient.requestJSON(session.BaseURL+"/api/channel/", requestOptions{
		Cookie: session.Cookie,
		UserID: session.UserID,
		Method: http.MethodPut,
		Body:   channel,
	})
	return err
}

// UpdateSub2APIAdminAccountStatus 通过 GET+PUT /api/v1/admin/accounts/:id 更新 sub2api 转发账号的
// 启用状态（"active"/"inactive"），供 connection_health 模块的自动降级/恢复动作使用。
// 采用与 UpdateNewAPIChannelWeightStatus / updateSub2APIAdminGroupMultiplier 一致的 GET+PUT-merge
// 手法：先 GET 账号详情复制原始字段，仅替换 status 后整体 PUT 回去，避免覆盖 credentials/
// group_ids/priority/concurrency/rate_multiplier/load_factor 等字段（PUT-merge 而不是直接发送
// {"status": "..."}，因为尚未确认 sub2api UpdateAccountRequest 支持 partial patch 语义）。
// 若 GET 失败（接口不存在、权限不足或账号已被删除），直接把错误透传给调用方，调用方应记录为
// remote_action=unsupported 并停止后续动作，不做任何猜测性的 PUT 请求。
// 明文 credentials 只是原样透传（从 GET 响应直接搬到 PUT 请求体），本方法不解析、不记录其内容。
func (s *PlatformService) UpdateSub2APIAdminAccountStatus(session Session, accountID string, status string) error {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return newRequestError(ErrorAuth, PlatformSub2API)
	}
	if strings.TrimSpace(accountID) == "" {
		return newRequestError(ErrorInvalidResponse, PlatformSub2API)
	}
	authOptions := requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType}

	getURL := session.BaseURL + "/api/v1/admin/accounts/" + url.PathEscape(accountID)
	response, err := s.httpClient.requestJSON(getURL, authOptions)
	if err != nil {
		return err
	}
	data := dataRecord(response.Payload)
	if len(data) == 0 {
		return newRequestError(ErrorInvalidResponse, PlatformSub2API)
	}

	payload := map[string]any{
		"status": status,
	}
	for _, key := range []string{
		"name", "notes", "type", "credentials", "extra", "proxy_id",
		"concurrency", "priority", "rate_multiplier", "load_factor",
		"expires_at", "auto_pause_on_expired", "confirm_mixed_channel_risk",
	} {
		if v, ok := data[key]; ok {
			payload[key] = v
		}
	}
	// group_ids 需要单独解析并保留原始元素类型（数字仍是数字）：sub2api 线上已确认 PUT 传
	// 字符串化的 group_ids（如 ["50"]）会返回 400，必须和 GET 响应的原始类型一致（如 [50]）。
	// 解析不到任何分组 ID 时完全不带这个字段，避免用空数组覆盖账号已有的分组绑定。
	if groupIDs := resolveSub2APIAccountGroupIDsForPayload(data); len(groupIDs) > 0 {
		payload["group_ids"] = groupIDs
	}

	_, err = s.httpClient.requestJSON(getURL, requestOptions{
		AccessToken: session.AccessToken,
		TokenType:   session.TokenType,
		Method:      http.MethodPut,
		Body:        payload,
	})
	return err
}

// sub2APIUserIDKeys/sub2APIUserCreatedAtKeys/sub2APIUserLastUsedAtKeys/sub2APIBalanceHistoryTimeKeys
// 是解析 Sub2API admin 用户资料相关字段时兼容的候选 key 列表（snake_case / camelCase）。
var (
	sub2APIUserIDKeys             = []string{"id", "user_id", "userId"}
	sub2APIUserCreatedAtKeys      = []string{"created_at", "createdAt", "registered_at", "registeredAt"}
	sub2APIUserLastUsedAtKeys     = []string{"last_used_at", "lastUsedAt"}
	sub2APIBalanceHistoryTimeKeys = []string{"created_at", "createdAt"}
)

// FetchSub2APIAdminUser 通过 GET /api/v1/admin/users/:id 查询指定 Sub2API 用户的只读资料，
// 仅用于工单模块"Sub2API 用户资料"弹窗展示，绝不写入/修改 Sub2API 数据。调用方必须传入
// 当前 TransitHub workspace 的 admin session（已经过 RequireSession 刷新并校验过 admin 身份），
// 不能使用 iframe 用户自己的 token 查询别的用户的资料。
func (s *PlatformService) FetchSub2APIAdminUser(session Session, userID string) (Sub2APIAdminUser, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return Sub2APIAdminUser{}, newRequestError(ErrorAuth, PlatformSub2API)
	}
	trimmedID := strings.TrimSpace(userID)
	if trimmedID == "" {
		return Sub2APIAdminUser{}, newRequestError(ErrorInvalidResponse, PlatformSub2API)
	}
	requestURL := session.BaseURL + "/api/v1/admin/users/" + url.PathEscape(trimmedID)
	response, err := s.httpClient.requestJSON(requestURL, requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType})
	if err != nil {
		return Sub2APIAdminUser{}, err
	}
	return parseSub2APIAdminUser(dataRecord(response.Payload)), nil
}

// FetchSub2APIAdminUserBalanceHistory 通过 GET /api/v1/admin/users/:id/balance-history 查询
// 指定 Sub2API 用户的余额/充值历史。page/pageSize 非法时分别回退到 1/20；codeType 为空时不带
// type 查询参数。
func (s *PlatformService) FetchSub2APIAdminUserBalanceHistory(session Session, userID string, page int, pageSize int, codeType string) (Sub2APIUserBalanceHistory, error) {
	if session.Platform != PlatformSub2API || strings.TrimSpace(session.AccessToken) == "" {
		return Sub2APIUserBalanceHistory{}, newRequestError(ErrorAuth, PlatformSub2API)
	}
	trimmedID := strings.TrimSpace(userID)
	if trimmedID == "" {
		return Sub2APIUserBalanceHistory{}, newRequestError(ErrorInvalidResponse, PlatformSub2API)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	requestURL := session.BaseURL + "/api/v1/admin/users/" + url.PathEscape(trimmedID) +
		"/balance-history?page=" + strconvInt(int64(page)) + "&page_size=" + strconvInt(int64(pageSize))
	if trimmedType := strings.TrimSpace(codeType); trimmedType != "" {
		requestURL += "&type=" + url.QueryEscape(trimmedType)
	}
	response, err := s.httpClient.requestJSON(requestURL, requestOptions{AccessToken: session.AccessToken, TokenType: session.TokenType})
	if err != nil {
		return Sub2APIUserBalanceHistory{}, err
	}
	history := Sub2APIUserBalanceHistory{
		TotalRecharged: firstNumber(dataRecord(response.Payload), []string{"total_recharged", "totalRecharged"}),
	}
	if total, ok := paginationTotal(response.Payload); ok {
		history.Total = total
	}
	for _, item := range dataArray(response.Payload) {
		history.Items = append(history.Items, parseSub2APIBalanceHistoryItem(item))
	}
	return history, nil
}

// parseSub2APIAdminUser 从 GET /api/v1/admin/users/:id 的响应记录中解析只读字段，兼容
// snake_case / camelCase；字段缺失或类型不匹配时保持零值，由调用方按需要降级展示。
func parseSub2APIAdminUser(value any) Sub2APIAdminUser {
	user := Sub2APIAdminUser{ID: firstStringy(value, sub2APIUserIDKeys)}
	if email := firstString(value, []string{"email"}); email != nil {
		user.Email = *email
	}
	if username := firstString(value, []string{"username"}); username != nil {
		user.Username = *username
	}
	if role := firstString(value, []string{"role"}); role != nil {
		user.Role = *role
	}
	user.Status = firstStringy(value, []string{"status"})
	user.Balance = firstNumber(value, []string{"balance"})
	user.FrozenBalance = firstNumber(value, []string{"frozen_balance", "frozenBalance"})
	if concurrency := firstNumber(value, []string{"concurrency"}); concurrency != nil {
		v := int(*concurrency)
		user.Concurrency = &v
	}
	if rpmLimit := firstNumber(value, []string{"rpm_limit", "rpmLimit"}); rpmLimit != nil {
		v := int(*rpmLimit)
		user.RPMLimit = &v
	}
	user.CreatedAt = parseFlexibleTime(firstAny(value, sub2APIUserCreatedAtKeys))
	user.LastUsedAt = parseFlexibleTime(firstAny(value, sub2APIUserLastUsedAtKeys))
	return user
}

// parseSub2APIBalanceHistoryItem 解析余额/充值历史单条记录，兼容 snake_case / camelCase。
func parseSub2APIBalanceHistoryItem(value any) Sub2APIBalanceHistoryItem {
	item := Sub2APIBalanceHistoryItem{ID: firstStringy(value, []string{"id"})}
	item.Type = firstStringy(value, []string{"type", "code_type", "codeType"})
	item.Amount = firstNumber(value, []string{"amount", "balance", "value"})
	if note := firstString(value, []string{"notes", "note", "remark"}); note != nil {
		item.Note = *note
	}
	item.CreatedAt = parseFlexibleTime(firstAny(value, sub2APIBalanceHistoryTimeKeys))
	return item
}
