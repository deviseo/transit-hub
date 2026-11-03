-- 系统级设置：对整个 TransitHub 实例生效，不按工作区隔离（例如访问上游站点时使用的出站代理）。
-- 只允许系统管理员（ADMIN_EMAIL 对应的账号）修改。
CREATE TABLE IF NOT EXISTS system_settings (
    key text PRIMARY KEY,
    value jsonb NOT NULL,
    updated_by text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);
