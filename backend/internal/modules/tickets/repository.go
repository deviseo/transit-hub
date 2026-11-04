package tickets

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository 读写工单表（建表见迁移 000026）。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

const ticketColumns = `id, user_id, admin_account_id, platform, site_user_id, site_username, site_email, manual_email,
	client_info, title, category, status, last_author, last_message_at, created_at, updated_at, customer_seen_at`

func scanTicket(row pgx.Row) (*Ticket, error) {
	var t Ticket
	err := row.Scan(&t.ID, &t.UserID, &t.AdminAccountID, &t.User.Platform, &t.User.ID, &t.User.Username, &t.User.Email,
		&t.Contact, &t.ClientInfo, &t.Title, &t.Category, &t.Status, &t.LastAuthor, &t.LastMessageAt, &t.CreatedAt,
		&t.UpdatedAt, &t.CustomerSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func scanTickets(rows pgx.Rows) ([]Ticket, error) {
	defer rows.Close()
	tickets := make([]Ticket, 0)
	for rows.Next() {
		ticket, err := scanTicket(rows)
		if err != nil {
			return nil, err
		}
		tickets = append(tickets, *ticket)
	}
	return tickets, rows.Err()
}

// CreateTicket 在一个事务里新建工单和第一条消息（含附件）。
func (r *Repository) CreateTicket(ctx context.Context, ticket Ticket, message Message) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO tickets (id, user_id, admin_account_id, platform, site_user_id, site_username,
		site_email, manual_email, client_info, title, category, status, last_author, last_message_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $14, $14)`,
		ticket.ID, ticket.UserID, ticket.AdminAccountID, ticket.User.Platform, ticket.User.ID, ticket.User.Username,
		ticket.User.Email, ticket.Contact, ticket.ClientInfo, ticket.Title, ticket.Category, ticket.Status, ticket.LastAuthor,
		ticket.LastMessageAt); err != nil {
		return err
	}
	if err := insertMessage(ctx, tx, ticket.Workspace, message); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// AddMessage 追加一条消息，同时更新工单的状态、最后一条消息的作者和时间。工单不存在时返回 false。
func (r *Repository) AddMessage(ctx context.Context, ws Workspace, ticketID string, message Message, status string) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE tickets SET status = $1, last_author = $2, last_message_at = $3, updated_at = $3
		WHERE id = $4 AND user_id = $5 AND admin_account_id = $6`,
		status, message.AuthorType, message.CreatedAt, ticketID, ws.UserID, ws.AdminAccountID)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 0 {
		return false, nil
	}
	if err := insertMessage(ctx, tx, ws, message); err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func insertMessage(ctx context.Context, tx pgx.Tx, ws Workspace, message Message) error {
	if _, err := tx.Exec(ctx, `INSERT INTO ticket_messages (id, ticket_id, user_id, admin_account_id, author_type, author_name,
		body, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		message.ID, message.TicketID, ws.UserID, ws.AdminAccountID, message.AuthorType, message.AuthorName, message.Body,
		message.CreatedAt); err != nil {
		return err
	}
	for _, a := range message.Attachments {
		if _, err := tx.Exec(ctx, `INSERT INTO ticket_attachments (id, ticket_id, message_id, user_id, admin_account_id,
			author_type, original_name, content_type, size_bytes, storage_path, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			a.ID, message.TicketID, message.ID, ws.UserID, ws.AdminAccountID, message.AuthorType, a.OriginalName,
			a.ContentType, a.SizeBytes, a.StoragePath, message.CreatedAt); err != nil {
			return err
		}
	}
	return nil
}

// GetTicket 读取工作区里的一个工单；没有时返回 nil。
func (r *Repository) GetTicket(ctx context.Context, ws Workspace, id string) (*Ticket, error) {
	return scanTicket(r.db.QueryRow(ctx, `SELECT `+ticketColumns+` FROM tickets
		WHERE id = $1 AND user_id = $2 AND admin_account_id = $3`, id, ws.UserID, ws.AdminAccountID))
}

// ListForUser 返回一个用户在这个站点上的工单（最近更新的在前）。
func (r *Repository) ListForUser(ctx context.Context, ws Workspace, user SiteUser, limit int) ([]Ticket, error) {
	rows, err := r.db.Query(ctx, `SELECT `+ticketColumns+` FROM tickets
		WHERE user_id = $1 AND admin_account_id = $2 AND platform = $3 AND site_user_id = $4
		ORDER BY last_message_at DESC, id DESC LIMIT $5`, ws.UserID, ws.AdminAccountID, user.Platform, user.ID, limit)
	if err != nil {
		return nil, err
	}
	return scanTickets(rows)
}

// ListMessages 返回工单的全部消息和附件（按时间先后）。
func (r *Repository) ListMessages(ctx context.Context, ticketID string) ([]Message, error) {
	rows, err := r.db.Query(ctx, `SELECT id, ticket_id, author_type, author_name, body, created_at
		FROM ticket_messages WHERE ticket_id = $1 ORDER BY created_at ASC, id ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	messages := make([]Message, 0)
	index := map[string]int{}
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.TicketID, &m.AuthorType, &m.AuthorName, &m.Body, &m.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		m.Attachments = []Attachment{}
		index[m.ID] = len(messages)
		messages = append(messages, m)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	attachments, err := r.db.Query(ctx, `SELECT id, ticket_id, message_id, author_type, original_name, content_type,
		size_bytes, storage_path, created_at FROM ticket_attachments WHERE ticket_id = $1 ORDER BY created_at ASC, id ASC`, ticketID)
	if err != nil {
		return nil, err
	}
	defer attachments.Close()
	for attachments.Next() {
		var a Attachment
		if err := attachments.Scan(&a.ID, &a.TicketID, &a.MessageID, &a.AuthorType, &a.OriginalName, &a.ContentType,
			&a.SizeBytes, &a.StoragePath, &a.CreatedAt); err != nil {
			return nil, err
		}
		if i, ok := index[a.MessageID]; ok {
			messages[i].Attachments = append(messages[i].Attachments, a)
		}
	}
	return messages, attachments.Err()
}

// MarkCustomerSeen 记录用户看过工单的时间（之前的站长回复不再算未读）。
func (r *Repository) MarkCustomerSeen(ctx context.Context, ws Workspace, id string, at time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE tickets SET customer_seen_at = $1 WHERE id = $2 AND user_id = $3 AND admin_account_id = $4`,
		at, id, ws.UserID, ws.AdminAccountID)
	return err
}

// SetStatus 修改工单状态；工单不存在时返回 false。
func (r *Repository) SetStatus(ctx context.Context, ws Workspace, id string, status string) (bool, error) {
	tag, err := r.db.Exec(ctx, `UPDATE tickets SET status = $1, updated_at = now()
		WHERE id = $2 AND user_id = $3 AND admin_account_id = $4`, status, id, ws.UserID, ws.AdminAccountID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

// ListForAdmin 分页列出工作区的工单（status 为空时列出全部；最近有消息的在前）。
func (r *Repository) ListForAdmin(ctx context.Context, ws Workspace, status string, page int, pageSize int) ([]Ticket, int, error) {
	var total int
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE user_id = $1 AND admin_account_id = $2
		AND ($3 = '' OR status = $3)`, ws.UserID, ws.AdminAccountID, status).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `SELECT `+ticketColumns+` FROM tickets WHERE user_id = $1 AND admin_account_id = $2
		AND ($3 = '' OR status = $3) ORDER BY last_message_at DESC, id DESC LIMIT $4 OFFSET $5`,
		ws.UserID, ws.AdminAccountID, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	tickets, err := scanTickets(rows)
	return tickets, total, err
}

// CountByStatus 统计工作区各状态的工单数。
func (r *Repository) CountByStatus(ctx context.Context, ws Workspace) (StatusCounts, error) {
	rows, err := r.db.Query(ctx, `SELECT status, count(*) FROM tickets WHERE user_id = $1 AND admin_account_id = $2
		GROUP BY status`, ws.UserID, ws.AdminAccountID)
	if err != nil {
		return StatusCounts{}, err
	}
	defer rows.Close()
	var counts StatusCounts
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return StatusCounts{}, err
		}
		switch status {
		case StatusOpen:
			counts.Open = count
		case StatusPending:
			counts.Pending = count
		case StatusReplied:
			counts.Replied = count
		case StatusClosed:
			counts.Closed = count
		}
	}
	return counts, rows.Err()
}

// GetAttachment 读取工作区里的一个附件；没有时返回 nil。
func (r *Repository) GetAttachment(ctx context.Context, ws Workspace, id string) (*Attachment, error) {
	var a Attachment
	err := r.db.QueryRow(ctx, `SELECT id, ticket_id, message_id, author_type, original_name, content_type, size_bytes,
		storage_path, created_at FROM ticket_attachments WHERE id = $1 AND user_id = $2 AND admin_account_id = $3`,
		id, ws.UserID, ws.AdminAccountID).Scan(&a.ID, &a.TicketID, &a.MessageID, &a.AuthorType, &a.OriginalName,
		&a.ContentType, &a.SizeBytes, &a.StoragePath, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// AttachmentNames 返回工作区所有附件的文件名（删除工作区时删文件用）。
func (r *Repository) AttachmentNames(ctx context.Context, ws Workspace) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT storage_path FROM ticket_attachments WHERE user_id = $1 AND admin_account_id = $2`,
		ws.UserID, ws.AdminAccountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	return names, rows.Err()
}
