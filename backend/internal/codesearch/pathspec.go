package codesearch

import (
	"path"
	"strings"
	"unicode"
)

// matchPathspecs 报告文件路径是否命中任一 git pathspec；patterns 为空表示全部命中。
// 仅用于索引实现的后置过滤，近似 git 的通配语义（`*` 可跨 `/`，`*.go` 匹配任意层级）。
func matchPathspecs(patterns []string, file string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if matchPathspec(p, file) {
			return true
		}
	}
	return false
}

func matchPathspec(pattern, file string) bool {
	p := strings.TrimSpace(pattern)
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" {
		return true
	}
	file = strings.TrimPrefix(file, "./")
	if strings.HasSuffix(p, "/") { // 目录前缀
		return strings.HasPrefix(file, p) || strings.HasPrefix(file, strings.TrimSuffix(p, "/")+"/")
	}
	if ok, _ := path.Match(p, file); ok {
		return true
	}
	if strings.HasPrefix(file, p+"/") { // 目录前缀（如 path:src）
		return true
	}
	// 无斜杠的 pattern（如 *.go）匹配任意层级的 basename。
	if !strings.Contains(p, "/") {
		if ok, _ := path.Match(p, path.Base(file)); ok {
			return true
		}
	}
	return false
}

// containsAll 报告 text 是否同时包含全部关键词。采用「智能大小写」：
// 关键词含大写字母时区分大小写，否则忽略大小写（与索引的 lowercase 分词一致，
// 因此搜 Alpha 能找到 uniqueAlphaToken，搜 alpha 也能找到）。
func containsAll(text string, terms []string) bool {
	low := strings.ToLower(text)
	for _, t := range terms {
		if t == "" {
			continue
		}
		if hasUpper(t) {
			if !strings.Contains(text, t) {
				return false
			}
		} else if !strings.Contains(low, strings.ToLower(t)) {
			return false
		}
	}
	return true
}

// hasUpper 报告字符串是否含大写字母。
func hasUpper(s string) bool {
	for _, r := range s {
		if unicode.IsUpper(r) {
			return true
		}
	}
	return false
}
