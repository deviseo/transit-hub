-- Workspace-scoped mass email queue. The schema is additive and idempotent so it can
-- be applied safely to already deployed installations.

CREATE TABLE IF NOT EXISTS mass_email_batches (
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
  cancelled_at timestamptz,
  CONSTRAINT mass_email_batches_status_check CHECK (status IN ('queued', 'running', 'cancelling', 'cancelled', 'completed', 'completed_with_errors', 'failed')),
  CONSTRAINT mass_email_batches_selection_mode_check CHECK (selection_mode IN ('selected', 'all')),
  CONSTRAINT mass_email_batches_counts_check CHECK (
    recipient_count >= 0 AND skipped_count >= 0 AND sent_count >= 0 AND failed_count >= 0 AND uncertain_count >= 0 AND cancelled_count >= 0
  )
);

CREATE TABLE IF NOT EXISTS mass_email_batch_items (
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
  finished_at timestamptz,
  CONSTRAINT mass_email_batch_items_status_check CHECK (status IN ('pending', 'sending', 'sent', 'failed', 'uncertain', 'cancelled'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mass_email_batches_workspace_request
  ON mass_email_batches (user_id, admin_account_id, request_id);

CREATE INDEX IF NOT EXISTS idx_mass_email_batches_workspace_created
  ON mass_email_batches (user_id, admin_account_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_mass_email_batches_status
  ON mass_email_batches (status, updated_at);

CREATE UNIQUE INDEX IF NOT EXISTS idx_mass_email_items_batch_normalized_email
  ON mass_email_batch_items (batch_id, normalized_email);

CREATE INDEX IF NOT EXISTS idx_mass_email_items_batch_created
  ON mass_email_batch_items (batch_id, created_at ASC);

CREATE INDEX IF NOT EXISTS idx_mass_email_items_pending_claim
  ON mass_email_batch_items (status, created_at)
  WHERE status = 'pending';

CREATE INDEX IF NOT EXISTS idx_mass_email_items_sending_claimed
  ON mass_email_batch_items (status, claimed_at)
  WHERE status = 'sending';
