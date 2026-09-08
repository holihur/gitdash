package gitsvc

import (
	"fmt"
	"strconv"
	"strings"
)

// ---- blame ----

type BlameCommit struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

type BlameLine struct {
	Line    int    `json:"line"`
	Commit  string `json:"commit"`
	Content string `json:"content"`
}

type Blame struct {
	Path    string                 `json:"path"`
	Commits map[string]BlameCommit `json:"commits"`
	Lines   []BlameLine            `json:"lines"`
}

// Blame 基于 git blame --porcelain 返回文件的逐行归属。
func BlameFile(owner, name, ref, file string) (*Blame, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	file, err := CleanPath(file)
	if err != nil || file == "" {
		return nil, fmt.Errorf("invalid file path")
	}
	out, err := gitOut(RepoPath(owner, name), "blame", "--porcelain", ref, "--", file)
	if err != nil {
		return nil, fmt.Errorf("blame failed: %w", err)
	}
	b := &Blame{Path: file, Commits: map[string]BlameCommit{}, Lines: []BlameLine{}}
	lines := strings.Split(out, "\n")
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		if ln == "" {
			continue
		}
		if strings.HasPrefix(ln, "\t") {
			continue
		}
		// 记录行：<sha> <origline> <finalline> [<numlines>]
		parts := strings.Fields(ln)
		if len(parts) < 3 {
			continue
		}
		sha := parts[0]
		finalLine, _ := strconv.Atoi(parts[2])
		// 若是首次出现，后续若干行为该提交的元数据头
		if _, seen := b.Commits[sha]; !seen {
			c := BlameCommit{SHA: sha}
			i++
			for ; i < len(lines); i++ {
				h := lines[i]
				if h == "" || strings.HasPrefix(h, "\t") {
					break
				}
				if key, val, ok := strings.Cut(h, " "); ok {
					switch key {
					case "author":
						c.Author = val
					case "author-time":
						c.Date = val
					case "summary":
						c.Message = val
					}
				}
			}
			b.Commits[sha] = c
		}
		// 内容行：<TAB>content（首次出现时 header 循环已停在内容行，否则在记录行的下一行）
		idx := i
		if !strings.HasPrefix(lines[idx], "\t") && idx+1 < len(lines) && strings.HasPrefix(lines[idx+1], "\t") {
			idx++
			i++
		}
		if strings.HasPrefix(lines[idx], "\t") {
			b.Lines = append(b.Lines, BlameLine{Line: finalLine, Commit: sha, Content: strings.TrimPrefix(lines[idx], "\t")})
		}
	}
	return b, nil
}

// RevSHA 解析分支/提交引用为完整 SHA（仅接受仓库内已存在的提交）。
