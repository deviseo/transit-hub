-- TransitLink 支持了更多编程工具：Gemini CLI、Grok Build、OpenCode、DeepSeek Harness 也可以设置推荐分组和模型。
-- 空字符串表示不指定（客户端按分组名自动推荐）。
ALTER TABLE client_configs
    ADD COLUMN IF NOT EXISTS gemini_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS gemini_model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS grok_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS grok_model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS opencode_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS opencode_model text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dsh_group text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS dsh_model text NOT NULL DEFAULT '';
