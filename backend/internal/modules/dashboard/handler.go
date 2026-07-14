package dashboard

import (
	"errors"
	"net/http"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

// Handler 只负责 HTTP 收参、状态码与 JSON 返回，业务逻辑都在 Service / MetricsService。
type Handler struct {
	service        *Service
	metricsService *MetricsService
}

// RegisterRoutes 注册仪表盘 admin 账户相关路由和指标数据路由。
// 这些路径已纳入 httpserver 的 protectedPath，需先通过 TransitHub 用户鉴权。
func RegisterRoutes(mux *http.ServeMux, service *Service, metricsService *MetricsService) {
	handler := &Handler{service: service, metricsService: metricsService}
	mux.HandleFunc("GET /api/dashboard/admin/status", handler.status)
	mux.HandleFunc("POST /api/dashboard/admin/login", handler.login)
	mux.HandleFunc("POST /api/dashboard/admin/logout", handler.logout)
	mux.HandleFunc("POST /api/dashboard/admin/refresh", handler.refreshAdminSession)
	mux.HandleFunc("GET /api/dashboard/metrics", handler.metrics)
	mux.HandleFunc("GET /api/dashboard/trends", handler.trends)
	mux.HandleFunc("GET /api/dashboard/groups", handler.adminGroups)
	mux.HandleFunc("GET /api/dashboard/group-usage-today", handler.groupUsageToday)
	mux.HandleFunc("GET /api/dashboard/upstream-key-usage-today", handler.upstreamKeyUsageToday)
	mux.HandleFunc("GET /api/dashboard/upstream-balance-breakdown", handler.upstreamBalanceBreakdown)
	mux.HandleFunc("GET /api/dashboard/balance-filter", handler.getBalanceFilter)
	mux.HandleFunc("PUT /api/dashboard/balance-filter", handler.saveBalanceFilter)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.service.Status(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto LoginRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	response, err := h.service.Login(r.Context(), userID, dto)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	if err := h.service.Logout(r.Context(), userID); err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, StatusResponse{Authenticated: false})
}

// refreshAdminSession 主动刷新当前 admin session 并重新校验 admin 身份。
func (h *Handler) refreshAdminSession(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.service.RefreshAdminSession(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// metrics 返回当前用户的仪表盘五项核心指标实时数据。
func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.metricsService.LiveMetrics(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// trends 返回历史趋势数据，通过 ?days=7 或 ?days=30 指定查询范围。
func (h *Handler) trends(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	days := 7
	if r.URL.Query().Get("days") == "30" {
		days = 30
	}
	response, err := h.metricsService.Trends(r.Context(), userID, days)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// adminGroups 返回管理员站点的分组列表。
func (h *Handler) adminGroups(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.metricsService.AdminGroups(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// groupUsageToday 返回当前工作区「我的站点」所有分组今日的使用额度明细。
func (h *Handler) groupUsageToday(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.metricsService.GroupUsageToday(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// upstreamKeyUsageToday 返回当前工作区所有上游站点中，今天有消费的 key 明细（仪表盘「今日成本」下钻）。
func (h *Handler) upstreamKeyUsageToday(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.metricsService.UpstreamKeyUsageToday(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// upstreamBalanceBreakdown 返回当前工作区所有上游站点的余额明细（仪表盘「上游总余额」下钻）。
func (h *Handler) upstreamBalanceBreakdown(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.metricsService.UpstreamBalanceBreakdown(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// getBalanceFilter 返回当前用户的站点用户余额筛选配置。
func (h *Handler) getBalanceFilter(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	config, err := h.metricsService.GetBalanceFilter(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, config)
}

// saveBalanceFilter 保存当前用户的站点用户余额筛选配置。
func (h *Handler) saveBalanceFilter(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var config BalanceFilterConfig
	if err := httpjson.Decode(r, &config); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	config.UserID = userID
	if config.ExcludeBalances == nil {
		config.ExcludeBalances = []float64{}
	}
	if err := h.metricsService.SaveBalanceFilter(r.Context(), userID, config); err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, config)
}

// writeError 把 service 的业务错误映射成合适的 HTTP 状态码与 i18n 错误 key。
func writeError(w http.ResponseWriter, err error) {
	var requestErr requestError
	if errors.As(err, &requestErr) {
		status := http.StatusBadRequest
		switch requestErr {
		case requestError(ErrorAdminOnly):
			status = http.StatusForbidden
		case requestError(ErrorPlatformUnsupported):
			status = http.StatusNotImplemented
		}
		httpjson.WriteError(w, status, requestErr.Error())
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorUnknown)
}
