package httpserver

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/profit"
	"transithub/backend/internal/modules/settings"
	"transithub/backend/internal/modules/upstream"
)

// 站点同步后的预警测试：策略、机器人都必须取站点所属工作区的配置。数据均为合成数据。

type fakeStrategyReader map[string]settings.StrategySettings

func (f fakeStrategyReader) GetStrategyForWorkspace(ctx context.Context, userID string, adminAccountID string) (settings.StrategySettings, error) {
	if strategy, ok := f[userID+"|"+adminAccountID]; ok {
		return strategy, nil
	}
	return settings.StrategySettings{}, nil
}

type sentAlert struct {
	workspace string
	botIDs    []string
	message   string
}

type fakeAlertNotifier struct{ sent []sentAlert }

func (f *fakeAlertNotifier) SendFormattedToWorkspaceBots(ctx context.Context, userID string, adminAccountID string, botIDs []string, message string, format settings.NotificationTemplateFormat) {
	f.sent = append(f.sent, sentAlert{workspace: userID + "|" + adminAccountID, botIDs: botIDs, message: message})
}

// fakePriceAssessor 记录调价估算请求，并返回固定的估算结果（日均成本 ¥20，每天多 ¥4）。
type fakePriceAssessor struct{ calls []string }

func (f *fakePriceAssessor) AssessPriceChanges(ctx context.Context, ws profit.Workspace, siteID string, changes []profit.PriceChangeInput) []profit.PriceChange {
	results := make([]profit.PriceChange, 0, len(changes))
	for _, change := range changes {
		f.calls = append(f.calls, ws.AdminAccountID+"|"+siteID+"|"+change.Group.Name)
		average, impact := 20.0, 4.0
		results = append(results, profit.PriceChange{EstimateStatus: profit.EstimateOK, WindowDays: 7, AvgDailyCost: &average, DailyImpact: &impact})
	}
	return results
}

type fakeAlertSites map[string]*upstream.Site

func (f fakeAlertSites) GetSite(ctx context.Context, siteID string) (*upstream.Site, error) {
	if site, ok := f[siteID]; ok {
		return site, nil
	}
	return nil, errors.New("not found")
}

func balanceMetrics(value float64) upstream.Metrics {
	return upstream.Metrics{Balance: upstream.MetricValue{Value: &value}}
}

func groupMetrics(multiplier float64) upstream.Metrics {
	return upstream.Metrics{Groups: []upstream.GroupInfo{{ID: "1", Name: "vip", Multiplier: &multiplier}}}
}

func TestSyncAlertsUseTheSitesWorkspaceStrategyAndBots(t *testing.T) {
	notifier := &fakeAlertNotifier{}
	alerts := syncAlerts{
		strategies: fakeStrategyReader{
			"user-1|ws-a": {EnableBalanceWarning: true, DefaultBalanceThreshold: 1000, BalanceNotifyBotIDs: []string{"bot-a"}},
			"user-1|ws-b": {EnableBalanceWarning: true, DefaultBalanceThreshold: 10, BalanceNotifyBotIDs: []string{"bot-b"}, EnableMultiplierAlert: true, MultiplierNotifyBotIDs: []string{"bot-b-rate"}},
		},
		notifier: notifier,
		sites: fakeAlertSites{
			"site-a": {ID: "site-a", RechargeRate: 1},
			"site-b": {ID: "site-b", RechargeRate: 2},
		},
		cooldown: newMemoryAlertCooldown(),
	}
	ctx := context.Background()

	// ws-b 的阈值是 ¥10：余额 $20 × 充值倍率 2 = ¥40，不预警；ws-a 的阈值 ¥1000 不能被借用。
	alerts.AfterSync(ctx, "user-1", "ws-b", "site-b", "Site B", balanceMetrics(25), balanceMetrics(20))
	if len(notifier.sent) != 0 {
		t.Fatalf("ws-b must use its own threshold: %+v", notifier.sent)
	}
	alerts.AfterSync(ctx, "user-1", "ws-b", "site-b", "Site B", balanceMetrics(20), balanceMetrics(4))
	if len(notifier.sent) != 1 || notifier.sent[0].workspace != "user-1|ws-b" || notifier.sent[0].botIDs[0] != "bot-b" {
		t.Fatalf("low balance must alert ws-b's bots in ws-b: %+v", notifier.sent)
	}
	alerts.AfterSync(ctx, "user-1", "ws-a", "site-a", "Site A", balanceMetrics(600), balanceMetrics(500))
	if len(notifier.sent) != 2 || notifier.sent[1].workspace != "user-1|ws-a" || notifier.sent[1].botIDs[0] != "bot-a" {
		t.Fatalf("ws-a must alert with ws-a's bots: %+v", notifier.sent)
	}

	// 倍率变更只在开启了倍率预警的工作区发送。
	alerts.AfterSync(ctx, "user-1", "ws-b", "site-b", "Site B", groupMetrics(0.5), groupMetrics(0.6))
	alerts.AfterSync(ctx, "user-1", "ws-a", "site-a", "Site A", groupMetrics(0.5), groupMetrics(0.6))
	rateAlerts := 0
	for _, alert := range notifier.sent {
		if len(alert.botIDs) == 1 && alert.botIDs[0] == "bot-b-rate" {
			rateAlerts++
		}
	}
	if rateAlerts != 1 || len(notifier.sent) != 3 {
		t.Fatalf("multiplier alerts must follow each workspace's switch: %+v", notifier.sent)
	}

	// 缺少工作区时不发送任何通知。
	alerts.AfterSync(ctx, "user-1", "", "site-b", "Site B", balanceMetrics(20), balanceMetrics(1))
	if len(notifier.sent) != 3 {
		t.Fatalf("syncs without a workspace must not alert: %+v", notifier.sent)
	}
}

