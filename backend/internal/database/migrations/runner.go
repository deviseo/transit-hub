package migrations

import (
	"context"
	"embed"
	"fmt"
	"log"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed *.sql
var migrationFiles embed.FS

// Run 按文件名顺序执行所有未应用的 SQL 迁移。
// 每个迁移在独立事务中执行，已执行的版本记录在 schema_migrations 表中。
func Run(ctx context.Context, db *pgxpool.Pool) error {
	// 先确保 schema_migrations 表存在（不走迁移记录，直接执行）
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return fmt.Errorf("migrations: create schema_migrations table: %w", err)
	}

	// 读取所有嵌入的 SQL 文件
	entries, err := migrationFiles.ReadDir(".")
	if err != nil {
		return fmt.Errorf("migrations: read embedded dir: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// 获取已执行的版本集合
	applied, err := appliedVersions(ctx, db)
	if err != nil {
		return fmt.Errorf("migrations: query applied versions: %w", err)
	}

	for _, file := range files {
		version := strings.TrimSuffix(file, ".sql")
		if applied[version] {
			continue
		}

		sql, err := migrationFiles.ReadFile(file)
		if err != nil {
			return fmt.Errorf("migrations: read file %s: %w", file, err)
		}

		// 事务执行迁移
		tx, err := db.Begin(ctx)
		if err != nil {
			return fmt.Errorf("migrations: begin tx for %s: %w", file, err)
		}

		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrations: exec %s: %w", file, err)
		}

		if _, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version) VALUES ($1)", version); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("migrations: record version %s: %w", file, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("migrations: commit %s: %w", file, err)
		}

		log.Printf("[migrations] applied %s", file)
	}

	return nil
}

// ensureMigrationsTable 确保 schema_migrations 表存在。
// 这个建表语句不记录到迁移历史中，因为它是迁移系统自身的基础设施。
func ensureMigrationsTable(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    TEXT        PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	return err
}

// appliedVersions 返回已执行迁移版本的集合
func appliedVersions(ctx context.Context, db *pgxpool.Pool) (map[string]bool, error) {
	rows, err := db.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		result[v] = true
	}
	return result, rows.Err()
}
