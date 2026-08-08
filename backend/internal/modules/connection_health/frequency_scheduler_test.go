package connection_health

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

type countingFrequencyReader struct {
	groups        []upstream.AdminGroupInfo
	accounts      map[string][]upstream.AdminGroupAccountInfo
	accountsErr   map[string]error
	fetchErr      error
	fetchCalls    int
	accountsCalls int
	resolveCalls  int
	blockFetch    bool
	fetchStarted  chan struct{}
}

func (r *countingFrequencyReader) FetchAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	r.fetchCalls++
	if r.fetchErr != nil {
		return nil, r.fetchErr
	}
	return r.groups, nil
}

func (r *countingFrequencyReader) ListAdminGroupAccounts(session upstream.Session, group upstream.AdminGroupInfo) ([]upstream.AdminGroupAccountInfo, error) {
	r.accountsCalls++
	if err := r.accountsErr[group.ID]; err != nil {
		return nil, err
	}
	return r.accounts[group.ID], nil
}

func (r *countingFrequencyReader) FetchAdminAllGroupsContext(ctx context.Context, session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	if !r.blockFetch {
		return r.FetchAdminAllGroups(session)
	}
	r.fetchCalls++
	if r.fetchStarted != nil {
		select {
		case r.fetchStarted <- struct{}{}:
		default:
		}
	}
	<-ctx.Done()
	return nil, ctx.Err()
}

func (r *countingFrequencyReader) ListAdminGroupAccountsContext(ctx context.Context, session upstream.Session, group upstream.AdminGroupInfo) ([]upstream.AdminGroupAccountInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.ListAdminGroupAccounts(session, group)
}

func (r *countingFrequencyReader) ResolveProbeCredential(session upstream.Session, account upstream.AdminGroupAccountInfo) (upstream.ProbeCredential, error) {
	r.resolveCalls++
	return upstream.ProbeCredential{}, errors.New("not used")
}

func (r *countingFrequencyReader) ResolveProbeCredentialContext(ctx context.Context, session upstream.Session, account upstream.AdminGroupAccountInfo) (upstream.ProbeCredential, error) {
	if err := ctx.Err(); err != nil {
		return upstream.ProbeCredential{}, err
	}
	return r.ResolveProbeCredential(session, account)
}

type invalidateSnapshotOnListPoliciesRepository struct {
	*fakeRepository
	onList sync.Once
	mutate func()
}

func (r *invalidateSnapshotOnListPoliciesRepository) ListPolicies(ctx context.Context, userID string, adminAccountID string) ([]Policy, error) {
	policies, err := r.fakeRepository.ListPolicies(ctx, userID, adminAccountID)
	r.onList.Do(r.mutate)
	return policies, err
}

type invalidateSnapshotOnListSettingsRepository struct {
	*fakeRepository
	onList sync.Once
	mutate func()
}

func (r *invalidateSnapshotOnListSettingsRepository) ListGroupProbeSortSettings(ctx context.Context, userID string, adminAccountID string) ([]GroupProbeSortSetting, error) {
	settings, err := r.fakeRepository.ListGroupProbeSortSettings(ctx, userID, adminAccountID)
	r.onList.Do(r.mutate)
	return settings, err
}

type invalidateSnapshotOnTargetActionUpsertRepository struct {
	*fakeRepository
	onUpsert sync.Once
	mutate   func()
}

func (r *invalidateSnapshotOnTargetActionUpsertRepository) UpsertTargetActionState(ctx context.Context, state TargetActionState) error {
	r.onUpsert.Do(r.mutate)
	return r.fakeRepository.UpsertTargetActionState(ctx, state)
}

type contextAwarePriorityWorkspaceRepository struct {
	*fakeRepository
}

func (r *contextAwarePriorityWorkspaceRepository) UpsertPriorityWorkspaceSyncState(ctx context.Context, state PriorityWorkspaceSyncState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.fakeRepository.UpsertPriorityWorkspaceSyncState(ctx, state)
}

type countingFrequencyKeyReader struct {
	fakeAdminGroupKeyReader
	keyCalls int
}

func (r *countingFrequencyKeyReader) ListUpstreamKeys(ctx context.Context, userID string, siteID string) ([]upstream.Sub2APIKeyItem, error) {
	r.keyCalls++
	return r.fakeAdminGroupKeyReader.ListUpstreamKeys(ctx, userID, siteID)
}

type countingTargetActionRunner struct {
	calls int
}

