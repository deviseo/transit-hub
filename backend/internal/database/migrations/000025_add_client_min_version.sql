-- TransitLink 最低版本要求：低于这个版本的客户端提示用户更新。空字符串表示不限制。
ALTER TABLE client_configs ADD COLUMN IF NOT EXISTS min_client_version text NOT NULL DEFAULT '';
