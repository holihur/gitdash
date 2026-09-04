package gitsvc

import (
	"bytes"
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
	nameRe   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	refRe    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

func Init(dataDir string) error {
	reposDir = filepath.Join(dataDir, "repos")
	return os.MkdirAll(reposDir, 0o755)
}

func ReposDir() string { return reposDir }

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

func RepoPath(name string) string { return filepath.Join(reposDir, name+".git") }

func Exists(name string) bool {
	fi, err := os.Stat(RepoPath(name))
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

func CreateBare(name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid repo name %q", name)
	}
	path := RepoPath(name)
	if _, err := gitOut("", "init", "--bare", "--initial-branch=main", path); err != nil {
		// older git without --initial-branch
		if _, err2 := gitOut("", "init", "--bare", path); err2 != nil {
			return err
		}
	}
	// allow deleting the default branch in a bare repo
	_, _ = gitOut(path, "config", "receive.denyDeleteCurrent", "ignore")
	return nil
}

func Delete(name string) error {
	return os.RemoveAll(RepoPath(name))
}

type Branch struct {
	Name   string `json:"name"`
	IsHead bool   `json:"is_head"`
}

func HeadBranch(name string) (string, error) {
	out, err := gitOut(RepoPath(name), "symbolic-ref", "--short", "HEAD")
	return strings.TrimSpace(out), err
}

func Branches(name string) ([]Branch, error) {
	out, err := gitOut(RepoPath(name), "for-each-ref", "--format=%(refname:short)", "refs/heads")
	if err != nil {
		return nil, err
	}
	head, _ := HeadBranch(name)
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
	Name string `json:"name"`
	Type string `json:"type"` // blob | tree
	Mode string `json:"mode"`
	Size int64  `json:"size"`
	SHA  string `json:"sha"`
}

func Tree(name, ref, dir string) ([]Entry, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	path := RepoPath(name)
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
	return entries, nil
}

const MaxBlobSize = 512 * 1024

type Blob struct {
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Encoding string `json:"encoding"` // utf-8 | binary | truncated
	Content  string `json:"content"`
}

func ReadBlob(name, ref, file string) (*Blob, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	file, err := CleanPath(file)
	if err != nil || file == "" {
		return nil, fmt.Errorf("invalid file path")
	}
	path := RepoPath(name)
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
		return nil, fmt.Errorf("read blob: %v", err)
	}
	data := stdout.Bytes()
	if bytes.IndexByte(data, 0) >= 0 {
		b.Encoding = "binary"
		return b, nil
	}
	b.Content = string(data)
	return b, nil
}

type Commit struct {
	SHA     string `json:"sha"`
	Author  string `json:"author"`
	Date    string `json:"date"`
	Message string `json:"message"`
}

func Commits(name, ref string, limit int) ([]Commit, error) {
	if !ValidRef(ref) {
		return nil, fmt.Errorf("invalid ref %q", ref)
	}
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	out, err := gitOut(RepoPath(name),
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
