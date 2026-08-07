-- B phase keeps workspace-level priority writeback intent separate from per-target checkpoints.
ALTER TABLE IF EXISTS connection_health_policies
    ADD COLUMN IF NOT EXISTS priority_min_write_interval_seconds integer NOT NULL DEFAULT 30;
ALTER TABLE IF EXISTS connection_health_policies
    ADD COLUMN IF NOT EXISTS priority_max_pending_age_seconds integer NOT NULL DEFAULT 300;
ALTER TABLE IF EXISTS connection_health_policies
    ADD COLUMN IF NOT EXISTS priority_drift_action text NOT NULL DEFAULT 'alert_only';
ALTER TABLE IF EXISTS connection_health_policies
    ADD COLUMN IF NOT EXISTS priority_read_mode text NOT NULL DEFAULT 'inventory_snapshot';

CREATE TABLE IF NOT EXISTS connection_health_priority_workspace_sync_states (
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    applied_signature text NOT NULL DEFAULT '',
    pending_signature text NOT NULL DEFAULT '',
    pending_since timestamptz NULL,
    last_evaluation_at timestamptz NULL,
    last_write_attempt_at timestamptz NULL,
    last_write_success_at timestamptz NULL,
    last_drift_at timestamptz NULL,
    last_decision text NOT NULL DEFAULT '',
    last_suppression_reason text NOT NULL DEFAULT '',
    last_error text NOT NULL DEFAULT '',
    min_write_interval_seconds integer NOT NULL DEFAULT 30,
    max_pending_age_seconds integer NOT NULL DEFAULT 300,
    drift_action text NOT NULL DEFAULT 'alert_only',
    read_mode text NOT NULL DEFAULT 'inventory_snapshot',
    evaluation_count bigint NOT NULL DEFAULT 0,
    signature_change_count bigint NOT NULL DEFAULT 0,
    write_attempt_count bigint NOT NULL DEFAULT 0,
    write_success_count bigint NOT NULL DEFAULT 0,
    write_failure_count bigint NOT NULL DEFAULT 0,
    unchanged_skip_count bigint NOT NULL DEFAULT 0,
    window_suppression_count bigint NOT NULL DEFAULT 0,
    drift_count bigint NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id)
);
