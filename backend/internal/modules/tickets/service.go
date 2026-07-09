package tickets

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"transithub/backend/internal/modules/upstream"
)

// emailPattern 是新增工单时手动邮箱的基本格式校验，不追求 RFC 5322 完整实现，
// 只拦截明显不是邮箱的输入。
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// ticketRepository 是 Service 对 PostgreSQL 存储层的全部依赖，由 *Repository 结构性满足。
// 定义为接口而不是直接依赖 *Repository 具体类型，使 Service 的业务逻辑可以用内存假实现单测覆盖，
// 不需要连接真实数据库（与 group_rate_campaigns/connection_health 等模块的既有约定一致）。
type ticketRepository interface {
	EnsureSchema(ctx context.Context) error

	GetEmbedConfigByToken(ctx context.Context, embedToken string) (*EmbedConfig, error)
	GetEmbedConfigByWorkspace(ctx context.Context, userID string, adminAccountID string) (*EmbedConfig, error)
	InsertEmbedConfig(ctx context.Context, c EmbedConfig) error
	UpdateEmbedConfig(ctx context.Context, userID string, adminAccountID string, template string, maxImagesPerTicket int, categoryOptions []string, priorityOptions []string) error
	RotateEmbedToken(ctx context.Context, userID string, adminAccountID string, newToken string) error

	InsertTicketWithMessage(ctx context.Context, t Ticket, m TicketMessage, attachments []TicketAttachment) error
	InsertMessage(ctx context.Context, m TicketMessage) error
	ListMessages(ctx context.Context, ticketID string) ([]TicketMessage, error)
	ListAttachmentsByTicket(ctx context.Context, ticketID string) ([]TicketAttachment, error)
	GetAttachmentByID(ctx context.Context, id string) (*TicketAttachment, error)
	ListEmbedTickets(ctx context.Context, userID string, adminAccountID string, srcHost string, sub2apiUserID string) ([]Ticket, error)
	GetEmbedTicket(ctx context.Context, userID string, adminAccountID string, srcHost string, sub2apiUserID string, id string) (*Ticket, error)
	ListAdminTickets(ctx context.Context, userID string, adminAccountID string, status string, page int, pageSize int) ([]Ticket, int, error)
	GetAdminTicket(ctx context.Context, userID string, adminAccountID string, id string) (*Ticket, error)
	TouchTicket(ctx context.Context, id string, status string, lastMessageAt time.Time) error
	UpdateStatus(ctx context.Context, id string, status string) error
}

// embedSessionStore 是 Service 对 Redis 会话存储的依赖，由 *EmbedSessionStore 结构性满足。
type embedSessionStore interface {
	Save(ctx context.Context, token string, session EmbedSession) error
	Get(ctx context.Context, token string) (*EmbedSession, error)
}

// sub2APIFetcher 是 Service 对 Sub2API 只读身份接口的依赖，由 *Sub2APIClient 结构性满足。
type sub2APIFetcher interface {
	FetchCurrentUser(srcHost string, token string) (Sub2APIUser, error)
}

// attachmentStorage 是 Service 对本地磁盘附件存储的依赖，由 *AttachmentStorage 结构性满足。
type attachmentStorage interface {
	Save(contentType string, data []byte) (string, error)
	Read(storagePath string) ([]byte, error)
	Delete(storagePath string) error
}

// idGenerator/tokenGenerator 允许测试注入固定值；生产环境使用 repository.go 中的
// randomID/randomToken（crypto/rand）。
type idGenerator func() (string, error)

// adminSessionProvider 是 Service 对"当前 workspace 已刷新并验证的 Sub2API admin 会话"的依赖，
// 由 my_sites.Service 结构性满足。只用于只读查询 Sub2API 用户资料，绝不用它写入/修改数据，
// 也不会把这里拿到的 token 存进 tickets 表或返回给前端。
type adminSessionProvider interface {
	RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error)
}

// sub2APIAdminClient 是 Service 对 Sub2API admin 用户只读查询接口的依赖，
// 由 *upstream.PlatformService 结构性满足。
type sub2APIAdminClient interface {
	FetchSub2APIAdminUser(session upstream.Session, userID string) (upstream.Sub2APIAdminUser, error)
	FetchSub2APIAdminUserBalanceHistory(session upstream.Session, userID string, page int, pageSize int, codeType string) (upstream.Sub2APIUserBalanceHistory, error)
}

type Service struct {
	repository    ticketRepository
	sessions      embedSessionStore
	sub2api       sub2APIFetcher
	storage       attachmentStorage
	accounts      AdminAccountResolver
	adminSessions adminSessionProvider
	sub2apiAdmin  sub2APIAdminClient
	newID         idGenerator
	newToken      idGenerator
	now           func() time.Time
}

func NewService(repository *Repository, sessions *EmbedSessionStore, sub2api *Sub2APIClient, storage *AttachmentStorage) *Service {
	return &Service{
		repository: repository,
		sessions:   sessions,
		sub2api:    sub2api,
		storage:    storage,
		newID:      randomID,
		newToken:   randomToken,
		now:        time.Now,
	}
}

func (s *Service) EnsureSchema(ctx context.Context) error {
	return s.repository.EnsureSchema(ctx)
}

func (s *Service) SetAdminAccountResolver(accounts AdminAccountResolver) {
	s.accounts = accounts
}

