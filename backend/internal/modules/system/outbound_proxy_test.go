package system

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"transithub/backend/internal/config"
	"transithub/backend/internal/shared/outbound"
)

type fakeProxyStore struct {
	mu     sync.Mutex
	stored *StoredOutboundProxy
	saves  int
}

func (f *fakeProxyStore) GetOutboundProxy(ctx context.Context) (StoredOutboundProxy, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stored == nil {
		return StoredOutboundProxy{}, false, nil
	}
	return *f.stored, true, nil
}

func (f *fakeProxyStore) SaveOutboundProxy(ctx context.Context, settings outbound.Settings, updatedBy string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saves++
	f.stored = &StoredOutboundProxy{Settings: settings, UpdatedAt: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)}
	return f.stored.UpdatedAt, nil
}

type fakeAdmins struct{ allowed bool }

func (f fakeAdmins) IsSystemAdmin(ctx context.Context, userID string) (bool, error) {
	return f.allowed, nil
}

func newProxyTestService(store *fakeProxyStore, allowed bool) (*Service, *outbound.Controller) {
	controller := outbound.NewController()
	service := NewService(config.Config{})
	service.SetOutboundProxy(store, controller, fakeAdmins{allowed: allowed})
	return service, controller
}

func clearProxyEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{"HTTPS_PROXY", "https_proxy", "HTTP_PROXY", "http_proxy", "ALL_PROXY", "all_proxy", "NO_PROXY", "no_proxy"} {
		t.Setenv(key, "")
	}
}

func assertRequestError(t *testing.T, err error, want string) {
	t.Helper()
	var requestErr requestError
	if !errors.As(err, &requestErr) || string(requestErr) != want {
		t.Fatalf("expected %s, got %v", want, err)
	}
}

// TestGetOutboundProxyPrefillsFromEnvironment 验证从未保存过配置时沿用环境变量，并用它预填表单（密码脱敏）。
func TestGetOutboundProxyPrefillsFromEnvironment(t *testing.T) {
	clearProxyEnv(t)
	t.Setenv("HTTPS_PROXY", "http://alice:pw@127.0.0.1:7897")
	t.Setenv("NO_PROXY", "localhost,127.0.0.1")
	service, _ := newProxyTestService(&fakeProxyStore{}, true)

	response, err := service.GetOutboundProxy(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("GetOutboundProxy returned error: %v", err)
	}
	if response.Configured || response.Mode != outbound.ModeEnvironment || !response.CanEdit {
		t.Fatalf("unexpected state: %+v", response)
	}
	if !response.Enabled || response.ProxyURL != "http://alice:***@127.0.0.1:7897" || response.Bypass != "localhost,127.0.0.1" {
		t.Fatalf("expected the form to be prefilled from the environment, got %+v", response)
	}
	if response.Environment.ProxyURL != "http://alice:***@127.0.0.1:7897" {
		t.Fatalf("environment proxy must be masked, got %q", response.Environment.ProxyURL)
	}
}

// TestSaveOutboundProxyRequiresSystemAdmin 验证只有系统管理员能修改全局代理，普通用户既不能保存也不能测试。
func TestSaveOutboundProxyRequiresSystemAdmin(t *testing.T) {
	clearProxyEnv(t)
	store := &fakeProxyStore{}
	service, controller := newProxyTestService(store, false)

	_, err := service.SaveOutboundProxy(context.Background(), "user-2", SaveOutboundProxyRequest{Enabled: true, ProxyURL: "http://evil.example:8080"})
	assertRequestError(t, err, ErrorProxyForbidden)
	_, err = service.TestOutboundProxy(context.Background(), "user-2", TestOutboundProxyRequest{TargetURL: "https://example.com"})
	assertRequestError(t, err, ErrorProxyForbidden)
	if store.saves != 0 || controller.Mode() != outbound.ModeEnvironment {
		t.Fatalf("a forbidden request must not change anything, saves=%d mode=%s", store.saves, controller.Mode())
	}
	response, err := service.GetOutboundProxy(context.Background(), "user-2")
	if err != nil || response.CanEdit {
		t.Fatalf("non-admins can view but not edit, got %+v err=%v", response, err)
	}
}

// TestSaveOutboundProxyPersistsAndAppliesImmediately 验证开启 -> 关闭 -> 更换都立即生效。
func TestSaveOutboundProxyPersistsAndAppliesImmediately(t *testing.T) {
	clearProxyEnv(t)
	store := &fakeProxyStore{}
	service, controller := newProxyTestService(store, true)
	ctx := context.Background()

	response, err := service.SaveOutboundProxy(ctx, "user-1", SaveOutboundProxyRequest{Enabled: true, ProxyURL: "127.0.0.1:7897", Bypass: "milus.one"})
	if err != nil {
		t.Fatalf("enable returned error: %v", err)
	}
	if !response.Configured || response.Mode != outbound.ModeProxy || response.ProxyURL != "http://127.0.0.1:7897" || response.Bypass != "milus.one" {
		t.Fatalf("unexpected response after enabling: %+v", response)
	}

	response, err = service.SaveOutboundProxy(ctx, "user-1", SaveOutboundProxyRequest{Enabled: false, ProxyURL: "http://127.0.0.1:7897"})
	if err != nil || response.Mode != outbound.ModeDirect || controller.Mode() != outbound.ModeDirect {
		t.Fatalf("disable should switch to direct immediately, got %+v err=%v", response, err)
	}
	if store.stored.Settings.ProxyURL != "http://127.0.0.1:7897" {
		t.Fatal("disabling must keep the address so it can be re-enabled later")
	}

	response, err = service.SaveOutboundProxy(ctx, "user-1", SaveOutboundProxyRequest{Enabled: true, ProxyURL: "socks5://10.0.0.2:1080"})
	if err != nil || response.Mode != outbound.ModeProxy || response.ProxyURL != "socks5://10.0.0.2:1080" {
		t.Fatalf("changing the proxy should apply immediately, got %+v err=%v", response, err)
	}
}

