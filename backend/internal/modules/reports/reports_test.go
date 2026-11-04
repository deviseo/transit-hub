package reports

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/dashboard"
	"transithub/backend/internal/modules/profit"
	"transithub/backend/internal/modules/sales"
	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/modules/user_activity"
)

// 经营日报测试：所有数据均为合成数据。

type fakeSnapshots struct {
	snapshots []dashboard.DailySnapshot
	finalized bool
	err       error
}

func (f fakeSnapshots) ListRange(ctx context.Context, userID, adminAccountID string, today time.Time, days int) ([]dashboard.DailySnapshot, error) {
	return f.snapshots, f.err
}

func (f fakeSnapshots) IsFinalized(ctx context.Context, userID, adminAccountID string, date time.Time) (bool, error) {
	return f.finalized, nil
}

type fakeSales struct {
	summary sales.SummaryResponse
	queries []sales.RangeQuery
}

func (f *fakeSales) Summary(ctx context.Context, ws sales.Workspace, query sales.RangeQuery) (sales.SummaryResponse, error) {
	f.queries = append(f.queries, query)
	return f.summary, nil
}

type fakeActivity struct{ summary user_activity.SummaryResponse }

func (f fakeActivity) Summary(ctx context.Context, ws user_activity.Workspace, seriesDays int) (user_activity.SummaryResponse, error) {
	return f.summary, nil
}

type fakeNames string

func (f fakeNames) WorkspaceName(ctx context.Context, userID string, adminAccountID string) string {
	return string(f)
}

func snapshotOn(day string, revenue, cost, upstream float64) dashboard.DailySnapshot {
	date, _ := time.Parse("2006-01-02", day)
	return dashboard.DailySnapshot{Date: date, TodayProfit: revenue, TodayPurchase: cost, NetProfit: revenue - cost, UpstreamBalance: upstream}
}

// 北京时间 2026-11-03 09:00：日报汇总 11-02（周一）。
var reportNow = time.Date(2026, 11, 3, 1, 0, 0, 0, time.UTC)

func intPtr(value int) *int { return &value }

func TestBuildFullReport(t *testing.T) {
	salesReader := &fakeSales{summary: sales.SummaryResponse{
		Coverage: sales.Coverage{Synced: true, RechargeComplete: true},
		Current:  sales.PeriodTotals{Credits: 2300, Payers: 18, NewPayers: intPtr(5)},
		Channels: []sales.ChannelTotal{
			{Channel: sales.ChannelPayment, Amount: 1800},
			{Channel: sales.ChannelRedeem, Amount: 500},
			{Channel: sales.ChannelAdminDeduct, Amount: -20},
		},
	}}
	builder := NewBuilder(
		fakeSnapshots{finalized: true, snapshots: []dashboard.DailySnapshot{
			snapshotOn("2026-10-31", 1500, 600, 4000),
			snapshotOn("2026-11-01", 1656.81, 700, 3600),
			snapshotOn("2026-11-02", 1860.5, 742.3, 3200),
		}},
		salesReader,
		fakeActivity{summary: user_activity.SummaryResponse{
			HasData: true,
			Periods: map[string]user_activity.PeriodComparison{
				"day": {Current: user_activity.PeriodMetrics{ActiveUsers: 86, NewUsers: 12}, Previous: &user_activity.PeriodMetrics{ActiveUsers: 89}},
			},
			Inactive: []user_activity.InactiveBucket{{Days: 2, Count: 400}, {Days: 7, Count: 230}},
			Money:    user_activity.MoneySummary{LowBalanceActive: 9},
		}},
		fakeNames("我的站点"),
	)

	report, err := builder.Build(context.Background(), "u", "a", reportNow)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if report.Date != "2026-11-02" || report.Workspace != "我的站点" {
		t.Fatalf("unexpected report header: %+v", report)
	}
	for _, want := range []string{
		"📊 经营日报 · 2026-11-02（周一）\n工作区：我的站点",
		"【经营】\n营收 ¥1,860.50（较前日 +12.3%）\n成本 ¥742.30 · 毛利 ¥1,118.20（毛利率 60.1%）",
		"【销售】\n入账 ¥2,300.00 · 18 人（首次入账 5 人）\n在线充值 ¥1,800.00 · 兑换码 ¥500.00",
		"【用户】\n活跃 86 人（较前日 -3.4%）· 新注册 12 人\n超过 7 天未使用 230 人 · 近期活跃但余额不足 9 人",
		"【上游】\n上游余额 ¥3,200.00\n按近 3 天日均成本 ¥680.77 估算，约可用 4.7 天",
	} {
		if !strings.Contains(report.Text, want) {
			t.Fatalf("report is missing %q:\n%s", want, report.Text)
		}
	}
	if strings.Contains(report.Text, "管理员扣款") || strings.Contains(report.Text, "⚠️") {
		t.Fatalf("deductions and warnings must not appear:\n%s", report.Text)
	}
	if len(salesReader.queries) != 1 || salesReader.queries[0].From != "2026-11-02" || salesReader.queries[0].To != "2026-11-02" {
		t.Fatalf("sales must be queried for yesterday only: %+v", salesReader.queries)
	}
}

