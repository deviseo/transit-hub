package sales

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 只读「用户活跃分析」同步到本地的用户、每日用量与每日入账数据（金额为站点额度，即美元口径）。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// syncState 是用户活跃同步状态中销售分析需要的部分。
type syncState struct {
	Platform     string
	UsageFrom    *time.Time
	UsageTo      *time.Time
	RechargeFrom *time.Time
	RechargeTo   *time.Time
	LastSyncedAt *time.Time
}

// rawTotals 是一个区间的汇总（站点额度口径，未换算）。
type rawTotals struct {
	Credits        float64
	Deductions     float64
	Events         int
	Payers         int
	RepeatPayers   int
	NewPayers      int
	Registered     int
	RegisteredPaid int
	Consumption    float64
}

type dailyChannelRow struct {
	Day     time.Time
	Channel string
	Amount  float64
}

type dailyValueRow struct {
	Day   time.Time
	Value float64
}

func (r *Repository) SyncState(ctx context.Context, ws Workspace) (syncState, bool, error) {
	var state syncState
	err := r.db.QueryRow(ctx, `
		SELECT platform, synced_from, synced_to, recharge_synced_from, recharge_synced_to, last_synced_at
		FROM user_activity_sync_states WHERE user_id = $1 AND admin_account_id = $2
	`, ws.UserID, ws.AdminAccountID).Scan(&state.Platform, &state.UsageFrom, &state.UsageTo, &state.RechargeFrom, &state.RechargeTo, &state.LastSyncedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return syncState{}, false, nil
	}
	if err != nil {
		return syncState{}, false, err
	}
	return state, true, nil
}

// PeriodTotals 汇总 [from, to] 的入账、入账用户、复购、首次入账、新注册与转化、消耗。
// registeredFrom / registeredTo 是区间对应的时间戳边界（业务时区的 from 日 00:00 至 to 次日 00:00）。
func (r *Repository) PeriodTotals(ctx context.Context, ws Workspace, from, to time.Time, registeredFrom, registeredTo time.Time) (rawTotals, error) {
	var totals rawTotals
	var events int64
	err := r.db.QueryRow(ctx, `
		WITH r AS (
			SELECT site_user_id, day, amount, events
			FROM user_activity_recharge_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4
		), payers AS (
			SELECT site_user_id, count(DISTINCT day) AS days FROM r WHERE amount > 0 GROUP BY site_user_id
		), first_credit AS (
			SELECT site_user_id, min(day) AS first_day
			FROM user_activity_recharge_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND amount > 0 AND day <= $4
			GROUP BY site_user_id
		), registered AS (
			SELECT site_user_id FROM user_activity_profiles
			WHERE user_id = $1 AND admin_account_id = $2 AND site_created_at >= $5 AND site_created_at < $6
		)
		SELECT
			COALESCE((SELECT sum(amount) FROM r WHERE amount > 0), 0),
			COALESCE((SELECT -sum(amount) FROM r WHERE amount < 0), 0),
			COALESCE((SELECT sum(events) FROM r WHERE amount > 0), 0),
			(SELECT count(*) FROM payers),
			(SELECT count(*) FROM payers WHERE days >= 2),
			(SELECT count(*) FROM first_credit WHERE first_day >= $3),
			(SELECT count(*) FROM registered),
			(SELECT count(*) FROM registered g WHERE EXISTS (SELECT 1 FROM first_credit f WHERE f.site_user_id = g.site_user_id)),
			COALESCE((SELECT sum(cost) FROM user_activity_daily WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4), 0)
	`, ws.UserID, ws.AdminAccountID, from, to, registeredFrom, registeredTo).Scan(
		&totals.Credits, &totals.Deductions, &events, &totals.Payers, &totals.RepeatPayers, &totals.NewPayers,
		&totals.Registered, &totals.RegisteredPaid, &totals.Consumption,
	)
	totals.Events = int(events)
	return totals, err
}

