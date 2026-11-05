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
	welcome, claude_group, claude_model, codex_group, codex_model, gemini_group, gemini_model, grok_group, grok_model,
	opencode_group, opencode_model, dsh_group, dsh_model, notice, notice_updated_at, support, min_client_version,
	tickets_enabled, ticket_categories, updated_at`

func scanConfig(row pgx.Row) (*Config, error) {
	var config Config
	var support, categories []byte
	var updatedAt time.Time
	targets := []any{&config.UserID, &config.AdminAccountID, &config.Code, &config.Enabled, &config.HubURL, &config.SiteURL,
		&config.Brand.Name, &config.Brand.Logo, &config.Brand.Color, &config.Brand.Welcome}
	for _, tool := range config.Recommend.fields() {
		targets = append(targets, &tool.Group, &tool.Model)
	}
	targets = append(targets, &config.Notice, &config.NoticeUpdatedAt, &support, &config.MinClientVersion,
		&config.TicketsEnabled, &categories, &updatedAt)
	err := row.Scan(targets...)
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
	if len(categories) > 0 {
		if err := json.Unmarshal(categories, &config.TicketCategories); err != nil {
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
	categories := config.TicketCategories
	if categories == nil {
		categories = DefaultTicketCategories
	}
	categoriesJSON, err := json.Marshal(categories)
	if err != nil {
		return err
	}
	args := []any{config.UserID, config.AdminAccountID, config.Code, config.Enabled, config.HubURL, config.SiteURL,
		config.Brand.Name, config.Brand.Logo, config.Brand.Color, config.Brand.Welcome}
	for _, tool := range config.Recommend.fields() {
		args = append(args, tool.Group, tool.Model)
	}
	args = append(args, config.Notice, config.NoticeUpdatedAt, support, config.MinClientVersion, config.TicketsEnabled,
		string(categoriesJSON))
	_, err = r.db.Exec(ctx, `INSERT INTO client_configs (user_id, admin_account_id, code, enabled, hub_url, site_url, brand_name,
		brand_logo, brand_color, welcome, claude_group, claude_model, codex_group, codex_model, gemini_group, gemini_model,
		grok_group, grok_model, opencode_group, opencode_model, dsh_group, dsh_model, notice, notice_updated_at, support,
		min_client_version, tickets_enabled, ticket_categories, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24,
			$25, $26, $27, $28::jsonb, now(), now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET code = EXCLUDED.code, enabled = EXCLUDED.enabled,
			hub_url = EXCLUDED.hub_url, site_url = EXCLUDED.site_url, brand_name = EXCLUDED.brand_name,
			brand_logo = EXCLUDED.brand_logo, brand_color = EXCLUDED.brand_color, welcome = EXCLUDED.welcome,
			claude_group = EXCLUDED.claude_group, claude_model = EXCLUDED.claude_model, codex_group = EXCLUDED.codex_group,
			codex_model = EXCLUDED.codex_model, gemini_group = EXCLUDED.gemini_group, gemini_model = EXCLUDED.gemini_model,
			grok_group = EXCLUDED.grok_group, grok_model = EXCLUDED.grok_model, opencode_group = EXCLUDED.opencode_group,
			opencode_model = EXCLUDED.opencode_model, dsh_group = EXCLUDED.dsh_group, dsh_model = EXCLUDED.dsh_model,
			notice = EXCLUDED.notice, notice_updated_at = EXCLUDED.notice_updated_at,
			support = EXCLUDED.support, min_client_version = EXCLUDED.min_client_version,
			tickets_enabled = EXCLUDED.tickets_enabled, ticket_categories = EXCLUDED.ticket_categories, updated_at = now()`,
		args...)
	return err
}
