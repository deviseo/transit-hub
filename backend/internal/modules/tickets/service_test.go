package tickets

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// fakeTicketRepository 是 ticketRepository 的内存假实现，在过滤逻辑上镜像 repository.go 里
// 真实 SQL 查询的 WHERE 条件（user_id/admin_account_id/sub2api_src_host/sub2api_user_id），
// 用于在不连接真实 PostgreSQL 的前提下验证 Service 层的工作区/用户隔离是否正确传参。
type fakeTicketRepository struct {
	configsByWorkspace map[string]*EmbedConfig
	configsByToken     map[string]*EmbedConfig
	tickets            map[string]*Ticket
	messages           map[string][]*TicketMessage
	attachments        map[string]*TicketAttachment
}

func newFakeTicketRepository() *fakeTicketRepository {
	return &fakeTicketRepository{
		configsByWorkspace: map[string]*EmbedConfig{},
		configsByToken:     map[string]*EmbedConfig{},
		tickets:            map[string]*Ticket{},
		messages:           map[string][]*TicketMessage{},
		attachments:        map[string]*TicketAttachment{},
	}
}

func workspaceKey(userID, adminAccountID string) string { return userID + "|" + adminAccountID }

func (f *fakeTicketRepository) EnsureSchema(ctx context.Context) error { return nil }

