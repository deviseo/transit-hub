package client_config

import (
	"net"
	"net/url"
	"strings"

	"golang.org/x/net/publicsuffix"
)

// sameOwner 判断两个网址是不是同一个站长的，和 TransitLink 的判断一致：主机相同（IP 地址只认完全相同），
// 或者属于同一个可注册域名（按公共后缀列表：api.example.com 和 hub.example.com 算；a.github.io 和
// b.github.io 不算）。TransitLink 对不在同一个域名下的专属链接会提醒用户确认链接是站长发的。
func sameOwner(a, b string) bool {
	hostA, okA := urlHost(a)
	hostB, okB := urlHost(b)
	if !okA || !okB {
		return false
	}
	if hostA == hostB {
		return true
	}
	if net.ParseIP(hostA) != nil || net.ParseIP(hostB) != nil {
		return false
	}
	domainA, errA := publicsuffix.EffectiveTLDPlusOne(hostA)
	domainB, errB := publicsuffix.EffectiveTLDPlusOne(hostB)
	return errA == nil && errB == nil && domainA == domainB
}

func urlHost(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", false
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	return host, host != ""
}
