package tests

import (
	"os"
	"path/filepath"
	"testing"

	"gitdash/backend/internal/api"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

// 回归测试：默认配置（相对路径 data 目录）下，用 readme 模板创建仓库时
// InitReadme 会在临时目录执行 git push，相对的 RepoPath 会指向不存在的路径。
func TestRepoReadmeTemplateRelativeDataDir(t *testing.T) {
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := gitsvc.Init("./data"); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	t.Cleanup(func() { _ = gitsvc.Init(oldWd) })

	st, err := store.Open(filepath.Join("./data", "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	hs := startHTTPOnly(t, api.New(st, "test"))
	env := &Env{t: t, BaseURL: hs.URL, ReposDir: gitsvc.ReposDir(), DataDir: dir}

	alice := register(t, env, "reluser", "rel-pass-123")
	m := alice.mustStatus("POST", "/repos",
		map[string]string{"name": "relreadme", "template": "readme"}, 201)
	if m["name"] != "relreadme" {
		t.Fatalf("create = %v", m)
	}

	b := alice.mustStatus("GET", "/repos/relreadme/blob?ref=main&path=README.md", nil, 200)
	if b["content"] != "# relreadme\n" {
		t.Fatalf("readme content = %q", b["content"])
	}
}
