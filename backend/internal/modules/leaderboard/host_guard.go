package leaderboard

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var hostDomainPattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$`)

// normalizeSrcHost turns a user supplied Sub2API origin into a canonical origin.
// This value is used both for config storage and outbound /auth/me requests, so
// it rejects private IP literals to avoid turning the embed session endpoint into
// a server-side request primitive.
func normalizeSrcHost(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", requestError(ErrorEmbedInvalidSrcHost)
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !isAllowedSrcHost(parsed.Hostname()) {
		return "", requestError(ErrorEmbedInvalidSrcHost)
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isAllowedSrcHost(host string) bool {
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !isReservedIP(ip)
	}
	return hostDomainPattern.MatchString(host)
}

func isReservedIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
