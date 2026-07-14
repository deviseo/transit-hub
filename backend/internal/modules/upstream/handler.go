package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service  *Service
	accounts HandlerAccountResolver
}

// HandlerAccountResolver 由 handler 用来在请求路径上解析当前工作区。
type HandlerAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

func RegisterRoutes(mux *http.ServeMux, service *Service, accounts HandlerAccountResolver) {
	handler := &Handler{service: service, accounts: accounts}
	mux.HandleFunc("GET /api/upstream-sites", handler.list)
	mux.HandleFunc("POST /api/upstream-sites", handler.create)
	mux.HandleFunc("POST /api/upstream-sites/sync-all", handler.syncAll)
	mux.HandleFunc("GET /api/upstream-sites/sync-stream", handler.syncStream)
	mux.HandleFunc("PUT /api/upstream-sites/", handler.update)
	mux.HandleFunc("PATCH /api/upstream-sites/", handler.update)
	mux.HandleFunc("POST /api/upstream-sites/", handler.sync)
	mux.HandleFunc("DELETE /api/upstream-sites/", handler.remove)
}

func (h *Handler) syncAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if _, err := h.requireWorkspace(r.Context(), userID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	responses, err := h.service.SyncAll(r.Context(), userID)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, responses)
}

// syncStream 以 SSE 流方式逐站同步，每个站点的进度实时推送到前端。
// 与 syncAll 的区别：站点按顺序处理（不并发），遇 Cloudflare 自动重试，
// 结果逐个流式返回而非等所有站点完成后一次性返回。
func (h *Handler) syncStream(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if _, err := h.requireWorkspace(r.Context(), userID); err != nil {
		writeWorkspaceError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		httpjson.WriteError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	emit := func(event SyncEvent) {
		data, err := json.Marshal(event)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	if err := h.service.SyncAllStream(r.Context(), userID, emit); err != nil {
		return
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if _, err := h.requireWorkspace(r.Context(), userID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, h.service.List(r.Context(), userID))
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if _, err := h.requireWorkspace(r.Context(), userID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	var dto CreateRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	response, err := h.service.Create(r.Context(), userID, dto)
	if err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	httpjson.Write(w, http.StatusCreated, response)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if _, err := h.requireWorkspace(r.Context(), userID); err != nil {
		writeWorkspaceError(w, err)
		return
	}

	// PATCH /api/upstream-sites/{id}/settings → 站点级预警覆盖设置
	if id, ok := pathID(r.URL.Path, "/api/upstream-sites/", "/settings"); ok && r.Method == http.MethodPatch {
		h.updateSettings(w, r, userID, id)
		return
	}

	id, ok := pathID(r.URL.Path, "/api/upstream-sites/", "")
	if !ok {
		httpjson.WriteError(w, http.StatusNotFound, "Not Found")
		return
	}
	var dto UpdateRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	response, err := h.service.Update(r.Context(), userID, id, dto)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// updateSettings 更新单个站点的预警覆盖配置（余额阈值）。
func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request, userID, siteID string) {
	var dto SiteSettings
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	response, err := h.service.UpdateSettings(r.Context(), userID, siteID, dto)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if _, err := h.requireWorkspace(r.Context(), userID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	id, ok := pathID(r.URL.Path, "/api/upstream-sites/", "/sync")
	if !ok {
		httpjson.WriteError(w, http.StatusNotFound, "Not Found")
		return
	}
	response, err := h.service.Sync(r.Context(), userID, id)
	if err != nil {
		writeUpstreamError(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, response)
}

func (h *Handler) remove(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if _, err := h.requireWorkspace(r.Context(), userID); err != nil {
		writeWorkspaceError(w, err)
		return
	}
	id, ok := pathID(r.URL.Path, "/api/upstream-sites/", "")
	if !ok {
		httpjson.WriteError(w, http.StatusNotFound, "Not Found")
		return
	}
	if err := h.service.Remove(r.Context(), userID, id); err != nil {
		writeUpstreamError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]bool{"success": true})
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

func (h *Handler) requireWorkspace(ctx context.Context, userID string) (string, error) {
	if h.accounts == nil {
		return "", newRequestError("admin.adminAccounts.errors.noCurrentAccount", "")
	}
	return h.accounts.RequireCurrentID(ctx, userID)
}

func writeWorkspaceError(w http.ResponseWriter, err error) {
	if err != nil && strings.Contains(err.Error(), "adminAccounts") {
		httpjson.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, "Failed to resolve admin account")
}

func writeUpstreamError(w http.ResponseWriter, err error) {
	if err != nil && strings.Contains(err.Error(), "adminAccounts") {
		httpjson.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	var requestErr *RequestError
	if errors.As(err, &requestErr) && requestErr.MessageKey == ErrorNotFound {
		httpjson.WriteError(w, http.StatusNotFound, requestErr.MessageKey)
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, errorKey(err))
}
