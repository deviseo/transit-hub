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

		-- finalized_at：零点后按日期直接向各站点查询全天营收 / 成本写入（定稿）的时间；为空表示还是白天某次打开仪表盘时的数。
		ALTER TABLE dashboard_daily_stats ADD COLUMN IF NOT EXISTS finalized_at timestamptz;

		-- 每个工作区「我的站点」售价：站点余额 1 个单位（通常是 1 美元）卖多少元，营收与站点用户余额按此换算成人民币。
		CREATE TABLE IF NOT EXISTS dashboard_pricing (
			user_id          text NOT NULL,
			admin_account_id text NOT NULL,
			site_unit_price  double precision NOT NULL DEFAULT 1,
			updated_at       timestamptz NOT NULL DEFAULT now(),
			PRIMARY KEY (user_id, admin_account_id)
		);
	`)
	return err
}

// DefaultSiteUnitPrice 是未配置时的售价：按 1 个站点余额单位 = 1 元换算，与之前直接显示站点金额的口径一致。
const DefaultSiteUnitPrice = 1.0

// GetPricing 读取工作区的售价配置，未配置时返回默认值。
func (r *MetricsRepository) GetPricing(ctx context.Context, userID, adminAccountID string) (PricingConfig, error) {
	config := PricingConfig{UserID: userID, AdminAccountID: adminAccountID, SiteUnitPrice: DefaultSiteUnitPrice}
	err := r.db.QueryRow(ctx, `SELECT site_unit_price FROM dashboard_pricing WHERE user_id = $1 AND admin_account_id = $2`,
		userID, adminAccountID).Scan(&config.SiteUnitPrice)
	if errors.Is(err, pgx.ErrNoRows) {
		return config, nil
	}
	return config, err
}

// SavePricing 保存工作区的售价配置。
func (r *MetricsRepository) SavePricing(ctx context.Context, config PricingConfig) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO dashboard_pricing (user_id, admin_account_id, site_unit_price, updated_at)
		SELECT $1, $2, $3, now()
		WHERE EXISTS (SELECT 1 FROM admin_accounts WHERE user_id = $1 AND id = $2)
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET site_unit_price = EXCLUDED.site_unit_price, updated_at = now()
	`, config.UserID, config.AdminAccountID, config.SiteUnitPrice)
	return err
}

// FinalizeSnapshot 用按日期查询到的全天数据写入（覆盖）某天的快照并标记定稿。
// keepBalances 为 true 时保留已有行中的余额（定稿发生在事后很久，此刻的余额已不是那天结束时的值）。
func (r *MetricsRepository) FinalizeSnapshot(ctx context.Context, snapshot DailySnapshot, keepBalances bool) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO dashboard_daily_stats (id, user_id, admin_account_id, date, today_profit, site_balance, today_purchase, net_profit, upstream_balance, created_at, finalized_at)
		SELECT $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10
		WHERE EXISTS (SELECT 1 FROM admin_accounts WHERE user_id = $2 AND id = $3)
		ON CONFLICT (user_id, admin_account_id, date) DO UPDATE SET
			today_profit     = EXCLUDED.today_profit,
			today_purchase   = EXCLUDED.today_purchase,
			net_profit       = EXCLUDED.net_profit,
			site_balance     = CASE WHEN $11 THEN dashboard_daily_stats.site_balance ELSE EXCLUDED.site_balance END,
			upstream_balance = CASE WHEN $11 THEN dashboard_daily_stats.upstream_balance ELSE EXCLUDED.upstream_balance END,
			created_at       = EXCLUDED.created_at,
			finalized_at     = EXCLUDED.finalized_at
	`, snapshot.ID, snapshot.UserID, snapshot.AdminAccountID, snapshot.Date, snapshot.TodayProfit, snapshot.SiteBalance,
		snapshot.TodayPurchase, snapshot.NetProfit, snapshot.UpstreamBalance, snapshot.CreatedAt, keepBalances)
	return err
}

// UnfinalizedDates 返回 [from, to] 内已有快照但还没有定稿的日期（升序）。
func (r *MetricsRepository) UnfinalizedDates(ctx context.Context, userID, adminAccountID string, from time.Time, to time.Time) ([]time.Time, error) {
	rows, err := r.db.Query(ctx, `
		SELECT date FROM dashboard_daily_stats
		WHERE user_id = $1 AND admin_account_id = $2 AND date BETWEEN $3 AND $4 AND finalized_at IS NULL
		ORDER BY date ASC
	`, userID, adminAccountID, calendarDate(from), calendarDate(to))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	dates := make([]time.Time, 0)
	for rows.Next() {
		var date time.Time
		if err := rows.Scan(&date); err != nil {
			return nil, err
		}
		dates = append(dates, date)
	}
	return dates, rows.Err()
}

// IsFinalized 判断某天的快照是否已定稿。
func (r *MetricsRepository) IsFinalized(ctx context.Context, userID, adminAccountID string, date time.Time) (bool, error) {
	var finalized bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM dashboard_daily_stats WHERE user_id = $1 AND admin_account_id = $2 AND date = $3 AND finalized_at IS NOT NULL)
	`, userID, adminAccountID, calendarDate(date)).Scan(&finalized)
	return finalized, err
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
			-- 已定稿的日期不再被白天的实时数覆盖（例如零点前开始、零点后才写入的请求）。
			AND dashboard_daily_stats.finalized_at IS NULL
	`, snapshot.ID, snapshot.UserID, snapshot.AdminAccountID, snapshot.Date, snapshot.TodayProfit, snapshot.SiteBalance,
		snapshot.TodayPurchase, snapshot.NetProfit, snapshot.UpstreamBalance, snapshot.CreatedAt)
	return err
}

// ListRange 查询指定用户指定工作区截至 today（不含）之前 days 天的快照记录，按日期升序返回。
// 不包含当天（当天的数据由 LiveMetrics 实时提供），仅返回已保存的历史日期。
// today 由调用方按服务进程本地时区（TZ）计算并传入，不使用数据库会话时区的 CURRENT_DATE：
// 两者不一致时（例如容器为北京时间而 PostgreSQL 会话为 UTC），北京时间 0–8 点会漏掉昨天的快照。
func (r *MetricsRepository) ListRange(ctx context.Context, userID, adminAccountID string, today time.Time, days int) ([]DailySnapshot, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, user_id, admin_account_id, date, today_profit, site_balance, today_purchase, net_profit, upstream_balance, created_at
		FROM dashboard_daily_stats
		WHERE user_id = $1 AND admin_account_id = $2 AND date >= ($3::date - $4::int) AND date < $3::date
		ORDER BY date ASC
	`, userID, adminAccountID, calendarDate(today), days)
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

// calendarDate 取 t 在其自身时区下的日历日期，并归一化为 UTC 零点，作为 PostgreSQL date 参数传入。
// 直接使用 t.UTC() 会在北京时间 0–8 点把日期算成前一天。
func calendarDate(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
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
