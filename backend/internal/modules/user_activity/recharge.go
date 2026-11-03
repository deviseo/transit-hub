package user_activity

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// sub2api 入账记录：在线充值（balance，code 以 PAY- 开头）、兑换码（balance）、管理员调整余额（admin_balance，带正负号）
// 都在 /api/v1/admin/redeem-codes 中；邀请返利转余额在 /api/v1/admin/affiliates/transfers 中。

const (
	redeemPageSize        = 500
	maxRedeemPagesPerScan = 400
	affiliatePageSize     = 100
	maxAffiliatePages     = 200
	sub2apiPaymentPrefix  = "PAY-"
	redeemTypeBalance     = "balance"
	redeemTypeAdminAdjust = "admin_balance"
	affiliateTransfer     = "transfer"
)

var (
	// errUnsortedRecords：站点没有按使用时间排序返回记录（较旧版本不支持 sort_by=used_at），无法可靠地按时间读取。
	errUnsortedRecords = errors.New("redeem records are not sorted by used_at")
	// errTooManyRecords：单次需要扫描的记录过多，放弃本次读取，避免写入不完整的数据。
	errTooManyRecords = errors.New("too many redeem records to scan")
)

// rechargeScan 是一段时间内的入账（按「用户 × 天 × 渠道」聚合）。
// NoneBefore 表示站点上不存在比这段时间更早的入账记录，回补可以直接视为覆盖到保留期起点。
type rechargeScan struct {
	Recharges  []RechargeDaily
	NoneBefore bool
}

// fetchSub2APIRecharges 读取 [start, end) 内的全部入账记录。任一来源失败都返回错误，调用方不写入（避免某天数据不完整）。
func (s *Service) fetchSub2APIRecharges(ctx context.Context, session upstream.Session, start time.Time, end time.Time, retainFrom time.Time) (rechargeScan, error) {
	aggregator := newRechargeAggregator(s)
	noneBefore := true
	for _, codeType := range []string{redeemTypeBalance, redeemTypeAdminAdjust} {
		records, none, err := s.scanRedeemRecords(ctx, session, codeType, start, end)
		if err != nil {
			return rechargeScan{}, err
		}
		noneBefore = noneBefore && none
		for _, record := range records {
			if record.UsedAt == nil {
				continue
			}
			aggregator.add(record.UsedBy, *record.UsedAt, redeemChannel(codeType, record), record.Value)
		}
	}
	transfers, supported, err := s.fetchAffiliateTransfers(ctx, session, start, end)
	if err != nil {
		return rechargeScan{}, err
	}
	for _, transfer := range transfers {
		if transfer.CreatedAt != nil {
			aggregator.add(transfer.UserID, *transfer.CreatedAt, ChannelAffiliate, transfer.Amount)
		}
	}
	if noneBefore && supported {
		older, err := s.affiliateHasRecordsBefore(ctx, session, retainFrom, start)
		if err != nil {
			return rechargeScan{}, err
		}
		noneBefore = !older
	}
	return rechargeScan{Recharges: aggregator.list(), NoneBefore: noneBefore}, nil
}

func redeemChannel(codeType string, record upstream.Sub2APIRedeemRecord) string {
	if codeType == redeemTypeAdminAdjust {
		if record.Value < 0 {
			return ChannelAdminDeduct
		}
		return ChannelAdminAdd
	}
	if strings.HasPrefix(strings.ToUpper(record.Code), sub2apiPaymentPrefix) {
		return ChannelPayment
	}
	return ChannelRedeem
}

