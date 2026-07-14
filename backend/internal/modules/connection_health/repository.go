package connection_health

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// EnsureSchema 创建健康探活模块所需的四张表和索引。语句全部幂等（IF NOT EXISTS），
// 不修改任何已有表，符合线上兼容要求。表结构与任务书给定的建表语句保持一致。
func (r *Repository) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS connection_health_policies (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			name text NOT NULL,
			enabled boolean NOT NULL DEFAULT true,
			own_group_id text NOT NULL DEFAULT '',
			own_group_name text NOT NULL DEFAULT '',
			model_pattern text NOT NULL DEFAULT '*',
			probe_mode text NOT NULL DEFAULT 'real_model',
			probe_interval_seconds integer NOT NULL DEFAULT 60,
			failure_threshold integer NOT NULL DEFAULT 3,
			success_threshold integer NOT NULL DEFAULT 2,
			cooldown_seconds integer NOT NULL DEFAULT 300,
			observation_seconds integer NOT NULL DEFAULT 300,
			recovery_step_percent integer NOT NULL DEFAULT 25,
			auto_degrade_enabled boolean NOT NULL DEFAULT true,
			auto_remote_action_enabled boolean NOT NULL DEFAULT true,
			daily_probe_budget integer NOT NULL DEFAULT 1000,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_policies_workspace_enabled ON connection_health_policies (user_id, admin_account_id, enabled)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_policies_group_model ON connection_health_policies (user_id, admin_account_id, own_group_name, model_pattern)`,

		`CREATE TABLE IF NOT EXISTS connection_health_model_targets (
			id text PRIMARY KEY,
			policy_id text NOT NULL,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			model_name text NOT NULL,
			provider_family text NOT NULL DEFAULT '',
			enabled boolean NOT NULL DEFAULT true,
			probe_prompt text NOT NULL DEFAULT '',
			max_probe_tokens integer NOT NULL DEFAULT 1,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_model_targets_policy ON connection_health_model_targets (policy_id)`,

		`CREATE TABLE IF NOT EXISTS connection_health_states (
			connection_id text NOT NULL,
			model_name text NOT NULL DEFAULT '*',
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			own_group_id text NOT NULL DEFAULT '',
			own_group_name text NOT NULL DEFAULT '',
			upstream_site_id text NOT NULL,
			upstream_group_id text NOT NULL DEFAULT '',
			upstream_group_name text NOT NULL,
			state text NOT NULL,
			current_weight integer NOT NULL DEFAULT 100,
			consecutive_failures integer NOT NULL DEFAULT 0,
			consecutive_successes integer NOT NULL DEFAULT 0,
			last_probe_at timestamptz NULL,
			last_success_at timestamptz NULL,
			last_failure_at timestamptz NULL,
			cooldown_until timestamptz NULL,
			observing_until timestamptz NULL,
			last_latency_ms integer NULL,
			last_error_key text NOT NULL DEFAULT '',
			last_error_detail text NOT NULL DEFAULT '',
			last_remote_action text NOT NULL DEFAULT '',
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (connection_id, model_name)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_states_workspace_state ON connection_health_states (user_id, admin_account_id, state)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_states_group ON connection_health_states (user_id, admin_account_id, own_group_name)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_states_site_group ON connection_health_states (upstream_site_id, upstream_group_name)`,

		`CREATE TABLE IF NOT EXISTS connection_health_events (
			id text PRIMARY KEY,
			connection_id text NOT NULL,
			model_name text NOT NULL DEFAULT '*',
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			own_group_name text NOT NULL DEFAULT '',
			upstream_site_id text NOT NULL DEFAULT '',
			upstream_group_name text NOT NULL DEFAULT '',
			result text NOT NULL,
			from_state text NOT NULL DEFAULT '',
			to_state text NOT NULL DEFAULT '',
			latency_ms integer NULL,
			error_key text NOT NULL DEFAULT '',
			error_detail text NOT NULL DEFAULT '',
			remote_action text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_events_connection_created ON connection_health_events (connection_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_events_workspace_created ON connection_health_events (user_id, admin_account_id, created_at DESC)`,

		`CREATE TABLE IF NOT EXISTS connection_health_policy_assignments (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			target_id text NOT NULL,
			policy_id text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			UNIQUE (user_id, admin_account_id, target_id, policy_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_policy_assignments_workspace_target ON connection_health_policy_assignments (user_id, admin_account_id, target_id)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_policy_assignments_policy ON connection_health_policy_assignments (policy_id)`,
	}
	for _, stmt := range statements {
		if _, err := r.db.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// UpsertPolicy 插入一条新策略，或按 id 更新已有策略（不含 model targets）。
func (r *Repository) UpsertPolicy(ctx context.Context, p Policy) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO connection_health_policies (
			id, user_id, admin_account_id, name, enabled, own_group_id, own_group_name, model_pattern, probe_mode,
			probe_interval_seconds, failure_threshold, success_threshold, cooldown_seconds, observation_seconds,
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, daily_probe_budget, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,now(),now())
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			enabled = EXCLUDED.enabled,
			own_group_id = EXCLUDED.own_group_id,
			own_group_name = EXCLUDED.own_group_name,
			model_pattern = EXCLUDED.model_pattern,
			probe_mode = EXCLUDED.probe_mode,
			probe_interval_seconds = EXCLUDED.probe_interval_seconds,
			failure_threshold = EXCLUDED.failure_threshold,
			success_threshold = EXCLUDED.success_threshold,
			cooldown_seconds = EXCLUDED.cooldown_seconds,
			observation_seconds = EXCLUDED.observation_seconds,
			recovery_step_percent = EXCLUDED.recovery_step_percent,
			auto_degrade_enabled = EXCLUDED.auto_degrade_enabled,
			auto_remote_action_enabled = EXCLUDED.auto_remote_action_enabled,
			daily_probe_budget = EXCLUDED.daily_probe_budget,
			updated_at = now()
	`, p.ID, p.UserID, p.AdminAccountID, p.Name, p.Enabled, p.OwnGroupID, p.OwnGroupName, p.ModelPattern, p.ProbeMode,
		p.ProbeIntervalSeconds, p.FailureThreshold, p.SuccessThreshold, p.CooldownSeconds, p.ObservationSeconds,
		p.RecoveryStepPercent, p.AutoDegradeEnabled, p.AutoRemoteActionEnabled, p.DailyProbeBudget)
	return err
}

// ReplaceModelTargets 用给定的目标列表整体替换一个策略下的模型目标（先删后插，事务保证一致）。
func (r *Repository) ReplaceModelTargets(ctx context.Context, policyID string, targets []ModelTarget) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM connection_health_model_targets WHERE policy_id = $1`, policyID); err != nil {
		return err
	}
	for _, t := range targets {
		if _, err := tx.Exec(ctx, `
			INSERT INTO connection_health_model_targets
				(id, policy_id, user_id, admin_account_id, model_name, provider_family, enabled, probe_prompt, max_probe_tokens, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),now())
		`, t.ID, policyID, t.UserID, t.AdminAccountID, t.ModelName, t.ProviderFamily, t.Enabled, t.ProbePrompt, t.MaxProbeTokens); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Repository) ListModelTargets(ctx context.Context, policyID string) ([]ModelTarget, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, policy_id, user_id, admin_account_id, model_name, provider_family, enabled, probe_prompt, max_probe_tokens, created_at, updated_at
		FROM connection_health_model_targets WHERE policy_id = $1 ORDER BY created_at ASC
	`, policyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]ModelTarget, 0)
	for rows.Next() {
		var t ModelTarget
		if err := rows.Scan(&t.ID, &t.PolicyID, &t.UserID, &t.AdminAccountID, &t.ModelName, &t.ProviderFamily, &t.Enabled, &t.ProbePrompt, &t.MaxProbeTokens, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, rows.Err()
}

func (r *Repository) GetPolicy(ctx context.Context, id string, userID string, adminAccountID string) (*Policy, error) {
	row := r.db.QueryRow(ctx, `
		SELECT id, user_id, admin_account_id, name, enabled, own_group_id, own_group_name, model_pattern, probe_mode,
			probe_interval_seconds, failure_threshold, success_threshold, cooldown_seconds, observation_seconds,
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, daily_probe_budget, created_at, updated_at
		FROM connection_health_policies WHERE id = $1 AND user_id = $2 AND admin_account_id = $3
	`, id, userID, adminAccountID)
	p, err := scanPolicy(row)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, nil
	}
	targets, err := r.ListModelTargets(ctx, p.ID)
	if err != nil {
		return nil, err
	}
	p.ModelTargets = targets
	return p, nil
}

// ListPolicies 返回指定 workspace 下的全部策略（含各自的 model targets）。
func (r *Repository) ListPolicies(ctx context.Context, userID string, adminAccountID string) ([]Policy, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, name, enabled, own_group_id, own_group_name, model_pattern, probe_mode,
			probe_interval_seconds, failure_threshold, success_threshold, cooldown_seconds, observation_seconds,
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, daily_probe_budget, created_at, updated_at
		FROM connection_health_policies WHERE user_id = $1 AND admin_account_id = $2 ORDER BY created_at ASC
	`, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	policies := make([]Policy, 0)
	for rows.Next() {
		p, err := scanPolicyRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		policies = append(policies, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	for i := range policies {
		targets, err := r.ListModelTargets(ctx, policies[i].ID)
		if err != nil {
			return nil, err
		}
		policies[i].ModelTargets = targets
	}
	return policies, nil
}

// ListEnabledPolicies 返回全部 workspace 中已启用的策略（含 model targets），供调度器全局扫描使用。
func (r *Repository) ListEnabledPolicies(ctx context.Context) ([]Policy, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, name, enabled, own_group_id, own_group_name, model_pattern, probe_mode,
			probe_interval_seconds, failure_threshold, success_threshold, cooldown_seconds, observation_seconds,
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, daily_probe_budget, created_at, updated_at
		FROM connection_health_policies WHERE enabled = true ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	policies := make([]Policy, 0)
	for rows.Next() {
		p, err := scanPolicyRow(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		policies = append(policies, *p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	for i := range policies {
		targets, err := r.ListModelTargets(ctx, policies[i].ID)
		if err != nil {
			return nil, err
		}
		enabled := make([]ModelTarget, 0, len(targets))
		for _, t := range targets {
			if t.Enabled {
				enabled = append(enabled, t)
			}
		}
		policies[i].ModelTargets = enabled
	}
	return policies, nil
}

func scanPolicy(row pgx.Row) (*Policy, error) {
	var p Policy
	if err := row.Scan(&p.ID, &p.UserID, &p.AdminAccountID, &p.Name, &p.Enabled, &p.OwnGroupID, &p.OwnGroupName, &p.ModelPattern, &p.ProbeMode,
		&p.ProbeIntervalSeconds, &p.FailureThreshold, &p.SuccessThreshold, &p.CooldownSeconds, &p.ObservationSeconds,
		&p.RecoveryStepPercent, &p.AutoDegradeEnabled, &p.AutoRemoteActionEnabled, &p.DailyProbeBudget, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPolicyRow(row rowScanner) (*Policy, error) {
	var p Policy
	if err := row.Scan(&p.ID, &p.UserID, &p.AdminAccountID, &p.Name, &p.Enabled, &p.OwnGroupID, &p.OwnGroupName, &p.ModelPattern, &p.ProbeMode,
		&p.ProbeIntervalSeconds, &p.FailureThreshold, &p.SuccessThreshold, &p.CooldownSeconds, &p.ObservationSeconds,
		&p.RecoveryStepPercent, &p.AutoDegradeEnabled, &p.AutoRemoteActionEnabled, &p.DailyProbeBudget, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	return &p, nil
}

// UpsertState 按 (connection_id, model_name) 写入或更新一条健康状态。
func (r *Repository) UpsertState(ctx context.Context, s ConnectionHealthState) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO connection_health_states (
			connection_id, model_name, user_id, admin_account_id, own_group_id, own_group_name,
			upstream_site_id, upstream_group_id, upstream_group_name, state, current_weight,
			consecutive_failures, consecutive_successes, last_probe_at, last_success_at, last_failure_at,
			cooldown_until, observing_until, last_latency_ms, last_error_key, last_error_detail, last_remote_action, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,now())
		ON CONFLICT (connection_id, model_name) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			admin_account_id = EXCLUDED.admin_account_id,
			own_group_id = EXCLUDED.own_group_id,
			own_group_name = EXCLUDED.own_group_name,
			upstream_site_id = EXCLUDED.upstream_site_id,
			upstream_group_id = EXCLUDED.upstream_group_id,
			upstream_group_name = EXCLUDED.upstream_group_name,
			state = EXCLUDED.state,
			current_weight = EXCLUDED.current_weight,
			consecutive_failures = EXCLUDED.consecutive_failures,
			consecutive_successes = EXCLUDED.consecutive_successes,
			last_probe_at = EXCLUDED.last_probe_at,
			last_success_at = EXCLUDED.last_success_at,
			last_failure_at = EXCLUDED.last_failure_at,
			cooldown_until = EXCLUDED.cooldown_until,
			observing_until = EXCLUDED.observing_until,
			last_latency_ms = EXCLUDED.last_latency_ms,
			last_error_key = EXCLUDED.last_error_key,
			last_error_detail = EXCLUDED.last_error_detail,
			last_remote_action = EXCLUDED.last_remote_action,
			updated_at = now()
	`, s.ConnectionID, s.ModelName, s.UserID, s.AdminAccountID, s.OwnGroupID, s.OwnGroupName,
		s.UpstreamSiteID, s.UpstreamGroupID, s.UpstreamGroupName, string(s.State), s.CurrentWeight,
		s.ConsecutiveFailures, s.ConsecutiveSuccesses, s.LastProbeAt, s.LastSuccessAt, s.LastFailureAt,
		s.CooldownUntil, s.ObservingUntil, s.LastLatencyMs, s.LastErrorKey, s.LastErrorDetail, s.LastRemoteAction)
	return err
}

func (r *Repository) GetState(ctx context.Context, connectionID string, modelName string) (*ConnectionHealthState, error) {
	row := r.db.QueryRow(ctx, `
		SELECT connection_id, model_name, user_id, admin_account_id, own_group_id, own_group_name,
			upstream_site_id, upstream_group_id, upstream_group_name, state, current_weight,
			consecutive_failures, consecutive_successes, last_probe_at, last_success_at, last_failure_at,
			cooldown_until, observing_until, last_latency_ms, last_error_key, last_error_detail, last_remote_action, updated_at
		FROM connection_health_states WHERE connection_id = $1 AND model_name = $2
	`, connectionID, modelName)
	return scanState(row)
}

// ListStatesByWorkspace 返回指定 workspace 下的全部健康状态行，供聚合大屏使用。
func (r *Repository) ListStatesByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]ConnectionHealthState, error) {
	rows, err := r.db.Query(ctx, `
		SELECT connection_id, model_name, user_id, admin_account_id, own_group_id, own_group_name,
			upstream_site_id, upstream_group_id, upstream_group_name, state, current_weight,
			consecutive_failures, consecutive_successes, last_probe_at, last_success_at, last_failure_at,
			cooldown_until, observing_until, last_latency_ms, last_error_key, last_error_detail, last_remote_action, updated_at
		FROM connection_health_states WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make([]ConnectionHealthState, 0)
	for rows.Next() {
		s, err := scanStateRow(rows)
		if err != nil {
			return nil, err
		}
		states = append(states, *s)
	}
	return states, rows.Err()
}

// ListStatesByConnection 返回一条连接下全部模型的健康状态行。
func (r *Repository) ListStatesByConnection(ctx context.Context, connectionID string) ([]ConnectionHealthState, error) {
	rows, err := r.db.Query(ctx, `
		SELECT connection_id, model_name, user_id, admin_account_id, own_group_id, own_group_name,
			upstream_site_id, upstream_group_id, upstream_group_name, state, current_weight,
			consecutive_failures, consecutive_successes, last_probe_at, last_success_at, last_failure_at,
			cooldown_until, observing_until, last_latency_ms, last_error_key, last_error_detail, last_remote_action, updated_at
		FROM connection_health_states WHERE connection_id = $1
	`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make([]ConnectionHealthState, 0)
	for rows.Next() {
		s, err := scanStateRow(rows)
		if err != nil {
			return nil, err
		}
		states = append(states, *s)
	}
	return states, rows.Err()
}

func scanState(row pgx.Row) (*ConnectionHealthState, error) {
	var s ConnectionHealthState
	var state string
	if err := row.Scan(&s.ConnectionID, &s.ModelName, &s.UserID, &s.AdminAccountID, &s.OwnGroupID, &s.OwnGroupName,
		&s.UpstreamSiteID, &s.UpstreamGroupID, &s.UpstreamGroupName, &state, &s.CurrentWeight,
		&s.ConsecutiveFailures, &s.ConsecutiveSuccesses, &s.LastProbeAt, &s.LastSuccessAt, &s.LastFailureAt,
		&s.CooldownUntil, &s.ObservingUntil, &s.LastLatencyMs, &s.LastErrorKey, &s.LastErrorDetail, &s.LastRemoteAction, &s.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	s.State = State(state)
	return &s, nil
}

func scanStateRow(row rowScanner) (*ConnectionHealthState, error) {
	var s ConnectionHealthState
	var state string
	if err := row.Scan(&s.ConnectionID, &s.ModelName, &s.UserID, &s.AdminAccountID, &s.OwnGroupID, &s.OwnGroupName,
		&s.UpstreamSiteID, &s.UpstreamGroupID, &s.UpstreamGroupName, &state, &s.CurrentWeight,
		&s.ConsecutiveFailures, &s.ConsecutiveSuccesses, &s.LastProbeAt, &s.LastSuccessAt, &s.LastFailureAt,
		&s.CooldownUntil, &s.ObservingUntil, &s.LastLatencyMs, &s.LastErrorKey, &s.LastErrorDetail, &s.LastRemoteAction, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.State = State(state)
	return &s, nil
}

// InsertEvent 写入一条探活/远端动作事件，不吞错误。
func (r *Repository) InsertEvent(ctx context.Context, e ConnectionHealthEvent) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO connection_health_events (
			id, connection_id, model_name, user_id, admin_account_id, own_group_name,
			upstream_site_id, upstream_group_name, result, from_state, to_state,
			latency_ms, error_key, error_detail, remote_action, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now())
	`, e.ID, e.ConnectionID, e.ModelName, e.UserID, e.AdminAccountID, e.OwnGroupName,
		e.UpstreamSiteID, e.UpstreamGroupName, e.Result, e.FromState, e.ToState,
		e.LatencyMs, e.ErrorKey, e.ErrorDetail, e.RemoteAction)
	return err
}

