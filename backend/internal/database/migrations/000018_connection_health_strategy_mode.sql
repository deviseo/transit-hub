-- Separate multiplier-only priority automation from policies that run real model probes.
ALTER TABLE IF EXISTS connection_health_policies
    ADD COLUMN IF NOT EXISTS strategy_mode text NOT NULL DEFAULT 'health_probe';

-- Runtime EnsureSchema historically creates these tables after migrations run. Guard the
-- compatibility backfill so a fresh database can complete migrations before those tables exist.
-- Existing installations have both tables and still receive the rolling-deployment backfill.
DO $$
BEGIN
    IF to_regclass('public.connection_health_policies') IS NOT NULL
       AND to_regclass('public.connection_health_model_targets') IS NOT NULL THEN
        UPDATE connection_health_policies AS policy
        SET strategy_mode = 'multiplier_only'
        WHERE policy.priority_mode = 'multiplier'
          AND policy.auto_degrade_enabled = false
          AND NOT EXISTS (
              SELECT 1
              FROM connection_health_model_targets AS target
              WHERE target.policy_id = policy.id
          );
    END IF;
END
$$;