// scanRedeemRecords 读取某类型在 [start, end) 内使用的记录。列表按使用时间倒序：先用二分查找定位到包含 end 之前记录的页，
// 再向后顺序读取直到早于 start。noneBefore 表示已经读到最后一页、不存在早于 start 的记录。
func (s *Service) scanRedeemRecords(ctx context.Context, session upstream.Session, codeType string, start time.Time, end time.Time) ([]upstream.Sub2APIRedeemRecord, bool, error) {
	cache := map[int]upstream.Sub2APIRedeemRecordsPage{}
	requests := 0
	get := func(page int) (upstream.Sub2APIRedeemRecordsPage, error) {
		if cached, ok := cache[page]; ok {
			return cached, nil
		}
		if requests > 0 {
			if err := s.sleep(ctx); err != nil {
				return upstream.Sub2APIRedeemRecordsPage{}, err
			}
		}
		requests++
		if requests > maxRedeemPagesPerScan {
			return upstream.Sub2APIRedeemRecordsPage{}, errTooManyRecords
		}
		result, err := s.platform.FetchSub2APIUsedRedeemRecords(session, codeType, page, redeemPageSize)
		if err != nil {
			return upstream.Sub2APIRedeemRecordsPage{}, err
		}
		if !sortedByUsedAt(result.Items) {
			return upstream.Sub2APIRedeemRecordsPage{}, errUnsortedRecords
		}
		cache[page] = result
		return result, nil
	}

	first, err := get(1)
	if err != nil {
		return nil, false, err
	}
	if len(first.Items) == 0 {
		return nil, true, nil
	}
	pages := -1
	if first.TotalKnown {
		pages = int(math.Ceil(float64(first.Total) / float64(redeemPageSize)))
	}
	isLastPage := func(page int, result upstream.Sub2APIRedeemRecordsPage) bool {
		if pages > 0 {
			return page >= pages
		}
		return len(result.Items) < redeemPageSize
	}

	startPage := 1
	if oldest, ok := oldestUsedAt(first.Items); ok && !oldest.Before(end) {
		// 第一页全部晚于 end：定位第一张包含 end 之前记录的页。
		if isLastPage(1, first) {
			return nil, true, nil
		}
		found := -1
		if pages > 0 {
			lo, hi := 2, pages
			for lo <= hi {
				mid := (lo + hi) / 2
				result, err := get(mid)
				if err != nil {
					return nil, false, err
				}
				oldestMid, ok := oldestUsedAt(result.Items)
				if !ok {
					hi = mid - 1
					continue
				}
				if oldestMid.Before(end) {
					found = mid
					hi = mid - 1
				} else {
					lo = mid + 1
				}
			}
		} else {
			for page := 2; ; page++ {
				result, err := get(page)
				if err != nil {
					return nil, false, err
				}
				if oldestPage, ok := oldestUsedAt(result.Items); ok && oldestPage.Before(end) {
					found = page
					break
				}
				if isLastPage(page, result) {
					break
				}
			}
		}
		if found < 0 {
			// 所有记录都晚于 end，更早的记录不存在。
			return nil, true, nil
		}
		startPage = found
	}

	records := make([]upstream.Sub2APIRedeemRecord, 0)
	seen := map[string]struct{}{}
	// 从前一页开始读，容忍读取期间新增记录导致的分页偏移（重复的记录按 ID 去重）。
	page := startPage - 1
	if page < 1 {
		page = 1
	}
	for {
		result, err := get(page)
		if err != nil {
			return nil, false, err
		}
		if len(result.Items) == 0 {
			return records, true, nil
		}
		for _, item := range result.Items {
			if item.UsedAt == nil || item.UsedAt.Before(start) || !item.UsedAt.Before(end) {
				continue
			}
			if _, duplicate := seen[item.ID]; duplicate {
				continue
			}
			seen[item.ID] = struct{}{}
			records = append(records, item)
		}
		if oldest, ok := oldestUsedAt(result.Items); ok && oldest.Before(start) {
			return records, false, nil
		}
		if isLastPage(page, result) {
			return records, true, nil
		}
		page++
	}
}

func sortedByUsedAt(items []upstream.Sub2APIRedeemRecord) bool {
	var previous *time.Time
	for _, item := range items {
		if item.UsedAt == nil {
			continue
		}
		if previous != nil && item.UsedAt.After(*previous) {
			return false
		}
		previous = item.UsedAt
	}
	return true
}

func oldestUsedAt(items []upstream.Sub2APIRedeemRecord) (time.Time, bool) {
	for i := len(items) - 1; i >= 0; i-- {
		if items[i].UsedAt != nil {
			return *items[i].UsedAt, true
		}
	}
	return time.Time{}, false
}

