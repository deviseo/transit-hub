package vip

import (
	"context"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/settings"
)

// 服务层测试：内存假仓储和假机器人，数据都是合成的。

type fakeStore struct {
	synced     bool
	lastSynced *time.Time
	stateReads int
	stats      []statRow
	settings   Settings
	states     map[string]alertState
	notified   map[string]string
	recovered  []string
	saves      int
}

func (f *fakeStore) SyncState(ctx context.Context, ws Workspace) (syncState, bool, error) {
	return syncState{Platform: "sub2api", LastSyncedAt: f.lastSynced}, f.synced, nil
}

func (f *fakeStore) Watchlist(ctx context.Context, ws Workspace, win windows, dailyTop, lifetimeTop int) ([]statRow, error) {
	return append([]statRow(nil), f.stats...), nil
}

func (f *fakeStore) DismissedCount(ctx context.Context, ws Workspace) (int, error) { return 0, nil }

func (f *fakeStore) Search(ctx context.Context, ws Workspace, win windows, query string, limit int) ([]Candidate, []float64, error) {
	return nil, nil, nil
}

func (f *fakeStore) Watch(ctx context.Context, ws Workspace, siteUserID string, note string) (bool, error) {
	return siteUserID != "missing", nil
}

func (f *fakeStore) Unwatch(ctx context.Context, ws Workspace, siteUserID string) error { return nil }

func (f *fakeStore) RestoreDismissed(ctx context.Context, ws Workspace) (int, error) { return 0, nil }

func (f *fakeStore) Settings(ctx context.Context, ws Workspace) (Settings, error) {
	return f.settings, nil
}

func (f *fakeStore) SaveSettings(ctx context.Context, ws Workspace, settings Settings) error {
	f.settings = settings
	return nil
}

func (f *fakeStore) AlertWorkspaces(ctx context.Context) ([]Workspace, error) {
	return []Workspace{{UserID: "u", AdminAccountID: "a"}}, nil
}

func (f *fakeStore) AlertStates(ctx context.Context, ws Workspace) (map[string]alertState, error) {
	f.stateReads++
	copied := map[string]alertState{}
	for id, state := range f.states {
		copied[id] = state
	}
	return copied, nil
}

func (f *fakeStore) SaveAlertStates(ctx context.Context, ws Workspace, notified map[string]string, at time.Time, recovered []string) error {
	f.saves++
	f.notified = notified
	f.recovered = recovered
	if f.states == nil {
		f.states = map[string]alertState{}
	}
	for id, state := range notified {
		f.states[id] = alertState{State: state, NotifiedAt: at}
	}
	for _, id := range recovered {
		state := f.states[id]
		state.Recovered = true
		f.states[id] = state
	}
	return nil
}

type fakeNotifier struct {
	messages []string
	bots     [][]string
}

func (f *fakeNotifier) SendFormattedToWorkspaceBots(ctx context.Context, userID string, adminAccountID string, botIDs []string, message string, format settings.NotificationTemplateFormat) {
	f.messages = append(f.messages, message)
	f.bots = append(f.bots, botIDs)
}

type fixedPrice float64

func (p fixedPrice) SiteUnitPrice(ctx context.Context, userID string, adminAccountID string) (float64, error) {
	return float64(p), nil
}

type fakeNames struct{}

func (fakeNames) WorkspaceName(ctx context.Context, userID string, adminAccountID string) string {
	return "示例中转"
}
func (fakeNames) BrandName(ctx context.Context, userID string, adminAccountID string) string {
	return "极速中转"
}

var testNow = time.Date(2026, 11, 9, 4, 0, 0, 0, time.UTC)

func ptr(value float64) *float64 { return &value }

func timePtr(value time.Time) *time.Time { return &value }

