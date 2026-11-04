package sales

import (
	"context"
	"errors"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"transithub/backend/internal/shared/bizclock"
)

// store 是 Repository 的只读接口，便于用内存实现测试服务层逻辑。
type store interface {
	SyncState(ctx context.Context, ws Workspace) (syncState, bool, error)
	PeriodTotals(ctx context.Context, ws Workspace, from, to time.Time, registeredFrom, registeredTo time.Time) (rawTotals, error)
	SiteBalance(ctx context.Context, ws Workspace, day time.Time) (*float64, error)
	Channels(ctx context.Context, ws Workspace, from, to time.Time) ([]ChannelTotal, error)
	DailyChannels(ctx context.Context, ws Workspace, from, to time.Time) ([]dailyChannelRow, error)
	DailyConsumption(ctx context.Context, ws Workspace, from, to time.Time) ([]dailyValueRow, error)
	PayerAmounts(ctx context.Context, ws Workspace, from, to time.Time) ([]float64, error)
	TopPayers(ctx context.Context, ws Workspace, from, to time.Time, limit int) ([]PayerRow, error)
}

// PriceProvider 返回工作区「我的站点售价」（站点余额 1 个单位卖多少元），与仪表盘营收换算一致。
type PriceProvider interface {
	SiteUnitPrice(ctx context.Context, userID string, adminAccountID string) (float64, error)
}

// AccountResolver 解析用户当前选中的 admin 工作区。
type AccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

// Service 基于「用户活跃分析」已同步到本地的数据计算销售指标，不访问站点。
type Service struct {
	store    store
	prices   PriceProvider
	accounts AccountResolver
	now      func() time.Time
}

func NewService(repository *Repository, prices PriceProvider, accounts AccountResolver) *Service {
	return &Service{store: repository, prices: prices, accounts: accounts, now: time.Now}
}

// CurrentWorkspace 返回用户当前选中的工作区。
func (s *Service) CurrentWorkspace(ctx context.Context, userID string) (Workspace, error) {
	if s.accounts == nil {
		return Workspace{}, requestError(ErrorNoCurrentAccount)
	}
	adminAccountID, err := s.accounts.RequireCurrentID(ctx, userID)
	if err != nil || strings.TrimSpace(adminAccountID) == "" {
		return Workspace{}, requestError(ErrorNoCurrentAccount)
	}
	return Workspace{UserID: userID, AdminAccountID: adminAccountID}, nil
}

