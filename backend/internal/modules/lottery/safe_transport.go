package lottery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"
)

type resolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

func newSafeViewerTransport(resolver resolver) http.RoundTripper {
	return newViewerTransport(resolver, false)
}

// newViewerTransport 在两种模式下都禁用环境代理并固定实际拨号 IP，防止代理泄露凭据和 DNS 重绑定。
// allowPrivateTargets 只能由本地调试配置开启。
func newViewerTransport(resolver resolver, allowPrivateTargets bool) http.RoundTripper {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	base.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ip, err := resolveTargetIP(ctx, resolver, host, allowPrivateTargets)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	return base
}

func resolveSafeIP(ctx context.Context, resolver resolver, host string) (net.IP, error) {
	return resolveTargetIP(ctx, resolver, host, false)
}

func resolveTargetIP(ctx context.Context, resolver resolver, host string, allowPrivateTargets bool) (net.IP, error) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if ip := net.ParseIP(host); ip != nil {
		if isAllowedDialIP(ip, allowPrivateTargets) {
			return ip, nil
		}
		return nil, errors.New("sub2api target ip is not allowed")
	}
	ips, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, ip := range ips {
		if isAllowedDialIP(ip.IP, allowPrivateTargets) {
			return ip.IP, nil
		}
	}
	return nil, fmt.Errorf("sub2api target resolved to no allowed addresses")
}

func isAllowedDialIP(ip net.IP, allowPrivateTargets bool) bool {
	if allowPrivateTargets {
		return isAllowedDevelopmentIP(ip)
	}
	return isSafeDialIP(ip)
}

func isSafeDialIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		ip = ip4
	} else {
		ip = ip.To16()
	}
	if ip == nil {
		return false
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() {
		return false
	}
	return !isCGNAT(ip)
}

func isCGNAT(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 100 && ip4[1]&0xc0 == 64
	}
	return false
}