// SiteBalance 返回某天最后一次同步时的全站用户余额；当天没有快照时返回 nil。
func (r *Repository) SiteBalance(ctx context.Context, ws Workspace, day time.Time) (*float64, error) {
	var balance float64
	err := r.db.QueryRow(ctx, `
		SELECT total_balance FROM user_activity_site_daily WHERE user_id = $1 AND admin_account_id = $2 AND day = $3
	`, ws.UserID, ws.AdminAccountID, day).Scan(&balance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &balance, nil
}

// Channels 按渠道汇总区间内的入账（金额带符号）。
func (r *Repository) Channels(ctx context.Context, ws Workspace, from, to time.Time) ([]ChannelTotal, error) {
	rows, err := r.db.Query(ctx, `
		SELECT channel, COALESCE(sum(amount), 0), COALESCE(sum(events), 0), count(DISTINCT site_user_id)
		FROM user_activity_recharge_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4
		GROUP BY channel
		ORDER BY sum(amount) DESC, channel
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := make([]ChannelTotal, 0)
	for rows.Next() {
		var item ChannelTotal
		var events int64
		if err := rows.Scan(&item.Channel, &item.Amount, &events, &item.Users); err != nil {
			return nil, err
		}
		item.Events = int(events)
		channels = append(channels, item)
	}
	return channels, rows.Err()
}

// DailyChannels 返回区间内每天每个渠道的入账合计。
func (r *Repository) DailyChannels(ctx context.Context, ws Workspace, from, to time.Time) ([]dailyChannelRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT day, channel, COALESCE(sum(amount), 0)
		FROM user_activity_recharge_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4
		GROUP BY day, channel
		ORDER BY day, channel
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dailyChannelRow, 0)
	for rows.Next() {
		var row dailyChannelRow
		if err := rows.Scan(&row.Day, &row.Channel, &row.Amount); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// DailyConsumption 返回区间内每天的用户消耗合计。
func (r *Repository) DailyConsumption(ctx context.Context, ws Workspace, from, to time.Time) ([]dailyValueRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT day, COALESCE(sum(cost), 0)
		FROM user_activity_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4
		GROUP BY day
		ORDER BY day
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]dailyValueRow, 0)
	for rows.Next() {
		var row dailyValueRow
		if err := rows.Scan(&row.Day, &row.Value); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// PayerAmounts 返回区间内每个入账用户的入账合计（从大到小），用于集中度与金额分布。
func (r *Repository) PayerAmounts(ctx context.Context, ws Workspace, from, to time.Time) ([]float64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT sum(amount) AS total
		FROM user_activity_recharge_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 AND amount > 0
		GROUP BY site_user_id
		ORDER BY total DESC
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	amounts := make([]float64, 0)
	for rows.Next() {
		var amount float64
		if err := rows.Scan(&amount); err != nil {
			return nil, err
		}
		amounts = append(amounts, amount)
	}
	return amounts, rows.Err()
}

// TopPayers 返回区间内入账最多的用户，附带渠道构成、区间内消耗和当前余额。
func (r *Repository) TopPayers(ctx context.Context, ws Workspace, from, to time.Time, limit int) ([]PayerRow, error) {
	rows, err := r.db.Query(ctx, `
		WITH per_user AS (
			SELECT site_user_id, sum(amount) AS credits, sum(events) AS events, count(DISTINCT day) AS days, max(day) AS last_day
			FROM user_activity_recharge_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 AND amount > 0
			GROUP BY site_user_id
			ORDER BY credits DESC, site_user_id
			LIMIT $5
		), mix AS (
			SELECT r.site_user_id, r.channel, sum(r.amount) AS amount, sum(r.events) AS events
			FROM user_activity_recharge_daily r
			JOIN per_user u ON u.site_user_id = r.site_user_id
			WHERE r.user_id = $1 AND r.admin_account_id = $2 AND r.day BETWEEN $3 AND $4 AND r.amount > 0
			GROUP BY r.site_user_id, r.channel
		), usage AS (
			SELECT d.site_user_id, sum(d.cost) AS cost
			FROM user_activity_daily d
			JOIN per_user u ON u.site_user_id = d.site_user_id
			WHERE d.user_id = $1 AND d.admin_account_id = $2 AND d.day BETWEEN $3 AND $4
			GROUP BY d.site_user_id
		)
		SELECT u.site_user_id, COALESCE(p.email, ''), COALESCE(p.username, ''), COALESCE(p.status, ''), p.balance,
			u.credits, u.events, u.days, u.last_day, COALESCE(us.cost, 0),
			COALESCE((
				SELECT json_agg(json_build_object('channel', m.channel, 'amount', m.amount, 'events', m.events) ORDER BY m.amount DESC, m.channel)
				FROM mix m WHERE m.site_user_id = u.site_user_id
			), '[]'::json)
		FROM per_user u
		LEFT JOIN user_activity_profiles p ON p.user_id = $1 AND p.admin_account_id = $2 AND p.site_user_id = u.site_user_id
		LEFT JOIN usage us ON us.site_user_id = u.site_user_id
		ORDER BY u.credits DESC, u.site_user_id
	`, ws.UserID, ws.AdminAccountID, from, to, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]PayerRow, 0)
	for rows.Next() {
		var row PayerRow
		var events, days int64
		var lastDay time.Time
		var mixJSON []byte
		if err := rows.Scan(&row.SiteUserID, &row.Email, &row.Username, &row.Status, &row.Balance,
			&row.Credits, &events, &days, &lastDay, &row.Consumption, &mixJSON); err != nil {
			return nil, err
		}
		row.Events = int(events)
		row.ActiveDays = int(days)
		row.LastCreditDay = lastDay.Format("2006-01-02")
		row.ChannelBreakout = make([]ChannelTotal, 0)
		if err := json.Unmarshal(mixJSON, &row.ChannelBreakout); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