func (r *countingTargetActionRunner) Degrade(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (string, error) {
	return "", nil
}

func (r *countingTargetActionRunner) Restore(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (string, error) {
	return "", nil
}

func (r *countingTargetActionRunner) DegradeTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (string, error) {
	return "", nil
}

func (r *countingTargetActionRunner) RestoreTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (string, error) {
	return "", nil
}

func (r *countingTargetActionRunner) ApplyTargetState(ctx context.Context, session upstream.Session, target AdminProbeTarget, weight *int, status string) (string, error) {
	r.calls++
	return "applied", nil
}

func TestFrequencyScheduler_ReconcileProbeEvaluationAndWritebackAreIndependent(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 8, 0, 0, 0, time.UTC)
	policy := priorityGatePolicy()
	policy.ProbeIntervalSeconds = 60
	policy.PrioritySyncPreset.ReconcileIntervalSeconds = 30
	policy.PrioritySyncPreset.InventorySnapshotTTLSeconds = 60
	policy.PrioritySyncPreset.ReconcileFailureBackoffSeconds = 30
	policy.UpdatedAt = now
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: policy.ID,
	}}
	fallback := 0.4
	repo.groupSortSettings["user1|ws1|g1"] = GroupProbeSortSetting{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", FallbackMultiplier: &fallback,
	}
	currentPriority := 99
	reader := &countingFrequencyReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: func() *float64 { value := 0.4; return &value }()}},
		accounts: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "a", Name: "a", Priority: &currentPriority, Models: "gpt-4o"}},
		},
	}
	actions := &gatePriorityActioner{}
	service := &Service{
		repo:            repo,
		mySites:         fakeAdminGroupKeyReader{fakeMySitesReader: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, keysBySite: map[string][]upstream.Sub2APIKeyItem{}},
		sites:           fakeSiteLookup{site: &upstream.Site{}},
		platformGroups:  reader,
		priorityActions: actions,
		now:             func() time.Time { return now },
	}

	service.runPriorityReconcileTick(context.Background())
	if reader.fetchCalls != 1 || reader.accountsCalls != 1 {
		t.Fatalf("initial reconcile must perform one inventory cycle: fetch=%d accounts=%d", reader.fetchCalls, reader.accountsCalls)
	}
	if len(actions.calls) != 0 {
		t.Fatalf("reconcile must not write priority: %+v", actions.calls)
	}
	state := priorityWorkspaceState(t, repo)
	if state.PendingSignature == "" || state.ReconcileSuccessCount != 1 || state.InventoryStatus != "ready" {
		t.Fatalf("reconcile must refresh the snapshot and queue local ordering: %+v", state)
	}

	service.evaluateCurrentWorkspacePriorities(context.Background(), "user1", "ws1", priorityActionProbe)
	if reader.fetchCalls != 1 || len(actions.calls) != 0 {
		t.Fatalf("probe evaluation must reuse the snapshot without a read or write: reads=%d writes=%+v", reader.fetchCalls, actions.calls)
	}
	state = priorityWorkspaceState(t, repo)
	if state.ProbeEvaluationCount != 1 || state.LastActionSource != priorityActionProbe {
		t.Fatalf("probe evaluation source must be observable: %+v", state)
	}

	service.runPriorityWritebackTick(context.Background())
	if reader.fetchCalls != 1 || len(actions.calls) != 1 {
		t.Fatalf("writeback must use the existing snapshot and perform only the queued write: reads=%d writes=%+v", reader.fetchCalls, actions.calls)
	}
	state = priorityWorkspaceState(t, repo)
	if state.LastActionSource != priorityActionWriteback || state.PendingSignature != "" || state.WriteSuccessCount != 1 {
		t.Fatalf("writeback must converge the pending signature independently: %+v", state)
	}
}

func TestFrequencyScheduler_ReconcileFailureIsUnknownAndHonorsBackoff(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 9, 0, 0, 0, time.UTC)
	policy := priorityGatePolicy()
	policy.PrioritySyncPreset.ReconcileIntervalSeconds = 10
	policy.PrioritySyncPreset.InventorySnapshotTTLSeconds = 20
	policy.PrioritySyncPreset.ReconcileFailureBackoffSeconds = 7
	policy.UpdatedAt = now
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID,
	}}
	priority := 50
	reader := &countingFrequencyReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accounts: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "a", Name: "a", Priority: &priority, Models: "gpt-4o"}},
		},
	}
	service := &Service{
		repo:            repo,
		mySites:         fakeAdminGroupKeyReader{fakeMySitesReader: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, keysBySite: map[string][]upstream.Sub2APIKeyItem{}},
		sites:           fakeSiteLookup{site: &upstream.Site{}},
		platformGroups:  reader,
		priorityActions: &gatePriorityActioner{},
		now:             func() time.Time { return now },
	}
	service.runPriorityReconcileTick(context.Background())
	state := priorityWorkspaceState(t, repo)
	state.AppliedSignature = "confirmed"
	state.PendingSignature = "still-pending"
	pendingSince := now.Add(-time.Minute)
	state.PendingSince = &pendingSince
	repo.UpsertPriorityWorkspaceSyncState(context.Background(), state)

	reader.fetchErr = errors.New("upstream unavailable")
	now = now.Add(10 * time.Second)
	service.runPriorityReconcileTick(context.Background())
	failed := priorityWorkspaceState(t, repo)
	if failed.InventoryStatus != "unknown" || failed.LastInventoryError != "inventory_read_failed" || failed.ReconcileFailureCount != 1 {
		t.Fatalf("failed read must be observable as unknown: %+v", failed)
	}
	if failed.AppliedSignature != "confirmed" || failed.PendingSignature != "still-pending" {
		t.Fatalf("failed read must preserve applied and pending state: %+v", failed)
	}
	readsAfterFailure := reader.fetchCalls
	now = now.Add(6 * time.Second)
	service.runPriorityReconcileTick(context.Background())
	if reader.fetchCalls != readsAfterFailure {
		t.Fatalf("reconcile must honor configured failure backoff: before=%d after=%d", readsAfterFailure, reader.fetchCalls)
	}
	now = now.Add(time.Second)
	service.runPriorityReconcileTick(context.Background())
	if reader.fetchCalls != readsAfterFailure+1 {
		t.Fatalf("reconcile must retry when the configured backoff expires: before=%d after=%d", readsAfterFailure, reader.fetchCalls)
	}
}

