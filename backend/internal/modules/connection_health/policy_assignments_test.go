package connection_health

import (
	"context"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

// TestSetTargetPolicyAssignments_CreateThenGetRoundTrips 验证分配创建后 GET 能读回同样的
// policyIds，且携带策略名称/启用状态摘要。
func TestSetTargetPolicyAssignments_CreateThenGetRoundTrips(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := newAdminGroupsService(fakePlatformGroupReader{}, mySites, repo)

	saved, err := svc.SetTargetPolicyAssignments(context.Background(), "user1", "newapi:ws1:100", []string{"policy-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved.PolicyIDs) != 1 || saved.PolicyIDs[0] != "policy-1" {
		t.Fatalf("expected 1 assigned policy id, got %+v", saved)
	}
	if len(saved.Policies) != 1 || saved.Policies[0].PolicyName != "p" || !saved.Policies[0].Enabled {
		t.Fatalf("expected policy summary with name/enabled, got %+v", saved.Policies)
	}

	fetched, err := svc.GetTargetPolicyAssignments(context.Background(), "user1", "newapi:ws1:100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(fetched.PolicyIDs) != 1 || fetched.PolicyIDs[0] != "policy-1" {
		t.Fatalf("expected GET to round-trip the assignment, got %+v", fetched)
	}
}

// TestSetTargetPolicyAssignments_EmptyClearsAssignments 验证传入空 policyIds 清空该 target 的分配。
func TestSetTargetPolicyAssignments_EmptyClearsAssignments(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := newAdminGroupsService(fakePlatformGroupReader{}, mySites, repo)

	if _, err := svc.SetTargetPolicyAssignments(context.Background(), "user1", "newapi:ws1:100", []string{"policy-1"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cleared, err := svc.SetTargetPolicyAssignments(context.Background(), "user1", "newapi:ws1:100", nil)
	if err != nil {
		t.Fatalf("unexpected error clearing assignments: %v", err)
	}
	if len(cleared.PolicyIDs) != 0 {
		t.Fatalf("expected assignments cleared, got %+v", cleared)
	}
}

// TestSetTargetPolicyAssignments_RejectsForeignWorkspacePolicy 验证不能分配不属于当前 workspace
// （或不存在）的策略——即便某个用户的另一个 workspace 里确实存在同名 policyId。
func TestSetTargetPolicyAssignments_RejectsForeignWorkspacePolicy(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{
		{ID: "policy-ws2", UserID: "user1", AdminAccountID: "ws2", Name: "other workspace policy", Enabled: true},
	}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := newAdminGroupsService(fakePlatformGroupReader{}, mySites, repo)

	_, err := svc.SetTargetPolicyAssignments(context.Background(), "user1", "newapi:ws1:100", []string{"policy-ws2"})
	if err == nil || err.Error() != ErrorPolicyNotFound {
		t.Fatalf("expected ErrorPolicyNotFound for foreign-workspace policy, got %v", err)
	}
	// 拒绝时不应留下任何部分写入的分配。
	assignments, listErr := repo.ListPolicyAssignmentsForTarget(context.Background(), "user1", "ws1", "newapi:ws1:100")
	if listErr != nil {
		t.Fatalf("unexpected error: %v", listErr)
	}
	if len(assignments) != 0 {
		t.Fatalf("expected no assignment persisted after rejection, got %+v", assignments)
	}
}

// TestSetTargetPolicyAssignments_RejectsForeignWorkspaceTarget 验证 targetId 本身跨 workspace 时拒绝。
func TestSetTargetPolicyAssignments_RejectsForeignWorkspaceTarget(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := newAdminGroupsService(fakePlatformGroupReader{}, mySites, repo)

	_, err := svc.SetTargetPolicyAssignments(context.Background(), "user1", "newapi:ws2:100", []string{"policy-1"})
	if err == nil || err.Error() != ErrorProbeTargetNotFound {
		t.Fatalf("expected target not found for foreign workspace targetId, got %v", err)
	}
}

// TestSetTargetPolicyAssignments_DedupesPolicyIDs 验证重复的 policyId 只保留一份。
func TestSetTargetPolicyAssignments_DedupesPolicyIDs(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := newAdminGroupsService(fakePlatformGroupReader{}, mySites, repo)

	saved, err := svc.SetTargetPolicyAssignments(context.Background(), "user1", "newapi:ws1:100", []string{"policy-1", "policy-1", " policy-1 "})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(saved.PolicyIDs) != 1 {
		t.Fatalf("expected deduped to 1 policy id, got %+v", saved.PolicyIDs)
	}
}
