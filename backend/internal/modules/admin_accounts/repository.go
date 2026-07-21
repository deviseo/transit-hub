package admin_accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type workspaceDeleteStatement struct {
	Name string
	SQL  string
}

type workspaceTableDescriptor struct {
	Name            string
	WorkspaceColumn string
}

const (
	lockUserForWorkspaceDeleteSQL    = `SELECT current_admin_account_id FROM users WHERE id = $1 FOR UPDATE`
	lockAccountForWorkspaceDeleteSQL = `SELECT id FROM admin_accounts WHERE user_id = $1 AND id = $2 FOR UPDATE`
	nextCurrentWorkspaceIDSQL        = `SELECT id FROM admin_accounts WHERE user_id = $1 AND id <> $2 ORDER BY updated_at DESC, id ASC LIMIT 1`
)

var workspaceDeleteStatements = []workspaceDeleteStatement{
	{Name: "lottery_reward_jobs", SQL: `DELETE FROM lottery_reward_jobs WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "lottery_winners", SQL: `DELETE FROM lottery_winners WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "lottery_draws", SQL: `DELETE FROM lottery_draws WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "lottery_entries", SQL: `DELETE FROM lottery_entries WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "lottery_prizes", SQL: `DELETE FROM lottery_prizes WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "lottery_audit_logs", SQL: `DELETE FROM lottery_audit_logs WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "lottery_campaigns", SQL: `DELETE FROM lottery_campaigns WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "lottery_embed_configs", SQL: `DELETE FROM lottery_embed_configs WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "mass_email_batch_items", SQL: `DELETE FROM mass_email_batch_items WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "mass_email_batches", SQL: `DELETE FROM mass_email_batches WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "group_rate_campaign_items", SQL: `DELETE FROM group_rate_campaign_items WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "group_rate_campaigns", SQL: `DELETE FROM group_rate_campaigns WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_target_action_states", SQL: `DELETE FROM connection_health_target_action_states WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_probe_budget_usage", SQL: `DELETE FROM connection_health_probe_budget_usage WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_priority_sync_states", SQL: `DELETE FROM connection_health_priority_sync_states WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_group_target_exclusions", SQL: `DELETE FROM connection_health_group_target_exclusions WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_group_policy_assignments", SQL: `DELETE FROM connection_health_group_policy_assignments WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_policy_assignments", SQL: `DELETE FROM connection_health_policy_assignments WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_model_targets", SQL: `DELETE FROM connection_health_model_targets WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_states", SQL: `DELETE FROM connection_health_states WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_events", SQL: `DELETE FROM connection_health_events WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "connection_health_policies", SQL: `DELETE FROM connection_health_policies WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "ticket_attachments", SQL: `DELETE FROM ticket_attachments WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "ticket_messages", SQL: `DELETE FROM ticket_messages WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "tickets", SQL: `DELETE FROM tickets WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "ticket_embed_configs", SQL: `DELETE FROM ticket_embed_configs WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "leaderboard_embed_configs", SQL: `DELETE FROM leaderboard_embed_configs WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "group_rate_snapshots", SQL: `DELETE FROM group_rate_snapshots WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "strategy_settings", SQL: `DELETE FROM strategy_settings WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "notification_channel_settings", SQL: `DELETE FROM notification_channel_settings WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "smtp_settings", SQL: `DELETE FROM smtp_settings WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "email_templates", SQL: `DELETE FROM email_templates WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "my_site_states", SQL: `DELETE FROM my_site_states WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "real_connections", SQL: `DELETE FROM real_connections WHERE user_id = $1 AND workspace_admin_account_id = $2`},
	{Name: "dashboard_daily_stats", SQL: `DELETE FROM dashboard_daily_stats WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "dashboard_balance_filter", SQL: `DELETE FROM dashboard_balance_filter WHERE user_id = $1 AND admin_account_id = $2`},
	{Name: "upstream_sites", SQL: `DELETE FROM upstream_sites WHERE user_id = $1 AND admin_account_id = $2`},
}

var legacyWorkspaceTables = []workspaceTableDescriptor{
	{Name: "lottery_embed_configs", WorkspaceColumn: "admin_account_id"},
	{Name: "lottery_campaigns", WorkspaceColumn: "admin_account_id"},
	{Name: "lottery_prizes", WorkspaceColumn: "admin_account_id"},
	{Name: "lottery_entries", WorkspaceColumn: "admin_account_id"},
	{Name: "lottery_draws", WorkspaceColumn: "admin_account_id"},
	{Name: "lottery_winners", WorkspaceColumn: "admin_account_id"},
	{Name: "lottery_reward_jobs", WorkspaceColumn: "admin_account_id"},
	{Name: "lottery_audit_logs", WorkspaceColumn: "admin_account_id"},
	{Name: "upstream_sites", WorkspaceColumn: "admin_account_id"},
	{Name: "group_rate_snapshots", WorkspaceColumn: "admin_account_id"},
	{Name: "strategy_settings", WorkspaceColumn: "admin_account_id"},
	{Name: "notification_channel_settings", WorkspaceColumn: "admin_account_id"},
	{Name: "smtp_settings", WorkspaceColumn: "admin_account_id"},
	{Name: "email_templates", WorkspaceColumn: "admin_account_id"},
	{Name: "my_site_states", WorkspaceColumn: "admin_account_id"},
	{Name: "real_connections", WorkspaceColumn: "workspace_admin_account_id"},
	{Name: "dashboard_daily_stats", WorkspaceColumn: "admin_account_id"},
	{Name: "dashboard_balance_filter", WorkspaceColumn: "admin_account_id"},
	{Name: "mass_email_batches", WorkspaceColumn: "admin_account_id"},
	{Name: "mass_email_batch_items", WorkspaceColumn: "admin_account_id"},
	{Name: "group_rate_campaigns", WorkspaceColumn: "admin_account_id"},
	{Name: "group_rate_campaign_items", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_target_action_states", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_probe_budget_usage", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_priority_sync_states", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_group_target_exclusions", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_group_policy_assignments", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_policies", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_model_targets", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_states", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_events", WorkspaceColumn: "admin_account_id"},
	{Name: "connection_health_policy_assignments", WorkspaceColumn: "admin_account_id"},
	{Name: "ticket_embed_configs", WorkspaceColumn: "admin_account_id"},
	{Name: "leaderboard_embed_configs", WorkspaceColumn: "admin_account_id"},
	{Name: "tickets", WorkspaceColumn: "admin_account_id"},
	{Name: "ticket_messages", WorkspaceColumn: "admin_account_id"},
	{Name: "ticket_attachments", WorkspaceColumn: "admin_account_id"},
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// EnsureSchema 负责 admin_accounts 表和 users 表的工作区字段。
// 各业务模块的 admin_account_id 列由各自的 EnsureSchema 负责添加，
// 此处仅管理 admin_accounts 表本身、users.current_admin_account_id，
// 以及对旧数据的 legacy workspace 创建和归属分配。
//
// 调用前提：所有业务模块的 EnsureSchema 必须先执行完毕，
// 以保证业务表和 admin_account_id 列已存在，legacy 迁移 SQL 不会报错。
func (r *Repository) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS admin_accounts (id text PRIMARY KEY, user_id text NOT NULL, platform text NOT NULL, base_url text NOT NULL, identity text NOT NULL, display_name text NOT NULL, auth_method text NOT NULL, last_used_at timestamptz NULL, created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE (user_id, platform, base_url, identity))`,
		`ALTER TABLE users ADD COLUMN IF NOT EXISTS current_admin_account_id text NOT NULL DEFAULT ''`,
		`CREATE INDEX IF NOT EXISTS idx_admin_accounts_user_updated ON admin_accounts (user_id, updated_at DESC, id ASC)`,
	}
	for _, statement := range statements {
		if _, err := r.db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	if err := r.createLegacyAccounts(ctx); err != nil {
		return err
	}
	return r.assignLegacyRows(ctx)
}

func (r *Repository) AssignLegacyRows(ctx context.Context) error {
	if err := r.createLegacyAccounts(ctx); err != nil {
		return err
	}
	return r.assignLegacyRows(ctx)
}

// createLegacyAccounts 为已有业务数据但尚无 admin account 的用户创建 legacy workspace。
func (r *Repository) createLegacyAccounts(ctx context.Context) error {
	for _, table := range legacyWorkspaceTables {
		if err := r.createLegacyAccountsForTable(ctx, table); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) createLegacyAccountsForTable(ctx context.Context, table workspaceTableDescriptor) error {
	exists, err := r.tableExists(ctx, table.Name)
	if err != nil || !exists {
		return err
	}
	_, err = r.db.Exec(ctx, fmt.Sprintf(`
		WITH scoped_users AS (
			SELECT DISTINCT user_id FROM %s WHERE user_id <> ''
		), legacy AS (
			SELECT users.id AS user_id, 'adminacct_' || encode(sha256(convert_to(users.id || '|legacy||legacy', 'UTF8')), 'hex') AS account_id
			FROM users JOIN scoped_users ON scoped_users.user_id = users.id
		)
		INSERT INTO admin_accounts (id, user_id, platform, base_url, identity, display_name, auth_method, last_used_at, created_at, updated_at)
		SELECT account_id, user_id, 'legacy', '', 'legacy', 'Legacy workspace', 'legacy', now(), now(), now() FROM legacy
		ON CONFLICT (user_id, platform, base_url, identity) DO UPDATE SET updated_at = EXCLUDED.updated_at
	`, table.Name))
	return err
}

// assignLegacyRows 将 admin_account_id 为空的旧业务行归属到用户的第一个工作区（legacy workspace）。
// 不删除旧数据，只补值。
//
// 关键设计：旧数据始终归属到 platform='legacy' 的工作区（即第一个工作区），
// 而非 users.current_admin_account_id。这保证即使用户已有多个 workspace 并
// 切换到了非 legacy 的工作区，历史数据也不会散落到其他 workspace。
func (r *Repository) assignLegacyRows(ctx context.Context) error {
	if _, err := r.db.Exec(ctx, `UPDATE users SET current_admin_account_id = legacy.id FROM admin_accounts AS legacy WHERE users.id = legacy.user_id AND users.current_admin_account_id = '' AND legacy.platform = 'legacy'`); err != nil {
		return err
	}
	for _, table := range legacyWorkspaceTables {
		if err := r.assignLegacyRowsForTable(ctx, table); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) assignLegacyRowsForTable(ctx context.Context, table workspaceTableDescriptor) error {
	exists, err := r.tableExists(ctx, table.Name)
	if err != nil || !exists {
		return err
	}
	_, err = r.db.Exec(ctx, fmt.Sprintf(`UPDATE %s SET %s = legacy.id FROM admin_accounts AS legacy WHERE %s.user_id = legacy.user_id AND %s.%s = '' AND legacy.platform = 'legacy'`, table.Name, table.WorkspaceColumn, table.Name, table.Name, table.WorkspaceColumn))
	return err
}

func (r *Repository) tableExists(ctx context.Context, name string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists)
	return exists, err
}

func (r *Repository) List(ctx context.Context, userID string) ([]Account, error) {
	rows, err := r.db.Query(ctx, `SELECT a.id, a.user_id, a.platform, a.base_url, a.identity, a.display_name, a.auth_method, a.id = users.current_admin_account_id AS current, a.last_used_at, a.created_at, a.updated_at FROM admin_accounts AS a JOIN users ON users.id = a.user_id WHERE a.user_id = $1 ORDER BY current DESC, a.last_used_at DESC NULLS LAST, a.updated_at DESC, a.id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	accounts := make([]Account, 0)
	for rows.Next() {
		var account Account
		if err := rows.Scan(&account.ID, &account.UserID, &account.Platform, &account.BaseURL, &account.Identity, &account.DisplayName, &account.AuthMethod, &account.Current, &account.LastUsedAt, &account.CreatedAt, &account.UpdatedAt); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

func (r *Repository) Current(ctx context.Context, userID string) (*Account, error) {
	row := r.db.QueryRow(ctx, `SELECT a.id, a.user_id, a.platform, a.base_url, a.identity, a.display_name, a.auth_method, true AS current, a.last_used_at, a.created_at, a.updated_at FROM users JOIN admin_accounts AS a ON a.id = users.current_admin_account_id AND a.user_id = users.id WHERE users.id = $1`, userID)
	var account Account
	if err := row.Scan(&account.ID, &account.UserID, &account.Platform, &account.BaseURL, &account.Identity, &account.DisplayName, &account.AuthMethod, &account.Current, &account.LastUsedAt, &account.CreatedAt, &account.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return &account, nil
}

func (r *Repository) CurrentID(ctx context.Context, userID string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT current_admin_account_id FROM users WHERE id = $1 AND current_admin_account_id <> ''`, userID).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

func (r *Repository) UpsertAndSwitch(ctx context.Context, userID string, input UpsertInput) (Account, error) {
	accountID := StableID(userID, input.Platform, input.BaseURL, input.Identity)
	if strings.TrimSpace(input.DisplayName) == "" {
		input.DisplayName = input.Identity
	}
	now := time.Now()
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return Account{}, err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `INSERT INTO admin_accounts (id, user_id, platform, base_url, identity, display_name, auth_method, last_used_at, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8, $8) ON CONFLICT (user_id, platform, base_url, identity) DO UPDATE SET display_name = EXCLUDED.display_name, auth_method = EXCLUDED.auth_method, last_used_at = EXCLUDED.last_used_at, updated_at = EXCLUDED.updated_at`, accountID, userID, input.Platform, input.BaseURL, input.Identity, input.DisplayName, input.AuthMethod, now)
	if err != nil {
		return Account{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE users SET current_admin_account_id = $2 WHERE id = $1`, userID, accountID); err != nil {
		return Account{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Account{}, err
	}
	account, err := r.Current(ctx, userID)
	if err != nil {
		return Account{}, err
	}
	return *account, nil
}

func (r *Repository) Switch(ctx context.Context, userID string, accountID string) (*Account, error) {
	result, err := r.db.Exec(ctx, `UPDATE users SET current_admin_account_id = $2 WHERE id = $1 AND EXISTS (SELECT 1 FROM admin_accounts WHERE id = $2 AND user_id = $1)`, userID, accountID)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, nil
	}
	_, _ = r.db.Exec(ctx, `UPDATE admin_accounts SET last_used_at = now(), updated_at = now() WHERE id = $1 AND user_id = $2`, accountID, userID)
	return r.Current(ctx, userID)
}

func (r *Repository) Update(ctx context.Context, userID string, accountID string, displayName string) (*Account, error) {
	result, err := r.db.Exec(ctx, `UPDATE admin_accounts SET display_name = $3, updated_at = now() WHERE id = $1 AND user_id = $2`, accountID, userID, strings.TrimSpace(displayName))
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, nil
	}
	accounts, err := r.List(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range accounts {
		if accounts[i].ID == accountID {
			return &accounts[i], nil
		}
	}
	return nil, nil
}

func (r *Repository) DeleteWorkspace(ctx context.Context, userID string, accountID string) (*DeleteResult, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return nil, nil
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	currentID, err := r.lockUserForWorkspaceDelete(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	accountFound, err := r.lockAccountForWorkspaceDelete(ctx, tx, userID, accountID)
	if err != nil {
		return nil, err
	}
	if !accountFound {
		return nil, nil
	}

	attachmentPaths, err := collectStrings(ctx, tx, `SELECT storage_path FROM ticket_attachments WHERE user_id = $1 AND admin_account_id = $2 ORDER BY storage_path ASC`, userID, accountID)
	if err != nil {
		return nil, err
	}
	upstreamSiteIDs, err := collectStrings(ctx, tx, `SELECT id FROM upstream_sites WHERE user_id = $1 AND admin_account_id = $2 ORDER BY id ASC`, userID, accountID)
	if err != nil {
		return nil, err
	}
	cleanupJobID, err := randomCleanupJobID()
	if err != nil {
		return nil, err
	}
	if err := insertCleanupJob(ctx, tx, cleanupJobID, userID, accountID, attachmentPaths, upstreamSiteIDs); err != nil {
		return nil, err
	}

	nextCurrentID := currentID
	wasCurrent := currentID == accountID
	if wasCurrent {
		nextCurrentID, err = r.nextCurrentWorkspaceID(ctx, tx, userID, accountID)
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(ctx, `UPDATE users SET current_admin_account_id = $2 WHERE id = $1`, userID, nextCurrentID); err != nil {
			return nil, err
		}
	}

	for _, stmt := range workspaceDeleteStatements {
		if _, err := tx.Exec(ctx, stmt.SQL, userID, accountID); err != nil {
			return nil, err
		}
	}
	result, err := tx.Exec(ctx, `DELETE FROM admin_accounts WHERE user_id = $1 AND id = $2`, userID, accountID)
	if err != nil {
		return nil, err
	}
	if result.RowsAffected() == 0 {
		return nil, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &DeleteResult{
		CleanupJobID:           cleanupJobID,
		DeletedID:              accountID,
		WasCurrent:             wasCurrent,
		CurrentAdminAccountID:  nextCurrentID,
		AttachmentStoragePaths: attachmentPaths,
		UpstreamSiteIDs:        upstreamSiteIDs,
	}, nil
}

func insertCleanupJob(ctx context.Context, tx pgx.Tx, id string, userID string, adminAccountID string, attachmentPaths []string, upstreamSiteIDs []string) error {
	attachmentJSON, err := json.Marshal(attachmentPaths)
	if err != nil {
		return err
	}
	upstreamJSON, err := json.Marshal(upstreamSiteIDs)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO workspace_cleanup_jobs (id, user_id, admin_account_id, attachment_paths, upstream_site_ids, attempts, next_attempt_at, last_error, created_at, updated_at)
		VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, 0, now(), '', now(), now())
	`, id, userID, adminAccountID, string(attachmentJSON), string(upstreamJSON))
	return err
}

func (r *Repository) ClaimDueCleanupJobs(ctx context.Context, limit int) ([]CleanupJob, error) {
	if limit < 1 {
		limit = 1
	}
	rows, err := r.db.Query(ctx, `
		UPDATE workspace_cleanup_jobs
		SET attempts = attempts + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM workspace_cleanup_jobs
			WHERE next_attempt_at <= now()
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, user_id, admin_account_id, attachment_paths, upstream_site_ids, attempts, next_attempt_at, last_error, created_at, updated_at
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	jobs := make([]CleanupJob, 0)
	for rows.Next() {
		job, err := scanCleanupJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (r *Repository) CompleteCleanupJob(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM workspace_cleanup_jobs WHERE id = $1`, id)
	return err
}

func (r *Repository) MarkCleanupJobRetry(ctx context.Context, id string, attempt int, err error) error {
	lastError := boundedCleanupError(err)
	delay := cleanupRetryDelay(attempt)
	_, execErr := r.db.Exec(ctx, `UPDATE workspace_cleanup_jobs SET last_error = $2, next_attempt_at = now() + ($3 * interval '1 second'), updated_at = now() WHERE id = $1`, id, lastError, int(delay.Seconds()))
	return execErr
}

func scanCleanupJob(row pgx.Row) (CleanupJob, error) {
	var job CleanupJob
	var attachmentJSON, upstreamJSON []byte
	err := row.Scan(&job.ID, &job.UserID, &job.AdminAccountID, &attachmentJSON, &upstreamJSON, &job.Attempts, &job.NextAttemptAt, &job.LastError, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return CleanupJob{}, err
	}
	_ = json.Unmarshal(attachmentJSON, &job.AttachmentStoragePaths)
	_ = json.Unmarshal(upstreamJSON, &job.UpstreamSiteIDs)
	return job, nil
}

func boundedCleanupError(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 500 {
		message = message[:500]
	}
	return message
}

func cleanupRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Duration(attempt) * time.Minute
	if delay > 30*time.Minute {
		return 30 * time.Minute
	}
	return delay
}

func randomCleanupJobID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "wscleanup_" + hex.EncodeToString(buf), nil
}

func (r *Repository) lockUserForWorkspaceDelete(ctx context.Context, tx pgx.Tx, userID string) (string, error) {
	var currentID string
	err := tx.QueryRow(ctx, lockUserForWorkspaceDeleteSQL, userID).Scan(&currentID)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return currentID, err
}

func (r *Repository) lockAccountForWorkspaceDelete(ctx context.Context, tx pgx.Tx, userID string, accountID string) (bool, error) {
	var id string
	err := tx.QueryRow(ctx, lockAccountForWorkspaceDeleteSQL, userID, accountID).Scan(&id)
	if err == pgx.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func (r *Repository) nextCurrentWorkspaceID(ctx context.Context, tx pgx.Tx, userID string, deletedAccountID string) (string, error) {
	var id string
	err := tx.QueryRow(ctx, nextCurrentWorkspaceIDSQL, userID, deletedAccountID).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

func collectStrings(ctx context.Context, tx pgx.Tx, sql string, args ...any) ([]string, error) {
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]string, 0)
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if strings.TrimSpace(value) != "" {
			values = append(values, value)
		}
	}
	return values, rows.Err()
}

func StableID(userID, platform, baseURL, identity string) string {
	parts := strings.Join([]string{strings.TrimSpace(userID), strings.TrimSpace(platform), strings.TrimSpace(baseURL), strings.TrimSpace(identity)}, "|")
	sum := sha256.Sum256([]byte(parts))
	return "adminacct_" + hex.EncodeToString(sum[:])
}
