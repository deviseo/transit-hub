package user_activity

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 读写用户活跃分析相关的表（按 user_id + admin_account_id 工作区隔离）。
// 所有日期参数（day 列）都使用「业务时区的自然日、以 UTC 零点表示」的 time.Time。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Snapshot 是一次同步要写入的数据。
type Snapshot struct {
	// Profiles 为空时不更新用户快照（例如只回补历史用量）。
	Profiles []Profile
	// Days 是本次完整拉取过的日期：这些日期原有的每日用量会被整体替换为 Daily 中的数据。
	Days  []time.Time
	Daily []DailyUsage
	// LastUsed 是从用量数据推算出的最近使用时间（new-api 没有该字段），只会把 last_used_at 往后推。
	LastUsed map[string]time.Time
	// SyncStamp 是本次同步的时间戳：写入/更新的用户 updated_at 都是该值；PruneMissingProfiles 时
	// 删除没有被本次同步更新到的用户（站点上已删除的用户）。用应用侧时间戳，避免依赖数据库时钟。
	SyncStamp            time.Time
	PruneMissingProfiles bool
	// RetainFrom 非零时清理该日期之前的每日用量、入账与全站快照。
	RetainFrom time.Time
	// RechargeDays / Recharges：sub2api 充值记录按「用户 × 天 × 渠道」聚合后，整体替换这些日期的记录类入账。
	RechargeDays []time.Time
	Recharges    []RechargeDaily
	// FundedDeltaDay 非零时（new-api）先用 Profiles 中的 FundedTotal 与本地上次保存的值求差，
	// 把增加 / 减少累加到这一天的 credit / debit，再保存新的 FundedTotal。首次出现的用户没有基准，不计入。
	FundedDeltaDay time.Time
	// SiteDaily 非 nil 时写入（覆盖）当天的全站快照。
	SiteDaily *SiteDaily
}

// recordChannels 是 sub2api 充值记录产生的渠道，替换某天数据时只删除这些渠道。
var recordChannels = []string{ChannelPayment, ChannelRedeem, ChannelAdminAdd, ChannelAdminDeduct, ChannelAffiliate}

// fundedEpsilon 忽略浮点误差带来的极小差值（美元）。
const fundedEpsilon = 0.000001

// Windows 是计算「今天 / 近 7 天 / 近 30 天」等指标用到的日期。「近 N 天」截止到昨天（完整自然日），今天单独统计。
type Windows struct {
	Now        time.Time
	Today      time.Time
	Yesterday  time.Time
	From7      time.Time
	PrevFrom7  time.Time
	PrevTo7    time.Time
	From30     time.Time
	PrevFrom30 time.Time
	PrevTo30   time.Time
}

// NewWindows 根据业务时区的「今天」计算各统计窗口。
func NewWindows(now time.Time, today time.Time) Windows {
	yesterday := today.AddDate(0, 0, -1)
	from7 := yesterday.AddDate(0, 0, -6)
	from30 := yesterday.AddDate(0, 0, -29)
	return Windows{
		Now:        now,
		Today:      today,
		Yesterday:  yesterday,
		From7:      from7,
		PrevFrom7:  from7.AddDate(0, 0, -7),
		PrevTo7:    from7.AddDate(0, 0, -1),
		From30:     from30,
		PrevFrom30: from30.AddDate(0, 0, -30),
		PrevTo30:   from30.AddDate(0, 0, -1),
	}
}

const upsertProfileSQL = `
	INSERT INTO user_activity_profiles (
		user_id, admin_account_id, site_user_id, email, username, role, status, user_group,
		balance, total_recharged, total_used, funded_total, ever_used, site_created_at, last_used_at, first_seen_at, updated_at
	)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)
	ON CONFLICT (user_id, admin_account_id, site_user_id) DO UPDATE SET
		email = EXCLUDED.email,
		username = EXCLUDED.username,
		role = EXCLUDED.role,
		status = EXCLUDED.status,
		user_group = EXCLUDED.user_group,
		balance = EXCLUDED.balance,
		total_recharged = EXCLUDED.total_recharged,
		total_used = EXCLUDED.total_used,
		funded_total = EXCLUDED.funded_total,
		ever_used = user_activity_profiles.ever_used OR EXCLUDED.ever_used,
		site_created_at = COALESCE(EXCLUDED.site_created_at, user_activity_profiles.site_created_at),
		last_used_at = GREATEST(user_activity_profiles.last_used_at, EXCLUDED.last_used_at),
		updated_at = EXCLUDED.updated_at`

