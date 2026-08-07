package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegisterRoutesDoesNotExposeDeprecatedAuthEndpoints(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewService(nil))

	tests := []struct {
		name string
		path string
		body string
	}{
		{name: "email code", path: "/api/auth/email-code", body: `{"email":"user@example.com"}`},
		{name: "register", path: "/api/auth/register", body: `{"email":"user@example.com","password":"secret","code":"123456"}`},
		{name: "placeholder password login", path: "/api/auth/password", body: `{"account":"user@example.com","password":"secret"}`},
		{name: "placeholder API key login", path: "/api/auth/api-key", body: `{"apiKey":"secret-key"}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, test.path, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")

			mux.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusNotFound {
				t.Fatalf("POST %s status = %d, want %d", test.path, recorder.Code, http.StatusNotFound)
			}
		})
	}
}

func TestRegisterRoutesKeepsEmailPasswordLogin(t *testing.T) {
	mux := http.NewServeMux()
	RegisterRoutes(mux, NewService(nil))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")

	mux.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("POST /api/auth/login status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
}
