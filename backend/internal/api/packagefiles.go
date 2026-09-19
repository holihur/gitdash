package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
)

// 包文件浏览（Web UI）：列出包制品、列出归档内容、预览 / 下载单个文件。
//
// 命名空间用独立的 /package-files/ 前缀，避免与包管理器协议路由
// （/packages/{type}/{owner}/{name...}）在 Go 1.22 ServeMux 下冲突。

// packageArchiveEntry 归档内的一个条目。
type packageArchiveEntry struct {
	Name  string `json:"name"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir"`
}

// maxPreviewBytes 单个条目预览上限；超过则截断（响应带 X-Truncated 头）。
const maxPreviewBytes = 2 << 20 // 2MB

// archiveKind 按文件名后缀返回归档类型（zip / tar / tar.gz），非归档返回 ""。
func archiveKind(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"), strings.HasSuffix(lower, ".crate"):
		return "tar.gz"
	case strings.HasSuffix(lower, ".zip"), strings.HasSuffix(lower, ".jar"), strings.HasSuffix(lower, ".whl"):
		return "zip"
	case strings.HasSuffix(lower, ".tar"), strings.HasSuffix(lower, ".gem"):
		return "tar"
	}
	return ""
}

// textContentType 返回文本类文件的 Content-Type（用于内联预览），非文本返回 ""。
func textContentType(filename string) string {
	lower := strings.ToLower(path.Base(filename))
	switch {
	case strings.HasSuffix(lower, ".json"):
		return "application/json"
	case strings.HasSuffix(lower, ".md"), strings.HasSuffix(lower, ".markdown"):
		return "text/markdown"
	case strings.HasSuffix(lower, ".xml"), strings.HasSuffix(lower, ".pom"):
		return "application/xml"
	case strings.HasSuffix(lower, ".yml"), strings.HasSuffix(lower, ".yaml"):
		return "text/yaml"
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".mjs"), strings.HasSuffix(lower, ".cjs"):
		return "text/javascript"
	case strings.HasSuffix(lower, ".html"), strings.HasSuffix(lower, ".htm"):
		return "text/html"
	case strings.HasSuffix(lower, ".css"):
		return "text/css"
	case strings.HasSuffix(lower, ".csv"):
		return "text/csv"
	}
	switch lower {
	case "go.mod", "go.sum", "cargo.toml", "cargo.lock", "gemfile", "rakefile", "makefile",
		"readme", "license", "licence", "notice", "changelog":
		return "text/plain"
	}
	for _, ext := range []string{
		".txt", ".toml", ".cfg", ".ini", ".conf", ".log", ".env", ".mod", ".sum",
		".go", ".py", ".rb", ".php", ".java", ".rs", ".ts", ".tsx", ".jsx", ".c", ".h",
		".cpp", ".hpp", ".sh", ".bash", ".zsh", ".sql", ".gemspec", ".lock",
	} {
		if strings.HasSuffix(lower, ext) {
			return "text/plain"
		}
	}
	return ""
}

// listArchiveEntries 列出归档内的条目。
func listArchiveEntries(kind string, data []byte) ([]packageArchiveEntry, error) {
	switch kind {
	case "zip":
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, err
		}
		out := make([]packageArchiveEntry, 0, len(zr.File))
		for _, f := range zr.File {
			out = append(out, packageArchiveEntry{Name: f.Name, Size: int64(f.UncompressedSize64), IsDir: f.FileInfo().IsDir()})
		}
		return out, nil
	case "tar", "tar.gz":
		tr, closer, err := tarReader(kind, data)
		if err != nil {
			return nil, err
		}
		defer closer()
		var out []packageArchiveEntry
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			out = append(out, packageArchiveEntry{Name: h.Name, Size: h.Size, IsDir: h.Typeflag == tar.TypeDir})
		}
		return out, nil
	}
	return nil, fmt.Errorf("not an archive")
}

// readArchiveEntry 读取归档内指定条目的内容（最多 max 字节），返回 (内容, 是否截断, 是否找到)。
func readArchiveEntry(kind string, data []byte, name string, max int64) ([]byte, bool, bool) {
	readLimited := func(r io.Reader) ([]byte, bool, bool) {
		buf, err := io.ReadAll(io.LimitReader(r, max+1))
		if err != nil {
			return nil, false, false
		}
		if int64(len(buf)) > max {
			return buf[:max], true, true
		}
		return buf, false, true
	}

	switch kind {
	case "zip":
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return nil, false, false
		}
		for _, f := range zr.File {
			if f.Name == name {
				rc, err := f.Open()
				if err != nil {
					return nil, false, false
				}
				defer func() { _ = rc.Close() }()
				return readLimited(rc)
			}
		}
	case "tar", "tar.gz":
		tr, closer, err := tarReader(kind, data)
		if err != nil {
			return nil, false, false
		}
		defer closer()
		for {
			h, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, false, false
			}
			if h.Name == name {
				return readLimited(tr)
			}
		}
	}
	return nil, false, false
}

