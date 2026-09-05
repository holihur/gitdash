package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"strconv"
)

// ---- inbox（收件箱通知）----

func (a *API) listInbox(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	items, err := a.store.ListNotifications(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, items)
}

func (a *API) inboxUnread(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	n, err := a.store.UnreadNotifications(me)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"count": n})
}

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

func (a *API) inboxReadAll(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	if err := a.store.MarkAllNotificationsRead(me); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

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
