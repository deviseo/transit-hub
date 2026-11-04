package settings

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgreSQL 测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中只写入合成数据，结束后整体删除。
func openSettingsTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("settings_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err := adminPool.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		adminPool.Close()
		t.Fatalf("create schema: %v", err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		adminPool.Close()
		t.Fatalf("parse config: %v", err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		adminPool.Close()
		t.Fatalf("connect schema: %v", err)
	}
	t.Cleanup(func() {
		pool.Close()
		if _, err := adminPool.Exec(context.Background(), "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Errorf("drop schema: %v", err)
		}
		adminPool.Close()
	})
	return pool
}

type staticAccounts map[string]string

func (a staticAccounts) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	return a[userID], nil
}

// 每个工作区的策略、机器人互相独立：后台任务按数据所属的工作区读取和发送，与用户当前选中的工作区无关。
func TestWorkspaceStrategiesAndBotsArePerWorkspace(t *testing.T) {
	pool := openSettingsTestPool(t)
	ctx := context.Background()
	repo := NewRepository(pool)
	if err := repo.EnsureSchema(ctx); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}

	var mu sync.Mutex
	hits := map[string][]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path] = append(hits[r.URL.Path], r.Method)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	service := NewService(server.Client(), repo)
	// 用户当前选中的是 ws-a；ws-b 的后台通知仍必须走 ws-b 自己的机器人。
	service.SetAdminAccountResolver(staticAccounts{"user-1": "ws-a"})
	changed := []string{}
	service.OnStrategyChanged = func(userID string, adminAccountID string, settings StrategySettings) {
		changed = append(changed, fmt.Sprintf("%s|%s|%v", userID, adminAccountID, settings.EnableRefreshInterval))
	}

	channelsA := NotificationChannelSettings{Wecom: []WebhookChannelSettings{{ID: "bot-a", Name: "A", Enabled: true, Webhook: server.URL + "/ws-a"}}}
	channelsB := NotificationChannelSettings{Wecom: []WebhookChannelSettings{{ID: "bot-b", Name: "B", Enabled: true, Webhook: server.URL + "/ws-b"}}}
	if err := repo.SaveNotificationChannels(ctx, "user-1", "ws-a", channelsA); err != nil {
		t.Fatalf("save channels a: %v", err)
	}
	if err := repo.SaveNotificationChannels(ctx, "user-1", "ws-b", channelsB); err != nil {
		t.Fatalf("save channels b: %v", err)
	}
	if err := repo.SaveStrategy(ctx, "user-1", "ws-b", StrategySettings{EnableRefreshInterval: true, RefreshInterval: 600, EnableBalanceWarning: true, DefaultBalanceThreshold: 50, BalanceNotifyBotIDs: []string{"bot-b"}}); err != nil {
		t.Fatalf("save strategy b: %v", err)
	}
	if _, err := service.SaveStrategy(ctx, "user-1", StrategySettings{EnableRefreshInterval: false, RefreshInterval: 10}); err != nil {
		t.Fatalf("save strategy a: %v", err)
	}
	if len(changed) != 1 || changed[0] != "user-1|ws-a|false" {
		t.Fatalf("strategy changes must report the saved workspace: %v", changed)
	}
	// 尚未归属工作区的旧行不参与恢复。
	if _, err := pool.Exec(ctx, `INSERT INTO strategy_settings (user_id, admin_account_id, settings) VALUES ('user-legacy', '', '{"enableRefreshInterval": true}')`); err != nil {
		t.Fatalf("insert legacy row: %v", err)
	}

	strategies, err := service.ListStrategies(ctx)
	if err != nil || len(strategies) != 2 {
		t.Fatalf("unexpected strategies: %+v %v", strategies, err)
	}
	if strategies[0].AdminAccountID != "ws-a" || strategies[0].Settings.RefreshInterval != minRefreshIntervalSeconds {
		t.Fatalf("listed strategies must be normalized: %+v", strategies[0])
	}
	if strategies[1].AdminAccountID != "ws-b" || !strategies[1].Settings.EnableRefreshInterval || strategies[1].Settings.RefreshInterval != 600 {
		t.Fatalf("unexpected ws-b strategy: %+v", strategies[1])
	}

	strategyB, err := service.GetStrategyForWorkspace(ctx, "user-1", "ws-b")
	if err != nil || strategyB.DefaultBalanceThreshold != 50 || len(strategyB.BalanceNotifyBotIDs) != 1 {
		t.Fatalf("unexpected ws-b strategy: %+v %v", strategyB, err)
	}
	if missing, err := service.GetStrategyForWorkspace(ctx, "user-1", "ws-missing"); err != nil || missing.EnableBalanceWarning || missing.RefreshInterval != minRefreshIntervalSeconds {
		t.Fatalf("missing workspaces must use defaults: %+v %v", missing, err)
	}

	service.SendFormattedToWorkspaceBots(ctx, "user-1", "ws-b", strategyB.BalanceNotifyBotIDs, "余额预警", NotificationTemplateFormatText)
	// ws-a 的机器人 ID 在 ws-b 中不存在：不能借用用户当前工作区的同名配置发出去。
	service.SendFormattedToWorkspaceBots(ctx, "user-1", "ws-b", []string{"bot-a"}, "should not send", NotificationTemplateFormatText)
	service.SendFormattedToWorkspaceBots(ctx, "user-1", "", []string{"bot-a"}, "should not send", NotificationTemplateFormatText)
	mu.Lock()
	defer mu.Unlock()
	if len(hits["/ws-b"]) != 1 || len(hits["/ws-a"]) != 0 {
		t.Fatalf("notifications must use the target workspace's bots only: %v", hits)
	}
}
