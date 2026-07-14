package tickets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 封装工单模块在 PostgreSQL 中的全部读写，不涉及 Redis（embed session 由
// EmbedSessionStore 单独管理，见 session_store.go）。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// EnsureSchema 幂等创建工单模块的三张表和索引，语句与任务文档规定的 DDL 保持一致，
// 全部使用 IF NOT EXISTS，符合线上兼容要求（不删除、不改名任何已有对象）。
func (r *Repository) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS ticket_embed_configs (
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			embed_token text NOT NULL,
			enabled boolean NOT NULL DEFAULT true,
			allowed_src_host text NOT NULL DEFAULT '',
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ticket_embed_configs_workspace
		ON ticket_embed_configs (user_id, admin_account_id)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_ticket_embed_configs_token
		ON ticket_embed_configs (embed_token)`,
		// 嵌入页面风格模板（第二阶段新增）。ADD COLUMN ... DEFAULT 会自动给已有行回填 'default'，
		// 不需要额外的 UPDATE 迁移语句。
		`ALTER TABLE ticket_embed_configs ADD COLUMN IF NOT EXISTS template text NOT NULL DEFAULT 'default'`,
		// 每次工单最多上传图片数（第三阶段新增）。默认 0 表示关闭图片上传，保证线上旧 workspace
		// 不会在升级后突然开放上传能力。
		`ALTER TABLE ticket_embed_configs ADD COLUMN IF NOT EXISTS max_images_per_ticket integer NOT NULL DEFAULT 0`,
		// 工单分类/优先级可配置选项（增量新增）。用 jsonb 存储字符串数组，DEFAULT 与
		// tickets.category/priority 的默认值、Service 层 DefaultCategoryOptions/DefaultPriorityOptions
		// 三处保持一致；ADD COLUMN ... DEFAULT 自动给已有行回填，旧 workspace 不需要额外迁移脚本。
		`ALTER TABLE ticket_embed_configs ADD COLUMN IF NOT EXISTS category_options jsonb NOT NULL DEFAULT '["通用问题","余额/计费","接口调用","生图问题","账号/登录"]'::jsonb`,
		`ALTER TABLE ticket_embed_configs ADD COLUMN IF NOT EXISTS priority_options jsonb NOT NULL DEFAULT '["低","普通","高","紧急"]'::jsonb`,
		`CREATE TABLE IF NOT EXISTS tickets (
			id text PRIMARY KEY,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			sub2api_src_host text NOT NULL DEFAULT '',
			sub2api_src_url text NOT NULL DEFAULT '',
			sub2api_user_id text NOT NULL DEFAULT '',
			sub2api_email text NOT NULL DEFAULT '',
			sub2api_role text NOT NULL DEFAULT '',
			manual_email text NOT NULL,
			title text NOT NULL,
			status text NOT NULL DEFAULT 'open',
			last_message_at timestamptz NOT NULL DEFAULT now(),
			created_at timestamptz NOT NULL DEFAULT now(),
			updated_at timestamptz NOT NULL DEFAULT now()
		)`,
		// 工单分类/优先级（增量新增）。默认值与 ticket_embed_configs 的默认选项组第一项一致，
		// 保证线上旧工单升级后立刻拥有合法、可展示的分类/优先级，而不是空字符串。
		`ALTER TABLE tickets ADD COLUMN IF NOT EXISTS category text NOT NULL DEFAULT '通用问题'`,
		`ALTER TABLE tickets ADD COLUMN IF NOT EXISTS priority text NOT NULL DEFAULT '普通'`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_workspace_status_updated
		ON tickets (user_id, admin_account_id, status, updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tickets_embed_user_updated
		ON tickets (user_id, admin_account_id, sub2api_src_host, sub2api_user_id, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS ticket_messages (
			id text PRIMARY KEY,
			ticket_id text NOT NULL,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			author_type text NOT NULL,
			author_name text NOT NULL DEFAULT '',
			body text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ticket_messages_ticket_created
		ON ticket_messages (ticket_id, created_at ASC)`,
		// 工单图片附件（第三阶段新增）。图片二进制不落库，只存 metadata 和本地存储路径。
		`CREATE TABLE IF NOT EXISTS ticket_attachments (
			id text PRIMARY KEY,
			ticket_id text NOT NULL,
			message_id text NOT NULL,
			user_id text NOT NULL,
			admin_account_id text NOT NULL DEFAULT '',
			author_type text NOT NULL,
			original_name text NOT NULL,
			content_type text NOT NULL,
			size_bytes bigint NOT NULL,
			storage_path text NOT NULL,
			created_at timestamptz NOT NULL DEFAULT now()
		)`,
		`CREATE INDEX IF NOT EXISTS idx_ticket_attachments_ticket_created
		ON ticket_attachments (ticket_id, created_at ASC)`,
		`CREATE INDEX IF NOT EXISTS idx_ticket_attachments_message_created
		ON ticket_attachments (message_id, created_at ASC)`,
	}
	for _, stmt := range statements {
		if _, err := r.db.Exec(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

// ---- ticket_embed_configs ----

// embedConfigColumns 是 ticket_embed_configs 表在所有查询中统一使用的列顺序，供 scanEmbedConfig 复用。
const embedConfigColumns = `user_id, admin_account_id, embed_token, enabled, allowed_src_host, template, max_images_per_ticket, category_options, priority_options, created_at, updated_at`

func (r *Repository) GetEmbedConfigByToken(ctx context.Context, embedToken string) (*EmbedConfig, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+embedConfigColumns+`
		FROM ticket_embed_configs
		WHERE embed_token = $1
	`, embedToken)
	return scanEmbedConfig(row)
}

func (r *Repository) GetEmbedConfigByWorkspace(ctx context.Context, userID string, adminAccountID string) (*EmbedConfig, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+embedConfigColumns+`
		FROM ticket_embed_configs
		WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID)
	return scanEmbedConfig(row)
}

func scanEmbedConfig(row pgx.Row) (*EmbedConfig, error) {
	var c EmbedConfig
	var categoryJSON, priorityJSON []byte
	if err := row.Scan(&c.UserID, &c.AdminAccountID, &c.EmbedToken, &c.Enabled, &c.AllowedSrcHost, &c.Template, &c.MaxImagesPerTicket, &categoryJSON, &priorityJSON, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	// category_options/priority_options 是 NOT NULL jsonb 列，反序列化失败在正常运行中不应该发生；
	// 即便发生（例如手工改坏了数据），也只是让对应字段保持零值切片，由 Service 层
	// withDefaultTicketOptions 兜底为默认选项，不让整条查询因为一行脏数据而失败。
	_ = json.Unmarshal(categoryJSON, &c.CategoryOptions)
	_ = json.Unmarshal(priorityJSON, &c.PriorityOptions)
	return &c, nil
}

// InsertEmbedConfig 创建一条新的嵌入配置。ON CONFLICT DO NOTHING 防止并发请求下重复初始化
// 同一个 workspace（唯一索引 idx_ticket_embed_configs_workspace 已经保证了这一点，这里只是
// 让"配置不存在时自动创建默认配置"这条路径在并发下也是幂等的，不会返回错误）。
func (r *Repository) InsertEmbedConfig(ctx context.Context, c EmbedConfig) error {
	categoryJSON, err := json.Marshal(c.CategoryOptions)
	if err != nil {
		return err
	}
	priorityJSON, err := json.Marshal(c.PriorityOptions)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO ticket_embed_configs (user_id, admin_account_id, embed_token, enabled, allowed_src_host, template, max_images_per_ticket, category_options, priority_options, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9::jsonb, now(), now())
		ON CONFLICT (user_id, admin_account_id) DO NOTHING
	`, c.UserID, c.AdminAccountID, c.EmbedToken, c.Enabled, c.AllowedSrcHost, c.Template, c.MaxImagesPerTicket, categoryJSON, priorityJSON)
	return err
}

