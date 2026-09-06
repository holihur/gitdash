package api

// 管理面板的用户管理端点。

import (
	"errors"
	"gitdash/backend/internal/store"
	"log"
	"net/http"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

// adminListUsers 列出所有用户（支持 ?q= 用户名模糊过滤与 limit/offset 分页）。
//
//	@Summary     管理端用户列表
//	@Tags        admin
//	@Produce     json
//	@Param       q      query string false "用户名模糊过滤"
//	@Param       limit  query int    false "每页数量（默认 200，最大 500）"
//	@Param       offset query int    false "偏移"
//	@Success     200 {array} store.User
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/users [get]
func (a *API) adminListUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := pageParams(r)
	users, total, err := a.store.AdminListUsers(strings.TrimSpace(r.URL.Query().Get("q")), limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, users)
}

// adminCreateUser 创建用户（用户名/密码校验规则与 register 一致）。
//
//	@Summary     管理端创建用户
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body adminCreateUserReq true "用户名、密码与可选邮箱"
//	@Success     201 {object} store.User
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/users [post]
func (a *API) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	var in adminCreateUserReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	username := strings.ToLower(strings.TrimSpace(in.Username))
	if a.store.IsOrg(username) {
		writeCode(w, http.StatusConflict, "username_taken", "username is already taken")
		return
	}
	if !usernameRe.MatchString(username) {
		writeCode(w, http.StatusBadRequest, "username_invalid", "username must be 2-32 chars: lowercase letters, digits, '_' or '-', starting alphanumeric")
		return
	}
	if len(in.Password) < 8 {
		writeCode(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters")
		return
	}
	email := strings.TrimSpace(in.Email)
	if email != "" && !emailRe.MatchString(email) {
		writeCode(w, http.StatusBadRequest, "email_invalid", "invalid email address")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	u, err := a.store.CreateUser(username, string(hash))
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "username_taken", "username is already taken")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if email != "" {
		if err := a.store.SetUserEmail(username, email); err != nil {
			writeErr(w, http.StatusInternalServerError, err.Error())
			return
		}
		u.Email = email
	}
	log.Printf("admin %q created user %q", userFrom(r), username)
	writeJSON(w, http.StatusCreated, u)
}

// adminResetPassword 重置指定用户的密码（撤销其全部会话）。
//
//	@Summary     管理端重置用户密码
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       username path string true "用户名"
//	@Param       body body adminResetPasswordReq true "新密码"
//	@Success     204 {object} nil
//	@Failure     400 {object} map[string]string
//	@Failure     404 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/users/{username}/reset_password [post]
func (a *API) adminResetPassword(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	var in adminResetPasswordReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if len(in.Password) < 8 {
		writeCode(w, http.StatusBadRequest, "password_too_short", "password must be at least 8 characters")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.UpdatePassword(username, string(hash)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "user")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// 重置后撤销该用户全部会话，强制重新登录
	if err := a.store.DeleteSessionsExcept(username, ""); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("admin %q reset password of user %q", userFrom(r), username)
	w.WriteHeader(http.StatusNoContent)
}

// adminDeleteUser 删除用户及其全部归属数据。
//
//	@Summary     管理端删除用户
//	@Description 级联删除用户的会话/密钥/PAT/OAuth/仓库等归属数据。返回 204。
//	@Tags        admin
//	@Produce     json
//	@Param       username path string true "用户名"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Failure     500 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/users/{username} [delete]
func (a *API) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	username := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	if err := a.store.AdminDeleteUser(username); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "user")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	log.Printf("admin %q deleted user %q", userFrom(r), username)
	w.WriteHeader(http.StatusNoContent)
}

// ---- 请求体 ----

type adminCreateUserReq struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

type adminResetPasswordReq struct {
	Password string `json:"password"`
}
