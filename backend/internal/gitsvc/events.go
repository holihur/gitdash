package gitsvc

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WritePushEvent 安全地把一条 push 事件写入 webhook spool。
//
// 由 `gitdash post-receive` 子命令调用（post-receive hook 通过管道把
// `oldrev newrev refname` 传进来）。JSON 完全由 encoding/json 生成，
// ref/user 等可控字段不会被拼接注入——旧实现用 shell printf 拼 JSON，
// 恶意的分支名（git 允许 `"`,`,`,`{`,`}` 等）可伪造 owner/repo 等字段。
func WritePushEvent(owner, repo, oldrev, newrev, ref, user string) error {
	spool := SpoolDir()
	if spool == "" {
		return nil
	}
	if err := os.MkdirAll(spool, 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(map[string]string{
		"event":      "push",
		"owner":      owner,
		"repo":       repo,
		"old":        oldrev,
		"new":        newrev,
		"ref":        ref,
		"user":       user,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	name := fmt.Sprintf("%s__%s-%d-%d.json", owner, repo, os.Getpid(), time.Now().UnixNano())
	tmp := filepath.Join(spool, name+".tmp")
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(spool, name))
}
