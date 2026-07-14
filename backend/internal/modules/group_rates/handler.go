package group_rates

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service  *Service
	accounts AdminAccountResolver
}

type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

func RegisterRoutes(mux *http.ServeMux, service *Service, accounts AdminAccountResolver) {
	handler := &Handler{service: service, accounts: accounts}
	mux.HandleFunc("GET /api/group-rates", handler.list)
	mux.HandleFunc("GET /api/group-rates/history", handler.history)
	mux.HandleFunc("PATCH /api/group-rates/type", handler.updateType)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	query := r.URL.Query()
	adminAccountID, err := h.currentAdminAccountID(r.Context(), userID)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	rows, err := h.service.List(r.Context(), userID, adminAccountID, ListQuery{
		Page:     intQuery(query.Get("page"), 1),
		PageSize: 30,
		Search:   query.Get("search"),
		Type:     query.Get("type"),
		Platform: query.Get("platform"),
	})
	if err != nil {
		log.Printf("list group rates: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "Failed to list group rates")
		return
	}
	httpjson.Write(w, http.StatusOK, rows)
}

func (h *Handler) updateType(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto UpdateTypeRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	adminAccountID, err := h.currentAdminAccountID(r.Context(), userID)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	if err := h.service.UpdateType(r.Context(), userID, adminAccountID, GroupRef{SiteID: dto.SiteID, GroupName: dto.GroupName}, dto.Type); err != nil {
		log.Printf("update group rate type: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "Failed to update group rate type")
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]bool{"success": true})
}

func intQuery(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed < 1 {
		return fallback
	}
	return parsed
}

func (h *Handler) history(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	query := r.URL.Query()
	siteID := strings.TrimSpace(query.Get("siteId"))
	groupName := strings.TrimSpace(query.Get("groupName"))
	platform := strings.TrimSpace(query.Get("platform"))
	if siteID == "" || groupName == "" || platform == "" {
		httpjson.WriteError(w, http.StatusBadRequest, "siteId, groupName, and platform are required")
		return
	}

	adminAccountID, err := h.currentAdminAccountID(r.Context(), userID)
	if err != nil {
		writeWorkspaceError(w, err)
		return
	}
	rows, err := h.service.History(r.Context(), userID, adminAccountID, siteID, groupName, platform)
	if err != nil {
		log.Printf("list group rate history: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "Failed to list group rate history")
		return
	}
	httpjson.Write(w, http.StatusOK, rows)
}

func (h *Handler) currentAdminAccountID(ctx context.Context, userID string) (string, error) {
	if h.accounts == nil {
		return "", requestError("admin.adminAccounts.errors.noCurrentAccount")
	}
	return h.accounts.RequireCurrentID(ctx, userID)
}

func writeWorkspaceError(w http.ResponseWriter, err error) {
	if err != nil && err.Error() == "admin.adminAccounts.errors.noCurrentAccount" {
		httpjson.WriteError(w, http.StatusConflict, err.Error())
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, "Failed to resolve admin account")
}

type requestError string

func (e requestError) Error() string { return string(e) }
