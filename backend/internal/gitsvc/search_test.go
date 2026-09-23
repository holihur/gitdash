package gitsvc

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseGrepLine(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		prefix string
		want   SearchHit
		ok     bool
	}{
		{
			name:   "normal",
			line:   "main:src/app.go:42:\tfmt.Println()",
			prefix: "main:",
			want:   SearchHit{Path: "src/app.go", Line: 42, Text: "\tfmt.Println()"},
			ok:     true,
		},
		{
			name:   "trailing cr trimmed",
			line:   "main:a.txt:1:hello\r",
			prefix: "main:",
			want:   SearchHit{Path: "a.txt", Line: 1, Text: "hello"},
			ok:     true,
		},
		{
			name:   "prefix absent still parses",
			line:   "a.txt:1:hello",
			prefix: "main:",
			want:   SearchHit{Path: "a.txt", Line: 1, Text: "hello"},
			ok:     true,
		},
		{
			name:   "too few fields",
			line:   "main:a.txt:hello",
			prefix: "main:",
			ok:     false,
		},
		{
			name:   "bad line number",
			line:   "main:a.txt:x:hello",
			prefix: "main:",
			ok:     false,
		},
		{
			name:   "zero line number",
			line:   "main:a.txt:0:hello",
			prefix: "main:",
			ok:     false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseGrepLine(tc.line, tc.prefix)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
		})
	}
}

// 超长多字节文本必须按 rune 边界截断，产出合法 UTF-8 且不超过 500 字节。
func TestParseGrepLineTruncatesOnRuneBoundary(t *testing.T) {
	body := strings.Repeat("你", 300) // 900 bytes
	hit, ok := parseGrepLine("main:a.txt:1:"+body, "main:")
	if !ok {
		t.Fatal("expected a hit")
	}
	if len(hit.Text) > 500 {
		t.Fatalf("text length = %d, want <= 500", len(hit.Text))
	}
	if len(hit.Text)%3 != 0 {
		t.Fatalf("text length = %d, not a whole number of 3-byte runes", len(hit.Text))
	}
	if !utf8.ValidString(hit.Text) {
		t.Fatalf("truncated text is not valid UTF-8: %q", hit.Text)
	}
	if hit.Text != strings.Repeat("你", 166) {
		t.Fatalf("unexpected truncated text (len=%d)", len(hit.Text))
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
		{"hello world", []string{"HELLO"}, false}, // 区分大小写，与 git grep 默认一致
		{"anything", nil, true},
	}
	for _, tc := range cases {
		if got := containsAll(tc.text, tc.terms); got != tc.want {
			t.Errorf("containsAll(%q, %v) = %v, want %v", tc.text, tc.terms, got, tc.want)
		}
	}
}
