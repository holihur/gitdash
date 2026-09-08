package api

import (
	"crypto/sha1"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"gitdash/backend/internal/store"
)

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
		internalError(w, err)
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
				internalError(w, err)
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
			internalError(w, err)
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
		internalError(w, err)
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
