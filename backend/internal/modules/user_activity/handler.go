package user_activity

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/user-activity/summary", handler.summary)
	mux.HandleFunc("GET /api/user-activity/sync", handler.syncStatus)
	mux.HandleFunc("POST /api/user-activity/sync", handler.sync)
	mux.HandleFunc("GET /api/user-activity/users", handler.listUsers)
	mux.HandleFunc("GET /api/user-activity/users/{id}", handler.userDetail)
	mux.HandleFunc("GET /api/user-activity/recipients/preview", handler.previewRecipients)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	result, err := h.service.Summary(r.Context(), ws, intQuery(r.URL.Query().Get("days"), defaultSeriesDays))
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	status, err := h.service.TriggerSync(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusAccepted, status)
}

// syncStatus 只返回同步状态（页面顶部的后台同步提醒轮询使用）。
func (h *Handler) syncStatus(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	status, err := h.service.SyncStatusOf(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, status)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	result, err := h.service.ListUsers(r.Context(), ws, UserListQuery{
		Filter:   filterFromQuery(query.Get),
		Sort:     strings.TrimSpace(query.Get("sort")),
		Order:    strings.TrimSpace(query.Get("order")),
		Page:     intQuery(query.Get("page"), 1),
		PageSize: intQuery(query.Get("pageSize"), defaultPageSize),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) userDetail(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	result, err := h.service.UserDetail(r.Context(), ws, r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) previewRecipients(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	result, err := h.service.PreviewRecipients(r.Context(), ws, filterFromQuery(r.URL.Query().Get))
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (Workspace, bool) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return Workspace{}, false
	}
	ws, err := h.service.CurrentWorkspace(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return Workspace{}, false
	}
	return ws, true
}

func filterFromQuery(get func(string) string) UserFilter {
	hasEmail, _ := strconv.ParseBool(strings.TrimSpace(get("hasEmail")))
	return UserFilter{
		Segment:      strings.TrimSpace(get("segment")),
		InactiveDays: intQuery(get("inactiveDays"), 0),
		Search:       get("search"),
		Status:       strings.TrimSpace(get("status")),
		Role:         strings.TrimSpace(get("role")),
		HasEmail:     hasEmail,
	}
}

// writeError 把业务错误转成 HTTP 响应。注意：站点（sub2api/new-api）会话失效不能返回 401，
// 前端会把 401 当作 TransitHub 登录过期并退出登录。
func writeError(w http.ResponseWriter, err error) {
	var keyErr requestError
	if !errors.As(err, &keyErr) {
		log.Printf("[user-activity] internal error err=%v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, ErrorRequest)
		return
	}
	status := http.StatusBadRequest
	switch string(keyErr) {
	case ErrorNotFound:
		status = http.StatusNotFound
	case ErrorSyncInProgress:
		status = http.StatusConflict
	}
	httpjson.WriteError(w, status, string(keyErr))
}

func intQuery(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}
