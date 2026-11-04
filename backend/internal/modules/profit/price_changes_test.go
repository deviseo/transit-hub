package profit

import (
	"context"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// 调价影响估算测试：上游分组消费为合成数据。

func groupChange(id, name string, oldRate, newRate float64) PriceChangeInput {
	return PriceChangeInput{Group: upstream.GroupInfo{ID: id, Name: name}, OldMultiplier: oldRate, NewMultiplier: newRate}
}

func TestAssessPriceChanges(t *testing.T) {
	store := newMemoryStore()
	ranges := []string{}
	// 北京时间 2026-11-03 10:00：估算窗口是 10-27 ~ 11-02 这 7 个完整自然日。
	now := time.Date(2026, 11, 3, 2, 0, 0, 0, time.UTC)
	service := &Service{store: store, upstreams: fakeUpstreams{rate: 2, groupConsumes: map[string]float64{"5|claude": 70, "6|gpt": 0}, ranges: &ranges}, now: func() time.Time { return now }, running: map[Workspace]struct{}{}}

	results := service.AssessPriceChanges(context.Background(), ws, "s1", []PriceChangeInput{
		groupChange("5", "claude", 1, 1.2),
		groupChange("6", "gpt", 1, 0.8),
		groupChange("7", "missing", 1, 2),
		groupChange("8", "fresh", 0, 1),
	})
	if len(results) != 4 || len(store.prices) != 4 {
		t.Fatalf("every change is recorded: %+v", results)
	}
	if len(ranges) != 1 || ranges[0] != "2026-10-27..2026-11-02@Asia/Shanghai#3" {
		t.Fatalf("only groups with a previous rate are queried, over the last 7 complete days: %v", ranges)
	}
	claude := results[0]
	// 日均成本 = 70 ÷ 7 × 充值倍率 2 = ¥20；涨价 20% → 每天多 ¥4。
	if claude.EstimateStatus != EstimateOK || !near(*claude.AvgDailyCost, 20) || !near(*claude.DailyImpact, 4) || claude.UpstreamKey != "alpha.example" || claude.ID == 0 || !claude.DetectedAt.Equal(now) {
		t.Fatalf("unexpected claude estimate: %+v", claude)
	}
	if text := claude.ImpactText(); text != "按该分组近 7 天日均成本 ¥20.00 估算，每天约多 ¥4.00" {
		t.Fatalf("unexpected impact text: %q", text)
	}
	if gpt := results[1]; gpt.EstimateStatus != EstimateNoUsage || *gpt.DailyImpact != 0 || !strings.Contains(gpt.ImpactText(), "没有使用该分组") {
		t.Fatalf("unused groups have no impact: %+v", gpt)
	}
	if results[2].EstimateStatus != EstimateFailed || results[2].WindowConsume != nil || results[3].EstimateStatus != EstimateFailed {
		t.Fatalf("failed or unknown estimates are marked: %+v %+v", results[2], results[3])
	}
	if !strings.Contains(results[2].ImpactText(), "无法估算") {
		t.Fatalf("unexpected failed text: %q", results[2].ImpactText())
	}

	// 降价：每天节省。
	cheaper := estimatePriceChange(groupChange("5", "claude", 1.2, 0.9), upstream.SiteGroupConsumes{RechargeRate: 1, Consumes: map[string]float64{"5|claude": 84}})
	if !near(*cheaper.DailyImpact, -3) || cheaper.ImpactText() != "按该分组近 7 天日均成本 ¥12.00 估算，每天约省 ¥3.00" {
		t.Fatalf("price cuts save money: %+v %q", cheaper, cheaper.ImpactText())
	}
	// 没有设置充值倍率的站点无法换算成本。
	noRate := estimatePriceChange(groupChange("5", "claude", 1, 2), upstream.SiteGroupConsumes{Consumes: map[string]float64{"5|claude": 84}})
	if noRate.EstimateStatus != EstimateNoRate || noRate.DailyImpact != nil {
		t.Fatalf("sites without a recharge rate cannot be estimated: %+v", noRate)
	}

	// 站点不存在：不记录。
	if results := service.AssessPriceChanges(context.Background(), ws, "gone", []PriceChangeInput{groupChange("5", "claude", 1, 2)}); results != nil || len(store.prices) != 4 {
		t.Fatalf("unknown sites are skipped: %+v", results)
	}
}

func TestPriceInsightsAndFormatting(t *testing.T) {
	now := time.Date(2026, 11, 3, 2, 0, 0, 0, time.UTC)
	impact := 4.0
	changes := []PriceChange{ // 最新的在前
		{SiteID: "s1", SiteName: "Alpha", GroupID: "5", GroupName: "claude", OldMultiplier: 1.2, NewMultiplier: 1.5, DailyImpact: &impact, DetectedAt: now.Add(-time.Hour)},
		{SiteID: "s1", SiteName: "Alpha", GroupID: "5", GroupName: "claude", OldMultiplier: 1, NewMultiplier: 1.2, DetectedAt: now.Add(-48 * time.Hour)},
		{SiteID: "s1", SiteName: "Alpha", GroupID: "6", GroupName: "gpt", OldMultiplier: 1, NewMultiplier: 0.8, DetectedAt: now.Add(-time.Hour)},
		{SiteID: "s2", SiteName: "Beta", GroupID: "1", GroupName: "vip", OldMultiplier: 1, NewMultiplier: 2, DetectedAt: now.Add(-8 * 24 * time.Hour)},
		{SiteID: "s3", SiteName: "Gamma", GroupID: "2", GroupName: "idle", OldMultiplier: 1, NewMultiplier: 2, EstimateStatus: EstimateNoUsage, DetectedAt: now.Add(-time.Hour)},
	}
	insights := priceInsights(changes, now)
	if len(insights) != 1 || insights[0].Kind != InsightPriceIncrease || insights[0].Subject != "Alpha / claude" || *insights[0].Value != 4 || !near(*insights[0].Extra, 0.25) {
		t.Fatalf("only the latest recent increase per used group is an insight: %+v", insights)
	}
	if formatYuan(1234567.891) != "¥1,234,567.89" || formatYuan(-0.5) != "-¥0.50" || formatYuan(0) != "¥0.00" {
		t.Fatalf("unexpected yuan formatting: %s %s", formatYuan(1234567.891), formatYuan(-0.5))
	}
}
