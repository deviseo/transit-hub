package system

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

// maxOutboundProxyRequestBytes 限制出站代理接口的请求体大小。
const maxOutboundProxyRequestBytes = 16 * 1024

// Handler 处理系统信息 HTTP 请求
type Handler struct {
	service *Service
}

// RegisterRoutes 注册系统相关 API 路由。
// 鉴权由调用方在 mux 层统一处理（bearer token 校验），此处只负责业务逻辑。
// 开源版保留版本查询，以及系统级的出站代理设置。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/system/version", handler.version)
	mux.HandleFunc("GET /api/system/outbound-proxy", handler.getOutboundProxy)
	mux.HandleFunc("PUT /api/system/outbound-proxy", handler.saveOutboundProxy)
	mux.HandleFunc("POST /api/system/outbound-proxy/test", handler.testOutboundProxy)
}

func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	httpjson.Write(w, http.StatusOK, h.service.Version())
}

func (h *Handler) getOutboundProxy(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.service.GetOutboundProxy(r.Context(), userID)
	if err != nil {
		writeOutboundProxyError(w, "get", err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) saveOutboundProxy(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req SaveOutboundProxyRequest
	if !decodeOutboundProxyRequest(w, r, &req) {
		return
	}
	response, err := h.service.SaveOutboundProxy(r.Context(), userID, req)
	if err != nil {
		writeOutboundProxyError(w, "save", err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) testOutboundProxy(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req TestOutboundProxyRequest
	if !decodeOutboundProxyRequest(w, r, &req) {
		return
	}
	response, err := h.service.TestOutboundProxy(r.Context(), userID, req)
	if err != nil {
		writeOutboundProxyError(w, "test", err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func decodeOutboundProxyRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxOutboundProxyRequestBytes)
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorProxyInvalidURL)
		return false
	}
	return true
}

func writeOutboundProxyError(w http.ResponseWriter, action string, err error) {
	var requestErr requestError
	if errors.As(err, &requestErr) {
		status := http.StatusBadRequest
		switch requestErr {
		case requestError(ErrorProxyForbidden):
			status = http.StatusForbidden
		case requestError(ErrorProxyUnavailable):
			status = http.StatusServiceUnavailable
		}
		httpjson.WriteError(w, status, requestErr.Error())
		return
	}
	log.Printf("[outbound-proxy] %s failed: %v", action, err)
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorProxyRequest)
}
