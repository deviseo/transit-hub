package sales

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"
)

// 服务层测试：内存假仓储，数据均为合成数据。

type fakeStore struct {
	state      syncState
	synced     bool
	totals     map[string]rawTotals // key: from|to
	balances   map[string]float64   // key: day
	channels   []ChannelTotal
	daily      []dailyChannelRow
	usage      []dailyValueRow
	amounts    []float64
	payers     []PayerRow
	totalCalls []string
}

func rangeKey(from, to time.Time) string { return formatDay(from) + "|" + formatDay(to) }

func (f *fakeStore) SyncState(ctx context.Context, ws Workspace) (syncState, bool, error) {
	return f.state, f.synced, nil
}

func (f *fakeStore) PeriodTotals(ctx context.Context, ws Workspace, from, to time.Time, registeredFrom, registeredTo time.Time) (rawTotals, error) {
	f.totalCalls = append(f.totalCalls, rangeKey(from, to)+"@"+registeredFrom.Format(time.RFC3339)+"~"+registeredTo.Format(time.RFC3339))
	return f.totals[rangeKey(from, to)], nil
}

func (f *fakeStore) SiteBalance(ctx context.Context, ws Workspace, day time.Time) (*float64, error) {
	if value, ok := f.balances[formatDay(day)]; ok {
		return &value, nil
	}
	return nil, nil
}

func (f *fakeStore) Channels(ctx context.Context, ws Workspace, from, to time.Time) ([]ChannelTotal, error) {
	return append([]ChannelTotal(nil), f.channels...), nil
}

func (f *fakeStore) DailyChannels(ctx context.Context, ws Workspace, from, to time.Time) ([]dailyChannelRow, error) {
	return f.daily, nil
}

func (f *fakeStore) DailyConsumption(ctx context.Context, ws Workspace, from, to time.Time) ([]dailyValueRow, error) {
	return f.usage, nil
}

func (f *fakeStore) PayerAmounts(ctx context.Context, ws Workspace, from, to time.Time) ([]float64, error) {
	return append([]float64(nil), f.amounts...), nil
}

func (f *fakeStore) TopPayers(ctx context.Context, ws Workspace, from, to time.Time, limit int) ([]PayerRow, error) {
	result := make([]PayerRow, 0, len(f.payers))
	for _, payer := range f.payers {
		copied := payer
		copied.ChannelBreakout = append([]ChannelTotal(nil), payer.ChannelBreakout...)
		result = append(result, copied)
	}
	return result, nil
}

type fixedPrice float64

func (p fixedPrice) SiteUnitPrice(ctx context.Context, userID string, adminAccountID string) (float64, error) {
	return float64(p), nil
}

type failingPrice struct{}

func (failingPrice) SiteUnitPrice(ctx context.Context, userID string, adminAccountID string) (float64, error) {
	return 0, errors.New("db down")
}

func day(value string) time.Time {
	parsed, _ := time.Parse("2006-01-02", value)
	return parsed
}

func dayPtr(value string) *time.Time {
	parsed := day(value)
	return &parsed
}

func serviceAt(store *fakeStore, price PriceProvider, now time.Time) *Service {
	return &Service{store: store, prices: price, now: func() time.Time { return now }}
}

// 北京时间 2026-11-03 01:00（UTC 11-02 17:00）：业务上的「今天」是 11-03，默认区间截至 11-02。
var testNow = time.Date(2026, 11, 2, 17, 0, 0, 0, time.UTC)

func TestResolveRange(t *testing.T) {
	today := day("2026-11-03")
	from, to, err := resolveRange(RangeQuery{}, today)
	if err != nil || formatDay(from) != "2026-10-04" || formatDay(to) != "2026-11-02" {
		t.Fatalf("default range must be the 30 complete days ending yesterday: %s..%s %v", formatDay(from), formatDay(to), err)
	}
	from, to, err = resolveRange(RangeQuery{Days: 7}, today)
	if err != nil || formatDay(from) != "2026-10-27" || formatDay(to) != "2026-11-02" {
		t.Fatalf("7-day range: %s..%s %v", formatDay(from), formatDay(to), err)
	}
	from, to, err = resolveRange(RangeQuery{From: "2026-10-01", To: "2026-11-03"}, today)
	if err != nil || formatDay(from) != "2026-10-01" || formatDay(to) != "2026-11-03" {
		t.Fatalf("custom range may end today: %s..%s %v", formatDay(from), formatDay(to), err)
	}
	for _, query := range []RangeQuery{
		{Days: -1}, {Days: maxDays + 1},
		{From: "2026-11-02", To: "2026-11-01"},
		{From: "2026-11-01", To: "2026-11-04"},
		{From: "2025-01-01", To: "2026-11-01"},
		{From: "11/01", To: "2026-11-01"},
		{From: "2026-11-01"},
	} {
		if _, _, err := resolveRange(query, today); err == nil || err.Error() != ErrorInvalidRange {
			t.Fatalf("range %+v must be rejected, got %v", query, err)
		}
	}
}

