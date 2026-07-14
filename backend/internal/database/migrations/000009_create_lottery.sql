CREATE TABLE IF NOT EXISTS lottery_embed_configs (
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    embed_token text NOT NULL,
    sub2api_source_origin text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS lottery_campaigns (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('draft','scheduled','open','closed','drawing','drawn','fulfilling','completed','partial','cancelled')),
    registration_start timestamptz,
    registration_end timestamptz,
    draw_at timestamptz,
    draw_mode text NOT NULL CHECK (draw_mode IN ('manual','scheduled')),
    public_winners boolean NOT NULL DEFAULT false,
    seed_secret text NOT NULL DEFAULT '',
    seed_commitment text NOT NULL DEFAULT '',
    entry_snapshot_hash text NOT NULL DEFAULT '',
    revealed_seed text NOT NULL DEFAULT '',
    algorithm_version text NOT NULL DEFAULT 'lottery-hmac-sha256-v1',
    entry_count integer NOT NULL DEFAULT 0 CHECK (entry_count >= 0),
    winner_count integer NOT NULL DEFAULT 0 CHECK (winner_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    opened_at timestamptz,
    closed_at timestamptz,
    drawn_at timestamptz,
    completed_at timestamptz,
    cancelled_at timestamptz
);

CREATE TABLE IF NOT EXISTS lottery_prizes (
    id text PRIMARY KEY,
    campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    type text NOT NULL CHECK (type IN ('balance','subscription')),
    name text NOT NULL,
    quantity integer NOT NULL CHECK (quantity > 0),
    sort_order integer NOT NULL DEFAULT 0,
    balance_amount numeric CHECK (balance_amount IS NULL OR balance_amount > 0),
    group_id text NOT NULL DEFAULT '',
    group_name text NOT NULL DEFAULT '',
    multiplier numeric,
    validity_days integer CHECK (validity_days IS NULL OR (validity_days >= 1 AND validity_days <= 36500)),
    value_marker integer NOT NULL DEFAULT 1 CHECK (value_marker = 1),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((type = 'balance' AND balance_amount IS NOT NULL AND group_id = '' AND validity_days IS NULL) OR (type = 'subscription' AND balance_amount IS NULL AND group_id <> '' AND validity_days IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS lottery_entries (
    id text PRIMARY KEY,
    campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    sub2api_user_id text NOT NULL,
    masked_email text NOT NULL DEFAULT '',
    receipt_token text NOT NULL,
    receipt_hash text NOT NULL,
    status text NOT NULL CHECK (status IN ('active','withdrawn')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    withdrawn_at timestamptz
);

CREATE TABLE IF NOT EXISTS lottery_draws (
    id text PRIMARY KEY,
    campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    entry_snapshot_hash text NOT NULL,
    revealed_seed text NOT NULL,
    algorithm_version text NOT NULL,
    entry_count integer NOT NULL CHECK (entry_count >= 0),
    winner_count integer NOT NULL CHECK (winner_count >= 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS lottery_winners (
    id text PRIMARY KEY,
    campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE,
    prize_id text NOT NULL REFERENCES lottery_prizes(id) ON DELETE CASCADE,
    draw_id text NOT NULL REFERENCES lottery_draws(id) ON DELETE CASCADE,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    entry_id text NOT NULL REFERENCES lottery_entries(id) ON DELETE CASCADE,
    sub2api_user_id text NOT NULL,
    masked_email text NOT NULL DEFAULT '',
    prize_slot integer NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS lottery_reward_jobs (
    id text PRIMARY KEY,
    campaign_id text NOT NULL REFERENCES lottery_campaigns(id) ON DELETE CASCADE,
    winner_id text NOT NULL REFERENCES lottery_winners(id) ON DELETE CASCADE,
    prize_id text NOT NULL REFERENCES lottery_prizes(id) ON DELETE CASCADE,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN ('pending','processing','fulfilled','retryable_failed','manual_attention','failed')),
    attempt_count integer NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    locked_at timestamptz,
    error_key text NOT NULL DEFAULT '',
    error_detail text NOT NULL DEFAULT '',
    remote_reference text NOT NULL DEFAULT '',
    idempotency_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    fulfilled_at timestamptz
);

CREATE TABLE IF NOT EXISTS lottery_audit_logs (
    id text PRIMARY KEY,
    campaign_id text NOT NULL DEFAULT '',
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    actor_type text NOT NULL CHECK (actor_type IN ('admin','embed','worker','scheduler','system')),
    actor_id text NOT NULL DEFAULT '',
    event text NOT NULL,
    detail jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_embed_configs_workspace ON lottery_embed_configs (user_id, admin_account_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_embed_configs_token ON lottery_embed_configs (embed_token);
CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_workspace ON lottery_campaigns (user_id, admin_account_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_due_open ON lottery_campaigns (status, registration_start) WHERE status = 'scheduled';
CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_due_close ON lottery_campaigns (status, registration_end) WHERE status = 'open';
CREATE INDEX IF NOT EXISTS idx_lottery_campaigns_due_draw ON lottery_campaigns (status, draw_mode, draw_at) WHERE status = 'closed';
CREATE INDEX IF NOT EXISTS idx_lottery_prizes_campaign ON lottery_prizes (campaign_id, sort_order, id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_entries_campaign_user ON lottery_entries (campaign_id, sub2api_user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_entries_receipt_hash ON lottery_entries (receipt_hash);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_draws_campaign ON lottery_draws (campaign_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_winners_campaign_user ON lottery_winners (campaign_id, sub2api_user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_reward_jobs_winner_prize ON lottery_reward_jobs (winner_id, prize_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_lottery_reward_jobs_idempotency ON lottery_reward_jobs (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_lottery_reward_jobs_claim ON lottery_reward_jobs (status, next_attempt_at, locked_at);
CREATE INDEX IF NOT EXISTS idx_lottery_audit_logs_campaign ON lottery_audit_logs (campaign_id, created_at DESC);
