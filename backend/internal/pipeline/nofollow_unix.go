//go:build !windows

package pipeline

import "syscall"

// oNoFollow 让 open(2) 在目标是符号链接时返回 ELOOP（见 copyFile 的注释）。
const oNoFollow = syscall.O_NOFOLLOW
