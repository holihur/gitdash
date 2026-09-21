package gitsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func createBare(owner, name string) error {
	if !ValidName(owner) || !ValidName(name) {
		return fmt.Errorf("invalid repo %s/%s", owner, name)
	}
	path := repoPath(owner, name)
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
	applyReceiveLimits(path)
	return installPostReceiveHook(path, owner, name)
}

// applyReceiveLimits 给仓库设置单次 push 的输入上限（receive.maxInputSize），
// 阻止用超大 packfile 写满磁盘。GITDASH_MAX_PUSH_BYTES 可覆盖，0 = 不限。
func applyReceiveLimits(path string) {
	_, _ = gitOut(path, "config", "receive.maxInputSize", strconv.FormatInt(maxPushBytes(), 10))
}

// MaxPushBytes 返回单次 push 输入上限（字节），0 表示不限。
// SSH 网关通过 `git -c receive.maxInputSize=` 传入，覆盖历史的未配置仓库。
func MaxPushBytes() int64 { return maxPushBytes() }

// maxPushBytes 单次 push 输入上限（GITDASH_MAX_PUSH_BYTES，默认 5GiB，0 = 不限）。
func maxPushBytes() int64 {
	return envBytes("GITDASH_MAX_PUSH_BYTES", int64(5)<<30)
}

// maxRepoBytes 导入仓库的体积硬上限（GITDASH_MAX_REPO_BYTES，默认 5GiB，0 = 不限）。
func maxRepoBytes() int64 {
	return envBytes("GITDASH_MAX_REPO_BYTES", int64(5)<<30)
}

func envBytes(key string, def int64) int64 {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n >= 0 {
			return n
		}
	}
	return def
}

// repoSizeBytes 用 git count-objects 估算仓库占用（loose + pack），单位字节。
func repoSizeBytes(dir string) int64 {
	out, err := gitOut(dir, "count-objects", "-v")
	if err != nil {
		return 0
	}
	var total int64
	for _, ln := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "size", "size-pack": // 值单位为 KiB
			if n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err == nil {
				total += n * 1024
			}
		}
	}
	return total
}

// installPostReceiveHook 安装 post-receive hook：把每次 push 事件写成一行 JSON
// 追加到 spool 目录下的独立文件，供服务端 webhook 调度器投递。
func installPostReceiveHook(repoPath, owner, name string) error {
	hooksDir := filepath.Join(repoPath, "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	// 只把 `oldrev newrev refname` 转交给 `gitdash post-receive` 子命令，
	// 由 Go 侧生成 JSON（避免 shell 拼接被恶意 refname 注入）。
	script := `#!/bin/sh
# gitdash: record push events via the gitdash binary (safe JSON encoding)
while read oldrev newrev refname; do
	[ -z "$refname" ] && continue
	echo "$oldrev $newrev $refname"
done | @BIN@ post-receive "@OWNER@" "@REPO@"
`
	script = strings.NewReplacer(
		"@BIN@", selfBin(),
		"@OWNER@", owner,
		"@REPO@", name,
	).Replace(script)
	hook := filepath.Join(hooksDir, "post-receive")
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		return err
	}
	return installPreReceiveHook(hooksDir, owner, name)
}

// selfBin 返回当前可执行文件路径（供 hook 回调 `gitdash <subcommand>`）。
func selfBin() string {
	self, err := os.Executable()
	if err != nil || self == "" {
		return "gitdash" // 退化为 PATH 查找
	}
	return self
}

// installPreReceiveHook 安装 pre-receive hook：分支保护（禁删/禁强推）在 push 落盘前校验。
// 校验逻辑在 gitdash 主程序（gitdash pre-receive 子命令）里查库执行；二进制缺失时放行，避免阻断 push。
func installPreReceiveHook(hooksDir, owner, name string) error {
	script := `#!/bin/sh
# gitdash: branch protection (deletion / force-push) enforced before update
while read oldrev newrev refname; do
	[ -z "$refname" ] && continue
	echo "$oldrev $newrev $refname"
done | @BIN@ pre-receive "@OWNER@" "@REPO@"
`
	script = strings.NewReplacer(
		"@BIN@", selfBin(),
		"@OWNER@", owner,
		"@REPO@", name,
	).Replace(script)
	return os.WriteFile(filepath.Join(hooksDir, "pre-receive"), []byte(script), 0o755)
}