func (f *fakeTicketRepository) GetEmbedConfigByToken(ctx context.Context, embedToken string) (*EmbedConfig, error) {
	c, ok := f.configsByToken[embedToken]
	if !ok {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (f *fakeTicketRepository) GetEmbedConfigByWorkspace(ctx context.Context, userID string, adminAccountID string) (*EmbedConfig, error) {
	c, ok := f.configsByWorkspace[workspaceKey(userID, adminAccountID)]
	if !ok {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (f *fakeTicketRepository) InsertEmbedConfig(ctx context.Context, c EmbedConfig) error {
	key := workspaceKey(c.UserID, c.AdminAccountID)
	if _, exists := f.configsByWorkspace[key]; exists {
		return nil // 与真实仓库的 ON CONFLICT DO NOTHING 行为一致
	}
	cp := c
	f.configsByWorkspace[key] = &cp
	f.configsByToken[c.EmbedToken] = &cp
	return nil
}

// UpdateEmbedConfig 镜像 repository.go 的真实行为：写入 template/maxImagesPerTicket 的同时把
// enabled/allowedSrcHost 强制修正为 true/""，模拟"取消这两项配置能力后顺带修复历史遗留数据"的效果。
func (f *fakeTicketRepository) UpdateEmbedConfig(ctx context.Context, userID string, adminAccountID string, template string, maxImagesPerTicket int, categoryOptions []string, priorityOptions []string) error {
	c, ok := f.configsByWorkspace[workspaceKey(userID, adminAccountID)]
	if !ok {
		return errors.New("config not found")
	}
	c.Enabled = true
	c.AllowedSrcHost = ""
	c.Template = template
	c.MaxImagesPerTicket = maxImagesPerTicket
	c.CategoryOptions = categoryOptions
	c.PriorityOptions = priorityOptions
	return nil
}

func (f *fakeTicketRepository) RotateEmbedToken(ctx context.Context, userID string, adminAccountID string, newToken string) error {
	c, ok := f.configsByWorkspace[workspaceKey(userID, adminAccountID)]
	if !ok {
		return errors.New("config not found")
	}
	delete(f.configsByToken, c.EmbedToken)
	c.EmbedToken = newToken
	f.configsByToken[newToken] = c
	return nil
}

func (f *fakeTicketRepository) InsertTicketWithMessage(ctx context.Context, t Ticket, m TicketMessage, attachments []TicketAttachment) error {
	cp := t
	f.tickets[t.ID] = &cp
	if err := f.InsertMessage(ctx, m); err != nil {
		return err
	}
	for _, a := range attachments {
		acp := a
		f.attachments[a.ID] = &acp
	}
	return nil
}

func (f *fakeTicketRepository) ListAttachmentsByTicket(ctx context.Context, ticketID string) ([]TicketAttachment, error) {
	out := make([]TicketAttachment, 0)
	for _, a := range f.attachments {
		if a.TicketID == ticketID {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (f *fakeTicketRepository) GetAttachmentByID(ctx context.Context, id string) (*TicketAttachment, error) {
	a, ok := f.attachments[id]
	if !ok {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (f *fakeTicketRepository) InsertMessage(ctx context.Context, m TicketMessage) error {
	cp := m
	f.messages[m.TicketID] = append(f.messages[m.TicketID], &cp)
	return nil
}

func (f *fakeTicketRepository) ListMessages(ctx context.Context, ticketID string) ([]TicketMessage, error) {
	out := make([]TicketMessage, 0)
	for _, m := range f.messages[ticketID] {
		out = append(out, *m)
	}
	return out, nil
}

func (f *fakeTicketRepository) ListEmbedTickets(ctx context.Context, userID string, adminAccountID string, srcHost string, sub2apiUserID string) ([]Ticket, error) {
	out := make([]Ticket, 0)
	for _, t := range f.tickets {
		if t.UserID == userID && t.AdminAccountID == adminAccountID && t.Sub2apiSrcHost == srcHost && t.Sub2apiUserID == sub2apiUserID {
			out = append(out, *t)
		}
	}
	return out, nil
}

func (f *fakeTicketRepository) GetEmbedTicket(ctx context.Context, userID string, adminAccountID string, srcHost string, sub2apiUserID string, id string) (*Ticket, error) {
	t, ok := f.tickets[id]
	if !ok || t.UserID != userID || t.AdminAccountID != adminAccountID || t.Sub2apiSrcHost != srcHost || t.Sub2apiUserID != sub2apiUserID {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (f *fakeTicketRepository) ListAdminTickets(ctx context.Context, userID string, adminAccountID string, status string, page int, pageSize int) ([]Ticket, int, error) {
	matched := make([]Ticket, 0)
	for _, t := range f.tickets {
		if t.UserID != userID || t.AdminAccountID != adminAccountID {
			continue
		}
		if status != "" && t.Status != status {
			continue
		}
		matched = append(matched, *t)
	}
	total := len(matched)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return matched[start:end], total, nil
}

func (f *fakeTicketRepository) GetAdminTicket(ctx context.Context, userID string, adminAccountID string, id string) (*Ticket, error) {
	t, ok := f.tickets[id]
	if !ok || t.UserID != userID || t.AdminAccountID != adminAccountID {
		return nil, nil
	}
	cp := *t
	return &cp, nil
}

func (f *fakeTicketRepository) TouchTicket(ctx context.Context, id string, status string, lastMessageAt time.Time) error {
	t, ok := f.tickets[id]
	if !ok {
		return errors.New("ticket not found")
	}
	t.Status = status
	t.LastMessageAt = lastMessageAt
	return nil
}

func (f *fakeTicketRepository) UpdateStatus(ctx context.Context, id string, status string) error {
	t, ok := f.tickets[id]
	if !ok {
		return errors.New("ticket not found")
	}
	t.Status = status
	return nil
}

// fakeSessionStore 是 embedSessionStore 的内存假实现。
type fakeSessionStore struct {
	sessions map[string]EmbedSession
}

func newFakeSessionStore() *fakeSessionStore {
	return &fakeSessionStore{sessions: map[string]EmbedSession{}}
}

func (f *fakeSessionStore) Save(ctx context.Context, token string, session EmbedSession) error {
	f.sessions[token] = session
	return nil
}

func (f *fakeSessionStore) Get(ctx context.Context, token string) (*EmbedSession, error) {
	session, ok := f.sessions[token]
	if !ok {
		return nil, nil
	}
	cp := session
	return &cp, nil
}

// fakeSub2API 是 sub2APIFetcher 的假实现，按测试用例预设固定返回值/错误。
type fakeSub2API struct {
	user Sub2APIUser
	err  error
}

func (f *fakeSub2API) FetchCurrentUser(srcHost string, token string) (Sub2APIUser, error) {
	return f.user, f.err
}

// fakeAccountResolver 是 AdminAccountResolver 的假实现。
type fakeAccountResolver struct {
	id  string
	err error
}

func (f *fakeAccountResolver) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	return f.id, f.err
}

// fakeAdminSessionProvider 是 adminSessionProvider 的假实现。
type fakeAdminSessionProvider struct {
	session upstream.Session
	err     error
}

func (f *fakeAdminSessionProvider) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	return f.session, f.err
}

// fakeSub2APIAdminClient 是 sub2APIAdminClient 的假实现。
type fakeSub2APIAdminClient struct {
	user       upstream.Sub2APIAdminUser
	userErr    error
	history    upstream.Sub2APIUserBalanceHistory
	historyErr error
}

func (f *fakeSub2APIAdminClient) FetchSub2APIAdminUser(session upstream.Session, userID string) (upstream.Sub2APIAdminUser, error) {
	return f.user, f.userErr
}

func (f *fakeSub2APIAdminClient) FetchSub2APIAdminUserBalanceHistory(session upstream.Session, userID string, page int, pageSize int, codeType string) (upstream.Sub2APIUserBalanceHistory, error) {
	return f.history, f.historyErr
}

// fakeAttachmentStorage 是 attachmentStorage 的内存假实现，用 storagePath 当 map key。
type fakeAttachmentStorage struct {
	files map[string][]byte
	n     int
}

func newFakeAttachmentStorage() *fakeAttachmentStorage {
	return &fakeAttachmentStorage{files: map[string][]byte{}}
}

func (f *fakeAttachmentStorage) Save(contentType string, data []byte) (string, error) {
	f.n++
	path := "fake-" + string(rune('a'+f.n-1))
	cp := make([]byte, len(data))
	copy(cp, data)
	f.files[path] = cp
	return path, nil
}

func (f *fakeAttachmentStorage) Read(storagePath string) ([]byte, error) {
	data, ok := f.files[storagePath]
	if !ok {
		return nil, errors.New("file not found")
	}
	return data, nil
}

func (f *fakeAttachmentStorage) Delete(storagePath string) error {
	delete(f.files, storagePath)
	return nil
}

// sequentialIDs 返回一个每次调用递增的 idGenerator，让测试断言可预期的 ID 值。
func sequentialIDs(prefix string) idGenerator {
	n := 0
	return func() (string, error) {
		n++
		return prefix + "-" + string(rune('a'+n-1)), nil
	}
}

func newTestService(repo *fakeTicketRepository, sessions *fakeSessionStore, sub2api *fakeSub2API, accounts *fakeAccountResolver) *Service {
	return &Service{
		repository: repo,
		sessions:   sessions,
		sub2api:    sub2api,
		storage:    newFakeAttachmentStorage(),
		accounts:   accounts,
		newID:      sequentialIDs("id"),
		newToken:   sequentialIDs("token"),
		now:        time.Now,
	}
}

func seedEmbedConfig(repo *fakeTicketRepository, userID, adminAccountID, embedToken string, enabled bool, allowedSrcHost string) {
	c := &EmbedConfig{UserID: userID, AdminAccountID: adminAccountID, EmbedToken: embedToken, Enabled: enabled, AllowedSrcHost: allowedSrcHost, Template: TemplateDefault}
	repo.configsByWorkspace[workspaceKey(userID, adminAccountID)] = c
	repo.configsByToken[embedToken] = c
}

// ---- iframe session 校验 ----

func TestCreateEmbedSession_Success(t *testing.T) {
	repo := newFakeTicketRepository()
	seedEmbedConfig(repo, "user1", "account1", "embed-token", true, "")
	sub2api := &fakeSub2API{user: Sub2APIUser{ID: "42", Email: "sub2api@example.com", Role: "member"}}
	svc := newTestService(repo, newFakeSessionStore(), sub2api, nil)

	resp, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   "embed-token",
		Sub2apiToken: "sub2api-jwt",
		UrlUserID:    "42",
		SrcHost:      "https://web.example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.SessionToken == "" {
		t.Fatalf("expected non-empty session token")
	}

	session, err := svc.sessions.Get(context.Background(), resp.SessionToken)
	if err != nil {
		t.Fatalf("unexpected error reading back session: %v", err)
	}
	if session == nil {
		t.Fatalf("expected session to be persisted")
	}
	if session.UserID != "user1" || session.AdminAccountID != "account1" || session.Sub2apiUserID != "42" {
		t.Fatalf("unexpected session contents: %+v", session)
	}
}

// TestCreateEmbedSession_Sub2apiUnauthorized 校验 Sub2API /auth/me 返回 401 时拒绝签发会话。
func TestCreateEmbedSession_Sub2apiUnauthorized(t *testing.T) {
	repo := newFakeTicketRepository()
	seedEmbedConfig(repo, "user1", "account1", "embed-token", true, "")
	sub2api := &fakeSub2API{err: &sub2APIError{unauthorized: true, detail: "sub2api auth failed status=401"}}
	svc := newTestService(repo, newFakeSessionStore(), sub2api, nil)

	_, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   "embed-token",
		Sub2apiToken: "bad-token",
		SrcHost:      "https://web.example.com",
	})
	if !errors.Is(err, requestError(ErrorEmbedSub2apiAuth)) {
		t.Fatalf("expected ErrorEmbedSub2apiAuth, got %v", err)
	}
}

// TestCreateEmbedSession_UserIDMismatch 校验 URL user_id 与 /auth/me 返回的用户 ID 不一致时拒绝。
func TestCreateEmbedSession_UserIDMismatch(t *testing.T) {
	repo := newFakeTicketRepository()
	seedEmbedConfig(repo, "user1", "account1", "embed-token", true, "")
	sub2api := &fakeSub2API{user: Sub2APIUser{ID: "42"}}
	svc := newTestService(repo, newFakeSessionStore(), sub2api, nil)

	_, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   "embed-token",
		Sub2apiToken: "sub2api-jwt",
		UrlUserID:    "99",
		SrcHost:      "https://web.example.com",
	})
	if !errors.Is(err, requestError(ErrorEmbedUserMismatch)) {
		t.Fatalf("expected ErrorEmbedUserMismatch, got %v", err)
	}
}

// TestCreateEmbedSession_LegacyDisabledConfigStillWorks 校验第二阶段取消"启用嵌入工单"配置后，
// 历史上被设置为 enabled=false 的旧数据不会继续阻止 iframe 会话建立。
func TestCreateEmbedSession_LegacyDisabledConfigStillWorks(t *testing.T) {
	repo := newFakeTicketRepository()
	seedEmbedConfig(repo, "user1", "account1", "embed-token", false, "")
	sub2api := &fakeSub2API{user: Sub2APIUser{ID: "42"}}
	svc := newTestService(repo, newFakeSessionStore(), sub2api, nil)

	resp, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   "embed-token",
		Sub2apiToken: "sub2api-jwt",
		SrcHost:      "https://web.example.com",
	})
	if err != nil {
		t.Fatalf("expected legacy disabled config to still allow session creation, got error: %v", err)
	}
	if resp.SessionToken == "" {
		t.Fatalf("expected non-empty session token")
	}
}

func TestCreateEmbedSession_ConfigNotFound(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)

	_, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   "missing-token",
		Sub2apiToken: "sub2api-jwt",
		SrcHost:      "https://web.example.com",
	})
	if !errors.Is(err, requestError(ErrorEmbedConfigNotFound)) {
		t.Fatalf("expected ErrorEmbedConfigNotFound, got %v", err)
	}
}

