package gitsvc

import (
	"fmt"
	"os"
)

// GCResult 仓库垃圾回收的结果（字节）。
type GCResult struct {
	BeforeBytes int64 `json:"before_bytes"`
	AfterBytes  int64 `json:"after_bytes"`
	FreedBytes  int64 `json:"freed_bytes"`
}

// GC 对仓库执行 `git gc`：打包松散对象、合并 pack 并清理不再引用的对象。
// 仅所有者可触发（权限由 API 层校验）。返回执行前后的磁盘占用，
// 便于前端展示本次回收释放的空间。
func GC(owner, name string) (*GCResult, error) {
	if !ValidName(owner) || !ValidName(name) {
		return nil, fmt.Errorf("invalid repo %s/%s", owner, name)
	}
	path := RepoPath(owner, name)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("repo %s/%s not on disk", owner, name)
	}

	before, _ := RepoSize(owner, name)
	if _, err := gitOut(path, "gc", "--quiet"); err != nil {
		return nil, err
	}
	after, _ := RepoSize(owner, name)

	return &GCResult{
		BeforeBytes: before,
		AfterBytes:  after,
		FreedBytes:  before - after,
	}, nil
}
