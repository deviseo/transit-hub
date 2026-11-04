package client_config

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 读写 client_configs。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const selectColumns = `user_id, admin_account_id, code, enabled, hub_url, site_url, brand_name, brand_logo, brand_color,
	welcome, claude_group, claude_model, codex_group, codex_model, notice, notice_updated_at, support, min_client_version, updated_at`

func scanConfig(row pgx.Row) (*Config, error) {
	var config Config
	var support []byte
	var updatedAt time.Time
	err := row.Scan(&config.UserID, &config.AdminAccountID, &config.Code, &config.Enabled, &config.HubURL, &config.SiteURL,
		&config.Brand.Name, &config.Brand.Logo, &config.Brand.Color, &config.Brand.Welcome,
		&config.Recommend.ClaudeCode.Group, &config.Recommend.ClaudeCode.Model,
		&config.Recommend.Codex.Group, &config.Recommend.Codex.Model,
		&config.Notice, &config.NoticeUpdatedAt, &support, &config.MinClientVersion, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(support) > 0 {
		if err := json.Unmarshal(support, &config.Support); err != nil {
			return nil, err
		}
	}
	config.UpdatedAt = &updatedAt
	return &config, nil
}

// Get 返回工作区的配置；还没保存过时返回 nil。
func (r *Repository) Get(ctx context.Context, ws Workspace) (*Config, error) {
	return scanConfig(r.db.QueryRow(ctx, `SELECT `+selectColumns+` FROM client_configs WHERE user_id = $1 AND admin_account_id = $2`,
		ws.UserID, ws.AdminAccountID))
}

// GetByCode 按专属链接里的代码查配置；没有时返回 nil。
func (r *Repository) GetByCode(ctx context.Context, code string) (*Config, error) {
	return scanConfig(r.db.QueryRow(ctx, `SELECT `+selectColumns+` FROM client_configs WHERE code = $1`, code))
}

// Save 插入或更新整行。
func (r *Repository) Save(ctx context.Context, config Config) error {
	support, err := json.Marshal(config.Support)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `INSERT INTO client_configs (user_id, admin_account_id, code, enabled, hub_url, site_url, brand_name,
		brand_logo, brand_color, welcome, claude_group, claude_model, codex_group, codex_model, notice, notice_updated_at, support,
		min_client_version, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, now(), now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET code = EXCLUDED.code, enabled = EXCLUDED.enabled,
			hub_url = EXCLUDED.hub_url, site_url = EXCLUDED.site_url, brand_name = EXCLUDED.brand_name,
			brand_logo = EXCLUDED.brand_logo, brand_color = EXCLUDED.brand_color, welcome = EXCLUDED.welcome,
			claude_group = EXCLUDED.claude_group, claude_model = EXCLUDED.claude_model, codex_group = EXCLUDED.codex_group,
			codex_model = EXCLUDED.codex_model, notice = EXCLUDED.notice, notice_updated_at = EXCLUDED.notice_updated_at,
			support = EXCLUDED.support, min_client_version = EXCLUDED.min_client_version, updated_at = now()`,
		config.UserID, config.AdminAccountID, config.Code, config.Enabled, config.HubURL, config.SiteURL, config.Brand.Name,
		config.Brand.Logo, config.Brand.Color, config.Brand.Welcome, config.Recommend.ClaudeCode.Group,
		config.Recommend.ClaudeCode.Model, config.Recommend.Codex.Group, config.Recommend.Codex.Model, config.Notice,
		config.NoticeUpdatedAt, support, config.MinClientVersion)
	return err
}
