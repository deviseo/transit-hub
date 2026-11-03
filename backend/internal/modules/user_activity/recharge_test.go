package user_activity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// 入账相关测试，数据均为合成数据。

func redeemRecord(id int, code string, value float64, userID string, usedAt time.Time) upstream.Sub2APIRedeemRecord {
	at := usedAt
	return upstream.Sub2APIRedeemRecord{ID: fmt.Sprint(id), Code: code, Value: value, UsedBy: userID, UsedAt: &at}
}

func sub2apiSession() *fakeSessions {
	return &fakeSessions{session: upstream.Session{Platform: upstream.PlatformSub2API, AccessToken: "token"}}
}

func rechargeRows(snapshot Snapshot) map[string]RechargeDaily {
	rows := map[string]RechargeDaily{}
	for _, row := range snapshot.Recharges {
		rows[fmt.Sprintf("%s|%s|%s", row.SiteUserID, formatDay(row.Day), row.Channel)] = row
	}
	return rows
}

func TestSub2APIRechargesAreSplitByChannelAndBusinessDay(t *testing.T) {
	store := newFakeStore()
	shanghai := func(day int, hour int) time.Time {
		return time.Date(2026, 11, day, hour, 0, 0, 0, time.FixedZone("CST", 8*3600))
	}
	transferAt, withdrawAt := shanghai(2, 9), shanghai(2, 9)
	platform := &fakePlatform{
		sub2apiUsers: sub2apiUsers(3),
		redeem: map[string][]upstream.Sub2APIRedeemRecord{
			redeemTypeBalance: {
				redeemRecord(9, "PAY-17-12345", 20, "1", shanghai(2, 10)),
				redeemRecord(8, "CARD-AAA", 5, "1", shanghai(2, 0)),
				redeemRecord(7, "CARD-BBB", 3, "2", shanghai(1, 23)), // 上海时间 11 月 1 日 23:00，算前一天
				redeemRecord(6, "CARD-NEG", -2, "2", shanghai(1, 22)),
			},
			redeemTypeAdminAdjust: {
				redeemRecord(5, "ADJ-1", 10, "3", shanghai(2, 8)),
				redeemRecord(4, "ADJ-2", -4, "3", shanghai(2, 7)),
			},
		},
		affiliate: []upstream.Sub2APIAffiliateTransfer{
			{ID: "2", Action: "transfer", UserID: "2", Amount: 1.5, CreatedAt: &transferAt},
			{ID: "1", Action: "withdraw", UserID: "2", Amount: 9, CreatedAt: &withdrawAt},
		},
	}
	service := newTestService(store, sub2apiSession(), platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	rows := rechargeRows(store.snapshots[0])
	expect := map[string]float64{
		"1|2026-11-02|payment":      20,
		"1|2026-11-02|redeem":       5,
		"2|2026-11-01|redeem":       1, // 3 + (-2)
		"3|2026-11-02|admin_add":    10,
		"3|2026-11-02|admin_deduct": -4,
		"2|2026-11-02|affiliate":    1.5,
	}
	if len(rows) != len(expect) {
		t.Fatalf("unexpected recharge rows: %+v", rows)
	}
	for key, amount := range expect {
		if row, ok := rows[key]; !ok || row.Amount != amount {
			t.Fatalf("row %s = %+v, want amount %v", key, row, amount)
		}
	}
	if rows["2|2026-11-01|redeem"].Events != 2 {
		t.Fatalf("events must be counted per record: %+v", rows["2|2026-11-01|redeem"])
	}
	state := store.state(t)
	if state.WarningKey != "" || formatDay(*state.RechargeSyncedTo) != "2026-11-02" {
		t.Fatalf("unexpected recharge state: %+v", state)
	}
	// 站点上没有更早的入账记录：回补直接把覆盖延伸到本地保留期起点。
	if want := day("2026-11-02").AddDate(0, 0, -(retentionDays - 1)); !state.RechargeSyncedFrom.Equal(want) {
		t.Fatalf("recharge coverage = %s, want %s", formatDay(*state.RechargeSyncedFrom), formatDay(want))
	}
}

func TestScanRedeemRecordsUsesBinarySearchForOlderWindows(t *testing.T) {
	records := make([]upstream.Sub2APIRedeemRecord, 0, 3000)
	newest := time.Date(2026, 11, 2, 2, 0, 0, 0, time.UTC)
	for i := 0; i < 3000; i++ { // 每小时一条，约 125 天
		records = append(records, redeemRecord(10000-i, fmt.Sprintf("CARD-%d", i), 1, "7", newest.Add(-time.Duration(i)*time.Hour)))
	}
	platform := &fakePlatform{redeem: map[string][]upstream.Sub2APIRedeemRecord{redeemTypeBalance: records}}
	service := newTestService(newFakeStore(), sub2apiSession(), platform)

	start := newest.Add(-2100 * time.Hour)
	end := newest.Add(-2000 * time.Hour)
	got, noneBefore, err := service.scanRedeemRecords(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API}, redeemTypeBalance, start, end)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}
	if len(got) != 100 || noneBefore {
		t.Fatalf("expected the 100 records inside the window and older records to exist, got %d noneBefore=%v", len(got), noneBefore)
	}
	for _, record := range got {
		if record.UsedAt.Before(start) || !record.UsedAt.Before(end) {
			t.Fatalf("record outside window: %v", record.UsedAt)
		}
	}
	// 6 页数据：第 1 页 + 二分查找 + 从前一页开始顺序读，不需要逐页从头读。
	if len(platform.redeemCalls) > 6 {
		t.Fatalf("too many page requests: %v", platform.redeemCalls)
	}

	all, noneBefore, err := service.scanRedeemRecords(context.Background(), upstream.Session{Platform: upstream.PlatformSub2API}, redeemTypeBalance, newest.Add(-5000*time.Hour), newest.Add(time.Hour))
	if err != nil || len(all) != 3000 || !noneBefore {
		t.Fatalf("a window covering everything must return all records and report nothing older: %d %v %v", len(all), noneBefore, err)
	}
}

