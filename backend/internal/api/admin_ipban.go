package api

// 管理端 IP / CIDR 黑名单：列出 / 新增 / 删除，以及全局请求拦截中间件。
//
// 黑名单命中后，HTTP 请求在进入路由前即被 403 拒绝；SSH 连接在握手前关闭。

import (
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/store"
)

type ipBanReq struct {
	CIDR string `json:"cidr"`
	Note string `json:"note"`
}

// ipBanMiddleware 拦截来源 IP 命中黑名单的请求。放在日志中间件内侧，
// 使被封禁的请求仍会被访问日志记录。
func ipBanMiddleware(st *store.Store, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := clientIP(r); st.IsIPBanned(ip) {
			logx.Infof("blocked blacklisted ip %s (%s %s)", ip, r.Method, r.URL.Path)
			writeCode(w, http.StatusForbidden, "ip_banned", "your IP address has been blocked")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ipInsideCIDR 判断 ip 是否落在 cidr 内（均需为合法地址）。
func ipInsideCIDR(ip, cidr string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	p, err := netip.ParsePrefix(strings.TrimSpace(cidr))
	if err != nil {
		return false
	}
	return p.Contains(addr.Unmap())
}

// adminListIPBans 列出 IP 黑名单条目。
//
//	@Summary     管理端 IP 黑名单列表
//	@Tags        admin
//	@Produce     json
//	@Success     200 {array} store.IPBan
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/ip-bans [get]
func (a *API) adminListIPBans(w http.ResponseWriter, r *http.Request) {
	bans, err := a.store.ListIPBans()
	if err != nil {
		internalError(w, err)
		return
	}
	setTotal(w, len(bans))
	writeJSON(w, http.StatusOK, bans)
}

// adminAddIPBan 新增 IP / CIDR 黑名单条目。
//
// 为避免管理员把自己锁在门外，拒绝新增覆盖当前请求来源 IP 的条目。
//
//	@Summary     管理端新增 IP 黑名单
//	@Tags        admin
//	@Accept      json
//	@Param       body body ipBanReq true "cidr 与备注"
//	@Success     201 {object} store.IPBan
//	@Failure     400 {object} map[string]string
//	@Failure     409 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/ip-bans [post]
func (a *API) adminAddIPBan(w http.ResponseWriter, r *http.Request) {
	var in ipBanReq
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	normalized, ok := store.NormalizeIPCIDR(in.CIDR)
	if !ok {
		writeCode(w, http.StatusBadRequest, "invalid_cidr", "invalid IP or CIDR")
		return
	}
	if ipInsideCIDR(clientIP(r), normalized) {
		writeCode(w, http.StatusBadRequest, "ip_ban_self", "the entry would block your own current IP")
		return
	}
	ban, err := a.store.AddIPBan(normalized, in.Note, userFrom(r))
	if err != nil {
		switch {
		case errors.Is(err, store.ErrExists):
			writeCode(w, http.StatusConflict, "ip_ban_exists", "this IP or CIDR is already blacklisted")
		case errors.Is(err, store.ErrInvalidCIDR):
			writeCode(w, http.StatusBadRequest, "invalid_cidr", "invalid IP or CIDR")
		default:
			internalError(w, err)
		}
		return
	}
	logx.Infof("admin %q blacklisted %s", userFrom(r), ban.CIDR)
	writeJSON(w, http.StatusCreated, ban)
}

// adminDeleteIPBan 删除 IP 黑名单条目。
//
//	@Summary     管理端删除 IP 黑名单
//	@Tags        admin
//	@Param       id path int true "条目 ID"
//	@Success     204 {object} nil
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/ip-bans/{id} [delete]
func (a *API) adminDeleteIPBan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeNotFound(w, "ip ban")
		return
	}
	if err := a.store.DeleteIPBan(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "ip ban")
			return
		}
		internalError(w, err)
		return
	}
	logx.Infof("admin %q removed ip ban %d", userFrom(r), id)
	w.WriteHeader(http.StatusNoContent)
}
