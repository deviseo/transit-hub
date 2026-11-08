package sales

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 只读「用户活跃分析」同步到本地的用户、每日用量与每日入账数据（金额为站点额度，即美元口径），
// 另外读写用户选择不计入的入账（sales_excluded_entries、sales_settings）。
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

// excludedEpsilon：减去不计入的金额后剩下的零头（浮点误差）当作 0。
const excludedEpsilon = 1e-6

// rechargeSource 是销售分析用的入账（放在 CTE 里用，$1、$2 是工作区）：减去用户选择不计入的明细
// （按排除时记下的金额和笔数，最多减到 0）；dropChannels 时再去掉整个不计入的渠道。
// raw_amount 是站点同步来的原始金额，excluded_amount 是这一行里不计入的部分。
func rechargeSource(dropChannels bool) string {
	source := `
		SELECT r.site_user_id, r.day, r.channel, r.amount AS raw_amount,
			CASE
				WHEN e.site_user_id IS NULL OR r.amount <= 0 THEN r.amount
				WHEN r.amount - e.amount > ` + excludedEpsilonSQL + ` THEN r.amount - e.amount
				ELSE 0
			END AS amount,
			CASE
				WHEN e.site_user_id IS NULL OR r.amount <= 0 THEN r.events
				ELSE GREATEST(r.events - e.events, 0)
			END AS events,
			CASE
				WHEN e.site_user_id IS NULL OR r.amount <= 0 THEN 0
				ELSE LEAST(e.amount, r.amount)
			END AS excluded_amount,
			e.site_user_id IS NOT NULL AND r.amount > 0 AS has_exclusion
		FROM user_activity_recharge_daily r
		LEFT JOIN sales_excluded_entries e
			ON e.user_id = r.user_id AND e.admin_account_id = r.admin_account_id
			AND e.site_user_id = r.site_user_id AND e.day = r.day AND e.channel = r.channel
		WHERE r.user_id = $1 AND r.admin_account_id = $2`
	if dropChannels {
		source += `
			AND r.channel <> ALL(COALESCE((SELECT s.excluded_channels FROM sales_settings s
				WHERE s.user_id = $1 AND s.admin_account_id = $2), '{}'::text[]))`
	}
	return source
}

