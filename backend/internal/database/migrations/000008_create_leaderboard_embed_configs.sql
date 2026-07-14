CREATE TABLE IF NOT EXISTS leaderboard_embed_configs (
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    embed_token text NOT NULL,
    sub2api_source_origin text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_leaderboard_embed_configs_workspace
    ON leaderboard_embed_configs (user_id, admin_account_id);

CREATE UNIQUE INDEX IF NOT EXISTS idx_leaderboard_embed_configs_token
    ON leaderboard_embed_configs (embed_token);
