package store

import (
	"path/filepath"
	"testing"
)

func TestNormalizeTopics(t *testing.T) {
	got, ok := NormalizeTopics([]string{" Go ", "web", "go", "cli"})
	if !ok {
		t.Fatal("valid topics should be accepted")
	}
	want := []string{"go", "web", "cli"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	for _, bad := range [][]string{{"has space"}, {"UPPER!"}, {"-leading"}} {
		if _, ok := NormalizeTopics(bad); ok {
			t.Fatalf("invalid topic %v should be rejected", bad)
		}
	}
	many := make([]string, MaxRepoTopics+1)
	for i := range many {
		many[i] = string(rune('a'+i%26)) + string(rune('0'+i/26))
	}
	if _, ok := NormalizeTopics(many); ok {
		t.Fatal("too many topics should be rejected")
	}
}

func TestRepoTopicsAndExploreFilter(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "web", "web app", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "lib", "a library", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "secret", "private", true); err != nil {
		t.Fatal(err)
	}

	if err := s.SetRepoTopics("alice", "web", []string{"go", "web"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepoTopics("alice", "lib", []string{"go"}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepoTopics("alice", "secret", []string{"go"}); err != nil {
		t.Fatal(err)
	}

	topics, err := s.ListRepoTopics("alice", "web")
	if err != nil || len(topics) != 2 || topics[0] != "go" || topics[1] != "web" {
		t.Fatalf("list topics = %v, err %v", topics, err)
	}

	// 批量
	m := s.TopicsForRepos([][2]string{{"alice", "web"}, {"alice", "lib"}, {"alice", "nope"}})
	if len(m[[2]string{"alice", "web"}]) != 2 || len(m[[2]string{"alice", "lib"}]) != 1 {
		t.Fatalf("batch topics = %v", m)
	}

	// 按 topic 过滤（不含私有仓库）
	repos, err := s.ExploreReposFiltered("", "go", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 {
		t.Fatalf("topic=go -> %d public repos, want 2", len(repos))
	}
	n, err := s.CountExploreReposFiltered("", "go")
	if err != nil || n != 2 {
		t.Fatalf("count topic=go = %d, err %v", n, err)
	}

	// 关键词过滤
	repos, err = s.ExploreReposFiltered("library", "", 0, 0)
	if err != nil || len(repos) != 1 || repos[0].Name != "lib" {
		t.Fatalf("q=library -> %v, err %v", repos, err)
	}

	// 所有标签仅统计公开仓库
	all, err := s.AllTopics(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	byTopic := map[string]int{}
	for _, tc := range all {
		byTopic[tc.Topic] = tc.Count
	}
	if byTopic["go"] != 2 || byTopic["web"] != 1 {
		t.Fatalf("all topics = %v", all)
	}

	// 替换 topic 后旧值消失
	if err := s.SetRepoTopics("alice", "web", []string{"frontend"}); err != nil {
		t.Fatal(err)
	}
	topics, _ = s.ListRepoTopics("alice", "web")
	if len(topics) != 1 || topics[0] != "frontend" {
		t.Fatalf("after replace = %v", topics)
	}
}