func TestSummaryWithoutSyncReturnsEmptyResponse(t *testing.T) {
	service := serviceAt(&fakeStore{}, fixedPrice(2), testNow)
	response, err := service.Summary(context.Background(), Workspace{UserID: "u", AdminAccountID: "a"}, RangeQuery{Days: 7})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if response.Coverage.Synced || response.Previous != nil || len(response.Daily) != 0 || response.Range.From != "2026-10-27" || response.UnitPrice != 2 {
		t.Fatalf("unexpected unsynced response: %+v", response)
	}
}

func TestSummaryConvertsMoneyAndReportsCoverage(t *testing.T) {
	store := &fakeStore{
		synced: true,
		state: syncState{
			Platform: "sub2api", RechargeFrom: dayPtr("2026-01-01"), RechargeTo: dayPtr("2026-11-03"),
			UsageFrom: dayPtr("2026-10-29"), UsageTo: dayPtr("2026-11-03"),
		},
		totals: map[string]rawTotals{
			rangeKey(day("2026-10-27"), day("2026-11-02")): {Credits: 100, Deductions: 5, Events: 7, Payers: 4, RepeatPayers: 1, NewPayers: 2, Registered: 10, RegisteredPaid: 3, Consumption: 80},
			rangeKey(day("2026-10-20"), day("2026-10-26")): {Credits: 50, Payers: 2, NewPayers: 1},
		},
		balances: map[string]float64{"2026-10-26": 40, "2026-11-02": 55},
		channels: []ChannelTotal{{Channel: ChannelPayment, Amount: 90, Events: 6, Users: 3}, {Channel: ChannelAdminDeduct, Amount: -5, Events: 1, Users: 1}},
		daily: []dailyChannelRow{
			{Day: day("2026-10-28"), Channel: ChannelPayment, Amount: 30},
			{Day: day("2026-10-28"), Channel: ChannelAdminDeduct, Amount: -5},
		},
		usage:   []dailyValueRow{{Day: day("2026-10-30"), Value: 12}},
		amounts: []float64{60, 30, 10},
		payers:  []PayerRow{{SiteUserID: "7", Credits: 60, Consumption: 20, Balance: floatPtr(15), ChannelBreakout: []ChannelTotal{{Channel: ChannelPayment, Amount: 60}}}},
	}
	service := serviceAt(store, fixedPrice(0.5), testNow)
	response, err := service.Summary(context.Background(), Workspace{UserID: "u", AdminAccountID: "a"}, RangeQuery{Days: 7})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if response.Range != (DayRange{From: "2026-10-27", To: "2026-11-02", Days: 7}) || response.PreviousRange.From != "2026-10-20" {
		t.Fatalf("unexpected ranges: %+v %+v", response.Range, response.PreviousRange)
	}
	cov := response.Coverage
	if !cov.Synced || cov.Method != RechargeMethodRecords || !cov.RechargeComplete || !cov.PreviousComplete || cov.UsageComplete {
		t.Fatalf("unexpected coverage: %+v", cov)
	}
	cur := response.Current
	if cur.Credits != 50 || cur.Deductions != 2.5 || cur.Payers != 4 || cur.NewPayers == nil || *cur.NewPayers != 2 {
		t.Fatalf("current totals must be converted with the site price: %+v", cur)
	}
	if cur.Consumption != nil {
		t.Fatalf("consumption must be nil when usage data does not cover the range: %v", *cur.Consumption)
	}
	if cur.BalanceStart == nil || *cur.BalanceStart != 20 || cur.BalanceEnd == nil || *cur.BalanceEnd != 27.5 {
		t.Fatalf("balances are taken at the end of the day before and the last day: %+v", cur)
	}
	if response.Previous == nil || response.Previous.Credits != 25 {
		t.Fatalf("previous period must be returned when covered: %+v", response.Previous)
	}
	if response.Channels[0].Amount != 45 || response.Channels[1].Amount != -2.5 {
		t.Fatalf("channel amounts must be converted: %+v", response.Channels)
	}
	if len(response.Daily) != 7 {
		t.Fatalf("one point per day expected, got %d", len(response.Daily))
	}
	oct28 := response.Daily[1]
	if oct28.Day != "2026-10-28" || *oct28.Credits != 15 || *oct28.Deductions != 2.5 || oct28.Channels[ChannelPayment] != 15 || len(oct28.Channels) != 1 {
		t.Fatalf("deductions must stay out of the channel stack: %+v", oct28)
	}
	if response.Daily[0].Consumption != nil || response.Daily[3].Consumption == nil || *response.Daily[3].Consumption != 6 {
		t.Fatalf("consumption must be masked outside the usage coverage: %+v %+v", response.Daily[0], response.Daily[3])
	}
	if math.Abs(response.Concentration.Top1Share-0.6) > 1e-9 || response.Concentration.Top10Share != 1 {
		t.Fatalf("unexpected concentration: %+v", response.Concentration)
	}
	if response.TopPayers[0].Credits != 30 || response.TopPayers[0].Consumption != 10 || *response.TopPayers[0].Balance != 7.5 || response.TopPayers[0].ChannelBreakout[0].Amount != 30 {
		t.Fatalf("payer amounts must be converted: %+v", response.TopPayers[0])
	}
	// 注册时间边界按北京时间切分：10-27 00:00 +08:00 至 11-03 00:00 +08:00。
	if len(store.totalCalls) != 2 || store.totalCalls[0] != "2026-10-27|2026-11-02@2026-10-27T00:00:00+08:00~2026-11-03T00:00:00+08:00" {
		t.Fatalf("unexpected registration bounds: %v", store.totalCalls)
	}
}

