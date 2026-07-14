package my_sites

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"transithub/backend/internal/modules/upstream"
)

type testStateRepo struct {
	state        *State
	saveErr      error
	mutateErr    error
	mutateBefore func(*State)
	saves        []State
}

func (r *testStateRepo) Get(ctx context.Context, userID string, adminAccountID string) (*State, error) {
	if r.state == nil {
		return nil, nil
	}
	copy := cloneState(r.state)
	return copy, nil
}

func (r *testStateRepo) Save(ctx context.Context, state State) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.state = cloneState(&state)
	r.saves = append(r.saves, state)
	return nil
}

func (r *testStateRepo) MutateState(ctx context.Context, userID string, adminAccountID string, mutate StateMutation) (*State, error) {
	if r.mutateErr != nil {
		return nil, r.mutateErr
	}
	if r.state == nil {
		return nil, nil
	}
	latest := cloneState(r.state)
	if r.mutateBefore != nil {
		r.mutateBefore(latest)
	}
	if err := mutate(latest); err != nil {
		return nil, err
	}
	r.state = cloneState(latest)
	r.saves = append(r.saves, *cloneState(latest))
	return cloneState(latest), nil
}

type testConnRepo struct {
	connection   *RealConnection
	stateRepo    *testStateRepo
	deleteErr    error
	deleteCalls  int
	saveCalls    int
	lastSavedIDs []string
}

func (r *testConnRepo) SaveRealConnection(ctx context.Context, conn RealConnection) error {
	r.saveCalls++
	r.connection = &conn
	r.lastSavedIDs = append(r.lastSavedIDs, conn.ID)
	return nil
}

func (r *testConnRepo) ListRealConnections(ctx context.Context, userID string, adminAccountID string) ([]RealConnection, error) {
	if r.connection == nil {
		return nil, nil
	}
	if r.connection.UserID != userID || r.connection.WorkspaceAdminAccountID != adminAccountID {
		return nil, nil
	}
	return []RealConnection{*r.connection}, nil
}

func (r *testConnRepo) GetRealConnection(ctx context.Context, id string, userID string, adminAccountID string) (*RealConnection, error) {
	if r.connection == nil || r.connection.ID != id || r.connection.UserID != userID || r.connection.WorkspaceAdminAccountID != adminAccountID {
		return nil, nil
	}
	conn := *r.connection
	return &conn, nil
}

func (r *testConnRepo) DeleteRealConnection(ctx context.Context, id string, userID string, adminAccountID string) error {
	r.deleteCalls++
	if r.deleteErr != nil {
		return r.deleteErr
	}
	if r.connection != nil && r.connection.ID == id && r.connection.UserID == userID && r.connection.WorkspaceAdminAccountID == adminAccountID {
		r.connection = nil
	}
	return nil
}

func (r *testConnRepo) RemoveUpstreamMappingAndDeleteConnection(ctx context.Context, userID string, adminAccountID string, connectionID string, siteID string, groupName string) error {
	beforeState := cloneState(r.stateRepo.state)
	beforeConn := (*RealConnection)(nil)
	if r.connection != nil {
		conn := *r.connection
		beforeConn = &conn
	}
	if r.stateRepo.state != nil {
		removeMappingTargetFromState(r.stateRepo.state, siteID, groupName)
	}
	if err := r.DeleteRealConnection(ctx, connectionID, userID, adminAccountID); err != nil {
		r.stateRepo.state = beforeState
		r.connection = beforeConn
		return err
	}
	return nil
}

type testAdminResolver struct{ currentID string }

func (r testAdminResolver) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	return r.currentID, nil
}

type testUpstreamLookup struct {
	sites map[string]*upstream.Site
}

func (l testUpstreamLookup) GetSite(ctx context.Context, siteID string) (*upstream.Site, error) {
	if site, ok := l.sites[siteID]; ok {
		return cloneUpstreamSite(site), nil
	}
	return nil, nil
}

func cloneState(state *State) *State {
	if state == nil {
		return nil
	}
	copy := *state
	if state.Mappings != nil {
		copy.Mappings = make([]GroupMapping, len(state.Mappings))
		for i := range state.Mappings {
			copy.Mappings[i] = cloneGroupMapping(state.Mappings[i])
		}
	}
	if state.OwnGroups != nil {
		copy.OwnGroups = append([]GroupOption(nil), state.OwnGroups...)
	}
	return &copy
}

