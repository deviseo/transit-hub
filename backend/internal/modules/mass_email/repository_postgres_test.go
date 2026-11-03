package mass_email

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgreSQL 仓储测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中只写入合成数据，结束后整体删除。

const postgresTestTimeout = 20 * time.Second

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
	schema := fmt.Sprintf("mass_email_test_%d", time.Now().UnixNano())
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

func applyMigrations(t *testing.T, pool *pgxpool.Pool, names ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	for _, name := range names {
		sql, err := os.ReadFile(filepath.Join("..", "..", "database", "migrations", name))
		if err != nil {
			t.Fatalf("read migration %s: %v", name, err)
		}
		if _, err := pool.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("apply migration %s: %v", name, err)
		}
	}
}

func TestRepositoryPostgresSegmentBatchesAndSuppressions(t *testing.T) {
	pool := openPostgresTestPool(t)
	applyMigrations(t, pool, "000005_create_mass_email_batches.sql", "000006_limit_active_mass_email_batches.sql", "000020_create_user_activity.sql")
	repo := NewRepository(pool)
	if err := repo.EnsureSchema(context.Background()); err != nil {
		t.Fatalf("ensure schema after migrations: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()

	batch := Batch{ID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", RequestID: "req-1", TemplateID: "tpl", TemplateName: "T",
		TemplateSubject: "S", TemplateHTML: "<p>x</p>", SelectionMode: SelectionModeSegment, Filters: BatchFilters{Segment: "inactive_7d", Status: "active"},
		PublicBaseURL: "https://th.example.test/api", Status: BatchStatusQueued, RecipientCount: 1}
	item := BatchItem{ID: "0123456789abcdef0123456789abcdef", BatchID: "batch-1", UserID: "user-1", AdminAccountID: "admin-1", UpstreamUserID: "7",
		RecipientEmail: "Alice@Example.test", NormalizedEmail: "alice@example.test", Username: "alice", Status: ItemStatusPending}
	created, inserted, err := repo.CreateBatch(ctx, batch, []BatchItem{item})
	if err != nil || !inserted {
		t.Fatalf("segment batches must satisfy the selection mode constraint: %v", err)
	}
	if created.SelectionMode != SelectionModeSegment || created.PublicBaseURL != batch.PublicBaseURL || created.Filters.Segment != "inactive_7d" {
		t.Fatalf("unexpected created batch: %+v", created)
	}
	loaded, err := repo.GetBatchByID(ctx, "batch-1")
	if err != nil || loaded.PublicBaseURL != batch.PublicBaseURL {
		t.Fatalf("public base url must round-trip: %+v %v", loaded, err)
	}
	bogus := batch
	bogus.ID, bogus.RequestID, bogus.SelectionMode, bogus.Status = "batch-2", "req-2", SelectionMode("bogus"), BatchStatusCompleted
	if _, _, err := repo.CreateBatch(ctx, bogus, nil); err == nil {
		t.Fatal("unknown selection modes must still be rejected by the constraint")
	}

	gotItem, err := repo.GetItemByID(ctx, item.ID)
	if err != nil || gotItem.RecipientEmail != item.RecipientEmail || gotItem.UserID != "user-1" {
		t.Fatalf("unexpected item: %+v %v", gotItem, err)
	}
	if _, err := repo.GetItemByID(ctx, strings.Repeat("f", 32)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing item must return ErrNotFound, got %v", err)
	}

	added, err := repo.AddSuppressions(ctx, "user-1", "admin-1", []string{"alice@example.test", "bob@example.test"}, SuppressionReasonManual)
	if err != nil || added != 2 {
		t.Fatalf("add suppressions = %d %v", added, err)
	}
	if added, err := repo.AddSuppressions(ctx, "user-1", "admin-1", []string{"alice@example.test"}, SuppressionReasonUnsubscribe); err != nil || added != 0 {
		t.Fatalf("re-adding must be a no-op: %d %v", added, err)
	}
	among, err := repo.SuppressedAmong(ctx, "user-1", "admin-1", []string{"alice@example.test", "carol@example.test"})
	if _, ok := among["alice@example.test"]; err != nil || len(among) != 1 || !ok {
		t.Fatalf("unexpected suppressed set: %v %v", among, err)
	}
	if suppressed, err := repo.IsSuppressed(ctx, "user-1", "admin-2", "alice@example.test"); err != nil || suppressed {
		t.Fatalf("suppressions are per workspace: %v %v", suppressed, err)
	}
	items, total, err := repo.ListSuppressions(ctx, "user-1", "admin-1", "alice@", 1, 20)
	if err != nil || total != 1 || len(items) != 1 || items[0].Email != "alice@example.test" || items[0].Reason != SuppressionReasonManual {
		t.Fatalf("unexpected search result: %+v total=%d %v", items, total, err)
	}
	if items, total, err := repo.ListSuppressions(ctx, "user-1", "admin-1", "%", 1, 20); err != nil || total != 0 || len(items) != 0 {
		t.Fatalf("LIKE wildcards in the search must be literal: %+v total=%d %v", items, total, err)
	}
	if items, total, err := repo.ListSuppressions(ctx, "user-1", "admin-1", "", 2, 20); err != nil || total != 2 || len(items) != 0 {
		t.Fatalf("out-of-range page must still report the total: %+v total=%d %v", items, total, err)
	}
	if removed, err := repo.RemoveSuppression(ctx, "user-1", "admin-1", "alice@example.test"); err != nil || !removed {
		t.Fatalf("remove = %v %v", removed, err)
	}
	if removed, err := repo.RemoveSuppression(ctx, "user-1", "admin-1", "alice@example.test"); err != nil || removed {
		t.Fatalf("second remove must report false: %v %v", removed, err)
	}
}