// UpdateEmbedConfig 保存一个 workspace 的模板选择和图片数量上限。第二阶段取消了"启用/禁用"和
// "允许来源域名"两项配置能力，这里连带把它们强制修正为 enabled=true、allowed_src_host=空字符串：
// 既让本次保存动作本身不再依赖这两个字段的旧值，也顺带修复历史上可能被设置为 disabled 或
// 限制了来源域名、导致 iframe 无法访问的旧数据，不需要额外的一次性迁移脚本。
func (r *Repository) UpdateEmbedConfig(ctx context.Context, userID string, adminAccountID string, template string, maxImagesPerTicket int, categoryOptions []string, priorityOptions []string) error {
	categoryJSON, err := json.Marshal(categoryOptions)
	if err != nil {
		return err
	}
	priorityJSON, err := json.Marshal(priorityOptions)
	if err != nil {
		return err
	}
	_, err = r.db.Exec(ctx, `
		UPDATE ticket_embed_configs
		SET enabled = true, allowed_src_host = '', template = $3, max_images_per_ticket = $4,
			category_options = $5::jsonb, priority_options = $6::jsonb, updated_at = now()
		WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID, template, maxImagesPerTicket, categoryJSON, priorityJSON)
	return err
}

// RotateEmbedToken 轮换一个 workspace 的 embed_token，旧 iframe 地址随之失效。
func (r *Repository) RotateEmbedToken(ctx context.Context, userID string, adminAccountID string, newToken string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE ticket_embed_configs
		SET embed_token = $3, updated_at = now()
		WHERE user_id = $1 AND admin_account_id = $2
	`, userID, adminAccountID, newToken)
	return err
}

