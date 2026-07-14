CREATE TABLE IF NOT EXISTS workspace_cleanup_jobs (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    attachment_paths jsonb NOT NULL DEFAULT '[]'::jsonb,
    upstream_site_ids jsonb NOT NULL DEFAULT '[]'::jsonb,
    attempts integer NOT NULL DEFAULT 0,
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    last_error text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_workspace_cleanup_jobs_due
ON workspace_cleanup_jobs (next_attempt_at ASC, created_at ASC);
