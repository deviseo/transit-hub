package connection_health

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

// fakePlatformGroupReader 是 PlatformGroupReader 的内存实现：按分组 ID 返回预置账号列表，
// 可为指定分组注入读取错误；ResolveProbeCredential 可注入凭据或不可探活错误。
type fakePlatformGroupReader struct {
	groups        []upstream.AdminGroupInfo
	accountsByGrp map[string][]upstream.AdminGroupAccountInfo
	errByGrp      map[string]error
	credByAccount map[string]upstream.ProbeCredential
	credErr       map[string]error
}

func (f fakePlatformGroupReader) FetchAdminAllGroups(session upstream.Session) ([]upstream.AdminGroupInfo, error) {
	return f.groups, nil
}

func (f fakePlatformGroupReader) ListAdminGroupAccounts(session upstream.Session, group upstream.AdminGroupInfo) ([]upstream.AdminGroupAccountInfo, error) {
	if err, ok := f.errByGrp[group.ID]; ok {
		return nil, err
	}
	return f.accountsByGrp[group.ID], nil
}

func (f fakePlatformGroupReader) ResolveProbeCredential(session upstream.Session, account upstream.AdminGroupAccountInfo) (upstream.ProbeCredential, error) {
	if err, ok := f.credErr[account.ID]; ok {
		return upstream.ProbeCredential{}, err
	}
	return f.credByAccount[account.ID], nil
}

func newAdminGroupsService(reader PlatformGroupReader, mySites MySitesReader, repo *fakeRepository) *Service {
	return &Service{
		repo:           repo,
		mySites:        mySites,
		accounts:       fakeAdminAccountResolver{id: "ws1"},
		dispatcher:     noopRemoteActionRunner{},
		probeRunner:    NewRealProbeRunner(),
		platformGroups: reader,
	}
}

// probePolicy 返回一条启用策略，含一个启用的 gpt-4o 模型目标，供候选模型/可探活判断使用。
func probePolicy() Policy {
	return Policy{
		ID: "policy-1", UserID: "user1", AdminAccountID: "ws1", Name: "p", Enabled: true, DailyProbeBudget: 1000,
		ModelTargets: []ModelTarget{{ID: "t1", PolicyID: "policy-1", ModelName: "gpt-4o", ProviderFamily: ProviderOpenAI, Enabled: true, MaxProbeTokens: 1}},
	}
}

// TestAdminGroups_TargetIDProbeAvailableAndModelHealth 验证独立探活主列表：
//   - 每个账号/渠道生成稳定 targetId。
//   - 有候选模型 + 有 base_url 的 new-api channel 标记可探活，并叠加以 targetId 为键的探活状态。
//   - 缺 base_url 的 channel 标记不可探活，原因 base_url_unavailable。
//   - 探活字段来自独立探活状态，不依赖 real_connections（connectionId）。
func TestAdminGroups_TargetIDProbeAvailableAndModelHealth(t *testing.T) {
	repo := newFakeRepository()
	policy := probePolicy()
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: policy.ID,
	}}
	// 独立探活状态：targetId = newapi:ws1:100，model gpt-4o = healthy。
	repo.states["newapi:ws1:100"] = map[string]ConnectionHealthState{
		"gpt-4o":        {ConnectionID: "newapi:ws1:100", ModelName: "gpt-4o", UserID: "user1", AdminAccountID: "ws1", State: StateHealthy, CurrentWeight: 100},
		"removed-model": {ConnectionID: "newapi:ws1:100", ModelName: "removed-model", UserID: "user1", AdminAccountID: "ws1", State: StateSuspended, CurrentWeight: 0},
	}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip", Platform: "newapi", Status: "active"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {
				{ID: "100", Name: "ch-ok", Models: "gpt-4o", BaseURL: "https://up.example.com"}, // 可探活
				{ID: "200", Name: "ch-nobaseurl", Models: "gpt-4o"},                             // 缺 base_url
			},
		},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	groups, err := svc.AdminGroups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(groups) != 1 || len(groups[0].Accounts) != 2 {
		t.Fatalf("expected 1 group with 2 accounts, got %+v", groups)
	}

	var ok, noBase *AdminGroupAccount
	for i := range groups[0].Accounts {
		switch groups[0].Accounts[i].ID {
		case "100":
			ok = &groups[0].Accounts[i]
		case "200":
			noBase = &groups[0].Accounts[i]
		}
	}
	if ok == nil || noBase == nil {
		t.Fatalf("missing accounts")
	}
	if ok.TargetID != "newapi:ws1:100" {
		t.Fatalf("unexpected targetId: %q", ok.TargetID)
	}
	if !ok.ProbeAvailable || ok.ProbeUnavailableReason != "" {
		t.Fatalf("account 100 should be probe-available, got available=%v reason=%q", ok.ProbeAvailable, ok.ProbeUnavailableReason)
	}
	if len(ok.ModelHealth) != 1 || ok.ModelHealth[0].State != StateHealthy {
		t.Fatalf("expected overlaid healthy model, got %+v", ok.ModelHealth)
	}
	if groups[0].HealthSummary.SuspendedModels != 0 {
		t.Fatalf("removed model state must not affect summary: %+v", groups[0].HealthSummary)
	}
	if noBase.ProbeAvailable || noBase.ProbeUnavailableReason != upstream.ReasonBaseURLUnavailable {
		t.Fatalf("account 200 should be base_url_unavailable, got available=%v reason=%q", noBase.ProbeAvailable, noBase.ProbeUnavailableReason)
	}
	if groups[0].HealthSummary.ProbeableAccounts != 1 || groups[0].HealthSummary.UnprobeableAccounts != 1 {
		t.Fatalf("unexpected summary: %+v", groups[0].HealthSummary)
	}
	if groups[0].HealthSummary.HealthyModels != 1 {
		t.Fatalf("healthyModels = %d, want 1", groups[0].HealthSummary.HealthyModels)
	}
}

