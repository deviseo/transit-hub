package my_sites

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"transithub/backend/internal/shared/secretbox"
)

type Repository struct {
	db      *pgxpool.Pool
	secrets *secretbox.Box
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// SetSecretBox 设置加密会话用的密钥（nil 表示不加密）。要在读写之前设置。
func (r *Repository) SetSecretBox(box *secretbox.Box) {
	r.secrets = box
}

// sessionAAD 把密文绑定到所属用户。不绑定工作区：旧版本的行在补齐工作区时会改 admin_account_id。
func sessionAAD(userID string) string {
	return "my_site_states/" + userID
}

// EnsureSchema 只维护会话所需的列。旧版本留下的 mappings / own_groups 列（已下线的调价映射功能）
// 保持原样、不删除，也不再读写；它们带默认值，不影响新行写入。
func (r *Repository) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS my_site_states (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			base_url text NOT NULL,
			email text NOT NULL,
			session jsonb NOT NULL,
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`ALTER TABLE my_site_states ADD COLUMN IF NOT EXISTS admin_account_id text NOT NULL DEFAULT ''`,
		`ALTER TABLE my_site_states DROP CONSTRAINT IF EXISTS my_site_states_pkey`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_my_site_states_workspace ON my_site_states (user_id, admin_account_id)`,
	}
	for _, statement := range statements {
		if _, err := r.db.Exec(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, userID string, adminAccountID string) (*State, error) {
	var state State
	var sessionJSON []byte
	err := r.db.QueryRow(ctx, `
		SELECT user_id, admin_account_id, base_url, email, session
		FROM my_site_states
		WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID).Scan(&state.UserID, &state.AdminAccountID, &state.BaseURL, &state.Email, &sessionJSON)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sessionJSON, err = r.secrets.OpenJSON(sessionJSON, sessionAAD(userID))
	if err != nil {
		// 读不出来（例如密钥丢了）：当作没有保存会话，提示重新登录站点。
		secretbox.LogUnreadable(sessionAAD(userID)+"/"+adminAccountID, err)
		return nil, nil
	}
	if err := json.Unmarshal(sessionJSON, &state.Session); err != nil {
		return nil, err
	}
	return &state, nil
}

// Save 只更新会话相关列，同一行里的其他列保持不变。
func (r *Repository) Save(ctx context.Context, state State) error {
	sessionJSON, err := json.Marshal(state.Session)
	if err != nil {
		return err
	}
	sessionJSON, err = r.secrets.SealJSON(sessionJSON, sessionAAD(state.UserID))
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO my_site_states (user_id, admin_account_id, base_url, email, session, updated_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET
			base_url = EXCLUDED.base_url,
			email = EXCLUDED.email,
			session = EXCLUDED.session,
			updated_at = EXCLUDED.updated_at
	`, state.UserID, state.AdminAccountID, state.BaseURL, state.Email, string(sessionJSON))
	return err
}

// ListSessionWorkspaces 返回保存了管理员会话、且工作区仍然存在的工作区（不校验会话是否仍然有效）。
func (r *Repository) ListSessionWorkspaces(ctx context.Context) ([]SessionWorkspace, error) {
	rows, err := r.db.Query(ctx, `
		SELECT s.user_id, s.admin_account_id FROM my_site_states s
		JOIN admin_accounts a ON a.user_id = s.user_id AND a.id = s.admin_account_id
		WHERE s.admin_account_id <> ''
		ORDER BY s.user_id, s.admin_account_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	workspaces := make([]SessionWorkspace, 0)
	for rows.Next() {
		var ws SessionWorkspace
		if err := rows.Scan(&ws.UserID, &ws.AdminAccountID); err != nil {
			return nil, err
		}
		workspaces = append(workspaces, ws)
	}
	return workspaces, rows.Err()
}

// EncryptLegacySessions 把加密之前保存的明文会话改成加密保存，返回改了几行。
// 只改仍是明文、且期间没被改过的行（同时有新会话写入时以新会话为准）。
func (r *Repository) EncryptLegacySessions(ctx context.Context) (int, error) {
	if r.secrets == nil {
		return 0, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT user_id, admin_account_id, session::text FROM my_site_states WHERE jsonb_typeof(session) = 'object'
	`)
	if err != nil {
		return 0, err
	}
	type legacyRow struct{ userID, adminAccountID, session string }
	var legacy []legacyRow
	for rows.Next() {
		var row legacyRow
		if err := rows.Scan(&row.userID, &row.adminAccountID, &row.session); err != nil {
			rows.Close()
			return 0, err
		}
		legacy = append(legacy, row)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	changed := 0
	for _, row := range legacy {
		sealed, err := r.secrets.SealJSON([]byte(row.session), sessionAAD(row.userID))
		if err != nil {
			return changed, err
		}
		tag, err := r.db.Exec(ctx, `
			UPDATE my_site_states SET session = $1::jsonb
			WHERE user_id = $2 AND admin_account_id = $3 AND session = $4::jsonb
		`, string(sealed), row.userID, row.adminAccountID, row.session)
		if err != nil {
			return changed, err
		}
		changed += int(tag.RowsAffected())
	}
	return changed, nil
}