// ---- tickets / ticket_messages ----

// InsertTicketWithMessage 在同一事务中创建工单、首条 customer 消息和该消息的图片附件 metadata，
// 满足文档"同事务创建 ticket、首条 message、attachment metadata"的要求：任何一步失败都不会
// 留下没有首条消息的孤立工单，也不会留下没有对应工单/消息的孤立附件记录。
//
// 注意：本方法只负责数据库记录；图片文件本身必须由调用方（Service）在调用本方法之前写入磁盘，
// 并在本方法返回错误时自行删除已写入的文件——数据库事务失败后不应该在磁盘上留下孤儿文件。
func (r *Repository) InsertTicketWithMessage(ctx context.Context, t Ticket, m TicketMessage, attachments []TicketAttachment) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `
		INSERT INTO tickets (
			id, user_id, admin_account_id, sub2api_src_host, sub2api_src_url, sub2api_user_id, sub2api_email, sub2api_role,
			manual_email, title, status, category, priority, last_message_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, now(), now())
	`, t.ID, t.UserID, t.AdminAccountID, t.Sub2apiSrcHost, t.Sub2apiSrcURL, t.Sub2apiUserID, t.Sub2apiEmail, t.Sub2apiRole,
		t.ManualEmail, t.Title, t.Status, t.Category, t.Priority, t.LastMessageAt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ticket_messages (id, ticket_id, user_id, admin_account_id, author_type, author_name, body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
	`, m.ID, m.TicketID, m.UserID, m.AdminAccountID, m.AuthorType, m.AuthorName, m.Body); err != nil {
		return err
	}
	for _, a := range attachments {
		if _, err := tx.Exec(ctx, `
			INSERT INTO ticket_attachments (id, ticket_id, message_id, user_id, admin_account_id, author_type, original_name, content_type, size_bytes, storage_path, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, now())
		`, a.ID, a.TicketID, a.MessageID, a.UserID, a.AdminAccountID, a.AuthorType, a.OriginalName, a.ContentType, a.SizeBytes, a.StoragePath); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ListAttachmentsByTicket 按创建时间正序返回一个工单下的全部附件，供构建消息详情视图使用。
func (r *Repository) ListAttachmentsByTicket(ctx context.Context, ticketID string) ([]TicketAttachment, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+attachmentColumns+`
		FROM ticket_attachments
		WHERE ticket_id = $1
		ORDER BY created_at ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	attachments := make([]TicketAttachment, 0)
	for rows.Next() {
		var a TicketAttachment
		if err := scanAttachment(rows, &a); err != nil {
			return nil, err
		}
		attachments = append(attachments, a)
	}
	return attachments, rows.Err()
}