// calendarDay 把业务日表示为 UTC 零点（与 user_activity 的 date 列口径一致）。
func calendarDay(t time.Time) time.Time {
	local := t.In(bizclock.Location())
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func formatDay(day time.Time) string { return day.Format(bizclock.DateLayout) }

func parseDay(value string) (time.Time, bool) {
	day, err := time.Parse(bizclock.DateLayout, strings.TrimSpace(value))
	return day, err == nil
}

// resolveRange 计算统计区间：默认是截至昨天的最近 N 个完整自然日；也可以指定 From / To（不晚于今天）。
func resolveRange(query RangeQuery, today time.Time) (time.Time, time.Time, error) {
	if strings.TrimSpace(query.From) != "" || strings.TrimSpace(query.To) != "" {
		from, okFrom := parseDay(query.From)
		to, okTo := parseDay(query.To)
		if !okFrom || !okTo || to.Before(from) || to.After(today) {
			return time.Time{}, time.Time{}, requestError(ErrorInvalidRange)
		}
		if int(to.Sub(from).Hours()/24)+1 > maxDays {
			return time.Time{}, time.Time{}, requestError(ErrorInvalidRange)
		}
		return from, to, nil
	}
	days := query.Days
	if days == 0 {
		days = defaultDays
	}
	if days < 1 || days > maxDays {
		return time.Time{}, time.Time{}, requestError(ErrorInvalidRange)
	}
	to := today.AddDate(0, 0, -1)
	return to.AddDate(0, 0, -(days - 1)), to, nil
}

func methodOf(platform string) string {
	switch platform {
	case "sub2api":
		return RechargeMethodRecords
	case "newapi":
		return RechargeMethodSnapshot
	}
	return ""
}

// rechargeCovers 判断入账数据是否完整覆盖 [from, to]。new-api 首次同步那天只统计了同步之后的部分，不算完整。
func rechargeCovers(state syncState, method string, from, to time.Time) bool {
	if state.RechargeFrom == nil || state.RechargeTo == nil || state.RechargeTo.Before(to) {
		return false
	}
	if method == RechargeMethodSnapshot {
		return state.RechargeFrom.Before(from)
	}
	return !state.RechargeFrom.After(from)
}

func usageCovers(state syncState, from, to time.Time) bool {
	return state.UsageFrom != nil && state.UsageTo != nil && !state.UsageFrom.After(from) && !state.UsageTo.Before(to)
}

func (s *Service) unitPrice(ctx context.Context, ws Workspace) float64 {
	if s.prices == nil {
		return 1
	}
	price, err := s.prices.SiteUnitPrice(ctx, ws.UserID, ws.AdminAccountID)
	if err != nil || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		if err != nil {
			log.Printf("[sales] load site unit price failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
		}
		return 1
	}
	return price
}

// Summary 返回销售分析：区间汇总与环比、渠道构成、每日序列、集中度、金额分布和入账排行。
func (s *Service) Summary(ctx context.Context, ws Workspace, query RangeQuery) (SummaryResponse, error) {
	today := calendarDay(s.now())
	from, to, err := resolveRange(query, today)
	if err != nil {
		return SummaryResponse{}, err
	}
	days := int(to.Sub(from).Hours()/24) + 1
	prevTo := from.AddDate(0, 0, -1)
	prevFrom := prevTo.AddDate(0, 0, -(days - 1))
	response := SummaryResponse{
		Range:         DayRange{From: formatDay(from), To: formatDay(to), Days: days},
		PreviousRange: DayRange{From: formatDay(prevFrom), To: formatDay(prevTo), Days: days},
		UnitPrice:     s.unitPrice(ctx, ws),
		Channels:      []ChannelTotal{},
		Daily:         []DailyPoint{},
		Distribution:  []DistributionBucket{},
		TopPayers:     []PayerRow{},
	}

	state, synced, err := s.store.SyncState(ctx, ws)
	if err != nil {
		return SummaryResponse{}, err
	}
	if !synced {
		return response, nil
	}
	method := methodOf(state.Platform)
	response.Coverage = Coverage{
		Synced:           true,
		Platform:         state.Platform,
		Method:           method,
		RechargeComplete: rechargeCovers(state, method, from, to),
		UsageComplete:    usageCovers(state, from, to),
		PreviousComplete: rechargeCovers(state, method, prevFrom, prevTo),
		LastSyncedAt:     state.LastSyncedAt,
	}
	if state.RechargeFrom != nil {
		response.Coverage.RechargeFrom = formatDay(*state.RechargeFrom)
	}
	if state.RechargeTo != nil {
		response.Coverage.RechargeTo = formatDay(*state.RechargeTo)
	}
	if state.UsageFrom != nil {
		response.Coverage.UsageFrom = formatDay(*state.UsageFrom)
	}
	if state.UsageTo != nil {
		response.Coverage.UsageTo = formatDay(*state.UsageTo)
	}

	price := response.UnitPrice
	// 首次入账只有在按充值记录统计、且区间开始前已有入账数据时才可靠（否则无法区分老用户）。
	newPayersKnown := method == RechargeMethodRecords && state.RechargeFrom != nil && state.RechargeFrom.Before(from)
	current, err := s.periodTotals(ctx, ws, from, to, price, newPayersKnown, response.Coverage.UsageComplete)
	if err != nil {
		return SummaryResponse{}, err
	}
	response.Current = current
	if response.Coverage.PreviousComplete {
		previousNewKnown := method == RechargeMethodRecords && state.RechargeFrom.Before(prevFrom)
		previous, err := s.periodTotals(ctx, ws, prevFrom, prevTo, price, previousNewKnown, usageCovers(state, prevFrom, prevTo))
		if err != nil {
			return SummaryResponse{}, err
		}
		response.Previous = &previous
	}

	channels, err := s.store.Channels(ctx, ws, from, to)
	if err != nil {
		return SummaryResponse{}, err
	}
	for i := range channels {
		channels[i].Amount *= price
	}
	response.Channels = channels

	if response.Daily, err = s.dailySeries(ctx, ws, state, from, to, price); err != nil {
		return SummaryResponse{}, err
	}

	amounts, err := s.store.PayerAmounts(ctx, ws, from, to)
	if err != nil {
		return SummaryResponse{}, err
	}
	for i := range amounts {
		amounts[i] *= price
	}
	response.Concentration = concentration(amounts)
	response.Distribution = distribution(amounts)

	payers, err := s.store.TopPayers(ctx, ws, from, to, topPayersLimit)
	if err != nil {
		return SummaryResponse{}, err
	}
	for i := range payers {
		payers[i].Credits *= price
		payers[i].Consumption *= price
		if payers[i].Balance != nil {
			balance := *payers[i].Balance * price
			payers[i].Balance = &balance
		}
		for j := range payers[i].ChannelBreakout {
			payers[i].ChannelBreakout[j].Amount *= price
		}
	}
	response.TopPayers = payers
	return response, nil
}

func (s *Service) periodTotals(ctx context.Context, ws Workspace, from, to time.Time, price float64, newPayersKnown bool, usageComplete bool) (PeriodTotals, error) {
	loc := bizclock.Location()
	registeredFrom := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
	registeredTo := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
	raw, err := s.store.PeriodTotals(ctx, ws, from, to, registeredFrom, registeredTo)
	if err != nil {
		return PeriodTotals{}, err
	}
	totals := PeriodTotals{
		Credits:        raw.Credits * price,
		Deductions:     raw.Deductions * price,
		Events:         raw.Events,
		Payers:         raw.Payers,
		RepeatPayers:   raw.RepeatPayers,
		Registered:     raw.Registered,
		RegisteredPaid: raw.RegisteredPaid,
	}
	if newPayersKnown {
		newPayers := raw.NewPayers
		totals.NewPayers = &newPayers
	}
	if usageComplete {
		consumption := raw.Consumption * price
		totals.Consumption = &consumption
	}
	if totals.BalanceStart, err = s.convertedBalance(ctx, ws, from.AddDate(0, 0, -1), price); err != nil {
		return PeriodTotals{}, err
	}
	if totals.BalanceEnd, err = s.convertedBalance(ctx, ws, to, price); err != nil {
		return PeriodTotals{}, err
	}
	return totals, nil
}

func (s *Service) convertedBalance(ctx context.Context, ws Workspace, day time.Time, price float64) (*float64, error) {
	balance, err := s.store.SiteBalance(ctx, ws, day)
	if err != nil || balance == nil {
		return nil, err
	}
	converted := *balance * price
	return &converted, nil
}

// dailySeries 生成每天的入账（按渠道）、扣减与消耗；本地数据未覆盖的日期为 nil。
func (s *Service) dailySeries(ctx context.Context, ws Workspace, state syncState, from, to time.Time, price float64) ([]DailyPoint, error) {
	channelRows, err := s.store.DailyChannels(ctx, ws, from, to)
	if err != nil {
		return nil, err
	}
	consumptionRows, err := s.store.DailyConsumption(ctx, ws, from, to)
	if err != nil {
		return nil, err
	}
	byDay := map[string]map[string]float64{}
	for _, row := range channelRows {
		key := formatDay(row.Day)
		if byDay[key] == nil {
			byDay[key] = map[string]float64{}
		}
		byDay[key][row.Channel] += row.Amount * price
	}
	consumption := map[string]float64{}
	for _, row := range consumptionRows {
		consumption[formatDay(row.Day)] = row.Value * price
	}
	points := make([]DailyPoint, 0)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		key := formatDay(day)
		point := DailyPoint{Day: key, Channels: map[string]float64{}}
		if state.RechargeFrom != nil && state.RechargeTo != nil && !day.Before(*state.RechargeFrom) && !day.After(*state.RechargeTo) {
			credits, deductions := 0.0, 0.0
			for channel, amount := range byDay[key] {
				if amount >= 0 {
					credits += amount
					point.Channels[channel] = amount
				} else {
					deductions -= amount
				}
			}
			point.Credits = &credits
			point.Deductions = &deductions
		}
		if state.UsageFrom != nil && state.UsageTo != nil && !day.Before(*state.UsageFrom) && !day.After(*state.UsageTo) {
			value := consumption[key]
			point.Consumption = &value
		}
		points = append(points, point)
	}
	return points, nil
}

