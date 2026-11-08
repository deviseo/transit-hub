package sales

import (
	"encoding/json"
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

// RegisterRoutes 注册销售分析接口。数据来自用户活跃同步的本地数据；写入的只有「不计入」的选择，不改站点。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/sales/summary", handler.summary)
	mux.HandleFunc("GET /api/sales/entries", handler.entries)
	mux.HandleFunc("POST /api/sales/exclusions", handler.exclude)
	mux.HandleFunc("POST /api/sales/exclusions/delete", handler.restore)
	mux.HandleFunc("PUT /api/sales/settings", handler.settings)
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

func (h *Handler) entries(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	page, _ := strconv.Atoi(strings.TrimSpace(query.Get("page")))
	response, err := h.service.Entries(r.Context(), ws, EntryQuery{
		From:         query.Get("from"),
		To:           query.Get("to"),
		Channel:      query.Get("channel"),
		Search:       query.Get("q"),
		ExcludedOnly: query.Get("excluded") == "1",
		Page:         page,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

type entriesRequest struct {
	Entries []EntryKey `json:"entries"`
}

func decodeEntries(w http.ResponseWriter, r *http.Request) ([]EntryKey, bool) {
	var request entriesRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&request); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorInvalidEntry)
		return nil, false
	}
	return request.Entries, true
}

func (h *Handler) exclude(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	keys, ok := decodeEntries(w, r)
	if !ok {
		return
	}
	count, err := h.service.Exclude(r.Context(), ws, keys)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]int{"changed": count})
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	keys, ok := decodeEntries(w, r)
	if !ok {
		return
	}
	count, err := h.service.Restore(r.Context(), ws, keys)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]int{"changed": count})
}

type settingsRequest struct {
	ExcludedChannels []string `json:"excludedChannels"`
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var request settingsRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&request); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorInvalidChannel)
		return
	}
	channels, err := h.service.SetExcludedChannels(r.Context(), ws, request.ExcludedChannels)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string][]string{"excludedChannels": channels})
}

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
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

// writeError 把业务错误映射为 HTTP 状态码。不返回 401：前端会把 401 当作 TransitHub 登录过期。
func writeError(w http.ResponseWriter, err error) {
	if !IsRequestError(err) {
		log.Printf("[sales] internal error err=%v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, ErrorRequest)
		return
	}
	status := http.StatusBadRequest
	if err.Error() == ErrorNoCurrentAccount {
		status = http.StatusConflict
	}
	httpjson.WriteError(w, status, err.Error())
}
