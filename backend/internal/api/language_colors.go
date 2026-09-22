package api

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"gitdash/backend/internal/gitsvc"
)

// 语言配色覆盖（管理端全局配置）：language -> #RRGGBB，JSON 存在 settings 表。
const languageColorsKey = "language_colors"

var hexColorRe = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// languageColorOverrides 读取管理端配置的语言颜色覆盖。
func (a *API) languageColorOverrides() map[string]string {
	out := map[string]string{}
	raw := strings.TrimSpace(a.store.GetSetting(languageColorsKey))
	if raw == "" {
		return out
	}
	_ = json.Unmarshal([]byte(raw), &out)
	if out == nil {
		out = map[string]string{}
	}
	return out
}

// languages 返回检测器支持的语言列表、默认配色与管理端配置的颜色覆盖。
//
//	@Summary     语言列表与配色
//	@Description 返回内置支持的语言、每种语言的默认配色，以及管理端配置的颜色覆盖（language -> #RRGGBB）。公开接口。
//	@Tags        misc
//	@Produce     json
//	@Success     200 {object} object "languages、defaults 与 colors"
//	@Router      /languages [get]
func (a *API) languages(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"languages": gitsvc.KnownLanguages(),
		"defaults":  gitsvc.DefaultLanguageColors(),
		"colors":    a.languageColorOverrides(),
	})
}

// adminSaveLanguageColors 保存语言配色覆盖（全量替换）。
//
//	@Summary     保存语言配色
//	@Description 以 language -> #RRGGBB 的形式全量替换配色覆盖；空 map 表示全部恢复默认。
//	@Tags        admin
//	@Accept      json
//	@Produce     json
//	@Param       body body map[string]any true "colors 映射"
//	@Success     200 {object} map[string]any
//	@Failure     400 {object} map[string]string
//	@Failure     401 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /admin/language-colors [post]
func (a *API) adminSaveLanguageColors(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Colors map[string]string `json:"colors"`
	}
	if err := readJSON(w, r, &in); err != nil {
		return
	}
	if len(in.Colors) > 500 {
		writeCode(w, http.StatusBadRequest, "too_many_colors", "too many language colors (max 500)")
		return
	}
	clean := make(map[string]string, len(in.Colors))
	for lang, color := range in.Colors {
		lang = strings.TrimSpace(lang)
		color = strings.TrimSpace(color)
		if lang == "" || len(lang) > 64 || !hexColorRe.MatchString(color) {
			writeCode(w, http.StatusBadRequest, "invalid_language_color",
				"invalid language or color (expected #RRGGBB)")
			return
		}
		clean[lang] = strings.ToLower(color)
	}
	b, err := json.Marshal(clean)
	if err != nil {
		internalError(w, err)
		return
	}
	if err := a.store.SetSetting(languageColorsKey, string(b)); err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "colors": clean})
}