func TestFrequencyScheduler_PolicyVersionChangeAppliesNewReconcileInterval(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 10, 0, 0, 0, time.UTC)
	policy := priorityGatePolicy()
	policy.PrioritySyncPreset.ReconcileIntervalSeconds = 30
	policy.PrioritySyncPreset.InventorySnapshotTTLSeconds = 60
	policy.PrioritySyncPreset.ReconcileFailureBackoffSeconds = 30
	policy.UpdatedAt = now
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID}}
	priority := 50
	reader := &countingFrequencyReader{
		groups:   []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accounts: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "a", Priority: &priority, Models: "gpt-4o"}}},
	}
	service := &Service{
		repo:            repo,
		mySites:         fakeAdminGroupKeyReader{fakeMySitesReader: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, keysBySite: map[string][]upstream.Sub2APIKeyItem{}},
		sites:           fakeSiteLookup{site: &upstream.Site{}},
		platformGroups:  reader,
		priorityActions: &gatePriorityActioner{},
		now:             func() time.Time { return now },
	}
	service.runPriorityReconcileTick(context.Background())
	before := priorityWorkspaceState(t, repo)

	repo.policies[0].PrioritySyncPreset.ReconcileIntervalSeconds = 5
	repo.policies[0].UpdatedAt = now.Add(time.Second)
	now = now.Add(time.Second)
	service.runPriorityReconcileTick(context.Background())
	after := priorityWorkspaceState(t, repo)
	if reader.fetchCalls != 2 || after.ReconcileIntervalSeconds != 5 || after.PolicyVersion == before.PolicyVersion {
		t.Fatalf("policy version change must apply the new reconcile cadence immediately: reads=%d before=%+v after=%+v", reader.fetchCalls, before, after)
	}
}

func TestFrequencyScheduler_PartialInventoryFailureUsesBackoff(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 11, 0, 0, 0, time.UTC)
	policy := priorityGatePolicy()
	policy.PrioritySyncPreset.ReconcileIntervalSeconds = 5
	policy.PrioritySyncPreset.ReconcileFailureBackoffSeconds = 17
	policy.UpdatedAt = now
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID}}
	priority := 50
	reader := &countingFrequencyReader{
		groups:      []upstream.AdminGroupInfo{{ID: "g1", Name: "broken"}, {ID: "g2", Name: "healthy"}},
		accounts:    map[string][]upstream.AdminGroupAccountInfo{"g2": {{ID: "a", Priority: &priority, Models: "gpt-4o"}}},
		accountsErr: map[string]error{"g1": errors.New("group accounts unavailable")},
	}
	service := &Service{
		repo:    repo,
		mySites: fakeAdminGroupKeyReader{fakeMySitesReader: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, keysBySite: map[string][]upstream.Sub2APIKeyItem{}},
		sites:   fakeSiteLookup{site: &upstream.Site{}}, platformGroups: reader,
		priorityActions: &gatePriorityActioner{}, now: func() time.Time { return now },
	}
	pendingSince := now.Add(-time.Minute)
	repo.UpsertPriorityWorkspaceSyncState(context.Background(), PriorityWorkspaceSyncState{
		UserID: "user1", AdminAccountID: "ws1", AppliedSignature: "confirmed",
		PendingSignature: "still-pending", PendingSince: &pendingSince,
	})

	service.runPriorityReconcileTick(context.Background())
	failed := priorityWorkspaceState(t, repo)
	if failed.InventoryStatus != "unknown" || failed.LastInventoryError != "inventory_read_failed" || failed.ReconcileFailureCount != 1 {
		t.Fatalf("partial read must be observable as failed/unknown: %+v", failed)
	}
	if failed.NextReconcileAt == nil || !failed.NextReconcileAt.Equal(now.Add(17*time.Second)) {
		t.Fatalf("partial read must use configured backoff: %+v", failed.NextReconcileAt)
	}
	if failed.AppliedSignature != "confirmed" || failed.PendingSignature != "still-pending" {
		t.Fatalf("partial read must preserve applied and pending state: %+v", failed)
	}
	reads := reader.fetchCalls
	now = now.Add(16 * time.Second)
	service.runPriorityReconcileTick(context.Background())
	if reader.fetchCalls != reads {
		t.Fatalf("partial read must not retry before backoff: before=%d after=%d", reads, reader.fetchCalls)
	}
}

