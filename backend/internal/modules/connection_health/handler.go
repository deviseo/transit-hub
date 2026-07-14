package connection_health

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

// RegisterRoutes 注册链路健康探活模块的全部路由。响应体一律不含 upstream_key。
func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/connection-health/overview", handler.overview)
	mux.HandleFunc("GET /api/connection-health/groups", handler.groups)
	mux.HandleFunc("GET /api/connection-health/admin-groups", handler.adminGroups)
	mux.HandleFunc("GET /api/connection-health/events", handler.events)
	mux.HandleFunc("POST /api/connection-health/connections/{id}/probe", handler.probe)
	mux.HandleFunc("POST /api/connection-health/targets/{id}/probe", handler.probeTarget)
	mux.HandleFunc("POST /api/connection-health/connections/{id}/disable", handler.disable)
	mux.HandleFunc("POST /api/connection-health/connections/{id}/restore", handler.restore)
	mux.HandleFunc("GET /api/connection-health/policies", handler.listPolicies)
	mux.HandleFunc("POST /api/connection-health/policies", handler.createPolicy)
	mux.HandleFunc("PUT /api/connection-health/policies/{id}", handler.updatePolicy)
	mux.HandleFunc("GET /api/connection-health/targets/{id}/models", handler.discoverTargetModels)
	mux.HandleFunc("POST /api/connection-health/targets/{id}/manual-probe", handler.manualProbeTarget)
	mux.HandleFunc("GET /api/connection-health/targets/{id}/policy-assignments", handler.getPolicyAssignments)
	mux.HandleFunc("PUT /api/connection-health/targets/{id}/policy-assignments", handler.putPolicyAssignments)
}

func (h *Handler) overview(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.Overview(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) groups(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	groups, err := h.service.Groups(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	if groups == nil {
		groups = []OwnGroupHealth{}
	}
	httpjson.Write(w, http.StatusOK, groups)
}

// adminGroups 返回当前 admin workspace 下的 admin 全量分组健康主列表（含账号/渠道与探活叠加）。
// 与旧的 /groups 路由并存，不破坏旧调用方。
func (h *Handler) adminGroups(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	groups, err := h.service.AdminGroups(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	if groups == nil {
		groups = []AdminGroupHealth{}
	}
	httpjson.Write(w, http.StatusOK, groups)
}

func (h *Handler) events(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	connectionID := r.URL.Query().Get("connectionId")
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	events, err := h.service.Events(r.Context(), userID, connectionID, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	if events == nil {
		events = []EventView{}
	}
	httpjson.Write(w, http.StatusOK, events)
}

func (h *Handler) probe(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	connectionID := r.PathValue("id")

	// 请求体是可选的：旧调用不带 body（或带空 body）时，Decode 返回 io.EOF，视为
	// "未指定 models"，保持旧行为（探活全部匹配模型），不当作请求错误处理。
	var input ProbeConnectionInput
	if err := httpjson.Decode(r, &input); err != nil && !errors.Is(err, io.EOF) {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}

	results, err := h.service.ProbeConnection(r.Context(), userID, connectionID, input)
	if err != nil {
		writeError(w, err)
		return
	}
	if results == nil {
		results = []ModelHealth{}
	}
	httpjson.Write(w, http.StatusOK, results)
}

// probeTarget 手动探活一个独立 admin 目标：路径参数是 targetId（不是 connectionId）。
// 请求体可选携带 models 限定探活模型。不可探活时返回结构化 i18n 错误 key。
func (h *Handler) probeTarget(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	targetID := r.PathValue("id")

	var input ProbeConnectionInput
	if err := httpjson.Decode(r, &input); err != nil && !errors.Is(err, io.EOF) {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}

	results, err := h.service.ProbeTarget(r.Context(), userID, targetID, input.Models)
	if err != nil {
		writeError(w, err)
		return
	}
	if results == nil {
		results = []ModelHealth{}
	}
	httpjson.Write(w, http.StatusOK, results)
}

func (h *Handler) disable(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	connectionID := r.PathValue("id")
	if err := h.service.DisableConnection(r.Context(), userID, connectionID); err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	connectionID := r.PathValue("id")
	if err := h.service.RestoreConnection(r.Context(), userID, connectionID); err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]bool{"ok": true})
}

func (h *Handler) listPolicies(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	policies, err := h.service.ListPolicies(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	if policies == nil {
		policies = []Policy{}
	}
	httpjson.Write(w, http.StatusOK, policies)
}

func (h *Handler) createPolicy(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req PolicyInput
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	req.ID = ""
	policy, err := h.service.SavePolicy(r.Context(), userID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, policy)
}

func (h *Handler) updatePolicy(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req PolicyInput
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	req.ID = r.PathValue("id")
	policy, err := h.service.SavePolicy(r.Context(), userID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, policy)
}

// discoverTargetModels 是手动一次性探活弹窗打开时调用的 server-only 模型发现接口：
// 后端用当前 admin session 临时解析该 target 的 base_url + key，请求上游 /v1/models，
// 只把安全字段（id/name/ownedBy/providerFamily）返回前端，不回传/落库任何凭据。
func (h *Handler) discoverTargetModels(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	targetID := r.PathValue("id")
	models, err := h.service.DiscoverTargetModels(r.Context(), userID, targetID)
	if err != nil {
		writeError(w, err)
		return
	}
	if models == nil {
		models = []DiscoveredModel{}
	}
	httpjson.Write(w, http.StatusOK, models)
}

// manualProbeTarget 一次性手动探活：不写状态/事件、不消耗策略预算、不触发状态机或远端动作，
// 结果仅用于弹窗内即时展示。models 必须非空。
func (h *Handler) manualProbeTarget(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	targetID := r.PathValue("id")

	var input ProbeConnectionInput
	if err := httpjson.Decode(r, &input); err != nil && !errors.Is(err, io.EOF) {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}

	results, err := h.service.ManualProbeTarget(r.Context(), userID, targetID, input.Models)
	if err != nil {
		writeError(w, err)
		return
	}
	if results == nil {
		results = []ManualProbeResult{}
	}
	httpjson.Write(w, http.StatusOK, results)
}

func (h *Handler) getPolicyAssignments(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	targetID := r.PathValue("id")
	result, err := h.service.GetTargetPolicyAssignments(r.Context(), userID, targetID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

// PolicyAssignmentInput 是策略分配 PUT 接口的请求体：policyIds 为空表示清空该 target 的分配。
type PolicyAssignmentInput struct {
	PolicyIDs []string `json:"policyIds"`
}

func (h *Handler) putPolicyAssignments(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	targetID := r.PathValue("id")

	var input PolicyAssignmentInput
	if err := httpjson.Decode(r, &input); err != nil && !errors.Is(err, io.EOF) {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}

	result, err := h.service.SetTargetPolicyAssignments(r.Context(), userID, targetID, input.PolicyIDs)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func writeError(w http.ResponseWriter, err error) {
	var requestErr requestError
	if errors.As(err, &requestErr) {
		status := http.StatusBadRequest
		if requestErr == requestError(ErrorNotFound) {
			status = http.StatusNotFound
		}
		if requestErr == requestError(ErrorNoCurrentAccount) {
			status = http.StatusConflict
		}
		httpjson.WriteError(w, status, requestErr.Error())
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorUnknown)
}