func TestBuildReportFlagsGapsAndShortRunway(t *testing.T) {
	builder := NewBuilder(
		fakeSnapshots{finalized: false, snapshots: []dashboard.DailySnapshot{snapshotOn("2026-11-02", 0, 500, 900)}},
		&fakeSales{summary: sales.SummaryResponse{Coverage: sales.Coverage{Synced: true, RechargeComplete: false}, Current: sales.PeriodTotals{Credits: 10, Payers: 1}}},
		fakeActivity{},
		nil,
	)
	report, err := builder.Build(context.Background(), "u", "a", reportNow)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	for _, want := range []string{"【经营】（数据未定稿，可能不完整）", "【销售】（入账数据不完整）", "入账 ¥10.00 · 1 人\n", "⚠️ 按近 1 天日均成本 ¥500.00 估算，约可用 1.8 天，请尽快充值上游"} {
		if !strings.Contains(report.Text+"\n", want) {
			t.Fatalf("report is missing %q:\n%s", want, report.Text)
		}
	}
	if strings.Contains(report.Text, "毛利率") || strings.Contains(report.Text, "较前日") || strings.Contains(report.Text, "首次入账") || strings.Contains(report.Text, "工作区：") {
		t.Fatalf("unavailable figures must be omitted:\n%s", report.Text)
	}
}

func TestBuildReportWithoutAnyData(t *testing.T) {
	builder := NewBuilder(fakeSnapshots{}, &fakeSales{}, fakeActivity{}, fakeNames(""))
	report, err := builder.Build(context.Background(), "u", "a", reportNow)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(report.Text, "暂无可汇总的数据") {
		t.Fatalf("an empty report must explain what is missing:\n%s", report.Text)
	}
	if _, err := NewBuilder(fakeSnapshots{err: errors.New("db down")}, nil, nil, nil).Build(context.Background(), "u", "a", reportNow); err == nil {
		t.Fatal("storage errors must be returned so the scheduler can retry")
	}
}

func TestFormatCNY(t *testing.T) {
	cases := map[float64]string{0: "¥0.00", 1234567.891: "¥1,234,567.89", -1118.2: "-¥1,118.20", 999.999: "¥1,000.00", -0.001: "¥0.00"}
	for value, want := range cases {
		if got := formatCNY(value); got != want {
			t.Fatalf("formatCNY(%v) = %q, want %q", value, got, want)
		}
	}
}

type fakeBuilder struct {
	calls int
	err   error
}

func (f *fakeBuilder) Build(ctx context.Context, userID string, adminAccountID string, now time.Time) (DailyReport, error) {
	f.calls++
	if f.err != nil {
		return DailyReport{}, f.err
	}
	return DailyReport{Date: "2026-11-02", Text: "report for " + adminAccountID}, nil
}

type fakeStrategies struct {
	list  []settings.WorkspaceStrategy
	saved settings.StrategySettings
}

