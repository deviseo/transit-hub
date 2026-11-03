package httpserver

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/admin_accounts"
)

type fakeDashboardSessionCleaner struct {
	calls          int
	userID         string
	adminAccountID string
	err            error
}

func (f *fakeDashboardSessionCleaner) Delete(ctx context.Context, userID string, adminAccountID string) error {
	f.calls++
	f.userID = userID
	f.adminAccountID = adminAccountID
	return f.err
}

type fakeUpstreamSiteCleaner struct {
	calls       int
	userID      string
	upstreamIDs []string
	err         error
}

func (f *fakeUpstreamSiteCleaner) CleanupDeletedWorkspaceSites(ctx context.Context, userID string, siteIDs []string) error {
	f.calls++
	f.userID = userID
	f.upstreamIDs = append([]string(nil), siteIDs...)
	return f.err
}

func TestWorkspaceCleanupRunsOnlyLocalCleanup(t *testing.T) {
	dashboard := &fakeDashboardSessionCleaner{}
	upstream := &fakeUpstreamSiteCleaner{}
	cleanup := workspaceCleanup{dashboardSessions: dashboard, upstreamSites: upstream}

	err := cleanup.CleanupDeletedWorkspace(context.Background(), admin_accounts.WorkspaceCleanupPayload{
		UserID:          "user-1",
		AdminAccountID:  "acct-1",
		UpstreamSiteIDs: []string{"site-1", "site-2"},
	})
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if dashboard.calls != 1 || dashboard.userID != "user-1" || dashboard.adminAccountID != "acct-1" {
		t.Fatalf("dashboard cleanup not scoped: %+v", dashboard)
	}
	if upstream.calls != 1 || upstream.userID != "user-1" || len(upstream.upstreamIDs) != 2 {
		t.Fatalf("upstream cleanup not scoped: %+v", upstream)
	}
}

func TestWorkspaceCleanupReturnsErrorsAfterAttemptingAll(t *testing.T) {
	dashboard := &fakeDashboardSessionCleaner{err: errors.New("redis failed")}
	upstream := &fakeUpstreamSiteCleaner{err: errors.New("cache failed")}
	cleanup := workspaceCleanup{dashboardSessions: dashboard, upstreamSites: upstream}

	err := cleanup.CleanupDeletedWorkspace(context.Background(), admin_accounts.WorkspaceCleanupPayload{
		UserID:          "user-1",
		AdminAccountID:  "acct-1",
		UpstreamSiteIDs: []string{"site-1"},
	})
	if err == nil {
		t.Fatal("expected cleanup error")
	}
	if dashboard.calls != 1 || upstream.calls != 1 {
		t.Fatalf("cleanup did not attempt all local cleaners")
	}
	if !errors.Is(err, dashboard.err) || !errors.Is(err, upstream.err) {
		t.Fatalf("cleanup error must include every failure: %v", err)
	}
}
