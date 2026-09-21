package api

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"strings"
)

// announcementPayload 构造公告响应体（纯函数，便于测试）：
// 标题与正文都为空时返回 enabled=false；level 仅接受 warning/critical，其余归一为 info；
// id 为内容指纹，内容变化时改变，前端据此判断用户关闭过的公告是否需重新展示。
func announcementPayload(title, message, level string) map[string]any {
	title = strings.TrimSpace(title)
	message = strings.TrimSpace(message)
	if level != "warning" && level != "critical" {
		level = "info"
	}
	if title == "" && message == "" {
		return map[string]any{"enabled": false}
	}
	sum := sha1.Sum([]byte(title + "\x00" + message + "\x00" + level))
	return map[string]any{
		"enabled": true,
		"id":      hex.EncodeToString(sum[:]),
		"level":   level,
		"title":   title,
		"message": message,
	}
}

// announcement 返回管理端配置的全站通知（公开接口，登录与否都可读取）。
//
//	@Summary     全站通知
//	@Description 返回当前启用的全站公告（标题/正文/级别）；未启用或内容为空时 enabled=false。id 为内容指纹，客户端据此判断公告是否已被用户关闭。
//	@Tags        misc
//	@Produce     json
//	@Success     200 {object} map[string]any
//	@Router      /announcement [get]
func (a *API) announcement(w http.ResponseWriter, r *http.Request) {
	if a.store.GetSetting("announcement_enabled") != "1" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	writeJSON(w, http.StatusOK, announcementPayload(
		a.store.GetSetting("announcement_title"),
		a.store.GetSetting("announcement_message"),
		a.store.GetSetting("announcement_level"),
	))
}
