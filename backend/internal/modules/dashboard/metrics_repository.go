package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MetricsRepository 负责 dashboard_daily_stats 表的持久化操作。
// 与 Redis 的 SessionStore 独立，专门用于存储每日统计快照。
type MetricsRepository struct {
	db *pgxpool.Pool
}

func NewMetricsRepository(db *pgxpool.Pool) *MetricsRepository {
	return &MetricsRepository{db: db}
}

// EnsureSchema 在服务启动时创建 dashboard_daily_stats 和 dashboard_balance_filter 表及索引，
// 并将旧数据迁移到 workspace 维度。
//
// dashboard_daily_stats: (user_id, admin_account_id, date) 唯一索引保证每工作区每天至多一行。
// dashboard_balance_filter: (user_id, admin_account_id) 唯一索引保证每工作区至多一行配置。
//
// 迁移策略：
//  1. 新增 admin_account_id 列（DEFAULT ” 兼容旧行）。
//  2. 删除旧的单维度唯一索引/约束，创建新的多维度唯一索引。
//  3. 旧数据 admin_account_id=” 保留原样，由 admin_accounts 统一归属迁移负责补值。
func (r *MetricsRepository) EnsureSchema(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS dashboard_daily_stats (
			id               text PRIMARY KEY,
			user_id          text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			date             date NOT NULL,
			today_profit     double precision NOT NULL DEFAULT 0,
			site_balance     double precision NOT NULL DEFAULT 0,
			today_purchase   double precision NOT NULL DEFAULT 0,
			net_profit       double precision NOT NULL DEFAULT 0,
			upstream_balance double precision NOT NULL DEFAULT 0,
			created_at       timestamptz NOT NULL DEFAULT now()
		);

		-- 新增 admin_account_id 列（旧表迁移，IF NOT EXISTS 语义通过 DO NOTHING 实现）。
		DO $$ BEGIN
			ALTER TABLE dashboard_daily_stats ADD COLUMN admin_account_id text NOT NULL DEFAULT '';
		EXCEPTION WHEN duplicate_column THEN NULL;
		END $$;

		-- 删除旧的 (user_id, date) 唯一索引，避免与新索引冲突。
		DROP INDEX IF EXISTS idx_dashboard_daily_stats_user_date;

		-- 创建新的 (user_id, admin_account_id, date) 唯一索引。
		CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_daily_stats_user_account_date
			ON dashboard_daily_stats (user_id, admin_account_id, date);
		CREATE INDEX IF NOT EXISTS idx_dashboard_daily_stats_user_date_desc
			ON dashboard_daily_stats (user_id, admin_account_id, date DESC);

		CREATE TABLE IF NOT EXISTS dashboard_balance_filter (
			user_id          text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			exclude_admin    boolean NOT NULL DEFAULT true,
			exclude_balances jsonb NOT NULL DEFAULT '[]',
			updated_at       timestamptz NOT NULL DEFAULT now()
		);

		-- 新增 admin_account_id 列（旧表迁移）。
		DO $$ BEGIN
			ALTER TABLE dashboard_balance_filter ADD COLUMN admin_account_id text NOT NULL DEFAULT '';
		EXCEPTION WHEN duplicate_column THEN NULL;
		END $$;

		-- 删除旧的 user_id 主键约束，改为复合唯一索引。
		-- 旧表可能用 user_id 做主键或唯一约束，需要先去除。
		ALTER TABLE dashboard_balance_filter DROP CONSTRAINT IF EXISTS dashboard_balance_filter_pkey;
		CREATE UNIQUE INDEX IF NOT EXISTS idx_dashboard_balance_filter_user_account
			ON dashboard_balance_filter (user_id, admin_account_id);
	`)
	return err
}

// Upsert 插入或更新指定用户指定工作区指定日期的快照行。
// 冲突时用最新的指标值覆盖旧值，保证一天内多次调用始终保留最新数据。
func (r *MetricsRepository) Upsert(ctx context.Context, snapshot DailySnapshot) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO dashboard_daily_stats (id, user_id, admin_account_id, date, today_profit, site_balance, today_purchase, net_profit, upstream_balance, created_at)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10
		WHERE EXISTS (SELECT 1 FROM admin_accounts WHERE user_id = $2 AND id = $3)
		ON CONFLICT (user_id, admin_account_id, date) DO UPDATE SET
			today_profit     = EXCLUDED.today_profit,
			site_balance     = EXCLUDED.site_balance,
			today_purchase   = EXCLUDED.today_purchase,
			net_profit       = EXCLUDED.net_profit,
			upstream_balance = EXCLUDED.upstream_balance,
			created_at       = EXCLUDED.created_at
		WHERE EXISTS (SELECT 1 FROM admin_accounts WHERE user_id = EXCLUDED.user_id AND id = EXCLUDED.admin_account_id)
	`, snapshot.ID, snapshot.UserID, snapshot.AdminAccountID, snapshot.Date, snapshot.TodayProfit, snapshot.SiteBalance,
		snapshot.TodayPurchase, snapshot.NetProfit, snapshot.UpstreamBalance, snapshot.CreatedAt)
	return err
}

