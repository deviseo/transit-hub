package profit

import (
	"context"
	"errors"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
	"transithub/backend/internal/shared/bizclock"
)

const (
	defaultDays = 30
	maxDays     = 366
	// runwayWindowDays 是计算日均成本（续航、建议充值额）所用的最近天数；suggestDays 是建议充值覆盖的天数。
	runwayWindowDays  = 7
	suggestDays       = 7
	lowRunwayDays     = 3
	spikeMinCost      = 10.0
	spikeRatioFactor  = 1.5
	spikeBaselineDays = 7
)

// resolveRange 计算统计区间：默认截至昨天的最近 N 个完整自然日；自定义区间不能晚于昨天（今天还没结束）。
func resolveRange(query RangeQuery, today time.Time) (time.Time, time.Time, error) {
	yesterday := today.AddDate(0, 0, -1)
	if strings.TrimSpace(query.From) != "" || strings.TrimSpace(query.To) != "" {
		from, errFrom := time.Parse(bizclock.DateLayout, strings.TrimSpace(query.From))
		to, errTo := time.Parse(bizclock.DateLayout, strings.TrimSpace(query.To))
		if errFrom != nil || errTo != nil || to.Before(from) || to.After(yesterday) || int(to.Sub(from).Hours()/24)+1 > maxDays {
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
	return yesterday.AddDate(0, 0, -(days - 1)), yesterday, nil
}

func (s *Service) unitPrice(ctx context.Context, ws Workspace) float64 {
	if s.prices == nil {
		return 1
	}
	price, err := s.prices.SiteUnitPrice(ctx, ws.UserID, ws.AdminAccountID)
	if err != nil || price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
		if err != nil {
			log.Printf("[profit] load site unit price failed admin_account_id=%s err=%v", ws.AdminAccountID, err)
		}
		return 1
	}
	return price
}

func ratio(numerator, denominator float64) *float64 {
	if denominator <= 0 {
		return nil
	}
	value := numerator / denominator
	return &value
}

// analysis 是按上游、分组归因后的结果（金额为人民币）。
type analysis struct {
	totals       Totals
	upstreams    []UpstreamRow
	groups       []GroupRow
	unattributed []UnattributedRow
	// siteKeys 记录每个站点归入的上游，供续航与异常检测使用。
	siteKeys map[string]string
}

// analyze 把营收按路由的上游地址、成本按站点地址归到同一个上游（可注册域名），计算毛利；
// 分组成本按该分组在每个上游的营收占比分摊。price 把营收（站点额度）换算成人民币。
func analyze(routes []Route, revenue []RouteRevenue, costs []SiteCost, live []upstream.Response, price float64) analysis {
	routeByID := make(map[string]Route, len(routes))
	for _, route := range routes {
		routeByID[route.ID] = route
	}
	liveByID := make(map[string]upstream.Response, len(live))
	for _, site := range live {
		liveByID[site.ID] = site
	}

	result := analysis{siteKeys: map[string]string{}}
	entities := map[string]*UpstreamRow{}
	entity := func(key string) *UpstreamRow {
		row, ok := entities[key]
		if !ok {
			row = &UpstreamRow{Key: key, Sites: []UpstreamSite{}, Routes: []RouteRef{}}
			entities[key] = row
		}
		return row
	}

	// 成本侧：已登记的上游站点（含区间内有记录但已删除的站点）。
	seenSites := map[string]bool{}
	for _, cost := range costs {
		baseURL := cost.BaseURL
		site := UpstreamSite{ID: cost.SiteID, Name: cost.SiteName, RechargeRate: cost.RechargeRate, Cost: cost.Cost, FailedDays: cost.FailedDays, NoRate: cost.NoRateDays > 0, Deleted: true}
		if current, ok := liveByID[cost.SiteID]; ok {
			baseURL, site.Name, site.Platform, site.RechargeRate, site.Deleted = current.BaseURL, current.Name, string(current.Platform), current.RechargeRate, false
			site.NoRate = current.RechargeRate <= 0
		}
		key := upstreamKey(baseURL)
		if key == "" {
			continue
		}
		result.siteKeys[cost.SiteID] = key
		row := entity(key)
		row.Sites = append(row.Sites, site)
		row.Cost += cost.Cost
		result.totals.Cost += cost.Cost
		result.totals.FailedSiteDays += cost.FailedDays
		seenSites[cost.SiteID] = true
	}
	for _, site := range live {
		if seenSites[site.ID] {
			continue
		}
		key := upstreamKey(site.BaseURL)
		if key == "" {
			continue
		}
		result.siteKeys[site.ID] = key
		row := entity(key)
		row.Sites = append(row.Sites, UpstreamSite{ID: site.ID, Name: site.Name, Platform: string(site.Platform), RechargeRate: site.RechargeRate, NoRate: site.RechargeRate <= 0})
	}

	// 营收侧：路由的上游地址匹配到已登记上游的计入该上游，其余列为未归因。
	unattributed := map[string]*UnattributedRow{}
	groupRows := map[string]*GroupRow{}
	groupEntityRevenue := map[string]map[string]float64{}
	routeRevenue := map[string]float64{}
	routeKeys := map[string]string{}
	for _, row := range revenue {
		amount := row.Revenue * price
		if amount == 0 {
			continue
		}
		result.totals.Revenue += amount
		group := groupRows[row.Group]
		if group == nil {
			group = &GroupRow{Group: row.Group}
			groupRows[row.Group] = group
		}
		group.Revenue += amount

		route, known := routeByID[row.RouteID]
		key := ""
		if known {
			key = upstreamKey(route.BaseURL)
		}
		routeKeys[row.RouteID] = key
		routeRevenue[row.RouteID] += amount
		if target, ok := entities[key]; known && key != "" && ok {
			target.Revenue += amount
			result.totals.AttributedRevenue += amount
			group.AttributedRevenue += amount
			if groupEntityRevenue[row.Group] == nil {
				groupEntityRevenue[row.Group] = map[string]float64{}
			}
			groupEntityRevenue[row.Group][key] += amount
			continue
		}
		category := unattributedCategory(key)
		bucketKey := key
		if !known {
			category, bucketKey = CategoryUnknown, ""
		}
		bucket := unattributed[category+"|"+bucketKey]
		if bucket == nil {
			bucket = &UnattributedRow{Key: bucketKey, Category: category, Routes: []RouteRef{}}
			unattributed[category+"|"+bucketKey] = bucket
		}
		bucket.Revenue += amount
		result.totals.UnattributedRevenue += amount
		group.UnattributedRevenue += amount
	}

	// 路由列表：上游下列出所有指向它的路由（含区间内没有营收的），未归因下列出有营收的路由。
	for _, route := range routes {
		key := upstreamKey(route.BaseURL)
		ref := RouteRef{ID: route.ID, Name: route.Name, Groups: route.Groups, Status: route.Status, Deleted: route.Deleted, Revenue: routeRevenue[route.ID]}
		if ref.Groups == nil {
			ref.Groups = []string{}
		}
		if target, ok := entities[key]; ok && key != "" {
			if !route.Deleted || ref.Revenue > 0 {
				target.Routes = append(target.Routes, ref)
			}
			continue
		}
		if ref.Revenue > 0 {
			if bucket := unattributed[unattributedCategory(key)+"|"+key]; bucket != nil {
				bucket.Routes = append(bucket.Routes, ref)
			}
		}
	}
	if bucket := unattributed[CategoryUnknown+"|"]; bucket != nil {
		bucket.Routes = append(bucket.Routes, RouteRef{ID: "", Name: "", Groups: []string{}, Revenue: bucket.Revenue})
	}

	for _, row := range entities {
		row.Profit = row.Revenue - row.Cost
		row.Margin = ratio(row.Profit, row.Revenue)
		if row.Revenue == 0 {
			result.totals.UnmatchedCost += row.Cost
		}
		sort.Slice(row.Routes, func(i, j int) bool {
			if row.Routes[i].Revenue != row.Routes[j].Revenue {
				return row.Routes[i].Revenue > row.Routes[j].Revenue
			}
			return row.Routes[i].Name < row.Routes[j].Name
		})
		sort.Slice(row.Sites, func(i, j int) bool {
			return row.Sites[i].Cost > row.Sites[j].Cost || (row.Sites[i].Cost == row.Sites[j].Cost && row.Sites[i].Name < row.Sites[j].Name)
		})
		result.upstreams = append(result.upstreams, *row)
	}
	sort.Slice(result.upstreams, func(i, j int) bool {
		a, b := result.upstreams[i], result.upstreams[j]
		if a.Revenue != b.Revenue {
			return a.Revenue > b.Revenue
		}
		if a.Cost != b.Cost {
			return a.Cost > b.Cost
		}
		return a.Key < b.Key
	})

	// 分组成本：每个上游的成本按各分组在该上游的营收占比分摊。毛利与毛利率只针对已归因的部分。
	for name, group := range groupRows {
		for key, amount := range groupEntityRevenue[name] {
			if target := entities[key]; target != nil && target.Revenue > 0 {
				group.Cost += target.Cost * amount / target.Revenue
			}
		}
		group.Profit = group.AttributedRevenue - group.Cost
		group.Margin = ratio(group.Profit, group.AttributedRevenue)
		result.groups = append(result.groups, *group)
	}
	sort.Slice(result.groups, func(i, j int) bool {
		if result.groups[i].Revenue != result.groups[j].Revenue {
			return result.groups[i].Revenue > result.groups[j].Revenue
		}
		return result.groups[i].Group < result.groups[j].Group
	})

	for _, bucket := range unattributed {
		sort.Slice(bucket.Routes, func(i, j int) bool { return bucket.Routes[i].Revenue > bucket.Routes[j].Revenue })
		result.unattributed = append(result.unattributed, *bucket)
	}
	sort.Slice(result.unattributed, func(i, j int) bool {
		if result.unattributed[i].Revenue != result.unattributed[j].Revenue {
			return result.unattributed[i].Revenue > result.unattributed[j].Revenue
		}
		return result.unattributed[i].Key < result.unattributed[j].Key
	})

	result.totals.Profit = result.totals.Revenue - result.totals.Cost
	result.totals.Margin = ratio(result.totals.Profit, result.totals.Revenue)
	result.totals.AttributedShare = ratio(result.totals.AttributedRevenue, result.totals.Revenue)
	return result
}

// Summary 返回区间内的利润分析：总览与环比、每日序列、按上游 / 分组的毛利、未归因营收、站点续航和提示。
func (s *Service) Summary(ctx context.Context, ws Workspace, query RangeQuery) (SummaryResponse, error) {
	now := s.now()
	today := calendarDay(now)
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
		Daily:         []DailyPoint{},
		Upstreams:     []UpstreamRow{},
		Groups:        []GroupRow{},
		Unattributed:  []UnattributedRow{},
		Insights:      []Insight{},
	}
	if response.PriceChanges, err = s.recentPriceChanges(ctx, ws, now); err != nil {
		return SummaryResponse{}, err
	}
	state, exists, err := s.store.GetSyncState(ctx, ws)
	if err != nil {
		return SummaryResponse{}, err
	}
	response.Sync = statusOf(state)
	response.Synced = exists && state.SyncedFrom != nil && state.SyncedTo != nil
	if !response.Synced {
		return response, nil
	}
	response.Complete = !state.SyncedFrom.After(from) && state.CompleteThrough != nil && !state.CompleteThrough.Before(to)

	routes, err := s.store.ListRoutes(ctx, ws)
	if err != nil {
		return SummaryResponse{}, err
	}
	revenue, err := s.store.RouteRevenue(ctx, ws, from, to)
	if err != nil {
		return SummaryResponse{}, err
	}
	costs, err := s.store.SiteCosts(ctx, ws, from, to)
	if err != nil {
		return SummaryResponse{}, err
	}
	var live []upstream.Response
	if s.upstreams != nil {
		live = s.upstreams.ListForAccount(ctx, ws.UserID, ws.AdminAccountID)
	}
	result := analyze(routes, revenue, costs, live, response.UnitPrice)
	response.Totals = result.totals
	response.Upstreams = result.upstreams
	response.Groups = result.groups
	response.Unattributed = result.unattributed
	if response.Upstreams == nil {
		response.Upstreams = []UpstreamRow{}
	}
	if response.Groups == nil {
		response.Groups = []GroupRow{}
	}
	if response.Unattributed == nil {
		response.Unattributed = []UnattributedRow{}
	}

	if response.Daily, err = s.dailySeries(ctx, ws, state, from, to, response.UnitPrice); err != nil {
		return SummaryResponse{}, err
	}
	if !state.SyncedFrom.After(prevFrom) {
		previous, err := s.periodTotals(ctx, ws, prevFrom, prevTo, response.UnitPrice)
		if err != nil {
			return SummaryResponse{}, err
		}
		response.Previous = &previous
	}
	if err := s.applyRunway(ctx, ws, today, live, &response); err != nil {
		return SummaryResponse{}, err
	}
	spikes, err := s.costSpikes(ctx, ws, today, routes, result.siteKeys, response.UnitPrice)
	if err != nil {
		return SummaryResponse{}, err
	}
	response.Insights = buildInsights(response, spikes, now)
	return response, nil
}

