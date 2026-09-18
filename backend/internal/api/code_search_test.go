package api

import (
	"reflect"
	"testing"
)

func TestParseCodeQuery(t *testing.T) {
	cases := []struct {
		raw  string
		want codeQuery
	}{
		{"hello world", codeQuery{Keyword: "hello world"}},
		{"repo:acme/web lang:go path:src/ foo", codeQuery{Keyword: "foo", Repo: "acme/web", Lang: "go", Path: "src/"}},
		{"symbol:Foo", codeQuery{Keyword: "Foo", Symbol: "Foo"}},
		{"foo:bar baz", codeQuery{Keyword: "foo:bar baz"}},
	}
	for _, tc := range cases {
		if got := parseCodeQuery(tc.raw); got != tc.want {
			t.Errorf("parseCodeQuery(%q) = %+v, want %+v", tc.raw, got, tc.want)
		}
	}
}

func TestBuildPathspecs(t *testing.T) {
	cases := []struct {
		lang, path string
		want       []string
	}{
		{"", "", nil},
		{"go", "", []string{"*.go"}},
		{"", "src", []string{"src"}},
		{"go", "src/", []string{"src/*.go"}},
		{"ts", "src", []string{"src/*.ts", "src/*.tsx"}},
		{"weird", "a/b", []string{"a/b/*.weird"}},
	}
	for _, tc := range cases {
		got := buildPathspecs(tc.lang, tc.path)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("buildPathspecs(%q,%q) = %v, want %v", tc.lang, tc.path, got, tc.want)
		}
	}
}