func TestFrequencyScheduler_UnassignedPolicyDoesNotReadInventory(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{priorityGatePolicy()}
	reader := &countingFrequencyReader{}
	service := &Service{repo: repo, platformGroups: reader, priorityActions: &gatePriorityActioner{}}

	service.runPriorityReconcileTick(context.Background())

	if reader.fetchCalls != 0 || reader.accountsCalls != 0 {
		t.Fatalf("unassigned policy must not read inventory: fetch=%d accounts=%d", reader.fetchCalls, reader.accountsCalls)
	}
}

func TestFrequencyScheduler_UnassignedPolicyDoesNotForceRepeatedInventoryReads(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 11, 15, 0, 0, time.UTC)
	assigned := priorityGatePolicy()
	assigned.UpdatedAt = now
	unassigned := assigned
	unassigned.ID = "unassigned"
	unassigned.Name = "unassigned"
	repo.policies = []Policy{assigned, unassigned}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: assigned.ID,
	}}
	priority := 50
	reader := &countingFrequencyReader{
		groups:   []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accounts: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "a", Priority: &priority, Models: "gpt-4o"}}},
	}
	service := &Service{
		repo: repo,
		mySites: fakeAdminGroupKeyReader{
			fakeMySitesReader: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
			keysBySite:        map[string][]upstream.Sub2APIKeyItem{},
		},
		platformGroups: reader, priorityActions: &gatePriorityActioner{}, now: func() time.Time { return now },
	}

	service.runPriorityReconcileTick(context.Background())
	state := priorityWorkspaceState(t, repo)
	if state.PolicyVersion != priorityPolicyVersion([]Policy{assigned}) {
		t.Fatalf("workspace version must exclude the unassigned policy: %+v", state)
	}
	service.runPriorityReconcileTick(context.Background())
	if reader.fetchCalls != 1 {
		t.Fatalf("an unassigned policy must not make the next reconcile read again: reads=%d", reader.fetchCalls)
	}
}

func TestFrequencyScheduler_ProbeEvaluationReusesSnapshotKeyMetadata(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 11, 20, 0, 0, time.UTC)
	policy := priorityGatePolicy()
	policy.UpdatedAt = now
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID,
	}}
	priority := 50
	reader := &countingFrequencyReader{
		groups:   []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)}},
		accounts: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "a", Priority: &priority, Models: "gpt-4o"}}},
	}
	keyReader := &countingFrequencyKeyReader{fakeAdminGroupKeyReader: fakeAdminGroupKeyReader{
		fakeMySitesReader: fakeMySitesReader{
			session: upstream.Session{Platform: upstream.PlatformNewAPI},
			connections: []my_sites.RealConnection{{
				UserID: "user1", WorkspaceAdminAccountID: "ws1", AdminAccountID: "a", UpstreamSiteID: "site-1", UpstreamKeyID: "key-1",
			}},
		},
		keysBySite: map[string][]upstream.Sub2APIKeyItem{"site-1": {{ID: "key-1", GroupID: "g-upstream", GroupName: "vip"}}},
	}}
	service := &Service{
		repo: repo, mySites: keyReader,
		sites:           fakeSiteLookup{site: &upstream.Site{Metrics: upstream.Metrics{Groups: []upstream.GroupInfo{{ID: "g-upstream", Name: "vip", Multiplier: float64Ptr(0.4)}}}}},
		platformGroups:  reader,
		priorityActions: &gatePriorityActioner{}, now: func() time.Time { return now },
	}

	service.runPriorityReconcileTick(context.Background())
	if keyReader.keyCalls != 1 {
		t.Fatalf("reconcile must resolve key metadata once: calls=%d", keyReader.keyCalls)
	}
	service.evaluateCurrentWorkspacePriorities(context.Background(), "user1", "ws1", priorityActionProbe)
	if keyReader.keyCalls != 1 {
		t.Fatalf("probe evaluation must use key metadata in the inventory snapshot: calls=%d", keyReader.keyCalls)
	}
}

