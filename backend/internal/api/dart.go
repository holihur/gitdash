package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"unicode/utf8"

	"gitdash/backend/internal/store"

	"sigs.k8s.io/yaml"
)

// ---- Dart / Pub hosted package repository（type "dart"）----
//
// 实现 pub 的 hosted API，使原生客户端可直接使用：
//
//	# 发布（需要 repo scope 的 PAT 作为 pub token）
//	dart pub publish --server http://<host>/api/packages/dart/<owner>
//
//	# 依赖（pubspec.yaml）
//	dependencies:
//	  hello:
//	    hosted:
//	      name: hello
//	      url: http://<host>/api/packages/dart/<owner>
//	    version: ^1.0.0
//
// pub 协议端点：
//
//	GET  <server>/api/packages/versions/new                 -> {url, fields}
//	POST <server>/api/packages/versions/newUpload            -> multipart archive
//	GET  <server>/api/packages/{name}                        -> 包与版本列表
//	GET  <server>/api/packages/{name}/versions/{version}     -> 单版本
//	GET  <server>/api/packages/{name}/download/{version}     -> 归档下载
//
// 另提供手工 multipart 发布端点 POST <server>/publish。

// dartPkgMeta 存 packages.meta（名称 / 版本 / 描述 + 原始 pubspec，供客户端解析依赖）。
type dartPkgMeta struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description,omitempty"`
	PubspecYAML string `json:"pubspec_yaml,omitempty"`
}

func decodeDartMeta(raw string) dartPkgMeta {
	var m dartPkgMeta
	if raw != "" {
		_ = json.Unmarshal([]byte(raw), &m)
	}
	return m
}

// archiveFileByName 在 tar / tar.gz / zip 归档中按 basename 查找文件内容。
func archiveFileByName(data []byte, kind, base string, max int64) ([]byte, bool) {
	entries, err := listArchiveEntries(kind, data)
	if err != nil {
		return nil, false
	}
	for _, e := range entries {
		if e.IsDir || path.Base(e.Name) != base {
			continue
		}
		b, _, found := readArchiveEntry(kind, data, e.Name, max)
		if found {
			return b, true
		}
	}
	return nil, false
}

// pubParseArchive 从 pub 归档（tar.gz/tar）中解析 pubspec.yaml 的 name / version / description。
func pubParseArchive(data []byte) (name, version, description, pubspecYAML string, ok bool) {
	raw, found := archiveFileByName(data, "tar.gz", "pubspec.yaml", int64(maxPackageSize))
	if !found {
		raw, found = archiveFileByName(data, "tar", "pubspec.yaml", int64(maxPackageSize))
	}
	if !found {
		return "", "", "", "", false
	}
	var spec map[string]any
	if err := yaml.Unmarshal(raw, &spec); err != nil {
		return "", "", "", "", false
	}
	name = scalarString(spec["name"])
	version = scalarString(spec["version"])
	description = scalarString(spec["description"])
	if name == "" || version == "" {
		return "", "", "", "", false
	}
	return name, version, description, string(raw), true
}

func scalarString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func clipString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// dartStore 校验归档并落库；返回 (pkg, 错误码, error)。
func (a *API) dartStore(owner, user, repo, filename string, data []byte) (*store.Package, string, error) {
	name, version, description, pubspecYAML, ok := pubParseArchive(data)
	if !ok {
		return nil, "invalid_package", errors.New("archive is missing a valid pubspec.yaml with name and version")
	}
	if filename == "" || filename == "." || filename == "/" {
		filename = fmt.Sprintf("%s-%s.tar.gz", name, version)
	}
	meta := dartPkgMeta{
		Name:        name,
		Version:     version,
		Description: description,
		PubspecYAML: clipString(pubspecYAML, 6000),
	}
	metaRaw, _ := json.Marshal(meta)
	p := &store.Package{
		Owner: owner, Repo: repo, Type: "dart", Name: name, Version: version,
		Filename: filename, Uploader: user, Meta: string(metaRaw),
	}
	if err := a.store.CreatePackage(p, data); err != nil {
		if errors.Is(err, store.ErrExists) {
			return nil, "package_exists", err
		}
		return nil, "internal", err
	}
	_ = a.store.AddPackageAudit(owner, "dart", name, version, "publish", user)
	return p, "", nil
}