// TestSaveOutboundProxyKeepsMaskedPassword 验证表单回传密码占位符时沿用已保存的密码。
func TestSaveOutboundProxyKeepsMaskedPassword(t *testing.T) {
	clearProxyEnv(t)
	store := &fakeProxyStore{stored: &StoredOutboundProxy{Settings: outbound.Settings{Enabled: true, ProxyURL: "http://alice:s3cret@proxy.local:7897"}}}
	service, _ := newProxyTestService(store, true)

	response, err := service.SaveOutboundProxy(context.Background(), "user-1", SaveOutboundProxyRequest{Enabled: true, ProxyURL: "http://alice:***@proxy.local:7897", Bypass: "milus.one"})
	if err != nil {
		t.Fatalf("save returned error: %v", err)
	}
	if store.stored.Settings.ProxyURL != "http://alice:s3cret@proxy.local:7897" {
		t.Fatalf("stored password must be kept, got %q", store.stored.Settings.ProxyURL)
	}
	if response.ProxyURL != "http://alice:***@proxy.local:7897" {
		t.Fatalf("response must stay masked, got %q", response.ProxyURL)
	}
}

// TestSaveOutboundProxyRejectsInvalidURL 验证非法地址被拒绝，且不影响当前生效配置。
func TestSaveOutboundProxyRejectsInvalidURL(t *testing.T) {
	clearProxyEnv(t)
	store := &fakeProxyStore{}
	service, controller := newProxyTestService(store, true)

	for _, invalid := range []SaveOutboundProxyRequest{
		{Enabled: true, ProxyURL: ""},
		{Enabled: true, ProxyURL: "ftp://proxy:21"},
		{Enabled: true, ProxyURL: "http://proxy:8080/path"},
	} {
		_, err := service.SaveOutboundProxy(context.Background(), "user-1", invalid)
		assertRequestError(t, err, ErrorProxyInvalidURL)
	}
	if store.saves != 0 || controller.Mode() != outbound.ModeEnvironment {
		t.Fatalf("invalid input must not be saved or applied, saves=%d mode=%s", store.saves, controller.Mode())
	}
}

// TestLoadOutboundProxyAppliesSavedSettings 验证启动时应用已保存的配置。
func TestLoadOutboundProxyAppliesSavedSettings(t *testing.T) {
	clearProxyEnv(t)
	store := &fakeProxyStore{stored: &StoredOutboundProxy{Settings: outbound.Settings{Enabled: false, ProxyURL: "http://127.0.0.1:7897"}}}
	service, controller := newProxyTestService(store, true)
	if err := service.LoadOutboundProxy(context.Background()); err != nil {
		t.Fatalf("LoadOutboundProxy returned error: %v", err)
	}
	if controller.Mode() != outbound.ModeDirect {
		t.Fatalf("saved settings must be applied at startup, mode=%s", controller.Mode())
	}
}

// TestTestOutboundProxyReportsRouteAndFailure 验证连接测试使用表单里未保存的配置，并给出出站方式与失败原因。
func TestTestOutboundProxyReportsRouteAndFailure(t *testing.T) {
	clearProxyEnv(t)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "upstream.test" {
			t.Errorf("unexpected proxied host %q", r.URL.Host)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	deadProxy := "http://" + listener.Addr().String()
	_ = listener.Close()

	store := &fakeProxyStore{}
	service, controller := newProxyTestService(store, true)
	ctx := context.Background()

	result, err := service.TestOutboundProxy(ctx, "user-1", TestOutboundProxyRequest{Enabled: true, ProxyURL: proxy.URL, TargetURL: "http://upstream.test/api/status"})
	if err != nil || !result.OK || result.Route != outbound.ModeProxy || result.StatusCode != http.StatusNoContent {
		t.Fatalf("expected a successful proxied request, got %+v err=%v", result, err)
	}

	result, err = service.TestOutboundProxy(ctx, "user-1", TestOutboundProxyRequest{Enabled: true, ProxyURL: deadProxy, TargetURL: "http://upstream.test/"})
	if err != nil || result.OK || result.Route != outbound.ModeProxy || result.Failure != outbound.FailureProxy {
		t.Fatalf("expected a proxy failure, got %+v err=%v", result, err)
	}

	result, err = service.TestOutboundProxy(ctx, "user-1", TestOutboundProxyRequest{Enabled: true, ProxyURL: proxy.URL, Bypass: "upstream.test", TargetURL: "http://upstream.test/"})
	if err != nil || result.OK || result.Route != outbound.ModeDirect || result.Failure != outbound.FailureDNS {
		t.Fatalf("bypassed host should connect directly (and fail DNS here), got %+v err=%v", result, err)
	}

	if store.saves != 0 || controller.Mode() != outbound.ModeEnvironment {
		t.Fatal("testing must not save or apply the proposed settings")
	}

	_, err = service.TestOutboundProxy(ctx, "user-1", TestOutboundProxyRequest{TargetURL: "ftp://example.com"})
	assertRequestError(t, err, ErrorProxyInvalidTarget)
}
