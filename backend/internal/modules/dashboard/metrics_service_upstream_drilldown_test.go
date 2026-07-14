package dashboard

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

// fakeUpstreamLister 是 UpstreamLister 的桩实现，只有测试用到的方法有真实行为。
type fakeUpstreamLister struct {
	keyUsageItems []upstream.KeyUsageTodayItem
	keyUsageErr   error
	balanceItems  []upstream.BalanceBreakdownItem
	balanceErr    error
}

func (f *fakeUpstreamLister) List(ctx context.Context, userID string) []upstream.Response {
	return nil
}

func (f *fakeUpstreamLister) ListForAccount(ctx context.Context, userID, adminAccountID string) []upstream.Response {
	return nil
}

func (f *fakeUpstreamLister) KeyUsageToday(ctx context.Context, userID string) ([]upstream.KeyUsageTodayItem, error) {
	return f.keyUsageItems, f.keyUsageErr
}

func (f *fakeUpstreamLister) BalanceBreakdown(ctx context.Context, userID string) ([]upstream.BalanceBreakdownItem, error) {
	return f.balanceItems, f.balanceErr
}

// TestUpstreamKeyUsageToday_SortsDescendingAndSumsTotal 覆盖测试要求 7、8：
// 按 todayAmount 降序排序；total 等于所有 keys[].todayAmount 求和。
func TestUpstreamKeyUsageToday_SortsDescendingAndSumsTotal(t *testing.T) {
	upstreams := &fakeUpstreamLister{
		keyUsageItems: []upstream.KeyUsageTodayItem{
			{SiteID: "site-a", KeyID: "1", KeyName: "low", TodayAmount: 5, RawAmount: 2.5, RechargeRate: 2},
			{SiteID: "site-a", KeyID: "2", KeyName: "high", TodayAmount: 66.6, RawAmount: 33.3, RechargeRate: 2},
			{SiteID: "site-b", KeyID: "3", KeyName: "mid", TodayAmount: 20, RawAmount: 10, RechargeRate: 2},
		},
	}
	service := NewMetricsService(nil, nil, upstreams, nil, nil)

	resp, err := service.UpstreamKeyUsageToday(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(resp.Keys))
	}
	if resp.Keys[0].KeyName != "high" || resp.Keys[1].KeyName != "mid" || resp.Keys[2].KeyName != "low" {
		t.Fatalf("keys not sorted descending by todayAmount: %+v", resp.Keys)
	}
	wantTotal := 5 + 66.6 + 20.0
	if resp.Total != wantTotal {
		t.Fatalf("total = %.4f, want %.4f (sum of keys[].todayAmount)", resp.Total, wantTotal)
	}
}

// TestUpstreamKeyUsageToday_PropagatesUpstreamError 覆盖测试要求 9：
// 任一上游站点失败时，接口整体返回错误，不静默降级为 0。
func TestUpstreamKeyUsageToday_PropagatesUpstreamError(t *testing.T) {
	upstreams := &fakeUpstreamLister{keyUsageErr: errors.New("upstream site unreachable")}
	service := NewMetricsService(nil, nil, upstreams, nil, nil)

	_, err := service.UpstreamKeyUsageToday(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error to propagate from upstream.Service.KeyUsageToday, got nil")
	}
}

// TestUpstreamBalanceBreakdown_SortsWithUnknownBalanceLastAndSumsKnownOnly 覆盖测试要求 7、8：
// 按 balance 降序排序，未知余额站点排在最后；total 只对已知余额求和。
func TestUpstreamBalanceBreakdown_SortsWithUnknownBalanceLastAndSumsKnownOnly(t *testing.T) {
	high := 100.0
	low := 10.0
	upstreams := &fakeUpstreamLister{
		balanceItems: []upstream.BalanceBreakdownItem{
			{SiteID: "site-unknown", Balance: nil},
			{SiteID: "site-low", Balance: &low},
			{SiteID: "site-high", Balance: &high},
		},
	}
	service := NewMetricsService(nil, nil, upstreams, nil, nil)

	resp, err := service.UpstreamBalanceBreakdown(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Sites) != 3 {
		t.Fatalf("expected 3 sites, got %d", len(resp.Sites))
	}
	if resp.Sites[0].SiteID != "site-high" || resp.Sites[1].SiteID != "site-low" || resp.Sites[2].SiteID != "site-unknown" {
		t.Fatalf("sites not sorted descending with unknown balance last: %+v", resp.Sites)
	}
	if resp.Total != 110 {
		t.Fatalf("total = %.2f, want 110.00 (100 + 10, unknown balance excluded)", resp.Total)
	}
}

// TestUpstreamBalanceBreakdown_PropagatesUpstreamError 补充：底层错误直接透传，不吞掉。
func TestUpstreamBalanceBreakdown_PropagatesUpstreamError(t *testing.T) {
	upstreams := &fakeUpstreamLister{balanceErr: errors.New("cache read failed")}
	service := NewMetricsService(nil, nil, upstreams, nil, nil)

	_, err := service.UpstreamBalanceBreakdown(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error to propagate from upstream.Service.BalanceBreakdown, got nil")
	}
}