var (
	errPkgBadMultipart = errors.New("invalid multipart form")
	errPkgNoFile       = errors.New("multipart field 'file' is required")
	errPkgTooLarge     = errors.New("package exceeds 64MB")
)

// parseMultipartPackage 解析 multipart 中的 file 字段（限制 64MB）。
func parseMultipartPackage(w http.ResponseWriter, r *http.Request) ([]byte, string, error) {
	r.Body = http.MaxBytesReader(w, r.Body, maxPackageSize+(4<<20))
	if err := r.ParseMultipartForm(maxPackageSize + (4 << 20)); err != nil {
		return nil, "", errPkgBadMultipart
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		return nil, "", errPkgNoFile
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxPackageSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxPackageSize {
		return nil, "", errPkgTooLarge
	}
	return data, path.Base(hdr.Filename), nil
}

// dartPublish 手工发布（multipart: file=<archive>.tar.gz，命名空间 owner）。
//
//	@Summary     手工发布 Dart 包
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       file  formData file true "pub 归档 (.tar.gz)"
//	@Success     201 {object} store.Package
//	@Security    BearerAuth
//	@Router      /packages/dart/{owner}/publish [post]
func (a *API) dartPublish(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		pkgForbidden(w)
		return
	}
	data, filename, err := parseMultipartPackage(w, r)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_upload", err.Error())
		return
	}
	p, code, err := a.dartStore(owner, user, strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"), filename, data)
	if err != nil {
		switch code {
		case "package_exists":
			writeCode(w, http.StatusConflict, "package_exists", "package version already exists")
		case "invalid_package":
			writeCode(w, http.StatusBadRequest, "invalid_package", err.Error())
		default:
			internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func dartError(message string) map[string]any {
	return map[string]any{"error": map[string]string{"message": message}}
}

// dartNewUpload pub 发布第一步：返回上传地址与表单字段。
//
//	@Summary     Pub 发布上传地址
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Success     200 {object} map[string]any
//	@Security    BearerAuth
//	@Router      /packages/dart/{owner}/api/packages/versions/new [get]
func (a *API) dartNewUpload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	if !a.canPublishPackage(owner, pkgUser(r)) {
		writeJSON(w, http.StatusForbidden, dartError("forbidden"))
		return
	}
	base := externalBaseURL(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"url":    base + "/api/packages/dart/" + url.PathEscape(owner) + "/api/packages/versions/newUpload",
		"fields": map[string]string{},
	})
}

// dartNewUploadUpload pub 发布第二步：接收归档并落库。
//
//	@Summary     Pub 发布上传
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       file  formData file true "pub 归档 (.tar.gz)"
//	@Success     200 {object} map[string]any
//	@Security    BearerAuth
//	@Router      /packages/dart/{owner}/api/packages/versions/newUpload [post]
func (a *API) dartNewUploadUpload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	user := pkgUser(r)
	if !a.canPublishPackage(owner, user) {
		writeJSON(w, http.StatusForbidden, dartError("forbidden"))
		return
	}
	data, filename, err := parseMultipartPackage(w, r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, dartError(err.Error()))
		return
	}
	_, code, err := a.dartStore(owner, user, strings.TrimPrefix(r.Header.Get("X-Gitdash-Repo"), "/"), filename, data)
	if err != nil {
		switch code {
		case "package_exists":
			writeJSON(w, http.StatusConflict, dartError("package version already exists"))
		case "invalid_package":
			writeJSON(w, http.StatusBadRequest, dartError(err.Error()))
		default:
			internalError(w, err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": map[string]string{"message": "Successfully uploaded package."}})
}

// dartPubspecJSON 把存储的 pubspec.yaml 转成 JSON；缺失时回落到最小 pubspec。
func dartPubspecJSON(m dartPkgMeta) json.RawMessage {
	if m.PubspecYAML != "" {
		if b, err := yaml.YAMLToJSON([]byte(m.PubspecYAML)); err == nil && len(b) > 0 && string(b) != "null" {
			return b
		}
	}
	b, _ := json.Marshal(map[string]string{"name": m.Name, "version": m.Version})
	return b
}

func dartVersionObject(base, owner, name string, p store.Package) map[string]any {
	m := decodeDartMeta(p.Meta)
	v := map[string]any{
		"version": p.Version,
		"pubspec": dartPubspecJSON(m),
		"archive_url": base + "/api/packages/dart/" + url.PathEscape(owner) +
			"/api/packages/" + url.PathEscape(name) + "/download/" + url.PathEscape(p.Version),
	}
	if p.Checksum != "" {
		v["archive_sha256"] = p.Checksum
	}
	if p.CreatedAt != "" {
		v["published"] = p.CreatedAt
	}
	return v
}

// dartPackage 返回包信息与全部有效版本（pub hosted API）。
//
//	@Summary     Pub 包信息
//	@Tags        packages
//	@Param       owner path string true "用户或组织"
//	@Param       name  path string true "包名"
//	@Success     200 {object} map[string]any
//	@Router      /packages/dart/{owner}/api/packages/{name} [get]
func (a *API) dartPackage(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	if !a.canReadPackage(owner, "dart", name, pkgUser(r)) {
		writeNotFound(w, "package")
		return
	}
	pkgs, err := a.store.ListPackageVersions(owner, "dart", name)
	if err != nil || len(pkgs) == 0 {
		writeNotFound(w, "package")
		return
	}
	base := externalBaseURL(r)
	versions := make([]map[string]any, 0, len(pkgs))
	for _, p := range pkgs {
		if p.Yanked {
			continue
		}
		versions = append(versions, dartVersionObject(base, owner, name, p))
	}
	if len(versions) == 0 {
		writeNotFound(w, "package")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":     name,
		"latest":   versions[0],
		"versions": versions,
	})
}

// dartVersion 返回单个版本。
//
//	@Summary     Pub 单版本信息
//	@Tags        packages
//	@Param       owner   path string true "用户或组织"
//	@Param       name    path string true "包名"
//	@Param       version path string true "版本"
//	@Success     200 {object} map[string]any
//	@Router      /packages/dart/{owner}/api/packages/{name}/versions/{version} [get]
func (a *API) dartVersion(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	version := r.PathValue("version")
	if !a.canReadPackage(owner, "dart", name, pkgUser(r)) {
		writeNotFound(w, "version")
		return
	}
	p, _, err := a.store.GetPackageVersion(owner, "dart", name, version)
	if err != nil {
		writeNotFound(w, "version")
		return
	}
	writeJSON(w, http.StatusOK, dartVersionObject(externalBaseURL(r), owner, name, p))
}

// dartDownload 下载某个版本的归档。
//
//	@Summary     下载 Dart 包
//	@Tags        packages
//	@Param       owner   path string true "用户或组织"
//	@Param       name    path string true "包名"
//	@Param       version path string true "版本"
//	@Success     200 {file} file
//	@Router      /packages/dart/{owner}/api/packages/{name}/download/{version} [get]
func (a *API) dartDownload(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	name := r.PathValue("name")
	version := r.PathValue("version")
	if !a.canReadPackage(owner, "dart", name, pkgUser(r)) {
		writeNotFound(w, "version")
		return
	}
	p, _, err := a.store.GetPackageVersion(owner, "dart", name, version)
	if err != nil {
		writeNotFound(w, "version")
		return
	}
	_, content, err := a.store.GetPackageFile(owner, "dart", name, version, p.Filename)
	if err != nil {
		writeNotFound(w, "version")
		return
	}
	servePackageBytes(w, p, content, "application/gzip")
}
