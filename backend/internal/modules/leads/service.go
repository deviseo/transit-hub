package leads

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log"
	"math"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"transithub/backend/internal/modules/mass_email"
)

// NoSource 是筛选「没填来源」的线索时用的来源值。
const NoSource = "__none__"

// maxImportTextBytes 是一次导入的文本（粘贴或文件内容）最大字节数。
const maxImportTextBytes = 1 << 20

type store interface {
	Counts(ctx context.Context, ws Workspace, filter leadFilter) (StatusCounts, error)
	Page(ctx context.Context, ws Workspace, filter leadFilter, status string, limit int, offset int) ([]Lead, error)
	Get(ctx context.Context, ws Workspace, id string) (Lead, error)
	Selection(ctx context.Context, ws Workspace, filter leadFilter, status string) (int, int, error)
	SendableRecipients(ctx context.Context, ws Workspace, filter leadFilter, status string, limit int) ([]mass_email.LeadRecipient, error)
	Insert(ctx context.Context, ws Workspace, lead newLead) (bool, error)
	InsertMany(ctx context.Context, ws Workspace, source string, leads []newLead) (int, error)
	Suppressed(ctx context.Context, ws Workspace, normalized []string) (map[string]struct{}, error)
	Sources(ctx context.Context, ws Workspace) ([]SourceCount, error)
	Update(ctx context.Context, ws Workspace, id string, source string, note string) error
	Delete(ctx context.Context, ws Workspace, ids []string) (int, error)
	SentSince(ctx context.Context, ws Workspace, since time.Time) (int, error)
	DailyLimit(ctx context.Context, ws Workspace) (int, error)
	SetDailyLimit(ctx context.Context, ws Workspace, limit int) error
}

// batchCreator 是群发邮件模块（*mass_email.Service）：线索的邮件和普通群发走同一个发送队列。
type batchCreator interface {
	CreateLeadBatch(ctx context.Context, userID string, adminAccountID string, req mass_email.CreateBatchRequest, recipients []mass_email.LeadRecipient) (mass_email.BatchDTO, error)
	BatchForRequest(ctx context.Context, userID string, adminAccountID string, requestID string) (mass_email.BatchDTO, bool)
}

type Service struct {
	repo    store
	batches batchCreator
	now     func() time.Time
}

func NewService(repo store, batches batchCreator) *Service {
	return &Service{repo: repo, batches: batches, now: time.Now}
}

