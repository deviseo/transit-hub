package tickets

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

// multipart 创建工单的请求体限制：文本字段开销 + 最多 9 张图片（每张最多 5MB），
// 留一点余量。这是一个不依赖当前 workspace 实际配置的绝对硬上限（配置范围本身就是 0-9 张），
// 在真正校验数量/大小之前先用它防止请求体本身被恶意撑到很大。
const (
	maxUploadRequestBytes = int64(maxImagesPerTicketUpperBound)*maxImageSizeBytes + 2<<20
	maxMultipartMemory    = 10 << 20
)

type Handler struct {
	service *Service
}

// RegisterRoutes 注册工单模块的全部路由。
//
// 公开 iframe 路由（/api/embed/tickets...）不会被加入 httpserver.protectedPath，因此不会
// 触发 TransitHub 登录校验、也不会注入 authctx 用户态；它们完全依赖 embedToken + sub2apiToken
// 换来的 embed session token（Authorization: Bearer <sessionToken>）鉴权。
//
// 后台路由（/api/tickets...）必须由调用方在 httpserver.protectedPath 中加入 "/api/tickets" 前缀，
// 使用现有 TransitHub 登录态；本文件内的 handler 一律通过 authctx.UserID 读取当前用户。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}

	mux.HandleFunc("POST /api/embed/tickets/session", handler.createEmbedSession)
	mux.HandleFunc("GET /api/embed/tickets", handler.listEmbedTickets)
	mux.HandleFunc("POST /api/embed/tickets", handler.createEmbedTicket)
	mux.HandleFunc("GET /api/embed/tickets/{id}", handler.getEmbedTicket)
	mux.HandleFunc("POST /api/embed/tickets/{id}/messages", handler.addEmbedMessage)
	mux.HandleFunc("GET /api/embed/tickets/attachments/{id}", handler.getEmbedAttachment)

	// Go 1.22+ ServeMux 会拒绝互相重叠但谁也不更具体的动态模式，例如
	// "/api/tickets/attachments/{id}" 与 "/api/tickets/{id}/sub2api-user-profile"。
	// 因此后台 GET 工单详情和 Sub2API 用户资料共用一个 trailing-slash catch-all，
	// 在 handler 内部分发，同时保留外部 URL 形状。
	mux.HandleFunc("GET /api/tickets/embed-config", handler.getEmbedConfig)
	mux.HandleFunc("PUT /api/tickets/embed-config", handler.updateEmbedConfig)
	mux.HandleFunc("POST /api/tickets/embed-config/rotate-token", handler.rotateEmbedToken)
	mux.HandleFunc("GET /api/tickets/attachments/{id}", handler.getAdminAttachment)
	mux.HandleFunc("GET /api/tickets", handler.listAdminTickets)
	mux.HandleFunc("GET /api/tickets/", handler.routeAdminTicketGet)
	mux.HandleFunc("POST /api/tickets/{id}/messages", handler.addAdminMessage)
	mux.HandleFunc("PUT /api/tickets/{id}/status", handler.updateStatus)
}

// ---- 公开 iframe handler ----

func (h *Handler) createEmbedSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorEmbedRequest)
		return
	}
	resp, err := h.service.CreateEmbedSession(r.Context(), req)
	if err != nil {
		writeEmbedError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) listEmbedTickets(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.ListMyTickets(r.Context(), embedSessionToken(r))
	if err != nil {
		writeEmbedError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

// createEmbedTicket 支持两种请求体：
//   - application/json（旧格式，无图片）：解析规则和第一版完全一致，不破坏旧前端。
//   - multipart/form-data（新格式，可带图片）：manualEmail/title/body 走表单字段，
//     图片走重复的 images 字段。
func (h *Handler) createEmbedTicket(w http.ResponseWriter, r *http.Request) {
	var req CreateTicketRequest
	var uploads []AttachmentUpload

	if isMultipartRequest(r) {
		var err error
		req, uploads, err = parseMultipartTicketRequest(w, r)
		if err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, ErrorEmbedRequest)
			return
		}
	} else if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorEmbedRequest)
		return
	}

	resp, err := h.service.CreateTicket(r.Context(), embedSessionToken(r), req, uploads)
	if err != nil {
		writeEmbedError(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, resp)
}

func isMultipartRequest(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data")
}