func (s *Service) dailySeries(ctx context.Context, ws Workspace, state SyncState, from, to time.Time, price float64) ([]DailyPoint, error) {
	totals, err := s.store.DailyTotals(ctx, ws, from, to)
	if err != nil {
		return nil, err
	}
	points := make([]DailyPoint, 0, len(totals))
	for _, day := range totals {
		point := DailyPoint{Day: formatDay(day.Day)}
		if !day.Day.Before(*state.SyncedFrom) && !day.Day.After(*state.SyncedTo) {
			revenue := day.Revenue * price
			cost := day.Cost
			profit := revenue - cost
			point.Revenue, point.Cost, point.Profit = &revenue, &cost, &profit
		}
		points = append(points, point)
	}
	return points, nil
}

func (s *Service) periodTotals(ctx context.Context, ws Workspace, from, to time.Time, price float64) (Totals, error) {
	totals, err := s.store.DailyTotals(ctx, ws, from, to)
	if err != nil {
		return Totals{}, err
	}
	result := Totals{}
	for _, day := range totals {
		result.Revenue += day.Revenue * price
		result.Cost += day.Cost
		result.FailedSiteDays += day.FailedSites
	}
	result.Profit = result.Revenue - result.Cost
	result.Margin = ratio(result.Profit, result.Revenue)
	return result, nil
}

