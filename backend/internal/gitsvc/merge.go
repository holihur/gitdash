package gitsvc

import (
	"fmt"
	"os"
	"strings"
)

func MergeNonFF(owner, name, target, source, message, committer, method string) (string, error) {
	path := RepoPath(owner, name)
	tmp, err := os.MkdirTemp("", "gitdash-merge-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if _, err := gitOut("", "clone", "-q", "--no-local", path, tmp); err != nil {
		return "", err
	}
	ident := []string{"-c", "user.name=" + committer, "-c", "user.email=" + committer + "@gitdash"}
	git := func(args ...string) (string, error) { return gitOut(tmp, args...) }
	if _, err := git(append([]string{"checkout", "-q", "-B", "_gd_target", "origin/" + target}, nil...)...); err != nil {
		return "", err
	}
	var head string
	switch method {
	case "squash":
		if _, merr := git(append(ident, "merge", "--squash", "-q", "origin/"+source)...); merr != nil {
			return "", fmt.Errorf("merge conflict or error: %w", merr)
		}
		if _, err := git(append(ident, "commit", "-q", "-m", message)...); err != nil {
			return "", fmt.Errorf("merge conflict: %w", err)
		}
	case "merge":
		if _, err := git(append(ident, "merge", "--no-ff", "-q", "-m", message, "origin/"+source)...); err != nil {
			return "", fmt.Errorf("merge conflict or error: %w", err)
		}
	default:
		return "", fmt.Errorf("unsupported merge method %q", method)
	}
	out, err := git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	head = strings.TrimSpace(out)
	if _, err := git("push", "-q", "origin", "HEAD:refs/heads/"+target); err != nil {
		return "", err
	}
	return head, nil
}

// MergeRebase 把 source 的提交逐个变基到 target 之上（rebase and merge），
// 线性历史；成功后目标分支快进到变基结果并返回新的 tip SHA。冲突时返回错误。
func MergeRebase(owner, name, target, source, committer string) (string, error) {
	path := RepoPath(owner, name)
	tmp, err := os.MkdirTemp("", "gitdash-rebase-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if _, cerr := gitOut("", "clone", "-q", "--no-local", path, tmp); cerr != nil {
		return "", cerr
	}
	ident := []string{"-c", "user.name=" + committer, "-c", "user.email=" + committer + "@gitdash"}
	if _, cerr := gitOut(tmp, append(ident, "checkout", "-q", "-B", "_gd_rebase", "origin/"+source)...); cerr != nil {
		return "", cerr
	}
	if _, rerr := gitOut(tmp, append(ident, "rebase", "--quiet", "origin/"+target)...); rerr != nil {
		_, _ = gitOut(tmp, "rebase", "--abort")
		return "", fmt.Errorf("rebase conflict or error: %w", rerr)
	}
	out, err := gitOut(tmp, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(out)
	if _, err := gitOut(tmp, "push", "-q", "origin", "HEAD:refs/heads/"+target); err != nil {
		return "", err
	}
	return head, nil
}

// InitTemplate 把刚创建的 bare 仓库初始化为默认模版：main 分支 + 以仓库名生成的 README.md + 示例流水线 .gitdash.yml。