func (f fakeStrategies) ListStrategies(ctx context.Context) ([]settings.WorkspaceStrategy, error) {
	return f.list, nil
}

func (f fakeStrategies) GetStrategyForWorkspace(ctx context.Context, userID string, adminAccountID string) (settings.StrategySettings, error) {
	return f.saved, nil
}

type sentReport struct {
	workspace string
	bots      []string
	text      string
	format    settings.NotificationTemplateFormat
}

type fakeNotifier struct{ sent []sentReport }

func (f *fakeNotifier) SendFormattedToWorkspaceBots(ctx context.Context, userID string, adminAccountID string, botIDs []string, message string, format settings.NotificationTemplateFormat) {
	f.sent = append(f.sent, sentReport{workspace: userID + "|" + adminAccountID, bots: botIDs, text: message, format: format})
}

type staticAccounts string

func (a staticAccounts) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	if a == "" {
		return "", errors.New("none")
	}
	return string(a), nil
}

func TestSchedulerSendsOncePerDayAfterTheConfiguredTime(t *testing.T) {
	builder := &fakeBuilder{}
	notifier := &fakeNotifier{}
	strategies := fakeStrategies{list: []settings.WorkspaceStrategy{
		{UserID: "u", AdminAccountID: "ws-a", Settings: settings.StrategySettings{EnableDailyReport: true, DailyReportTime: "09:30", DailyReportBotIDs: []string{"bot-a", " ", "bot-a"}}},
		{UserID: "u", AdminAccountID: "ws-off", Settings: settings.StrategySettings{EnableDailyReport: false, DailyReportTime: "00:00", DailyReportBotIDs: []string{"bot"}}},
		{UserID: "u", AdminAccountID: "ws-nobots", Settings: settings.StrategySettings{EnableDailyReport: true, DailyReportTime: "00:00"}},
	}}
	service := &Service{builder: builder, strategies: strategies, notifier: notifier, once: newMemoryOnceStore()}
	at := func(hour, minute int) time.Time {
		return time.Date(2026, 11, 3, hour, minute, 0, 0, time.FixedZone("CST", 8*3600))
	}

	service.now = func() time.Time { return at(9, 29) }
	service.runDue(context.Background())
	if len(notifier.sent) != 0 {
		t.Fatalf("nothing may be sent before 09:30: %+v", notifier.sent)
	}
	service.now = func() time.Time { return at(9, 30) }
	service.runDue(context.Background())
	service.now = func() time.Time { return at(15, 0) }
	service.runDue(context.Background())
	if len(notifier.sent) != 1 || notifier.sent[0].workspace != "u|ws-a" || len(notifier.sent[0].bots) != 1 || notifier.sent[0].format != settings.NotificationTemplateFormatText {
		t.Fatalf("exactly one plain-text report to the cleaned bot list: %+v", notifier.sent)
	}

	// 第二天到点再发一次。
	service.now = func() time.Time { return at(9, 30).AddDate(0, 0, 1) }
	service.runDue(context.Background())
	if len(notifier.sent) != 2 {
		t.Fatalf("the next day must be sent again: %d", len(notifier.sent))
	}
}

func TestSchedulerRetriesAfterBuildFailure(t *testing.T) {
	builder := &fakeBuilder{err: errors.New("db down")}
	notifier := &fakeNotifier{}
	strategies := fakeStrategies{list: []settings.WorkspaceStrategy{
		{UserID: "u", AdminAccountID: "ws-a", Settings: settings.StrategySettings{EnableDailyReport: true, DailyReportTime: "bad", DailyReportBotIDs: []string{"bot-a"}}},
	}}
	service := &Service{builder: builder, strategies: strategies, notifier: notifier, once: newMemoryOnceStore()}
	service.now = func() time.Time { return time.Date(2026, 11, 3, 9, 0, 0, 0, time.FixedZone("CST", 8*3600)) }

	service.runDue(context.Background())
	if len(notifier.sent) != 0 || builder.calls != 1 {
		t.Fatalf("a failed build must not send: %+v", notifier.sent)
	}
	builder.err = nil
	service.runDue(context.Background())
	if len(notifier.sent) != 1 {
		t.Fatalf("the report must be retried after a failure (invalid times fall back to 09:00): %+v", notifier.sent)
	}
}