// ListEventsByConnection 返回某条连接最近的事件，按时间倒序。必须带 user_id + admin_account_id
// 过滤：仅按 connection_id 查询会让同一登录用户读取到其他 workspace 的事件（IDOR），
// 调用方（Service.Events）已先校验连接归属，这里的过滤是第二道防线，双重保险。
func (r *Repository) ListEventsByConnection(ctx context.Context, connectionID string, userID string, adminAccountID string, limit int) ([]ConnectionHealthEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, connection_id, model_name, user_id, admin_account_id, own_group_name,
			upstream_site_id, upstream_group_name, result, from_state, to_state,
			latency_ms, error_key, error_detail, remote_action, created_at
		FROM connection_health_events WHERE connection_id = $1 AND user_id = $2 AND admin_account_id = $3 ORDER BY created_at DESC LIMIT $4
	`, connectionID, userID, adminAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// ListRecentEventsByWorkspace 返回 workspace 下最近的事件，供大屏「最近探活和远端动作」使用。
func (r *Repository) ListRecentEventsByWorkspace(ctx context.Context, userID string, adminAccountID string, limit int) ([]ConnectionHealthEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, connection_id, model_name, user_id, admin_account_id, own_group_name,
			upstream_site_id, upstream_group_name, result, from_state, to_state,
			latency_ms, error_key, error_detail, remote_action, created_at
		FROM connection_health_events WHERE user_id = $1 AND admin_account_id = $2 ORDER BY created_at DESC LIMIT $3
	`, userID, adminAccountID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEvents(rows)
}

