//go:build !windows

package pipeline

import (
	"os/exec"
	"syscall"
)

// configureProcAttr 让子进程成为独立进程组组长，便于超时/取消时整组终止。
func configureProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup 杀掉命令所在进程组（含 sh 派生的子进程）。
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
