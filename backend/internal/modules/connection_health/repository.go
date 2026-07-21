package connection_health

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// EnsureSchema 创建健康探活模块所需的表和索引。所有新增列/表都使用 IF NOT EXISTS，
// 已上线实例可以原地升级；旧策略的 priority_mode 默认 none，行为保持不变。
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
			priority_mode text NOT NULL DEFAULT 'none',
			daily_probe_budget integer NOT NULL DEFAULT 1000,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`ALTER TABLE connection_health_policies ADD COLUMN IF NOT EXISTS priority_mode text NOT NULL DEFAULT 'none'`,
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
			policy_id text NOT NULL DEFAULT '',
			admin_group_id text NOT NULL DEFAULT '',
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
		`ALTER TABLE connection_health_events ADD COLUMN IF NOT EXISTS policy_id text NOT NULL DEFAULT ''`,
		`ALTER TABLE connection_health_events ADD COLUMN IF NOT EXISTS admin_group_id text NOT NULL DEFAULT ''`,
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

		`CREATE TABLE IF NOT EXISTS connection_health_group_policy_assignments (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			admin_group_id text NOT NULL,
			admin_group_name text NOT NULL DEFAULT '',
			policy_id text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			UNIQUE (user_id, admin_account_id, admin_group_id, policy_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_group_policy_workspace_group ON connection_health_group_policy_assignments (user_id, admin_account_id, admin_group_id)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_group_policy_policy ON connection_health_group_policy_assignments (policy_id)`,

		`CREATE TABLE IF NOT EXISTS connection_health_group_target_exclusions (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			admin_group_id text NOT NULL,
			target_id text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now(),
			UNIQUE (user_id, admin_account_id, admin_group_id, target_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_group_exclusions_workspace_group ON connection_health_group_target_exclusions (user_id, admin_account_id, admin_group_id)`,

		`CREATE TABLE IF NOT EXISTS connection_health_priority_sync_states (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			target_id text NOT NULL,
			original_priority integer NOT NULL DEFAULT 0,
			last_applied_priority integer NOT NULL DEFAULT 0,
			effective_multiplier double precision NOT NULL DEFAULT 0,
			conflict boolean NOT NULL DEFAULT false,
			last_conflict_priority integer NULL,
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, admin_account_id, target_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_priority_sync_workspace ON connection_health_priority_sync_states (user_id, admin_account_id)`,
		`ALTER TABLE connection_health_priority_sync_states ADD COLUMN IF NOT EXISTS pending_priority integer NULL`,

		`CREATE TABLE IF NOT EXISTS connection_health_target_action_states (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			target_id text NOT NULL,
			original_status text NOT NULL DEFAULT '',
			original_weight integer NULL,
			last_applied_status text NOT NULL DEFAULT '',
			last_applied_weight integer NULL,
			conflict boolean NOT NULL DEFAULT false,
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, admin_account_id, target_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_connection_health_target_action_workspace ON connection_health_target_action_states (user_id, admin_account_id)`,
		`ALTER TABLE connection_health_target_action_states ADD COLUMN IF NOT EXISTS pending_status text NOT NULL DEFAULT ''`,
		`ALTER TABLE connection_health_target_action_states ADD COLUMN IF NOT EXISTS pending_weight integer NULL`,

		`CREATE TABLE IF NOT EXISTS connection_health_probe_budget_usage (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			policy_id text NOT NULL,
			day_start timestamptz NOT NULL,
			used integer NOT NULL DEFAULT 0,
			updated_at timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, admin_account_id, policy_id, day_start)
		)`,
		`CREATE TABLE IF NOT EXISTS connection_health_runtime_leases (
			lease_key text PRIMARY KEY,
			owner_id text NOT NULL,
			expires_at timestamptz NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
	}
	for _, stmt := range statements {
		if _, err := r.db.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

type policyExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// upsertPolicyWithExecutor 让单独保存和组合事务复用完全相同的 upsert 语义。
func upsertPolicyWithExecutor(ctx context.Context, executor policyExecutor, p Policy) error {
	_, err := executor.Exec(ctx, `
		INSERT INTO connection_health_policies (
			id, user_id, admin_account_id, name, enabled, own_group_id, own_group_name, model_pattern, probe_mode,
			probe_interval_seconds, failure_threshold, success_threshold, cooldown_seconds, observation_seconds,
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, priority_mode, daily_probe_budget, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,now(),now())
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
			priority_mode = EXCLUDED.priority_mode,
			daily_probe_budget = EXCLUDED.daily_probe_budget,
			updated_at = now()
	`, p.ID, p.UserID, p.AdminAccountID, p.Name, p.Enabled, p.OwnGroupID, p.OwnGroupName, p.ModelPattern, p.ProbeMode,
		p.ProbeIntervalSeconds, p.FailureThreshold, p.SuccessThreshold, p.CooldownSeconds, p.ObservationSeconds,
		p.RecoveryStepPercent, p.AutoDegradeEnabled, p.AutoRemoteActionEnabled, normalizePriorityMode(p.PriorityMode), p.DailyProbeBudget)
	return err
}

// UpsertPolicy 保留给模块内旧调用兼容；新的 Service 保存链路使用 SavePolicyWithTargets。
func (r *Repository) UpsertPolicy(ctx context.Context, p Policy) error {
	return upsertPolicyWithExecutor(ctx, r.db, p)
}

func replaceModelTargetsTx(ctx context.Context, tx pgx.Tx, policyID string, targets []ModelTarget) error {
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
	return nil
}

// ReplaceModelTargets 用给定的目标列表整体替换一个策略下的模型目标（先删后插，事务保证一致）。
func (r *Repository) ReplaceModelTargets(ctx context.Context, policyID string, targets []ModelTarget) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := replaceModelTargetsTx(ctx, tx, policyID, targets); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SavePolicyWithTargets 在一个事务中保存策略主体并整体替换模型目标。接口只有在两部分都
// 成功提交后才返回成功，避免调度器读取到“新策略参数 + 旧模型目标”的半完成配置。
func (r *Repository) SavePolicyWithTargets(ctx context.Context, policy Policy, targets []ModelTarget) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := upsertPolicyWithExecutor(ctx, tx, policy); err != nil {
		return err
	}
	if err := replaceModelTargetsTx(ctx, tx, policy.ID, targets); err != nil {
		return err
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
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, priority_mode, daily_probe_budget, created_at, updated_at
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
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, priority_mode, daily_probe_budget, created_at, updated_at
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
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, priority_mode, daily_probe_budget, created_at, updated_at
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
		&p.RecoveryStepPercent, &p.AutoDegradeEnabled, &p.AutoRemoteActionEnabled, &p.PriorityMode, &p.DailyProbeBudget, &p.CreatedAt, &p.UpdatedAt); err != nil {
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
		&p.RecoveryStepPercent, &p.AutoDegradeEnabled, &p.AutoRemoteActionEnabled, &p.PriorityMode, &p.DailyProbeBudget, &p.CreatedAt, &p.UpdatedAt); err != nil {
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
			id, connection_id, model_name, user_id, admin_account_id, policy_id, admin_group_id, own_group_name,
			upstream_site_id, upstream_group_name, result, from_state, to_state,
			latency_ms, error_key, error_detail, remote_action, created_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,now())
	`, e.ID, e.ConnectionID, e.ModelName, e.UserID, e.AdminAccountID, e.PolicyID, e.AdminGroupID, e.OwnGroupName,
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
		SELECT id, connection_id, model_name, user_id, admin_account_id, policy_id, admin_group_id, own_group_name,
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
		SELECT id, connection_id, model_name, user_id, admin_account_id, policy_id, admin_group_id, own_group_name,
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

