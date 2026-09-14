package api

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// uiOnlyErrorKeys 是纯前端展示用、不由后端错误码产生的 i18n key（后端来源是 DB 状态字段）。
var uiOnlyErrorKeys = map[string]bool{
	"import_failed":      true,
	"mirror_sync_failed": true,
}

// backendStringLiterals 收集 api 包（非测试）源码里的全部字符串字面量。
// 错误码可能由 writeCode(...) 直接给出，也可能由 passwordIssue 之类的辅助函数返回，
// 因此这里不做调用点匹配，只做「字面量存在性」契约，避免机制变化导致漏检。
func backendStringLiterals(t *testing.T, dir string) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read api dir: %v", err)
	}
	fset := token.NewFileSet()
	out := map[string]bool{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", name, perr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if s, uerr := strconv.Unquote(lit.Value); uerr == nil {
				out[s] = true
			}
			return true
		})
	}
	return out
}

// frontendErrorKeys 解析 en.ts 中 errors: { ... } 块的键。
func frontendErrorKeys(t *testing.T, path string) map[string]bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	m := regexp.MustCompile(`(?s)\n  errors: \{(.*?)\n  \},`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatalf("could not locate errors block in %s", path)
	}
	out := map[string]bool{}
	for _, k := range regexp.MustCompile(`(?m)^\s{4}([a-z0-9_]+):`).FindAllStringSubmatch(m[1], -1) {
		out[k[1]] = true
	}
	return out
}

// TestFrontendErrorKeysHaveBackendSource 防契约漂移：前端 errors.* 的每个 key
// 都必须在后端 api 包源码里存在对应字符串（错误码被改名/删除即失败）。
func TestFrontendErrorKeysHaveBackendSource(t *testing.T) {
	backend := backendStringLiterals(t, ".")
	keys := frontendErrorKeys(t, filepath.Join("..", "..", "..", "frontend", "src", "locales", "en.ts"))
	if len(keys) == 0 {
		t.Fatal("no frontend errors.* keys parsed (locale path/format changed?)")
	}
	if len(backend) < 100 {
		t.Fatalf("suspiciously few backend string literals parsed: %d", len(backend))
	}

	var stale []string
	for k := range keys {
		if !backend[k] && !uiOnlyErrorKeys[k] {
			stale = append(stale, k)
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		t.Fatalf("frontend errors.* keys with no backend string literal (contract drift): %v", stale)
	}
	t.Logf("error-code contract ok: %d frontend keys, %d backend literals", len(keys), len(backend))
}