// TestCreateEmbedSession_LegacyAllowedSrcHostIgnored 校验第二阶段取消"允许来源域名"配置后，
// 历史上限制过来源域名的旧数据不会继续拒绝来自其它域名的会话请求。
func TestCreateEmbedSession_LegacyAllowedSrcHostIgnored(t *testing.T) {
	repo := newFakeTicketRepository()
	seedEmbedConfig(repo, "user1", "account1", "embed-token", true, "https://allowed.example.com")
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{user: Sub2APIUser{ID: "42"}}, nil)

	resp, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   "embed-token",
		Sub2apiToken: "sub2api-jwt",
		SrcHost:      "https://other.example.com",
	})
	if err != nil {
		t.Fatalf("expected legacy allowedSrcHost to no longer restrict src_host, got error: %v", err)
	}
	if resp.SessionToken == "" {
		t.Fatalf("expected non-empty session token")
	}
}

// TestCreateEmbedSession_InvalidSrcHostStillRejected 校验取消"允许来源域名"白名单不等于取消
// src_host 的基础合法性校验：明显非法的 host 仍然必须被拒绝（SSRF 防护）。
func TestCreateEmbedSession_InvalidSrcHostStillRejected(t *testing.T) {
	repo := newFakeTicketRepository()
	seedEmbedConfig(repo, "user1", "account1", "embed-token", true, "")
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{user: Sub2APIUser{ID: "42"}}, nil)

	_, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   "embed-token",
		Sub2apiToken: "sub2api-jwt",
		SrcHost:      "ftp://not-http",
	})
	if !errors.Is(err, requestError(ErrorEmbedInvalidSrcHost)) {
		t.Fatalf("expected ErrorEmbedInvalidSrcHost, got %v", err)
	}
}

// TestCreateEmbedSession_IncludesWorkspaceTemplate 校验会话响应携带当前 workspace 的模板，
// 供前端立即应用视觉样式，不需要额外请求。
func TestCreateEmbedSession_IncludesWorkspaceTemplate(t *testing.T) {
	repo := newFakeTicketRepository()
	c := &EmbedConfig{UserID: "user1", AdminAccountID: "account1", EmbedToken: "embed-token", Enabled: true, Template: TemplateSupport}
	repo.configsByWorkspace[workspaceKey("user1", "account1")] = c
	repo.configsByToken["embed-token"] = c
	sub2api := &fakeSub2API{user: Sub2APIUser{ID: "42"}}
	svc := newTestService(repo, newFakeSessionStore(), sub2api, nil)

	resp, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   "embed-token",
		Sub2apiToken: "sub2api-jwt",
		SrcHost:      "https://web.example.com",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Template != TemplateSupport {
		t.Fatalf("expected session response template %q, got %q", TemplateSupport, resp.Template)
	}

	session, err := svc.sessions.Get(context.Background(), resp.SessionToken)
	if err != nil {
		t.Fatalf("unexpected error reading back session: %v", err)
	}
	if session == nil || session.Template != TemplateSupport {
		t.Fatalf("expected stored session to carry template %q, got %+v", TemplateSupport, session)
	}
}

// ---- 新增工单 ----

func establishedSession(svc *Service, t *testing.T) string {
	t.Helper()
	sessionToken := "session-1"
	svc.sessions.(*fakeSessionStore).sessions[sessionToken] = EmbedSession{
		UserID: "user1", AdminAccountID: "account1", SrcHost: "https://web.example.com", Sub2apiUserID: "42", Sub2apiEmail: "sub2api@example.com",
	}
	return sessionToken
}

func TestCreateTicket_ManualEmailRequired(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "",
		Title:       "title",
		Body:        "body",
	}, nil)
	if !errors.Is(err, requestError(ErrorEmbedInvalidEmail)) {
		t.Fatalf("expected ErrorEmbedInvalidEmail, got %v", err)
	}
}

func TestCreateTicket_Success(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	detail, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "something is broken",
		Category:    "通用问题",
		Priority:    "普通",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Status != StatusOpen {
		t.Fatalf("expected new ticket status open, got %q", detail.Status)
	}
	if len(detail.Messages) != 1 || detail.Messages[0].Body != "something is broken" {
		t.Fatalf("expected first customer message to be saved, got %+v", detail.Messages)
	}
	if detail.Category != "通用问题" || detail.Priority != "普通" {
		t.Fatalf("expected category/priority to be persisted, got %+v", detail)
	}
}

// ---- iframe 隔离 ----

func TestListMyTickets_OnlyOwnTickets(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = &Ticket{ID: "t1", UserID: "user1", AdminAccountID: "account1", Sub2apiSrcHost: "https://web.example.com", Sub2apiUserID: "42", ManualEmail: "a@example.com", Title: "mine"}
	repo.tickets["t2"] = &Ticket{ID: "t2", UserID: "user1", AdminAccountID: "account1", Sub2apiSrcHost: "https://web.example.com", Sub2apiUserID: "43", ManualEmail: "b@example.com", Title: "not mine"}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	resp, err := svc.ListMyTickets(context.Background(), sessionToken)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "t1" {
		t.Fatalf("expected only own ticket t1, got %+v", resp.Items)
	}
}

// TestGetMyTicket_CannotReadOtherSub2apiUsersTicket 校验同一工作区内，iframe 用户不能通过猜测
// 工单 ID 读取另一个 Sub2API 用户的工单。
func TestGetMyTicket_CannotReadOtherSub2apiUsersTicket(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["other-user-ticket"] = &Ticket{
		ID: "other-user-ticket", UserID: "user1", AdminAccountID: "account1",
		Sub2apiSrcHost: "https://web.example.com", Sub2apiUserID: "999", ManualEmail: "other@example.com", Title: "not mine",
	}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	_, err := svc.GetMyTicket(context.Background(), sessionToken, "other-user-ticket")
	if !errors.Is(err, requestError(ErrorNotFound)) {
		t.Fatalf("expected ErrorNotFound when reading another sub2api user's ticket, got %v", err)
	}
}

