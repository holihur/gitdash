package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"gitdash/backend/internal/store"
)

// maxUploadBytes Markdown 上传文件大小上限（10MB）。
const maxUploadBytes = 10 << 20

// uploadAllowedTypes 允许上传并内联展示的类型（由内容嗅探得出，不信任客户端声明）。
// 出于 XSS 考虑不允许 SVG（同源直接打开可执行脚本）。
var uploadAllowedTypes = map[string]bool{
	"image/png":       true,
	"image/jpeg":      true,
	"image/gif":       true,
	"image/webp":      true,
	"application/pdf": true,
	"text/plain":      true,
}

// uploadFile 上传 Markdown 附件 / 图片（multipart 字段 file）。
//
//	@Summary     上传附件
//	@Description multipart 字段 file；支持 png/jpeg/gif/webp/pdf/txt，最大 10MB。返回可直接嵌入 Markdown 的 URL。
//	@Tags        users
//	@Accept      multipart/form-data
//	@Produce     json
//	@Param       file formData file true "文件"
//	@Success     201 {object} store.Upload
//	@Failure     400 {object} map[string]string
//	@Failure     413 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /uploads [post]
func (a *API) uploadFile(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+(1<<20))
	if err := r.ParseMultipartForm(maxUploadBytes + (1 << 20)); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_multipart", "invalid multipart form: "+err.Error())
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeCode(w, http.StatusBadRequest, "file_required", "multipart field 'file' is required")
		return
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxUploadBytes+1))
	if err != nil {
		writeCode(w, http.StatusBadRequest, "read_failed", "could not read uploaded file")
		return
	}
	if len(data) > maxUploadBytes {
		writeCode(w, http.StatusRequestEntityTooLarge, "file_too_large", "file exceeds the 10MB limit")
		return
	}
	ct := http.DetectContentType(data)
	if i := strings.IndexByte(ct, ';'); i > 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	if !uploadAllowedTypes[ct] {
		writeCode(w, http.StatusBadRequest, "unsupported_file_type", "unsupported file type: "+ct)
		return
	}
	name := "file"
	if header != nil && header.Filename != "" {
		name = header.Filename
	}
	up, err := a.store.CreateUpload(name, ct, data, userFrom(r))
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, up)
}

// getUpload 返回上传的文件内容（随机 key，不可猜测）。
//
//	@Summary     读取上传附件
//	@Description 以正确的 Content-Type 返回上传的文件；不存在返回 404。
//	@Tags        users
//	@Produce     application/octet-stream
//	@Param       key path string true "上传 key"
//	@Success     200 {file} binary
//	@Failure     404 {object} map[string]string
//	@Router      /uploads/{key} [get]
func (a *API) getUpload(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	if key == "" || len(key) > 64 {
		writeNotFound(w, "upload")
		return
	}
	data, ct, err := a.store.GetUploadData(key)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "upload")
			return
		}
		internalError(w, err)
		return
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(data)
}
