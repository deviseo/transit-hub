package tickets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// Store 是 Repository 的接口（测试用内存实现替换）。
type Store interface {
	CreateTicket(ctx context.Context, ticket Ticket, message Message) error
	AddMessage(ctx context.Context, ws Workspace, ticketID string, message Message, status string) (bool, error)
	GetTicket(ctx context.Context, ws Workspace, id string) (*Ticket, error)
	ListForUser(ctx context.Context, ws Workspace, user SiteUser, limit int) ([]Ticket, error)
	ListMessages(ctx context.Context, ticketID string) ([]Message, error)
	MarkCustomerSeen(ctx context.Context, ws Workspace, id string, at time.Time) error
	SetStatus(ctx context.Context, ws Workspace, id string, status string) (bool, error)
	ListForAdmin(ctx context.Context, ws Workspace, status string, page int, pageSize int) ([]Ticket, int, error)
	CountByStatus(ctx context.Context, ws Workspace) (StatusCounts, error)
	GetAttachment(ctx context.Context, ws Workspace, id string) (*Attachment, error)
}

// Sessions 保存工单会话（Redis 实现见 SessionStore）。
type Sessions interface {
	Create(ctx context.Context, session ClientSession) (string, error)
	Get(ctx context.Context, token string) (*ClientSession, error)
}

// Verifier 去站点核实用户身份（实现见 SiteVerifier）。
type Verifier interface {
	Verify(ctx context.Context, platform string, baseURL string, credential Credential) (SiteUser, error)
}

// Portals 由「客户端」配置提供：专属链接对应的工作区和站点，以及工作区的工单设置。
type Portals interface {
	// Lookup 按专属链接代码查工单入口；链接不存在或没开启时返回 nil。
	Lookup(ctx context.Context, code string) (*Portal, error)
	// Settings 返回工作区现在是否开着工单和问题分类（站长可能在用户拿到会话后关掉）。
	Settings(ctx context.Context, ws Workspace) (enabled bool, categories []string, err error)
}

// Workspaces 解析站长当前选中的工作区。
type Workspaces interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

// Files 保存图片附件（实现见 AttachmentStorage）。
type Files interface {
	Save(contentType string, data []byte) (string, error)
	Read(name string) ([]byte, error)
	// Thumbnail 返回缩略图（JPEG）；做不了时返回错误，改用原图。
	Thumbnail(name string) ([]byte, error)
	Delete(name string) error
}

// Notifier 把新工单和用户追问推送给站长（工作区的通知机器人）。
type Notifier interface {
	TicketCreated(ctx context.Context, ticket Ticket, message Message)
	CustomerReplied(ctx context.Context, ticket Ticket, message Message)
}

type Service struct {
	store      Store
	sessions   Sessions
	verifier   Verifier
	portals    Portals
	workspaces Workspaces
	files      Files
	notifier   Notifier
	limiter    *rateLimiter
	now        func() time.Time
	newID      func() string
}

// NewService 创建工单服务。files 为 nil 时不能上传图片；notifier 为 nil 时不推送。
func NewService(store Store, sessions Sessions, verifier Verifier, portals Portals, workspaces Workspaces, files Files, notifier Notifier) *Service {
	return &Service{
		store: store, sessions: sessions, verifier: verifier, portals: portals, workspaces: workspaces, files: files, notifier: notifier,
		// 换取会话会请求站点：每个专属链接每分钟最多 30 次，免得被拿来频繁请求站点。
		limiter: newRateLimiter(30, time.Minute),
		now:     time.Now,
		newID:   randomID,
	}
}

func randomID() string {
	buffer := make([]byte, 16)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}

// ---- 用户（TransitLink） ----

// CreateSession 核实用户在站点上的身份，换一个工单会话。
func (s *Service) CreateSession(ctx context.Context, code string, req CreateSessionRequest) (SessionResponse, error) {
	code = strings.TrimSpace(code)
	if code == "" || !s.limiter.Allow(code) {
		return SessionResponse{}, requestError(ErrorTooManyRequests)
	}
	portal, err := s.portals.Lookup(ctx, code)
	if err != nil {
		return SessionResponse{}, err
	}
	if portal == nil || !portal.Enabled {
		return SessionResponse{}, requestError(ErrorDisabled)
	}
	user, err := s.verifier.Verify(ctx, portal.Platform, portal.BaseURL, req.Credential)
	if err != nil {
		return SessionResponse{}, err
	}
	session := ClientSession{
		Workspace:  portal.Workspace,
		User:       user,
		ClientInfo: clip(req.ClientInfo, maxClientInfoRunes),
		ExpiresAt:  s.now().Add(sessionTTL),
	}
	token, err := s.sessions.Create(ctx, session)
	if err != nil {
		return SessionResponse{}, err
	}
	return SessionResponse{
		SessionToken: token,
		ExpiresAt:    session.ExpiresAt,
		User:         SessionUser{ID: user.ID, Name: user.Username, Email: user.Email},
		Categories:   categoriesOrDefault(portal.Categories),
		Limits:       UploadLimits{Enabled: s.files != nil, MaxImages: maxImagesPerMessage, MaxImageBytes: maxImageBytes},
	}, nil
}