func TestFrequencyScheduler_InvalidatedSnapshotPreventsPriorityWriteback(t *testing.T) {
	baseRepo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 11, 25, 0, 0, time.UTC)
	policy := priorityGatePolicy()
	baseRepo.policies = []Policy{policy}
	baseRepo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID,
	}}
	repo := &invalidateSnapshotOnListSettingsRepository{fakeRepository: baseRepo}
	priority := 50
	inventory := &adminWorkspaceInventory{
		session: upstream.Session{Platform: upstream.PlatformNewAPI}, complete: true, multiplierLookupLoaded: true,
		groups: []adminInventoryGroup{{
			group:    upstream.AdminGroupInfo{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)},
			accounts: []upstream.AdminGroupAccountInfo{{ID: "a", Priority: &priority, Models: "gpt-4o"}},
		}},
	}
	actions := &gatePriorityActioner{}
	service := &Service{repo: repo, priorityActions: actions, platformGroups: &countingFrequencyReader{}, now: func() time.Time { return now }}
	service.putAdminInventorySnapshot(context.Background(), "user1", "ws1", inventory, now, time.Minute)
	repo.mutate = func() { service.invalidateAdminInventorySnapshot("user1", "ws1") }
	cache := service.inventoryCacheForIdentities(map[string][2]string{"user1|ws1": {"user1", "ws1"}})

	service.syncMultiplierPrioritiesWithCacheRunMode(context.Background(), []Policy{policy}, nil, baseRepo.groupAssignments, nil, nil, cache, prioritySyncRunMode{
		source: priorityActionWriteback, write: true, workspaceFilter: map[string]struct{}{"user1|ws1": {}},
	})
	if len(actions.calls) != 0 {
		t.Fatalf("invalidated snapshot must not write a priority: %+v", actions.calls)
	}
}

func TestFrequencyScheduler_WritebackPersistsStateBeforeSnapshotInvalidation(t *testing.T) {
	baseRepo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 11, 26, 0, 0, time.UTC)
	policy := priorityGatePolicy()
	policy.ProbeIntervalSeconds = 60
	policy.PrioritySyncPreset.ReconcileIntervalSeconds = 30
	policy.PrioritySyncPreset.InventorySnapshotTTLSeconds = 60
	policy.PrioritySyncPreset.ReconcileFailureBackoffSeconds = 30
	policy.UpdatedAt = now
	baseRepo.policies = []Policy{policy}
	baseRepo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: policy.ID,
	}}
	repo := &contextAwarePriorityWorkspaceRepository{fakeRepository: baseRepo}
	priority := 99
	reader := &countingFrequencyReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)}},
		accounts: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "a", Name: "a", Priority: &priority, Models: "gpt-4o"}},
		},
	}
	actions := &gatePriorityActioner{}
	service := &Service{
		repo: repo,
		mySites: fakeAdminGroupKeyReader{
			fakeMySitesReader: fakeMySitesReader{
				session: upstream.Session{Platform: upstream.PlatformNewAPI},
				connections: []my_sites.RealConnection{{
					UserID: "user1", WorkspaceAdminAccountID: "ws1", AdminAccountID: "a", UpstreamSiteID: "site-1", UpstreamKeyID: "key-1",
				}},
			},
			keysBySite: map[string][]upstream.Sub2APIKeyItem{"site-1": {{ID: "key-1", GroupID: "g1", GroupName: "vip"}}},
		},
		sites:          fakeSiteLookup{site: &upstream.Site{Metrics: upstream.Metrics{Groups: []upstream.GroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)}}}}},
		platformGroups: reader, priorityActions: actions, now: func() time.Time { return now },
	}

	service.runPriorityReconcileTick(context.Background())
	queued := priorityWorkspaceState(t, baseRepo)
	if queued.PendingSignature == "" {
		t.Fatalf("reconcile must queue a writeback: %+v", queued)
	}
	service.runPriorityWritebackTick(context.Background())

	state := priorityWorkspaceState(t, baseRepo)
	if state.PendingSignature != "" || state.WriteSuccessCount != 1 || state.LastDecision != "applied" {
		t.Fatalf("successful writeback must persist its state before invalidation: %+v", state)
	}
	if len(actions.calls) != 1 {
		t.Fatalf("expected one priority writeback, got %+v", actions.calls)
	}
	if _, valid := service.getAdminInventorySnapshot("user1", "ws1", now); valid {
		t.Fatal("successful writeback must invalidate the completed snapshot")
	}
}

