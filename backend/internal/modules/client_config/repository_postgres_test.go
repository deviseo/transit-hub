package client_config

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"transithub/backend/internal/database/migrations"
)

// PostgreSQL 测试：需要 TEST_DATABASE_URL，在独立的临时 schema 中执行全部迁移并只写入合成数据，结束后整体删除。
func openClientConfigTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL repository tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adminPool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect PostgreSQL: %v", err)
	}
	schema := fmt.Sprintf("client_config_test_%d", time.Now().UnixNano())
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
	if err := migrations.Run(ctx, pool); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	return pool
}

func TestRepositorySavesAndFindsConfigs(t *testing.T) {
	pool := openClientConfigTestPool(t)
	repo := NewRepository(pool)
	ctx := context.Background()
	ws := Workspace{UserID: "user-1", AdminAccountID: "acct-1"}

	if got, err := repo.Get(ctx, ws); err != nil || got != nil {
		t.Fatalf("empty workspace: %v %+v", err, got)
	}
	notice := time.Date(2026, 11, 4, 8, 0, 0, 0, time.UTC)
	config := Config{
		Workspace:       ws,
		Code:            "abcdefghij",
		Enabled:         true,
		HubURL:          "https://hub.example.com",
		Brand:           Brand{Name: "极速中转", Color: "#2563eb", Logo: "https://cdn.example.com/logo.png"},
		Recommend:       Recommend{Codex: ToolRecommend{Group: "codex", Model: "gpt-6.1-sol"}},
		Notice:          "维护",
		NoticeUpdatedAt: &notice,
		Support:         []SupportLink{{Type: "telegram", Label: "TG", Value: "https://t.me/x"}},
	}
	if err := repo.Save(ctx, config); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByCode(ctx, "abcdefghij")
	if err != nil || got == nil {
		t.Fatalf("by code: %v %+v", err, got)
	}
	if got.Workspace != ws || got.Brand.Name != "极速中转" || got.Recommend.Codex.Model != "gpt-6.1-sol" ||
		len(got.Support) != 1 || got.Support[0].Value != "https://t.me/x" || !got.NoticeUpdatedAt.Equal(notice) || got.UpdatedAt == nil {
		t.Fatalf("round trip: %+v", got)
	}

	// 更新同一工作区：换代码后旧代码查不到。
	config.Code = "kmnpqrstuv"
	config.Enabled = false
	config.Support = nil
	if err := repo.Save(ctx, config); err != nil {
		t.Fatal(err)
	}
	if old, _ := repo.GetByCode(ctx, "abcdefghij"); old != nil {
		t.Fatalf("old code still found")
	}
	got, _ = repo.Get(ctx, ws)
	if got.Code != "kmnpqrstuv" || got.Enabled || len(got.Support) != 0 {
		t.Fatalf("update: %+v", got)
	}

	// 代码全局唯一。
	other := Config{Workspace: Workspace{UserID: "user-2", AdminAccountID: "acct-2"}, Code: "kmnpqrstuv"}
	if err := repo.Save(ctx, other); err == nil {
		t.Fatalf("duplicate code should be rejected")
	}
}
