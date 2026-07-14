package lottery

import (
	"errors"
	"net/http"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct{ service *Service }

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	h := &Handler{service: service}
	mux.HandleFunc("GET /api/lottery/embed-config", h.getEmbedConfig)
	mux.HandleFunc("GET /api/lottery/subscription-groups", h.listSubscriptionGroups)
	mux.HandleFunc("POST /api/lottery/embed-config/rotate-token", h.rotateToken)
	mux.HandleFunc("GET /api/lottery/campaigns", h.listCampaigns)
	mux.HandleFunc("POST /api/lottery/campaigns", h.createCampaign)
	mux.HandleFunc("GET /api/lottery/campaigns/{id}", h.getCampaign)
	mux.HandleFunc("PUT /api/lottery/campaigns/{id}", h.updateCampaign)
	mux.HandleFunc("POST /api/lottery/campaigns/{id}/publish", h.publishCampaign)
	mux.HandleFunc("POST /api/lottery/campaigns/{id}/close", h.closeCampaign)
	mux.HandleFunc("POST /api/lottery/campaigns/{id}/draw", h.drawCampaign)
	mux.HandleFunc("POST /api/lottery/campaigns/{id}/cancel", h.cancelCampaign)
	mux.HandleFunc("GET /api/lottery/campaigns/{id}/entries", h.listEntries)
	mux.HandleFunc("GET /api/lottery/campaigns/{id}/audit", h.audit)
	mux.HandleFunc("POST /api/lottery/reward-jobs/{id}/retry", h.retryReward)
	mux.HandleFunc("POST /api/lottery/reward-jobs/{id}/complete", h.completeManualReward)

	mux.HandleFunc("POST /api/embed/lottery/session", h.createEmbedSession)
	mux.HandleFunc("GET /api/embed/lottery/campaigns", h.listEmbedCampaigns)
	mux.HandleFunc("GET /api/embed/lottery/campaigns/{id}", h.getEmbedCampaign)
	mux.HandleFunc("POST /api/embed/lottery/campaigns/{id}/entries", h.enterCampaign)
	mux.HandleFunc("DELETE /api/embed/lottery/campaigns/{id}/entries", h.withdrawEntry)
}

func (h *Handler) getEmbedConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.GetEmbedConfig(r.Context(), userID)
	writeResult(w, resp, err)
}
func (h *Handler) listSubscriptionGroups(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.ListSubscriptionGroups(r.Context(), userID)
	writeResult(w, resp, err)
}
func (h *Handler) rotateToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.RotateEmbedToken(r.Context(), userID)
	writeResult(w, resp, err)
}
func (h *Handler) listCampaigns(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.ListCampaigns(r.Context(), userID)
	writeResult(w, resp, err)
}
func (h *Handler) getCampaign(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.GetCampaign(r.Context(), userID, r.PathValue("id"))
	writeResult(w, resp, err)
}

func (h *Handler) createCampaign(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req CreateCampaignRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	resp, err := h.service.CreateCampaign(r.Context(), userID, req)
	writeResult(w, resp, err)
}

func (h *Handler) updateCampaign(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var req UpdateCampaignRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	resp, err := h.service.UpdateCampaign(r.Context(), userID, r.PathValue("id"), req)
	writeResult(w, resp, err)
}

func (h *Handler) publishCampaign(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.PublishCampaign(r.Context(), userID, r.PathValue("id"))
	writeResult(w, resp, err)
}
func (h *Handler) closeCampaign(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.CloseCampaign(r.Context(), userID, r.PathValue("id"))
	writeResult(w, resp, err)
}
func (h *Handler) drawCampaign(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.DrawCampaign(r.Context(), userID, r.PathValue("id"))
	writeResult(w, resp, err)
}
func (h *Handler) cancelCampaign(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.CancelCampaign(r.Context(), userID, r.PathValue("id"))
	writeResult(w, resp, err)
}
func (h *Handler) listEntries(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.ListEntries(r.Context(), userID, r.PathValue("id"))
	writeResult(w, resp, err)
}
func (h *Handler) audit(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.ListAudit(r.Context(), userID, r.PathValue("id"))
	writeResult(w, resp, err)
}
func (h *Handler) retryReward(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	err := h.service.RetryReward(r.Context(), userID, r.PathValue("id"))
	writeResult(w, map[string]bool{"ok": true}, err)
}
func (h *Handler) completeManualReward(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	err := h.service.CompleteManualReward(r.Context(), userID, r.PathValue("id"))
	writeResult(w, map[string]bool{"ok": true}, err)
}

func (h *Handler) createEmbedSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorEmbedRequest)
		return
	}
	resp, err := h.service.CreateEmbedSession(r.Context(), req)
	writeResult(w, resp, err)
}
func (h *Handler) listEmbedCampaigns(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.ListEmbedCampaigns(r.Context(), bearerToken(r.Header.Get("Authorization")))
	writeResult(w, resp, err)
}
func (h *Handler) getEmbedCampaign(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetEmbedCampaign(r.Context(), bearerToken(r.Header.Get("Authorization")), r.PathValue("id"))
	writeResult(w, resp, err)
}
func (h *Handler) enterCampaign(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.EnterCampaign(r.Context(), bearerToken(r.Header.Get("Authorization")), r.PathValue("id"))
	writeResult(w, resp, err)
}
func (h *Handler) withdrawEntry(w http.ResponseWriter, r *http.Request) {
	err := h.service.WithdrawEntry(r.Context(), bearerToken(r.Header.Get("Authorization")), r.PathValue("id"))
	writeResult(w, map[string]bool{"ok": true}, err)
}

func writeResult(w http.ResponseWriter, payload any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, payload)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	message := ErrorUnknown
	var reqErr requestError
	if errors.As(err, &reqErr) {
		message = reqErr.Error()
		switch reqErr {
		case requestError(ErrorNotFound), requestError(ErrorEmbedEntryNotFound):
			status = http.StatusNotFound
		case requestError(ErrorInvalidState), requestError(ErrorAlreadyEntered):
			status = http.StatusConflict
		case requestError(ErrorNoCurrentAccount):
			status = http.StatusConflict
		case requestError(ErrorAdminOnly), requestError(ErrorEmbedAdminSession), requestError(ErrorEmbedSourceBinding), requestError(ErrorInvalidSourceOrigin), requestError(ErrorEmbedSrcHostMismatch):
			status = http.StatusForbidden
		case requestError(ErrorEmbedSessionInvalid), requestError(ErrorEmbedSub2apiAuth):
			status = http.StatusUnauthorized
		default:
			status = http.StatusBadRequest
		}
	}
	httpjson.WriteError(w, status, message)
}

func bearerToken(header string) string {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return parts[1]
}