// SetAdminSessionProvider 注入当前 workspace 的 Sub2API admin 会话来源（my_sites.Service）。
// 未注入时 GetSub2apiUserProfile 的实时字段一律降级为不可用，不会 panic。
func (s *Service) SetAdminSessionProvider(provider adminSessionProvider) {
	s.adminSessions = provider
}

// SetSub2APIAdminClient 注入 Sub2API admin 用户资料/余额历史只读查询能力（upstream.PlatformService）。
func (s *Service) SetSub2APIAdminClient(client sub2APIAdminClient) {
	s.sub2apiAdmin = client
}

func (s *Service) requireCurrentAdminAccountID(ctx context.Context, userID string) (string, error) {
	if s.accounts == nil {
		return "", requestError(ErrorNoCurrentAccount)
	}
	return s.accounts.RequireCurrentID(ctx, userID)
}

// ---- 公开 iframe 接口 ----

// CreateEmbedSession 实现文档 POST /api/embed/tickets/session 的完整校验链路：
//  1. 按 embedToken 找到 workspace 配置，不存在/禁用一律 403（ErrorEmbedConfigNotFound/ErrorEmbedDisabled）。
//  2. allowedSrcHost 非空时必须与请求 srcHost 规范化后完全匹配，否则拒绝（防止 iframe 被嵌入到未授权域名）。
//  3. 用请求携带的 sub2apiToken 向 srcHost 请求 Sub2API 当前用户，只信任这个接口返回的身份，
//     不解析/信任 JWT payload，也不直接信任 URL 里的 user_id。
//  4. URL user_id（如果非空）必须与 Sub2API 返回的用户 ID 一致，不一致视为伪造，拒绝。
//  5. 签发一个不含任何 Sub2API token 的短期 embed session token，交给前端后续请求使用。
func (s *Service) CreateEmbedSession(ctx context.Context, req CreateSessionRequest) (CreateSessionResponse, error) {
	embedToken := strings.TrimSpace(req.EmbedToken)
	sub2apiToken := strings.TrimSpace(req.Sub2apiToken)
	if embedToken == "" || sub2apiToken == "" {
		return CreateSessionResponse{}, requestError(ErrorEmbedRequest)
	}

	config, err := s.repository.GetEmbedConfigByToken(ctx, embedToken)
	if err != nil {
		return CreateSessionResponse{}, err
	}
	if config == nil {
		return CreateSessionResponse{}, requestError(ErrorEmbedConfigNotFound)
	}
	// 第二阶段取消了"启用嵌入工单"和"允许来源域名"两项配置能力：不再依据 config.Enabled/
	// config.AllowedSrcHost 拒绝会话请求，避免历史上被关闭或限制过来源的旧数据继续导致 iframe
	// 无法访问。字段本身保留在数据库和结构体中用于兼容，只是不再参与这里的判定。

	normalizedSrcHost, err := normalizeSrcHost(req.SrcHost)
	if err != nil {
		return CreateSessionResponse{}, err
	}

	user, err := s.sub2api.FetchCurrentUser(normalizedSrcHost, sub2apiToken)
	if err != nil {
		var sub2apiErr *sub2APIError
		if errors.As(err, &sub2apiErr) && sub2apiErr.unauthorized {
			return CreateSessionResponse{}, requestError(ErrorEmbedSub2apiAuth)
		}
		return CreateSessionResponse{}, requestError(ErrorEmbedSub2apiRequest)
	}

	urlUserID := strings.TrimSpace(req.UrlUserID)
	if urlUserID != "" && urlUserID != user.ID {
		return CreateSessionResponse{}, requestError(ErrorEmbedUserMismatch)
	}

	sessionToken, err := s.newToken()
	if err != nil {
		return CreateSessionResponse{}, err
	}
	template := normalizeEmbedTemplate(config.Template)
	normalizedOptions := withDefaultTicketOptions(*config)
	session := EmbedSession{
		UserID:             config.UserID,
		AdminAccountID:     config.AdminAccountID,
		EmbedToken:         config.EmbedToken,
		SrcHost:            normalizedSrcHost,
		SrcURL:             strings.TrimSpace(req.SrcURL),
		Sub2apiUserID:      user.ID,
		Sub2apiEmail:       user.Email,
		Sub2apiRole:        user.Role,
		Template:           template,
		MaxImagesPerTicket: config.MaxImagesPerTicket,
		CategoryOptions:    normalizedOptions.CategoryOptions,
		PriorityOptions:    normalizedOptions.PriorityOptions,
	}
	if err := s.sessions.Save(ctx, sessionToken, session); err != nil {
		return CreateSessionResponse{}, err
	}
	return CreateSessionResponse{
		SessionToken:       sessionToken,
		Template:           template,
		MaxImagesPerTicket: config.MaxImagesPerTicket,
		CategoryOptions:    normalizedOptions.CategoryOptions,
		PriorityOptions:    normalizedOptions.PriorityOptions,
	}, nil
}

