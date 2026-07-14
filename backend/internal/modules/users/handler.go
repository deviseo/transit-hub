package users

import (
	"log"
	"net/http"

	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service *Service
}

func RegisterRoutes(mux *http.ServeMux, service *Service) {
	handler := &Handler{service: service}
	mux.HandleFunc("GET /api/users", handler.findAll)
}

func (h *Handler) findAll(w http.ResponseWriter, r *http.Request) {
	users, err := h.service.FindAll(r.Context())
	if err != nil {
		log.Printf("list users: %v", err)
		httpjson.WriteError(w, http.StatusInternalServerError, "Failed to list users")
		return
	}
	httpjson.Write(w, http.StatusOK, users)
}
