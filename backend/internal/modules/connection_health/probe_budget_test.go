package connection_health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

func TestProbeOnce_StopsRealProbingAfterDailyBudgetExhausted(t *testing.T) {
	hits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	sites := fakeSiteLookup{site: &upstream.Site{ID: "site-1", BaseURL: server.URL, Platform: upstream.PlatformNewAPI}}
	svc := &Service{repo: repo, sites: sites, dispatcher: noopRemoteActionRunner{}, probeRunner: NewRealProbeRunner()}

	conn := my_sites.RealConnection{ID: "conn-1", UpstreamSiteID: "site-1", UpstreamKey: "key-1", UserID: "user1", WorkspaceAdminAccountID: "ws1"}
	policy := Policy{UserID: "user1", AdminAccountID: "ws1", DailyProbeBudget: 1, RecoveryStepPercent: 25, FailureThreshold: 3, SuccessThreshold: 2, CooldownSeconds: 300, ObservationSeconds: 300, AutoDegradeEnabled: true}
	target := ModelTarget{ModelName: "gpt-4o-mini", ProviderFamily: ProviderOpenAI, MaxProbeTokens: 1}

	if _, err := svc.probeOnce(context.Background(), conn, policy, target); err != nil {
		t.Fatalf("unexpected error on first probe: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected 1 real request after first probe, got %d", hits)
	}

	if _, err := svc.probeOnce(context.Background(), conn, policy, target); err != nil {
		t.Fatalf("unexpected error on second probe: %v", err)
	}
	if hits != 1 {
		t.Fatalf("expected daily budget to block the second real probe request, got %d hits", hits)
	}
}
