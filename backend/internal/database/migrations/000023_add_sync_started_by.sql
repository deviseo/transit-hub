-- 记录每次采集 / 同步是怎么开始的，前端据此决定完成后要不要提醒：
-- manual（手动开始）、auto（后台自动开始的首次采集）、resume（服务重启后自动继续）、schedule（每小时例行，只在失败时提醒）。
ALTER TABLE IF EXISTS profit_sync_states ADD COLUMN IF NOT EXISTS started_by text NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS user_activity_sync_states ADD COLUMN IF NOT EXISTS started_by text NOT NULL DEFAULT '';
