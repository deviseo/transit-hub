-- SMTP 设置专用表：按 (user_id, admin_account_id) 隔离，密码仅以密文形式存储。
-- 不包含 enabled / skip_tls_verification 字段：一期只要保存了配置即可用于测试邮件，且强制 TLS 1.2+ 不允许跳过证书校验。

CREATE TABLE IF NOT EXISTS smtp_settings (
  user_id text NOT NULL,
  admin_account_id text NOT NULL,
  host text NOT NULL,
  port integer NOT NULL,
  username text NOT NULL DEFAULT '',
  password_ciphertext text NOT NULL DEFAULT '',
  from_email text NOT NULL,
  from_name text NOT NULL DEFAULT '',
  tls_mode text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, admin_account_id),
  CONSTRAINT smtp_settings_tls_mode_check CHECK (tls_mode IN ('implicit', 'starttls')),
  CONSTRAINT smtp_settings_port_check CHECK (port BETWEEN 1 AND 65535)
);
