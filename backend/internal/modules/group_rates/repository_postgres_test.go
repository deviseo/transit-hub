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

	adminaccounts "transithub/backend/internal/modules/admin_accounts"
	mysites "transithub/backend/internal/modules/my_sites"
)

const postgresTestTimeout = 15 * time.Second

func TestRepositoryListMappedWithPostgres(t *testing.T) {
	tests := []struct {
		name            string
		snapshotGroupID string
		wantMapped      bool
		arrange         func(t *testing.T, pool *pgxpool.Pool)
	}{
		{
			name:            "stable ID survives renamed connection",
			snapshotGroupID: "54",
			wantMapped:      true,
			arrange: func(t *testing.T, pool *pgxpool.Pool) {
				insertRealConnection(t, pool, "workspace-a", "54", "old-name")
			},
		},
		{
			name:            "different non-empty IDs do not match by name",
			snapshotGroupID: "54",
			wantMapped:      false,
			arrange: func(t *testing.T, pool *pgxpool.Pool) {
				insertRealConnection(t, pool, "workspace-a", "55", "current-name")
			},
		},
		{
			name:            "legacy connection without ID falls back to name",
			snapshotGroupID: "54",
			wantMapped:      true,
			arrange: func(t *testing.T, pool *pgxpool.Pool) {
				insertRealConnection(t, pool, "workspace-a", "", "current-name")
			},
		},
		{
			name:            "legacy snapshot without ID falls back to name",
			snapshotGroupID: "",
			wantMapped:      true,
			arrange: func(t *testing.T, pool *pgxpool.Pool) {
				insertRealConnection(t, pool, "workspace-a", "54", "current-name")
			},
		},
		{
			name:            "other workspace is isolated",
			snapshotGroupID: "54",
			wantMapped:      false,
			arrange: func(t *testing.T, pool *pgxpool.Pool) {
				insertRealConnection(t, pool, "workspace-b", "54", "old-name")
				insertJSONMapping(t, pool, "workspace-b")
			},
		},
		{
			name:            "existing JSON mapping remains supported",
			snapshotGroupID: "54",
			wantMapped:      true,
			arrange: func(t *testing.T, pool *pgxpool.Pool) {
				insertJSONMapping(t, pool, "workspace-a")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			pool := openPostgresTestPool(t)
			repository := prepareGroupRatesRepository(t, pool)
			insertSnapshot(t, pool, "workspace-a", test.snapshotGroupID)
			test.arrange(t, pool)

			ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
			defer cancel()
			result, err := repository.List(ctx, "user-a", "workspace-a", ListQuery{Page: 1, PageSize: 10})
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if len(result.Items) != 1 {
				t.Fatalf("List returned %d items, want 1", len(result.Items))
			}
			if result.Items[0].Mapped != test.wantMapped {
				t.Fatalf("Mapped = %v, want %v", result.Items[0].Mapped, test.wantMapped)
			}
		})
	}
}

