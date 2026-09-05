package gitsvc

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
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
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}

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
	return nil
}

func Delete(owner, name string) error {
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

type Branch struct {
	Name   string `json:"name"`
	IsHead bool   `json:"is_head"`
}

func HeadBranch(owner, name string) (string, error) {
	out, err := gitOut(RepoPath(owner, name), "symbolic-ref", "--short", "HEAD")
	return strings.TrimSpace(out), err
}

func Branches(owner, name string) ([]Branch, error) {
	out, err := gitOut(RepoPath(owner, name), "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	head, _ := HeadBranch(owner, name)
	branches := []Branch{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		branches = append(branches, Branch{Name: line, IsHead: line == head})
	}
	return branches, nil
}

type Entry struct {
	Name        string `json:"name"`
	Type        string `json:"type"` // blob | tree
	Mode        string `json:"mode"`
	Size        int64  `json:"size"`
	SHA         string `json:"sha"`
	ModifiedAt  string `json:"modified_at,omitempty"`
	ModifiedBy  string `json:"modified_by,omitempty"`
	ModifiedMsg string `json:"modified_msg,omitempty"`
	LastCommit  string `json:"last_commit,omitempty"`
}

func Tree(owner, name, ref, dir string) ([]Entry, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	path := RepoPath(owner, name)
	treeish := ref
	if dir != "" {
		t, err := gitOut(path, "cat-file", "-t", ref+":"+dir)
		if err != nil {
			return nil, fmt.Errorf("path not found: %s", dir)
		}
		if strings.TrimSpace(t) != "tree" {
			return nil, fmt.Errorf("not a directory: %s", dir)
		}
		treeish = ref + ":" + dir
	}
	out, err := gitOut(path, "ls-tree", "-l", "-z", treeish)
	if err != nil {
		return nil, err
	}
	entries := []Entry{}
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		meta, file, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 4 {
			continue
		}
		size, _ := strconv.ParseInt(fields[3], 10, 64)
		entries = append(entries, Entry{Name: file, Type: fields[1], Mode: fields[0], Size: size, SHA: fields[2]})
	}
	// 每个条目的最后变更提交：sha/时间/作者/说明（一次 git log 每项）
	for i := range entries {
		p := entries[i].Name
		if dir != "" {
			p = dir + "/" + p
		}
		out, err := gitOut(path, "log", "-1", "--pretty=format:%H%x1f%cI%x1f%an%x1f%s", ref, "--", p)
		if err != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(out), "\x1f", 4)
		if len(parts) >= 1 && parts[0] != "" {
			entries[i].LastCommit = parts[0]
		}
		if len(parts) >= 2 {
			entries[i].ModifiedAt = parts[1]
		}
		if len(parts) >= 3 {
			entries[i].ModifiedBy = parts[2]
		}
		if len(parts) >= 4 {
			entries[i].ModifiedMsg = parts[3]
		}
	}
	return entries, nil
}

const MaxBlobSize = 512 * 1024

type Blob struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Encoding string `json:"encoding"` // utf-8 | binary | truncated
	Content  string `json:"content"`
}

func ReadBlob(owner, name, ref, file string) (*Blob, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	file, err := CleanPath(file)
	if err != nil || file == "" {
		return nil, fmt.Errorf("invalid file path")
	}
	path := RepoPath(owner, name)
	obj := ref + ":" + file

	sizeOut, err := gitOut(path, "cat-file", "-s", obj)
	if err != nil {
		return nil, fmt.Errorf("file not found: %s", file)
	}
	size, _ := strconv.ParseInt(strings.TrimSpace(sizeOut), 10, 64)
	b := &Blob{Path: file, Size: size, Encoding: "utf-8"}

	if size > MaxBlobSize {
		b.Encoding = "truncated"
		return b, nil
	}
	cmd := exec.Command("git", "-C", path, "cat-file", "blob", obj)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}
	data := stdout.Bytes()
	if bytes.IndexByte(data, 0) >= 0 {
		b.Encoding = "binary"
		return b, nil
	}
	b.Content = string(data)
	return b, nil
}

// RevSHA 解析分支/提交引用为完整 SHA（仅接受仓库内已存在的提交）。
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
func RawCommit(owner, name, sha string) ([]byte, error) {
	path := RepoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "cat-file", "commit", sha)
	return cmd.Output()
}

type Commit struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

func Commits(owner, name, ref string, limit int) ([]Commit, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	out, err := gitOut(RepoPath(owner, name),
		"log", "--max-count="+strconv.Itoa(limit), "--date=iso-strict",
		"--pretty=format:%H%x1f%an%x1f%ad%x1f%s%x1e", ref)
	if err != nil {
		return nil, err
	}
	commits := []Commit{}
	for _, rec := range strings.Split(strings.TrimSpace(out), "\x1e") {
		rec = strings.TrimPrefix(rec, "\n")
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x1f")
		if len(parts) < 4 {
			continue
		}
		commits = append(commits, Commit{SHA: parts[0], Author: parts[1], Date: parts[2], Message: parts[3]})
	}
	return commits, nil
}

