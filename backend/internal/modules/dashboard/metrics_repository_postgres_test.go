package dashboard

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgreSQL 仓储测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中只写入合成数据，结束后整体删除。
func openDashboardTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("dashboard_test_%d", time.Now().UnixNano())
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
	return pool
}

func TestMetricsRepositoryPostgresFinalizeAndPricing(t *testing.T) {
	pool := openDashboardTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := pool.Exec(ctx, `CREATE TABLE admin_accounts (id text PRIMARY KEY, user_id text NOT NULL); INSERT INTO admin_accounts VALUES ('account-1', 'user-1')`); err != nil {
		t.Fatalf("create admin_accounts: %v", err)
	}
	repo := NewMetricsRepository(pool)
	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema must be idempotent: %v", err)
	}

	day := time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	live := DailySnapshot{ID: "s1", UserID: "user-1", AdminAccountID: "account-1", Date: day, TodayProfit: 10, SiteBalance: 111, TodayPurchase: 3, NetProfit: 7, UpstreamBalance: 222, CreatedAt: time.Now()}
	if err := repo.Upsert(ctx, live); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if err := repo.Upsert(ctx, DailySnapshot{ID: "s0", UserID: "user-1", AdminAccountID: "account-1", Date: older, TodayProfit: 1, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("upsert older: %v", err)
	}
	pending, err := repo.UnfinalizedDates(ctx, "user-1", "account-1", older, day)
	if err != nil || len(pending) != 2 {
		t.Fatalf("unexpected pending dates: %v %v", pending, err)
	}

	// 事后定稿：保留当天记录的余额，只替换营收 / 成本。
	if err := repo.FinalizeSnapshot(ctx, DailySnapshot{ID: "s2", UserID: "user-1", AdminAccountID: "account-1", Date: day, TodayProfit: 200, SiteBalance: 999, TodayPurchase: 60, NetProfit: 140, UpstreamBalance: 888, CreatedAt: time.Now()}, true); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	snapshots, err := repo.ListRange(ctx, "user-1", "account-1", day.AddDate(0, 0, 1), 7)
	if err != nil || len(snapshots) != 2 {
		t.Fatalf("list range: %+v %v", snapshots, err)
	}
	got := snapshots[1]
	if got.TodayProfit != 200 || got.TodayPurchase != 60 || got.NetProfit != 140 || got.SiteBalance != 111 || got.UpstreamBalance != 222 {
		t.Fatalf("unexpected finalized snapshot: %+v", got)
	}
	if finalized, err := repo.IsFinalized(ctx, "user-1", "account-1", day); err != nil || !finalized {
		t.Fatalf("day must be finalized: %v %v", finalized, err)
	}
	// 白天的实时写入不能覆盖已定稿的日期。
	if err := repo.Upsert(ctx, DailySnapshot{ID: "s3", UserID: "user-1", AdminAccountID: "account-1", Date: day, TodayProfit: 1, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("late upsert: %v", err)
	}
	snapshots, _ = repo.ListRange(ctx, "user-1", "account-1", day.AddDate(0, 0, 1), 7)
	if snapshots[1].TodayProfit != 200 {
		t.Fatalf("a finalized day was overwritten: %+v", snapshots[1])
	}
	if pending, err := repo.UnfinalizedDates(ctx, "user-1", "account-1", older, day); err != nil || len(pending) != 1 || !pending[0].Equal(older) {
		t.Fatalf("only the older day is still pending: %v %v", pending, err)
	}
	// 新建定稿行（之前没有快照）时使用传入的余额。
	fresh := time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)
	if err := repo.FinalizeSnapshot(ctx, DailySnapshot{ID: "s4", UserID: "user-1", AdminAccountID: "account-1", Date: fresh, TodayProfit: 5, SiteBalance: 50, UpstreamBalance: 60, CreatedAt: time.Now()}, true); err != nil {
		t.Fatalf("finalize new row: %v", err)
	}
	snapshots, _ = repo.ListRange(ctx, "user-1", "account-1", day.AddDate(0, 0, 1), 7)
	if len(snapshots) != 3 || snapshots[1].SiteBalance != 50 || snapshots[1].UpstreamBalance != 60 {
		t.Fatalf("unexpected new finalized row: %+v", snapshots)
	}

	pricing, err := repo.GetPricing(ctx, "user-1", "account-1")
	if err != nil || pricing.SiteUnitPrice != DefaultSiteUnitPrice {
		t.Fatalf("default pricing: %+v %v", pricing, err)
	}
	if err := repo.SavePricing(ctx, PricingConfig{UserID: "user-1", AdminAccountID: "account-1", SiteUnitPrice: 0.85}); err != nil {
		t.Fatalf("save pricing: %v", err)
	}
	if err := repo.SavePricing(ctx, PricingConfig{UserID: "user-1", AdminAccountID: "missing", SiteUnitPrice: 3}); err != nil {
		t.Fatalf("save pricing for unknown workspace must be a no-op: %v", err)
	}
	if pricing, err := repo.GetPricing(ctx, "user-1", "account-1"); err != nil || pricing.SiteUnitPrice != 0.85 {
		t.Fatalf("saved pricing: %+v %v", pricing, err)
	}
	if pricing, err := repo.GetPricing(ctx, "user-1", "missing"); err != nil || pricing.SiteUnitPrice != DefaultSiteUnitPrice {
		t.Fatalf("pricing must not be stored for workspaces that do not exist: %+v %v", pricing, err)
	}
}