// TestAdminGroups_NoPolicyStillAllowsManualProbe 验证尚未创建策略时，只要静态凭据条件满足，
// 目标仍可进入一次性手动探活并实时发现模型；自动调度仍会因没有策略而跳过。
func TestAdminGroups_NoPolicyStillAllowsManualProbe(t *testing.T) {
	repo := newFakeRepository() // 无策略
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: "https://up", Models: "gpt-4o"}}},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	groups, err := svc.AdminGroups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	acc := groups[0].Accounts[0]
	if !acc.ProbeAvailable || acc.ProbeUnavailableReason != "" {
		t.Fatalf("expected manual probe to remain available, got available=%v reason=%q", acc.ProbeAvailable, acc.ProbeUnavailableReason)
	}
}

// TestAdminGroups_SingleGroupAccountsErrorDoesNotBreakList 验证单个分组账号读取失败时，
// 该分组返回 accountCount=0 + AccountsError，其余分组仍正常返回，且不泄露上游错误明文。
func TestAdminGroups_SingleGroupAccountsErrorDoesNotBreakList(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g-ok", Name: "ok"}, {ID: "g-bad", Name: "bad"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g-ok": {{ID: "1", Name: "acc", BaseURL: "https://up", Models: "gpt-4o"}},
		},
		errByGrp: map[string]error{"g-bad": errors.New("upstream 500 secret-detail")},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	groups, err := svc.AdminGroups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("whole list must not fail when one group errors: %v", err)
	}
	byID := map[string]AdminGroupHealth{}
	for _, g := range groups {
		byID[g.ID] = g
	}
	if byID["g-ok"].AccountCount != 1 || byID["g-ok"].AccountsError != "" {
		t.Fatalf("healthy group unexpected: %+v", byID["g-ok"])
	}
	if byID["g-bad"].AccountsError != ErrorAccountsFetch || byID["g-bad"].AccountCount != 0 {
		t.Fatalf("bad group should carry AccountsError and count 0, got %+v", byID["g-bad"])
	}
	encoded, _ := json.Marshal(groups)
	if strings.Contains(string(encoded), "secret-detail") {
		t.Fatalf("raw upstream error leaked into response: %s", encoded)
	}
}

// TestAdminGroups_WorkspaceIsolationAndNoSensitiveFields 验证 workspace 隔离与敏感字段不泄露：
// 其它 workspace 的状态不叠加；响应里不出现 key/token/credentials 等字段。
func TestAdminGroups_WorkspaceIsolationAndNoSensitiveFields(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	// ws2 的状态（AdminAccountID=ws2）不应出现在 ws1 聚合里。
	repo.states["newapi:ws2:100"] = map[string]ConnectionHealthState{
		"gpt-4o": {ConnectionID: "newapi:ws2:100", ModelName: "gpt-4o", UserID: "user1", AdminAccountID: "ws2", State: StateSuspended},
	}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: "https://up", Models: "gpt-4o"}}},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	groups, err := svc.AdminGroups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	acc := groups[0].Accounts[0]
	if len(acc.ModelHealth) != 0 {
		t.Fatalf("ws2 state must not leak into ws1 target, got %+v", acc.ModelHealth)
	}
	if groups[0].HealthSummary.SuspendedModels != 0 {
		t.Fatalf("ws2 suspended must not leak, got %+v", groups[0].HealthSummary)
	}

	encoded, _ := json.Marshal(groups)
	for _, secret := range []string{"credentials", "\"key\"", "token", "cookie", "authorization"} {
		if strings.Contains(strings.ToLower(string(encoded)), strings.ToLower(secret)) {
			t.Fatalf("sensitive field %q leaked into admin-groups response: %s", secret, encoded)
		}
	}
}

