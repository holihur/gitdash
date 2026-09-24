package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode"

	"gitdash/backend/internal/store"
)

// 徽章系统：仅管理后台可定义与授予；目标（用户/仓库/组织）可挂出最多 3 个。
// 任意徽章可发给任意类型目标。

const maxBadgeImageBytes = 1 << 20 // 1MB

// maxBadgeEmojiRunes emoji 兜底最大码点数（允许组合序列）。
const maxBadgeEmojiRunes = 8

// normalizeBadgeEmoji 校验并清理 emoji 兜底字段；含控制字符或过长时返回 ok=false。
func normalizeBadgeEmoji(raw string) (string, bool) {
	e := strings.TrimSpace(raw)
	if e == "" {
		return "", true
	}
	if len([]rune(e)) > maxBadgeEmojiRunes {
		return "", false
	}
	for _, r := range e {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return e, true
}

// readOptionalBadgeImage 解析可选的多部分图片字段；no file 视为“无图”。
// 返回 (contentType, data, hasImage, ok)；校验失败时已写入响应。
func readOptionalBadgeImage(w http.ResponseWriter, r *http.Request, field string) (string, []byte, bool, bool) {
	// 非 multipart（纯表单字段，无图标）直接解析表单。
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		if err := r.ParseForm(); err != nil {
			writeCode(w, http.StatusBadRequest, "invalid_form", "invalid form: "+err.Error())
			return "", nil, false, false
		}
		return "", nil, false, true
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBadgeImageBytes+(1<<20))
	if err := r.ParseMultipartForm(maxBadgeImageBytes + (1 << 20)); err != nil {
		writeCode(w, http.StatusBadRequest, "invalid_multipart", "invalid multipart form: "+err.Error())
		return "", nil, false, false
	}
	file, _, err := r.FormFile(field)
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil, false, true
		}
		writeCode(w, http.StatusBadRequest, "file_required", "multipart field '"+field+"' is invalid")
		return "", nil, false, false
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, maxBadgeImageBytes+1))
	if err != nil || len(data) > maxBadgeImageBytes {
		writeCode(w, http.StatusBadRequest, "image_too_large", "badge image too large (max 1MB)")
		return "", nil, false, false
	}
	if len(data) == 0 {
		writeCode(w, http.StatusBadRequest, "image_empty", "empty image file")
		return "", nil, false, false
	}
	ct := http.DetectContentType(data)
	if !allowedAvatarTypes[ct] {
		writeCode(w, http.StatusBadRequest, "image_type_invalid", "unsupported image type (png/jpeg/gif/webp only)")
		return "", nil, false, false
	}
	return ct, data, true, true
}

// validBadgeTarget 校验目标类型与标识。
func validBadgeTarget(kind, owner, repo string) bool {
	switch kind {
	case "user", "org":
		return owner != "" && repo == ""
	case "repo":
		return owner != "" && repo != ""
	}
	return false
}

// badgeTargetPermission 判断当前用户是否有权设置某目标的挂出徽章。
func (a *API) badgeTargetPermission(kind, owner, repo, me string) bool {
	switch kind {
	case "user":
		return me != "" && me == owner
	case "org":
		return a.store.OrgRole(owner, me) == "owner"
	case "repo":
		return a.store.CanWrite(owner, repo, me)
	}
	return false
}

// ---- admin: badge definitions ----

// adminListBadges 列出全部徽章定义。
//
//	@Summary     管理端徽章列表
//	@Tags        admin
//	@Produce     json
//	@Success     200 {array} store.Badge
//	@Security    BearerAuth
//	@Router      /admin/badges [get]
func (a *API) adminListBadges(w http.ResponseWriter, r *http.Request) {
	badges, err := a.store.ListBadges()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, badges)
}