// applyRunway 用最近 runwayWindowDays 天（有数据的日期）的日均成本和当前余额，计算每个站点的续航与建议充值额。
func (s *Service) applyRunway(ctx context.Context, ws Workspace, today time.Time, live []upstream.Response, response *SummaryResponse) error {
	daily, err := s.store.SiteDailyCosts(ctx, ws, today.AddDate(0, 0, -runwayWindowDays), today.AddDate(0, 0, -1))
	if err != nil {
		return err
	}
	balances := map[string]*float64{}
	for _, site := range live {
		if site.Metrics.Balance.Value != nil && site.RechargeRate > 0 {
			balance := *site.Metrics.Balance.Value * site.RechargeRate
			balances[site.ID] = &balance
		}
	}
	for i := range response.Upstreams {
		for j := range response.Upstreams[i].Sites {
			site := &response.Upstreams[i].Sites[j]
			site.Balance = balances[site.ID]
			if days := daily[site.ID]; len(days) > 0 {
				total := 0.0
				for _, cost := range days {
					total += cost
				}
				average := total / float64(len(days))
				site.AvgDailyCost = &average
				if site.Balance != nil && average > 0 {
					runway := *site.Balance / average
					site.RunwayDays = &runway
					topUp := math.Max(0, average*suggestDays-*site.Balance)
					site.SuggestedTopUp = &topUp
				}
			}
		}
	}
	return nil
}

