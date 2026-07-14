package leaderboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type sub2APIError struct {
	unauthorized bool
	detail       string
}

func (e *sub2APIError) Error() string { return e.detail }

type Sub2APIClient struct {
	client *http.Client
}

func NewSub2APIClient(client *http.Client) *Sub2APIClient {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if client.Transport == nil || client.Transport == http.DefaultTransport {
		clone := *client
		clone.Transport = newSafeViewerTransport(net.DefaultResolver)
		client = &clone
	}
	return &Sub2APIClient{client: client}
}

// FetchCurrentUser verifies the viewer token with the configured Sub2API origin.
// The caller never stores the viewer token; only the returned identity snapshot
// is persisted in the short-lived Redis session.
func (c *Sub2APIClient) FetchCurrentUser(srcHost string, token string) (Sub2APIUser, error) {
	req, err := http.NewRequest(http.MethodGet, strings.TrimRight(srcHost, "/")+"/api/v1/auth/me", nil)
	if err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "build sub2api request failed"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.client.Do(req)
	if err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "sub2api network error"}
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "read sub2api response failed"}
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return Sub2APIUser{}, &sub2APIError{unauthorized: true, detail: fmt.Sprintf("sub2api auth failed status=%d", resp.StatusCode)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Sub2APIUser{}, &sub2APIError{detail: fmt.Sprintf("sub2api request failed status=%d", resp.StatusCode)}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return Sub2APIUser{}, &sub2APIError{detail: "invalid json response from sub2api"}
	}
	fields := payload
	if nested, ok := payload["data"].(map[string]any); ok {
		fields = nested
	}
	user := Sub2APIUser{ID: firstStringField(fields, "id", "user_id", "userId"), Email: firstStringField(fields, "email"), Role: firstStringField(fields, "role")}
	if user.ID == "" {
		return Sub2APIUser{}, &sub2APIError{detail: "sub2api response missing user id"}
	}
	return user, nil
}

func firstStringField(fields map[string]any, keys ...string) string {
	for _, key := range keys {
		switch value := fields[key].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		case float64:
			return strconv.FormatInt(int64(value), 10)
		case int:
			return strconv.Itoa(value)
		}
	}
	return ""
}

type ipResolver interface {
	LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error)
}

func newSafeViewerTransport(resolver ipResolver) *http.Transport {
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	base.DialContext = func(ctx context.Context, network string, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ip, err := resolveSafeIP(ctx, resolver, host)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
	return base
}

func resolveSafeIP(ctx context.Context, resolver ipResolver, host string) (net.IP, error) {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	if ip := net.ParseIP(host); ip != nil {
		if isSafeDialIP(ip) {
			return ip, nil
		}
		return nil, errors.New("sub2api target ip is not public")
	}
	addresses, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	for _, address := range addresses {
		if isSafeDialIP(address.IP) {
			return address.IP, nil
		}
	}
	return nil, errors.New("sub2api target resolved to no public addresses")
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
