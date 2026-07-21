-- Atomically reserve daily probe budget across concurrent requests and backend replicas.
CREATE TABLE IF NOT EXISTS connection_health_probe_budget_usage (
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    policy_id text NOT NULL,
    day_start timestamptz NOT NULL,
    used integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, policy_id, day_start)
);

CREATE TABLE IF NOT EXISTS connection_health_runtime_leases (
    lease_key text PRIMARY KEY,
    owner_id text NOT NULL,
    expires_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- Existing installations may already have the snapshot tables from runtime EnsureSchema.
ALTER TABLE IF EXISTS connection_health_priority_sync_states
    ADD COLUMN IF NOT EXISTS pending_priority integer NULL;
ALTER TABLE IF EXISTS connection_health_target_action_states
    ADD COLUMN IF NOT EXISTS pending_status text NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS connection_health_target_action_states
    ADD COLUMN IF NOT EXISTS pending_weight integer NULL;