func cloneGroupMapping(mapping GroupMapping) GroupMapping {
	copy := mapping
	if mapping.UpstreamTargets != nil {
		copy.UpstreamTargets = append([]UpstreamGroupRef(nil), mapping.UpstreamTargets...)
	}
	if mapping.AutoPricingNotifyBotIDs != nil {
		copy.AutoPricingNotifyBotIDs = append([]string(nil), mapping.AutoPricingNotifyBotIDs...)
	}
	if mapping.LastAutoPricingRun != nil {
		status := *mapping.LastAutoPricingRun
		copy.LastAutoPricingRun = &status
	}
	return copy
}

func cloneUpstreamSite(site *upstream.Site) *upstream.Site {
	if site == nil {
		return nil
	}
	copy := *site
	if site.Session != nil {
		session := *site.Session
		copy.Session = &session
	}
	if site.Metrics.Groups != nil {
		copy.Metrics.Groups = append([]upstream.GroupInfo(nil), site.Metrics.Groups...)
	}
	return &copy
}

func newNewAPITestServer(t *testing.T, ratios map[string]float64, groupNames []string) (*httptest.Server, *float64) {
	t.Helper()
	var lastUpdated *float64
	mux := http.NewServeMux()
	mux.HandleFunc("/api/user/self", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"role": 10}})
	})
	mux.HandleFunc("/api/group/", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": groupNames})
	})
	mux.HandleFunc("/api/user/self/groups", func(w http.ResponseWriter, r *http.Request) {
		groupRatio := make(map[string]any, len(ratios))
		for name, ratio := range ratios {
			groupRatio[name] = ratio
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"group_ratio": groupRatio}})
	})
	mux.HandleFunc("/api/pricing", func(w http.ResponseWriter, r *http.Request) {
		groupRatio := make(map[string]any, len(ratios))
		for name, ratio := range ratios {
			groupRatio[name] = ratio
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"group_ratio": groupRatio}})
	})
	mux.HandleFunc("/api/option/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			encoded, _ := json.Marshal(ratios)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"GroupRatio": string(encoded)}})
		case http.MethodPut:
			var payload struct {
				Key   string `json:"key"`
				Value string `json:"value"`
			}
			_ = json.NewDecoder(r.Body).Decode(&payload)
			if payload.Key == "GroupRatio" {
				var next map[string]float64
				_ = json.Unmarshal([]byte(payload.Value), &next)
				ratios = next
				if v, ok := next["vip"]; ok {
					lastUpdated = &v
				}
			}
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})
	return httptest.NewServer(mux), lastUpdated
}

func newTestService(t *testing.T, repo *testStateRepo, lookup testUpstreamLookup) (*Service, *httptest.Server, *float64) {
	t.Helper()
	server, lastUpdated := newNewAPITestServer(t, map[string]float64{"vip": 1.1, "low": 1.0, "high": 3.0}, []string{"vip"})
	service := NewService(repo, upstream.NewPlatformService(upstream.NewHTTPClient(server.Client())), lookup)
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})
	return service, server, lastUpdated
}

func testSession(serverURL string) upstream.Session {
	return upstream.Session{Platform: upstream.PlatformNewAPI, BaseURL: serverURL, Cookie: "cookie", UserID: "user-1"}
}

func TestSaveMappingsPreservesLastAutoPricingRun(t *testing.T) {
	repo := &testStateRepo{state: &State{
		UserID:         "user-1",
		AdminAccountID: "admin-1",
		Session:        testSession("http://example.invalid"),
		Mappings: []GroupMapping{{
			OwnGroup:           "VIP",
			LastAutoPricingRun: &AutoPricingRunStatus{Status: "applied", Trigger: "manual", RanAt: time.Unix(100, 0)},
		}},
	}}
	server, _ := newNewAPITestServer(t, map[string]float64{"VIP": 1.1}, []string{"VIP"})
	defer server.Close()
	repo.state.Session = testSession(server.URL)
	service := NewService(repo, upstream.NewPlatformService(upstream.NewHTTPClient(server.Client())), testUpstreamLookup{})
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})

	response, err := service.SaveMappings(context.Background(), "user-1", []MappingRequest{{OwnGroup: " vip "}})
	if err != nil {
		t.Fatalf("SaveMappings returned error: %v", err)
	}
	if len(response.Mappings) != 1 || response.Mappings[0].LastAutoPricingRun == nil {
		t.Fatalf("expected preserved lastAutoPricingRun in response, got %#v", response.Mappings)
	}
	if response.Mappings[0].LastAutoPricingRun.Status != "applied" || response.Mappings[0].LastAutoPricingRun.Trigger != "manual" {
		t.Fatalf("unexpected preserved status: %#v", response.Mappings[0].LastAutoPricingRun)
	}
}