func TestAddCustomerMessage_RejectedWhenClosed(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["closed-ticket"] = &Ticket{
		ID: "closed-ticket", UserID: "user1", AdminAccountID: "account1",
		Sub2apiSrcHost: "https://web.example.com", Sub2apiUserID: "42", ManualEmail: "user@example.com", Title: "t", Status: StatusClosed,
	}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	_, err := svc.AddCustomerMessage(context.Background(), sessionToken, "closed-ticket", CreateMessageRequest{Body: "still broken"})
	if !errors.Is(err, requestError(ErrorTicketClosed)) {
		t.Fatalf("expected ErrorTicketClosed, got %v", err)
	}
}

// ---- 后台接口 ----

func TestListTickets_IsolatedByWorkspace(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = &Ticket{ID: "t1", UserID: "user1", AdminAccountID: "account1", ManualEmail: "a@example.com", Title: "workspace1 ticket"}
	repo.tickets["t2"] = &Ticket{ID: "t2", UserID: "user1", AdminAccountID: "account2", ManualEmail: "b@example.com", Title: "workspace2 ticket"}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	resp, err := svc.ListTickets(context.Background(), "user1", AdminListQuery{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].ID != "t1" {
		t.Fatalf("expected only workspace1's ticket, got %+v", resp.Items)
	}
}

func TestAddAdminMessage_SetsRepliedStatus(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = &Ticket{ID: "t1", UserID: "user1", AdminAccountID: "account1", ManualEmail: "a@example.com", Title: "t", Status: StatusOpen}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	detail, err := svc.AddAdminMessage(context.Background(), "user1", "t1", CreateMessageRequest{Body: "we are looking into it"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if detail.Status != StatusReplied {
		t.Fatalf("expected status replied after admin reply, got %q", detail.Status)
	}
	if len(detail.Messages) != 1 || detail.Messages[0].AuthorType != AuthorAdmin {
		t.Fatalf("expected one admin message, got %+v", detail.Messages)
	}
}

func TestAddAdminMessage_BodyRequired(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = &Ticket{ID: "t1", UserID: "user1", AdminAccountID: "account1", ManualEmail: "a@example.com", Title: "t", Status: StatusOpen}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	_, err := svc.AddAdminMessage(context.Background(), "user1", "t1", CreateMessageRequest{Body: "  "})
	if !errors.Is(err, requestError(ErrorBodyRequired)) {
		t.Fatalf("expected ErrorBodyRequired, got %v", err)
	}
}

func TestUpdateStatus_RejectsUnknownStatus(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = &Ticket{ID: "t1", UserID: "user1", AdminAccountID: "account1", ManualEmail: "a@example.com", Title: "t", Status: StatusOpen}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	_, err := svc.UpdateStatus(context.Background(), "user1", "t1", UpdateStatusRequest{Status: "archived"})
	if !errors.Is(err, requestError(ErrorInvalidStatus)) {
		t.Fatalf("expected ErrorInvalidStatus, got %v", err)
	}
}

func TestGetEmbedConfig_AutoCreatesDefault(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	config, err := svc.GetEmbedConfig(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.EmbedToken == "" || !config.Enabled {
		t.Fatalf("expected auto-created enabled config with a token, got %+v", config)
	}
	if config.Template != TemplateDefault {
		t.Fatalf("expected auto-created config to default to template %q, got %q", TemplateDefault, config.Template)
	}

	// 第二次调用必须复用同一条配置，而不是重新生成一个新 token。
	again, err := svc.GetEmbedConfig(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if again.EmbedToken != config.EmbedToken {
		t.Fatalf("expected embed config to be stable across calls, got %q then %q", config.EmbedToken, again.EmbedToken)
	}
}

// ---- 嵌入配置：模板保存与兼容 ----

func TestUpdateEmbedConfig_SavesValidTemplate(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	config, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{Template: TemplateMinimal})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Template != TemplateMinimal {
		t.Fatalf("expected template %q, got %q", TemplateMinimal, config.Template)
	}

	// 重新读取必须看到已保存的模板，而不是每次都重置回 default。
	reloaded, err := svc.GetEmbedConfig(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reloaded.Template != TemplateMinimal {
		t.Fatalf("expected reloaded template %q, got %q", TemplateMinimal, reloaded.Template)
	}
}

func TestUpdateEmbedConfig_RejectsInvalidTemplate(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	_, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{Template: "flashy"})
	if !errors.Is(err, requestError(ErrorInvalidTemplate)) {
		t.Fatalf("expected ErrorInvalidTemplate, got %v", err)
	}
}

func TestUpdateEmbedConfig_MissingTemplateDefaultsToDefault(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	config, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.Template != TemplateDefault {
		t.Fatalf("expected missing template to default to %q, got %q", TemplateDefault, config.Template)
	}
}

// TestUpdateEmbedConfig_LegacyEnabledFalseDoesNotDisable 校验旧前端仍然传 enabled=false 时
// 不会把配置改成 disabled——第二阶段起这个字段完全不再影响行为。
func TestUpdateEmbedConfig_LegacyEnabledFalseDoesNotDisable(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	disabled := false
	config, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{Enabled: &disabled, Template: TemplateDefault})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !config.Enabled {
		t.Fatalf("expected config to remain enabled regardless of legacy enabled=false request, got %+v", config)
	}
}

// TestUpdateEmbedConfig_LegacyAllowedSrcHostRequestIgnored 校验旧前端仍然传 allowedSrcHost 时
// 不会继续限制 iframe 来源（保存后 CreateEmbedSession 必须仍然对任意来源成功）。
func TestUpdateEmbedConfig_LegacyAllowedSrcHostRequestIgnored(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{user: Sub2APIUser{ID: "42"}}, &fakeAccountResolver{id: "account1"})

	if _, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{AllowedSrcHost: "https://only-this-host.example.com", Template: TemplateDefault}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	config, err := svc.GetEmbedConfig(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.AllowedSrcHost != "" {
		t.Fatalf("expected allowedSrcHost to remain cleared, got %q", config.AllowedSrcHost)
	}

	resp, err := svc.CreateEmbedSession(context.Background(), CreateSessionRequest{
		EmbedToken:   config.EmbedToken,
		Sub2apiToken: "sub2api-jwt",
		SrcHost:      "https://some-other-host.example.com",
	})
	if err != nil {
		t.Fatalf("expected session creation from an unlisted host to succeed, got error: %v", err)
	}
	if resp.SessionToken == "" {
		t.Fatalf("expected non-empty session token")
	}
}

// ---- 嵌入配置：图片数量上限 ----

func TestUpdateEmbedConfig_SavesValidMaxImages(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	maxImages := 3
	config, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{Template: TemplateDefault, MaxImagesPerTicket: &maxImages})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.MaxImagesPerTicket != 3 {
		t.Fatalf("expected maxImagesPerTicket 3, got %d", config.MaxImagesPerTicket)
	}
}

func TestUpdateEmbedConfig_RejectsInvalidMaxImages(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	tooMany := 10
	_, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{Template: TemplateDefault, MaxImagesPerTicket: &tooMany})
	if !errors.Is(err, requestError(ErrorInvalidMaxImages)) {
		t.Fatalf("expected ErrorInvalidMaxImages for value above range, got %v", err)
	}

	negative := -1
	_, err = svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{Template: TemplateDefault, MaxImagesPerTicket: &negative})
	if !errors.Is(err, requestError(ErrorInvalidMaxImages)) {
		t.Fatalf("expected ErrorInvalidMaxImages for negative value, got %v", err)
	}
}

