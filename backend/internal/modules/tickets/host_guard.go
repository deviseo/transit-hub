package tickets

import (
	"net"
	"net/url"
	"regexp"
	"strings"
)

// hostDomainPattern 校验合法域名形状，与 upstream.isValidHost 使用的模式一致（不强耦合 upstream 包，
// 按任务文档要求各模块保持解耦，这里保留一份私有拷贝）。
var hostDomainPattern = regexp.MustCompile(`^([a-zA-Z0-9]([a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,63}$`)

// normalizeSrcHost 校验并规范化 iframe 请求携带的 Sub2API 来源域名：只允许 http/https，
// host 必须是合法域名、公网 IP 或 localhost。
//
// 与 upstream.isValidHost（允许内网 IP，方便自托管调试）不同，这里默认拒绝私有/回环/链路本地
// 地址：src_host 在这个接口里是完全由 iframe 请求方（不受信任的客户端）传入的，服务端会直接
// 拿它发起 HTTP 请求（SSRF 风险），必须比"仅校验站点地址格式"更严格。
// 说明：这里只做 IP 字面量层面的拦截，不做 DNS 解析后二次校验域名实际指向的地址（DNS rebinding），
// 该防护超出本任务范围。
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
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return !isReservedIP(ip)
	}
	return hostDomainPattern.MatchString(host)
}

// isReservedIP 拦截私有网段、回环、链路本地、未指定地址等不应被服务端直接请求的 IP 字面量。
func isReservedIP(ip net.IP) bool {
	return ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