func (s *Service) List(ctx context.Context, ws Workspace, query ListQuery) (ListResult, error) {
	status, ok := normalizeStatus(query.Status)
	if !ok {
		return ListResult{}, ErrInvalidRequest
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	pageSize := query.PageSize
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	filter := leadFilter{source: normalizeSourceFilter(query.Source), search: normalizeSearch(query.Search)}
	counts, err := s.repo.Counts(ctx, ws, filter)
	if err != nil {
		return ListResult{}, s.persistence("count", ws, err)
	}
	items, err := s.repo.Page(ctx, ws, filter, status, pageSize, (page-1)*pageSize)
	if err != nil {
		return ListResult{}, s.persistence("list", ws, err)
	}
	total := counts.All
	switch status {
	case StatusNotSent:
		total = counts.NotSent
	case StatusSent:
		total = counts.Sent
	case StatusUnsubscribed:
		total = counts.Unsubscribed
	case StatusBounced:
		total = counts.Bounced
	}
	return ListResult{Items: items, Total: total, Page: page, PageSize: pageSize, Pages: pages(total, pageSize), Counts: counts}, nil
}

func (s *Service) Sources(ctx context.Context, ws Workspace) ([]SourceCount, error) {
	sources, err := s.repo.Sources(ctx, ws)
	if err != nil {
		return nil, s.persistence("sources", ws, err)
	}
	return sources, nil
}

// Add 添加一个邮箱。已经在线索里的、在退订名单里的都不加。
func (s *Service) Add(ctx context.Context, ws Workspace, req AddRequest) (Lead, error) {
	email, normalized, ok := normalizeEmail(req.Email)
	if !ok {
		return Lead{}, ErrInvalidEmail
	}
	suppressed, err := s.repo.Suppressed(ctx, ws, []string{normalized})
	if err != nil {
		return Lead{}, s.persistence("check suppression", ws, err)
	}
	if _, ok := suppressed[normalized]; ok {
		return Lead{}, ErrSuppressed
	}
	id, err := newID()
	if err != nil {
		return Lead{}, ErrPersistence
	}
	created, err := s.repo.Insert(ctx, ws, newLead{ID: id, Email: email, Normalized: normalized, Source: clip(req.Source, maxSourceRunes), Note: clip(req.Note, maxNoteRunes)})
	if err != nil {
		return Lead{}, s.persistence("insert", ws, err)
	}
	if !created {
		return Lead{}, ErrDuplicate
	}
	lead, err := s.repo.Get(ctx, ws, id)
	if err != nil {
		return Lead{}, s.persistence("load", ws, err)
	}
	return lead, nil
}

// Import 批量导入：从文本里找出邮箱（不带 @ 的内容，比如 CSV 表头、名字，直接忽略），去重，跳过退订名单里的。
func (s *Service) Import(ctx context.Context, ws Workspace, req ImportRequest) (ImportResult, error) {
	if len(req.Text) > maxImportTextBytes {
		return ImportResult{}, ErrTooMany
	}
	emails, duplicates, invalid := parseImport(req.Text)
	if len(emails) > maxImportEmails {
		return ImportResult{}, ErrTooMany
	}
	result := ImportResult{Duplicates: duplicates, Invalid: invalid}
	if len(emails) == 0 {
		return result, nil
	}
	keys := make([]string, 0, len(emails))
	for _, email := range emails {
		keys = append(keys, email.normalized)
	}
	suppressed, err := s.repo.Suppressed(ctx, ws, keys)
	if err != nil {
		return ImportResult{}, s.persistence("check suppressions", ws, err)
	}
	leads := make([]newLead, 0, len(emails))
	for _, email := range emails {
		if _, skip := suppressed[email.normalized]; skip {
			result.Suppressed++
			continue
		}
		id, err := newID()
		if err != nil {
			return ImportResult{}, ErrPersistence
		}
		leads = append(leads, newLead{ID: id, Email: email.email, Normalized: email.normalized})
	}
	added, err := s.repo.InsertMany(ctx, ws, clip(req.Source, maxSourceRunes), leads)
	if err != nil {
		return ImportResult{}, s.persistence("import", ws, err)
	}
	result.Added = added
	result.Duplicates += len(leads) - added
	return result, nil
}

func (s *Service) Update(ctx context.Context, ws Workspace, id string, req UpdateRequest) (Lead, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Lead{}, ErrNotFound
	}
	if err := s.repo.Update(ctx, ws, id, clip(req.Source, maxSourceRunes), clip(req.Note, maxNoteRunes)); err != nil {
		if errors.Is(err, ErrNotFound) {
			return Lead{}, ErrNotFound
		}
		return Lead{}, s.persistence("update", ws, err)
	}
	lead, err := s.repo.Get(ctx, ws, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Lead{}, ErrNotFound
		}
		return Lead{}, s.persistence("load", ws, err)
	}
	return lead, nil
}

func (s *Service) Delete(ctx context.Context, ws Workspace, req DeleteRequest) (DeleteResult, error) {
	ids, ok := normalizeIDs(req.IDs)
	if !ok {
		return DeleteResult{}, ErrInvalidRequest
	}
	deleted, err := s.repo.Delete(ctx, ws, ids)
	if err != nil {
		return DeleteResult{}, s.persistence("delete", ws, err)
	}
	return DeleteResult{Deleted: deleted}, nil
}

func (s *Service) Settings(ctx context.Context, ws Workspace) (Settings, error) {
	limit, sent, err := s.quota(ctx, ws)
	if err != nil {
		return Settings{}, err
	}
	return Settings{DailyLimit: limit, SentLast24h: sent, Remaining: remaining(limit, sent)}, nil
}

func (s *Service) SaveSettings(ctx context.Context, ws Workspace, req SettingsRequest) (Settings, error) {
	if req.DailyLimit < 1 || req.DailyLimit > maxDailyLimit {
		return Settings{}, ErrInvalidDailyLimit
	}
	if err := s.repo.SetDailyLimit(ctx, ws, req.DailyLimit); err != nil {
		return Settings{}, s.persistence("save settings", ws, err)
	}
	return s.Settings(ctx, ws)
}

