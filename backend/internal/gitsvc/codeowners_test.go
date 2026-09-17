package gitsvc

import (
	"reflect"
	"testing"
)

func TestParseCodeownersAndOwners(t *testing.T) {
	content := "# owners\n" +
		"*            @alice\n" +
		"*.go         @bob\n" +
		"/docs/       @carol\n" +
		"src/**/*.ts  @dave @erin\n" +
		"vendor/      @frank\n" +
		"\n" +
		"# emails/teams ignored\n" +
		"config/  dev@example.com  @org/team\n"
	co := ParseCodeowners(content)

	cases := []struct {
		path string
		want []string
	}{
		{"README.md", []string{"alice"}},
		{"main.go", []string{"bob"}},
		{"pkg/util.go", []string{"bob"}},
		{"docs/guide.md", []string{"carol"}},
		{"docs/sub/x.md", []string{"carol"}},
		{"src/c.ts", []string{"dave", "erin"}},
		{"src/a/b/c.ts", []string{"dave", "erin"}},
		{"vendor/x.js", []string{"frank"}},
		{"vendor/sub/y.js", []string{"frank"}},
		{"other.txt", []string{"alice"}},
		{"config/app.yml", []string{"alice"}}, // 无效 owner 的规则被忽略，回退到 `*`
	}
	for _, tc := range cases {
		got := co.Owners(tc.path)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("Owners(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}

	if co.Owners("README.md")[0] != "alice" {
		t.Fatal("expected alice for README.md")
	}
}

func TestParseCodeownersLastMatchWins(t *testing.T) {
	co := ParseCodeowners("* @a\nfoo/bar.txt @b\n")
	if got := co.Owners("foo/bar.txt"); !reflect.DeepEqual(got, []string{"b"}) {
		t.Fatalf("last match should win: %v", got)
	}
	if got := co.Owners("foo/other.txt"); !reflect.DeepEqual(got, []string{"a"}) {
		t.Fatalf("fallback wildcard: %v", got)
	}
}
