package outbound

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalize(t *testing.T) {
	cases := []struct {
		name    string
		input   Settings
		want    Settings
		wantErr bool
	}{
		{name: "scheme defaults to http", input: Settings{Enabled: true, ProxyURL: " 127.0.0.1:7897 "}, want: Settings{Enabled: true, ProxyURL: "http://127.0.0.1:7897"}},
		{name: "socks5 with credentials", input: Settings{Enabled: true, ProxyURL: "SOCKS5://user:pass@proxy.local:1080"}, want: Settings{Enabled: true, ProxyURL: "socks5://user:pass@proxy.local:1080"}},
		{name: "trailing slash dropped", input: Settings{Enabled: true, ProxyURL: "https://proxy.example.com:8443/"}, want: Settings{Enabled: true, ProxyURL: "https://proxy.example.com:8443"}},
		{name: "disabled keeps url for later", input: Settings{Enabled: false, ProxyURL: "http://127.0.0.1:7897"}, want: Settings{Enabled: false, ProxyURL: "http://127.0.0.1:7897"}},
		{name: "disabled without url", input: Settings{Enabled: false}, want: Settings{Enabled: false}},
		{name: "bypass normalized", input: Settings{Bypass: " Milus.one ;\n*.Example.com, milus.one ,, 10.0.0.0/8 "}, want: Settings{Bypass: "milus.one,*.example.com,10.0.0.0/8"}},
		{name: "enabled requires url", input: Settings{Enabled: true}, wantErr: true},
		{name: "unsupported scheme", input: Settings{Enabled: true, ProxyURL: "ftp://proxy:21"}, wantErr: true},
		{name: "path not allowed", input: Settings{Enabled: true, ProxyURL: "http://proxy:8080/some/path"}, wantErr: true},
		{name: "missing host", input: Settings{Enabled: true, ProxyURL: "http://:8080"}, wantErr: true},
	}
	for _, tc := range cases {
		got, err := Normalize(tc.input)
		if tc.wantErr {
			if !errors.Is(err, ErrInvalidProxyURL) {
				t.Fatalf("%s: expected ErrInvalidProxyURL, got settings=%+v err=%v", tc.name, got, err)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("%s: Normalize = %+v err=%v, want %+v", tc.name, got, err, tc.want)
		}
	}
}

func TestMaskAndRestorePassword(t *testing.T) {
	stored := "http://alice:s3cret@proxy.local:7897"
	masked := MaskProxyURL(stored)
	if masked != "http://alice:***@proxy.local:7897" {
		t.Fatalf("MaskProxyURL = %q, want a readable placeholder", masked)
	}
	if MaskProxyURL("http://proxy.local:7897") != "http://proxy.local:7897" || MaskProxyURL("socks5://bob@proxy.local:1080") != "socks5://bob@proxy.local:1080" {
		t.Fatal("addresses without a password must be returned unchanged")
	}
	if !HasPassword(stored) || HasPassword("http://proxy.local:7897") {
		t.Fatal("HasPassword reported the wrong result")
	}
	if got := RestoreMaskedPassword(masked, stored); got != stored {
		t.Fatalf("RestoreMaskedPassword = %q, want the stored password back", got)
	}
	// 换了主机或用户名时，不能把旧密码带到新代理上。
	if got := RestoreMaskedPassword("http://alice:***@other.local:7897", stored); got != "http://alice:***@other.local:7897" {
		t.Fatalf("password must not move to a different host, got %q", got)
	}
	if got := RestoreMaskedPassword("http://bob:***@proxy.local:7897", stored); got != "http://bob:***@proxy.local:7897" {
		t.Fatalf("password must not move to a different user, got %q", got)
	}
	if got := RestoreMaskedPassword("http://alice:newpass@proxy.local:7897", stored); got != "http://alice:newpass@proxy.local:7897" {
		t.Fatalf("an explicitly typed password must win, got %q", got)
	}
}

func proxyFor(t *testing.T, controller *Controller, rawURL string) *url.URL {
	t.Helper()
	target, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	proxyURL, err := controller.Proxy(&http.Request{URL: target})
	if err != nil {
		t.Fatalf("Proxy(%s) returned error: %v", rawURL, err)
	}
	return proxyURL
}

func TestControllerModes(t *testing.T) {
	controller := NewController()
	envProxy, _ := url.Parse("http://env-proxy.local:3128")
	controller.environmentProxy = func(*http.Request) (*url.URL, error) { return envProxy, nil }

	if controller.Mode() != ModeEnvironment || proxyFor(t, controller, "https://upstream.example.com") != envProxy {
		t.Fatal("an unconfigured controller must follow the environment proxy")
	}

	if err := controller.Apply(Settings{Enabled: true, ProxyURL: "http://127.0.0.1:7897", Bypass: "milus.one,.internal.test"}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if controller.Mode() != ModeProxy {
		t.Fatalf("mode = %s, want proxy", controller.Mode())
	}
	if got := proxyFor(t, controller, "https://upstream.example.com/api/status"); got == nil || got.Host != "127.0.0.1:7897" {
		t.Fatalf("expected upstream.example.com to use the proxy, got %v", got)
	}
	for _, direct := range []string{"https://milus.one", "https://newapius.milus.one/api/status", "https://a.internal.test", "http://127.0.0.1:8081", "http://localhost:5555"} {
		if got := proxyFor(t, controller, direct); got != nil {
			t.Fatalf("expected %s to bypass the proxy, got %v", direct, got)
		}
	}

	if err := controller.Apply(Settings{Enabled: false, ProxyURL: "http://127.0.0.1:7897"}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if controller.Mode() != ModeDirect || proxyFor(t, controller, "https://upstream.example.com") != nil {
		t.Fatal("a disabled proxy must connect directly, ignoring the environment")
	}

	if err := controller.Apply(Settings{Enabled: true, ProxyURL: "ftp://bad"}); err == nil {
		t.Fatal("expected invalid settings to be rejected")
	}
	if controller.Mode() != ModeDirect {
		t.Fatal("rejected settings must not change the active configuration")
	}
}

// TestControllerSwitchesLiveTransports 验证切换配置后，已创建的 client 立即按新配置出站，无需重启。
func TestControllerSwitchesLiveTransports(t *testing.T) {
	var proxied int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 作为 HTTP 正向代理：请求行是绝对地址 http://upstream.test/...
		if r.URL.Host != "upstream.test" {
			t.Errorf("unexpected proxied host %q", r.URL.Host)
		}
		atomic.AddInt32(&proxied, 1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer proxy.Close()

	controller := NewController()
	client := controller.NewClient(5 * time.Second)

	if err := controller.Apply(Settings{Enabled: true, ProxyURL: proxy.URL}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	response, err := client.Get("http://upstream.test/api/status")
	if err != nil {
		t.Fatalf("request through proxy failed: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusNoContent || atomic.LoadInt32(&proxied) != 1 {
		t.Fatalf("expected the request to go through the proxy, status=%d proxied=%d", response.StatusCode, proxied)
	}

	if err := controller.Apply(Settings{Enabled: false}); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if _, err := client.Get("http://upstream.test/api/status"); err == nil {
		t.Fatal("expected a direct request to the unresolvable host to fail after disabling the proxy")
	} else if failure := ClassifyError(err); failure != FailureDNS {
		t.Fatalf("direct failure classified as %q, want %q (err=%v)", failure, FailureDNS, err)
	}
	if atomic.LoadInt32(&proxied) != 1 {
		t.Fatal("no request may reach the proxy after it was disabled")
	}
}

func TestClassifyError(t *testing.T) {
	cases := map[string]struct {
		err  error
		want string
	}{
		"proxy connect": {&url.Error{Op: "Get", URL: "https://x", Err: &net.OpError{Op: "proxyconnect", Net: "tcp", Err: errors.New("connection refused")}}, FailureProxy},
		"dns":           {&url.Error{Op: "Get", URL: "https://x", Err: &net.DNSError{Err: "no such host", Name: "x"}}, FailureDNS},
		"deadline":      {&url.Error{Op: "Get", URL: "https://x", Err: context.DeadlineExceeded}, FailureTimeout},
		"tls text":      {errors.New("tls: handshake failure"), FailureTLS},
		"other":         {errors.New("EOF"), FailureNetwork},
	}
	for name, tc := range cases {
		if got := ClassifyError(tc.err); got != tc.want {
			t.Fatalf("%s: ClassifyError = %q, want %q", name, got, tc.want)
		}
	}
	if ClassifyError(nil) != "" {
		t.Fatal("nil error must classify as empty")
	}
}