const excludedEpsilonSQL = "0.000001"

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
		WITH src AS (`+rechargeSource(true)+`
		), r AS (
			SELECT site_user_id, day, amount, events FROM src WHERE day BETWEEN $3 AND $4
		), payers AS (
			SELECT site_user_id, count(DISTINCT day) AS days FROM r WHERE amount > 0 GROUP BY site_user_id
		), first_credit AS (
			SELECT site_user_id, min(day) AS first_day
			FROM src
			WHERE amount > 0 AND day <= $4
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

// Channels 按渠道汇总区间内的入账（金额带符号，已减去不计入的明细），附带不计入的金额和条数。
// 整个不计入的渠道也列出来（由服务层标记），方便在页面上重新打开。
func (r *Repository) Channels(ctx context.Context, ws Workspace, from, to time.Time) ([]ChannelTotal, error) {
	rows, err := r.db.Query(ctx, `
		WITH src AS (`+rechargeSource(false)+`
			AND r.day BETWEEN $3 AND $4)
		SELECT channel, COALESCE(sum(amount), 0), COALESCE(sum(events), 0),
			count(DISTINCT site_user_id) FILTER (WHERE amount <> 0),
			COALESCE(sum(excluded_amount), 0), count(*) FILTER (WHERE has_exclusion)
		FROM src
		GROUP BY channel
		ORDER BY sum(raw_amount) DESC, channel
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	channels := make([]ChannelTotal, 0)
	for rows.Next() {
		var item ChannelTotal
		var events, excludedEntries int64
		if err := rows.Scan(&item.Channel, &item.Amount, &events, &item.Users, &item.ExcludedAmount, &excludedEntries); err != nil {
			return nil, err
		}
		item.Events = int(events)
		item.ExcludedEntries = int(excludedEntries)
		channels = append(channels, item)
	}
	return channels, rows.Err()
}

// DailyChannels 返回区间内每天每个渠道的入账合计。
func (r *Repository) DailyChannels(ctx context.Context, ws Workspace, from, to time.Time) ([]dailyChannelRow, error) {
	rows, err := r.db.Query(ctx, `
		WITH src AS (`+rechargeSource(true)+`
			AND r.day BETWEEN $3 AND $4)
		SELECT day, channel, COALESCE(sum(amount), 0)
		FROM src
		GROUP BY day, channel
		HAVING sum(amount) <> 0
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
		WITH src AS (`+rechargeSource(true)+`
			AND r.day BETWEEN $3 AND $4)
		SELECT sum(amount) AS total
		FROM src
		WHERE amount > 0
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
		WITH src AS (`+rechargeSource(true)+`
			AND r.day BETWEEN $3 AND $4
		), per_user AS (
			SELECT site_user_id, sum(amount) AS credits, sum(events) AS events, count(DISTINCT day) AS days, max(day) AS last_day
			FROM src
			WHERE amount > 0
			GROUP BY site_user_id
			ORDER BY credits DESC, site_user_id
			LIMIT $5
		), mix AS (
			SELECT s.site_user_id, s.channel, sum(s.amount) AS amount, sum(s.events) AS events
			FROM src s
			JOIN per_user u ON u.site_user_id = s.site_user_id
			WHERE s.amount > 0
			GROUP BY s.site_user_id, s.channel
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

// ExcludedChannels 返回整个不计入的渠道（没设置过时为空）。
func (r *Repository) ExcludedChannels(ctx context.Context, ws Workspace) ([]string, error) {
	var channels []string
	err := r.db.QueryRow(ctx, `
		SELECT excluded_channels FROM sales_settings WHERE user_id = $1 AND admin_account_id = $2
	`, ws.UserID, ws.AdminAccountID).Scan(&channels)
	if errors.Is(err, pgx.ErrNoRows) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	if channels == nil {
		channels = []string{}
	}
	return channels, nil
}

// SetExcludedChannels 保存整个不计入的渠道。
func (r *Repository) SetExcludedChannels(ctx context.Context, ws Workspace, channels []string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO sales_settings (user_id, admin_account_id, excluded_channels, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE
		SET excluded_channels = EXCLUDED.excluded_channels, updated_at = now()
	`, ws.UserID, ws.AdminAccountID, channels)
	return err
}

// entryFilter 是入账明细的筛选条件（金额为站点额度口径）。
type entryFilter struct {
	From, To     time.Time
	Channel      string
	Search       string
	ExcludedOnly bool
	Limit        int
	Offset       int
}

// Entries 列出区间内的正向入账（每行是「用户 × 天 × 渠道」），附带不计入的部分；返回这一页和符合条件的总条数。
func (r *Repository) Entries(ctx context.Context, ws Workspace, filter entryFilter) ([]EntryRow, int, error) {
	const where = `
		FROM user_activity_recharge_daily r
		LEFT JOIN sales_excluded_entries e
			ON e.user_id = r.user_id AND e.admin_account_id = r.admin_account_id
			AND e.site_user_id = r.site_user_id AND e.day = r.day AND e.channel = r.channel
		LEFT JOIN user_activity_profiles p
			ON p.user_id = r.user_id AND p.admin_account_id = r.admin_account_id AND p.site_user_id = r.site_user_id
		WHERE r.user_id = $1 AND r.admin_account_id = $2 AND r.day BETWEEN $3 AND $4 AND r.amount > 0
			AND ($5 = '' OR r.channel = $5)
			AND ($6 = '' OR r.site_user_id = $6
				OR strpos(lower(COALESCE(p.email, '')), lower($6)) > 0
				OR strpos(lower(COALESCE(p.username, '')), lower($6)) > 0)
			AND (NOT $7 OR e.site_user_id IS NOT NULL)`
	args := []any{ws.UserID, ws.AdminAccountID, filter.From, filter.To, filter.Channel, filter.Search, filter.ExcludedOnly}
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT r.site_user_id, COALESCE(p.email, ''), COALESCE(p.username, ''), r.day, r.channel, r.amount, r.events,
			-- LEAST 会忽略 NULL：没有排除时要明确给 0。
			CASE WHEN e.site_user_id IS NULL THEN 0 ELSE LEAST(e.amount, r.amount) END,
			CASE WHEN e.site_user_id IS NULL THEN 0 ELSE LEAST(e.events, r.events) END
		`+where+`
		ORDER BY r.day DESC, r.amount DESC, r.site_user_id, r.channel
		LIMIT $8 OFFSET $9
	`, append(args, filter.Limit, filter.Offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	items := make([]EntryRow, 0)
	for rows.Next() {
		var item EntryRow
		var day time.Time
		var events, excludedEvents int64
		if err := rows.Scan(&item.SiteUserID, &item.Email, &item.Username, &day, &item.Channel, &item.Amount, &events,
			&item.ExcludedAmount, &excludedEvents); err != nil {
			return nil, 0, err
		}
		item.Day = day.Format("2006-01-02")
		item.Events = int(events)
		item.ExcludedEvents = int(excludedEvents)
		items = append(items, item)
	}
	return items, total, rows.Err()
}

// ExcludeEntries 把这些明细（现在的金额和笔数）记为不计入；已经排除过的更新成现在的金额。
// 只排除正向入账；找不到的（站点上没有这一行）跳过。返回排除了几条。
func (r *Repository) ExcludeEntries(ctx context.Context, ws Workspace, keys []EntryKey) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	count := 0
	for _, key := range keys {
		tag, err := tx.Exec(ctx, `
			INSERT INTO sales_excluded_entries (user_id, admin_account_id, site_user_id, day, channel, amount, events)
			SELECT user_id, admin_account_id, site_user_id, day, channel, amount, events
			FROM user_activity_recharge_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND site_user_id = $3 AND day = $4 AND channel = $5 AND amount > 0
			ON CONFLICT (user_id, admin_account_id, site_user_id, day, channel) DO UPDATE
			SET amount = EXCLUDED.amount, events = EXCLUDED.events, created_at = now()
		`, ws.UserID, ws.AdminAccountID, key.SiteUserID, key.day, key.Channel)
		if err != nil {
			return 0, err
		}
		count += int(tag.RowsAffected())
	}
	return count, tx.Commit(ctx)
}

// RestoreEntries 把这些明细重新计入。返回恢复了几条。
func (r *Repository) RestoreEntries(ctx context.Context, ws Workspace, keys []EntryKey) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	count := 0
	for _, key := range keys {
		tag, err := tx.Exec(ctx, `
			DELETE FROM sales_excluded_entries
			WHERE user_id = $1 AND admin_account_id = $2 AND site_user_id = $3 AND day = $4 AND channel = $5
		`, ws.UserID, ws.AdminAccountID, key.SiteUserID, key.day, key.Channel)
		if err != nil {
			return 0, err
		}
		count += int(tag.RowsAffected())
	}
	return count, tx.Commit(ctx)
}
