package connection_health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"transithub/backend/internal/modules/my_sites"
	"transithub/backend/internal/modules/upstream"
)

// TestManualProbeTarget_EmptyModelsRejected 验证 models 为空时明确拒绝，不静默退化成
// "探活全部候选"（手动一次性探活没有候选池概念，必须由用户显式勾选）。
func TestManualProbeTarget_EmptyModelsRejected(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := newAdminGroupsService(fakePlatformGroupReader{}, mySites, repo)

	_, err := svc.ManualProbeTarget(context.Background(), "user1", "newapi:ws1:100", nil)
	if err == nil || err.Error() != ErrorManualModelsRequired {
		t.Fatalf("expected ErrorManualModelsRequired, got %v", err)
	}
	_, err = svc.ManualProbeTarget(context.Background(), "user1", "newapi:ws1:100", []string{"  ", ""})
	if err == nil || err.Error() != ErrorManualModelsRequired {
		t.Fatalf("expected ErrorManualModelsRequired for whitespace-only models, got %v", err)
	}
}

// TestManualProbeTarget_RejectsForeignWorkspaceTarget 验证跨 workspace targetId 被拒绝。
func TestManualProbeTarget_RejectsForeignWorkspaceTarget(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := newAdminGroupsService(fakePlatformGroupReader{}, mySites, repo)

	_, err := svc.ManualProbeTarget(context.Background(), "user1", "newapi:ws2:100", []string{"gpt-4o"})
	if err == nil || err.Error() != ErrorProbeTargetNotFound {
		t.Fatalf("expected target not found for foreign workspace, got %v", err)
	}
}

// TestManualProbeTarget_CredentialUnavailableReturnsStructuredError 验证凭据解析失败时返回
// 结构化错误，且不执行任何探活请求。
func TestManualProbeTarget_CredentialUnavailableReturnsStructuredError(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: "https://up", Models: "gpt-4o"}}},
		credErr:       map[string]error{"100": &upstream.ProbeCredentialError{Reason: upstream.ReasonBaseURLUnavailable}},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	results, err := svc.ManualProbeTarget(context.Background(), "user1", "newapi:ws1:100", []string{"gpt-4o"})
	if err == nil || err.Error() != ErrorBaseURLUnavailable {
		t.Fatalf("expected base_url_unavailable error, got results=%v err=%v", results, err)
	}
}

// TestManualProbeTarget_SuccessDoesNotTouchStateOrEvents 核心隔离验证：手动一次性探活成功执行后，
// 既不写 connection_health_states 也不写 connection_health_events、不消耗探活预算——
// 与旧 ProbeTarget（会落库状态/事件）形成对照。
func TestManualProbeTarget_SuccessDoesNotTouchStateOrEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: server.URL, Models: "model-a,model-b"}}},
		credByAccount: map[string]upstream.ProbeCredential{"100": {BaseURL: server.URL, Key: "secret-key"}},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	results, err := svc.ManualProbeTarget(context.Background(), "user1", "newapi:ws1:100", []string{"model-a", "model-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 transient results, got %+v", results)
	}
	for _, r := range results {
		if !r.Healthy || r.Result != string(ResultOK) {
			t.Fatalf("expected healthy ok result, got %+v", r)
		}
		if r.ProbedAt.IsZero() {
			t.Fatalf("expected probedAt to be set, got %+v", r)
		}
	}
	if len(repo.states) != 0 {
		t.Fatalf("manual one-time probe must not write any state, got %+v", repo.states)
	}
	if len(repo.events) != 0 {
		t.Fatalf("manual one-time probe must not write any event, got %+v", repo.events)
	}
}

// TestManualProbeTarget_FailureResultIncludesRedactedDetail 验证探活失败时结果携带脱敏后的
// errorKey/errorDetail（不含明文 key），且仍不落库。
func TestManualProbeTarget_FailureResultIncludesRedactedDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid key secret-key-value"}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: server.URL, Models: "model-a"}}},
		credByAccount: map[string]upstream.ProbeCredential{"100": {BaseURL: server.URL, Key: "secret-key-value"}},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	results, err := svc.ManualProbeTarget(context.Background(), "user1", "newapi:ws1:100", []string{"model-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Healthy || results[0].ErrorKey != string(ResultAuth) {
		t.Fatalf("expected unhealthy auth result, got %+v", results)
	}
	if results[0].ErrorDetail == "" {
		t.Fatalf("expected error detail to be populated")
	}
	if strings.Contains(results[0].ErrorDetail, "secret-key-value") {
		t.Fatalf("error detail leaked the plaintext key: %q", results[0].ErrorDetail)
	}
	if len(repo.states) != 0 || len(repo.events) != 0 {
		t.Fatalf("failed manual probe must still not write state/events, states=%v events=%v", repo.states, repo.events)
	}
}

// panicIfCalledRemoteActionRunner is a RemoteActionRunner that panics on any invocation,
// used to prove manual one-time probing never reaches the remote-action dispatcher at all
// (not even indirectly through a code path that would call it and swallow the result).
type panicIfCalledRemoteActionRunner struct{}

func (panicIfCalledRemoteActionRunner) Degrade(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (string, error) {
	panic("Degrade must never be called by manual one-time probing")
}

func (panicIfCalledRemoteActionRunner) Restore(ctx context.Context, conn my_sites.RealConnection, state ConnectionHealthState) (string, error) {
	panic("Restore must never be called by manual one-time probing")
}

func (panicIfCalledRemoteActionRunner) DegradeTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (string, error) {
	panic("DegradeTarget must never be called by manual one-time probing")
}

func (panicIfCalledRemoteActionRunner) RestoreTarget(ctx context.Context, session upstream.Session, target AdminProbeTarget, state ConnectionHealthState) (string, error) {
	panic("RestoreTarget must never be called by manual one-time probing")
}

// TestManualProbeTarget_NeverRunsRemoteAction 验证手动一次性探活即使遭遇会在策略路径触发
// 远端动作的硬失败（如 401），也绝不调用 dispatcher（用一个"任何方法被调用就 panic"的
// RemoteActionRunner 兜底验证），也仍然不写 state/event。
func TestManualProbeTarget_NeverRunsRemoteAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "acc-1", Name: "acc", Models: "gpt-4o"}}},
		credByAccount: map[string]upstream.ProbeCredential{"acc-1": {BaseURL: server.URL, Key: "k"}},
	}
	svc := &Service{
		repo: repo, mySites: mySites, accounts: fakeAdminAccountResolver{id: "ws1"},
		dispatcher: panicIfCalledRemoteActionRunner{}, probeRunner: NewRealProbeRunner(), platformGroups: reader,
	}

	results, err := svc.ManualProbeTarget(context.Background(), "user1", "sub2api:ws1:acc-1", []string{"gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Healthy {
		t.Fatalf("expected one unhealthy transient result, got %+v", results)
	}
	if len(repo.states) != 0 || len(repo.events) != 0 {
		t.Fatalf("manual one-time probe must not write any state/event even on hard failure, states=%v events=%v", repo.states, repo.events)
	}
}
