package leaderboard

import (
	"context"
	"net"
	"net/http"
	"testing"
)

type fakeResolver struct {
	addresses []net.IPAddr
	err       error
}

func (f fakeResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.addresses, nil
}

type fakeRoundTripper struct{}

func (fakeRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) { return nil, nil }

func TestResolveSafeIPRejectsPrivateAndSpecialUseTargets(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.1.1",
		"0.0.0.0",
		"100.64.0.1",
		"::1",
		"fc00::1",
		"fe80::1",
	}
	for _, value := range blocked {
		if ip, err := resolveSafeIP(context.Background(), fakeResolver{}, value); err == nil {
			t.Fatalf("expected %s to be rejected, got %v", value, ip)
		}
	}
}

func TestResolveSafeIPPinsFirstPublicResolverAddress(t *testing.T) {
	resolver := fakeResolver{addresses: []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}, {IP: net.ParseIP("93.184.216.34")}}}
	ip, err := resolveSafeIP(context.Background(), resolver, "src.example.com")
	if err != nil {
		t.Fatalf("expected public resolver address, got err=%v", err)
	}
	if got := ip.String(); got != "93.184.216.34" {
		t.Fatalf("expected pinned public IP, got %s", got)
	}
}

func TestResolveSafeIPRejectsResolverWithOnlyUnsafeAddresses(t *testing.T) {
	resolver := fakeResolver{addresses: []net.IPAddr{{IP: net.ParseIP("192.168.1.10")}, {IP: net.ParseIP("100.64.0.2")}}}
	if ip, err := resolveSafeIP(context.Background(), resolver, "src.example.com"); err == nil {
		t.Fatalf("expected unsafe resolver addresses to fail, got %v", ip)
	}
}

func TestNewSub2APIClientPreservesCustomTransport(t *testing.T) {
	transport := fakeRoundTripper{}
	httpClient := &http.Client{Transport: transport}
	client := NewSub2APIClient(httpClient)
	if client.client.Transport != transport {
		t.Fatalf("custom transport was not preserved")
	}
}

func TestNewSub2APIClientReplacesDefaultTransportAndDisablesProxy(t *testing.T) {
	client := NewSub2APIClient(&http.Client{Transport: http.DefaultTransport})
	transport, ok := client.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected safe http transport, got %T", client.client.Transport)
	}
	if transport == http.DefaultTransport {
		t.Fatal("default transport must be cloned and replaced")
	}
	if transport.Proxy != nil {
		t.Fatal("safe viewer transport must not use environment proxy")
	}
}
