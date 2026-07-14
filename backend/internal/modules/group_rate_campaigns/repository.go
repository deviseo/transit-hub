package group_rate_campaigns

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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

// EnsureSchema 创建活动调价所需的两张表和索引。语句全部幂等（IF NOT EXISTS），
// 不修改任何已有表，符合线上兼容要求。
func (r *Repository) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS group_rate_campaigns (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			name text NOT NULL,
			description text NOT NULL DEFAULT '',
			status text NOT NULL,
			selection jsonb NOT NULL DEFAULT '{}'::jsonb,
			adjustment jsonb NOT NULL DEFAULT '{}'::jsonb,
			notify jsonb NOT NULL DEFAULT '{}'::jsonb,
			start_mode text NOT NULL,
			start_at timestamptz,
			end_mode text NOT NULL,
			end_at timestamptz,
			started_at timestamptz,
			ended_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS group_rate_campaign_items (
			id text PRIMARY KEY,
			campaign_id text NOT NULL,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			group_id text NOT NULL DEFAULT '',
			group_name text NOT NULL,
			original_multiplier double precision,
			campaign_multiplier double precision NOT NULL,
			restored_multiplier double precision,
			apply_status text NOT NULL DEFAULT 'pending',
			restore_status text NOT NULL DEFAULT 'pending',
			apply_reason text NOT NULL DEFAULT '',
			restore_reason text NOT NULL DEFAULT '',
			applied_at timestamptz,
			restored_at timestamptz,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_group_rate_campaigns_workspace ON group_rate_campaigns (user_id, admin_account_id, status, start_at, end_at)`,
		`CREATE INDEX IF NOT EXISTS idx_group_rate_campaign_items_campaign ON group_rate_campaign_items (campaign_id)`,
	}
	for _, stmt := range statements {
		if _, err := r.db.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// Insert 落库一个新活动（不含 items）。
func (r *Repository) Insert(ctx context.Context, c Campaign) error {
	selectionJSON, err := json.Marshal(c.Selection)
	if err != nil {
		return err
	}
	adjustmentJSON, err := json.Marshal(c.Adjustment)
	if err != nil {
		return err
	}
	notifyJSON, err := json.Marshal(c.Notify)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO group_rate_campaigns
			(id, user_id, admin_account_id, name, description, status, selection, adjustment, notify, start_mode, start_at, end_mode, end_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, now(), now())
	`, c.ID, c.UserID, c.AdminAccountID, c.Name, c.Description, c.Status, selectionJSON, adjustmentJSON, notifyJSON, string(c.StartMode), c.StartAt, string(c.EndMode), c.EndAt)
	return err
}

