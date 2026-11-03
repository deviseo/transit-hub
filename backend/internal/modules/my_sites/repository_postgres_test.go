package my_sites

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"transithub/backend/internal/modules/upstream"
)

// PostgreSQL 仓储测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中只写入合成数据，结束后整体删除。
func openMySitesTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("my_sites_test_%d", time.Now().UnixNano())
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

func TestRepositoryPostgresSavesSessionsOnFreshSchema(t *testing.T) {
	pool := openMySitesTestPool(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	for i := 0; i < 2; i++ {
		if err := repo.EnsureSchema(ctx); err != nil {
			t.Fatalf("ensure schema (run %d): %v", i+1, err)
		}
	}
	if state, err := repo.Get(ctx, "user-1", "account-1"); err != nil || state != nil {
		t.Fatalf("missing rows must return nil: %+v %v", state, err)
	}
	session := upstream.Session{Platform: upstream.PlatformNewAPI, BaseURL: "https://site.example.test", AccessToken: "token-1", QuotaPerUnit: 500000}
	if err := repo.Save(ctx, State{UserID: "user-1", AdminAccountID: "account-1", BaseURL: session.BaseURL, Email: "admin@example.test", Session: session}); err != nil {
		t.Fatalf("save: %v", err)
	}
	session.AccessToken = "token-2"
	if err := repo.Save(ctx, State{UserID: "user-1", AdminAccountID: "account-1", BaseURL: session.BaseURL, Email: "admin@example.test", Session: session}); err != nil {
		t.Fatalf("save again: %v", err)
	}
	state, err := repo.Get(ctx, "user-1", "account-1")
	if err != nil || state == nil || state.Session.AccessToken != "token-2" || state.Session.QuotaPerUnit != 500000 || state.Email != "admin@example.test" {
		t.Fatalf("unexpected state: %+v %v", state, err)
	}
	if other, err := repo.Get(ctx, "user-1", "account-2"); err != nil || other != nil {
		t.Fatalf("workspaces must stay isolated: %+v %v", other, err)
	}
}

// 旧版本的 my_site_states 带有已下线功能的 mappings / own_groups 列：保存会话时不能清空它们。
func TestRepositoryPostgresKeepsLegacyColumnsUntouched(t *testing.T) {
	pool := openMySitesTestPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE my_site_states (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			base_url text NOT NULL,
			email text NOT NULL,
			session jsonb NOT NULL,
			mappings jsonb NOT NULL DEFAULT '[]'::jsonb,
			own_groups jsonb NOT NULL DEFAULT '[]'::jsonb,
			updated_at timestamptz NOT NULL DEFAULT now()
		);
		CREATE UNIQUE INDEX idx_my_site_states_workspace ON my_site_states (user_id, admin_account_id);
		INSERT INTO my_site_states (user_id, admin_account_id, base_url, email, session, mappings, own_groups)
		VALUES ('user-1', 'account-1', 'https://old.example.test', 'old@example.test', '{"Platform":"sub2api","AccessToken":"old"}', '[{"ownGroup":"vip"}]', '[{"id":"1"}]');
	`); err != nil {
		t.Fatalf("create legacy table: %v", err)
	}
	repo := NewRepository(pool)
	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema on legacy table: %v", err)
	}
	session := upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://site.example.test", AccessToken: "new"}
	if err := repo.Save(ctx, State{UserID: "user-1", AdminAccountID: "account-1", BaseURL: session.BaseURL, Email: "admin@example.test", Session: session}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if err := repo.Save(ctx, State{UserID: "user-2", AdminAccountID: "account-9", BaseURL: session.BaseURL, Email: "other@example.test", Session: session}); err != nil {
		t.Fatalf("insert into legacy table: %v", err)
	}
	var mappings, ownGroups string
	if err := pool.QueryRow(ctx, `SELECT mappings::text, own_groups::text FROM my_site_states WHERE user_id = 'user-1'`).Scan(&mappings, &ownGroups); err != nil {
		t.Fatalf("read legacy columns: %v", err)
	}
	if mappings != `[{"ownGroup": "vip"}]` || ownGroups != `[{"id": "1"}]` {
		t.Fatalf("legacy columns must be preserved, got mappings=%s own_groups=%s", mappings, ownGroups)
	}
	state, err := repo.Get(ctx, "user-1", "account-1")
	if err != nil || state == nil || state.Session.AccessToken != "new" || state.BaseURL != "https://site.example.test" {
		t.Fatalf("unexpected state: %+v %v", state, err)
	}
}