// EnsureHooks 遍历所有仓库重装 hooks（启动时调用，让存量仓库获得 pre-receive 分支保护）。
func ensureHooks() error {
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

func deleteRepo(owner, name string) error {
	invalidateRefs(owner, name)
	return os.RemoveAll(repoPath(owner, name))
}

// ForkRepo 把源仓库（bare）镜像复制到目标路径，用于 fork：保留全部分支/标签。
func forkRepo(sourceOwner, sourceName, targetOwner, targetName string) error {
	if !ValidName(sourceOwner) || !ValidName(sourceName) || !ValidName(targetOwner) || !ValidName(targetName) {
		return fmt.Errorf("invalid fork repo")
	}
	src := repoPath(sourceOwner, sourceName)
	if fi, err := os.Stat(src); err != nil || !fi.IsDir() {
		return fmt.Errorf("source repo %s/%s not on disk", sourceOwner, sourceName)
	}
	dst := repoPath(targetOwner, targetName)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if _, err := gitOut("", "clone", "--mirror", "--quiet", src, dst); err != nil {
		return err
	}
	// mirror clone 不继承源 hooks；重新安装 post-receive 并放开默认分支删除限制
	_, _ = gitOut(dst, "config", "receive.denyDeleteCurrent", "ignore")
	applyReceiveLimits(dst)
	return installPostReceiveHook(dst, targetOwner, targetName)
}

// ImportRepo 从远程 URL 镜像导入仓库到目标路径（保留全部分支/标签）。
// privateKey 非空时用于 SSH 认证（专用导入 key，如 GitHub/GitLab 的只读 deploy key）。
// credential 非空时（"user:token"）通过 GIT_ASKPASS 提供 HTTPS Basic 认证，
// 用于 OAuth 绑定的账号导入私有仓库，避免令牌出现在命令行参数中。
func importRepo(url, targetOwner, targetName, privateKey, credential string) error {
	if !ValidName(targetOwner) || !ValidName(targetName) {
		return fmt.Errorf("invalid target repo")
	}
	// 执行点复查：注册时校验过的域名可能已重绑定到内网（TOCTOU），
	// 且 scp-like 地址曾完全绕过主机黑名单。
	if RemoteURLBlocked(url) {
		return fmt.Errorf("blocked host")
	}
	dst := repoPath(targetOwner, targetName)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	env, cleanup, err := sshEnv(url, privateKey)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	cenv, ccleanup, err := credEnv(credential)
	if err != nil {
		return err
	}
	if ccleanup != nil {
		defer ccleanup()
	}
	env = append(env, cenv...)
	if _, err := gitOutEnv(env, "", "clone", "--mirror", "--quiet", url, dst); err != nil {
		_ = os.RemoveAll(dst)
		return err
	}
	if max := maxRepoBytes(); max > 0 {
		if size := repoSizeBytes(dst); size > max {
			_ = os.RemoveAll(dst)
			return fmt.Errorf("repository exceeds size limit")
		}
	}
	_, _ = gitOut(dst, "config", "receive.denyDeleteCurrent", "ignore")
	applyReceiveLimits(dst)
	return installPostReceiveHook(dst, targetOwner, targetName)
}

// PushMirror 把仓库的全部 refs 推送到远程镜像目标（同步到 GitHub/GitLab 等）。
func pushMirror(owner, name, url, privateKey string) error {
	if !ValidName(owner) || !ValidName(name) {
		return fmt.Errorf("invalid repo")
	}
	// 执行点复查（同 importRepo，防 DNS 重绑定与 scp-like 绕过）。
	if RemoteURLBlocked(url) {
		return fmt.Errorf("blocked host")
	}
	path := repoPath(owner, name)
	if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
		return fmt.Errorf("repo %s/%s not on disk", owner, name)
	}
	env, cleanup, err := sshEnv(url, privateKey)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}
	_, err = gitOutEnv(env, path, "push", "--mirror", url)
	return err
}

