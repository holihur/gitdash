package gitsvc

import (
	"regexp"
	"strings"
)

// CodeownerRule 一条 CODEOWNERS 规则。
type CodeownerRule struct {
	Pattern string
	Owners  []string // 用户名（已去掉前导 @）
	re      *regexp.Regexp
}

// Codeowners 解析后的 CODEOWNERS：按声明顺序，后匹配的规则优先。
type Codeowners struct {
	Rules []CodeownerRule
}

// codeownerPaths CODEOWNERS 文件的查找位置（按优先级）。
var codeownerPaths = []string{"CODEOWNERS", ".github/CODEOWNERS", "docs/CODEOWNERS"}

// LoadCodeowners 读取 ref 上的 CODEOWNERS（根目录 / .github / docs）；无则返回 nil。
func LoadCodeowners(owner, name, ref string) *Codeowners {
	for _, p := range codeownerPaths {
		blob, err := ReadBlob(owner, name, ref, p)
		if err == nil && blob.Encoding == "utf-8" && strings.TrimSpace(blob.Content) != "" {
			return ParseCodeowners(blob.Content)
		}
	}
	return nil
}

// ParseCodeowners 解析 CODEOWNERS 文本（忽略空行与 # 注释）。
func ParseCodeowners(content string) *Codeowners {
	c := &Codeowners{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		owners := make([]string, 0, len(fields)-1)
		for _, f := range fields[1:] {
			f = strings.TrimPrefix(f, "@")
			// 仅支持用户（@name）；跳过邮箱（含 @）与团队（@org/team，含 /）
			if f == "" || strings.ContainsAny(f, "@/") {
				continue
			}
			owners = append(owners, f)
		}
		if len(owners) == 0 {
			continue
		}
		re, err := compileCodeownerPattern(fields[0])
		if err != nil {
			continue
		}
		c.Rules = append(c.Rules, CodeownerRule{Pattern: fields[0], Owners: owners, re: re})
	}
	return c
}

// Owners 返回某个仓库内路径的 code owners（最后匹配的规则优先）；无匹配返回 nil。
func (c *Codeowners) Owners(path string) []string {
	if c == nil {
		return nil
	}
	path = strings.TrimPrefix(path, "/")
	var out []string
	for _, r := range c.Rules {
		if r.re != nil && r.re.MatchString(path) {
			out = r.Owners
		}
	}
	return out
}

// compileCodeownerPattern 把 CODEOWNERS 通配符编译为正则：
// `*` 匹配非 `/`，`**` 匹配任意，`?` 匹配单个非 `/`；
// 含 `/`（或前导 `/`）按根锚定，否则匹配任意层级的 basename；尾随 `/` 匹配目录及其内容。
func compileCodeownerPattern(pat string) (*regexp.Regexp, error) {
	dirOnly := strings.HasSuffix(pat, "/")
	pat = strings.TrimSuffix(pat, "/")
	anchored := strings.HasPrefix(pat, "/") || strings.Contains(pat, "/")
	pat = strings.TrimPrefix(pat, "/")

	var b strings.Builder
	for i := 0; i < len(pat); i++ {
		ch := pat[i]
		switch ch {
		case '*':
			if i+1 < len(pat) && pat[i+1] == '*' {
				// `**/` 匹配零或多层目录
				if i+2 < len(pat) && pat[i+2] == '/' {
					b.WriteString("(.*/)?")
					i += 2
				} else {
					b.WriteString(".*")
					i++
				}
			} else {
				b.WriteString("[^/]*")
			}
		case '?':
			b.WriteString("[^/]")
		default:
			b.WriteString(regexp.QuoteMeta(string(ch)))
		}
	}
	re := b.String()
	if anchored {
		re = "^" + re
	} else {
		re = "(^|/)" + re
	}
	if dirOnly {
		re += "(/.*)?$"
	} else {
		re += "$"
	}
	return regexp.Compile(re)
}
