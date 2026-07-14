package system

import (
	"net/http"

	"transithub/backend/internal/shared/httpjson"
)

// Handler 处理系统信息 HTTP 请求
type Handler struct {
	service *Service
}

// RegisterRoutes 注册系统相关 API 路由。
// 鉴权由调用方在 mux 层统一处理（bearer token 校验），此处只负责业务逻辑。
// 开源版仅保留版本查询，商业更新检查/触发接口已移除。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/system/version", handler.version)
}

func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	httpjson.Write(w, http.StatusOK, h.service.Version())
}
