package leaderboard

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS leaderboard_embed_configs (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			embed_token text NOT NULL,
			sub2api_source_origin text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_leaderboard_embed_configs_workspace ON leaderboard_embed_configs (user_id, admin_account_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_leaderboard_embed_configs_token ON leaderboard_embed_configs (embed_token)`,
	}
	for _, statement := range statements {
		if _, err := r.db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) GetEmbedConfigByToken(ctx context.Context, embedToken string) (*EmbedConfig, error) {
	row := r.db.QueryRow(ctx, `SELECT user_id, admin_account_id, embed_token, sub2api_source_origin, created_at, updated_at FROM leaderboard_embed_configs WHERE embed_token = $1`, embedToken)
	return scanEmbedConfig(row)
}

func (r *Repository) GetEmbedConfigByWorkspace(ctx context.Context, userID string, adminAccountID string) (*EmbedConfig, error) {
	row := r.db.QueryRow(ctx, `SELECT user_id, admin_account_id, embed_token, sub2api_source_origin, created_at, updated_at FROM leaderboard_embed_configs WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID)
	return scanEmbedConfig(row)
}

func (r *Repository) InsertEmbedConfig(ctx context.Context, config EmbedConfig) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO leaderboard_embed_configs (user_id, admin_account_id, embed_token, sub2api_source_origin, created_at, updated_at)
		VALUES ($1, $2, $3, $4, now(), now())
		ON CONFLICT (user_id, admin_account_id) DO NOTHING
	`, config.UserID, config.AdminAccountID, config.EmbedToken, config.Sub2apiSourceOrigin)
	return err
}

func (r *Repository) UpdateEmbedConfig(ctx context.Context, userID string, adminAccountID string, origin string) error {
	_, err := r.db.Exec(ctx, `UPDATE leaderboard_embed_configs SET sub2api_source_origin = $3, updated_at = now() WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID, origin)
	return err
}

func (r *Repository) RotateEmbedToken(ctx context.Context, userID string, adminAccountID string, newToken string) error {
	_, err := r.db.Exec(ctx, `UPDATE leaderboard_embed_configs SET embed_token = $3, updated_at = now() WHERE user_id = $1 AND admin_account_id = $2`, userID, adminAccountID, newToken)
	return err
}

func scanEmbedConfig(row pgx.Row) (*EmbedConfig, error) {
	var config EmbedConfig
	if err := row.Scan(&config.UserID, &config.AdminAccountID, &config.EmbedToken, &config.Sub2apiSourceOrigin, &config.CreatedAt, &config.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &config, nil
}

func randomToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
