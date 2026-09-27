package gitsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPipelineYAML 是默认模版写入仓库根目录的 .gitdash.yml 示例流水线。
// 开启仓库「流水线」开关后，push 即会触发；DSL 语法详见 README「CI 流水线」章节。
const DefaultPipelineYAML = `# .gitdash.yml - gitdash CI pipeline configuration
# Enable the pipeline in the repo's Pipeline tab, then push to trigger it.
# See the README "CI Pipeline" section for the full DSL reference.
image: alpine:3.19

steps:
  - name: hello
    run: |
      echo "Hello from gitdash pipeline"
      echo "repo: ${GITDASH_REPO}"
      echo "ref:  ${GITDASH_REF}"
      echo "sha:  ${GITDASH_SHA}"
`

// InitTemplate 把刚创建的 bare 仓库初始化为默认模版：
// main 分支 + 以仓库名生成的 README.md + 示例流水线 .gitdash.yml。
func initTemplate(owner, name string) error {
	bare, err := filepath.Abs(repoPath(owner, name))
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
	if err := os.WriteFile(filepath.Join(tmp, ".gitdash.yml"), []byte(DefaultPipelineYAML), 0o644); err != nil {
		return err
	}
	if _, err := gitOut(tmp, "config", "user.name", "gitdash"); err != nil {
		return err
	}
	if _, err := gitOut(tmp, "config", "user.email", "noreply@gitdash.local"); err != nil {
		return err
	}
	if _, err := gitOut(tmp, "add", "README.md", ".gitdash.yml"); err != nil {
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

// FileChange 一次网页端文件/目录操作（action: create | update | delete | delete_tree | move）。
// move 用 From 指定原路径、Path 指定新路径，支持文件与目录（底层等价于 git mv）。
type FileChange struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Content string `json:"content"`
	From    string `json:"from,omitempty"`
}

// WriteCommit 在目标分支上应用一组文件操作并提交（bare 仓库在临时工作区完成）。
// branch 不存在（空仓库）时会以该分支名创建首个提交。返回提交 SHA。
func writeCommit(owner, name, branch, message, author string, changes []FileChange) (string, error) {
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
	bare, err := filepath.Abs(repoPath(owner, name))
	if err != nil {
		return "", err
	}
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
			if (c.Action == "create" || c.Action == "update" || c.Action == "move") && c.Path != "" {
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
			if err := safeWriteFile(tmp, p, []byte(c.Content)); err != nil {
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
		case "move":
			from, err := CleanPath(c.From)
			if err != nil {
				return "", err
			}
			if from == "" || from == p {
				continue
			}
			// 目标父目录需先存在，git mv 不会自动创建中间目录。逐段校验
			// 拒绝符号链接分量，防止经由仓库内 symlink 写入宿主目录。
			if _, err := safeParentDir(tmp, filepath.ToSlash(filepath.Dir(filepath.FromSlash(p)))); err != nil {
				return "", err
			}
			if _, err := gitOut(tmp, "mv", "--", from, p); err != nil {
				return "", fmt.Errorf("move %q -> %q: %w", from, p, err)
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

// safeParentDir 逐段创建/校验 tmp 下的目录，拒绝任何符号链接分量，
// 确保最终目录仍位于 tmp 内。rel 为斜杠分隔的相对目录（可为 "."）。
//
// 安全审计 A3：CleanPath 只拦 "."/".."，无法识别仓库里提交的 symlink，
// 因此 Web 文件编辑器会被诱导跟随 symlink 写入宿主任意路径。
func safeParentDir(tmp, rel string) (string, error) {
	cur := tmp
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == "" || part == "." {
			continue
		}
		if part == ".." {
			return "", fmt.Errorf("invalid path %q", rel)
		}
		cur = filepath.Join(cur, part)
		fi, err := os.Lstat(cur)
		if err == nil {
			if fi.Mode()&os.ModeSymlink != 0 {
				return "", fmt.Errorf("refusing to follow symlink in path %q", rel)
			}
			if !fi.IsDir() {
				return "", fmt.Errorf("path component is not a directory: %q", rel)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if err := os.Mkdir(cur, 0o755); err != nil && !os.IsExist(err) {
			return "", err
		}
	}
	return cur, nil
}

// safeWriteFile 把内容写入 tmp 内相对路径 p：中间目录逐段拒绝符号链接，
// 目标文件以 O_NOFOLLOW 打开，确保仓库内提交的 symlink 无法逃逸出工作区。
func safeWriteFile(tmp, p string, content []byte) error {
	native := filepath.FromSlash(p)
	parent, err := safeParentDir(tmp, filepath.ToSlash(filepath.Dir(native)))
	if err != nil {
		return err
	}
	dst := filepath.Join(parent, filepath.Base(native))
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY|oNoFollow, 0o644)
	if err != nil {
		return err
	}
	if _, err := f.Write(content); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