// Session 按令牌读取会话；没有或过期时返回 ErrorSessionExpired（客户端重新换取）。
func (s *Service) Session(ctx context.Context, token string) (*ClientSession, error) {
	session, err := s.sessions.Get(ctx, token)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, requestError(ErrorSessionExpired)
	}
	return session, nil
}

// ListMine 列出用户自己的工单，和有新回复的工单数。
func (s *Service) ListMine(ctx context.Context, session *ClientSession) (ClientListResponse, error) {
	tickets, err := s.store.ListForUser(ctx, session.Workspace, session.User, clientListLimit)
	if err != nil {
		return ClientListResponse{}, err
	}
	response := ClientListResponse{Items: make([]ClientTicketView, 0, len(tickets))}
	for _, ticket := range tickets {
		view := clientView(ticket)
		if view.Unread {
			response.Unread++
		}
		response.Items = append(response.Items, view)
	}
	return response, nil
}

// CreateMine 新建工单。
func (s *Service) CreateMine(ctx context.Context, session *ClientSession, req CreateTicketRequest, uploads []AttachmentUpload) (ClientTicketDetail, error) {
	categories, err := s.enabledCategories(ctx, session.Workspace)
	if err != nil {
		return ClientTicketDetail{}, err
	}
	title, titleOK := limited(req.Title, maxTitleRunes)
	body, bodyOK := limited(req.Body, maxBodyRunes)
	contact, contactOK := limited(req.Contact, maxContactRunes)
	category := strings.TrimSpace(req.Category)
	if category == "" {
		category = categories[0]
	}
	if !titleOK || !bodyOK || !contactOK || title == "" || body == "" || !contains(categories, category) {
		return ClientTicketDetail{}, requestError(ErrorInvalid)
	}
	now := s.now()
	ticket := Ticket{
		ID: s.newID(), Workspace: session.Workspace, User: session.User, Contact: contact, ClientInfo: session.ClientInfo,
		Title: title, Category: category, Status: StatusOpen, LastAuthor: AuthorCustomer, LastMessageAt: now, CreatedAt: now, UpdatedAt: now,
	}
	message, err := s.newMessage(ticket.ID, AuthorCustomer, session.User.Username, body, uploads, now)
	if err != nil {
		return ClientTicketDetail{}, err
	}
	if err := s.store.CreateTicket(ctx, ticket, message); err != nil {
		s.discard(message)
		return ClientTicketDetail{}, err
	}
	s.notify(func(ctx context.Context) { s.notifier.TicketCreated(ctx, ticket, message) })
	return s.DetailMine(ctx, session, ticket.ID)
}

// DetailMine 返回用户自己的一个工单（含消息），并记下已看过。
func (s *Service) DetailMine(ctx context.Context, session *ClientSession, id string) (ClientTicketDetail, error) {
	ticket, err := s.ownTicket(ctx, session, id)
	if err != nil {
		return ClientTicketDetail{}, err
	}
	messages, err := s.store.ListMessages(ctx, ticket.ID)
	if err != nil {
		return ClientTicketDetail{}, err
	}
	now := s.now()
	if err := s.store.MarkCustomerSeen(ctx, session.Workspace, ticket.ID, now); err != nil {
		return ClientTicketDetail{}, err
	}
	ticket.CustomerSeenAt = &now
	return ClientTicketDetail{ClientTicketView: clientView(*ticket), Messages: messageViews(messages)}, nil
}

