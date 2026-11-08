package vip

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 读「用户分析」同步到本地的用户、每日用量和入账（金额为站点额度，即美元口径），
// 读写大户关注自己的名单、设置和提醒记录。不访问站点。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// syncState 是用户分析的同步情况（大户关注只需要平台和最近同步时间）。
type syncState struct {
	Platform     string
	LastSyncedAt *time.Time
}

// statRow 是一个用户的原始数据（站点额度口径，未换算）。
type statRow struct {
	SiteUserID   string
	Email        string
	Username     string
	UserStatus   string
	Balance      *float64
	Cost7        float64
	CostPrev7    float64
	Lifetime     float64
	LastUsedAt   *time.Time
	CreditDay    *time.Time
	CreditAmount float64
	Kind         string
	Note         string
	DailyRank    int
	LifetimeRank int
}

// windows 是近 7 天和再往前 7 天（完整自然日，业务时区）。
type windows struct {
	From7, To7         time.Time
	PrevFrom7, PrevTo7 time.Time
}

// alertState 是上次提醒站长时的状态。
type alertState struct {
	State      string
	NotifiedAt time.Time
	Recovered  bool
}

func (r *Repository) SyncState(ctx context.Context, ws Workspace) (syncState, bool, error) {
	var state syncState
	err := r.db.QueryRow(ctx, `
		SELECT platform, last_synced_at FROM user_activity_sync_states WHERE user_id = $1 AND admin_account_id = $2
	`, ws.UserID, ws.AdminAccountID).Scan(&state.Platform, &state.LastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return syncState{}, false, nil
	}
	if err != nil {
		return syncState{}, false, err
	}
	return state, true, nil
}

// Watchlist 返回名单：手动关注的，加上近 7 天消耗前 dailyTop 名、累计消耗前 lifetimeTop 名
// （停用的、移出过的不推荐）。按近 7 天消耗从多到少排。
func (r *Repository) Watchlist(ctx context.Context, ws Workspace, win windows, dailyTop, lifetimeTop int) ([]statRow, error) {
	rows, err := r.db.Query(ctx, `
		WITH usage AS (
			SELECT site_user_id,
				COALESCE(sum(cost) FILTER (WHERE day BETWEEN $3 AND $4), 0) AS cost_7,
				COALESCE(sum(cost) FILTER (WHERE day BETWEEN $5 AND $6), 0) AS cost_prev7,
				COALESCE(sum(cost), 0) AS cost_all
			FROM user_activity_daily
			WHERE user_id = $1 AND admin_account_id = $2
			GROUP BY site_user_id
		), credit AS (
			SELECT DISTINCT ON (site_user_id) site_user_id, day, amount
			FROM (
				SELECT site_user_id, day, sum(amount) AS amount
				FROM user_activity_recharge_daily
				WHERE user_id = $1 AND admin_account_id = $2 AND amount > 0
				GROUP BY site_user_id, day
			) d
			ORDER BY site_user_id, day DESC
		), base AS (
			SELECT p.site_user_id, p.email, p.username, p.status, p.balance, p.last_used_at,
				COALESCE(u.cost_7, 0) AS cost_7, COALESCE(u.cost_prev7, 0) AS cost_prev7,
				-- 累计消耗：new-api 用站点的累计已用；sub2api 没有，用本地保存的每日消耗合计。
				COALESCE(p.total_used, u.cost_all, 0) AS lifetime,
				c.day AS credit_day, COALESCE(c.amount, 0) AS credit_amount,
				COALESCE(w.kind, '') AS kind, COALESCE(w.note, '') AS note,
				p.status IS DISTINCT FROM 'disabled' AND w.kind IS DISTINCT FROM 'dismissed' AS recommendable
			FROM user_activity_profiles p
			LEFT JOIN usage u ON u.site_user_id = p.site_user_id
			LEFT JOIN credit c ON c.site_user_id = p.site_user_id
			LEFT JOIN vip_watch w
				ON w.user_id = p.user_id AND w.admin_account_id = p.admin_account_id AND w.site_user_id = p.site_user_id
			WHERE p.user_id = $1 AND p.admin_account_id = $2
		), ranked AS (
			SELECT *,
				CASE WHEN recommendable AND cost_7 > 0 THEN
					row_number() OVER (PARTITION BY recommendable AND cost_7 > 0 ORDER BY cost_7 DESC, site_user_id)
				END AS daily_rank,
				CASE WHEN recommendable AND lifetime > 0 THEN
					row_number() OVER (PARTITION BY recommendable AND lifetime > 0 ORDER BY lifetime DESC, site_user_id)
				END AS lifetime_rank
			FROM base
		)
		SELECT site_user_id, email, username, status, balance, cost_7, cost_prev7, lifetime, last_used_at,
			credit_day, credit_amount, kind, note,
			CASE WHEN daily_rank <= $7 THEN daily_rank ELSE 0 END,
			CASE WHEN lifetime_rank <= $8 THEN lifetime_rank ELSE 0 END
		FROM ranked
		WHERE kind = 'manual' OR daily_rank <= $7 OR lifetime_rank <= $8
		ORDER BY cost_7 DESC, lifetime DESC, site_user_id
	`, ws.UserID, ws.AdminAccountID, win.From7, win.To7, win.PrevFrom7, win.PrevTo7, dailyTop, lifetimeTop)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]statRow, 0)
	for rows.Next() {
		var row statRow
		var dailyRank, lifetimeRank int64
		if err := rows.Scan(&row.SiteUserID, &row.Email, &row.Username, &row.UserStatus, &row.Balance, &row.Cost7, &row.CostPrev7,
			&row.Lifetime, &row.LastUsedAt, &row.CreditDay, &row.CreditAmount, &row.Kind, &row.Note, &dailyRank, &lifetimeRank); err != nil {
			return nil, err
		}
		row.DailyRank = int(dailyRank)
		row.LifetimeRank = int(lifetimeRank)
		result = append(result, row)
	}
	return result, rows.Err()
}

