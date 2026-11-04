package upstream

import (
	"errors"
	"net/url"
	"strconv"
	"strings"

	"transithub/backend/internal/shared/bizclock"
)

// 利润分析用到的自己站点（sub2api / new-api）只读接口：路由清单与按路由、分组的营收。

// OwnSiteRoute 是自己站点上的一条转发路由：sub2api 的上游账号或 new-api 的渠道。不含任何密钥。
type OwnSiteRoute struct {
	ID      string
	Name    string
	Kind    string
	Status  string
	BaseURL string
	Groups  []string
}

// RouteGroupRevenue 是一条路由在某个分组下的营收（站点额度，美元口径）。
type RouteGroupRevenue struct {
	RouteID  string
	Group    string
	Revenue  float64
	Requests int64
}

const (
	ownSiteRoutesPageSize = 100
	ownSiteRoutesMaxPages = 50
)

// routeBaseURLFields 是 sub2api 账号 credentials 中可能存放上游地址的字段（只读这些字段，不读取任何密钥）。
var routeBaseURLFields = []string{"base_url", "baseUrl", "endpoint", "api_base", "apiBase", "url"}

// ErrFlowUnsupported 表示 new-api 站点没有 /api/data/flow（旧版本）或数据看板未开启，需要按渠道回退查询。
var ErrFlowUnsupported = errors.New("new-api flow data unsupported")

// ListOwnSiteRoutes 读取自己站点的全部路由：sub2api 读 /api/v1/admin/accounts（credentials 中只取上游地址，
// 分组 ID 换成分组名）；new-api 读 /api/channel/（服务端不返回 key）。
func (s *PlatformService) ListOwnSiteRoutes(session Session) ([]OwnSiteRoute, error) {
	switch session.Platform {
	case PlatformSub2API:
		return s.listSub2APIRoutes(session)
	case PlatformNewAPI:
		return s.listNewAPIRoutes(session)
	default:
		return nil, newRequestError(ErrorAuth, session.Platform)
	}
}

func (s *PlatformService) listSub2APIRoutes(session Session) ([]OwnSiteRoute, error) {
	if !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	groupNames := map[string]string{}
	groups, err := s.FetchSub2APIAdminAllGroups(session)
	if err != nil {
		return nil, err
	}
	for _, group := range groups {
		groupNames[group.ID] = group.Name
	}
	authOptions := adminAuthOptions(session)
	routes := make([]OwnSiteRoute, 0)
	seen := map[string]struct{}{}
	for page := 1; page <= ownSiteRoutesMaxPages; page++ {
		pageURL := session.BaseURL + "/api/v1/admin/accounts?page=" + strconv.Itoa(page) + "&page_size=" + strconv.Itoa(ownSiteRoutesPageSize)
		response, err := s.httpClient.requestJSON(pageURL, authOptions)
		if err != nil {
			return nil, err
		}
		items := dataArray(response.Payload)
		added := 0
		for _, item := range items {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			route := OwnSiteRoute{
				ID:     groupID2(record),
				Name:   safeString(record, "name"),
				Kind:   firstStringy(record, []string{"type"}),
				Status: firstStringy(record, []string{"status"}),
				Groups: sub2APIAccountGroupNames(record, groupNames),
			}
			if credentials, ok := record["credentials"].(map[string]any); ok {
				if value := firstString(credentials, routeBaseURLFields); value != nil {
					route.BaseURL = strings.TrimSpace(*value)
				}
			}
			if route.ID == "" {
				continue
			}
			if _, ok := seen[route.ID]; ok {
				continue
			}
			seen[route.ID] = struct{}{}
			routes = append(routes, route)
			added++
		}
		total, hasTotal := paginationTotal(response.Payload)
		// 站点可能把每页条数限制得比请求的小：按已收集的数量判断是否取完，没有总数时取到空页或没有新增为止。
		if len(items) == 0 || added == 0 || (hasTotal && len(routes) >= total) {
			break
		}
	}
	return routes, nil
}

// sub2APIAccountGroupNames 读取账号所属分组名：优先 groups[].name，其次 account_groups[].group.name，最后用 group_ids 查名称。
func sub2APIAccountGroupNames(record map[string]any, groupNames map[string]string) []string {
	names := make([]string, 0)
	seen := map[string]struct{}{}
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		names = append(names, name)
	}
	if groups, ok := record["groups"].([]any); ok {
		for _, group := range groups {
			if value := firstString(group, []string{"name"}); value != nil {
				add(*value)
			}
		}
	}
	if len(names) == 0 {
		if accountGroups, ok := record["account_groups"].([]any); ok {
			for _, item := range accountGroups {
				if entry, ok := item.(map[string]any); ok {
					if value := firstString(entry["group"], []string{"name"}); value != nil {
						add(*value)
					} else if id := firstStringy(entry, []string{"group_id"}); id != "" {
						add(groupNames[id])
					}
				}
			}
		}
	}
	if len(names) == 0 {
		if ids, ok := record["group_ids"].([]any); ok {
			for _, id := range ids {
				switch typed := id.(type) {
				case float64:
					add(groupNames[strconv.FormatInt(int64(typed), 10)])
				case string:
					add(groupNames[strings.TrimSpace(typed)])
				}
			}
		}
	}
	return names
}