// ReplaceSnapshot 在一个事务里写入用户快照，并替换已拉取日期的每日用量。
func (r *Repository) ReplaceSnapshot(ctx context.Context, ws Workspace, snapshot Snapshot) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if !snapshot.FundedDeltaDay.IsZero() && len(snapshot.Profiles) > 0 {
		if err := recordFundedDeltas(ctx, tx, ws, snapshot.FundedDeltaDay, snapshot.Profiles); err != nil {
			return err
		}
	}
	if len(snapshot.Profiles) > 0 {
		batch := &pgx.Batch{}
		for _, profile := range snapshot.Profiles {
			batch.Queue(upsertProfileSQL, ws.UserID, ws.AdminAccountID, profile.SiteUserID, profile.Email, profile.Username,
				profile.Role, profile.Status, profile.Group, profile.Balance, profile.TotalRecharged, profile.TotalUsed,
				profile.FundedTotal, profile.EverUsed, profile.SiteCreatedAt, profile.LastUsedAt, snapshot.SyncStamp)
		}
		results := tx.SendBatch(ctx, batch)
		for i := 0; i < batch.Len(); i++ {
			if _, err := results.Exec(); err != nil {
				_ = results.Close()
				return fmt.Errorf("upsert profile: %w", err)
			}
		}
		if err := results.Close(); err != nil {
			return err
		}
		if snapshot.PruneMissingProfiles {
			if _, err := tx.Exec(ctx, `DELETE FROM user_activity_profiles WHERE user_id = $1 AND admin_account_id = $2 AND updated_at < $3`,
				ws.UserID, ws.AdminAccountID, snapshot.SyncStamp); err != nil {
				return err
			}
		}
	}
	if len(snapshot.LastUsed) > 0 {
		ids := make([]string, 0, len(snapshot.LastUsed))
		for id := range snapshot.LastUsed {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		times := make([]time.Time, 0, len(ids))
		for _, id := range ids {
			times = append(times, snapshot.LastUsed[id])
		}
		if _, err := tx.Exec(ctx, `
			UPDATE user_activity_profiles AS p
			SET last_used_at = GREATEST(p.last_used_at, v.last_used_at), ever_used = true
			FROM unnest($3::text[], $4::timestamptz[]) AS v(site_user_id, last_used_at)
			WHERE p.user_id = $1 AND p.admin_account_id = $2 AND p.site_user_id = v.site_user_id
		`, ws.UserID, ws.AdminAccountID, ids, times); err != nil {
			return fmt.Errorf("update last used: %w", err)
		}
	}
	if len(snapshot.Days) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM user_activity_daily WHERE user_id = $1 AND admin_account_id = $2 AND day = ANY($3::date[])`,
			ws.UserID, ws.AdminAccountID, snapshot.Days); err != nil {
			return err
		}
	}
	if len(snapshot.Daily) > 0 {
		rows := make([][]any, 0, len(snapshot.Daily))
		for _, usage := range snapshot.Daily {
			rows = append(rows, []any{ws.UserID, ws.AdminAccountID, usage.SiteUserID, usage.Day, usage.Requests, usage.Tokens, usage.Cost})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"user_activity_daily"},
			[]string{"user_id", "admin_account_id", "site_user_id", "day", "requests", "tokens", "cost"},
			pgx.CopyFromRows(rows)); err != nil {
			return fmt.Errorf("copy daily usage: %w", err)
		}
	}
	if len(snapshot.RechargeDays) > 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM user_activity_recharge_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND day = ANY($3::date[]) AND channel = ANY($4::text[])
		`, ws.UserID, ws.AdminAccountID, snapshot.RechargeDays, recordChannels); err != nil {
			return err
		}
	}
	if len(snapshot.Recharges) > 0 {
		rows := make([][]any, 0, len(snapshot.Recharges))
		for _, recharge := range snapshot.Recharges {
			rows = append(rows, []any{ws.UserID, ws.AdminAccountID, recharge.SiteUserID, recharge.Day, recharge.Channel, recharge.Amount, recharge.Events})
		}
		if _, err := tx.CopyFrom(ctx, pgx.Identifier{"user_activity_recharge_daily"},
			[]string{"user_id", "admin_account_id", "site_user_id", "day", "channel", "amount", "events"},
			pgx.CopyFromRows(rows)); err != nil {
			return fmt.Errorf("copy recharges: %w", err)
		}
	}
	if snapshot.SiteDaily != nil {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_activity_site_daily (user_id, admin_account_id, day, total_balance, users_with_balance, user_count, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
			ON CONFLICT (user_id, admin_account_id, day) DO UPDATE SET
				total_balance = EXCLUDED.total_balance, users_with_balance = EXCLUDED.users_with_balance,
				user_count = EXCLUDED.user_count, updated_at = EXCLUDED.updated_at
		`, ws.UserID, ws.AdminAccountID, snapshot.SiteDaily.Day, snapshot.SiteDaily.TotalBalance, snapshot.SiteDaily.UsersWithBalance,
			snapshot.SiteDaily.UserCount, snapshot.SyncStamp); err != nil {
			return err
		}
	}
	if !snapshot.RetainFrom.IsZero() {
		for _, table := range []string{"user_activity_daily", "user_activity_recharge_daily", "user_activity_site_daily"} {
			if _, err := tx.Exec(ctx, `DELETE FROM `+table+` WHERE user_id = $1 AND admin_account_id = $2 AND day < $3`,
				ws.UserID, ws.AdminAccountID, snapshot.RetainFrom); err != nil {
				return err
			}
		}
	}
	return tx.Commit(ctx)
}

// recordFundedDeltas 计算 new-api 用户「剩余额度 + 累计已用」相对本地上次保存值的变化，累加到 day 的 credit / debit。
// 必须在更新 profiles.funded_total 之前、同一个事务里执行，避免重复计入或漏计。
func recordFundedDeltas(ctx context.Context, tx pgx.Tx, ws Workspace, day time.Time, profiles []Profile) error {
	ids := make([]string, 0, len(profiles))
	funded := make([]float64, 0, len(profiles))
	for _, profile := range profiles {
		if profile.FundedTotal == nil {
			continue
		}
		ids = append(ids, profile.SiteUserID)
		funded = append(funded, *profile.FundedTotal)
	}
	if len(ids) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO user_activity_recharge_daily (user_id, admin_account_id, site_user_id, day, channel, amount, events)
		SELECT $1, $2, v.site_user_id, $3::date,
			CASE WHEN v.funded > p.funded_total THEN $6::text ELSE $7::text END,
			v.funded - p.funded_total, 1
		FROM unnest($4::text[], $5::float8[]) AS v(site_user_id, funded)
		JOIN user_activity_profiles p ON p.user_id = $1 AND p.admin_account_id = $2 AND p.site_user_id = v.site_user_id
		WHERE p.funded_total IS NOT NULL AND abs(v.funded - p.funded_total) > $8
		ON CONFLICT (user_id, admin_account_id, site_user_id, day, channel) DO UPDATE SET
			amount = user_activity_recharge_daily.amount + EXCLUDED.amount,
			events = user_activity_recharge_daily.events + EXCLUDED.events
	`, ws.UserID, ws.AdminAccountID, day, ids, funded, ChannelCredit, ChannelDebit, fundedEpsilon)
	if err != nil {
		return fmt.Errorf("record funded deltas: %w", err)
	}
	return nil
}