// TestUpdateEmbedConfig_MissingMaxImagesKeepsExisting 校验请求体不携带 maxImagesPerTicket 时
// （nil 指针）保留已保存的值，而不是被静默重置为 0。
func TestUpdateEmbedConfig_MissingMaxImagesKeepsExisting(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	maxImages := 5
	if _, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{Template: TemplateDefault, MaxImagesPerTicket: &maxImages}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	config, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{Template: TemplateMinimal})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.MaxImagesPerTicket != 5 {
		t.Fatalf("expected maxImagesPerTicket to remain 5 when omitted, got %d", config.MaxImagesPerTicket)
	}
}

func TestGetEmbedConfig_DefaultsMaxImagesToZero(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	config, err := svc.GetEmbedConfig(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if config.MaxImagesPerTicket != 0 {
		t.Fatalf("expected auto-created config to default maxImagesPerTicket to 0 (uploads disabled), got %d", config.MaxImagesPerTicket)
	}
}

// ---- 嵌入配置：分类/优先级选项 ----

func TestGetEmbedConfig_DefaultsCategoryAndPriorityOptions(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	config, err := svc.GetEmbedConfig(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(config.CategoryOptions, DefaultCategoryOptions) {
		t.Fatalf("expected default category options %v, got %v", DefaultCategoryOptions, config.CategoryOptions)
	}
	if !slices.Equal(config.PriorityOptions, DefaultPriorityOptions) {
		t.Fatalf("expected default priority options %v, got %v", DefaultPriorityOptions, config.PriorityOptions)
	}
}

// TestGetEmbedConfig_LegacyConfigWithoutOptionsFallsBackToDefaults 覆盖"旧配置或 fake repository
// 中没有显式 options"的场景：seedEmbedConfig 绕过 InsertEmbedConfig 直接写入一条没有
// CategoryOptions/PriorityOptions 的历史配置，读取路径必须退回默认选项而不是 panic 或返回空数组。
func TestGetEmbedConfig_LegacyConfigWithoutOptionsFallsBackToDefaults(t *testing.T) {
	repo := newFakeTicketRepository()
	seedEmbedConfig(repo, "user1", "account1", "embed-token", true, "")
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	config, err := svc.GetEmbedConfig(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(config.CategoryOptions, DefaultCategoryOptions) {
		t.Fatalf("expected default category options for legacy config without explicit options, got %v", config.CategoryOptions)
	}
	if !slices.Equal(config.PriorityOptions, DefaultPriorityOptions) {
		t.Fatalf("expected default priority options for legacy config without explicit options, got %v", config.PriorityOptions)
	}
}

func TestUpdateEmbedConfig_SavesCategoryAndPriorityOptions(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	categories := []string{"Billing", "Bugs"}
	priorities := []string{"Low", "High"}
	config, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{
		Template:        TemplateDefault,
		CategoryOptions: categories,
		PriorityOptions: priorities,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(config.CategoryOptions, categories) {
		t.Fatalf("expected category options %v, got %v", categories, config.CategoryOptions)
	}
	if !slices.Equal(config.PriorityOptions, priorities) {
		t.Fatalf("expected priority options %v, got %v", priorities, config.PriorityOptions)
	}

	// 只更新分类、不传优先级：优先级必须保留上一次保存的值，而不是被静默重置为默认值。
	onlyCategoryUpdate := []string{"Only Category"}
	reloaded, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{
		Template:        TemplateDefault,
		CategoryOptions: onlyCategoryUpdate,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !slices.Equal(reloaded.CategoryOptions, onlyCategoryUpdate) {
		t.Fatalf("expected category options updated to %v, got %v", onlyCategoryUpdate, reloaded.CategoryOptions)
	}
	if !slices.Equal(reloaded.PriorityOptions, priorities) {
		t.Fatalf("expected priority options to remain %v when omitted, got %v", priorities, reloaded.PriorityOptions)
	}
}

func TestUpdateEmbedConfig_DedupesCategoryOptionsPreservingOrder(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	config, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{
		Template:        TemplateDefault,
		CategoryOptions: []string{" Billing ", "Bugs", "Billing", " Bugs"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"Billing", "Bugs"}
	if !slices.Equal(config.CategoryOptions, want) {
		t.Fatalf("expected deduped/trimmed options %v, got %v", want, config.CategoryOptions)
	}
}

func TestUpdateEmbedConfig_RejectsInvalidCategoryOptions(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	tooMany := make([]string, 21)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("opt-%d", i)
	}

	cases := []struct {
		name    string
		options []string
	}{
		{"empty array", []string{}},
		{"blank entries only", []string{"  ", "\t"}},
		{"entry too long", []string{strings.Repeat("x", 41)}},
		{"too many entries", tooMany},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{
				Template:        TemplateDefault,
				CategoryOptions: tc.options,
			})
			if !errors.Is(err, requestError(ErrorInvalidCategoryOptions)) {
				t.Fatalf("expected ErrorInvalidCategoryOptions, got %v", err)
			}
		})
	}
}

func TestUpdateEmbedConfig_RejectsInvalidPriorityOptions(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	_, err := svc.UpdateEmbedConfig(context.Background(), "user1", UpdateEmbedConfigRequest{
		Template:        TemplateDefault,
		PriorityOptions: []string{},
	})
	if !errors.Is(err, requestError(ErrorInvalidPriorityOptions)) {
		t.Fatalf("expected ErrorInvalidPriorityOptions, got %v", err)
	}
}

// ---- 新建工单：分类/优先级必填与校验 ----

func TestCreateTicket_CategoryRequired(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "body",
		Priority:    "普通",
	}, nil)
	if !errors.Is(err, requestError(ErrorEmbedCategoryRequired)) {
		t.Fatalf("expected ErrorEmbedCategoryRequired, got %v", err)
	}
}

func TestCreateTicket_PriorityRequired(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "body",
		Category:    "通用问题",
	}, nil)
	if !errors.Is(err, requestError(ErrorEmbedPriorityRequired)) {
		t.Fatalf("expected ErrorEmbedPriorityRequired, got %v", err)
	}
}

func TestCreateTicket_RejectsCategoryNotInWorkspaceOptions(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "body",
		Category:    "not-a-real-category",
		Priority:    "普通",
	}, nil)
	if !errors.Is(err, requestError(ErrorEmbedInvalidCategory)) {
		t.Fatalf("expected ErrorEmbedInvalidCategory, got %v", err)
	}
}

func TestCreateTicket_RejectsPriorityNotInWorkspaceOptions(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSession(svc, t)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "body",
		Category:    "通用问题",
		Priority:    "not-a-real-priority",
	}, nil)
	if !errors.Is(err, requestError(ErrorEmbedInvalidPriority)) {
		t.Fatalf("expected ErrorEmbedInvalidPriority, got %v", err)
	}
}

