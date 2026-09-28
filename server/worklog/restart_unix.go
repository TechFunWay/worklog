//go:build !windows

package worklog

import (
	"os"
	"syscall"
)

// restartProcess 重新执行自身进程：监听端口等由 Go 运行时按 close-on-exec
// 处理，新进程按原启动参数重新拉起，等于一次干净的重启。
func restartProcess() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return syscall.Exec(exe, os.Args, os.Environ())
}