// DismissedCount 返回移出过、不再推荐的人数。
func (r *Repository) DismissedCount(ctx context.Context, ws Workspace) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM vip_watch WHERE user_id = $1 AND admin_account_id = $2 AND kind = 'dismissed'
	`, ws.UserID, ws.AdminAccountID).Scan(&count)
	return count, err
}

// Search 按邮箱、用户名或站点用户 ID 找用户（添加关注用），近 7 天用得多的在前。
func (r *Repository) Search(ctx context.Context, ws Workspace, win windows, query string, limit int) ([]Candidate, []float64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.site_user_id, p.email, p.username, p.balance,
			COALESCE((
				SELECT sum(d.cost) FROM user_activity_daily d
				WHERE d.user_id = p.user_id AND d.admin_account_id = p.admin_account_id AND d.site_user_id = p.site_user_id
					AND d.day BETWEEN $4 AND $5
			), 0) AS cost_7,
			COALESCE(w.kind, '')
		FROM user_activity_profiles p
		LEFT JOIN vip_watch w
			ON w.user_id = p.user_id AND w.admin_account_id = p.admin_account_id AND w.site_user_id = p.site_user_id
		WHERE p.user_id = $1 AND p.admin_account_id = $2
			AND (p.site_user_id = $3 OR strpos(lower(p.email), lower($3)) > 0 OR strpos(lower(p.username), lower($3)) > 0)
		ORDER BY cost_7 DESC, p.site_user_id
		LIMIT $6
	`, ws.UserID, ws.AdminAccountID, query, win.From7, win.To7, limit)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	candidates := make([]Candidate, 0)
	costs := make([]float64, 0)
	for rows.Next() {
		var item Candidate
		var cost float64
		var kind string
		if err := rows.Scan(&item.SiteUserID, &item.Email, &item.Username, &item.Balance, &cost, &kind); err != nil {
			return nil, nil, err
		}
		item.Watched = kind == kindManual
		item.Dismissed = kind == kindDismissed
		candidates = append(candidates, item)
		costs = append(costs, cost)
	}
	return candidates, costs, rows.Err()
}

// Watch 手动关注（或者更新备注）；之前移出过的也回到名单。用户不存在时返回 false。
func (r *Repository) Watch(ctx context.Context, ws Workspace, siteUserID string, note string) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO vip_watch (user_id, admin_account_id, site_user_id, kind, note)
		SELECT $1, $2, $3, 'manual', $4
		WHERE EXISTS (
			SELECT 1 FROM user_activity_profiles WHERE user_id = $1 AND admin_account_id = $2 AND site_user_id = $3
		)
		ON CONFLICT (user_id, admin_account_id, site_user_id) DO UPDATE
		SET kind = 'manual', note = EXCLUDED.note, updated_at = now()
	`, ws.UserID, ws.AdminAccountID, siteUserID, note)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// Unwatch 把这个人移出名单：手动关注的取消，系统也不再推荐他。
func (r *Repository) Unwatch(ctx context.Context, ws Workspace, siteUserID string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO vip_watch (user_id, admin_account_id, site_user_id, kind, note)
		VALUES ($1, $2, $3, 'dismissed', '')
		ON CONFLICT (user_id, admin_account_id, site_user_id) DO UPDATE
		SET kind = 'dismissed', note = '', updated_at = now()
	`, ws.UserID, ws.AdminAccountID, siteUserID)
	return err
}

