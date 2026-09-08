package gitsvc

import (
	"fmt"
	"strconv"
	"strings"
)

func RevSHA(owner, name, rev string) (string, error) {
	if !ValidName(owner) || !ValidName(name) || rev == "" || strings.Contains(rev, "..") {
		return "", fmt.Errorf("invalid rev %q", rev)
	}
	out, err := gitOut(RepoPath(owner, name), "rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// CanFastForward 判断 target 是否可 fast-forward 到 source（target 是 source 的祖先）。
func CanFastForward(owner, name, target, source string) bool {
	_, err := gitOut(RepoPath(owner, name), "merge-base", "--is-ancestor", "refs/heads/"+target, "refs/heads/"+source)
	return err == nil
}

// MergeCheck PR 可合并性预检：mergeable（可合并）与 conflicted（分支分叉且合并冲突）。
func MergeCheck(owner, name, target, source string) (mergeable, conflicted bool) {
	if CanFastForward(owner, name, target, source) {
		return true, false
	}
	// merge-tree 干跑（不落盘）：--write-tree 退出码 0 = 干净合并，1 = 冲突
	base, err := gitOut(RepoPath(owner, name), "merge-base", "refs/heads/"+target, "refs/heads/"+source)
	if err != nil {
		return false, false
	}
	_, merr := gitOut(RepoPath(owner, name), "merge-tree", "--write-tree",
		strings.TrimSpace(base), "refs/heads/"+source)
	if merr == nil {
		return true, false
	}
	return false, true
}

// MergeFastForward 把 target 分支快进到 source 分支，返回新的 target SHA。
func MergeFastForward(owner, name, target, source string) (string, error) {
	sha, err := RevSHA(owner, name, "refs/heads/"+source)
	if err != nil {
		return "", fmt.Errorf("source branch %q missing", source)
	}
	if !CanFastForward(owner, name, target, source) {
		return "", fmt.Errorf("cannot fast-forward %q to %q (diverged)", target, source)
	}
	if _, err := gitOut(RepoPath(owner, name), "update-ref", "refs/heads/"+target, sha); err != nil {
		return "", err
	}
	return sha, nil
}

type DiffFile struct {
	Path       string `json:"path"`
	Status     string `json:"status"` // A / M / D
	Insertions int    `json:"insertions"`
	Deletions  int    `json:"deletions"`
}

// DiffStats base..head 变更文件统计。
func DiffStats(owner, name, base, head string) ([]DiffFile, error) {
	numstat, err := gitOut(RepoPath(owner, name), "diff", "--numstat", base, head)
	if err != nil {
		return nil, err
	}
	statuses, err := gitOut(RepoPath(owner, name), "diff", "--name-status", base, head)
	if err != nil {
		return nil, err
	}
	stat := map[string]struct{ ins, del int }{}
	for _, ln := range strings.Split(strings.TrimSpace(numstat), "\n") {
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "	", 3)
		if len(parts) < 3 {
			continue
		}
		ins, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		stat[parts[2]] = struct{ ins, del int }{ins, del}
	}
	files := []DiffFile{}
	for _, ln := range strings.Split(strings.TrimSpace(statuses), "\n") {
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "	", 2)
		if len(parts) != 2 {
			continue
		}
		st := strings.SplitN(parts[0], "	", 1)[0]
		path := parts[1]
		if strings.Contains(st, "R") { // 重命名：取目标路径
			path = strings.SplitN(path, "	", 1)[0]
		}
		d := stat[path]
		files = append(files, DiffFile{Path: path, Status: st[:1], Insertions: d.ins, Deletions: d.del})
	}
	return files, nil
}

// DiffPatch 返回 base..head 的统一 diff 文本（截断防滥用）。
func DiffPatch(owner, name, base, head string) (string, error) {
	out, err := gitOut(RepoPath(owner, name), "diff", "-U3", base, head)
	if err != nil {
		return "", err
	}
	const max = 512 * 1024
	if len(out) > max {
		out = out[:max] + "\n... (truncated)\n"
	}
	return out, nil
}

// RawCommit 返回提交对象的原始内容（含 gpgsig 头，供签名校验）。
