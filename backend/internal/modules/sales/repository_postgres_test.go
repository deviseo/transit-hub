package sales

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
)

// PostgreSQL 测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中执行全部迁移并只写入合成数据，结束后整体删除。
func openSalesTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("sales_test_%d", time.Now().UnixNano())
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
	return pool
}

func seedSalesData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	statements := []string{
		`INSERT INTO user_activity_profiles (user_id, admin_account_id, site_user_id, email, username, status, balance, site_created_at) VALUES
			('u', 'a', '1', 'alice@example.test', 'alice', 'active', 5,  '2026-10-28 10:00:00+08'),
			('u', 'a', '2', 'bob@example.test',   'bob',   'active', 20, '2026-09-01 10:00:00+08'),
			('u', 'a', '3', 'carol@example.test', 'carol', 'active', 0,  '2026-10-30 23:30:00+08'),
			('u', 'a', '4', 'dave@example.test',  'dave',  'active', 1,  '2026-10-01 10:00:00+08'),
			('u', 'b', '9', 'other@example.test', 'other', 'active', 1,  '2026-10-29 10:00:00+08')`,
		// carol 在 11-03 00:30（北京时间）注册：不在 10-27 ~ 11-02 区间内。
		`INSERT INTO user_activity_profiles (user_id, admin_account_id, site_user_id, email, site_created_at) VALUES
			('u', 'a', '5', 'eve@example.test', '2026-11-03 00:30:00+08')`,
		`INSERT INTO user_activity_recharge_daily (user_id, admin_account_id, site_user_id, day, channel, amount, events) VALUES
			('u', 'a', '2', '2026-10-10', 'payment', 10, 1),
			('u', 'a', '2', '2026-10-28', 'payment', 30, 1),
			('u', 'a', '2', '2026-10-30', 'redeem', 20, 1),
			('u', 'a', '1', '2026-10-29', 'payment', 50, 2),
			('u', 'a', '4', '2026-10-31', 'admin_add', 15, 1),
			('u', 'a', '4', '2026-10-31', 'admin_deduct', -5, 1),
			('u', 'b', '9', '2026-10-29', 'payment', 999, 1)`,
		`INSERT INTO user_activity_daily (user_id, admin_account_id, site_user_id, day, requests, tokens, cost) VALUES
			('u', 'a', '1', '2026-10-29', 3, 300, 3),
			('u', 'a', '2', '2026-10-28', 7, 700, 7),
			('u', 'a', '2', '2026-10-20', 9, 900, 100),
			('u', 'b', '9', '2026-10-29', 1, 100, 500)`,
		`INSERT INTO user_activity_site_daily (user_id, admin_account_id, day, total_balance, users_with_balance, user_count) VALUES
			('u', 'a', '2026-10-26', 100, 3, 4),
			('u', 'a', '2026-11-02', 150, 3, 5)`,
		`INSERT INTO user_activity_sync_states (user_id, admin_account_id, platform, status, synced_from, synced_to, user_count, last_synced_at, recharge_synced_from, recharge_synced_to) VALUES
			('u', 'a', 'sub2api', 'success', '2026-08-01', '2026-11-03', 5, now(), '2026-01-01', '2026-11-03')`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(context.Background(), statement); err != nil {
			t.Fatalf("seed %q: %v", statement[:40], err)
		}
	}
}

