package lottery

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

var hostDomainPattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$`)

func normalizeSrcHost(value string) (string, error) {
	return normalizeSrcHostWithPrivateTargets(value, false)
}

// normalizeSrcHostWithPrivateTargets 仅在显式开启本地调试开关时接受 localhost、回环和私网 IP。
// 生产默认路径始终调用严格模式，避免公开 embed 接口访问 TransitHub 所在内网。
func normalizeSrcHostWithPrivateTargets(value string, allowPrivateTargets bool) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", requestError(ErrorEmbedInvalidSrcHost)
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || !isAllowedSrcHost(parsed.Hostname(), allowPrivateTargets) {
		return "", requestError(ErrorEmbedInvalidSrcHost)
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func isAllowedSrcHost(host string, allowPrivateTargets bool) bool {
	if host == "" {
		return false
	}
	if allowPrivateTargets && strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		if allowPrivateTargets {
			return isAllowedDevelopmentIP(ip)
		}
		return !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() && !ip.IsLinkLocalMulticast() && !ip.IsUnspecified()
	}
	return hostDomainPattern.MatchString(host)
}

// isAllowedDevelopmentIP 仍拒绝未指定、组播和链路本地地址，只额外放行本机回环与私网单播。
func isAllowedDevelopmentIP(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || (ip.IsGlobalUnicast() && !ip.IsLinkLocalUnicast()))
}

func maskEmail(email string) string {
	trimmed := strings.TrimSpace(email)
	at := strings.LastIndex(trimmed, "@")
	if at <= 0 {
		return ""
	}
	local, domain := trimmed[:at], trimmed[at+1:]
	if len(local) == 1 {
		return "*@" + domain
	}
	return local[:1] + "***@" + domain
}

func viewerActive(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "" || s == "active" || s == "enabled" || s == "normal" || s == "1"
}
