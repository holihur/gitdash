package gitsvc

import (
	"bufio"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
)

func RawCommit(owner, name, sha string) ([]byte, error) {
	path := RepoPath(owner, name)
	cmd := exec.Command("git", "-C", path, "cat-file", "commit", sha)
	return cmd.Output()
}

// RawCommits 用单次 `git cat-file --batch` 进程读取多个 commit 对象。
// 返回 map[sha]raw；读取失败的 sha 不出现在结果中。
func RawCommits(owner, name string, shas []string) map[string][]byte {
	out := map[string][]byte{}
	if len(shas) == 0 {
		return out
	}
	path := RepoPath(owner, name)
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
