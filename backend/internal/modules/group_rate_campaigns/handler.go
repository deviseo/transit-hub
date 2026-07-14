package group_rate_campaigns

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service  *Service
	accounts AdminAccountResolver
}

// RegisterRoutes 注册活动调价的全部路由。更精确的路径（如 .../preview）
// 会被 Go 1.22 ServeMux 优先于 "/api/group-rate-campaigns/" 这类前缀通配路由匹配，
// 因此可以像 upstream 模块一样把动作类路由和资源子路径共用同一个 trailing-slash catch-all。
func RegisterRoutes(mux *http.ServeMux, service *Service, accounts AdminAccountResolver) {
	handler := &Handler{service: service, accounts: accounts}
	mux.HandleFunc("GET /api/group-rate-campaigns", handler.list)
	mux.HandleFunc("POST /api/group-rate-campaigns/preview", handler.preview)
	mux.HandleFunc("POST /api/group-rate-campaigns", handler.create)
	mux.HandleFunc("GET /api/group-rate-campaigns/", handler.get)
	mux.HandleFunc("POST /api/group-rate-campaigns/", handler.action)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	adminAccountID, err := h.currentAdminAccountID(r.Context(), userID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	query := r.URL.Query()
	result, err := h.service.List(r.Context(), userID, adminAccountID, ListQuery{
		Page:     intQuery(query.Get("page"), 1),
		PageSize: intQuery(query.Get("pageSize"), 20),
		Status:   query.Get("status"),
	})
	if err != nil {
		log.Printf("list group rate campaigns: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "Failed to list group rate campaigns")
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto CreateCampaignRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	adminAccountID, err := h.currentAdminAccountID(r.Context(), userID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	response, err := h.service.Preview(r.Context(), userID, adminAccountID, dto)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto CreateCampaignRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	adminAccountID, err := h.currentAdminAccountID(r.Context(), userID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	response, err := h.service.Create(r.Context(), userID, adminAccountID, dto)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, response)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	id, ok := pathID(r.URL.Path, "/api/group-rate-campaigns/", "")
	if !ok {
		httpjson.WriteError(w, http.StatusNotFound, "Not Found")
		return
	}
	adminAccountID, err := h.currentAdminAccountID(r.Context(), userID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	response, err := h.service.Get(r.Context(), userID, adminAccountID, id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// action 分发 {id}/start、{id}/end、{id}/cancel 三个动作类路由。
func (h *Handler) action(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	adminAccountID, err := h.currentAdminAccountID(r.Context(), userID)
	if err != nil {
		writeDomainError(w, err)
		return
	}

	var id string
	var run func(ctx context.Context, userID string, adminAccountID string, id string) (CampaignDetail, error)
	switch {
	case pathHasID(r.URL.Path, "/start"):
		id, ok = pathID(r.URL.Path, "/api/group-rate-campaigns/", "/start")
		run = h.service.StartNow
	case pathHasID(r.URL.Path, "/end"):
		id, ok = pathID(r.URL.Path, "/api/group-rate-campaigns/", "/end")
		run = h.service.End
	case pathHasID(r.URL.Path, "/cancel"):
		id, ok = pathID(r.URL.Path, "/api/group-rate-campaigns/", "/cancel")
		run = h.service.Cancel
	default:
		ok = false
	}
	if !ok {
		httpjson.WriteError(w, http.StatusNotFound, "Not Found")
		return
	}

	response, err := run(r.Context(), userID, adminAccountID, id)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func pathHasID(path string, suffix string) bool {
	return strings.HasSuffix(path, suffix)
}

func pathID(path string, prefix string, suffix string) (string, bool) {
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

func intQuery(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func (h *Handler) currentAdminAccountID(ctx context.Context, userID string) (string, error) {
	if h.accounts == nil {
		return "", ErrNoCurrentAccount
	}
	return h.accounts.RequireCurrentID(ctx, userID)
}

// writeDomainError 把本模块的领域/校验错误映射为合适的 HTTP 状态码；
// 未识别的错误一律按 500 处理并记录日志，避免把内部错误细节泄露给前端。
func writeDomainError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpjson.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrInvalidState):
		httpjson.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrNoCurrentAccount):
		httpjson.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, ErrEmptySelection),
		errors.Is(err, ErrNoNotifyBots),
		errors.Is(err, ErrInvalidName),
		errors.Is(err, ErrInvalidAdjustment),
		errors.Is(err, ErrInvalidSchedule),
		errors.Is(err, ErrDuplicateGroup):
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		log.Printf("group rate campaigns: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "Failed to process group rate campaign request")
	}
}
