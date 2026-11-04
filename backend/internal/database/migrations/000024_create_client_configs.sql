-- TransitLink 客户端配置：每个工作区（站长的一个站点）一行。站长在「客户端」页面设置品牌、
-- 推荐分组和模型、公告、客服；用户用专属链接打开 TransitLink 时，客户端通过公开接口
-- GET /api/client/v1/sites/{code} 读取这份配置。
CREATE TABLE IF NOT EXISTS client_configs (
    user_id text NOT NULL,
    admin_account_id text NOT NULL,
    code text NOT NULL,
    enabled boolean NOT NULL DEFAULT false,
    hub_url text NOT NULL DEFAULT '',
    site_url text NOT NULL DEFAULT '',
    brand_name text NOT NULL DEFAULT '',
    brand_logo text NOT NULL DEFAULT '',
    brand_color text NOT NULL DEFAULT '',
    welcome text NOT NULL DEFAULT '',
    claude_group text NOT NULL DEFAULT '',
    claude_model text NOT NULL DEFAULT '',
    codex_group text NOT NULL DEFAULT '',
    codex_model text NOT NULL DEFAULT '',
    notice text NOT NULL DEFAULT '',
    notice_updated_at timestamptz,
    support jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, admin_account_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS client_configs_code_idx ON client_configs (code);
