package mass_email

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS mass_email_batches (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			admin_account_id text NOT NULL,
			request_id text NOT NULL,
			template_id text NOT NULL,
			template_name text NOT NULL,
			template_subject text NOT NULL,
			template_html text NOT NULL,
			selection_mode text NOT NULL,
			filters jsonb NOT NULL DEFAULT '{}'::jsonb,
			status text NOT NULL,
			recipient_count integer NOT NULL DEFAULT 0,
			skipped_count integer NOT NULL DEFAULT 0,
			sent_count integer NOT NULL DEFAULT 0,
			failed_count integer NOT NULL DEFAULT 0,
			uncertain_count integer NOT NULL DEFAULT 0,
			cancelled_count integer NOT NULL DEFAULT 0,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			started_at timestamptz,
			finished_at timestamptz,
			cancelled_at timestamptz
		)`,
		`CREATE TABLE IF NOT EXISTS mass_email_batch_items (
			id text PRIMARY KEY,
			batch_id text NOT NULL REFERENCES mass_email_batches(id) ON DELETE CASCADE,
			user_id text NOT NULL,
			admin_account_id text NOT NULL,
			upstream_user_id text NOT NULL,
			recipient_email text NOT NULL,
			normalized_email text NOT NULL,
			username text NOT NULL DEFAULT '',
			status text NOT NULL DEFAULT 'pending',
			error_key text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			claimed_at timestamptz,
			sent_at timestamptz,
			finished_at timestamptz
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_mass_email_batches_workspace_request ON mass_email_batches (user_id, admin_account_id, request_id)`,
		`CREATE INDEX IF NOT EXISTS idx_mass_email_batches_workspace_created ON mass_email_batches (user_id, admin_account_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_mass_email_batches_status ON mass_email_batches (status, updated_at)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_mass_email_items_batch_normalized_email ON mass_email_batch_items (batch_id, normalized_email)`,
		`CREATE INDEX IF NOT EXISTS idx_mass_email_items_batch_created ON mass_email_batch_items (batch_id, created_at ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_mass_email_items_pending_claim ON mass_email_batch_items (status, created_at) WHERE status = 'pending'`,
		`CREATE INDEX IF NOT EXISTS idx_mass_email_items_sending_claimed ON mass_email_batch_items (status, claimed_at) WHERE status = 'sending'`,
		`WITH ranked AS (
			SELECT id, row_number() OVER (PARTITION BY user_id, admin_account_id ORDER BY created_at ASC, id ASC) AS active_rank
			FROM mass_email_batches
			WHERE status IN ('queued', 'running', 'cancelling')
		), cancelled_batches AS (
			UPDATE mass_email_batches b
			SET status = 'cancelled', cancelled_at = COALESCE(cancelled_at, now()), finished_at = COALESCE(finished_at, now()), updated_at = now()
			FROM ranked r
			WHERE b.id = r.id AND r.active_rank > 1
			RETURNING b.id
		), cancelled_items AS (
			UPDATE mass_email_batch_items i
			SET status = 'cancelled', finished_at = COALESCE(finished_at, now()), updated_at = now()
			FROM cancelled_batches b
			WHERE i.batch_id = b.id AND i.status = 'pending'
			RETURNING i.batch_id
		)
		SELECT count(*) FROM cancelled_items`,
		`WITH counts AS (
			SELECT b.id,
				count(i.id) FILTER (WHERE i.status = 'sent') AS sent_count,
				count(i.id) FILTER (WHERE i.status = 'failed') AS failed_count,
				count(i.id) FILTER (WHERE i.status = 'uncertain') AS uncertain_count,
				count(i.id) FILTER (WHERE i.status = 'cancelled') AS cancelled_count
			FROM mass_email_batches b
			LEFT JOIN mass_email_batch_items i ON i.batch_id = b.id
			GROUP BY b.id
		)
		UPDATE mass_email_batches b
		SET sent_count = counts.sent_count, failed_count = counts.failed_count,
			uncertain_count = counts.uncertain_count, cancelled_count = counts.cancelled_count, updated_at = now()
		FROM counts
		WHERE b.id = counts.id`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_mass_email_batches_one_active_workspace ON mass_email_batches (user_id, admin_account_id) WHERE status IN ('queued', 'running', 'cancelling')`,
	}
	for _, statement := range statements {
		if _, err := r.db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) CreateBatch(ctx context.Context, batch Batch, items []BatchItem) (Batch, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Batch{}, false, err
	}
	defer tx.Rollback(ctx)

	filtersJSON, err := json.Marshal(batch.Filters)
	if err != nil {
		return Batch{}, false, err
	}
	row := tx.QueryRow(ctx, `
		INSERT INTO mass_email_batches (
			id, user_id, admin_account_id, request_id, template_id, template_name, template_subject,
			template_html, selection_mode, filters, status, recipient_count, skipped_count
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13)
		ON CONFLICT (user_id, admin_account_id, request_id) DO NOTHING
		RETURNING id, user_id, admin_account_id, request_id, template_id, template_name, template_subject,
			template_html, selection_mode, filters, status, recipient_count, skipped_count, sent_count,
			failed_count, uncertain_count, cancelled_count, created_at, updated_at, started_at, finished_at, cancelled_at
	`, batch.ID, batch.UserID, batch.AdminAccountID, batch.RequestID, batch.TemplateID, batch.TemplateName,
		batch.TemplateSubject, batch.TemplateHTML, string(batch.SelectionMode), string(filtersJSON), batch.Status,
		batch.RecipientCount, batch.SkippedCount)
	created, err := scanBatch(row)
	if err != nil {
		if isActiveBatchUniqueViolation(err) {
			return Batch{}, false, ErrActiveBatchExists
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return Batch{}, false, err
		}
		existing, getErr := r.GetByRequestID(ctx, batch.UserID, batch.AdminAccountID, batch.RequestID)
		return existing, false, getErr
	}
	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO mass_email_batch_items (
				id, batch_id, user_id, admin_account_id, upstream_user_id, recipient_email, normalized_email, username, status
			)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		`, item.ID, item.BatchID, item.UserID, item.AdminAccountID, item.UpstreamUserID, item.RecipientEmail,
			item.NormalizedEmail, item.Username, item.Status); err != nil {
			if isActiveBatchUniqueViolation(err) {
				return Batch{}, false, ErrActiveBatchExists
			}
			return Batch{}, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		if isActiveBatchUniqueViolation(err) {
			return Batch{}, false, ErrActiveBatchExists
		}
		return Batch{}, false, err
	}
	return created, true, nil
}

func (r *Repository) GetByRequestID(ctx context.Context, userID string, adminAccountID string, requestID string) (Batch, error) {
	return scanBatch(r.db.QueryRow(ctx, batchSelectSQL+` WHERE user_id = $1 AND admin_account_id = $2 AND request_id = $3`, userID, adminAccountID, requestID))
}

func (r *Repository) HasActiveBatch(ctx context.Context, userID string, adminAccountID string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM mass_email_batches
			WHERE user_id = $1 AND admin_account_id = $2 AND status IN ('queued', 'running', 'cancelling')
		)
	`, userID, adminAccountID).Scan(&exists)
	return exists, err
}

