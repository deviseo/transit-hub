// Package datadir 判断数据目录是不是持久保存的。
//
// 数据目录里放着在线更新装的新版本和凭据加密的密钥。Docker 部署时它需要挂载成卷（../data/app:/app/data）；
// 没挂载时文件在容器自己的可写层里，重建容器就没了。
package datadir

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Persistent 判断 dir 重建容器后是否还在：在 Linux 上，dir 所在的挂载点是容器根文件系统（overlay）时返回 false。
// 其他情况（单独挂载的卷、直接装在主机上、非 Linux、看不出来）都返回 true。
func Persistent(dir string) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	mountinfo, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return true
	}
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return true
	}
	return persistentIn(string(mountinfo), absolute)
}

// persistentIn 在 /proc/self/mountinfo 的内容里找 dir 所在的挂载点（最长的前缀）。
func persistentIn(mountinfo string, dir string) bool {
	dir = filepath.Clean(dir)
	best, bestType := "", ""
	for _, line := range strings.Split(mountinfo, "\n") {
		fields := strings.Fields(line)
		separator := -1
		for i := 6; i < len(fields); i++ {
			if fields[i] == "-" {
				separator = i
				break
			}
		}
		if len(fields) < 5 || separator < 0 || separator+1 >= len(fields) {
			continue
		}
		mountPoint := unescape(fields[4])
		if !within(dir, mountPoint) || len(mountPoint) < len(best) {
			continue
		}
		best, bestType = mountPoint, fields[separator+1]
	}
	return !(best == "/" && bestType == "overlay")
}

func within(dir string, mountPoint string) bool {
	return mountPoint == "/" || dir == mountPoint || strings.HasPrefix(dir, mountPoint+"/")
}

// unescape 还原 mountinfo 里转义的字符（空格写成 \040 等）。
func unescape(value string) string {
	if !strings.Contains(value, `\`) {
		return value
	}
	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+3 < len(value) {
			if code, err := strconv.ParseUint(value[i+1:i+4], 8, 8); err == nil {
				builder.WriteByte(byte(code))
				i += 3
				continue
			}
		}
		builder.WriteByte(value[i])
	}
	return builder.String()
}
