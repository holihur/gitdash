package api

import (
	"crypto/rand"
	"encoding/hex"
	"gitdash/backend/internal/store"
	"net"
	"net/http"
	"strings"
	"time"
)

func (a *API) rateKey(username, ip string) string { return username + "|" + ip }

func (a *API) rateBlocked(key string) bool {
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	if len(a.rateFails) > loginRateCutoff { // 防止内存无限增长
		a.rateFails = map[string]rateRec{}
	}
	rec, ok := a.rateFails[key]
	if !ok {
		return false
	}
	if time.Now().After(rec.until) {
		delete(a.rateFails, key)
		return false
	}
	return rec.count >= loginMaxFails
}

func (a *API) rateFail(key string) {
	a.rateMu.Lock()
	defer a.rateMu.Unlock()
	rec, ok := a.rateFails[key]
	if !ok || time.Now().After(rec.until) {
		rec = rateRec{until: time.Now().Add(loginWindow)}
	}
	rec.count++
	a.rateFails[key] = rec
}

func (a *API) rateReset(key string) {
	a.rateMu.Lock()
	delete(a.rateFails, key)
	a.rateMu.Unlock()
}

func clientIP(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-For"); h != "" {
		if i := strings.IndexByte(h, ','); i > 0 {
			return strings.TrimSpace(h[:i])
		}
		return strings.TrimSpace(h)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

const sessionCookie = "gitdash_session"

func (a *API) setSessionCookie(w http.ResponseWriter, r *http.Request, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(store.SessionTTL.Seconds()),
	})
}

func (a *API) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1})
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

func (a *API) sessionUser(r *http.Request) string {
	tok := bearerToken(r)
	if tok == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			tok = c.Value
		}
	}
	if tok == "" {
		return ""
	}
	username, err := a.store.GetSession(tok)
	if err != nil {
		return ""
	}
	return username
}

func newSessionToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func (a *API) startSession(w http.ResponseWriter, r *http.Request, status int, username string) {
	ua, err := a.store.GetByUsername(username)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	token, err := newSessionToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := a.store.CreateSession(token, ua.ID); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	a.setSessionCookie(w, r, token)
	writeJSON(w, status, map[string]string{"token": token, "username": ua.Username})
}

func (a *API) oauthIssueSession(w http.ResponseWriter, r *http.Request, username string) {
	if ua, err := a.store.GetByUsername(username); err == nil {
		if token, err := newSessionToken(); err == nil {
			if err := a.store.CreateSession(token, ua.ID); err == nil {
				a.setSessionCookie(w, r, token)
			}
		}
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

// ---- admin & oauth providers ----

const adminCookie = "gitdash_admin"