// ListRange 查询指定用户指定工作区最近 days 天的快照记录，按日期升序返回。
// 不包含当天（当天的数据由 LiveMetrics 实时提供），仅返回已保存的历史日期。
func (r *MetricsRepository) ListRange(ctx context.Context, userID, adminAccountID string, days int) ([]DailySnapshot, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, date, today_profit, site_balance, today_purchase, net_profit, upstream_balance, created_at
		FROM dashboard_daily_stats
		WHERE user_id = $1 AND admin_account_id = $2 AND date >= (CURRENT_DATE - $3::int) AND date < CURRENT_DATE
		ORDER BY date ASC
	`, userID, adminAccountID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	snapshots := make([]DailySnapshot, 0)
	for rows.Next() {
		var s DailySnapshot
		if err := rows.Scan(&s.ID, &s.UserID, &s.AdminAccountID, &s.Date, &s.TodayProfit, &s.SiteBalance,
			&s.TodayPurchase, &s.NetProfit, &s.UpstreamBalance, &s.CreatedAt); err != nil {
			return nil, err
		}
		snapshots = append(snapshots, s)
	}
	return snapshots, rows.Err()
}

// Exists 检查指定用户指定工作区指定日期是否已有快照行。
func (r *MetricsRepository) Exists(ctx context.Context, userID, adminAccountID string, date time.Time) (bool, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM dashboard_daily_stats WHERE user_id = $1 AND admin_account_id = $2 AND date = $3
	`, userID, adminAccountID, date).Scan(&count)
	return count > 0, err
}

// GetBalanceFilter 读取指定用户指定工作区的余额筛选配置。
// 若用户尚未配置，返回默认值（排除 admin、不排除任何余额值）。
func (r *MetricsRepository) GetBalanceFilter(ctx context.Context, userID, adminAccountID string) (BalanceFilterConfig, error) {
	config := BalanceFilterConfig{
		UserID:          userID,
		AdminAccountID:  adminAccountID,
		ExcludeAdmin:    true,
		ExcludeBalances: []float64{},
	}
	var balancesJSON []byte
	err := r.db.QueryRow(ctx, `
		SELECT exclude_admin, exclude_balances FROM dashboard_balance_filter WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID).Scan(&config.ExcludeAdmin, &balancesJSON)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return config, nil
		}
		return config, err
	}
	if len(balancesJSON) > 0 {
		if err := json.Unmarshal(balancesJSON, &config.ExcludeBalances); err != nil {
			return config, err
		}
	}
	return config, nil
}

// SaveBalanceFilter 保存或更新指定用户指定工作区的余额筛选配置。
// 使用 upsert 确保幂等写入，用户首次配置和后续修改都走同一路径。
func (r *MetricsRepository) SaveBalanceFilter(ctx context.Context, config BalanceFilterConfig) error {
	balancesJSON, err := json.Marshal(config.ExcludeBalances)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO dashboard_balance_filter (user_id, admin_account_id, exclude_admin, exclude_balances, updated_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET
			exclude_admin    = EXCLUDED.exclude_admin,
			exclude_balances = EXCLUDED.exclude_balances,
			updated_at       = now()
	`, config.UserID, config.AdminAccountID, config.ExcludeAdmin, balancesJSON)
	return err
}
