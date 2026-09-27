//go:build !windows

package gitsvc

import "syscall"

// oNoFollow 让 open(2) 在目标是符号链接时返回 ELOOP。写工作区文件时用它
// 防止仓库内提交的 symlink 被跟随（安全审计 A3）。
const oNoFollow = syscall.O_NOFOLLOW