// sshEnv 构造 git 通过 SSH 访问远端所需的环境变量。
// 仅对 SSH 远端（git@host:path 或 ssh://）注入：默认 accept-new（首次连接自动
// 接受 host key，避免公网导入卡交互）。设置 GITDASH_SSH_KNOWN_HOSTS 指向
// known_hosts 文件后改为 StrictHostKeyChecking=yes，杜绝首次导入 MITM。
// https:// 与 git:// 远端无需该环境变量，保持原样以免干扰。
// privateKey 非空时额外写入临时 key 文件（0600）并加 -i/-o IdentitiesOnly。
// 返回的 cleanup 用于清理临时 key 文件（无 key 时为 nil）。
func sshEnv(url, privateKey string) ([]string, func(), error) {
	hasKey := strings.TrimSpace(privateKey) != ""
	if !hasKey && !isSSHURL(url) {
		return nil, nil, nil
	}
	cmd := sshBaseOptions()
	if !hasKey {
		return []string{"GIT_SSH_COMMAND=" + cmd}, nil, nil
	}
	keyPath, err := writeTempImportKey(privateKey)
	if err != nil {
		return nil, nil, err
	}
	cmd += " -i '" + strings.ReplaceAll(keyPath, "'", "'\\''") + "' -o IdentitiesOnly=yes"
	return []string{"GIT_SSH_COMMAND=" + cmd}, func() { _ = os.Remove(keyPath) }, nil
}

// credEnv 为 HTTPS 远端构造 GIT_ASKPASS 环境变量，从环境变量读取用户名/口令，
// 避免把令牌写入命令行参数或 URL（后者会出现在 ps / 错误日志里）。
// credential 形如 "user:token"；无冒号时视为纯 token（用户名留空）。
func credEnv(credential string) ([]string, func(), error) {
	credential = strings.TrimSpace(credential)
	if credential == "" {
		return nil, nil, nil
	}
	user, pass, ok := strings.Cut(credential, ":")
	if !ok {
		user, pass = "", credential
	}
	dir, err := os.MkdirTemp("", "gitdash-askpass-*")
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, "askpass.sh")
	script := "#!/bin/sh\ncase \"$1\" in\n*[Uu]sername*) printf '%s' \"$GITDASH_GIT_USER\" ;;\n*) printf '%s' \"$GITDASH_GIT_PASS\" ;;\nesac\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		_ = os.RemoveAll(dir)
		return nil, nil, err
	}
	return []string{
		"GIT_ASKPASS=" + path,
		"GIT_TERMINAL_PROMPT=0",
		"GITDASH_GIT_USER=" + user,
		"GITDASH_GIT_PASS=" + pass,
	}, func() { _ = os.RemoveAll(dir) }, nil
}

// sshBaseOptions 返回 git 远端 SSH 的 -o 选项。设置 GITDASH_SSH_KNOWN_HOSTS
// 时启用严格 host key 校验（防首次导入 MITM）；未设置时退化为 accept-new
// （首次连接自动接受，避免公网导入卡交互）。
func sshBaseOptions() string {
	if kh := strings.TrimSpace(os.Getenv("GITDASH_SSH_KNOWN_HOSTS")); kh != "" {
		return "ssh -o StrictHostKeyChecking=yes -o UserKnownHostsFile='" + strings.ReplaceAll(kh, "'", "'\\''") + "'"
	}
	return "ssh -o StrictHostKeyChecking=accept-new"
}

// isSSHURL 判断远端地址是否走 SSH：ssh:// 前缀，或 scp 风格 user@host:path。
func isSSHURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "ssh://") {
		return true
	}
	if strings.Contains(raw, "://") { // http(s):// / git:// 等其它 scheme
		return false
	}
	at := strings.IndexByte(raw, '@')
	colon := strings.IndexByte(raw, ':')
	return at > 0 && colon > at
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
func repoSize(owner, name string) (int64, error) {
	out, err := gitOut(repoPath(owner, name), "count-objects", "-v")
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