// requireSession 解析 Authorization: Bearer <embedSessionToken>，会话不存在/过期一律视为
// 未认证。embed_token 本身只用于定位 iframe 归属的 workspace，真正的用户身份鉴权始终依赖这个
// 会话（而会话的建立又依赖 Sub2API token 校验），不会把 embed_token 当成后台鉴权凭证。
func (s *Service) requireSession(ctx context.Context, sessionToken string) (*EmbedSession, error) {
	sessionToken = strings.TrimSpace(sessionToken)
	if sessionToken == "" {
		return nil, requestError(ErrorEmbedSessionInvalid)
	}
	session, err := s.sessions.Get(ctx, sessionToken)
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, requestError(ErrorEmbedSessionInvalid)
	}
	return session, nil
}

func (s *Service) ListMyTickets(ctx context.Context, sessionToken string) (EmbedTicketListResponse, error) {
	session, err := s.requireSession(ctx, sessionToken)
	if err != nil {
		return EmbedTicketListResponse{}, err
	}
	tickets, err := s.repository.ListEmbedTickets(ctx, session.UserID, session.AdminAccountID, session.SrcHost, session.Sub2apiUserID)
	if err != nil {
		return EmbedTicketListResponse{}, err
	}
	items := make([]EmbedTicketListItem, 0, len(tickets))
	for _, t := range tickets {
		items = append(items, toEmbedTicketListItem(t))
	}
	return EmbedTicketListResponse{Items: items}, nil
}

// CreateTicket 创建工单，可选携带图片附件（新建工单场景是第一版唯一支持上传图片的入口，
// 见文档"新建工单时可上传图片"）。上传数量上限来自当前 workspace 的实时配置（不是 embed
// session 建立时刻的快照，配置随时可能被后台改动），因此这里重新查一次 embed config。
//
// 附件处理顺序：先校验（数量/大小/content-type），再写磁盘，最后写数据库；数据库事务失败时
// 主动删除刚写入的文件，避免留下没有对应记录的孤儿文件（磁盘写入本身失败也不会污染数据库，
// 因为这一步永远在数据库写入之前）。
func (s *Service) CreateTicket(ctx context.Context, sessionToken string, req CreateTicketRequest, uploads []AttachmentUpload) (EmbedTicketDetail, error) {
	session, err := s.requireSession(ctx, sessionToken)
	if err != nil {
		return EmbedTicketDetail{}, err
	}

	manualEmail := strings.TrimSpace(req.ManualEmail)
	title := strings.TrimSpace(req.Title)
	body := strings.TrimSpace(req.Body)
	if manualEmail == "" || !emailPattern.MatchString(manualEmail) {
		return EmbedTicketDetail{}, requestError(ErrorEmbedInvalidEmail)
	}
	if title == "" {
		return EmbedTicketDetail{}, requestError(ErrorEmbedTitleRequired)
	}
	if body == "" {
		return EmbedTicketDetail{}, requestError(ErrorEmbedBodyRequired)
	}

	// 分类/优先级必须按当前 workspace 的实时配置校验（不是 embed session 建立时刻的快照，
	// 配置随时可能被后台改动），因此和 maxImages 一起复用同一次 GetEmbedConfigByWorkspace 查询。
	// config 为 nil（理论上不应该发生，embed session 存在意味着 embed config 存在）时退回默认
	// 选项组，与 withDefaultTicketOptions 的兜底语义保持一致，不让整个创建流程 panic。
	maxImages := 0
	categoryOptions := append([]string(nil), DefaultCategoryOptions...)
	priorityOptions := append([]string(nil), DefaultPriorityOptions...)
	config, err := s.repository.GetEmbedConfigByWorkspace(ctx, session.UserID, session.AdminAccountID)
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	if config != nil {
		maxImages = config.MaxImagesPerTicket
		normalized := withDefaultTicketOptions(*config)
		categoryOptions = normalized.CategoryOptions
		priorityOptions = normalized.PriorityOptions
	}

	category := strings.TrimSpace(req.Category)
	if category == "" {
		return EmbedTicketDetail{}, requestError(ErrorEmbedCategoryRequired)
	}
	if !containsTicketOption(categoryOptions, category) {
		return EmbedTicketDetail{}, requestError(ErrorEmbedInvalidCategory)
	}
	priority := strings.TrimSpace(req.Priority)
	if priority == "" {
		return EmbedTicketDetail{}, requestError(ErrorEmbedPriorityRequired)
	}
	if !containsTicketOption(priorityOptions, priority) {
		return EmbedTicketDetail{}, requestError(ErrorEmbedInvalidPriority)
	}

	validatedUploads, err := validateAttachmentUploads(uploads, maxImages)
	if err != nil {
		return EmbedTicketDetail{}, err
	}

	ticketID, err := s.newID()
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	messageID, err := s.newID()
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	now := s.now()

	ticket := Ticket{
		ID:             ticketID,
		UserID:         session.UserID,
		AdminAccountID: session.AdminAccountID,
		Sub2apiSrcHost: session.SrcHost,
		Sub2apiSrcURL:  session.SrcURL,
		Sub2apiUserID:  session.Sub2apiUserID,
		Sub2apiEmail:   session.Sub2apiEmail,
		Sub2apiRole:    session.Sub2apiRole,
		ManualEmail:    manualEmail,
		Title:          title,
		Status:         StatusOpen,
		Category:       category,
		Priority:       priority,
		LastMessageAt:  now,
	}
	message := TicketMessage{
		ID:             messageID,
		TicketID:       ticketID,
		UserID:         session.UserID,
		AdminAccountID: session.AdminAccountID,
		AuthorType:     AuthorCustomer,
		AuthorName:     manualEmail,
		Body:           body,
	}

	attachments, savedPaths, err := s.saveAttachments(validatedUploads, ticketID, messageID, session.UserID, session.AdminAccountID, AuthorCustomer)
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	if err := s.repository.InsertTicketWithMessage(ctx, ticket, message, attachments); err != nil {
		s.cleanupAttachmentFiles(savedPaths)
		return EmbedTicketDetail{}, err
	}
	return s.GetMyTicket(ctx, sessionToken, ticketID)
}