// GetSyncState 读取工作区的同步状态。
func (r *Repository) GetSyncState(ctx context.Context, ws Workspace) (SyncState, bool, error) {
	var state SyncState
	err := r.db.QueryRow(ctx, `
		SELECT platform, status, error_key, synced_from, synced_to, user_count, last_started_at, last_synced_at,
			recharge_synced_from, recharge_synced_to, recharge_since, warning_key, started_by
		FROM user_activity_sync_states WHERE user_id = $1 AND admin_account_id = $2
	`, ws.UserID, ws.AdminAccountID).Scan(&state.Platform, &state.Status, &state.ErrorKey, &state.SyncedFrom, &state.SyncedTo,
		&state.UserCount, &state.LastStartedAt, &state.LastSyncedAt, &state.RechargeSyncedFrom, &state.RechargeSyncedTo,
		&state.RechargeSince, &state.WarningKey, &state.StartedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncState{}, false, nil
	}
	if err != nil {
		return SyncState{}, false, err
	}
	return state, true, nil
}

// SaveSyncState 保存工作区的同步状态。
func (r *Repository) SaveSyncState(ctx context.Context, ws Workspace, state SyncState) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO user_activity_sync_states (
			user_id, admin_account_id, platform, status, error_key, synced_from, synced_to, user_count, last_started_at, last_synced_at,
			recharge_synced_from, recharge_synced_to, recharge_since, warning_key, started_by
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET
			platform = EXCLUDED.platform, status = EXCLUDED.status, error_key = EXCLUDED.error_key,
			synced_from = EXCLUDED.synced_from, synced_to = EXCLUDED.synced_to, user_count = EXCLUDED.user_count,
			last_started_at = EXCLUDED.last_started_at, last_synced_at = EXCLUDED.last_synced_at,
			recharge_synced_from = EXCLUDED.recharge_synced_from, recharge_synced_to = EXCLUDED.recharge_synced_to,
			recharge_since = EXCLUDED.recharge_since, warning_key = EXCLUDED.warning_key, started_by = EXCLUDED.started_by
	`, ws.UserID, ws.AdminAccountID, state.Platform, state.Status, state.ErrorKey, state.SyncedFrom, state.SyncedTo,
		state.UserCount, state.LastStartedAt, state.LastSyncedAt, state.RechargeSyncedFrom, state.RechargeSyncedTo,
		state.RechargeSince, state.WarningKey, state.StartedBy)
	return err
}

// ResetRunningSyncs 把进程重启前未完成的同步标记为被打断（单实例部署：启动时不会有其它进程在同步）；
// 定时任务启动后会在后台继续。
func (r *Repository) ResetRunningSyncs(ctx context.Context, errorKey string) error {
	_, err := r.db.Exec(ctx, `UPDATE user_activity_sync_states SET status = $1, error_key = $2 WHERE status = $3`,
		SyncStatusFailed, errorKey, SyncStatusRunning)
	return err
}

// ListSyncedWorkspaces 返回同步过的工作区，供定时同步使用。
func (r *Repository) ListSyncedWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := r.db.Query(ctx, `SELECT user_id, admin_account_id FROM user_activity_sync_states ORDER BY user_id, admin_account_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workspaces := make([]Workspace, 0)
	for rows.Next() {
		var ws Workspace
		if err := rows.Scan(&ws.UserID, &ws.AdminAccountID); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, rows.Err()
}

// ProfileCounts 是用户数量统计。
type ProfileCounts struct {
	Total     int
	Disabled  int
	WithEmail int
	NoUsage   int
	Inactive  []InactiveBucket
}

// ProfileCounts 统计用户总数、已禁用人数、有邮箱的人数，以及（不含已禁用用户）从未使用、超过 N 天未使用的人数。
// 「超过 N 天未使用」包含最近使用早于 N 天前的用户，以及有过消费但已没有最近使用记录的老用户。
func (r *Repository) ProfileCounts(ctx context.Context, ws Workspace, now time.Time, days []int) (ProfileCounts, error) {
	args := []any{ws.UserID, ws.AdminAccountID, StatusDisabled}
	var columns strings.Builder
	columns.WriteString(`count(*), count(*) FILTER (WHERE status = $3), count(*) FILTER (WHERE email <> ''),
		count(*) FILTER (WHERE status <> $3 AND last_used_at IS NULL AND NOT ever_used)`)
	for _, d := range days {
		args = append(args, now.AddDate(0, 0, -d))
		placeholder := "$" + strconv.Itoa(len(args))
		columns.WriteString(", count(*) FILTER (WHERE status <> $3 AND (last_used_at < " + placeholder + " OR (last_used_at IS NULL AND ever_used)))")
	}
	query := "SELECT " + columns.String() + " FROM user_activity_profiles WHERE user_id = $1 AND admin_account_id = $2"
	values := make([]int, 4+len(days))
	targets := make([]any, len(values))
	for i := range values {
		targets[i] = &values[i]
	}
	if err := r.db.QueryRow(ctx, query, args...).Scan(targets...); err != nil {
		return ProfileCounts{}, err
	}
	counts := ProfileCounts{Total: values[0], Disabled: values[1], WithEmail: values[2], NoUsage: values[3], Inactive: make([]InactiveBucket, 0, len(days))}
	for i, d := range days {
		counts.Inactive = append(counts.Inactive, InactiveBucket{Days: d, Count: values[4+i]})
	}
	return counts, nil
}

// PeriodMetrics 汇总 [from, to]（含两端）内的活跃人数与用量；siteUserID 非空时只统计该用户。
// 新增用户按站点注册时间统计（只在统计全部用户时计算），日期边界按 loc 时区。
func (r *Repository) PeriodMetrics(ctx context.Context, ws Workspace, siteUserID string, from time.Time, to time.Time, loc *time.Location) (PeriodMetrics, error) {
	metrics := PeriodMetrics{From: formatDay(from), To: formatDay(to)}
	if err := r.db.QueryRow(ctx, `
		SELECT count(DISTINCT site_user_id) FILTER (WHERE requests > 0), COALESCE(sum(requests), 0), COALESCE(sum(tokens), 0), COALESCE(sum(cost), 0)
		FROM user_activity_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 AND ($5 = '' OR site_user_id = $5)
	`, ws.UserID, ws.AdminAccountID, from, to, siteUserID).Scan(&metrics.ActiveUsers, &metrics.Requests, &metrics.Tokens, &metrics.Cost); err != nil {
		return PeriodMetrics{}, err
	}
	if siteUserID == "" {
		start := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, loc)
		end := time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, 1)
		if err := r.db.QueryRow(ctx, `
			SELECT count(*) FROM user_activity_profiles
			WHERE user_id = $1 AND admin_account_id = $2 AND site_created_at >= $3 AND site_created_at < $4
		`, ws.UserID, ws.AdminAccountID, start, end).Scan(&metrics.NewUsers); err != nil {
			return PeriodMetrics{}, err
		}
	}
	return metrics, nil
}