// Preview 统计这次能发多少（已退订、退信的跳过）和最近 24 小时还能发多少。
func (s *Service) Preview(ctx context.Context, ws Workspace, selection Selection) (PreviewResult, error) {
	filter, status, err := normalizeSelection(selection)
	if err != nil {
		return PreviewResult{}, err
	}
	sendable, skipped, err := s.repo.Selection(ctx, ws, filter, status)
	if err != nil {
		return PreviewResult{}, s.persistence("preview", ws, err)
	}
	limit, sent, err := s.quota(ctx, ws)
	if err != nil {
		return PreviewResult{}, err
	}
	return PreviewResult{Sendable: sendable, Skipped: skipped, DailyLimit: limit, Remaining: remaining(limit, sent)}, nil
}

// Send 把选中的线索交给群发邮件发送。线索邮件必须带退订链接；超过最近 24 小时的上限时不发。
func (s *Service) Send(ctx context.Context, ws Workspace, req SendRequest) (mass_email.BatchDTO, error) {
	req.TemplateID = strings.TrimSpace(req.TemplateID)
	req.RequestID = strings.TrimSpace(req.RequestID)
	req.PublicBaseURL = strings.TrimSpace(req.PublicBaseURL)
	if req.TemplateID == "" || req.RequestID == "" {
		return mass_email.BatchDTO{}, ErrInvalidRequest
	}
	if req.PublicBaseURL == "" {
		return mass_email.BatchDTO{}, ErrUnsubscribeRequired
	}
	// 同一个请求重试（比如上次其实建好了、只是响应丢了）：直接返回建好的批次，不再按上限检查一遍。
	if batch, ok := s.batches.BatchForRequest(ctx, ws.UserID, ws.AdminAccountID, req.RequestID); ok {
		return batch, nil
	}
	filter, status, err := normalizeSelection(req.Selection)
	if err != nil {
		return mass_email.BatchDTO{}, err
	}
	recipients, err := s.repo.SendableRecipients(ctx, ws, filter, status, maxRecipients+1)
	if err != nil {
		return mass_email.BatchDTO{}, s.persistence("resolve recipients", ws, err)
	}
	if len(recipients) == 0 {
		return mass_email.BatchDTO{}, ErrNoRecipients
	}
	if len(recipients) > maxRecipients {
		return mass_email.BatchDTO{}, ErrTooMany
	}
	limit, sent, err := s.quota(ctx, ws)
	if err != nil {
		return mass_email.BatchDTO{}, err
	}
	if len(recipients) > remaining(limit, sent) {
		return mass_email.BatchDTO{}, ErrDailyLimit
	}
	return s.batches.CreateLeadBatch(ctx, ws.UserID, ws.AdminAccountID, mass_email.CreateBatchRequest{
		TemplateID:    req.TemplateID,
		RequestID:     req.RequestID,
		PublicBaseURL: req.PublicBaseURL,
		AdLabel:       req.AdLabel,
	}, recipients)
}

func (s *Service) quota(ctx context.Context, ws Workspace) (int, int, error) {
	limit, err := s.repo.DailyLimit(ctx, ws)
	if err != nil {
		return 0, 0, s.persistence("load daily limit", ws, err)
	}
	sent, err := s.repo.SentSince(ctx, ws, s.now().Add(-24*time.Hour))
	if err != nil {
		return 0, 0, s.persistence("count recent sends", ws, err)
	}
	return limit, sent, nil
}

func (s *Service) persistence(action string, ws Workspace, err error) error {
	log.Printf("[leads] %s failed user_id=%s admin_account_id=%s err=%v", action, ws.UserID, ws.AdminAccountID, err)
	return ErrPersistence
}

func remaining(limit int, sent int) int {
	if sent >= limit {
		return 0
	}
	return limit - sent
}