// TestCreateTicket_ValidatesAgainstCustomWorkspaceOptions 校验校验规则使用的是当前 workspace 的
// 实时配置，而不是硬编码的默认选项组：管理员把选项改成自定义值后，旧的默认分类必须被拒绝，
// 新的自定义分类必须被接受并落库。
func TestCreateTicket_ValidatesAgainstCustomWorkspaceOptions(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 0)
	repo.configsByWorkspace[workspaceKey("user1", "account1")].CategoryOptions = []string{"Billing"}
	repo.configsByWorkspace[workspaceKey("user1", "account1")].PriorityOptions = []string{"Urgent"}

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "body",
		Category:    "通用问题",
		Priority:    "Urgent",
	}, nil)
	if !errors.Is(err, requestError(ErrorEmbedInvalidCategory)) {
		t.Fatalf("expected ErrorEmbedInvalidCategory for default category no longer in custom options, got %v", err)
	}

	detail, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "body",
		Category:    "Billing",
		Priority:    "Urgent",
	}, nil)
	if err != nil {
		t.Fatalf("unexpected error creating with custom options: %v", err)
	}
	if detail.Category != "Billing" || detail.Priority != "Urgent" {
		t.Fatalf("expected custom category/priority persisted, got %+v", detail)
	}
}

// ---- 图片附件 ----

// gifSignatureBytes 构造一段带有效 GIF 魔数前缀的字节，http.DetectContentType 只依据前缀判断，
// 不要求完整合法的 GIF 结构，足够驱动 Service 的 content-type 嗅探校验。
func gifSignatureBytes(padding int) []byte {
	data := []byte("GIF87a")
	return append(data, make([]byte, padding)...)
}

func establishedSessionWithConfig(repo *fakeTicketRepository, svc *Service, t *testing.T, maxImages int) string {
	t.Helper()
	seedEmbedConfig(repo, "user1", "account1", "embed-token-x", true, "")
	repo.configsByWorkspace[workspaceKey("user1", "account1")].MaxImagesPerTicket = maxImages
	sessionToken := "session-1"
	svc.sessions.(*fakeSessionStore).sessions[sessionToken] = EmbedSession{
		UserID: "user1", AdminAccountID: "account1", SrcHost: "https://web.example.com", Sub2apiUserID: "42", Sub2apiEmail: "sub2api@example.com",
	}
	return sessionToken
}

func TestCreateTicket_MultipartWithImageSavesAttachment(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 3)

	detail, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "see attached screenshot",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{
		{OriginalName: "screenshot.gif", ContentType: "image/gif", Data: gifSignatureBytes(32)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(detail.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(detail.Messages))
	}
	attachments := detail.Messages[0].Attachments
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment on first message, got %d", len(attachments))
	}
	if attachments[0].ContentType != "image/gif" {
		t.Errorf("expected sniffed content type image/gif, got %q", attachments[0].ContentType)
	}
	if attachments[0].OriginalName != "screenshot.gif" {
		t.Errorf("expected original name preserved, got %q", attachments[0].OriginalName)
	}

	stored := repo.attachments
	if len(stored) != 1 {
		t.Fatalf("expected 1 attachment row persisted, got %d", len(stored))
	}
	for _, a := range stored {
		if a.UserID != "user1" || a.AdminAccountID != "account1" {
			t.Errorf("expected attachment stamped with workspace, got %+v", a)
		}
	}
}

func TestCreateTicket_RejectsImagesWhenUploadsDisabled(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 0)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "see attached",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{
		{OriginalName: "a.gif", ContentType: "image/gif", Data: gifSignatureBytes(16)},
	})
	if !errors.Is(err, requestError(ErrorEmbedTooManyImages)) {
		t.Fatalf("expected ErrorEmbedTooManyImages when uploads are disabled (max=0), got %v", err)
	}
}

func TestCreateTicket_RejectsTooManyImages(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 1)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "see attached",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{
		{OriginalName: "a.gif", ContentType: "image/gif", Data: gifSignatureBytes(16)},
		{OriginalName: "b.gif", ContentType: "image/gif", Data: gifSignatureBytes(16)},
	})
	if !errors.Is(err, requestError(ErrorEmbedTooManyImages)) {
		t.Fatalf("expected ErrorEmbedTooManyImages when upload count exceeds workspace config, got %v", err)
	}
}

func TestCreateTicket_RejectsNonImageContentType(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 3)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "see attached",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{
		{OriginalName: "notes.txt", ContentType: "image/gif", Data: []byte("this is plain text, not an image")},
	})
	if !errors.Is(err, requestError(ErrorEmbedInvalidImageType)) {
		t.Fatalf("expected ErrorEmbedInvalidImageType for sniffed non-image content, got %v", err)
	}
}

func TestCreateTicket_RejectsOversizedImage(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 3)

	oversized := gifSignatureBytes(maxImageSizeBytes + 1)
	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "see attached",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{
		{OriginalName: "big.gif", ContentType: "image/gif", Data: oversized},
	})
	if !errors.Is(err, requestError(ErrorEmbedImageTooLarge)) {
		t.Fatalf("expected ErrorEmbedImageTooLarge, got %v", err)
	}
}

