//go:build windows

package pipeline

// Windows 没有 O_NOFOLLOW；符号链接创建需要特权，风险较低，退化为 0。
const oNoFollow = 0
