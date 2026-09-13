package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"

	"gitdash/backend/internal/store"
)

// maxAvatarBytes 头像大小上限（2MB）。
const maxAvatarBytes = 2 << 20

// allowedAvatarTypes 允许的图片类型（由内容嗅探得出，不信任客户端声明）。
var allowedAvatarTypes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/gif":  true,
	"image/webp": true,
}

// avatarURL 返回用户头像地址（未设置头像时为空串）。
func (a *API) avatarURL(username string) string {
	if a.store.HasUserAvatar(username) {
		return "/api/users/" + username + "/avatar"
	}
	return ""
}

// getAvatar 返回用户头像图片。
//
//	@Summary     用户头像
//	@Description 返回用户头像图片；未设置头像返回 404。
//	@Tags        users
//	@Produce     image/png
//	@Param       username path string true "用户名"
//	@Success     200 {file} binary
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /users/{username}/avatar [get]
func (a *API) getAvatar(w http.ResponseWriter, r *http.Request) {
	username := r.PathValue("username")
	av, err := a.store.GetUserAvatar(username)
	if err != nil {
		writeNotFound(w, "avatar")
		return
	}
	ct := av.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "private, no-cache")
	w.Header().Set("Content-Length", strconv.Itoa(len(av.Data)))
	_, _ = w.Write(av.Data)
}

// uploadAvatar 上传（或替换）当前用户头像（multipart 字段 avatar）。
//
//	@Summary     上传头像
//	@Description multipart 字段 avatar；支持 png/jpeg/gif/webp，最大 2MB。
//	@Tags        users
//	@Accept      multipart/form-data
//	@Produce     json
//	@Param       avatar formData file true "头像图片"
//	@Success     200 {object} map[string]string
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /me/avatar [post]
func (a *API) uploadAvatar(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	r.Body = http.MaxBytesReader(w, r.Body, maxAvatarBytes+(1<<20))
	if err := r.ParseMultipartForm(maxAvatarBytes + (1 << 20)); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_multipart", "invalid multipart form: "+err.Error())
		return
	}
	file, _, err := r.FormFile("avatar")
	if err != nil {
		writeCode(w, http.StatusBadRequest, "file_required", "multipart field 'avatar' is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxAvatarBytes+1))
	if err != nil || len(data) > maxAvatarBytes {
		writeCode(w, http.StatusBadRequest, "avatar_too_large", "avatar too large (max 2MB)")
		return
	}
	if len(data) == 0 {
		writeCode(w, http.StatusBadRequest, "avatar_empty", "empty avatar file")
		return
	}
	ct := http.DetectContentType(data)
	if !allowedAvatarTypes[ct] {
		writeCode(w, http.StatusBadRequest, "avatar_type_invalid", "unsupported image type (png/jpeg/gif/webp only)")
		return
	}
	if err := a.store.SetUserAvatar(me, ct, data); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"avatar_url": "/api/users/" + me + "/avatar"})
}

// deleteAvatar 删除当前用户头像。
//
//	@Summary     删除头像
//	@Tags        users
//	@Produce     json
//	@Success     200 {object} map[string]bool
//	@Security    BearerAuth
//	@Router      /me/avatar [delete]
func (a *API) deleteAvatar(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if err := a.store.DeleteUserAvatar(me); err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
