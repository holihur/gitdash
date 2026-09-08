package api

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"sort"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- go (GOPROXY) ----

func (a *API) goPut(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	i := strings.Index(rest, "/@v/")
	if i < 0 {
		writeCode(w, http.StatusBadRequest, "invalid_path", "expected {module}/@v/{file}")
		return
	}
	r.SetPathValue("module", rest[:i])
	r.SetPathValue("file", rest[i+4:])
	a.goUpload(w, r)
}

// goRoute go GOPROXY 读取（@v/list、@latest、.info/.mod/.zip）
//
//	@Summary     go GOPROXY
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "{module}/@v/{file}"
//	@Router      /packages/go/{owner}/{rest} [get]
func (a *API) goRoute(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	i := strings.Index(rest, "/@v/")
	if i < 0 {
		writeCode(w, http.StatusBadRequest, "invalid_path", "expected {module}/@v/{file}")
		return
	}
	module, file := rest[:i], rest[i+4:]
	r.SetPathValue("module", module)
	r.SetPathValue("file", file)
	a.goGet(w, r)
}

// goUpload go module 上传（GOPROXY zip）
//
//	@Summary     go 上传
//	@Tags        packages
//	@Param       owner  path string true "用户或组织"
//	@Param       rest   path string true "{module}/@v/{version}.zip"
//	@Success     201
//	@Router      /packages/go/{owner}/{rest} [put]
func (a *API) goUpload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	module := r.PathValue("module")
	file := r.PathValue("file")
	if !strings.HasSuffix(file, ".zip") {
		writeCode(w, http.StatusBadRequest, "invalid_file", "only .zip uploads supported")
		return
	}
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	// 流式落盘：不把 module zip 全量读入内存
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageSize)
	tmp, err := os.CreateTemp("", "gitdash-go-*.zip")
	if err != nil {
		internalError(w, err)
		return
	}
	tmpName := tmp.Name()
	size, err := io.Copy(tmp, r.Body)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(tmpName)
		writeCode(w, http.StatusRequestEntityTooLarge, "package_too_large", fmt.Sprintf("package exceeds %d bytes", maxPackageSize))
		return
	}
	_ = size
	version := strings.TrimSuffix(file, ".zip")
	p := &store.Package{
		Owner: owner, Repo: strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"),
		Type: "go", Name: module, Version: version, Filename: file, Uploader: user,
		Size: size,
	}
	if err := a.store.CreatePackageFromFile(p, tmpName); err != nil {
		_ = os.Remove(tmpName)
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "package_exists", "package version already exists")
			return
		}
		internalError(w, err)
		return
	}
	_ = a.store.AddPackageAudit(owner, "go", module, version, "publish", user)
	writeJSON(w, http.StatusCreated, p)
}

func goModFromZip(zipData []byte) string {
	zr, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return ""
	}
	for _, f := range zr.File {
		if strings.HasSuffix(f.Name, "go.mod") && strings.Count(f.Name, "/") == 1 {
			rc, err := f.Open()
			if err != nil {
				return ""
			}
			defer func() { _ = rc.Close() }()
			b, err := io.ReadAll(io.LimitReader(rc, 1<<20))
			if err != nil {
				return ""
			}
			return string(b)
		}
	}
	return ""
}

func (a *API) goGet(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	module := r.PathValue("module")
	file := r.PathValue("file")
	switch file {
	case "list":
		pkgs, _ := a.store.ListPackageVersions(owner, "go", module)
		seen := map[string]bool{}
		var lines []string
		for _, p := range pkgs {
			if !seen[p.Version] {
				seen[p.Version] = true
				lines = append(lines, p.Version+"\n")
			}
		}
		sort.Strings(lines)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, l := range lines {
			_, _ = w.Write([]byte(l))
		}
		return
	case "latest":
		pkgs, _ := a.store.ListPackageVersions(owner, "go", module)
		if len(pkgs) == 0 {
			writeCode(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		p := pkgs[0]
		writeJSON(w, http.StatusOK, map[string]string{"Version": p.Version, "Time": p.CreatedAt})
		return
	}
	ext := path.Ext(file)
	version := strings.TrimSuffix(file, ext)
	p, content, err := a.store.GetPackageVersion(owner, "go", module, version)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	switch ext {
	case ".info":
		writeJSON(w, http.StatusOK, map[string]string{"Version": version, "Time": p.CreatedAt})
	case ".mod":
		mod := goModFromZip(content)
		if mod == "" {
			mod = "module " + module + "\n"
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(mod))
	case ".zip":
		servePackageBytes(w, p, content, "application/zip")
	default:
		writeCode(w, http.StatusNotFound, "not_found", "not found")
	}
}
