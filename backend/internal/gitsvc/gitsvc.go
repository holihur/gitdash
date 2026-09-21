package gitsvc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	reposDir string
	spoolDir string
	nameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	refRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

func initDirs(dataDir string) error {
	reposDir = filepath.Join(dataDir, "repos")
	spoolDir = filepath.Join(dataDir, "webhook-events")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(spoolDir, 0o755)
}

func ValidName(name string) bool {
	return nameRe.MatchString(name)
}

func ValidRef(ref string) bool {
	return ref != "" && !strings.Contains(ref, "..") && refRe.MatchString(ref)
}

// CleanPath validates a repo-internal path like "src/main.go".
func CleanPath(p string) (string, error) {
	p = strings.Trim(p, "/")
	if p == "" {
		return "", nil
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return "", fmt.Errorf("invalid path %q", p)
		}
	}
	return p, nil
}

func repoPath(owner, name string) string {
	return filepath.Join(reposDir, owner, name+".git")
}

func repoExists(owner, name string) bool {
	fi, err := os.Stat(repoPath(owner, name))
	return err == nil && fi.IsDir()
}

// IsEmptyRepo 报告仓库是否尚无任何提交（没有任何分支或标签引用）。
// 空仓库没有可检出的默认分支，浏览接口应据此返回空结果而非原始 git 报错。
func isEmptyRepo(owner, name string) bool {
	out, err := gitOut(repoPath(owner, name), "for-each-ref", "--count=1")
	return err == nil && strings.TrimSpace(out) == ""
}

func gitOut(dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", gitErr(dir, args, stderr.String(), err)
	}
	return stdout.String(), nil
}

// gitOutLimit 与 gitOut 相同，但最多读取 max 字节；超出时截断并终止子进程。
// 用于 diff 等可能产生巨量输出的场景，避免整份输出先进内存再截断（UGC 可推送
// 超大改动，git diff 的输出可达数 GB）。
func gitOutLimit(max int, dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", gitErr(dir, args, stderr.String(), err)
	}
	data, _ := io.ReadAll(io.LimitReader(stdout, int64(max)+1))
	if len(data) > max {
		data = data[:max]
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return string(data) + "\n... (truncated)\n", nil
	}
	if err := cmd.Wait(); err != nil {
		return "", gitErr(dir, args, stderr.String(), err)
	}
	return string(data), nil
}

// gitErr 构造不含服务器绝对路径的对外错误信息（完整细节记入服务端日志），
// 避免数据目录结构通过 5xx 响应泄漏给客户端。
func gitErr(dir string, args []string, stderr string, err error) error {
	msg := strings.TrimSpace(stderr)
	if msg == "" {
		msg = err.Error()
	}
	display := strings.Join(args, " ")
	if reposDir != "" {
		display = strings.ReplaceAll(display, reposDir+string(os.PathSeparator), "")
		msg = strings.ReplaceAll(msg, reposDir+string(os.PathSeparator), "")
		display = strings.ReplaceAll(display, reposDir, "")
		msg = strings.ReplaceAll(msg, reposDir, "")
	}
	return fmt.Errorf("git %s: %s", display, msg)
}

// gitOutEnv 与 gitOut 相同，但额外注入环境变量（如导入私有仓库时指定临时 SSH 私钥）。
// remoteGitTimeout 远程 git 操作（clone --mirror / push --mirror）的超时，
// 防止慢速或超大远端长期占用工作协程。GITDASH_GIT_OP_TIMEOUT_MS 可覆盖。
func remoteGitTimeout() time.Duration {
	const def = 30 * time.Minute
	if v := strings.TrimSpace(os.Getenv("GITDASH_GIT_OP_TIMEOUT_MS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return time.Duration(n) * time.Millisecond
		}
	}
	return def
}

func gitOutEnv(env []string, dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), remoteGitTimeout())
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("git operation timed out")
		}
		return "", gitErr(dir, args, stderr.String(), err)
	}
	return stdout.String(), nil
}
