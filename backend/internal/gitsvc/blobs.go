package gitsvc

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// TreeFile ref 上的一个普通文件条目（路径 + blob 大小 + blob SHA）。
// SHA 供增量索引比对文件是否变化。符号链接与子树不计入。
type TreeFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
	SHA  string `json:"sha,omitempty"`
}

// listTreeFiles 列出 ref 上全部 blob（递归），按路径升序。仅读取 `git ls-tree`
// 元信息，不读取文件内容，因此对超大仓库也足够轻量。
func listTreeFiles(owner, name, ref string) ([]TreeFile, error) {
	if !ValidName(owner) || !ValidName(name) {
		return nil, fmt.Errorf("invalid repo %s/%s", owner, name)
	}
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	out, err := gitOut(repoPath(owner, name), "ls-tree", "-r", "-l", "-z", ref)
	if err != nil {
		return nil, err
	}
	files := make([]TreeFile, 0, 256)
	for _, rec := range strings.Split(out, "\x00") {
		if rec == "" {
			continue
		}
		meta, file, ok := strings.Cut(rec, "\t")
		if !ok {
			continue
		}
		fields := strings.Fields(meta)
		if len(fields) < 4 || fields[1] != "blob" {
			continue
		}
		if fields[0] == "120000" { // 符号链接指向的路径不计入
			continue
		}
		size, perr := strconv.ParseInt(fields[3], 10, 64)
		if perr != nil {
			continue
		}
		files = append(files, TreeFile{Path: file, Size: size, SHA: fields[2]})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// readBlobsBatch 用一个 `git cat-file --batch` 进程批量读取 `<ref>:<path>` 的内容。
//
// 输入 paths 的顺序与 git 输出一一对应；不存在的对象（`<obj> missing`）被跳过，
// 非 blob 或超过 maxSize 的对象也会被消费输出以保证流不错位。返回 path → 内容。
// 调用方通常先用 ListTreeFiles 过滤掉超大文件，避免把巨量数据读进内存。
func readBlobsBatch(owner, name, ref string, paths []string, maxSize int64) (map[string][]byte, error) {
	if !ValidName(owner) || !ValidName(name) || !ValidRef(ref) {
		return nil, fmt.Errorf("invalid repo/ref")
	}
	out := make(map[string][]byte, len(paths))
	// 过滤含换行/回车的路径：`cat-file --batch` 以换行分隔输入，这类路径会破坏协议。
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		if p != "" && !strings.ContainsAny(p, "\n\r") {
			clean = append(clean, p)
		}
	}
	paths = clean
	if len(paths) == 0 {
		return out, nil
	}
	cmd := exec.Command("git", "-C", repoPath(owner, name), "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	// 写入端与读取端并发，避免大输入下管道互相阻塞死锁。
	writeDone := make(chan struct{})
	go func() {
		defer close(writeDone)
		w := bufio.NewWriter(stdin)
		for _, p := range paths {
			if _, werr := fmt.Fprintf(w, "%s:%s\n", ref, p); werr != nil {
				break
			}
		}
		_ = w.Flush()
		_ = stdin.Close()
	}()

	r := bufio.NewReaderSize(stdout, 1<<16)
	ok := true
	for i := range paths {
		header, herr := r.ReadString('\n')
		if herr != nil {
			ok = false
			break
		}
		line := strings.TrimRight(header, "\n")
		if strings.HasSuffix(line, " missing") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		size, perr := strconv.ParseInt(fields[2], 10, 64)
		if perr != nil || size < 0 {
			ok = false
			break
		}
		if fields[1] != "blob" || size > maxSize {
			if _, derr := io.CopyN(io.Discard, r, size+1); derr != nil {
				ok = false
				break
			}
			continue
		}
		buf := make([]byte, size)
		if _, rerr := io.ReadFull(r, buf); rerr != nil {
			ok = false
			break
		}
		if _, derr := r.Discard(1); derr != nil { // 结尾换行
			ok = false
			break
		}
		out[paths[i]] = buf
	}
	if !ok && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	<-writeDone
	if waitErr != nil {
		return out, gitErr(repoPath(owner, name), cmd.Args, stderr.String(), waitErr)
	}
	if !ok {
		return out, fmt.Errorf("git cat-file --batch: unexpected output")
	}
	return out, nil
}
