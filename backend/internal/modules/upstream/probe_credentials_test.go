package upstream

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestResolveProbeCredential_NewAPIChannelKeySuccess 验证 new-api：
//   - base_url 来自 channel 列表字段（account.BaseURL），不从 GET /api/channel/:id 取（那里没有 key）。
//   - key 通过 POST /api/channel/:id/key 临时获取，成功时可构造探活凭据。
func TestResolveProbeCredential_NewAPIChannelKeySuccess(t *testing.T) {
	keyCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/channel/100/key":
			keyCalled = true
			writeJSON(w, map[string]any{"data": map[string]any{"key": "sk-plain-secret"}})
		case r.URL.Path == "/api/channel/100":
			// GetChannel 不返回 key（selectAll=false 时 DB.Omit("key")）。如果解析器错误地
			// 依赖这里取 key，就会拿不到 key —— 但它不应该走这里。
			writeJSON(w, map[string]any{"data": map[string]any{"id": 100, "name": "ch", "base_url": "https://up"}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "s=1", UserID: "1"}
	account := AdminGroupAccountInfo{ID: "100", Name: "ch", BaseURL: "https://up.example.com", Models: "gpt-4o,gpt-4o-mini"}

	cred, err := service.ResolveProbeCredential(session, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !keyCalled {
		t.Fatalf("expected POST /api/channel/:id/key to be used for key")
	}
	if cred.Key != "sk-plain-secret" || cred.BaseURL != "https://up.example.com" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
	if len(cred.Models) != 2 {
		t.Fatalf("expected 2 models parsed, got %v", cred.Models)
	}
}

// TestResolveProbeCredential_NewAPIKeyUnauthorizedIsUnavailable 验证 key 接口 401 时目标不可探活
// （安全验证/根权限不足），映射为 secure_verification_required。
func TestResolveProbeCredential_NewAPIKeyUnauthorizedIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/channel/100/key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "s=1", UserID: "1"}
	account := AdminGroupAccountInfo{ID: "100", BaseURL: "https://up"}

	_, err := service.ResolveProbeCredential(session, account)
	if ProbeCredentialReason(err) != ReasonSecureVerificationRequired {
		t.Fatalf("expected secure_verification_required, got %v", err)
	}
}

// TestResolveProbeCredential_NewAPIKeyForbiddenIsSecureVerification 验证 key 接口返回 403
// （root/安全验证不足的常见状态码）时，归类为 secure_verification_required，而不是
// credential_unavailable，前端才能提示用户需要 root 安全验证。
func TestResolveProbeCredential_NewAPIKeyForbiddenIsSecureVerification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/channel/100/key" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "s=1", UserID: "1"}
	account := AdminGroupAccountInfo{ID: "100", BaseURL: "https://up"}

	_, err := service.ResolveProbeCredential(session, account)
	if ProbeCredentialReason(err) != ReasonSecureVerificationRequired {
		t.Fatalf("expected secure_verification_required for 403, got %v", err)
	}
}

// TestResolveProbeCredential_NewAPIMissingBaseURLIsUnavailable 验证缺 base_url 时不可探活，
// 且根本不去调用受保护的 key 接口。
func TestResolveProbeCredential_NewAPIMissingBaseURLIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("must not hit any endpoint when base_url is missing, got %s %s", r.Method, r.URL.Path)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformNewAPI, BaseURL: server.URL, Cookie: "s=1", UserID: "1"}
	account := AdminGroupAccountInfo{ID: "100"} // 无 base_url

	_, err := service.ResolveProbeCredential(session, account)
	if ProbeCredentialReason(err) != ReasonBaseURLUnavailable {
		t.Fatalf("expected base_url_unavailable, got %v", err)
	}
}

// TestResolveProbeCredential_Sub2APIExportSingleAccount 验证 sub2api：导出接口可用且正好返回
// 一个账号、含明文 credentials 时，可构造探活凭据。
func TestResolveProbeCredential_Sub2APIExportSingleAccount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/accounts/data" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("ids") != "55" || r.URL.Query().Get("include_proxies") != "false" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		writeJSON(w, map[string]any{"data": []map[string]any{
			{"name": "acc", "credentials": map[string]any{"api_key": "sk-sub2-secret", "base_url": "https://sub2-up"}},
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "tok"}
	account := AdminGroupAccountInfo{ID: "55", Name: "acc", Models: "gpt-4o"}

	cred, err := service.ResolveProbeCredential(session, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Key != "sk-sub2-secret" || cred.BaseURL != "https://sub2-up" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
}

