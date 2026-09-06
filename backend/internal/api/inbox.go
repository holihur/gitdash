package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"strconv"
)

// ---- inbox（收件箱通知）----

// listInbox 列出我的收件箱通知。
//
//	@Summary     列出收件箱通知
//	@Tags        inbox
//	@Produce     json
//	@Param       limit  query int false "每页数量（默认 50，最大 500）"
//	@Param       offset query int false "偏移量"
//	@Success     200 {array} store.Notification
//	@SuccessHeader X-Total-Count int "通知总数"
//	@Security    BearerAuth
//	@Router      /inbox [get]
func (a *API) listInbox(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	limit, offset := pageParams(r)
	if !r.URL.Query().Has("limit") {
		limit = 50
	}
	items, err := a.store.ListNotifications(me, limit, offset)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := a.store.CountNotifications(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	setTotal(w, total)
	writeJSON(w, http.StatusOK, items)
}

// inboxUnread 未读通知数量。
//
//	@Summary     未读通知数量
//	@Tags        inbox
//	@Produce     json
//	@Success     200 {object} object "count"
//	@Security    BearerAuth
//	@Router      /inbox/unread [get]
func (a *API) inboxUnread(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	n, err := a.store.UnreadNotifications(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

// inboxReadOne 标记单条通知为已读。
//
//	@Summary     标记通知已读
//	@Tags        inbox
//	@Produce     json
//	@Param       id path int true "通知 ID"
//	@Success     200 {object} object "ok"
//	@Security    BearerAuth
//	@Router      /inbox/read/{id} [post]
func (a *API) inboxReadOne(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if err := a.store.MarkNotificationRead(me, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "notification_not_found", "notification not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// inboxReadAll 标记全部通知为已读。
//
//	@Summary     全部标记已读
//	@Tags        inbox
//	@Produce     json
//	@Success     200 {object} object "ok"
//	@Security    BearerAuth
//	@Router      /inbox/read [post]
func (a *API) inboxReadAll(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if err := a.store.MarkAllNotificationsRead(me); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// deleteInbox 删除一条通知。
//
//	@Summary     删除通知
//	@Tags        inbox
//	@Param       id path int true "通知 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /inbox/{id} [delete]
func (a *API) deleteInbox(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if err := a.store.DeleteNotification(me, id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "notification_not_found", "notification not found")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
