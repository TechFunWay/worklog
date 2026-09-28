//go:build windows

package worklog

import "os"

// restartProcess 在 Windows 上没有 syscall.Exec：数据文件已就位，直接退出，
// 交给服务管理器（如 NSSM / 计划任务）把进程拉起来。
func restartProcess() error {
	os.Exit(0)
	return nil
}
