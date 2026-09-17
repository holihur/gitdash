package pipeline

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// cacheDir 缓存根目录（server 由 Init 设为 data/cache；runner 可用 SetCacheDir 覆盖）。
var cacheDir string

// SetCacheDir 覆盖缓存根目录。
func SetCacheDir(dir string) { cacheDir = dir }

// cacheRoot 返回缓存根目录：显式设置 > GITDASH_RUNNER_CACHE > 系统临时目录。
func cacheRoot() string {
	if cacheDir != "" {
		return cacheDir
	}
	if d := strings.TrimSpace(os.Getenv("GITDASH_RUNNER_CACHE")); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "gitdash-cache")
}

// cacheEntryDir 某个仓库 + key 对应的缓存目录。
func cacheEntryDir(job RunJob, key string) string {
	if key == "" {
		key = "default"
	}
	return filepath.Join(cacheRoot(), job.Owner, job.Repo, key)
}

// cacheRestore 把仓库/key 的缓存内容复制进工作区；无缓存时静默跳过。
func cacheRestore(cfg *Config, job RunJob, workdir string, logSink io.Writer) {
	if len(cfg.CachePaths) == 0 {
		return
	}
	base := cacheEntryDir(job, cfg.CacheKey)
	if _, err := os.Stat(base); err != nil {
		_, _ = fmt.Fprintf(logSink, "cache: no cache for key %q\n", cfg.CacheKey)
		return
	}
	restored := 0
	for _, p := range cfg.CachePaths {
		src := filepath.Join(base, filepath.FromSlash(p))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(workdir, filepath.FromSlash(p))
		if err := copyPath(src, dst); err != nil {
			_, _ = fmt.Fprintf(logSink, "!! cache: restore %s failed: %v\n", p, err)
			continue
		}
		restored++
	}
	_, _ = fmt.Fprintf(logSink, "cache: restored %d path(s) for key %q\n", restored, cfg.CacheKey)
}

// cacheSave 把工作区中配置的路径归档到缓存目录（原子替换）。
func cacheSave(cfg *Config, job RunJob, workdir string, logSink io.Writer) {
	if len(cfg.CachePaths) == 0 {
		return
	}
	base := cacheEntryDir(job, cfg.CacheKey)
	tmp := fmt.Sprintf("%s.tmp-%d", base, job.RunID)
	_ = os.RemoveAll(tmp)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		_, _ = fmt.Fprintf(logSink, "!! cache: mkdir failed: %v\n", err)
		return
	}
	saved := 0
	for _, p := range cfg.CachePaths {
		src := filepath.Join(workdir, filepath.FromSlash(p))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(tmp, filepath.FromSlash(p))
		if err := copyPath(src, dst); err != nil {
			_, _ = fmt.Fprintf(logSink, "!! cache: save %s failed: %v\n", p, err)
			_ = os.RemoveAll(tmp)
			return
		}
		saved++
	}
	if saved == 0 {
		_ = os.RemoveAll(tmp)
		_, _ = fmt.Fprintf(logSink, "cache: nothing to save for key %q\n", cfg.CacheKey)
		return
	}
	if err := os.MkdirAll(filepath.Dir(base), 0o755); err != nil {
		_ = os.RemoveAll(tmp)
		_, _ = fmt.Fprintf(logSink, "!! cache: mkdir failed: %v\n", err)
		return
	}
	_ = os.RemoveAll(base)
	if err := os.Rename(tmp, base); err != nil {
		_ = os.RemoveAll(tmp)
		_, _ = fmt.Fprintf(logSink, "!! cache: commit failed: %v\n", err)
		return
	}
	_, _ = fmt.Fprintf(logSink, "cache: saved %d path(s) for key %q\n", saved, cfg.CacheKey)
}

// copyPath 递归复制文件或目录；跳过非常规文件（socket/fifo/device）。
func copyPath(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		if err := os.MkdirAll(dst, info.Mode().Perm()|0o700); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return copyFile(src, dst, info.Mode().Perm())
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode|0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