// RestoreDismissed 让移出过的人重新参加推荐，返回恢复了几个。
func (r *Repository) RestoreDismissed(ctx context.Context, ws Workspace) (int, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM vip_watch WHERE user_id = $1 AND admin_account_id = $2 AND kind = 'dismissed'
	`, ws.UserID, ws.AdminAccountID)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// Settings 返回设置；没保存过时返回默认值。
func (r *Repository) Settings(ctx context.Context, ws Workspace) (Settings, error) {
	settings := DefaultSettings()
	err := r.db.QueryRow(ctx, `
		SELECT daily_top, lifetime_top, alert_days, alert_balance, alerts_enabled, bot_ids, reminder_text, empty_reminder_text
		FROM vip_settings WHERE user_id = $1 AND admin_account_id = $2
	`, ws.UserID, ws.AdminAccountID).Scan(&settings.DailyTop, &settings.LifetimeTop, &settings.AlertDays, &settings.AlertBalance,
		&settings.AlertsEnabled, &settings.BotIDs, &settings.ReminderText, &settings.EmptyReminderText)
	if errors.Is(err, pgx.ErrNoRows) {
		return DefaultSettings(), nil
	}
	if settings.BotIDs == nil {
		settings.BotIDs = []string{}
	}
	return settings, err
}

func (r *Repository) SaveSettings(ctx context.Context, ws Workspace, settings Settings) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO vip_settings (user_id, admin_account_id, daily_top, lifetime_top, alert_days, alert_balance,
			alerts_enabled, bot_ids, reminder_text, empty_reminder_text, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET
			daily_top = EXCLUDED.daily_top, lifetime_top = EXCLUDED.lifetime_top,
			alert_days = EXCLUDED.alert_days, alert_balance = EXCLUDED.alert_balance,
			alerts_enabled = EXCLUDED.alerts_enabled, bot_ids = EXCLUDED.bot_ids,
			reminder_text = EXCLUDED.reminder_text, empty_reminder_text = EXCLUDED.empty_reminder_text,
			updated_at = now()
	`, ws.UserID, ws.AdminAccountID, settings.DailyTop, settings.LifetimeTop, settings.AlertDays, settings.AlertBalance,
		settings.AlertsEnabled, settings.BotIDs, settings.ReminderText, settings.EmptyReminderText)
	return err
}

// AlertWorkspaces 返回打开了提醒、选了机器人、并且同步过用户数据的工作区。
func (r *Repository) AlertWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := r.db.Query(ctx, `
		SELECT s.user_id, s.admin_account_id
		FROM vip_settings s
		JOIN user_activity_sync_states a ON a.user_id = s.user_id AND a.admin_account_id = s.admin_account_id
		WHERE s.alerts_enabled AND cardinality(s.bot_ids) > 0
		ORDER BY s.user_id, s.admin_account_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Workspace, 0)
	for rows.Next() {
		var ws Workspace
		if err := rows.Scan(&ws.UserID, &ws.AdminAccountID); err != nil {
			return nil, err
		}
		result = append(result, ws)
	}
	return result, rows.Err()
}

// AlertStates 返回各大户上次提醒时的状态。
func (r *Repository) AlertStates(ctx context.Context, ws Workspace) (map[string]alertState, error) {
	rows, err := r.db.Query(ctx, `
		SELECT site_user_id, state, notified_at, recovered FROM vip_alert_states WHERE user_id = $1 AND admin_account_id = $2
	`, ws.UserID, ws.AdminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := map[string]alertState{}
	for rows.Next() {
		var id string
		var state alertState
		if err := rows.Scan(&id, &state.State, &state.NotifiedAt, &state.Recovered); err != nil {
			return nil, err
		}
		states[id] = state
	}
	return states, rows.Err()
}

// SaveAlertStates 记下这次提醒过的大户，把回升了的标记出来（下次降下来满一天再提醒）。
func (r *Repository) SaveAlertStates(ctx context.Context, ws Workspace, notified map[string]string, at time.Time, recovered []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for id, state := range notified {
		if _, err := tx.Exec(ctx, `
			INSERT INTO vip_alert_states (user_id, admin_account_id, site_user_id, state, notified_at, recovered)
			VALUES ($1, $2, $3, $4, $5, false)
			ON CONFLICT (user_id, admin_account_id, site_user_id) DO UPDATE
			SET state = EXCLUDED.state, notified_at = EXCLUDED.notified_at, recovered = false
		`, ws.UserID, ws.AdminAccountID, id, state, at); err != nil {
			return err
		}
	}
	if len(recovered) > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE vip_alert_states SET recovered = true
			WHERE user_id = $1 AND admin_account_id = $2 AND site_user_id = ANY($3)
		`, ws.UserID, ws.AdminAccountID, recovered); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
