package store

import (
	"path/filepath"
	"testing"
)

func TestRepoLanguagesReplaceAndQuery(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "lang.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "", false); err != nil {
		t.Fatal(err)
	}

	stats := []LanguageStat{
		{Language: "Go", Bytes: 300},
		{Language: "C++", Bytes: 100},
		{Language: "Python", Bytes: 200},
	}
	if err := s.ReplaceRepoLanguages("alice", "demo", "abc123", stats); err != nil {
		t.Fatal(err)
	}

	got, err := s.RepoLanguages("alice", "demo", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("languages = %v", got)
	}
	// 按字节降序
	if got[0].Language != "Go" || got[1].Language != "Python" || got[2].Language != "C++" {
		t.Errorf("order = %v", got)
	}
	// 占比：总量 600
	if got[0].Percent < 49.9 || got[0].Percent > 50.1 {
		t.Errorf("Go percent = %v", got[0].Percent)
	}

	// top-2 截断
	top, err := s.RepoLanguages("alice", "demo", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(top) != 2 || top[0].Language != "Go" || top[1].Language != "Python" {
		t.Errorf("top = %v", top)
	}

	// 主语言批量查询
	m := s.PrimaryLanguages([][2]string{{"alice", "demo"}, {"alice", "missing"}})
	if m[[2]string{"alice", "demo"}] != "Go" {
		t.Errorf("primary = %v", m)
	}
	if _, ok := m[[2]string{"alice", "missing"}]; ok {
		t.Errorf("missing repo should not be returned: %v", m)
	}

	// 替换为更少语言（幂等重算）
	if err := s.ReplaceRepoLanguages("alice", "demo", "def456", []LanguageStat{{Language: "Rust", Bytes: 10}}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.RepoLanguages("alice", "demo", 0)
	if len(got) != 1 || got[0].Language != "Rust" {
		t.Fatalf("after replace: %v", got)
	}
	if sha := s.RepoLanguageSHA("alice", "demo"); sha != "def456" {
		t.Errorf("sha = %q", sha)
	}

	// 空结果也会写 meta，回填不再返回该仓库
	if err := s.ReplaceRepoLanguages("alice", "demo", "ghi789", nil); err != nil {
		t.Fatal(err)
	}
	missing, err := s.ReposMissingLanguages(0, 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range missing {
		if m.Owner == "alice" && m.Repo == "demo" {
			t.Errorf("analyzed repo returned as missing: %v", m)
		}
	}
}

func TestReposMissingLanguagesCursor(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "lang.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a", "b", "c"} {
		if _, err := s.CreateRepo("alice", name, "", false); err != nil {
			t.Fatal(err)
		}
	}
	first, err := s.ReposMissingLanguages(0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 2 {
		t.Fatalf("first batch = %v", first)
	}
	second, err := s.ReposMissingLanguages(first[len(first)-1].ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(second) != 1 {
		t.Fatalf("second batch = %v", second)
	}
	if second[0].ID <= first[len(first)-1].ID {
		t.Errorf("cursor did not advance: first=%v second=%v", first, second)
	}
}

func TestDeleteRepoRemovesLanguages(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "lang.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "", false); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceRepoLanguages("alice", "demo", "sha", []LanguageStat{{Language: "Go", Bytes: 1}}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteRepo("alice", "demo"); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.RepoLanguages("alice", "demo", 0); len(got) != 0 {
		t.Errorf("languages survived repo delete: %v", got)
	}
}
