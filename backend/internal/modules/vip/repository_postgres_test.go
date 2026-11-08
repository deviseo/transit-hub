package vip

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
func openVIPTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("vip_test_%d", time.Now().UnixNano())
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

func day(value string) time.Time {
	parsed, _ := time.Parse("2006-01-02", value)
	return parsed
}

// 近 7 天是 11-02 ~ 11-08，前 7 天是 10-26 ~ 11-01。
var testWindows = windows{From7: day("2026-11-02"), To7: day("2026-11-08"), PrevFrom7: day("2026-10-26"), PrevTo7: day("2026-11-01")}

func seedVIPData(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	statements := []string{
		`INSERT INTO user_activity_profiles (user_id, admin_account_id, site_user_id, email, username, status, balance, total_used, last_used_at) VALUES
			('u', 'a', '1', 'alice@example.test', 'alice', 'active', 30, NULL, '2026-11-08 20:00+08'),
			('u', 'a', '2', 'bob@example.test', 'bob', 'active', 500, NULL, '2026-11-07 20:00+08'),
			('u', 'a', '3', 'carol@example.test', 'carol', 'active', 0, NULL, '2026-09-01 20:00+08'),
			('u', 'a', '4', 'dave@example.test', 'dave', 'disabled', 0, NULL, '2026-11-08 20:00+08'),
			('u', 'a', '5', 'erin@example.test', 'erin', 'active', 2, NULL, NULL),
			('u', 'a', '6', 'frank@example.test', 'frank', 'active', 80, NULL, '2026-11-08 20:00+08'),
			('u', 'b', '1', 'other@example.test', 'other', 'active', 1, 9999, '2026-11-08 20:00+08')`,
		// 近 7 天：alice 70、frank 35、bob 7、dave 700（停用，不推荐）；前 7 天 bob 70。
		// 累计（本地每日消耗合计）：carol 5000（很早以前），alice 70，bob 77，frank 35，dave 700。
		`INSERT INTO user_activity_daily (user_id, admin_account_id, site_user_id, day, requests, tokens, cost) VALUES
			('u', 'a', '1', '2026-11-05', 10, 1000, 70),
			('u', 'a', '6', '2026-11-06', 10, 1000, 35),
			('u', 'a', '2', '2026-11-03', 10, 1000, 7),
			('u', 'a', '2', '2026-10-28', 10, 1000, 70),
			('u', 'a', '4', '2026-11-07', 10, 1000, 700),
			('u', 'a', '3', '2026-06-01', 10, 1000, 5000),
			('u', 'b', '1', '2026-11-05', 10, 1000, 999)`,
		`INSERT INTO user_activity_recharge_daily (user_id, admin_account_id, site_user_id, day, channel, amount, events) VALUES
			('u', 'a', '1', '2026-10-20', 'payment', 50, 1),
			('u', 'a', '1', '2026-11-04', 'payment', 30, 1),
			('u', 'a', '1', '2026-11-04', 'redeem', 20, 1),
			('u', 'a', '1', '2026-11-06', 'admin_deduct', -5, 1)`,
		`INSERT INTO user_activity_sync_states (user_id, admin_account_id, platform, status, synced_from, synced_to, user_count, last_synced_at)
			VALUES ('u', 'a', 'sub2api', 'success', '2026-06-01', '2026-11-09', 6, now())`,
	}
	for _, statement := range statements {
		if _, err := pool.Exec(context.Background(), statement); err != nil {
			t.Fatalf("seed %q: %v", statement[:40], err)
		}
	}
}

func ids(rows []statRow) string {
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, fmt.Sprintf("%s(d%d,l%d,%s)", row.SiteUserID, row.DailyRank, row.LifetimeRank, row.Kind))
	}
	return strings.Join(parts, " ")
}

