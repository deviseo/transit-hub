package upstream

import (
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// AdminGroupAccountInfo 是「某个 admin 分组下的账号(sub2api) / 渠道(new-api)」的平台中性信息，
// 供 connection_health 分组健康主列表的账号弹窗展示。
//
// 安全约束：这里只保留展示所需的基础字段与探活策略相关字段，绝不包含 credentials / key /
// token / cookie 等敏感明文——上游响应里的敏感字段在解析阶段就被丢弃，不会进入本结构。
//
// 字段可空性：不同平台/不同上游版本返回的字段并不一致，凡是「缺省时应展示为占位符」的
// 数值/布尔字段一律用指针，nil 表示上游未提供，由前端决定展示 "-" 还是隐藏，避免把
// 「上游没给」误当成「值为 0」。
type AdminGroupAccountInfo struct {
	ID             string     // sub2api account id / new-api channel id
	Name           string     // 账号或渠道名称
	Platform       string     // 上游平台标识（openai / anthropic / ...），可能为空
	Type           string     // sub2api 账号类型 / new-api channel 类型（数值转字符串）
	Status         string     // 状态（字符串或数值转字符串）
	Priority       *int       // 优先级
	Concurrency    *int       // 并发（sub2api）
	RateMultiplier *float64   // Sub2API admin 转发账号记录自身的 rate_multiplier，不代表上游 API Key 所属分组倍率。
	LoadFactor     *int       // 负载因子（sub2api）
	Weight         *int       // 权重（仅 new-api channel 有；sub2api 为 nil）
	Models         string     // 模型列表（new-api channel.models 等）
	GroupIDs       []string   // 所属分组 ID/名称列表
	Schedulable    *bool      // 是否可调度（sub2api）
	UpdatedAt      *time.Time // 上游账号记录最后更新时间；用于区分本地用户动作与后续主站直接修改
	// BaseURL 是 new-api channel 转发到的上游 provider 地址（channel.base_url）。
	// 独立探活需要用它 + channel key 直接对上游发起 OpenAI 兼容请求。sub2api 账号在列表阶段
	// 拿不到 base_url，探活前再从单账号导出凭据里解析，故此处可能为空。
	BaseURL string
}

// ListAdminGroupAccounts 平台中性地读取某个 admin 分组下的账号/渠道列表。
// sub2api 走 /api/v1/admin/accounts?group=<groupID>，new-api 走 channel 查询。
// 返回的每个条目都不含敏感字段。
func (s *PlatformService) ListAdminGroupAccounts(session Session, group AdminGroupInfo) ([]AdminGroupAccountInfo, error) {
	switch session.Platform {
	case PlatformNewAPI:
		return s.listNewAPIGroupChannels(session, group)
	default:
		return s.listSub2APIGroupAccounts(session, group)
	}
}

// listSub2APIGroupAccounts 分页拉取 sub2api 某分组下的账号。
// 注意 query 参数是 group=<分组ID>（不是 group_id）。逐页拉取直到没有下一页或达到 total。
func (s *PlatformService) listSub2APIGroupAccounts(session Session, group AdminGroupInfo) ([]AdminGroupAccountInfo, error) {
	if session.Platform != PlatformSub2API || !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	if strings.TrimSpace(group.ID) == "" {
		return []AdminGroupAccountInfo{}, nil
	}
	authOptions := adminAuthOptions(session)

	const pageSize = 100
	const maxPages = 100 // 安全上限，防止上游分页字段异常导致死循环
	accounts := make([]AdminGroupAccountInfo, 0)
	for page := 1; page <= maxPages; page++ {
		pageURL := session.BaseURL + "/api/v1/admin/accounts?group=" + url.QueryEscape(group.ID) +
			"&page=" + strconvInt(int64(page)) + "&page_size=" + strconvInt(pageSize)
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
			accounts = append(accounts, parseSub2APIAccount(record))
		}
		total, hasTotal := paginationTotal(response.Payload)
		if hasTotal && page*pageSize >= total {
			break
		}
		if !hasTotal && len(items) < pageSize {
			break
		}
	}
	return accounts, nil
}