func TestSaveMappingsMergesLockedLatestStatusAndOwnGroups(t *testing.T) {
	repo := &testStateRepo{state: &State{
		UserID:         "user-1",
		AdminAccountID: "admin-1",
		Session:        testSession(""),
		Mappings:       []GroupMapping{{OwnGroup: "vip"}},
		OwnGroups:      []GroupOption{{Name: "vip", Multiplier: 1.1}},
	}}
	server, _ := newNewAPITestServer(t, map[string]float64{"vip": 1.1}, []string{"vip"})
	defer server.Close()
	repo.state.Session = testSession(server.URL)
	repo.mutateBefore = func(latest *State) {
		latest.OwnGroups = []GroupOption{{Name: "vip", Multiplier: 9}}
		latest.Mappings[0].LastAutoPricingRun = &AutoPricingRunStatus{Status: "applied", Trigger: "after_sync", RanAt: time.Unix(200, 0)}
	}
	service := NewService(repo, upstream.NewPlatformService(upstream.NewHTTPClient(server.Client())), testUpstreamLookup{})
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})

	_, err := service.SaveMappings(context.Background(), "user-1", []MappingRequest{{OwnGroup: "vip"}})
	if err != nil {
		t.Fatalf("SaveMappings returned error: %v", err)
	}
	if repo.state.Mappings[0].LastAutoPricingRun == nil || repo.state.Mappings[0].LastAutoPricingRun.Trigger != "after_sync" {
		t.Fatalf("expected latest status to survive SaveMappings, got %#v", repo.state.Mappings[0].LastAutoPricingRun)
	}
	if len(repo.state.OwnGroups) != 1 || repo.state.OwnGroups[0].Multiplier != 9 {
		t.Fatalf("expected latest ownGroups to survive SaveMappings, got %#v", repo.state.OwnGroups)
	}
}

func TestPersistAutoPricingRunStatusPreservesLockedLatestMapping(t *testing.T) {
	repo := &testStateRepo{state: &State{
		UserID:         "user-1",
		AdminAccountID: "admin-1",
		Mappings:       []GroupMapping{{OwnGroup: "vip", UpstreamTargets: []UpstreamGroupRef{{SiteID: "old", GroupName: "g"}}}},
	}}
	repo.mutateBefore = func(latest *State) {
		latest.Mappings[0].UpstreamTargets = append(latest.Mappings[0].UpstreamTargets, UpstreamGroupRef{SiteID: "new", GroupName: "h"})
	}
	service := &Service{repository: repo}

	updated, err := service.persistAutoPricingRunStatus(context.Background(), "user-1", "admin-1", autoPricingResult{OwnGroup: "vip", Status: "applied", NewReference: 0, NewReferenceSet: true, TargetMultiplier: 0, TargetSet: true}, "manual", pointerFloat64(0))
	if err != nil {
		t.Fatalf("persistAutoPricingRunStatus returned error: %v", err)
	}
	if len(updated.UpstreamTargets) != 2 {
		t.Fatalf("expected latest mapping targets to survive status persistence, got %#v", updated.UpstreamTargets)
	}
	if updated.LastAutoPricingRun == nil || updated.LastAutoPricingRun.NewReference == nil || *updated.LastAutoPricingRun.NewReference != 0 {
		t.Fatalf("expected zero newReference to persist, got %#v", updated.LastAutoPricingRun)
	}
}