// SaveItems 批量插入活动的目标分组明细，初始状态为 pending/pending。
func (r *Repository) SaveItems(ctx context.Context, items []CampaignItem) error {
	if len(items) == 0 {
		return nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	for _, item := range items {
		if _, err := tx.Exec(ctx, `
			INSERT INTO group_rate_campaign_items
				(id, campaign_id, user_id, admin_account_id, group_id, group_name, original_multiplier, campaign_multiplier, apply_status, restore_status, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now(), now())
		`, item.ID, item.CampaignID, item.UserID, item.AdminAccountID, item.GroupID, item.GroupName, item.OriginalMultiplier, item.CampaignMultiplier, ItemPending, ItemPending); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// UpdateItemApply 记录一个分组的开启执行结果。
func (r *Repository) UpdateItemApply(ctx context.Context, id string, originalMultiplier *float64, status string, reason string, appliedAt *time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE group_rate_campaign_items
		SET original_multiplier = $2, apply_status = $3, apply_reason = $4, applied_at = $5, updated_at = now()
		WHERE id = $1
	`, id, originalMultiplier, status, reason, appliedAt)
	return err
}

// UpdateItemRestore 记录一个分组的恢复执行结果。
func (r *Repository) UpdateItemRestore(ctx context.Context, id string, restoredMultiplier *float64, status string, reason string, restoredAt *time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE group_rate_campaign_items
		SET restored_multiplier = $2, restore_status = $3, restore_reason = $4, restored_at = $5, updated_at = now()
		WHERE id = $1
	`, id, restoredMultiplier, status, reason, restoredAt)
	return err
}

// ListItems 返回一个活动的全部目标分组明细。
func (r *Repository) ListItems(ctx context.Context, campaignID string) ([]CampaignItem, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, campaign_id, user_id, admin_account_id, group_id, group_name, original_multiplier, campaign_multiplier, restored_multiplier,
			apply_status, restore_status, apply_reason, restore_reason, applied_at, restored_at, created_at, updated_at
		FROM group_rate_campaign_items
		WHERE campaign_id = $1
		ORDER BY group_name ASC
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]CampaignItem, 0)
	for rows.Next() {
		var item CampaignItem
		if err := rows.Scan(&item.ID, &item.CampaignID, &item.UserID, &item.AdminAccountID, &item.GroupID, &item.GroupName,
			&item.OriginalMultiplier, &item.CampaignMultiplier, &item.RestoredMultiplier,
			&item.ApplyStatus, &item.RestoreStatus, &item.ApplyReason, &item.RestoreReason,
			&item.AppliedAt, &item.RestoredAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// Get 按 workspace 范围读取单个活动，供 HTTP 接口使用（校验归属）。
func (r *Repository) Get(ctx context.Context, userID string, adminAccountID string, id string) (*Campaign, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, admin_account_id, name, description, status, selection, adjustment, notify,
			start_mode, start_at, end_mode, end_at, started_at, ended_at, created_at, updated_at
		FROM group_rate_campaigns
		WHERE id = $1 AND user_id = $2 AND admin_account_id = $3
	`, id, userID, adminAccountID)
	return scanCampaign(row)
}

// GetByID 按 ID 读取活动，不做 workspace 校验，供调度器/内部执行流程使用。
func (r *Repository) GetByID(ctx context.Context, id string) (*Campaign, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, admin_account_id, name, description, status, selection, adjustment, notify,
			start_mode, start_at, end_mode, end_at, started_at, ended_at, created_at, updated_at
		FROM group_rate_campaigns
		WHERE id = $1
	`, id)
	return scanCampaign(row)
}

func scanCampaign(row pgx.Row) (*Campaign, error) {
	var c Campaign
	var selectionJSON, adjustmentJSON, notifyJSON []byte
	var startMode, endMode, status string
	if err := row.Scan(&c.ID, &c.UserID, &c.AdminAccountID, &c.Name, &c.Description, &status, &selectionJSON, &adjustmentJSON, &notifyJSON,
		&startMode, &c.StartAt, &endMode, &c.EndAt, &c.StartedAt, &c.EndedAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	c.Status = status
	c.StartMode = StartMode(startMode)
	c.EndMode = EndMode(endMode)
	if err := json.Unmarshal(selectionJSON, &c.Selection); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(adjustmentJSON, &c.Adjustment); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(notifyJSON, &c.Notify); err != nil {
		return nil, err
	}
	return &c, nil
}

// List 分页返回一个 workspace 下的活动列表，可按状态筛选。
func (r *Repository) List(ctx context.Context, userID string, adminAccountID string, query ListQuery) ([]Campaign, int, error) {
	offset := (query.Page - 1) * query.PageSize
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, name, description, status, selection, adjustment, notify,
			start_mode, start_at, end_mode, end_at, started_at, ended_at, created_at, updated_at,
			count(*) OVER () AS total
		FROM group_rate_campaigns
		WHERE user_id = $1 AND admin_account_id = $2 AND ($3 = '' OR status = $3)
		ORDER BY created_at DESC
		LIMIT $4 OFFSET $5
	`, userID, adminAccountID, query.Status, query.PageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	campaigns := make([]Campaign, 0)
	total := 0
	for rows.Next() {
		var c Campaign
		var selectionJSON, adjustmentJSON, notifyJSON []byte
		var startMode, endMode, status string
		if err := rows.Scan(&c.ID, &c.UserID, &c.AdminAccountID, &c.Name, &c.Description, &status, &selectionJSON, &adjustmentJSON, &notifyJSON,
			&startMode, &c.StartAt, &endMode, &c.EndAt, &c.StartedAt, &c.EndedAt, &c.CreatedAt, &c.UpdatedAt, &total); err != nil {
			return nil, 0, err
		}
		c.Status = status
		c.StartMode = StartMode(startMode)
		c.EndMode = EndMode(endMode)
		if err := json.Unmarshal(selectionJSON, &c.Selection); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(adjustmentJSON, &c.Adjustment); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(notifyJSON, &c.Notify); err != nil {
			return nil, 0, err
		}
		campaigns = append(campaigns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return campaigns, total, nil
}

// ListDueScheduled 返回所有到达开始时间、尚未开启的活动，供调度器扫描。
func (r *Repository) ListDueScheduled(ctx context.Context, now time.Time) ([]string, error) {
	return r.listIDs(ctx, `SELECT id FROM group_rate_campaigns WHERE status = 'scheduled' AND start_at IS NOT NULL AND start_at <= $1`, now)
}

// ListDueRunning 返回所有到达结束时间、仍需恢复的活动，供调度器扫描。
// 覆盖 running 和 partial 两种状态：partial 表示开启阶段部分分组失败，
// 但仍有分组已经真实改价，到期时同样必须恢复。
func (r *Repository) ListDueRunning(ctx context.Context, now time.Time) ([]string, error) {
	return r.listIDs(ctx, `SELECT id FROM group_rate_campaigns WHERE status IN ('running', 'partial') AND end_mode = 'scheduled' AND end_at IS NOT NULL AND end_at <= $1`, now)
}

func (r *Repository) listIDs(ctx context.Context, sql string, args ...any) ([]string, error) {
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ClaimForRunning 用条件更新把 draft/scheduled 活动抢占为 running，返回是否抢占成功。
// 抢占失败（返回 false 且无 error）意味着活动已被其他调用处理，调用方应直接跳过，
// 这是幂等保护的核心：多次扫描同一活动只会有一次抢占成功。
func (r *Repository) ClaimForRunning(ctx context.Context, id string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE group_rate_campaigns SET status = 'running', started_at = now(), updated_at = now()
		WHERE id = $1 AND status IN ('draft', 'scheduled')
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// ClaimForEnding 用条件更新把 running/partial 活动抢占为 ending，返回是否抢占成功。
// partial 活动可能仍有已成功改价、尚未恢复的分组，必须允许进入 ending 才能被恢复。
func (r *Repository) ClaimForEnding(ctx context.Context, id string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE group_rate_campaigns SET status = 'ending', updated_at = now()
		WHERE id = $1 AND status IN ('running', 'partial')
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// FinishStart 在开启执行完成后写入最终状态（running/partial/failed）。
func (r *Repository) FinishStart(ctx context.Context, id string, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE group_rate_campaigns SET status = $2, updated_at = now() WHERE id = $1`, id, status)
	return err
}

// FinishEnd 在恢复执行完成后写入最终状态（ended/partial）并记录结束时间。
func (r *Repository) FinishEnd(ctx context.Context, id string, status string) error {
	_, err := r.db.Exec(ctx, `UPDATE group_rate_campaigns SET status = $2, ended_at = now(), updated_at = now() WHERE id = $1`, id, status)
	return err
}

// ClaimForCancel 用条件更新把 draft/scheduled 活动抢占为 cancelled，返回是否抢占成功。
func (r *Repository) ClaimForCancel(ctx context.Context, id string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE group_rate_campaigns SET status = 'cancelled', updated_at = now()
		WHERE id = $1 AND status IN ('draft', 'scheduled')
	`, id)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New("generate group rate campaign id")
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