func (r *Repository) Get(ctx context.Context, userID string, adminAccountID string, id string) (Batch, error) {
	return scanBatch(r.db.QueryRow(ctx, batchSelectSQL+` WHERE user_id = $1 AND admin_account_id = $2 AND id = $3`, userID, adminAccountID, id))
}

func (r *Repository) List(ctx context.Context, userID string, adminAccountID string, page int, pageSize int) ([]Batch, int, error) {
	offset := (page - 1) * pageSize
	rows, err := r.db.Query(ctx, batchSelectSQL+`
		WHERE user_id = $1 AND admin_account_id = $2
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4
	`, userID, adminAccountID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	batches, err := scanBatches(rows)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM mass_email_batches WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID).Scan(&total); err != nil {
		return nil, 0, err
	}
	return batches, total, nil
}

func (r *Repository) ListItems(ctx context.Context, userID string, adminAccountID string, batchID string, page int, pageSize int) ([]BatchItem, int, error) {
	offset := (page - 1) * pageSize
	rows, err := r.db.Query(ctx, itemSelectSQL+`
		WHERE batch_id = $1 AND user_id = $2 AND admin_account_id = $3
		ORDER BY created_at ASC
		LIMIT $4 OFFSET $5
	`, batchID, userID, adminAccountID, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items, err := scanItems(rows)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM mass_email_batch_items WHERE batch_id = $1 AND user_id = $2 AND admin_account_id = $3`, batchID, userID, adminAccountID).Scan(&total); err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *Repository) CancelBatch(ctx context.Context, userID string, adminAccountID string, batchID string) (Batch, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Batch{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		UPDATE mass_email_batches
		SET status = CASE WHEN status IN ('queued','running') THEN 'cancelling' ELSE status END,
			cancelled_at = CASE WHEN status IN ('queued','running') THEN now() ELSE cancelled_at END,
			updated_at = now()
		WHERE id = $1 AND user_id = $2 AND admin_account_id = $3
	`, batchID, userID, adminAccountID); err != nil {
		return Batch{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE mass_email_batch_items
		SET status = 'cancelled', finished_at = now(), updated_at = now()
		WHERE batch_id = $1 AND user_id = $2 AND admin_account_id = $3 AND status = 'pending'
	`, batchID, userID, adminAccountID); err != nil {
		return Batch{}, err
	}
	if err := finalizeBatchInTx(ctx, tx, batchID); err != nil {
		return Batch{}, err
	}
	batch, err := scanBatch(tx.QueryRow(ctx, batchSelectSQL+` WHERE id = $1 AND user_id = $2 AND admin_account_id = $3`, batchID, userID, adminAccountID))
	if err != nil {
		return Batch{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Batch{}, err
	}
	return batch, nil
}

func (r *Repository) RecoverStaleSending(ctx context.Context, staleBefore time.Time) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE mass_email_batch_items
		SET status = 'uncertain', error_key = $2, finished_at = now(), updated_at = now()
		WHERE status = 'sending' AND claimed_at < $1
		RETURNING batch_id
	`, staleBefore, string(ErrSendFailed))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	seen := map[string]struct{}{}
	var batchIDs []string
	for rows.Next() {
		var batchID string
		if err := rows.Scan(&batchID); err != nil {
			return nil, err
		}
		if _, exists := seen[batchID]; exists {
			continue
		}
		seen[batchID] = struct{}{}
		batchIDs = append(batchIDs, batchID)
	}
	return batchIDs, rows.Err()
}

func (r *Repository) ClaimPendingItems(ctx context.Context, limit int) ([]BatchItem, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Lock order must match CancelBatch: parent batch rows first, then child item rows.
	// SKIP LOCKED lets workers move past a batch currently being cancelled without
	// waiting and without creating a batch/item lock cycle.
	batchRows, err := tx.Query(ctx, `
		SELECT b.id
		FROM mass_email_batches b
		WHERE b.status IN ('queued', 'running')
			AND EXISTS (
				SELECT 1
				FROM mass_email_batch_items i
				WHERE i.batch_id = b.id AND i.status = 'pending'
			)
		ORDER BY b.created_at ASC, b.id ASC
		FOR UPDATE OF b SKIP LOCKED
		LIMIT $1
	`, limit)
	if err != nil {
		return nil, err
	}
	var batchIDs []string
	for batchRows.Next() {
		var batchID string
		if err := batchRows.Scan(&batchID); err != nil {
			batchRows.Close()
			return nil, err
		}
		batchIDs = append(batchIDs, batchID)
	}
	batchRows.Close()
	if err := batchRows.Err(); err != nil {
		return nil, err
	}
	if len(batchIDs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return []BatchItem{}, nil
	}

	// With parent batches already locked, lock and claim pending child rows only from
	// those batches. The item LIMIT preserves workerConcurrency as the hard per-tick cap.
	rows, err := tx.Query(ctx, itemSelectSQL+`
		WHERE batch_id = ANY($1) AND status = 'pending'
		ORDER BY created_at ASC, id ASC
		FOR UPDATE OF mass_email_batch_items SKIP LOCKED
		LIMIT $2
	`, batchIDs, limit)
	if err != nil {
		return nil, err
	}
	items, err := scanItems(rows)
	rows.Close()
	if err != nil || len(items) == 0 {
		if err != nil {
			return items, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return items, nil
	}
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE mass_email_batch_items SET status = 'sending', claimed_at = now(), updated_at = now()
		WHERE id = ANY($1)
	`, ids); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE mass_email_batches SET status = 'running', started_at = COALESCE(started_at, now()), updated_at = now()
		WHERE id = ANY($1) AND status = 'queued'
	`, batchIDs); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	for i := range items {
		items[i].Status = ItemStatusSending
		now := time.Now()
		items[i].ClaimedAt = &now
	}
	return items, nil
}

func (r *Repository) GetBatchByID(ctx context.Context, id string) (Batch, error) {
	return scanBatch(r.db.QueryRow(ctx, batchSelectSQL+` WHERE id = $1`, id))
}

func (r *Repository) CompleteItem(ctx context.Context, itemID string, status string, errorKey string) error {
	return r.withItemBatchTx(ctx, itemID, func(tx pgx.Tx, batchID string) error {
		_, err := tx.Exec(ctx, `
			UPDATE mass_email_batch_items
			SET status = $2, error_key = $3, sent_at = CASE WHEN $2 = 'sent' THEN now() ELSE sent_at END,
				finished_at = now(), updated_at = now()
			WHERE id = $1 AND status = 'sending'
		`, itemID, status, errorKey)
		return err
	})
}

func (r *Repository) FinalizeBatch(ctx context.Context, batchID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := finalizeBatchInTx(ctx, tx, batchID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) withItemBatchTx(ctx context.Context, itemID string, fn func(pgx.Tx, string) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var batchID string
	if err := tx.QueryRow(ctx, `SELECT batch_id FROM mass_email_batch_items WHERE id = $1`, itemID).Scan(&batchID); err != nil {
		return err
	}
	if err := fn(tx, batchID); err != nil {
		return err
	}
	if err := finalizeBatchInTx(ctx, tx, batchID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func finalizeBatchInTx(ctx context.Context, tx pgx.Tx, batchID string) error {
	// 状态汇总与终态切换必须在同一事务里完成，避免 worker 并发完成最后几个 item 时把批次计数写回旧快照。
	_, err := tx.Exec(ctx, `
		WITH counts AS (
			SELECT
				count(*) FILTER (WHERE status = 'sent') AS sent_count,
				count(*) FILTER (WHERE status = 'failed') AS failed_count,
				count(*) FILTER (WHERE status = 'uncertain') AS uncertain_count,
				count(*) FILTER (WHERE status = 'cancelled') AS cancelled_count,
				count(*) FILTER (WHERE status IN ('pending','sending')) AS active_count
			FROM mass_email_batch_items
			WHERE batch_id = $1
		)
		UPDATE mass_email_batches b
		SET sent_count = counts.sent_count,
			failed_count = counts.failed_count,
			uncertain_count = counts.uncertain_count,
			cancelled_count = counts.cancelled_count,
			status = CASE
				WHEN counts.active_count > 0 AND b.status = 'cancelling' THEN 'cancelling'
				WHEN counts.active_count > 0 THEN b.status
				WHEN counts.cancelled_count > 0 AND counts.sent_count = 0 AND counts.failed_count = 0 AND counts.uncertain_count = 0 THEN 'cancelled'
				WHEN counts.failed_count > 0 OR counts.uncertain_count > 0 OR counts.cancelled_count > 0 THEN 'completed_with_errors'
				WHEN b.recipient_count = 0 THEN 'failed'
				ELSE 'completed'
			END,
			finished_at = CASE WHEN counts.active_count = 0 THEN COALESCE(b.finished_at, now()) ELSE b.finished_at END,
			updated_at = now()
		FROM counts
		WHERE b.id = $1
	`, batchID)
	return err
}

const batchSelectSQL = `
	SELECT id, user_id, admin_account_id, request_id, template_id, template_name, template_subject,
		template_html, selection_mode, filters, status, recipient_count, skipped_count, sent_count,
		failed_count, uncertain_count, cancelled_count, created_at, updated_at, started_at, finished_at, cancelled_at
	FROM mass_email_batches`

const itemSelectSQL = `
	SELECT id, batch_id, user_id, admin_account_id, upstream_user_id, recipient_email, normalized_email,
		username, status, error_key, created_at, updated_at, claimed_at, sent_at, finished_at
	FROM mass_email_batch_items`

type batchScanner interface{ Scan(dest ...any) error }

func scanBatch(row batchScanner) (Batch, error) {
	var batch Batch
	var selectionMode string
	var filtersJSON []byte
	if err := row.Scan(&batch.ID, &batch.UserID, &batch.AdminAccountID, &batch.RequestID, &batch.TemplateID,
		&batch.TemplateName, &batch.TemplateSubject, &batch.TemplateHTML, &selectionMode, &filtersJSON, &batch.Status,
		&batch.RecipientCount, &batch.SkippedCount, &batch.SentCount, &batch.FailedCount, &batch.UncertainCount,
		&batch.CancelledCount, &batch.CreatedAt, &batch.UpdatedAt, &batch.StartedAt, &batch.FinishedAt, &batch.CancelledAt); err != nil {
		return Batch{}, err
	}
	batch.SelectionMode = SelectionMode(selectionMode)
	_ = json.Unmarshal(filtersJSON, &batch.Filters)
	return batch, nil
}

func scanBatches(rows pgx.Rows) ([]Batch, error) {
	var batches []Batch
	for rows.Next() {
		batch, err := scanBatch(rows)
		if err != nil {
			return nil, err
		}
		batches = append(batches, batch)
	}
	return batches, rows.Err()
}

func scanItems(rows pgx.Rows) ([]BatchItem, error) {
	var items []BatchItem
	for rows.Next() {
		var item BatchItem
		if err := rows.Scan(&item.ID, &item.BatchID, &item.UserID, &item.AdminAccountID, &item.UpstreamUserID,
			&item.RecipientEmail, &item.NormalizedEmail, &item.Username, &item.Status, &item.ErrorKey, &item.CreatedAt,
			&item.UpdatedAt, &item.ClaimedAt, &item.SentAt, &item.FinishedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func isActiveBatchUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "idx_mass_email_batches_one_active_workspace"
}
