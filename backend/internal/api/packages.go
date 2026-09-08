package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gitdash/backend/internal/store"
)

// ---- 私有包注册表（npm / composer / pypi / rubygems / go / cargo / maven）----
//
// 命名空间：/api/packages/{type}/{owner}/...，owner 为用户或组织。
// 读：任意已认证用户（若包关联了仓库，则跟随该仓库可见性）；
// 写（发布/删除/yank）：owner 本人或组织 owner 角色成员。
// 文件内容走内容寻址磁盘存储，元数据/审计/下载计数在 DB。

const maxPackageSize = 64 << 20 // 64MB

// pkgUser 取已认证用户名（由 a.auth 注入）。
func pkgUser(r *http.Request) string {
	if u, ok := r.Context().Value(ctxUser{}).(string); ok {
		return u
	}
	return ""
}

// canPublishPackage 发布/删除权限：命名空间 owner 本人或组织 owner 角色成员。
func (a *API) canPublishPackage(owner, username string) bool {
	if username == owner {
		return true
	}
	return a.store.OrgRole(owner, username) == "owner"
}

// pkgLinkedRepo 返回包关联的仓库（X-Gitdash-Repo 头指定，取任一版本的关联值）。
func (a *API) pkgLinkedRepo(owner, typ, name string) string {
	pkgs, err := a.store.ListPackageVersions(owner, typ, name)
	if err != nil {
		return ""
	}
	for _, p := range pkgs {
		if p.Repo != "" {
			return p.Repo
		}
	}
	return ""
}

// canReadPackage 读权限：默认任意已认证用户；若包关联了私有仓库，则要求仓库读权限。
func (a *API) canReadPackage(owner, typ, name, username string) bool {
	repo := a.pkgLinkedRepo(owner, typ, name)
	if repo == "" {
		return true
	}
	r, err := a.store.GetRepo(owner, repo)
	if err != nil || !r.Private {
		return true
	}
	if username == owner || a.store.OrgRole(owner, username) != "" {
		return true
	}
	collabs, err := a.store.ListCollabs(owner, repo)
	if err != nil {
		return false
	}
	for _, c := range collabs {
		if c.Username == username {
			return true
		}
	}
	return false
}

func pkgForbidden(w http.ResponseWriter) {
	writeCode(w, http.StatusForbidden, "forbidden", "you do not have permission to publish to this namespace")
}

// baseURL 请求的基础 URL（http(s)://host）。
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

// limitBody 限制请求体大小（包文件上传），超限返回 413。
func limitBody(w http.ResponseWriter, r *http.Request) ([]byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageSize)
	b, err := io.ReadAll(r.Body)
	if err != nil {
		writeCode(w, http.StatusRequestEntityTooLarge, "package_too_large", fmt.Sprintf("package exceeds %d bytes", maxPackageSize))
		return nil, false
	}
	return b, true
}

var validPkgTypes = map[string]bool{
	"npm": true, "composer": true, "pypi": true, "rubygems": true,
	"go": true, "cargo": true, "maven": true,
}

// savePackage 存储包文件（内容寻址）并记录审计。
func (a *API) savePackage(w http.ResponseWriter, r *http.Request, owner, typ, name, version, filename string, content []byte) {
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	p := &store.Package{Owner: owner, Repo: strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"),
		Type: typ, Name: name, Version: version, Filename: filename, Uploader: user}
	if err := a.store.CreatePackage(p, content); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "package_exists", "package version already exists")
			return
		}
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = a.store.AddPackageAudit(owner, typ, name, version, "publish", user)
	writeJSON(w, http.StatusCreated, p)
}

// servePackageBytes 统一文件下载（计数由 store 递增）。
func servePackageBytes(w http.ResponseWriter, p store.Package, content []byte, contentType string) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("X-Checksum-Sha256", p.Checksum)
	_, _ = w.Write(content)
}

// ---- 通用 listing（web UI）----

// listPackagesUI 列出命名空间下的包
//
//	@Summary     列出包
//	@Description 列出 owner 命名空间下的包（type 可选过滤：npm/composer/pypi/rubygems/go/cargo/maven）
//	@Tags        packages
//	@Param       owner  path string true "用户或组织"
//	@Param       type   path string false "包类型"
//	@Produce     json
//	@Success     200 {array} store.Package
//	@Router      /packages/{owner}/{type} [get]
func (a *API) listPackagesUI(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	typ := r.PathValue("type")
	if typ != "" && !validPkgTypes[typ] {
		writeCode(w, http.StatusBadRequest, "invalid_type", "unknown package type")
		return
	}
	limit, offset := pageParams(r)
	pkgs, total, err := a.store.ListPackages(owner, typ, limit, offset)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, pkgs)
}

// listPackageAudit 包操作审计记录
//
//	@Summary     包审计记录
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       type  query string false "包类型"
//	@Param       name  query string false "包名"
//	@Produce     json
//	@Success     200 {array} store.PackageAudit
//	@Router      /packages/{owner}/audit [get]
func (a *API) listPackageAudit(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	typ := r.URL.Query().Get("type")
	name := r.URL.Query().Get("name")
	limit, offset := pageParams(r)
	rows, total, err := a.store.ListPackageAudit(owner, typ, name, limit, offset)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, rows)
}

