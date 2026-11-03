-- 用户活跃分析：从当前工作区「我的站点」（sub2api / new-api）同步的用户快照与每日用量，按工作区隔离。
-- 只保存运营需要的字段；每日用量保留约 400 天，用于周/月环比与去年同期对比。
CREATE TABLE IF NOT EXISTS user_activity_profiles (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    site_user_id text NOT NULL,
    email text NOT NULL DEFAULT '',
    username text NOT NULL DEFAULT '',
    role text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT '',
    user_group text NOT NULL DEFAULT '',
    balance double precision,
    -- total_recharged：累计充值（sub2api total_recharged）；total_used：累计消费（new-api used_quota）。平台不提供时为 NULL。
    total_recharged double precision,
    total_used double precision,
    -- funded_total：new-api 用户「剩余额度 + 累计已用」（美元口径）。消费不改变这个和，充值 / 兑换 / 管理员加减额度会改变它，
    -- 两次同步之间的差值即为这段时间的入账（见 user_activity_recharge_daily 的 credit / debit）。sub2api 为 NULL。
    funded_total double precision,
    -- ever_used：站点记录显示该用户有过消费。sub2api 的使用记录只保留 90 天，因此超过 90 天未使用的
    -- 老用户可能 last_used_at 为空但 ever_used 为 true，按「长期未活跃」而不是「从未使用」统计。
    ever_used boolean NOT NULL DEFAULT false,
    site_created_at timestamptz,
    last_used_at timestamptz,
    first_seen_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, site_user_id)
);
CREATE INDEX IF NOT EXISTS idx_user_activity_profiles_last_used
    ON user_activity_profiles (user_id, admin_account_id, last_used_at);

CREATE TABLE IF NOT EXISTS user_activity_daily (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    site_user_id text NOT NULL,
    day date NOT NULL,
    requests bigint NOT NULL DEFAULT 0,
    tokens bigint NOT NULL DEFAULT 0,
    cost double precision NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, admin_account_id, site_user_id, day)
);
CREATE INDEX IF NOT EXISTS idx_user_activity_daily_workspace_day
    ON user_activity_daily (user_id, admin_account_id, day);

CREATE TABLE IF NOT EXISTS user_activity_sync_states (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    platform text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT '',
    error_key text NOT NULL DEFAULT '',
    synced_from date,
    synced_to date,
    user_count integer NOT NULL DEFAULT 0,
    last_started_at timestamptz,
    last_synced_at timestamptz,
    -- 入账数据的本地覆盖区间：sub2api 按充值记录逐段回补（不受 90 天用量保留期限制）；
    -- new-api 从首次同步开始按余额快照差值统计，recharge_since 记录开始统计的时间。
    recharge_synced_from date,
    recharge_synced_to date,
    recharge_since timestamptz,
    -- warning_key：不影响同步结果的问题（例如充值记录读取失败），前端提示。
    warning_key text NOT NULL DEFAULT '',
    PRIMARY KEY (user_id, admin_account_id)
);

-- 每个用户每天、每个渠道的入账（美元口径，带符号）。
-- sub2api 渠道：payment（在线充值）、redeem（兑换码）、admin_add / admin_deduct（管理员加款 / 扣款）、affiliate（邀请返利转余额），
--   来自站点充值记录，重新拉取某天时整体替换。
-- new-api 渠道：credit / debit（两次同步之间「剩余额度 + 累计已用」的增加 / 减少），按同步累加。
CREATE TABLE IF NOT EXISTS user_activity_recharge_daily (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    site_user_id text NOT NULL,
    day date NOT NULL,
    channel text NOT NULL,
    amount double precision NOT NULL DEFAULT 0,
    events integer NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, admin_account_id, site_user_id, day, channel)
);
CREATE INDEX IF NOT EXISTS idx_user_activity_recharge_daily_workspace_day
    ON user_activity_recharge_daily (user_id, admin_account_id, day);

-- 全站每日快照：当天最后一次同步时的用户余额合计等，用于余额趋势和「较昨日末」对比。
CREATE TABLE IF NOT EXISTS user_activity_site_daily (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    day date NOT NULL,
    total_balance double precision NOT NULL DEFAULT 0,
    users_with_balance integer NOT NULL DEFAULT 0,
    user_count integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, day)
);

-- 邮件退订名单（按工作区隔离）：群发时跳过这些地址。
CREATE TABLE IF NOT EXISTS email_suppressions (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    email_normalized text NOT NULL,
    reason text NOT NULL DEFAULT 'unsubscribe',
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id, email_normalized)
);

-- 群发邮件记录生成退订链接所用的 TransitHub 访问地址（创建批次时的页面地址）。
ALTER TABLE IF EXISTS mass_email_batches ADD COLUMN IF NOT EXISTS public_base_url text NOT NULL DEFAULT '';

-- 新增 segment 选择方式：按「用户活跃分析」的筛选条件，从本地同步的用户快照解析收件人（sub2api / new-api 通用）。
ALTER TABLE IF EXISTS mass_email_batches DROP CONSTRAINT IF EXISTS mass_email_batches_selection_mode_check;
ALTER TABLE IF EXISTS mass_email_batches
    ADD CONSTRAINT mass_email_batches_selection_mode_check CHECK (selection_mode IN ('selected', 'all', 'segment'));
