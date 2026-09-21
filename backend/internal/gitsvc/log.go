package gitsvc

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

func rawCommit(owner, name, sha string) ([]byte, error) {
	path := repoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "cat-file", "commit", sha)
	return cmd.Output()
}

// RawCommits 用单次 `git cat-file --batch` 进程读取多个 commit 对象。
// 返回 map[sha]raw；读取失败的 sha 不出现在结果中。
func rawCommits(owner, name string, shas []string) map[string][]byte {
	out := map[string][]byte{}
	if len(shas) == 0 {
		return out
	}
	path := repoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return out
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return out
	}
	if err := cmd.Start(); err != nil {
		return out
	}
	go func() {
		for _, sha := range shas {
			_, _ = io.WriteString(stdin, sha+"\n")
		}
		_ = stdin.Close()
	}()
	// 逐条读取：<sha> <type> <size>\n<content>\n
	r := bufio.NewReader(stdout)
	for range shas {
		line, err := r.ReadString('\n')
		if err != nil {
			break
		}
		fields := strings.Fields(line)
		if len(fields) != 3 {
			continue // missing object 等
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil || size < 0 {
			break
		}
		buf := make([]byte, size+1) // 内容 + 结尾换行
		if _, err := io.ReadFull(r, buf); err != nil {
			break
		}
		out[fields[0]] = buf[:size]
	}
	_ = cmd.Wait()
	return out
}

type Commit struct {
	SHA     string   `json:"sha"`
	Author  string   `json:"author"`
	Date    string   `json:"date"`
	Message string   `json:"message"`
	Parents []string `json:"parents,omitempty"` // 父提交 sha（用于提交图）；根提交为空
	Refs    []string `json:"refs,omitempty"`    // 指向该提交的本地分支/标签（如 "HEAD -> main"、"tag: v1.0"）
}

// maxCommitScan 带搜索词时最多向后扫描的提交数（避免超大仓库全量遍历）。
const maxCommitScan = 500

func commits(owner, name, ref string, limit, offset int, query string) ([]Commit, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(query)
	scan := limit
	args := []string{"log", "--date=iso-strict",
		"--pretty=format:%H%x1f%P%x1f%an%x1f%ad%x1f%D%x1f%s%x1e"}
	if q != "" {
		// 搜索需要向后多扫描，再在内存里过滤匹配项。
		scan = maxCommitScan
	} else {
		// 无搜索时用 --skip 直接跳过已加载的提交，避免重复传输。
		args = append(args, "--skip="+strconv.Itoa(offset))
	}
	args = append(args, "--max-count="+strconv.Itoa(scan), ref)
	out, err := gitOut(repoPath(owner, name), args...)
	if err != nil {
		return nil, err
	}
	commits := []Commit{}
	skipped := 0
	for _, rec := range strings.Split(strings.TrimSpace(out), "\x1e") {
		rec = strings.TrimPrefix(rec, "\n")
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x1f")
		if len(parts) < 6 {
			continue
		}
		c := Commit{
			SHA: parts[0], Author: parts[2], Date: parts[3], Message: parts[5],
			Parents: strings.Fields(parts[1]), Refs: splitRefDecorations(parts[4]),
		}
		if q != "" && !commitMatches(c, q) {
			continue
		}
		// 搜索路径下 offset 指“跳过前 N 个匹配项”。
		if q != "" && skipped < offset {
			skipped++
			continue
		}
		commits = append(commits, c)
		if len(commits) >= limit {
			break
		}
	}
	return commits, nil
}

// splitRefDecorations 拆分 `git log --pretty=%D` 的引用列表（逗号分隔）。
func splitRefDecorations(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	out := []string{}
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// commitMatches 大小写不敏感地匹配 sha 前缀 / 作者 / 提交信息。
func commitMatches(c Commit, query string) bool {
	q := strings.ToLower(query)
	return strings.HasPrefix(strings.ToLower(c.SHA), q) ||
		strings.Contains(strings.ToLower(c.Author), q) ||
		strings.Contains(strings.ToLower(c.Message), q)
}

// LastCommit 返回 ref 上最近一次改动 path（文件或目录；空 = 仓库根）的提交。
// 空仓库（无提交）返回 (nil, nil)。
func lastCommit(owner, name, ref, path string) (*Commit, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	args := []string{"log", "-1", "--date=iso-strict", "--pretty=format:%H%x1f%an%x1f%ad%x1f%s", ref}
	if path != "" {
		args = append(args, "--", path)
	}
	out, err := gitOut(repoPath(owner, name), args...)
	if err != nil {
		return nil, err
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return nil, nil
	}
	parts := strings.SplitN(out, "\x1f", 4)
	if len(parts) < 4 {
		return nil, nil
	}
	return &Commit{SHA: parts[0], Author: parts[1], Date: parts[2], Message: parts[3]}, nil
}

// CommitDiff 返回某提交相对第一父提交（根提交相对空树）的变更。
func commitDiff(owner, name, sha string) ([]DiffFile, string, error) {
	path := repoPath(owner, name)
	parent := ""
	if out, err := gitOut(path, "rev-parse", "--verify", "--quiet", sha+"^1"); err == nil {
		parent = strings.TrimSpace(out)
	}
	if parent != "" {
		files, err := diffStats(owner, name, parent, sha)
		if err != nil {
			return nil, "", err
		}
		patch, err := diffPatch(owner, name, parent, sha)
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
