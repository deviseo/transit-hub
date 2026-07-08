package connection_health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestProbe_AllProviderFamiliesUseOpenAICompatibleGatewayEndpoint 验证 real_connections
// 里的 base_url/upstream_key 是 new-api/sub2api 网关凭据，不是 provider 官方凭据：
// 无论 providerFamily 是 gemini/anthropic/openai/custom，探活请求都必须统一打到
// {baseURL}/v1/chat/completions，用 Bearer <upstreamKey> 鉴权，不能拼 Gemini
// generateContent 或 Anthropic messages 原生端点，否则网关会返回 404/未知路径导致
// Gemini/Anthropic 探活被系统性误判为失败。
func TestProbe_AllProviderFamiliesUseOpenAICompatibleGatewayEndpoint(t *testing.T) {
	providerFamilies := []string{ProviderGemini, ProviderAnthropic, ProviderOpenAI, ProviderCustom}

	for _, family := range providerFamilies {
		t.Run(family, func(t *testing.T) {
			var gotPath string
			var gotAuth string
			var gotMaxTokens float64

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				gotMaxTokens, _ = body["max_tokens"].(float64)
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
			}))
			defer server.Close()

			runner := NewRealProbeRunner()
			outcome := runner.Probe(context.Background(), ProbeRequest{
				BaseURL: server.URL, UpstreamKey: "gateway-key", ProviderFamily: family, MaxTokens: 1,
			})
			if outcome.Result != ResultOK {
				t.Fatalf("expected ok, got %s (%s)", outcome.Result, outcome.Detail)
			}
			if gotPath != "/v1/chat/completions" {
				t.Fatalf("expected gateway-compatible path /v1/chat/completions, got %s", gotPath)
			}
			if gotAuth != "Bearer gateway-key" {
				t.Fatalf("expected Bearer auth with gateway key, got %q", gotAuth)
			}
			if gotMaxTokens != 1 {
				t.Fatalf("expected max_probe_tokens=1 to propagate, got %v", gotMaxTokens)
			}
		})
	}
}

func TestProbe_DefaultModelPerProviderWhenModelNameEmpty(t *testing.T) {
	cases := map[string]string{
		ProviderGemini:    "gemini-1.5-flash",
		ProviderAnthropic: "claude-3-haiku-20240307",
		ProviderOpenAI:    "gpt-4o-mini",
		ProviderCustom:    "gpt-4o-mini",
	}

	for family, wantModel := range cases {
		var gotModel string
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			gotModel, _ = body["model"].(string)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
		}))

		runner := NewRealProbeRunner()
		runner.Probe(context.Background(), ProbeRequest{BaseURL: server.URL, UpstreamKey: "k", ProviderFamily: family})
		server.Close()

		if gotModel != wantModel {
			t.Fatalf("provider=%s: expected default model %s, got %s", family, wantModel, gotModel)
		}
	}
}

func TestProbe_RateLimited(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"rate limited"}`))
	}))
	defer server.Close()

	runner := NewRealProbeRunner()
	outcome := runner.Probe(context.Background(), ProbeRequest{
		BaseURL: server.URL, UpstreamKey: "secret-key", ProviderFamily: ProviderAnthropic,
	})
	if outcome.Result != ResultRateLimited {
		t.Fatalf("expected rate_limited, got %s", outcome.Result)
	}
}

func TestProbe_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer server.Close()

	runner := NewRealProbeRunner()
	outcome := runner.Probe(context.Background(), ProbeRequest{
		BaseURL: server.URL, UpstreamKey: "secret-key", ProviderFamily: ProviderOpenAI,
	})
	if outcome.Result != ResultServerError {
		t.Fatalf("expected server_error, got %s", outcome.Result)
	}
}

func TestProbe_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	runner := NewRealProbeRunner()
	outcome := runner.Probe(context.Background(), ProbeRequest{
		BaseURL: server.URL, UpstreamKey: "secret-key", ProviderFamily: ProviderOpenAI,
	})
	if outcome.Result != ResultAuth {
		t.Fatalf("expected auth, got %s", outcome.Result)
	}
}

func TestProbe_ModelNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	runner := NewRealProbeRunner()
	outcome := runner.Probe(context.Background(), ProbeRequest{
		BaseURL: server.URL, UpstreamKey: "secret-key", ProviderFamily: ProviderGemini, ModelName: "does-not-exist",
	})
	if outcome.Result != ResultModelNotFound {
		t.Fatalf("expected model_not_found, got %s", outcome.Result)
	}
}

func TestProbe_InvalidResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer server.Close()

	runner := NewRealProbeRunner()
	outcome := runner.Probe(context.Background(), ProbeRequest{
		BaseURL: server.URL, UpstreamKey: "secret-key", ProviderFamily: ProviderOpenAI,
	})
	if outcome.Result != ResultInvalidResponse {
		t.Fatalf("expected invalid_response, got %s", outcome.Result)
	}
}

func TestProbe_TimeoutClassifiedAsNetworkFluctuation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	runner := &RealProbeRunner{client: &http.Client{Timeout: 20 * time.Millisecond}}
	outcome := runner.Probe(context.Background(), ProbeRequest{
		BaseURL: server.URL, UpstreamKey: "secret-key", ProviderFamily: ProviderOpenAI,
	})
	if outcome.Result != ResultNetworkFluctuation {
		t.Fatalf("expected network_fluctuation on timeout, got %s", outcome.Result)
	}
}

func TestProbe_KeyNeverLeaksIntoDetail(t *testing.T) {
	const secret = "sk-super-secret-upstream-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`error using key ` + secret))
	}))
	defer server.Close()

	runner := NewRealProbeRunner()
	outcome := runner.Probe(context.Background(), ProbeRequest{
		BaseURL: server.URL, UpstreamKey: secret, ProviderFamily: ProviderAnthropic,
	})
	if strings.Contains(outcome.Detail, secret) {
		t.Fatalf("upstream key leaked into probe outcome detail: %s", outcome.Detail)
	}
}
