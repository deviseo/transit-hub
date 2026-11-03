package upstream

import (
	"net/url"
	"strconv"
	"strings"
	"time"
)

// 用户活跃分析使用的数据源：只读取当前工作区「我的站点」（管理员会话）的用户与用量，
// 不访问任何上游站点的用户数据。

// Sub2APIUserUsageTrendPoint 是 /api/v1/admin/dashboard/users-trend 中某用户某天的用量。
type Sub2APIUserUsageTrendPoint struct {
	Date       string
	UserID     string
	Email      string
	Username   string
	Requests   int64
	Tokens     int64
	Cost       float64
	ActualCost float64
}

// FetchSub2APIUserUsageTrend 读取 [startDate, endDate]（含两端，YYYY-MM-DD）内每个用户每天的用量。
// sub2api 按用量取前 limit 名用户，传入站点用户总数即可覆盖全部有用量的用户；timezone 决定按哪个时区切分自然日。
func (s *PlatformService) FetchSub2APIUserUsageTrend(session Session, startDate string, endDate string, timezone string, limit int) ([]Sub2APIUserUsageTrendPoint, error) {
	if session.Platform != PlatformSub2API || !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformSub2API)
	}
	if limit < 1 {
		limit = 1
	}
	values := url.Values{}
	values.Set("start_date", startDate)
	values.Set("end_date", endDate)
	values.Set("granularity", "day")
	values.Set("metric", "actual_cost")
	values.Set("limit", strconv.Itoa(limit))
	if strings.TrimSpace(timezone) != "" {
		values.Set("timezone", timezone)
	}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/admin/dashboard/users-trend?"+values.Encode(), adminAuthOptions(session))
	if err != nil {
		return nil, err
	}
	rows, _ := dataRecord(response.Payload)["trend"].([]any)
	points := make([]Sub2APIUserUsageTrendPoint, 0, len(rows))
	for _, row := range rows {
		record, ok := row.(map[string]any)
		if !ok {
			continue
		}
		point := Sub2APIUserUsageTrendPoint{
			Date:       strings.TrimSpace(safeString(record, "date")),
			UserID:     firstStringy(record, sub2APIUserIDKeys),
			Email:      safeString(record, "email"),
			Username:   safeString(record, "username"),
			Requests:   int64FromRecord(record, []string{"requests"}),
			Tokens:     int64FromRecord(record, []string{"tokens", "total_tokens"}),
			Cost:       floatFromRecord(record, []string{"cost"}),
			ActualCost: floatFromRecord(record, []string{"actual_cost", "actualCost"}),
		}
		// sub2api 用户 ID 从 1 开始；0 或缺失都视为无效行。
		if point.UserID == "" || point.UserID == "0" || point.Date == "" {
			continue
		}
		points = append(points, point)
	}
	return points, nil
}

// NewAPIAdminUser 是 new-api 管理员用户列表中的一行。Balance / UsedQuota 为剩余额度 / 累计消耗换算后的美元口径；
// EverUsed 表示该用户有过消费（used_quota 或 request_count 大于 0），用于区分「从未使用」与「很久没用」。
type NewAPIAdminUser struct {
	ID          string
	Username    string
	Email       string
	Group       string
	Role        int
	Status      int
	Balance     float64
	UsedQuota   float64
	EverUsed    bool
	CreatedAt   *time.Time
	LastLoginAt *time.Time
}

// NewAPIAdminUsersPage 是 new-api 用户列表的一页。
type NewAPIAdminUsersPage struct {
	Items      []NewAPIAdminUser
	Total      int
	TotalKnown bool
}