func TestClassifyCoversEveryStatus(t *testing.T) {
	settings := DefaultSettings()
	recent := timePtr(testNow.Add(-2 * time.Hour))
	cases := []struct {
		name   string
		stat   statRow
		status string
	}{
		{"disabled on the site", statRow{UserStatus: "disabled", Balance: ptr(0), Cost7: 70}, StatusDisabled},
		{"balance used up", statRow{Balance: ptr(0), Cost7: 70}, StatusEmpty},
		{"used up even if no longer using", statRow{Balance: ptr(-1)}, StatusEmpty},
		// 日均 ¥10（price 1），余额 ¥15：1.5 天 ≤ 2 天。
		{"runs out within the alert days", statRow{Balance: ptr(15), Cost7: 70, LastUsedAt: recent}, StatusLow},
		// 余额 ¥19 < ¥20，日均很低也算快用完。
		{"below the alert balance", statRow{Balance: ptr(19), Cost7: 0.7, LastUsedAt: recent}, StatusLow},
		{"low but not using any more", statRow{Balance: ptr(5)}, StatusNormal},
		// 前 7 天日均 ¥10，近 7 天 ¥4：下降超过一半。
		{"usage dropped by half", statRow{Balance: ptr(500), Cost7: 28, CostPrev7: 70}, StatusDeclining},
		{"tiny previous usage is not a decline", statRow{Balance: ptr(500), Cost7: 0, CostPrev7: 3}, StatusNormal},
		{"plenty left", statRow{Balance: ptr(500), Cost7: 70, CostPrev7: 70}, StatusNormal},
	}
	for _, tc := range cases {
		row := buildRow(tc.stat, settings, 1, testNow)
		if row.Status != tc.status {
			t.Fatalf("%s: status = %s, want %s (%+v)", tc.name, row.Status, tc.status, row)
		}
	}
}

func TestBuildRowConvertsMoneyAndEstimatesDaysLeft(t *testing.T) {
	credit := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	row := buildRow(statRow{
		SiteUserID: "7", Email: "alice@example.test", Kind: kindManual, Note: "XX 公司", DailyRank: 2,
		Balance: ptr(30), Cost7: 70, CostPrev7: 35, Lifetime: 900, CreditDay: &credit, CreditAmount: 50,
	}, DefaultSettings(), 2, testNow)
	if *row.Balance != 60 || row.DailyAverage != 20 || row.PreviousAverage != 10 || row.Lifetime != 1800 {
		t.Fatalf("money not converted at ¥2/$1: %+v", row)
	}
	if row.Trend == nil || *row.Trend != 1 || row.DaysLeft == nil || *row.DaysLeft != 3 {
		t.Fatalf("trend %v, days left %v", row.Trend, row.DaysLeft)
	}
	if !row.Source.Manual || row.Source.Note != "XX 公司" || row.Source.DailyRank != 2 || row.LastCreditDay != "2026-11-05" || row.LastCreditAmount != 100 {
		t.Fatalf("source and last credit: %+v", row)
	}
	if quiet := buildRow(statRow{Balance: ptr(30)}, DefaultSettings(), 1, testNow); quiet.DaysLeft != nil || quiet.Trend != nil {
		t.Fatalf("no usage, no estimate: %+v", quiet)
	}
}

