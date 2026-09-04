package api

import (
	"errors"
	"gitdash/backend/internal/store"
	"net/http"
	"strings"
)

func (a *API) createOrg(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name    string `json:"name"`
		Display string `json:"display"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Name = strings.ToLower(strings.TrimSpace(in.Name))
	if !usernameRe.MatchString(in.Name) {
		writeCode(w, http.StatusBadRequest, "username_invalid", "org name must be 2-32 chars: lowercase letters, digits, '_' or '-', starting alphanumeric")
		return
	}
	o, err := a.store.CreateOrg(in.Name, strings.TrimSpace(in.Display), userFrom(r))
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "org_exists", "organization name already taken")
		return
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (a *API) listOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := a.store.ListMyOrgs(userFrom(r))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := []map[string]any{}
	for _, o := range orgs {
		out = append(out, map[string]any{
			"name": o.Name, "display": o.Display, "created_at": o.CreatedAt,
			"role": a.store.OrgRole(o.Name, userFrom(r)),
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *API) getOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	role := a.store.OrgRole(org, userFrom(r))
	if role == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": org, "role": role})
}

func (a *API) deleteOrg(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	if err := a.store.DeleteOrg(org); err != nil {
		if strings.Contains(err.Error(), "not empty") {
			writeCode(w, http.StatusConflict, "org_not_empty", "delete or move all repositories before deleting the organization")
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listOrgMembers(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) == "" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	members, err := a.store.OrgMembers(org)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, members)
}

func (a *API) addOrgMember(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	if a.store.OrgRole(org, userFrom(r)) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	var in struct {
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Username = strings.ToLower(strings.TrimSpace(in.Username))
	if in.Role == "" {
		in.Role = "member"
	}
	if in.Role != "member" && in.Role != "owner" {
		writeCode(w, http.StatusBadRequest, "invalid_permission", "role must be member or owner")
		return
	}
	if _, err := a.store.GetByUsername(in.Username); err != nil {
		writeCode(w, http.StatusNotFound, "user_not_found", "user not found")
		return
	}
	if err := a.store.AddOrgMember(org, in.Username, in.Role); err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"org": org, "username": in.Username, "role": in.Role})
}

func (a *API) removeOrgMember(w http.ResponseWriter, r *http.Request) {
	org := r.PathValue("org")
	me := userFrom(r)
	if a.store.OrgRole(org, me) != "owner" {
		writeCode(w, http.StatusNotFound, "org_not_found", "organization not found")
		return
	}
	target := strings.ToLower(strings.TrimSpace(r.PathValue("username")))
	if target == me {
		writeCode(w, http.StatusBadRequest, "cannot_remove_self", "transfer ownership or delete the organization instead")
		return
	}
	if a.store.OrgRole(org, target) == "owner" {
		// 仅剩一个 owner 时不允许移除 owner
		members, _ := a.store.OrgMembers(org)
		owners := 0
		for _, m := range members {
			if m.Role == "owner" {
				owners++
			}
		}
		if owners <= 1 {
			writeCode(w, http.StatusBadRequest, "last_owner", "organization must keep at least one owner")
			return
		}
	}
	if errors.Is(a.store.RemoveOrgMember(org, target), store.ErrNotFound) {
		writeCode(w, http.StatusNotFound, "member_not_found", "member not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