// adminCreateBadge 创建徽章（multipart：slug / label / description / image）。
//
//	@Summary     管理端创建徽章
//	@Tags        admin
//	@Accept      multipart/form-data
//	@Produce     json
//	@Param       slug        formData string false "唯一标识（省略时按 label 生成）"
//	@Param       label       formData string true  "名称"
//	@Param       description formData string false "描述（悬停显示）"
//	@Param       emoji       formData string false "无图标时的 emoji 兜底"
//	@Param       image       formData file   false "图标（png/jpeg/gif/webp，≤1MB）"
//	@Success     201 {object} store.Badge
//	@Security    BearerAuth
//	@Router      /admin/badges [post]
func (a *API) adminCreateBadge(w http.ResponseWriter, r *http.Request) {
	ct, data, hasImage, ok := readOptionalBadgeImage(w, r, "image")
	if !ok {
		return
	}
	label := strings.TrimSpace(r.FormValue("label"))
	if label == "" {
		writeCode(w, http.StatusBadRequest, "label_required", "label is required")
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	if slug == "" {
		slug = slugifyBadge(label)
	}
	emoji, ok := normalizeBadgeEmoji(r.FormValue("emoji"))
	if !ok {
		writeCode(w, http.StatusBadRequest, "emoji_invalid", "emoji must be at most 8 characters with no control characters")
		return
	}
	b, err := a.store.CreateBadge(slug, label, r.FormValue("description"))
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "badge_slug_exists", "badge slug already exists")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	if emoji != "" {
		if _, err := a.store.UpdateBadge(b.ID, store.BadgeUpdate{Emoji: &emoji}); err != nil {
			internalError(w, err)
			return
		}
	}
	if hasImage {
		if err := a.store.SetBadgeImage(b.ID, ct, data); err != nil {
			internalError(w, err)
			return
		}
	}
	b, _ = a.store.GetBadge(b.ID)
	writeJSON(w, http.StatusCreated, b)
}

// adminUpdateBadge 更新徽章文字信息。
//
//	@Summary     管理端更新徽章
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       id   path int true "徽章 ID"
//	@Param       body body map[string]string true "slug / label / description"
//	@Success     200 {object} store.Badge
//	@Security    BearerAuth
//	@Router      /admin/badges/{id} [patch]
func (a *API) adminUpdateBadge(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var in struct {
		Slug        *string `json:"slug"`
		Label       *string `json:"label"`
		Description *string `json:"description"`
		Emoji       *string `json:"emoji"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if in.Label != nil && strings.TrimSpace(*in.Label) == "" {
		writeCode(w, http.StatusBadRequest, "label_required", "label is required")
		return
	}
	if in.Emoji != nil {
		emoji, ok := normalizeBadgeEmoji(*in.Emoji)
		if !ok {
			writeCode(w, http.StatusBadRequest, "emoji_invalid", "emoji must be at most 8 characters with no control characters")
			return
		}
		in.Emoji = &emoji
	}
	b, err := a.store.UpdateBadge(id, store.BadgeUpdate{Slug: in.Slug, Label: in.Label, Description: in.Description, Emoji: in.Emoji})
	if errors.Is(err, store.ErrExists) {
		writeCode(w, http.StatusConflict, "badge_slug_exists", "badge slug already exists")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, "badge")
		return
	}
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// adminSetBadgeImage 上传 / 替换徽章图标（multipart 字段 image）。
//
//	@Summary     管理端设置徽章图标
//	@Tags        admin
//	@Accept      multipart/form-data
//	@Produce     json
//	@Param       id    path     int  true "徽章 ID"
//	@Param       image formData file true "图标（png/jpeg/gif/webp，≤1MB）"
//	@Success     200 {object} store.Badge
//	@Security    BearerAuth
//	@Router      /admin/badges/{id}/image [post]
func (a *API) adminSetBadgeImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	ct, data, has, ok := readOptionalBadgeImage(w, r, "image")
	if !ok {
		return
	}
	if !has {
		writeCode(w, http.StatusBadRequest, "file_required", "multipart field 'image' is required")
		return
	}
	if err := a.store.SetBadgeImage(id, ct, data); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "badge")
			return
		}
		internalError(w, err)
		return
	}
	b, _ := a.store.GetBadge(id)
	writeJSON(w, http.StatusOK, b)
}

// adminDeleteBadgeImage 删除徽章图标（保留徽章定义）。
//
//	@Summary     管理端删除徽章图标
//	@Tags        admin
//	@Produce     json
//	@Param       id path int true "徽章 ID"
//	@Success     200 {object} store.Badge
//	@Failure     404 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/badges/{id}/image [delete]
func (a *API) adminDeleteBadgeImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if _, err := a.store.GetBadge(id); err != nil {
		writeNotFound(w, "badge")
		return
	}
	if err := a.store.DeleteBadgeImage(id); err != nil && !errors.Is(err, store.ErrNotFound) {
		internalError(w, err)
		return
	}
	b, err := a.store.GetBadge(id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

// adminDeleteBadge 删除徽章。
//
//	@Summary     管理端删除徽章
//	@Tags        admin
//	@Param       id path int true "徽章 ID"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /admin/badges/{id} [delete]
func (a *API) adminDeleteBadge(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	if err := a.store.DeleteBadge(id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "badge")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- admin: grants ----

// adminListBadgeGrants 列出某徽章的授予记录。
//
//	@Summary     管理端徽章授予列表
//	@Tags        admin
//	@Produce     json
//	@Param       id path int true "徽章 ID"
//	@Success     200 {array} store.BadgeGrant
//	@Security    BearerAuth
//	@Router      /admin/badges/{id}/grants [get]
func (a *API) adminListBadgeGrants(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	grants, err := a.store.ListBadgeGrants(id)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, grants)
}

// adminGrantBadge 授予徽章给目标（body: kind/owner/repo），并给用户目标发站内信。
//
//	@Summary     管理端授予徽章
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       id   path int true "徽章 ID"
//	@Param       body body map[string]string true "kind / owner / repo"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /admin/badges/{id}/grants [post]
func (a *API) adminGrantBadge(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	var in struct {
		Kind  string `json:"kind"`
		Owner string `json:"owner"`
		Repo  string `json:"repo"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Kind = strings.TrimSpace(in.Kind)
	in.Owner = strings.TrimSpace(in.Owner)
	in.Repo = strings.TrimSpace(in.Repo)
	if !validBadgeTarget(in.Kind, in.Owner, in.Repo) {
		writeCode(w, http.StatusBadRequest, "invalid_target", "invalid badge target")
		return
	}
	if !a.store.BadgeTargetExists(in.Kind, in.Owner, in.Repo) {
		writeCode(w, http.StatusNotFound, "target_not_found", "badge target not found")
		return
	}
	if err := a.store.GrantBadge(id, in.Kind, in.Owner, in.Repo); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeNotFound(w, "badge")
			return
		}
		if errors.Is(err, store.ErrExists) {
			writeCode(w, http.StatusConflict, "badge_already_granted", "badge already granted to this target")
			return
		}
		internalError(w, err)
		return
	}
	// 站内信：用户目标直接发给本人；组织目标发给全部成员；仓库目标发给 owner。
	b, _ := a.store.GetBadge(id)
	a.notifyBadgeGranted(in.Kind, in.Owner, in.Repo, b.Label)
	w.WriteHeader(http.StatusNoContent)
}

