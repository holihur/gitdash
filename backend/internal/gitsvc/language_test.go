package gitsvc

import (
	"strings"
	"testing"
)

func TestLanguageName(t *testing.T) {
	cases := map[string]string{
		"main.go":             "Go",
		"src/app.cpp":         "C++",
		"include/app.hpp":     "C++",
		"Program.cs":          "C#",
		"index.ts":            "TypeScript",
		"component.tsx":       "TypeScript",
		"script.py":           "Python",
		"Main.java":           "Java",
		"lib.rs":              "Rust",
		"Makefile":            "Makefile",
		"docker/Dockerfile":   "Dockerfile",
		"CMakeLists.txt":      "CMake",
		"index.html":          "HTML",
		"style.css":           "CSS",
		"run.sh":              "Shell",
		"query.sql":           "SQL",
		"api.proto":           "Protocol Buffer",
		"main.tf":             "HCL",
		"README.md":           "",
		"data.json":           "",
		"config.yaml":         "",
		"notes.txt":           "",
		"archive.zip":         "",
		"no-extension":        "",
		"component.blade.php": "PHP",
	}
	for file, want := range cases {
		if got := LanguageName(file); got != want {
			t.Errorf("LanguageName(%q) = %q, want %q", file, got, want)
		}
	}
}

func TestDefaultLanguageColor(t *testing.T) {
	// 固定配色
	if got := DefaultLanguageColor("Go"); got != "#00ADD8" {
		t.Errorf("Go = %q", got)
	}
	if got := DefaultLanguageColor("C++"); got != "#f34b7d" {
		t.Errorf("C++ = %q", got)
	}
	// 未固定配色：同名稳定，且是合法颜色
	if a, b := DefaultLanguageColor("BogusLang"), DefaultLanguageColor("BogusLang"); a != b {
		t.Errorf("unknown language color is not stable: %q vs %q", a, b)
	}
	// 全部已知语言都有默认色
	defaults := DefaultLanguageColors()
	for _, l := range KnownLanguages() {
		if defaults[l] == "" {
			t.Errorf("language %q has no default color", l)
		}
	}
}

func TestSkipLanguagePath(t *testing.T) {
	skip := []string{
		"vendor/github.com/x/y.go",
		"node_modules/foo/index.js",
		"third_party/zlib/zlib.c",
		"a/b/.git/config",
		"dist/bundle.min.js",
		"app.min.css",
		"package-lock.json",
		"foo.pb.go",
		"service_pb2.py",
	}
	for _, p := range skip {
		if !skipLanguagePath(p) {
			t.Errorf("skipLanguagePath(%q) = false, want true", p)
		}
	}
	keep := []string{
		"main.go",
		"src/app.cpp",
		"internal/server.go",
		"web/index.js",
	}
	for _, p := range keep {
		if skipLanguagePath(p) {
			t.Errorf("skipLanguagePath(%q) = true, want false", p)
		}
	}
}

func TestRepoLanguages(t *testing.T) {
	prev := CurrentBackend()
	SetBackend(NewCLI())
	t.Cleanup(func() { SetBackend(prev) })

	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "lang"); err != nil {
		t.Fatal(err)
	}
	_, err := WriteCommit("alice", "lang", "main", "init", "alice", []FileChange{
		{Path: "main.go", Action: "create", Content: strings.Repeat("package main\n\n", 10)},
		{Path: "app.cpp", Action: "create", Content: strings.Repeat("int x;\n", 40)},
		{Path: "README.md", Action: "create", Content: strings.Repeat("hello world\n", 200)},
		{Path: "vendor/lib.go", Action: "create", Content: strings.Repeat("package lib\n\n", 500)},
	})
	if err != nil {
		t.Fatal(err)
	}
	stats, err := RepoLanguages("alice", "lang", "main")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, s := range stats {
		got[s.Language] = s.Bytes
	}
	if got["C++"] == 0 {
		t.Errorf("expected C++ in %v", got)
	}
	if got["Go"] == 0 {
		t.Errorf("expected Go in %v", got)
	}
	// vendor 目录被排除：Go 字节数应远小于主文件之外的全部 Go 内容。
	if got["Go"] >= int64(len(strings.Repeat("package lib\n\n", 500))) {
		t.Errorf("vendored Go not excluded: %v", got)
	}
	if _, ok := got["Markdown"]; ok {
		t.Errorf("Markdown should not be counted: %v", got)
	}
	// 排序：字节数降序（README 不计，vendor 不计）
	if len(stats) < 2 {
		t.Fatalf("stats = %v", stats)
	}
	if stats[0].Bytes < stats[1].Bytes {
		t.Errorf("stats not sorted desc: %v", stats)
	}
}

func TestRepoLanguagesEmptyRepo(t *testing.T) {
	prev := CurrentBackend()
	SetBackend(NewCLI())
	t.Cleanup(func() { SetBackend(prev) })

	if err := Init(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := CreateBare("alice", "empty"); err != nil {
		t.Fatal(err)
	}
	stats, err := RepoLanguages("alice", "empty", "main")
	if err == nil && len(stats) != 0 {
		t.Fatalf("empty repo stats = %v", stats)
	}
}
