package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"gitdash/backend/internal/store"

	"sigs.k8s.io/yaml"
)

// ---- Kubernetes / Helm chart repository（type "k8s"）----
//
// 一个标准的 Helm HTTP 仓库：上传 <chart>-<version>.tgz，服务端据此生成
// 官方格式的 index.yaml。客户端用法：
//
//	helm repo add <alias> http://<host>/api/packages/k8s/<owner>/<repo>
//	helm repo update
//	helm install <release> <alias>/<chart>
//
// 命名空间（owner）与仓库名（repo）都是路径段，鉴权与其它包类型一致：
// 发布 = owner 本人 / 组织 owner，读取 = 任意已认证用户（按包可见性收严）。

// k8sChartMeta Chart.yaml 中与本仓库索引相关的字段（存 packages.meta JSON）。
type k8sChartMeta struct {
	Name        string   `json:"name,omitempty"`
	Version     string   `json:"version,omitempty"`
	AppVersion  string   `json:"appVersion,omitempty"`
	APIVersion  string   `json:"apiVersion,omitempty"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type,omitempty"`
	Home        string   `json:"home,omitempty"`
	Icon        string   `json:"icon,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
	Deprecated  bool     `json:"deprecated,omitempty"`
}

func decodeK8sMeta(raw string) k8sChartMeta {
	var m k8sChartMeta
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &m)
	}
	return m
}

// mergeK8sMeta 显式提交的 meta 优先，缺省字段回落到 Chart.yaml 解析结果。
func mergeK8sMeta(explicit, fromChart k8sChartMeta) k8sChartMeta {
	out := fromChart
	if explicit.Name != "" {
		out.Name = explicit.Name
	}
	if explicit.Version != "" {
		out.Version = explicit.Version
	}
	if explicit.AppVersion != "" {
		out.AppVersion = explicit.AppVersion
	}
	if explicit.APIVersion != "" {
		out.APIVersion = explicit.APIVersion
	}
	if explicit.Description != "" {
		out.Description = explicit.Description
	}
	if explicit.Type != "" {
		out.Type = explicit.Type
	}
	if explicit.Home != "" {
		out.Home = explicit.Home
	}
	if explicit.Icon != "" {
		out.Icon = explicit.Icon
	}
	if len(explicit.Keywords) > 0 {
		out.Keywords = explicit.Keywords
	}
	if explicit.Deprecated {
		out.Deprecated = true
	}
	return out
}

// k8sPublish 发布一个 Helm chart（multipart: file=<name>-<version>.tgz，meta 可选 JSON）。
//
//	@Summary     发布 Helm chart
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       repo  path string true "Helm 仓库名"
//	@Param       file  formData file true "chart 归档 (.tgz)"
//	@Param       meta  formData string false "chart 元数据 JSON"
//	@Success     201 {object} store.Package
//	@Security    BearerAuth
//	@Router      /packages/k8s/{owner}/{repo}/publish [post]
func (a *API) k8sPublish(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageSize+(4<<20))
	if err := r.ParseMultipartForm(maxPackageSize + (4 << 20)); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_multipart", "invalid multipart form")
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		writeCode(w, http.StatusBadRequest, "file_required", "multipart field 'file' is required")
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(io.LimitReader(file, maxPackageSize+1))
	if err != nil {
		internalError(w, err)
		return
	}
	if len(data) > maxPackageSize {
		writeCode(w, http.StatusRequestEntityTooLarge, "package_too_large", "package exceeds 64MB")
		return
	}

	var meta k8sChartMeta
	if raw := r.FormValue("meta"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &meta)
	}
	// Chart.yaml 位于归档顶层 <chart>/Chart.yaml；解析出来补全缺失字段。
	if chartYAML, ok := archiveFileByName(data, "tar.gz", "Chart.yaml", int64(maxPackageSize)); ok {
		var fromChart k8sChartMeta
		if err := yaml.Unmarshal(chartYAML, &fromChart); err == nil {
			meta = mergeK8sMeta(meta, fromChart)
		}
	}
	if meta.Name == "" {
		meta.Name = r.FormValue("name")
	}
	if meta.Version == "" {
		meta.Version = r.FormValue("version")
	}
	if meta.Name == "" || meta.Version == "" {
		writeCode(w, http.StatusBadRequest, "metadata_required", "chart name and version are required (from Chart.yaml or meta)")
		return
	}

	filename := path.Base(hdr.Filename)
	if filename == "" || filename == "." || filename == "/" {
		filename = fmt.Sprintf("%s-%s.tgz", meta.Name, meta.Version)
	}
	metaRaw, _ := json.Marshal(meta)
	p := &store.Package{
		Owner: owner, Repo: strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"),
		Type: "k8s", Name: repo, Version: meta.Version, Filename: filename, Uploader: user, Meta: string(metaRaw),
	}
	if err := a.store.CreatePackage(p, data); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "package_exists", "chart version already exists")
			return
		}
		internalError(w, err)
		return
	}
	_ = a.store.AddPackageAudit(owner, "k8s", repo, meta.Version, "publish", user)
	writeJSON(w, http.StatusCreated, p)
}

// k8sIndex 生成并返回仓库的 index.yaml（Helm 仓库索引）。
//
//	@Summary     获取 Helm 仓库索引
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       repo  path string true "Helm 仓库名"
//	@Produce     text/yaml
//	@Success     200 {string} string "index.yaml"
//	@Router      /packages/k8s/{owner}/{repo}/index.yaml [get]
func (a *API) k8sIndex(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	if !a.canReadPackage(owner, "k8s", repo, pkgUser(r)) {
		writeNotFound(w, "repository")
		return
	}
	pkgs, err := a.store.ListPackageRepoFiles(owner, "k8s", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	base := externalBaseURL(r)
	entries := map[string][]map[string]any{}
	for _, p := range pkgs {
		m := decodeK8sMeta(p.Meta)
		name := valueOr(m.Name, p.Filename)
		entry := map[string]any{
			"apiVersion": valueOr(m.APIVersion, "v2"),
			"name":       name,
			"version":    p.Version,
			"created":    p.CreatedAt,
			"digest":     "sha256:" + p.Checksum,
			"urls": []string{
				base + "/api/packages/k8s/" + url.PathEscape(owner) + "/" + url.PathEscape(repo) +
					"/charts/" + url.PathEscape(p.Filename),
			},
		}
		if m.AppVersion != "" {
			entry["appVersion"] = m.AppVersion
		}
		if m.Description != "" {
			entry["description"] = m.Description
		}
		if m.Type != "" {
			entry["type"] = m.Type
		}
		if m.Home != "" {
			entry["home"] = m.Home
		}
		if m.Icon != "" {
			entry["icon"] = m.Icon
		}
		if len(m.Keywords) > 0 {
			entry["keywords"] = m.Keywords
		}
		if m.Deprecated {
			entry["deprecated"] = true
		}
		entries[name] = append(entries[name], entry)
	}
	// 每个 chart 的版本按字符串降序（索引展示用；Helm 客户端自身按 semver 解析）。
	for name := range entries {
		list := entries[name]
		sort.SliceStable(list, func(i, j int) bool {
			return fmt.Sprint(list[i]["version"]) > fmt.Sprint(list[j]["version"])
		})
	}
	doc := map[string]any{
		"apiVersion": "v1",
		"generated":  time.Now().UTC().Format(time.RFC3339),
		"entries":    entries,
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/yaml; charset=utf-8")
	_, _ = w.Write(out)
}

// k8sDownload 下载 chart 包（.tgz）。
//
//	@Summary     下载 Helm chart
//	@Tags        packages
//	@Param       owner    path string true "用户或组织"
//	@Param       repo     path string true "Helm 仓库名"
//	@Param       filename path string true "chart 文件名"
//	@Success     200 {file} file
//	@Router      /packages/k8s/{owner}/{repo}/charts/{filename} [get]
func (a *API) k8sDownload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	if !a.canReadPackage(owner, "k8s", repo, pkgUser(r)) {
		writeNotFound(w, "chart")
		return
	}
	filename := r.PathValue("filename")
	pkgs, err := a.store.ListPackageRepoFiles(owner, "k8s", repo)
	if err != nil {
		internalError(w, err)
		return
	}
	p, found := findRepoFile(pkgs, filename)
	if !found {
		writeNotFound(w, "chart")
		return
	}
	_, content, err := a.store.GetPackageFile(owner, "k8s", repo, p.Version, filename)
	if err != nil {
		writeNotFound(w, "chart")
		return
	}
	servePackageBytes(w, p, content, "application/gzip")
}