func TestRealConnectionIndexesExistInPostgres(t *testing.T) {
	pool := openPostgresTestPool(t)
	prepareGroupRatesRepository(t, pool)

	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	rows, err := pool.Query(ctx, `SELECT indexname, indexdef FROM pg_indexes WHERE schemaname = current_schema() AND tablename = 'real_connections'`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	defer rows.Close()

	indexes := map[string]string{}
	for rows.Next() {
		var name, definition string
		if err := rows.Scan(&name, &definition); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		indexes[name] = definition
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate indexes: %v", err)
	}
	assertIndexColumns(t, indexes, "idx_real_connections_workspace_group_id", "(user_id, workspace_admin_account_id, upstream_site_id, upstream_group_id)")
	assertIndexColumns(t, indexes, "idx_real_connections_workspace_group_name", "(user_id, workspace_admin_account_id, upstream_site_id, upstream_group_name)")
}

func TestLegacyRealConnectionWorkspaceIsAssignedInPostgres(t *testing.T) {
	pool := openPostgresTestPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()

	if _, err := pool.Exec(ctx, `CREATE TABLE users (id text PRIMARY KEY)`); err != nil {
		t.Fatalf("create users: %v", err)
	}
	createMinimalUpstreamSites(t, pool)
	if err := mysites.NewRepository(pool).EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure my_sites schema: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO users (id) VALUES ('user-a')`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	insertRealConnection(t, pool, "", "54", "current-name")

	if err := adminaccounts.NewRepository(pool).EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure admin_accounts schema: %v", err)
	}

	var workspaceID, legacyID, currentID string
	if err := pool.QueryRow(ctx, `SELECT workspace_admin_account_id FROM real_connections WHERE id = 'connection-a'`).Scan(&workspaceID); err != nil {
		t.Fatalf("read migrated connection: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM admin_accounts WHERE user_id = 'user-a' AND platform = 'legacy'`).Scan(&legacyID); err != nil {
		t.Fatalf("read legacy workspace: %v", err)
	}
	if err := pool.QueryRow(ctx, `SELECT current_admin_account_id FROM users WHERE id = 'user-a'`).Scan(&currentID); err != nil {
		t.Fatalf("read current workspace: %v", err)
	}
	if workspaceID == "" || workspaceID != legacyID || currentID != legacyID {
		t.Fatalf("legacy assignment connection=%q current=%q legacy=%q", workspaceID, currentID, legacyID)
	}

	repository := NewRepository(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure group_rates schema: %v", err)
	}
	insertSnapshot(t, pool, legacyID, "54")
	result, err := repository.List(ctx, "user-a", legacyID, ListQuery{Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("list migrated connection: %v", err)
	}
	if len(result.Items) != 1 || !result.Items[0].Mapped {
		t.Fatalf("migrated connection was not recognized: %#v", result.Items)
	}
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
	createMinimalUpstreamSites(t, pool)
	if err := mysites.NewRepository(pool).EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure my_sites schema: %v", err)
	}
	repository := NewRepository(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure group_rates schema: %v", err)
	}
	return repository
}

func createMinimalUpstreamSites(t *testing.T, pool *pgxpool.Pool) {
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
}

func insertSnapshot(t *testing.T, pool *pgxpool.Pool, workspaceID, groupID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO group_rate_snapshots (
			id, user_id, admin_account_id, site_id, site_name, group_id, group_name,
			platform, type, multiplier, created_at, deleted
		) VALUES ('snapshot-a', 'user-a', $1, 'site-a', 'Site A', $2, 'current-name', 'openai', 'chat', 0.015, now(), false)
	`, workspaceID, groupID)
	if err != nil {
		t.Fatalf("insert snapshot: %v", err)
	}
}

func insertRealConnection(t *testing.T, pool *pgxpool.Pool, workspaceID, groupID, groupName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO real_connections (
			id, user_id, workspace_admin_account_id, upstream_site_id, upstream_group_id,
			upstream_group_name, upstream_key_id, upstream_key, admin_account_id,
			admin_account_name, own_group_ids, group_type
		) VALUES ('connection-a', 'user-a', $1, 'site-a', $2, $3, 'key-a', 'secret-a', 'remote-a', 'Remote A', '[]'::jsonb, 'openai')
	`, workspaceID, groupID, groupName)
	if err != nil {
		t.Fatalf("insert real connection: %v", err)
	}
}

func insertJSONMapping(t *testing.T, pool *pgxpool.Pool, workspaceID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	_, err := pool.Exec(ctx, `
		INSERT INTO my_site_states (user_id, admin_account_id, base_url, email, session, mappings, own_groups)
		VALUES (
			'user-a', $1, 'https://admin.example', 'admin@example.com', '{}'::jsonb,
			'[{
				"ownGroup": "own-a",
				"upstreamTargets": [{"siteId": "site-a", "groupName": "current-name"}]
			}]'::jsonb,
			'[]'::jsonb
		)
	`, workspaceID)
	if err != nil {
		t.Fatalf("insert JSON mapping: %v", err)
	}
}

func assertIndexColumns(t *testing.T, indexes map[string]string, name, columns string) {
	t.Helper()
	definition, ok := indexes[name]
	if !ok {
		t.Fatalf("missing index %s", name)
	}
	if !strings.Contains(definition, columns) {
		t.Fatalf("index %s = %q, want columns %s", name, definition, columns)
	}
}