func tarReader(kind string, data []byte) (*tar.Reader, func(), error) {
	var r io.Reader = bytes.NewReader(data)
	closer := func() {}
	if kind == "tar.gz" {
		gz, err := gzip.NewReader(r)
		if err != nil {
			return nil, closer, err
		}
		r = gz
		closer = func() { _ = gz.Close() }
	}
	return tar.NewReader(r), closer, nil
}

// packageFiles 列出包制品 / 浏览归档 / 预览或下载文件。
//
//	@Summary     浏览包文件
//	@Description 不带 filename 时列出包的全部版本制品；带 filename 时：
//	             entries=1 返回归档条目列表，entry=<name> 返回条目内容，
//	             否则返回制品原始内容（download=1 强制附件下载）。
//	@Tags        packages
//	@Param       type     path  string true  "包类型"
//	@Param       owner    path  string true  "用户或组织"
//	@Param       name     path  string true  "包名"
//	@Param       version  query string false "版本"
//	@Param       filename query string false "制品文件名"
//	@Param       entries  query string false "=1 列出归档条目"
//	@Param       entry    query string false "归档内条目名"
//	@Param       download query string false "=1 作为附件下载"
//	@Success     200
//	@Router      /package-files/{type}/{owner}/{name} [get]
func (a *API) packageFiles(w http.ResponseWriter, r *http.Request) {
	typ := r.PathValue("type")
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	if !validPkgTypes[typ] {
		writeCode(w, http.StatusBadRequest, "invalid_type", "unknown package type")
		return
	}
	if !a.canReadPackage(owner, typ, name, pkgUser(r)) {
		writeCode(w, http.StatusForbidden, "forbidden", "you do not have permission to read this package")
		return
	}

	version := r.URL.Query().Get("version")
	filename := r.URL.Query().Get("filename")

	// 列出制品
	if filename == "" {
		files, err := a.store.ListPackageVersions(owner, typ, name)
		if err != nil {
			internalError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, files)
		return
	}

	// 归档条目列表不需要文件内容，但仍校验存在性与权限。
	if r.URL.Query().Get("entries") == "1" {
		kind := archiveKind(filename)
		if kind == "" {
			writeCode(w, http.StatusBadRequest, "not_archive", "file is not a supported archive")
			return
		}
		_, data, err := a.store.ReadPackageFile(owner, typ, name, version, filename)
		if err != nil {
			writeCode(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		entries, err := listArchiveEntries(kind, data)
		if err != nil {
			writeCode(w, http.StatusBadRequest, "bad_archive", "could not read archive")
			return
		}
		writeJSON(w, http.StatusOK, entries)
		return
	}

	// 单个归档条目内容
	entry := r.URL.Query().Get("entry")
	if entry != "" {
		kind := archiveKind(filename)
		if kind == "" {
			writeCode(w, http.StatusBadRequest, "not_archive", "file is not a supported archive")
			return
		}
		_, data, err := a.store.ReadPackageFile(owner, typ, name, version, filename)
		if err != nil {
			writeCode(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		content, truncated, found := readArchiveEntry(kind, data, entry, maxPreviewBytes)
		if !found {
			writeCode(w, http.StatusNotFound, "not_found", "entry not found")
			return
		}
		if truncated {
			w.Header().Set("X-Truncated", "1")
		}
		ct := textContentType(entry)
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", path.Base(entry)))
		_, _ = w.Write(content)
		return
	}

	// 制品原始内容（download=1 计入下载并作为附件）
	download := r.URL.Query().Get("download") == "1"
	var content []byte
	var err error
	if download {
		_, content, err = a.store.GetPackageFile(owner, typ, name, version, filename)
	} else {
		_, content, err = a.store.ReadPackageFile(owner, typ, name, version, filename)
	}
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	ct := textContentType(filename)
	disposition := "inline"
	if download || ct == "" {
		disposition = "attachment"
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Disposition", fmt.Sprintf("%s; filename=%q", disposition, path.Base(filename)))
	_, _ = w.Write(content)
}
