package gitsvc

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

type Branch struct {
	Name   string `json:"name"`
	IsHead bool   `json:"is_head"`
}

func HeadBranch(owner, name string) (string, error) {
	out, err := gitOut(RepoPath(owner, name), "symbolic-ref", "--short", "HEAD")
	return strings.TrimSpace(out), err
}

func Branches(owner, name string) ([]Branch, error) {
	if v, ok := refCacheGet("b", owner, name); ok {
		return v.([]Branch), nil
	}
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
	refCacheSet("b", owner, name, branches)
	return branches, nil
}

// ---- 引用（分支/标签）TTL 缓存：RepoView 每次加载都会拉 branches+tags，
// 各自 spawn 一次 for-each-ref；缓存 15s 并在 push 后失效。----

var (
	refCacheMu   sync.Mutex
	refCacheData = map[string]any{}
	refCacheAt   = map[string]time.Time{}
)

const refCacheTTL = 15 * time.Second

func refCacheGet(kind, owner, name string) (any, bool) {
	refCacheMu.Lock()
	defer refCacheMu.Unlock()
	key := kind + "|" + owner + "/" + name
	v, ok := refCacheData[key]
	if ok && time.Since(refCacheAt[key]) < refCacheTTL {
		return v, true
	}
	return nil, false
}

func refCacheSet(kind, owner, name string, v any) {
	refCacheMu.Lock()
	refCacheData[kind+"|"+owner+"/"+name] = v
	refCacheAt[kind+"|"+owner+"/"+name] = time.Now()
	refCacheMu.Unlock()
}

// InvalidateRefs 失效某仓库（或全部，owner 为空时）的分支/标签缓存；push 后调用。
func InvalidateRefs(owner, name string) {
	refCacheMu.Lock()
	defer refCacheMu.Unlock()
	if owner == "" {
		refCacheData = map[string]any{}
		refCacheAt = map[string]time.Time{}
		return
	}
	for _, kind := range []string{"b", "t"} {
		delete(refCacheData, kind+"|"+owner+"/"+name)
		delete(refCacheAt, kind+"|"+owner+"/"+name)
	}
}

type Tag struct {
	Name    string `json:"name"`
	SHA     string `json:"sha"`
	Message string `json:"message"`
}

// Tags 列出标签（附注标签取被指提交）。
func Tags(owner, name string) ([]Tag, error) {
	if v, ok := refCacheGet("t", owner, name); ok {
		return v.([]Tag), nil
	}
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
	refCacheSet("t", owner, name, tags)
	return tags, nil
}

// CreateRef 创建分支或标签（lightweight），from 可为任意可解析 rev。
func CreateRef(owner, name, kind, refName, from string) (string, error) {
	defer InvalidateRefs(owner, name)
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
	defer InvalidateRefs(owner, name)
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
