package gitsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ApplyPatchSeries 把一份 mbox 补丁（git format-patch / git send-email 产出）
// 应用到 base 分支的新分支 patches/<ts> 上，并 push 回 bare 仓库。
// 返回新分支名与末端提交 SHA。
//
// patchData 可以是多封 patch 拼接的 mbox；git am 会依次提交。
func applyPatchSeries(owner, name, base string, patchData []byte) (branch, sha string, err error) {
	if !ValidName(owner) || !ValidName(name) {
		return "", "", fmt.Errorf("invalid repo")
	}
	base = strings.TrimSpace(strings.TrimPrefix(base, "refs/heads/"))
	if base == "" {
		base = "main"
	}
	if !ValidRef(base) {
		return "", "", fmt.Errorf("invalid base branch %q", base)
	}
	if len(strings.TrimSpace(string(patchData))) == 0 {
		return "", "", fmt.Errorf("empty patch series")
	}
	bare, err := filepath.Abs(repoPath(owner, name))
	if err != nil {
		return "", "", err
	}
	if fi, err := os.Stat(bare); err != nil || !fi.IsDir() {
		return "", "", fmt.Errorf("repo %s/%s not on disk", owner, name)
	}
	tmp, err := os.MkdirTemp("", "gitdash-patch-*")
	if err != nil {
		return "", "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	init := func() error {
		if _, err := gitOut(tmp, "init", "-q", "--initial-branch=_gd_patch"); err == nil {
			return nil
		}
		if _, err := gitOut(tmp, "init", "-q"); err != nil {
			return err
		}
		_, err := gitOut(tmp, "checkout", "-q", "-b", "_gd_patch")
		return err
	}
	if err := init(); err != nil {
		return "", "", err
	}
	if _, err := gitOut(tmp, "remote", "add", "origin", bare); err != nil {
		return "", "", err
	}
	if _, err := gitOut(tmp, "fetch", "-q", "origin"); err != nil {
		return "", "", err
	}
	if _, err := gitOut(tmp, "rev-parse", "-q", "--verify", "refs/remotes/origin/"+base); err != nil {
		return "", "", fmt.Errorf("base branch %q not found", base)
	}
	if _, err := gitOut(tmp, "checkout", "-q", "-B", "_gd_patch", "origin/"+base); err != nil {
		return "", "", err
	}
	// git am 会用补丁里的 Author，但需要 committer 身份。
	if _, err := gitOut(tmp, "config", "user.name", "gitdash"); err != nil {
		return "", "", err
	}
	if _, err := gitOut(tmp, "config", "user.email", "noreply@gitdash.local"); err != nil {
		return "", "", err
	}
	// 分支名带纳秒时间戳，避免同一时刻并发冲突；如仍存在则追加随机后缀。
	branch = fmt.Sprintf("patches/%d", time.Now().UnixNano())
	for i := 1; ; i++ {
		_, e := gitOut(tmp, "rev-parse", "-q", "--verify", "refs/remotes/origin/"+branch)
		if e != nil {
			break
		}
		branch = fmt.Sprintf("patches/%d-%d", time.Now().UnixNano(), i)
	}
	if _, err := gitOut(tmp, "checkout", "-q", "-b", branch); err != nil {
		return "", "", err
	}
	patchFile := filepath.Join(tmp, "series.mbox")
	if err := os.WriteFile(patchFile, patchData, 0o600); err != nil {
		return "", "", err
	}
	if _, err := gitOut(tmp, "am", "--3way", patchFile); err != nil {
		_, _ = gitOut(tmp, "am", "--abort")
		return "", "", fmt.Errorf("apply patch failed: %w", err)
	}
	if _, err := gitOut(tmp, "push", "-q", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return "", "", fmt.Errorf("push failed: %w", err)
	}
	out, err := gitOut(tmp, "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	return branch, strings.TrimSpace(out), nil
}