// TestAccumulateSummary_PreservesLegacyDegradedTotalAndExposesTransitionStates 验证新增的
// observing/recovering 明细不会改变旧 degradedModels 的聚合语义，已上线旧前端仍可正常统计。
func TestAccumulateSummary_PreservesLegacyDegradedTotalAndExposesTransitionStates(t *testing.T) {
	summary := AdminGroupHealthSummary{}
	accumulateSummary(&summary, []ModelHealth{
		{State: StateDegraded},
		{State: StateObserving},
		{State: StateRecovering},
		{State: StateSuspended},
		{State: StateDisabled},
	})

	if summary.DegradedModels != 3 {
		t.Fatalf("degradedModels = %d, want legacy aggregate 3", summary.DegradedModels)
	}
	if summary.ObservingModels != 1 || summary.RecoveringModels != 1 {
		t.Fatalf("unexpected transition state counts: %+v", summary)
	}
	if summary.SuspendedModels != 1 || summary.DisabledModels != 1 {
		t.Fatalf("unexpected terminal state counts: %+v", summary)
	}
}

func TestAdminGroups_PreservesConfiguredModelsWithoutProbeState(t *testing.T) {
	repo := newFakeRepository()
	policy := probePolicy()
	policy.ModelTargets = append(policy.ModelTargets, ModelTarget{
		ID: "t2", PolicyID: policy.ID, ModelName: "gpt-4.1", ProviderFamily: ProviderOpenAI, Enabled: true,
	})
	repo.policies = []Policy{policy}
	repo.groupAssignments = []GroupPolicyAssignment{{
		UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", AdminGroupName: "vip", PolicyID: policy.ID,
	}}
	repo.states["newapi:ws1:100"] = map[string]ConnectionHealthState{
		"gpt-4o": {ConnectionID: "newapi:ws1:100", ModelName: "gpt-4o", UserID: "user1", AdminAccountID: "ws1", State: StateHealthy, CurrentWeight: 100},
	}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {{ID: "100", BaseURL: "https://up", Models: "gpt-4o,gpt-4.1"}},
		},
	}
	service := newAdminGroupsService(reader, fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, repo)

	groups, err := service.AdminGroups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	account := groups[0].Accounts[0]
	if len(account.ModelHealth) != 1 || account.ModelHealth[0].ModelName != "gpt-4o" {
		t.Fatalf("expected the persisted model state, got %+v", account.ModelHealth)
	}
	if len(account.UnprobedModels) != 1 || account.UnprobedModels[0].ModelName != "gpt-4.1" {
		t.Fatalf("configured model without state must remain visible: %+v", account.UnprobedModels)
	}
	if groups[0].HealthSummary.HealthyModels != 1 || groups[0].HealthSummary.UnconfiguredModels != 1 {
		t.Fatalf("partially-probed models must be counted independently: %+v", groups[0].HealthSummary)
	}
}

func TestOverview_MergesModelsForTargetSharedAcrossGroups(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{
		{ID: "p1", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ModelTargets: []ModelTarget{{ModelName: "model-a", Enabled: true}}},
		{ID: "p2", UserID: "user1", AdminAccountID: "ws1", Enabled: true, ModelTargets: []ModelTarget{{ModelName: "model-b", Enabled: true}}},
	}
	repo.groupAssignments = []GroupPolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g1", PolicyID: "p1"},
		{UserID: "user1", AdminAccountID: "ws1", AdminGroupID: "g2", PolicyID: "p2"},
	}
	targetID := "newapi:ws1:100"
	repo.states[targetID] = map[string]ConnectionHealthState{
		"model-a": {ConnectionID: targetID, ModelName: "model-a", UserID: "user1", AdminAccountID: "ws1", State: StateHealthy, CurrentWeight: 100},
		"model-b": {ConnectionID: targetID, ModelName: "model-b", UserID: "user1", AdminAccountID: "ws1", State: StateDegraded, CurrentWeight: 75},
	}
	account := upstream.AdminGroupAccountInfo{ID: "100", BaseURL: "https://up", Models: "model-a,model-b"}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "first"}, {ID: "g2", Name: "second"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {account}, "g2": {account}},
	}
	service := newAdminGroupsService(reader, fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}, repo)

	overview, err := service.Overview(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if overview.TotalConnections != 1 || overview.Healthy != 1 || overview.Degraded != 1 {
		t.Fatalf("shared target overview must merge both groups' models: %+v", overview)
	}
}

