-- 工单：用户在 TransitLink 里提交问题，站长在 TransitHub 里回复。按工作区（user_id, admin_account_id）隔离。
-- 以前嵌入网页版本留下的工单表如果还在，就在原表上补齐新列，旧工单照样能看到。

CREATE TABLE IF NOT EXISTS tickets (
    id text PRIMARY KEY,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    manual_email text NOT NULL DEFAULT '',
    title text NOT NULL,
    status text NOT NULL DEFAULT 'open',
    last_message_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- 提问的用户在站点上的身份（sub2api / new-api 的用户 ID、用户名、邮箱），和客户端信息。
ALTER TABLE tickets
    ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT '通用问题',
    ADD COLUMN IF NOT EXISTS platform text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS site_user_id text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS site_username text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS site_email text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS client_info text NOT NULL DEFAULT '',
    -- 最后一条消息是谁发的（customer / admin），和用户最后一次查看的时间：用来提示用户有新回复。
    ADD COLUMN IF NOT EXISTS last_author text NOT NULL DEFAULT 'customer',
    ADD COLUMN IF NOT EXISTS customer_seen_at timestamptz;

-- 旧的嵌入工单只来自 sub2api：把用户身份搬到新列。
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_attribute
        WHERE attrelid = 'tickets'::regclass AND attname = 'sub2api_user_id' AND NOT attisdropped
    ) THEN
        UPDATE tickets
        SET platform = 'sub2api', site_user_id = sub2api_user_id, site_email = sub2api_email
        WHERE site_user_id = '' AND sub2api_user_id <> '';
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_tickets_workspace_status_updated
    ON tickets (user_id, admin_account_id, status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_tickets_site_user_updated
    ON tickets (user_id, admin_account_id, site_user_id, updated_at DESC);

CREATE TABLE IF NOT EXISTS ticket_messages (
    id text PRIMARY KEY,
    ticket_id text NOT NULL,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    author_type text NOT NULL,
    author_name text NOT NULL DEFAULT '',
    body text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket_created
    ON ticket_messages (ticket_id, created_at ASC);

-- 图片附件：文件存在数据目录的 ticket-uploads 里，这里只存元数据和文件名。
CREATE TABLE IF NOT EXISTS ticket_attachments (
    id text PRIMARY KEY,
    ticket_id text NOT NULL,
    message_id text NOT NULL,
    user_id text NOT NULL,
    admin_account_id text NOT NULL DEFAULT '',
    author_type text NOT NULL,
    original_name text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL,
    storage_path text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_ticket_attachments_ticket_created
    ON ticket_attachments (ticket_id, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_ticket_attachments_message_created
    ON ticket_attachments (message_id, created_at ASC);

-- 「客户端」页面里的工单开关和问题分类。
ALTER TABLE client_configs
    ADD COLUMN IF NOT EXISTS tickets_enabled boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS ticket_categories jsonb NOT NULL DEFAULT '["通用问题","余额/计费","接口调用","账号/登录"]'::jsonb;
