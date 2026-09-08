package api

import (
	"fmt"
	"net/http"
)

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