// adminRevokeBadge 撤销授予（query: kind/owner/repo）。
//
//	@Summary     管理端撤销徽章
//	@Tags        admin
//	@Param       id    path  int    true "徽章 ID"
//	@Param       kind  query string true "目标类型 user/repo/org"
//	@Param       owner query string true "用户名 / 组织名 / 仓库所有者"
//	@Param       repo  query string false "仓库名（kind=repo 时必填）"
//	@Success     204 {object} nil
//	@Security    BearerAuth
//	@Router      /admin/badges/{id}/grants [delete]
func (a *API) adminRevokeBadge(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeCode(w, http.StatusBadRequest, "invalid_id", "invalid id")
		return
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if !validBadgeTarget(kind, owner, repo) {
		writeCode(w, http.StatusBadRequest, "invalid_target", "invalid badge target")
		return
	}
	if err := a.store.RevokeBadge(id, kind, owner, repo); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeCode(w, http.StatusNotFound, "grant_not_found", "badge grant not found")
			return
		}
		internalError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// notifyBadgeGranted 给授予目标的用户发送站内信。
func (a *API) notifyBadgeGranted(kind, owner, repo, label string) {
	recipients := []string{}
	switch kind {
	case "user":
		recipients = append(recipients, owner)
	case "org":
		if ms, err := a.store.OrgMembers(owner); err == nil {
			for _, m := range ms {
				recipients = append(recipients, m.Username)
			}
		}
	case "repo":
		recipients = append(recipients, owner)
	}
	title := label
	_ = a.store.AddNotifications(recipients, "system", "badge_granted", owner, repo, 0, title, "")
}

// ---- public ----

// getBadges 返回某目标公开展示的徽章。
//
//	@Summary     目标展示的徽章
//	@Description 返回用户 / 仓库 / 组织主页上挂出的徽章（公开）。
//	@Tags        badges
//	@Produce     json
//	@Param       kind  query string true "user/repo/org"
//	@Param       owner query string true "用户名 / 组织名 / 仓库所有者"
//	@Param       repo  query string false "仓库名（kind=repo 时必填）"
//	@Success     200 {array} store.Badge
//	@Router      /badges [get]
func (a *API) getBadges(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if !validBadgeTarget(kind, owner, repo) {
		writeCode(w, http.StatusBadRequest, "invalid_target", "invalid badge target")
		return
	}
	badges, err := a.store.DisplayedBadges(kind, owner, repo)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, badges)
}

