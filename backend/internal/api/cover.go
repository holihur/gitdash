package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"gitdash/backend/internal/store"
)

// maxCoverBytes 封面大小上限（5MB）。
const maxCoverBytes = 5 << 20

// userCoverURL 返回用户封面地址（未设置封面时为空串）。
func (a *API) userCoverURL(username string) string {
	if a.store.HasUserCover(username) {
		return "/api/users/" + username + "/cover"
	}
	return ""
}

// orgCoverURL 返回组织封面地址（未设置封面时为空串）。
func (a *API) orgCoverURL(org string) string {
	if a.store.HasOrgCover(org) {
		return "/api/orgs/" + org + "/cover"
	}
	return ""
}

// readCoverUpload 解析 multipart 图片字段并做大小 / 类型校验（不信任客户端声明）。
func readCoverUpload(w http.ResponseWriter, r *http.Request, field string) (string, []byte, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCoverBytes+(1<<20))
	if err := r.ParseMultipartForm(maxCoverBytes + (1 << 20)); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_multipart", "invalid multipart form: "+err.Error())
		return "", nil, false
	}
	file, _, err := r.FormFile(field)
	if err != nil {
		writeCode(w, http.StatusBadRequest, "file_required", "multipart field '"+field+"' is required")
		return "", nil, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxCoverBytes+1))
	if err != nil || len(data) > maxCoverBytes {
		writeCode(w, http.StatusBadRequest, "cover_too_large", "cover too large (max 5MB)")
		return "", nil, false
	}
	if len(data) == 0 {
		writeCode(w, http.StatusBadRequest, "cover_empty", "empty cover file")
		return "", nil, false
	}
	ct := http.DetectContentType(data)
	if !allowedAvatarTypes[ct] {
		writeCode(w, http.StatusBadRequest, "cover_type_invalid", "unsupported image type (png/jpeg/gif/webp only)")
		return "", nil, false
	}
	return ct, data, true
}

// writeCoverImage 输出封面图片内容。
func writeCoverImage(w http.ResponseWriter, ct string, data []byte) {
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// getUserCover 返回用户封面图片。
//
//	@Summary     用户封面
//	@Description 返回用户主页顶部横幅图片；未设置封面返回 404。
//	@Tags        users
//	@Produce     image/png
//	@Param       username path string true "用户名"
//	@Success     200 {file} binary
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{username}/cover [get]
func (a *API) getUserCover(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	cv, err := a.store.GetUserCover(username)
	if err != nil {
		writeNotFound(w, "cover")
		return
	}
	writeCoverImage(w, cv.ContentType, cv.Data)
}

// uploadUserCover 上传（或替换）当前用户封面（multipart 字段 cover）。
//
//	@Summary     上传用户封面
//	@Description multipart 字段 cover；支持 png/jpeg/gif/webp，最大 5MB。
//	@Tags        users
//	@Accept      multipart/form-data
//	@Produce     json
//	@Param       cover formData file true "封面图片"
//	@Success     200 {object} map[string]string
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /me/cover [post]
func (a *API) uploadUserCover(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	ct, data, ok := readCoverUpload(w, r, "cover")
	if !ok {
		return
	}
	if err := a.store.SetUserCover(me, ct, data); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"cover_url": "/api/users/" + me + "/cover"})
}

// deleteUserCover 删除当前用户封面。
//
//	@Summary     删除用户封面
//	@Tags        users
//	@Produce     json
//	@Success     200 {object} map[string]bool
//	@Security    BearerAuth
//	@Router      /me/cover [delete]
func (a *API) deleteUserCover(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if err := a.store.DeleteUserCover(me); err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// getOrgCover 返回组织封面图片。
//
//	@Summary     组织封面
//	@Description 返回组织主页顶部横幅图片；未设置封面返回 404。
//	@Tags        orgs
//	@Produce     image/png
//	@Param       org path string true "组织名"
//	@Success     200 {file} binary
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/cover [get]
func (a *API) getOrgCover(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	cv, err := a.store.GetOrgCover(org)
	if err != nil {
		writeNotFound(w, "cover")
		return
	}
	writeCoverImage(w, cv.ContentType, cv.Data)
}

// uploadOrgCover 上传（或替换）组织封面（仅 owner，multipart 字段 cover）。
//
//	@Summary     上传组织封面
//	@Description multipart 字段 cover；支持 png/jpeg/gif/webp，最大 5MB；仅组织 owner。
//	@Tags        orgs
//	@Accept      multipart/form-data
//	@Produce     json
//	@Param       org   path     string true "组织名"
//	@Param       cover formData file   true "封面图片"
//	@Success     200 {object} map[string]string
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/cover [post]
func (a *API) uploadOrgCover(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	ct, data, ok := readCoverUpload(w, r, "cover")
	if !ok {
		return
	}
	if err := a.store.SetOrgCover(org, ct, data); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"cover_url": "/api/orgs/" + org + "/cover"})
}

// deleteOrgCover 删除组织封面（仅 owner）。
//
//	@Summary     删除组织封面
//	@Tags        orgs
//	@Produce     json
//	@Param       org path string true "组织名"
//	@Success     200 {object} map[string]bool
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /orgs/{org}/cover [delete]
func (a *API) deleteOrgCover(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	if err := a.store.DeleteOrgCover(org); err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