// deletePackageUI 删除包（全版本或单版本）
//
//	@Summary     删除包
//	@Tags        packages
//	@Param       type    path string true "包类型"
//	@Param       owner   path string true "用户或组织"
//	@Param       name    path string true "包名"
//	@Success     204
//	@Router      /packages/{type}/{owner}/{name} [delete]
func (a *API) deletePackageUI(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	typ := r.PathValue("type")
	name := r.PathValue("name")
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	if v := r.PathValue("version"); v != "" {
		if err := a.store.DeletePackageVersion(owner, typ, name, v); err != nil {
			writeCode(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		_ = a.store.AddPackageAudit(owner, typ, name, v, "delete", user)
	} else {
		if err := a.store.DeletePackage(owner, typ, name); err != nil {
			writeCode(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		_ = a.store.AddPackageAudit(owner, typ, name, "", "delete", user)
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- npm ----

type npmDist struct {
	Tarball   string `json:"tarball"`
	Shasum    string `json:"shasum"`
	Integrity string `json:"integrity"`
}

func npmDigests(content []byte) (shasum, integrity string) {
	s1 := sha1.Sum(content)
	s512 := sha512.Sum512(content)
	return hex.EncodeToString(s1[:]), "sha512-" + base64.StdEncoding.EncodeToString(s512[:])
}

// npmPublish npm 发布（npm publish，_attachments 内含 base64 tarball）或 dist-tag 操作
//
//	@Summary     npm 发布 / dist-tag
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "包名（可含 @scope/）或 dist-tag 路径"
//	@Success     201
//	@Router      /packages/npm/{owner}/{rest} [put]
func (a *API) npmPublish(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	rest := r.PathValue("rest")

	// dist-tag 路径：{name}/-/package/dist-tags 或 {name}/-/package/dist-tags/{tag}
	if rest != "" && r.URL.Path != "" && strings.Contains(rest, "dist-tags") {
		i := strings.Index(rest, "/-/package/dist-tags")
		if i < 0 {
			i = strings.Index(rest, "/dist-tags")
		}
		if i >= 0 {
			name := rest[:i]
			remainder := rest[i:]
			remainder = strings.TrimPrefix(remainder, "/-/package/dist-tags")
			remainder = strings.TrimPrefix(remainder, "/dist-tags")
			remainder = strings.Trim(remainder, "/")
			if !a.canPublishPackage(owner, pkgUser(r)) {
				pkgForbidden(w)
				return
			}
			if remainder == "" {
				// PUT 整个 tags 映射
				var tags map[string]string
				if err := json.NewDecoder(r.Body).Decode(&tags); err != nil {
					writeCode(w, http.StatusBadRequest, "invalid_tags", "invalid dist-tags body")
					return
				}
				for tag, ver := range tags {
					_ = a.store.SetPackageTag(owner, "npm", name, tag, ver)
				}
				writeJSON(w, http.StatusOK, map[string]any{"ok": true})
				return
			}
			// 单个 tag：body 为 JSON 字符串
			var ver string
			if err := json.NewDecoder(r.Body).Decode(&ver); err != nil {
				writeCode(w, http.StatusBadRequest, "invalid_tag", "expected JSON string version")
				return
			}
			if r.Method == http.MethodDelete {
				_ = a.store.SetPackageTag(owner, "npm", name, remainder, "")
			} else {
				_ = a.store.SetPackageTag(owner, "npm", name, remainder, ver)
				_ = a.store.AddPackageAudit(owner, "npm", name, ver, "tag:"+remainder, pkgUser(r))
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		}
	}

	name := rest
	body, ok := limitBody(w, r)
	if !ok {
		return
	}
	var doc struct {
		Name        string                     `json:"name"`
		Versions    map[string]json.RawMessage `json:"versions"`
		DistTags    map[string]string          `json:"dist-tags"`
		Attachments map[string]struct {
			Length int    `json:"length"`
			Data   string `json:"data"`
		} `json:"_attachments"`
	}
	if err := json.Unmarshal(body, &doc); err != nil || len(doc.Versions) == 0 {
		writeCode(w, http.StatusBadRequest, "invalid_npm_payload", "missing versions")
		return
	}
	if !a.canPublishPackage(owner, pkgUser(r)) {
		pkgForbidden(w)
		return
	}
	// 已存在的版本：返回 npm 客户端可识别的 EPUBLISHCONFLICT 冲突体
	existing, err := a.store.ListPackageVersions(owner, "npm", name)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	have := map[string]bool{}
	for _, p := range existing {
		have[p.Version] = true
	}
	for ver := range doc.Versions {
		if have[ver] {
			writeJSON(w, http.StatusConflict, map[string]string{
				"error":  "EPUBLISHCONFLICT",
				"reason": fmt.Sprintf("cannot publish over the previously published version %s.", ver),
			})
			return
		}
	}
	for ver := range doc.Versions {
		for fname, att := range doc.Attachments {
			raw, err := base64.StdEncoding.DecodeString(att.Data)
			if err != nil {
				writeCode(w, http.StatusBadRequest, "invalid_attachment", "attachment data is not valid base64")
				return
			}
			p := &store.Package{Owner: owner, Repo: strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"),
				Type: "npm", Name: name, Version: ver, Filename: fname, Uploader: pkgUser(r)}
			if err := a.store.CreatePackage(p, raw); errors.Is(err, store.ErrExists) {
				continue
			} else if err != nil {
				writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
				return
			}
			_ = a.store.AddPackageAudit(owner, "npm", name, ver, "publish", pkgUser(r))
		}
	}
	for tag, ver := range doc.DistTags {
		_ = a.store.SetPackageTag(owner, "npm", name, tag, ver)
	}
	// 无显式 latest 时设为本次发布的版本
	tags, _ := a.store.ListPackageTags(owner, "npm", name)
	if _, ok := tags["latest"]; !ok {
		latest := ""
		for ver := range doc.Versions {
			latest = ver
		}
		_ = a.store.SetPackageTag(owner, "npm", name, "latest", latest)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true})
}

// npmGet npm packument 元数据、dist-tags 或 tarball（npm install）
//
//	@Summary     npm 元数据 / tarball
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "包名 或 包名/-/文件名 或 dist-tags 路径"
//	@Router      /packages/npm/{owner}/{rest} [get]
func (a *API) npmGet(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	rest := r.PathValue("rest")

	if strings.Contains(rest, "dist-tags") {
		i := strings.Index(rest, "/-/package/dist-tags")
		if i < 0 {
			i = strings.Index(rest, "/dist-tags")
		}
		name := rest[:i]
		if !a.canReadPackage(owner, "npm", name, pkgUser(r)) {
			pkgForbidden(w)
			return
		}
		tags, err := a.store.ListPackageTags(owner, "npm", name)
		if err != nil {
			writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
		writeJSON(w, http.StatusOK, tags)
		return
	}

	// tarball：{name}/-/{filename}
	if i := strings.Index(rest, "/-/"); i >= 0 {
		name, filename := rest[:i], rest[i+3:]
		if !a.canReadPackage(owner, "npm", name, pkgUser(r)) {
			pkgForbidden(w)
			return
		}
		r.SetPathValue("name", name)
		r.SetPathValue("filename", filename)
		a.npmTarball(w, r)
		return
	}
	if !a.canReadPackage(owner, "npm", rest, pkgUser(r)) {
		pkgForbidden(w)
		return
	}
	r.SetPathValue("name", rest)
	a.npmPackument(w, r)
}

func (a *API) npmPackument(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	pkgs, err := a.store.ListPackageVersions(owner, "npm", name)
	if err != nil || len(pkgs) == 0 {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	base := baseURL(r)
	tags, _ := a.store.ListPackageTags(owner, "npm", name)
	if len(tags) == 0 {
		tags = map[string]string{"latest": pkgs[0].Version}
	}
	versions := map[string]any{}
	for _, p := range pkgs {
		_, content, err := a.store.GetPackageFile(owner, "npm", name, p.Version, p.Filename)
		if err != nil {
			continue
		}
		shasum, integrity := npmDigests(content)
		versions[p.Version] = map[string]any{
			"name":    name,
			"version": p.Version,
			"dist": npmDist{
				Tarball:   fmt.Sprintf("%s/api/packages/npm/%s/%s/-/%s", base, owner, name, p.Filename),
				Shasum:    shasum,
				Integrity: integrity,
			},
		}
	}
	maintainer := owner
	if pkgs[0].Uploader != "" {
		maintainer = pkgs[0].Uploader
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"_id": name, "name": name, "dist-tags": tags, "versions": versions,
		"maintainers": []map[string]string{{"name": maintainer}},
		"readme":      "",
	})
}

func (a *API) npmTarball(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	filename := r.PathValue("filename")
	pkgs, err := a.store.ListPackageVersions(owner, "npm", name)
	if err != nil {
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	for _, p := range pkgs {
		if p.Filename == filename {
			_, content, err := a.store.GetPackageFile(owner, "npm", name, p.Version, filename)
			if err != nil {
				writeCode(w, http.StatusNotFound, "not_found", "not found")
				return
			}
			servePackageBytes(w, p, content, "application/gzip")
			return
		}
	}
	writeCode(w, http.StatusNotFound, "not_found", "not found")
}

func (a *API) npmDelete(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	version := r.PathValue("version")
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	if version != "" {
		if err := a.store.DeletePackageVersion(owner, "npm", name, version); err != nil {
			writeCode(w, http.StatusNotFound, "not_found", "not found")
			return
		}
	} else if err := a.store.DeletePackage(owner, "npm", name); err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	_ = a.store.AddPackageAudit(owner, "npm", name, version, "delete", user)
	w.WriteHeader(http.StatusNoContent)
}

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
				writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
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
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
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
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
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
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
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
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
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

// ---- cargo ----

// cargoConfig cargo registry config
//
//	@Summary     cargo config
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Router      /packages/cargo/{owner}/config.json [get]
func (a *API) cargoConfig(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	base := baseURL(r)
	writeJSON(w, http.StatusOK, map[string]string{
		"dl":  base + "/api/packages/cargo/" + owner + "/dl",
		"api": base + "/api/packages/cargo/" + owner,
	})
}

// cargoPublish cargo publish
//
//	@Summary     cargo 发布
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Success     201
//	@Router      /packages/cargo/{owner}/api/v1/crates/new [put]
func (a *API) cargoPublish(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	body, ok := limitBody(w, r)
	if !ok {
		return
	}
	// cargo publish：第一行 JSON 元数据，随后是 .crate tarball
	idx := bytes.IndexByte(body, '\n')
	if idx < 0 {
		writeCode(w, http.StatusBadRequest, "invalid_cargo_payload", "missing metadata line")
		return
	}
	var meta struct {
		Name     string `json:"name"`
		Vers     string `json:"vers"`
		Checksum string `json:"cksum"`
	}
	if err := json.Unmarshal(body[:idx], &meta); err != nil || meta.Name == "" || meta.Vers == "" {
		writeCode(w, http.StatusBadRequest, "invalid_cargo_payload", "bad metadata JSON")
		return
	}
	content := body[idx+1:]
	if meta.Checksum != "" {
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != meta.Checksum {
			writeCode(w, http.StatusBadRequest, "cksum_mismatch", "crate checksum mismatch")
			return
		}
	}
	depsJSON, featuresJSON := cargoIndexMeta(content)
	p := &store.Package{Owner: owner, Repo: strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"),
		Type: "cargo", Name: meta.Name, Version: meta.Vers, Filename: meta.Name + "-" + meta.Vers + ".crate",
		Uploader: pkgUser(r), Meta: `{"deps":` + depsJSON + `,"features":` + featuresJSON + `}`}
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	if err := a.store.CreatePackage(p, content); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "package_exists", "package version already exists")
			return
		}
		writeCode(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	_ = a.store.AddPackageAudit(owner, "cargo", meta.Name, meta.Vers, "publish", user)
	writeJSON(w, http.StatusCreated, p)
}

// cargoIndexMeta 解析 .crate（gzip tar）内的 Cargo.toml，产出稀疏索引所需
// 的 deps 与 features JSON片段。解析失败返回空数组/空对象（不影响发布）。
func cargoIndexMeta(crate []byte) (depsJSON, featuresJSON string) {
	deps, features := []cargoDep{}, map[string][]string{}
	gz, err := gzip.NewReader(bytes.NewReader(crate))
	if err != nil {
		return "[]", "{}"
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	prefix := ""
	for {
		hdr, err := tr.Next()
		if err != nil {
			break
		}
		if prefix == "" && strings.HasSuffix(hdr.Name, "/Cargo.toml") && strings.Count(hdr.Name, "/") == 1 {
			prefix = hdr.Name
			b, err := io.ReadAll(io.LimitReader(tr, 4<<20))
			if err == nil {
				deps, features = parseCargoToml(string(b))
			}
			break
		}
	}
	_ = prefix
	db, _ := json.Marshal(deps)
	fb, _ := json.Marshal(features)
	return string(db), string(fb)
}

type cargoDep struct {
	Name            string   `json:"name"`
	Req             string   `json:"req"`
	Kind            string   `json:"kind,omitempty"` // dev / build；normal 省略
	Optional        bool     `json:"optional,omitempty"`
	DefaultFeatures bool     `json:"default_features"`
	Features        []string `json:"features,omitempty"`
	Package         string   `json:"package,omitempty"`
}

var cargoTomlKeyRe = regexp.MustCompile(`^([A-Za-z0-9_.-]+)\s*=\s*(.+)$`)

// parseCargoToml 极简 TOML 子集解析（仅依赖与特性段）。
func parseCargoToml(src string) ([]cargoDep, map[string][]string) {
	var deps []cargoDep
	features := map[string][]string{}
	section := ""
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") {
			section = strings.Trim(line, "[]")
			continue
		}
		m := cargoTomlKeyRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		key, val := m[1], strings.TrimSpace(m[2])
		switch section {
		case "features":
			if arr, ok := parseTomlArray(val); ok {
				if arr == nil {
					arr = []string{}
				}
				features[key] = arr
			}
		case "dependencies", "dev-dependencies", "build-dependencies":
			kind := ""
			switch section {
			case "dev-dependencies":
				kind = "dev"
			case "build-dependencies":
				kind = "build"
			}
			d := cargoDep{Name: key, DefaultFeatures: true, Kind: kind}
			if strings.HasPrefix(val, "{") {
				// TOML inline table 的 bare key 转成 JSON 引号键再解析
				jsonish := regexp.MustCompile(`([{,]\s*)([A-Za-z0-9_.-]+)\s*=`).
					ReplaceAllString(val, `$1"$2":`)
				jsonish = strings.TrimSuffix(jsonish, "}") + "}"
				// TOML 尾逗号清理
				jsonish = regexp.MustCompile(`,\s*}`).ReplaceAllString(jsonish, "}")
				var tbl map[string]any
				if err := json.Unmarshal([]byte(jsonish), &tbl); err != nil {
					continue
				}
				if v, ok := tbl["package"].(string); ok {
					d.Package = v
				}
				if v, ok := tbl["version"].(string); ok {
					d.Req = v
				} else if _, has := tbl["version"]; !has {
					if _, ok := tbl["path"]; ok {
						d.Req = "*"
					}
				}
				if v, ok := tbl["optional"].(bool); ok {
					d.Optional = v
				}
				if v, ok := tbl["default-features"].(bool); ok {
					d.DefaultFeatures = v
				}
				if v, ok := tbl["features"].([]any); ok {
					for _, f := range v {
						if s, ok := f.(string); ok {
							d.Features = append(d.Features, s)
						}
					}
				}
			} else {
				d.Req = strings.Trim(val, `"`)
			}
			deps = append(deps, d)
		}
	}
	return deps, features
}

// parseTomlArray 解析 ["a", "b"] 形式的数组字面量。
func parseTomlArray(s string) ([]string, bool) {
	if !strings.HasPrefix(s, "[") {
		return nil, false
	}
	end := strings.LastIndex(s, "]")
	if end < 0 {
		return nil, false
	}
	inner := s[1:end]
	var out []string
	for _, part := range strings.Split(inner, ",") {
		part = strings.TrimSpace(part)
		part = strings.Split(part, "#")[0] // 去注释
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if !strings.HasPrefix(part, `"`) || !strings.HasSuffix(part, `"`) {
			return nil, false
		}
		out = append(out, strings.Trim(part, `"`))
	}
	return out, true
}

// cargoIndex cargo 稀疏索引
//
//	@Summary     cargo 索引
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "稀疏索引路径（最后一段为 crate 名）"
//	@Router      /packages/cargo/{owner}/index/{rest} [get]
func (a *API) cargoIndex(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	rest := r.PathValue("rest")
	crate := path.Base(rest) // 稀疏索引路径最后一段即 crate 名
	if !a.canReadPackage(owner, "cargo", crate, pkgUser(r)) {
		pkgForbidden(w)
		return
	}
	pkgs, _ := a.store.ListPackageVersions(owner, "cargo", crate)
	if len(pkgs) == 0 {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	var lines []string
	for _, p := range pkgs {
		yanked := "false"
		if p.Yanked {
			yanked = "true"
		}
		deps, features := "[]", "{}"
		if p.Meta != "" {
			var m struct {
				Deps     json.RawMessage     `json:"deps"`
				Features map[string][]string `json:"features"`
			}
			if json.Unmarshal([]byte(p.Meta), &m) == nil && len(m.Deps) > 0 {
				deps = string(m.Deps)
			}
			if m.Features != nil {
				if fb, err := json.Marshal(m.Features); err == nil {
					features = string(fb)
				}
			}
		}
		lines = append(lines, fmt.Sprintf(
			`{"name":%q,"vers":%q,"deps":%s,"cksum":%q,"features":%s,"yanked":%s,"link":null}`,
			p.Name, p.Version, deps, p.Checksum, features, yanked))
	}
	sort.Strings(lines)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	for _, l := range lines {
		_, _ = w.Write([]byte(l + "\n"))
	}
}

// cargoYank cargo yank / unyank
//
//	@Summary     cargo yank / unyank
//	@Tags        packages
//	@Param       owner   path string true "用户或组织"
//	@Param       crate   path string true "crate 名"
//	@Param       version path string true "版本"
//	@Success     200
//	@Router      /packages/cargo/{owner}/api/v1/crates/{crate}/{version}/yank [delete]
func (a *API) cargoYank(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	crate := r.PathValue("crate")
	version := r.PathValue("version")
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	yanked := r.Method != http.MethodPut
	if err := a.store.SetPackageYanked(owner, "cargo", crate, version, yanked); err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	action := "unyank"
	if yanked {
		action = "yank"
	}
	_ = a.store.AddPackageAudit(owner, "cargo", crate, version, action, user)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "warnings": map[string]any{}})
}

func (a *API) cargoDownload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	crate := r.PathValue("crate")
	version := r.PathValue("version")
	filename := r.PathValue("filename")
	p, content, err := a.store.GetPackageFile(owner, "cargo", crate, version, filename)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	servePackageBytes(w, p, content, "application/octet-stream")
}

// ---- rubygems ----

var gemNameRe = regexp.MustCompile(`(?m)^\s*name:\s*(\S+)`)

// 真实版本行为 "  version: 1.0.0"（首字符为数字）；跳过 "version: !ruby/object:..." 行
var gemVersionRe = regexp.MustCompile(`(?m)^\s*version:\s*([0-9][^\s]*)`)

func gemInfoFromGem(gem []byte) (name, version string) {
	// gem 是未压缩 tar，内含 metadata.gz（gzip 的 YAML gemspec）
	tr := tar.NewReader(bytes.NewReader(gem))
	for {
		hdr, err := tr.Next()
		if err != nil {
			return "", ""
		}
		if hdr.Name == "metadata.gz" {
			gz, err := gzip.NewReader(tr)
			if err != nil {
				return "", ""
			}
			defer func() { _ = gz.Close() }()
			b, err := io.ReadAll(io.LimitReader(gz, 1<<20))
			if err != nil {
				return "", ""
			}
			m := gemNameRe.FindSubmatch(b)
			v := gemVersionRe.FindSubmatch(b)
			if m != nil {
				name = string(m[1])
			}
			if v != nil {
				version = string(v[1])
			}
			return name, version
		}
	}
}

// gemPush gem push
//
//	@Summary     rubygems 发布
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Success     201
//	@Router      /packages/rubygems/{owner}/api/v1/gems [post]
func (a *API) gemPush(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	body, ok := limitBody(w, r)
	if !ok {
		return
	}
	name, version := gemInfoFromGem(body)
	if name == "" || version == "" {
		writeCode(w, http.StatusBadRequest, "invalid_gem", "cannot parse gem metadata")
		return
	}
	a.savePackage(w, r, owner, "rubygems", name, version, name+"-"+version+".gem", body)
}

func (a *API) gemDownload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	filename := r.PathValue("filename")
	names, _ := a.store.ListAllPackageNames(owner, "rubygems")
	for _, n := range names {
		if !a.canReadPackage(owner, "rubygems", n, pkgUser(r)) {
			continue
		}
		ps, _ := a.store.ListPackageVersions(owner, "rubygems", n)
		for _, p := range ps {
			if p.Filename == filename {
				_, content, err := a.store.GetPackageFile(owner, "rubygems", p.Name, p.Version, filename)
				if err != nil {
					writeCode(w, http.StatusNotFound, "not_found", "not found")
					return
				}
				servePackageBytes(w, p, content, "application/octet-stream")
				return
			}
		}
	}
	writeCode(w, http.StatusNotFound, "not_found", "not found")
}

// ---- composer ----

type composerDist struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// composerUpload composer 包上传（?version=）
//
//	@Summary     composer 上传
//	@Tags        packages
//	@Param       owner   path string true "用户或组织"
//	@Param       vendor  path string true "vendor"
//	@Param       name    path string true "包名"
//	@Param       version query  string true "版本"
//	@Success     201
//	@Router      /packages/composer/{owner}/{vendor}/{name} [put]
func (a *API) composerUpload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	vendor := r.PathValue("vendor")
	name := r.PathValue("name")
	version := r.URL.Query().Get("version")
	if version == "" {
		writeCode(w, http.StatusBadRequest, "version_required", "?version= required")
		return
	}
	body, ok := limitBody(w, r)
	if !ok {
		return
	}
	filename := name + "-" + version + ".zip"
	a.savePackage(w, r, owner, "composer", vendor+"/"+name, version, filename, body)
}

// composerPackagesJSON composer 元数据（packages.json）
//
//	@Summary     composer 元数据
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Router      /packages/composer/{owner}/packages.json [get]
func (a *API) composerPackagesJSON(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	names, _ := a.store.ListAllPackageNames(owner, "composer")
	base := baseURL(r)
	packages := map[string]any{}
	for _, n := range names {
		if !a.canReadPackage(owner, "composer", n, pkgUser(r)) {
			continue
		}
		pkgs, _ := a.store.ListPackageVersions(owner, "composer", n)
		vers := map[string]any{}
		for _, p := range pkgs {
			vers[p.Version] = map[string]any{
				"name":    n,
				"version": p.Version,
				"dist":    composerDist{Type: "zip", URL: fmt.Sprintf("%s/api/packages/composer/%s/download/%s/%s/%s", base, owner, n, p.Version, p.Filename)},
			}
		}
		packages[n] = vers
	}
	writeJSON(w, http.StatusOK, map[string]any{"packages": packages, "minified": "composer/2.0"})
}

func (a *API) composerP2(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	vendor := r.PathValue("vendor")
	name := r.PathValue("name")
	full := vendor + "/" + name
	if !a.canReadPackage(owner, "composer", full, pkgUser(r)) {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	pkgs, err := a.store.ListPackageVersions(owner, "composer", full)
	if err != nil || len(pkgs) == 0 {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	base := baseURL(r)
	entries := []map[string]any{}
	for _, p := range pkgs {
		entries = append(entries, map[string]any{
			"name":    full,
			"version": p.Version,
			"dist":    composerDist{Type: "zip", URL: fmt.Sprintf("%s/api/packages/composer/%s/download/%s/%s/%s", base, owner, full, p.Version, p.Filename)},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"minified": "composer/2.0", "packages": []any{entries}})
}

func (a *API) composerDownload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	vendor := r.PathValue("vendor")
	name := r.PathValue("name")
	version := r.PathValue("version")
	filename := r.PathValue("filename")
	p, content, err := a.store.GetPackageFile(owner, "composer", vendor+"/"+name, version, filename)
	if err != nil {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	servePackageBytes(w, p, content, "application/zip")
}

// ---- maven ----

// mavenQualOrder maven 版本修饰符排序（越小越旧；release 空串比一切修饰符新）。
// 参考 Maven ComparableVersion：alpha < beta < milestone < rc = cr < snapshot < release < sp。
var mavenQualOrder = map[string]int{
	"alpha": 1, "a": 1, "beta": 2, "b": 2, "milestone": 3, "m": 3,
	"rc": 4, "cr": 4, "snapshot": 5, "": 6, "final": 6, "ga": 6, "sp": 7,
}

// mavenVersionTokens 把版本串按 ./- 切成段（小写；连续分隔符产生空段=release）。
func mavenVersionTokens(v string) []string {
	return strings.FieldsFunc(strings.ToLower(v), func(r rune) bool { return r == '.' || r == '-' })
}

// mavenTokenCmp 比较单个版本段：数字段 > 修饰符段；数字按数值；修饰符按 maven 次序。
func mavenTokenCmp(x, y string) int {
	xn, xe := strconv.Atoi(x)
	yn, ye := strconv.Atoi(y)
	switch {
	case xe == nil && ye == nil:
		return xn - yn
	case xe == nil: // 数字（release 级别）大于修饰符
		return 1
	case ye == nil:
		return -1
	}
	xo, yo := mavenQualOrder[x], mavenQualOrder[y]
	if xo != yo {
		return xo - yo
	}
	return strings.Compare(x, y)
}

// mavenVersionCmp 语义化比较 maven 版本（major.minor.patch + 修饰符）。
func mavenVersionCmp(a, b string) int {
	as, bs := mavenVersionTokens(a), mavenVersionTokens(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		x, y := "", ""
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if c := mavenTokenCmp(x, y); c != 0 {
			return c
		}
	}
	return 0
}

func mavenSortVersions(versions []string) {
	sort.Slice(versions, func(i, j int) bool { return mavenVersionCmp(versions[i], versions[j]) < 0 })
}

// mavenUpload maven/gradle 制品上传（路径式 {group}/{artifact}/{version}/{filename}）
//
//	@Summary     maven 上传
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "{group}/{artifact}/{version}/{filename}"
//	@Success     201
//	@Router      /packages/maven/{owner}/{rest} [put]
func (a *API) mavenUpload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	rest := strings.TrimPrefix(r.URL.Path, "/api/packages/maven/"+owner+"/")
	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) < 2 {
		writeCode(w, http.StatusBadRequest, "invalid_path", "expected {group...}/{artifact}/{version}/{filename}")
		return
	}
	body, ok := limitBody(w, r)
	if !ok {
		return
	}
	filename := segments[len(segments)-1]
	if len(segments) == 2 {
		// 元数据文件（maven-metadata.xml 等）挂在 artifact 层
		a.savePackage(w, r, owner, "maven", segments[0], "_", filename, body)
		return
	}
	version := segments[len(segments)-2]
	name := strings.Join(segments[:len(segments)-2], "/")
	a.savePackage(w, r, owner, "maven", name, version, filename, body)
}

// mavenGet maven/gradle 制品下载（同上传路径；自动生成 maven-metadata.xml 与 .sha1/.md5）
//
//	@Summary     maven 下载
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       rest  path string true "{group}/{artifact}/{version}/{filename}"
//	@Router      /packages/maven/{owner}/{rest} [get]
func (a *API) mavenGet(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	rest := strings.TrimPrefix(r.URL.Path, "/api/packages/maven/"+owner+"/")
	segments := strings.Split(strings.Trim(rest, "/"), "/")
	if len(segments) < 2 {
		writeCode(w, http.StatusBadRequest, "invalid_path", "path too short")
		return
	}
	filename := segments[len(segments)-1]

	// .sha1 / .md5 校验文件：对底层文件生成
	if strings.HasSuffix(filename, ".sha1") || strings.HasSuffix(filename, ".md5") {
		base := strings.TrimSuffix(filename, path.Ext(filename))
		content, ok := a.mavenLookup(owner, append(append([]string{}, segments[:len(segments)-1]...), base))
		if !ok {
			writeCode(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		var out string
		if strings.HasSuffix(filename, ".sha1") {
			s := sha1.Sum(content)
			out = hex.EncodeToString(s[:])
		} else {
			s := md5.Sum(content)
			out = hex.EncodeToString(s[:])
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(out))
		return
	}

	content, ok := a.mavenLookup(owner, segments)
	if !ok && len(segments) >= 3 {
		// SNAPSHOT 目录兜底：请求非时间戳文件名时解析为最新时间戳制品
		if p, c, resolved := a.mavenSnapshotResolve(owner, segments); resolved {
			content, ok = c, true
			w.Header().Set("X-Checksum-Sha256", p.Checksum)
		}
	}
	if !ok {
		writeCode(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	ct := "application/octet-stream"
	switch {
	case strings.HasSuffix(filename, ".pom"), strings.HasSuffix(filename, ".xml"):
		ct = "text/xml"
	case strings.HasSuffix(filename, ".jar"):
		ct = "application/java-archive"
	}
	w.Header().Set("Content-Type", ct)
	_, _ = w.Write(content)
}

// mavenLookup 按路径段查找 maven 制品；未命中时自动生成 maven-metadata.xml。
func (a *API) mavenLookup(owner string, segments []string) ([]byte, bool) {
	filename := segments[len(segments)-1]
	var content []byte
	var err error
	if len(segments) >= 3 {
		name := strings.Join(segments[:len(segments)-2], "/")
		_, content, err = a.store.GetPackageFile(owner, "maven", name, segments[len(segments)-2], filename)
	} else {
		err = store.ErrNotFound
	}
	if err != nil {
		name := strings.Join(segments[:len(segments)-1], "/")
		_, content, err = a.store.GetPackageFile(owner, "maven", name, "_", filename)
		if err != nil {
			if filename == "maven-metadata.xml" {
				if len(segments) >= 3 {
					// 版本级 metadata：SNAPSHOT 目录未显式上传时自动生成
					if xml, ok := a.mavenSnapshotMetadata(owner, segments); ok {
						return []byte(xml), true
					}
				}
				if len(segments) >= 2 {
					if xml, ok := a.mavenAutoMetadata(owner, segments); ok {
						return []byte(xml), true
					}
				}
			}
			return nil, false
		}
	}
	return content, true
}

// mavenAutoMetadata 自动生成 artifact 级 maven-metadata.xml（版本列表来自已上传制品）。
func (a *API) mavenAutoMetadata(owner string, segments []string) (string, bool) {
	// 路径形如 {group...}/{artifact}/maven-metadata.xml → 版本挂在 {group}/{artifact} 下
	name := strings.Join(segments[:len(segments)-1], "/")
	pkgs, err := a.store.ListPackageVersions(owner, "maven", name)
	if err != nil {
		return "", false
	}
	seen := map[string]bool{}
	var versions []string
	for _, p := range pkgs {
		if p.Version != "_" && !seen[p.Version] {
			seen[p.Version] = true
			versions = append(versions, p.Version)
		}
	}
	if len(versions) == 0 {
		return "", false
	}
	mavenSortVersions(versions)
	latest := versions[len(versions)-1]
	// release 取最新非 SNAPSHOT 版本（无则退回 latest）
	release := ""
	for _, v := range versions {
		if !strings.HasSuffix(v, "-SNAPSHOT") {
			if release == "" || mavenVersionCmp(v, release) > 0 {
				release = v
			}
		}
	}
	if release == "" {
		release = latest
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "<metadata>\n  <groupId>%s</groupId>\n  <artifactId>%s</artifactId>\n",
		escapeXML(mavenGroupOf(name)), escapeXML(segments[len(segments)-2]))
	sb.WriteString("  <versioning>\n")
	fmt.Fprintf(&sb, "    <latest>%s</latest>\n    <release>%s</release>\n", latest, release)
	sb.WriteString("    <versions>\n")
	for _, v := range versions {
		fmt.Fprintf(&sb, "      <version>%s</version>\n", v)
	}
	sb.WriteString("    </versions>\n  </versioning>\n</metadata>\n")
	return sb.String(), true
}

// mavenGroupOf maven 包名（group/path/artifact）转 groupId（点分）。
func mavenGroupOf(name string) string {
	return strings.ReplaceAll(path.Dir(name), "/", ".")
}

var mavenSnapshotFileRe = regexp.MustCompile(`^(\d{8}\.\d{6})-(\d+)\.([^.]+)$`)

// mavenSnapshotMetadata 为 {group}/{artifact}/{version-SNAPSHOT}/maven-metadata.xml 自动生成
// 版本级快照元数据（timestamp/buildNumber/snapshotVersions），依据已上传的时间戳文件名。
func (a *API) mavenSnapshotMetadata(owner string, segments []string) (string, bool) {
	name := strings.Join(segments[:len(segments)-2], "/")
	ver := segments[len(segments)-2]
	if !strings.HasSuffix(ver, "-SNAPSHOT") {
		return "", false
	}
	base := strings.TrimSuffix(ver, "-SNAPSHOT")
	artifact := path.Base(name)
	prefix := artifact + "-" + base + "-"
	pkgs, err := a.store.ListPackageVersions(owner, "maven", name)
	if err != nil {
		return "", false
	}
	type snapFile struct {
		ts, build, ext string
	}
	var files []snapFile
	latestUpdated := ""
	for _, p := range pkgs {
		if p.Version != ver || !strings.HasPrefix(p.Filename, prefix) {
			continue
		}
		if m := mavenSnapshotFileRe.FindStringSubmatch(strings.TrimPrefix(p.Filename, prefix)); m != nil {
			files = append(files, snapFile{ts: m[1], build: m[2], ext: m[3]})
			if u := strings.ReplaceAll(m[1]+m[2], ".", ""); u > latestUpdated {
				latestUpdated = u
			}
		}
	}
	if len(files) == 0 {
		return "", false
	}
	// 取最大 timestamp+buildNumber
	best := files[0]
	for _, f := range files[1:] {
		if f.ts > best.ts || (f.ts == best.ts && f.build > best.build) {
			best = f
		}
	}
	updated := strings.ReplaceAll(best.ts, ".", "")
	var sb strings.Builder
	fmt.Fprintf(&sb, "<metadata>\n  <groupId>%s</groupId>\n  <artifactId>%s</artifactId>\n  <version>%s</version>\n",
		escapeXML(mavenGroupOf(name)), escapeXML(artifact), escapeXML(ver))
	sb.WriteString("  <versioning>\n")
	fmt.Fprintf(&sb, "    <snapshot>\n      <timestamp>%s</timestamp>\n      <buildNumber>%s</buildNumber>\n    </snapshot>\n", best.ts, best.build)
	fmt.Fprintf(&sb, "    <lastUpdated>%s</lastUpdated>\n", updated)
	sb.WriteString("    <snapshotVersions>\n")
	for _, f := range files {
		v := base + "-" + f.ts + "-" + f.build
		fmt.Fprintf(&sb, "      <snapshotVersion>\n        <extension>%s</extension>\n        <value>%s</value>\n        <updated>%s</updated>\n      </snapshotVersion>\n",
			escapeXML(f.ext), escapeXML(v), strings.ReplaceAll(f.ts, ".", ""))
	}
	sb.WriteString("    </snapshotVersions>\n  </versioning>\n</metadata>\n")
	return sb.String(), true
}

// mavenSnapshotResolve SNAPSHOT 目录下按请求文件名模糊解析：Maven 客户端经版本级
// metadata 取时间戳文件名；这里额外兜底 `artifact-1.0-SNAPSHOT.<ext>` 与任意时间戳变体。
func (a *API) mavenSnapshotResolve(owner string, segments []string) (store.Package, []byte, bool) {
	name := strings.Join(segments[:len(segments)-2], "/")
	ver := segments[len(segments)-2]
	filename := segments[len(segments)-1]
	if !strings.HasSuffix(ver, "-SNAPSHOT") {
		return store.Package{}, nil, false
	}
	ext := path.Ext(filename)
	prefix := path.Base(name) + "-" + strings.TrimSuffix(ver, "-SNAPSHOT") + "-"
	pkgs, err := a.store.ListPackageVersions(owner, "maven", name)
	if err != nil {
		return store.Package{}, nil, false
	}
	var best *store.Package
	var bestKey string
	for i := range pkgs {
		p := pkgs[i]
		if p.Version != ver || !strings.HasPrefix(p.Filename, prefix) || path.Ext(p.Filename) != ext {
			continue
		}
		key := ""
		if m := mavenSnapshotFileRe.FindStringSubmatch(strings.TrimPrefix(p.Filename, prefix)); m != nil {
			key = m[1] + "-" + fmt.Sprintf("%06s", m[2])
		}
		if best == nil || key >= bestKey {
			best, bestKey = &pkgs[i], key
		}
	}
	if best == nil {
		return store.Package{}, nil, false
	}
	p, content, err := a.store.GetPackageFile(owner, "maven", name, ver, best.Filename)
	if err != nil {
		return store.Package{}, nil, false
	}
	return p, content, true
}

// escapeXML 最小 XML 转义。
func escapeXML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}
