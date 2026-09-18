package gitsvc

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var hexSHARe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

// RevertCommit 在 branch 上创建一个撤销 sha 变更的新提交（git revert），返回新提交 SHA。
// sha 为 merge 提交时按第一父提交撤销（-m 1）。冲突时报错并放弃，不影响远端分支。
func revertCommit(owner, name, branch, sha, message, committer string) (string, error) {
	if !ValidName(owner) || !ValidName(name) {
		return "", fmt.Errorf("invalid repo")
	}
	if !ValidRef(branch) {
		return "", fmt.Errorf("invalid branch %q", branch)
	}
	sha = strings.TrimSpace(sha)
	if !hexSHARe.MatchString(sha) {
		return "", fmt.Errorf("invalid commit %q", sha)
	}
	path := repoPath(owner, name)
	tmp, err := os.MkdirTemp("", "gitdash-revert-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	if _, err := gitOut("", "clone", "-q", "--no-local", path, tmp); err != nil {
		return "", err
	}
	ident := []string{"-c", "user.name=" + committer, "-c", "user.email=" + committer + "@gitdash"}
	git := func(args ...string) (string, error) { return gitOut(tmp, args...) }
	if _, err := git("checkout", "-q", "-B", "_gd_revert", "origin/"+branch); err != nil {
		return "", err
	}
	subject, err := git("log", "-1", "--format=%s", sha)
	if err != nil {
		return "", fmt.Errorf("commit not found: %s", sha)
	}
	message = strings.TrimSpace(message)
	if message == "" {
		message = fmt.Sprintf("Revert %q", strings.TrimSpace(subject))
	}
	args := []string{"revert", "--no-commit"}
	// merge 提交需指定主父提交（第一父）
	if parents, perr := git("rev-list", "--parents", "-n", "1", sha); perr == nil {
		if len(strings.Fields(parents)) > 2 {
			args = append(args, "-m", "1")
		}
	}
	args = append(args, sha)
	if _, err := git(args...); err != nil {
		_, _ = git("revert", "--abort")
		return "", fmt.Errorf("revert conflict or error: %w", err)
	}
	if _, err := git(append(ident, "commit", "-q", "-m", message)...); err != nil {
		return "", fmt.Errorf("revert commit failed: %w", err)
	}
	out, err := git("rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	head := strings.TrimSpace(out)
	if _, err := git("push", "-q", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return "", err
	}
	return head, nil
}
