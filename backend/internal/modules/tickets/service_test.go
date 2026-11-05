package tickets

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- 测试替身 ----

type memoryStore struct {
	mu          sync.Mutex
	tickets     map[string]*Ticket
	messages    map[string][]Message
	failCreates bool
}

func newMemoryStore() *memoryStore {
	return &memoryStore{tickets: map[string]*Ticket{}, messages: map[string][]Message{}}
}

func (m *memoryStore) CreateTicket(_ context.Context, ticket Ticket, message Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failCreates {
		return errors.New("database down")
	}
	copied := ticket
	m.tickets[ticket.ID] = &copied
	m.messages[ticket.ID] = append(m.messages[ticket.ID], message)
	return nil
}

func (m *memoryStore) AddMessage(_ context.Context, ws Workspace, id string, message Message, status string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, ok := m.tickets[id]
	if !ok || ticket.Workspace != ws {
		return false, nil
	}
	ticket.Status, ticket.LastAuthor, ticket.LastMessageAt, ticket.UpdatedAt = status, message.AuthorType, message.CreatedAt, message.CreatedAt
	m.messages[id] = append(m.messages[id], message)
	return true, nil
}

func (m *memoryStore) GetTicket(_ context.Context, ws Workspace, id string) (*Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, ok := m.tickets[id]
	if !ok || ticket.Workspace != ws {
		return nil, nil
	}
	copied := *ticket
	return &copied, nil
}

func (m *memoryStore) ListForUser(_ context.Context, ws Workspace, user SiteUser, limit int) ([]Ticket, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []Ticket
	for _, ticket := range m.tickets {
		if ticket.Workspace == ws && ticket.User.Platform == user.Platform && ticket.User.ID == user.ID {
			result = append(result, *ticket)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastMessageAt.After(result[j].LastMessageAt) })
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

func (m *memoryStore) ListMessages(_ context.Context, id string) ([]Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Message(nil), m.messages[id]...), nil
}

func (m *memoryStore) MarkCustomerSeen(_ context.Context, ws Workspace, id string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ticket, ok := m.tickets[id]; ok && ticket.Workspace == ws {
		ticket.CustomerSeenAt = &at
	}
	return nil
}

func (m *memoryStore) SetStatus(_ context.Context, ws Workspace, id string, status string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ticket, ok := m.tickets[id]
	if !ok || ticket.Workspace != ws {
		return false, nil
	}
	ticket.Status = status
	return true, nil
}

func (m *memoryStore) ListForAdmin(_ context.Context, ws Workspace, status string, page int, pageSize int) ([]Ticket, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []Ticket
	for _, ticket := range m.tickets {
		if ticket.Workspace == ws && (status == "" || ticket.Status == status) {
			all = append(all, *ticket)
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].LastMessageAt.After(all[j].LastMessageAt) })
	start := (page - 1) * pageSize
	if start > len(all) {
		start = len(all)
	}
	end := start + pageSize
	if end > len(all) {
		end = len(all)
	}
	return all[start:end], len(all), nil
}

func (m *memoryStore) CountByStatus(_ context.Context, ws Workspace) (StatusCounts, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var counts StatusCounts
	for _, ticket := range m.tickets {
		if ticket.Workspace != ws {
			continue
		}
		switch ticket.Status {
		case StatusOpen:
			counts.Open++
		case StatusPending:
			counts.Pending++
		case StatusReplied:
			counts.Replied++
		case StatusClosed:
			counts.Closed++
		}
	}
	return counts, nil
}

func (m *memoryStore) GetAttachment(_ context.Context, ws Workspace, id string) (*Attachment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for ticketID, messages := range m.messages {
		if m.tickets[ticketID].Workspace != ws {
			continue
		}
		for _, message := range messages {
			for _, attachment := range message.Attachments {
				if attachment.ID == id {
					copied := attachment
					return &copied, nil
				}
			}
		}
	}
	return nil, nil
}

type memorySessions struct {
	mu       sync.Mutex
	sessions map[string]ClientSession
	next     int
}

func (s *memorySessions) Create(_ context.Context, session ClientSession) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	token := fmt.Sprintf("token-%d", s.next)
	s.sessions[token] = session
	return token, nil
}

