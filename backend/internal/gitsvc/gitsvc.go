package gitsvc

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	reposDir string
	spoolDir string
	nameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	refRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

func Init(dataDir string) error {
	reposDir = filepath.Join(dataDir, "repos")
	spoolDir = filepath.Join(dataDir, "webhook-events")
	if err := os.MkdirAll(reposDir, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(spoolDir, 0o755)
}

func ReposDir() string { return reposDir }

// SpoolDir push 事件 spool 目录（post-receive hook 写入，webhook 调度器消费）
func SpoolDir() string { return spoolDir }

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

func RepoPath(owner, name string) string {
	return filepath.Join(reposDir, owner, name+".git")
}

func Exists(owner, name string) bool {
	fi, err := os.Stat(RepoPath(owner, name))
	return err == nil && fi.IsDir()
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

// GitOut 是 gitOut 的导出包装（供 pipeline 等内部包复用）。
func GitOut(dir string, args ...string) (string, error) { return gitOut(dir, args...) }

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
func gitOutEnv(env []string, dir string, args ...string) (string, error) {
	if dir != "" {
		args = append([]string{"-C", dir}, args...)
	}
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", gitErr(dir, args, stderr.String(), err)
	}
	return stdout.String(), nil
}
