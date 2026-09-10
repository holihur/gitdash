package gitsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func CreateBare(owner, name string) error {
	if !ValidName(owner) || !ValidName(name) {
		return fmt.Errorf("invalid repo %s/%s", owner, name)
	}
	path := RepoPath(owner, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if _, err := gitOut("", "init", "--bare", "--initial-branch=main", path); err != nil {
		// older git without --initial-branch
		if _, err2 := gitOut("", "init", "--bare", path); err2 != nil {
			return err
		}
	}
	// allow deleting the default branch in a bare repo
	_, _ = gitOut(path, "config", "receive.denyDeleteCurrent", "ignore")
	return installPostReceiveHook(path, owner, name)
}

// installPostReceiveHook 安装 post-receive hook：把每次 push 事件写成一行 JSON
// 追加到 spool 目录下的独立文件，供服务端 webhook 调度器投递。
func installPostReceiveHook(repoPath, owner, name string) error {
	hooksDir := filepath.Join(repoPath, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	script := `#!/bin/sh
# gitdash: record push events (consumed by the webhook dispatcher)
while read oldrev newrev refname; do
	[ -z "$refname" ] && continue
	f="@SPOOL@/@OWNER@__@REPO@-$$-$(date +%s%N).json"
	printf '{"event":"push","owner":"@OWNER@","repo":"@REPO@","old":"%s","new":"%s","ref":"%s","user":"%s","created_at":"%s"}\n' \
		"$oldrev" "$newrev" "$refname" "${GITDASH_USER:-}" "$(date -u +%Y-%m-%dT%H:%M:%SZ)" >> "$f"
done
`
	script = strings.NewReplacer(
		"@SPOOL@", spoolDir,
		"@OWNER@", owner,
		"@REPO@", name,
	).Replace(script)
	hook := filepath.Join(hooksDir, "post-receive")
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		return err
	}
	return installPreReceiveHook(hooksDir, owner, name)
}

// installPreReceiveHook 安装 pre-receive hook：分支保护（禁删/禁强推）在 push 落盘前校验。
// 校验逻辑在 gitdash 主程序（gitdash pre-receive 子命令）里查库执行；二进制缺失时放行，避免阻断 push。
func installPreReceiveHook(hooksDir, owner, name string) error {
	self, err := os.Executable()
	if err != nil || self == "" {
		self = "gitdash" // 退化为 PATH 查找
	}
	script := `#!/bin/sh
# gitdash: branch protection (deletion / force-push) enforced before update
while read oldrev newrev refname; do
	[ -z "$refname" ] && continue
	echo "$oldrev $newrev $refname"
done | @BIN@ pre-receive "@OWNER@" "@REPO@"
`
	script = strings.NewReplacer(
		"@BIN@", self,
		"@OWNER@", owner,
		"@REPO@", name,
	).Replace(script)
	return os.WriteFile(filepath.Join(hooksDir, "pre-receive"), []byte(script), 0o755)
}

// EnsureHooks 遍历所有仓库重装 hooks（启动时调用，让存量仓库获得 pre-receive 分支保护）。
func EnsureHooks() error {
	entries, err := os.ReadDir(reposDir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		owner := e.Name()
		repos, rerr := os.ReadDir(filepath.Join(reposDir, owner))
		if rerr != nil {
			continue
		}
		for _, r := range repos {
			if !strings.HasSuffix(r.Name(), ".git") {
				continue
			}
			_ = installPostReceiveHook(filepath.Join(reposDir, owner, r.Name()), owner, strings.TrimSuffix(r.Name(), ".git"))
		}
	}
	return nil
}

func Delete(owner, name string) error {
	InvalidateRefs(owner, name)
	return os.RemoveAll(RepoPath(owner, name))
}

// ForkRepo 把源仓库（bare）镜像复制到目标路径，用于 fork：保留全部分支/标签。
func ForkRepo(sourceOwner, sourceName, targetOwner, targetName string) error {
	if !ValidName(sourceOwner) || !ValidName(sourceName) || !ValidName(targetOwner) || !ValidName(targetName) {
		return fmt.Errorf("invalid fork repo")
	}
	src := RepoPath(sourceOwner, sourceName)
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return fmt.Errorf("source repo %s/%s not on disk", sourceOwner, sourceName)
	}
	dst := RepoPath(targetOwner, targetName)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := gitOut("", "clone", "--mirror", "--quiet", src, dst); err != nil {
		return err
	}
	// mirror clone 不继承源 hooks；重新安装 post-receive 并放开默认分支删除限制
	_, _ = gitOut(dst, "config", "receive.denyDeleteCurrent", "ignore")
	return installPostReceiveHook(dst, targetOwner, targetName)
}

// ImportRepo 从远程 URL 镜像导入仓库到目标路径（保留全部分支/标签）。
// privateKey 非空时用于 SSH 认证（专用导入 key，如 GitHub/GitLab 的只读 deploy key）。
func ImportRepo(url, targetOwner, targetName, privateKey string) error {
	if !ValidName(targetOwner) || !ValidName(targetName) {
		return fmt.Errorf("invalid target repo")
	}
	dst := RepoPath(targetOwner, targetName)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	var env []string
	if strings.TrimSpace(privateKey) != "" {
		keyPath, err := writeTempImportKey(privateKey)
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(keyPath) }()
		env = append(env,
			"GIT_SSH_COMMAND=ssh -i '"+strings.ReplaceAll(keyPath, "'", "'\\''")+"' -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new",
		)
	}
	if _, err := gitOutEnv(env, "", "clone", "--mirror", "--quiet", url, dst); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	_, _ = gitOut(dst, "config", "receive.denyDeleteCurrent", "ignore")
	return installPostReceiveHook(dst, targetOwner, targetName)
}

// PushMirror 把仓库的全部 refs 推送到远程镜像目标（同步到 GitHub/GitLab 等）。
func PushMirror(owner, name, url, privateKey string) error {
	if !ValidName(owner) || !ValidName(name) {
		return fmt.Errorf("invalid repo")
	}
	path := RepoPath(owner, name)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return fmt.Errorf("repo %s/%s not on disk", owner, name)
	}
	var env []string
	if strings.TrimSpace(privateKey) != "" {
		keyPath, err := writeTempImportKey(privateKey)
		if err != nil {
			return err
		}
		defer func() { _ = os.Remove(keyPath) }()
		env = append(env,
			"GIT_SSH_COMMAND=ssh -i '"+strings.ReplaceAll(keyPath, "'", "'\\''")+"' -o IdentitiesOnly=yes -o StrictHostKeyChecking=accept-new",
		)
	}
	_, err := gitOutEnv(env, path, "push", "--mirror", url)
	return err
}

// writeTempImportKey 把导入私钥写入临时文件（0600），调用方负责删除。
func writeTempImportKey(privateKey string) (string, error) {
	f, err := os.CreateTemp("", "gitdash-import-key-*")
	if err != nil {
		return "", err
	}
	path := f.Name()
	if _, err := f.WriteString(strings.TrimSpace(privateKey) + "\n"); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// RepoSize 返回仓库磁盘占用（字节）：松散对象大小 + pack 大小（不含 refs/config 等零头）。
func RepoSize(owner, name string) (int64, error) {
	out, err := gitOut(RepoPath(owner, name), "count-objects", "-v")
	if err != nil {
		return 0, err
	}
	var kb int64
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "size", "size-pack", "size-garbage":
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				kb += n
			}
		}
	}
	return kb * 1024, nil
}
