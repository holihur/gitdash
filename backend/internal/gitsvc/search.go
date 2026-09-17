package gitsvc

import (
	"bufio"
	"bytes"
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

// Search 在指定 ref（缺省默认分支）上做固定字符串全文搜索。
// 底层 `git grep -n -I --fixed-strings -e <query> <ref> --`：
// query 作为 -e 参数值传入，git 把它当 pattern 数据，无注入风险；
// --fixed-strings 避免正则语义；-I 跳过二进制文件。
// 无命中（退出码 1）返回空切片而非错误。
func Search(owner, name, query, ref string, max int) ([]SearchHit, error) {
	if !ValidName(owner) || !ValidName(name) {
		return nil, fmt.Errorf("invalid repo %s/%s", owner, name)
	}
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("empty query")
	}
	if max <= 0 {
		max = 50
	}
	if max > 200 {
		max = 200
	}
	if ref == "" {
		// 任何错误（含空仓库）都视为无可搜索内容
		head, _ := HeadBranch(owner, name)
		if head == "" {
			return []SearchHit{}, nil
		}
		ref = head
	}
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	cmd := exec.Command("git", "-C", RepoPath(owner, name),
		"grep", "-n", "-I", "--fixed-strings",
		"-e", query, ref, "--")
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
		var ee *exec.ExitError
		if errors.As(err, &ee) && ee.ExitCode() == 1 {
			return []SearchHit{}, nil // git grep 无命中
		}
		return nil, gitErr(RepoPath(owner, name), cmd.Args, stderr.String(), err)
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

// parseGrepOut 保留给测试使用的整段解析入口。
func parseGrepOut(out, ref string, max int) []SearchHit {
	prefix := ref + ":"
	hits := []SearchHit{}
	for _, ln := range strings.Split(out, "\n") {
		if hit, ok := parseGrepLine(ln, prefix); ok {
			hits = append(hits, hit)
			if len(hits) >= max {
				break
			}
		}
	}
	return hits
}
