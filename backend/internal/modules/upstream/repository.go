package upstream

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	// upstream_sites is intentionally independent from group_rate_snapshots: site
	// configuration and session restore are operational state, while historical
	// multiplier snapshots must remain readable even after a site is deleted.
	if _, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS upstream_sites (
			id text PRIMARY KEY,
			user_id text NOT NULL DEFAULT '',
			name text NOT NULL,
			base_url text NOT NULL,
			platform text NOT NULL,
			requested_platform text NOT NULL,
			account text NOT NULL,
			remark text NOT NULL DEFAULT '',
			recharge_rate double precision NOT NULL DEFAULT 1,
			status text NOT NULL,
			error_key text NULL,
			metrics jsonb NOT NULL,
			session jsonb NULL,
			last_synced_at bigint NULL,
			created_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE upstream_sites ADD COLUMN IF NOT EXISTS user_id text NOT NULL DEFAULT ''
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE upstream_sites ADD COLUMN IF NOT EXISTS settings jsonb NOT NULL DEFAULT '{}'::jsonb
	`); err != nil {
		return err
	}
	// 工作区隔离字段：每个站点归属到一个 admin workspace。
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE upstream_sites ADD COLUMN IF NOT EXISTS admin_account_id text NOT NULL DEFAULT ''
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		WITH single_user AS (
			SELECT min(id) AS id
			FROM users
			HAVING count(*) = 1
		)
		UPDATE upstream_sites
		SET user_id = single_user.id
		FROM single_user
		WHERE upstream_sites.user_id = ''
	`); err != nil {
		return err
	}
	_, err := r.db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_upstream_sites_user_created
		ON upstream_sites (user_id, created_at ASC, id ASC)
	`)
	return err
}

func (r *Repository) ListSites(ctx context.Context) ([]Site, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, name, base_url, platform, requested_platform, account, remark,
			recharge_rate, status, error_key, metrics, session, settings, last_synced_at
		FROM upstream_sites
		WHERE user_id <> ''
		ORDER BY created_at ASC, id ASC
	`)
	if err != nil {
		return nil, err
	}
	return scanSites(rows)
}

func (r *Repository) ListSitesForUser(ctx context.Context, userID string) ([]Site, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, name, base_url, platform, requested_platform, account, remark,
			recharge_rate, status, error_key, metrics, session, settings, last_synced_at
		FROM upstream_sites
		WHERE user_id = $1
		ORDER BY created_at ASC, id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	return scanSites(rows)
}

func (r *Repository) SaveSite(ctx context.Context, site Site) error {
	metricsJSON, err := json.Marshal(site.Metrics)
	if err != nil {
		return err
	}

	var sessionJSON []byte
	if site.Session != nil {
		sessionJSON, err = json.Marshal(site.Session)
		if err != nil {
			return err
		}
	}

	settingsJSON, err := json.Marshal(site.Settings)
	if err != nil {
		return err
	}

	now := time.Now()
	result, err := r.db.Exec(ctx, `
		INSERT INTO upstream_sites (
			id, user_id, admin_account_id, name, base_url, platform, requested_platform, account, remark,
			recharge_rate, status, error_key, metrics, session, settings, last_synced_at,
			created_at, updated_at
		)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13::jsonb, $14::jsonb, $15::jsonb, $16, $17, $17
		WHERE EXISTS (SELECT 1 FROM admin_accounts WHERE user_id = $2 AND id = $3)
		ON CONFLICT (id) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			admin_account_id = EXCLUDED.admin_account_id,
			name = EXCLUDED.name,
			base_url = EXCLUDED.base_url,
			platform = EXCLUDED.platform,
			requested_platform = EXCLUDED.requested_platform,
			account = EXCLUDED.account,
			remark = EXCLUDED.remark,
			recharge_rate = EXCLUDED.recharge_rate,
			status = EXCLUDED.status,
			error_key = EXCLUDED.error_key,
			metrics = EXCLUDED.metrics,
			session = EXCLUDED.session,
			settings = EXCLUDED.settings,
			last_synced_at = EXCLUDED.last_synced_at,
			updated_at = EXCLUDED.updated_at
		WHERE EXISTS (SELECT 1 FROM admin_accounts WHERE user_id = EXCLUDED.user_id AND id = EXCLUDED.admin_account_id)
	`, site.ID, site.UserID, site.AdminAccountID, site.Name, site.BaseURL, site.Platform, site.RequestedPlatform, site.Account, site.Remark,
		site.RechargeRate, site.Status, site.ErrorKey, string(metricsJSON), nullableJSONString(sessionJSON), string(settingsJSON), site.LastSyncedAt, now)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return newRequestError(ErrorNotFound, "")
	}
	return nil
}

func (r *Repository) DeleteSite(ctx context.Context, userID string, id string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// group_rate_snapshots intentionally stays independent from upstream_sites for
	// reads, but deleting a station is a user-visible lifecycle action and must
	// clear the station's multiplier history in the same transaction.
	if _, err := tx.Exec(ctx, `DELETE FROM group_rate_snapshots WHERE user_id = $1 AND site_id = $2`, userID, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM upstream_sites WHERE user_id = $1 AND id = $2`, userID, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func scanSites(rows pgx.Rows) ([]Site, error) {
	defer rows.Close()

	sites := make([]Site, 0)
	for rows.Next() {
		var site Site
		var metricsJSON []byte
		var sessionJSON []byte
		var settingsJSON []byte
		if err := rows.Scan(
			&site.ID,
			&site.UserID,
			&site.AdminAccountID,
			&site.Name,
			&site.BaseURL,
			&site.Platform,
			&site.RequestedPlatform,
			&site.Account,
			&site.Remark,
			&site.RechargeRate,
			&site.Status,
			&site.ErrorKey,
			&metricsJSON,
			&sessionJSON,
			&settingsJSON,
			&site.LastSyncedAt,
		); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(metricsJSON, &site.Metrics); err != nil {
			return nil, err
		}
		if len(sessionJSON) > 0 {
			var session Session
			if err := json.Unmarshal(sessionJSON, &session); err != nil {
				return nil, err
			}
			site.Session = &session
		}
		if len(settingsJSON) > 0 {
			_ = json.Unmarshal(settingsJSON, &site.Settings)
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sites, nil
}

func nullableJSONString(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return string(value)
}
