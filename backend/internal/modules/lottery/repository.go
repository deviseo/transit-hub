package lottery

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct{ db *pgxpool.Pool }

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

func (r *Repository) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS lottery_embed_configs (user_id text NOT NULL, admin_account_id text NOT NULL DEFAULT '', embed_token text NOT NULL, sub2api_source_origin text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now())`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_embed_configs_workspace ON lottery_embed_configs (user_id, admin_account_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_embed_configs_token ON lottery_embed_configs (embed_token)`,
		`CREATE TABLE IF NOT EXISTS lottery_campaigns (id text PRIMARY KEY, user_id text NOT NULL, admin_account_id text NOT NULL DEFAULT '', name text NOT NULL, description text NOT NULL DEFAULT '', status text NOT NULL, registration_start timestamptz, registration_end timestamptz, draw_at timestamptz, draw_mode text NOT NULL, public_winners boolean NOT NULL DEFAULT false, seed_secret text NOT NULL DEFAULT '', seed_commitment text NOT NULL DEFAULT '', entry_snapshot_hash text NOT NULL DEFAULT '', revealed_seed text NOT NULL DEFAULT '', algorithm_version text NOT NULL DEFAULT 'lottery-hmac-sha256-v1', entry_count integer NOT NULL DEFAULT 0, winner_count integer NOT NULL DEFAULT 0, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), published_at timestamptz, opened_at timestamptz, closed_at timestamptz, drawn_at timestamptz, completed_at timestamptz, cancelled_at timestamptz)`,
		`CREATE TABLE IF NOT EXISTS lottery_prizes (id text PRIMARY KEY, campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE, user_id text NOT NULL, admin_account_id text NOT NULL DEFAULT '', type text NOT NULL, name text NOT NULL, quantity integer NOT NULL, sort_order integer NOT NULL DEFAULT 0, balance_amount numeric, group_id text NOT NULL DEFAULT '', group_name text NOT NULL DEFAULT '', multiplier numeric, validity_days integer, value_marker integer NOT NULL DEFAULT 1, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now())`,
		`ALTER TABLE lottery_prizes ADD COLUMN IF NOT EXISTS delivery_mode text NOT NULL DEFAULT 'sub2api_auto'`,
		`ALTER TABLE lottery_prizes ADD COLUMN IF NOT EXISTS manual_contact text NOT NULL DEFAULT ''`,
		`ALTER TABLE lottery_prizes ADD COLUMN IF NOT EXISTS voucher_codes text[] NOT NULL DEFAULT ARRAY[]::text[]`,
		`CREATE TABLE IF NOT EXISTS lottery_entries (id text PRIMARY KEY, campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE, user_id text NOT NULL, admin_account_id text NOT NULL DEFAULT '', sub2api_user_id text NOT NULL, masked_email text NOT NULL DEFAULT '', receipt_token text NOT NULL, receipt_hash text NOT NULL, status text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), withdrawn_at timestamptz)`,
		`CREATE TABLE IF NOT EXISTS lottery_draws (id text PRIMARY KEY, campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE, user_id text NOT NULL, admin_account_id text NOT NULL DEFAULT '', entry_snapshot_hash text NOT NULL, revealed_seed text NOT NULL, algorithm_version text NOT NULL, entry_count integer NOT NULL, winner_count integer NOT NULL, created_at timestamptz NOT NULL DEFAULT now())`,
		`CREATE TABLE IF NOT EXISTS lottery_winners (id text PRIMARY KEY, campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE, prize_id text NOT NULL REFERENCES lottery_prizes(id) ON DELETE CASCADE, draw_id text NOT NULL REFERENCES lottery_draws(id) ON DELETE CASCADE, user_id text NOT NULL, admin_account_id text NOT NULL DEFAULT '', entry_id text NOT NULL REFERENCES lottery_entries(id) ON DELETE CASCADE, sub2api_user_id text NOT NULL, masked_email text NOT NULL DEFAULT '', prize_slot integer NOT NULL, created_at timestamptz NOT NULL DEFAULT now())`,
		`CREATE TABLE IF NOT EXISTS lottery_reward_jobs (id text PRIMARY KEY, campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE, winner_id text NOT NULL REFERENCES lottery_winners(id) ON DELETE CASCADE, prize_id text NOT NULL REFERENCES lottery_prizes(id) ON DELETE CASCADE, user_id text NOT NULL, admin_account_id text NOT NULL DEFAULT '', status text NOT NULL, attempt_count integer NOT NULL DEFAULT 0, next_attempt_at timestamptz NOT NULL DEFAULT now(), locked_at timestamptz, error_key text NOT NULL DEFAULT '', error_detail text NOT NULL DEFAULT '', remote_reference text NOT NULL DEFAULT '', idempotency_key text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), fulfilled_at timestamptz)`,
		`ALTER TABLE lottery_reward_jobs ADD COLUMN IF NOT EXISTS rate_cleanup_status text NOT NULL DEFAULT ''`,
		`ALTER TABLE lottery_reward_jobs ADD COLUMN IF NOT EXISTS rate_cleanup_at timestamptz`,
		`ALTER TABLE lottery_reward_jobs ADD COLUMN IF NOT EXISTS rate_cleanup_attempt_count integer NOT NULL DEFAULT 0`,
		`ALTER TABLE lottery_reward_jobs ADD COLUMN IF NOT EXISTS rate_cleanup_next_attempt_at timestamptz`,
		`ALTER TABLE lottery_reward_jobs ADD COLUMN IF NOT EXISTS rate_cleanup_locked_at timestamptz`,
		`ALTER TABLE lottery_reward_jobs ADD COLUMN IF NOT EXISTS rate_cleanup_error_detail text NOT NULL DEFAULT ''`,
		`ALTER TABLE lottery_reward_jobs ADD COLUMN IF NOT EXISTS rate_cleanup_completed_at timestamptz`,
		`CREATE TABLE IF NOT EXISTS lottery_audit_logs (id text PRIMARY KEY, campaign_id text NOT NULL DEFAULT '', user_id text NOT NULL, admin_account_id text NOT NULL DEFAULT '', actor_type text NOT NULL, actor_id text NOT NULL DEFAULT '', event text NOT NULL, detail jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now())`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_workspace ON lottery_campaigns (user_id, admin_account_id, status, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_prizes_campaign ON lottery_prizes (campaign_id, sort_order, id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_entries_campaign_user ON lottery_entries (campaign_id, sub2api_user_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_entries_receipt_hash ON lottery_entries (receipt_hash)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_draws_campaign ON lottery_draws (campaign_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_winners_campaign_user ON lottery_winners (campaign_id, sub2api_user_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_reward_jobs_winner_prize ON lottery_reward_jobs (winner_id, prize_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_reward_jobs_idempotency ON lottery_reward_jobs (idempotency_key)`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_due_open ON lottery_campaigns (status, registration_start) WHERE status = 'scheduled'`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_due_close ON lottery_campaigns (status, registration_end) WHERE status = 'open'`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_due_draw ON lottery_campaigns (status, draw_mode, draw_at) WHERE status = 'closed'`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_reward_jobs_claim ON lottery_reward_jobs (status, next_attempt_at, locked_at)`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_reward_jobs_rate_cleanup_claim ON lottery_reward_jobs (rate_cleanup_status, rate_cleanup_next_attempt_at, rate_cleanup_locked_at) WHERE rate_cleanup_status IN ('pending','processing','retryable_failed')`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_reward_jobs_rate_cleanup_target ON lottery_reward_jobs (user_id, admin_account_id, rate_cleanup_status, rate_cleanup_at DESC) WHERE rate_cleanup_status IN ('pending','processing','retryable_failed')`,
		`CREATE INDEX IF NOT EXISTS idx_lottery_audit_logs_campaign ON lottery_audit_logs (campaign_id, created_at DESC)`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_campaigns_status_check' AND conrelid = 'lottery_campaigns'::regclass) THEN ALTER TABLE lottery_campaigns ADD CONSTRAINT lottery_campaigns_status_check CHECK (status IN ('draft','scheduled','open','closed','drawing','drawn','fulfilling','completed','partial','cancelled')) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_campaigns_draw_mode_check' AND conrelid = 'lottery_campaigns'::regclass) THEN ALTER TABLE lottery_campaigns ADD CONSTRAINT lottery_campaigns_draw_mode_check CHECK (draw_mode IN ('manual','scheduled')) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_campaigns_entry_count_check' AND conrelid = 'lottery_campaigns'::regclass) THEN ALTER TABLE lottery_campaigns ADD CONSTRAINT lottery_campaigns_entry_count_check CHECK (entry_count >= 0) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_campaigns_winner_count_check' AND conrelid = 'lottery_campaigns'::regclass) THEN ALTER TABLE lottery_campaigns ADD CONSTRAINT lottery_campaigns_winner_count_check CHECK (winner_count >= 0) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_prizes_type_check' AND conrelid = 'lottery_prizes'::regclass) THEN ALTER TABLE lottery_prizes ADD CONSTRAINT lottery_prizes_type_check CHECK (type IN ('balance','subscription')) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_prizes_quantity_check' AND conrelid = 'lottery_prizes'::regclass) THEN ALTER TABLE lottery_prizes ADD CONSTRAINT lottery_prizes_quantity_check CHECK (quantity > 0) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_prizes_balance_amount_check' AND conrelid = 'lottery_prizes'::regclass) THEN ALTER TABLE lottery_prizes ADD CONSTRAINT lottery_prizes_balance_amount_check CHECK (balance_amount IS NULL OR balance_amount > 0) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_prizes_validity_days_check' AND conrelid = 'lottery_prizes'::regclass) THEN ALTER TABLE lottery_prizes ADD CONSTRAINT lottery_prizes_validity_days_check CHECK (validity_days IS NULL OR (validity_days >= 1 AND validity_days <= 36500)) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_prizes_value_marker_check' AND conrelid = 'lottery_prizes'::regclass) THEN ALTER TABLE lottery_prizes ADD CONSTRAINT lottery_prizes_value_marker_check CHECK (value_marker = 1) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_prizes_delivery_mode_check' AND conrelid = 'lottery_prizes'::regclass) THEN ALTER TABLE lottery_prizes ADD CONSTRAINT lottery_prizes_delivery_mode_check CHECK (delivery_mode IN ('sub2api_auto','voucher','manual')) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_prizes_check' AND conrelid = 'lottery_prizes'::regclass) THEN ALTER TABLE lottery_prizes ADD CONSTRAINT lottery_prizes_check CHECK ((type = 'balance' AND balance_amount IS NOT NULL AND group_id = '' AND validity_days IS NULL) OR (type = 'subscription' AND balance_amount IS NULL AND group_id <> '' AND validity_days IS NOT NULL)) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_entries_status_check' AND conrelid = 'lottery_entries'::regclass) THEN ALTER TABLE lottery_entries ADD CONSTRAINT lottery_entries_status_check CHECK (status IN ('active','withdrawn')) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_draws_entry_count_check' AND conrelid = 'lottery_draws'::regclass) THEN ALTER TABLE lottery_draws ADD CONSTRAINT lottery_draws_entry_count_check CHECK (entry_count >= 0) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_draws_winner_count_check' AND conrelid = 'lottery_draws'::regclass) THEN ALTER TABLE lottery_draws ADD CONSTRAINT lottery_draws_winner_count_check CHECK (winner_count >= 0) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_reward_jobs_status_check' AND conrelid = 'lottery_reward_jobs'::regclass) THEN ALTER TABLE lottery_reward_jobs ADD CONSTRAINT lottery_reward_jobs_status_check CHECK (status IN ('pending','processing','fulfilled','retryable_failed','manual_attention','failed')) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_reward_jobs_attempt_count_check' AND conrelid = 'lottery_reward_jobs'::regclass) THEN ALTER TABLE lottery_reward_jobs ADD CONSTRAINT lottery_reward_jobs_attempt_count_check CHECK (attempt_count >= 0) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_reward_jobs_rate_cleanup_status_check' AND conrelid = 'lottery_reward_jobs'::regclass) THEN ALTER TABLE lottery_reward_jobs ADD CONSTRAINT lottery_reward_jobs_rate_cleanup_status_check CHECK (rate_cleanup_status IN ('','pending','processing','retryable_failed','completed')) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_reward_jobs_rate_cleanup_attempt_count_check' AND conrelid = 'lottery_reward_jobs'::regclass) THEN ALTER TABLE lottery_reward_jobs ADD CONSTRAINT lottery_reward_jobs_rate_cleanup_attempt_count_check CHECK (rate_cleanup_attempt_count >= 0) NOT VALID; END IF; END $$`,
		`DO $$ BEGIN IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lottery_audit_logs_actor_type_check' AND conrelid = 'lottery_audit_logs'::regclass) THEN ALTER TABLE lottery_audit_logs ADD CONSTRAINT lottery_audit_logs_actor_type_check CHECK (actor_type IN ('admin','embed','worker','scheduler','system')) NOT VALID; END IF; END $$`,
	}
	for _, stmt := range statements {
		if _, err := r.db.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) GetEmbedConfigByToken(ctx context.Context, token string) (*EmbedConfig, error) {
	return scanEmbedConfig(r.db.QueryRow(ctx, `SELECT user_id, admin_account_id, embed_token, sub2api_source_origin, created_at, updated_at FROM lottery_embed_configs WHERE embed_token = $1`, token))
}

func (r *Repository) GetEmbedConfigByWorkspace(ctx context.Context, userID, adminAccountID string) (*EmbedConfig, error) {
	return scanEmbedConfig(r.db.QueryRow(ctx, `SELECT user_id, admin_account_id, embed_token, sub2api_source_origin, created_at, updated_at FROM lottery_embed_configs WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID))
}

func (r *Repository) InsertEmbedConfig(ctx context.Context, config EmbedConfig) error {
	_, err := r.db.Exec(ctx, `INSERT INTO lottery_embed_configs (user_id, admin_account_id, embed_token, sub2api_source_origin) VALUES ($1,$2,$3,$4) ON CONFLICT (user_id, admin_account_id) DO NOTHING`, config.UserID, config.AdminAccountID, config.EmbedToken, config.Sub2apiSourceOrigin)
	return err
}

func (r *Repository) UpdateEmbedConfig(ctx context.Context, userID, adminAccountID, origin string) error {
	_, err := r.db.Exec(ctx, `UPDATE lottery_embed_configs SET sub2api_source_origin = $3, updated_at = now() WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID, origin)
	return err
}

func (r *Repository) RotateEmbedToken(ctx context.Context, userID, adminAccountID, token string) error {
	_, err := r.db.Exec(ctx, `UPDATE lottery_embed_configs SET embed_token = $3, updated_at = now() WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID, token)
	return err
}

func (r *Repository) CreateCampaign(ctx context.Context, c Campaign, prizes []Prize) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := insertCampaign(ctx, tx, c); err != nil {
		return err
	}
	if err := replacePrizes(ctx, tx, c.ID, prizes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) UpdateDraftCampaign(ctx context.Context, c Campaign, prizes []Prize) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE lottery_campaigns SET name=$4, description=$5, registration_start=$6, registration_end=$7, draw_at=$8, draw_mode=$9, public_winners=$10, updated_at=now() WHERE user_id=$1 AND admin_account_id=$2 AND id=$3 AND status='draft'`, c.UserID, c.AdminAccountID, c.ID, c.Name, c.Description, c.RegistrationStart, c.RegistrationEnd, c.DrawAt, c.DrawMode, c.PublicWinners)
	if err != nil || tag.RowsAffected() == 0 {
		if err != nil {
			return err
		}
		return requestError(ErrorInvalidState)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM lottery_prizes WHERE campaign_id=$1`, c.ID); err != nil {
		return err
	}
	if err := replacePrizes(ctx, tx, c.ID, prizes); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListCampaigns(ctx context.Context, userID, adminAccountID string) ([]Campaign, error) {
	rows, err := r.db.Query(ctx, `SELECT `+campaignColumns+` FROM lottery_campaigns WHERE user_id=$1 AND admin_account_id=$2 ORDER BY created_at DESC`, userID, adminAccountID)
	return scanCampaignRows(rows, err)
}

func (r *Repository) ListEmbedCampaigns(ctx context.Context, userID, adminAccountID string) ([]Campaign, error) {
	rows, err := r.db.Query(ctx, `SELECT `+campaignColumns+` FROM lottery_campaigns WHERE user_id=$1 AND admin_account_id=$2 AND status IN ('scheduled','open','closed','drawing','drawn','fulfilling','completed','partial') ORDER BY created_at DESC, id DESC`, userID, adminAccountID)
	return scanCampaignRows(rows, err)
}

func (r *Repository) GetCampaign(ctx context.Context, userID, adminAccountID, id string) (*Campaign, error) {
	return scanCampaign(r.db.QueryRow(ctx, `SELECT `+campaignColumns+` FROM lottery_campaigns WHERE user_id=$1 AND admin_account_id=$2 AND id=$3`, userID, adminAccountID, id))
}

func (r *Repository) GetCampaignByID(ctx context.Context, id string) (*Campaign, error) {
	return scanCampaign(r.db.QueryRow(ctx, `SELECT `+campaignColumns+` FROM lottery_campaigns WHERE id=$1`, id))
}

func (r *Repository) ListPrizes(ctx context.Context, campaignID string) ([]Prize, error) {
	rows, err := r.db.Query(ctx, `SELECT `+prizeColumns+` FROM lottery_prizes WHERE campaign_id=$1 ORDER BY sort_order ASC, id ASC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prizes := []Prize{}
	for rows.Next() {
		p, err := scanPrize(rows)
		if err != nil {
			return nil, err
		}
		prizes = append(prizes, *p)
	}
	return prizes, rows.Err()
}

func (r *Repository) InsertEntry(ctx context.Context, entry Entry) (*Entry, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	// Lock the campaign before accepting the entry. Closing registration and
	// drawing winners touch the same row, so no entry can land outside the
	// committed public snapshot during a lifecycle transition.
	var campaignStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM lottery_campaigns WHERE id=$1 FOR UPDATE`, entry.CampaignID).Scan(&campaignStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, requestError(ErrorNotFound)
		}
		return nil, err
	}
	if campaignStatus != StatusOpen {
		return nil, requestError(ErrorEmbedCampaignNotOpen)
	}
	_, err = tx.Exec(ctx, `INSERT INTO lottery_entries (id,campaign_id,user_id,admin_account_id,sub2api_user_id,masked_email,receipt_token,receipt_hash,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, entry.ID, entry.CampaignID, entry.UserID, entry.AdminAccountID, entry.Sub2apiUserID, entry.MaskedEmail, entry.ReceiptToken, entry.ReceiptHash, entry.Status)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, requestError(ErrorAlreadyEntered)
		}
		return nil, err
	}
	// Keep the list-card count current before the draw snapshot is created.
	if _, err := tx.Exec(ctx, `UPDATE lottery_campaigns SET entry_count=(SELECT count(*) FROM lottery_entries WHERE campaign_id=$1 AND status='active'), updated_at=now() WHERE id=$1`, entry.CampaignID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r.GetEntryByUser(ctx, entry.CampaignID, entry.Sub2apiUserID)
}

func (r *Repository) GetEntryByUser(ctx context.Context, campaignID, sub2apiUserID string) (*Entry, error) {
	return scanEntry(r.db.QueryRow(ctx, `SELECT id,campaign_id,user_id,admin_account_id,sub2api_user_id,masked_email,receipt_token,receipt_hash,status,created_at,updated_at,withdrawn_at FROM lottery_entries WHERE campaign_id=$1 AND sub2api_user_id=$2`, campaignID, sub2apiUserID))
}

func (r *Repository) GetViewerCampaignState(ctx context.Context, campaignID, sub2apiUserID string) (*Entry, *Winner, *MyRewardStatus, error) {
	entry, err := r.GetEntryByUser(ctx, campaignID, sub2apiUserID)
	if err != nil || entry == nil {
		return entry, nil, nil, err
	}
	winner, err := scanWinner(r.db.QueryRow(ctx, `SELECT w.id,w.campaign_id,w.prize_id,w.draw_id,w.user_id,w.admin_account_id,w.entry_id,w.sub2api_user_id,w.masked_email,w.prize_slot,w.created_at FROM lottery_winners w INNER JOIN lottery_entries e ON e.id=w.entry_id WHERE w.campaign_id=$1 AND e.campaign_id=$1 AND e.sub2api_user_id=$2`, campaignID, sub2apiUserID))
	if err != nil || winner == nil {
		return entry, winner, nil, err
	}
	// Delivery secrets are selected only after the winner is bound to the current
	// Sub2API viewer. Automatic upstream references are never exposed as vouchers.
	reward, err := scanMyRewardStatus(r.db.QueryRow(ctx, `SELECT j.id,j.winner_id,j.prize_id,j.status,j.error_key,p.delivery_mode,CASE WHEN p.delivery_mode='voucher' AND j.status='fulfilled' THEN j.remote_reference ELSE '' END,CASE WHEN p.delivery_mode='manual' THEN p.manual_contact ELSE '' END FROM lottery_reward_jobs j INNER JOIN lottery_winners w ON w.id=j.winner_id INNER JOIN lottery_entries e ON e.id=w.entry_id INNER JOIN lottery_prizes p ON p.id=j.prize_id WHERE j.campaign_id=$1 AND w.campaign_id=$1 AND e.campaign_id=$1 AND e.sub2api_user_id=$2 ORDER BY j.created_at ASC LIMIT 1`, campaignID, sub2apiUserID))
	return entry, winner, reward, err
}

func (r *Repository) ListEntries(ctx context.Context, campaignID string, activeOnly bool) ([]Entry, error) {
	query := `SELECT id,campaign_id,user_id,admin_account_id,sub2api_user_id,masked_email,receipt_token,receipt_hash,status,created_at,updated_at,withdrawn_at FROM lottery_entries WHERE campaign_id=$1`
	if activeOnly {
		query += ` AND status='active'`
	}
	query += ` ORDER BY created_at ASC, id ASC`
	rows, err := r.db.Query(ctx, query, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	entries := []Entry{}
	for rows.Next() {
		entry, err := scanEntryRows(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (r *Repository) WithdrawEntry(ctx context.Context, campaignID, sub2apiUserID string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var campaignStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM lottery_campaigns WHERE id=$1 FOR UPDATE`, campaignID).Scan(&campaignStatus); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return requestError(ErrorNotFound)
		}
		return err
	}
	if campaignStatus != StatusOpen {
		return requestError(ErrorEmbedCampaignNotOpen)
	}
	tag, err := tx.Exec(ctx, `UPDATE lottery_entries SET status='withdrawn', withdrawn_at=now(), updated_at=now() WHERE campaign_id=$1 AND sub2api_user_id=$2 AND status='active'`, campaignID, sub2apiUserID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return requestError(ErrorEmbedEntryNotFound)
	}
	if _, err := tx.Exec(ctx, `UPDATE lottery_campaigns SET entry_count=(SELECT count(*) FROM lottery_entries WHERE campaign_id=$1 AND status='active'), updated_at=now() WHERE id=$1`, campaignID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) AppendAudit(ctx context.Context, log AuditLog) error {
	detail, err := json.Marshal(log.Detail)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `INSERT INTO lottery_audit_logs (id,campaign_id,user_id,admin_account_id,actor_type,actor_id,event,detail) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`, log.ID, log.CampaignID, log.UserID, log.AdminAccountID, log.ActorType, log.ActorID, log.Event, detail)
	return err
}

func (r *Repository) DrawCampaign(ctx context.Context, userID, adminAccountID, campaignID string) (*Campaign, []Winner, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	campaign, err := scanCampaign(tx.QueryRow(ctx, `SELECT `+campaignColumns+` FROM lottery_campaigns WHERE user_id=$1 AND admin_account_id=$2 AND id=$3 FOR UPDATE`, userID, adminAccountID, campaignID))
	if err != nil || campaign == nil {
		if err == nil {
			err = requestError(ErrorNotFound)
		}
		return campaign, nil, err
	}
	if campaign.Status == StatusDrawn || campaign.Status == StatusFulfilling || campaign.Status == StatusCompleted || campaign.Status == StatusPartial {
		winners, err := listWinnersTx(ctx, tx, campaignID)
		return campaign, winners, err
	}
	if campaign.Status != StatusClosed {
		return nil, nil, requestError(ErrorInvalidState)
	}
	if _, err := tx.Exec(ctx, `UPDATE lottery_campaigns SET status='drawing', updated_at=now() WHERE id=$1`, campaignID); err != nil {
		return nil, nil, err
	}
	entries, err := listEntriesTx(ctx, tx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	prizes, err := listPrizesTx(ctx, tx, campaignID)
	if err != nil {
		return nil, nil, err
	}
	algorithmVersion := campaign.AlgorithmVersion
	if algorithmVersion == "" {
		algorithmVersion = AlgorithmVersionV1
	}
	hash, err := snapshotHashForVersion(entries, algorithmVersion)
	if err != nil {
		return nil, nil, err
	}
	shuffled, err := deterministicShuffle(entries, finalSeed(campaign.SeedSecret, campaign.ID, hash))
	if err != nil {
		return nil, nil, err
	}
	slots := expandedPrizeSlots(prizes)
	if len(slots) > len(shuffled) {
		slots = slots[:len(shuffled)]
	}
	drawID, err := newID("ldraw")
	if err != nil {
		return nil, nil, err
	}
	revealed := campaign.SeedSecret
	if _, err := tx.Exec(ctx, `INSERT INTO lottery_draws (id,campaign_id,user_id,admin_account_id,entry_snapshot_hash,revealed_seed,algorithm_version,entry_count,winner_count) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`, drawID, campaign.ID, campaign.UserID, campaign.AdminAccountID, hash, revealed, algorithmVersion, len(entries), len(slots)); err != nil {
		return nil, nil, err
	}
	winners := make([]Winner, 0, len(slots))
	prizeSlotIndexes := make(map[string]int, len(prizes))
	hasAutomaticRewards := false
	hasManualRewards := false
	for i, prize := range slots {
		winnerID, err := newID("lwin")
		if err != nil {
			return nil, nil, err
		}
		jobID, err := newID("lrwd")
		if err != nil {
			return nil, nil, err
		}
		winner := Winner{ID: winnerID, CampaignID: campaign.ID, PrizeID: prize.ID, DrawID: drawID, UserID: campaign.UserID, AdminAccountID: campaign.AdminAccountID, EntryID: shuffled[i].ID, Sub2apiUserID: shuffled[i].Sub2apiUserID, MaskedEmail: shuffled[i].MaskedEmail, PrizeSlot: i + 1}
		if _, err := tx.Exec(ctx, `INSERT INTO lottery_winners (id,campaign_id,prize_id,draw_id,user_id,admin_account_id,entry_id,sub2api_user_id,masked_email,prize_slot) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, winner.ID, winner.CampaignID, winner.PrizeID, winner.DrawID, winner.UserID, winner.AdminAccountID, winner.EntryID, winner.Sub2apiUserID, winner.MaskedEmail, winner.PrizeSlot); err != nil {
			return nil, nil, err
		}
		localSlot := prizeSlotIndexes[prize.ID]
		prizeSlotIndexes[prize.ID] = localSlot + 1
		rewardStatus, errorKey, remoteReference, err := rewardDeliveryForSlot(prize, localSlot)
		if err != nil {
			return nil, nil, err
		}
		hasAutomaticRewards = hasAutomaticRewards || rewardStatus == RewardPending
		hasManualRewards = hasManualRewards || rewardStatus == RewardManualAttention
		idempotencyKey := "th-lottery-" + jobID
		if _, err := tx.Exec(ctx, `INSERT INTO lottery_reward_jobs (id,campaign_id,winner_id,prize_id,user_id,admin_account_id,status,error_key,remote_reference,idempotency_key,fulfilled_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,CASE WHEN $7='fulfilled' THEN now() ELSE NULL END)`, jobID, campaign.ID, winner.ID, prize.ID, campaign.UserID, campaign.AdminAccountID, rewardStatus, errorKey, remoteReference, idempotencyKey); err != nil {
			return nil, nil, err
		}
		winners = append(winners, winner)
	}
	status := StatusDrawn
	if hasAutomaticRewards {
		status = StatusFulfilling
	} else if hasManualRewards {
		status = StatusPartial
	} else if len(winners) > 0 {
		status = StatusCompleted
	}
	if _, err := tx.Exec(ctx, `UPDATE lottery_campaigns SET status=$2, entry_snapshot_hash=$3, revealed_seed=$4, algorithm_version=$5, entry_count=$6, winner_count=$7, drawn_at=now(), completed_at=CASE WHEN $2 IN ('completed','partial') THEN now() ELSE NULL END, updated_at=now() WHERE id=$1`, campaign.ID, status, hash, revealed, algorithmVersion, len(entries), len(winners)); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, nil, err
	}
	campaign.EntrySnapshotHash = hash
	campaign.RevealedSeed = revealed
	campaign.AlgorithmVersion = algorithmVersion
	campaign.Status = status
	campaign.EntryCount = len(entries)
	campaign.WinnerCount = len(winners)
	return campaign, winners, nil
}

// rewardDeliveryForSlot turns a configured prize unit into an immutable reward
// job state. Voucher codes are assigned in configured order, while manual prizes
// deliberately stay out of the automatic Sub2API worker queue.
func rewardDeliveryForSlot(prize Prize, localSlot int) (status, errorKey, remoteReference string, err error) {
	switch prize.DeliveryMode {
	case "", DeliverySub2APIAuto:
		return RewardPending, "", "", nil
	case DeliveryVoucher:
		if localSlot < 0 || localSlot >= len(prize.VoucherCodes) {
			return "", "", "", requestError(ErrorValidation)
		}
		return RewardFulfilled, "", prize.VoucherCodes[localSlot], nil
	case DeliveryManual:
		return RewardManualAttention, ErrorRewardManualRequired, "", nil
	default:
		return "", "", "", requestError(ErrorValidation)
	}
}

func (r *Repository) SetCampaignStatus(ctx context.Context, userID, adminAccountID, id, from, to string) error {
	tag, err := r.db.Exec(ctx, `UPDATE lottery_campaigns SET status=$5, updated_at=now(), opened_at=CASE WHEN $5='open' THEN now() ELSE opened_at END, closed_at=CASE WHEN $5='closed' THEN now() ELSE closed_at END, cancelled_at=CASE WHEN $5='cancelled' THEN now() ELSE cancelled_at END WHERE user_id=$1 AND admin_account_id=$2 AND id=$3 AND status=$4`, userID, adminAccountID, id, from, to)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return requestError(ErrorInvalidState)
	}
	return nil
}

func (r *Repository) PublishCampaign(ctx context.Context, userID, adminAccountID, id, status, secret, commitment string) error {
	tag, err := r.db.Exec(ctx, `UPDATE lottery_campaigns SET status=$4, seed_secret=$5, seed_commitment=$6, published_at=now(), opened_at=CASE WHEN $4='open' THEN now() ELSE opened_at END, updated_at=now() WHERE user_id=$1 AND admin_account_id=$2 AND id=$3 AND status='draft'`, userID, adminAccountID, id, status, secret, commitment)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return requestError(ErrorInvalidState)
	}
	return nil
}

func (r *Repository) ListDueOpen(ctx context.Context, now time.Time) ([]string, error) {
	return r.listIDs(ctx, `SELECT id FROM lottery_campaigns WHERE status='scheduled' AND registration_start IS NOT NULL AND registration_start <= $1`, now)
}
func (r *Repository) ListDueClose(ctx context.Context, now time.Time) ([]string, error) {
	return r.listIDs(ctx, `SELECT id FROM lottery_campaigns WHERE status='open' AND registration_end IS NOT NULL AND registration_end <= $1`, now)
}
func (r *Repository) ListDueDraw(ctx context.Context, now time.Time) ([]string, error) {
	return r.listIDs(ctx, `SELECT id FROM lottery_campaigns WHERE status='closed' AND draw_mode='scheduled' AND draw_at IS NOT NULL AND draw_at <= $1`, now)
}

func (r *Repository) listIDs(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) ClaimRewardJobs(ctx context.Context, limit int, staleAfter time.Duration) ([]RewardJob, error) {
	staleBefore := rewardJobStaleCutoff(staleAfter, time.Now())
	rows, err := r.db.Query(ctx, `WITH picked AS (SELECT id FROM lottery_reward_jobs WHERE (status IN ('pending','retryable_failed') AND next_attempt_at <= now()) OR (status='processing' AND locked_at < $2) ORDER BY next_attempt_at ASC LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE lottery_reward_jobs j SET status='processing', locked_at=now(), attempt_count=attempt_count+1, updated_at=now() FROM picked WHERE j.id=picked.id RETURNING j.id,j.campaign_id,j.winner_id,j.prize_id,j.user_id,j.admin_account_id,j.status,j.attempt_count,j.next_attempt_at,j.locked_at,j.error_key,j.error_detail,j.remote_reference,j.idempotency_key,j.created_at,j.updated_at,j.fulfilled_at`, limit, staleBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := []RewardJob{}
	for rows.Next() {
		job, err := scanRewardJobRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range jobs {
		winner, err := r.GetWinner(ctx, jobs[i].WinnerID)
		if err != nil {
			return nil, err
		}
		if winner == nil {
			return nil, fmt.Errorf("lottery reward job %s missing winner %s", jobs[i].ID, jobs[i].WinnerID)
		}
		prize, err := r.GetPrize(ctx, jobs[i].PrizeID)
		if err != nil {
			return nil, err
		}
		if prize == nil {
			return nil, fmt.Errorf("lottery reward job %s missing prize %s", jobs[i].ID, jobs[i].PrizeID)
		}
		jobs[i], err = completeClaimedRewardJobData(jobs[i], winner, prize)
		if err != nil {
			return nil, err
		}
	}
	return jobs, nil
}

func completeClaimedRewardJobData(job RewardJob, winner *Winner, prize *Prize) (RewardJob, error) {
	if winner == nil {
		return RewardJob{}, fmt.Errorf("lottery reward job %s missing winner %s", job.ID, job.WinnerID)
	}
	if prize == nil {
		return RewardJob{}, fmt.Errorf("lottery reward job %s missing prize %s", job.ID, job.PrizeID)
	}
	job.Winner = *winner
	job.Prize = *prize
	return job, nil
}

func rewardJobStaleCutoff(staleAfter time.Duration, now time.Time) time.Time {
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}
	return now.Add(-staleAfter)
}

func (r *Repository) CompleteRewardJob(ctx context.Context, id, status, errorKey, detail, remoteRef string, next time.Time, rateCleanupAt *time.Time) error {
	// 奖励完成与专属倍率清理计划在同一条 UPDATE 中落库。远端设置成功后即使进程退出，
	// 下次启动仍能从 rate_cleanup_next_attempt_at 恢复到期清理，不依赖内存定时器。
	_, err := r.db.Exec(ctx, `UPDATE lottery_reward_jobs SET status=$2, error_key=$3, error_detail=$4, remote_reference=$5, next_attempt_at=$6, locked_at=NULL, fulfilled_at=CASE WHEN $2='fulfilled' THEN now() ELSE fulfilled_at END, rate_cleanup_status=CASE WHEN $2='fulfilled' AND $7::timestamptz IS NOT NULL THEN 'pending' ELSE rate_cleanup_status END, rate_cleanup_at=CASE WHEN $2='fulfilled' AND $7::timestamptz IS NOT NULL THEN $7 ELSE rate_cleanup_at END, rate_cleanup_next_attempt_at=CASE WHEN $2='fulfilled' AND $7::timestamptz IS NOT NULL THEN $7 ELSE rate_cleanup_next_attempt_at END, rate_cleanup_locked_at=CASE WHEN $2='fulfilled' AND $7::timestamptz IS NOT NULL THEN NULL ELSE rate_cleanup_locked_at END, rate_cleanup_error_detail=CASE WHEN $2='fulfilled' AND $7::timestamptz IS NOT NULL THEN '' ELSE rate_cleanup_error_detail END, updated_at=now() WHERE id=$1`, id, status, errorKey, detail, remoteRef, next, rateCleanupAt)
	return err
}

// ClaimRateCleanupJobs 领取已经到期或需要重试的专属倍率清理任务。processing 超时任务会
// 被重新领取，以覆盖实例在发出上游请求前后异常退出的情况；远端 PUT/null 本身可安全重试。
func (r *Repository) ClaimRateCleanupJobs(ctx context.Context, limit int, staleAfter time.Duration) ([]RateCleanupJob, error) {
	staleBefore := rewardJobStaleCutoff(staleAfter, time.Now())
	rows, err := r.db.Query(ctx, `WITH picked AS (SELECT id FROM lottery_reward_jobs WHERE (rate_cleanup_status IN ('pending','retryable_failed') AND rate_cleanup_next_attempt_at <= now()) OR (rate_cleanup_status='processing' AND rate_cleanup_locked_at < $2) ORDER BY rate_cleanup_next_attempt_at ASC LIMIT $1 FOR UPDATE SKIP LOCKED) UPDATE lottery_reward_jobs j SET rate_cleanup_status='processing', rate_cleanup_locked_at=now(), rate_cleanup_attempt_count=rate_cleanup_attempt_count+1, updated_at=now() FROM picked WHERE j.id=picked.id RETURNING j.id,j.campaign_id,j.winner_id,j.prize_id,j.user_id,j.admin_account_id,j.status,j.attempt_count,j.next_attempt_at,j.locked_at,j.error_key,j.error_detail,j.remote_reference,j.idempotency_key,j.created_at,j.updated_at,j.fulfilled_at,j.rate_cleanup_attempt_count,j.rate_cleanup_at`, limit, staleBefore)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]RateCleanupJob, 0)
	for rows.Next() {
		var job RateCleanupJob
		if err := rows.Scan(&job.ID, &job.CampaignID, &job.WinnerID, &job.PrizeID, &job.UserID, &job.AdminAccountID, &job.Status, &job.AttemptCount, &job.NextAttemptAt, &job.LockedAt, &job.ErrorKey, &job.ErrorDetail, &job.RemoteRef, &job.IdempotencyKey, &job.CreatedAt, &job.UpdatedAt, &job.FulfilledAt, &job.CleanupAttemptCount, &job.CleanupAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range jobs {
		winner, err := r.GetWinner(ctx, jobs[i].WinnerID)
		if err != nil {
			return nil, err
		}
		prize, err := r.GetPrize(ctx, jobs[i].PrizeID)
		if err != nil {
			return nil, err
		}
		completed, err := completeClaimedRewardJobData(jobs[i].RewardJob, winner, prize)
		if err != nil {
			return nil, err
		}
		jobs[i].RewardJob = completed
	}
	return jobs, nil
}

// FindRateCleanupReplacement 查找同一工作区、同一 Sub2API 用户和分组仍未到期的
// 最新倍率奖励。存在时清理旧奖励应恢复该倍率，防止重叠奖励被较早到期任务误删。
func (r *Repository) FindRateCleanupReplacement(ctx context.Context, job RateCleanupJob, now time.Time) (*RateCleanupReplacement, error) {
	var replacement RateCleanupReplacement
	err := r.db.QueryRow(ctx, `SELECT candidate.id,COALESCE(prize.multiplier::text,''),candidate.rate_cleanup_at FROM lottery_reward_jobs candidate JOIN lottery_winners winner ON winner.id=candidate.winner_id JOIN lottery_prizes prize ON prize.id=candidate.prize_id WHERE candidate.id<>$1 AND candidate.user_id=$2 AND candidate.admin_account_id=$3 AND winner.sub2api_user_id=$4 AND prize.type='subscription' AND prize.group_id=$5 AND candidate.status='fulfilled' AND candidate.rate_cleanup_status='pending' AND candidate.rate_cleanup_at>$6 ORDER BY candidate.fulfilled_at DESC,candidate.id DESC LIMIT 1`, job.ID, job.UserID, job.AdminAccountID, job.Winner.Sub2apiUserID, job.Prize.GroupID, now).Scan(&replacement.RewardJobID, &replacement.Multiplier, &replacement.CleanupAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &replacement, nil
}

// CompleteRateCleanup 将清理任务写成可重试或已完成。已完成包含三种等价结果：远端
// 删除成功、倍率早已不存在、用户或分组已经删除；这些情况都不再影响活动奖励状态。
func (r *Repository) CompleteRateCleanup(ctx context.Context, id, status, detail string, next time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE lottery_reward_jobs SET rate_cleanup_status=$2, rate_cleanup_error_detail=$3, rate_cleanup_next_attempt_at=$4, rate_cleanup_locked_at=NULL, rate_cleanup_completed_at=CASE WHEN $2='completed' THEN now() ELSE rate_cleanup_completed_at END, updated_at=now() WHERE id=$1`, id, status, detail, next)
	return err
}

func (r *Repository) CompleteManualRewardJob(ctx context.Context, userID, adminAccountID, id string) (string, error) {
	var campaignID string
	err := r.db.QueryRow(ctx, `UPDATE lottery_reward_jobs SET status='fulfilled', error_key='', error_detail='', locked_at=NULL, fulfilled_at=now(), updated_at=now() WHERE user_id=$1 AND admin_account_id=$2 AND id=$3 AND status='manual_attention' RETURNING campaign_id`, userID, adminAccountID, id).Scan(&campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", requestError(ErrorInvalidState)
	}
	return campaignID, err
}

func (r *Repository) RetryRewardJob(ctx context.Context, userID, adminAccountID, id string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var campaignID string
	err = tx.QueryRow(ctx, `UPDATE lottery_reward_jobs AS j SET status='pending', next_attempt_at=now(), locked_at=NULL, error_key='', error_detail='', updated_at=now() FROM lottery_prizes p WHERE j.prize_id=p.id AND p.delivery_mode='sub2api_auto' AND j.user_id=$1 AND j.admin_account_id=$2 AND j.id=$3 AND j.status IN ('retryable_failed','manual_attention','failed') RETURNING j.campaign_id`, userID, adminAccountID, id).Scan(&campaignID)
	if errors.Is(err, pgx.ErrNoRows) {
		return requestError(ErrorInvalidState)
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE lottery_campaigns SET status='fulfilling', completed_at=NULL, updated_at=now() WHERE user_id=$1 AND admin_account_id=$2 AND id=$3 AND status='partial'`, userID, adminAccountID, campaignID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListRewardStatuses(ctx context.Context, campaignID string) ([]RewardStatus, error) {
	rows, err := r.db.Query(ctx, `SELECT id,winner_id,prize_id,status,error_key,error_detail FROM lottery_reward_jobs WHERE campaign_id=$1 ORDER BY created_at ASC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []RewardStatus{}
	for rows.Next() {
		var item RewardStatus
		if err := rows.Scan(&item.ID, &item.WinnerID, &item.PrizeID, &item.Status, &item.ErrorKey, &item.ErrorDetail); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) ListAuditLogs(ctx context.Context, campaignID string) ([]AuditLog, error) {
	rows, err := r.db.Query(ctx, `SELECT id,campaign_id,user_id,admin_account_id,actor_type,actor_id,event,detail,created_at FROM lottery_audit_logs WHERE campaign_id=$1 ORDER BY created_at DESC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []AuditLog{}
	for rows.Next() {
		var item AuditLog
		var detail []byte
		if err := rows.Scan(&item.ID, &item.CampaignID, &item.UserID, &item.AdminAccountID, &item.ActorType, &item.ActorID, &item.Event, &detail, &item.CreatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(detail, &item.Detail)
		items = append(items, item)
	}
	return items, rows.Err()
}

func (r *Repository) FinalizeCampaignRewards(ctx context.Context, campaignID string) error {
	// A retryable failure is no longer active delivery work. Keeping it in
	// fulfilling made unrelated-looking historical campaigns appear stuck as a
	// group whenever their workspace session expired. Each campaign is finalized
	// exclusively from reward jobs carrying its own campaign_id.
	_, err := r.db.Exec(ctx, `UPDATE lottery_campaigns SET status=CASE WHEN EXISTS (SELECT 1 FROM lottery_reward_jobs WHERE campaign_id=$1 AND status IN ('retryable_failed','manual_attention','failed')) THEN 'partial' ELSE 'completed' END, completed_at=now(), updated_at=now() WHERE id=$1 AND status IN ('fulfilling','partial') AND NOT EXISTS (SELECT 1 FROM lottery_reward_jobs WHERE campaign_id=$1 AND status IN ('pending','processing') )`, campaignID)
	return err
}

func (r *Repository) ReconcileRewardCampaignStatuses(ctx context.Context) error {
	// This periodic reconciliation also repairs a process interrupted after its
	// last reward job was written but before the campaign summary was updated.
	_, err := r.db.Exec(ctx, `UPDATE lottery_campaigns AS campaign SET status=CASE WHEN EXISTS (SELECT 1 FROM lottery_reward_jobs AS job WHERE job.campaign_id=campaign.id AND job.status IN ('retryable_failed','manual_attention','failed')) THEN 'partial' ELSE 'completed' END, completed_at=now(), updated_at=now() WHERE campaign.status IN ('fulfilling','partial') AND EXISTS (SELECT 1 FROM lottery_reward_jobs AS job WHERE job.campaign_id=campaign.id) AND NOT EXISTS (SELECT 1 FROM lottery_reward_jobs AS job WHERE job.campaign_id=campaign.id AND job.status IN ('pending','processing'))`)
	return err
}

func (r *Repository) GetWinner(ctx context.Context, id string) (*Winner, error) {
	return scanWinner(r.db.QueryRow(ctx, `SELECT id,campaign_id,prize_id,draw_id,user_id,admin_account_id,entry_id,sub2api_user_id,masked_email,prize_slot,created_at FROM lottery_winners WHERE id=$1`, id))
}

func (r *Repository) ListWinners(ctx context.Context, campaignID string) ([]Winner, error) {
	rows, err := r.db.Query(ctx, `SELECT id,campaign_id,prize_id,draw_id,user_id,admin_account_id,entry_id,sub2api_user_id,masked_email,prize_slot,created_at FROM lottery_winners WHERE campaign_id=$1 ORDER BY prize_slot ASC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	winners := []Winner{}
	for rows.Next() {
		winner, err := scanWinner(rows)
		if err != nil {
			return nil, err
		}
		winners = append(winners, *winner)
	}
	return winners, rows.Err()
}

func (r *Repository) GetPrize(ctx context.Context, id string) (*Prize, error) {
	return scanPrize(r.db.QueryRow(ctx, `SELECT `+prizeColumns+` FROM lottery_prizes WHERE id=$1`, id))
}

const campaignColumns = `id,user_id,admin_account_id,name,description,status,registration_start,registration_end,draw_at,draw_mode,public_winners,seed_secret,seed_commitment,entry_snapshot_hash,revealed_seed,algorithm_version,entry_count,winner_count,created_at,updated_at,published_at,opened_at,closed_at,drawn_at,completed_at,cancelled_at`
const prizeColumns = `id,campaign_id,user_id,admin_account_id,type,name,quantity,sort_order,COALESCE(balance_amount::text,''),group_id,group_name,COALESCE(multiplier::text,''),validity_days,delivery_mode,manual_contact,voucher_codes,value_marker`

func insertCampaign(ctx context.Context, tx pgx.Tx, c Campaign) error {
	_, err := tx.Exec(ctx, `INSERT INTO lottery_campaigns (`+campaignColumns+`) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,now(),now(),$19,$20,$21,$22,$23,$24)`, c.ID, c.UserID, c.AdminAccountID, c.Name, c.Description, c.Status, c.RegistrationStart, c.RegistrationEnd, c.DrawAt, c.DrawMode, c.PublicWinners, c.SeedSecret, c.SeedCommitment, c.EntrySnapshotHash, c.RevealedSeed, c.AlgorithmVersion, c.EntryCount, c.WinnerCount, c.PublishedAt, c.OpenedAt, c.ClosedAt, c.DrawnAt, c.CompletedAt, c.CancelledAt)
	return err
}

func replacePrizes(ctx context.Context, tx pgx.Tx, campaignID string, prizes []Prize) error {
	for _, p := range prizes {
		_, err := tx.Exec(ctx, `INSERT INTO lottery_prizes (id,campaign_id,user_id,admin_account_id,type,name,quantity,sort_order,balance_amount,group_id,group_name,multiplier,validity_days,delivery_mode,manual_contact,voucher_codes,value_marker) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::numeric,$10,$11,NULLIF($12,'')::numeric,$13,$14,$15,$16,$17)`, p.ID, campaignID, p.UserID, p.AdminAccountID, p.Type, p.Name, p.Quantity, p.SortOrder, p.BalanceAmount, p.GroupID, p.GroupName, p.Multiplier, p.ValidityDays, p.DeliveryMode, p.ManualContact, p.VoucherCodes, p.ValueMarker)
		if err != nil {
			return err
		}
	}
	return nil
}

func scanEmbedConfig(row pgx.Row) (*EmbedConfig, error) {
	var c EmbedConfig
	if err := row.Scan(&c.UserID, &c.AdminAccountID, &c.EmbedToken, &c.Sub2apiSourceOrigin, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}
func scanCampaign(row pgx.Row) (*Campaign, error) {
	var c Campaign
	if err := row.Scan(&c.ID, &c.UserID, &c.AdminAccountID, &c.Name, &c.Description, &c.Status, &c.RegistrationStart, &c.RegistrationEnd, &c.DrawAt, &c.DrawMode, &c.PublicWinners, &c.SeedSecret, &c.SeedCommitment, &c.EntrySnapshotHash, &c.RevealedSeed, &c.AlgorithmVersion, &c.EntryCount, &c.WinnerCount, &c.CreatedAt, &c.UpdatedAt, &c.PublishedAt, &c.OpenedAt, &c.ClosedAt, &c.DrawnAt, &c.CompletedAt, &c.CancelledAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}
func scanCampaignRows(rows pgx.Rows, err error) ([]Campaign, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Campaign{}
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *c)
	}
	return items, rows.Err()
}
func scanEntry(row pgx.Row) (*Entry, error) {
	e, err := scanEntryRows(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}
func scanEntryRows(row pgx.Row) (Entry, error) {
	var e Entry
	err := row.Scan(&e.ID, &e.CampaignID, &e.UserID, &e.AdminAccountID, &e.Sub2apiUserID, &e.MaskedEmail, &e.ReceiptToken, &e.ReceiptHash, &e.Status, &e.CreatedAt, &e.UpdatedAt, &e.WithdrawnAt)
	return e, err
}
func scanWinner(row pgx.Row) (*Winner, error) {
	var w Winner
	if err := row.Scan(&w.ID, &w.CampaignID, &w.PrizeID, &w.DrawID, &w.UserID, &w.AdminAccountID, &w.EntryID, &w.Sub2apiUserID, &w.MaskedEmail, &w.PrizeSlot, &w.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &w, nil
}
func scanRewardJobRows(row pgx.Row) (RewardJob, error) {
	var j RewardJob
	err := row.Scan(&j.ID, &j.CampaignID, &j.WinnerID, &j.PrizeID, &j.UserID, &j.AdminAccountID, &j.Status, &j.AttemptCount, &j.NextAttemptAt, &j.LockedAt, &j.ErrorKey, &j.ErrorDetail, &j.RemoteRef, &j.IdempotencyKey, &j.CreatedAt, &j.UpdatedAt, &j.FulfilledAt)
	return j, err
}

func scanMyRewardStatus(row pgx.Row) (*MyRewardStatus, error) {
	var item MyRewardStatus
	if err := row.Scan(&item.ID, &item.WinnerID, &item.PrizeID, &item.Status, &item.ErrorKey, &item.DeliveryMode, &item.VoucherCode, &item.ManualContact); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &item, nil
}

func scanPrize(row pgx.Row) (*Prize, error) {
	var p Prize
	if err := row.Scan(&p.ID, &p.CampaignID, &p.UserID, &p.AdminAccountID, &p.Type, &p.Name, &p.Quantity, &p.SortOrder, &p.BalanceAmount, &p.GroupID, &p.GroupName, &p.Multiplier, &p.ValidityDays, &p.DeliveryMode, &p.ManualContact, &p.VoucherCodes, &p.ValueMarker); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func listEntriesTx(ctx context.Context, tx pgx.Tx, campaignID string) ([]Entry, error) {
	rows, err := tx.Query(ctx, `SELECT id,campaign_id,user_id,admin_account_id,sub2api_user_id,masked_email,receipt_token,receipt_hash,status,created_at,updated_at,withdrawn_at FROM lottery_entries WHERE campaign_id=$1 AND status='active' ORDER BY created_at ASC, id ASC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		e, err := scanEntryRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func listPrizesTx(ctx context.Context, tx pgx.Tx, campaignID string) ([]Prize, error) {
	rows, err := tx.Query(ctx, `SELECT `+prizeColumns+` FROM lottery_prizes WHERE campaign_id=$1 ORDER BY sort_order ASC, id ASC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	prizes := []Prize{}
	for rows.Next() {
		p, err := scanPrize(rows)
		if err != nil {
			return nil, err
		}
		prizes = append(prizes, *p)
	}
	return prizes, rows.Err()
}
func listWinnersTx(ctx context.Context, tx pgx.Tx, campaignID string) ([]Winner, error) {
	rows, err := tx.Query(ctx, `SELECT id,campaign_id,prize_id,draw_id,user_id,admin_account_id,entry_id,sub2api_user_id,masked_email,prize_slot,created_at FROM lottery_winners WHERE campaign_id=$1 ORDER BY prize_slot ASC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	winners := []Winner{}
	for rows.Next() {
		w, err := scanWinner(rows)
		if err != nil {
			return nil, err
		}
		winners = append(winners, *w)
	}
	return winners, rows.Err()
}