func TestUnsortedRedeemRecordsDisableRechargeWithWarning(t *testing.T) {
	store := newFakeStore()
	platform := &fakePlatform{
		sub2apiUsers: sub2apiUsers(1),
		redeem: map[string][]upstream.Sub2APIRedeemRecord{redeemTypeBalance: {
			redeemRecord(2, "CARD-A", 1, "1", time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)),
			redeemRecord(1, "CARD-B", 1, "1", time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)), // 旧版本按 ID 排序，时间不单调
		}},
	}
	service := newTestService(store, sub2apiSession(), platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("recharge problems must not fail the sync: %v", err)
	}
	state := store.state(t)
	if state.Status != SyncStatusSuccess || state.WarningKey != WarningRechargeUnsupported || state.RechargeSyncedFrom != nil {
		t.Fatalf("unexpected state: %+v", state)
	}
	if len(store.snapshots[0].RechargeDays) != 0 || len(store.snapshots[0].Recharges) != 0 {
		t.Fatal("no recharge rows may be replaced when the records could not be read reliably")
	}
}

func TestRechargeFetchFailureKeepsPreviousCoverage(t *testing.T) {
	store := newFakeStore()
	from, to := day("2026-10-01"), day("2026-11-01")
	store.states[testWorkspace] = SyncState{Platform: "sub2api", SyncedFrom: &from, SyncedTo: &to, RechargeSyncedFrom: &from, RechargeSyncedTo: &to}
	platform := &fakePlatform{sub2apiUsers: sub2apiUsers(1), redeemErr: &upstream.RequestError{MessageKey: upstream.ErrorRequest, StatusCode: 403}}
	service := newTestService(store, sub2apiSession(), platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	state := store.state(t)
	if state.WarningKey != WarningRechargeFetch || formatDay(*state.RechargeSyncedTo) != "2026-11-01" || formatDay(*state.RechargeSyncedFrom) != "2026-10-01" {
		t.Fatalf("a failed read must keep the old coverage (so today shows as not up to date): %+v", state)
	}
}

func TestAffiliateEndpointMissingIsSkipped(t *testing.T) {
	store := newFakeStore()
	platform := &fakePlatform{
		sub2apiUsers: sub2apiUsers(1),
		redeem:       map[string][]upstream.Sub2APIRedeemRecord{redeemTypeBalance: {redeemRecord(1, "CARD-A", 2, "1", time.Date(2026, 11, 2, 1, 0, 0, 0, time.UTC))}},
		affiliateErr: &upstream.RequestError{MessageKey: upstream.ErrorInvalidResponse},
	}
	service := newTestService(store, sub2apiSession(), platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	state := store.state(t)
	if state.WarningKey != "" || state.RechargeSyncedFrom == nil {
		t.Fatalf("a missing affiliate endpoint is not an error: %+v", state)
	}
	if rows := rechargeRows(store.snapshots[0]); rows["1|2026-11-02|redeem"].Amount != 2 {
		t.Fatalf("redeem records must still be recorded: %+v", rows)
	}
}

func TestNewAPIRechargeUsesFundedSnapshots(t *testing.T) {
	store := newFakeStore()
	platform := &fakePlatform{newapiUsers: []upstream.NewAPIAdminUser{{ID: "5", Username: "alice", Status: 1, Balance: 2, UsedQuota: 7.5, EverUsed: true}}}
	sessions := &fakeSessions{session: upstream.Session{Platform: upstream.PlatformNewAPI, AccessToken: "nap_token"}}
	service := newTestService(store, sessions, platform)

	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("sync failed: %v", err)
	}
	recent := store.snapshots[0]
	if !recent.FundedDeltaDay.Equal(day("2026-11-02")) || recent.Profiles[0].FundedTotal == nil || *recent.Profiles[0].FundedTotal != 9.5 || len(recent.RechargeDays) != 0 {
		t.Fatalf("new-api must record funded totals and accumulate deltas on today: %+v", recent.Profiles[0])
	}
	state := store.state(t)
	if state.RechargeSince == nil || !state.RechargeSince.Equal(testNow) || formatDay(*state.RechargeSyncedFrom) != "2026-11-02" || formatDay(*state.RechargeSyncedTo) != "2026-11-02" {
		t.Fatalf("first new-api sync must start snapshot tracking now: %+v", state)
	}

	// 第二次同步：开始统计的时间保持不变。
	service.now = func() time.Time { return testNow.Add(time.Hour) }
	if err := service.SyncWorkspace(context.Background(), testWorkspace); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}
	if state := store.state(t); !state.RechargeSince.Equal(testNow) {
		t.Fatalf("recharge tracking start must not move: %v", state.RechargeSince)
	}
}