// postBadgesBatch 批量返回多个目标展示的徽章（列表卡片 / 搜索结果用，避免逐条请求）。
//
//	@Summary     批量目标徽章
//	@Tags        badges
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]interface{} true "kind + targets[{owner,repo}]"
//	@Success     200 {object} object "items: {\"owner/repo\": [Badge]}"
//	@Router      /badges/batch [post]
func (a *API) postBadgesBatch(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind    string `json:"kind"`
		Targets []struct {
			Owner string `json:"owner"`
			Repo  string `json:"repo"`
		} `json:"targets"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Kind = strings.TrimSpace(in.Kind)
	if in.Kind != "user" && in.Kind != "repo" && in.Kind != "org" {
		writeCode(w, http.StatusBadRequest, "invalid_kind", "kind must be user, repo or org")
		return
	}
	if len(in.Targets) > 100 {
		in.Targets = in.Targets[:100]
	}
	pairs := make([][2]string, 0, len(in.Targets))
	for _, t := range in.Targets {
		owner := strings.TrimSpace(t.Owner)
		if owner == "" {
			continue
		}
		pairs = append(pairs, [2]string{owner, strings.TrimSpace(t.Repo)})
	}
	items, err := a.store.DisplayedBadgesBatch(in.Kind, pairs)
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// getBadgeImage 返回徽章图标。
//
//	@Summary     徽章图标
//	@Tags        badges
//	@Produce     image/png
//	@Param       id path int true "徽章 ID"
//	@Success     200 {file} binary
//	@Failure     404 {object} map[string]string
//	@Router      /badges/{id}/image [get]
func (a *API) getBadgeImage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeNotFound(w, "badge")
		return
	}
	ct, data, err := a.store.GetBadgeImage(id)
	if err != nil {
		writeNotFound(w, "badge")
		return
	}
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

// getOwnedBadges 返回某目标「已获得 + 已挂出」的徽章，供设置界面选择（需写权限）。
//
//	@Summary     待挂载徽章（设置用）
//	@Tags        badges
//	@Produce     json
//	@Param       kind  query string true "user/repo/org"
//	@Param       owner query string true "用户名 / 组织名 / 仓库所有者"
//	@Param       repo  query string false "仓库名（kind=repo 时必填）"
//	@Success     200 {object} object "granted / displayed"
//	@Security    BearerAuth
//	@Router      /badges/owned [get]
func (a *API) getOwnedBadges(w http.ResponseWriter, r *http.Request) {
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	owner := strings.TrimSpace(r.URL.Query().Get("owner"))
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if !validBadgeTarget(kind, owner, repo) {
		writeCode(w, http.StatusBadRequest, "invalid_target", "invalid badge target")
		return
	}
	me := userFrom(r)
	if !a.badgeTargetPermission(kind, owner, repo, me) {
		writeCode(w, http.StatusForbidden, "badge_forbidden", "not allowed to manage badges for this target")
		return
	}
	granted, err := a.store.GrantedBadges(kind, owner, repo)
	if err != nil {
		internalError(w, err)
		return
	}
	displayed, err := a.store.DisplayedBadges(kind, owner, repo)
	if err != nil {
		internalError(w, err)
		return
	}
	ids := make([]int64, 0, len(displayed))
	for _, b := range displayed {
		ids = append(ids, b.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"granted":   granted,
		"displayed": ids,
		"max":       store.MaxDisplayedBadges,
	})
}

// setBadgeDisplay 设置目标挂出的徽章（需写权限）。
//
//	@Summary     设置挂出的徽章
//	@Tags        badges
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]interface{} true "kind/owner/repo/badge_ids"
//	@Success     200 {object} map[string]interface{}
//	@Security    BearerAuth
//	@Router      /badges/display [put]
func (a *API) setBadgeDisplay(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Kind     string  `json:"kind"`
		Owner    string  `json:"owner"`
		Repo     string  `json:"repo"`
		BadgeIDs []int64 `json:"badge_ids"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	in.Kind = strings.TrimSpace(in.Kind)
	in.Owner = strings.TrimSpace(in.Owner)
	in.Repo = strings.TrimSpace(in.Repo)
	if !validBadgeTarget(in.Kind, in.Owner, in.Repo) {
		writeCode(w, http.StatusBadRequest, "invalid_target", "invalid badge target")
		return
	}
	if !a.badgeTargetPermission(in.Kind, in.Owner, in.Repo, userFrom(r)) {
		writeCode(w, http.StatusForbidden, "badge_forbidden", "not allowed to manage badges for this target")
		return
	}
	if err := a.store.SetDisplayedBadges(in.Kind, in.Owner, in.Repo, in.BadgeIDs); err != nil {
		internalError(w, err)
		return
	}
	displayed, _ := a.store.DisplayedBadges(in.Kind, in.Owner, in.Repo)
	writeJSON(w, http.StatusOK, map[string]any{"displayed": displayed})
}

// slugifyBadge 由 label 生成 slug（保留字母数字，其余转连字符）。
func slugifyBadge(label string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(label) {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "badge"
	}
	return out
}
