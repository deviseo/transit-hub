package client_config

import (
	"errors"
	"log"
	"net/http"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

// RegisterRoutes 注册「客户端」配置接口（作用于当前工作区，需要登录），以及 TransitLink 读取配置的
// 公开接口（/api/client/v1/，不需要登录，只读）。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/client-config", handler.get)
	mux.HandleFunc("PUT /api/client-config", handler.update)
	mux.HandleFunc("POST /api/client-config/reset-code", handler.resetCode)
	mux.HandleFunc("GET /api/client-config/groups", handler.groups)
	mux.HandleFunc("GET "+PublicPathPrefix+"{code}", handler.public)
	mux.HandleFunc("GET "+PublicPathPrefix+"{code}/logo", handler.logo)
}

// RequestError 把可以直接给前端看的错误 key 包装成请求错误（server.go 的适配器用）。
func RequestError(key string) error { return requestError(key) }

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

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	view, err := h.service.Get(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, view)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	// 上传的 logo 是 data URL，请求体放宽到 1 MB。
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req UpdateRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorInvalid)
		return
	}
	view, err := h.service.Update(r.Context(), ws, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, view)
}

func (h *Handler) resetCode(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	view, err := h.service.ResetCode(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, view)
}

func (h *Handler) groups(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	groups, err := h.service.Groups(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	if groups == nil {
		groups = []Group{}
	}
	httpjson.Write(w, http.StatusOK, groups)
}

// public 返回给 TransitLink 的配置。短时间缓存，站长改了配置一两分钟内生效。
func (h *Handler) public(w http.ResponseWriter, r *http.Request) {
	config, err := h.service.Public(r.Context(), r.PathValue("code"))
	if err != nil {
		writePublicError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=60")
	httpjson.Write(w, http.StatusOK, config)
}

func (h *Handler) logo(w http.ResponseWriter, r *http.Request) {
	contentType, data, err := h.service.Logo(r.Context(), r.PathValue("code"))
	if err != nil {
		writePublicError(w, err)
		return
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=300")
	// 只当图片用，不允许在页面里执行任何东西。
	w.Header().Set("Content-Security-Policy", "default-src 'none'; sandbox")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func writeError(w http.ResponseWriter, err error) {
	if !IsRequestError(err) {
		log.Printf("[client_config] internal error err=%v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, ErrorRequest)
		return
	}
	status := http.StatusBadRequest
	// 没选工作区、站点登录失效：409，前端提示去处理（不用 401，免得被当成 TransitHub 登录过期）。
	if err.Error() == ErrorNoCurrentAccount || strings.HasPrefix(err.Error(), "admin.mySites.errors.") {
		status = http.StatusConflict
	}
	httpjson.WriteError(w, status, err.Error())
}

func writePublicError(w http.ResponseWriter, err error) {
	var request requestError
	if errors.As(err, &request) && string(request) == ErrorNotFound {
		httpjson.WriteError(w, http.StatusNotFound, ErrorNotFound)
		return
	}
	log.Printf("[client_config] public error err=%v", err)
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorRequest)
}