func TestFrequencyScheduler_InvalidatedSnapshotPreventsUnmanagedTargetRestore(t *testing.T) {
	baseRepo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 11, 27, 0, 0, time.UTC)
	targetID := buildTargetID(string(upstream.PlatformNewAPI), "ws1", "a")
	stored := TargetActionState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID,
		OriginalStatus: "1", OriginalWeight: intPtr(100), LastAppliedStatus: "2", LastAppliedWeight: intPtr(0),
	}
	baseRepo.targetActionStates["user1|ws1|"+targetID] = stored
	repo := &invalidateSnapshotOnTargetActionUpsertRepository{fakeRepository: baseRepo}
	inventory := &adminWorkspaceInventory{
		session: upstream.Session{Platform: upstream.PlatformNewAPI}, complete: true,
		groups: []adminInventoryGroup{{
			group:    upstream.AdminGroupInfo{ID: "g1", Name: "vip"},
			accounts: []upstream.AdminGroupAccountInfo{{ID: "a", Status: "2", Weight: intPtr(0)}},
		}},
	}
	runner := &countingTargetActionRunner{}
	service := &Service{repo: repo, dispatcher: runner, now: func() time.Time { return now }}
	service.putAdminInventorySnapshot(context.Background(), "user1", "ws1", inventory, now, time.Minute)
	repo.mutate = func() { service.invalidateAdminInventorySnapshot("user1", "ws1") }
	cache := service.inventoryCacheForIdentities(map[string][2]string{"user1|ws1": {"user1", "ws1"}})

	service.restoreUnmanagedTargetActions(context.Background(), nil, nil, nil, nil, []TargetActionState{stored}, cache)
	if runner.calls != 0 {
		t.Fatalf("invalidated snapshot must not restore target state upstream: calls=%d", runner.calls)
	}
}

func TestFrequencyScheduler_DeleteLastPolicyRestoresPriorityCheckpoint(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 11, 30, 0, 0, time.UTC)
	policy := priorityGatePolicy()
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: policy.ID,
	}}
	targetID := buildTargetID(string(upstream.PlatformNewAPI), "ws1", "a")
	repo.priorityStates["user1|ws1|"+targetID] = PrioritySyncState{
		UserID: "user1", AdminAccountID: "ws1", TargetID: targetID, OriginalPriority: 10, LastAppliedPriority: 20,
	}
	priority := 20
	reader := &countingFrequencyReader{
		groups:   []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Multiplier: float64Ptr(0.4)}},
		accounts: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "a", Priority: &priority, Models: "gpt-4o"}}},
	}
	actions := &gatePriorityActioner{}
	service := &Service{
		repo: repo, accounts: fakeAdminAccountResolver{id: "ws1"},
		mySites:        fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
		platformGroups: reader, priorityActions: actions, now: func() time.Time { return now },
	}
	if err := service.DeletePolicy(context.Background(), "user1", policy.ID); err != nil {
		t.Fatalf("delete policy: %v", err)
	}
	service.runPriorityReconcileTick(context.Background())
	service.runPriorityWritebackTick(context.Background())
	if len(actions.calls) != 1 || actions.calls[0].targetID != "a" || actions.calls[0].priority != 10 {
		t.Fatalf("deleting the final policy must restore the prior priority once: %+v", actions.calls)
	}
	if _, exists := repo.priorityStates["user1|ws1|"+targetID]; exists {
		t.Fatal("restored priority checkpoint must be cleared")
	}
}

func TestFrequencyScheduler_PendingWorkspaceSurvivesLastPolicyDeletion(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 11, 30, 0, 0, time.UTC)
	pendingSince := now.Add(-time.Minute)
	repo.UpsertPriorityWorkspaceSyncState(context.Background(), PriorityWorkspaceSyncState{
		UserID: "user1", AdminAccountID: "ws1", PendingSignature: "deleted-policy-signature", PendingSince: &pendingSince,
	})
	reader := &countingFrequencyReader{}
	service := &Service{
		repo: repo,
		mySites: fakeAdminGroupKeyReader{
			fakeMySitesReader: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
			keysBySite:        map[string][]upstream.Sub2APIKeyItem{},
		},
		platformGroups: reader, priorityActions: &gatePriorityActioner{}, now: func() time.Time { return now },
	}

	service.runPriorityReconcileTick(context.Background())
	if reader.fetchCalls != 1 {
		t.Fatalf("pending-only workspace must remain schedulable for convergence: reads=%d", reader.fetchCalls)
	}
	service.runPriorityWritebackTick(context.Background())
	state := priorityWorkspaceState(t, repo)
	if state.PendingSignature != "" || state.LastDecision != "observed_applied" {
		t.Fatalf("deleted-policy pending state must converge instead of becoming orphaned: %+v", state)
	}
}

