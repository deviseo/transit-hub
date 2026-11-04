package sales

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

// RegisterRoutes 注册销售分析接口（只读，数据来自用户活跃同步的本地数据）。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/sales/summary", handler.summary)
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
