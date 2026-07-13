package group_rates

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	// group_rate_snapshots deliberately has no foreign key to upstream sites: those sites are
	// stored in memory, while rate history must survive process restarts independently.
	if _, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS group_rate_snapshots (
			id text PRIMARY KEY,
			user_id text NOT NULL DEFAULT '',
			site_id text NOT NULL,
			site_name text NOT NULL,
			group_name text NOT NULL,
			platform text NOT NULL,
			type text NOT NULL DEFAULT '',
			multiplier double precision NOT NULL,
			created_at timestamptz NOT NULL
		)
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		UPDATE group_rate_snapshots SET platform = '' WHERE platform IS NULL
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE group_rate_snapshots ALTER COLUMN platform SET NOT NULL
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE group_rate_snapshots ADD COLUMN IF NOT EXISTS type text NOT NULL DEFAULT ''
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE group_rate_snapshots ADD COLUMN IF NOT EXISTS user_id text NOT NULL DEFAULT ''
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE group_rate_snapshots ADD COLUMN IF NOT EXISTS admin_account_id text NOT NULL DEFAULT ''
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		ALTER TABLE group_rate_snapshots ADD COLUMN IF NOT EXISTS group_id text NOT NULL DEFAULT ''
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		UPDATE group_rate_snapshots AS snapshots
		SET user_id = sites.user_id
		FROM upstream_sites AS sites
		WHERE snapshots.user_id = '' AND snapshots.site_id = sites.id AND sites.user_id <> ''
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		UPDATE group_rate_snapshots SET type = '' WHERE type IN ('newapi', 'sub2api')
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		WITH inferred AS (
			SELECT empty.id, filled.type
			FROM group_rate_snapshots AS empty
			JOIN group_rate_snapshots AS filled
				ON empty.user_id = filled.user_id
				AND empty.site_id = filled.site_id
				AND empty.group_name = filled.group_name
				AND filled.type <> ''
				AND filled.type NOT IN ('newapi', 'sub2api')
			WHERE empty.type = ''
			ORDER BY filled.created_at DESC
		)
		UPDATE group_rate_snapshots AS snapshots
		SET type = inferred.type
		FROM inferred
		WHERE snapshots.id = inferred.id
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_group_rate_snapshots_group_latest
		ON group_rate_snapshots (user_id, site_id, group_id, group_name, created_at DESC)
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_group_rate_snapshots_latest_lookup
		ON group_rate_snapshots (user_id, site_id, group_id, group_name, created_at DESC, id DESC)
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_group_rate_snapshots_history_lookup
		ON group_rate_snapshots (user_id, site_id, group_name, platform, created_at DESC, id DESC)
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_group_rate_snapshots_user_created
		ON group_rate_snapshots (user_id, created_at DESC)
	`); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `
		CREATE INDEX IF NOT EXISTS idx_group_rate_snapshots_multiplier
		ON group_rate_snapshots (user_id, multiplier)
	`); err != nil {
		return err
	}
	// 已删除标记：上游同步时分组消失，将最新快照标记为 deleted，后续不再为其生成新快照。
	_, err := r.db.Exec(ctx, `
		ALTER TABLE group_rate_snapshots ADD COLUMN IF NOT EXISTS deleted boolean NOT NULL DEFAULT false
	`)
	return err
}

// LatestGroupKeysForSite 返回指定站点每个分组最新一行的 ID、group_id、group_name、倍率和 deleted 状态，
// 用于在同步时判断哪些分组已消失以及倍率是否发生变化。
func (r *Repository) LatestGroupKeysForSite(ctx context.Context, userID string, adminAccountID string, siteID string) ([]latestGroupKey, error) {
	rows, err := r.db.Query(ctx, `
		WITH ranked AS (
			SELECT
				id,
				group_id,
				group_name,
				deleted,
				multiplier,
				ROW_NUMBER() OVER (
					PARTITION BY COALESCE(NULLIF(group_id, ''), group_name)
					ORDER BY created_at DESC, id DESC
				) AS row_number
			FROM group_rate_snapshots
			WHERE user_id = $1 AND admin_account_id = $2 AND site_id = $3
		)
		SELECT id, group_id, group_name, deleted, multiplier
		FROM ranked
		WHERE row_number = 1
	`, userID, adminAccountID, siteID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []latestGroupKey
	for rows.Next() {
		var k latestGroupKey
		if err := rows.Scan(&k.ID, &k.GroupID, &k.GroupName, &k.Deleted, &k.Multiplier); err != nil {
			return nil, err
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// MarkDeleted 将指定 ID 的快照行标记为已删除。
func (r *Repository) MarkDeleted(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `
		UPDATE group_rate_snapshots SET deleted = true WHERE id = ANY($1)
	`, ids)
	return err
}

// TouchSnapshots 刷新指定快照的同步时间和站点名称，用于倍率未变时更新"最后确认"时间戳，
// 避免插入重复行导致涨跌幅只展示一个同步周期。
func (r *Repository) TouchSnapshots(ctx context.Context, ids []string, siteName string, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `
		UPDATE group_rate_snapshots SET created_at = $2, site_name = $3 WHERE id = ANY($1)
	`, ids, now, siteName)
	return err
}

func (r *Repository) InsertSnapshots(ctx context.Context, records []snapshotRecord) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, record := range records {
		if _, err := tx.Exec(ctx, `
			INSERT INTO group_rate_snapshots (id, user_id, admin_account_id, site_id, site_name, group_id, group_name, platform, type, multiplier, created_at)
			SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			WHERE EXISTS (SELECT 1 FROM admin_accounts WHERE user_id = $2 AND id = $3)
		`, record.ID, record.UserID, record.AdminAccountID, record.SiteID, record.SiteName, record.GroupID, record.GroupName, record.Platform, record.Type, record.Multiplier, record.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) List(ctx context.Context, userID string, adminAccountID string, query ListQuery) (listRecords, error) {
	search := strings.ToLower(strings.TrimSpace(query.Search))
	filterType := strings.TrimSpace(query.Type)
	filterPlatform := strings.TrimSpace(query.Platform)
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 {
		query.PageSize = 10
	}
	offset := (query.Page - 1) * query.PageSize

	// latest keeps one current row per site URL/group ID tuple, then the
	// filtered CTE applies admin list controls before LIMIT/OFFSET. Facets are
	// intentionally calculated from latest without active filters so the UI can
	// keep showing all available filter choices while one filter is selected.
	rows, err := r.db.Query(ctx, `
		WITH enriched AS (
			SELECT
				snapshots.id,
				snapshots.user_id,
				snapshots.site_id,
				snapshots.site_name,
				snapshots.group_id,
				snapshots.group_name,
				snapshots.platform,
				snapshots.type,
				snapshots.multiplier,
				snapshots.deleted,
				COALESCE(sites.recharge_rate, 1) AS recharge_rate,
				snapshots.created_at,
				COALESCE(sites.base_url, snapshots.site_id) AS site_key,
				COALESCE(NULLIF(snapshots.group_id, ''), snapshots.group_name) AS group_key
			FROM group_rate_snapshots AS snapshots
			LEFT JOIN upstream_sites AS sites
				ON sites.user_id = snapshots.user_id
				AND sites.id = snapshots.site_id
			WHERE snapshots.user_id = $1 AND snapshots.admin_account_id = $2
		), ranked AS (
			SELECT
				id,
				user_id,
				site_id,
				site_name,
				group_id,
				group_name,
				platform,
				type,
				multiplier,
				deleted,
				recharge_rate,
				created_at,
				ROW_NUMBER() OVER (
					PARTITION BY user_id, site_key, group_key
					ORDER BY created_at DESC, id DESC
				) AS row_number,
				LEAD(multiplier) OVER (
					PARTITION BY user_id, site_key, group_key
					ORDER BY created_at DESC, id DESC
				) AS previous_multiplier
			FROM enriched
		), latest AS (
			SELECT id, user_id, site_id, site_name, group_id, group_name, platform, type, multiplier, deleted, recharge_rate, created_at, previous_multiplier
			FROM ranked
			WHERE row_number = 1
		), mapped AS (
			SELECT latest.*, (
				EXISTS (
					SELECT 1
					FROM my_site_states AS states
					CROSS JOIN LATERAL jsonb_array_elements(states.mappings) AS mapping
					CROSS JOIN LATERAL jsonb_array_elements(mapping->'upstreamTargets') AS target
					WHERE states.user_id = latest.user_id
						AND states.admin_account_id = $2
						AND target->>'siteId' = latest.site_id
						AND target->>'groupName' = latest.group_name
				)
				OR EXISTS (
					SELECT 1
					FROM real_connections AS connections
					WHERE connections.user_id = latest.user_id
						AND connections.workspace_admin_account_id = $2
						AND connections.upstream_site_id = latest.site_id
						AND (
							connections.upstream_group_id = latest.group_id
							OR ((connections.upstream_group_id = '' OR latest.group_id = '')
								AND connections.upstream_group_name = latest.group_name)
						)
				)
			) AS mapped
			FROM latest
		), filtered AS (
			SELECT *
			FROM mapped
			WHERE ($3 = '' OR lower(site_name) LIKE '%' || $3 || '%' OR lower(group_name) LIKE '%' || $3 || '%' OR lower(platform) LIKE '%' || $3 || '%' OR lower(type) LIKE '%' || $3 || '%')
				AND ($4 = '' OR type = $4)
				AND ($5 = '' OR platform = $5)
		), facets AS (
			SELECT
				COALESCE(array_agg(DISTINCT type ORDER BY type) FILTER (WHERE type <> ''), ARRAY[]::text[]) AS types,
				COALESCE(array_agg(DISTINCT platform ORDER BY platform) FILTER (WHERE platform <> ''), ARRAY[]::text[]) AS platforms
			FROM latest
		), counted AS (
			SELECT count(*)::int AS total FROM filtered
		)
		SELECT filtered.id, filtered.user_id, filtered.site_id, filtered.site_name, filtered.group_id, filtered.group_name, filtered.platform, filtered.type, filtered.mapped, filtered.deleted,
			filtered.multiplier, filtered.recharge_rate, filtered.created_at, filtered.previous_multiplier, counted.total, facets.types, facets.platforms
		FROM filtered
		CROSS JOIN counted
		CROSS JOIN facets
		ORDER BY filtered.mapped DESC, filtered.multiplier * filtered.recharge_rate ASC, filtered.site_name ASC, filtered.group_name ASC, filtered.platform ASC, filtered.type ASC
		LIMIT $6 OFFSET $7
	`, userID, adminAccountID, search, filterType, filterPlatform, query.PageSize, offset)
	if err != nil {
		return listRecords{}, err
	}
	result, err := scanListSnapshots(rows)
	if err != nil {
		return listRecords{}, err
	}
	if result.Total == 0 {
		facets, err := r.facets(ctx, userID, adminAccountID)
		if err != nil {
			return listRecords{}, err
		}
		result.Types = facets.Types
		result.Platforms = facets.Platforms
	}
	return result, nil
}

// ListDistinctGroupNames 返回按 search/type/platform 匹配的最新分组快照的去重分组名列表，
// 供 group_rate_campaigns 模块的 "按分组类型"/"当前筛选结果" 选择模式与 admin 自有分组名取交集。
// 只看每个站点+分组的最新一行（与 List 的 ranked/latest CTE 同一语义），已删除分组不参与匹配。
func (r *Repository) ListDistinctGroupNames(ctx context.Context, userID string, adminAccountID string, search string, groupType string, platform string) ([]string, error) {
	search = strings.ToLower(strings.TrimSpace(search))
	groupType = strings.TrimSpace(groupType)
	platform = strings.TrimSpace(platform)
	rows, err := r.db.Query(ctx, `
		WITH enriched AS (
			SELECT
				snapshots.group_name,
				snapshots.platform,
				snapshots.type,
				snapshots.deleted,
				snapshots.created_at,
				snapshots.id,
				COALESCE(sites.base_url, snapshots.site_id) AS site_key,
				COALESCE(NULLIF(snapshots.group_id, ''), snapshots.group_name) AS group_key
			FROM group_rate_snapshots AS snapshots
			LEFT JOIN upstream_sites AS sites
				ON sites.user_id = snapshots.user_id
				AND sites.id = snapshots.site_id
			WHERE snapshots.user_id = $1 AND snapshots.admin_account_id = $2
		), ranked AS (
			SELECT group_name, platform, type, deleted,
				ROW_NUMBER() OVER (PARTITION BY site_key, group_key ORDER BY created_at DESC, id DESC) AS row_number
			FROM enriched
		)
		SELECT DISTINCT group_name
		FROM ranked
		WHERE row_number = 1
			AND NOT deleted
			AND ($3 = '' OR lower(group_name) LIKE '%' || $3 || '%')
			AND ($4 = '' OR type = $4)
			AND ($5 = '' OR platform = $5)
	`, userID, adminAccountID, search, groupType, platform)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}

func (r *Repository) UpdateType(ctx context.Context, userID string, adminAccountID string, ref GroupRef, groupType string) error {
	if userID == "" || adminAccountID == "" || ref.SiteID == "" || ref.GroupName == "" {
		return nil
	}
	_, err := r.db.Exec(ctx, `
		UPDATE group_rate_snapshots
		SET type = $5
		WHERE user_id = $1
			AND admin_account_id = $2
			AND site_id = $3
			AND (group_id = $4 OR group_name = $4)
	`, userID, adminAccountID, ref.SiteID, ref.GroupName, groupType)
	return err
}

func (r *Repository) History(ctx context.Context, userID string, adminAccountID string, siteID string, groupName string, platform string) ([]snapshotRecord, error) {
	rows, err := r.db.Query(ctx, `
		SELECT
			group_rate_snapshots.id,
			group_rate_snapshots.user_id,
			group_rate_snapshots.site_id,
			group_rate_snapshots.site_name,
			group_rate_snapshots.group_id,
			group_rate_snapshots.group_name,
			group_rate_snapshots.platform,
			group_rate_snapshots.type,
			group_rate_snapshots.multiplier,
			group_rate_snapshots.deleted,
			COALESCE(upstream_sites.recharge_rate, 1) AS recharge_rate,
			group_rate_snapshots.created_at,
			LEAD(group_rate_snapshots.multiplier) OVER (ORDER BY group_rate_snapshots.created_at DESC, group_rate_snapshots.id DESC) AS previous_multiplier
		FROM group_rate_snapshots
		LEFT JOIN upstream_sites
			ON upstream_sites.user_id = group_rate_snapshots.user_id
			AND upstream_sites.id = group_rate_snapshots.site_id
		WHERE group_rate_snapshots.user_id = $1 AND group_rate_snapshots.admin_account_id = $2 AND group_rate_snapshots.site_id = $3 AND (group_rate_snapshots.group_id = $4 OR (group_rate_snapshots.group_id = '' AND group_rate_snapshots.group_name = $4)) AND ($5 = '' OR group_rate_snapshots.platform = $5 OR group_rate_snapshots.type = $5)
		ORDER BY group_rate_snapshots.created_at DESC, group_rate_snapshots.id DESC
	`, userID, adminAccountID, siteID, groupName, platform)
	if err != nil {
		return nil, err
	}
	return scanSnapshots(rows)
}

func scanListSnapshots(rows pgxRows) (listRecords, error) {
	defer rows.Close()

	result := listRecords{Items: make([]snapshotRecord, 0)}
	for rows.Next() {
		var record snapshotRecord
		var previous sql.NullFloat64
		if err := rows.Scan(
			&record.ID,
			&record.UserID,
			&record.SiteID,
			&record.SiteName,
			&record.GroupID,
			&record.GroupName,
			&record.Platform,
			&record.Type,
			&record.Mapped,
			&record.Deleted,
			&record.Multiplier,
			&record.RechargeRate,
			&record.CreatedAt,
			&previous,
			&result.Total,
			&result.Types,
			&result.Platforms,
		); err != nil {
			return listRecords{}, err
		}
		if previous.Valid {
			value := previous.Float64
			record.PreviousMultiplier = &value
		}
		result.Items = append(result.Items, record)
	}
	if err := rows.Err(); err != nil {
		return listRecords{}, err
	}
	return result, nil
}

func scanSnapshots(rows pgxRows) ([]snapshotRecord, error) {
	defer rows.Close()

	records := make([]snapshotRecord, 0)
	for rows.Next() {
		var record snapshotRecord
		var previous sql.NullFloat64
		if err := rows.Scan(
			&record.ID,
			&record.UserID,
			&record.SiteID,
			&record.SiteName,
			&record.GroupID,
			&record.GroupName,
			&record.Platform,
			&record.Type,
			&record.Multiplier,
			&record.Deleted,
			&record.RechargeRate,
			&record.CreatedAt,
			&previous,
		); err != nil {
			return nil, err
		}
		if previous.Valid {
			value := previous.Float64
			record.PreviousMultiplier = &value
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *Repository) facets(ctx context.Context, userID string, adminAccountID string) (listRecords, error) {
	var result listRecords
	err := r.db.QueryRow(ctx, `
		WITH enriched AS (
			SELECT
				snapshots.user_id,
				snapshots.site_id,
				snapshots.group_name,
				COALESCE(NULLIF(snapshots.group_id, ''), snapshots.group_name) AS group_key,
				snapshots.platform,
				snapshots.type,
				snapshots.created_at,
				snapshots.id,
				COALESCE(sites.base_url, snapshots.site_id) AS site_key
			FROM group_rate_snapshots AS snapshots
			LEFT JOIN upstream_sites AS sites
				ON sites.user_id = snapshots.user_id
				AND sites.id = snapshots.site_id
			WHERE snapshots.user_id = $1 AND snapshots.admin_account_id = $2
		), ranked AS (
			SELECT platform, type,
				ROW_NUMBER() OVER (PARTITION BY user_id, site_key, group_key ORDER BY created_at DESC, id DESC) AS row_number
			FROM enriched
		), latest AS (
			SELECT platform, type FROM ranked WHERE row_number = 1
		)
		SELECT
			COALESCE(array_agg(DISTINCT type ORDER BY type) FILTER (WHERE type <> ''), ARRAY[]::text[]) AS types,
			COALESCE(array_agg(DISTINCT platform ORDER BY platform) FILTER (WHERE platform <> ''), ARRAY[]::text[]) AS platforms
		FROM latest
	`, userID, adminAccountID).Scan(&result.Types, &result.Platforms)
	return result, err
}

func newSnapshotID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New("generate group rate snapshot id")
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}

type pgxRows interface {
	Close()
	Next() bool
	Scan(dest ...any) error
	Err() error
}
