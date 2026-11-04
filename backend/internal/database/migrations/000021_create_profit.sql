-- 利润分析：本站路由（sub2api 上游账号 / new-api 渠道）清单、每条路由每天每个分组的营收，以及上游站点每天的成本。
-- 全部按工作区隔离，只保存完整自然日（北京时间）的数据；由后台采集任务从自己的站点和已登记的上游站点只读拉取。

CREATE TABLE IF NOT EXISTS profit_routes (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    route_id text NOT NULL,
    name text NOT NULL DEFAULT '',
    -- kind：sub2api 账号类型（apikey / oauth …）或 new-api 渠道类型编号。
    kind text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT '',
    -- base_url：路由指向的上游地址（不含任何密钥）；为空表示官方账号或渠道类型的默认地址。
    base_url text NOT NULL DEFAULT '',
    groups text[] NOT NULL DEFAULT '{}',
    deleted boolean NOT NULL DEFAULT false,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, route_id)
);

-- 本站营收（站点额度，美元口径）：route_id 为空表示站点没有记录路由的营收。
CREATE TABLE IF NOT EXISTS profit_route_daily (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    day date NOT NULL,
    route_id text NOT NULL,
    group_name text NOT NULL,
    revenue double precision NOT NULL DEFAULT 0,
    requests bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, admin_account_id, day, route_id, group_name)
);
CREATE INDEX IF NOT EXISTS idx_profit_route_daily_workspace_day
    ON profit_route_daily (user_id, admin_account_id, day);

-- 上游成本：consume 是上游额度（上游站点的美元口径），cost = consume × 充值倍率（人民币）。
-- status：ok 已查询；approximated 查询失败、用当天最后一次同步的缓存值；failed 无法得到；no_rate 未设置充值倍率（不计成本）。
CREATE TABLE IF NOT EXISTS profit_upstream_daily (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    day date NOT NULL,
    site_id text NOT NULL,
    site_name text NOT NULL DEFAULT '',
    base_url text NOT NULL DEFAULT '',
    consume double precision NOT NULL DEFAULT 0,
    recharge_rate double precision NOT NULL DEFAULT 0,
    cost double precision NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'ok',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, day, site_id)
);
CREATE INDEX IF NOT EXISTS idx_profit_upstream_daily_workspace_day
    ON profit_upstream_daily (user_id, admin_account_id, day);

CREATE TABLE IF NOT EXISTS profit_sync_states (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    platform text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT '',
    error_key text NOT NULL DEFAULT '',
    warning_key text NOT NULL DEFAULT '',
    -- revenue_source：sub2api_groups（按账号逐日查询分组统计）/ newapi_flow / newapi_log_stat（旧版 new-api 的回退）。
    revenue_source text NOT NULL DEFAULT '',
    route_count integer NOT NULL DEFAULT 0,
    synced_from date,
    synced_to date,
    -- complete_through：这一天及之前的数据已在当天结束后重新采集过（不会再变）。
    complete_through date,
    last_started_at timestamptz,
    last_synced_at timestamptz,
    PRIMARY KEY (user_id, admin_account_id)
);
