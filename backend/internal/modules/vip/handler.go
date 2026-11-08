package vip

import (
	"encoding/json"
	"log"
	"net/http"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

// RegisterRoutes 注册大户关注接口。数据来自用户分析同步到本地的数据；写入的只有关注名单和设置，不改站点。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/vip/summary", handler.summary)
	mux.HandleFunc("GET /api/vip/search", handler.search)
	mux.HandleFunc("POST /api/vip/watch", handler.watch)
	mux.HandleFunc("POST /api/vip/unwatch", handler.unwatch)
	mux.HandleFunc("POST /api/vip/dismissed/restore", handler.restoreDismissed)
	mux.HandleFunc("PUT /api/vip/settings", handler.saveSettings)
}

// workspace 取当前用户和工作区；失败时已经写好了错误响应。
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

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	summary, err := h.service.Summary(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, summary)
}

func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	candidates, err := h.service.Search(r.Context(), ws, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string][]Candidate{"items": candidates})
}

type watchRequest struct {
	SiteUserID string `json:"siteUserId"`
	Note       string `json:"note"`
}

func decode(w http.ResponseWriter, r *http.Request, target any, errorKey string) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(target); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, errorKey)
		return false
	}
	return true
}

func (h *Handler) watch(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var request watchRequest
	if !decode(w, r, &request, ErrorInvalidUser) {
		return
	}
	if err := h.service.Watch(r.Context(), ws, request.SiteUserID, request.Note); err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) unwatch(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var request watchRequest
	if !decode(w, r, &request, ErrorInvalidUser) {
		return
	}
	if err := h.service.Unwatch(r.Context(), ws, request.SiteUserID); err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) restoreDismissed(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	count, err := h.service.RestoreDismissed(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]int{"restored": count})
}

func (h *Handler) saveSettings(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var request Settings
	if !decode(w, r, &request, ErrorInvalidSettings) {
		return
	}
	saved, err := h.service.SaveSettings(r.Context(), ws, request)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, saved)
}

// writeError 把业务错误映射为 HTTP 状态码。不返回 401：前端会把 401 当作 TransitHub 登录过期。
func writeError(w http.ResponseWriter, err error) {
	if !IsRequestError(err) {
		log.Printf("[vip] internal error err=%v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, ErrorRequest)
		return
	}
	status := http.StatusBadRequest
	switch err.Error() {
	case ErrorNoCurrentAccount:
		status = http.StatusConflict
	case ErrorUserNotFound:
		status = http.StatusNotFound
	}
	httpjson.WriteError(w, status, err.Error())
}