// TestResolveProbeCredential_Sub2APIExportAccountsShapeSuccess 验证 sub2api 真实导出接口结构：
// data 是对象、账号数组在 data.accounts（而不是旧假设的 data 直接是数组）时仍能正确解析。
func TestResolveProbeCredential_Sub2APIExportAccountsShapeSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/admin/accounts/data" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("ids") != "1443" || r.URL.Query().Get("include_proxies") != "false" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		writeJSON(w, map[string]any{"data": map[string]any{
			"accounts": []map[string]any{
				{"name": "acc", "credentials": map[string]any{"api_key": "sk-sub2-secret", "base_url": "https://sub2-up"}},
			},
			"proxies":     []any{},
			"exported_at": "2026-07-07T00:00:00Z",
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "tok"}
	account := AdminGroupAccountInfo{ID: "1443", Name: "acc", Models: "gpt-4o"}

	cred, err := service.ResolveProbeCredential(session, account)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cred.Key != "sk-sub2-secret" || cred.BaseURL != "https://sub2-up" {
		t.Fatalf("unexpected credential: %+v", cred)
	}
}

// TestResolveProbeCredential_Sub2APIExportAccountsShapeEmptyIsUnavailable 验证 data.accounts[]
// 结构下账号数量为 0 时仍标记 credential_unavailable，不编造凭据。
func TestResolveProbeCredential_Sub2APIExportAccountsShapeEmptyIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{
			"accounts": []map[string]any{},
			"proxies":  []any{},
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "tok"}
	account := AdminGroupAccountInfo{ID: "1443"}

	_, err := service.ResolveProbeCredential(session, account)
	if ProbeCredentialReason(err) != ReasonCredentialUnavailable {
		t.Fatalf("expected credential_unavailable for empty data.accounts, got %v", err)
	}
}

// TestResolveProbeCredential_Sub2APIExportAccountsShapeNotExactlyOne 验证 data.accounts[]
// 结构下账号数量不是 1（多个）时不可探活，不按 name 猜测。
func TestResolveProbeCredential_Sub2APIExportAccountsShapeNotExactlyOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": map[string]any{
			"accounts": []map[string]any{
				{"name": "acc-a", "credentials": map[string]any{"api_key": "k1", "base_url": "https://a"}},
				{"name": "acc-b", "credentials": map[string]any{"api_key": "k2", "base_url": "https://b"}},
			},
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "tok"}
	account := AdminGroupAccountInfo{ID: "1443"}

	_, err := service.ResolveProbeCredential(session, account)
	if ProbeCredentialReason(err) != ReasonCredentialUnavailable {
		t.Fatalf("expected credential_unavailable for ambiguous data.accounts export, got %v", err)
	}
}

// TestResolveProbeCredential_Sub2APIRedactedIsUnavailable 验证导出返回的 credentials 已脱敏
// （没有任何明文 key 字段）时不可探活。
func TestResolveProbeCredential_Sub2APIRedactedIsUnavailable(t *testing.T) {
	// 常规 list/detail 已脱敏：credentials 里没有任何明文 key 字段（只剩 note），标记不可探活。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": []map[string]any{
			{"name": "acc", "credentials": map[string]any{"note": "redacted"}},
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "tok"}
	account := AdminGroupAccountInfo{ID: "55"}

	_, err := service.ResolveProbeCredential(session, account)
	if ProbeCredentialReason(err) != ReasonCredentialsRedacted {
		t.Fatalf("expected credentials_redacted, got %v", err)
	}
}

// TestResolveProbeCredential_Sub2APIExportUnavailable 验证导出接口不可达（404，模拟旧版本
// 路由被 /:id 抢占）时不可探活，reason=export_unavailable。
func TestResolveProbeCredential_Sub2APIExportUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "tok"}
	account := AdminGroupAccountInfo{ID: "55"}

	_, err := service.ResolveProbeCredential(session, account)
	if ProbeCredentialReason(err) != ReasonExportUnavailable {
		t.Fatalf("expected export_unavailable, got %v", err)
	}
}

// TestResolveProbeCredential_Sub2APIExportNotExactlyOne 验证导出返回账号数量不是 1（无法确认
// 映射到当前账号）时不可探活，不按 name 猜测。
func TestResolveProbeCredential_Sub2APIExportNotExactlyOne(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"data": []map[string]any{
			{"name": "acc-a", "credentials": map[string]any{"api_key": "k1", "base_url": "https://a"}},
			{"name": "acc-b", "credentials": map[string]any{"api_key": "k2", "base_url": "https://b"}},
		}})
	}))
	defer server.Close()

	service := NewPlatformService(NewHTTPClient(server.Client()))
	session := Session{Platform: PlatformSub2API, BaseURL: server.URL, AccessToken: "tok"}
	account := AdminGroupAccountInfo{ID: "55"}

	_, err := service.ResolveProbeCredential(session, account)
	if ProbeCredentialReason(err) != ReasonCredentialUnavailable {
		t.Fatalf("expected credential_unavailable for ambiguous export, got %v", err)
	}
}