func TestScheduledProbeUsesSnapshotMembershipWithoutInventoryRefresh(t *testing.T) {
	repo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	policy := probePolicy()
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: policy.ID,
	}}
	reader := &countingFrequencyReader{}
	service := &Service{
		repo: repo, platformGroups: reader, now: func() time.Time { return now },
	}
	service.putAdminInventorySnapshot(context.Background(), "user1", "ws1", &adminWorkspaceInventory{complete: true}, now, time.Minute)
	snapshot, valid := service.getAdminInventorySnapshot("user1", "ws1", now)
	if !valid {
		t.Fatal("expected valid inventory snapshot")
	}
	job := adminProbeJob{
		userID: "user1", adminAccountID: "ws1", session: upstream.Session{Platform: upstream.PlatformNewAPI},
		target: AdminProbeTarget{
			TargetID: buildTargetID(string(upstream.PlatformNewAPI), "ws1", "a"), Platform: string(upstream.PlatformNewAPI),
			AccountID: "a", Models: []string{"gpt-4o"},
		},
		account:        upstream.AdminGroupAccountInfo{ID: "a", Models: "gpt-4o"},
		memberships:    []adminTargetMembership{{groupID: "g1", groupName: "vip"}},
		snapshotBacked: true, snapshotFetchedAt: now, snapshotExpiresAt: now.Add(time.Minute),
		snapshotGeneration: snapshot.generation,
		dueSpecs:           []probeModelSpec{{modelName: "gpt-4o", policy: policy, policies: []Policy{policy}, budgetPolicy: policy}},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	service.runAdminProbeJob(context.Background(), job, func() {}, &wg)
	wg.Wait()
	if reader.fetchCalls != 0 || reader.accountsCalls != 0 {
		t.Fatalf("probe execution must not refresh inventory: fetch=%d accounts=%d", reader.fetchCalls, reader.accountsCalls)
	}
	if reader.resolveCalls != 1 {
		t.Fatalf("valid snapshot generation must reach credential resolution: calls=%d", reader.resolveCalls)
	}
}

func TestScheduledProbeRejectsReplacedOrExpiredSnapshotGeneration(t *testing.T) {
	for _, test := range []struct {
		name        string
		mutate      func(*Service, time.Time)
		advance     time.Duration
		wantResolve int
	}{
		{name: "valid", wantResolve: 1},
		{
			name: "replaced",
			mutate: func(service *Service, now time.Time) {
				service.invalidateAdminInventorySnapshot("user1", "ws1")
				service.putAdminInventorySnapshot(context.Background(), "user1", "ws1", &adminWorkspaceInventory{complete: true}, now.Add(time.Second), time.Minute)
			},
			advance: time.Second,
		},
		{name: "expired", mutate: func(*Service, time.Time) {}, advance: time.Minute},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepository()
			now := time.Date(2026, time.August, 8, 12, 30, 0, 0, time.UTC)
			current := now
			policy := probePolicy()
			repo.policies = []Policy{policy}
			repo.groupAssignments = []GroupPolicyAssignment{{
				UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: policy.ID,
			}}
			reader := &countingFrequencyReader{}
			service := &Service{repo: repo, platformGroups: reader, now: func() time.Time { return current }}
			service.putAdminInventorySnapshot(context.Background(), "user1", "ws1", &adminWorkspaceInventory{complete: true}, now, time.Minute)
			snapshot, valid := service.getAdminInventorySnapshot("user1", "ws1", now)
			if !valid {
				t.Fatal("expected valid inventory snapshot")
			}
			job := adminProbeJob{
				userID: "user1", adminAccountID: "ws1", session: upstream.Session{Platform: upstream.PlatformNewAPI},
				snapshotFetchedAt: now, snapshotExpiresAt: now.Add(time.Minute),
				snapshotBacked:     true,
				snapshotGeneration: snapshot.generation,
				target: AdminProbeTarget{
					TargetID: buildTargetID(string(upstream.PlatformNewAPI), "ws1", "a"), Platform: string(upstream.PlatformNewAPI),
					AccountID: "a", Models: []string{"gpt-4o"},
				},
				account:     upstream.AdminGroupAccountInfo{ID: "a", Models: "gpt-4o"},
				memberships: []adminTargetMembership{{groupID: "g1", groupName: "vip"}},
				dueSpecs:    []probeModelSpec{{modelName: "gpt-4o", policy: policy, policies: []Policy{policy}, budgetPolicy: policy}},
			}
			if test.mutate != nil {
				test.mutate(service, now)
			}
			current = now.Add(test.advance)
			var wg sync.WaitGroup
			wg.Add(1)
			service.runAdminProbeJob(context.Background(), job, func() {}, &wg)
			wg.Wait()
			if reader.resolveCalls != test.wantResolve {
				t.Fatalf("credential resolution calls=%d, want %d", reader.resolveCalls, test.wantResolve)
			}
		})
	}
}