// saveAttachments 把已校验的图片依次写入磁盘，并构造对应的 TicketAttachment 记录。
// 任意一张图片写盘失败都会清理掉本次已经成功写入的文件后返回错误，不留下部分文件。
func (s *Service) saveAttachments(uploads []AttachmentUpload, ticketID, messageID, userID, adminAccountID, authorType string) ([]TicketAttachment, []string, error) {
	if len(uploads) == 0 {
		return nil, nil, nil
	}
	attachments := make([]TicketAttachment, 0, len(uploads))
	savedPaths := make([]string, 0, len(uploads))
	for _, upload := range uploads {
		storagePath, err := s.storage.Save(upload.ContentType, upload.Data)
		if err != nil {
			s.cleanupAttachmentFiles(savedPaths)
			return nil, nil, err
		}
		savedPaths = append(savedPaths, storagePath)

		id, err := s.newID()
		if err != nil {
			s.cleanupAttachmentFiles(savedPaths)
			return nil, nil, err
		}
		attachments = append(attachments, TicketAttachment{
			ID:             id,
			TicketID:       ticketID,
			MessageID:      messageID,
			UserID:         userID,
			AdminAccountID: adminAccountID,
			AuthorType:     authorType,
			OriginalName:   sanitizeAttachmentName(upload.OriginalName),
			ContentType:    upload.ContentType,
			SizeBytes:      int64(len(upload.Data)),
			StoragePath:    storagePath,
		})
	}
	return attachments, savedPaths, nil
}

// cleanupAttachmentFiles 是数据库写入失败后的补偿清理，逐个删除已经落盘的文件；
// 删除本身失败只忽略（最坏情况是留下极少数孤儿文件，不影响正确性和安全边界)。
func (s *Service) cleanupAttachmentFiles(storagePaths []string) {
	for _, path := range storagePaths {
		_ = s.storage.Delete(path)
	}
}

// sanitizeAttachmentName 只保留原始文件名用于展示，裁剪长度并去除首尾空白；
// 该值绝不参与磁盘路径拼接（见 AttachmentStorage.Save 使用服务端生成的文件名），因此这里
// 不需要过滤路径穿越字符，只是防止界面被异常长的文件名撑破。
func sanitizeAttachmentName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "image"
	}
	const maxNameLength = 200
	if len(trimmed) > maxNameLength {
		return trimmed[:maxNameLength]
	}
	return trimmed
}

func (s *Service) GetMyTicket(ctx context.Context, sessionToken string, id string) (EmbedTicketDetail, error) {
	session, err := s.requireSession(ctx, sessionToken)
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	ticket, err := s.repository.GetEmbedTicket(ctx, session.UserID, session.AdminAccountID, session.SrcHost, session.Sub2apiUserID, id)
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	if ticket == nil {
		return EmbedTicketDetail{}, requestError(ErrorNotFound)
	}
	messages, attachmentsByMessage, err := s.loadMessagesWithAttachments(ctx, id)
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	return toEmbedTicketDetail(*ticket, messages, attachmentsByMessage), nil
}

// loadMessagesWithAttachments 一次性拉取一个工单的全部消息和全部附件，并把附件按 message_id
// 分组，避免逐条消息单独查询附件（N+1）。
func (s *Service) loadMessagesWithAttachments(ctx context.Context, ticketID string) ([]TicketMessage, map[string][]TicketAttachment, error) {
	messages, err := s.repository.ListMessages(ctx, ticketID)
	if err != nil {
		return nil, nil, err
	}
	attachments, err := s.repository.ListAttachmentsByTicket(ctx, ticketID)
	if err != nil {
		return nil, nil, err
	}
	byMessage := make(map[string][]TicketAttachment, len(attachments))
	for _, a := range attachments {
		byMessage[a.MessageID] = append(byMessage[a.MessageID], a)
	}
	return messages, byMessage, nil
}

// ReadEmbedAttachment 供公开 iframe 接口下载附件：先校验 embed session，再校验该附件所属的
// 工单确实匹配当前会话的四段过滤条件（workspace + src_host + sub2api_user_id），复用
// GetEmbedTicket 已有的归属校验逻辑，避免重复实现一遍同样的判断。
func (s *Service) ReadEmbedAttachment(ctx context.Context, sessionToken string, attachmentID string) (TicketAttachment, []byte, error) {
	session, err := s.requireSession(ctx, sessionToken)
	if err != nil {
		return TicketAttachment{}, nil, err
	}
	attachment, err := s.repository.GetAttachmentByID(ctx, attachmentID)
	if err != nil {
		return TicketAttachment{}, nil, err
	}
	if attachment == nil {
		return TicketAttachment{}, nil, requestError(ErrorNotFound)
	}
	ticket, err := s.repository.GetEmbedTicket(ctx, session.UserID, session.AdminAccountID, session.SrcHost, session.Sub2apiUserID, attachment.TicketID)
	if err != nil {
		return TicketAttachment{}, nil, err
	}
	if ticket == nil {
		return TicketAttachment{}, nil, requestError(ErrorNotFound)
	}
	data, err := s.storage.Read(attachment.StoragePath)
	if err != nil {
		return TicketAttachment{}, nil, err
	}
	return *attachment, data, nil
}

