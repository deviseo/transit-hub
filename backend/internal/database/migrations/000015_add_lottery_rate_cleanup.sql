ALTER TABLE lottery_reward_jobs
    ADD COLUMN IF NOT EXISTS rate_cleanup_status text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS rate_cleanup_at timestamptz,
    ADD COLUMN IF NOT EXISTS rate_cleanup_attempt_count integer NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS rate_cleanup_next_attempt_at timestamptz,
    ADD COLUMN IF NOT EXISTS rate_cleanup_locked_at timestamptz,
    ADD COLUMN IF NOT EXISTS rate_cleanup_error_detail text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS rate_cleanup_completed_at timestamptz;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'lottery_reward_jobs_rate_cleanup_status_check'
          AND conrelid = 'lottery_reward_jobs'::regclass
    ) THEN
        ALTER TABLE lottery_reward_jobs
            ADD CONSTRAINT lottery_reward_jobs_rate_cleanup_status_check
            CHECK (rate_cleanup_status IN ('', 'pending', 'processing', 'retryable_failed', 'completed')) NOT VALID;
    END IF;
END
$$;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'lottery_reward_jobs_rate_cleanup_attempt_count_check'
          AND conrelid = 'lottery_reward_jobs'::regclass
    ) THEN
        ALTER TABLE lottery_reward_jobs
            ADD CONSTRAINT lottery_reward_jobs_rate_cleanup_attempt_count_check
            CHECK (rate_cleanup_attempt_count >= 0) NOT VALID;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_lottery_reward_jobs_rate_cleanup_claim
    ON lottery_reward_jobs (rate_cleanup_status, rate_cleanup_next_attempt_at, rate_cleanup_locked_at)
    WHERE rate_cleanup_status IN ('pending', 'processing', 'retryable_failed');

CREATE INDEX IF NOT EXISTS idx_lottery_reward_jobs_rate_cleanup_target
    ON lottery_reward_jobs (user_id, admin_account_id, rate_cleanup_status, rate_cleanup_at DESC)
    WHERE rate_cleanup_status IN ('pending', 'processing', 'retryable_failed');