func TestSummaryMoneyRespectsCoverage(t *testing.T) {
	store := newFakeStore()
	usageFrom, today := day("2026-09-01"), day("2026-11-02")
	since := testNow.Add(-30 * time.Hour)
	rechargeFrom := day("2026-11-01")
	store.states[testWorkspace] = SyncState{Platform: "newapi", Status: SyncStatusSuccess, SyncedFrom: &usageFrom, SyncedTo: &today,
		RechargeSyncedFrom: &rechargeFrom, RechargeSyncedTo: &today, RechargeSince: &since}
	store.rechargeDays["2026-11-02"] = RechargeDaySummary{Channels: []ChannelAmount{{Channel: ChannelCredit, Amount: 30, Events: 3, Users: 2}, {Channel: ChannelDebit, Amount: -5, Events: 1, Users: 1}}, Users: 2, Positive: 30, Negative: -5}
	store.rechargeDays["2026-11-01"] = RechargeDaySummary{Positive: 12}
	store.balanceTotals = BalanceTotals{TotalBalance: 321.5, UsersWithBalance: 40, LowBalanceActive: 3}
	store.siteBalances["2026-11-01"] = 300
	service := newTestService(store, &fakeSessions{}, &fakePlatform{})

	summary, err := service.Summary(context.Background(), testWorkspace, 30)
	if err != nil {
		t.Fatalf("summary failed: %v", err)
	}
	money := summary.Money
	if money.Method != RechargeMethodSnapshot || money.TodayRecharge != 30 || money.TodayDeduction != 5 || money.TodayRechargeUsers != 2 || len(money.TodayChannels) != 2 {
		t.Fatalf("unexpected today money: %+v", money)
	}
	if money.YesterdayRecharge != nil {
		t.Fatal("the first snapshot day is partial and must not be reported as a full day")
	}
	if !money.RechargeUpToDate || money.RechargeFrom != "2026-11-01" || money.TotalBalance != 321.5 || money.BalanceYesterday == nil || *money.BalanceYesterday != 300 || money.LowBalanceActive != 3 {
		t.Fatalf("unexpected balance money: %+v", money)
	}
	if money.YesterdayCost == nil {
		t.Fatal("yesterday cost is covered by usage data")
	}
	for _, point := range summary.Daily {
		if (point.Day < "2026-11-01") != (point.Recharge == nil) {
			t.Fatalf("recharge must be null exactly outside the covered days: %+v", point)
		}
	}

	// sub2api：入账覆盖到昨天即可报告昨日全天。
	store.states[testWorkspace] = SyncState{Platform: "sub2api", Status: SyncStatusSuccess, SyncedFrom: &usageFrom, SyncedTo: &today, RechargeSyncedFrom: &rechargeFrom, RechargeSyncedTo: &today}
	summary, err = service.Summary(context.Background(), testWorkspace, 30)
	if err != nil || summary.Money.Method != RechargeMethodRecords || summary.Money.YesterdayRecharge == nil || *summary.Money.YesterdayRecharge != 12 {
		t.Fatalf("unexpected sub2api money: %+v %v", summary.Money, err)
	}
}