// ReplyMine 用户追问。已关闭的工单不能再回复。
func (s *Service) ReplyMine(ctx context.Context, session *ClientSession, id string, req ReplyRequest, uploads []AttachmentUpload) (ClientTicketDetail, error) {
	if _, err := s.enabledCategories(ctx, session.Workspace); err != nil {
		return ClientTicketDetail{}, err
	}
	ticket, err := s.ownTicket(ctx, session, id)
	if err != nil {
		return ClientTicketDetail{}, err
	}
	if ticket.Status == StatusClosed {
		return ClientTicketDetail{}, requestError(ErrorClosed)
	}
	body, ok := limited(req.Body, maxBodyRunes)
	if !ok || (body == "" && len(uploads) == 0) {
		return ClientTicketDetail{}, requestError(ErrorInvalid)
	}
	message, err := s.newMessage(ticket.ID, AuthorCustomer, session.User.Username, body, uploads, s.now())
	if err != nil {
		return ClientTicketDetail{}, err
	}
	// 站长回复过以后用户再追问：重新变成「待处理」。
	status := ticket.Status
	if status == StatusReplied {
		status = StatusPending
	}
	if found, err := s.store.AddMessage(ctx, session.Workspace, ticket.ID, message, status); err != nil || !found {
		s.discard(message)
		if err == nil {
			err = requestError(ErrorNotFound)
		}
		return ClientTicketDetail{}, err
	}
	ticket.Status = status
	s.notify(func(ctx context.Context) { s.notifier.CustomerReplied(ctx, *ticket, message) })
	return s.DetailMine(ctx, session, ticket.ID)
}

// CloseMine 用户关闭自己的工单（问题解决了）。
func (s *Service) CloseMine(ctx context.Context, session *ClientSession, id string) (ClientTicketDetail, error) {
	ticket, err := s.ownTicket(ctx, session, id)
	if err != nil {
		return ClientTicketDetail{}, err
	}
	if _, err := s.store.SetStatus(ctx, session.Workspace, ticket.ID, StatusClosed); err != nil {
		return ClientTicketDetail{}, err
	}
	return s.DetailMine(ctx, session, ticket.ID)
}

// AttachmentMine 返回用户自己工单里的一张图片。
func (s *Service) AttachmentMine(ctx context.Context, session *ClientSession, attachmentID string, thumb bool) (*Attachment, []byte, error) {
	attachment, err := s.store.GetAttachment(ctx, session.Workspace, attachmentID)
	if err != nil {
		return nil, nil, err
	}
	if attachment == nil {
		return nil, nil, requestError(ErrorNotFound)
	}
	if _, err := s.ownTicket(ctx, session, attachment.TicketID); err != nil {
		return nil, nil, err
	}
	return s.readAttachment(attachment, thumb)
}

func (s *Service) ownTicket(ctx context.Context, session *ClientSession, id string) (*Ticket, error) {
	ticket, err := s.store.GetTicket(ctx, session.Workspace, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if ticket == nil || ticket.User.Platform != session.User.Platform || ticket.User.ID != session.User.ID {
		return nil, requestError(ErrorNotFound)
	}
	return ticket, nil
}

func (s *Service) enabledCategories(ctx context.Context, ws Workspace) ([]string, error) {
	enabled, categories, err := s.portals.Settings(ctx, ws)
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, requestError(ErrorDisabled)
	}
	return categoriesOrDefault(categories), nil
}

// ---- 站长（TransitHub） ----

func (s *Service) adminWorkspace(ctx context.Context, userID string) (Workspace, error) {
	accountID, err := s.workspaces.RequireCurrentID(ctx, userID)
	if err != nil || accountID == "" {
		return Workspace{}, requestError(ErrorNoCurrentAccount)
	}
	return Workspace{UserID: userID, AdminAccountID: accountID}, nil
}

// AdminList 分页列出当前工作区的工单和各状态数量。
func (s *Service) AdminList(ctx context.Context, userID string, status string, page int, pageSize int) (AdminListResponse, error) {
	ws, err := s.adminWorkspace(ctx, userID)
	if err != nil {
		return AdminListResponse{}, err
	}
	if status != "" && !validStatus(status) {
		return AdminListResponse{}, requestError(ErrorInvalid)
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultAdminPageSize
	}
	if pageSize > maxAdminPageSize {
		pageSize = maxAdminPageSize
	}
	tickets, total, err := s.store.ListForAdmin(ctx, ws, status, page, pageSize)
	if err != nil {
		return AdminListResponse{}, err
	}
	counts, err := s.store.CountByStatus(ctx, ws)
	if err != nil {
		return AdminListResponse{}, err
	}
	response := AdminListResponse{Items: make([]AdminTicketView, 0, len(tickets)), Total: total, Page: page, PageSize: pageSize, Counts: counts}
	response.TotalPages = (total + pageSize - 1) / pageSize
	for _, ticket := range tickets {
		response.Items = append(response.Items, adminView(ticket))
	}
	return response, nil
}