func TestAutoPricingStatusFromResultPreservesZeroValues(t *testing.T) {
	status := autoPricingStatusFromResult(autoPricingResult{
		OwnGroup:         "zero",
		Status:           "applied",
		OldReference:     0,
		OldReferenceSet:  true,
		NewReference:     0,
		NewReferenceSet:  true,
		TargetMultiplier: 0,
		TargetSet:        true,
	}, "manual", time.Unix(300, 0))
	if status.OldReference == nil || *status.OldReference != 0 {
		t.Fatalf("expected zero oldReference pointer, got %#v", status.OldReference)
	}
	if status.NewReference == nil || *status.NewReference != 0 {
		t.Fatalf("expected zero newReference pointer, got %#v", status.NewReference)
	}
	if status.TargetMultiplier == nil || *status.TargetMultiplier != 0 {
		t.Fatalf("expected zero target pointer, got %#v", status.TargetMultiplier)
	}
}

func TestRunAutoPricingNowCalculatesCurrentReference(t *testing.T) {
	repo := &testStateRepo{state: &State{
		UserID:         "user-1",
		AdminAccountID: "admin-1",
		Session:        testSession(""),
		Mappings: []GroupMapping{{
			OwnGroup:               "vip",
			EnableAutoPricing:      true,
			AutoPricingSource:      "average_upstream",
			AutoPricingStrategy:    "fixed",
			FixedIncrease:          0.5,
			AdjustThresholdPercent: 10,
			UpstreamTargets:        []UpstreamGroupRef{{SiteID: "site-a", GroupName: "a"}, {SiteID: "site-b", GroupName: "b"}},
		}},
		OwnGroups: []GroupOption{{Name: "vip", Multiplier: 1.1}},
	}}
	server, _ := newNewAPITestServer(t, map[string]float64{"vip": 1.1, "low": 1.0, "high": 3.0}, []string{"vip"})
	defer server.Close()
	repo.state.Session = testSession(server.URL)
	lookup := testUpstreamLookup{sites: map[string]*upstream.Site{
		"site-a": {ID: "site-a", UserID: "user-1", AdminAccountID: "admin-1", Status: upstream.StatusConnected, LastSyncedAt: int64Ptr(1), Metrics: upstream.Metrics{Groups: []upstream.GroupInfo{{Name: "a", Multiplier: floatPtr(1.0)}}}},
		"site-b": {ID: "site-b", UserID: "user-1", AdminAccountID: "admin-1", Status: upstream.StatusConnected, LastSyncedAt: int64Ptr(1), Metrics: upstream.Metrics{Groups: []upstream.GroupInfo{{Name: "b", Multiplier: floatPtr(3.0)}}}},
	}}
	service := NewService(repo, upstream.NewPlatformService(upstream.NewHTTPClient(server.Client())), lookup)
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})

	response, err := service.RunAutoPricingNow(context.Background(), "user-1", AutoPricingRunRequest{OwnGroup: "vip"})
	if err != nil {
		t.Fatalf("RunAutoPricingNow returned error: %v", err)
	}
	if response.Result.Status != "applied" || response.Result.Trigger != "manual" {
		t.Fatalf("unexpected result: %#v", response.Result)
	}
	if response.Result.OldReference != nil {
		t.Fatalf("expected manual oldReference to be nil, got %#v", response.Result.OldReference)
	}
	if response.Result.NewReference == nil || *response.Result.NewReference != 2.0 {
		t.Fatalf("expected current reference 2.0, got %#v", response.Result.NewReference)
	}
	if response.Result.OldOwnMultiplier == nil || *response.Result.OldOwnMultiplier != 1.1 {
		t.Fatalf("expected old own multiplier 1.1, got %#v", response.Result.OldOwnMultiplier)
	}
	if response.Result.NewOwnMultiplier == nil || *response.Result.NewOwnMultiplier != 2.5 {
		t.Fatalf("expected new own multiplier 2.5, got %#v", response.Result.NewOwnMultiplier)
	}
	if response.Result.TargetMultiplier == nil || *response.Result.TargetMultiplier != 2.5 {
		t.Fatalf("expected target 2.5, got %#v", response.Result.TargetMultiplier)
	}
	if response.Mapping.LastAutoPricingRun == nil || response.Mapping.LastAutoPricingRun.Status != "applied" {
		t.Fatalf("expected persisted status, got %#v", response.Mapping.LastAutoPricingRun)
	}
	if repo.state == nil || len(repo.state.OwnGroups) != 1 || repo.state.OwnGroups[0].Multiplier != 2.5 {
		t.Fatalf("expected own group multiplier to persist to 2.5, got %#v", repo.state.OwnGroups)
	}
}

