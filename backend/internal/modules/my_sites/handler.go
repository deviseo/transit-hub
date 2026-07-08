package my_sites

import (
	"errors"
	"net/http"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

// RegisterRoutes 注册分组映射相关的路由。
// 包含映射选项查询、映射关系保存、真实对接创建和记录查询。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/my-sites/mapping-options", handler.mappingOptions)
	mux.HandleFunc("PUT /api/my-sites/mappings", handler.saveMappings)
	mux.HandleFunc("POST /api/my-sites/real-connect", handler.realConnect)
	mux.HandleFunc("POST /api/my-sites/real-bind", handler.realBind)
	mux.HandleFunc("GET /api/my-sites/upstream-keys", handler.listUpstreamKeys)
	mux.HandleFunc("GET /api/my-sites/real-connections", handler.listRealConnections)
	mux.HandleFunc("POST /api/my-sites/real-disconnect", handler.realDisconnect)
}

func (h *Handler) mappingOptions(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	response, err := h.service.MappingOptions(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) saveMappings(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto struct {
		Mappings []MappingRequest `json:"mappings"`
	}
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	response, err := h.service.SaveMappings(r.Context(), userID, dto.Mappings)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// realConnect 真实对接：在上游站点创建 key，在 admin 站点创建转发账号。
func (h *Handler) realConnect(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req RealConnectRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	response, err := h.service.RealConnect(r.Context(), userID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// listUpstreamKeys 获取指定上游站点的 API Key 列表，供手动绑定时选择。
func (h *Handler) listUpstreamKeys(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	siteID := r.URL.Query().Get("siteId")
	if siteID == "" {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	keys, err := h.service.ListUpstreamKeys(r.Context(), userID, siteID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, keys)
}

// realBind 手动绑定已有的上游 Key，仅创建绑定记录。
func (h *Handler) realBind(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req RealBindRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	response, err := h.service.RealBind(r.Context(), userID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

// listRealConnections 查询当前用户的所有真实对接绑定记录。
func (h *Handler) listRealConnections(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	connections, err := h.service.ListRealConnections(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	if connections == nil {
		connections = []RealConnection{}
	}
	httpjson.Write(w, http.StatusOK, connections)
}

// realDisconnect 取消真实对接：删除记录，可选同时删除上游 key 和 admin 账号。
func (h *Handler) realDisconnect(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req RealDisconnectRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	if err := h.service.RealDisconnect(r.Context(), userID, req); err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]bool{"ok": true})
}

func writeError(w http.ResponseWriter, err error) {
	var requestErr requestError
	if errors.As(err, &requestErr) {
		status := http.StatusBadRequest
		if requestErr == requestError(ErrorAuthRequired) {
			status = http.StatusUnauthorized
		}
		if requestErr == requestError(ErrorAdminOnly) {
			status = http.StatusForbidden
		}
		if requestErr == requestError("admin.adminAccounts.errors.noCurrentAccount") {
			status = http.StatusConflict
		}
		httpjson.WriteError(w, status, requestErr.Error())
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorUnknown)
}
