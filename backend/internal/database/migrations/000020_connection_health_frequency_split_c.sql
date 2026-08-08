-- C phase separates probe evaluation, priority writeback and upstream inventory reconciliation.
ALTER TABLE IF EXISTS connection_health_policies
    ADD COLUMN IF NOT EXISTS priority_reconcile_interval_seconds integer NOT NULL DEFAULT 30;
ALTER TABLE IF EXISTS connection_health_policies
    ADD COLUMN IF NOT EXISTS priority_inventory_snapshot_ttl_seconds integer NOT NULL DEFAULT 60;
ALTER TABLE IF EXISTS connection_health_policies
    ADD COLUMN IF NOT EXISTS priority_reconcile_failure_backoff_seconds integer NOT NULL DEFAULT 30;

ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS last_reconcile_attempt_at timestamptz NULL;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS last_reconcile_success_at timestamptz NULL;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS last_reconcile_failure_at timestamptz NULL;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS next_reconcile_at timestamptz NULL;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS inventory_snapshot_expires_at timestamptz NULL;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS last_inventory_error text NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS inventory_status text NOT NULL DEFAULT 'unknown';
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS last_action_source text NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS policy_version text NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS reconcile_interval_seconds integer NOT NULL DEFAULT 30;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS inventory_snapshot_ttl_seconds integer NOT NULL DEFAULT 60;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS reconcile_failure_backoff_seconds integer NOT NULL DEFAULT 30;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS pending_age_seconds bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS last_inventory_read_duration_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS last_write_duration_ms bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS probe_evaluation_count bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS reconcile_attempt_count bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS reconcile_success_count bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS reconcile_failure_count bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS snapshot_hit_count bigint NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS connection_health_priority_workspace_sync_states
    ADD COLUMN IF NOT EXISTS snapshot_miss_count bigint NOT NULL DEFAULT 0;
