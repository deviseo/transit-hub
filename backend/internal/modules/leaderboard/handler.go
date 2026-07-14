package leaderboard

import (
	"errors"
	"net/http"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct{ service *Service }

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/leaderboard/data", handler.getAdminData)
	mux.HandleFunc("GET /api/leaderboard/embed-config", handler.getEmbedConfig)
	mux.HandleFunc("PUT /api/leaderboard/embed-config", handler.updateEmbedConfig)
	mux.HandleFunc("POST /api/leaderboard/embed-config", handler.updateEmbedConfig)
	mux.HandleFunc("POST /api/leaderboard/embed-config/rotate-token", handler.rotateEmbedToken)
	mux.HandleFunc("POST /api/embed/leaderboard/session", handler.createEmbedSession)
	mux.HandleFunc("GET /api/embed/leaderboard", handler.getEmbedData)
}

func (h *Handler) getAdminData(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.GetData(r.Context(), userID, queryFromRequest(r))
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) getEmbedConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.GetEmbedConfig(r.Context(), userID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) updateEmbedConfig(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	// Legacy clients may still send sub2apiSourceOrigin or older extra fields, but
	// source binding is now server-derived from the current workspace admin session.
	resp, err := h.service.UpdateEmbedConfig(r.Context(), userID, UpdateEmbedConfigRequest{})
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) rotateEmbedToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	resp, err := h.service.RotateEmbedToken(r.Context(), userID)
	if err != nil {
		writeAdminError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) createEmbedSession(w http.ResponseWriter, r *http.Request) {
	var req CreateSessionRequest
	if err := httpjson.Decode(r, &req); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorEmbedRequest)
		return
	}
	resp, err := h.service.CreateEmbedSession(r.Context(), req)
	if err != nil {
		writeEmbedError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func (h *Handler) getEmbedData(w http.ResponseWriter, r *http.Request) {
	resp, err := h.service.GetEmbedData(r.Context(), embedSessionToken(r), queryFromRequest(r))
	if err != nil {
		writeEmbedError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, resp)
}

func queryFromRequest(r *http.Request) LeaderboardQuery {
	return LeaderboardQuery{StartDate: r.URL.Query().Get("start_date"), EndDate: r.URL.Query().Get("end_date")}
}

func embedSessionToken(r *http.Request) string {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
		return parts[1]
	}
	return ""
}

func writeAdminError(w http.ResponseWriter, err error) {
	var requestErr requestError
	if errors.As(err, &requestErr) {
		switch string(requestErr) {
		case ErrorRequest, ErrorInvalidSourceOrigin:
			httpjson.WriteError(w, http.StatusBadRequest, string(requestErr))
		case ErrorAdminOnly:
			// TransitHub authentication is enforced by httpserver before this
			// handler. A missing Sub2API admin session is a workspace permission
			// state, not an expired TransitHub login.
			httpjson.WriteError(w, http.StatusForbidden, string(requestErr))
		case ErrorUpstreamUnsupported:
			httpjson.WriteError(w, http.StatusBadGateway, string(requestErr))
		default:
			httpjson.WriteError(w, http.StatusBadRequest, string(requestErr))
		}
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorUnknown)
}

func writeEmbedError(w http.ResponseWriter, err error) {
	var requestErr requestError
	if errors.As(err, &requestErr) {
		switch string(requestErr) {
		case ErrorEmbedRequest, ErrorEmbedInvalidSrcHost:
			httpjson.WriteError(w, http.StatusBadRequest, string(requestErr))
		case ErrorEmbedConfigNotFound, ErrorEmbedSrcHostMismatch, ErrorEmbedSourceBinding, ErrorEmbedSub2apiAuth, ErrorEmbedUserMismatch:
			httpjson.WriteError(w, http.StatusForbidden, string(requestErr))
		case ErrorEmbedSessionInvalid, ErrorEmbedAdminSession:
			httpjson.WriteError(w, http.StatusUnauthorized, string(requestErr))
		case ErrorEmbedSub2apiRequest, ErrorEmbedUpstreamRequest, ErrorEmbedUpstreamUnsupported:
			httpjson.WriteError(w, http.StatusBadGateway, string(requestErr))
		default:
			httpjson.WriteError(w, http.StatusBadRequest, string(requestErr))
		}
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorUnknown)
}
