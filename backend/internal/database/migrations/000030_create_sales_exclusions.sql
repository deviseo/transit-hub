-- 销售分析里不计入的入账。只影响 TransitHub 自己的统计，不改站点上的任何数据。

-- 按「用户 × 天 × 渠道」排除（user_activity_recharge_daily 的一行）：记下排除时这一行的金额和笔数，统计时减去它们。
-- 之后同一天同一渠道又有新的入账（例如管理员又加了一次），新增的部分照常计入。
CREATE TABLE IF NOT EXISTS sales_excluded_entries (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    site_user_id text NOT NULL,
    day date NOT NULL,
    channel text NOT NULL,
    amount double precision NOT NULL,
    events integer NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, site_user_id, day, channel)
);

CREATE INDEX IF NOT EXISTS idx_sales_excluded_entries_workspace_day
    ON sales_excluded_entries (user_id, admin_account_id, day);

-- 整个渠道都不计入（例如管理员加款都是测试或补偿，不算销售）。每个工作区一行。
CREATE TABLE IF NOT EXISTS sales_settings (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    excluded_channels text[] NOT NULL DEFAULT '{}',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id)
);
