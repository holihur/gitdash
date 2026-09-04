package gitsvc

import "testing"

func TestValidName(t *testing.T) {
	cases := []struct {
		name string
		ok   bool
	}{
		{"demo", true},
		{"my-repo", true},
		{"repo.name_01", true},
		{"", false},
		{"-leading", false},
		{".hidden", false},
		{"..", false},
		{"a/b", false},
		{"a b", false},
		{"a\tb", false},
	}
	for _, c := range cases {
		if got := ValidName(c.name); got != c.ok {
			t.Errorf("ValidName(%q) = %v, want %v", c.name, got, c.ok)
		}
	}
}

func TestValidRef(t *testing.T) {
	cases := []struct {
		ref string
		ok  bool
	}{
		{"main", true},
		{"feature/x", true},
		{"v1.0.0", true},
		{"", false},
		{"-evil", false},
		{"a..b", false},
		{"has space", false},
	}
	for _, c := range cases {
		if got := ValidRef(c.ref); got != c.ok {
			t.Errorf("ValidRef(%q) = %v, want %v", c.ref, got, c.ok)
		}
	}
}

func TestCleanPath(t *testing.T) {
	if p, err := CleanPath("src/main.go"); err != nil || p != "src/main.go" {
		t.Errorf("CleanPath simple = %q, %v", p, err)
	}
	if p, err := CleanPath("/src/main.go/"); err != nil || p != "src/main.go" {
		t.Errorf("CleanPath trim = %q, %v", p, err)
	}
	if _, err := CleanPath("a/../b"); err == nil {
		t.Error("CleanPath traversal should fail")
	}
	if _, err := CleanPath("a//b"); err == nil {
		t.Error("CleanPath empty segment should fail")
	}
	if p, err := CleanPath(""); err != nil || p != "" {
		t.Errorf("CleanPath empty = %q, %v", p, err)
	}
}