// AddCustomerMessage 追加一条 iframe 用户回复。已关闭工单直接拒绝（409），
// 否则按状态流转规则把工单标记为 pending（等待后台再次处理）。
func (s *Service) AddCustomerMessage(ctx context.Context, sessionToken string, id string, req CreateMessageRequest) (EmbedTicketDetail, error) {
	session, err := s.requireSession(ctx, sessionToken)
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	ticket, err := s.repository.GetEmbedTicket(ctx, session.UserID, session.AdminAccountID, session.SrcHost, session.Sub2apiUserID, id)
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	if ticket == nil {
		return EmbedTicketDetail{}, requestError(ErrorNotFound)
	}
	if ticket.Status == StatusClosed {
		return EmbedTicketDetail{}, requestError(ErrorTicketClosed)
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return EmbedTicketDetail{}, requestError(ErrorEmbedBodyRequired)
	}

	messageID, err := s.newID()
	if err != nil {
		return EmbedTicketDetail{}, err
	}
	message := TicketMessage{
		ID:             messageID,
		TicketID:       id,
		UserID:         session.UserID,
		AdminAccountID: session.AdminAccountID,
		AuthorType:     AuthorCustomer,
		AuthorName:     ticket.ManualEmail,
		Body:           body,
	}
	if err := s.repository.InsertMessage(ctx, message); err != nil {
		return EmbedTicketDetail{}, err
	}
	if err := s.repository.TouchTicket(ctx, id, StatusPending, s.now()); err != nil {
		return EmbedTicketDetail{}, err
	}
	return s.GetMyTicket(ctx, sessionToken, id)
}

// ---- TransitHub 后台接口 ----

func (s *Service) ListTickets(ctx context.Context, userID string, query AdminListQuery) (AdminTicketListResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return AdminTicketListResponse{}, err
	}
	query = normalizeAdminListQuery(query)
	tickets, total, err := s.repository.ListAdminTickets(ctx, userID, adminAccountID, query.Status, query.Page, query.PageSize)
	if err != nil {
		return AdminTicketListResponse{}, err
	}
	items := make([]AdminTicketListItem, 0, len(tickets))
	for _, t := range tickets {
		items = append(items, toAdminTicketListItem(t))
	}
	return AdminTicketListResponse{
		Items:      items,
		Total:      total,
		Page:       query.Page,
		PageSize:   query.PageSize,
		TotalPages: totalPages(total, query.PageSize),
	}, nil
}

func (s *Service) GetTicket(ctx context.Context, userID string, id string) (AdminTicketDetail, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	ticket, err := s.repository.GetAdminTicket(ctx, userID, adminAccountID, id)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	if ticket == nil {
		return AdminTicketDetail{}, requestError(ErrorNotFound)
	}
	messages, attachmentsByMessage, err := s.loadMessagesWithAttachments(ctx, id)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	return toAdminTicketDetail(*ticket, messages, attachmentsByMessage), nil
}

// ReadAdminAttachment 供后台接口下载附件：先校验 TransitHub 登录态和当前 workspace，
// 再校验该附件所属的工单确实属于 user_id + admin_account_id，复用 GetAdminTicket 已有的
// 归属校验逻辑，不允许跨 workspace 读取附件。
func (s *Service) ReadAdminAttachment(ctx context.Context, userID string, attachmentID string) (TicketAttachment, []byte, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return TicketAttachment{}, nil, err
	}
	attachment, err := s.repository.GetAttachmentByID(ctx, attachmentID)
	if err != nil {
		return TicketAttachment{}, nil, err
	}
	if attachment == nil {
		return TicketAttachment{}, nil, requestError(ErrorNotFound)
	}
	ticket, err := s.repository.GetAdminTicket(ctx, userID, adminAccountID, attachment.TicketID)
	if err != nil {
		return TicketAttachment{}, nil, err
	}
	if ticket == nil {
		return TicketAttachment{}, nil, requestError(ErrorNotFound)
	}
	data, err := s.storage.Read(attachment.StoragePath)
	if err != nil {
		return TicketAttachment{}, nil, err
	}
	return *attachment, data, nil
}