// GetAttachmentByID 按 ID 读取单个附件；不做 workspace/ticket 归属校验，调用方（Service）必须
// 再结合 attachment.TicketID 走一次工单归属校验（embed 走 GetEmbedTicket，admin 走 GetAdminTicket），
// 避免在这里重复实现四段/三段过滤逻辑。
func (r *Repository) GetAttachmentByID(ctx context.Context, id string) (*TicketAttachment, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+attachmentColumns+`
		FROM ticket_attachments
		WHERE id = $1
	`, id)
	var a TicketAttachment
	if err := scanAttachment(row, &a); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &a, nil
}

const attachmentColumns = `id, ticket_id, message_id, user_id, admin_account_id, author_type, original_name, content_type, size_bytes, storage_path, created_at`

func scanAttachment(row rowScanner, a *TicketAttachment) error {
	return row.Scan(&a.ID, &a.TicketID, &a.MessageID, &a.UserID, &a.AdminAccountID, &a.AuthorType, &a.OriginalName, &a.ContentType, &a.SizeBytes, &a.StoragePath, &a.CreatedAt)
}

func (r *Repository) InsertMessage(ctx context.Context, m TicketMessage) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO ticket_messages (id, ticket_id, user_id, admin_account_id, author_type, author_name, body, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, now())
	`, m.ID, m.TicketID, m.UserID, m.AdminAccountID, m.AuthorType, m.AuthorName, m.Body)
	return err
}

// ListMessages 按创建时间正序返回一个工单的全部回复记录。
func (r *Repository) ListMessages(ctx context.Context, ticketID string) ([]TicketMessage, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, ticket_id, user_id, admin_account_id, author_type, author_name, body, created_at
		FROM ticket_messages
		WHERE ticket_id = $1
		ORDER BY created_at ASC
	`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := make([]TicketMessage, 0)
	for rows.Next() {
		var m TicketMessage
		if err := rows.Scan(&m.ID, &m.TicketID, &m.UserID, &m.AdminAccountID, &m.AuthorType, &m.AuthorName, &m.Body, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

// ListEmbedTickets 只返回同时匹配 workspace + Sub2API 来源域名 + Sub2API 用户 ID 的工单，
// 是"iframe 用户只能看到自己工单"这条安全边界在数据库层面的强制执行点。
func (r *Repository) ListEmbedTickets(ctx context.Context, userID string, adminAccountID string, srcHost string, sub2apiUserID string) ([]Ticket, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+ticketColumns+`
		FROM tickets
		WHERE user_id = $1 AND admin_account_id = $2 AND sub2api_src_host = $3 AND sub2api_user_id = $4
		ORDER BY updated_at DESC
	`, userID, adminAccountID, srcHost, sub2apiUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTicketRows(rows)
}

// GetEmbedTicket 按同样的四段过滤条件读取单个工单，防止 iframe 用户通过猜测工单 ID
// 读取同一工作区内其他 Sub2API 用户的工单。
func (r *Repository) GetEmbedTicket(ctx context.Context, userID string, adminAccountID string, srcHost string, sub2apiUserID string, id string) (*Ticket, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+ticketColumns+`
		FROM tickets
		WHERE id = $1 AND user_id = $2 AND admin_account_id = $3 AND sub2api_src_host = $4 AND sub2api_user_id = $5
	`, id, userID, adminAccountID, srcHost, sub2apiUserID)
	return scanTicketRow(row)
}

// ListAdminTickets 按 workspace 分页返回工单列表，可选按状态筛选，total 通过窗口函数一并取出。
func (r *Repository) ListAdminTickets(ctx context.Context, userID string, adminAccountID string, status string, page int, pageSize int) ([]Ticket, int, error) {
	offset := (page - 1) * pageSize
	rows, err := r.db.Query(ctx, `
		SELECT `+ticketColumns+`, count(*) OVER () AS total
		FROM tickets
		WHERE user_id = $1 AND admin_account_id = $2 AND ($3 = '' OR status = $3)
		ORDER BY updated_at DESC
		LIMIT $4 OFFSET $5
	`, userID, adminAccountID, status, pageSize, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	tickets := make([]Ticket, 0)
	total := 0
	for rows.Next() {
		var t Ticket
		if err := scanTicketFields(rows, &t, &total); err != nil {
			return nil, 0, err
		}
		tickets = append(tickets, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return tickets, total, nil
}

// GetAdminTicket 按 user_id + admin_account_id + ticket_id 过滤，保证后台不能跨 workspace 读取工单。
func (r *Repository) GetAdminTicket(ctx context.Context, userID string, adminAccountID string, id string) (*Ticket, error) {
	row := r.db.QueryRow(ctx, `
		SELECT `+ticketColumns+`
		FROM tickets
		WHERE id = $1 AND user_id = $2 AND admin_account_id = $3
	`, id, userID, adminAccountID)
	return scanTicketRow(row)
}

// TouchTicket 在追加一条消息后更新工单状态与最后回复时间（last_message_at 与 updated_at）。
func (r *Repository) TouchTicket(ctx context.Context, id string, status string, lastMessageAt time.Time) error {
	_, err := r.db.Exec(ctx, `
		UPDATE tickets
		SET status = $2, last_message_at = $3, updated_at = now()
		WHERE id = $1
	`, id, status, lastMessageAt)
	return err
}

// UpdateStatus 单独修改工单状态（后台"修改状态"接口），不触碰 last_message_at。
func (r *Repository) UpdateStatus(ctx context.Context, id string, status string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE tickets SET status = $2, updated_at = now() WHERE id = $1
	`, id, status)
	return err
}

// ticketColumns 是 tickets 表在所有查询中统一使用的列顺序，供 scanTicketRow/scanTicketFields 复用。
const ticketColumns = `id, user_id, admin_account_id, sub2api_src_host, sub2api_src_url, sub2api_user_id, sub2api_email, sub2api_role,
			manual_email, title, status, category, priority, last_message_at, created_at, updated_at`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanTicketRow(row pgx.Row) (*Ticket, error) {
	var t Ticket
	if err := scanTicketFields(row, &t, nil); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &t, nil
}

func scanTicketFields(row rowScanner, t *Ticket, total *int) error {
	dest := []any{
		&t.ID, &t.UserID, &t.AdminAccountID, &t.Sub2apiSrcHost, &t.Sub2apiSrcURL, &t.Sub2apiUserID, &t.Sub2apiEmail, &t.Sub2apiRole,
		&t.ManualEmail, &t.Title, &t.Status, &t.Category, &t.Priority, &t.LastMessageAt, &t.CreatedAt, &t.UpdatedAt,
	}
	if total != nil {
		dest = append(dest, total)
	}
	return row.Scan(dest...)
}

func scanTicketRows(rows pgx.Rows) ([]Ticket, error) {
	tickets := make([]Ticket, 0)
	for rows.Next() {
		var t Ticket
		if err := scanTicketFields(rows, &t, nil); err != nil {
			return nil, err
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

// randomID 生成 UUID v4 形状的字符串主键，与仓库内其它模块（upstream/group_rate_campaigns 等）
// 的私有 ID 生成器保持一致的实现方式（无第三方 uuid 依赖）。
func randomID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", errors.New("generate ticket id")
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}

// randomToken 生成用于 embed_token / embed session token 的高熵随机十六进制字符串。
func randomToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", errors.New("generate token")
	}
	return hex.EncodeToString(buf), nil
}
