package gitsvc

import (
	"bytes"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

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
	// 单次 git log 遍历最近 500 条提交，按 --name-only 汇总每个条目的最后变更，
	// 避免对每个条目各 spawn 一次 git 进程（原实现大目录会 spawn 上千次）。
	idx := make(map[string]int, len(entries))
	for i := range entries {
		idx[entries[i].Name] = i
	}
	if len(entries) > 0 {
		pathSpec := "."
		if dir != "" {
			pathSpec = dir
		}
		out, err := gitOut(path, "log", "-500", "--name-only", "-z",
			"--pretty=format:%x1e%H%x1f%cI%x1f%an%x1f%s", ref, "--", pathSpec)
		if err == nil {
			for _, rec := range strings.Split(out, "\x1e") {
				head, files, ok := strings.Cut(strings.TrimPrefix(rec, "\n"), "\n")
				if !ok {
					continue
				}
				fields := strings.SplitN(head, "\x1f", 4)
				if len(fields) < 4 {
					continue
				}
				for _, f := range strings.Split(files, "\x00") {
					f = strings.TrimSpace(f)
					if f == "" {
						continue
					}
					if dir != "" {
						if rest, ok := strings.CutPrefix(f, dir+"/"); ok {
							f = rest
						} else {
							continue
						}
					}
					if i, ok := idx[f]; ok && entries[i].LastCommit == "" {
						entries[i].LastCommit = fields[0]
						entries[i].ModifiedAt = fields[1]
						entries[i].ModifiedBy = fields[2]
						entries[i].ModifiedMsg = fields[3]
					}
				}
			}
		}
	}
	// --name-only 只列文件；目录条目回退到单次 git log（目录数量通常很少）
	for i := range entries {
		if entries[i].Type != "tree" || entries[i].LastCommit != "" {
			continue
		}
		p := entries[i].Name
		if dir != "" {
			p = dir + "/" + p
		}
		out, err := gitOut(path, "log", "-1", "--pretty=format:%H%x1f%cI%x1f%an%x1f%s", ref, "--", p)
		if err != nil {
			continue
		}
		parts := strings.SplitN(strings.TrimSpace(out), "\x1f", 4)
		if len(parts) >= 4 {
			entries[i].LastCommit, entries[i].ModifiedAt, entries[i].ModifiedBy, entries[i].ModifiedMsg = parts[0], parts[1], parts[2], parts[3]
		}
	}
	// 目录在前、文件在后，各自按名称（忽略大小写）排序。
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Type != entries[j].Type {
			return entries[i].Type == "tree"
		}
		a, b := strings.ToLower(entries[i].Name), strings.ToLower(entries[j].Name)
		if a != b {
			return a < b
		}
		return entries[i].Name < entries[j].Name
	})
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
