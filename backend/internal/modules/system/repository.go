package system

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"transithub/backend/internal/shared/outbound"
	"transithub/backend/internal/shared/secretbox"
)

// outboundProxyKey 是出站代理配置在 system_settings 表中的 key。
const outboundProxyKey = "outbound_proxy"

// Repository 读写 system_settings 表（系统级配置，不按工作区隔离）。
// 出站代理的地址里可能带用户名和密码，加密保存。
type Repository struct {
	db      *pgxpool.Pool
	secrets *secretbox.Box
}

// outboundProxyAAD 把密文绑定到这条配置。
const outboundProxyAAD = "system_settings/" + outboundProxyKey

// SetSecretBox 设置加密配置用的密钥（nil 表示不加密）。要在读写之前设置。
func (r *Repository) SetSecretBox(box *secretbox.Box) {
	r.secrets = box
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
	raw, err = r.secrets.OpenJSON(raw, outboundProxyAAD)
	if err != nil {
		// 读不出来（例如密钥丢了）：当作没有保存过，需要重新填写代理。
		secretbox.LogUnreadable(outboundProxyAAD, err)
		return StoredOutboundProxy{}, false, nil
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
	raw, err = r.secrets.SealJSON(raw, outboundProxyAAD)
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

// EncryptLegacyOutboundProxy 把加密之前保存的明文代理配置改成加密保存（期间被改过就不动）。
func (r *Repository) EncryptLegacyOutboundProxy(ctx context.Context) (int, error) {
	if r.secrets == nil {
		return 0, nil
	}
	var raw string
	err := r.db.QueryRow(ctx, `
		SELECT value::text FROM system_settings WHERE key = $1 AND jsonb_typeof(value) = 'object'
	`, outboundProxyKey).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	sealed, err := r.secrets.SealJSON([]byte(raw), outboundProxyAAD)
	if err != nil {
		return 0, err
	}
	tag, err := r.db.Exec(ctx, `UPDATE system_settings SET value = $1::jsonb WHERE key = $2 AND value = $3::jsonb`,
		string(sealed), outboundProxyKey, raw)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}
