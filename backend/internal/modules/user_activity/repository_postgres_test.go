package user_activity

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgreSQL 仓储测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中只写入合成数据，结束后整体删除。

const postgresTestTimeout = 20 * time.Second

func openPostgresTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("user_activity_test_%d", time.Now().UnixNano())
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		adminPool.Close()
		t.Fatalf("create test schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		adminPool.Close()
		t.Fatalf("parse PostgreSQL config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		adminPool.Close()
		t.Fatalf("connect test schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		dropCtx, dropCancel := context.WithTimeout(context.Background(), postgresTestTimeout)
		defer dropCancel()
		if _, err := adminPool.Exec(dropCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop test schema: %v", err)
		}
		adminPool.Close()
	})
	return pool
}

func applyMigration(t *testing.T, pool *pgxpool.Pool, name string) {
	t.Helper()
	sql, err := os.ReadFile(filepath.Join("..", "..", "database", "migrations", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		t.Fatalf("apply migration %s: %v", name, err)
	}
}

func timePtr(value time.Time) *time.Time { return &value }

func siteIDs(rows []UserRow) string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.SiteUserID)
	}
	return strings.Join(ids, ",")
}

func TestRepositoryPostgresSnapshotAndAnalytics(t *testing.T) {
	pool := openPostgresTestPool(t)
	applyMigration(t, pool, "000020_create_user_activity.sql")
	applyMigration(t, pool, "000023_add_sync_started_by.sql")
	repo := NewRepository(pool)
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()

	loc, _ := time.LoadLocation(BusinessTimezone)
	ws := testWorkspace
	other := Workspace{UserID: "user-1", AdminAccountID: "account-2"}
	now := testNow
	today := day("2026-11-02")
	yesterday := today.AddDate(0, 0, -1)
	aliceUsed := now.Add(-2 * time.Hour)
	bobUsed := now.AddDate(0, 0, -10)
	aliceCreated := time.Date(2026, 11, 1, 9, 0, 0, 0, loc)

	profiles := []Profile{
		{SiteUserID: "1", Email: "a@example.test", Username: "alice", Role: RoleUser, Status: StatusActive, Balance: floatPtr(5), EverUsed: true, SiteCreatedAt: &aliceCreated, LastUsedAt: &aliceUsed},
		{SiteUserID: "2", Email: "b@example.test", Username: "bob_x", Role: RoleAdmin, Status: StatusActive, TotalRecharged: floatPtr(30), EverUsed: true, LastUsedAt: &bobUsed},
		{SiteUserID: "3", Email: "", Username: "carol", Role: RoleUser, Status: StatusActive},
		{SiteUserID: "4", Email: "d@example.test", Username: "dave", Role: RoleUser, Status: StatusDisabled, EverUsed: true},
		{SiteUserID: "5", Email: "e@example.test", Username: "eve", Role: RoleUser, Status: StatusActive, EverUsed: true},
	}
	daily := []DailyUsage{
		{SiteUserID: "1", Day: today, Requests: 2, Tokens: 20, Cost: 0.2},
		{SiteUserID: "1", Day: yesterday, Requests: 3, Tokens: 30, Cost: 0.3},
		{SiteUserID: "1", Day: day("2026-10-28"), Requests: 1, Tokens: 10, Cost: 1.0},
		{SiteUserID: "1", Day: day("2026-10-20"), Requests: 1, Tokens: 10, Cost: 4.0},
		{SiteUserID: "2", Day: day("2026-10-23"), Requests: 5, Tokens: 50, Cost: 2.0},
		{SiteUserID: unmatchedUserPrefix + "ghost", Day: yesterday, Requests: 1, Tokens: 5, Cost: 9.0},
		{SiteUserID: "1", Day: day("2025-09-01"), Requests: 1, Tokens: 1, Cost: 1},
	}
	if err := repo.ReplaceSnapshot(ctx, ws, Snapshot{Profiles: profiles, Days: dayList(day("2026-10-01"), today), Daily: daily, SyncStamp: now, PruneMissingProfiles: true, RetainFrom: today.AddDate(0, 0, -(retentionDays - 1))}); err != nil {
		t.Fatalf("replace snapshot: %v", err)
	}
	if err := repo.ReplaceSnapshot(ctx, other, Snapshot{Profiles: profiles[:1], Days: []time.Time{yesterday}, Daily: []DailyUsage{{SiteUserID: "1", Day: yesterday, Requests: 100, Cost: 100}}, SyncStamp: now}); err != nil {
		t.Fatalf("replace other snapshot: %v", err)
	}
	var retainedOld int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM user_activity_daily WHERE day < $1`, today.AddDate(0, 0, -(retentionDays-1))).Scan(&retainedOld); err != nil || retainedOld != 0 {
		t.Fatalf("usage older than the retention window must be deleted, got %d %v", retainedOld, err)
	}

	counts, err := repo.ProfileCounts(ctx, ws, now, inactiveBucketDays)
	if err != nil {
		t.Fatalf("profile counts: %v", err)
	}
	if counts.Total != 5 || counts.Disabled != 1 || counts.WithEmail != 4 || counts.NoUsage != 1 {
		t.Fatalf("unexpected counts: %+v", counts)
	}
	gotBuckets := []string{}
	for _, bucket := range counts.Inactive {
		gotBuckets = append(gotBuckets, fmt.Sprintf("%d:%d", bucket.Days, bucket.Count))
	}
	if strings.Join(gotBuckets, ",") != "2:2,7:2,14:1,30:1,60:1,90:1" {
		t.Fatalf("inactive buckets (excluding disabled users) = %v", gotBuckets)
	}

	metrics, err := repo.PeriodMetrics(ctx, ws, "", yesterday, yesterday, loc)
	if err != nil {
		t.Fatalf("period metrics: %v", err)
	}
	if metrics.ActiveUsers != 2 || metrics.Requests != 4 || metrics.Cost != 9.3 || metrics.NewUsers != 1 || metrics.From != "2026-11-01" {
		t.Fatalf("unexpected period metrics (other workspace must not leak): %+v", metrics)
	}
	single, err := repo.PeriodMetrics(ctx, ws, "1", yesterday, yesterday, loc)
	if err != nil || single.ActiveUsers != 1 || single.Requests != 3 || single.NewUsers != 0 {
		t.Fatalf("unexpected single-user metrics: %+v %v", single, err)
	}
	retention, err := repo.PeriodRetention(ctx, ws, [2]time.Time{day("2026-10-26"), yesterday}, [2]time.Time{day("2026-10-19"), day("2026-10-25")})
	if err != nil || retention != (Retention{Retained: 1, Churned: 1, Gained: 1}) {
		t.Fatalf("unexpected retention: %+v %v", retention, err)
	}
	series, err := repo.DailySeries(ctx, ws, "", day("2026-10-31"), today)
	if err != nil || len(series) != 3 || series[0].Requests != 0 || series[1].ActiveUsers != 2 || series[1].Requests != 4 || series[2].Day != "2026-11-02" || series[2].Requests != 2 {
		t.Fatalf("unexpected daily series: %+v %v", series, err)
	}

	windows := NewWindows(now, today)
	list := func(filter UserFilter, sortKey string, order string, limit int, offset int) ([]UserRow, int) {
		t.Helper()
		rows, total, err := repo.ListUsers(ctx, ws, windows, filter, sortKey, order, limit, offset)
		if err != nil {
			t.Fatalf("list users %+v: %v", filter, err)
		}
		return rows, total
	}
	rows, total := list(UserFilter{}, "", "", 20, 0)
	if total != 5 || siteIDs(rows) != "1,2,3,4,5" {
		t.Fatalf("default order must be last used desc with nulls last: %s total=%d", siteIDs(rows), total)
	}
	alice := rows[0]
	if alice.RequestsToday != 2 || alice.CostToday != 0.2 || alice.Requests7d != 4 || alice.Cost7d != 1.3 || alice.CostPrev7d != 4 ||
		alice.Cost30d != 5.3 || alice.ActiveDays30d != 3 || alice.DaysSinceLastUse == nil || *alice.DaysSinceLastUse != 0 || *alice.Balance != 5 {
		t.Fatalf("unexpected alice row: %+v", alice)
	}
	if rows[1].TotalRecharged == nil || *rows[1].TotalRecharged != 30 || rows[1].RequestsPrev7d != 5 {
		t.Fatalf("unexpected bob row: %+v", rows[1])
	}
	cases := []struct {
		filter UserFilter
		want   string
	}{
		{UserFilter{Segment: SegmentInactive7d}, "2,4,5"},
		{UserFilter{Segment: SegmentInactive7d, Status: StatusActive}, "2,5"},
		{UserFilter{Segment: SegmentInactive14d, Status: StatusActive}, "5"},
		{UserFilter{Segment: SegmentInactive, InactiveDays: 1}, "2,4,5"},
		{UserFilter{Segment: SegmentNoUsage}, "3"},
		{UserFilter{Segment: SegmentChurned7d}, "2"},
		{UserFilter{Segment: SegmentDeclining}, "1,2"},
		{UserFilter{Segment: SegmentGrowing}, ""},
		{UserFilter{Segment: SegmentActive1d}, "1"},
		{UserFilter{Segment: SegmentActive30d}, "1,2"},
		{UserFilter{Search: "bob_"}, "2"},
		{UserFilter{Search: "b_b"}, ""},
		{UserFilter{Search: "3"}, "3"},
		{UserFilter{Search: "A@EXAMPLE"}, "1"},
		{UserFilter{Role: RoleAdmin}, "2"},
		{UserFilter{HasEmail: true, Status: StatusActive}, "1,2,5"},
	}
	for _, tc := range cases {
		if got, _ := list(tc.filter, "", "", 20, 0); siteIDs(got) != tc.want {
			t.Fatalf("filter %+v = %q, want %q", tc.filter, siteIDs(got), tc.want)
		}
	}
	if got, _ := list(UserFilter{}, SortChange7d, "asc", 20, 0); siteIDs(got) != "2,1,3,4,5" {
		t.Fatalf("change sort must order by ratio with nulls last, got %s", siteIDs(got))
	}
	if got, _ := list(UserFilter{}, SortTotalRecharged, "desc", 20, 0); got[0].SiteUserID != "2" {
		t.Fatalf("total recharged sort failed: %s", siteIDs(got))
	}
	if got, total := list(UserFilter{}, "", "", 2, 4); siteIDs(got) != "5" || total != 5 {
		t.Fatalf("last page = %s total=%d", siteIDs(got), total)
	}
	if got, total := list(UserFilter{}, "", "", 2, 10); len(got) != 0 || total != 5 {
		t.Fatalf("out-of-range page must still report the total, got %d rows total=%d", len(got), total)
	}
	if got, _ := list(UserFilter{SiteUserID: "4"}, "", "", 1, 0); siteIDs(got) != "4" {
		t.Fatalf("exact id lookup failed: %s", siteIDs(got))
	}

	if _, err := pool.Exec(ctx, `INSERT INTO email_suppressions (user_id, admin_account_id, email_normalized) VALUES ($1, $2, 'e@example.test'), ($1, 'account-2', 'b@example.test')`, ws.UserID, ws.AdminAccountID); err != nil {
		t.Fatalf("insert suppression: %v", err)
	}
	preview, err := repo.RecipientPreview(ctx, ws, windows, UserFilter{Segment: SegmentInactive7d})
	if err != nil || preview != (RecipientPreview{Matched: 3, WithEmail: 3, Suppressed: 1, Sendable: 2}) {
		t.Fatalf("unexpected recipient preview: %+v %v", preview, err)
	}
	if count, err := repo.SuppressionCount(ctx, ws); err != nil || count != 1 {
		t.Fatalf("suppression count = %d %v", count, err)
	}

	// 再次同步：last_used_at 只会往后推、ever_used 不会被清掉、缺失的注册时间保留旧值；站点上已删除的用户被清理。
	later := now.Add(time.Hour)
	older := aliceUsed.Add(-time.Hour)
	if err := repo.ReplaceSnapshot(ctx, ws, Snapshot{
		Profiles: []Profile{
			{SiteUserID: "1", Email: "a2@example.test", Username: "alice", Role: RoleUser, Status: StatusActive, LastUsedAt: &older},
			{SiteUserID: "2", Email: "b@example.test", Username: "bob_x", Role: RoleAdmin, Status: StatusActive},
			{SiteUserID: "3", Email: "", Username: "carol", Role: RoleUser, Status: StatusActive},
		},
		LastUsed:             map[string]time.Time{"3": later.Add(-30 * time.Minute)},
		Days:                 []time.Time{yesterday},
		Daily:                []DailyUsage{{SiteUserID: "1", Day: yesterday, Requests: 7, Cost: 0.7}},
		SyncStamp:            later,
		PruneMissingProfiles: true,
	}); err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	rows, total = list(UserFilter{}, "", "", 20, 0)
	if total != 3 || siteIDs(rows) != "3,1,2" {
		t.Fatalf("profiles missing from the site must be pruned and carol's derived last use applied: %s total=%d", siteIDs(rows), total)
	}
	carol, alice2, bob := rows[0], rows[1], rows[2]
	if !carol.EverUsed || carol.LastUsedAt == nil {
		t.Fatalf("derived last use must mark the user as used: %+v", carol)
	}
	if alice2.Email != "a2@example.test" || !alice2.LastUsedAt.Equal(aliceUsed) || !alice2.EverUsed || alice2.CreatedAt == nil || !alice2.CreatedAt.Equal(aliceCreated) || alice2.Balance != nil {
		t.Fatalf("unexpected merged alice profile: %+v", alice2)
	}
	if !bob.EverUsed || bob.LastUsedAt == nil || !bob.LastUsedAt.Equal(bobUsed) {
		t.Fatalf("bob must keep his history: %+v", bob)
	}
	yesterdayMetrics, err := repo.PeriodMetrics(ctx, ws, "", yesterday, yesterday, loc)
	if err != nil || yesterdayMetrics.Requests != 7 || yesterdayMetrics.ActiveUsers != 1 {
		t.Fatalf("replaced day must only contain the new rows: %+v %v", yesterdayMetrics, err)
	}
	if metrics, err := repo.PeriodMetrics(ctx, ws, "", day("2026-10-20"), day("2026-10-28"), loc); err != nil || metrics.Requests != 7 {
		t.Fatalf("days that were not re-fetched must be kept: %+v %v", metrics, err)
	}
	if otherMetrics, err := repo.PeriodMetrics(ctx, other, "", yesterday, yesterday, loc); err != nil || otherMetrics.Requests != 100 {
		t.Fatalf("other workspace must be untouched: %+v %v", otherMetrics, err)
	}
}

func TestRepositoryPostgresSyncState(t *testing.T) {
	pool := openPostgresTestPool(t)
	applyMigration(t, pool, "000020_create_user_activity.sql")
	applyMigration(t, pool, "000023_add_sync_started_by.sql")
	repo := NewRepository(pool)
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()

	if _, ok, err := repo.GetSyncState(ctx, testWorkspace); err != nil || ok {
		t.Fatalf("missing state must report ok=false: %v %v", ok, err)
	}
	from, to := day("2026-09-01"), day("2026-11-02")
	state := SyncState{Platform: "sub2api", Status: SyncStatusRunning, SyncedFrom: &from, SyncedTo: &to, UserCount: 3, LastStartedAt: timePtr(testNow), StartedBy: StartedByAuto}
	if err := repo.SaveSyncState(ctx, testWorkspace, state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	if err := repo.ResetRunningSyncs(ctx, ErrorInterrupted); err != nil {
		t.Fatalf("reset running: %v", err)
	}
	got, ok, err := repo.GetSyncState(ctx, testWorkspace)
	if err != nil || !ok || got.Status != SyncStatusFailed || got.ErrorKey != ErrorInterrupted || formatDay(*got.SyncedFrom) != "2026-09-01" || formatDay(*got.SyncedTo) != "2026-11-02" || got.UserCount != 3 || got.StartedBy != StartedByAuto {
		t.Fatalf("unexpected state after reset: %+v %v %v", got, ok, err)
	}
	workspaces, err := repo.ListSyncedWorkspaces(ctx)
	if err != nil || len(workspaces) != 1 || workspaces[0] != testWorkspace {
		t.Fatalf("unexpected workspaces: %+v %v", workspaces, err)
	}
}

func TestRepositoryPostgresRechargeAndBalance(t *testing.T) {
	pool := openPostgresTestPool(t)
	applyMigration(t, pool, "000020_create_user_activity.sql")
	applyMigration(t, pool, "000023_add_sync_started_by.sql")
	repo := NewRepository(pool)
	ctx, cancel := context.WithTimeout(context.Background(), postgresTestTimeout)
	defer cancel()

	ws := testWorkspace
	now := testNow
	today := day("2026-11-02")
	yesterday := today.AddDate(0, 0, -1)
	recent := now.Add(-time.Hour)
	profiles := func(fundedAlice float64, fundedBob float64) []Profile {
		return []Profile{
			{SiteUserID: "1", Email: "a@example.test", Status: StatusActive, Balance: floatPtr(0.5), FundedTotal: &fundedAlice, EverUsed: true, LastUsedAt: &recent},
			{SiteUserID: "2", Email: "b@example.test", Status: StatusActive, Balance: floatPtr(20), TotalRecharged: floatPtr(50), FundedTotal: &fundedBob, EverUsed: true},
		}
	}

	// 1) 首次快照：没有基准，不产生入账。
	if err := repo.ReplaceSnapshot(ctx, ws, Snapshot{Profiles: profiles(10, 30), FundedDeltaDay: today, SyncStamp: now,
		SiteDaily: &SiteDaily{Day: yesterday, TotalBalance: 18, UsersWithBalance: 2, UserCount: 2}}); err != nil {
		t.Fatalf("first snapshot: %v", err)
	}
	if summary, err := repo.RechargeDay(ctx, ws, today); err != nil || summary.Positive != 0 || len(summary.Channels) != 0 {
		t.Fatalf("the first snapshot only establishes the baseline: %+v %v", summary, err)
	}
	// 2) alice 充值 15、bob 被扣 4；3) alice 再充值 5：同一天累加。
	if err := repo.ReplaceSnapshot(ctx, ws, Snapshot{Profiles: profiles(25, 26), FundedDeltaDay: today, SyncStamp: now.Add(time.Minute)}); err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if err := repo.ReplaceSnapshot(ctx, ws, Snapshot{Profiles: profiles(30, 26), FundedDeltaDay: today, SyncStamp: now.Add(2 * time.Minute),
		SiteDaily: &SiteDaily{Day: today, TotalBalance: 20.5, UsersWithBalance: 2, UserCount: 2}}); err != nil {
		t.Fatalf("third snapshot: %v", err)
	}
	summary, err := repo.RechargeDay(ctx, ws, today)
	if err != nil || summary.Positive != 20 || summary.Negative != -4 || summary.Users != 1 || len(summary.Channels) != 2 {
		t.Fatalf("unexpected funded deltas: %+v %v", summary, err)
	}
	if credit := summary.Channels[0]; credit.Channel != ChannelCredit || credit.Amount != 20 || credit.Events != 2 || credit.Users != 1 {
		t.Fatalf("unexpected credit channel: %+v", credit)
	}

	// sub2api 记录类入账：替换某天时只替换记录渠道，不影响 credit / debit。
	records := []RechargeDaily{
		{SiteUserID: "2", Day: yesterday, Channel: ChannelPayment, Amount: 8, Events: 1},
		{SiteUserID: "2", Day: today, Channel: ChannelRedeem, Amount: 3, Events: 1},
	}
	if err := repo.ReplaceSnapshot(ctx, ws, Snapshot{RechargeDays: []time.Time{yesterday, today}, Recharges: records, SyncStamp: now}); err != nil {
		t.Fatalf("record recharges: %v", err)
	}
	if err := repo.ReplaceSnapshot(ctx, ws, Snapshot{RechargeDays: []time.Time{today}, Recharges: []RechargeDaily{{SiteUserID: "2", Day: today, Channel: ChannelRedeem, Amount: 7, Events: 2}}, SyncStamp: now}); err != nil {
		t.Fatalf("replace today: %v", err)
	}
	summary, err = repo.RechargeDay(ctx, ws, today)
	if err != nil || summary.Positive != 27 || summary.Users != 2 {
		t.Fatalf("records must be replaced while snapshot deltas stay: %+v %v", summary, err)
	}
	if y, err := repo.RechargeDay(ctx, ws, yesterday); err != nil || y.Positive != 8 {
		t.Fatalf("days that were not re-fetched must be kept: %+v %v", y, err)
	}

	series, err := repo.DailySeries(ctx, ws, "", yesterday, today)
	if err != nil || len(series) != 2 || series[0].Recharge == nil || *series[0].Recharge != 8 || *series[1].Recharge != 27 {
		t.Fatalf("unexpected recharge series: %+v %v", series, err)
	}
	single, err := repo.DailySeries(ctx, ws, "1", today, today)
	if err != nil || *single[0].Recharge != 20 {
		t.Fatalf("per-user recharge series: %+v %v", single, err)
	}

	totals, err := repo.BalanceTotals(ctx, ws, now)
	if err != nil || totals.TotalBalance != 20.5 || totals.UsersWithBalance != 2 || totals.LifetimeRecharged == nil || *totals.LifetimeRecharged != 50 || totals.LowBalanceActive != 1 {
		t.Fatalf("unexpected balance totals: %+v %v", totals, err)
	}
	if balance, err := repo.SiteBalance(ctx, ws, yesterday); err != nil || balance == nil || *balance != 18 {
		t.Fatalf("unexpected yesterday site balance: %v %v", balance, err)
	}
	if balance, err := repo.SiteBalance(ctx, ws, today.AddDate(0, 0, -5)); err != nil || balance != nil {
		t.Fatalf("missing snapshot must be nil: %v %v", balance, err)
	}

	windows := NewWindows(now, today)
	rows, _, err := repo.ListUsers(ctx, ws, windows, UserFilter{}, SortRechargeToday, "desc", 10, 0)
	if err != nil || siteIDs(rows) != "1,2" || rows[0].RechargeToday != 20 || rows[1].RechargeToday != 7 || rows[1].Recharge30d != 8 {
		t.Fatalf("unexpected recharge columns: %+v %v", rows, err)
	}
	for filter, want := range map[string]string{SegmentRechargedToday: "1,2", SegmentLowBalance: "1"} {
		got, _, err := repo.ListUsers(ctx, ws, windows, UserFilter{Segment: filter}, "", "", 10, 0)
		if err != nil || siteIDs(got) != want {
			t.Fatalf("segment %s = %q, want %q (%v)", filter, siteIDs(got), want, err)
		}
	}

	// 保留期清理覆盖入账和全站快照。
	if err := repo.ReplaceSnapshot(ctx, ws, Snapshot{RetainFrom: today, SyncStamp: now}); err != nil {
		t.Fatalf("retention: %v", err)
	}
	if y, err := repo.RechargeDay(ctx, ws, yesterday); err != nil || y.Positive != 0 {
		t.Fatalf("recharges before the retention start must be deleted: %+v %v", y, err)
	}
	if balance, err := repo.SiteBalance(ctx, ws, yesterday); err != nil || balance != nil {
		t.Fatalf("site snapshots before the retention start must be deleted: %v %v", balance, err)
	}
}
