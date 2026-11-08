package leads

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"transithub/backend/internal/modules/mass_email"
)

// Repository 读写 leads、lead_settings；线索的发送情况从 mass_email_batch_items 统计，退订看 email_suppressions。
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

type newLead struct {
	ID         string
	Email      string
	Normalized string
	Source     string
	Note       string
}

// leadsWithStatus 查出线索和它的状态：$1 user_id、$2 admin_account_id、$3 退信的 error key，后面可以接筛选条件。
// 子查询加上「lead_id 不为空」的条件，才能用上 mass_email_batch_items 上的部分索引。
const leadsWithStatus = `
SELECT l.id, l.email, l.source, l.note, l.created_at,
       COALESCE(s.sent_count, 0) AS sent_count,
       s.last_sent_at,
       CASE
         WHEN sup.email_normalized IS NOT NULL THEN 'unsubscribed'
         WHEN COALESCE(s.bounced, false) THEN 'bounced'
         WHEN COALESCE(s.sent_count, 0) > 0 THEN 'sent'
         ELSE 'not_sent'
       END AS status
FROM leads l
LEFT JOIN LATERAL (
  SELECT count(*) FILTER (WHERE i.status = 'sent') AS sent_count,
         max(i.sent_at) FILTER (WHERE i.status = 'sent') AS last_sent_at,
         bool_or(i.status = 'failed' AND i.error_key = $3) AS bounced
  FROM mass_email_batch_items i
  WHERE i.lead_id = l.id AND i.lead_id <> ''
) s ON true
LEFT JOIN email_suppressions sup
  ON sup.user_id = l.user_id AND sup.admin_account_id = l.admin_account_id AND sup.email_normalized = l.email_normalized
WHERE l.user_id = $1 AND l.admin_account_id = $2`

// leadFilter 是来源、搜索词（已转小写）和指定的线索 ID。
type leadFilter struct {
	source string
	search string
	ids    []string
	byIDs  bool
}

func (f leadFilter) query(ws Workspace) (string, []any) {
	sql := leadsWithStatus
	args := []any{ws.UserID, ws.AdminAccountID, string(mass_email.ErrRecipientRejected)}
	switch {
	case f.source == NoSource:
		sql += " AND l.source = ''"
	case f.source != "":
		args = append(args, f.source)
		sql += fmt.Sprintf(" AND l.source = $%d", len(args))
	}
	if f.search != "" {
		args = append(args, f.search)
		n := len(args)
		sql += fmt.Sprintf(" AND (strpos(l.email_normalized, $%d) > 0 OR strpos(lower(l.source), $%d) > 0 OR strpos(lower(l.note), $%d) > 0)", n, n, n)
	}
	if f.byIDs {
		args = append(args, f.ids)
		sql += fmt.Sprintf(" AND l.id = ANY($%d)", len(args))
	}
	return sql, args
}

func scanLead(row pgx.Row) (Lead, error) {
	var lead Lead
	err := row.Scan(&lead.ID, &lead.Email, &lead.Source, &lead.Note, &lead.CreatedAt, &lead.SentCount, &lead.LastSentAt, &lead.Status)
	return lead, err
}

// Counts 按状态统计（不受状态筛选影响）。
func (r *Repository) Counts(ctx context.Context, ws Workspace, filter leadFilter) (StatusCounts, error) {
	base, args := filter.query(ws)
	rows, err := r.db.Query(ctx, "WITH base AS ("+base+") SELECT status, count(*) FROM base GROUP BY status", args...)
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
		counts.All += count
		switch status {
		case StatusNotSent:
			counts.NotSent = count
		case StatusSent:
			counts.Sent = count
		case StatusUnsubscribed:
			counts.Unsubscribed = count
		case StatusBounced:
			counts.Bounced = count
		}
	}
	return counts, rows.Err()
}