// GetSub2apiUserProfile 返回后台"Sub2API 用户资料"只读弹窗的数据。先校验该工单属于当前
// workspace（不属于则 404，不做任何远程查询——这一条是唯一会让接口整体失败的情况），
// 再尽量用当前 workspace 的 Sub2API admin 会话实时查询用户详情与余额历史来补全展示字段。
// 会话缺失/非 admin/远程查询失败等情况都只把对应字段标记为不可用并返回 200，不影响已经
// 成功获取的快照字段，也不会把 token 暴露给前端或写进日志。
func (s *Service) GetSub2apiUserProfile(ctx context.Context, userID string, ticketID string) (Sub2apiUserProfileResponse, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return Sub2apiUserProfileResponse{}, err
	}
	ticket, err := s.repository.GetAdminTicket(ctx, userID, adminAccountID, ticketID)
	if err != nil {
		return Sub2apiUserProfileResponse{}, err
	}
	if ticket == nil {
		return Sub2apiUserProfileResponse{}, requestError(ErrorNotFound)
	}

	profile := Sub2apiUserProfileResponse{
		Sub2apiUserID:  ticket.Sub2apiUserID,
		Sub2apiEmail:   ticket.Sub2apiEmail,
		Sub2apiRole:    ticket.Sub2apiRole,
		Sub2apiSrcHost: ticket.Sub2apiSrcHost,
		Sub2apiSrcURL:  ticket.Sub2apiSrcURL,
	}

	sub2apiUserID := strings.TrimSpace(ticket.Sub2apiUserID)
	if sub2apiUserID == "" {
		profile.RemoteUnavailableReason = Sub2apiRemoteUnavailableNoUserID
		return profile, nil
	}
	if s.adminSessions == nil || s.sub2apiAdmin == nil {
		profile.RemoteUnavailableReason = Sub2apiRemoteUnavailableNoAdminSession
		return profile, nil
	}
	session, err := s.adminSessions.RequireSession(ctx, userID, adminAccountID)
	if err != nil {
		profile.RemoteUnavailableReason = Sub2apiRemoteUnavailableNoAdminSession
		return profile, nil
	}

	s.enrichSub2apiUserProfile(&profile, session, sub2apiUserID)
	return profile, nil
}

// enrichSub2apiUserProfile 用当前 workspace 的 Sub2API admin 会话查询用户详情与余额历史，
// 把结果合并进 profile。用户详情查询失败（网络错误、非 2xx、远端用户不存在等）只标记
// RemoteUnavailableReason 并直接返回，不再继续查询余额历史；用户详情成功但余额历史失败时，
// 已经解析出的用户详情字段依然保留，只是余额历史相关字段维持不可用。
func (s *Service) enrichSub2apiUserProfile(profile *Sub2apiUserProfileResponse, session upstream.Session, sub2apiUserID string) {
	user, err := s.sub2apiAdmin.FetchSub2APIAdminUser(session, sub2apiUserID)
	if err != nil {
		profile.RemoteUnavailableReason = Sub2apiRemoteUnavailableUserNotFound
		return
	}
	profile.Username = user.Username
	profile.Status = user.Status
	profile.FrozenBalance = user.FrozenBalance
	profile.Concurrency = user.Concurrency
	profile.RPMLimit = user.RPMLimit
	profile.LastUsedAt = user.LastUsedAt
	if user.Balance != nil {
		profile.BalanceAvailable = true
		profile.Balance = user.Balance
	}
	if user.CreatedAt != nil {
		profile.RegisteredAtAvailable = true
		profile.RegisteredAt = user.CreatedAt
	}

	const rechargeHistoryPageSize = 20
	history, err := s.sub2apiAdmin.FetchSub2APIAdminUserBalanceHistory(session, sub2apiUserID, 1, rechargeHistoryPageSize, "balance")
	if err != nil {
		return
	}
	if history.TotalRecharged != nil {
		profile.TotalRechargedAvailable = true
		profile.TotalRecharged = history.TotalRecharged
	}
	profile.RechargeHistoryAvailable = true
	profile.RechargeHistory = make([]Sub2apiRechargeHistoryItem, 0, len(history.Items))
	for _, item := range history.Items {
		profile.RechargeHistory = append(profile.RechargeHistory, Sub2apiRechargeHistoryItem{
			ID:        item.ID,
			Type:      item.Type,
			Amount:    item.Amount,
			Note:      item.Note,
			CreatedAt: item.CreatedAt,
		})
	}
}

// AddAdminMessage 后台回复工单：插入一条 admin 消息，并把状态置为 replied（无条件，
// 已关闭工单后台仍可回复用于说明关闭原因，文档只限制了 iframe 用户不能回复已关闭工单）。
func (s *Service) AddAdminMessage(ctx context.Context, userID string, id string, req CreateMessageRequest) (AdminTicketDetail, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	ticket, err := s.repository.GetAdminTicket(ctx, userID, adminAccountID, id)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	if ticket == nil {
		return AdminTicketDetail{}, requestError(ErrorNotFound)
	}
	body := strings.TrimSpace(req.Body)
	if body == "" {
		return AdminTicketDetail{}, requestError(ErrorBodyRequired)
	}

	messageID, err := s.newID()
	if err != nil {
		return AdminTicketDetail{}, err
	}
	message := TicketMessage{
		ID:             messageID,
		TicketID:       id,
		UserID:         userID,
		AdminAccountID: adminAccountID,
		AuthorType:     AuthorAdmin,
		Body:           body,
	}
	if err := s.repository.InsertMessage(ctx, message); err != nil {
		return AdminTicketDetail{}, err
	}
	if err := s.repository.TouchTicket(ctx, id, StatusReplied, s.now()); err != nil {
		return AdminTicketDetail{}, err
	}
	return s.GetTicket(ctx, userID, id)
}