func TestUntilNextSlotAlignsToTheHour(t *testing.T) {
	service := newTestService(newFakeStore(), &fakeSessions{}, &fakePlatform{})
	// 10:30（上海）→ 11:00:30
	if got := service.untilNextSlot(testNow); got != 30*time.Minute+30*time.Second {
		t.Fatalf("untilNextSlot = %v", got)
	}
	// 10:59:45 距下一个整点不足 1 分钟，跳到 12:00:30
	if got := service.untilNextSlot(testNow.Add(29*time.Minute + 45*time.Second)); got != time.Hour+45*time.Second {
		t.Fatalf("untilNextSlot near the hour = %v", got)
	}
}

func TestRedeemChannelMapping(t *testing.T) {
	cases := []struct {
		codeType string
		record   upstream.Sub2APIRedeemRecord
		want     string
	}{
		{redeemTypeBalance, upstream.Sub2APIRedeemRecord{Code: "pay-1-2", Value: 1}, ChannelPayment},
		{redeemTypeBalance, upstream.Sub2APIRedeemRecord{Code: "ABC", Value: 1}, ChannelRedeem},
		{redeemTypeAdminAdjust, upstream.Sub2APIRedeemRecord{Value: 3}, ChannelAdminAdd},
		{redeemTypeAdminAdjust, upstream.Sub2APIRedeemRecord{Value: -3}, ChannelAdminDeduct},
	}
	for _, tc := range cases {
		if got := redeemChannel(tc.codeType, tc.record); got != tc.want {
			t.Fatalf("redeemChannel(%s, %+v) = %s, want %s", tc.codeType, tc.record, got, tc.want)
		}
	}
	if !unsupportedEndpoint(&upstream.RequestError{StatusCode: 404}) || unsupportedEndpoint(errors.New("x")) || unsupportedEndpoint(&upstream.RequestError{MessageKey: upstream.ErrorNetwork}) {
		t.Fatal("unexpected unsupportedEndpoint result")
	}
	if !strings.HasPrefix(rechargeWarningKey(errUnsortedRecords), "admin.userActivity.warnings.") {
		t.Fatal("warning keys must be i18n keys")
	}
}
