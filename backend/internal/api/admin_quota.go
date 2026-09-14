package api

import (
	"net/http"
	"strings"

	"gitdash/backend/internal/store"
)

// adminGetQuota 获取实例默认配额与全部覆盖。
//
//	@Summary     获取配额配置
//	@Tags        admin
//	@Produce     json
//	@Success     200 {object} map[string]any
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/quota [get]
func (a *API) adminGetQuota(w http.ResponseWriter, r *http.Request) {
	overrides, err := a.store.ListQuotaOverrides()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"default":   a.store.QuotaDefault(),
		"overrides": overrides,
	})
}

// adminSaveQuotaDefault 保存实例默认配额（改后即时生效）。
//
//	@Summary     保存默认配额
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body store.Quota true "默认配额（0 = 不限）"
//	@Success     200 {object} store.Quota
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/quota [post]
func (a *API) adminSaveQuotaDefault(w http.ResponseWriter, r *http.Request) {
	var q store.Quota
	if err := readJSON(w, r, &q); err != nil {
		return
	}
	if err := a.store.SetQuotaDefault(q); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.store.QuotaDefault())
}

// adminSaveQuotaOverride 保存单个用户/组织的配额覆盖（整体替换默认）。
//
//	@Summary     保存配额覆盖
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       scope path string      true "user 或 org"
//	@Param       name  path string      true "用户名或组织名"
//	@Param       body  body store.Quota true "覆盖配额"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/quota/{scope}/{name} [put]
func (a *API) adminSaveQuotaOverride(w http.ResponseWriter, r *http.Request) {
	scope := r.PathValue("scope")
	name := strings.TrimSpace(r.PathValue("name"))
	var q store.Quota
	if err := readJSON(w, r, &q); err != nil {
		return
	}
	switch scope {
	case "user":
		if _, err := a.store.GetByUsername(name); err != nil {
			writeCode(w, http.StatusNotFound, "user_not_found", "user not found")
			return
		}
		if err := a.store.SetUserQuota(name, q); err != nil {
			internalError(w, err)
			return
		}
	case "org":
		if !a.store.IsOrg(name) {
			writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
			return
		}
		if err := a.store.SetOrgQuota(name, q); err != nil {
			internalError(w, err)
			return
		}
	default:
		writeCode(w, http.StatusBadRequest, "invalid_scope", "scope must be 'user' or 'org'")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// adminDeleteQuotaOverride 删除配额覆盖，恢复默认。
//
//	@Summary     删除配额覆盖
//	@Tags        admin
//	@Produce     json
//	@Param       scope path string true "user 或 org"
//	@Param       name  path string true "用户名或组织名"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/quota/{scope}/{name} [delete]
func (a *API) adminDeleteQuotaOverride(w http.ResponseWriter, r *http.Request) {
	scope := r.PathValue("scope")
	name := strings.TrimSpace(r.PathValue("name"))
	switch scope {
	case "user":
		if err := a.store.DeleteUserQuota(name); err != nil {
			internalError(w, err)
			return
		}
	case "org":
		if err := a.store.DeleteOrgQuota(name); err != nil {
			internalError(w, err)
			return
		}
	default:
		writeCode(w, http.StatusBadRequest, "invalid_scope", "scope must be 'user' or 'org'")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
