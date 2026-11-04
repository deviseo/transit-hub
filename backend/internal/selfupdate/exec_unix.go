//go:build unix

package selfupdate

import "syscall"

// execProcess 用新程序替换当前进程（进程号不变，容器里仍是 1 号进程）。
func execProcess(binary string, args []string, env []string) error {
	return syscall.Exec(binary, args, env)
}
