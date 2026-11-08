-- 线索：站长自己收集的潜在用户邮箱（按工作区隔离），可以单个添加（带来源）或批量导入，再通过群发邮件联系。
-- 发送次数、最近发送时间和退信都从 mass_email_batch_items 统计，退订看 email_suppressions，这里不重复保存。
CREATE TABLE IF NOT EXISTS leads (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    email text NOT NULL,
    email_normalized text NOT NULL,
    source text NOT NULL DEFAULT '',
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_leads_workspace_email
    ON leads (user_id, admin_account_id, email_normalized);

CREATE INDEX IF NOT EXISTS idx_leads_workspace_created
    ON leads (user_id, admin_account_id, created_at DESC);

-- 线索发信设置（每个工作区一行）：最近 24 小时最多发给线索多少封，避免发得太猛被邮件服务商封号。
CREATE TABLE IF NOT EXISTS lead_settings (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    daily_limit integer NOT NULL DEFAULT 200,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id),
    CONSTRAINT lead_settings_daily_limit_check CHECK (daily_limit BETWEEN 1 AND 10000)
);

-- 群发邮件可以发给线索：记下收件人对应的线索（发给站点用户时为空字符串）。
ALTER TABLE IF EXISTS mass_email_batch_items ADD COLUMN IF NOT EXISTS lead_id text NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_mass_email_items_lead
    ON mass_email_batch_items (lead_id)
    WHERE lead_id <> '';

-- 统计最近 24 小时发给线索的数量。
CREATE INDEX IF NOT EXISTS idx_mass_email_items_lead_workspace_created
    ON mass_email_batch_items (user_id, admin_account_id, created_at)
    WHERE lead_id <> '';

ALTER TABLE IF EXISTS mass_email_batches DROP CONSTRAINT IF EXISTS mass_email_batches_selection_mode_check;
ALTER TABLE IF EXISTS mass_email_batches
    ADD CONSTRAINT mass_email_batches_selection_mode_check CHECK (selection_mode IN ('selected', 'all', 'segment', 'leads'));