func (s *memorySessions) Get(_ context.Context, token string) (*ClientSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[token]
	if !ok {
		return nil, nil
	}
	return &session, nil
}

type fakeVerifier struct {
	user      SiteUser
	err       error
	gotURL    string
	gotKind   string
	callCount int
}

func (v *fakeVerifier) Verify(_ context.Context, platform string, baseURL string, credential Credential) (SiteUser, error) {
	v.callCount++
	v.gotURL, v.gotKind = baseURL, credential.Kind
	if v.err != nil {
		return SiteUser{}, v.err
	}
	user := v.user
	user.Platform = platform
	return user, nil
}

type fakePortals struct {
	portals    map[string]*Portal
	disabled   bool
	categories []string
}

func (p *fakePortals) Lookup(_ context.Context, code string) (*Portal, error) {
	return p.portals[code], nil
}

func (p *fakePortals) Settings(_ context.Context, _ Workspace) (bool, []string, error) {
	return !p.disabled, p.categories, nil
}

type fakeWorkspaces map[string]string

func (f fakeWorkspaces) RequireCurrentID(_ context.Context, userID string) (string, error) {
	if id, ok := f[userID]; ok {
		return id, nil
	}
	return "", errors.New("no current account")
}

type memoryFiles struct {
	mu      sync.Mutex
	files   map[string][]byte
	deleted []string
}

func (f *memoryFiles) Save(contentType string, data []byte) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	name := fmt.Sprintf("file-%d%s", len(f.files)+len(f.deleted), imageExtensions[contentType])
	f.files[name] = data
	return name, nil
}

func (f *memoryFiles) Read(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	data, ok := f.files[name]
	if !ok {
		return nil, errors.New("missing")
	}
	return data, nil
}

func (f *memoryFiles) Thumbnail(name string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.files[name]; !ok {
		return nil, errors.New("missing")
	}
	return []byte("thumbnail of " + name), nil
}

func (f *memoryFiles) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.files, name)
	f.deleted = append(f.deleted, name)
	return nil
}

type recordingNotifier struct {
	events chan string
}

func (n *recordingNotifier) TicketCreated(_ context.Context, ticket Ticket, message Message) {
	n.events <- "created:" + ticket.Title + ":" + message.Body
}

func (n *recordingNotifier) CustomerReplied(_ context.Context, ticket Ticket, message Message) {
	n.events <- "replied:" + ticket.Title + ":" + message.Body
}

// ---- 测试环境 ----

var (
	shop  = Workspace{UserID: "owner-1", AdminAccountID: "acct-1"}
	alice = SiteUser{ID: "42", Username: "alice", Email: "alice@example.com"}
)

type testEnv struct {
	service  *Service
	store    *memoryStore
	verifier *fakeVerifier
	portals  *fakePortals
	files    *memoryFiles
	notifier *recordingNotifier
	clock    time.Time
}

func newTestEnv() *testEnv {
	env := &testEnv{
		store:    newMemoryStore(),
		verifier: &fakeVerifier{user: alice},
		portals: &fakePortals{portals: map[string]*Portal{
			"abcdefghij": {Workspace: shop, Platform: "sub2api", BaseURL: "https://relay.example.com", Enabled: true},
			"turnedoff0": {Workspace: shop, Platform: "sub2api", BaseURL: "https://relay.example.com", Enabled: false},
		}},
		files:    &memoryFiles{files: map[string][]byte{}},
		notifier: &recordingNotifier{events: make(chan string, 10)},
		clock:    time.Date(2026, 11, 5, 9, 0, 0, 0, time.UTC),
	}
	env.service = NewService(env.store, &memorySessions{sessions: map[string]ClientSession{}}, env.verifier, env.portals,
		fakeWorkspaces{"owner-1": "acct-1"}, env.files, env.notifier)
	ids := 0
	env.service.newID = func() string { ids++; return fmt.Sprintf("id-%03d", ids) }
	env.service.now = func() time.Time { env.clock = env.clock.Add(time.Minute); return env.clock }
	return env
}

