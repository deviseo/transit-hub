package mass_email

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"transithub/backend/internal/shared/authctx"
	"transithub/backend/internal/shared/httpjson"
)

type AdminAccountResolver interface {
	RequireCurrentID(ctx context.Context, userID string) (string, error)
}

type Handler struct {
	service  *Service
	accounts AdminAccountResolver
}

const createBatchBodyLimitBytes = 128 * 1024

func RegisterRoutes(mux *http.ServeMux, service *Service, accounts AdminAccountResolver) {
	handler := &Handler{service: service, accounts: accounts}
	mux.HandleFunc("GET /api/mass-email/users", handler.listUsers)
	mux.HandleFunc("POST /api/mass-email/batches", handler.createBatch)
	mux.HandleFunc("GET /api/mass-email/batches", handler.listBatches)
	mux.HandleFunc("GET /api/mass-email/batches/", handler.getBatch)
	mux.HandleFunc("POST /api/mass-email/batches/", handler.batchAction)
}

func (h *Handler) listUsers(w http.ResponseWriter, r *http.Request) {
	userID, adminAccountID, ok := h.workspace(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	result, err := h.service.ListUsers(r.Context(), userID, adminAccountID, UserQuery{
		Page:      intQuery(query.Get("page"), 1),
		PageSize:  intQuery(firstNonEmpty(query.Get("page_size"), query.Get("pageSize")), 20),
		Status:    query.Get("status"),
		Role:      query.Get("role"),
		Search:    query.Get("search"),
		SortBy:    query.Get("sort_by"),
		SortOrder: query.Get("sort_order"),
		Timezone:  query.Get("timezone"),
	})
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) createBatch(w http.ResponseWriter, r *http.Request) {
	userID, adminAccountID, ok := h.workspace(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, createBatchBodyLimitBytes)
	var req CreateBatchRequest
	if err := httpjson.Decode(r, &req); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "http: request body too large") {
			httpjson.WriteError(w, http.StatusBadRequest, string(ErrInvalidRequest))
			return
		}
		httpjson.WriteError(w, http.StatusBadRequest, string(ErrInvalidRequest))
		return
	}
	batch, err := h.service.CreateBatch(r.Context(), userID, adminAccountID, req)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusAccepted, batch)
}

func (h *Handler) listBatches(w http.ResponseWriter, r *http.Request) {
	userID, adminAccountID, ok := h.workspace(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	result, err := h.service.ListBatches(r.Context(), userID, adminAccountID, intQuery(query.Get("page"), 1), intQuery(firstNonEmpty(query.Get("page_size"), query.Get("pageSize")), 20))
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) getBatch(w http.ResponseWriter, r *http.Request) {
	userID, adminAccountID, ok := h.workspace(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/mass-email/batches/")
	if strings.HasSuffix(path, "/items") {
		batchID := strings.TrimSuffix(path, "/items")
		query := r.URL.Query()
		result, err := h.service.ListItems(r.Context(), userID, adminAccountID, batchID, intQuery(query.Get("page"), 1), intQuery(firstNonEmpty(query.Get("page_size"), query.Get("pageSize")), 50))
		if err != nil {
			writeDomainError(w, err)
			return
		}
		httpjson.Write(w, http.StatusOK, result)
		return
	}
	if strings.Contains(path, "/") || strings.TrimSpace(path) == "" {
		httpjson.WriteError(w, http.StatusNotFound, string(ErrNotFound))
		return
	}
	batch, err := h.service.GetBatch(r.Context(), userID, adminAccountID, path)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, batch)
}

func (h *Handler) batchAction(w http.ResponseWriter, r *http.Request) {
	userID, adminAccountID, ok := h.workspace(w, r)
	if !ok {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/mass-email/batches/")
	if !strings.HasSuffix(path, "/cancel") {
		httpjson.WriteError(w, http.StatusNotFound, string(ErrNotFound))
		return
	}
	batchID := strings.TrimSuffix(path, "/cancel")
	batch, err := h.service.CancelBatch(r.Context(), userID, adminAccountID, batchID)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, batch)
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (string, string, bool) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return "", "", false
	}
	adminAccountID, err := h.accounts.RequireCurrentID(r.Context(), userID)
	if err != nil {
		writeDomainError(w, ErrNoCurrentAccount)
		return "", "", false
	}
	return userID, adminAccountID, true
}

func writeDomainError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrUpstreamAuth):
		status = http.StatusUnauthorized
	case errors.Is(err, ErrActiveBatchExists):
		status = http.StatusConflict
	case errors.Is(err, ErrPersistence):
		status = http.StatusInternalServerError
	}
	message := err.Error()
	if message == "" {
		message = string(ErrInvalidRequest)
	}
	if status == http.StatusInternalServerError {
		log.Printf("[mass-email] internal handler error key=%s", message)
	}
	httpjson.WriteError(w, status, message)
}

func intQuery(value string, fallback int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return fallback
	}
	return parsed
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
