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

type fakeAttachmentCleaner struct {
	paths []string
	err   error
}

func (f *fakeAttachmentCleaner) Delete(storagePath string) error {
	f.paths = append(f.paths, storagePath)
	return f.err
}

type fakeTicketEmbedSessionCleaner struct {
	calls          int
	userID         string
	adminAccountID string
	err            error
}

type fakeLeaderboardEmbedSessionCleaner struct {
	calls          int
	userID         string
	adminAccountID string
	err            error
}

type fakeLotteryEmbedSessionCleaner struct {
	calls          int
	userID         string
	adminAccountID string
	err            error
}

func (f *fakeLeaderboardEmbedSessionCleaner) DeleteWorkspace(ctx context.Context, userID string, adminAccountID string) error {
	f.calls++
	f.userID = userID
	f.adminAccountID = adminAccountID
	return f.err
}

func (f *fakeLotteryEmbedSessionCleaner) DeleteWorkspace(ctx context.Context, userID string, adminAccountID string) error {
	f.calls++
	f.userID = userID
	f.adminAccountID = adminAccountID
	return f.err
}

func (f *fakeTicketEmbedSessionCleaner) DeleteWorkspace(ctx context.Context, userID string, adminAccountID string) error {
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
	ticketSessions := &fakeTicketEmbedSessionCleaner{}
	leaderboardSessions := &fakeLeaderboardEmbedSessionCleaner{}
	lotterySessions := &fakeLotteryEmbedSessionCleaner{}
	attachments := &fakeAttachmentCleaner{}
	upstream := &fakeUpstreamSiteCleaner{}
	cleanup := workspaceCleanup{dashboardSessions: dashboard, ticketSessions: ticketSessions, leaderboardSessions: leaderboardSessions, lotterySessions: lotterySessions, attachments: attachments, upstreamSites: upstream}

	err := cleanup.CleanupDeletedWorkspace(context.Background(), admin_accounts.WorkspaceCleanupPayload{
		UserID:                 "user-1",
		AdminAccountID:         "acct-1",
		AttachmentStoragePaths: []string{"a.png", "", "b.png"},
		UpstreamSiteIDs:        []string{"site-1", "site-2"},
	})
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if dashboard.calls != 1 || dashboard.userID != "user-1" || dashboard.adminAccountID != "acct-1" {
		t.Fatalf("dashboard cleanup not scoped: %+v", dashboard)
	}
	if ticketSessions.calls != 1 || ticketSessions.userID != "user-1" || ticketSessions.adminAccountID != "acct-1" {
		t.Fatalf("ticket embed session cleanup not scoped: %+v", ticketSessions)
	}
	if leaderboardSessions.calls != 1 || leaderboardSessions.userID != "user-1" || leaderboardSessions.adminAccountID != "acct-1" {
		t.Fatalf("leaderboard embed session cleanup not scoped: %+v", leaderboardSessions)
	}
	if lotterySessions.calls != 1 || lotterySessions.userID != "user-1" || lotterySessions.adminAccountID != "acct-1" {
		t.Fatalf("lottery embed session cleanup not scoped: %+v", lotterySessions)
	}
	if upstream.calls != 1 || upstream.userID != "user-1" || len(upstream.upstreamIDs) != 2 {
		t.Fatalf("upstream cleanup not scoped: %+v", upstream)
	}
	if len(attachments.paths) != 2 || attachments.paths[0] != "a.png" || attachments.paths[1] != "b.png" {
		t.Fatalf("attachment cleanup paths = %+v", attachments.paths)
	}
}

func TestWorkspaceCleanupReturnsErrorsAfterAttemptingAll(t *testing.T) {
	dashboard := &fakeDashboardSessionCleaner{err: errors.New("redis failed")}
	ticketSessions := &fakeTicketEmbedSessionCleaner{err: errors.New("ticket sessions failed")}
	leaderboardSessions := &fakeLeaderboardEmbedSessionCleaner{err: errors.New("leaderboard sessions failed")}
	lotterySessions := &fakeLotteryEmbedSessionCleaner{err: errors.New("lottery sessions failed")}
	attachments := &fakeAttachmentCleaner{err: errors.New("file failed")}
	upstream := &fakeUpstreamSiteCleaner{err: errors.New("cache failed")}
	cleanup := workspaceCleanup{dashboardSessions: dashboard, ticketSessions: ticketSessions, leaderboardSessions: leaderboardSessions, lotterySessions: lotterySessions, attachments: attachments, upstreamSites: upstream}

	err := cleanup.CleanupDeletedWorkspace(context.Background(), admin_accounts.WorkspaceCleanupPayload{
		UserID:                 "user-1",
		AdminAccountID:         "acct-1",
		AttachmentStoragePaths: []string{"a.png"},
		UpstreamSiteIDs:        []string{"site-1"},
	})
	if err == nil {
		t.Fatal("expected cleanup error")
	}
	if dashboard.calls != 1 || ticketSessions.calls != 1 || leaderboardSessions.calls != 1 || lotterySessions.calls != 1 || upstream.calls != 1 || len(attachments.paths) != 1 {
		t.Fatalf("cleanup did not attempt all local cleaners")
	}
	if !errors.Is(err, lotterySessions.err) {
		t.Fatalf("cleanup error did not include lottery session failure: %v", err)
	}
}