func TestCreateTicket_RejectsEmptyImage(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	sessionToken := establishedSessionWithConfig(repo, svc, t, 3)

	_, err := svc.CreateTicket(context.Background(), sessionToken, CreateTicketRequest{
		ManualEmail: "user@example.com",
		Title:       "help",
		Body:        "see attached",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{
		{OriginalName: "empty.gif", ContentType: "image/gif", Data: []byte{}},
	})
	if !errors.Is(err, requestError(ErrorEmbedEmptyImage)) {
		t.Fatalf("expected ErrorEmbedEmptyImage, got %v", err)
	}
}

// TestReadEmbedAttachment_CannotReadOtherUsersAttachment 校验 embed 用户不能读取同一工作区内
// 其它 Sub2API 用户的附件——即使猜中了附件 ID。
func TestReadEmbedAttachment_CannotReadOtherUsersAttachment(t *testing.T) {
	repo := newFakeTicketRepository()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)

	// user1 (sub2api user 42) 创建带附件的工单。
	ownerSession := establishedSessionWithConfig(repo, svc, t, 3)
	detail, err := svc.CreateTicket(context.Background(), ownerSession, CreateTicketRequest{
		ManualEmail: "owner@example.com",
		Title:       "help",
		Body:        "see attached",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{{OriginalName: "a.gif", ContentType: "image/gif", Data: gifSignatureBytes(16)}})
	if err != nil {
		t.Fatalf("unexpected error creating ticket: %v", err)
	}
	attachmentID := detail.Messages[0].Attachments[0].ID

	// 另一个 sub2api 用户（同一 workspace，不同 sub2api_user_id）的会话尝试读取该附件。
	otherSessionToken := "session-other-user"
	svc.sessions.(*fakeSessionStore).sessions[otherSessionToken] = EmbedSession{
		UserID: "user1", AdminAccountID: "account1", SrcHost: "https://web.example.com", Sub2apiUserID: "999",
	}

	_, _, err = svc.ReadEmbedAttachment(context.Background(), otherSessionToken, attachmentID)
	if !errors.Is(err, requestError(ErrorNotFound)) {
		t.Fatalf("expected ErrorNotFound when reading another sub2api user's attachment, got %v", err)
	}
}

// TestReadAdminAttachment_CannotReadOtherWorkspaceAttachment 校验后台不能跨 workspace 读取附件。
func TestReadAdminAttachment_CannotReadOtherWorkspaceAttachment(t *testing.T) {
	repo := newFakeTicketRepository()
	ownerSvc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	ownerSession := establishedSessionWithConfig(repo, ownerSvc, t, 3)
	detail, err := ownerSvc.CreateTicket(context.Background(), ownerSession, CreateTicketRequest{
		ManualEmail: "owner@example.com",
		Title:       "help",
		Body:        "see attached",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{{OriginalName: "a.gif", ContentType: "image/gif", Data: gifSignatureBytes(16)}})
	if err != nil {
		t.Fatalf("unexpected error creating ticket: %v", err)
	}
	attachmentID := detail.Messages[0].Attachments[0].ID

	// 后台以另一个 workspace（account2）身份尝试读取 account1 的附件。
	adminSvc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account2"})
	_, _, err = adminSvc.ReadAdminAttachment(context.Background(), "user1", attachmentID)
	if !errors.Is(err, requestError(ErrorNotFound)) {
		t.Fatalf("expected ErrorNotFound when reading another workspace's attachment, got %v", err)
	}
}

// TestReadAdminAttachment_OwnerCanRead 校验正确 workspace 能正常读取自己的附件内容。
func TestReadAdminAttachment_OwnerCanRead(t *testing.T) {
	repo := newFakeTicketRepository()
	embedSvc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, nil)
	ownerSession := establishedSessionWithConfig(repo, embedSvc, t, 3)
	imageData := gifSignatureBytes(16)
	detail, err := embedSvc.CreateTicket(context.Background(), ownerSession, CreateTicketRequest{
		ManualEmail: "owner@example.com",
		Title:       "help",
		Body:        "see attached",
		Category:    "通用问题",
		Priority:    "普通",
	}, []AttachmentUpload{{OriginalName: "a.gif", ContentType: "image/gif", Data: imageData}})
	if err != nil {
		t.Fatalf("unexpected error creating ticket: %v", err)
	}
	attachmentID := detail.Messages[0].Attachments[0].ID

	adminSvc := &Service{
		repository: repo,
		sessions:   embedSvc.sessions,
		storage:    embedSvc.storage,
		accounts:   &fakeAccountResolver{id: "account1"},
		newID:      sequentialIDs("id2"),
		newToken:   sequentialIDs("token2"),
		now:        time.Now,
	}
	attachment, data, err := adminSvc.ReadAdminAttachment(context.Background(), "user1", attachmentID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attachment.ID != attachmentID {
		t.Errorf("expected attachment id %q, got %q", attachmentID, attachment.ID)
	}
	if string(data) != string(imageData) {
		t.Errorf("expected attachment bytes to round-trip, got mismatch")
	}
}

// ---- Sub2API 用户资料 ----

func TestGetSub2apiUserProfile_ValidatesTicketOwnership(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = &Ticket{ID: "t1", UserID: "user1", AdminAccountID: "account1", ManualEmail: "a@example.com", Title: "t"}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account2"})

	_, err := svc.GetSub2apiUserProfile(context.Background(), "user1", "t1")
	if !errors.Is(err, requestError(ErrorNotFound)) {
		t.Fatalf("expected ErrorNotFound when ticket belongs to a different workspace, got %v", err)
	}
}

// TestGetSub2apiUserProfile_DegradesWhenExternalDataUnavailable 校验第一版在没有已确认的
// "按任意 Sub2API 用户 ID 只读查询余额/充值/注册时间"接口时，返回可展示的降级响应而不是报错。
func TestGetSub2apiUserProfile_DegradesWhenExternalDataUnavailable(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = &Ticket{
		ID: "t1", UserID: "user1", AdminAccountID: "account1", ManualEmail: "a@example.com", Title: "t",
		Sub2apiUserID: "42", Sub2apiEmail: "sub2api@example.com", Sub2apiRole: "member",
		Sub2apiSrcHost: "https://web.example.com", Sub2apiSrcURL: "https://web.example.com/custom/abc",
	}
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})

	profile, err := svc.GetSub2apiUserProfile(context.Background(), "user1", "t1")
	if err != nil {
		t.Fatalf("expected degraded response instead of error, got %v", err)
	}
	if profile.Sub2apiUserID != "42" || profile.Sub2apiEmail != "sub2api@example.com" || profile.Sub2apiRole != "member" {
		t.Fatalf("expected ticket snapshot fields to be populated, got %+v", profile)
	}
	if profile.BalanceAvailable || profile.TotalRechargedAvailable || profile.RegisteredAtAvailable || profile.RechargeHistoryAvailable {
		t.Fatalf("expected all enrichment fields to be marked unavailable when no admin session provider/client is injected, got %+v", profile)
	}
	if profile.RemoteUnavailableReason != Sub2apiRemoteUnavailableNoAdminSession {
		t.Fatalf("expected remoteUnavailableReason %q, got %q", Sub2apiRemoteUnavailableNoAdminSession, profile.RemoteUnavailableReason)
	}
}

// sub2apiEnrichmentTicket 是余下几个 enrichment 测试共用的工单快照。
func sub2apiEnrichmentTicket() *Ticket {
	return &Ticket{
		ID: "t1", UserID: "user1", AdminAccountID: "account1", ManualEmail: "a@example.com", Title: "t",
		Sub2apiUserID: "42", Sub2apiEmail: "sub2api@example.com", Sub2apiRole: "member",
		Sub2apiSrcHost: "https://web.example.com", Sub2apiSrcURL: "https://web.example.com/custom/abc",
	}
}

// TestGetSub2apiUserProfile_EnrichesFromAdminSession 覆盖文档要求的主成功路径：当前 workspace
// 能通过 fake admin session provider + fake Sub2API admin client 填充 balance/registeredAt/
// totalRecharged/history。
func TestGetSub2apiUserProfile_EnrichesFromAdminSession(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = sub2apiEnrichmentTicket()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})
	svc.SetAdminSessionProvider(&fakeAdminSessionProvider{session: upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://site.example.com", AccessToken: "admin-token"}})

	balance := 88.5
	totalRecharged := 200.0
	registeredAt := time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC)
	frozen := 3.0
	concurrency := 2
	rpmLimit := 60
	svc.SetSub2APIAdminClient(&fakeSub2APIAdminClient{
		user: upstream.Sub2APIAdminUser{
			ID: "42", Username: "alice", Status: "active", Balance: &balance,
			FrozenBalance: &frozen, Concurrency: &concurrency, RPMLimit: &rpmLimit, CreatedAt: &registeredAt,
		},
		history: upstream.Sub2APIUserBalanceHistory{
			TotalRecharged: &totalRecharged,
			Items: []upstream.Sub2APIBalanceHistoryItem{
				{ID: "h1", Type: "balance", Amount: &totalRecharged, Note: "recharge"},
			},
		},
	})

	profile, err := svc.GetSub2apiUserProfile(context.Background(), "user1", "t1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if profile.RemoteUnavailableReason != "" {
		t.Fatalf("expected no remoteUnavailableReason when enrichment succeeds, got %q", profile.RemoteUnavailableReason)
	}
	if !profile.BalanceAvailable || profile.Balance == nil || *profile.Balance != balance {
		t.Fatalf("expected balance %v to be available, got %+v", balance, profile)
	}
	if !profile.RegisteredAtAvailable || profile.RegisteredAt == nil || !profile.RegisteredAt.Equal(registeredAt) {
		t.Fatalf("expected registeredAt %v to be available, got %+v", registeredAt, profile)
	}
	if !profile.TotalRechargedAvailable || profile.TotalRecharged == nil || *profile.TotalRecharged != totalRecharged {
		t.Fatalf("expected totalRecharged %v to be available, got %+v", totalRecharged, profile)
	}
	if !profile.RechargeHistoryAvailable || len(profile.RechargeHistory) != 1 || profile.RechargeHistory[0].ID != "h1" {
		t.Fatalf("expected recharge history to be populated, got %+v", profile.RechargeHistory)
	}
	if profile.Username != "alice" || profile.Status != "active" {
		t.Fatalf("expected username/status to be populated, got %+v", profile)
	}
	if profile.FrozenBalance == nil || *profile.FrozenBalance != frozen {
		t.Fatalf("expected frozenBalance to be populated, got %+v", profile.FrozenBalance)
	}
	if profile.Concurrency == nil || *profile.Concurrency != concurrency || profile.RPMLimit == nil || *profile.RPMLimit != rpmLimit {
		t.Fatalf("expected concurrency/rpmLimit to be populated, got %+v", profile)
	}
}