type spike struct {
	key      string
	ratio    float64
	baseline float64
}

// costSpikes 检测「成本 / 营收」比例异常上升的上游：昨天的比例超过前 spikeBaselineDays 天中位数的 spikeRatioFactor 倍，
// 且昨天成本不少于 spikeMinCost 元。常见原因是上游涨价，或 key 被本站以外的地方使用。
func (s *Service) costSpikes(ctx context.Context, ws Workspace, today time.Time, routes []Route, siteKeys map[string]string, price float64) ([]spike, error) {
	yesterday := today.AddDate(0, 0, -1)
	from := yesterday.AddDate(0, 0, -spikeBaselineDays)
	siteDaily, err := s.store.SiteDailyCosts(ctx, ws, from, yesterday)
	if err != nil {
		return nil, err
	}
	routeDaily, err := s.store.RouteDailyRevenue(ctx, ws, from, yesterday)
	if err != nil {
		return nil, err
	}
	costByKey := map[string]map[string]float64{}
	for siteID, days := range siteDaily {
		key := siteKeys[siteID]
		if key == "" {
			continue
		}
		if costByKey[key] == nil {
			costByKey[key] = map[string]float64{}
		}
		for day, cost := range days {
			costByKey[key][day] += cost
		}
	}
	revenueByKey := map[string]map[string]float64{}
	for _, route := range routes {
		key := upstreamKey(route.BaseURL)
		if key == "" || costByKey[key] == nil {
			continue
		}
		if revenueByKey[key] == nil {
			revenueByKey[key] = map[string]float64{}
		}
		for day, revenue := range routeDaily[route.ID] {
			revenueByKey[key][day] += revenue * price
		}
	}
	spikes := make([]spike, 0)
	last := formatDay(yesterday)
	for key, costs := range costByKey {
		lastCost, lastRevenue := costs[last], revenueByKey[key][last]
		if lastCost < spikeMinCost || lastRevenue <= 0 {
			continue
		}
		baseline := make([]float64, 0, spikeBaselineDays)
		for offset := 1; offset <= spikeBaselineDays; offset++ {
			day := formatDay(yesterday.AddDate(0, 0, -offset))
			if revenue := revenueByKey[key][day]; revenue > 0 {
				baseline = append(baseline, costs[day]/revenue)
			}
		}
		if len(baseline) < 3 {
			continue
		}
		sort.Float64s(baseline)
		median := baseline[len(baseline)/2]
		current := lastCost / lastRevenue
		if median > 0 && current > median*spikeRatioFactor {
			spikes = append(spikes, spike{key: key, ratio: current, baseline: median})
		}
	}
	sort.Slice(spikes, func(i, j int) bool { return spikes[i].key < spikes[j].key })
	return spikes, nil
}

