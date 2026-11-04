package profit

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// upstreamKey 把一个地址归一成「上游」的匹配键：可注册域名（eTLD+1，例如 api.foo.com 与 www.foo.com 都是 foo.com）。
// *.vercel.app 这类公共托管后缀在公共后缀列表里，不同项目不会被合并。IP 地址和单标签主机名（localhost 等）
// 保留端口，避免同一台机器上的不同服务被合并。地址为空或无法解析时返回 ""。
func upstreamKey(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host == "" {
		return ""
	}
	if net.ParseIP(host) != nil || !strings.Contains(host, ".") {
		if port := parsed.Port(); port != "" {
			return net.JoinHostPort(host, port)
		}
		return host
	}
	domain, err := publicsuffix.EffectiveTLDPlusOne(host)
	if err != nil {
		return host
	}
	return domain
}

// officialDomains 是常见模型厂商官方接口的可注册域名：指向它们的路由归类为「官方接口」，而不是「未登记的上游」。
var officialDomains = map[string]struct{}{
	"anthropic.com":  {},
	"openai.com":     {},
	"azure.com":      {},
	"googleapis.com": {},
	"x.ai":           {},
	"deepseek.com":   {},
	"moonshot.cn":    {},
	"bigmodel.cn":    {},
	"aliyuncs.com":   {},
	"volces.com":     {},
	"mistral.ai":     {},
	"cohere.ai":      {},
	"groq.com":       {},
	"minimaxi.com":   {},
}

// unattributedCategory 判断一个没有匹配上游站点的路由地址属于哪一类。
func unattributedCategory(key string) string {
	if key == "" {
		return CategoryOfficial
	}
	if _, ok := officialDomains[key]; ok {
		return CategoryOfficial
	}
	return CategoryUnregistered
}
