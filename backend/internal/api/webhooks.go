package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func validWebhookURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func (a *API) listWebhooks(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	hooks, err := a.store.ListWebhooks(owner, name)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

func (a *API) createWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	var in struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.URL = strings.TrimSpace(in.URL)
	in.Secret = strings.TrimSpace(in.Secret)
	if !validWebhookURL(in.URL) {
		writeCode(w, http.StatusBadRequest, "invalid_url", "url must be a valid http(s) url")
		return
	}
	if len([]rune(in.URL)) > 2048 {
		writeCode(w, http.StatusBadRequest, "invalid_url", "url too long (max 2048 chars)")
		return
	}
	if in.Secret != "" && len(in.Secret) < 16 {
		writeCode(w, http.StatusBadRequest, "invalid_secret", "webhook secret must be at least 16 characters")
		return
	}
	hk, err := a.store.CreateWebhook(owner, name, in.URL, in.Secret)
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "webhook_exists", "webhook already registered")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, hk)
}

func (a *API) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	owner, name, ok := a.requireOwner(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if errors.Is(a.store.DeleteWebhook(owner, name, id), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "webhook_not_found", "webhook not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- ssh keys ----
