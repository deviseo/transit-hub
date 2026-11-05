package tickets

import (
	"errors"
	"io"
	"log"
	"mime"
	"net/http"
	"strconv"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

// RegisterRoutes 注册工单接口。
//   - /api/client/v1/...：TransitLink 用，在公开的 /api/client/ 下，用工单会话（Bearer）鉴权。
//   - /api/tickets/...：TransitHub 后台用，需要登录（在受保护的路径里）。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	h := &Handler{service: service}
	mux.HandleFunc("POST /api/client/v1/sites/{code}/tickets/session", h.createSession)
	mux.HandleFunc("GET /api/client/v1/tickets", h.listMine)
	mux.HandleFunc("POST /api/client/v1/tickets", h.createMine)
	mux.HandleFunc("GET /api/client/v1/tickets/{id}", h.detailMine)
	mux.HandleFunc("POST /api/client/v1/tickets/{id}/messages", h.replyMine)
	mux.HandleFunc("POST /api/client/v1/tickets/{id}/close", h.closeMine)
	mux.HandleFunc("GET /api/client/v1/tickets/attachments/{attachmentID}", h.attachmentMine)

	mux.HandleFunc("GET /api/tickets", h.adminList)
	mux.HandleFunc("GET /api/tickets/counts", h.adminCounts)
	mux.HandleFunc("GET /api/tickets/{id}", h.adminDetail)
	mux.HandleFunc("POST /api/tickets/{id}/messages", h.adminReply)
	mux.HandleFunc("PUT /api/tickets/{id}/status", h.adminStatus)
	mux.HandleFunc("GET /api/tickets/attachments/{attachmentID}", h.adminAttachment)
}

// ---- TransitLink ----

func (h *Handler) createSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorInvalid)
		return
	}
	response, err := h.service.CreateSession(r.Context(), r.PathValue("code"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// session 读取请求里的工单会话（Authorization: Bearer <令牌>）。
func (h *Handler) session(w http.ResponseWriter, r *http.Request) (*ClientSession, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	session, err := h.service.Session(r.Context(), token)
	if err != nil {
		writeError(w, err)
		return nil, false
	}
	return session, true
}

func (h *Handler) listMine(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	response, err := h.service.ListMine(r.Context(), session)
	respond(w, http.StatusOK, response, err)
}

func (h *Handler) createMine(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	var req CreateTicketRequest
	uploads, err := readForm(w, r, &req, func(get func(string) string) {
		req = CreateTicketRequest{Title: get("title"), Category: get("category"), Body: get("body"), Contact: get("contact")}
	})
	if err != nil {
		writeError(w, err)
		return
	}
	detail, err := h.service.CreateMine(r.Context(), session, req, uploads)
	respond(w, http.StatusCreated, detail, err)
}

func (h *Handler) detailMine(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	detail, err := h.service.DetailMine(r.Context(), session, r.PathValue("id"))
	respond(w, http.StatusOK, detail, err)
}

func (h *Handler) replyMine(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	var req ReplyRequest
	uploads, err := readForm(w, r, &req, func(get func(string) string) {
		req = ReplyRequest{Body: get("body")}
	})
	if err != nil {
		writeError(w, err)
		return
	}
	detail, err := h.service.ReplyMine(r.Context(), session, r.PathValue("id"), req, uploads)
	respond(w, http.StatusOK, detail, err)
}

func (h *Handler) closeMine(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	detail, err := h.service.CloseMine(r.Context(), session, r.PathValue("id"))
	respond(w, http.StatusOK, detail, err)
}

func (h *Handler) attachmentMine(w http.ResponseWriter, r *http.Request) {
	session, ok := h.session(w, r)
	if !ok {
		return
	}
	attachment, data, err := h.service.AttachmentMine(r.Context(), session, r.PathValue("attachmentID"), wantsThumbnail(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeAttachment(w, attachment, data)
}

// ---- TransitHub 后台 ----

func (h *Handler) adminUser(w http.ResponseWriter, r *http.Request) (string, bool) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return "", false
	}
	return userID, true
}

func (h *Handler) adminList(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.adminUser(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("pageSize"))
	response, err := h.service.AdminList(r.Context(), userID, strings.TrimSpace(query.Get("status")), page, pageSize)
	respond(w, http.StatusOK, response, err)
}

func (h *Handler) adminCounts(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.adminUser(w, r)
	if !ok {
		return
	}
	counts, err := h.service.AdminCounts(r.Context(), userID)
	respond(w, http.StatusOK, counts, err)
}

func (h *Handler) adminDetail(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.adminUser(w, r)
	if !ok {
		return
	}
	detail, err := h.service.AdminDetail(r.Context(), userID, r.PathValue("id"))
	respond(w, http.StatusOK, detail, err)
}

func (h *Handler) adminReply(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.adminUser(w, r)
	if !ok {
		return
	}
	var req ReplyRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorInvalid)
		return
	}
	detail, err := h.service.AdminReply(r.Context(), userID, r.PathValue("id"), req)
	respond(w, http.StatusOK, detail, err)
}