// Page 是一页线索（按添加时间倒序）。status 为空时不按状态筛选。
func (r *Repository) Page(ctx context.Context, ws Workspace, filter leadFilter, status string, limit int, offset int) ([]Lead, error) {
	base, args := filter.query(ws)
	n := len(args)
	args = append(args, status, limit, offset)
	sql := fmt.Sprintf(`WITH base AS (%s)
		SELECT id, email, source, note, created_at, sent_count, last_sent_at, status FROM base
		WHERE ($%d = '' OR status = $%d)
		ORDER BY created_at DESC, id DESC
		LIMIT $%d OFFSET $%d`, base, n+1, n+1, n+2, n+3)
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []Lead{}
	for rows.Next() {
		lead, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, lead)
	}
	return items, rows.Err()
}

// Get 读一条线索（带状态）。
func (r *Repository) Get(ctx context.Context, ws Workspace, id string) (Lead, error) {
	base, args := leadFilter{ids: []string{id}, byIDs: true}.query(ws)
	lead, err := scanLead(r.db.QueryRow(ctx, base, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Lead{}, ErrNotFound
	}
	return lead, err
}

// Selection 统计一组线索里能发的（未发送、已发送）和要跳过的（已退订、退信）。
func (r *Repository) Selection(ctx context.Context, ws Workspace, filter leadFilter, status string) (int, int, error) {
	base, args := filter.query(ws)
	n := len(args)
	args = append(args, status)
	sql := fmt.Sprintf(`WITH base AS (%s)
		SELECT count(*) FILTER (WHERE status IN ('not_sent', 'sent')),
		       count(*) FILTER (WHERE status IN ('unsubscribed', 'bounced'))
		FROM base WHERE ($%d = '' OR status = $%d)`, base, n+1, n+1)
	var sendable, skipped int
	if err := r.db.QueryRow(ctx, sql, args...).Scan(&sendable, &skipped); err != nil {
		return 0, 0, err
	}
	return sendable, skipped, nil
}

// SendableRecipients 是一组线索里能发的收件人（按添加时间正序），最多 limit 个。
func (r *Repository) SendableRecipients(ctx context.Context, ws Workspace, filter leadFilter, status string, limit int) ([]mass_email.LeadRecipient, error) {
	base, args := filter.query(ws)
	n := len(args)
	args = append(args, status, limit)
	sql := fmt.Sprintf(`WITH base AS (%s)
		SELECT id, email FROM base
		WHERE status IN ('not_sent', 'sent') AND ($%d = '' OR status = $%d)
		ORDER BY created_at ASC, id ASC
		LIMIT $%d`, base, n+1, n+1, n+2)
	rows, err := r.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	recipients := []mass_email.LeadRecipient{}
	for rows.Next() {
		var recipient mass_email.LeadRecipient
		if err := rows.Scan(&recipient.LeadID, &recipient.Email); err != nil {
			return nil, err
		}
		recipients = append(recipients, recipient)
	}
	return recipients, rows.Err()
}

// Insert 添加一条线索；同一个工作区里已经有这个邮箱时返回 false。
func (r *Repository) Insert(ctx context.Context, ws Workspace, lead newLead) (bool, error) {
	tag, err := r.db.Exec(ctx, `
		INSERT INTO leads (id, user_id, admin_account_id, email, email_normalized, source, note)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (user_id, admin_account_id, email_normalized) DO NOTHING`,
		lead.ID, ws.UserID, ws.AdminAccountID, lead.Email, lead.Normalized, lead.Source, lead.Note)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// InsertMany 批量添加（同一个来源），已经有的邮箱跳过，返回新加的数量。
func (r *Repository) InsertMany(ctx context.Context, ws Workspace, source string, leads []newLead) (int, error) {
	if len(leads) == 0 {
		return 0, nil
	}
	ids := make([]string, len(leads))
	emails := make([]string, len(leads))
	normalized := make([]string, len(leads))
	for index, lead := range leads {
		ids[index], emails[index], normalized[index] = lead.ID, lead.Email, lead.Normalized
	}
	tag, err := r.db.Exec(ctx, `
		INSERT INTO leads (id, user_id, admin_account_id, email, email_normalized, source, note)
		SELECT u.id, $1, $2, u.email, u.normalized, $3, ''
		FROM unnest($4::text[], $5::text[], $6::text[]) AS u(id, email, normalized)
		ON CONFLICT (user_id, admin_account_id, email_normalized) DO NOTHING`,
		ws.UserID, ws.AdminAccountID, source, ids, emails, normalized)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// Suppressed 返回这些（已转小写的）邮箱里在退订名单中的。
func (r *Repository) Suppressed(ctx context.Context, ws Workspace, normalized []string) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	if len(normalized) == 0 {
		return result, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT email_normalized FROM email_suppressions
		WHERE user_id = $1 AND admin_account_id = $2 AND email_normalized = ANY($3)`,
		ws.UserID, ws.AdminAccountID, normalized)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return nil, err
		}
		result[email] = struct{}{}
	}
	return result, rows.Err()
}

// Sources 列出用过的来源和各自的线索数（多的在前）。
func (r *Repository) Sources(ctx context.Context, ws Workspace) ([]SourceCount, error) {
	rows, err := r.db.Query(ctx, `
		SELECT source, count(*) FROM leads
		WHERE user_id = $1 AND admin_account_id = $2
		GROUP BY source
		ORDER BY count(*) DESC, source ASC
		LIMIT $3`, ws.UserID, ws.AdminAccountID, maxSources)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sources := []SourceCount{}
	for rows.Next() {
		var source SourceCount
		if err := rows.Scan(&source.Source, &source.Count); err != nil {
			return nil, err
		}
		sources = append(sources, source)
	}
	return sources, rows.Err()
}

// Update 改来源和备注；没有这条线索时返回 ErrNotFound。
func (r *Repository) Update(ctx context.Context, ws Workspace, id string, source string, note string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE leads SET source = $4, note = $5, updated_at = now()
		WHERE user_id = $1 AND admin_account_id = $2 AND id = $3`,
		ws.UserID, ws.AdminAccountID, id, source, note)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete 删除线索，返回删掉的数量。发送记录留着（群发记录里还能看到发过给谁）。
func (r *Repository) Delete(ctx context.Context, ws Workspace, ids []string) (int, error) {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM leads WHERE user_id = $1 AND admin_account_id = $2 AND id = ANY($3)`,
		ws.UserID, ws.AdminAccountID, ids)
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

// SentSince 是从 since 起排进群发、发给线索的邮件数（取消的不算），用来控制最近 24 小时的发送量。
func (r *Repository) SentSince(ctx context.Context, ws Workspace, since time.Time) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT count(*) FROM mass_email_batch_items
		WHERE user_id = $1 AND admin_account_id = $2 AND lead_id <> '' AND created_at >= $3 AND status <> 'cancelled'`,
		ws.UserID, ws.AdminAccountID, since).Scan(&count)
	return count, err
}

// DailyLimit 是最近 24 小时最多发给线索的数量（没设置过时用默认值）。
func (r *Repository) DailyLimit(ctx context.Context, ws Workspace) (int, error) {
	var limit int
	err := r.db.QueryRow(ctx, `
		SELECT daily_limit FROM lead_settings WHERE user_id = $1 AND admin_account_id = $2`,
		ws.UserID, ws.AdminAccountID).Scan(&limit)
	if errors.Is(err, pgx.ErrNoRows) {
		return defaultDailyLimit, nil
	}
	return limit, err
}

func (r *Repository) SetDailyLimit(ctx context.Context, ws Workspace, limit int) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO lead_settings (user_id, admin_account_id, daily_limit, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (user_id, admin_account_id) DO UPDATE SET daily_limit = EXCLUDED.daily_limit, updated_at = now()`,
		ws.UserID, ws.AdminAccountID, limit)
	return err
}
