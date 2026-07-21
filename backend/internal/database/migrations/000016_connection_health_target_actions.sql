-- Preserve the upstream status/weight that connection health managed before failover.
-- The connection health module historically created its own tables at startup, so the
-- events table may not exist yet on a fresh install when database migrations run.
DO $$
BEGIN
    IF to_regclass('public.connection_health_events') IS NOT NULL THEN
        ALTER TABLE connection_health_events
            ADD COLUMN IF NOT EXISTS policy_id text NOT NULL DEFAULT '';
        ALTER TABLE connection_health_events
            ADD COLUMN IF NOT EXISTS admin_group_id text NOT NULL DEFAULT '';
    END IF;
END
$$;

CREATE TABLE IF NOT EXISTS connection_health_target_action_states (
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    target_id text NOT NULL,
    original_status text NOT NULL DEFAULT '',
    original_weight integer NULL,
    last_applied_status text NOT NULL DEFAULT '',
    last_applied_weight integer NULL,
    pending_status text NOT NULL DEFAULT '',
    pending_weight integer NULL,
    conflict boolean NOT NULL DEFAULT false,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, target_id)
);

ALTER TABLE connection_health_target_action_states
    ADD COLUMN IF NOT EXISTS pending_status text NOT NULL DEFAULT '';
ALTER TABLE connection_health_target_action_states
    ADD COLUMN IF NOT EXISTS pending_weight integer NULL;

CREATE INDEX IF NOT EXISTS idx_connection_health_target_action_workspace
    ON connection_health_target_action_states (user_id, admin_account_id);
