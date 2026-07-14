package lottery

import (
	"context"
	"errors"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

type subscriptionGroupAccountResolver struct{}

func (subscriptionGroupAccountResolver) RequireCurrentID(context.Context, string) (string, error) {
	return "account-1", nil
}

type subscriptionGroupSessionProvider struct {
	session upstream.Session
	err     error
}

func (p subscriptionGroupSessionProvider) RequireSession(context.Context, string, string) (upstream.Session, error) {
	return p.session, p.err
}

type subscriptionGroupFetcher struct {
	groups []upstream.AdminGroupInfo
	err    error
}

func (f subscriptionGroupFetcher) FetchSub2APIAdminAllGroups(upstream.Session) ([]upstream.AdminGroupInfo, error) {
	return f.groups, f.err
}

func TestListSubscriptionGroupsReturnsActiveNumericGroupsWithMultiplier(t *testing.T) {
	firstMultiplier := 1.5
	secondMultiplier := 0.8
	service := &Service{
		accounts:      subscriptionGroupAccountResolver{},
		adminSessions: subscriptionGroupSessionProvider{session: upstream.Session{Platform: upstream.PlatformSub2API, AccessToken: "token"}},
		groupProvider: subscriptionGroupFetcher{groups: []upstream.AdminGroupInfo{
			{ID: "20", Name: "Beta", Status: "active", Multiplier: &firstMultiplier},
			{ID: "10", Name: "Alpha", Status: "active", Multiplier: &secondMultiplier},
			{ID: "30", Name: "Disabled", Status: "inactive", Multiplier: &firstMultiplier},
			{ID: "not-numeric", Name: "Invalid", Status: "active", Multiplier: &firstMultiplier},
			{ID: "40", Name: "No rate", Status: "active"},
		}},
	}

	result, err := service.ListSubscriptionGroups(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("ListSubscriptionGroups returned error: %v", err)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items = %#v, want two selectable groups", result.Items)
	}
	if result.Items[0].ID != "10" || result.Items[0].Name != "Alpha" || result.Items[0].Multiplier != "0.8" {
		t.Fatalf("first item = %#v", result.Items[0])
	}
	if result.Items[1].ID != "20" || result.Items[1].Multiplier != "1.5" {
		t.Fatalf("second item = %#v", result.Items[1])
	}
}

func TestListSubscriptionGroupsMapsSessionAndUpstreamFailures(t *testing.T) {
	service := &Service{
		accounts:      subscriptionGroupAccountResolver{},
		adminSessions: subscriptionGroupSessionProvider{err: errors.New("expired")},
		groupProvider: subscriptionGroupFetcher{},
	}
	if _, err := service.ListSubscriptionGroups(context.Background(), "user-1"); !errors.Is(err, requestError(ErrorRewardAdminSession)) {
		t.Fatalf("session error = %v", err)
	}

	service.adminSessions = subscriptionGroupSessionProvider{session: upstream.Session{Platform: upstream.PlatformSub2API, AccessToken: "token"}}
	service.groupProvider = subscriptionGroupFetcher{err: errors.New("upstream failed")}
	if _, err := service.ListSubscriptionGroups(context.Background(), "user-1"); !errors.Is(err, requestError(ErrorSubscriptionGroups)) {
		t.Fatalf("upstream error = %v", err)
	}
}
