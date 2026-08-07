package upstream

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoginAdminWithKeySub2APIUsesXAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/groups" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.Header.Get("x-api-key"); got != "admin-key" {
			t.Fatalf("expected x-api-key, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("admin key must not be sent as Authorization, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session, err := service.LoginAdminWithKey(server.URL, PlatformSub2API, "admin-key", "")
	if err != nil {
		t.Fatalf("LoginAdminWithKey returned error: %v", err)
	}
	if session.AdminAPIKey != "admin-key" || session.AccessToken != "" {
		t.Fatalf("unexpected session: %+v", session)
	}
}

func TestLoginWithUserKeyNewAPIUsesBearerAndUserID(t *testing.T) {
	seenSelf := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/api/status" {
			if got := r.Header.Get("Authorization"); got != "Bearer system-token" {
				t.Fatalf("expected bearer system token for %s, got %q", r.URL.Path, got)
			}
			if got := r.Header.Get("New-Api-User"); got != "42" {
				t.Fatalf("expected New-Api-User=42 for %s, got %q", r.URL.Path, got)
			}
		}
		switch r.URL.Path {
		case "/api/status":
			_, _ = w.Write([]byte(`{"data":{"quota_per_unit":500000}}`))
		case "/api/user/self":
			seenSelf = true
			_, _ = w.Write([]byte(`{"data":{"id":42,"role":1,"quota":500000,"used_quota":100000,"group":"default"}}`))
		case "/api/log/self/stat":
			_, _ = w.Write([]byte(`{"data":{"quota":1000}}`))
		case "/api/user/self/groups":
			_, _ = w.Write([]byte(`{"data":{"default":1}}`))
		case "/api/pricing":
			_, _ = w.Write([]byte(`{"data":[]}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	result, err := service.LoginWithUserKey(server.URL, "42", "system-token")
	if err != nil {
		t.Fatalf("LoginWithUserKey returned error: %v", err)
	}
	if !seenSelf {
		t.Fatal("expected /api/user/self to be requested")
	}
	if !result.Session.IsAuthenticated() || result.Session.UserID != "42" || result.Session.AccessToken != "system-token" {
		t.Fatalf("unexpected session: %+v", result.Session)
	}
}

func TestLoginAdminWithKeyNewAPIRejectsNonAdminRole(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/status" {
			_, _ = w.Write([]byte(`{"data":{"quota_per_unit":500000}}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer root-token" || r.Header.Get("New-Api-User") != "7" {
			t.Fatalf("missing new-api key headers: %+v", r.Header)
		}
		_, _ = w.Write([]byte(`{"data":{"id":7,"role":1}}`))
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	_, err := service.LoginAdminWithKey(server.URL, PlatformNewAPI, "root-token", "7")
	if err == nil || !strings.Contains(err.Error(), ErrorAuth) {
		t.Fatalf("expected admin role rejection, got %v", err)
	}
}

func TestLoginWithUserKeyRejectsSuccessFalseEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/status" {
			_, _ = w.Write([]byte(`{"success":true,"data":{"quota_per_unit":500000}}`))
			return
		}
		_, _ = w.Write([]byte(`{"success":false,"message":"access token invalid"}`))
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	if _, err := service.LoginWithUserKey(server.URL, "42", "invalid-token"); err == nil {
		t.Fatal("expected success=false response to reject the user key")
	}
}

func TestFetchSub2APIAdminUsageStatsUsesAdminAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("x-api-key"); got != "admin-key" {
			t.Fatalf("expected admin key header, got %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("unexpected Authorization header: %q", got)
		}
		if got := r.URL.Query().Get("timezone"); got != "Asia/Shanghai" {
			t.Fatalf("timezone = %q, want Asia/Shanghai", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"total_actual_cost":12.5}}`))
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	value, err := service.FetchSub2APIAdminUsageStats(Session{
		Platform: PlatformSub2API, BaseURL: server.URL, AdminAPIKey: "admin-key",
	}, "2026-07-14", "2026-07-14")
	if err != nil || value != 12.5 {
		t.Fatalf("unexpected result value=%v err=%v", value, err)
	}
}

func TestFetchAdminUsageStatsForScopeUsesAccountAndGroupFilters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/usage/stats" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		for key, want := range map[string]string{
			"account_id": "account-1",
			"group_id":   "group-1",
			"start_date": "2026-08-07",
			"end_date":   "2026-08-07",
			"timezone":   "Asia/Shanghai",
		} {
			if got := r.URL.Query().Get(key); got != want {
				t.Fatalf("%s = %q, want %q", key, got, want)
			}
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q, want bearer token", got)
		}
		writeJSON(w, map[string]any{"data": map[string]any{
			"total_actual_cost":  123.45,
			"total_account_cost": 100.0,
			"total_requests":     7,
		}})
	}))
	defer server.Close()

	stats, err := NewPlatformService(NewHTTPClient(server.Client())).FetchAdminUsageStatsForScope(
		Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "token"},
		"account-1", "group-1", "2026-08-07", "2026-08-07",
	)
	if err != nil {
		t.Fatalf("FetchAdminUsageStatsForScope() error: %v", err)
	}
	if stats.TotalActualCost != 123.45 || stats.TotalAccountCost == nil || *stats.TotalAccountCost != 100 || stats.TotalRequests != 7 {
		t.Fatalf("unexpected scoped stats: %+v", stats)
	}
}

func TestFetchNewAPIAdminUsageStatsUsesShanghaiBusinessDayBounds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/log/self/stat" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("start_timestamp"); got != "1785427200" {
			t.Fatalf("start_timestamp = %q, want Shanghai 2026-07-31 00:00:00", got)
		}
		if got := r.URL.Query().Get("end_timestamp"); got != "1785513599" {
			t.Fatalf("end_timestamp = %q, want Shanghai 2026-07-31 23:59:59", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"quota":250000}}`))
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	value, err := service.FetchAdminUsageStats(Session{
		Platform:     PlatformNewAPI,
		BaseURL:      server.URL,
		Cookie:       "session=abc",
		UserID:       "7",
		QuotaPerUnit: 100000,
	}, "2026-07-31", "2026-07-31")
	if err != nil || value != 2.5 {
		t.Fatalf("unexpected result value=%v err=%v", value, err)
	}
}
