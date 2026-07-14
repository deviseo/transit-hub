package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

type EmailVerification struct {
	ID        string
	CodeHash  string
	ExpiresAt time.Time
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

func (r *Repository) EnsureSchema(ctx context.Context) error {
	// 当前线上 users 表早期没有密码列；启动时补齐该列，保证注册/登录接口能在旧库上直接运行。
	_, err := r.db.Exec(ctx, `ALTER TABLE users ADD COLUMN IF NOT EXISTS "passwordHash" text NOT NULL DEFAULT ''`)
	return err
}

func (r *Repository) SaveEmailCode(ctx context.Context, email string, codeHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO email_verification_codes (id, email, "codeHash", "expiresAt", "createdAt")
		VALUES ($1, $2, $3, $4, $5)
	`, prefixedID("evc"), email, codeHash, expiresAt, time.Now())
	return err
}

func (r *Repository) LatestEmailCode(ctx context.Context, email string) (*EmailVerification, error) {
	var verification EmailVerification
	err := r.db.QueryRow(ctx, `
		SELECT id, "codeHash", "expiresAt"
		FROM email_verification_codes
		WHERE email = $1 AND "consumedAt" IS NULL
		ORDER BY "createdAt" DESC
		LIMIT 1
	`, email).Scan(&verification.ID, &verification.CodeHash, &verification.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &verification, nil
}

func (r *Repository) ConsumeEmailCode(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE email_verification_codes
		SET "consumedAt" = $2
		WHERE id = $1
	`, id, time.Now())
	return err
}

func (r *Repository) CreateUser(ctx context.Context, email string, passwordHash string) error {
	now := time.Now()
	_, err := r.db.Exec(ctx, `
		INSERT INTO users (id, email, "passwordHash", "emailVerifiedAt", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, $4, $4)
	`, prefixedID("usr"), email, passwordHash, now)
	return err
}

// CountUsers 返回 users 表中的用户总数，用于管理员初始化判断
func (r *Repository) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count)
	return count, err
}

func (r *Repository) PasswordHashByEmail(ctx context.Context, email string) (string, error) {
	var passwordHash string
	err := r.db.QueryRow(ctx, `SELECT "passwordHash" FROM users WHERE email = $1`, email).Scan(&passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return passwordHash, err
}

func (r *Repository) UserIDByEmail(ctx context.Context, email string) (string, error) {
	var id string
	err := r.db.QueryRow(ctx, `SELECT id FROM users WHERE email = $1`, email).Scan(&id)
	return id, err
}

func (r *Repository) CreateSession(ctx context.Context, userID string, tokenHash string, expiresAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO auth_sessions (id, "tokenHash", "userId", "expiresAt", "createdAt")
		VALUES ($1, $2, $3, $4, $5)
	`, prefixedID("ses"), tokenHash, userID, expiresAt, time.Now())
	return err
}

func (r *Repository) UserIDBySessionToken(ctx context.Context, tokenHash string) (string, error) {
	var userID string
	err := r.db.QueryRow(ctx, `
		SELECT "userId"
		FROM auth_sessions
		WHERE "tokenHash" = $1 AND "expiresAt" > $2
	`, tokenHash, time.Now()).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return userID, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

func prefixedID(prefix string) string {
	data := make([]byte, 12)
	if _, err := rand.Read(data); err != nil {
		return prefix + "_fallback_" + hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return prefix + "_" + hex.EncodeToString(data)
}