// parseSub2APIAccount 把 sub2api 账号原始记录解析为平台中性结构，主动丢弃 credentials 等敏感字段。
func parseSub2APIAccount(record map[string]any) AdminGroupAccountInfo {
	account := AdminGroupAccountInfo{
		ID:             groupID2(record),
		Name:           safeString(record, "name"),
		Type:           stringOrNumberField(record, []string{"type"}),
		Status:         stringOrNumberField(record, []string{"status"}),
		Priority:       firstInt(record, []string{"priority"}),
		Concurrency:    firstInt(record, []string{"concurrency"}),
		RateMultiplier: firstNumber(record, []string{"rate_multiplier", "rateMultiplier"}),
		LoadFactor:     firstInt(record, []string{"load_factor", "loadFactor"}),
		GroupIDs:       parseGroupIDList(record),
		Schedulable:    firstBoolValue(record, []string{"schedulable"}),
		UpdatedAt:      parseFlexibleTime(firstAny(record, []string{"updated_at", "updatedAt"})),
	}
	if p := firstString(record, []string{"platform"}); p != nil {
		account.Platform = *p
	}
	if m := firstString(record, []string{"models"}); m != nil {
		account.Models = *m
	}
	return account
}

// listNewAPIGroupChannels 读取 new-api 某分组下的 channel 列表。
// 优先使用 /api/channel/search?group=<分组名>（server 端已按分组过滤，兼容较老部署也普遍支持）；
// search 失败时兜底 /api/channel/ 分页拉取后在本地按「逗号分组精确匹配」过滤。
func (s *PlatformService) listNewAPIGroupChannels(session Session, group AdminGroupInfo) ([]AdminGroupAccountInfo, error) {
	if session.Platform != PlatformNewAPI || !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	groupName := strings.TrimSpace(group.Name)
	if groupName == "" {
		return []AdminGroupAccountInfo{}, nil
	}

	channels, err := s.searchNewAPIGroupChannels(session, groupName)
	if err == nil {
		return channels, nil
	}
	log.Printf("[connection-health] new-api /api/channel/search 拉取失败，回退 /api/channel/ 本地过滤 base_url=%s group=%s err=%v", session.BaseURL, groupName, err)
	return s.listNewAPIChannelsWithLocalFilter(session, groupName)
}

// searchNewAPIGroupChannels 通过 /api/channel/search?group= 分页读取指定分组的 channel。
func (s *PlatformService) searchNewAPIGroupChannels(session Session, groupName string) ([]AdminGroupAccountInfo, error) {
	cookieOptions := newAPIAuthOptions(session)
	const pageSize = 100
	const maxPages = 100
	channels := make([]AdminGroupAccountInfo, 0)
	for page := 1; page <= maxPages; page++ {
		pageURL := session.BaseURL + "/api/channel/search?group=" + url.QueryEscape(groupName) +
			"&p=" + strconvInt(int64(page)) + "&page_size=" + strconvInt(pageSize)
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
			channels = append(channels, parseNewAPIChannel(record))
		}
		total, hasTotal := paginationTotal(response.Payload)
		if hasTotal && page*pageSize >= total {
			break
		}
		if !hasTotal && len(items) < pageSize {
			break
		}
	}
	return channels, nil
}

// listNewAPIChannelsWithLocalFilter 兜底：分页拉取 /api/channel/ 全量 channel，
// 再在本地按「逗号分组精确匹配」过滤出属于 groupName 的 channel。
// 精确匹配：channel.group 按逗号拆分后逐段 TrimSpace 比较，避免 "vip" 命中 "vip2"（substring）。
func (s *PlatformService) listNewAPIChannelsWithLocalFilter(session Session, groupName string) ([]AdminGroupAccountInfo, error) {
	cookieOptions := newAPIAuthOptions(session)
	const pageSize = 100
	const maxPages = 100
	channels := make([]AdminGroupAccountInfo, 0)
	for page := 1; page <= maxPages; page++ {
		pageURL := session.BaseURL + "/api/channel/?p=" + strconvInt(int64(page)) + "&page_size=" + strconvInt(pageSize)
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
			if !channelBelongsToGroup(record, groupName) {
				continue
			}
			channels = append(channels, parseNewAPIChannel(record))
		}
		total, hasTotal := paginationTotal(response.Payload)
		if hasTotal && page*pageSize >= total {
			break
		}
		if !hasTotal && len(items) < pageSize {
			break
		}
	}
	return channels, nil
}

