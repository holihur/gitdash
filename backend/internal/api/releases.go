package api

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

const (
	maxAssetSize        = 10 << 20 // 10MB
	maxAssetsPerRelease = 10
)

type createReleaseReq struct {
	TagName string `json:"tag_name"`
	Name    string `json:"name"`
	Body    string `json:"body"`
}

// listReleases 列出仓库 release。
//
//	@Summary     列出 release
//	@Tags        repos
//	@Produce     json
//	@Param       owner  path string false "仓库所有者（简写路由时省略）"
//	@Param       name   path string true  "仓库名"
//	@Param       limit  query int false "每页数量（默认 200，最大 500）"
//	@Param       offset query int false "偏移量"
//	@Success     200 {array} store.Release
//	@SuccessHeader X-Total-Count int "release 总数"
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/releases [get]
//	@Router      /users/{owner}/repos/{name}/releases [get]
func (a *API) listReleases(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	limit, offset := pageParams(r)
	rels, total, err := a.store.ListReleases(owner, name, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, rels)
}

// createRelease 依据已有 tag 创建 release。
//
//	@Summary     创建 release
//	@Description tag 必须已存在（不存在返回 400 tag_not_found）；同 tag 重复创建返回 409。
//	@Tags        repos
//	@Accept      json
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       body  body createReleaseReq true "tag_name、name（可选）与 body（可选，markdown）"
//	@Success     201 {object} store.Release
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/releases [post]
//	@Router      /users/{owner}/repos/{name}/releases [post]
func (a *API) createRelease(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	var in createReleaseReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.TagName = strings.TrimSpace(in.TagName)
	if in.TagName == "" {
		writeCode(w, http.StatusBadRequest, "tag_required", "tag_name is required")
		return
	}
	if _, err := gitsvc.RevSHA(owner, name, "refs/tags/"+in.TagName); err != nil {
		writeCode(w, http.StatusBadRequest, "tag_not_found", "tag not found: "+in.TagName)
		return
	}
	rel := store.Release{
		Owner: owner, Repo: name, TagName: in.TagName,
		Name: strings.TrimSpace(in.Name), Body: in.Body, Author: userFrom(r),
	}
	if err := a.store.CreateRelease(&rel); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "release_exists", "release already exists for tag "+in.TagName)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, rel)
}

// getRelease 读取单个 release。
//
//	@Summary     读取 release
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       tag   path string true  "tag 名"
//	@Success     200 {object} store.Release
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/releases/{tag} [get]
//	@Router      /users/{owner}/repos/{name}/releases/{tag} [get]
func (a *API) getRelease(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	rel, err := a.store.GetRelease(owner, name, r.PathValue("tag"))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "release")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, rel)
}

// deleteRelease 删除 release（级联删除附件）。仅仓库所有者可操作。
//
//	@Summary     删除 release
//	@Tags        repos
//	@Param       owner path string false "仓库所有者（简写路由时省略）"
//	@Param       name  path string true  "仓库名"
//	@Param       tag   path string true  "tag 名"
//	@Success     204 {string} string ""
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /repos/{name}/releases/{tag} [delete]
//	@Router      /users/{owner}/repos/{name}/releases/{tag} [delete]
func (a *API) deleteRelease(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	if !a.store.IsRepoOwner(owner, userFrom(r)) { // 与 deleteRepo 一致：仅所有者可删除
		writeNotFound(w, "release")
		return
	}
	tag := r.PathValue("tag")
	if err := a.store.DeleteRelease(owner, name, tag); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "release")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// getReleaseOr404 按路径 tag 解析 release；不存在时写响应。
func (a *API) getReleaseOr404(w http.ResponseWriter, owner, name, tag string) (store.Release, bool) {
	rel, err := a.store.GetRelease(owner, name, tag)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "release")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return store.Release{}, false
	}
	return rel, true
}

// cleanFilename 规范化附件文件名（取基名，禁止空/越界路径）。
func cleanFilename(raw string) (string, bool) {
	name := path.Base(strings.TrimSpace(raw))
	if name == "" || name == "." || name == "/" || len(name) > 255 {
		return "", false
	}
	return name, true
}

