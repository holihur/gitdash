package gitsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func InitReadme(owner, name string) error {
	bare, err := filepath.Abs(RepoPath(owner, name))
	if err != nil {
		return err
	}
	if fi, err := os.Stat(bare); err != nil || !fi.IsDir() {
		return fmt.Errorf("repo %s/%s not on disk", owner, name)
	}
	tmp, err := os.MkdirTemp("", "gitdash-tpl-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	initBranch := func() error {
		if _, err := gitOut(tmp, "init", "-q", "--initial-branch=main"); err == nil {
			return nil
		}
		if _, err := gitOut(tmp, "init", "-q"); err != nil {
			return err
		}
		_, err := gitOut(tmp, "checkout", "-q", "-b", "main")
		return err
	}
	if err := initBranch(); err != nil {
		return err
	}
	readme := "# " + name + "\n"
	if err := os.WriteFile(filepath.Join(tmp, "README.md"), []byte(readme), 0o644); err != nil {
		return err
	}
	if _, err := gitOut(tmp, "config", "user.name", "gitdash"); err != nil {
		return err
	}
	if _, err := gitOut(tmp, "config", "user.email", "noreply@gitdash.local"); err != nil {
		return err
	}
	if _, err := gitOut(tmp, "add", "README.md"); err != nil {
		return err
	}
	if _, err := gitOut(tmp, "commit", "-q", "-m", "Initial commit"); err != nil {
		return err
	}
	if _, err := gitOut(tmp, "push", "-q", bare, "HEAD:refs/heads/main"); err != nil {
		return err
	}
	return nil
}

// FileChange 一次网页端文件/目录操作（action: create | update | delete | delete_tree）。
type FileChange struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Content string `json:"content"`
}

// WriteCommit 在目标分支上应用一组文件操作并提交（bare 仓库在临时工作区完成）。
// branch 不存在（空仓库）时会以该分支名创建首个提交。返回提交 SHA。
func WriteCommit(owner, name, branch, message, author string, changes []FileChange) (string, error) {
	if !ValidName(owner) || !ValidName(name) {
		return "", fmt.Errorf("invalid repo")
	}
	if !ValidRef(branch) {
		return "", fmt.Errorf("invalid branch %q", branch)
	}
	if strings.TrimSpace(message) == "" {
		return "", fmt.Errorf("commit message is required")
	}
	if len(changes) == 0 {
		return "", fmt.Errorf("no changes to commit")
	}
	bare := RepoPath(owner, name)
	if fi, err := os.Stat(bare); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("repo %s/%s not on disk", owner, name)
	}
	tmp, err := os.MkdirTemp("", "gitdash-write-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	init := func() error {
		if _, err := gitOut(tmp, "init", "-q", "--initial-branch=_gd_work"); err == nil {
			return nil
		}
		if _, err := gitOut(tmp, "init", "-q"); err != nil {
			return err
		}
		_, err := gitOut(tmp, "checkout", "-q", "-b", "_gd_work")
		return err
	}
	if err := init(); err != nil {
		return "", err
	}
	if _, err := gitOut(tmp, "remote", "add", "origin", bare); err != nil {
		return "", err
	}
	if _, err := gitOut(tmp, "fetch", "-q", "origin"); err != nil {
		return "", err
	}
	// 若分支已存在，把工作区切到该分支
	if _, err := gitOut(tmp, "rev-parse", "-q", "--verify", "refs/remotes/origin/"+branch); err == nil {
		if _, err := gitOut(tmp, "checkout", "-q", "-B", "_gd_work", "origin/"+branch); err != nil {
			return "", err
		}
	}

	// 目录内出现第一个真实文件时，自动移除隐藏占位 .gitkeep
	if lsOut, err := gitOut(tmp, "ls-files"); err == nil {
		tracked := map[string]bool{}
		for _, ln := range strings.Split(strings.TrimSpace(lsOut), "\n") {
			if ln != "" {
				tracked[ln] = true
			}
		}
		cleaned := map[string]bool{}
		for _, c := range changes {
			if (c.Action == "create" || c.Action == "update") && c.Path != "" {
				if i := strings.LastIndex(c.Path, "/"); i > 0 {
					gk := c.Path[:i] + "/.gitkeep"
					if tracked[gk] && !cleaned[gk] {
						cleaned[gk] = true
						changes = append(changes, FileChange{Path: gk, Action: "delete"})
					}
				}
			}
		}
	}

	for _, c := range changes {
		p, err := CleanPath(c.Path)
		if err != nil {
			return "", err
		}
		switch c.Action {
		case "create", "update":
			file := filepath.Join(tmp, filepath.FromSlash(p))
			parent := filepath.Dir(file)
			if _, err := os.Stat(parent); err != nil {
				if err := os.MkdirAll(parent, 0o755); err != nil {
					return "", err
				}
			}
			if err := os.WriteFile(file, []byte(c.Content), 0o644); err != nil {
				return "", err
			}
		case "delete":
			if _, err := gitOut(tmp, "rm", "-q", "--", p); err != nil {
				return "", fmt.Errorf("delete %q: %w", p, err)
			}
		case "delete_tree":
			if _, err := gitOut(tmp, "rm", "-q", "-r", "--", p); err != nil {
				return "", fmt.Errorf("delete directory %q: %w", p, err)
			}
		default:
			return "", fmt.Errorf("invalid action %q", c.Action)
		}
	}
	if _, err := gitOut(tmp, "add", "-A"); err != nil {
		return "", err
	}
	if _, err := gitOut(tmp, "config", "user.name", author); err != nil {
		return "", err
	}
	if _, err := gitOut(tmp, "config", "user.email", author+"@gitdash.local"); err != nil {
		return "", err
	}
	if _, err := gitOut(tmp, "commit", "-q", "-m", message); err != nil {
		return "", fmt.Errorf("commit failed: %w", err)
	}
	if _, err := gitOut(tmp, "push", "-q", "origin", "HEAD:refs/heads/"+branch); err != nil {
		return "", fmt.Errorf("push failed: %w", err)
	}
	out, err := gitOut(tmp, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// Tag 轻量/附注标签信息（sha 为指向的提交）。