func TestApplyAutoPricingAfterSyncPersistsStatus(t *testing.T) {
	repo := &testStateRepo{state: &State{
		UserID:         "user-1",
		AdminAccountID: "admin-1",
		Session:        testSession(""),
		Mappings: []GroupMapping{{
			OwnGroup:                 "vip",
			UpstreamTargets:          []UpstreamGroupRef{{SiteID: "sync-site", GroupName: "default"}},
			EnableAutoPricing:        true,
			AutoPricingSource:        "primary_upstream",
			PrimaryUpstreamSiteID:    "sync-site",
			PrimaryUpstreamGroupName: "default",
			AutoPricingStrategy:      "percentage",
			PercentageIncrease:       10,
			AdjustThresholdPercent:   50,
		}},
		OwnGroups: []GroupOption{{Name: "vip", Multiplier: 1.1}},
	}}
	server, _ := newNewAPITestServer(t, map[string]float64{"vip": 1.1}, []string{"vip"})
	defer server.Close()
	repo.state.Session = testSession(server.URL)
	lookup := testUpstreamLookup{sites: map[string]*upstream.Site{
		"sync-site": {ID: "sync-site", UserID: "user-1", AdminAccountID: "admin-1", Status: upstream.StatusConnected, LastSyncedAt: int64Ptr(1), Metrics: upstream.Metrics{Groups: []upstream.GroupInfo{{Name: "default", Multiplier: floatPtr(1.2)}}}},
	}}
	service := NewService(repo, upstream.NewPlatformService(upstream.NewHTTPClient(server.Client())), lookup)
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})

	service.ApplyAutoPricingAfterSync(context.Background(), "user-1", "admin-1", "sync-site", "sync-site-name", upstream.Metrics{Groups: []upstream.GroupInfo{{ID: "default", Name: "default", Multiplier: floatPtr(1.0)}}}, upstream.Metrics{Groups: []upstream.GroupInfo{{ID: "default", Name: "default", Multiplier: floatPtr(1.2)}}})

	if repo.state == nil || len(repo.state.Mappings) != 1 || repo.state.Mappings[0].LastAutoPricingRun == nil {
		t.Fatalf("expected persisted after-sync status, got %#v", repo.state)
	}
	status := repo.state.Mappings[0].LastAutoPricingRun
	if status.Status != "applied" || status.Trigger != "after_sync" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.OldReference == nil || *status.OldReference != 1.0 {
		t.Fatalf("expected source oldReference 1.0, got %#v", status.OldReference)
	}
	if status.NewReference == nil || *status.NewReference != 1.2 {
		t.Fatalf("expected source newReference 1.2, got %#v", status.NewReference)
	}
	if status.OldOwnMultiplier == nil || *status.OldOwnMultiplier != 1.1 {
		t.Fatalf("expected old own multiplier 1.1, got %#v", status.OldOwnMultiplier)
	}
	if status.NewOwnMultiplier == nil || *status.NewOwnMultiplier != 1.32 {
		t.Fatalf("expected new own multiplier 1.32, got %#v", status.NewOwnMultiplier)
	}
	if status.TargetMultiplier == nil || *status.TargetMultiplier != 1.32 {
		t.Fatalf("expected target 1.32, got %#v", status.TargetMultiplier)
	}
	if repo.state.OwnGroups[0].Multiplier != 1.32 {
		t.Fatalf("expected own group multiplier persisted to 1.32, got %#v", repo.state.OwnGroups[0].Multiplier)
	}
}

func TestRealDisconnectAtomicRollbackOnDeleteFailure(t *testing.T) {
	repo := &testStateRepo{state: &State{
		UserID:         "user-1",
		AdminAccountID: "admin-1",
		Session:        testSession("http://example.invalid"),
		Mappings:       []GroupMapping{{OwnGroup: "vip", UpstreamTargets: []UpstreamGroupRef{{SiteID: "site-a", GroupName: "g1"}}}},
	}}
	connRepo := &testConnRepo{stateRepo: repo, connection: &RealConnection{ID: "conn-1", UserID: "user-1", WorkspaceAdminAccountID: "admin-1", UpstreamSiteID: "site-a", UpstreamGroupName: "g1"}, deleteErr: errors.New("delete failed")}
	service := &Service{repository: repo, connRepository: connRepo}
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})

	err := service.RealDisconnect(context.Background(), "user-1", RealDisconnectRequest{ConnectionID: "conn-1", Mode: "unlink"})
	if err == nil {
		t.Fatal("expected delete error")
	}
	if connRepo.connection == nil {
		t.Fatal("expected connection rollback to preserve row")
	}
	if len(repo.state.Mappings) != 1 || len(repo.state.Mappings[0].UpstreamTargets) != 1 {
		t.Fatalf("expected mapping rollback to preserve target, got %#v", repo.state.Mappings)
	}
}