func TestVIPRepositoryPostgres(t *testing.T) {
	pool := openVIPTestPool(t)
	seedVIPData(t, pool)
	repo := NewRepository(pool)
	ctx := context.Background()
	ws := Workspace{UserID: "u", AdminAccountID: "a"}

	// 推荐：近 7 天前 2 名（alice、frank；dave 停用不算）、累计前 1 名（carol）。
	rows, err := repo.Watchlist(ctx, ws, testWindows, 2, 1)
	if err != nil {
		t.Fatalf("watchlist: %v", err)
	}
	if got := ids(rows); got != "1(d1,l0,) 6(d2,l0,) 3(d0,l1,)" {
		t.Fatalf("recommended = %s", got)
	}
	alice := rows[0]
	if alice.Email != "alice@example.test" || alice.Cost7 != 70 || alice.CostPrev7 != 0 || alice.Lifetime != 70 ||
		alice.Balance == nil || *alice.Balance != 30 || alice.CreditDay == nil || alice.CreditDay.Format("2006-01-02") != "2026-11-04" || alice.CreditAmount != 50 {
		t.Fatalf("unexpected alice: %+v", alice)
	}

	// 手动关注 erin（不在推荐里）、给 alice 写备注；移出 carol。
	for _, item := range []struct{ id, note string }{{"5", "老客户"}, {"1", "XX 公司"}} {
		if found, err := repo.Watch(ctx, ws, item.id, item.note); err != nil || !found {
			t.Fatalf("watch %s: %v %v", item.id, found, err)
		}
	}
	if found, err := repo.Watch(ctx, ws, "404", ""); err != nil || found {
		t.Fatalf("unknown user: %v %v", found, err)
	}
	if err := repo.Unwatch(ctx, ws, "3"); err != nil {
		t.Fatalf("unwatch: %v", err)
	}
	rows, err = repo.Watchlist(ctx, ws, testWindows, 2, 1)
	if err != nil {
		t.Fatalf("watchlist: %v", err)
	}
	// carol 移出后，累计第 1 名换成下一个（bob 77；dave 停用不算）；erin 手动关注。
	if got := ids(rows); got != "1(d1,l0,manual) 6(d2,l0,) 2(d0,l1,) 5(d0,l0,manual)" {
		t.Fatalf("after watch and unwatch = %s", got)
	}
	if rows[0].Note != "XX 公司" {
		t.Fatalf("note: %+v", rows[0])
	}
	if count, err := repo.DismissedCount(ctx, ws); err != nil || count != 1 {
		t.Fatalf("dismissed count: %d %v", count, err)
	}

	// 搜索：按邮箱、用户名、ID；标出已关注、移出过的。
	found, costs, err := repo.Search(ctx, ws, testWindows, "CAROL", 20)
	if err != nil || len(found) != 1 || !found[0].Dismissed || costs[0] != 0 {
		t.Fatalf("search carol: %+v %v %v", found, costs, err)
	}
	found, _, err = repo.Search(ctx, ws, testWindows, "example.test", 2)
	if err != nil || len(found) != 2 || found[0].SiteUserID != "4" || found[1].SiteUserID != "1" || !found[1].Watched {
		t.Fatalf("search by domain, busiest first: %+v %v", found, err)
	}
	if found, _, err = repo.Search(ctx, ws, testWindows, "%", 20); err != nil || len(found) != 0 {
		t.Fatalf("wildcards are literal: %+v %v", found, err)
	}

	// 恢复移出的：carol 回到推荐。
	if restored, err := repo.RestoreDismissed(ctx, ws); err != nil || restored != 1 {
		t.Fatalf("restore: %d %v", restored, err)
	}

	// 设置：默认值，保存后读出来。
	settings, err := repo.Settings(ctx, ws)
	if err != nil || settings.DailyTop != 20 || !settings.AlertsEnabled || len(settings.BotIDs) != 0 {
		t.Fatalf("default settings: %+v %v", settings, err)
	}
	if workspaces, err := repo.AlertWorkspaces(ctx); err != nil || len(workspaces) != 0 {
		t.Fatalf("no bots yet: %+v %v", workspaces, err)
	}
	settings.BotIDs = []string{"bot-1"}
	settings.AlertBalance = 50
	settings.ReminderText = "您好 {name}"
	if err := repo.SaveSettings(ctx, ws, settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if saved, err := repo.Settings(ctx, ws); err != nil || saved.AlertBalance != 50 || saved.BotIDs[0] != "bot-1" || saved.ReminderText != "您好 {name}" {
		t.Fatalf("saved settings: %+v %v", saved, err)
	}
	if workspaces, err := repo.AlertWorkspaces(ctx); err != nil || len(workspaces) != 1 || workspaces[0] != ws {
		t.Fatalf("alert workspaces: %+v %v", workspaces, err)
	}

	// 提醒记录：记下提醒过的，回升的做标记。
	at := time.Date(2026, 11, 9, 4, 0, 0, 0, time.UTC)
	if err := repo.SaveAlertStates(ctx, ws, map[string]string{"1": StatusLow, "6": StatusEmpty}, at, nil); err != nil {
		t.Fatalf("save states: %v", err)
	}
	if err := repo.SaveAlertStates(ctx, ws, map[string]string{"6": StatusEmpty}, at.Add(time.Hour), []string{"1"}); err != nil {
		t.Fatalf("save states again: %v", err)
	}
	states, err := repo.AlertStates(ctx, ws)
	if err != nil || len(states) != 2 || !states["1"].Recovered || states["1"].State != StatusLow ||
		states["6"].Recovered || !states["6"].NotifiedAt.Equal(at.Add(time.Hour)) {
		t.Fatalf("states: %+v %v", states, err)
	}
	if other, err := repo.AlertStates(ctx, Workspace{UserID: "u", AdminAccountID: "b"}); err != nil || len(other) != 0 {
		t.Fatalf("other workspace: %+v %v", other, err)
	}
}

func TestVIPSummaryPostgresEndToEnd(t *testing.T) {
	pool := openVIPTestPool(t)
	seedVIPData(t, pool)
	service := NewService(NewRepository(pool), fixedPrice(2), nil, fakeNames{}, nil)
	// 北京时间 2026-11-09 12:00：近 7 天是 11-02 ~ 11-08。
	service.now = func() time.Time { return time.Date(2026, 11, 9, 4, 0, 0, 0, time.UTC) }
	summary, err := service.Summary(context.Background(), Workspace{UserID: "u", AdminAccountID: "a"})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if !summary.Synced || summary.UnitPrice != 2 || summary.SiteName != "极速中转" || summary.Counts.All != 4 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	statuses := map[string]string{}
	for _, row := range summary.Items {
		statuses[row.SiteUserID] = row.Status
	}
	// alice：余额 ¥60、日均 ¥20 → 3 天，余额 ≥ ¥20 → 正常；frank：¥160 / ¥10 → 16 天；carol：用完了；
	// bob：前 7 天日均 ¥20，近 7 天 ¥2 → 用量下降。
	want := map[string]string{"1": StatusNormal, "6": StatusNormal, "3": StatusEmpty, "2": StatusDeclining}
	if fmt.Sprint(statuses) != fmt.Sprint(want) {
		t.Fatalf("statuses = %v, want %v", statuses, want)
	}
	if summary.Counts.Empty != 1 || summary.Counts.Declining != 1 {
		t.Fatalf("counts: %+v", summary.Counts)
	}
}
