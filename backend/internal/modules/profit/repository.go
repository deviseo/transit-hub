package profit

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 读写利润分析的本地数据（profit_* 表）。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetSyncState(ctx context.Context, ws Workspace) (SyncState, bool, error) {
	var state SyncState
	err := r.db.QueryRow(ctx, `
		SELECT platform, status, error_key, warning_key, revenue_source, route_count, synced_from, synced_to, complete_through,
			last_started_at, last_synced_at, started_by
		FROM profit_sync_states WHERE user_id = $1 AND admin_account_id = $2
	`, ws.UserID, ws.AdminAccountID).Scan(&state.Platform, &state.Status, &state.ErrorKey, &state.WarningKey, &state.RevenueSource,
		&state.RouteCount, &state.SyncedFrom, &state.SyncedTo, &state.CompleteThrough, &state.LastStartedAt, &state.LastSyncedAt, &state.StartedBy)
	if errors.Is(err, pgx.ErrNoRows) {
		return SyncState{}, false, nil
	}
	if err != nil {
		return SyncState{}, false, err
	}
	return state, true, nil
}

func (r *Repository) SaveSyncState(ctx context.Context, ws Workspace, state SyncState) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO profit_sync_states (
			user_id, admin_account_id, platform, status, error_key, warning_key, revenue_source, route_count,
			synced_from, synced_to, complete_through, last_started_at, last_synced_at, started_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET
			platform = EXCLUDED.platform, status = EXCLUDED.status, error_key = EXCLUDED.error_key, warning_key = EXCLUDED.warning_key,
			revenue_source = EXCLUDED.revenue_source, route_count = EXCLUDED.route_count, synced_from = EXCLUDED.synced_from,
			synced_to = EXCLUDED.synced_to, complete_through = EXCLUDED.complete_through,
			last_started_at = EXCLUDED.last_started_at, last_synced_at = EXCLUDED.last_synced_at, started_by = EXCLUDED.started_by
	`, ws.UserID, ws.AdminAccountID, state.Platform, state.Status, state.ErrorKey, state.WarningKey, state.RevenueSource,
		state.RouteCount, state.SyncedFrom, state.SyncedTo, state.CompleteThrough, state.LastStartedAt, state.LastSyncedAt, state.StartedBy)
	return err
}

// ResetRunningSyncs 把进程重启前未完成的采集标记为中断；定时任务启动后会在后台从中断处继续。
func (r *Repository) ResetRunningSyncs(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `UPDATE profit_sync_states SET status = $1, error_key = $2 WHERE status = $3`, StatusInterrupted, ErrorInterrupted, StatusRunning)
	return err
}

// ListSyncedWorkspaces 返回采集过（定时任务需要继续采集）的工作区，只包含仍然存在的工作区。
func (r *Repository) ListSyncedWorkspaces(ctx context.Context) ([]Workspace, error) {
	rows, err := r.db.Query(ctx, `
		SELECT s.user_id, s.admin_account_id FROM profit_sync_states s
		JOIN admin_accounts a ON a.user_id = s.user_id AND a.id = s.admin_account_id
		ORDER BY s.user_id, s.admin_account_id
	`)
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

// UpsertRoutes 保存当前路由清单；这次没有出现的路由标记为已删除（保留历史营收的名称）。
func (r *Repository) UpsertRoutes(ctx context.Context, ws Workspace, routes []Route, now time.Time) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	ids := make([]string, 0, len(routes))
	for _, route := range routes {
		groups := route.Groups
		if groups == nil {
			groups = []string{}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO profit_routes (user_id, admin_account_id, route_id, name, kind, status, base_url, groups, deleted, first_seen_at, last_seen_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,false,$9,$9)
			ON CONFLICT (user_id, admin_account_id, route_id) DO UPDATE SET
				name = EXCLUDED.name, kind = EXCLUDED.kind, status = EXCLUDED.status, base_url = EXCLUDED.base_url,
				groups = EXCLUDED.groups, deleted = false, last_seen_at = EXCLUDED.last_seen_at
		`, ws.UserID, ws.AdminAccountID, route.ID, route.Name, route.Kind, route.Status, route.BaseURL, groups, now); err != nil {
			return err
		}
		ids = append(ids, route.ID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE profit_routes SET deleted = true
		WHERE user_id = $1 AND admin_account_id = $2 AND NOT (route_id = ANY($3)) AND NOT deleted
	`, ws.UserID, ws.AdminAccountID, ids); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListRoutes(ctx context.Context, ws Workspace) ([]Route, error) {
	rows, err := r.db.Query(ctx, `
		SELECT route_id, name, kind, status, base_url, groups, deleted
		FROM profit_routes WHERE user_id = $1 AND admin_account_id = $2
		ORDER BY deleted, name, route_id
	`, ws.UserID, ws.AdminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	routes := make([]Route, 0)
	for rows.Next() {
		var route Route
		if err := rows.Scan(&route.ID, &route.Name, &route.Kind, &route.Status, &route.BaseURL, &route.Groups, &route.Deleted); err != nil {
			return nil, err
		}
		routes = append(routes, route)
	}
	return routes, rows.Err()
}

// ReplaceDay 用新采集的结果整体替换某一天的营收与上游成本（同一事务）。
func (r *Repository) ReplaceDay(ctx context.Context, ws Workspace, day time.Time, revenue []RouteRevenue, costs []SiteCost) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM profit_route_daily WHERE user_id = $1 AND admin_account_id = $2 AND day = $3`, ws.UserID, ws.AdminAccountID, day); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM profit_upstream_daily WHERE user_id = $1 AND admin_account_id = $2 AND day = $3`, ws.UserID, ws.AdminAccountID, day); err != nil {
		return err
	}
	merged := map[[2]string]RouteRevenue{}
	order := make([][2]string, 0, len(revenue))
	for _, row := range revenue {
		key := [2]string{row.RouteID, row.Group}
		existing, ok := merged[key]
		if !ok {
			order = append(order, key)
		}
		existing.RouteID, existing.Group = row.RouteID, row.Group
		existing.Revenue += row.Revenue
		existing.Requests += row.Requests
		merged[key] = existing
	}
	for _, key := range order {
		row := merged[key]
		if _, err := tx.Exec(ctx, `
			INSERT INTO profit_route_daily (user_id, admin_account_id, day, route_id, group_name, revenue, requests)
			VALUES ($1,$2,$3,$4,$5,$6,$7)
		`, ws.UserID, ws.AdminAccountID, day, row.RouteID, row.Group, row.Revenue, row.Requests); err != nil {
			return err
		}
	}
	for _, cost := range costs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO profit_upstream_daily (user_id, admin_account_id, day, site_id, site_name, base_url, consume, recharge_rate, cost, status, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
		`, ws.UserID, ws.AdminAccountID, day, cost.SiteID, cost.SiteName, cost.BaseURL, cost.Consume, cost.RechargeRate, cost.Cost, cost.Status); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// RouteRevenue 汇总区间内每条路由、每个分组的营收（站点额度）。
