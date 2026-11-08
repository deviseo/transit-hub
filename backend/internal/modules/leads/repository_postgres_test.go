package leads

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
	"transithub/backend/internal/modules/mass_email"
)

// PostgreSQL 测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中执行全部迁移并只写入合成数据，结束后整体删除。
func openLeadsTestPool(t *testing.T) *pgxpool.Pool {
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
	schema := fmt.Sprintf("leads_test_%d", time.Now().UnixNano())
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

func TestRepositoryStatusesFiltersAndRecipients(t *testing.T) {
	pool := openLeadsTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	ws := Workspace{UserID: "user-1", AdminAccountID: "acct-1"}
	other := Workspace{UserID: "user-1", AdminAccountID: "acct-2"}

	// minutesAgo 固定添加的先后顺序，排序结果可预期。
	insert := func(ws Workspace, id string, email string, source string, minutesAgo int) {
		t.Helper()
		created, err := repo.Insert(ctx, ws, newLead{ID: id, Email: email, Normalized: strings.ToLower(email), Source: source})
		if err != nil || !created {
			t.Fatalf("insert %s: %v %t", id, err, created)
		}
		if _, err := pool.Exec(ctx, `UPDATE leads SET created_at = now() - make_interval(mins => $2) WHERE id = $1`, id, minutesAgo); err != nil {
			t.Fatal(err)
		}
	}
	insert(ws, "lead-1", "Sent@Example.com", "V2EX", 40)
	insert(ws, "lead-2", "gone@example.com", "V2EX", 30)
	insert(ws, "lead-3", "unsub@example.com", "QQ 群", 20)
	insert(ws, "lead-4", "fresh@example.com", "", 10)
	insert(other, "lead-9", "sent@example.com", "V2EX", 5)

	if created, err := repo.Insert(ctx, ws, newLead{ID: "dup", Email: "SENT@example.com", Normalized: "sent@example.com"}); err != nil || created {
		t.Fatalf("the same email twice in one workspace must be skipped: %v %t", err, created)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO mass_email_batches (id, user_id, admin_account_id, request_id, template_id, template_name, template_subject, template_html, selection_mode, status)
		VALUES ('batch-1', 'user-1', 'acct-1', 'req-1', 'tpl', 'T', 'S', '<p>x</p>', 'leads', 'completed_with_errors')`); err != nil {
		t.Fatalf("insert batch: %v", err)
	}
	for _, item := range []struct{ id, lead, email, status, errorKey string }{
		{"item-1", "lead-1", "sent@example.com", "sent", ""},
		{"item-2", "lead-2", "gone@example.com", "failed", string(mass_email.ErrRecipientRejected)},
		{"item-3", "lead-4", "fresh@example.com", "cancelled", ""},
	} {
		sentAt := any(nil)
		if item.status == "sent" {
			sentAt = time.Now()
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO mass_email_batch_items (id, batch_id, user_id, admin_account_id, upstream_user_id, lead_id, recipient_email, normalized_email, status, error_key, sent_at)
			VALUES ($1, 'batch-1', 'user-1', 'acct-1', '', $2, $3, $3, $4, $5, $6)`,
			item.id, item.lead, item.email, item.status, item.errorKey, sentAt); err != nil {
			t.Fatalf("insert item: %v", err)
		}
	}
	if _, err := pool.Exec(ctx, `INSERT INTO email_suppressions (user_id, admin_account_id, email_normalized) VALUES ('user-1', 'acct-1', 'unsub@example.com')`); err != nil {
		t.Fatal(err)
	}

	counts, err := repo.Counts(ctx, ws, leadFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if counts != (StatusCounts{All: 4, NotSent: 1, Sent: 1, Unsubscribed: 1, Bounced: 1}) {
		t.Fatalf("counts = %+v", counts)
	}

	sent, err := repo.Page(ctx, ws, leadFilter{}, StatusSent, 20, 0)
	if err != nil || len(sent) != 1 || sent[0].ID != "lead-1" || sent[0].SentCount != 1 || sent[0].LastSentAt == nil {
		t.Fatalf("sent page: %+v %v", sent, err)
	}
	all, err := repo.Page(ctx, ws, leadFilter{}, "", 20, 0)
	if err != nil || len(all) != 4 || all[0].ID != "lead-4" || all[3].ID != "lead-1" {
		t.Fatalf("newest first: %+v %v", all, err)
	}
	if v2ex, _ := repo.Page(ctx, ws, leadFilter{source: "V2EX"}, "", 20, 0); len(v2ex) != 2 {
		t.Fatalf("source filter: %+v", v2ex)
	}
	if none, _ := repo.Page(ctx, ws, leadFilter{source: NoSource}, "", 20, 0); len(none) != 1 || none[0].ID != "lead-4" {
		t.Fatalf("no-source filter: %+v", none)
	}
	if found, _ := repo.Page(ctx, ws, leadFilter{search: "qq"}, "", 20, 0); len(found) != 1 || found[0].ID != "lead-3" {
		t.Fatalf("search matches the source too: %+v", found)
	}

	sendable, skipped, err := repo.Selection(ctx, ws, leadFilter{}, "")
	if err != nil || sendable != 2 || skipped != 2 {
		t.Fatalf("selection: %d %d %v", sendable, skipped, err)
	}
	recipients, err := repo.SendableRecipients(ctx, ws, leadFilter{ids: []string{"lead-1", "lead-2", "lead-3", "lead-4", "lead-9"}, byIDs: true}, "", 10)
	if err != nil || len(recipients) != 2 || recipients[0].LeadID != "lead-1" || recipients[1].LeadID != "lead-4" {
		t.Fatalf("only sent/not-sent leads of this workspace, oldest first: %+v %v", recipients, err)
	}

	if since, err := repo.SentSince(ctx, ws, time.Now().Add(-24*time.Hour)); err != nil || since != 2 {
		t.Fatalf("sent in 24h (cancelled excluded): %d %v", since, err)
	}

	added, err := repo.InsertMany(ctx, ws, "导入", []newLead{
		{ID: "lead-5", Email: "new1@example.com", Normalized: "new1@example.com"},
		{ID: "lead-6", Email: "fresh@example.com", Normalized: "fresh@example.com"},
	})
	if err != nil || added != 1 {
		t.Fatalf("insert many: %d %v", added, err)
	}
	suppressed, err := repo.Suppressed(ctx, ws, []string{"unsub@example.com", "new1@example.com"})
	if err != nil || len(suppressed) != 1 {
		t.Fatalf("suppressed: %+v %v", suppressed, err)
	}

	sources, err := repo.Sources(ctx, ws)
	if err != nil || len(sources) != 4 || sources[0] != (SourceCount{Source: "V2EX", Count: 2}) {
		t.Fatalf("sources: %+v %v", sources, err)
	}

	if err := repo.Update(ctx, ws, "lead-4", "GitHub", "README 里留的"); err != nil {
		t.Fatal(err)
	}
	if lead, err := repo.Get(ctx, ws, "lead-4"); err != nil || lead.Source != "GitHub" || lead.Note != "README 里留的" {
		t.Fatalf("updated: %+v %v", lead, err)
	}
	if err := repo.Update(ctx, other, "lead-4", "x", ""); err != ErrNotFound {
		t.Fatalf("other workspace can't edit: %v", err)
	}
	if deleted, err := repo.Delete(ctx, ws, []string{"lead-5", "lead-9"}); err != nil || deleted != 1 {
		t.Fatalf("delete only this workspace's leads: %d %v", deleted, err)
	}

	if limit, err := repo.DailyLimit(ctx, ws); err != nil || limit != defaultDailyLimit {
		t.Fatalf("default limit: %d %v", limit, err)
	}
	if err := repo.SetDailyLimit(ctx, ws, 50); err != nil {
		t.Fatal(err)
	}
	if limit, err := repo.DailyLimit(ctx, ws); err != nil || limit != 50 {
		t.Fatalf("saved limit: %d %v", limit, err)
	}
}
