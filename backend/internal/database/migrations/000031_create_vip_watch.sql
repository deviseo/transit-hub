-- 大户关注：系统推荐的大户（近 7 天日均消耗、累计消耗排前面的）每次按本地数据现算，不存；
-- 这里只存站长手动关注的用户（带备注），和站长移出、不想再被推荐的用户。
CREATE TABLE IF NOT EXISTS vip_watch (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    site_user_id text NOT NULL,
    -- manual：手动关注；dismissed：移出了，不再推荐。
    kind text NOT NULL,
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, site_user_id),
    CONSTRAINT vip_watch_kind_check CHECK (kind IN ('manual', 'dismissed'))
);

-- 大户关注的设置（每个工作区一行；没有这一行时用默认值）。
CREATE TABLE IF NOT EXISTS vip_settings (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    -- 推荐：近 7 天日均消耗前几名、累计消耗前几名（0 表示不按这一项推荐）。
    daily_top integer NOT NULL DEFAULT 20,
    lifetime_top integer NOT NULL DEFAULT 20,
    -- 快用完：预计几天内用完，或者余额（人民币）低于多少（0 表示不按这一项判断）。
    alert_days double precision NOT NULL DEFAULT 2,
    alert_balance double precision NOT NULL DEFAULT 20,
    -- 快用完、用完时用这些机器人提醒站长。
    alerts_enabled boolean NOT NULL DEFAULT true,
    bot_ids text[] NOT NULL DEFAULT '{}',
    -- 「复制提醒」的话术（空表示用默认的）。
    reminder_text text NOT NULL DEFAULT '',
    empty_reminder_text text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id),
    CONSTRAINT vip_settings_top_check CHECK (daily_top BETWEEN 0 AND 100 AND lifetime_top BETWEEN 0 AND 100),
    CONSTRAINT vip_settings_alert_check CHECK (alert_days BETWEEN 0 AND 30 AND alert_balance BETWEEN 0 AND 1000000)
);

-- 提醒站长的记录：每个大户一行，记下上次提醒时的状态（快用完 / 用完了）和时间，以及之后是否回升过。
-- 同一个人一天最多提醒一次；从快用完变成用完了会再提醒；充值回升后又降下来，满一天再提醒。
CREATE TABLE IF NOT EXISTS vip_alert_states (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    site_user_id text NOT NULL,
    state text NOT NULL,
    notified_at timestamptz NOT NULL,
    recovered boolean NOT NULL DEFAULT false,
    PRIMARY KEY (user_id, admin_account_id, site_user_id),
    CONSTRAINT vip_alert_states_state_check CHECK (state IN ('low', 'empty'))
);