// TestProbeTarget_UsesTargetIDNotConnectionID 验证手动探活走 targetId：解析凭据成功时对候选
// 模型发起探活并落库以 targetId 为键的状态，不依赖 connectionId。
func TestProbeTarget_UsesTargetIDNotConnectionID(t *testing.T) {
	svc, repo, server := newProbeTestService(t)
	defer server.Close()
	// 覆盖 platformGroups：目标账号 100 在 vip 分组，凭据指向本地 httptest server。
	svc.mySites = fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc.platformGroups = fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: server.URL, Models: "model-a"}}},
		credByAccount: map[string]upstream.ProbeCredential{"100": {BaseURL: server.URL, Key: "k"}},
	}

	targetID := "newapi:ws1:100"
	results, err := svc.ProbeTarget(context.Background(), "user1", targetID, []string{"model-a"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].ModelName != "model-a" {
		t.Fatalf("expected model-a probed, got %+v", results)
	}
	// 状态以 targetId 为键落库。
	if _, ok := repo.states[targetID]; !ok {
		t.Fatalf("expected state stored under targetId %q, states=%v", targetID, repo.states)
	}
}

// TestProbeTarget_CredentialUnavailableReturnsStructuredError 验证凭据解析失败时手动探活返回
// 对应的结构化 i18n 错误，且不执行任何探活。
func TestProbeTarget_CredentialUnavailableReturnsStructuredError(t *testing.T) {
	svc, repo, server := newProbeTestService(t)
	defer server.Close()
	svc.mySites = fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc.platformGroups = fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: server.URL, Models: "model-a"}}},
		credErr:       map[string]error{"100": &upstream.ProbeCredentialError{Reason: upstream.ReasonSecureVerificationRequired}},
	}

	results, err := svc.ProbeTarget(context.Background(), "user1", "newapi:ws1:100", []string{"model-a"})
	if err == nil {
		t.Fatalf("expected structured error, got results=%v", results)
	}
	if err.Error() != ErrorSecureVerificationRequired {
		t.Fatalf("expected secure verification error, got %v", err)
	}
	if len(repo.events) != 0 {
		t.Fatalf("no probe should have executed, got %d events", len(repo.events))
	}
}

// TestProbeTarget_RejectsForeignWorkspaceTarget 验证不能探活别的 workspace 的 targetId。
func TestProbeTarget_RejectsForeignWorkspaceTarget(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{}
	svc := newAdminGroupsService(reader, mySites, repo)

	_, err := svc.ProbeTarget(context.Background(), "user1", "newapi:ws2:100", []string{"gpt-4o"})
	if err == nil || err.Error() != ErrorProbeTargetNotFound {
		t.Fatalf("expected target not found for foreign workspace, got %v", err)
	}
}

// TestProbeTarget_RejectsWrongPlatformSegment 验证 targetId 的 platform 段与当前 session 平台
// 不一致时（如 session 是 new-api 却传 sub2api:ws1:100）必须拒绝，不能被重建成 canonical
// newapi:ws1:100 后照常探活。
func TestProbeTarget_RejectsWrongPlatformSegment(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	// 即使账号 100 确实存在于 workspace 里，platform 段错误也必须拒绝。
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: "https://up", Models: "gpt-4o"}}},
		credByAccount: map[string]upstream.ProbeCredential{"100": {BaseURL: "https://up", Key: "k"}},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	_, err := svc.ProbeTarget(context.Background(), "user1", "sub2api:ws1:100", []string{"gpt-4o"})
	if err == nil || err.Error() != ErrorProbeTargetNotFound {
		t.Fatalf("expected target not found for wrong platform segment, got %v", err)
	}
	if len(repo.events) != 0 {
		t.Fatalf("no probe should have executed for spoofed platform segment, got %d events", len(repo.events))
	}
}

// TestProbeTarget_AccountsReadErrorSurfacedWhenTargetMissing 验证手动探活查目标时，若目标未找到
// 且期间某分组账号列表读取失败，返回账号列表读取错误（而非误导性的 targetNotFound）。
func TestProbeTarget_AccountsReadErrorSurfacedWhenTargetMissing(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{probePolicy()}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:   []upstream.AdminGroupInfo{{ID: "g-bad", Name: "bad"}},
		errByGrp: map[string]error{"g-bad": errors.New("upstream 500 secret")},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	_, err := svc.ProbeTarget(context.Background(), "user1", "newapi:ws1:100", []string{"gpt-4o"})
	if err == nil || err.Error() != ErrorAccountsFetch {
		t.Fatalf("expected accounts-fetch error when target missing due to read failure, got %v", err)
	}
}