// channelBelongsToGroup 判断 channel 是否属于指定分组：channel.group 是逗号分隔的分组名字符串，
// 拆分后按段精确匹配，不做 substring 匹配。
func channelBelongsToGroup(record map[string]any, groupName string) bool {
	raw := firstString(record, []string{"group"})
	if raw == nil {
		return false
	}
	for _, part := range strings.Split(*raw, ",") {
		if strings.TrimSpace(part) == groupName {
			return true
		}
	}
	return false
}

// parseNewAPIChannel 把 new-api channel 原始记录解析为平台中性结构，主动丢弃 key 等敏感字段。
func parseNewAPIChannel(record map[string]any) AdminGroupAccountInfo {
	channel := AdminGroupAccountInfo{
		ID:       groupID2(record),
		Name:     safeString(record, "name"),
		Type:     stringOrNumberField(record, []string{"type"}),
		Status:   stringOrNumberField(record, []string{"status"}),
		Priority: firstInt(record, []string{"priority"}),
		Weight:   firstInt(record, []string{"weight"}),
	}
	if m := firstString(record, []string{"models"}); m != nil {
		channel.Models = *m
	}
	if b := firstString(record, []string{"base_url", "baseUrl"}); b != nil {
		channel.BaseURL = strings.TrimSpace(*b)
	}
	// channel.group 是逗号分隔的分组名字符串，拆成列表方便前端展示。
	if raw := firstString(record, []string{"group"}); raw != nil {
		parts := make([]string, 0)
		for _, part := range strings.Split(*raw, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				parts = append(parts, trimmed)
			}
		}
		channel.GroupIDs = parts
	}
	return channel
}

// stringOrNumberField 依次尝试把字段解析成字符串；不是字符串时回退按数值解析并转成字符串。
// 用于 status/type 这类上游可能返回字符串也可能返回数值枚举的字段。
func stringOrNumberField(record map[string]any, keys []string) string {
	if v := firstString(record, keys); v != nil {
		return *v
	}
	if n := firstNumber(record, keys); n != nil {
		return strconv.FormatInt(int64(*n), 10)
	}
	return ""
}

// firstInt 复用 firstNumber 读取整数字段，缺省或非法时返回 nil（区分「未提供」和「值为 0」）。
func firstInt(record map[string]any, keys []string) *int {
	if n := firstNumber(record, keys); n != nil {
		v := int(*n)
		return &v
	}
	return nil
}

// firstBoolValue 读取布尔字段，仅当上游明确给出 bool 时返回指针，否则返回 nil。
func firstBoolValue(record map[string]any, keys []string) *bool {
	for _, key := range keys {
		if b, ok := record[key].(bool); ok {
			return &b
		}
	}
	return nil
}

// parseGroupIDList 解析账号所属分组 ID 列表，兼容数值数组、字符串数组和逗号分隔字符串三种形态。
func parseGroupIDList(record map[string]any) []string {
	for _, key := range []string{"group_ids", "groupIds"} {
		value, ok := record[key]
		if !ok {
			continue
		}
		if arr, ok := value.([]any); ok {
			ids := make([]string, 0, len(arr))
			for _, item := range arr {
				switch typed := item.(type) {
				case float64:
					ids = append(ids, strconv.FormatInt(int64(typed), 10))
				case string:
					if trimmed := strings.TrimSpace(typed); trimmed != "" {
						ids = append(ids, trimmed)
					}
				}
			}
			return ids
		}
		if str, ok := value.(string); ok {
			ids := make([]string, 0)
			for _, part := range strings.Split(str, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					ids = append(ids, trimmed)
				}
			}
			return ids
		}
	}
	return nil
}