// CountProbesToday 按策略统计当天真实探活次数。旧事件没有 policy_id，不再与新策略共享预算；
// 这样升级后每条策略都严格消费自己的 DailyProbeBudget。
func (r *Repository) CountProbesToday(ctx context.Context, userID string, adminAccountID string, policyID string, dayStart time.Time) (int, error) {
	row := r.db.QueryRow(ctx, `
		SELECT GREATEST(
			(SELECT count(*) FROM connection_health_events
			 WHERE user_id = $1 AND admin_account_id = $2 AND policy_id = $3 AND created_at >= $4
			   AND result = ANY($5)),
			COALESCE((SELECT used FROM connection_health_probe_budget_usage
			          WHERE user_id = $1 AND admin_account_id = $2 AND policy_id = $3 AND day_start = $4), 0)
		)
	`, userID, adminAccountID, policyID, dayStart, probeResultKeys())
	var count int
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

// TryConsumeProbeBudget atomically reserves one probe. The first reservation of a day seeds
// the counter from existing events so a rolling upgrade does not reset an already-used budget.
func (r *Repository) TryConsumeProbeBudget(ctx context.Context, userID string, adminAccountID string, policyID string, dayStart time.Time, limit int) (bool, error) {
	if limit <= 0 {
		return false, nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO connection_health_probe_budget_usage (
			user_id, admin_account_id, policy_id, day_start, used, updated_at
		)
		SELECT $1, $2, $3, $4, count(*)::integer, now()
		FROM connection_health_events
		WHERE user_id = $1 AND admin_account_id = $2 AND policy_id = $3 AND created_at >= $4
			AND result = ANY($5)
		ON CONFLICT (user_id, admin_account_id, policy_id, day_start) DO NOTHING
	`, userID, adminAccountID, policyID, dayStart, probeResultKeys()); err != nil {
		return false, err
	}
	var used int
	err = tx.QueryRow(ctx, `
		UPDATE connection_health_probe_budget_usage
		SET used = used + 1, updated_at = now()
		WHERE user_id = $1 AND admin_account_id = $2 AND policy_id = $3 AND day_start = $4 AND used < $5
		RETURNING used
	`, userID, adminAccountID, policyID, dayStart, limit).Scan(&used)
	if errors.Is(err, pgx.ErrNoRows) {
		if commitErr := tx.Commit(ctx); commitErr != nil {
			return false, commitErr
		}
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) TryAcquireSchedulerLease(ctx context.Context) (func(), bool, error) {
	release, acquired, err := r.acquireRuntimeLease(ctx, "connection-health:scheduler", false)
	return release, acquired, err
}

func (r *Repository) AcquireTargetLease(ctx context.Context, targetID string) (func(), error) {
	release, _, err := r.acquireRuntimeLease(ctx, "connection-health:target:"+targetID, true)
	return release, err
}

// acquireRuntimeLease is a database-backed lease that does not pin a pgx pool connection.
// Scheduler acquisition is non-blocking; target acquisition waits until the current probe
// releases it. A heartbeat lets crashed processes recover without manual cleanup.
func (r *Repository) acquireRuntimeLease(ctx context.Context, key string, wait bool) (func(), bool, error) {
	ownerID, err := newID()
	if err != nil {
		return nil, false, err
	}
	const leaseTTL = 2 * time.Minute
	const leaseQueryTimeout = 5 * time.Second
	leaseTTLSeconds := int(leaseTTL / time.Second)
	for {
		var returnedOwner string
		err = r.db.QueryRow(ctx, `
			INSERT INTO connection_health_runtime_leases (lease_key, owner_id, expires_at, updated_at)
			VALUES ($1, $2, now() + make_interval(secs => $3), now())
			ON CONFLICT (lease_key) DO UPDATE SET
				owner_id = EXCLUDED.owner_id,
				expires_at = EXCLUDED.expires_at,
				updated_at = now()
			WHERE connection_health_runtime_leases.expires_at <= now()
			RETURNING owner_id
		`, key, ownerID, leaseTTLSeconds).Scan(&returnedOwner)
		if err == nil {
			break
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, false, err
		}
		if !wait {
			return nil, false, nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, false, ctx.Err()
		case <-timer.C:
		}
	}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				// A database outage must not leave the heartbeat goroutine blocked forever,
				// otherwise releasing the lease could also block service shutdown.
				heartbeatCtx, cancelHeartbeat := context.WithTimeout(context.Background(), leaseQueryTimeout)
				_, heartbeatErr := r.db.Exec(heartbeatCtx, `
					UPDATE connection_health_runtime_leases
					SET expires_at = now() + make_interval(secs => $3), updated_at = now()
					WHERE lease_key = $1 AND owner_id = $2
				`, key, ownerID, leaseTTLSeconds)
				cancelHeartbeat()
				if heartbeatErr != nil {
					log.Printf("[connection-health] runtime lease heartbeat failed key=%s err=%v", key, heartbeatErr)
				}
			}
		}
	}()

	var once sync.Once
	release := func() {
		once.Do(func() {
			close(stop)
			<-done
			// Lease expiration remains the final crash-recovery mechanism if this
			// best-effort delete cannot reach PostgreSQL during shutdown.
			releaseCtx, cancelRelease := context.WithTimeout(context.Background(), leaseQueryTimeout)
			defer cancelRelease()
			_, _ = r.db.Exec(releaseCtx, `
				DELETE FROM connection_health_runtime_leases WHERE lease_key = $1 AND owner_id = $2
			`, key, ownerID)
		})
	}
	return release, true, nil
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
		if err := rows.Scan(&e.ID, &e.ConnectionID, &e.ModelName, &e.UserID, &e.AdminAccountID, &e.PolicyID, &e.AdminGroupID, &e.OwnGroupName,
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

// ReplaceGroupPolicyConfiguration 原子替换一个 admin 分组的策略列表和目标排除列表。分组级
// 配置独立于旧 target 分配表，清空分组配置不会删除任何旧版逐目标分配。
func (r *Repository) ReplaceGroupPolicyConfiguration(ctx context.Context, userID string, adminAccountID string, adminGroupID string, adminGroupName string, policyIDs []string, excludedTargetIDs []string, groupTargetIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := replaceGroupPolicyConfigurationTx(ctx, tx, userID, adminAccountID, adminGroupID, adminGroupName, policyIDs, excludedTargetIDs, groupTargetIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CreatePolicyAndReplaceGroupConfiguration 在一个事务里创建向导策略、模型目标和分组绑定。
// 任意一步失败都会整体回滚，避免线上留下无法从向导继续使用的孤立策略。
func (r *Repository) CreatePolicyAndReplaceGroupConfiguration(ctx context.Context, policy Policy, targets []ModelTarget, adminGroupID string, adminGroupName string, policyIDs []string, excludedTargetIDs []string, groupTargetIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
		INSERT INTO connection_health_policies (
			id, user_id, admin_account_id, name, enabled, own_group_id, own_group_name, model_pattern, probe_mode,
			probe_interval_seconds, failure_threshold, success_threshold, cooldown_seconds, observation_seconds,
			recovery_step_percent, auto_degrade_enabled, auto_remote_action_enabled, priority_mode, daily_probe_budget, created_at, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,now(),now())
	`, policy.ID, policy.UserID, policy.AdminAccountID, policy.Name, policy.Enabled, policy.OwnGroupID, policy.OwnGroupName,
		policy.ModelPattern, policy.ProbeMode, policy.ProbeIntervalSeconds, policy.FailureThreshold, policy.SuccessThreshold,
		policy.CooldownSeconds, policy.ObservationSeconds, policy.RecoveryStepPercent, policy.AutoDegradeEnabled,
		policy.AutoRemoteActionEnabled, normalizePriorityMode(policy.PriorityMode), policy.DailyProbeBudget); err != nil {
		return err
	}
	for _, target := range targets {
		if _, err := tx.Exec(ctx, `
			INSERT INTO connection_health_model_targets
				(id, policy_id, user_id, admin_account_id, model_name, provider_family, enabled, probe_prompt, max_probe_tokens, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),now())
		`, target.ID, target.PolicyID, target.UserID, target.AdminAccountID, target.ModelName, target.ProviderFamily,
			target.Enabled, target.ProbePrompt, target.MaxProbeTokens); err != nil {
			return err
		}
	}
	if err := replaceGroupPolicyConfigurationTx(ctx, tx, policy.UserID, policy.AdminAccountID, adminGroupID, adminGroupName, policyIDs, excludedTargetIDs, groupTargetIDs); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func replaceGroupPolicyConfigurationTx(ctx context.Context, tx pgx.Tx, userID string, adminAccountID string, adminGroupID string, adminGroupName string, policyIDs []string, excludedTargetIDs []string, groupTargetIDs []string) error {

	if _, err := tx.Exec(ctx, `
		DELETE FROM connection_health_group_policy_assignments
		WHERE user_id = $1 AND admin_account_id = $2 AND admin_group_id = $3
	`, userID, adminAccountID, adminGroupID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM connection_health_group_target_exclusions
		WHERE user_id = $1 AND admin_account_id = $2 AND admin_group_id = $3
	`, userID, adminAccountID, adminGroupID); err != nil {
		return err
	}

	for _, policyID := range policyIDs {
		id, err := newID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO connection_health_group_policy_assignments
				(id, user_id, admin_account_id, admin_group_id, admin_group_name, policy_id, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,now(),now())
		`, id, userID, adminAccountID, adminGroupID, adminGroupName, policyID); err != nil {
			return err
		}
	}
	for _, targetID := range excludedTargetIDs {
		id, err := newID()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO connection_health_group_target_exclusions
				(id, user_id, admin_account_id, admin_group_id, target_id, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,now(),now())
		`, id, userID, adminAccountID, adminGroupID, targetID); err != nil {
			return err
		}
	}
	// 管理员重新保存分组策略等价于明确要求系统重新接管。这里只清理该分组目标中已经
	// 标记 conflict 的行；正常接管中的行保留 original_priority，避免丢失恢复基准。
	if len(groupTargetIDs) > 0 {
		if _, err := tx.Exec(ctx, `
			DELETE FROM connection_health_priority_sync_states
			WHERE user_id = $1 AND admin_account_id = $2 AND conflict = true
				AND target_id = ANY($3::text[])
		`, userID, adminAccountID, groupTargetIDs); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			DELETE FROM connection_health_target_action_states
			WHERE user_id = $1 AND admin_account_id = $2 AND conflict = true
				AND target_id = ANY($3::text[])
		`, userID, adminAccountID, groupTargetIDs); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ListGroupPolicyAssignmentsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]GroupPolicyAssignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, admin_group_id, admin_group_name, policy_id, created_at, updated_at
		FROM connection_health_group_policy_assignments
		WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupPolicyAssignments(rows)
}

