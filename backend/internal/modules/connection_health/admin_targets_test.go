package connection_health

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

// newAdminTargetsRemoteActionService 构造一个用真实 remoteActionDispatcher（而不是
// noopRemoteActionRunner）驱动的 Service，供本文件测试断言 probeTargetOnce 触发的真实远端动作调用。
func newAdminTargetsRemoteActionService(reader PlatformGroupReader, mySites MySitesReader, repo *fakeRepository, platform *fakePlatformActioner) *Service {
	return &Service{
		repo:           repo,
		mySites:        mySites,
		accounts:       fakeAdminAccountResolver{id: "ws1"},
		dispatcher:     newRemoteActionDispatcher(fakeSiteLookup{}, fakeSessionProvider{}, platform),
		probeRunner:    NewRealProbeRunner(),
		platformGroups: reader,
	}
}

// sub2APIProbePolicy 返回一条启用策略：自动降级开启，自动远端动作按参数控制。
func sub2APIProbePolicy(autoRemoteAction bool) Policy {
	return Policy{
		ID: "policy-1", UserID: "user1", AdminAccountID: "ws1", Name: "p", Enabled: true, DailyProbeBudget: 1000,
		AutoDegradeEnabled: true, AutoRemoteActionEnabled: autoRemoteAction,
		FailureThreshold: 3, SuccessThreshold: 2, CooldownSeconds: 300, ObservationSeconds: 300, RecoveryStepPercent: 25,
		ModelTargets: []ModelTarget{{ID: "t1", PolicyID: "policy-1", ModelName: "gpt-4o", ProviderFamily: ProviderOpenAI, Enabled: true, MaxProbeTokens: 1}},
	}
}

// TestProbeTargetOnce_Sub2APIAutoRemoteDegradeUpdatesInactive 验证 AutoRemoteActionEnabled=true
// 时，sub2api target 探活遭遇硬失败（触发 TriggerRemoteDegrade）会真实调用
// UpdateSub2APIAdminAccountStatus(session, target.AccountID, "inactive")，state/event 的
// remoteAction 记录为 sub2api_account_status_inactive。
func TestProbeTargetOnce_Sub2APIAutoRemoteDegradeUpdatesInactive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	repo.policies = []Policy{sub2APIProbePolicy(true)}
	platform := &fakePlatformActioner{}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "acc-1", Name: "acc", Models: "gpt-4o"}}},
		credByAccount: map[string]upstream.ProbeCredential{"acc-1": {BaseURL: server.URL, Key: "k"}},
	}
	svc := newAdminTargetsRemoteActionService(reader, mySites, repo, platform)

	targetID := "sub2api:ws1:acc-1"
	results, err := svc.ProbeTarget(context.Background(), "user1", targetID, []string{"gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].State != StateSuspended {
		t.Fatalf("expected hard failure to suspend immediately, got %+v", results)
	}
	if len(platform.sub2APICalls) != 1 || platform.sub2APICalls[0].accountID != "acc-1" || platform.sub2APICalls[0].status != "inactive" {
		t.Fatalf("expected one call accountID=acc-1 status=inactive, got %+v", platform.sub2APICalls)
	}
	st := repo.states[targetID]["gpt-4o"]
	if st.LastRemoteAction != RemoteActionSub2APIStatusInactive {
		t.Fatalf("expected state.LastRemoteAction=%s, got %q", RemoteActionSub2APIStatusInactive, st.LastRemoteAction)
	}
	if len(repo.events) != 1 || repo.events[0].RemoteAction != RemoteActionSub2APIStatusInactive {
		t.Fatalf("expected event.RemoteAction=%s, got %+v", RemoteActionSub2APIStatusInactive, repo.events)
	}
}

