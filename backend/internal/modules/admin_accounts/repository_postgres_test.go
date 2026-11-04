package admin_accounts

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
func openAdminAccountsTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("admin_accounts_test_%d", time.Now().UnixNano())
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

// 新安装没有已下线功能的表，删除工作区不能因此失败；老安装里仍存在的这些表要按工作区清理干净。
func TestDeleteWorkspacePostgresHandlesRetiredFeatureTables(t *testing.T) {
	pool := openAdminAccountsTestPool(t)
	ctx := context.Background()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("exec %q: %v", sql, err)
		}
	}
	exec(`CREATE TABLE users (id text PRIMARY KEY, current_admin_account_id text NOT NULL DEFAULT '')`)
	exec(`CREATE TABLE admin_accounts (id text PRIMARY KEY, user_id text NOT NULL, updated_at timestamptz NOT NULL DEFAULT now())`)
	exec(`CREATE TABLE workspace_cleanup_jobs (
		id text PRIMARY KEY,
		user_id text NOT NULL,
		admin_account_id text NOT NULL,
		attachment_paths jsonb NOT NULL DEFAULT '[]'::jsonb,
		upstream_site_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
		attempts integer NOT NULL DEFAULT 0,
		next_attempt_at timestamptz NOT NULL DEFAULT now(),
		last_error text NOT NULL DEFAULT '',
		created_at timestamptz NOT NULL DEFAULT now(),
		updated_at timestamptz NOT NULL DEFAULT now()
	)`)
	for _, stmt := range workspaceDeleteStatements {
		exec(fmt.Sprintf(`CREATE TABLE %s (id text NOT NULL DEFAULT '', user_id text NOT NULL, admin_account_id text NOT NULL, storage_path text NOT NULL DEFAULT '')`, stmt.Name))
	}
	// 只建两张已下线功能的表，其余保持不存在（模拟新安装 + 老安装的混合情况）。
	exec(`CREATE TABLE ticket_embed_configs (user_id text NOT NULL, admin_account_id text NOT NULL)`)
	exec(`CREATE TABLE real_connections (user_id text NOT NULL, workspace_admin_account_id text NOT NULL)`)

	exec(`INSERT INTO users (id, current_admin_account_id) VALUES ('user-1', 'account-1')`)
	exec(`INSERT INTO admin_accounts (id, user_id, updated_at) VALUES ('account-1', 'user-1', now()), ('account-2', 'user-1', now() - interval '1 hour')`)
	for _, account := range []string{"account-1", "account-2"} {
		exec(`INSERT INTO upstream_sites (id, user_id, admin_account_id) VALUES ($1, 'user-1', $2)`, "site-"+account, account)
		exec(`INSERT INTO ticket_embed_configs (user_id, admin_account_id) VALUES ('user-1', $1)`, account)
		exec(`INSERT INTO ticket_attachments (user_id, admin_account_id, storage_path) VALUES ('user-1', $1, $2)`, account, "file-"+account+".png")
		exec(`INSERT INTO real_connections (user_id, workspace_admin_account_id) VALUES ('user-1', $1)`, account)
	}

	result, err := NewRepository(pool).DeleteWorkspace(ctx, "user-1", "account-1")
	if err != nil {
		t.Fatalf("delete workspace: %v", err)
	}
	if result == nil || result.DeletedID != "account-1" || !result.WasCurrent || result.CurrentAdminAccountID != "account-2" {
		t.Fatalf("unexpected delete result: %+v", result)
	}
	if len(result.UpstreamSiteIDs) != 1 || result.UpstreamSiteIDs[0] != "site-account-1" {
		t.Fatalf("unexpected upstream site ids: %+v", result.UpstreamSiteIDs)
	}
	if len(result.AttachmentPaths) != 1 || result.AttachmentPaths[0] != "file-account-1.png" {
		t.Fatalf("attachment files of the deleted workspace must be handed to cleanup: %+v", result.AttachmentPaths)
	}
	count := func(sql string) int {
		t.Helper()
		var n int
		if err := pool.QueryRow(ctx, sql).Scan(&n); err != nil {
			t.Fatalf("query %q: %v", sql, err)
		}
		return n
	}
	if n := count(`SELECT count(*) FROM ticket_embed_configs WHERE admin_account_id = 'account-1'`) + count(`SELECT count(*) FROM ticket_attachments WHERE admin_account_id = 'account-1'`); n != 0 {
		t.Fatalf("retired table rows of the deleted workspace must be removed, %d left", n)
	}
	if n := count(`SELECT count(*) FROM real_connections WHERE workspace_admin_account_id = 'account-1'`); n != 0 {
		t.Fatalf("real_connections rows of the deleted workspace must be removed, %d left", n)
	}
	if n := count(`SELECT count(*) FROM ticket_embed_configs WHERE admin_account_id = 'account-2'`) + count(`SELECT count(*) FROM real_connections WHERE workspace_admin_account_id = 'account-2'`) + count(`SELECT count(*) FROM upstream_sites WHERE admin_account_id = 'account-2'`) + count(`SELECT count(*) FROM ticket_attachments WHERE admin_account_id = 'account-2'`); n != 4 {
		t.Fatalf("other workspaces must keep their rows, got %d", n)
	}
	if n := count(`SELECT count(*) FROM admin_accounts WHERE id = 'account-1'`); n != 0 {
		t.Fatal("the workspace row must be deleted")
	}
	var current string
	if err := pool.QueryRow(ctx, `SELECT current_admin_account_id FROM users WHERE id = 'user-1'`).Scan(&current); err != nil || current != "account-2" {
		t.Fatalf("current workspace must move to account-2, got %q %v", current, err)
	}
	jobs, err := NewRepository(pool).ClaimDueCleanupJobs(ctx, 10)
	if err != nil || len(jobs) != 1 || jobs[0].AdminAccountID != "account-1" || len(jobs[0].UpstreamSiteIDs) != 1 ||
		len(jobs[0].AttachmentPaths) != 1 || jobs[0].AttachmentPaths[0] != "file-account-1.png" {
		t.Fatalf("unexpected cleanup jobs: %+v %v", jobs, err)
	}
}
