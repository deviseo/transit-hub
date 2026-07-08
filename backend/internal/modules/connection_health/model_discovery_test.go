package connection_health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"transithub/backend/internal/modules/upstream"
)

// newModelDiscoveryTestService 复用 admin_groups_test.go 的 fakePlatformGroupReader/fakeMySitesReader，
// 额外补上 modelDiscovery 依赖（newAdminGroupsService 不初始化它，DiscoverTargetModels 需要）。
func newModelDiscoveryTestService(reader PlatformGroupReader, mySites MySitesReader, repo *fakeRepository) *Service {
	svc := newAdminGroupsService(reader, mySites, repo)
	svc.modelDiscovery = NewModelDiscoveryRunner()
	return svc
}

// TestListModels_ParsesOpenAICompatibleResponse 验证成功解析 {"data":[{"id":...,"owned_by":...}]}，
// 并按 id 去空、去重、排序。
func TestListModels_ParsesOpenAICompatibleResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer secret-key" {
			t.Fatalf("unexpected auth header: %s", r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.5","owned_by":"acme"},{"id":"gpt-4o-mini","owned_by":""},{"id":"gpt-5.5","owned_by":"dup"},{"id":""}]}`))
	}))
	defer server.Close()

	runner := NewModelDiscoveryRunner()
	models, err := runner.ListModels(context.Background(), server.URL, "secret-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 deduped models, got %+v", models)
	}
	if models[0].ID != "gpt-4o-mini" || models[1].ID != "gpt-5.5" {
		t.Fatalf("expected sorted by id, got %+v", models)
	}
	if models[1].OwnedBy != "acme" {
		t.Fatalf("expected first-seen owned_by kept, got %+v", models[1])
	}
}

// TestListModels_UpstreamErrorStatusReturnsUnavailable 验证 401/403/404/5xx 都归类为
// ErrorModelListUnavailable，不透传上游状态码/报文细节。
func TestListModels_UpstreamErrorStatusReturnsUnavailable(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))
		runner := NewModelDiscoveryRunner()
		_, err := runner.ListModels(context.Background(), server.URL, "k")
		server.Close()
		if err == nil || err.Error() != ErrorModelListUnavailable {
			t.Fatalf("status %d: expected ErrorModelListUnavailable, got %v", status, err)
		}
	}
}

// TestListModels_InvalidBodyReturnsInvalid 验证非 JSON / 非预期结构的响应体归类为
// ErrorModelListInvalid，而不是伪装成空列表。
func TestListModels_InvalidBodyReturnsInvalid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("not-json"))
	}))
	defer server.Close()

	runner := NewModelDiscoveryRunner()
	_, err := runner.ListModels(context.Background(), server.URL, "k")
	if err == nil || err.Error() != ErrorModelListInvalid {
		t.Fatalf("expected ErrorModelListInvalid, got %v", err)
	}
}

// TestListModels_EmptyDataReturnsEmptySlice 验证模型列表为空时返回空数组，不编造默认模型。
func TestListModels_EmptyDataReturnsEmptySlice(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[]}`))
	}))
	defer server.Close()

	runner := NewModelDiscoveryRunner()
	models, err := runner.ListModels(context.Background(), server.URL, "k")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 0 {
		t.Fatalf("expected empty slice, got %+v", models)
	}
}

// TestDiscoverTargetModels_RejectsForeignWorkspaceTarget 验证模型发现接口复用 resolveManualTarget
// 的 targetId 归属校验，跨 workspace targetId 一律拒绝。
func TestDiscoverTargetModels_RejectsForeignWorkspaceTarget(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	svc := newModelDiscoveryTestService(fakePlatformGroupReader{}, mySites, repo)

	_, err := svc.DiscoverTargetModels(context.Background(), "user1", "newapi:ws2:100")
	if err == nil || err.Error() != ErrorProbeTargetNotFound {
		t.Fatalf("expected target not found for foreign workspace, got %v", err)
	}
}

// TestDiscoverTargetModels_CredentialUnavailableReturnsStructuredError 验证凭据解析失败时
// 返回结构化不可探活错误，不调用 /v1/models。
func TestDiscoverTargetModels_CredentialUnavailableReturnsStructuredError(t *testing.T) {
	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: "https://up", Models: "gpt-4o"}}},
		credErr:       map[string]error{"100": &upstream.ProbeCredentialError{Reason: upstream.ReasonSecureVerificationRequired}},
	}
	svc := newModelDiscoveryTestService(reader, mySites, repo)

	_, err := svc.DiscoverTargetModels(context.Background(), "user1", "newapi:ws1:100")
	if err == nil || err.Error() != ErrorSecureVerificationRequired {
		t.Fatalf("expected secure verification error, got %v", err)
	}
}

// TestDiscoverTargetModels_SuccessReturnsModelsFromUpstream 端到端验证：凭据解析成功后请求
// 真实探活凭据指向的 /v1/models 并返回解析后的模型列表。
func TestDiscoverTargetModels_SuccessReturnsModelsFromUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-4o-mini","owned_by":"acme"}]}`))
	}))
	defer server.Close()

	repo := newFakeRepository()
	mySites := fakeMySitesReader{session: upstream.Session{Platform: upstream.PlatformNewAPI}}
	reader := fakePlatformGroupReader{
		groups:        []upstream.AdminGroupInfo{{ID: "g1", Name: "vip"}},
		accountsByGrp: map[string][]upstream.AdminGroupAccountInfo{"g1": {{ID: "100", Name: "ch", BaseURL: server.URL, Models: "gpt-4o-mini"}}},
		credByAccount: map[string]upstream.ProbeCredential{"100": {BaseURL: server.URL, Key: "secret"}},
	}
	svc := newModelDiscoveryTestService(reader, mySites, repo)

	models, err := svc.DiscoverTargetModels(context.Background(), "user1", "newapi:ws1:100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(models) != 1 || models[0].ID != "gpt-4o-mini" {
		t.Fatalf("expected gpt-4o-mini discovered, got %+v", models)
	}
}
