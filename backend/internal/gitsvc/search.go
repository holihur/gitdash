package gitsvc

import (
	"bytes"
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
		head, err := HeadBranch(owner, name)
		if err != nil || head == "" {
			return []SearchHit{}, nil // 空仓库：无可搜索内容
		}
		ref = head
	}
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	cmd := exec.Command("git", "-C", RepoPath(owner, name),
		"grep", "-n", "-I", "--fixed-strings",
		"-e", query, ref, "--")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok && ee.ExitCode() == 1 {
			return []SearchHit{}, nil // git grep 无命中
		}
		return nil, gitErr(RepoPath(owner, name), cmd.Args, "", err)
	}
	return parseGrepOut(stdout.String(), ref, max), nil
}

// parseGrepOut 解析 `<ref>:<path>:<line>:<text>` 行（text 截断 500 字符）。
func parseGrepOut(out, ref string, max int) []SearchHit {
	prefix := ref + ":"
	hits := []SearchHit{}
	for _, ln := range strings.Split(out, "\n") {
		ln = strings.TrimPrefix(ln, prefix)
		path, rest, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		numStr, text, ok := strings.Cut(rest, ":")
		if !ok {
			continue
		}
		line, err := strconv.Atoi(numStr)
		if err != nil || line < 1 {
			continue
		}
		text = strings.TrimSuffix(text, "\r")
		if len(text) > 500 {
			text = text[:500]
		}
		hits = append(hits, SearchHit{Path: path, Line: line, Text: text})
		if len(hits) >= max {
			break
		}
	}
	return hits
}