func normalizeSelection(selection Selection) (leadFilter, string, error) {
	switch selection.Mode {
	case SelectionSelected:
		ids, ok := normalizeIDs(selection.IDs)
		if !ok {
			return leadFilter{}, "", ErrInvalidRequest
		}
		return leadFilter{ids: ids, byIDs: true}, "", nil
	case SelectionFilter:
		status, ok := normalizeStatus(selection.Status)
		if !ok {
			return leadFilter{}, "", ErrInvalidRequest
		}
		return leadFilter{source: normalizeSourceFilter(selection.Source), search: normalizeSearch(selection.Search)}, status, nil
	default:
		return leadFilter{}, "", ErrInvalidRequest
	}
}

func normalizeIDs(raw []string) ([]string, bool) {
	if len(raw) == 0 || len(raw) > maxSelectedIDs {
		return nil, false
	}
	seen := map[string]struct{}{}
	ids := make([]string, 0, len(raw))
	for _, value := range raw {
		id := strings.TrimSpace(value)
		if id == "" {
			return nil, false
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	return ids, true
}

func normalizeStatus(value string) (string, bool) {
	switch status := strings.TrimSpace(value); status {
	case "", StatusNotSent, StatusSent, StatusUnsubscribed, StatusBounced:
		return status, true
	default:
		return "", false
	}
}

func normalizeSourceFilter(value string) string {
	source := strings.TrimSpace(value)
	if source == NoSource {
		return NoSource
	}
	return clip(source, maxSourceRunes)
}

func normalizeSearch(value string) string {
	return strings.ToLower(clip(value, maxSearchRunes))
}

// clip 去掉首尾空白、把换行和制表符换成空格，最多保留 limit 个字。
func clip(value string, limit int) string {
	value = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r == '\t' {
			return ' '
		}
		return r
	}, value))
	runes := []rune(value)
	if len(runes) > limit {
		return strings.TrimSpace(string(runes[:limit]))
	}
	return value
}

// normalizeEmail 检查一个邮箱地址，返回原样（去掉首尾空白）和转小写后的写法。
// 域名里要有点（排除 localhost 之类），不接受「名字 <地址>」这种写法。
func normalizeEmail(raw string) (string, string, bool) {
	email := strings.TrimSpace(raw)
	if email == "" || len(email) > 254 || strings.ContainsFunc(email, unicode.IsSpace) {
		return "", "", false
	}
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Name != "" || parsed.Address != email {
		return "", "", false
	}
	at := strings.LastIndex(email, "@")
	if at <= 0 || at == len(email)-1 {
		return "", "", false
	}
	domain := email[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", "", false
	}
	return email, strings.ToLower(email), true
}

type parsedEmail struct {
	email      string
	normalized string
}

// importSeparator：逗号、分号、冒号（中英文）、顿号、竖线和各种空白都当分隔符，粘贴表格、CSV、聊天记录
// （「邮箱：a@b.com」）都能用。
func importSeparator(r rune) bool {
	switch r {
	case ',', ';', '，', '；', '、', '|', ':', '：':
		return true
	}
	return unicode.IsSpace(r)
}

const importTrim = "\"'`<>()[]{}《》「」“”‘’.。!！?？"

// parseImport 从文本里找出邮箱：返回去重后的邮箱、重复的数量和格式不对的数量（只算带 @ 的）。
func parseImport(text string) ([]parsedEmail, int, int) {
	var emails []parsedEmail
	seen := map[string]struct{}{}
	duplicates, invalid := 0, 0
	for _, token := range strings.FieldsFunc(text, importSeparator) {
		// 「名字<地址>」：取尖括号里面的。
		if open, close := strings.Index(token, "<"), strings.LastIndex(token, ">"); open >= 0 && close > open {
			token = token[open+1 : close]
		}
		if len(token) >= 7 && strings.EqualFold(token[:7], "mailto:") {
			token = token[7:]
		}
		token = strings.Trim(token, importTrim)
		if !strings.Contains(token, "@") {
			continue
		}
		email, normalized, ok := normalizeEmail(token)
		if !ok {
			invalid++
			continue
		}
		if _, exists := seen[normalized]; exists {
			duplicates++
			continue
		}
		seen[normalized] = struct{}{}
		emails = append(emails, parsedEmail{email: email, normalized: normalized})
	}
	return emails, duplicates, invalid
}

func newID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func pages(total int, pageSize int) int {
	if total <= 0 || pageSize <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) / float64(pageSize)))
}
