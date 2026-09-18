package gitsvc

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// SearchHit 一次代码搜索命中。
type SearchHit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// SearchOpts 代码搜索选项。
type SearchOpts struct {
	// Ref 分支/标签/commit；为空时使用默认分支。
	Ref string
	// Pathspec 额外的 git pathspec（如 "*.go"、"src/"）。
	Pathspec []string
	// Word 为 true 时按单词边界匹配（`git grep -w`），用于 symbol 搜索。
	Word bool
	// Max 返回条数上限（默认 50，最大 200）。
	Max int
}

// Search 在指定 ref（缺省默认分支）上做固定字符串全文搜索。
// 底层 `git grep -n -I --fixed-strings -e <query> <ref> --`：
// query 作为 -e 参数值传入，git 把它当 pattern 数据，无注入风险；
// --fixed-strings 避免正则语义；-I 跳过二进制文件。
// 无命中（退出码 1）返回空切片而非错误。
func Search(owner, name, query, ref string, max int) ([]SearchHit, error) {
	return SearchWith(context.Background(), owner, name, query, SearchOpts{Ref: ref, Max: max})
}

// SearchWith 与 Search 相同，但支持 pathspec / 单词匹配，并可通过 ctx 取消
// （用于全局搜索的总超时）。ctx 取消时子进程会被杀掉。
func searchWith(ctx context.Context, owner, name, query string, opts SearchOpts) ([]SearchHit, error) {
	if !ValidName(owner) || !ValidName(name) {
		return nil, fmt.Errorf("invalid repo %s/%s", owner, name)
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("empty query")
	}
	max := opts.Max
	if max <= 0 {
		max = 50
	}
	if max > 200 {
		max = 200
	}
	ref := opts.Ref
	if ref == "" {
		// 任何错误（含空仓库）都视为无可搜索内容
		head, _ := headBranch(owner, name)
		if head == "" {
			return []SearchHit{}, nil
		}
		ref = head
	}
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	args := []string{"-C", repoPath(owner, name), "grep", "-n", "-I", "--fixed-strings"}
	if opts.Word {
		args = append(args, "-w")
	}
	args = append(args, "-e", query, ref, "--")
	for _, p := range opts.Pathspec {
		if s := strings.TrimSpace(p); s != "" {
			args = append(args, s)
		}
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	// 流式读取，命中达到 max 立即停止，避免把整个 git grep 输出缓冲进内存。
	prefix := ref + ":"
	hits := []SearchHit{}
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		if hit, ok := parseGrepLine(sc.Text(), prefix); ok {
			hits = append(hits, hit)
			if len(hits) >= max {
				break
			}
		}
	}
	if len(hits) >= max {
		// 提前退出：终止 git 进程，避免它继续向已关闭的管道写入。
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
		return hits, nil
	}
	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return []SearchHit{}, nil // git grep 无命中
		}
		return nil, gitErr(repoPath(owner, name), cmd.Args, stderr.String(), err)
	}
	return hits, nil
}

// parseGrepLine 解析单行 `<ref>:<path>:<line>:<text>`（text 截断 500 字符）。
func parseGrepLine(ln, prefix string) (SearchHit, bool) {
	ln = strings.TrimPrefix(ln, prefix)
	path, rest, ok := strings.Cut(ln, ":")
	if !ok {
		return SearchHit{}, false
	}
	numStr, text, ok := strings.Cut(rest, ":")
	if !ok {
		return SearchHit{}, false
	}
	line, err := strconv.Atoi(numStr)
	if err != nil || line < 1 {
		return SearchHit{}, false
	}
	text = strings.TrimSuffix(text, "\r")
	if len(text) > 500 {
		text = text[:500]
	}
	return SearchHit{Path: path, Line: line, Text: text}, true
}