// TestProbeTarget_FindsTargetDespiteOtherGroupReadError 验证某分组账号读取失败时，仍能在其它
// 分组找到目标并正常探活（单分组失败不阻断整体）。
func TestProbeTarget_FindsTargetDespiteOtherGroupReadError(t *testing.T) {
	svc, repo, server := newProbeTestService(t)
	defer server.Close()
	svc.mySites = fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc.platformGroups = fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g-bad", Name: "bad"}, {ID: "g-good", Name: "good"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g-good": {{ID: "100", Name: "ch", BaseURL: server.URL, Models: "model-a"}},
		},
		errByGrp:      map[string]error{"g-bad": errors.New("upstream 500")},
		credByAccount: map[string]upstream.ProbeCredential{"100": {BaseURL: server.URL, Key: "k"}},
	}

	results, err := svc.ProbeTarget(context.Background(), "user1", "newapi:ws1:100", []string{"model-a"})
	if err != nil {
		t.Fatalf("expected success finding target in the good group, got %v", err)
	}
	if len(results) != 1 || results[0].ModelName != "model-a" {
		t.Fatalf("expected model-a probed, got %+v", results)
	}
	if _, ok := repo.states["newapi:ws1:100"]; !ok {
		t.Fatalf("expected state under canonical targetId")
	}
}

// TestAdminGroups_AssignedPolicyFieldsReflectAssignments 验证账号/渠道的策略分配展示字段：
// 已分配的 target 展示 assignedPolicyIds/assignedPolicies/hasAssignedPolicy=true（即使策略已
// 被停用也要能展示名字）；未分配的 target 展示 hasAssignedPolicy=false 且不影响可否手动探活。
func TestAdminGroups_AssignedPolicyFieldsReflectAssignments(t *testing.T) {
	repo := newFakeRepository()
	repo.policies = []Policy{
		probePolicy(),
		{ID: "policy-disabled", UserID: "user1", AdminAccountID: "ws1", Name: "disabled-one", Enabled: false},
	}
	repo.assignments = []PolicyAssignment{
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: "policy-1"},
		{UserID: "user1", AdminAccountID: "ws1", TargetID: "newapi:ws1:100", PolicyID: "policy-disabled"},
	}
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups: []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{
			"g1": {
				{ID: "100", Name: "assigned", BaseURL: "https://up", Models: "gpt-4o"},
				{ID: "200", Name: "unassigned", BaseURL: "https://up", Models: "gpt-4o"},
			},
		},
	}
	svc := newAdminGroupsService(reader, mySites, repo)

	groups, err := svc.AdminGroups(context.Background(), "user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var assigned, unassigned *AdminGroupAccount
	for i := range groups[0].Accounts {
		switch groups[0].Accounts[i].ID {
		case "100":
			assigned = &groups[0].Accounts[i]
		case "200":
			unassigned = &groups[0].Accounts[i]
		}
	}
	if assigned == nil || unassigned == nil {
		t.Fatalf("missing accounts")
	}
	if !assigned.HasAssignedPolicy || len(assigned.AssignedPolicyIDs) != 2 {
		t.Fatalf("expected 2 assigned policy ids, got %+v", assigned)
	}
	var sawEnabled, sawDisabled bool
	for _, p := range assigned.AssignedPolicies {
		if p.PolicyID == "policy-1" && p.Enabled {
			sawEnabled = true
		}
		if p.PolicyID == "policy-disabled" && !p.Enabled && p.PolicyName == "disabled-one" {
			sawDisabled = true
		}
	}
	if !sawEnabled || !sawDisabled {
		t.Fatalf("expected both enabled and disabled assigned policy summaries, got %+v", assigned.AssignedPolicies)
	}
	if unassigned.HasAssignedPolicy || len(unassigned.AssignedPolicyIDs) != 0 {
		t.Fatalf("expected no assignment for account 200, got %+v", unassigned)
	}
	// 未分配策略不影响是否可手动探活：账号 200 仍然凭 base_url + 策略模型池可探活。
	if !unassigned.ProbeAvailable {
		t.Fatalf("unassigned account should still be manually probeable, got %+v", unassigned)
	}
}

// 确保 fakeMySitesReader 仍满足 MySitesReader（含 ListRealConnectionsForWorkspace）。
var _ MySitesReader = fakeMySitesReader{}
