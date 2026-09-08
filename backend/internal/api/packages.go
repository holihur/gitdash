package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
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
		internalError(w, err)
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
		internalError(w, err)
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
		internalError(w, err)
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