func (s *Service) UpdateStatus(ctx context.Context, userID string, id string, req UpdateStatusRequest) (AdminTicketDetail, error) {
	status := strings.TrimSpace(req.Status)
	if !isValidTicketStatus(status) {
		return AdminTicketDetail{}, requestError(ErrorInvalidStatus)
	}
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	ticket, err := s.repository.GetAdminTicket(ctx, userID, adminAccountID, id)
	if err != nil {
		return AdminTicketDetail{}, err
	}
	if ticket == nil {
		return AdminTicketDetail{}, requestError(ErrorNotFound)
	}
	if err := s.repository.UpdateStatus(ctx, id, status); err != nil {
		return AdminTicketDetail{}, err
	}
	return s.GetTicket(ctx, userID, id)
}

// GetEmbedConfig 返回当前 workspace 的嵌入配置；不存在时自动创建一条默认配置
// （enabled=true，未限制来源域名），满足"后台首次打开工单页面即可复制嵌入地址"的体验要求。
func (s *Service) GetEmbedConfig(ctx context.Context, userID string) (EmbedConfig, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedConfig{}, err
	}
	return s.ensureEmbedConfig(ctx, userID, adminAccountID)
}

func (s *Service) ensureEmbedConfig(ctx context.Context, userID string, adminAccountID string) (EmbedConfig, error) {
	config, err := s.repository.GetEmbedConfigByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedConfig{}, err
	}
	if config != nil {
		return withDefaultTicketOptions(*config), nil
	}
	token, err := s.newToken()
	if err != nil {
		return EmbedConfig{}, err
	}
	// 新建 config 必须显式带上默认分类/优先级，不能只依赖数据库列的 DEFAULT 子句：
	// fakeTicketRepository 等内存假实现不会执行真实 DDL，只有这里显式赋值才能让单测和生产行为一致。
	created := EmbedConfig{
		UserID: userID, AdminAccountID: adminAccountID, EmbedToken: token, Enabled: true, Template: TemplateDefault,
		CategoryOptions: append([]string(nil), DefaultCategoryOptions...),
		PriorityOptions: append([]string(nil), DefaultPriorityOptions...),
	}
	if err := s.repository.InsertEmbedConfig(ctx, created); err != nil {
		return EmbedConfig{}, err
	}
	// 并发场景下 InsertEmbedConfig 可能因为唯一索引 ON CONFLICT DO NOTHING 而实际没有插入这条，
	// 重新读一次保证返回的是数据库中真实生效的那一条（可能是另一个并发请求创建的）。
	config, err = s.repository.GetEmbedConfigByWorkspace(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedConfig{}, err
	}
	if config == nil {
		return EmbedConfig{}, errors.New("embed config missing after insert")
	}
	return withDefaultTicketOptions(*config), nil
}

// UpdateEmbedConfig 保存当前 workspace 的嵌入配置。第二阶段起只有 Template 真正生效；
// 第三阶段新增 MaxImagesPerTicket。req.Enabled/req.AllowedSrcHost 仅为兼容旧前端请求体保留在
// DTO 上，这里显式忽略，不再用它们关闭功能或限制来源（repository.UpdateEmbedConfig 还会顺带
// 把底层 enabled/allowed_src_host 修正为 true/空字符串，修复历史遗留数据）。
func (s *Service) UpdateEmbedConfig(ctx context.Context, userID string, req UpdateEmbedConfigRequest) (EmbedConfig, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedConfig{}, err
	}
	template, err := normalizeEmbedTemplateInput(req.Template)
	if err != nil {
		return EmbedConfig{}, err
	}
	existing, err := s.ensureEmbedConfig(ctx, userID, adminAccountID)
	if err != nil {
		return EmbedConfig{}, err
	}
	maxImages, err := normalizeMaxImagesInput(existing.MaxImagesPerTicket, req.MaxImagesPerTicket)
	if err != nil {
		return EmbedConfig{}, err
	}
	// req.CategoryOptions/PriorityOptions 为 nil 表示未传，保留已有配置（existing 已经过
	// withDefaultTicketOptions 兜底，一定是合法非空选项组）；非 nil（包括空数组）一律校验，
	// 不合法直接拒绝，不静默丢弃用户的修改意图。
	categoryOptions := existing.CategoryOptions
	if req.CategoryOptions != nil {
		normalized, ok := normalizeTicketOptions(req.CategoryOptions)
		if !ok {
			return EmbedConfig{}, requestError(ErrorInvalidCategoryOptions)
		}
		categoryOptions = normalized
	}
	priorityOptions := existing.PriorityOptions
	if req.PriorityOptions != nil {
		normalized, ok := normalizeTicketOptions(req.PriorityOptions)
		if !ok {
			return EmbedConfig{}, requestError(ErrorInvalidPriorityOptions)
		}
		priorityOptions = normalized
	}
	if err := s.repository.UpdateEmbedConfig(ctx, userID, adminAccountID, template, maxImages, categoryOptions, priorityOptions); err != nil {
		return EmbedConfig{}, err
	}
	return s.GetEmbedConfig(ctx, userID)
}

// normalizeMaxImagesInput 写入路径使用：nil 表示未传，保留现有值；传了但超出 0-9 范围则报错拒绝。
func normalizeMaxImagesInput(current int, input *int) (int, error) {
	if input == nil {
		return current, nil
	}
	if !isValidMaxImagesPerTicket(*input) {
		return 0, requestError(ErrorInvalidMaxImages)
	}
	return *input, nil
}