func (r *Repository) RouteRevenue(ctx context.Context, ws Workspace, from, to time.Time) ([]RouteRevenue, error) {
	rows, err := r.db.Query(ctx, `
		SELECT route_id, group_name, COALESCE(sum(revenue), 0), COALESCE(sum(requests), 0)
		FROM profit_route_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4
		GROUP BY route_id, group_name
		ORDER BY route_id, group_name
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RouteRevenue, 0)
	for rows.Next() {
		var row RouteRevenue
		if err := rows.Scan(&row.RouteID, &row.Group, &row.Revenue, &row.Requests); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// SiteCosts 汇总区间内每个上游站点的成本与各状态天数；名称和地址取最近一天的记录。
func (r *Repository) SiteCosts(ctx context.Context, ws Workspace, from, to time.Time) ([]SiteCost, error) {
	rows, err := r.db.Query(ctx, `
		SELECT site_id,
			(array_agg(site_name ORDER BY day DESC))[1],
			(array_agg(base_url ORDER BY day DESC))[1],
			(array_agg(recharge_rate ORDER BY day DESC))[1],
			COALESCE(sum(consume), 0), COALESCE(sum(cost), 0),
			count(*) FILTER (WHERE status = 'failed'),
			count(*) FILTER (WHERE status = 'approximated'),
			count(*) FILTER (WHERE status = 'no_rate'),
			count(*)
		FROM profit_upstream_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4
		GROUP BY site_id
		ORDER BY site_id
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]SiteCost, 0)
	for rows.Next() {
		var row SiteCost
		var failed, approximated, noRate, days int64
		if err := rows.Scan(&row.SiteID, &row.SiteName, &row.BaseURL, &row.RechargeRate, &row.Consume, &row.Cost, &failed, &approximated, &noRate, &days); err != nil {
			return nil, err
		}
		row.FailedDays, row.ApproximatedDays, row.NoRateDays, row.Days = int(failed), int(approximated), int(noRate), int(days)
		result = append(result, row)
	}
	return result, rows.Err()
}

// SiteDailyCosts 返回区间内每个站点每天的成本（只含能得到数据的天：ok / approximated），用于续航和异常检测。
func (r *Repository) SiteDailyCosts(ctx context.Context, ws Workspace, from, to time.Time) (map[string]map[string]float64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT site_id, day, cost FROM profit_upstream_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 AND status IN ('ok', 'approximated')
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]map[string]float64{}
	for rows.Next() {
		var siteID string
		var day time.Time
		var cost float64
		if err := rows.Scan(&siteID, &day, &cost); err != nil {
			return nil, err
		}
		if result[siteID] == nil {
			result[siteID] = map[string]float64{}
		}
		result[siteID][day.Format("2006-01-02")] = cost
	}
	return result, rows.Err()
}

// RouteDailyRevenue 返回区间内每条路由每天的营收（站点额度），用于按上游的日序列与异常检测。
func (r *Repository) RouteDailyRevenue(ctx context.Context, ws Workspace, from, to time.Time) (map[string]map[string]float64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT route_id, day, COALESCE(sum(revenue), 0) FROM profit_route_daily
		WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4
		GROUP BY route_id, day
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]map[string]float64{}
	for rows.Next() {
		var routeID string
		var day time.Time
		var revenue float64
		if err := rows.Scan(&routeID, &day, &revenue); err != nil {
			return nil, err
		}
		if result[routeID] == nil {
			result[routeID] = map[string]float64{}
		}
		result[routeID][day.Format("2006-01-02")] = revenue
	}
	return result, rows.Err()
}