// concentration 计算前 1 / 前 10 名入账用户的金额占比（amounts 已从大到小排序）。
func concentration(amounts []float64) Concentration {
	total := 0.0
	for _, amount := range amounts {
		total += amount
	}
	if total <= 0 {
		return Concentration{}
	}
	top10 := 0.0
	for i, amount := range amounts {
		if i >= 10 {
			break
		}
		top10 += amount
	}
	return Concentration{Top1Share: amounts[0] / total, Top10Share: top10 / total}
}

// distributionEdges 是人均入账金额（人民币）分档的下界。
var distributionEdges = []float64{0, 10, 50, 100, 500, 1000}

func distribution(amounts []float64) []DistributionBucket {
	buckets := make([]DistributionBucket, len(distributionEdges))
	for i, min := range distributionEdges {
		buckets[i].Min = min
		if i+1 < len(distributionEdges) {
			max := distributionEdges[i+1]
			buckets[i].Max = &max
		}
	}
	for _, amount := range amounts {
		index := sort.Search(len(distributionEdges), func(i int) bool { return distributionEdges[i] > amount }) - 1
		if index < 0 {
			index = 0
		}
		buckets[index].Users++
		buckets[index].Amount += amount
	}
	return buckets
}

// IsRequestError 判断是否为可直接返回给前端的业务错误。
func IsRequestError(err error) bool {
	var target requestError
	return errors.As(err, &target)
}