func TestMappingOptionsPrunesOnlyAuthoritativeMissingTargets(t *testing.T) {
	repo := &testStateRepo{state: &State{
		UserID:         "user-1",
		AdminAccountID: "admin-1",
		Session:        testSession(""),
		Mappings: []GroupMapping{{
			OwnGroup: "vip",
			UpstreamTargets: []UpstreamGroupRef{
				{SiteID: "site-ok", GroupName: "missing"},
				{SiteID: "site-offline", GroupName: "missing-offline"},
			},
		}},
	}}
	server, _ := newNewAPITestServer(t, map[string]float64{"vip": 1.1}, []string{"vip"})
	defer server.Close()
	repo.state.Session = testSession(server.URL)
	lookup := testUpstreamLookup{sites: map[string]*upstream.Site{
		"site-ok":      {ID: "site-ok", UserID: "user-1", AdminAccountID: "admin-1", Status: upstream.StatusConnected, LastSyncedAt: int64Ptr(1), Metrics: upstream.Metrics{Groups: []upstream.GroupInfo{{Name: "keep", Multiplier: floatPtr(1.0)}}}},
		"site-offline": {ID: "site-offline", UserID: "user-1", AdminAccountID: "admin-1", Status: upstream.StatusError, LastSyncedAt: int64Ptr(1), Metrics: upstream.Metrics{Groups: []upstream.GroupInfo{{Name: "keep-offline", Multiplier: floatPtr(1.0)}}}},
	}}
	service := NewService(repo, upstream.NewPlatformService(upstream.NewHTTPClient(server.Client())), lookup)
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})

	response, err := service.MappingOptions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("MappingOptions returned error: %v", err)
	}
	if len(response.Mappings) != 1 {
		t.Fatalf("expected one mapping, got %#v", response.Mappings)
	}
	targets := response.Mappings[0].UpstreamTargets
	if len(targets) != 1 || targets[0].SiteID != "site-offline" {
		t.Fatalf("expected only offline target to remain, got %#v", targets)
	}
}

func TestMappingOptionsDoesNotResurrectDisconnectedBackfillTarget(t *testing.T) {
	repo := &testStateRepo{state: &State{
		UserID:         "user-1",
		AdminAccountID: "admin-1",
		Session:        testSession(""),
		Mappings: []GroupMapping{{
			OwnGroup:        "vip",
			UpstreamTargets: []UpstreamGroupRef{{SiteID: "site-a", GroupName: "g1"}},
		}},
	}}
	connRepo := &testConnRepo{
		stateRepo:  repo,
		connection: &RealConnection{ID: "conn-1", UserID: "user-1", WorkspaceAdminAccountID: "admin-1", UpstreamSiteID: "site-a", UpstreamGroupName: "g1", OwnGroupIDs: []string{"1"}},
	}
	server, _ := newNewAPITestServer(t, map[string]float64{"vip": 1.1}, []string{"vip"})
	defer server.Close()
	repo.state.Session = testSession(server.URL)
	repo.mutateBefore = func(latest *State) {
		removeMappingTargetFromState(latest, "site-a", "g1")
		connRepo.connection = nil
	}
	service := NewService(repo, upstream.NewPlatformService(upstream.NewHTTPClient(server.Client())), testUpstreamLookup{})
	service.SetAdminAccountResolver(testAdminResolver{currentID: "admin-1"})
	service.connRepository = connRepo

	response, err := service.MappingOptions(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("MappingOptions returned error: %v", err)
	}
	if len(response.Mappings) != 0 {
		t.Fatalf("expected disconnected target not to be resurrected, got %#v", response.Mappings)
	}
	if connRepo.deleteCalls != 0 {
		t.Fatalf("test should simulate completed disconnect without invoking delete, got %d", connRepo.deleteCalls)
	}
}

func int64Ptr(v int64) *int64 { return &v }