func TestSummarySnapshotMethodHidesUnreliableNumbers(t *testing.T) {
	// new-api：首次同步那天只统计了同步之后的部分；首次入账也无法区分老用户。
	store := &fakeStore{
		synced: true,
		state: syncState{
			Platform: "newapi", RechargeFrom: dayPtr("2026-10-27"), RechargeTo: dayPtr("2026-11-03"),
			UsageFrom: dayPtr("2025-11-01"), UsageTo: dayPtr("2026-11-03"),
		},
		totals: map[string]rawTotals{rangeKey(day("2026-10-27"), day("2026-11-02")): {Credits: 10, NewPayers: 3, Consumption: 4}},
	}
	service := serviceAt(store, failingPrice{}, testNow)
	response, err := service.Summary(context.Background(), Workspace{UserID: "u", AdminAccountID: "a"}, RangeQuery{Days: 7})
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if response.UnitPrice != 1 {
		t.Fatalf("a failing price lookup must fall back to 1, got %v", response.UnitPrice)
	}
	if response.Coverage.Method != RechargeMethodSnapshot || response.Coverage.RechargeComplete || response.Coverage.PreviousComplete || !response.Coverage.UsageComplete {
		t.Fatalf("unexpected coverage: %+v", response.Coverage)
	}
	if response.Current.NewPayers != nil || response.Previous != nil {
		t.Fatalf("new payers and the previous period are unknown for snapshot data: %+v %+v", response.Current, response.Previous)
	}
	if response.Current.Consumption == nil || *response.Current.Consumption != 4 {
		t.Fatalf("consumption is still available: %+v", response.Current)
	}
	if response.Daily[0].Credits == nil || *response.Daily[0].Credits != 0 {
		t.Fatalf("the first synced day is shown (partial) like on the user activity page: %+v", response.Daily[0])
	}
}

func TestDistributionAndConcentrationEdgeCases(t *testing.T) {
	if got := concentration(nil); got != (Concentration{}) {
		t.Fatalf("empty concentration: %+v", got)
	}
	buckets := distribution([]float64{1200, 500, 499.99, 50, 9.99, 0.5})
	want := []int{2, 0, 1, 1, 1, 1}
	for i, bucket := range buckets {
		if bucket.Users != want[i] {
			t.Fatalf("bucket %d (min %v) users = %d, want %d: %+v", i, bucket.Min, bucket.Users, want[i], buckets)
		}
	}
	if buckets[5].Max != nil || buckets[0].Max == nil || *buckets[0].Max != 10 {
		t.Fatalf("unexpected bucket bounds: %+v", buckets)
	}
	if math.Abs(buckets[0].Amount-10.49) > 1e-9 {
		t.Fatalf("bucket amounts must be summed: %v", buckets[0].Amount)
	}
}

func floatPtr(value float64) *float64 { return &value }
