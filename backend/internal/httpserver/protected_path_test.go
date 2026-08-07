package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"transithub/backend/internal/config"
)

func TestProtectedPathIncludesUsersPrefix(t *testing.T) {
	server := &Server{}
	for _, path := range []string{"/api/users", "/api/users/user-1"} {
		if !server.protectedPath(path) {
			t.Fatalf("expected %s to be protected", path)
		}
	}
}

func TestCORSWithoutConfiguredOriginsDoesNotAuthorizeCrossOriginRequest(t *testing.T) {
	server := &Server{cfg: config.Config{}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.Header.Set("Origin", "https://review.invalid")

	server.cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("unconfigured CORS must not authorize origin, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("unconfigured CORS must not allow credentials, got %q", got)
	}
}

func TestCORSWithoutOriginDoesNotEmitWildcardCredentials(t *testing.T) {
	server := &Server{cfg: config.Config{}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)

	server.cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("same-origin request must not emit wildcard CORS, got %q", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("same-origin request must not emit CORS credentials, got %q", got)
	}
}

func TestCORSExplicitlyConfiguredOriginIsAuthorized(t *testing.T) {
	origin := "https://app.example.com"
	cfg := config.Config{CORSOrigins: []string{origin}}
	server := &Server{cfg: cfg, allowed: makeAllowedOrigins(cfg.CORSOrigins)}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	request.Header.Set("Origin", origin)

	server.cors(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).ServeHTTP(recorder, request)

	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Fatalf("configured origin = %q, want %q", got, origin)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("configured origin credentials = %q, want true", got)
	}
}

func TestProtectedPathIncludesMassEmailPrefix(t *testing.T) {
	server := &Server{}
	for _, path := range []string{"/api/mass-email", "/api/mass-email/users", "/api/mass-email/batches/batch-1/items"} {
		if !server.protectedPath(path) {
			t.Fatalf("expected %s to be protected", path)
		}
	}
}

func TestProtectedPathDoesNotOvermatchMassEmailLookalikes(t *testing.T) {
	server := &Server{}
	if server.protectedPath("/api/public-mass-email") {
		t.Fatalf("unexpected protected match for unrelated mass-email lookalike")
	}
}

func TestProtectedPathIncludesLeaderboardAdminPrefix(t *testing.T) {
	server := &Server{}
	for _, path := range []string{"/api/leaderboard/data", "/api/leaderboard/embed-config", "/api/leaderboard/embed-config/rotate-token"} {
		if !server.protectedPath(path) {
			t.Fatalf("expected %s to be protected", path)
		}
	}
	if server.protectedPath("/api/embed/leaderboard") {
		t.Fatalf("embed leaderboard API must remain public at global middleware")
	}
}

func TestSecurityHeadersForLeaderboardEmbedStaticRoute(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/embed/leaderboard?embed_token=embed-token&src_host=https://evil.example.com", nil)
	server := &Server{leaderboardFrameAncestorOrigin: func(ctx context.Context, embedToken string) (string, bool) {
		if embedToken != "embed-token" {
			return "", false
		}
		return "https://src.example.com", true
	}}
	server.setSecurityHeaders(recorder, request)
	if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
	if got := recorder.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("expected no-referrer policy, got %q", got)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got != "frame-ancestors https://src.example.com" {
		t.Fatalf("expected route-specific frame policy, got %q", got)
	}
}

func TestSecurityHeadersForLeaderboardEmbedInvalidTokenDenyFrames(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("GET", "/embed/leaderboard?embed_token=missing", nil)
	server := &Server{leaderboardFrameAncestorOrigin: func(ctx context.Context, embedToken string) (string, bool) {
		return "", false
	}}
	server.setSecurityHeaders(recorder, request)
	if got := recorder.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("expected no-referrer policy, got %q", got)
	}
	if got := recorder.Header().Get("Content-Security-Policy"); got != "frame-ancestors 'none'" {
		t.Fatalf("expected frame denial for invalid token, got %q", got)
	}
}
