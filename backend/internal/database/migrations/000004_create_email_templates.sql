-- Workspace-scoped email templates. Built-in rows are editable, so the seed path must use
-- INSERT ... ON CONFLICT DO NOTHING and never overwrite an operator's saved copy.

CREATE TABLE IF NOT EXISTS email_templates (
  user_id text NOT NULL,
  admin_account_id text NOT NULL,
  id text NOT NULL,
  name text NOT NULL,
  subject text NOT NULL,
  html_body text NOT NULL,
  is_builtin boolean NOT NULL DEFAULT false,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (user_id, admin_account_id, id),
  CONSTRAINT email_templates_name_check CHECK (length(btrim(name)) > 0 AND length(name) <= 120),
  CONSTRAINT email_templates_subject_check CHECK (length(btrim(subject)) > 0 AND length(subject) <= 255),
  CONSTRAINT email_templates_html_body_check CHECK (length(btrim(html_body)) > 0 AND octet_length(html_body) <= 102400)
);