// TestProbeTargetOnce_Sub2APIAutoRemoteRestoreUpdatesActive 验证从 observing 状态达到成功阈值时
// （触发 TriggerRemoteRestore），真实调用 UpdateSub2APIAdminAccountStatus(session,
// target.AccountID, "active")，state/event 的 remoteAction 记录为 sub2api_account_status_active。
func TestProbeTargetOnce_Sub2APIAutoRemoteRestoreUpdatesActive(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	repo.policies = []Policy{sub2APIProbePolicy(true)}
	targetID := "sub2api:ws1:acc-1"
	observingUntil := time.Now().Add(-1 * time.Minute)
	repo.states[targetID] = map[string]ConnectionHealthState{
		"gpt-4o": {
			ConnectionID: targetID, ModelName: "gpt-4o", UserID: "user1", AdminAccountID: "ws1",
			State: StateObserving, ConsecutiveSuccesses: 1, ObservingUntil: &observingUntil, CurrentWeight: 0,
			LastRemoteAction: RemoteActionSub2APIStatusInactive,
		},
	}
	platform := &fakePlatformActioner{}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "acc-1", Name: "acc", Status: "inactive", Models: "gpt-4o"}}},
		credByAccount: map[string]upstream.ProbeCredential{"acc-1": {BaseURL: server.URL, Key: "k"}},
	}
	svc := newAdminTargetsRemoteActionService(reader, mySites, repo, platform)

	results, err := svc.ProbeTarget(context.Background(), "user1", targetID, []string{"gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].State != StateRecovering {
		t.Fatalf("expected transition to recovering after success threshold, got %+v", results)
	}
	if len(platform.sub2APICalls) != 1 || platform.sub2APICalls[0].accountID != "acc-1" || platform.sub2APICalls[0].status != "active" {
		t.Fatalf("expected one call accountID=acc-1 status=active, got %+v", platform.sub2APICalls)
	}
	st := repo.states[targetID]["gpt-4o"]
	if st.LastRemoteAction != RemoteActionSub2APIStatusActive {
		t.Fatalf("expected state.LastRemoteAction=%s, got %q", RemoteActionSub2APIStatusActive, st.LastRemoteAction)
	}
}

// TestProbeTargetOnce_Sub2APIAutoRemoteDegradeFailureRecordsFailedAction 验证远端降级调用失败时
// （UpdateSub2APIAdminAccountStatus 返回 error），state/event 的 remoteAction 记录为
// sub2api_account_status_inactive_failed，绝不能回退成 unsupported——sub2api 已经支持这个
// 动作，真的发起了调用只是失败了，和「平台不支持」是两回事。
func TestProbeTargetOnce_Sub2APIAutoRemoteDegradeFailureRecordsFailedAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	repo.policies = []Policy{sub2APIProbePolicy(true)}
	platform := &fakePlatformActioner{sub2APIErr: errors.New("upstream 500")}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "acc-1", Name: "acc", Models: "gpt-4o"}}},
		credByAccount: map[string]upstream.ProbeCredential{"acc-1": {BaseURL: server.URL, Key: "k"}},
	}
	svc := newAdminTargetsRemoteActionService(reader, mySites, repo, platform)

	targetID := "sub2api:ws1:acc-1"
	results, err := svc.ProbeTarget(context.Background(), "user1", targetID, []string{"gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].State != StateSuspended {
		t.Fatalf("expected hard failure to suspend, got %+v", results)
	}
	st := repo.states[targetID]["gpt-4o"]
	if st.LastRemoteAction != RemoteActionSub2APIStatusInactiveFailed {
		t.Fatalf("expected state.LastRemoteAction=%s, got %q", RemoteActionSub2APIStatusInactiveFailed, st.LastRemoteAction)
	}
	if len(repo.events) != 1 || repo.events[0].RemoteAction != RemoteActionSub2APIStatusInactiveFailed {
		t.Fatalf("expected event.RemoteAction=%s, got %+v", RemoteActionSub2APIStatusInactiveFailed, repo.events)
	}
}

// TestProbeTargetOnce_Sub2APIAutoRemoteRestoreFailureRecordsFailedAction 验证远端恢复调用失败
// 时，state/event 记录 sub2api_account_status_active_failed，不能回退成 unsupported。
func TestProbeTargetOnce_Sub2APIAutoRemoteRestoreFailureRecordsFailedAction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	repo.policies = []Policy{sub2APIProbePolicy(true)}
	targetID := "sub2api:ws1:acc-1"
	observingUntil := time.Now().Add(-1 * time.Minute)
	repo.states[targetID] = map[string]ConnectionHealthState{
		"gpt-4o": {
			ConnectionID: targetID, ModelName: "gpt-4o", UserID: "user1", AdminAccountID: "ws1",
			State: StateObserving, ConsecutiveSuccesses: 1, ObservingUntil: &observingUntil, CurrentWeight: 0,
			LastRemoteAction: RemoteActionSub2APIStatusInactive,
		},
	}
	platform := &fakePlatformActioner{sub2APIErr: errors.New("upstream 500")}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "acc-1", Name: "acc", Status: "inactive", Models: "gpt-4o"}}},
		credByAccount: map[string]upstream.ProbeCredential{"acc-1": {BaseURL: server.URL, Key: "k"}},
	}
	svc := newAdminTargetsRemoteActionService(reader, mySites, repo, platform)

	results, err := svc.ProbeTarget(context.Background(), "user1", targetID, []string{"gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].State != StateRecovering {
		t.Fatalf("expected transition to recovering, got %+v", results)
	}
	st := repo.states[targetID]["gpt-4o"]
	if st.LastRemoteAction != RemoteActionSub2APIStatusActiveFailed {
		t.Fatalf("expected state.LastRemoteAction=%s, got %q", RemoteActionSub2APIStatusActiveFailed, st.LastRemoteAction)
	}
}