func (h *Handler) adminStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.adminUser(w, r)
	if !ok {
		return
	}
	var req StatusRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorInvalid)
		return
	}
	detail, err := h.service.AdminSetStatus(r.Context(), userID, r.PathValue("id"), strings.TrimSpace(req.Status))
	respond(w, http.StatusOK, detail, err)
}

func (h *Handler) adminAttachment(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.adminUser(w, r)
	if !ok {
		return
	}
	attachment, data, err := h.service.AdminAttachment(r.Context(), userID, r.PathValue("attachmentID"), wantsThumbnail(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeAttachment(w, attachment, data)
}

// ---- 公共 ----

// readForm 读取 JSON 或 multipart 表单（带图片时用 multipart，图片字段名 images）。
// multipart 的临时文件读完后马上删除。
func readForm(w http.ResponseWriter, r *http.Request, target any, fromForm func(get func(string) string)) ([]AttachmentUpload, error) {
	mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaType != "multipart/form-data" {
		r.Body = http.MaxBytesReader(w, r.Body, maxJSONRequestBytes)
		if err := httpjson.Decode(r, target); err != nil {
			return nil, requestError(ErrorInvalid)
		}
		return nil, nil
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadRequestBytes)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, requestError(ErrorImageTooLarge)
		}
		return nil, requestError(ErrorInvalid)
	}
	defer func() { _ = r.MultipartForm.RemoveAll() }()
	fromForm(func(key string) string {
		if values := r.MultipartForm.Value[key]; len(values) > 0 {
			return values[0]
		}
		return ""
	})
	files := r.MultipartForm.File["images"]
	if len(files) > maxImagesPerMessage {
		return nil, requestError(ErrorTooManyImages)
	}
	uploads := make([]AttachmentUpload, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			return nil, requestError(ErrorInvalidImage)
		}
		data, err := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
		_ = file.Close()
		if err != nil {
			return nil, requestError(ErrorInvalidImage)
		}
		if len(data) > maxImageBytes {
			return nil, requestError(ErrorImageTooLarge)
		}
		uploads = append(uploads, AttachmentUpload{OriginalName: header.Filename, ContentType: header.Header.Get("Content-Type"), Data: data})
	}
	return uploads, nil
}

// writeAttachment 返回图片（类型是保存时按内容判断出的，禁止浏览器另行猜测）。
// wantsThumbnail：图片地址带 ?size=thumb 时给缩略图（对话里的小图），否则给原图。
func wantsThumbnail(r *http.Request) bool {
	return r.URL.Query().Get("size") == "thumb"
}

// writeAttachment 写图片。附件不会被修改，浏览器可以缓存一天。
func writeAttachment(w http.ResponseWriter, attachment *Attachment, data []byte) {
	header := w.Header()
	header.Set("Content-Type", attachment.ContentType)
	header.Set("Content-Length", strconv.Itoa(len(data)))
	header.Set("Cache-Control", "private, max-age=86400")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	if disposition := mime.FormatMediaType("inline", map[string]string{"filename": attachment.OriginalName}); disposition != "" {
		header.Set("Content-Disposition", disposition)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func respond(w http.ResponseWriter, status int, payload any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, status, payload)
}

func writeError(w http.ResponseWriter, err error) {
	key, ok := isRequestError(err)
	if !ok {
		log.Printf("[tickets] %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "tickets.errors.request")
		return
	}
	status := http.StatusBadRequest
	switch string(key) {
	case ErrorNotFound:
		status = http.StatusNotFound
	case ErrorClosed, ErrorNoCurrentAccount:
		status = http.StatusConflict
	case ErrorDisabled, ErrorSiteLoginExpired:
		status = http.StatusForbidden
	case ErrorSessionExpired:
		status = http.StatusUnauthorized
	case ErrorSiteUnavailable:
		status = http.StatusBadGateway
	case ErrorTooManyRequests:
		status = http.StatusTooManyRequests
	}
	httpjson.WriteError(w, status, string(key))
}
