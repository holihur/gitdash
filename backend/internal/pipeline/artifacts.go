package pipeline

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// BuildArtifactArchive 把 cfg.ArtifactPaths 匹配的文件/目录打成 tar.gz 返回。
// agent 侧远程执行时用，服务端用 ExtractArtifactArchive 落盘。
// 未匹配到任何路径时返回 (nil, nil)。
func BuildArtifactArchive(cfg *Config, workdir string) ([]byte, error) {
	if cfg == nil || len(cfg.ArtifactPaths) == 0 {
		return nil, nil
	}
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	added := 0
	for _, p := range cfg.ArtifactPaths {
		src := filepath.Join(workdir, filepath.FromSlash(p))
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := writeTarPath(tw, src, filepath.ToSlash(p)); err != nil {
			_ = tw.Close()
			_ = gw.Close()
			return nil, err
		}
		added++
	}
	if added == 0 {
		_ = tw.Close()
		_ = gw.Close()
		return nil, nil
	}
	if err := tw.Close(); err != nil {
		_ = gw.Close()
		return nil, err
	}
	if err := gw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeTarPath 递归写入一个文件/目录（name 为归档内路径，目录自动补 '/'）。
func writeTarPath(tw *tar.Writer, src, name string) error {
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		hdr := &tar.Header{
			Name: name + "/", Mode: int64(info.Mode().Perm()),
			Typeflag: tar.TypeDir, ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		entries, err := os.ReadDir(src)
		if err != nil {
			return err
		}
		for _, e := range entries {
			if err := writeTarPath(tw, filepath.Join(src, e.Name()), name+"/"+e.Name()); err != nil {
				return err
			}
		}
		return nil
	}
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = name
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	_, cErr := io.Copy(tw, f)
	closeErr := f.Close()
	if cErr != nil {
		return cErr
	}
	return closeErr
}

// ExtractArtifactArchive 把 tar.gz 解包到该次运行的产物目录（服务端侧，远程回传用）。
func ExtractArtifactArchive(owner, repo string, id int64, r io.Reader) error {
	if artifactsDir == "" {
		return fmt.Errorf("artifacts storage not initialized")
	}
	base := ArtifactDir(owner, repo, id)
	_ = os.RemoveAll(base)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() { _ = gr.Close() }()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(hdr.Name)
		if name == "" || strings.Contains(name, "..") {
			continue
		}
		dst := filepath.Join(base, name)
		if rel, rerr := filepath.Rel(base, dst); rerr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			continue // 防目录穿越
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			f, err := os.Create(dst)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				_ = f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		}
	}
	return nil
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