func TestSyncAlertsBalanceCooldown(t *testing.T) {
	notifier := &fakeAlertNotifier{}
	cooldown := newMemoryAlertCooldown()
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	cooldown.now = func() time.Time { return now }
	alerts := syncAlerts{
		strategies: fakeStrategyReader{"user-1|ws-a": {EnableBalanceWarning: true, DefaultBalanceThreshold: 100, BalanceNotifyBotIDs: []string{"bot-a"}}},
		notifier:   notifier,
		sites:      fakeAlertSites{"site-a": {ID: "site-a", RechargeRate: 1}},
		cooldown:   cooldown,
	}
	ctx := context.Background()
	afterSync := func(balance float64) {
		alerts.AfterSync(ctx, "user-1", "ws-a", "site-a", "Site A", balanceMetrics(balance), balanceMetrics(balance))
	}

	afterSync(50)
	afterSync(40)
	if len(notifier.sent) != 1 {
		t.Fatalf("repeat alerts within the cooldown must be suppressed: %d", len(notifier.sent))
	}
	now = now.Add(balanceAlertCooldown + time.Minute)
	afterSync(40)
	if len(notifier.sent) != 2 {
		t.Fatalf("a still-low balance must alert again after the cooldown: %d", len(notifier.sent))
	}
	afterSync(150) // 恢复：清除冷却
	afterSync(30)
	if len(notifier.sent) != 3 {
		t.Fatalf("a new drop after recovery must alert immediately: %d", len(notifier.sent))
	}
}

func TestRedisAlertCooldownFallsBackWithoutRedis(t *testing.T) {
	cooldown := newRedisAlertCooldown(nil)
	ctx := context.Background()
	if !cooldown.Acquire(ctx, "balance:site-1", time.Hour) || cooldown.Acquire(ctx, "balance:site-1", time.Hour) {
		t.Fatal("the in-process fallback must enforce the cooldown")
	}
	cooldown.Reset(ctx, "balance:site-1")
	if !cooldown.Acquire(ctx, "balance:site-1", time.Hour) {
		t.Fatal("reset must clear the cooldown")
	}
}

func TestMultiplierChangesAreAssessedAndIncludeCostImpact(t *testing.T) {
	notifier := &fakeAlertNotifier{}
	assessor := &fakePriceAssessor{}
	alerts := syncAlerts{
		strategies: fakeStrategyReader{
			"user-1|ws-b": {EnableMultiplierAlert: true, MultiplierNotifyBotIDs: []string{"bot-b"}},
			"user-1|ws-c": {EnableMultiplierAlert: true, MultiplierNotifyBotIDs: []string{"bot-c"}, MultiplierTemplate: "{siteName} {groupName} {oldRate}->{newRate}", MultiplierTemplateFormat: settings.NotificationTemplateFormatMarkdown},
		},
		notifier: notifier,
		sites:    fakeAlertSites{},
		cooldown: newMemoryAlertCooldown(),
		prices:   assessor,
	}
	ctx := context.Background()

	// 关闭了倍率预警的工作区：仍然估算并记录（利润分析页使用），但不发通知。
	alerts.AfterSync(ctx, "user-1", "ws-a", "site-a", "Site A", groupMetrics(1), groupMetrics(1.2))
	if len(assessor.calls) != 1 || assessor.calls[0] != "ws-a|site-a|vip" || len(notifier.sent) != 0 {
		t.Fatalf("changes are assessed even without alerts: %v %+v", assessor.calls, notifier.sent)
	}
	// 倍率没有变化：不估算。
	alerts.AfterSync(ctx, "user-1", "ws-a", "site-a", "Site A", groupMetrics(1.2), groupMetrics(1.2))
	if len(assessor.calls) != 1 {
		t.Fatalf("unchanged multipliers are not assessed: %v", assessor.calls)
	}

	// 默认模板带 {costImpact}。
	alerts.AfterSync(ctx, "user-1", "ws-b", "site-b", "Site B", groupMetrics(1), groupMetrics(1.2))
	if len(notifier.sent) != 1 || !strings.Contains(notifier.sent[0].message, "💸 成本影响：按该分组近 7 天日均成本 ¥20.00 估算，每天约多 ¥4.00") {
		t.Fatalf("the default template shows the cost impact: %+v", notifier.sent)
	}
	// 之前保存的模板没有 {costImpact}：按格式追加在末尾。
	alerts.AfterSync(ctx, "user-1", "ws-c", "site-c", "Site C", groupMetrics(1), groupMetrics(0.8))
	if len(notifier.sent) != 2 || notifier.sent[1].message != "Site C vip 1.0000->0.8000\n\n💸 **成本影响：** 按该分组近 7 天日均成本 ¥20.00 估算，每天约多 ¥4.00" {
		t.Fatalf("older templates get the impact appended: %q", notifier.sent[1].message)
	}

	if got := formatMultiplierChange("S", "<g>", 1, 2, "a<b", "<b>{groupName}</b>", settings.NotificationTemplateFormatHTML); got != `<b><g></b><p style="margin:10px 0 0">💸 成本影响：a&lt;b</p>` {
		t.Fatalf("html templates get an escaped paragraph: %q", got)
	}
	if got := formatMultiplierChange("S", "g", 1, 2, "", "{costImpact}", settings.NotificationTemplateFormatText); got != costImpactUnavailable {
		t.Fatalf("missing estimates render a placeholder: %q", got)
	}
	if got := formatMultiplierChange("S", "g", 1, 2, "", "{siteName}", settings.NotificationTemplateFormatText); got != "S" {
		t.Fatalf("nothing is appended without an estimate: %q", got)
	}
}