func (r *Repository) ListAllGroupPolicyAssignments(ctx context.Context) ([]GroupPolicyAssignment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, admin_group_id, admin_group_name, policy_id, created_at, updated_at
		FROM connection_health_group_policy_assignments
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupPolicyAssignments(rows)
}

func scanGroupPolicyAssignments(rows pgx.Rows) ([]GroupPolicyAssignment, error) {
	assignments := make([]GroupPolicyAssignment, 0)
	for rows.Next() {
		var a GroupPolicyAssignment
		if err := rows.Scan(&a.ID, &a.UserID, &a.AdminAccountID, &a.AdminGroupID, &a.AdminGroupName, &a.PolicyID, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		assignments = append(assignments, a)
	}
	return assignments, rows.Err()
}

func (r *Repository) ListGroupTargetExclusionsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]GroupTargetExclusion, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, admin_group_id, target_id, created_at, updated_at
		FROM connection_health_group_target_exclusions
		WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupTargetExclusions(rows)
}

func (r *Repository) ListAllGroupTargetExclusions(ctx context.Context) ([]GroupTargetExclusion, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, admin_group_id, target_id, created_at, updated_at
		FROM connection_health_group_target_exclusions
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanGroupTargetExclusions(rows)
}

func scanGroupTargetExclusions(rows pgx.Rows) ([]GroupTargetExclusion, error) {
	exclusions := make([]GroupTargetExclusion, 0)
	for rows.Next() {
		var exclusion GroupTargetExclusion
		if err := rows.Scan(&exclusion.ID, &exclusion.UserID, &exclusion.AdminAccountID, &exclusion.AdminGroupID, &exclusion.TargetID, &exclusion.CreatedAt, &exclusion.UpdatedAt); err != nil {
			return nil, err
		}
		exclusions = append(exclusions, exclusion)
	}
	return exclusions, rows.Err()
}