func (s *PlatformService) listNewAPIRoutes(session Session) ([]OwnSiteRoute, error) {
	if !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	routes := make([]OwnSiteRoute, 0)
	seen := map[string]struct{}{}
	for page := 1; page <= ownSiteRoutesMaxPages; page++ {
		response, err := s.httpClient.requestJSON(session.BaseURL+"/api/channel/?p="+strconv.Itoa(page)+"&page_size="+strconv.Itoa(ownSiteRoutesPageSize), newAPIAuthOptions(session))
		if err != nil {
			return nil, err
		}
		items := dataArray(response.Payload)
		added := 0
		for _, item := range items {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			route := OwnSiteRoute{
				ID:     groupID2(record),
				Name:   safeString(record, "name"),
				Kind:   firstStringy(record, []string{"type"}),
				Status: firstStringy(record, []string{"status"}),
			}
			if route.ID == "" {
				continue
			}
			if _, ok := seen[route.ID]; ok {
				continue
			}
			seen[route.ID] = struct{}{}
			if value := firstString(record, []string{"base_url", "baseUrl"}); value != nil {
				route.BaseURL = strings.TrimSpace(*value)
			}
			if raw := firstString(record, []string{"group"}); raw != nil {
				for _, part := range strings.Split(*raw, ",") {
					if trimmed := strings.TrimSpace(part); trimmed != "" {
						route.Groups = append(route.Groups, trimmed)
					}
				}
			}
			routes = append(routes, route)
			added++
		}
		total, hasTotal := paginationTotal(response.Payload)
		// 不认识分页参数的旧版本会每页都返回同样的数据：没有新增就停止。
		if len(items) == 0 || added == 0 || (hasTotal && len(routes) >= total) {
			break
		}
	}
	return routes, nil
}

// FetchSub2APIAccountGroupRevenue 读取 sub2api 某个账号在某一天（北京时间）各分组的营收：
// GET /api/v1/admin/dashboard/groups?start_date=D&end_date=D&timezone=…&account_id=…，取 actual_cost（用户实际扣费）。
// 账号统计和趋势接口按数据库会话时区标注日期，只有这个接口按传入的时区切分日界，因此逐日查询。
func (s *PlatformService) FetchSub2APIAccountGroupRevenue(session Session, accountID string, date string) ([]RouteGroupRevenue, error) {
	if session.Platform != PlatformSub2API || !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	query := url.Values{}
	query.Set("start_date", date)
	query.Set("end_date", date)
	query.Set("timezone", bizclock.Name)
	query.Set("account_id", accountID)
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/admin/dashboard/groups?"+query.Encode(), adminAuthOptions(session))
	if err != nil {
		return nil, err
	}
	rows := make([]RouteGroupRevenue, 0)
	for _, item := range dataArray(response.Payload) {
		revenue := firstNumber(item, []string{"actual_cost", "actualCost"})
		if revenue == nil {
			continue
		}
		requests := firstNumber(item, []string{"requests"})
		row := RouteGroupRevenue{RouteID: accountID, Group: firstStringy(item, []string{"group_name", "groupName"}), Revenue: *revenue}
		if requests != nil {
			row.Requests = int64(*requests)
		}
		if row.Revenue == 0 && row.Requests == 0 {
			continue
		}
		rows = append(rows, row)
	}
	return rows, nil
}

// FetchNewAPIFlowRevenue 读取 new-api 在 [start, end]（Unix 秒，含两端）内按渠道、分组汇总的营收：
// GET /api/data/flow（需要管理员权限，数据来自按小时汇总的 quota_data）。旧版本没有该接口时返回 ErrFlowUnsupported。
func (s *PlatformService) FetchNewAPIFlowRevenue(session Session, start int64, end int64) ([]RouteGroupRevenue, error) {
	if session.Platform != PlatformNewAPI || !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	query := url.Values{}
	query.Set("start_timestamp", strconvInt(start))
	query.Set("end_timestamp", strconvInt(end))
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/data/flow?"+query.Encode(), newAPIAuthOptions(session))
	if err != nil {
		var requestErr *RequestError
		if errors.As(err, &requestErr) && (requestErr.StatusCode == 404 || requestErr.MessageKey == ErrorInvalidResponse) {
			return nil, ErrFlowUnsupported
		}
		return nil, err
	}
	record, ok := response.Payload.(map[string]any)
	if !ok {
		return nil, ErrFlowUnsupported
	}
	items, ok := record["data"].([]any)
	if !ok {
		return nil, ErrFlowUnsupported
	}
	type key struct{ route, group string }
	totals := map[key]*RouteGroupRevenue{}
	order := make([]key, 0)
	for _, item := range items {
		quota := firstNumber(item, []string{"quota"})
		if quota == nil {
			continue
		}
		k := key{route: firstStringy(item, []string{"channel_id", "channelId"}), group: firstStringy(item, []string{"use_group", "useGroup"})}
		if k.route == "0" {
			k.route = ""
		}
		row, exists := totals[k]
		if !exists {
			row = &RouteGroupRevenue{RouteID: k.route, Group: k.group}
			totals[k] = row
			order = append(order, k)
		}
		row.Revenue += quotaToUSDValueWithUnit(quota, session.QuotaPerUnit)
		if count := firstNumber(item, []string{"count"}); count != nil {
			row.Requests += int64(*count)
		}
	}
	rows := make([]RouteGroupRevenue, 0, len(order))
	for _, k := range order {
		rows = append(rows, *totals[k])
	}
	return rows, nil
}

// FetchNewAPIChannelRevenue 读取 new-api 某个渠道（可选分组）在 [start, end] 内的营收：GET /api/log/stat?type=2&channel=…&group=…。
// 用于没有 /api/data/flow 的旧版本。
func (s *PlatformService) FetchNewAPIChannelRevenue(session Session, channelID string, group string, start int64, end int64) (float64, error) {
	if session.Platform != PlatformNewAPI || !session.IsAuthenticated() {
		return 0, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	filters := url.Values{}
	filters.Set("channel", channelID)
	if strings.TrimSpace(group) != "" {
		filters.Set("group", group)
	}
	return s.fetchNewAPIConsumeUSD(session, newAPISiteLogStatPath, start, end, filters)
}