// parseMultipartTicketRequest 解析 multipart/form-data 请求体。r.Body 先经过
// http.MaxBytesReader 限制，防止请求体本身被撑到远超合理范围；单张图片是否真的超过
// 5MB、总数量是否超过当前 workspace 配置，交给 Service 层用实际业务规则校验。
//
// ParseMultipartForm 会把超过 maxMultipartMemory 的文件部分写入系统临时目录（multipart-*），
// 必须在本函数返回前调用 RemoveAll 清理，否则多次上传接近上限的大图片会在系统临时目录堆积。
// 用 defer 保证成功、业务校验失败（数量超限/类型非法等，那些校验发生在 Service 层，本函数
// 只负责把文件内容读进内存）、读取中途出错这三类返回路径都会执行清理；defer 触发时点在
// return 之后，所以所有 readMultipartFile 调用一定已经把需要的数据拷贝进 []byte，不会出现
// "先删临时文件、后读内容" 的时序问题。
func parseMultipartTicketRequest(w http.ResponseWriter, r *http.Request) (CreateTicketRequest, []AttachmentUpload, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestBytes)
	if err := r.ParseMultipartForm(maxMultipartMemory); err != nil {
		return CreateTicketRequest{}, nil, err
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	req := CreateTicketRequest{
		ManualEmail: r.FormValue("manualEmail"),
		Title:       r.FormValue("title"),
		Body:        r.FormValue("body"),
		Category:    r.FormValue("category"),
		Priority:    r.FormValue("priority"),
	}

	var uploads []AttachmentUpload
	if r.MultipartForm != nil {
		for _, fileHeader := range r.MultipartForm.File["images"] {
			data, err := readMultipartFile(fileHeader)
			if err != nil {
				return CreateTicketRequest{}, nil, err
			}
			uploads = append(uploads, AttachmentUpload{
				OriginalName: fileHeader.Filename,
				ContentType:  fileHeader.Header.Get("Content-Type"),
				Data:         data,
			})
		}
	}
	return req, uploads, nil
}

// readMultipartFile 读取单个上传文件，读取上限比 maxImageSizeBytes 多 1 字节，
// 使 Service 层的大小校验能准确识别"超限"而不是被这里的截断掩盖。
func readMultipartFile(fileHeader *multipart.FileHeader) ([]byte, error) {
	file, err := fileHeader.Open()
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, maxImageSizeBytes+1))
}

func (h *Handler) getEmbedTicket(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetMyTicket(r.Context(), embedSessionToken(r), r.PathValue("id"))
	if err != nil {
		writeEmbedError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) getEmbedAttachment(w http.ResponseWriter, r *http.Request) {
	attachment, data, err := h.service.ReadEmbedAttachment(r.Context(), embedSessionToken(r), r.PathValue("id"))
	if err != nil {
		writeEmbedError(w, err)
		return
	}
	writeAttachment(w, attachment, data)
}

// writeAttachment 输出附件二进制内容，Content-Type 使用落库时嗅探得到的真实类型（不是用户
// 上传时声明的、可能被伪造的类型）。Cache-Control 限制为私有短期缓存，避免被公共 CDN/代理缓存。
func writeAttachment(w http.ResponseWriter, attachment TicketAttachment, data []byte) {
	w.Header().Set("Content-Type", attachment.ContentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *Handler) addEmbedMessage(w http.ResponseWriter, r *http.Request) {
	var req CreateMessageRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorEmbedRequest)
		return
	}
	resp, err := h.service.AddCustomerMessage(r.Context(), embedSessionToken(r), r.PathValue("id"), req)
	if err != nil {
		writeEmbedError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

// embedSessionToken 从 Authorization 头解析 embed session token，解析规则与 httpserver 对
// TransitHub 登录态使用的 Bearer 解析规则保持一致。
func embedSessionToken(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}

// ---- TransitHub 后台 handler ----

func (h *Handler) listAdminTickets(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	query := r.URL.Query()
	resp, err := h.service.ListTickets(r.Context(), userID, AdminListQuery{
		Status:   query.Get("status"),
		Page:     intQuery(query.Get("page"), 1),
		PageSize: intQuery(query.Get("pageSize"), 20),
	})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) routeAdminTicketGet(w http.ResponseWriter, r *http.Request) {
	relativePath := strings.TrimPrefix(r.URL.Path, "/api/tickets/")
	parts := strings.Split(relativePath, "/")
	if len(parts) == 1 && parts[0] != "" {
		r.SetPathValue("id", parts[0])
		h.getAdminTicket(w, r)
		return
	}
	if len(parts) == 2 && parts[0] != "" && parts[1] == "sub2api-user-profile" {
		r.SetPathValue("id", parts[0])
		h.getSub2apiUserProfile(w, r)
		return
	}
	http.NotFound(w, r)
}

func (h *Handler) getAdminTicket(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.GetTicket(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) getAdminAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	attachment, data, err := h.service.ReadAdminAttachment(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	writeAttachment(w, attachment, data)
}

func (h *Handler) getSub2apiUserProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	profile, err := h.service.GetSub2apiUserProfile(r.Context(), userID, r.PathValue("id"))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, profile)
}

func (h *Handler) addAdminMessage(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req CreateMessageRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	resp, err := h.service.AddAdminMessage(r.Context(), userID, r.PathValue("id"), req)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req UpdateStatusRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	resp, err := h.service.UpdateStatus(r.Context(), userID, r.PathValue("id"), req)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) getEmbedConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	config, err := h.service.GetEmbedConfig(r.Context(), userID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toEmbedConfigResponse(config, requestOrigin(r)))
}