// RotateEmbedToken 轮换当前 workspace 的 embed_token，旧 iframe 地址（含旧 token）随之失效。
func (s *Service) RotateEmbedToken(ctx context.Context, userID string) (EmbedConfig, error) {
	adminAccountID, err := s.requireCurrentAdminAccountID(ctx, userID)
	if err != nil {
		return EmbedConfig{}, err
	}
	if _, err := s.ensureEmbedConfig(ctx, userID, adminAccountID); err != nil {
		return EmbedConfig{}, err
	}
	newToken, err := s.newToken()
	if err != nil {
		return EmbedConfig{}, err
	}
	if err := s.repository.RotateEmbedToken(ctx, userID, adminAccountID, newToken); err != nil {
		return EmbedConfig{}, err
	}
	return s.GetEmbedConfig(ctx, userID)
}

// ---- 映射 / 工具函数 ----

func toEmbedTicketListItem(t Ticket) EmbedTicketListItem {
	return EmbedTicketListItem{
		ID:            t.ID,
		Title:         t.Title,
		Status:        t.Status,
		ManualEmail:   t.ManualEmail,
		Category:      t.Category,
		Priority:      t.Priority,
		LastMessageAt: t.LastMessageAt,
		CreatedAt:     t.CreatedAt,
		UpdatedAt:     t.UpdatedAt,
	}
}

func toEmbedTicketDetail(t Ticket, messages []TicketMessage, attachmentsByMessage map[string][]TicketAttachment) EmbedTicketDetail {
	return EmbedTicketDetail{
		EmbedTicketListItem: toEmbedTicketListItem(t),
		Messages:            toMessageViews(messages, attachmentsByMessage),
	}
}

func toAdminTicketListItem(t Ticket) AdminTicketListItem {
	return AdminTicketListItem{
		ID:             t.ID,
		Title:          t.Title,
		Status:         t.Status,
		ManualEmail:    t.ManualEmail,
		Category:       t.Category,
		Priority:       t.Priority,
		Sub2apiUserID:  t.Sub2apiUserID,
		Sub2apiEmail:   t.Sub2apiEmail,
		Sub2apiRole:    t.Sub2apiRole,
		Sub2apiSrcHost: t.Sub2apiSrcHost,
		LastMessageAt:  t.LastMessageAt,
		CreatedAt:      t.CreatedAt,
	}
}

func toAdminTicketDetail(t Ticket, messages []TicketMessage, attachmentsByMessage map[string][]TicketAttachment) AdminTicketDetail {
	return AdminTicketDetail{
		AdminTicketListItem: toAdminTicketListItem(t),
		Sub2apiSrcURL:       t.Sub2apiSrcURL,
		Messages:            toMessageViews(messages, attachmentsByMessage),
	}
}

func toMessageViews(messages []TicketMessage, attachmentsByMessage map[string][]TicketAttachment) []TicketMessageView {
	views := make([]TicketMessageView, 0, len(messages))
	for _, m := range messages {
		views = append(views, TicketMessageView{
			ID:          m.ID,
			AuthorType:  m.AuthorType,
			AuthorName:  m.AuthorName,
			Body:        m.Body,
			CreatedAt:   m.CreatedAt,
			Attachments: toAttachmentViews(attachmentsByMessage[m.ID]),
		})
	}
	return views
}

func toAttachmentViews(attachments []TicketAttachment) []TicketAttachmentView {
	views := make([]TicketAttachmentView, 0, len(attachments))
	for _, a := range attachments {
		views = append(views, TicketAttachmentView{
			ID:           a.ID,
			OriginalName: a.OriginalName,
			ContentType:  a.ContentType,
			SizeBytes:    a.SizeBytes,
			CreatedAt:    a.CreatedAt,
		})
	}
	return views
}

// normalizeEmbedTemplate 读取路径使用：兼容历史数据中可能存在的空模板值（列有 DEFAULT，正常
// 不会为空，这里只是兜底），不认识的值一律回退到 default，不返回错误——展示层不应该因为一个
// 意外的模板值就整体失败。
func normalizeEmbedTemplate(template string) string {
	if isValidEmbedTemplate(template) {
		return template
	}
	return TemplateDefault
}

// normalizeEmbedTemplateInput 写入路径使用：区分"未传/空值 -> 默认 default"和"传了但不合法 ->
// 报错拒绝"，避免用户提交了拼写错误的模板名却被静默改成 default。
func normalizeEmbedTemplateInput(template string) (string, error) {
	trimmed := strings.TrimSpace(template)
	if trimmed == "" {
		return TemplateDefault, nil
	}
	if !isValidEmbedTemplate(trimmed) {
		return "", requestError(ErrorInvalidTemplate)
	}
	return trimmed, nil
}

func normalizeAdminListQuery(q AdminListQuery) AdminListQuery {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 20
	}
	if q.PageSize > 100 {
		q.PageSize = 100
	}
	q.Status = strings.TrimSpace(q.Status)
	return q
}

func totalPages(total int, pageSize int) int {
	if total == 0 || pageSize <= 0 {
		return 0
	}
	pages := total / pageSize
	if total%pageSize != 0 {
		pages++
	}
	return pages
}
