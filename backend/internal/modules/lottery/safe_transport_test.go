package lottery

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
)

type lotteryFakeResolver struct {
	addresses []net.IPAddr
	err       error
}

func (f lotteryFakeResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.addresses, nil
}

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
		if ip, err := resolveSafeIP(context.Background(), lotteryFakeResolver{}, value); err == nil {
			t.Fatalf("expected %s to be rejected, got %v", value, ip)
		}
	}
}

func TestResolveTargetIPAllowsLoopbackAndPrivateAddressesForLocalDebugging(t *testing.T) {
	for _, value := range []string{"127.0.0.1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "::1"} {
		ip, err := resolveTargetIP(context.Background(), lotteryFakeResolver{}, value, true)
		if err != nil {
			t.Fatalf("expected %s to be allowed for local debugging, got err=%v", value, err)
		}
		if ip.String() != value {
			t.Fatalf("resolved %s as %s", value, ip.String())
		}
	}
}

func TestResolveTargetIPAllowsPrivateDNSResultForLocalDebugging(t *testing.T) {
	resolver := lotteryFakeResolver{addresses: []net.IPAddr{{IP: net.ParseIP("192.168.1.10")}}}
	ip, err := resolveTargetIP(context.Background(), resolver, "sub2api.test", true)
	if err != nil {
		t.Fatalf("expected private DNS result to be allowed for local debugging, got err=%v", err)
	}
	if got := ip.String(); got != "192.168.1.10" {
		t.Fatalf("expected private resolver address, got %s", got)
	}
}

func TestResolveTargetIPStillRejectsSpecialAddressesForLocalDebugging(t *testing.T) {
	for _, value := range []string{"0.0.0.0", "169.254.169.254", "224.0.0.1", "fe80::1"} {
		if ip, err := resolveTargetIP(context.Background(), lotteryFakeResolver{}, value, true); err == nil {
			t.Fatalf("expected %s to remain rejected, got %v", value, ip)
		}
	}
}

func TestResolveSafeIPPinsFirstPublicResolverAddress(t *testing.T) {
	resolver := lotteryFakeResolver{addresses: []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}, {IP: net.ParseIP("93.184.216.34")}}}
	ip, err := resolveSafeIP(context.Background(), resolver, "src.example.com")
	if err != nil {
		t.Fatalf("expected public resolver address, got err=%v", err)
	}
	if got := ip.String(); got != "93.184.216.34" {
		t.Fatalf("expected pinned public IP, got %s", got)
	}
}

func TestResolveSafeIPRejectsResolverWithOnlyUnsafeAddresses(t *testing.T) {
	resolver := lotteryFakeResolver{addresses: []net.IPAddr{{IP: net.ParseIP("192.168.1.10")}, {IP: net.ParseIP("100.64.0.2")}}}
	if ip, err := resolveSafeIP(context.Background(), resolver, "src.example.com"); err == nil {
		t.Fatalf("expected unsafe resolver addresses to fail, got %v", ip)
	}
}

func TestResolveSafeIPReturnsResolverError(t *testing.T) {
	want := errors.New("resolver failed")
	_, err := resolveSafeIP(context.Background(), lotteryFakeResolver{err: want}, "src.example.com")
	if !errors.Is(err, want) {
		t.Fatalf("expected resolver error, got %v", err)
	}
}

func TestNewSafeViewerTransportDisablesEnvironmentProxy(t *testing.T) {
	transport, ok := newSafeViewerTransport(lotteryFakeResolver{}).(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport")
	}
	if transport.Proxy != nil {
		t.Fatal("safe viewer transport must not use environment proxy")
	}
	if transport == http.DefaultTransport {
		t.Fatal("default transport must be cloned before mutation")
	}
}