func TestShouldNotifyOncePerDropAndOnEscalation(t *testing.T) {
	earlier := testNow.Add(-3 * time.Hour)
	yesterday := testNow.Add(-25 * time.Hour)
	cases := []struct {
		name    string
		current string
		last    *alertState
		want    bool
	}{
		{"first time low", StatusLow, nil, true},
		{"first time empty", StatusEmpty, nil, true},
		{"still low", StatusLow, &alertState{State: StatusLow, NotifiedAt: yesterday}, false},
		{"low became empty", StatusEmpty, &alertState{State: StatusLow, NotifiedAt: earlier}, true},
		{"empty after top-up to low", StatusLow, &alertState{State: StatusEmpty, NotifiedAt: earlier}, false},
		{"recovered and dropped the same day", StatusLow, &alertState{State: StatusLow, NotifiedAt: earlier, Recovered: true}, false},
		{"recovered and dropped a day later", StatusLow, &alertState{State: StatusLow, NotifiedAt: yesterday, Recovered: true}, true},
		{"fine again", StatusNormal, &alertState{State: StatusLow, NotifiedAt: yesterday}, false},
	}
	for _, tc := range cases {
		if got := shouldNotify(tc.current, tc.last, testNow); got != tc.want {
			t.Fatalf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestAlertWorkspaceSendsOneMessageAndRemembers(t *testing.T) {
	recent := timePtr(testNow.Add(-time.Hour))
	store := &fakeStore{
		synced:   true,
		settings: Settings{DailyTop: 20, LifetimeTop: 20, AlertDays: 2, AlertBalance: 20, AlertsEnabled: true, BotIDs: []string{"bot-1"}},
		stats: []statRow{
			{SiteUserID: "1", Email: "alice@example.test", Balance: ptr(6), Cost7: 70, LastUsedAt: recent, Note: "XX 公司", Kind: kindManual},
			{SiteUserID: "2", Email: "bob@example.test", Balance: ptr(0), Cost7: 35, LastUsedAt: recent},
			{SiteUserID: "3", Email: "carol@example.test", Balance: ptr(0)},
			{SiteUserID: "4", Email: "dave@example.test", Balance: ptr(900), Cost7: 70, LastUsedAt: recent},
		},
		states: map[string]alertState{"4": {State: StatusLow, NotifiedAt: testNow.Add(-48 * time.Hour)}},
	}
	notifier := &fakeNotifier{}
	service := &Service{store: store, prices: fixedPrice(1), names: fakeNames{}, notifier: notifier, now: func() time.Time { return testNow }}

	service.runAlerts(context.Background())
	if len(notifier.messages) != 1 || notifier.bots[0][0] != "bot-1" {
		t.Fatalf("one message to the chosen bot: %+v", notifier)
	}
	message := notifier.messages[0]
	for _, want := range []string{"👑 大户余额提醒", "工作区：示例中转",
		"⚠️ alice@example.test 余额 ¥6.00，近 7 天日均 ¥10.00，预计不到 1 天用完（XX 公司）",
		"⛔ bob@example.test 余额用完了（近 7 天日均 ¥5.00）"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message lacks %q:\n%s", want, message)
		}
	}
	if strings.Contains(message, "carol") {
		t.Fatalf("someone who stopped using long ago is not alerted:\n%s", message)
	}
	if store.notified["1"] != StatusLow || store.notified["2"] != StatusEmpty || len(store.recovered) != 1 || store.recovered[0] != "4" {
		t.Fatalf("states not remembered: %+v %+v", store.notified, store.recovered)
	}

	// 下一次检查：情况没变，不再发。
	service.runAlerts(context.Background())
	if len(notifier.messages) != 1 {
		t.Fatalf("no repeat for the same state: %d messages", len(notifier.messages))
	}

	// 没选机器人：不发。
	store.settings.BotIDs = nil
	store.states = nil
	service.runAlerts(context.Background())
	if len(notifier.messages) != 1 {
		t.Fatalf("no bots, no message")
	}
}

func TestAlertChecksOncePerSync(t *testing.T) {
	synced := testNow.Add(-10 * time.Minute)
	store := &fakeStore{
		synced:     true,
		lastSynced: &synced,
		settings:   Settings{AlertsEnabled: true, BotIDs: []string{"bot-1"}, AlertDays: 2, AlertBalance: 20},
	}
	service := &Service{store: store, prices: fixedPrice(1), notifier: &fakeNotifier{}, now: func() time.Time { return testNow }}
	service.runAlerts(context.Background())
	service.runAlerts(context.Background())
	if store.stateReads != 1 {
		t.Fatalf("same sync checked %d times, want once", store.stateReads)
	}
	next := synced.Add(time.Hour)
	store.lastSynced = &next
	service.runAlerts(context.Background())
	if store.stateReads != 2 {
		t.Fatalf("a new sync is checked again: %d", store.stateReads)
	}
}

func TestBriefForTheDailyReport(t *testing.T) {
	recent := timePtr(testNow.Add(-time.Hour))
	store := &fakeStore{
		synced:   true,
		settings: DefaultSettings(),
		stats: []statRow{
			{SiteUserID: "1", Email: "alice@example.test", Balance: ptr(15), Cost7: 70, LastUsedAt: recent},
			{SiteUserID: "2", Username: "bob", Balance: ptr(0), Cost7: 35, LastUsedAt: recent},
			{SiteUserID: "3", Balance: ptr(800), Cost7: 70, LastUsedAt: recent},
		},
	}
	service := &Service{store: store, prices: fixedPrice(1), now: func() time.Time { return testNow }}
	brief, err := service.Brief(context.Background(), Workspace{UserID: "u", AdminAccountID: "a"})
	if err != nil {
		t.Fatalf("brief: %v", err)
	}
	want := "【大户】关注 3 人 · 快用完 1 人 · 已用完 1 人\n⚠️ 快用完：alice@example.test（约 1.5 天）\n⛔ 已用完：bob"
	if got := brief.ReportSection(); got != want {
		t.Fatalf("section:\n%s\nwant:\n%s", got, want)
	}
	if got := (Brief{Synced: true, Watched: 4}).ReportSection(); got != "【大户】关注 4 人，余额都还够用" {
		t.Fatalf("all fine: %q", got)
	}
	if got := (Brief{Synced: true}).ReportSection(); got != "" {
		t.Fatalf("nobody watched, no section: %q", got)
	}
	many := Brief{Synced: true, Watched: 9, Low: []string{"a", "b", "c", "d", "e"}}
	if got := many.ReportSection(); !strings.Contains(got, "⚠️ 快用完：a、b、c 等 5 人") {
		t.Fatalf("long lists are cut: %q", got)
	}
}

func TestSettingsAreValidated(t *testing.T) {
	store := &fakeStore{}
	service := &Service{store: store, now: func() time.Time { return testNow }}
	saved, err := service.SaveSettings(context.Background(), Workspace{}, Settings{
		DailyTop: 10, LifetimeTop: 0, AlertDays: 3, AlertBalance: 50, AlertsEnabled: true,
		BotIDs: []string{" bot-1 ", "bot-1", "", "bot-2"}, ReminderText: "  您好 {name}  ",
	})
	if err != nil || len(saved.BotIDs) != 2 || saved.BotIDs[0] != "bot-1" || saved.ReminderText != "您好 {name}" {
		t.Fatalf("saved = %+v (%v)", saved, err)
	}
	for _, bad := range []Settings{
		{DailyTop: 101}, {LifetimeTop: -1}, {AlertDays: 31}, {AlertBalance: -1},
		{ReminderText: strings.Repeat("话", maxReminderRunes+1)},
	} {
		if _, err := service.SaveSettings(context.Background(), Workspace{}, bad); err == nil || err.Error() != ErrorInvalidSettings {
			t.Fatalf("settings %+v must be rejected: %v", bad, err)
		}
	}
}

func TestWatchChecksTheUser(t *testing.T) {
	service := &Service{store: &fakeStore{}, now: func() time.Time { return testNow }}
	if err := service.Watch(context.Background(), Workspace{}, " 7 ", strings.Repeat("备", maxNoteRunes+50)); err != nil {
		t.Fatalf("watch: %v", err)
	}
	if err := service.Watch(context.Background(), Workspace{}, "missing", ""); err == nil || err.Error() != ErrorUserNotFound {
		t.Fatalf("unknown users can't be watched: %v", err)
	}
	if err := service.Unwatch(context.Background(), Workspace{}, "  "); err == nil || err.Error() != ErrorInvalidUser {
		t.Fatalf("empty id: %v", err)
	}
}

func TestDaysText(t *testing.T) {
	for days, want := range map[float64]string{0.4: "不到 1 天", 1.56: "约 1.5 天", 3: "约 3 天", 9.99: "约 9.9 天", 12.7: "约 12 天"} {
		if got := daysText(days); got != want {
			t.Fatalf("daysText(%v) = %q, want %q", days, got, want)
		}
	}
}