// CommitDiff 返回某提交相对第一父提交（根提交相对空树）的变更。
func CommitDiff(owner, name, sha string) ([]DiffFile, string, error) {
	path := RepoPath(owner, name)
	parent := ""
	if out, err := gitOut(path, "rev-parse", "--verify", "--quiet", sha+"^1"); err == nil {
		parent = strings.TrimSpace(out)
	}
	if parent != "" {
		files, err := DiffStats(owner, name, parent, sha)
		if err != nil {
			return nil, "", err
		}
		patch, err := DiffPatch(owner, name, parent, sha)
		return files, patch, err
	}
	// 根提交：相对空树
	numstat, err := gitOut(path, "show", "--numstat", "--format=", sha)
	if err != nil {
		return nil, "", err
	}
	files := []DiffFile{}
	for _, ln := range strings.Split(strings.TrimSpace(numstat), "\n") {
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		ins, _ := strconv.Atoi(parts[0])
		del, _ := strconv.Atoi(parts[1])
		files = append(files, DiffFile{Path: parts[2], Status: "A", Insertions: ins, Deletions: del})
	}
	patch, err := gitOut(path, "show", "-U3", "--format=", sha)
	return files, patch, err
}

// MergeNonFF 在临时工作区把 source 并入 target（method: merge | squash），
// 成功后更新目标分支并返回新的目标 tip SHA。冲突时返回错误。
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
		if _, err := git("merge", "--squash", "-q", "origin/"+source); err != nil {
			return "", fmt.Errorf("merge conflict or error: %w", err)
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

// InitReadme 把刚创建的 bare 仓库初始化为默认模版：main 分支 + 以仓库名生成的 README.md。
func InitReadme(owner, name string) error {
	bare := RepoPath(owner, name)
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
type Tag struct {
	Name    string `json:"name"`
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

// Tags 列出标签（附注标签取被指提交）。
func Tags(owner, name string) ([]Tag, error) {
	out, err := gitOut(RepoPath(owner, name),
		"for-each-ref", "--format=%(refname:short)%1f%(objectname)%1f%(*objectname)%1f%(*subject)",
		"refs/tags")
	if err != nil {
		return nil, err
	}
	tags := []Tag{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\x1f")
		if len(parts) < 2 {
			continue
		}
		sha := parts[1]
		msg := ""
		if len(parts) > 3 && parts[3] != "" {
			msg = parts[3]
			if len(parts) > 2 && parts[2] != "" {
				sha = parts[2] // 附注标签指向提交
			}
		}
		tags = append(tags, Tag{Name: parts[0], SHA: sha, Message: msg})
	}
	return tags, nil
}

// CreateRef 创建分支或标签（lightweight），from 可为任意可解析 rev。
func CreateRef(owner, name, kind, refName, from string) (string, error) {
	full := "refs/heads/" + refName
	if kind == "tag" {
		full = "refs/tags/" + refName
	}
	if err := checkRefFormat(full); err != nil {
		return "", err
	}
	sha, err := RevSHA(owner, name, from)
	if err != nil {
		return "", fmt.Errorf("cannot resolve %q to a commit", from)
	}
	repo := RepoPath(owner, name)
	if _, err := gitOut(repo, "rev-parse", "-q", "--verify", full); err == nil {
		return "", ErrRefExists
	}
	if _, err := gitOut(repo, "update-ref", full, sha); err != nil {
		return "", err
	}
	return sha, nil
}

// DeleteRef 删除分支或标签；默认分支(HEAD)不可删除。
func DeleteRef(owner, name, kind, refName string) error {
	full := "refs/heads/" + refName
	if kind == "tag" {
		full = "refs/tags/" + refName
	} else if kind != "branch" {
		return fmt.Errorf("invalid ref kind %q", kind)
	}
	if err := checkRefFormat(full); err != nil {
		return err
	}
	repo := RepoPath(owner, name)
	if _, err := gitOut(repo, "rev-parse", "-q", "--verify", full); err != nil {
		return ErrRefNotFound
	}
	if kind == "branch" {
		if head, err := HeadBranch(owner, name); err == nil && head == refName {
			return ErrHeadBranch
		}
	}
	_, err := gitOut(repo, "update-ref", "-d", full)
	return err
}

var (
	ErrRefExists   = errors.New("ref already exists")
	ErrRefNotFound = errors.New("ref not found")
	ErrHeadBranch  = errors.New("cannot delete the default (HEAD) branch")
)

func checkRefFormat(full string) error {
	if _, err := gitOut("", "check-ref-format", full); err != nil {
		return fmt.Errorf("invalid ref name %q", strings.TrimPrefix(full, "refs/heads/"))
	}
	return nil
}
