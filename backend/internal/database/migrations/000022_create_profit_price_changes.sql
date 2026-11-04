-- 上游分组调价记录：上游站点同步时检测到分组倍率变化，按该分组近 7 天在上游的消费估算对每天成本的影响。
-- 只是估算和提示，不会修改任何站点的价格。按工作区隔离。

CREATE TABLE IF NOT EXISTS profit_price_changes (
    id bigserial PRIMARY KEY,
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    site_id text NOT NULL,
    site_name text NOT NULL DEFAULT '',
    base_url text NOT NULL DEFAULT '',
    group_id text NOT NULL DEFAULT '',
    group_name text NOT NULL DEFAULT '',
    old_multiplier double precision NOT NULL,
    new_multiplier double precision NOT NULL,
    recharge_rate double precision NOT NULL DEFAULT 0,
    window_days integer NOT NULL DEFAULT 0,
    -- window_consume：该分组在估算窗口内的上游消费（上游额度，美元口径）；NULL 表示没有查到。
    window_consume double precision,
    -- estimate_status：ok / no_usage / no_rate / failed。
    estimate_status text NOT NULL DEFAULT '',
    -- avg_daily_cost / daily_impact：人民币；daily_impact 为负表示每天节省。
    avg_daily_cost double precision,
    daily_impact double precision,
    detected_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS profit_price_changes_workspace_idx
    ON profit_price_changes (user_id, admin_account_id, detected_at DESC);