// DailyTotals 返回区间内每天的营收（站点额度）与成本（人民币）；某天没有任何记录时对应标记为 false。
func (r *Repository) DailyTotals(ctx context.Context, ws Workspace, from, to time.Time) ([]DayTotals, error) {
	rows, err := r.db.Query(ctx, `
		WITH days AS (SELECT generate_series($3::date, $4::date, interval '1 day')::date AS day),
		revenue AS (
			SELECT day, sum(revenue) AS revenue FROM profit_route_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 GROUP BY day
		), cost AS (
			SELECT day, sum(cost) AS cost, count(*) FILTER (WHERE status = 'failed') AS failed, count(*) AS rows FROM profit_upstream_daily
			WHERE user_id = $1 AND admin_account_id = $2 AND day BETWEEN $3 AND $4 GROUP BY day
		)
		SELECT d.day, COALESCE(r.revenue, 0), COALESCE(c.cost, 0), r.day IS NOT NULL, c.day IS NOT NULL, COALESCE(c.failed, 0)
		FROM days d LEFT JOIN revenue r ON r.day = d.day LEFT JOIN cost c ON c.day = d.day
		ORDER BY d.day
	`, ws.UserID, ws.AdminAccountID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]DayTotals, 0)
	for rows.Next() {
		var row DayTotals
		var failed int64
		if err := rows.Scan(&row.Day, &row.Revenue, &row.Cost, &row.HasRevenue, &row.HasCost, &failed); err != nil {
			return nil, err
		}
		row.FailedSites = int(failed)
		result = append(result, row)
	}
	return result, rows.Err()
}

// Prune 删除 before 之前的营收与成本记录。
func (r *Repository) Prune(ctx context.Context, ws Workspace, before time.Time) error {
	if _, err := r.db.Exec(ctx, `DELETE FROM profit_route_daily WHERE user_id = $1 AND admin_account_id = $2 AND day < $3`, ws.UserID, ws.AdminAccountID, before); err != nil {
		return err
	}
	if _, err := r.db.Exec(ctx, `DELETE FROM profit_upstream_daily WHERE user_id = $1 AND admin_account_id = $2 AND day < $3`, ws.UserID, ws.AdminAccountID, before); err != nil {
		return err
	}
	_, err := r.db.Exec(ctx, `DELETE FROM profit_price_changes WHERE user_id = $1 AND admin_account_id = $2 AND detected_at < $3`, ws.UserID, ws.AdminAccountID, before)
	return err
}

// InsertPriceChange 记录一次上游分组调价及其影响估算，返回带 ID 和检测时间的记录。
func (r *Repository) InsertPriceChange(ctx context.Context, ws Workspace, change PriceChange) (PriceChange, error) {
	err := r.db.QueryRow(ctx, `
		INSERT INTO profit_price_changes (
			user_id, admin_account_id, site_id, site_name, base_url, group_id, group_name, old_multiplier, new_multiplier,
			recharge_rate, window_days, window_consume, estimate_status, avg_daily_cost, daily_impact, detected_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		RETURNING id
	`, ws.UserID, ws.AdminAccountID, change.SiteID, change.SiteName, change.BaseURL, change.GroupID, change.GroupName, change.OldMultiplier,
		change.NewMultiplier, change.RechargeRate, change.WindowDays, change.WindowConsume, change.EstimateStatus, change.AvgDailyCost,
		change.DailyImpact, change.DetectedAt).Scan(&change.ID)
	return change, err
}

// ListPriceChanges 返回 [since, until) 内检测到的调价记录，最新的在前，最多 limit 条。
func (r *Repository) ListPriceChanges(ctx context.Context, ws Workspace, since time.Time, until time.Time, limit int) ([]PriceChange, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, site_id, site_name, base_url, group_id, group_name, old_multiplier, new_multiplier, recharge_rate, window_days,
			window_consume, estimate_status, avg_daily_cost, daily_impact, detected_at
		FROM profit_price_changes
		WHERE user_id = $1 AND admin_account_id = $2 AND detected_at >= $3 AND detected_at < $4
		ORDER BY detected_at DESC, id DESC
		LIMIT $5
	`, ws.UserID, ws.AdminAccountID, since, until, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	changes := make([]PriceChange, 0)
	for rows.Next() {
		var change PriceChange
		if err := rows.Scan(&change.ID, &change.SiteID, &change.SiteName, &change.BaseURL, &change.GroupID, &change.GroupName,
			&change.OldMultiplier, &change.NewMultiplier, &change.RechargeRate, &change.WindowDays, &change.WindowConsume,
			&change.EstimateStatus, &change.AvgDailyCost, &change.DailyImpact, &change.DetectedAt); err != nil {
			return nil, err
		}
		changes = append(changes, change)
	}
	return changes, rows.Err()
}