func floatPtr(value float64) *float64 { return &value }

func buildInsights(response SummaryResponse, spikes []spike, now time.Time) []Insight {
	insights := make([]Insight, 0)
	for _, row := range response.Upstreams {
		if row.Revenue > 0 && row.Profit < 0 {
			insights = append(insights, Insight{Kind: InsightNegativeMargin, Severity: "critical", Subject: row.Key, Value: floatPtr(row.Profit)})
		}
		if row.Revenue == 0 && row.Cost > 0 {
			insights = append(insights, Insight{Kind: InsightUnmatchedCost, Severity: "warning", Subject: row.Key, Value: floatPtr(row.Cost)})
		}
		for _, site := range row.Sites {
			if site.Deleted {
				continue
			}
			if site.RunwayDays != nil && *site.RunwayDays < lowRunwayDays {
				insights = append(insights, Insight{Kind: InsightLowRunway, Severity: "critical", Subject: site.Name, Value: floatPtr(*site.RunwayDays), Extra: site.SuggestedTopUp})
			}
			if site.NoRate {
				insights = append(insights, Insight{Kind: InsightNoRate, Severity: "warning", Subject: site.Name})
			}
			if site.FailedDays > 0 {
				insights = append(insights, Insight{Kind: InsightCostFailed, Severity: "warning", Subject: site.Name, Value: floatPtr(float64(site.FailedDays))})
			}
		}
	}
	for _, item := range spikes {
		insights = append(insights, Insight{Kind: InsightCostSpike, Severity: "warning", Subject: item.key, Value: floatPtr(item.ratio), Extra: floatPtr(item.baseline)})
	}
	insights = append(insights, priceInsights(response.PriceChanges, now)...)
	for _, bucket := range response.Unattributed {
		if bucket.Category == CategoryUnregistered && bucket.Revenue > 0 {
			insights = append(insights, Insight{Kind: InsightUnregistered, Severity: "info", Subject: bucket.Key, Value: floatPtr(bucket.Revenue)})
		}
	}
	order := map[string]int{"critical": 0, "warning": 1, "info": 2}
	sort.SliceStable(insights, func(i, j int) bool { return order[insights[i].Severity] < order[insights[j].Severity] })
	return insights
}

// IsRequestError 判断是否为可直接返回给前端的业务错误。
func IsRequestError(err error) bool {
	var target requestError
	return errors.As(err, &target)
}