// TestProbeTargetOnce_Sub2APIRealPlatformServiceComboDegradeSucceeds 是覆盖「PlatformService
// 单测通过、dispatcher fake 单测通过，但真实组合路径失败」这类盲区的端到端测试：
// 用同一个 httptest.Server 同时模拟探活端点（返回 500 触发 healthy -> suspended）和 sub2api
// admin accounts 的 GET/PUT，dispatcher 的 PlatformActioner 用真实 *upstream.PlatformService
// （不是 fake），断言最终状态/事件里的 remoteAction 是 sub2api_account_status_inactive，
// 且 PUT 请求体确实把 status 改成了 inactive。
func TestProbeTargetOnce_Sub2APIRealPlatformServiceComboDegradeSucceeds(t *testing.T) {
	var putBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/admin/accounts/acc-1":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{"id": "acc-1", "name": "acc", "status": "active"},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/admin/accounts/acc-1":
			if err := json.NewDecoder(r.Body).Decode(&putBody); err != nil {
				t.Fatalf("failed to decode PUT body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	realPlatform := upstream.NewPlatformService(upstream.NewHTTPClient(server.Client()))
	repo := newFakeRepository()
	repo.policies = []Policy{sub2APIProbePolicy(true)}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API, BaseURL: server.URL, AccessToken: "token-1", TokenType: "Bearer"}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "acc-1", Name: "acc", Models: "gpt-4o"}}},
		credByAccount: map[string]upstream.ProbeCredential{"acc-1": {BaseURL: server.URL, Key: "probe-key"}},
	}
	svc := &Service{
		repo: repo, mySites: mySites, accounts: fakeAdminAccountResolver{id: "ws1"},
		dispatcher:     newRemoteActionDispatcher(fakeSiteLookup{}, fakeSessionProvider{}, realPlatform),
		probeRunner:    NewRealProbeRunner(),
		platformGroups: reader,
	}

	targetID := "sub2api:ws1:acc-1"
	results, err := svc.ProbeTarget(context.Background(), "user1", targetID, []string{"gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].State != StateSuspended {
		t.Fatalf("expected hard failure to suspend, got %+v", results)
	}

	st := repo.states[targetID]["gpt-4o"]
	if st.LastRemoteAction != RemoteActionSub2APIStatusInactive {
		t.Fatalf("expected state.LastRemoteAction=%s, got %q", RemoteActionSub2APIStatusInactive, st.LastRemoteAction)
	}
	if len(repo.events) != 1 || repo.events[0].RemoteAction != RemoteActionSub2APIStatusInactive {
		t.Fatalf("expected event.RemoteAction=%s, got %+v", RemoteActionSub2APIStatusInactive, repo.events)
	}
	if putBody == nil {
		t.Fatalf("expected a real PUT request to the sub2api admin accounts API")
	}
	if putBody["status"] != "inactive" {
		t.Fatalf("expected PUT body status=inactive, got %+v", putBody)
	}
}

// TestProbeTargetOnce_Sub2APIRemoteActionDisabledSkipsUpstream 验证 AutoRemoteActionEnabled=false
// 时，即使状态机触发远端动作，也绝不调用 sub2api PUT 接口，只记录 skipped_independent_probe。
func TestProbeTargetOnce_Sub2APIRemoteActionDisabledSkipsUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	repo := newFakeRepository()
	repo.policies = []Policy{sub2APIProbePolicy(false)}
	platform := &fakePlatformActioner{}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformSub2API}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "acc-1", Name: "acc", Models: "gpt-4o"}}},
		credByAccount: map[string]upstream.ProbeCredential{"acc-1": {BaseURL: server.URL, Key: "k"}},
	}
	svc := newAdminTargetsRemoteActionService(reader, mySites, repo, platform)

	targetID := "sub2api:ws1:acc-1"
	results, err := svc.ProbeTarget(context.Background(), "user1", targetID, []string{"gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].State != StateSuspended {
		t.Fatalf("expected hard failure to suspend, got %+v", results)
	}
	if len(platform.sub2APICalls) != 0 {
		t.Fatalf("expected no upstream call when AutoRemoteActionEnabled=false, got %+v", platform.sub2APICalls)
	}
	st := repo.states[targetID]["gpt-4o"]
	if st.LastRemoteAction != RemoteActionSkippedIndependentProbe {
		t.Fatalf("expected LastRemoteAction=%s, got %q", RemoteActionSkippedIndependentProbe, st.LastRemoteAction)
	}
}
