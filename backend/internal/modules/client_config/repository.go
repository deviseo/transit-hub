package client_config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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

// saveColumns 是存库的列，顺序和 scanConfig、Save 的参数一致；推荐设置的列按 recommendColumns 生成。
var saveColumns = func() []string {
	columns := []string{"user_id", "admin_account_id", "code", "enabled", "hub_url", "site_url", "brand_name",
		"brand_logo", "brand_color", "welcome"}
	for _, prefix := range recommendColumns {
		columns = append(columns, prefix+"_group", prefix+"_model")
	}
	return append(columns, "notice", "notice_updated_at", "support", "min_client_version", "tickets_enabled",
		"ticket_categories")
}()

var selectColumns = strings.Join(saveColumns, ", ") + ", updated_at"

// upsertSQL 插入一行；同一个工作区已有时，除归属外的列全部更新。
var upsertSQL = func() string {
	placeholders := make([]string, len(saveColumns))
	updates := make([]string, 0, len(saveColumns))
	for index, column := range saveColumns {
		placeholders[index] = fmt.Sprintf("$%d", index+1)
		if column == "ticket_categories" {
			placeholders[index] += "::jsonb"
		}
		if column != "user_id" && column != "admin_account_id" {
			updates = append(updates, column+" = EXCLUDED."+column)
		}
	}
	return `INSERT INTO client_configs (` + strings.Join(saveColumns, ", ") + `, created_at, updated_at)
		VALUES (` + strings.Join(placeholders, ", ") + `, now(), now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET ` + strings.Join(updates, ", ") + `, updated_at = now()`
}()

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
	_, err = r.db.Exec(ctx, upsertSQL, args...)
	return err
}
