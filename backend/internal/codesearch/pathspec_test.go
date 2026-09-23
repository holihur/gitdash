package codesearch

import "testing"

func TestMatchPathspecs(t *testing.T) {
	cases := []struct {
		patterns []string
		file     string
		want     bool
	}{
		{nil, "src/app.go", true},
		{[]string{"*.go"}, "src/app.go", true},
		{[]string{"*.go"}, "src/app.js", false},
		{[]string{"*.go"}, "app.go", true},
		{[]string{"src/"}, "src/app.go", true},
		{[]string{"src"}, "src/app.go", true},
		{[]string{"src/*.go"}, "src/app.go", true},
		{[]string{"src/*.go"}, "other/app.go", false},
		{[]string{"*.go", "*.md"}, "docs/README.md", true},
		{[]string{"*.go", "*.md"}, "docs/README.txt", false},
	}
	for _, tc := range cases {
		if got := matchPathspecs(tc.patterns, tc.file); got != tc.want {
			t.Errorf("matchPathspecs(%v, %q) = %v, want %v", tc.patterns, tc.file, got, tc.want)
		}
	}
}

func TestContainsAll(t *testing.T) {
	cases := []struct {
		text  string
		terms []string
		want  bool
	}{
		{"func main() {", []string{"func", "main"}, true},
		{"func main() {", []string{"func", "missing"}, false},
		{"hello world", []string{"hello"}, true},
		{"hello world", []string{"HELLO"}, false}, // 区分大小写，与 git grep 一致
		{"anything", nil, true},
		{"anything", []string{"", "any"}, true},
	}
	for _, tc := range cases {
		if got := containsAll(tc.text, tc.terms); got != tc.want {
			t.Errorf("containsAll(%q, %v) = %v, want %v", tc.text, tc.terms, got, tc.want)
		}
	}
}