// login 换取会话并返回会话（模拟 TransitLink 后续请求）。
func (env *testEnv) login(t *testing.T, user SiteUser) *ClientSession {
	t.Helper()
	env.verifier.user = user
	response, err := env.service.CreateSession(context.Background(), "abcdefghij", CreateSessionRequest{
		Credential: Credential{Kind: "bearer", Token: "user-token"}, ClientInfo: "TransitLink 0.2.0 (macOS)",
	})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	session, err := env.service.Session(context.Background(), response.SessionToken)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

func (env *testEnv) waitEvent(t *testing.T) string {
	t.Helper()
	select {
	case event := <-env.notifier.events:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("no notification")
		return ""
	}
}

func pngImage(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func errorKey(err error) string {
	if key, ok := isRequestError(err); ok {
		return string(key)
	}
	if err == nil {
		return ""
	}
	return "other: " + err.Error()
}

// ---- 测试 ----

func TestCreateSessionVerifiesTheUserWithTheConfiguredSite(t *testing.T) {
	env := newTestEnv()
	ctx := context.Background()
	request := CreateSessionRequest{Credential: Credential{Kind: "bearer", Token: "user-token"}, ClientInfo: strings.Repeat("x", 300)}

	for code, want := range map[string]string{"nope000000": ErrorDisabled, "turnedoff0": ErrorDisabled} {
		if _, err := env.service.CreateSession(ctx, code, request); errorKey(err) != want {
			t.Fatalf("%s: want %s, got %v", code, want, err)
		}
	}
	response, err := env.service.CreateSession(ctx, "abcdefghij", request)
	if err != nil {
		t.Fatal(err)
	}
	if env.verifier.gotURL != "https://relay.example.com" || env.verifier.gotKind != "bearer" {
		t.Fatalf("must verify against the configured site: %q %q", env.verifier.gotURL, env.verifier.gotKind)
	}
	if response.SessionToken == "" || response.User.ID != "42" || response.User.Name != "alice" ||
		strings.Join(response.Categories, ",") != strings.Join(DefaultCategories, ",") || !response.Limits.Enabled {
		t.Fatalf("unexpected response: %+v", response)
	}
	session, _ := env.service.Session(ctx, response.SessionToken)
	if session.User.Platform != "sub2api" || len([]rune(session.ClientInfo)) != maxClientInfoRunes || session.Workspace != shop {
		t.Fatalf("unexpected session: %+v", session)
	}
	if _, err := env.service.Session(ctx, "forged"); errorKey(err) != ErrorSessionExpired {
		t.Fatalf("unknown tokens are rejected, got %v", err)
	}

	env.verifier.err = requestError(ErrorSiteLoginExpired)
	if _, err := env.service.CreateSession(ctx, "abcdefghij", request); errorKey(err) != ErrorSiteLoginExpired {
		t.Fatalf("site rejection must reach the client, got %v", err)
	}
}

func TestCreateSessionIsRateLimitedPerLink(t *testing.T) {
	env := newTestEnv()
	request := CreateSessionRequest{Credential: Credential{Kind: "bearer", Token: "t"}}
	for i := 0; i < 30; i++ {
		if _, err := env.service.CreateSession(context.Background(), "abcdefghij", request); err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
	}
	if _, err := env.service.CreateSession(context.Background(), "abcdefghij", request); errorKey(err) != ErrorTooManyRequests {
		t.Fatalf("expected the limit, got %v", err)
	}
	if env.verifier.callCount != 30 {
		t.Fatalf("the site must not be asked once the limit is hit, calls %d", env.verifier.callCount)
	}
}

func TestUsersOnlySeeTheirOwnTickets(t *testing.T) {
	env := newTestEnv()
	ctx := context.Background()
	mine := env.login(t, alice)
	created, err := env.service.CreateMine(ctx, mine, CreateTicketRequest{Title: " 充值没到账 ", Body: "昨天充了 100 元", Category: "余额/计费", Contact: "alice@qq.com"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if created.Title != "充值没到账" || created.Status != StatusOpen || created.Category != "余额/计费" || len(created.Messages) != 1 || created.Unread {
		t.Fatalf("unexpected ticket: %+v", created)
	}
	if event := env.waitEvent(t); event != "created:充值没到账:昨天充了 100 元" {
		t.Fatalf("unexpected notification: %s", event)
	}

	bob := env.login(t, SiteUser{ID: "7", Username: "bob"})
	if list, _ := env.service.ListMine(ctx, bob); len(list.Items) != 0 {
		t.Fatalf("bob must not see alice's tickets: %+v", list)
	}
	if _, err := env.service.DetailMine(ctx, bob, created.ID); errorKey(err) != ErrorNotFound {
		t.Fatalf("bob must not open alice's ticket, got %v", err)
	}
	// 另一个平台上同样 ID 的用户也看不到。
	otherPlatform := *mine
	otherPlatform.User.Platform = "newapi"
	if _, err := env.service.DetailMine(ctx, &otherPlatform, created.ID); errorKey(err) != ErrorNotFound {
		t.Fatalf("same id on another platform must not match, got %v", err)
	}
	// 另一个工作区的站长也看不到。
	if _, err := env.service.AdminDetail(ctx, "someone-else", created.ID); errorKey(err) != ErrorNoCurrentAccount {
		t.Fatalf("other owners have no access, got %v", err)
	}
	if list, _ := env.service.ListMine(ctx, mine); len(list.Items) != 1 {
		t.Fatalf("alice sees her ticket: %+v", list)
	}
}

func TestTicketValidation(t *testing.T) {
	env := newTestEnv()
	ctx := context.Background()
	session := env.login(t, alice)
	cases := map[string]CreateTicketRequest{
		"no title":         {Body: "x"},
		"no body":          {Title: "x"},
		"long title":       {Title: strings.Repeat("长", maxTitleRunes+1), Body: "x"},
		"long body":        {Title: "x", Body: strings.Repeat("长", maxBodyRunes+1)},
		"unknown category": {Title: "x", Body: "x", Category: "随便"},
	}
	for name, req := range cases {
		if _, err := env.service.CreateMine(ctx, session, req, nil); errorKey(err) != ErrorInvalid {
			t.Errorf("%s: want invalid, got %v", name, err)
		}
	}
	// 站长自定义的分类；不填分类时用第一个。
	env.portals.categories = []string{"售后", "合作"}
	created, err := env.service.CreateMine(ctx, session, CreateTicketRequest{Title: "x", Body: "y"}, nil)
	if err != nil || created.Category != "售后" {
		t.Fatalf("default category: %+v %v", created, err)
	}
	env.waitEvent(t)
	// 站长关掉工单后不能再提交和回复（但还能看）。
	env.portals.disabled = true
	if _, err := env.service.CreateMine(ctx, session, CreateTicketRequest{Title: "x", Body: "y"}, nil); errorKey(err) != ErrorDisabled {
		t.Fatalf("disabled: %v", err)
	}
	if _, err := env.service.ReplyMine(ctx, session, created.ID, ReplyRequest{Body: "y"}, nil); errorKey(err) != ErrorDisabled {
		t.Fatalf("disabled reply: %v", err)
	}
	if _, err := env.service.DetailMine(ctx, session, created.ID); err != nil {
		t.Fatalf("reading stays possible: %v", err)
	}
}

func TestStatusFlowAndUnreadReplies(t *testing.T) {
	env := newTestEnv()
	ctx := context.Background()
	session := env.login(t, alice)
	ticket, _ := env.service.CreateMine(ctx, session, CreateTicketRequest{Title: "接口报错", Body: "返回 500"}, nil)
	env.waitEvent(t)

	adminDetail, err := env.service.AdminReply(ctx, "owner-1", ticket.ID, ReplyRequest{Body: "已修复，请重试"})
	if err != nil || adminDetail.Status != StatusReplied || adminDetail.LastAuthor != AuthorAdmin || len(adminDetail.Messages) != 2 {
		t.Fatalf("admin reply: %+v %v", adminDetail, err)
	}
	list, _ := env.service.ListMine(ctx, session)
	if list.Unread != 1 || !list.Items[0].Unread {
		t.Fatalf("the reply must show as unread: %+v", list)
	}
	if detail, _ := env.service.DetailMine(ctx, session, ticket.ID); detail.Unread {
		t.Fatalf("opening the ticket marks it read")
	}
	if list, _ := env.service.ListMine(ctx, session); list.Unread != 0 {
		t.Fatalf("no unread after viewing: %+v", list)
	}

	reply, err := env.service.ReplyMine(ctx, session, ticket.ID, ReplyRequest{Body: "还是不行"}, nil)
	if err != nil || reply.Status != StatusPending || len(reply.Messages) != 3 {
		t.Fatalf("customer follow-up: %+v %v", reply, err)
	}
	if event := env.waitEvent(t); event != "replied:接口报错:还是不行" {
		t.Fatalf("unexpected notification: %s", event)
	}
	if _, err := env.service.ReplyMine(ctx, session, ticket.ID, ReplyRequest{Body: "  "}, nil); errorKey(err) != ErrorInvalid {
		t.Fatalf("empty replies are rejected, got %v", err)
	}

	closed, err := env.service.CloseMine(ctx, session, ticket.ID)
	if err != nil || closed.Status != StatusClosed {
		t.Fatalf("close: %+v %v", closed, err)
	}
	if _, err := env.service.ReplyMine(ctx, session, ticket.ID, ReplyRequest{Body: "再问一下"}, nil); errorKey(err) != ErrorClosed {
		t.Fatalf("closed tickets refuse replies, got %v", err)
	}
	// 站长回复已关闭的工单会重新打开（已回复）。
	if reopened, err := env.service.AdminReply(ctx, "owner-1", ticket.ID, ReplyRequest{Body: "补充一句"}); err != nil || reopened.Status != StatusReplied {
		t.Fatalf("admin reply reopens: %+v %v", reopened, err)
	}
	if _, err := env.service.AdminSetStatus(ctx, "owner-1", ticket.ID, "archived"); errorKey(err) != ErrorInvalid {
		t.Fatalf("unknown status: %v", err)
	}
	if detail, err := env.service.AdminSetStatus(ctx, "owner-1", ticket.ID, StatusClosed); err != nil || detail.Status != StatusClosed {
		t.Fatalf("set status: %+v %v", detail, err)
	}
	select {
	case event := <-env.notifier.events:
		t.Fatalf("admin actions must not notify: %s", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestImageAttachments(t *testing.T) {
	env := newTestEnv()
	ctx := context.Background()
	session := env.login(t, alice)
	picture := pngImage(t)

	created, err := env.service.CreateMine(ctx, session, CreateTicketRequest{Title: "截图", Body: "见图"},
		[]AttachmentUpload{{OriginalName: "../../screen.png", ContentType: "text/html", Data: picture}})
	if err != nil {
		t.Fatal(err)
	}
	env.waitEvent(t)
	attachments := created.Messages[0].Attachments
	if len(attachments) != 1 || attachments[0].ContentType != "image/png" || attachments[0].OriginalName != "screen.png" {
		t.Fatalf("attachment metadata: %+v", attachments)
	}
	got, data, err := env.service.AttachmentMine(ctx, session, attachments[0].ID, false)
	if err != nil || !bytes.Equal(data, picture) || got.ContentType != "image/png" {
		t.Fatalf("download: %v", err)
	}
	// 缩略图是 JPEG；附件本身的类型不变。
	thumb, thumbData, err := env.service.AttachmentMine(ctx, session, attachments[0].ID, true)
	if err != nil || thumb.ContentType != "image/jpeg" || !strings.HasPrefix(string(thumbData), "thumbnail of ") || attachments[0].ContentType != "image/png" {
		t.Fatalf("thumbnail: %+v %q %v", thumb, thumbData, err)
	}
	if _, _, err := env.service.AdminAttachment(ctx, "owner-1", attachments[0].ID, false); err != nil {
		t.Fatalf("admin download: %v", err)
	}
	if got, _, err := env.service.AdminAttachment(ctx, "owner-1", attachments[0].ID, true); err != nil || got.ContentType != "image/jpeg" {
		t.Fatalf("admin thumbnail: %+v %v", got, err)
	}
	bob := env.login(t, SiteUser{ID: "7", Username: "bob"})
	if _, _, err := env.service.AttachmentMine(ctx, bob, attachments[0].ID, true); errorKey(err) != ErrorNotFound {
		t.Fatalf("other users cannot download, got %v", err)
	}

	bad := map[string][]AttachmentUpload{
		"not an image":    {{OriginalName: "a.png", Data: []byte("<html>")}},
		"empty":           {{OriginalName: "a.png"}},
		"too large":       {{OriginalName: "a.png", Data: append(picture, make([]byte, maxImageBytes)...)}},
		"too many images": make([]AttachmentUpload, maxImagesPerMessage+1),
	}
	want := map[string]string{"not an image": ErrorInvalidImage, "empty": ErrorInvalidImage, "too large": ErrorImageTooLarge, "too many images": ErrorTooManyImages}
	for name, uploads := range bad {
		for i := range uploads {
			if uploads[i].Data == nil && name == "too many images" {
				uploads[i].Data = picture
			}
		}
		if _, err := env.service.CreateMine(ctx, session, CreateTicketRequest{Title: "x", Body: "y"}, uploads); errorKey(err) != want[name] {
			t.Errorf("%s: want %s, got %v", name, want[name], err)
		}
	}

	// 数据库写入失败：已经保存的图片要删掉。
	env.store.failCreates = true
	before := len(env.files.files)
	if _, err := env.service.CreateMine(ctx, session, CreateTicketRequest{Title: "x", Body: "y"},
		[]AttachmentUpload{{OriginalName: "a.png", Data: picture}}); err == nil {
		t.Fatal("expected the store error")
	}
	if len(env.files.files) != before || len(env.files.deleted) == 0 {
		t.Fatalf("orphaned files must be removed: %d files, deleted %v", len(env.files.files), env.files.deleted)
	}

	// 没有可写的存储目录：不能上传图片，但文字工单照常。
	env.store.failCreates = false
	env.service.files = nil
	if _, err := env.service.CreateMine(ctx, session, CreateTicketRequest{Title: "x", Body: "y"},
		[]AttachmentUpload{{OriginalName: "a.png", Data: picture}}); errorKey(err) != ErrorUploadsDisabled {
		t.Fatalf("uploads disabled: %v", err)
	}
}

func TestAdminListCountsAndPaging(t *testing.T) {
	env := newTestEnv()
	ctx := context.Background()
	session := env.login(t, alice)
	for i := 0; i < 3; i++ {
		if _, err := env.service.CreateMine(ctx, session, CreateTicketRequest{Title: fmt.Sprintf("问题 %d", i), Body: "y"}, nil); err != nil {
			t.Fatal(err)
		}
		env.waitEvent(t)
	}
	first, _ := env.service.ListMine(ctx, session)
	if _, err := env.service.AdminReply(ctx, "owner-1", first.Items[0].ID, ReplyRequest{Body: "好的"}); err != nil {
		t.Fatal(err)
	}

	page, err := env.service.AdminList(ctx, "owner-1", "", 1, 2)
	if err != nil || page.Total != 3 || len(page.Items) != 2 || page.TotalPages != 2 || page.Counts.Open != 2 || page.Counts.Replied != 1 {
		t.Fatalf("page 1: %+v %v", page, err)
	}
	if page.Items[0].SiteUserID != "42" || page.Items[0].ClientInfo != "TransitLink 0.2.0 (macOS)" || page.Items[0].Platform != "sub2api" {
		t.Fatalf("admin view carries the user: %+v", page.Items[0])
	}
	open, _ := env.service.AdminList(ctx, "owner-1", StatusOpen, 1, 20)
	if open.Total != 2 {
		t.Fatalf("status filter: %+v", open)
	}
	if _, err := env.service.AdminList(ctx, "owner-1", "nonsense", 1, 20); errorKey(err) != ErrorInvalid {
		t.Fatalf("bad filter: %v", err)
	}
	if capped, _ := env.service.AdminList(ctx, "owner-1", "", 0, 1000); capped.PageSize != maxAdminPageSize || capped.Page != 1 {
		t.Fatalf("paging defaults: %+v", capped)
	}
	if counts, err := env.service.AdminCounts(ctx, "owner-1"); err != nil || counts.Open != 2 {
		t.Fatalf("counts: %+v %v", counts, err)
	}
}
