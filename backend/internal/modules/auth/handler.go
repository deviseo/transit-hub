package auth

import (
	"net/http"

	"transithub/backend/internal/shared/httpjson"
)

type Handler struct {
	service             *Service
	allowPublicRegister bool
}

func RegisterRoutes(mux *http.ServeMux, service *Service, allowPublicRegister bool) {
	handler := &Handler{service: service, allowPublicRegister: allowPublicRegister}
	mux.HandleFunc("POST /api/auth/email-code", handler.requestEmailCode)
	mux.HandleFunc("POST /api/auth/register", handler.register)
	mux.HandleFunc("POST /api/auth/login", handler.login)
	mux.HandleFunc("POST /api/auth/password", handler.loginWithPassword)
	mux.HandleFunc("POST /api/auth/api-key", handler.loginWithAPIKey)
}

func (h *Handler) requestEmailCode(w http.ResponseWriter, r *http.Request) {
	// ALLOW_PUBLIC_REGISTER=false 时禁用验证码接口
	if !h.allowPublicRegister {
		httpjson.WriteError(w, http.StatusForbidden, "auth.errors.registrationDisabled")
		return
	}
	var dto EmailCodeRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	response, err := h.service.RequestEmailCode(r.Context(), dto)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	httpjson.Write(w, http.StatusOK, response)
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	// ALLOW_PUBLIC_REGISTER=false 时禁用注册接口
	if !h.allowPublicRegister {
		httpjson.WriteError(w, http.StatusForbidden, "auth.errors.registrationDisabled")
		return
	}
	var dto RegisterRequest
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	response, err := h.service.Register(r.Context(), dto)
	if err != nil {
		writeAuthError(w, err)
		return
	}
	httpjson.Write(w, http.StatusCreated, response)
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

func (h *Handler) loginWithPassword(w http.ResponseWriter, r *http.Request) {
	var dto PasswordLogin
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	response, ok := h.service.LoginWithPassword(dto)
	if !ok {
		httpjson.WriteError(w, http.StatusBadRequest, "account and password are required")
		return
	}
	httpjson.Write(w, http.StatusCreated, response)
}

func (h *Handler) loginWithAPIKey(w http.ResponseWriter, r *http.Request) {
	var dto APIKeyLogin
	if err := httpjson.Decode(r, &dto); err != nil {
		httpjson.WriteError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	response, ok := h.service.LoginWithAPIKey(dto)
	if !ok {
		httpjson.WriteError(w, http.StatusBadRequest, "apiKey is required")
		return
	}
	httpjson.Write(w, http.StatusCreated, response)
}

func writeAuthError(w http.ResponseWriter, err error) {
	if authErr, ok := err.(*RequestError); ok {
		httpjson.WriteError(w, authErr.Status, authErr.Message)
		return
	}
	httpjson.WriteError(w, http.StatusInternalServerError, "auth.errors.unknown")
}