// FetchNewAPIAdminUsersPage 读取 new-api 用户列表的一页（GET /api/user/?p=&page_size=，page 从 1 开始，每页最多 100）。
func (s *PlatformService) FetchNewAPIAdminUsersPage(session Session, page int, pageSize int) (NewAPIAdminUsersPage, error) {
	if session.Platform != PlatformNewAPI || !session.IsAuthenticated() {
		return NewAPIAdminUsersPage{}, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 100
	}
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/user/?p="+strconv.Itoa(page)+"&page_size="+strconv.Itoa(pageSize), newAPIAuthOptions(session))
	if err != nil {
		return NewAPIAdminUsersPage{}, err
	}
	items := dataArray(response.Payload)
	result := NewAPIAdminUsersPage{Items: make([]NewAPIAdminUser, 0, len(items))}
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		user := NewAPIAdminUser{
			ID:          groupID2(record),
			Username:    safeString(record, "username"),
			Email:       safeString(record, "email"),
			Group:       safeString(record, "group"),
			Role:        intFromRecord(record, []string{"role"}, 0),
			Status:      intFromRecord(record, []string{"status"}, 0),
			Balance:     quotaToUSDValueWithUnit(firstNumber(record, []string{"quota"}), session.QuotaPerUnit),
			UsedQuota:   quotaToUSDValueWithUnit(firstNumber(record, []string{"used_quota"}), session.QuotaPerUnit),
			EverUsed:    floatFromRecord(record, []string{"used_quota"}) > 0 || floatFromRecord(record, []string{"request_count"}) > 0,
			CreatedAt:   unixSecondsTime(record, "created_at"),
			LastLoginAt: unixSecondsTime(record, "last_login_at"),
		}
		if user.ID == "" {
			continue
		}
		result.Items = append(result.Items, user)
	}
	result.Total, result.TotalKnown = paginationTotal(response.Payload)
	return result, nil
}

// NewAPIUserQuotaBucket 是 /api/data/users 中某用户某小时的用量。Cost 已按 quota_per_unit 换算为美元口径。
type NewAPIUserQuotaBucket struct {
	Username  string
	HourStart time.Time
	Requests  int64
	Tokens    int64
	Cost      float64
}

// FetchNewAPIUserQuotaData 读取 [start, end] 内按「用户 × 小时」聚合的用量（new-api quota_data，需要管理员权限，
// PAT 需要 log:read）。new-api 默认开启数据看板统计（DataExportEnabled）；关闭时这里没有数据。
func (s *PlatformService) FetchNewAPIUserQuotaData(session Session, start time.Time, end time.Time) ([]NewAPIUserQuotaBucket, error) {
	if session.Platform != PlatformNewAPI || !session.IsAuthenticated() {
		return nil, newRequestError(ErrorAuth, PlatformNewAPI)
	}
	query := url.Values{}
	query.Set("start_timestamp", strconv.FormatInt(start.Unix(), 10))
	query.Set("end_timestamp", strconv.FormatInt(end.Unix(), 10))
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/data/users?"+query.Encode(), newAPIAuthOptions(session))
	if err != nil {
		return nil, err
	}
	rows := dataArray(response.Payload)
	buckets := make([]NewAPIUserQuotaBucket, 0, len(rows))
	for _, row := range rows {
		record, ok := row.(map[string]any)
		if !ok {
			continue
		}
		hour := unixSecondsTime(record, "created_at")
		username := strings.TrimSpace(safeString(record, "username"))
		if hour == nil || username == "" {
			continue
		}
		buckets = append(buckets, NewAPIUserQuotaBucket{
			Username:  username,
			HourStart: *hour,
			Requests:  int64FromRecord(record, []string{"count"}),
			Tokens:    int64FromRecord(record, []string{"token_used"}),
			Cost:      quotaToUSDValueWithUnit(firstNumber(record, []string{"quota"}), session.QuotaPerUnit),
		})
	}
	return buckets, nil
}

// unixSecondsTime 读取 Unix 秒时间戳字段，0 或缺失时返回 nil。
func unixSecondsTime(record map[string]any, key string) *time.Time {
	value := firstNumber(record, []string{key})
	if value == nil || *value <= 0 {
		return nil
	}
	parsed := time.Unix(int64(*value), 0)
	return &parsed
}

// Sub2APIRedeemRecord 是 /api/v1/admin/redeem-codes 中的一条记录。sub2api 的余额入账都记在这里：
// 在线充值（type=balance，code 以 PAY- 开头）、兑换码（type=balance）、管理员调整余额（type=admin_balance，value 带正负号）。
type Sub2APIRedeemRecord struct {
	ID     string
	Code   string
	Type   string
	Value  float64
	UsedBy string
	UsedAt *time.Time
}

// Sub2APIRedeemRecordsPage 是一页记录。
type Sub2APIRedeemRecordsPage struct {
	Items      []Sub2APIRedeemRecord
	Total      int
	TotalKnown bool
}