// TestGetSub2apiUserProfile_CrossWorkspaceNeverCallsRemote 校验工单不属于当前 workspace 时
// 仍然返回 404，且绝不触发任何远程查询（即使 admin session provider/client 都已注入）。
func TestGetSub2apiUserProfile_CrossWorkspaceNeverCallsRemote(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = sub2apiEnrichmentTicket()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account2"})
	sessionCalled := false
	svc.SetAdminSessionProvider(&fakeAdminSessionProviderFunc{fn: func() { sessionCalled = true }})
	svc.SetSub2APIAdminClient(&fakeSub2APIAdminClient{})

	_, err := svc.GetSub2apiUserProfile(context.Background(), "user1", "t1")
	if !errors.Is(err, requestError(ErrorNotFound)) {
		t.Fatalf("expected ErrorNotFound for cross-workspace ticket, got %v", err)
	}
	if sessionCalled {
		t.Fatalf("must not query the admin session for a ticket outside the current workspace")
	}
}

// TestGetSub2apiUserProfile_DegradesWhenAdminSessionUnavailable 校验 admin session provider
// 返回错误（会话缺失/过期/非 admin 等）时降级为快照 + 不可用，不 panic，不整体失败。
func TestGetSub2apiUserProfile_DegradesWhenAdminSessionUnavailable(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = sub2apiEnrichmentTicket()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})
	svc.SetAdminSessionProvider(&fakeAdminSessionProvider{err: errors.New("no sub2api admin session bound")})
	svc.SetSub2APIAdminClient(&fakeSub2APIAdminClient{})

	profile, err := svc.GetSub2apiUserProfile(context.Background(), "user1", "t1")
	if err != nil {
		t.Fatalf("expected degraded response instead of error, got %v", err)
	}
	if profile.Sub2apiUserID != "42" {
		t.Fatalf("expected snapshot fields preserved, got %+v", profile)
	}
	if profile.BalanceAvailable || profile.RegisteredAtAvailable || profile.TotalRechargedAvailable || profile.RechargeHistoryAvailable {
		t.Fatalf("expected all enrichment fields unavailable when admin session is unavailable, got %+v", profile)
	}
	if profile.RemoteUnavailableReason != Sub2apiRemoteUnavailableNoAdminSession {
		t.Fatalf("expected remoteUnavailableReason %q, got %q", Sub2apiRemoteUnavailableNoAdminSession, profile.RemoteUnavailableReason)
	}
}

// TestGetSub2apiUserProfile_DegradesWhenSub2apiUserDetailFails 校验 Sub2API 用户详情查询失败
// （网络错误/非 2xx/远端用户不存在）时降级为快照 + 不可用。
func TestGetSub2apiUserProfile_DegradesWhenSub2apiUserDetailFails(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = sub2apiEnrichmentTicket()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})
	svc.SetAdminSessionProvider(&fakeAdminSessionProvider{session: upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://site.example.com", AccessToken: "admin-token"}})
	svc.SetSub2APIAdminClient(&fakeSub2APIAdminClient{userErr: errors.New("sub2api user not found")})

	profile, err := svc.GetSub2apiUserProfile(context.Background(), "user1", "t1")
	if err != nil {
		t.Fatalf("expected degraded response instead of error, got %v", err)
	}
	if profile.Sub2apiUserID != "42" {
		t.Fatalf("expected snapshot fields preserved, got %+v", profile)
	}
	if profile.BalanceAvailable || profile.RegisteredAtAvailable || profile.TotalRechargedAvailable || profile.RechargeHistoryAvailable {
		t.Fatalf("expected all enrichment fields unavailable when remote user detail fails, got %+v", profile)
	}
	if profile.RemoteUnavailableReason != Sub2apiRemoteUnavailableUserNotFound {
		t.Fatalf("expected remoteUnavailableReason %q, got %q", Sub2apiRemoteUnavailableUserNotFound, profile.RemoteUnavailableReason)
	}
}

// TestGetSub2apiUserProfile_KeepsUserDetailWhenBalanceHistoryFails 校验余额历史查询失败时，
// 已经成功解析的用户详情字段（balance/registeredAt 等）依然保留，只是充值历史相关字段不可用。
func TestGetSub2apiUserProfile_KeepsUserDetailWhenBalanceHistoryFails(t *testing.T) {
	repo := newFakeTicketRepository()
	repo.tickets["t1"] = sub2apiEnrichmentTicket()
	svc := newTestService(repo, newFakeSessionStore(), &fakeSub2API{}, &fakeAccountResolver{id: "account1"})
	svc.SetAdminSessionProvider(&fakeAdminSessionProvider{session: upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: "https://site.example.com", AccessToken: "admin-token"}})

	balance := 10.0
	svc.SetSub2APIAdminClient(&fakeSub2APIAdminClient{
		user:       upstream.Sub2APIAdminUser{ID: "42", Balance: &balance},
		historyErr: errors.New("balance history request failed"),
	})

	profile, err := svc.GetSub2apiUserProfile(context.Background(), "user1", "t1")
	if err != nil {
		t.Fatalf("expected degraded response instead of error, got %v", err)
	}
	if !profile.BalanceAvailable || profile.Balance == nil || *profile.Balance != balance {
		t.Fatalf("expected balance to remain available even though balance history failed, got %+v", profile)
	}
	if profile.TotalRechargedAvailable || profile.RechargeHistoryAvailable {
		t.Fatalf("expected recharge history fields unavailable when balance history query fails, got %+v", profile)
	}
}

// fakeAdminSessionProviderFunc 是 adminSessionProvider 的假实现，调用时触发回调，
// 用于断言"从未被调用"这类场景（例如跨 workspace 校验应在触达远程查询前就已失败返回）。
type fakeAdminSessionProviderFunc struct {
	fn func()
}

func (f *fakeAdminSessionProviderFunc) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	f.fn()
	return upstream.Session{}, errors.New("should not be called")
}
