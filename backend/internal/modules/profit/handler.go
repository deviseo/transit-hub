package profit

import (
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

// RegisterRoutes 注册利润分析接口（作用于当前工作区）。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/profit/summary", handler.summary)
	mux.HandleFunc("GET /api/profit/sync", handler.syncStatus)
	mux.HandleFunc("POST /api/profit/sync", handler.sync)
}

// syncStatus 只返回采集状态（页面顶部的后台采集提醒轮询使用，不计算分析数据）。
func (h *Handler) syncStatus(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	ws, err := h.service.CurrentWorkspace(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	status, err := h.service.SyncStatus(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, status)
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	ws, err := h.service.CurrentWorkspace(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	query := r.URL.Query()
	days := 0
	if value := strings.TrimSpace(query.Get("days")); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			httpjson.WriteError(w, http.StatusBadRequest, ErrorInvalidRange)
			return
		}
		days = parsed
	}
	response, err := h.service.Summary(r.Context(), ws, RangeQuery{Days: days, From: query.Get("from"), To: query.Get("to")})
	if err != nil {
		writeError(w, err)
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
	ws, err := h.service.CurrentWorkspace(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	status, err := h.service.TriggerSync(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusAccepted, status)
}

// writeError 不返回 401：前端会把 401 当作 TransitHub 登录过期。
func writeError(w http.ResponseWriter, err error) {
	if !IsRequestError(err) {
		log.Printf("[profit] internal error err=%v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, ErrorRequest)
		return
	}
	status := http.StatusBadRequest
	switch err.Error() {
	case ErrorNoCurrentAccount, ErrorSyncInProgress:
		status = http.StatusConflict
	}
	httpjson.WriteError(w, status, err.Error())
}