// FetchSub2APIUsedRedeemRecords 按使用时间倒序读取某类型已使用的记录（一页，page 从 1 开始，每页最多 500 条）。
func (s *PlatformService) FetchSub2APIUsedRedeemRecords(session Session, codeType string, page int, pageSize int) (Sub2APIRedeemRecordsPage, error) {
	if session.Platform != PlatformSub2API || !session.IsAuthenticated() {
		return Sub2APIRedeemRecordsPage{}, newRequestError(ErrorAuth, PlatformSub2API)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 500 {
		pageSize = 500
	}
	values := url.Values{}
	values.Set("type", codeType)
	values.Set("status", "used")
	values.Set("sort_by", "used_at")
	values.Set("sort_order", "desc")
	values.Set("page", strconv.Itoa(page))
	values.Set("page_size", strconv.Itoa(pageSize))
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/admin/redeem-codes?"+values.Encode(), adminAuthOptions(session))
	if err != nil {
		return Sub2APIRedeemRecordsPage{}, err
	}
	items := dataArray(response.Payload)
	result := Sub2APIRedeemRecordsPage{Items: make([]Sub2APIRedeemRecord, 0, len(items))}
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		parsed := Sub2APIRedeemRecord{
			ID:     firstStringy(record, []string{"id"}),
			Code:   strings.TrimSpace(safeString(record, "code")),
			Type:   strings.TrimSpace(safeString(record, "type")),
			Value:  floatFromRecord(record, []string{"value"}),
			UsedBy: firstStringy(record, []string{"used_by", "usedBy"}),
			UsedAt: parseFlexibleTime(firstAny(record, []string{"used_at", "usedAt"})),
		}
		if parsed.ID == "" {
			continue
		}
		result.Items = append(result.Items, parsed)
	}
	result.Total, result.TotalKnown = paginationTotal(response.Payload)
	return result, nil
}

// Sub2APIAffiliateTransfer 是邀请返利的流水（action=transfer 表示转入余额，withdraw 不入余额）。
type Sub2APIAffiliateTransfer struct {
	ID        string
	Action    string
	UserID    string
	Amount    float64
	CreatedAt *time.Time
}

// Sub2APIAffiliateTransfersPage 是一页返利流水。
type Sub2APIAffiliateTransfersPage struct {
	Items      []Sub2APIAffiliateTransfer
	Total      int
	TotalKnown bool
}

// FetchSub2APIAffiliateTransfers 读取 [startDate, endDate]（含两端，YYYY-MM-DD，按 timezone 切分）内的返利流水（一页，最多 100 条）。
// 较早的 sub2api 版本没有该接口（返回 404），调用方应视为不支持而不是失败。
func (s *PlatformService) FetchSub2APIAffiliateTransfers(session Session, startDate string, endDate string, timezone string, page int, pageSize int) (Sub2APIAffiliateTransfersPage, error) {
	if session.Platform != PlatformSub2API || !session.IsAuthenticated() {
		return Sub2APIAffiliateTransfersPage{}, newRequestError(ErrorAuth, PlatformSub2API)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 100
	}
	values := url.Values{}
	values.Set("start_at", startDate)
	values.Set("end_at", endDate)
	if strings.TrimSpace(timezone) != "" {
		values.Set("timezone", timezone)
	}
	values.Set("sort_by", "created_at")
	values.Set("sort_order", "desc")
	values.Set("page", strconv.Itoa(page))
	values.Set("page_size", strconv.Itoa(pageSize))
	response, err := s.httpClient.requestJSON(session.BaseURL+"/api/v1/admin/affiliates/transfers?"+values.Encode(), adminAuthOptions(session))
	if err != nil {
		return Sub2APIAffiliateTransfersPage{}, err
	}
	items := dataArray(response.Payload)
	result := Sub2APIAffiliateTransfersPage{Items: make([]Sub2APIAffiliateTransfer, 0, len(items))}
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		parsed := Sub2APIAffiliateTransfer{
			ID:        firstStringy(record, []string{"ledger_id", "id"}),
			Action:    strings.TrimSpace(safeString(record, "action")),
			UserID:    firstStringy(record, []string{"user_id", "userId"}),
			Amount:    floatFromRecord(record, []string{"amount"}),
			CreatedAt: parseFlexibleTime(firstAny(record, []string{"created_at", "createdAt"})),
		}
		if parsed.ID == "" {
			continue
		}
		result.Items = append(result.Items, parsed)
	}
	result.Total, result.TotalKnown = paginationTotal(response.Payload)
	return result, nil
}