// fetchAffiliateTransfers 读取 [start, end) 内转入余额的返利；supported=false 表示站点版本没有这个接口。
func (s *Service) fetchAffiliateTransfers(ctx context.Context, session upstream.Session, start time.Time, end time.Time) ([]upstream.Sub2APIAffiliateTransfer, bool, error) {
	startDate := formatDay(s.dayOf(start))
	endDate := formatDay(s.dayOf(end.Add(-time.Nanosecond)))
	transfers := make([]upstream.Sub2APIAffiliateTransfer, 0)
	for page := 1; page <= maxAffiliatePages; page++ {
		if err := s.sleep(ctx); err != nil {
			return nil, false, err
		}
		result, err := s.platform.FetchSub2APIAffiliateTransfers(session, startDate, endDate, s.tzName, page, affiliatePageSize)
		if err != nil {
			if unsupportedEndpoint(err) {
				return nil, false, nil
			}
			return nil, false, err
		}
		for _, item := range result.Items {
			if item.Action != affiliateTransfer || item.CreatedAt == nil || item.CreatedAt.Before(start) || !item.CreatedAt.Before(end) {
				continue
			}
			transfers = append(transfers, item)
		}
		if len(result.Items) < affiliatePageSize || (result.TotalKnown && page*affiliatePageSize >= result.Total) {
			return transfers, true, nil
		}
	}
	return nil, false, errTooManyRecords
}

// affiliateHasRecordsBefore 判断 [retainFrom, before) 内是否还有返利流水（只读一条）。
func (s *Service) affiliateHasRecordsBefore(ctx context.Context, session upstream.Session, retainFrom time.Time, before time.Time) (bool, error) {
	lastDay := s.dayOf(before.Add(-time.Nanosecond))
	if lastDay.Before(retainFrom) {
		return false, nil
	}
	if err := s.sleep(ctx); err != nil {
		return false, err
	}
	result, err := s.platform.FetchSub2APIAffiliateTransfers(session, formatDay(retainFrom), formatDay(lastDay), s.tzName, 1, 1)
	if err != nil {
		if unsupportedEndpoint(err) {
			return false, nil
		}
		return false, err
	}
	return len(result.Items) > 0 || (result.TotalKnown && result.Total > 0), nil
}

// unsupportedEndpoint：接口不存在（404，或返回的不是 JSON，例如前端页面）。
func unsupportedEndpoint(err error) bool {
	var requestErr *upstream.RequestError
	if !errors.As(err, &requestErr) {
		return false
	}
	return requestErr.StatusCode == 404 || requestErr.MessageKey == upstream.ErrorInvalidResponse
}

// rechargeWarningKey 把入账读取失败转换为提示文案。
func rechargeWarningKey(err error) string {
	switch {
	case errors.Is(err, errUnsortedRecords):
		return WarningRechargeUnsupported
	case errors.Is(err, errTooManyRecords):
		return WarningRechargeTruncated
	default:
		return WarningRechargeFetch
	}
}

type rechargeKey struct {
	user    string
	day     time.Time
	channel string
}

type rechargeAggregator struct {
	service *Service
	rows    map[rechargeKey]*RechargeDaily
}

func newRechargeAggregator(service *Service) *rechargeAggregator {
	return &rechargeAggregator{service: service, rows: map[rechargeKey]*RechargeDaily{}}
}

func (a *rechargeAggregator) add(userID string, at time.Time, channel string, amount float64) {
	userID = strings.TrimSpace(userID)
	if userID == "" || userID == "0" || amount == 0 {
		return
	}
	key := rechargeKey{user: userID, day: a.service.dayOf(at), channel: channel}
	row, ok := a.rows[key]
	if !ok {
		row = &RechargeDaily{SiteUserID: userID, Day: key.day, Channel: channel}
		a.rows[key] = row
	}
	row.Amount += amount
	row.Events++
}

func (a *rechargeAggregator) list() []RechargeDaily {
	rows := make([]RechargeDaily, 0, len(a.rows))
	for _, row := range a.rows {
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if !rows[i].Day.Equal(rows[j].Day) {
			return rows[i].Day.Before(rows[j].Day)
		}
		if rows[i].SiteUserID != rows[j].SiteUserID {
			return rows[i].SiteUserID < rows[j].SiteUserID
		}
		return rows[i].Channel < rows[j].Channel
	})
	return rows
}

// siteDailyOf 根据本次同步的用户快照计算当天的全站快照。
func siteDailyOf(day time.Time, profiles []Profile) *SiteDaily {
	snapshot := &SiteDaily{Day: day, UserCount: len(profiles)}
	for _, profile := range profiles {
		if profile.Balance == nil {
			continue
		}
		snapshot.TotalBalance += *profile.Balance
		if *profile.Balance > 0 {
			snapshot.UsersWithBalance++
		}
	}
	return snapshot
}

// localMidnight 返回某个自然日（UTC 零点表示）在业务时区的零点。
func (s *Service) localMidnight(day time.Time) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, s.loc)
}