func TestScheduledProbeCancelsWhenSnapshotInvalidatesAfterInitialCheck(t *testing.T) {
	baseRepo := newFakeRepository()
	now := time.Date(2026, time.August, 8, 12, 45, 0, 0, time.UTC)
	policy := probePolicy()
	baseRepo.policies = []Policy{policy}
	baseRepo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: policy.ID,
	}}
	reader := &countingFrequencyReader{}
	repo := &invalidateSnapshotOnListPoliciesRepository{fakeRepository: baseRepo}
	service := &Service{repo: repo, platformGroups: reader, now: func() time.Time { return now }}
	repo.mutate = func() { service.invalidateAdminInventorySnapshot("user1", "ws1") }
	service.putAdminInventorySnapshot(context.Background(), "user1", "ws1", &adminWorkspaceInventory{complete: true}, now, time.Minute)
	snapshot, valid := service.getAdminInventorySnapshot("user1", "ws1", now)
	if !valid {
		t.Fatal("expected valid inventory snapshot")
	}
	job := adminProbeJob{
		userID: "user1", adminAccountID: "ws1", session: upstream.Session{Platform: upstream.PlatformNewAPI},
		snapshotFetchedAt: now, snapshotExpiresAt: now.Add(time.Minute), snapshotGeneration: snapshot.generation,
		snapshotBacked: true,
		target: AdminProbeTarget{
			TargetID: buildTargetID(string(upstream.PlatformNewAPI), "ws1", "a"), Platform: string(upstream.PlatformNewAPI),
			AccountID: "a", Models: []string{"gpt-4o"},
		},
		account:     upstream.AdminGroupAccountInfo{ID: "a", Models: "gpt-4o"},
		memberships: []adminTargetMembership{{groupID: "g1", groupName: "vip"}},
		dueSpecs:    []probeModelSpec{{modelName: "gpt-4o", policy: policy, policies: []Policy{policy}, budgetPolicy: policy}},
	}
	var wg sync.WaitGroup
	wg.Add(1)
	service.runAdminProbeJob(context.Background(), job, func() {}, &wg)
	wg.Wait()
	if reader.resolveCalls != 0 {
		t.Fatalf("invalidated generation must cancel before credential resolution: calls=%d", reader.resolveCalls)
	}
}

func TestInventorySnapshotCacheRejectsInvalidatedSnapshot(t *testing.T) {
	now := time.Date(2026, time.August, 8, 13, 0, 0, 0, time.UTC)
	service := &Service{now: func() time.Time { return now }}
	service.putAdminInventorySnapshot(context.Background(), "user1", "ws1", &adminWorkspaceInventory{complete: true}, now, time.Minute)
	cache := service.inventoryCacheForIdentities(map[string][2]string{"user1|ws1": {"user1", "ws1"}})
	service.invalidateAdminInventorySnapshot("user1", "ws1")
	if _, err := service.loadAdminInventory(context.Background(), "user1", "ws1", cache); !errors.Is(err, errInventorySnapshotUnavailable) {
		t.Fatalf("invalidated snapshot must not be consumed: %v", err)
	}
}

func TestInventorySnapshotMissSchedulesRecoveryWithoutBypassingFailureBackoff(t *testing.T) {
	now := time.Date(2026, time.August, 8, 13, 30, 0, 0, time.UTC)
	for _, test := range []struct {
		name       string
		status     string
		wantNextAt time.Time
	}{
		{name: "ready snapshot disappeared", status: "ready", wantNextAt: now},
		{name: "unknown failure remains backed off", status: "unknown", wantNextAt: now.Add(time.Minute)},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newFakeRepository()
			future := now.Add(time.Minute)
			repo.UpsertPriorityWorkspaceSyncState(context.Background(), PriorityWorkspaceSyncState{
				UserID: "user1", AdminAccountID: "ws1", InventoryStatus: test.status, NextReconcileAt: &future,
			})
			service := &Service{repo: repo, now: func() time.Time { return now }}

			service.recordInventorySnapshotMissLocked(context.Background(), "user1", "ws1", nil, priorityActionProbe)

			state := priorityWorkspaceState(t, repo)
			if state.NextReconcileAt == nil || !state.NextReconcileAt.Equal(test.wantNextAt) {
				t.Fatalf("next reconcile = %v, want %v", state.NextReconcileAt, test.wantNextAt)
			}
		})
	}
}

func TestFrequencySchedulersStopCancelsBlockedInventoryRead(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: repo.policies[0].ID,
	}}
	reader := &countingFrequencyReader{blockFetch: true, fetchStarted: make(chan struct{}, 1)}
	service := &Service{
		repo: repo,
		mySites: fakeAdminGroupKeyReader{
			fakeMySitesReader: fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}},
			keysBySite:        map[string][]upstream.Sub2APIKeyItem{},
		},
		platformGroups: reader,
	}
	service.StartScheduler(context.Background())
	select {
	case <-reader.fetchStarted:
	case <-time.After(time.Second):
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.StopScheduler(stopCtx)
		t.Fatal("scheduler did not begin the blocking inventory read")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.StopScheduler(stopCtx); err != nil {
		t.Fatalf("scheduler stop must cancel the blocking inventory read: %v", err)
	}
	if reader.fetchCalls != 1 {
		t.Fatalf("expected one canceled inventory read, got %d", reader.fetchCalls)
	}
}