// PeriodRetention 对比两个区间的活跃用户：两期都活跃（留存）、上期活跃本期未活跃（流失）、本期新活跃。
func (r *Repository) PeriodRetention(ctx context.Context, ws Workspace, current [2]time.Time, previous [2]time.Time) (Retention, error) {
	var retention Retention
	err := r.db.QueryRow(ctx, `
		WITH cur AS (
			SELECT DISTINCT site_user_id FROM user_activity_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 AND requests > 0
		), prev AS (
			SELECT DISTINCT site_user_id FROM user_activity_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $5 AND $6 AND requests > 0
		)
		SELECT
			(SELECT count(*) FROM cur WHERE EXISTS (SELECT 1 FROM prev WHERE prev.site_user_id = cur.site_user_id)),
			(SELECT count(*) FROM prev WHERE NOT EXISTS (SELECT 1 FROM cur WHERE cur.site_user_id = prev.site_user_id)),
			(SELECT count(*) FROM cur WHERE NOT EXISTS (SELECT 1 FROM prev WHERE prev.site_user_id = cur.site_user_id))
	`, ws.UserID, ws.AdminAccountID, current[0], current[1], previous[0], previous[1]).Scan(&retention.Retained, &retention.Churned, &retention.Gained)
	return retention, err
}

// DailySeries 返回 [from, to] 内每天的活跃人数与用量（没有数据的日期补 0）；siteUserID 非空时只统计该用户。
func (r *Repository) DailySeries(ctx context.Context, ws Workspace, siteUserID string, from time.Time, to time.Time) ([]DailyPoint, error) {
	rows, err := r.db.Query(ctx, `
		SELECT day, count(*) FILTER (WHERE requests > 0), COALESCE(sum(requests), 0), COALESCE(sum(tokens), 0), COALESCE(sum(cost), 0)
		FROM user_activity_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 AND ($5 = '' OR site_user_id = $5)
		GROUP BY day
	`, ws.UserID, ws.AdminAccountID, from, to, siteUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byDay := map[string]DailyPoint{}
	for rows.Next() {
		var day time.Time
		var point DailyPoint
		if err := rows.Scan(&day, &point.ActiveUsers, &point.Requests, &point.Tokens, &point.Cost); err != nil {
			return nil, err
		}
		point.Day = formatDay(day)
		byDay[point.Day] = point
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rechargeRows, err := r.db.Query(ctx, `
		SELECT day, COALESCE(sum(amount) FILTER (WHERE amount > 0), 0)
		FROM user_activity_recharge_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 AND ($5 = '' OR site_user_id = $5)
		GROUP BY day
	`, ws.UserID, ws.AdminAccountID, from, to, siteUserID)
	if err != nil {
		return nil, err
	}
	defer rechargeRows.Close()
	recharges := map[string]float64{}
	for rechargeRows.Next() {
		var day time.Time
		var amount float64
		if err := rechargeRows.Scan(&day, &amount); err != nil {
			return nil, err
		}
		recharges[formatDay(day)] = amount
	}
	if err := rechargeRows.Err(); err != nil {
		return nil, err
	}
	series := fillDailySeries(byDay, from, to)
	for i := range series {
		amount := recharges[series[i].Day]
		series[i].Recharge = &amount
	}
	return series, nil
}

// RechargeDaySummary 是某一天的入账汇总。
type RechargeDaySummary struct {
	Channels []ChannelAmount
	// Users 是当天有正向入账的人数。
	Users    int
	Positive float64
	Negative float64
}

// RechargeDay 汇总某天各渠道的入账。
func (r *Repository) RechargeDay(ctx context.Context, ws Workspace, day time.Time) (RechargeDaySummary, error) {
	summary := RechargeDaySummary{Channels: []ChannelAmount{}}
	rows, err := r.db.Query(ctx, `
		SELECT channel, COALESCE(sum(amount), 0), COALESCE(sum(events), 0), count(DISTINCT site_user_id)
		FROM user_activity_recharge_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day = $3
		GROUP BY channel
		ORDER BY channel
	`, ws.UserID, ws.AdminAccountID, day)
	if err != nil {
		return RechargeDaySummary{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item ChannelAmount
		var events int64
		if err := rows.Scan(&item.Channel, &item.Amount, &events, &item.Users); err != nil {
			return RechargeDaySummary{}, err
		}
		item.Events = int(events)
		summary.Channels = append(summary.Channels, item)
	}
	if err := rows.Err(); err != nil {
		return RechargeDaySummary{}, err
	}
	err = r.db.QueryRow(ctx, `
		SELECT COALESCE(sum(amount) FILTER (WHERE amount > 0), 0), COALESCE(sum(amount) FILTER (WHERE amount < 0), 0),
			count(DISTINCT site_user_id) FILTER (WHERE amount > 0)
		FROM user_activity_recharge_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day = $3
	`, ws.UserID, ws.AdminAccountID, day).Scan(&summary.Positive, &summary.Negative, &summary.Users)
	return summary, err
}

// BalanceTotals 是全站用户余额的汇总（当前快照）。
type BalanceTotals struct {
	TotalBalance      float64
	UsersWithBalance  int
	LifetimeRecharged *float64
	LowBalanceActive  int
}

// BalanceTotals 汇总全站用户余额、有余额的人数、累计充值（sub2api total_recharged）以及近期活跃但余额不足的人数。
func (r *Repository) BalanceTotals(ctx context.Context, ws Workspace, now time.Time) (BalanceTotals, error) {
	var totals BalanceTotals
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(sum(balance), 0), count(*) FILTER (WHERE balance > 0), sum(total_recharged),
			count(*) FILTER (WHERE balance < $3 AND last_used_at >= $4 AND status <> $5)
		FROM user_activity_profiles
		WHERE user_id = $1 AND admin_account_id = $2
	`, ws.UserID, ws.AdminAccountID, lowBalanceThreshold, now.AddDate(0, 0, -lowBalanceActiveDays), StatusDisabled).Scan(
		&totals.TotalBalance, &totals.UsersWithBalance, &totals.LifetimeRecharged, &totals.LowBalanceActive)
	return totals, err
}

// SiteBalance 返回某天全站快照中的用户余额合计，没有快照时返回 nil。
func (r *Repository) SiteBalance(ctx context.Context, ws Workspace, day time.Time) (*float64, error) {
	var total float64
	err := r.db.QueryRow(ctx, `SELECT total_balance FROM user_activity_site_daily WHERE user_id = $1 AND admin_account_id = $2 AND day = $3`,
		ws.UserID, ws.AdminAccountID, day).Scan(&total)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &total, nil
}

func fillDailySeries(byDay map[string]DailyPoint, from time.Time, to time.Time) []DailyPoint {
	series := make([]DailyPoint, 0)
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		key := formatDay(day)
		if point, ok := byDay[key]; ok {
			series = append(series, point)
			continue
		}
		series = append(series, DailyPoint{Day: key})
	}
	return series
}

// SuppressionCount 返回工作区退订名单的人数。
func (r *Repository) SuppressionCount(ctx context.Context, ws Workspace) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM email_suppressions WHERE user_id = $1 AND admin_account_id = $2`, ws.UserID, ws.AdminAccountID).Scan(&count)
	return count, err
}

// userSortColumns 是允许的排序字段（白名单，SQL 片段不来自用户输入）。
var userSortColumns = map[string]string{
	SortLastUsedAt:     "last_used_at",
	SortCostToday:      "cost_today",
	SortCost7d:         "cost_7",
	SortCost30d:        "cost_30",
	SortRequests7d:     "requests_7",
	SortChange7d:       "(CASE WHEN cost_prev7 > 0 THEN (cost_7 - cost_prev7) / cost_prev7 END)",
	SortActiveDays30d:  "active_days_30",
	SortBalance:        "balance",
	SortTotalRecharged: "total_recharged",
	SortTotalUsed:      "total_used",
	SortCreatedAt:      "site_created_at",
	SortRechargeToday:  "recharge_today",
	SortRecharge30d:    "recharge_30",
}

const listUsersSQL = `
	WITH agg AS (
		SELECT site_user_id,
			COALESCE(sum(requests) FILTER (WHERE day = $3), 0) AS requests_today,
			COALESCE(sum(cost) FILTER (WHERE day = $3), 0) AS cost_today,
			COALESCE(sum(requests) FILTER (WHERE day BETWEEN $4 AND $5), 0) AS requests_7,
			COALESCE(sum(requests) FILTER (WHERE day BETWEEN $6 AND $7), 0) AS requests_prev7,
			COALESCE(sum(cost) FILTER (WHERE day BETWEEN $4 AND $5), 0) AS cost_7,
			COALESCE(sum(cost) FILTER (WHERE day BETWEEN $6 AND $7), 0) AS cost_prev7,
			COALESCE(sum(cost) FILTER (WHERE day BETWEEN $8 AND $5), 0) AS cost_30,
			COALESCE(sum(cost) FILTER (WHERE day BETWEEN $9 AND $10), 0) AS cost_prev30,
			count(*) FILTER (WHERE requests > 0 AND day BETWEEN $8 AND $5) AS active_days_30
		FROM user_activity_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $9 AND $3
		GROUP BY site_user_id
	), rch AS (
		SELECT site_user_id,
			COALESCE(sum(amount) FILTER (WHERE day = $3 AND amount > 0), 0) AS recharge_today,
			COALESCE(sum(amount) FILTER (WHERE day BETWEEN $8 AND $5 AND amount > 0), 0) AS recharge_30
		FROM user_activity_recharge_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $8 AND $3
		GROUP BY site_user_id
	), user_rows AS (
		SELECT p.site_user_id, p.email, p.username, p.role, p.status, p.user_group, p.balance, p.total_recharged,
			p.total_used, p.ever_used, p.site_created_at, p.last_used_at,
			COALESCE(a.requests_today, 0) AS requests_today, COALESCE(a.cost_today, 0) AS cost_today,
			COALESCE(a.requests_7, 0) AS requests_7, COALESCE(a.requests_prev7, 0) AS requests_prev7,
			COALESCE(a.cost_7, 0) AS cost_7, COALESCE(a.cost_prev7, 0) AS cost_prev7,
			COALESCE(a.cost_30, 0) AS cost_30, COALESCE(a.cost_prev30, 0) AS cost_prev30,
			COALESCE(a.active_days_30, 0) AS active_days_30,
			COALESCE(r.recharge_today, 0) AS recharge_today, COALESCE(r.recharge_30, 0) AS recharge_30
		FROM user_activity_profiles p
		LEFT JOIN agg a ON a.site_user_id = p.site_user_id
		LEFT JOIN rch r ON r.site_user_id = p.site_user_id
		WHERE p.user_id = $1 AND p.admin_account_id = $2
	)`

func listUsersArgs(ws Workspace, windows Windows) []any {
	return []any{ws.UserID, ws.AdminAccountID, windows.Today, windows.From7, windows.Yesterday, windows.PrevFrom7, windows.PrevTo7,
		windows.From30, windows.PrevFrom30, windows.PrevTo30}
}

// ListUsers 按筛选条件分页查询用户及其近期用量；limit <= 0 时不分页。
func (r *Repository) ListUsers(ctx context.Context, ws Workspace, windows Windows, filter UserFilter, sortKey string, order string, limit int, offset int) ([]UserRow, int, error) {
	args := listUsersArgs(ws, windows)
	next := func(value any) string {
		args = append(args, value)
		return "$" + strconv.Itoa(len(args))
	}
	where, err := userWhere(filter, windows.Now, next)
	if err != nil {
		return nil, 0, err
	}
	sortColumn, ok := userSortColumns[sortKey]
	if !ok {
		sortColumn = userSortColumns[SortLastUsedAt]
	}
	direction := "DESC"
	if strings.EqualFold(order, "asc") {
		direction = "ASC"
	}
	pagination := ""
	if limit > 0 {
		pagination = " LIMIT " + next(limit) + " OFFSET " + next(offset)
	}
	query := listUsersSQL + `
		SELECT site_user_id, email, username, role, status, user_group, balance, total_recharged, total_used, ever_used,
			site_created_at, last_used_at, requests_today, cost_today, requests_7, requests_prev7, cost_7, cost_prev7,
			cost_30, cost_prev30, active_days_30, recharge_today, recharge_30, count(*) OVER () AS total
		FROM user_rows ` + where + `
		ORDER BY ` + sortColumn + ` ` + direction + ` NULLS LAST, site_user_id ASC` + pagination
	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	result := make([]UserRow, 0)
	total := 0
	for rows.Next() {
		var row UserRow
		var activeDays int64
		if err := rows.Scan(&row.SiteUserID, &row.Email, &row.Username, &row.Role, &row.Status, &row.Group, &row.Balance,
			&row.TotalRecharged, &row.TotalUsed, &row.EverUsed, &row.CreatedAt, &row.LastUsedAt, &row.RequestsToday,
			&row.CostToday, &row.Requests7d, &row.RequestsPrev7d, &row.Cost7d, &row.CostPrev7d, &row.Cost30d,
			&row.CostPrev30d, &activeDays, &row.RechargeToday, &row.Recharge30d, &total); err != nil {
			return nil, 0, err
		}
		row.ActiveDays30d = int(activeDays)
		row.DaysSinceLastUse = daysSince(row.LastUsedAt, windows.Now)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(result) == 0 && limit > 0 && offset > 0 {
		// 页码超出范围时 count(*) OVER () 没有行可返回，单独统计总数。
		countArgs := args[:len(args)-2]
		if err := r.db.QueryRow(ctx, listUsersSQL+` SELECT count(*) FROM user_rows `+where, countArgs...).Scan(&total); err != nil {
			return nil, 0, err
		}
	}
	return result, total, nil
}

// RecipientPreview 统计筛选结果中有邮箱、在退订名单中的人数（发送前预览）。
func (r *Repository) RecipientPreview(ctx context.Context, ws Workspace, windows Windows, filter UserFilter) (RecipientPreview, error) {
	args := listUsersArgs(ws, windows)
	next := func(value any) string {
		args = append(args, value)
		return "$" + strconv.Itoa(len(args))
	}
	where, err := userWhere(filter, windows.Now, next)
	if err != nil {
		return RecipientPreview{}, err
	}
	var preview RecipientPreview
	err = r.db.QueryRow(ctx, listUsersSQL+`
		SELECT count(*),
			count(*) FILTER (WHERE email <> ''),
			count(*) FILTER (WHERE email <> '' AND EXISTS (
				SELECT 1 FROM email_suppressions s
				WHERE s.user_id = $1 AND s.admin_account_id = $2 AND s.email_normalized = lower(btrim(user_rows.email))
			))
		FROM user_rows `+where, args...).Scan(&preview.Matched, &preview.WithEmail, &preview.Suppressed)
	if err != nil {
		return RecipientPreview{}, err
	}
	preview.Sendable = preview.WithEmail - preview.Suppressed
	if preview.Sendable < 0 {
		preview.Sendable = 0
	}
	return preview, nil
}

// userWhere 把筛选条件转换为 WHERE 子句（参数通过 next 绑定，不拼接用户输入）。
func userWhere(filter UserFilter, now time.Time, next func(any) string) (string, error) {
	conditions := make([]string, 0, 4)
	inactive := func(days int) string {
		return "(last_used_at < " + next(now.AddDate(0, 0, -days)) + " OR (last_used_at IS NULL AND ever_used))"
	}
	switch filter.Segment {
	case "", SegmentAll:
	case SegmentActive1d:
		conditions = append(conditions, "last_used_at >= "+next(now.AddDate(0, 0, -1)))
	case SegmentActive7d:
		conditions = append(conditions, "last_used_at >= "+next(now.AddDate(0, 0, -7)))
	case SegmentActive30d:
		conditions = append(conditions, "last_used_at >= "+next(now.AddDate(0, 0, -30)))
	case SegmentInactive2d:
		conditions = append(conditions, inactive(2))
	case SegmentInactive7d:
		conditions = append(conditions, inactive(7))
	case SegmentInactive14d:
		conditions = append(conditions, inactive(14))
	case SegmentInactive30d:
		conditions = append(conditions, inactive(30))
	case SegmentInactive60d:
		conditions = append(conditions, inactive(60))
	case SegmentInactive90d:
		conditions = append(conditions, inactive(90))
	case SegmentInactive:
		if filter.InactiveDays < 1 || filter.InactiveDays > maxInactiveDays {
			return "", requestError(ErrorInvalidFilter)
		}
		conditions = append(conditions, inactive(filter.InactiveDays))
	case SegmentNoUsage:
		conditions = append(conditions, "last_used_at IS NULL AND NOT ever_used")
	case SegmentChurned7d:
		conditions = append(conditions, "requests_prev7 > 0 AND requests_7 = 0")
	case SegmentDeclining:
		conditions = append(conditions, "cost_prev7 > 0 AND cost_7 <= cost_prev7 * 0.5")
	case SegmentGrowing:
		conditions = append(conditions, "cost_prev7 > 0 AND cost_7 >= cost_prev7 * 1.5")
	case SegmentRechargedToday:
		conditions = append(conditions, "recharge_today > 0")
	case SegmentLowBalance:
		conditions = append(conditions, "balance < "+next(lowBalanceThreshold)+" AND last_used_at >= "+next(now.AddDate(0, 0, -lowBalanceActiveDays)))
	default:
		return "", requestError(ErrorInvalidFilter)
	}
	switch filter.Status {
	case "":
	case StatusActive:
		// 「正常」= 未禁用，与概览中未活跃人数的统计口径一致。
		conditions = append(conditions, "status <> "+next(StatusDisabled))
	case StatusDisabled:
		conditions = append(conditions, "status = "+next(StatusDisabled))
	default:
		return "", requestError(ErrorInvalidFilter)
	}
	switch filter.Role {
	case "":
	case RoleUser:
		conditions = append(conditions, "role = "+next(RoleUser))
	case RoleAdmin:
		conditions = append(conditions, "role IN ("+next(RoleAdmin)+", "+next(RoleRoot)+")")
	default:
		return "", requestError(ErrorInvalidFilter)
	}
	if search := strings.TrimSpace(filter.Search); search != "" {
		if len([]rune(search)) > maxSearchRunes {
			return "", requestError(ErrorInvalidFilter)
		}
		pattern := next("%" + escapeLike(search) + "%")
		conditions = append(conditions, "(email ILIKE "+pattern+" ESCAPE '\\' OR username ILIKE "+pattern+" ESCAPE '\\' OR site_user_id = "+next(search)+")")
	}
	if filter.HasEmail {
		conditions = append(conditions, "email <> ''")
	}
	if filter.SiteUserID != "" {
		conditions = append(conditions, "site_user_id = "+next(filter.SiteUserID))
	}
	if len(conditions) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conditions, " AND "), nil
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

func daysSince(value *time.Time, now time.Time) *int {
	if value == nil {
		return nil
	}
	days := int(now.Sub(*value).Hours() / 24)
	if days < 0 {
		days = 0
	}
	return &days
}

func formatDay(day time.Time) string {
	return day.Format("2006-01-02")
}