func (h *Handler) updateEmbedConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req UpdateEmbedConfigRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	config, err := h.service.UpdateEmbedConfig(r.Context(), userID, req)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toEmbedConfigResponse(config, requestOrigin(r)))
}

func (h *Handler) rotateEmbedToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	config, err := h.service.RotateEmbedToken(r.Context(), userID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, toEmbedConfigResponse(config, requestOrigin(r)))
}

// toEmbedConfigResponse 把领域模型映射为响应 DTO。Enabled/AllowedSrcHost 恒返回 true/""：
// 第二阶段取消了这两项配置能力，响应字段仅为兼容旧前端保留，不反映数据库中可能存在的历史值。
// EmbedURL 仍然基于请求 Host 拼接，但前端不应再直接使用它作为最终复制/打开地址（本地开发环境下
// 请求经过 Vite 代理，这里拼出的是后端 API origin，而不是前端页面 origin）——前端改用
// window.location.origin 重新构造，这里保留字段只是为了不破坏旧前端调用方。
func toEmbedConfigResponse(config EmbedConfig, origin string) EmbedConfigResponse {
	normalized := withDefaultTicketOptions(config)
	return EmbedConfigResponse{
		Enabled:            true,
		EmbedToken:         config.EmbedToken,
		AllowedSrcHost:     "",
		EmbedURL:           origin + "/embed/tickets?embed_token=" + url.QueryEscape(config.EmbedToken),
		Template:           normalizeEmbedTemplate(config.Template),
		MaxImagesPerTicket: config.MaxImagesPerTicket,
		CategoryOptions:    normalized.CategoryOptions,
		PriorityOptions:    normalized.PriorityOptions,
	}
}

// requestOrigin 从请求推导 TransitHub 对外可访问的 scheme+host，用于拼接"复制嵌入地址"按钮里的
// 完整 URL。项目当前没有单独的 PUBLIC_BASE_URL 环境变量，优先信任反向代理设置的
// X-Forwarded-Proto/X-Forwarded-Host（生产环境常见部署方式），否则退回请求本身的 Host 和 TLS 状态。
func requestOrigin(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
		if first, _, ok := strings.Cut(proto, ","); ok {
			scheme = strings.TrimSpace(first)
		} else {
			scheme = strings.TrimSpace(proto)
		}
	}
	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		if first, _, ok := strings.Cut(forwardedHost, ","); ok {
			host = strings.TrimSpace(first)
		} else {
			host = strings.TrimSpace(forwardedHost)
		}
	}
	return scheme + "://" + host
}

func intQuery(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

// writeEmbedError 把公开 iframe 接口的领域错误映射为 HTTP 状态码。会话/校验失败一律只返回
// i18n key，不泄露内部细节（尤其不会包含 Sub2API 响应体或 token）。
func writeEmbedError(w http.ResponseWriter, err error) {
	var reqErr requestError
	if errors.As(err, &reqErr) {
		switch reqErr {
		case requestError(ErrorEmbedConfigNotFound), requestError(ErrorEmbedDisabled), requestError(ErrorEmbedUserMismatch):
			httpjson.WriteError(w, http.StatusForbidden, reqErr.Error())
		case requestError(ErrorEmbedSessionInvalid), requestError(ErrorEmbedSub2apiAuth):
			httpjson.WriteError(w, http.StatusUnauthorized, reqErr.Error())
		case requestError(ErrorNotFound):
			httpjson.WriteError(w, http.StatusNotFound, reqErr.Error())
		case requestError(ErrorTicketClosed):
			httpjson.WriteError(w, http.StatusConflict, reqErr.Error())
		default:
			httpjson.WriteError(w, http.StatusBadRequest, reqErr.Error())
		}
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorUnknown)
}

// writeAdminError 把后台接口的领域错误映射为 HTTP 状态码。
func writeAdminError(w http.ResponseWriter, err error) {
	var reqErr requestError
	if errors.As(err, &reqErr) {
		switch reqErr {
		case requestError(ErrorNotFound):
			httpjson.WriteError(w, http.StatusNotFound, reqErr.Error())
		case requestError(ErrorNoCurrentAccount):
			httpjson.WriteError(w, http.StatusConflict, reqErr.Error())
		default:
			httpjson.WriteError(w, http.StatusBadRequest, reqErr.Error())
		}
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorUnknown)
}
