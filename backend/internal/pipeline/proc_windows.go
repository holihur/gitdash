//go:build windows

package pipeline

import "os/exec"

// configureProcAttr Windows 下无进程组语义，保持默认。
func configureProcAttr(cmd *exec.Cmd) {}

// killProcessGroup 仅终止直接子进程（Windows 无进程组）。
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
