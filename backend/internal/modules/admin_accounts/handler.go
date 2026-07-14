package admin_accounts

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
	mux.HandleFunc("GET /api/admin-accounts", handler.list)
	mux.HandleFunc("GET /api/admin-accounts/current", handler.current)
	mux.HandleFunc("POST /api/admin-accounts/current", handler.switchCurrent)
	mux.HandleFunc("DELETE /api/admin-accounts/{id}", handler.delete)
	mux.HandleFunc("PUT /api/admin-accounts/", handler.update)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	accounts, err := h.service.List(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, accounts)
}

func (h *Handler) current(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	account, err := h.service.Current(r.Context(), userID)
	if err != nil {
		writeError(w, err)
		return
	}
	if account == nil {
		httpjson.WriteError(w, http.StatusConflict, ErrorNoCurrentAccount)
		return
	}
	httpjson.Write(w, http.StatusOK, account)
}

func (h *Handler) switchCurrent(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	var dto struct {
		ID string `json:"id"`
	}
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	account, err := h.service.Switch(r.Context(), userID, dto.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, account)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/admin-accounts/")
	var dto UpdateRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	account, err := h.service.Update(r.Context(), userID, id, dto)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, account)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	var dto DeleteRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, ErrorRequest)
		return
	}
	response, err := h.service.DeleteWorkspace(r.Context(), userID, id, dto)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
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
	httpjson.WriteError(w, http.StatusInternalServerError, ErrorRequest)
}
