package upstream

import (
	"context"
	"testing"
	"time"
)

type staticSiteRepository struct{ sites []Site }

func (r staticSiteRepository) ListSites(ctx context.Context) ([]Site, error) { return r.sites, nil }
func (r staticSiteRepository) ListSitesForUser(ctx context.Context, userID string) ([]Site, error) {
	return nil, nil
}
func (r staticSiteRepository) SaveSite(ctx context.Context, site Site) error { return nil }
func (r staticSiteRepository) DeleteSite(ctx context.Context, userID string, id string) error {
	return nil
}

// 定时同步按工作区配置：开启 A 不会给 B 的站点排定时器，关闭 A 也不会影响 B。
func TestWorkspaceRefreshConfigOnlySchedulesThatWorkspace(t *testing.T) {
	session := &Session{Platform: PlatformSub2API, BaseURL: "https://site.example.test", AccessToken: "token"}
	repo := staticSiteRepository{sites: []Site{
		{ID: "a-1", UserID: "u1", AdminAccountID: "ws-a", Session: session},
		{ID: "a-2", UserID: "u1", AdminAccountID: "ws-a"}, // 没有会话：不调度
		{ID: "b-1", UserID: "u1", AdminAccountID: "ws-b", Session: session},
		{ID: "other-user", UserID: "u2", AdminAccountID: "ws-a", Session: session},
	}}
	service := NewService(NewPlatformService(nil), repo, nil, newFakeSiteCache())
	defer service.Close()
	scheduled := func() map[string]bool {
		service.mu.Lock()
		defer service.mu.Unlock()
		ids := map[string]bool{}
		for id := range service.timers {
			ids[id] = true
		}
		return ids
	}

	service.SetWorkspaceRefreshConfig("u1", "ws-a", RefreshConfig{Enabled: true, Interval: time.Hour})
	if got := scheduled(); len(got) != 1 || !got["a-1"] {
		t.Fatalf("only ws-a sites with a session may be scheduled, got %v", got)
	}
	service.SetWorkspaceRefreshConfig("u1", "ws-b", RefreshConfig{Enabled: true, Interval: 2 * time.Hour})
	if got := scheduled(); len(got) != 2 || !got["a-1"] || !got["b-1"] {
		t.Fatalf("enabling ws-b must keep ws-a scheduled, got %v", got)
	}
	service.SetWorkspaceRefreshConfig("u1", "ws-a", RefreshConfig{Enabled: false, Interval: time.Hour})
	if got := scheduled(); len(got) != 1 || !got["b-1"] {
		t.Fatalf("disabling ws-a must leave ws-b scheduled, got %v", got)
	}

	// 同步完成后的重新调度也按站点自己的工作区取配置。
	service.mu.Lock()
	service.scheduleSyncLocked("a-1", &repo.sites[0])
	service.scheduleSyncLocked("b-1", &repo.sites[2])
	service.mu.Unlock()
	if got := scheduled(); len(got) != 1 || !got["b-1"] {
		t.Fatalf("rescheduling must respect each site's workspace, got %v", got)
	}
}