func TestPreviewAndSendTest(t *testing.T) {
	builder := &fakeBuilder{}
	notifier := &fakeNotifier{}
	service := &Service{builder: builder, strategies: fakeStrategies{saved: settings.StrategySettings{DailyReportBotIDs: []string{"saved-bot"}}}, notifier: notifier, accounts: staticAccounts("ws-a"), once: newMemoryOnceStore(), now: time.Now}

	report, err := service.Preview(context.Background(), "u")
	if err != nil || report.Text != "report for ws-a" || len(notifier.sent) != 0 {
		t.Fatalf("preview must build for the current workspace without sending: %+v %v", report, err)
	}
	if _, sent, err := service.SendTest(context.Background(), "u", []string{"form-bot"}); err != nil || sent != 1 || notifier.sent[0].bots[0] != "form-bot" {
		t.Fatalf("test sends go to the bots selected in the form: %v %v %+v", sent, err, notifier.sent)
	}
	if _, _, err := service.SendTest(context.Background(), "u", nil); err != nil || notifier.sent[1].bots[0] != "saved-bot" {
		t.Fatalf("without a selection the saved bots are used: %v %+v", err, notifier.sent)
	}
	service.strategies = fakeStrategies{}
	if _, _, err := service.SendTest(context.Background(), "u", nil); err == nil || err.Error() != ErrorNoBots {
		t.Fatalf("expected %s, got %v", ErrorNoBots, err)
	}
	service.accounts = staticAccounts("")
	if _, err := service.Preview(context.Background(), "u"); err == nil || err.Error() != ErrorNoCurrentAccount {
		t.Fatalf("expected %s, got %v", ErrorNoCurrentAccount, err)
	}
}

type fakePriceChanges struct {
	changes []profit.PriceChange
	since   time.Time
	until   time.Time
}

func (f *fakePriceChanges) PriceChangesBetween(ctx context.Context, ws profit.Workspace, since time.Time, until time.Time) ([]profit.PriceChange, error) {
	f.since, f.until = since, until
	return f.changes, nil
}

func TestBuildReportListsYesterdaysPriceChanges(t *testing.T) {
	impact, saving := 4.0, -1.5
	prices := &fakePriceChanges{changes: []profit.PriceChange{ // 最新的在前
		{SiteName: "Beta", GroupName: "gpt", OldMultiplier: 1, NewMultiplier: 0.8, EstimateStatus: profit.EstimateOK, WindowDays: 7, DailyImpact: &saving},
		{SiteName: "Alpha", GroupName: "claude", OldMultiplier: 1, NewMultiplier: 1.2, EstimateStatus: profit.EstimateOK, WindowDays: 7, DailyImpact: &impact},
		{SiteName: "Alpha", GroupName: "idle", OldMultiplier: 0.5, NewMultiplier: 0.75, EstimateStatus: profit.EstimateNoUsage, WindowDays: 7},
	}}
	builder := NewBuilder(fakeSnapshots{}, &fakeSales{}, fakeActivity{}, nil)
	builder.SetPriceChanges(prices)
	report, err := builder.Build(context.Background(), "u", "a", reportNow)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := "【上游】\n昨日上游调价 3 处：\n· Alpha / idle：0.5x → 0.75x，近 7 天未使用该分组\n· Alpha / claude：1x → 1.2x，每天约多 ¥4.00\n· Beta / gpt：1x → 0.8x，每天约省 ¥1.50"
	if !strings.Contains(report.Text, want) {
		t.Fatalf("report is missing the price changes:\n%s", report.Text)
	}
	// 昨天（北京时间 11-02 00:00 ~ 11-03 00:00）。
	if prices.since.Unix() != 1793548800 || prices.until.Unix() != 1793635200 {
		t.Fatalf("price changes must be read for yesterday: %v ~ %v", prices.since, prices.until)
	}
}
