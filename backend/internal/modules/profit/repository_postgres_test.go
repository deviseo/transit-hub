package profit

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"transithub/backend/internal/database/migrations"
	"transithub/backend/internal/modules/upstream"
)

// PostgreSQL 测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中执行全部迁移并只写入合成数据，结束后整体删除。
func openProfitTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("profit_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		adminPool.Close()
		t.Fatalf("create schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		adminPool.Close()
		t.Fatalf("parse config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		adminPool.Close()
		t.Fatalf("connect schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := adminPool.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		adminPool.Close()
	})
	if err := migrations.Run(ctx, pool); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	if _, err := pool.Exec(ctx, `CREATE TABLE admin_accounts (id text PRIMARY KEY, user_id text NOT NULL); INSERT INTO admin_accounts VALUES ('a', 'u')`); err != nil {
		t.Fatalf("create admin_accounts: %v", err)
	}
	return pool
}

func TestProfitRepositoryPostgres(t *testing.T) {
	pool := openProfitTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	now := time.Now()

	if err := repo.UpsertRoutes(ctx, ws, []Route{{ID: "1", Name: "alpha", BaseURL: "https://alpha.example", Groups: []string{"claude"}}, {ID: "2", Name: "beta"}}, now); err != nil {
		t.Fatalf("upsert routes: %v", err)
	}
	if err := repo.UpsertRoutes(ctx, ws, []Route{{ID: "1", Name: "alpha-renamed", BaseURL: "https://alpha.example", Groups: []string{"claude", "gpt"}}}, now); err != nil {
		t.Fatalf("upsert routes again: %v", err)
	}
	routes, err := repo.ListRoutes(ctx, ws)
	if err != nil || len(routes) != 2 || routes[0].Name != "alpha-renamed" || len(routes[0].Groups) != 2 || !routes[1].Deleted {
		t.Fatalf("routes missing from the latest list are marked deleted: %+v %v", routes, err)
	}

	day1, day2, day3 := dayOf("2026-10-30"), dayOf("2026-10-31"), dayOf("2026-11-01")
	mustReplace := func(day time.Time, revenue []RouteRevenue, costs []SiteCost) {
		t.Helper()
		if err := repo.ReplaceDay(ctx, ws, day, revenue, costs); err != nil {
			t.Fatalf("replace day: %v", err)
		}
	}
	mustReplace(day1, []RouteRevenue{{RouteID: "1", Group: "claude", Revenue: 99}}, nil)
	mustReplace(day1, []RouteRevenue{{RouteID: "1", Group: "claude", Revenue: 10, Requests: 4}, {RouteID: "1", Group: "claude", Revenue: 1, Requests: 1}},
		[]SiteCost{{SiteID: "s1", SiteName: "Old name", BaseURL: "https://alpha.example", Consume: 2, RechargeRate: 2, Cost: 4, Status: upstream.ConsumeStatusOK}})
	mustReplace(day2, []RouteRevenue{{RouteID: "1", Group: "gpt", Revenue: 5}, {RouteID: "", Group: "", Revenue: 1}},
		[]SiteCost{{SiteID: "s1", SiteName: "Alpha", BaseURL: "https://alpha.example", Consume: 1, RechargeRate: 2, Cost: 2, Status: upstream.ConsumeStatusApproximated},
			{SiteID: "s2", SiteName: "Beta", BaseURL: "https://beta.example", Status: upstream.ConsumeStatusFailed}})
	mustReplace(day3, nil, []SiteCost{{SiteID: "s2", SiteName: "Beta", BaseURL: "https://beta.example", Status: upstream.ConsumeStatusNoRate}})

	revenue, err := repo.RouteRevenue(ctx, ws, day1, day3)
	if err != nil || fmt.Sprint(revenue) != "[{  1 0} {1 claude 11 5} {1 gpt 5 0}]" {
		t.Fatalf("route revenue (duplicates merged, replaced days overwritten): %v %v", revenue, err)
	}
	costs, err := repo.SiteCosts(ctx, ws, day1, day3)
	if err != nil || len(costs) != 2 {
		t.Fatalf("site costs: %+v %v", costs, err)
	}
	if costs[0].SiteName != "Alpha" || costs[0].Cost != 6 || costs[0].ApproximatedDays != 1 || costs[0].Days != 2 {
		t.Fatalf("names come from the latest day and costs are summed: %+v", costs[0])
	}
	if costs[1].FailedDays != 1 || costs[1].NoRateDays != 1 {
		t.Fatalf("status days must be counted: %+v", costs[1])
	}
	daily, err := repo.SiteDailyCosts(ctx, ws, day1, day3)
	if err != nil || len(daily) != 1 || daily["s1"]["2026-10-31"] != 2 {
		t.Fatalf("daily costs exclude failed and no-rate rows: %v %v", daily, err)
	}
	routeDaily, err := repo.RouteDailyRevenue(ctx, ws, day1, day3)
	if err != nil || routeDaily["1"]["2026-10-30"] != 11 || routeDaily[""]["2026-10-31"] != 1 {
		t.Fatalf("route daily revenue: %v %v", routeDaily, err)
	}
	totals, err := repo.DailyTotals(ctx, ws, day1, dayOf("2026-11-02"))
	if err != nil || len(totals) != 4 {
		t.Fatalf("daily totals must cover every day: %+v %v", totals, err)
	}
	if !totals[0].HasRevenue || totals[0].Revenue != 11 || totals[0].Cost != 4 || totals[1].FailedSites != 1 || totals[2].HasRevenue || !totals[2].HasCost || totals[3].HasCost {
		t.Fatalf("unexpected daily totals: %+v", totals)
	}

	syncedFrom, syncedTo := day1, day3
	if err := repo.SaveSyncState(ctx, ws, SyncState{Platform: "sub2api", Status: StatusRunning, SyncedFrom: &syncedFrom, SyncedTo: &syncedTo, CompleteThrough: &day2, RouteCount: 2, StartedBy: StartedByResume}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := repo.SaveSyncState(ctx, Workspace{UserID: "u", AdminAccountID: "deleted"}, SyncState{Status: StatusSuccess}); err != nil {
		t.Fatalf("save state for a deleted workspace: %v", err)
	}
	if err := repo.ResetRunningSyncs(ctx); err != nil {
		t.Fatalf("reset: %v", err)
	}
	state, exists, err := repo.GetSyncState(ctx, ws)
	if err != nil || !exists || state.Status != StatusInterrupted || state.ErrorKey != ErrorInterrupted || formatDay(*state.CompleteThrough) != "2026-10-31" || state.StartedBy != StartedByResume {
		t.Fatalf("running syncs become interrupted: %+v %v", state, err)
	}
	workspaces, err := repo.ListSyncedWorkspaces(ctx)
	if err != nil || len(workspaces) != 1 || workspaces[0] != ws {
		t.Fatalf("only existing workspaces are scheduled: %v %v", workspaces, err)
	}

	// 调价记录：最新的在前，可空的估算值保持为空，until 不包含在内。
	detected := time.Date(2026, 11, 1, 3, 0, 0, 0, time.UTC)
	impact, average, consume := 4.0, 20.0, 70.0
	saved, err := repo.InsertPriceChange(ctx, ws, PriceChange{SiteID: "s1", SiteName: "Alpha", BaseURL: "https://alpha.example", GroupID: "5", GroupName: "claude",
		OldMultiplier: 1, NewMultiplier: 1.2, RechargeRate: 2, WindowDays: 7, WindowConsume: &consume, EstimateStatus: EstimateOK, AvgDailyCost: &average, DailyImpact: &impact, DetectedAt: detected})
	if err != nil || saved.ID == 0 {
		t.Fatalf("insert price change: %+v %v", saved, err)
	}
	for _, change := range []PriceChange{
		{SiteID: "s2", GroupName: "gpt", OldMultiplier: 1, NewMultiplier: 0.9, EstimateStatus: EstimateFailed, DetectedAt: detected.Add(time.Hour)},
		{SiteID: "s2", GroupName: "old", OldMultiplier: 1, NewMultiplier: 2, EstimateStatus: EstimateFailed, DetectedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
	} {
		if _, err := repo.InsertPriceChange(ctx, ws, change); err != nil {
			t.Fatalf("insert price change: %v", err)
		}
	}
	changes, err := repo.ListPriceChanges(ctx, ws, detected, detected.Add(2*time.Hour), 10)
	if err != nil || len(changes) != 2 || changes[0].SiteID != "s2" || changes[0].DailyImpact != nil || changes[0].WindowConsume != nil {
		t.Fatalf("unexpected price changes: %+v %v", changes, err)
	}
	if first := changes[1]; first.ID != saved.ID || *first.DailyImpact != 4 || *first.WindowConsume != 70 || first.BaseURL != "https://alpha.example" || !first.DetectedAt.Equal(detected) {
		t.Fatalf("price change fields must round-trip: %+v", first)
	}
	if changes, _ := repo.ListPriceChanges(ctx, ws, detected, detected.Add(time.Hour), 10); len(changes) != 1 {
		t.Fatalf("until is exclusive: %+v", changes)
	}
	if changes, _ := repo.ListPriceChanges(ctx, Workspace{UserID: "u", AdminAccountID: "other"}, time.Time{}, detected.AddDate(1, 0, 0), 10); len(changes) != 0 {
		t.Fatalf("price changes are per workspace: %+v", changes)
	}

	if err := repo.Prune(ctx, ws, day2); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if revenue, _ := repo.RouteRevenue(ctx, ws, day1, day3); len(revenue) != 2 {
		t.Fatalf("days before the cut-off are pruned: %v", revenue)
	}
	if changes, _ := repo.ListPriceChanges(ctx, ws, time.Time{}, detected.AddDate(1, 0, 0), 10); len(changes) != 2 {
		t.Fatalf("old price changes are pruned: %+v", changes)
	}
}

type liveSites []upstream.Response

func (l liveSites) SiteConsumesOnDate(ctx context.Context, userID string, adminAccountID string, date string, timezone string) ([]upstream.SiteDailyConsume, error) {
	return nil, nil
}

func (l liveSites) ListForAccount(ctx context.Context, userID, adminAccountID string) []upstream.Response {
	return l
}

func (l liveSites) GroupConsumesInRange(ctx context.Context, userID string, siteID string, groups []upstream.GroupInfo, startDate string, endDate string, timezone string) (upstream.SiteGroupConsumes, error) {
	return upstream.SiteGroupConsumes{Consumes: map[string]float64{}}, nil
}

type fixedPrice float64

func (p fixedPrice) SiteUnitPrice(ctx context.Context, userID string, adminAccountID string) (float64, error) {
	return float64(p), nil
}

func balance(value float64) upstream.Metrics {
	return upstream.Metrics{Balance: upstream.MetricValue{Value: &value}}
}

func TestProfitSummaryPostgresEndToEnd(t *testing.T) {
	pool := openProfitTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	if err := repo.UpsertRoutes(ctx, ws, []Route{
		{ID: "1", Name: "alpha", BaseURL: "https://api.alpha.example", Groups: []string{"claude"}},
		{ID: "2", Name: "beta", BaseURL: "https://beta.example", Groups: []string{"gpt"}},
		{ID: "3", Name: "gamma", BaseURL: "https://gamma.example", Groups: []string{"gpt"}},
	}, time.Now()); err != nil {
		t.Fatalf("routes: %v", err)
	}
	// 10-20 ~ 11-02：alpha 每天营收 20、成本 6；beta 每天营收 10、成本 8，11-02 成本跳到 30（异常）；delta 有成本没有路由；gamma 未登记。
	for day := dayOf("2026-10-20"); !day.After(dayOf("2026-11-02")); day = day.AddDate(0, 0, 1) {
		betaCost := 8.0
		if formatDay(day) == "2026-11-02" {
			betaCost = 30
		}
		if err := repo.ReplaceDay(ctx, ws, day,
			[]RouteRevenue{{RouteID: "1", Group: "claude", Revenue: 20}, {RouteID: "2", Group: "gpt", Revenue: 10}, {RouteID: "3", Group: "gpt", Revenue: 2}},
			[]SiteCost{
				{SiteID: "s-alpha", SiteName: "Alpha", BaseURL: "https://alpha.example", Consume: 3, RechargeRate: 2, Cost: 6, Status: upstream.ConsumeStatusOK},
				{SiteID: "s-beta", SiteName: "Beta", BaseURL: "https://beta.example", Consume: betaCost, RechargeRate: 1, Cost: betaCost, Status: upstream.ConsumeStatusOK},
				{SiteID: "s-delta", SiteName: "Delta", BaseURL: "https://delta.example", Consume: 1, RechargeRate: 1, Cost: 1, Status: upstream.ConsumeStatusOK},
			}); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	from, to, complete := dayOf("2026-10-20"), dayOf("2026-11-02"), dayOf("2026-11-02")
	if err := repo.SaveSyncState(ctx, ws, SyncState{Platform: "sub2api", Status: StatusSuccess, SyncedFrom: &from, SyncedTo: &to, CompleteThrough: &complete, RouteCount: 3}); err != nil {
		t.Fatalf("state: %v", err)
	}
	live := liveSites{
		{ID: "s-alpha", Name: "Alpha", BaseURL: "https://alpha.example", Platform: upstream.PlatformSub2API, RechargeRate: 2, Metrics: balance(30)},
		{ID: "s-beta", Name: "Beta", BaseURL: "https://beta.example", Platform: upstream.PlatformNewAPI, RechargeRate: 1, Metrics: balance(500)},
		{ID: "s-delta", Name: "Delta", BaseURL: "https://delta.example", RechargeRate: 1, Metrics: balance(100)},
	}
	now := time.Date(2026, 11, 3, 2, 0, 0, 0, time.UTC)
	service := &Service{store: repo, upstreams: live, prices: fixedPrice(1), now: func() time.Time { return now }, running: map[Workspace]struct{}{}}
	impact := 3.0
	if _, err := repo.InsertPriceChange(ctx, ws, PriceChange{SiteID: "s-beta", SiteName: "Beta", BaseURL: "https://beta.example", GroupID: "9", GroupName: "gpt",
		OldMultiplier: 1, NewMultiplier: 1.5, RechargeRate: 1, WindowDays: 7, EstimateStatus: EstimateOK, DailyImpact: &impact, DetectedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatalf("price change: %v", err)
	}

	response, err := service.Summary(ctx, ws, RangeQuery{Days: 7})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if response.Range.From != "2026-10-27" || response.Range.To != "2026-11-02" || !response.Synced || !response.Complete || response.Previous == nil {
		t.Fatalf("unexpected range or coverage: %+v", response)
	}
	totals := response.Totals
	// 7 天：营收 (20+10+2)×7 = 224；成本 alpha 42 + beta 8×6+30 = 78 + delta 7 = 127。
	if !near(totals.Revenue, 224) || !near(totals.Cost, 127) || !near(totals.Profit, 97) || !near(totals.UnattributedRevenue, 14) || !near(totals.UnmatchedCost, 7) {
		t.Fatalf("unexpected totals: %+v", totals)
	}
	if len(response.Daily) != 7 || response.Daily[6].Cost == nil || !near(*response.Daily[6].Cost, 6+30+1) {
		t.Fatalf("unexpected daily series: %+v", response.Daily)
	}
	var alphaSite *UpstreamSite
	for i := range response.Upstreams {
		if response.Upstreams[i].Key == "alpha.example" {
			alphaSite = &response.Upstreams[i].Sites[0]
		}
	}
	// alpha 余额 30 × 充值倍率 2 = 60 元；近 7 天日均成本 6 元 → 续航 10 天，建议充值 0。
	if alphaSite == nil || alphaSite.Balance == nil || !near(*alphaSite.Balance, 60) || !near(*alphaSite.RunwayDays, 10) || *alphaSite.SuggestedTopUp != 0 {
		t.Fatalf("unexpected alpha runway: %+v", alphaSite)
	}
	kinds := map[string]Insight{}
	for _, insight := range response.Insights {
		kinds[insight.Kind+"|"+insight.Subject] = insight
	}
	if _, ok := kinds[InsightCostSpike+"|beta.example"]; !ok {
		t.Fatalf("beta's cost/revenue ratio tripled yesterday and must be flagged: %+v", response.Insights)
	}
	if _, ok := kinds[InsightUnmatchedCost+"|delta.example"]; !ok {
		t.Fatalf("delta has cost but no routes: %+v", response.Insights)
	}
	if _, ok := kinds[InsightUnregistered+"|gamma.example"]; !ok {
		t.Fatalf("gamma is not registered: %+v", response.Insights)
	}
	if _, ok := kinds[InsightCostSpike+"|alpha.example"]; ok {
		t.Fatalf("a steady upstream must not be flagged: %+v", response.Insights)
	}
	if len(response.PriceChanges) != 1 || response.PriceChanges[0].UpstreamKey != "beta.example" {
		t.Fatalf("recent price changes are listed with their upstream: %+v", response.PriceChanges)
	}
	if insight, ok := kinds[InsightPriceIncrease+"|Beta / gpt"]; !ok || *insight.Value != 3 || !near(*insight.Extra, 0.5) {
		t.Fatalf("recent price increases are insights: %+v", response.Insights)
	}

	// 区间不能包含今天，也不能早于保留期。
	if _, err := service.Summary(ctx, ws, RangeQuery{From: "2026-11-01", To: "2026-11-03"}); err == nil || err.Error() != ErrorInvalidRange {
		t.Fatalf("today is not a complete day: %v", err)
	}
	// 区间早于已采集的数据：不完整，也没有上一周期。
	partial, err := service.Summary(ctx, ws, RangeQuery{Days: 30})
	if err != nil || partial.Complete || partial.Previous != nil || partial.Daily[0].Revenue != nil {
		t.Fatalf("ranges before the synced data are incomplete: %+v %v", partial.Daily[0], err)
	}
	// 尚未采集的工作区。
	empty, err := service.Summary(ctx, Workspace{UserID: "u", AdminAccountID: "other"}, RangeQuery{})
	if err != nil || empty.Synced || len(empty.Upstreams) != 0 {
		t.Fatalf("unsynced workspaces return an empty summary: %+v %v", empty, err)
	}
}