// AdminCounts 返回当前工作区各状态的工单数（菜单上的提醒用）。
func (s *Service) AdminCounts(ctx context.Context, userID string) (StatusCounts, error) {
	ws, err := s.adminWorkspace(ctx, userID)
	if err != nil {
		return StatusCounts{}, err
	}
	return s.store.CountByStatus(ctx, ws)
}

// AdminDetail 返回一个工单和全部消息。
func (s *Service) AdminDetail(ctx context.Context, userID string, id string) (AdminTicketDetail, error) {
	ws, err := s.adminWorkspace(ctx, userID)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	return s.adminDetail(ctx, ws, id)
}

func (s *Service) adminDetail(ctx context.Context, ws Workspace, id string) (AdminTicketDetail, error) {
	ticket, err := s.store.GetTicket(ctx, ws, strings.TrimSpace(id))
	if err != nil {
		return AdminTicketDetail{}, err
	}
	if ticket == nil {
		return AdminTicketDetail{}, requestError(ErrorNotFound)
	}
	messages, err := s.store.ListMessages(ctx, ticket.ID)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	return AdminTicketDetail{AdminTicketView: adminView(*ticket), Messages: messageViews(messages)}, nil
}

// AdminReply 站长回复：工单变成「已回复」（已关闭的工单回复后也重新打开）。
func (s *Service) AdminReply(ctx context.Context, userID string, id string, req ReplyRequest) (AdminTicketDetail, error) {
	ws, err := s.adminWorkspace(ctx, userID)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	body, ok := limited(req.Body, maxBodyRunes)
	if !ok || body == "" {
		return AdminTicketDetail{}, requestError(ErrorInvalid)
	}
	message, err := s.newMessage(strings.TrimSpace(id), AuthorAdmin, "", body, nil, s.now())
	if err != nil {
		return AdminTicketDetail{}, err
	}
	found, err := s.store.AddMessage(ctx, ws, message.TicketID, message, StatusReplied)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	if !found {
		return AdminTicketDetail{}, requestError(ErrorNotFound)
	}
	return s.adminDetail(ctx, ws, message.TicketID)
}

// AdminSetStatus 站长修改工单状态。
func (s *Service) AdminSetStatus(ctx context.Context, userID string, id string, status string) (AdminTicketDetail, error) {
	ws, err := s.adminWorkspace(ctx, userID)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	if !validStatus(status) {
		return AdminTicketDetail{}, requestError(ErrorInvalid)
	}
	found, err := s.store.SetStatus(ctx, ws, strings.TrimSpace(id), status)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	if !found {
		return AdminTicketDetail{}, requestError(ErrorNotFound)
	}
	return s.adminDetail(ctx, ws, id)
}

// AdminAttachment 返回当前工作区工单里的一张图片。
func (s *Service) AdminAttachment(ctx context.Context, userID string, attachmentID string, thumb bool) (*Attachment, []byte, error) {
	ws, err := s.adminWorkspace(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	attachment, err := s.store.GetAttachment(ctx, ws, attachmentID)
	if err != nil {
		return nil, nil, err
	}
	if attachment == nil {
		return nil, nil, requestError(ErrorNotFound)
	}
	return s.readAttachment(attachment, thumb)
}

// ---- 公共 ----

// newMessage 检查并保存图片，组成一条消息（保存失败时删掉已保存的图片）。
func (s *Service) newMessage(ticketID string, author string, authorName string, body string, uploads []AttachmentUpload, at time.Time) (Message, error) {
	message := Message{ID: s.newID(), TicketID: ticketID, AuthorType: author, AuthorName: authorName, Body: body, CreatedAt: at, Attachments: []Attachment{}}
	if len(uploads) == 0 {
		return message, nil
	}
	if s.files == nil {
		return Message{}, requestError(ErrorUploadsDisabled)
	}
	validated, err := validateUploads(uploads)
	if err != nil {
		return Message{}, err
	}
	for _, upload := range validated {
		name, err := s.files.Save(upload.ContentType, upload.Data)
		if err != nil {
			s.discard(message)
			return Message{}, err
		}
		message.Attachments = append(message.Attachments, Attachment{
			ID: s.newID(), TicketID: ticketID, MessageID: message.ID, AuthorType: author, OriginalName: upload.OriginalName,
			ContentType: upload.ContentType, SizeBytes: int64(len(upload.Data)), StoragePath: name, CreatedAt: at,
		})
	}
	return message, nil
}

// discard 删除一条没能保存的消息已经写到磁盘的图片。
func (s *Service) discard(message Message) {
	if s.files == nil {
		return
	}
	for _, attachment := range message.Attachments {
		if err := s.files.Delete(attachment.StoragePath); err != nil {
			log.Printf("[tickets] remove unsaved attachment: %v", err)
		}
	}
}

// readAttachment 读取图片。thumb 时给缩略图（JPEG），做不了缩略图就给原图。
func (s *Service) readAttachment(attachment *Attachment, thumb bool) (*Attachment, []byte, error) {
	if s.files == nil {
		return nil, nil, requestError(ErrorNotFound)
	}
	if thumb {
		if data, err := s.files.Thumbnail(attachment.StoragePath); err == nil {
			small := *attachment
			small.ContentType = "image/jpeg"
			return &small, data, nil
		}
	}
	data, err := s.files.Read(attachment.StoragePath)
	if err != nil {
		return nil, nil, requestError(ErrorNotFound)
	}
	return attachment, data, nil
}

// notify 在后台推送通知（不影响用户的请求）。
func (s *Service) notify(send func(ctx context.Context)) {
	if s.notifier == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		send(ctx)
	}()
}

