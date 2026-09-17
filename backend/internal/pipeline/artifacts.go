package pipeline

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// ArtifactFile 一个归档产物的文件条目（路径相对归档根）。
type ArtifactFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// ArtifactDir 某次运行的产物目录：data/artifacts/{owner}/{repo}/run-{id}。
func ArtifactDir(owner, repo string, id int64) string {
	return filepath.Join(artifactsDir, owner, repo, fmt.Sprintf("run-%d", id))
}

// HasArtifacts 该运行是否存有产物（目录存在且非空）。
func HasArtifacts(owner, repo string, id int64) bool {
	if artifactsDir == "" {
		return false
	}
	entries, err := os.ReadDir(ArtifactDir(owner, repo, id))
	return err == nil && len(entries) > 0
}

// ListArtifacts 列出该运行归档的文件（相对路径 + 大小）。
func ListArtifacts(owner, repo string, id int64) ([]ArtifactFile, error) {
	base := ArtifactDir(owner, repo, id)
	var out []ArtifactFile
	err := filepath.Walk(base, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		if info.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(base, path)
		if rerr != nil {
			return rerr
		}
		out = append(out, ArtifactFile{Path: filepath.ToSlash(rel), Size: info.Size()})
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// WriteArtifactArchive 把该运行的产物打成 tar.gz 写入 w。
func WriteArtifactArchive(owner, repo string, id int64, w io.Writer) error {
	base := ArtifactDir(owner, repo, id)
	if _, err := os.Stat(base); err != nil {
		return err
	}
	gw := gzip.NewWriter(w)
	tw := tar.NewWriter(gw)
	walkErr := filepath.Walk(base, func(path string, info os.FileInfo, werr error) error {
		if werr != nil {
			return werr
		}
		rel, rerr := filepath.Rel(base, path)
		if rerr != nil {
			return rerr
		}
		if rel == "." {
			return nil
		}
		hdr, herr := tar.FileInfoHeader(info, "")
		if herr != nil {
			return herr
		}
		hdr.Name = filepath.ToSlash(rel)
		if info.IsDir() {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			return nil
		}
		f, ferr := os.Open(path)
		if ferr != nil {
			return ferr
		}
		_, cErr := io.Copy(tw, f)
		closeErr := f.Close()
		if cErr != nil {
			return cErr
		}
		return closeErr
	})
	if walkErr != nil {
		_ = tw.Close()
		_ = gw.Close()
		return walkErr
	}
	if err := tw.Close(); err != nil {
		_ = gw.Close()
		return err
	}
	return gw.Close()
}

// collectArtifacts 运行成功后把配置的路径归档到运行产物目录（仅服务端本地执行）。
func collectArtifacts(cfg *Config, job RunJob, workdir string, logSink io.Writer) {
	if len(cfg.ArtifactPaths) == 0 || artifactsDir == "" {
		return
	}
	base := ArtifactDir(job.Owner, job.Repo, job.RunID)
	_ = os.RemoveAll(base)
	if err := os.MkdirAll(base, 0o755); err != nil {
		_, _ = fmt.Fprintf(logSink, "!! artifacts: mkdir failed: %v\n", err)
		return
	}
	saved := 0
	for _, p := range cfg.ArtifactPaths {
		src := filepath.Join(workdir, filepath.FromSlash(p))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(base, filepath.FromSlash(p))
		if err := copyPath(src, dst); err != nil {
			_, _ = fmt.Fprintf(logSink, "!! artifacts: %s failed: %v\n", p, err)
			_ = os.RemoveAll(base)
			return
		}
		saved++
	}
	if saved == 0 {
		_ = os.RemoveAll(base)
		_, _ = fmt.Fprintf(logSink, "artifacts: no files matched\n")
		return
	}
	_, _ = fmt.Fprintf(logSink, "artifacts: stored %d path(s)\n", saved)
}
