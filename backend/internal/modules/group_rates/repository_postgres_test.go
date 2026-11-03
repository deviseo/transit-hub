package group_rates

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

const postgresTestTimeout = 15 * time.Second

// 分组倍率列表只读 group_rate_snapshots + upstream_sites：新安装里没有旧的对接 / 映射表也能正常查询。
func TestRepositoryListFiltersSortsAndCountsWithPostgres(t *testing.T) {
	pool := openPostgresTestPool(t)
	repository := prepareGroupRatesRepository(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()

	for _, table := range []string{"real_connections", "my_site_states"} {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&exists); err != nil || exists {
			t.Fatalf("test schema must not contain %s (exists=%v err=%v)", table, exists, err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO upstream_sites (id, user_id, admin_account_id, base_url, recharge_rate) VALUES
		('site-a', 'user-a', 'workspace-a', 'https://a.example.test', 2),
		('site-b', 'user-a', 'workspace-a', 'https://b.example.test', 1)`); err != nil {
		t.Fatalf("insert sites: %v", err)
	}
	now := time.Now()
	insertSnapshot(t, pool, snapshotRow{id: "a1-old", workspace: "workspace-a", site: "site-a", siteName: "Site A", groupID: "1", group: "claude-pro", platform: "sub2api", groupType: "anthropic", multiplier: 0.5, createdAt: now.Add(-2 * time.Hour)})
	insertSnapshot(t, pool, snapshotRow{id: "a1-new", workspace: "workspace-a", site: "site-a", siteName: "Site A", groupID: "1", group: "claude-pro", platform: "sub2api", groupType: "anthropic", multiplier: 0.6, createdAt: now.Add(-time.Hour)})
	insertSnapshot(t, pool, snapshotRow{id: "a2", workspace: "workspace-a", site: "site-a", siteName: "Site A", groupID: "2", group: "gpt-basic", platform: "sub2api", groupType: "openai", multiplier: 0.2, createdAt: now.Add(-time.Hour)})
	insertSnapshot(t, pool, snapshotRow{id: "b1", workspace: "workspace-a", site: "site-b", siteName: "Site B", groupID: "7", group: "default", platform: "newapi", multiplier: 1.5, createdAt: now.Add(-time.Hour)})
	insertSnapshot(t, pool, snapshotRow{id: "b2", workspace: "workspace-a", site: "site-b", siteName: "Site B", groupID: "8", group: "retired", platform: "newapi", multiplier: 0.1, createdAt: now.Add(-time.Hour), deleted: true})
	insertSnapshot(t, pool, snapshotRow{id: "other", workspace: "workspace-b", site: "site-a", siteName: "Site A", groupID: "1", group: "claude-pro", platform: "sub2api", groupType: "anthropic", multiplier: 9, createdAt: now})

	active, err := repository.List(ctx, "user-a", "workspace-a", ListQuery{Page: 1, PageSize: 10, Status: "all", Sort: "default"})
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	// 默认按实际倍率（上游倍率 × 充值倍率）升序：gpt-basic 0.4、claude-pro 1.2、default 1.5。
	if got := groupNames(active.Items); got != "gpt-basic,claude-pro,default" {
		t.Fatalf("unexpected default order: %s", got)
	}
	if active.Total != 3 || active.StatusCounts != (StatusCounts{All: 3, Deleted: 1}) {
		t.Fatalf("unexpected totals: total=%d counts=%+v", active.Total, active.StatusCounts)
	}
	if strings.Join(active.Types, ",") != "anthropic,openai" || strings.Join(active.Platforms, ",") != "newapi,sub2api" {
		t.Fatalf("unexpected facets: types=%v platforms=%v", active.Types, active.Platforms)
	}
	claude := active.Items[1]
	if claude.PreviousMultiplier == nil || *claude.PreviousMultiplier != 0.5 || claude.Multiplier != 0.6 || claude.RechargeRate != 2 {
		t.Fatalf("latest row must carry the previous multiplier and site recharge rate: %+v", claude)
	}

	deleted, err := repository.List(ctx, "user-a", "workspace-a", ListQuery{Page: 1, PageSize: 10, Status: "deleted", Sort: "default"})
	if err != nil || groupNames(deleted.Items) != "retired" || deleted.Total != 1 {
		t.Fatalf("unexpected deleted list: %+v %v", deleted.Items, err)
	}
	legacy, err := repository.List(ctx, "user-a", "workspace-a", ListQuery{Page: 1, PageSize: 10, Status: "legacy", Sort: "default"})
	if err != nil || legacy.Total != 4 {
		t.Fatalf("legacy status returns active and deleted rows together: total=%d err=%v", legacy.Total, err)
	}

	filtered, err := repository.List(ctx, "user-a", "workspace-a", ListQuery{Page: 1, PageSize: 10, Status: "all", Sort: "multiplierDesc", Platform: "sub2api"})
	if err != nil || groupNames(filtered.Items) != "claude-pro,gpt-basic" || filtered.StatusCounts != (StatusCounts{All: 2, Deleted: 0}) {
		t.Fatalf("unexpected platform filter: %+v %+v %v", filtered.Items, filtered.StatusCounts, err)
	}
	searched, err := repository.List(ctx, "user-a", "workspace-a", ListQuery{Page: 1, PageSize: 10, Status: "all", Sort: "default", Search: "SITE b"})
	if err != nil || groupNames(searched.Items) != "default" {
		t.Fatalf("unexpected search result: %+v %v", searched.Items, err)
	}
	typed, err := repository.List(ctx, "user-a", "workspace-a", ListQuery{Page: 1, PageSize: 10, Status: "all", Sort: "default", Type: "anthropic"})
	if err != nil || groupNames(typed.Items) != "claude-pro" {
		t.Fatalf("unexpected type filter: %+v %v", typed.Items, err)
	}
	paged, err := repository.List(ctx, "user-a", "workspace-a", ListQuery{Page: 2, PageSize: 2, Status: "all", Sort: "siteNameAsc"})
	if err != nil || len(paged.Items) != 1 || paged.Total != 3 {
		t.Fatalf("unexpected second page: %+v total=%d %v", paged.Items, paged.Total, err)
	}

	history, err := repository.History(ctx, "user-a", "workspace-a", "site-a", "1", "sub2api")
	if err != nil || len(history) != 2 || history[0].Multiplier != 0.6 || history[1].Multiplier != 0.5 {
		t.Fatalf("unexpected history: %+v %v", history, err)
	}
	if other, err := repository.List(ctx, "user-a", "workspace-b", ListQuery{Page: 1, PageSize: 10, Status: "all"}); err != nil || other.Total != 1 || other.Items[0].Multiplier != 9 {
		t.Fatalf("workspaces must stay isolated: %+v %v", other.Items, err)
	}
}

func TestServiceListMapsRowsWithPostgres(t *testing.T) {
	pool := openPostgresTestPool(t)
	repository := prepareGroupRatesRepository(t, pool)
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	if _, err := pool.Exec(ctx, `INSERT INTO upstream_sites (id, user_id, admin_account_id, base_url, recharge_rate) VALUES ('site-a', 'user-a', 'workspace-a', 'https://a.example.test', 2)`); err != nil {
		t.Fatalf("insert site: %v", err)
	}
	now := time.Now()
	insertSnapshot(t, pool, snapshotRow{id: "old", workspace: "workspace-a", site: "site-a", siteName: "Site A", groupID: "1", group: "vip", platform: "sub2api", multiplier: 0.5, createdAt: now.Add(-time.Hour)})
	insertSnapshot(t, pool, snapshotRow{id: "new", workspace: "workspace-a", site: "site-a", siteName: "Site A", groupID: "1", group: "vip", platform: "sub2api", multiplier: 0.75, createdAt: now})

	result, err := NewService(repository).List(ctx, "user-a", "workspace-a", ListQuery{Page: 1, PageSize: 30, Status: "all"})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("list: %+v %v", result, err)
	}
	row := result.Items[0]
	if row.UpstreamMultiplier != 0.75 || row.CurrentMultiplier != 1.5 || row.Delta == nil || *row.Delta != 0.25 || row.DeltaPercent == nil || *row.DeltaPercent != 50 {
		t.Fatalf("unexpected row: %+v", row)
	}
	if result.TotalPages != 1 || result.StatusCounts.All != 1 {
		t.Fatalf("unexpected paging: %+v", result)
	}
}

type snapshotRow struct {
	id, workspace, site, siteName, groupID, group, platform, groupType string
	multiplier                                                         float64
	createdAt                                                          time.Time
	deleted                                                            bool
}

func insertSnapshot(t *testing.T, pool *pgxpool.Pool, row snapshotRow) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO group_rate_snapshots (
			id, user_id, admin_account_id, site_id, site_name, group_id, group_name,
			platform, type, multiplier, created_at, last_seen_at, deleted
		) VALUES ($1, 'user-a', $2, $3, $4, $5, $6, $7, $8, $9, $10, $10, $11)
	`, row.id, row.workspace, row.site, row.siteName, row.groupID, row.group, row.platform, row.groupType, row.multiplier, row.createdAt, row.deleted)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
}

func groupNames(rows []snapshotRecord) string {
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		names = append(names, row.GroupName)
	}
	return strings.Join(names, ",")
}

func openPostgresTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	if err := adminPool.Ping(ctx); err != nil {
		adminPool.Close()
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	schema := fmt.Sprintf("group_rates_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}

	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		adminPool.Close()
		t.Fatalf("parse PostgreSQL config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		adminPool.Close()
		t.Fatalf("connect test schema: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		adminPool.Close()
		t.Fatalf("ping test schema: %v", err)
	}

	t.Cleanup(func() {
		pool.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), postgresTestTimeout)
		defer dropCancel()
		if _, err := adminPool.Exec(dropCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		adminPool.Close()
	})
	return pool
}

func prepareGroupRatesRepository(t *testing.T, pool *pgxpool.Pool) *Repository {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE upstream_sites (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			base_url text NOT NULL,
			recharge_rate double precision NOT NULL DEFAULT 1
		)
	`); err != nil {
		t.Fatalf("create upstream_sites: %v", err)
	}
	repository := NewRepository(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure group_rates schema: %v", err)
	}
	return repository
}
