package system

import (
	"errors"
	"net/http"

	"transithub/backend/internal/shared/httpjson"
)

// Handler 处理系统信息 HTTP 请求
type Handler struct {
	service *Service
}

// RegisterRoutes 注册系统相关 API 路由。
// 鉴权由调用方在 mux 层统一处理（bearer token 校验），此处只负责业务逻辑。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/system/version", handler.version)
	mux.HandleFunc("POST /api/system/upgrade", handler.startUpgrade)
	mux.HandleFunc("GET /api/system/upgrade", handler.upgradeStatus)
	mux.HandleFunc("POST /api/system/restart", handler.startRestart)
	mux.HandleFunc("GET /api/system/restart", handler.restartStatus)
}

func (h *Handler) startUpgrade(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.StartUpgrade(r.Context())
	if err != nil {
		if maintenanceConflict(err) {
			httpjson.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusAccepted, response)
}

func (h *Handler) startRestart(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.StartRestart(r.Context())
	if err != nil {
		if maintenanceConflict(err) {
			httpjson.WriteError(w, http.StatusConflict, err.Error())
			return
		}
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusAccepted, response)
}

func (h *Handler) upgradeStatus(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.UpgradeStatus(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) restartStatus(w http.ResponseWriter, r *http.Request) {
	response, err := h.service.RestartStatus(r.Context())
	if err != nil {
		httpjson.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func maintenanceConflict(err error) bool {
	return errors.Is(err, ErrUpgradeInProgress) || errors.Is(err, ErrRestartInProgress)
}

func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	httpjson.Write(w, http.StatusOK, h.service.Version())
}