// uploadAsset 上传 release 附件（multipart/form-data 字段 file）。
//
//	@Summary     上传 release 附件
//	@Description 单文件最大 10MB，每个 release 最多 10 个附件；同 release 内文件名重复返回 409。
//	@Tags        repos
//	@Accept      multipart/form-data
//	@Produce     json
//	@Param       owner    path string true "仓库所有者"
//	@Param       name     path string true "仓库名"
//	@Param       tag      path string true "tag 名"
//	@Param       file     formData file true "附件文件"
//	@Success     201 {object} store.ReleaseAsset
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/releases/{tag}/assets [post]
func (a *API) uploadAsset(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	rel, ok := a.getReleaseOr404(w, owner, name, r.PathValue("tag"))
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxAssetSize+(1<<20))
	if err := r.ParseMultipartForm(maxAssetSize + (1 << 20)); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_multipart", "invalid multipart form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeCode(w, http.StatusBadRequest, "file_required", "multipart field 'file' is required")
		return
	}
	defer func() { _ = file.Close() }()
	content, err := io.ReadAll(file)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "asset_too_large", "asset too large (max 10MB)")
		return
	}
	if len(content) > maxAssetSize {
		writeCode(w, http.StatusBadRequest, "asset_too_large", "asset too large (max 10MB)")
		return
	}
	filename, ok := cleanFilename(header.Filename)
	if !ok {
		writeCode(w, http.StatusBadRequest, "invalid_filename", "invalid asset filename")
		return
	}
	count, err := a.store.CountAssets(rel.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if count > 0 && count >= maxAssetsPerRelease {
		writeCode(w, http.StatusBadRequest, "too_many_assets", "too many assets (max 10 per release)")
		return
	}
	asset := store.ReleaseAsset{
		Owner: owner, Repo: name, ReleaseID: rel.ID,
		Filename: filename, Size: int64(len(content)), Content: content,
	}
	if err := a.store.AddAsset(&asset); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "asset_exists", "asset already exists: "+filename)
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, asset)
}

// listAssets 列出 release 附件元数据。
//
//	@Summary     列出 release 附件
//	@Tags        repos
//	@Produce     json
//	@Param       owner path string true "仓库所有者"
//	@Param       name  path string true "仓库名"
//	@Param       tag   path string true "tag 名"
//	@Success     200 {array} store.ReleaseAsset
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/releases/{tag}/assets [get]
func (a *API) listAssets(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false)
	if !ok {
		return
	}
	rel, ok := a.getReleaseOr404(w, owner, name, r.PathValue("tag"))
	if !ok {
		return
	}
	assets, err := a.store.ListAssets(owner, name, rel.ID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, assets)
}

// downloadAsset 下载 release 附件。
//
//	@Summary     下载 release 附件
//	@Tags        repos
//	@Produce     octet-stream
//	@Param       owner    path string true "仓库所有者"
//	@Param       name     path string true "仓库名"
//	@Param       tag      path string true "tag 名"
//	@Param       filename path string true "附件文件名"
//	@Success     200 {string} string ""
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/releases/{tag}/assets/{filename} [get]
func (a *API) downloadAsset(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, false) // 公开仓库任意登录用户可读
	if !ok {
		return
	}
	rel, ok := a.getReleaseOr404(w, owner, name, r.PathValue("tag"))
	if !ok {
		return
	}
	filename, ok := cleanFilename(r.PathValue("filename"))
	if !ok {
		writeCode(w, http.StatusBadRequest, "invalid_filename", "invalid asset filename")
		return
	}
	asset, err := a.store.GetAsset(owner, name, rel.ID, filename)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "asset")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(filename, `"`, `_`)+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(asset.Content)))
	_, _ = w.Write(asset.Content)
}

// deleteAsset 删除 release 附件。
//
//	@Summary     删除 release 附件
//	@Tags        repos
//	@Param       owner    path string true "仓库所有者"
//	@Param       name     path string true "仓库名"
//	@Param       tag      path string true "tag 名"
//	@Param       filename path string true "附件文件名"
//	@Success     204 {string} string ""
//	@Failure     404 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{owner}/repos/{name}/releases/{tag}/assets/{filename} [delete]
func (a *API) deleteAsset(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireAccess(w, r, true)
	if !ok {
		return
	}
	rel, ok := a.getReleaseOr404(w, owner, name, r.PathValue("tag"))
	if !ok {
		return
	}
	filename, ok := cleanFilename(r.PathValue("filename"))
	if !ok {
		writeCode(w, http.StatusBadRequest, "invalid_filename", "invalid asset filename")
		return
	}
	err := a.store.DeleteAsset(owner, name, rel.ID, filename)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "asset")
		} else {
			writeErr(w, http.StatusInternalServerError, err.Error())
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
