package settings

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

const createEmailTemplateSQL = `
	WITH workspace_lock AS (
		SELECT pg_advisory_xact_lock(hashtext($1), hashtext($2))
	)
	INSERT INTO email_templates (user_id, admin_account_id, id, name, subject, html_body, is_builtin, created_at, updated_at)
	SELECT $1, $2, $3, $4, $5, $6, false, now(), now()
	FROM workspace_lock
	WHERE (
		SELECT count(*)
		FROM email_templates
		WHERE user_id = $1 AND admin_account_id = $2 AND is_builtin = false
	) < $7
	RETURNING id, name, subject, html_body, is_builtin, created_at, updated_at
`

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS notification_channel_settings (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			settings jsonb NOT NULL DEFAULT '{}'::jsonb,
			updated_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS strategy_settings (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			settings jsonb NOT NULL DEFAULT '{}'::jsonb,
			updated_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return err
	}
	statements := []string{
		`ALTER TABLE notification_channel_settings ADD COLUMN IF NOT EXISTS admin_account_id text NOT NULL DEFAULT ''`,
		`ALTER TABLE strategy_settings ADD COLUMN IF NOT EXISTS admin_account_id text NOT NULL DEFAULT ''`,
		`ALTER TABLE notification_channel_settings DROP CONSTRAINT IF EXISTS notification_channel_settings_pkey`,
		`ALTER TABLE strategy_settings DROP CONSTRAINT IF EXISTS strategy_settings_pkey`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_notification_channel_settings_workspace ON notification_channel_settings (user_id, admin_account_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_strategy_settings_workspace ON strategy_settings (user_id, admin_account_id)`,
		`CREATE TABLE IF NOT EXISTS email_templates (
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
		)`,
	}
	for _, statement := range statements {
		if _, err := r.db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

// GetFirstStrategy 返回任意一条策略设置（不指定用户）。
// 启动时用于初始化上游定时同步配置，在单管理员场景下等同于获取唯一的设置记录。
func (r *Repository) GetFirstStrategy(ctx context.Context) (StrategySettings, error) {
	settings := DefaultStrategySettings()
	row := r.db.QueryRow(ctx, `SELECT settings FROM strategy_settings LIMIT 1`)
	var settingsJSON []byte
	if err := row.Scan(&settingsJSON); err != nil {
		if err == pgx.ErrNoRows {
			return settings, nil
		}
		return settings, err
	}
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return settings, err
	}
	return settings, nil
}

func (r *Repository) GetStrategy(ctx context.Context, userID string, adminAccountID string) (StrategySettings, error) {
	settings := DefaultStrategySettings()
	row := r.db.QueryRow(ctx, `SELECT settings FROM strategy_settings WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID)
	var settingsJSON []byte
	if err := row.Scan(&settingsJSON); err != nil {
		if err == pgx.ErrNoRows {
			return settings, nil
		}
		return settings, err
	}
	if err := json.Unmarshal(settingsJSON, &settings); err != nil {
		return settings, err
	}
	return settings, nil
}

func (r *Repository) SaveStrategy(ctx context.Context, userID string, adminAccountID string, settings StrategySettings) error {
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO strategy_settings (user_id, admin_account_id, settings, updated_at)
		VALUES ($1, $2, $3::jsonb, now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET
			settings = EXCLUDED.settings,
			updated_at = EXCLUDED.updated_at
	`, userID, adminAccountID, string(settingsJSON))
	return err
}

func (r *Repository) GetNotificationChannels(ctx context.Context, userID string, adminAccountID string) (NotificationChannelSettings, error) {
	settings := DefaultNotificationChannelSettings()
	row := r.db.QueryRow(ctx, `SELECT settings FROM notification_channel_settings WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID)
	var settingsJSON []byte
	if err := row.Scan(&settingsJSON); err != nil {
		if err == pgx.ErrNoRows {
			return settings, nil
		}
		return settings, err
	}
	if err := unmarshalNotificationChannelSettings(settingsJSON, &settings); err != nil {
		return settings, err
	}
	return settings, nil
}

func unmarshalNotificationChannelSettings(data []byte, settings *NotificationChannelSettings) error {
	if err := json.Unmarshal(data, settings); err == nil {
		return nil
	}
	var legacy struct {
		Dingtalk DingtalkChannelSettings `json:"dingtalk"`
		Feishu   WebhookChannelSettings  `json:"feishu"`
		Telegram TelegramChannelSettings `json:"telegram"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&legacy); err != nil {
		return err
	}
	if legacy.Dingtalk.Enabled || legacy.Dingtalk.Webhook != "" || legacy.Dingtalk.Secret != "" {
		settings.Dingtalk = []DingtalkChannelSettings{legacy.Dingtalk}
	}
	if legacy.Feishu.Enabled || legacy.Feishu.Webhook != "" || legacy.Feishu.Secret != "" {
		settings.Feishu = []WebhookChannelSettings{legacy.Feishu}
	}
	if legacy.Telegram.Enabled || legacy.Telegram.BotToken != "" || legacy.Telegram.ChatID != "" || legacy.Telegram.ProxyURL != "" {
		settings.Telegram = []TelegramChannelSettings{legacy.Telegram}
	}
	return nil
}

func (r *Repository) SaveNotificationChannels(ctx context.Context, userID string, adminAccountID string, settings NotificationChannelSettings) error {
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO notification_channel_settings (user_id, admin_account_id, settings, updated_at)
		VALUES ($1, $2, $3::jsonb, now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET
			settings = EXCLUDED.settings,
			updated_at = EXCLUDED.updated_at
	`, userID, adminAccountID, string(settingsJSON))
	return err
}

// smtpRow 是 smtp_settings 表在 repository 层的内部表示，包含密文，
// 不导出到 types.go：只在 service<->repository 之间传递，绝不直接序列化为 HTTP 响应。
type smtpRow struct {
	Host               string
	Port               int
	Username           string
	PasswordCiphertext string
	FromEmail          string
	FromName           string
	TLSMode            string
	UpdatedAt          *time.Time
}

// defaultSMTPRow 是没有已保存记录时的默认 smtpRow：port 587、tlsMode starttls。
// 抽成具名构造函数，便于在不连接真实数据库的单元测试中直接断言这个默认值契约，
// 因为 GetSMTPSettings 本身需要真实 Postgres 才能执行（本项目约定测试不连接真实数据库）。
func defaultSMTPRow() smtpRow {
	return smtpRow{Port: 587, TLSMode: string(SmtpTLSModeStarttls)}
}

// GetSMTPSettings 按 (user_id, admin_account_id) 查询 SMTP 配置。
// 没有记录时返回空默认值（TLSMode 回退到 starttls，Port 回退到 587），不自动插入空行。
// Port 必须在这里显式给出默认值——前端不应该再用 `|| 587` 掩盖 API 合同缺口。
func (r *Repository) GetSMTPSettings(ctx context.Context, userID string, adminAccountID string) (smtpRow, error) {
	row := defaultSMTPRow()
	dbRow := r.db.QueryRow(ctx, `
		SELECT host, port, username, password_ciphertext, from_email, from_name, tls_mode, updated_at
		FROM smtp_settings
		WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID)
	var updatedAt time.Time
	if err := dbRow.Scan(&row.Host, &row.Port, &row.Username, &row.PasswordCiphertext, &row.FromEmail, &row.FromName, &row.TLSMode, &updatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return row, nil
		}
		return row, err
	}
	row.UpdatedAt = &updatedAt
	return row, nil
}

// SaveSMTPSettings 按 (user_id, admin_account_id) upsert SMTP 配置。
func (r *Repository) SaveSMTPSettings(ctx context.Context, userID string, adminAccountID string, row smtpRow) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO smtp_settings (user_id, admin_account_id, host, port, username, password_ciphertext, from_email, from_name, tls_mode, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET
			host = EXCLUDED.host,
			port = EXCLUDED.port,
			username = EXCLUDED.username,
			password_ciphertext = EXCLUDED.password_ciphertext,
			from_email = EXCLUDED.from_email,
			from_name = EXCLUDED.from_name,
			tls_mode = EXCLUDED.tls_mode,
			updated_at = EXCLUDED.updated_at
	`, userID, adminAccountID, row.Host, row.Port, row.Username, row.PasswordCiphertext, row.FromEmail, row.FromName, row.TLSMode)
	return err
}

// EnsureBuiltInEmailTemplate seeds the workspace default template idempotently. ON CONFLICT DO NOTHING
// is intentional: built-in templates are editable, so later releases must not overwrite operator edits.
func (r *Repository) EnsureBuiltInEmailTemplate(ctx context.Context, userID string, adminAccountID string, template EmailTemplate) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO email_templates (user_id, admin_account_id, id, name, subject, html_body, is_builtin, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, true, now(), now())
		ON CONFLICT (user_id, admin_account_id, id) DO NOTHING
	`, userID, adminAccountID, template.ID, template.Name, template.Subject, template.HTMLBody)
	return err
}

func (r *Repository) ListEmailTemplates(ctx context.Context, userID string, adminAccountID string) ([]EmailTemplate, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, name, subject, html_body, is_builtin, created_at, updated_at
		FROM email_templates
		WHERE user_id = $1 AND admin_account_id = $2
		ORDER BY is_builtin DESC, updated_at DESC
	`, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	templates := []EmailTemplate{}
	for rows.Next() {
		template, err := scanEmailTemplate(rows)
		if err != nil {
			return nil, err
		}
		templates = append(templates, template)
	}
	return templates, rows.Err()
}