func clientView(ticket Ticket) ClientTicketView {
	unread := ticket.LastAuthor == AuthorAdmin && (ticket.CustomerSeenAt == nil || ticket.LastMessageAt.After(*ticket.CustomerSeenAt))
	return ClientTicketView{
		ID: ticket.ID, Title: ticket.Title, Category: ticket.Category, Status: ticket.Status, Contact: ticket.Contact,
		Unread: unread, LastMessageAt: ticket.LastMessageAt, CreatedAt: ticket.CreatedAt,
	}
}

func adminView(ticket Ticket) AdminTicketView {
	return AdminTicketView{
		ID: ticket.ID, Title: ticket.Title, Category: ticket.Category, Status: ticket.Status, Contact: ticket.Contact,
		Platform: ticket.User.Platform, SiteUserID: ticket.User.ID, SiteUsername: ticket.User.Username, SiteEmail: ticket.User.Email,
		ClientInfo: ticket.ClientInfo, LastAuthor: ticket.LastAuthor, LastMessageAt: ticket.LastMessageAt,
		CreatedAt: ticket.CreatedAt, UpdatedAt: ticket.UpdatedAt,
	}
}

func messageViews(messages []Message) []MessageView {
	views := make([]MessageView, 0, len(messages))
	for _, m := range messages {
		attachments := make([]AttachmentView, 0, len(m.Attachments))
		for _, a := range m.Attachments {
			attachments = append(attachments, AttachmentView{ID: a.ID, OriginalName: a.OriginalName, ContentType: a.ContentType, SizeBytes: a.SizeBytes, CreatedAt: a.CreatedAt})
		}
		views = append(views, MessageView{ID: m.ID, AuthorType: m.AuthorType, AuthorName: m.AuthorName, Body: m.Body, CreatedAt: m.CreatedAt, Attachments: attachments})
	}
	return views
}

func validStatus(status string) bool {
	switch status {
	case StatusOpen, StatusPending, StatusReplied, StatusClosed:
		return true
	}
	return false
}

func categoriesOrDefault(categories []string) []string {
	if len(categories) == 0 {
		return append([]string(nil), DefaultCategories...)
	}
	return categories
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

// limited 去掉首尾空白，并检查长度。
func limited(value string, maxRunes int) (string, bool) {
	value = strings.TrimSpace(value)
	return value, utf8.RuneCountInString(value) <= maxRunes
}

func clip(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	return string([]rune(value)[:maxRunes])
}

// rateLimiter 是按 key 的固定窗口限流。
type rateLimiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	counts map[string]windowCount
	now    func() time.Time
}

type windowCount struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, counts: map[string]windowCount{}, now: time.Now}
}

func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	current := l.counts[key]
	if now.Sub(current.start) >= l.window {
		current = windowCount{start: now}
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	l.counts[key] = current
	// 顺手清掉过期的窗口，免得 map 无限变大。
	if len(l.counts) > 1000 {
		for k, v := range l.counts {
			if now.Sub(v.start) >= l.window {
				delete(l.counts, k)
			}
		}
	}
	return true
}
