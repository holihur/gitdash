package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- pypi (twine) ----

// pypiUpload twine 上传（PEP 503 multipart）
//
//	@Summary     pypi 上传
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Success     200
//	@Router      /packages/pypi/{owner}/ [post]
func (a *API) pypiUpload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	if !a.canPublishPackage(owner, pkgUser(r)) {
		pkgForbidden(w)
		return
	}
	if err := r.ParseMultipartForm(maxPackageSize); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_form", err.Error())
		return
	}
	name := r.FormValue("name")
	version := r.FormValue("version")
	if name == "" || version == "" {
		writeCode(w, http.StatusBadRequest, "invalid_form", "name and version required")
		return
	}
	n := 0
	for _, files := range r.MultipartForm.File {
		for _, fh := range files {
			f, err := fh.Open()
			if err != nil {
				continue
			}
			content, err := io.ReadAll(io.LimitReader(f, maxPackageSize))
			_ = f.Close()
			if err != nil {
				continue
			}
			p := &store.Package{Owner: owner, Repo: strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"),
				Type: "pypi", Name: name, Version: version, Filename: fh.Filename, Uploader: pkgUser(r),
				Meta: pypiRequiresPython(fh.Filename, content)}
			if err := a.store.CreatePackage(p, content); err != nil && !errors.Is(err, store.ErrExists) {
				internalError(w, err)
				return
			}
			_ = a.store.AddPackageAudit(owner, "pypi", name, version, "publish", pkgUser(r))
			n++
		}
	}
	if n == 0 {
		writeCode(w, http.StatusBadRequest, "invalid_form", "no file uploaded")
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func pypiNorm(name string) string {
	s := strings.ToLower(name)
	return regexp.MustCompile(`[-_.]+`).ReplaceAllString(s, "-")
}

// pypiRequiresPython 从 wheel（zip 的 dist-info/METADATA）或 sdist（tar.gz 的
// PKG-INFO）提取 Requires-Python，用于 simple 页 data-requires-python 属性。
func pypiRequiresPython(filename string, content []byte) string {
	var meta []byte
	switch {
	case strings.HasSuffix(filename, ".whl"):
		zr, err := zip.NewReader(bytes.NewReader(content), int64(len(content)))
		if err != nil {
			return ""
		}
		for _, zf := range zr.File {
			if strings.HasSuffix(zf.Name, ".dist-info/METADATA") {
				rc, err := zf.Open()
				if err != nil {
					return ""
				}
				meta, _ = io.ReadAll(io.LimitReader(rc, 1<<20))
				_ = rc.Close()
				break
			}
		}
	case strings.HasSuffix(filename, ".tar.gz"):
		gz, err := gzip.NewReader(bytes.NewReader(content))
		if err != nil {
			return ""
		}
		defer func() { _ = gz.Close() }()
		tr := tar.NewReader(gz)
		for {
			hdr, err := tr.Next()
			if err != nil {
				break
			}
			if strings.HasSuffix(hdr.Name, "/PKG-INFO") && strings.Count(hdr.Name, "/") == 1 {
				meta, _ = io.ReadAll(io.LimitReader(tr, 1<<20))
				break
			}
		}
	default:
		return ""
	}
	for _, line := range strings.Split(string(meta), "\n") {
		if v, ok := strings.CutPrefix(line, "Requires-Python:"); ok {
			return strings.TrimSpace(v)
		}
		if line == "" {
			break // headers 结束
		}
	}
	return ""
}

// pypiSimpleIndex PEP 503 simple index
//
//	@Summary     pypi simple index
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Router      /packages/pypi/{owner}/simple [get]
func (a *API) pypiSimpleIndex(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	names, err := a.store.ListAllPackageNames(owner, "pypi")
	if err != nil {
		internalError(w, err)
		return
	}
	base := baseURL(r)
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html><html><head><title>Simple index</title></head><body>\n")
	for _, n := range names {
		fmt.Fprintf(&sb, "<a href=\"%s/api/packages/pypi/%s/simple/%s/\">%s</a><br/>\n",
			base, owner, url.PathEscape(n), n)
	}
	sb.WriteString("</body></html>\n")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(sb.String()))
}

func (a *API) pypiSimpleProject(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := strings.TrimSuffix(r.PathValue("name"), "/")
	names, _ := a.store.ListAllPackageNames(owner, "pypi")
	match := ""
	for _, n := range names {
		if pypiNorm(n) == pypiNorm(name) {
			match = n
			break
		}
	}
	if match == "" || !a.canReadPackage(owner, "pypi", match, pkgUser(r)) {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	pkgs, err := a.store.ListPackageVersions(owner, "pypi", match)
	if err != nil {
		internalError(w, err)
		return
	}
	base := baseURL(r)
	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html><html><head><title>Links</title></head><body>\n")
	for _, p := range pkgs {
		attrs := fmt.Sprintf(` data-hashes="sha256=%s"`, p.Checksum)
		if p.Meta != "" {
			attrs += fmt.Sprintf(` data-requires-python=%q`, html.EscapeString(p.Meta))
		}
		fmt.Fprintf(&sb, "<a%s href=\"%s/api/packages/pypi/%s/download/%s/%s/%s\">%s</a><br/>\n",
			attrs, base, owner, url.PathEscape(p.Name), url.PathEscape(p.Version), url.PathEscape(p.Filename), p.Filename)
	}
	sb.WriteString("</body></html>\n")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(sb.String()))
}

func (a *API) pypiDownload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	version := r.PathValue("version")
	filename := r.PathValue("filename")
	p, content, err := a.store.GetPackageFile(owner, "pypi", name, version, filename)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	servePackageBytes(w, p, content, "application/octet-stream")
}

// pypiJSON pypi JSON API（部分工具依赖）
//
//	@Summary     pypi JSON API
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       name  path string true "包名"
//	@Router      /packages/pypi/{owner}/pypi/{name}/json [get]
func (a *API) pypiJSON(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	if !a.canReadPackage(owner, "pypi", name, pkgUser(r)) {
		pkgForbidden(w)
		return
	}
	pkgs, err := a.store.ListPackageVersions(owner, "pypi", name)
	if err != nil || len(pkgs) == 0 {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	base := baseURL(r)
	releases := map[string]any{}
	latest := ""
	for _, p := range pkgs {
		if latest == "" {
			latest = p.Version
		}
		releases[p.Version] = []map[string]any{{
			"filename": p.Filename,
			"url":      fmt.Sprintf("%s/api/packages/pypi/%s/download/%s/%s/%s", base, owner, p.Name, p.Version, p.Filename),
			"digests":  map[string]string{"sha256": p.Checksum},
			"size":     p.Size,
		}}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"info":     map[string]any{"name": name, "version": latest},
		"releases": releases,
		"urls":     releases[latest],
	})
}
