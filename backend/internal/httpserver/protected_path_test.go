package httpserver

import (
	"context"
	"net/http/httptest"
	"testing"
)

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
