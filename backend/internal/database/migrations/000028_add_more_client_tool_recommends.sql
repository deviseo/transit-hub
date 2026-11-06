-- TransitLink 0.6.0 又支持了 Pi、Oh My Pi、Kimi Code、MiniMax Code、OpenClaw：也可以设置推荐分组和模型。
-- Antigravity CLI 只能用 Google 账号登录，TransitLink 没法一键配置，所以没有推荐设置。
-- 空字符串表示不指定（客户端按分组名自动推荐）。
ALTER TABLE client_configs
    ADD COLUMN IF NOT EXISTS pi_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS pi_model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS omp_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS omp_model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS kimi_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS kimi_model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS minimax_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS minimax_model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS openclaw_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS openclaw_model text NOT NULL DEFAULT '';