func TestSalesRepositoryPostgres(t *testing.T) {
	pool := openSalesTestPool(t)
	seedSalesData(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()
	ws := Workspace{UserID: "u", AdminAccountID: "a"}
	from, to := day("2026-10-27"), day("2026-11-02")
	loc := time.FixedZone("CST", 8*3600)
	regFrom := time.Date(2026, 10, 27, 0, 0, 0, 0, loc)
	regTo := time.Date(2026, 11, 3, 0, 0, 0, 0, loc)

	totals, err := repo.PeriodTotals(ctx, ws, from, to, regFrom, regTo)
	if err != nil {
		t.Fatalf("period totals: %v", err)
	}
	want := rawTotals{Credits: 115, Deductions: 5, Events: 5, Payers: 3, RepeatPayers: 1, NewPayers: 2, Registered: 2, RegisteredPaid: 1, Consumption: 10}
	if totals != want {
		t.Fatalf("period totals = %+v, want %+v", totals, want)
	}

	if balance, err := repo.SiteBalance(ctx, ws, day("2026-10-26")); err != nil || balance == nil || *balance != 100 {
		t.Fatalf("site balance: %v %v", balance, err)
	}
	if balance, err := repo.SiteBalance(ctx, ws, day("2026-10-27")); err != nil || balance != nil {
		t.Fatalf("missing snapshot must be nil: %v %v", balance, err)
	}

	channels, err := repo.Channels(ctx, ws, from, to)
	if err != nil {
		t.Fatalf("channels: %v", err)
	}
	wantChannels := []ChannelTotal{
		{Channel: "payment", Amount: 80, Events: 3, Users: 2},
		{Channel: "redeem", Amount: 20, Events: 1, Users: 1},
		{Channel: "admin_add", Amount: 15, Events: 1, Users: 1},
		{Channel: "admin_deduct", Amount: -5, Events: 1, Users: 1},
	}
	if fmt.Sprint(channels) != fmt.Sprint(wantChannels) {
		t.Fatalf("channels = %+v, want %+v", channels, wantChannels)
	}

	daily, err := repo.DailyChannels(ctx, ws, from, to)
	if err != nil || len(daily) != 5 || formatDay(daily[0].Day) != "2026-10-28" || daily[0].Amount != 30 {
		t.Fatalf("daily channels: %+v %v", daily, err)
	}
	usage, err := repo.DailyConsumption(ctx, ws, from, to)
	if err != nil || len(usage) != 2 || usage[0].Value != 7 || usage[1].Value != 3 {
		t.Fatalf("daily consumption: %+v %v", usage, err)
	}

	amounts, err := repo.PayerAmounts(ctx, ws, from, to)
	if err != nil || fmt.Sprint(amounts) != "[50 50 15]" {
		t.Fatalf("payer amounts: %v %v", amounts, err)
	}

	payers, err := repo.TopPayers(ctx, ws, from, to, 2)
	if err != nil || len(payers) != 2 {
		t.Fatalf("top payers: %+v %v", payers, err)
	}
	bob := payers[1]
	if payers[0].SiteUserID != "1" || bob.SiteUserID != "2" || bob.Email != "bob@example.test" || bob.Credits != 50 || bob.Events != 2 || bob.ActiveDays != 2 ||
		bob.LastCreditDay != "2026-10-30" || bob.Consumption != 7 || bob.Balance == nil || *bob.Balance != 20 {
		t.Fatalf("unexpected payer row: %+v", bob)
	}
	if len(bob.ChannelBreakout) != 2 || bob.ChannelBreakout[0].Channel != "payment" || bob.ChannelBreakout[0].Amount != 30 || bob.ChannelBreakout[1].Channel != "redeem" {
		t.Fatalf("unexpected channel mix: %+v", bob.ChannelBreakout)
	}

	state, synced, err := repo.SyncState(ctx, ws)
	if err != nil || !synced || state.Platform != "sub2api" || formatDay(*state.RechargeFrom) != "2026-01-01" {
		t.Fatalf("sync state: %+v %v %v", state, synced, err)
	}
	if _, synced, err := repo.SyncState(ctx, Workspace{UserID: "u", AdminAccountID: "missing"}); err != nil || synced {
		t.Fatalf("missing sync state: %v %v", synced, err)
	}
}

func TestSalesSummaryPostgresEndToEnd(t *testing.T) {
	pool := openSalesTestPool(t)
	seedSalesData(t, pool)
	service := NewService(NewRepository(pool), fixedPrice(2), nil)
	service.now = func() time.Time { return testNow }

	response, err := service.Summary(context.Background(), Workspace{UserID: "u", AdminAccountID: "a"}, RangeQuery{Days: 7})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	cur := response.Current
	if cur.Credits != 230 || cur.Deductions != 10 || cur.Payers != 3 || cur.NewPayers == nil || *cur.NewPayers != 2 || cur.Consumption == nil || *cur.Consumption != 20 {
		t.Fatalf("unexpected current totals: %+v", cur)
	}
	if cur.BalanceStart == nil || *cur.BalanceStart != 200 || cur.BalanceEnd == nil || *cur.BalanceEnd != 300 {
		t.Fatalf("unexpected balances: %+v", cur)
	}
	if response.Previous == nil || response.Previous.Credits != 0 {
		t.Fatalf("previous period is covered and empty: %+v", response.Previous)
	}
	if len(response.TopPayers) != 3 || response.TopPayers[0].Credits != 100 || response.Distribution[1].Users != 1 || response.Distribution[3].Users != 2 {
		t.Fatalf("unexpected payers or distribution: %+v %+v", response.TopPayers, response.Distribution)
	}
	if response.Concentration.Top10Share != 1 {
		t.Fatalf("unexpected concentration: %+v", response.Concentration)
	}
}

func TestSalesExclusionsPostgres(t *testing.T) {
	pool := openSalesTestPool(t)
	seedSalesData(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()
	ws := Workspace{UserID: "u", AdminAccountID: "a"}
	from, to := day("2026-10-27"), day("2026-11-02")
	loc := time.FixedZone("CST", 8*3600)
	regFrom := time.Date(2026, 10, 27, 0, 0, 0, 0, loc)
	regTo := time.Date(2026, 11, 3, 0, 0, 0, 0, loc)
	dave := EntryKey{SiteUserID: "4", Channel: "admin_add", day: day("2026-10-31")}
	totals := func() rawTotals {
		t.Helper()
		got, err := repo.PeriodTotals(ctx, ws, from, to, regFrom, regTo)
		if err != nil {
			t.Fatalf("period totals: %v", err)
		}
		return got
	}

	// 管理员给 dave 的加款不算销售：入账、笔数、入账用户、首次入账都去掉它，扣款照旧单独列。
	if count, err := repo.ExcludeEntries(ctx, ws, []EntryKey{dave, {SiteUserID: "404", Channel: "payment", day: day("2026-10-31")}}); err != nil || count != 1 {
		t.Fatalf("exclude: %d %v", count, err)
	}
	want := rawTotals{Credits: 100, Deductions: 5, Events: 4, Payers: 2, RepeatPayers: 1, NewPayers: 1, Registered: 2, RegisteredPaid: 1, Consumption: 10}
	if got := totals(); got != want {
		t.Fatalf("totals after excluding = %+v, want %+v", got, want)
	}
	channels, err := repo.Channels(ctx, ws, from, to)
	if err != nil {
		t.Fatalf("channels: %v", err)
	}
	wantChannels := []ChannelTotal{
		{Channel: "payment", Amount: 80, Events: 3, Users: 2},
		{Channel: "redeem", Amount: 20, Events: 1, Users: 1},
		{Channel: "admin_add", Amount: 0, Events: 0, Users: 0, ExcludedAmount: 15, ExcludedEntries: 1},
		{Channel: "admin_deduct", Amount: -5, Events: 1, Users: 1},
	}
	if fmt.Sprint(channels) != fmt.Sprint(wantChannels) {
		t.Fatalf("channels = %+v, want %+v", channels, wantChannels)
	}
	if amounts, err := repo.PayerAmounts(ctx, ws, from, to); err != nil || fmt.Sprint(amounts) != "[50 50]" {
		t.Fatalf("payer amounts: %v %v", amounts, err)
	}
	if payers, err := repo.TopPayers(ctx, ws, from, to, 10); err != nil || len(payers) != 2 {
		t.Fatalf("top payers without dave: %+v %v", payers, err)
	}
	if daily, err := repo.DailyChannels(ctx, ws, from, to); err != nil || len(daily) != 4 {
		t.Fatalf("daily without the excluded row: %+v %v", daily, err)
	}

	// 当天后来又加了一次（同一行变大）：只排除原来那笔，新增的照常计入。
	if _, err := pool.Exec(ctx, `UPDATE user_activity_recharge_daily SET amount = 40, events = 2
		WHERE user_id = 'u' AND admin_account_id = 'a' AND site_user_id = '4' AND day = '2026-10-31' AND channel = 'admin_add'`); err != nil {
		t.Fatalf("grow row: %v", err)
	}
	if got := totals(); got.Credits != 125 || got.Events != 5 || got.Payers != 3 {
		t.Fatalf("the later addition counts: %+v", got)
	}

	// 明细：按天倒序，带不计入的部分；可以按渠道、用户筛选，只看不计入的，分页。
	all := entryFilter{From: from, To: to, Limit: 50}
	items, total, err := repo.Entries(ctx, ws, all)
	if err != nil || total != 4 || len(items) != 4 {
		t.Fatalf("entries: %+v %d %v", items, total, err)
	}
	if first := items[0]; first.SiteUserID != "4" || first.Email != "dave@example.test" || first.Amount != 40 || first.Events != 2 ||
		first.ExcludedAmount != 15 || first.ExcludedEvents != 1 || first.Day != "2026-10-31" {
		t.Fatalf("unexpected first entry: %+v", first)
	}
	for _, check := range []struct {
		filter entryFilter
		total  int
	}{
		{entryFilter{From: from, To: to, Channel: "payment", Limit: 50}, 2},
		{entryFilter{From: from, To: to, Search: "BOB", Limit: 50}, 2},
		{entryFilter{From: from, To: to, Search: "1", Limit: 50}, 1},
		{entryFilter{From: from, To: to, Search: "%", Limit: 50}, 0},
		{entryFilter{From: from, To: to, ExcludedOnly: true, Limit: 50}, 1},
	} {
		if _, total, err := repo.Entries(ctx, ws, check.filter); err != nil || total != check.total {
			t.Fatalf("filter %+v: total %d, want %d (%v)", check.filter, total, check.total, err)
		}
	}
	if page, total, err := repo.Entries(ctx, ws, entryFilter{From: from, To: to, Limit: 1, Offset: 1}); err != nil || total != 4 || len(page) != 1 || page[0].Day != "2026-10-30" {
		t.Fatalf("second page: %+v %d %v", page, total, err)
	}
	// 别的工作区的同一个用户 ID 不受影响。
	if items, total, err := repo.Entries(ctx, Workspace{UserID: "u", AdminAccountID: "b"}, all); err != nil || total != 1 || items[0].ExcludedAmount != 0 {
		t.Fatalf("other workspace: %+v %d %v", items, total, err)
	}

	// 恢复：回到原来的数字。
	if count, err := repo.RestoreEntries(ctx, ws, []EntryKey{dave}); err != nil || count != 1 {
		t.Fatalf("restore: %d %v", count, err)
	}
	if got := totals(); got.Credits != 140 || got.Payers != 3 || got.NewPayers != 2 {
		t.Fatalf("restored totals: %+v", got)
	}

	// 整个渠道不计入：兑换码不算入账，但渠道列表里还在（页面上可以重新打开）。
	if err := repo.SetExcludedChannels(ctx, ws, []string{"redeem"}); err != nil {
		t.Fatalf("set channels: %v", err)
	}
	if saved, err := repo.ExcludedChannels(ctx, ws); err != nil || fmt.Sprint(saved) != "[redeem]" {
		t.Fatalf("saved channels: %v %v", saved, err)
	}
	if got := totals(); got.Credits != 120 || got.RepeatPayers != 0 {
		t.Fatalf("totals without redeem: %+v", got)
	}
	channels, err = repo.Channels(ctx, ws, from, to)
	if err != nil || channels[1].Channel != "redeem" && channels[2].Channel != "redeem" {
		t.Fatalf("redeem stays listed: %+v %v", channels, err)
	}
	if saved, err := repo.ExcludedChannels(ctx, Workspace{UserID: "u", AdminAccountID: "b"}); err != nil || len(saved) != 0 {
		t.Fatalf("other workspace has no setting: %v %v", saved, err)
	}

	// 服务层：整个渠道不计入时标出来，并算进「没有计入」的合计。
	service := NewService(repo, fixedPrice(2), nil)
	service.now = func() time.Time { return testNow }
	response, err := service.Summary(ctx, ws, RangeQuery{Days: 7})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if response.Current.Credits != 240 || fmt.Sprint(response.Exclusions) != fmt.Sprint(ExclusionSummary{Channels: []string{"redeem"}, Amount: 40}) {
		t.Fatalf("summary with excluded channel: %+v %+v", response.Current, response.Exclusions)
	}
}
