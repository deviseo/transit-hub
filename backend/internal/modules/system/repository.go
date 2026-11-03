package system

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"transithub/backend/internal/shared/outbound"
)

// outboundProxyKey 是出站代理配置在 system_settings 表中的 key。
const outboundProxyKey = "outbound_proxy"

// Repository 读写 system_settings 表（系统级配置，不按工作区隔离）。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// StoredOutboundProxy 是已保存的出站代理配置及其更新时间。
type StoredOutboundProxy struct {
	Settings  outbound.Settings
	UpdatedAt time.Time
}

// GetOutboundProxy 读取已保存的出站代理配置；从未保存过时返回 exists=false。
func (r *Repository) GetOutboundProxy(ctx context.Context) (StoredOutboundProxy, bool, error) {
	var raw []byte
	var updatedAt time.Time
	err := r.db.QueryRow(ctx, `SELECT value, updated_at FROM system_settings WHERE key = $1`, outboundProxyKey).Scan(&raw, &updatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return StoredOutboundProxy{}, false, nil
	}
	if err != nil {
		return StoredOutboundProxy{}, false, err
	}
	var settings outbound.Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return StoredOutboundProxy{}, false, err
	}
	return StoredOutboundProxy{Settings: settings, UpdatedAt: updatedAt}, true, nil
}

// SaveOutboundProxy 保存出站代理配置，返回更新时间。
func (r *Repository) SaveOutboundProxy(ctx context.Context, settings outbound.Settings, updatedBy string) (time.Time, error) {
	raw, err := json.Marshal(settings)
	if err != nil {
		return time.Time{}, err
	}
	var updatedAt time.Time
	err = r.db.QueryRow(ctx, `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2::jsonb, $3, now())
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()
		RETURNING updated_at
	`, outboundProxyKey, string(raw), updatedBy).Scan(&updatedAt)
	return updatedAt, err
}
