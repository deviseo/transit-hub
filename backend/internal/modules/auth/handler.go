package auth

import (
	"net/http"

	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("POST /api/auth/login", handler.login)
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var dto LoginRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	response, err := h.service.Login(r.Context(), dto)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func writeAuthError(w http.ResponseWriter, err error) {
	if authErr, ok := err.(*RequestError); ok {
		httpjson.WriteError(w, authErr.Status, authErr.Message)
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, "auth.errors.unknown")
}