func (r *Repository) ListPrioritySyncStates(ctx context.Context, userID string, adminAccountID string) ([]PrioritySyncState, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, admin_account_id, target_id, original_priority, last_applied_priority,
			pending_priority, effective_multiplier, conflict, last_conflict_priority, updated_at
		FROM connection_health_priority_sync_states
		WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make([]PrioritySyncState, 0)
	for rows.Next() {
		var state PrioritySyncState
		if err := rows.Scan(&state.UserID, &state.AdminAccountID, &state.TargetID, &state.OriginalPriority,
			&state.LastAppliedPriority, &state.PendingPriority, &state.EffectiveMultiplier, &state.Conflict, &state.LastConflictPriority, &state.UpdatedAt); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (r *Repository) ListAllPrioritySyncStates(ctx context.Context) ([]PrioritySyncState, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, admin_account_id, target_id, original_priority, last_applied_priority,
			pending_priority, effective_multiplier, conflict, last_conflict_priority, updated_at
		FROM connection_health_priority_sync_states
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make([]PrioritySyncState, 0)
	for rows.Next() {
		var state PrioritySyncState
		if err := rows.Scan(&state.UserID, &state.AdminAccountID, &state.TargetID, &state.OriginalPriority,
			&state.LastAppliedPriority, &state.PendingPriority, &state.EffectiveMultiplier, &state.Conflict, &state.LastConflictPriority, &state.UpdatedAt); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (r *Repository) UpsertPrioritySyncState(ctx context.Context, state PrioritySyncState) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO connection_health_priority_sync_states (
			user_id, admin_account_id, target_id, original_priority, last_applied_priority, pending_priority,
			effective_multiplier, conflict, last_conflict_priority, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())
		ON CONFLICT (user_id, admin_account_id, target_id) DO UPDATE SET
			original_priority = EXCLUDED.original_priority,
			last_applied_priority = EXCLUDED.last_applied_priority,
			pending_priority = EXCLUDED.pending_priority,
			effective_multiplier = EXCLUDED.effective_multiplier,
			conflict = EXCLUDED.conflict,
			last_conflict_priority = EXCLUDED.last_conflict_priority,
			updated_at = now()
	`, state.UserID, state.AdminAccountID, state.TargetID, state.OriginalPriority, state.LastAppliedPriority,
		state.PendingPriority, state.EffectiveMultiplier, state.Conflict, state.LastConflictPriority)
	return err
}

func (r *Repository) DeletePrioritySyncState(ctx context.Context, userID string, adminAccountID string, targetID string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM connection_health_priority_sync_states
		WHERE user_id = $1 AND admin_account_id = $2 AND target_id = $3
	`, userID, adminAccountID, targetID)
	return err
}

func (r *Repository) GetTargetActionState(ctx context.Context, userID string, adminAccountID string, targetID string) (*TargetActionState, error) {
	row := r.db.QueryRow(ctx, `
		SELECT user_id, admin_account_id, target_id, original_status, original_weight,
			last_applied_status, last_applied_weight, pending_status, pending_weight, conflict, updated_at
		FROM connection_health_target_action_states
		WHERE user_id = $1 AND admin_account_id = $2 AND target_id = $3
	`, userID, adminAccountID, targetID)
	var state TargetActionState
	if err := row.Scan(&state.UserID, &state.AdminAccountID, &state.TargetID, &state.OriginalStatus, &state.OriginalWeight,
		&state.LastAppliedStatus, &state.LastAppliedWeight, &state.PendingStatus, &state.PendingWeight, &state.Conflict, &state.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &state, nil
}

func (r *Repository) ListAllTargetActionStates(ctx context.Context) ([]TargetActionState, error) {
	rows, err := r.db.Query(ctx, `
		SELECT user_id, admin_account_id, target_id, original_status, original_weight,
			last_applied_status, last_applied_weight, pending_status, pending_weight, conflict, updated_at
		FROM connection_health_target_action_states
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	states := make([]TargetActionState, 0)
	for rows.Next() {
		var state TargetActionState
		if err := rows.Scan(&state.UserID, &state.AdminAccountID, &state.TargetID, &state.OriginalStatus, &state.OriginalWeight,
			&state.LastAppliedStatus, &state.LastAppliedWeight, &state.PendingStatus, &state.PendingWeight, &state.Conflict, &state.UpdatedAt); err != nil {
			return nil, err
		}
		states = append(states, state)
	}
	return states, rows.Err()
}

func (r *Repository) UpsertTargetActionState(ctx context.Context, state TargetActionState) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO connection_health_target_action_states (
			user_id, admin_account_id, target_id, original_status, original_weight,
			last_applied_status, last_applied_weight, pending_status, pending_weight, conflict, updated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,now())
		ON CONFLICT (user_id, admin_account_id, target_id) DO UPDATE SET
			original_status = EXCLUDED.original_status,
			original_weight = EXCLUDED.original_weight,
			last_applied_status = EXCLUDED.last_applied_status,
			last_applied_weight = EXCLUDED.last_applied_weight,
			pending_status = EXCLUDED.pending_status,
			pending_weight = EXCLUDED.pending_weight,
			conflict = EXCLUDED.conflict,
			updated_at = now()
	`, state.UserID, state.AdminAccountID, state.TargetID, state.OriginalStatus, state.OriginalWeight,
		state.LastAppliedStatus, state.LastAppliedWeight, state.PendingStatus, state.PendingWeight, state.Conflict)
	return err
}

func (r *Repository) DeleteTargetActionState(ctx context.Context, userID string, adminAccountID string, targetID string) error {
	_, err := r.db.Exec(ctx, `
		DELETE FROM connection_health_target_action_states
		WHERE user_id = $1 AND admin_account_id = $2 AND target_id = $3
	`, userID, adminAccountID, targetID)
	return err
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