func (r *Repository) GetEmailTemplate(ctx context.Context, userID string, adminAccountID string, id string) (EmailTemplate, bool, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, name, subject, html_body, is_builtin, created_at, updated_at
		FROM email_templates
		WHERE user_id = $1 AND admin_account_id = $2 AND id = $3
	`, userID, adminAccountID, id)
	template, err := scanEmailTemplate(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return EmailTemplate{}, false, nil
		}
		return EmailTemplate{}, false, err
	}
	return template, true, nil
}

// CreateEmailTemplate keeps the custom-template limit in the INSERT statement so concurrent
// app instances observe one database decision instead of racing a prior count against insert.
func (r *Repository) CreateEmailTemplate(ctx context.Context, userID string, adminAccountID string, template EmailTemplate, limit int) (EmailTemplate, bool, error) {
	row := r.db.QueryRow(ctx, createEmailTemplateSQL, userID, adminAccountID, template.ID, template.Name, template.Subject, template.HTMLBody, limit)
	created, err := scanEmailTemplate(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return EmailTemplate{}, false, nil
		}
		return EmailTemplate{}, false, err
	}
	return created, true, nil
}

func (r *Repository) UpdateEmailTemplate(ctx context.Context, userID string, adminAccountID string, id string, input SaveEmailTemplateInput) (EmailTemplate, bool, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE email_templates
		SET name = $4, subject = $5, html_body = $6, updated_at = now()
		WHERE user_id = $1 AND admin_account_id = $2 AND id = $3
		RETURNING id, name, subject, html_body, is_builtin, created_at, updated_at
	`, userID, adminAccountID, id, input.Name, input.Subject, input.HTMLBody)
	template, err := scanEmailTemplate(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return EmailTemplate{}, false, nil
		}
		return EmailTemplate{}, false, err
	}
	return template, true, nil
}

func (r *Repository) DeleteEmailTemplate(ctx context.Context, userID string, adminAccountID string, id string) (bool, error) {
	commandTag, err := r.db.Exec(ctx, `
		DELETE FROM email_templates
		WHERE user_id = $1 AND admin_account_id = $2 AND id = $3 AND is_builtin = false
	`, userID, adminAccountID, id)
	if err != nil {
		return false, err
	}
	return commandTag.RowsAffected() > 0, nil
}

type emailTemplateScanner interface {
	Scan(dest ...any) error
}

func scanEmailTemplate(scanner emailTemplateScanner) (EmailTemplate, error) {
	var template EmailTemplate
	var createdAt time.Time
	var updatedAt time.Time
	if err := scanner.Scan(&template.ID, &template.Name, &template.Subject, &template.HTMLBody, &template.IsBuiltIn, &createdAt, &updatedAt); err != nil {
		return EmailTemplate{}, err
	}
	template.CreatedAt = &createdAt
	template.UpdatedAt = &updatedAt
	return template, nil
}
