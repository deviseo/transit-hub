package leads

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"transithub/backend/internal/modules/mass_email"
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

const (
	smallBodyLimit  = 64 * 1024
	importBodyLimit = 2 << 20
)

func RegisterRoutes(mux *http.ServeMux, service *Service, accounts AdminAccountResolver) {
	handler := &Handler{service: service, accounts: accounts}
	mux.HandleFunc("GET /api/leads", handler.list)
	mux.HandleFunc("POST /api/leads", handler.add)
	mux.HandleFunc("GET /api/leads/sources", handler.sources)
	mux.HandleFunc("POST /api/leads/import", handler.importLeads)
	mux.HandleFunc("POST /api/leads/delete", handler.delete)
	mux.HandleFunc("PATCH /api/leads/{id}", handler.update)
	mux.HandleFunc("GET /api/leads/settings", handler.settings)
	mux.HandleFunc("PUT /api/leads/settings", handler.saveSettings)
	mux.HandleFunc("POST /api/leads/send/preview", handler.preview)
	mux.HandleFunc("POST /api/leads/send", handler.send)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	query := r.URL.Query()
	result, err := h.service.List(r.Context(), ws, ListQuery{
		Page:     intQuery(query.Get("page"), 1),
		PageSize: intQuery(firstNonEmpty(query.Get("page_size"), query.Get("pageSize")), defaultPageSize),
		Source:   query.Get("source"),
		Status:   query.Get("status"),
		Search:   query.Get("search"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) sources(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	sources, err := h.service.Sources(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, map[string]any{"items": sources})
}

func (h *Handler) add(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req AddRequest
	if !decode(w, r, smallBodyLimit, &req) {
		return
	}
	lead, err := h.service.Add(r.Context(), ws, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, lead)
}

func (h *Handler) importLeads(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req ImportRequest
	if !decode(w, r, importBodyLimit, &req) {
		return
	}
	result, err := h.service.Import(r.Context(), ws, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req UpdateRequest
	if !decode(w, r, smallBodyLimit, &req) {
		return
	}
	lead, err := h.service.Update(r.Context(), ws, r.PathValue("id"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, lead)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req DeleteRequest
	if !decode(w, r, importBodyLimit, &req) {
		return
	}
	result, err := h.service.Delete(r.Context(), ws, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) settings(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	settings, err := h.service.Settings(r.Context(), ws)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, settings)
}

func (h *Handler) saveSettings(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req SettingsRequest
	if !decode(w, r, smallBodyLimit, &req) {
		return
	}
	settings, err := h.service.SaveSettings(r.Context(), ws, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, settings)
}

func (h *Handler) preview(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req Selection
	if !decode(w, r, importBodyLimit, &req) {
		return
	}
	result, err := h.service.Preview(r.Context(), ws, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, result)
}

func (h *Handler) send(w http.ResponseWriter, r *http.Request) {
	ws, ok := h.workspace(w, r)
	if !ok {
		return
	}
	var req SendRequest
	if !decode(w, r, importBodyLimit, &req) {
		return
	}
	batch, err := h.service.Send(r.Context(), ws, req)
	if err != nil {
		writeError(w, err)
		return
	}
	httpjson.Write(w, http.StatusAccepted, batch)
}

func (h *Handler) workspace(w http.ResponseWriter, r *http.Request) (Workspace, bool) {
	userID, ok := authctx.UserID(r.Context())
	if !ok {
		httpjson.WriteError(w, http.StatusUnauthorized, "auth.errors.unauthorized")
		return Workspace{}, false
	}
	adminAccountID, err := h.accounts.RequireCurrentID(r.Context(), userID)
	if err != nil {
		writeError(w, ErrNoCurrentAccount)
		return Workspace{}, false
	}
	return Workspace{UserID: userID, AdminAccountID: adminAccountID}, true
}

func decode(w http.ResponseWriter, r *http.Request, limit int64, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	if err := httpjson.Decode(r, target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			httpjson.WriteError(w, http.StatusRequestEntityTooLarge, string(ErrTooMany))
			return false
		}
		httpjson.WriteError(w, http.StatusBadRequest, string(ErrInvalidRequest))
		return false
	}
	return true
}

// writeError 把业务错误写成 { message: <i18n key> }。群发模块的错误（模板不存在、SMTP 没配好、
// 已有批次在发送等）原样透传，前端用群发页面已有的文案。
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	switch {
	case errors.Is(err, ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrDuplicate), errors.Is(err, mass_email.ErrActiveBatchExists):
		status = http.StatusConflict
	case errors.Is(err, ErrTooMany):
		status = http.StatusRequestEntityTooLarge
	case errors.Is(err, ErrPersistence), errors.Is(err, mass_email.ErrPersistence):
		status = http.StatusInternalServerError
	}
	message := err.Error()
	if !strings.HasPrefix(message, "admin.") {
		// 不是业务错误 key（不该出现）：不把内部错误文本发给前端。
		log.Printf("[leads] unexpected error: %v", err)
		status, message = http.StatusInternalServerError, string(ErrPersistence)
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
