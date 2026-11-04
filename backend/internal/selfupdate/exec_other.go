//go:build !unix

package selfupdate

import "errors"

// 非 Unix 系统不支持在线更新（发布包只有 Linux 版本）。
func execProcess(string, []string, []string) error {
	return errors.New("online update is only supported on Linux")
}
