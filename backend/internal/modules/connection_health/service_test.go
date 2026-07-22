package connection_health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

// fakeRepository 是 healthRepository 的内存实现，供 service 单测使用，不连接真实数据库。
type fakeRepository struct {
	policies           []Policy
	states             map[string]map[string]ConnectionHealthState // connectionID -> modelName -> state
	events             []ConnectionHealthEvent
	assignments        []PolicyAssignment
	groupAssignments   []GroupPolicyAssignment
	groupExclusions    []GroupTargetExclusion
	priorityStates     map[string]PrioritySyncState
	targetActionStates map[string]TargetActionState
	budgetClaims       map[string]int
	savePolicyErr      error
}

func newFakeRepository() *fakeRepository {
	return &fakeRepository{
		states:             map[string]map[string]ConnectionHealthState{},
		priorityStates:     map[string]PrioritySyncState{},
		targetActionStates: map[string]TargetActionState{},
		budgetClaims:       map[string]int{},
	}
}

func (f *fakeRepository) EnsureSchema(ctx context.Context) error { return nil }

func (f *fakeRepository) ListPolicies(ctx context.Context, userID string, adminAccountID string) ([]Policy, error) {
	out := make([]Policy, 0)
	for _, p := range f.policies {
		if p.UserID == userID && p.AdminAccountID == adminAccountID {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListEnabledPolicies(ctx context.Context) ([]Policy, error) {
	out := make([]Policy, 0)
	for _, p := range f.policies {
		if p.Enabled {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeRepository) GetPolicy(ctx context.Context, id string, userID string, adminAccountID string) (*Policy, error) {
	for _, p := range f.policies {
		if p.ID == id && p.UserID == userID && p.AdminAccountID == adminAccountID {
			cp := p
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeRepository) UpsertPolicy(ctx context.Context, p Policy) error {
	for i, existing := range f.policies {
		if existing.ID == p.ID {
			p.ModelTargets = existing.ModelTargets
			f.policies[i] = p
			return nil
		}
	}
	f.policies = append(f.policies, p)
	return nil
}

func (f *fakeRepository) ReplaceModelTargets(ctx context.Context, policyID string, targets []ModelTarget) error {
	for i, p := range f.policies {
		if p.ID == policyID {
			f.policies[i].ModelTargets = targets
			return nil
		}
	}
	return nil
}

func (f *fakeRepository) SavePolicyWithTargets(ctx context.Context, policy Policy, targets []ModelTarget) error {
	if f.savePolicyErr != nil {
		return f.savePolicyErr
	}
	policy.ModelTargets = append([]ModelTarget(nil), targets...)
	for i, existing := range f.policies {
		if existing.ID == policy.ID {
			f.policies[i] = policy
			return nil
		}
	}
	f.policies = append(f.policies, policy)
	return nil
}

func TestSavePolicy_RepositoryFailureLeavesExistingPolicyUntouched(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{{
		ID: "p1", UserID: "user1", AdminAccountID: "ws1", Name: "before", Enabled: true,
		ModelTargets: []ModelTarget{{ID: "old-target", PolicyID: "p1", ModelName: "old-model", Enabled: true}},
	}}
	repo.savePolicyErr = errors.New("transaction failed")
	service := &Service{repo: repo, accounts: fakeAdminAccountResolver{id: "ws1"}}

	_, err := service.SavePolicy(context.Background(), "user1", PolicyInput{
		ID: "p1", Name: "after", Enabled: false,
		ModelTargets: []ModelTargetInput{{ModelName: "new-model", Enabled: true}},
	})
	if err == nil {
		t.Fatal("expected repository transaction failure")
	}
	if len(repo.policies) != 1 || repo.policies[0].Name != "before" || !repo.policies[0].Enabled {
		t.Fatalf("failed atomic save must not mutate the policy: %+v", repo.policies)
	}
	if len(repo.policies[0].ModelTargets) != 1 || repo.policies[0].ModelTargets[0].ModelName != "old-model" {
		t.Fatalf("failed atomic save must retain old model targets: %+v", repo.policies[0].ModelTargets)
	}
}

func TestBuildPolicyAndTargets_DisablesRemoteActionWithoutAutoDegrade(t *testing.T) {
	policy, _, err := buildPolicyAndTargets("user1", "ws1", "p1", PolicyInput{
		Name: "monitor only", AutoDegradeEnabled: false, AutoRemoteActionEnabled: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if policy.AutoRemoteActionEnabled {
		t.Fatal("remote action must be normalized off when automatic degradation is disabled")
	}
}

func (f *fakeRepository) ListStatesByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]ConnectionHealthState, error) {
	out := make([]ConnectionHealthState, 0)
	for _, byModel := range f.states {
		for _, st := range byModel {
			if st.UserID == userID && st.AdminAccountID == adminAccountID {
				out = append(out, st)
			}
		}
	}
	return out, nil
}

func (f *fakeRepository) ListStatesByConnection(ctx context.Context, connectionID string) ([]ConnectionHealthState, error) {
	out := make([]ConnectionHealthState, 0)
	for _, st := range f.states[connectionID] {
		out = append(out, st)
	}
	return out, nil
}

func (f *fakeRepository) GetState(ctx context.Context, connectionID string, modelName string) (*ConnectionHealthState, error) {
	if byModel, ok := f.states[connectionID]; ok {
		if st, ok := byModel[modelName]; ok {
			cp := st
			return &cp, nil
		}
	}
	return nil, nil
}

func (f *fakeRepository) UpsertState(ctx context.Context, s ConnectionHealthState) error {
	if f.states[s.ConnectionID] == nil {
		f.states[s.ConnectionID] = map[string]ConnectionHealthState{}
	}
	f.states[s.ConnectionID][s.ModelName] = s
	return nil
}

func (f *fakeRepository) InsertEvent(ctx context.Context, e ConnectionHealthEvent) error {
	f.events = append(f.events, e)
	return nil
}

func (f *fakeRepository) ListEventsByConnection(ctx context.Context, connectionID string, userID string, adminAccountID string, limit int) ([]ConnectionHealthEvent, error) {
	out := make([]ConnectionHealthEvent, 0)
	for _, e := range f.events {
		if e.ConnectionID == connectionID && e.UserID == userID && e.AdminAccountID == adminAccountID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListRecentEventsByWorkspace(ctx context.Context, userID string, adminAccountID string, limit int) ([]ConnectionHealthEvent, error) {
	out := make([]ConnectionHealthEvent, 0)
	for _, e := range f.events {
		if e.UserID == userID && e.AdminAccountID == adminAccountID {
			out = append(out, e)
		}
	}
	return out, nil
}

func (f *fakeRepository) CountFailureEventsSince(ctx context.Context, userID string, adminAccountID string, since time.Time) (int, error) {
	count := 0
	for _, event := range f.events {
		if event.UserID == userID && event.AdminAccountID == adminAccountID && !event.CreatedAt.Before(since) && slices.Contains(probeFailureResultKeys(), event.Result) {
			count++
		}
	}
	return count, nil
}

func (f *fakeRepository) CountProbesToday(ctx context.Context, userID string, adminAccountID string, policyID string, dayStart time.Time) (int, error) {
	count := 0
	for _, e := range f.events {
		if e.UserID == userID && e.AdminAccountID == adminAccountID && e.PolicyID == policyID && isProbeResultString(e.Result) {
			count++
		}
	}
	key := userID + "|" + adminAccountID + "|" + policyID + "|" + dayStart.Format(time.RFC3339)
	if claimed := f.budgetClaims[key]; claimed > count {
		return claimed, nil
	}
	return count, nil
}

func (f *fakeRepository) TryConsumeProbeBudget(ctx context.Context, userID string, adminAccountID string, policyID string, dayStart time.Time, limit int) (bool, error) {
	key := userID + "|" + adminAccountID + "|" + policyID + "|" + dayStart.Format(time.RFC3339)
	if _, initialized := f.budgetClaims[key]; !initialized {
		count := 0
		for _, event := range f.events {
			if event.UserID == userID && event.AdminAccountID == adminAccountID && event.PolicyID == policyID && isProbeResultString(event.Result) {
				count++
			}
		}
		f.budgetClaims[key] = count
	}
	if f.budgetClaims[key] >= limit {
		return false, nil
	}
	f.budgetClaims[key]++
	return true, nil
}

func (f *fakeRepository) TryAcquireSchedulerLease(ctx context.Context) (func(), bool, error) {
	return func() {}, true, nil
}

func (f *fakeRepository) AcquireTargetLease(ctx context.Context, targetID string) (func(), error) {
	return func() {}, nil
}

func isProbeResultString(result string) bool {
	return slices.Contains(probeResultKeys(), result)
}

// ReplacePolicyAssignments/List* 是策略分配表的内存实现，语义对齐 Repository 的真实实现：
// 先删后插整体替换，查询均按 (userID, adminAccountID[, targetID]) 过滤。
func (f *fakeRepository) ReplacePolicyAssignments(ctx context.Context, userID string, adminAccountID string, targetID string, policyIDs []string) error {
	kept := make([]PolicyAssignment, 0, len(f.assignments))
	for _, a := range f.assignments {
		if a.UserID == userID && a.AdminAccountID == adminAccountID && a.TargetID == targetID {
			continue
		}
		kept = append(kept, a)
	}
	for i, policyID := range policyIDs {
		kept = append(kept, PolicyAssignment{
			ID: targetID + "::" + policyID + fmt.Sprintf("::%d", i), UserID: userID, AdminAccountID: adminAccountID,
			TargetID: targetID, PolicyID: policyID,
		})
	}
	f.assignments = kept
	return nil
}

func (f *fakeRepository) ListPolicyAssignmentsForTarget(ctx context.Context, userID string, adminAccountID string, targetID string) ([]PolicyAssignment, error) {
	out := make([]PolicyAssignment, 0)
	for _, a := range f.assignments {
		if a.UserID == userID && a.AdminAccountID == adminAccountID && a.TargetID == targetID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListPolicyAssignmentsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]PolicyAssignment, error) {
	out := make([]PolicyAssignment, 0)
	for _, a := range f.assignments {
		if a.UserID == userID && a.AdminAccountID == adminAccountID {
			out = append(out, a)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListAllPolicyAssignments(ctx context.Context) ([]PolicyAssignment, error) {
	out := make([]PolicyAssignment, len(f.assignments))
	copy(out, f.assignments)
	return out, nil
}

func (f *fakeRepository) ReplaceGroupPolicyConfiguration(ctx context.Context, userID string, adminAccountID string, adminGroupID string, adminGroupName string, policyIDs []string, excludedTargetIDs []string, groupTargetIDs []string) error {
	assignments := make([]GroupPolicyAssignment, 0, len(f.groupAssignments)+len(policyIDs))
	for _, assignment := range f.groupAssignments {
		if assignment.UserID == userID && assignment.AdminAccountID == adminAccountID && assignment.AdminGroupID == adminGroupID {
			continue
		}
		assignments = append(assignments, assignment)
	}
	for i, policyID := range policyIDs {
		assignments = append(assignments, GroupPolicyAssignment{
			ID: fmt.Sprintf("%s::%s::%d", adminGroupID, policyID, i), UserID: userID,
			AdminAccountID: adminAccountID, AdminGroupID: adminGroupID, AdminGroupName: adminGroupName, PolicyID: policyID,
		})
	}
	f.groupAssignments = assignments

	exclusions := make([]GroupTargetExclusion, 0, len(f.groupExclusions)+len(excludedTargetIDs))
	for _, exclusion := range f.groupExclusions {
		if exclusion.UserID == userID && exclusion.AdminAccountID == adminAccountID && exclusion.AdminGroupID == adminGroupID {
			continue
		}
		exclusions = append(exclusions, exclusion)
	}
	for i, targetID := range excludedTargetIDs {
		exclusions = append(exclusions, GroupTargetExclusion{
			ID: fmt.Sprintf("%s::%s::%d", adminGroupID, targetID, i), UserID: userID,
			AdminAccountID: adminAccountID, AdminGroupID: adminGroupID, TargetID: targetID,
		})
	}
	f.groupExclusions = exclusions
	for _, targetID := range groupTargetIDs {
		key := userID + "|" + adminAccountID + "|" + targetID
		if state, ok := f.priorityStates[key]; ok && state.Conflict {
			delete(f.priorityStates, key)
		}
		if state, ok := f.targetActionStates[key]; ok && state.Conflict {
			delete(f.targetActionStates, key)
		}
	}
	return nil
}

func (f *fakeRepository) CreatePolicyAndReplaceGroupConfiguration(ctx context.Context, policy Policy, targets []ModelTarget, adminGroupID string, adminGroupName string, policyIDs []string, excludedTargetIDs []string, groupTargetIDs []string) error {
	policy.ModelTargets = append([]ModelTarget(nil), targets...)
	f.policies = append(f.policies, policy)
	return f.ReplaceGroupPolicyConfiguration(ctx, policy.UserID, policy.AdminAccountID, adminGroupID, adminGroupName, policyIDs, excludedTargetIDs, groupTargetIDs)
}

func (f *fakeRepository) ListGroupPolicyAssignmentsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]GroupPolicyAssignment, error) {
	out := make([]GroupPolicyAssignment, 0)
	for _, assignment := range f.groupAssignments {
		if assignment.UserID == userID && assignment.AdminAccountID == adminAccountID {
			out = append(out, assignment)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListAllGroupPolicyAssignments(ctx context.Context) ([]GroupPolicyAssignment, error) {
	return append([]GroupPolicyAssignment(nil), f.groupAssignments...), nil
}

func (f *fakeRepository) ListGroupTargetExclusionsByWorkspace(ctx context.Context, userID string, adminAccountID string) ([]GroupTargetExclusion, error) {
	out := make([]GroupTargetExclusion, 0)
	for _, exclusion := range f.groupExclusions {
		if exclusion.UserID == userID && exclusion.AdminAccountID == adminAccountID {
			out = append(out, exclusion)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListAllGroupTargetExclusions(ctx context.Context) ([]GroupTargetExclusion, error) {
	return append([]GroupTargetExclusion(nil), f.groupExclusions...), nil
}

func (f *fakeRepository) ListPrioritySyncStates(ctx context.Context, userID string, adminAccountID string) ([]PrioritySyncState, error) {
	out := make([]PrioritySyncState, 0)
	for _, state := range f.priorityStates {
		if state.UserID == userID && state.AdminAccountID == adminAccountID {
			out = append(out, state)
		}
	}
	return out, nil
}

func (f *fakeRepository) ListAllPrioritySyncStates(ctx context.Context) ([]PrioritySyncState, error) {
	out := make([]PrioritySyncState, 0, len(f.priorityStates))
	for _, state := range f.priorityStates {
		out = append(out, state)
	}
	return out, nil
}

func (f *fakeRepository) UpsertPrioritySyncState(ctx context.Context, state PrioritySyncState) error {
	if f.priorityStates == nil {
		f.priorityStates = map[string]PrioritySyncState{}
	}
	f.priorityStates[state.UserID+"|"+state.AdminAccountID+"|"+state.TargetID] = state
	return nil
}

func (f *fakeRepository) DeletePrioritySyncState(ctx context.Context, userID string, adminAccountID string, targetID string) error {
	delete(f.priorityStates, userID+"|"+adminAccountID+"|"+targetID)
	return nil
}

func (f *fakeRepository) GetTargetActionState(ctx context.Context, userID string, adminAccountID string, targetID string) (*TargetActionState, error) {
	state, ok := f.targetActionStates[userID+"|"+adminAccountID+"|"+targetID]
	if !ok {
		return nil, nil
	}
	copy := state
	return &copy, nil
}

func (f *fakeRepository) ListAllTargetActionStates(ctx context.Context) ([]TargetActionState, error) {
	states := make([]TargetActionState, 0, len(f.targetActionStates))
	for _, state := range f.targetActionStates {
		states = append(states, state)
	}
	return states, nil
}

func (f *fakeRepository) ListTargetActionStates(ctx context.Context, userID string, adminAccountID string) ([]TargetActionState, error) {
	states := make([]TargetActionState, 0)
	for _, state := range f.targetActionStates {
		if state.UserID == userID && state.AdminAccountID == adminAccountID {
			states = append(states, state)
		}
	}
	return states, nil
}

func (f *fakeRepository) UpsertTargetActionState(ctx context.Context, state TargetActionState) error {
	if f.targetActionStates == nil {
		f.targetActionStates = map[string]TargetActionState{}
	}
	f.targetActionStates[state.UserID+"|"+state.AdminAccountID+"|"+state.TargetID] = state
	return nil
}

func (f *fakeRepository) DeleteTargetActionState(ctx context.Context, userID string, adminAccountID string, targetID string) error {
	delete(f.targetActionStates, userID+"|"+adminAccountID+"|"+targetID)
	return nil
}

// fakeMySitesReader 是 MySitesReader 的内存实现。
type fakeMySitesReader struct {
	connections []my_sites.RealConnection
	ownGroups   []my_sites.MappingOwnGroupOption
	session     upstream.Session
}

func (f fakeMySitesReader) ListRealConnections(ctx context.Context, userID string) ([]my_sites.RealConnection, error) {
	return f.connections, nil
}

// ListRealConnectionsForWorkspace 模拟按显式 userID+adminAccountID 过滤的仓库行为，
// 用 RealConnection.UserID / WorkspaceAdminAccountID 字段做匹配，供 scheduler 的多 workspace
// 隔离测试使用；未设置这两个字段的旧 fixture 仍按零值（""）匹配，兼容既有测试用例。
func (f fakeMySitesReader) ListRealConnectionsForWorkspace(ctx context.Context, userID string, adminAccountID string) ([]my_sites.RealConnection, error) {
	out := make([]my_sites.RealConnection, 0)
	for _, c := range f.connections {
		if c.UserID == userID && c.WorkspaceAdminAccountID == adminAccountID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (f fakeMySitesReader) MappingOptions(ctx context.Context, userID string) (my_sites.MappingOptionsResponse, error) {
	return my_sites.MappingOptionsResponse{OwnGroups: f.ownGroups}, nil
}

func (f fakeMySitesReader) RequireSession(ctx context.Context, userID string, adminAccountID string) (upstream.Session, error) {
	return f.session, nil
}

type fakeAdminAccountResolver struct{ id string }

func (f fakeAdminAccountResolver) RequireCurrentID(ctx context.Context, userID string) (string, error) {
	return f.id, nil
}

type noopRemoteActionRunner struct{}

func (noopRemoteActionRunner) Degrade(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (string, error) {
	return "", nil
}

func (noopRemoteActionRunner) Restore(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (string, error) {
	return "", nil
}

func (noopRemoteActionRunner) DegradeTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (string, error) {
	return "", nil
}

func (noopRemoteActionRunner) RestoreTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (string, error) {
	return "", nil
}

func (noopRemoteActionRunner) ApplyTargetState(ctx context.Context, session upstream.Session, target AdminProbeTarget, weight *int, status string) (string, error) {
	return "", nil
}

func TestGroups_NoRealConnectionsShowsNotConnected(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{
		ownGroups: []my_sites.MappingOwnGroupOption{{ID: "g1", GroupName: "group-one"}},
	}
	svc := &Service{repo: repo, mySites: mySites, accounts: fakeAdminAccountResolver{id: "ws1"}, dispatcher: noopRemoteActionRunner{}, probeRunner: NewRealProbeRunner()}

	groups, err := svc.Groups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 group, got %d", len(groups))
	}
	if groups[0].HasConnections {
		t.Fatalf("expected group with zero real_connections to show as not connected")
	}
	if len(groups[0].Connections) != 0 {
		t.Fatalf("expected zero connections listed")
	}
}

func TestGroups_SiblingConnectionsInSameGroupAreIndependent(t *testing.T) {
	repo := newFakeRepository()
	repo.states["conn-healthy"] = map[string]ConnectionHealthState{
		"gpt-4o-mini": {ConnectionID: "conn-healthy", ModelName: "gpt-4o-mini", UserID: "user1", AdminAccountID: "ws1", State: StateHealthy, CurrentWeight: 100},
	}
	repo.states["conn-suspended"] = map[string]ConnectionHealthState{
		"gpt-4o-mini": {ConnectionID: "conn-suspended", ModelName: "gpt-4o-mini", UserID: "user1", AdminAccountID: "ws1", State: StateSuspended, CurrentWeight: 0},
	}

	mySites := fakeMySitesReader{
		ownGroups: []my_sites.MappingOwnGroupOption{{ID: "g1", GroupName: "group-one"}},
		connections: []my_sites.RealConnection{
			{ID: "conn-healthy", OwnGroupIDs: []string{"g1"}, UpstreamKey: "super-secret-key-1"},
			{ID: "conn-suspended", OwnGroupIDs: []string{"g1"}, UpstreamKey: "super-secret-key-2"},
		},
	}
	svc := &Service{repo: repo, mySites: mySites, accounts: fakeAdminAccountResolver{id: "ws1"}, dispatcher: noopRemoteActionRunner{}, probeRunner: NewRealProbeRunner()}

	groups, err := svc.Groups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 || !groups[0].HasConnections || len(groups[0].Connections) != 2 {
		t.Fatalf("expected 1 group with 2 connections, got %+v", groups)
	}

	var healthyState, suspendedState State
	for _, c := range groups[0].Connections {
		if c.ConnectionID == "conn-healthy" {
			healthyState = c.Models[0].State
		}
		if c.ConnectionID == "conn-suspended" {
			suspendedState = c.Models[0].State
		}
	}
	if healthyState != StateHealthy {
		t.Fatalf("expected sibling connection to remain healthy, got %s", healthyState)
	}
	if suspendedState != StateSuspended {
		t.Fatalf("expected the failing connection to be suspended, got %s", suspendedState)
	}
}

func TestGroups_NeverLeaksUpstreamKey(t *testing.T) {
	const secretKey = "sk-should-never-appear-anywhere"
	repo := newFakeRepository()
	mySites := fakeMySitesReader{
		ownGroups: []my_sites.MappingOwnGroupOption{{ID: "g1", GroupName: "group-one"}},
		connections: []my_sites.RealConnection{
			{ID: "conn-1", OwnGroupIDs: []string{"g1"}, UpstreamKey: secretKey, UpstreamKeyID: "key-id-1"},
		},
	}
	svc := &Service{repo: repo, mySites: mySites, accounts: fakeAdminAccountResolver{id: "ws1"}, dispatcher: noopRemoteActionRunner{}, probeRunner: NewRealProbeRunner()}

	groups, err := svc.Groups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	encoded, err := json.Marshal(groups)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	if strings.Contains(string(encoded), secretKey) {
		t.Fatalf("upstream key leaked into aggregated response: %s", encoded)
	}
}

func TestOverview_CountsByState(t *testing.T) {
	repo := newFakeRepository()
	repo.states["conn-1"] = map[string]ConnectionHealthState{
		"m1": {ConnectionID: "conn-1", ModelName: "m1", UserID: "user1", AdminAccountID: "ws1", State: StateHealthy},
		"m2": {ConnectionID: "conn-1", ModelName: "m2", UserID: "user1", AdminAccountID: "ws1", State: StateDegraded},
	}
	mySites := fakeMySitesReader{
		ownGroups:   []my_sites.MappingOwnGroupOption{{ID: "g1", GroupName: "group-one"}},
		connections: []my_sites.RealConnection{{ID: "conn-1", OwnGroupIDs: []string{"g1"}}},
	}
	svc := &Service{repo: repo, mySites: mySites, accounts: fakeAdminAccountResolver{id: "ws1"}, dispatcher: noopRemoteActionRunner{}, probeRunner: NewRealProbeRunner()}

	overview, err := svc.Overview(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overview.TotalConnections != 1 || overview.Healthy != 1 || overview.Degraded != 1 {
		t.Fatalf("unexpected overview counts: %+v", overview)
	}
}

func TestStoredSummary_DeduplicatesTargetsAndKeepsWorkspaceIsolated(t *testing.T) {
	now := time.Now()
	olderProbe := now.Add(-2 * time.Hour)
	latestProbe := now.Add(-30 * time.Minute)
	repo := newFakeRepository()
	repo.states["target-healthy"] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: "target-healthy", ModelName: "model-a", UserID: "user1", AdminAccountID: "ws1", State: StateHealthy, LastProbeAt: &olderProbe},
		"model-b": {ConnectionID: "target-healthy", ModelName: "model-b", UserID: "user1", AdminAccountID: "ws1", State: StateHealthy},
	}
	repo.states["target-attention"] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: "target-attention", ModelName: "model-a", UserID: "user1", AdminAccountID: "ws1", State: StateHealthy},
		"model-b": {ConnectionID: "target-attention", ModelName: "model-b", UserID: "user1", AdminAccountID: "ws1", State: StateRecovering, LastProbeAt: &latestProbe},
	}
	repo.states["target-suspended"] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: "target-suspended", ModelName: "model-a", UserID: "user1", AdminAccountID: "ws1", State: StateDisabled},
	}
	repo.states["other-workspace"] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: "other-workspace", ModelName: "model-a", UserID: "user1", AdminAccountID: "ws2", State: StateSuspended},
	}
	repo.targetActionStates["user1|ws1|target-suspended"] = TargetActionState{UserID: "user1", AdminAccountID: "ws1", TargetID: "target-suspended"}
	repo.targetActionStates["user1|ws2|other-workspace"] = TargetActionState{UserID: "user1", AdminAccountID: "ws2", TargetID: "other-workspace"}
	repo.events = []ConnectionHealthEvent{
		{ID: "failure-recent", UserID: "user1", AdminAccountID: "ws1", Result: string(ResultServerError), CreatedAt: now.Add(-time.Hour)},
		{ID: "success-recent", UserID: "user1", AdminAccountID: "ws1", Result: string(ResultOK), CreatedAt: now.Add(-time.Hour)},
		{ID: "failure-old", UserID: "user1", AdminAccountID: "ws1", Result: string(ResultAuth), CreatedAt: now.Add(-25 * time.Hour)},
		{ID: "failure-other", UserID: "user1", AdminAccountID: "ws2", Result: string(ResultAuth), CreatedAt: now.Add(-time.Hour)},
	}
	service := &Service{repo: repo, accounts: fakeAdminAccountResolver{id: "ws1"}}

	summary, err := service.StoredSummary(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.TotalTargets != 3 || summary.HealthyTargets != 1 || summary.AttentionTargets != 1 || summary.SuspendedTargets != 1 {
		t.Fatalf("unexpected target counts: %+v", summary)
	}
	if summary.ManagedTargets != 1 || summary.RecentFailureEvents != 1 {
		t.Fatalf("unexpected managed/failure counts: %+v", summary)
	}
	if summary.LastProbeAt == nil || !summary.LastProbeAt.Equal(latestProbe) {
		t.Fatalf("expected latest probe %v, got %v", latestProbe, summary.LastProbeAt)
	}
}

// TestEvents_ConnectionIDMustBelongToCurrentWorkspace 回归测试：验证 Events 方法在接收
// connectionId 参数时必须先校验该连接属于当前用户当前 workspace，防止 IDOR 越权读取其他
// workspace 的事件。
func TestEvents_ConnectionIDMustBelongToCurrentWorkspace(t *testing.T) {
	repo := newFakeRepository()
	// 插入两个 workspace 的连接和事件：ws1 拥有 conn-a，ws2 拥有 conn-b。
	repo.InsertEvent(context.Background(), ConnectionHealthEvent{
		ID: "event-a", ConnectionID: "conn-a", ModelName: "m1",
		UserID: "user1", AdminAccountID: "ws1", Result: "ok",
	})
	repo.InsertEvent(context.Background(), ConnectionHealthEvent{
		ID: "event-b", ConnectionID: "conn-b", ModelName: "m2",
		UserID: "user1", AdminAccountID: "ws2", Result: "ok",
	})

	// mySites 只返回当前 workspace (ws1) 的连接。
	mySites := fakeMySitesReader{
		ownGroups:   []my_sites.MappingOwnGroupOption{{ID: "g1", GroupName: "group-one"}},
		connections: []my_sites.RealConnection{{ID: "conn-a", OwnGroupIDs: []string{"g1"}}},
	}
	svc := &Service{
		repo:     repo,
		mySites:  mySites,
		accounts: fakeAdminAccountResolver{id: "ws1"},
	}

	// 场景 1：查询当前 workspace 拥有的连接，应该成功返回事件。
	events, err := svc.Events(context.Background(), "user1", "conn-a", 100)
	if err != nil {
		t.Fatalf("查询自己 workspace 的连接事件失败: %v", err)
	}
	if len(events) != 1 || events[0].ID != "event-a" {
		t.Fatalf("期望返回 event-a，实际返回 %+v", events)
	}

	// 场景 2：查询其他 workspace 的连接，即使 repository 里有该连接的事件，Service.Events
	// 必须先通过 findConnection 确认归属，发现不属于当前 workspace 后返回空列表，不能泄露数据。
	events, err = svc.Events(context.Background(), "user1", "conn-b", 100)
	if err != nil {
		t.Fatalf("查询他人 workspace 连接应返回空列表而非 error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("IDOR 漏洞：跨 workspace 查询 conn-b 不应返回任何事件，实际返回了 %d 条: %+v", len(events), events)
	}

	// 场景 3：查询不存在的连接，应返回空列表。
	events, err = svc.Events(context.Background(), "user1", "conn-nonexist", 100)
	if err != nil {
		t.Fatalf("查询不存在连接应返回空列表而非 error: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("不存在的连接不应返回事件，实际返回了 %d 条: %+v", len(events), events)
	}
}

// newProbeTestService 搭建一个可执行真实 ProbeConnection 流程的 Service：一条连接、一条
// 启用策略、两个启用模型目标（model-a、model-b），探活请求统一打到本地 httptest server，
// 固定返回 200 OK，避免访问外部网络。调用方需要在测试结束时 defer server.Close()。
func newProbeTestService(t *testing.T) (*Service, *fakeRepository, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))

	repo := newFakeRepository()
	repo.policies = []Policy{
		{
			ID: "policy-1", UserID: "user1", AdminAccountID: "ws1", Name: "policy-one", Enabled: true,
			DailyProbeBudget: 1000,
			ModelTargets: []ModelTarget{
				{ID: "target-a", PolicyID: "policy-1", ModelName: "model-a", ProviderFamily: ProviderOpenAI, Enabled: true, MaxProbeTokens: 1},
				{ID: "target-b", PolicyID: "policy-1", ModelName: "model-b", ProviderFamily: ProviderOpenAI, Enabled: true, MaxProbeTokens: 1},
			},
		},
	}
	mySites := fakeMySitesReader{
		connections: []my_sites.RealConnection{
			{ID: "conn-1", UserID: "user1", WorkspaceAdminAccountID: "ws1", UpstreamSiteID: "site-1", OwnGroupIDs: []string{"g1"}, UpstreamKey: "gateway-key"},
		},
	}
	svc := &Service{
		repo: repo, mySites: mySites, sites: fakeSiteLookup{site: &upstream.Site{ID: "site-1", BaseURL: server.URL}},
		accounts: fakeAdminAccountResolver{id: "ws1"}, dispatcher: noopRemoteActionRunner{}, probeRunner: NewRealProbeRunner(),
	}
	return svc, repo, server
}

func probedModelNames(results []ModelHealth) []string {
	names := make([]string, 0, len(results))
	for _, r := range results {
		names = append(names, r.ModelName)
	}
	slices.Sort(names)
	return names
}

// TestProbeConnection_NoBodyProbesAllMatchingModels 验证请求体为空（models 未指定）时保持
// 旧行为：探活该连接匹配到的全部启用模型目标。
func TestProbeConnection_NoBodyProbesAllMatchingModels(t *testing.T) {
	svc, _, server := newProbeTestService(t)
	defer server.Close()

	results, err := svc.ProbeConnection(context.Background(), "user1", "conn-1", ProbeConnectionInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := probedModelNames(results); !slices.Equal(got, []string{"model-a", "model-b"}) {
		t.Fatalf("expected all matching models probed, got %v", got)
	}
}

// TestProbeConnection_SingleModelOnlyProbesThatModel 验证传入单个模型时只探活该模型。
func TestProbeConnection_SingleModelOnlyProbesThatModel(t *testing.T) {
	svc, _, server := newProbeTestService(t)
	defer server.Close()

	results, err := svc.ProbeConnection(context.Background(), "user1", "conn-1", ProbeConnectionInput{Models: []string{"model-a"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := probedModelNames(results); !slices.Equal(got, []string{"model-a"}) {
		t.Fatalf("expected only model-a probed, got %v", got)
	}
}

// TestProbeConnection_MultipleModelsOnlyProbesThose 验证传入多个模型时只探活这些模型。
func TestProbeConnection_MultipleModelsOnlyProbesThose(t *testing.T) {
	svc, _, server := newProbeTestService(t)
	defer server.Close()

	results, err := svc.ProbeConnection(context.Background(), "user1", "conn-1", ProbeConnectionInput{Models: []string{"model-a", "model-b"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := probedModelNames(results); !slices.Equal(got, []string{"model-a", "model-b"}) {
		t.Fatalf("expected model-a and model-b probed, got %v", got)
	}
}

// TestProbeConnection_UnmatchedModelsReturnError 验证传入不匹配当前连接策略的模型时，
// 不得探活，必须返回前端可识别的业务错误（ErrorNoMatchingModels），而不是静默探活全部
// 或返回可能被误读为"探活完成但为空"的成功空结果。
func TestProbeConnection_UnmatchedModelsReturnError(t *testing.T) {
	svc, repo, server := newProbeTestService(t)
	defer server.Close()

	results, err := svc.ProbeConnection(context.Background(), "user1", "conn-1", ProbeConnectionInput{Models: []string{"model-not-configured"}})
	if err == nil {
		t.Fatalf("expected error for unmatched model, got results=%v", results)
	}
	if err.Error() != ErrorNoMatchingModels {
		t.Fatalf("expected ErrorNoMatchingModels, got %v", err)
	}
	if len(repo.events) != 0 {
		t.Fatalf("expected no probe to have executed, but %d events were recorded", len(repo.events))
	}
}

// TestProbeConnection_CannotProbeModelOutsidePolicyEvenIfMixedWithMatched 验证请求同时
// 混入策略内和策略外模型名时，只探活策略内命中的模型，绕过策略配置探活未授权模型的请求
// 会被安全地忽略而不是被放行。
func TestProbeConnection_CannotProbeModelOutsidePolicyEvenIfMixedWithMatched(t *testing.T) {
	svc, _, server := newProbeTestService(t)
	defer server.Close()

	results, err := svc.ProbeConnection(context.Background(), "user1", "conn-1", ProbeConnectionInput{Models: []string{"model-a", "model-outside-policy"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := probedModelNames(results); !slices.Equal(got, []string{"model-a"}) {
		t.Fatalf("expected only model-a (in-policy) probed, unauthorized model must not be probed, got %v", got)
	}
}

// TestProbeConnection_CannotProbeConnectionFromOtherWorkspace 验证不能跨 workspace 探活：
// 用户请求探活一个不属于自己当前 workspace 的 connectionId 时，必须返回 not found，不能
// 借助手动探活越权触达其他 workspace 的连接。
func TestProbeConnection_CannotProbeConnectionFromOtherWorkspace(t *testing.T) {
	svc, repo, server := newProbeTestService(t)
	defer server.Close()
	// 追加另一个 workspace（ws2）拥有的连接，user1 当前 workspace 是 ws1，看不到它。
	repo.policies = append(repo.policies, Policy{
		ID: "policy-2", UserID: "user2", AdminAccountID: "ws2", Name: "policy-two", Enabled: true, DailyProbeBudget: 1000,
		ModelTargets: []ModelTarget{{ID: "target-c", PolicyID: "policy-2", ModelName: "model-c", ProviderFamily: ProviderOpenAI, Enabled: true, MaxProbeTokens: 1}},
	})

	results, err := svc.ProbeConnection(context.Background(), "user1", "conn-not-owned", ProbeConnectionInput{Models: []string{"model-c"}})
	if err == nil {
		t.Fatalf("expected not found error for unowned connection, got results=%v", results)
	}
	if err.Error() != ErrorNotFound {
		t.Fatalf("expected ErrorNotFound, got %v", err)
	}
}
