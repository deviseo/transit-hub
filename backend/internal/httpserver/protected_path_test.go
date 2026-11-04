package httpserver

import (
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

func TestProtectedPathCoversKeptAdminAPIs(t *testing.T) {
	server := &Server{}
	for _, path := range []string{
		"/api/admin-accounts", "/api/upstream-sites/site-1", "/api/group-rates", "/api/group-rates/history",
		"/api/settings/strategy", "/api/dashboard/metrics", "/api/system/proxy", "/api/user-activity/summary",
		"/api/sales/summary", "/api/reports/daily/preview", "/api/profit/summary",
		"/api/client-config", "/api/client-config/groups",
		"/api/tickets", "/api/tickets/counts", "/api/tickets/t1/messages", "/api/tickets/attachments/a1",
	} {
		if !server.protectedPath(path) {
			t.Fatalf("expected %s to be protected", path)
		}
	}
	// 公开接口：健康检查、登录、邮件退订页，以及 TransitLink 读取客户端配置和工单接口（工单用自己的会话鉴权）
	// 不经过 TransitHub 登录校验。
	for _, path := range []string{"/api/health", "/api/auth/login", "/api/email-unsubscribe", "/api/client/v1/sites/abcdefghij",
		"/api/client/v1/sites/abcdefghij/logo", "/api/client/v1/sites/abcdefghij/tickets/session", "/api/client/v1/tickets",
		"/api/client/v1/tickets/t1/messages"} {
		if server.protectedPath(path) {
			t.Fatalf("expected %s to stay public", path)
		}
	}
}

func TestSecurityHeadersAreSetForEveryRequest(t *testing.T) {
	for _, path := range []string{"/", "/admin", "/embed/leaderboard"} {
		recorder := httptest.NewRecorder()
		server := &Server{}
		server.setSecurityHeaders(recorder, httptest.NewRequest("GET", path, nil))
		if got := recorder.Header().Get("X-Content-Type-Options"); got != "nosniff" {
			t.Fatalf("%s: expected nosniff header, got %q", path, got)
		}
		if got := recorder.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
			t.Fatalf("%s: unexpected referrer policy %q", path, got)
		}
		if got := recorder.Header().Get("Content-Security-Policy"); got != "" {
			t.Fatalf("%s: removed embed pages must not get a frame policy, got %q", path, got)
		}
	}
}