// CountProbesToday 统计 workspace 当天真实探活次数（不含人工操作和 unsupported 远端动作标记），
// 用于每日探活预算控制。
func (r *Repository) CountProbesToday(ctx context.Context, userID string, adminAccountID string, dayStart time.Time) (int, error) {
	row := r.db.QueryRow(ctx, `
		SELECT count(*) FROM connection_health_events
		WHERE user_id = $1 AND admin_account_id = $2 AND created_at >= $3
			AND result = ANY($4)
	`, userID, adminAccountID, dayStart, probeResultKeys())
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func probeResultKeys() []string {
	return []string{
		string(ResultOK), string(ResultNetworkFluctuation), string(ResultRateLimited),
		string(ResultServerError), string(ResultAuth), string(ResultModelNotFound), string(ResultInvalidResponse),
	}
}

func scanEvents(rows pgx.Rows) ([]ConnectionHealthEvent, error) {
	events := make([]ConnectionHealthEvent, 0)
	for rows.Next() {
		var e ConnectionHealthEvent
		if err := rows.Scan(&e.ID, &e.ConnectionID, &e.ModelName, &e.UserID, &e.AdminAccountID, &e.OwnGroupName,
			&e.UpstreamSiteID, &e.UpstreamGroupName, &e.Result, &e.FromState, &e.ToState,
			&e.LatencyMs, &e.ErrorKey, &e.ErrorDetail, &e.RemoteAction, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// ReplacePolicyAssignments 整体替换一个 target 在当前 workspace 下的策略分配（先删后插，事务保证一致）。
// policyIDs 为空即清空该 target 的全部分配。
func (r *Repository) ReplacePolicyAssignments(ctx context.Context, userID string, adminAccountID string, targetID string, policyIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		DELETE FROM connection_health_policy_assignments WHERE user_id = $1 AND admin_account_id = $2 AND target_id = $3
	`, userID, adminAccountID, targetID); err != nil {
		return err
	}
	for _, policyID := range policyIDs {
		id, err := newID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO connection_health_policy_assignments (id, user_id, admin_account_id, target_id, policy_id, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,now(),now())
		`, id, userID, adminAccountID, targetID, policyID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListPolicyAssignmentsForTarget 返回某个 target 在当前 workspace 下已分配的全部策略行。
func (r *Repository) ListPolicyAssignmentsForTarget(ctx context.Context, userID string, adminAccountID string, targetID string) ([]PolicyAssignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, target_id, policy_id, created_at, updated_at
		FROM connection_health_policy_assignments WHERE user_id = $1 AND admin_account_id = $2 AND target_id = $3
	`, userID, adminAccountID, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPolicyAssignments(rows)
}

// ListPolicyAssignmentsByWorkspace 返回当前 workspace 下全部 target 的策略分配行，
// 供 AdminGroups 聚合展示、事件按分配过滤复用，避免逐个 target 单独查询。
func (r *Repository) ListPolicyAssignmentsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]PolicyAssignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, target_id, policy_id, created_at, updated_at
		FROM connection_health_policy_assignments WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPolicyAssignments(rows)
}

// ListAllPolicyAssignments 返回全部 workspace 的策略分配行，供调度器全局扫描使用
// （风格对齐 ListEnabledPolicies：调度器用 context.Background() 启动，没有请求态 workspace）。
func (r *Repository) ListAllPolicyAssignments(ctx context.Context) ([]PolicyAssignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, target_id, policy_id, created_at, updated_at
		FROM connection_health_policy_assignments
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPolicyAssignments(rows)
}

func scanPolicyAssignments(rows pgx.Rows) ([]PolicyAssignment, error) {
	assignments := make([]PolicyAssignment, 0)
	for rows.Next() {
		var a PolicyAssignment
		if err := rows.Scan(&a.ID, &a.UserID, &a.AdminAccountID, &a.TargetID, &a.PolicyID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New("generate connection health id")
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
