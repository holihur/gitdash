package api

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"gitdash/backend/internal/store"
)

// reservedNameRe 保留名允许的字符：小写字母/数字/下划线/连字符（可短于普通用户名，如 "api"）。
var reservedNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,254}$`)

// adminListReservedNames 列出注册保留名黑名单。
//
//	@Summary     列出保留用户名
//	@Tags        admin
//	@Produce     json
//	@Success     200 {object} map[string]any
//	@Security    BearerAuth
//	@Router      /admin/reserved-names [get]
func (a *API) adminListReservedNames(w http.ResponseWriter, r *http.Request) {
	names, err := a.store.ListReservedNames()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"names": names})
}

// adminAddReservedName 新增一个注册保留名。
//
//	@Summary     新增保留用户名
//	@Description 名单中的用户名禁止自助注册，但管理员仍可创建。
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body reservedNameReq true "name"
//	@Success     201 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/reserved-names [post]
func (a *API) adminAddReservedName(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	name := strings.ToLower(strings.TrimSpace(in.Name))
	if !reservedNameRe.MatchString(name) {
		writeCode(w, http.StatusBadRequest, "invalid_name", "name must be 1-255 chars: lowercase letters, digits, '_' or '-', starting alphanumeric")
		return
	}
	if err := a.store.AddReservedName(name, userFrom(r)); err != nil {
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "name_exists", "this name is already reserved")
			return
		}
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"name": name})
}

// adminDeleteReservedName 移除一个注册保留名。
//
//	@Summary     移除保留用户名
//	@Tags        admin
//	@Produce     json
//	@Param       name path string true "保留名"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/reserved-names/{name} [delete]
func (a *API) adminDeleteReservedName(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(strings.TrimSpace(r.PathValue("name")))
	if err := a.store.DeleteReservedName(name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "reserved name")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
