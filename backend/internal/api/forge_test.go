package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// forgeTestServer 起一个按路径分发的 mock forge API。
func forgeTestServer(t *testing.T, routes map[string]func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h, ok := routes[r.URL.Path]; ok {
			h(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func writeJSONTest(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func TestForgeAuthorizeURL(t *testing.T) {
	cases := []struct {
		name string
		cfg  forgeConfig
		want string
	}{
		{"github", forgeConfig{Name: "github", BaseURL: "https://github.com", ClientID: "cid", Scope: "read:user repo"}, "https://github.com/login/oauth/authorize?"},
		{"gitea", forgeConfig{Name: "gitea", BaseURL: "https://gitea.example.com", ClientID: "cid", Scope: "repo"}, "https://gitea.example.com/login/oauth/authorize?"},
		{"gitlab", forgeConfig{Name: "gitlab", BaseURL: "https://gitlab.example.com", ClientID: "cid", Scope: "read_api"}, "https://gitlab.example.com/oauth/authorize?"},
		{"bitbucket", forgeConfig{Name: "bitbucket", BaseURL: "https://bitbucket.org", ClientID: "cid", Scope: "account"}, "https://bitbucket.org/site/oauth2/authorize?"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := forgeAuthorizeURL(tc.cfg, "https://app/cb", "st")
			if !strings.HasPrefix(got, tc.want) {
				t.Fatalf("authorize url = %q, want prefix %q", got, tc.want)
			}
			if !strings.Contains(got, "state=st") || !strings.Contains(got, "client_id=cid") {
				t.Fatalf("authorize url missing params: %q", got)
			}
		})
	}
}

func TestForgeExchangeToken(t *testing.T) {
	cases := []struct {
		name     string
		cfg      forgeConfig
		path     string
		checkReq func(t *testing.T, r *http.Request)
	}{
		{
			name: "github", cfg: forgeConfig{Name: "github", BaseURL: "%s", ClientID: "cid", ClientSecret: "sec"},
			path: "/login/oauth/access_token",
		},
		{
			name: "gitea", cfg: forgeConfig{Name: "gitea", BaseURL: "%s", ClientID: "cid", ClientSecret: "sec"},
			path: "/login/oauth/access_token",
		},
		{
			name: "gitlab", cfg: forgeConfig{Name: "gitlab", BaseURL: "%s", ClientID: "cid", ClientSecret: "sec"},
			path: "/oauth/token",
		},
		{
			name: "bitbucket", cfg: forgeConfig{Name: "bitbucket", BaseURL: "%s", ClientID: "cid", ClientSecret: "sec"},
			path: "/site/oauth2/access_token",
			checkReq: func(t *testing.T, r *http.Request) {
				user, pass, ok := r.BasicAuth()
				if !ok || user != "cid" || pass != "sec" {
					t.Fatalf("bitbucket token exchange should use basic auth, got %q/%q ok=%v", user, pass, ok)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sawForm string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					http.NotFound(w, r)
					return
				}
				if tc.checkReq != nil {
					tc.checkReq(t, r)
				}
				_ = r.ParseForm()
				sawForm = r.Form.Encode()
				writeJSONTest(w, map[string]string{"access_token": "tok-" + tc.name, "refresh_token": "ref", "scope": "repo"})
			}))
			defer srv.Close()

			cfg := tc.cfg
			cfg.BaseURL = srv.URL
			tok, err := forgeExchangeToken(context.Background(), cfg, "the-code", "https://app/cb")
			if err != nil {
				t.Fatalf("exchange: %v", err)
			}
			if tok.AccessToken != "tok-"+tc.name || tok.RefreshToken != "ref" || tok.Scope != "repo" {
				t.Fatalf("token = %+v", tok)
			}
			if !strings.Contains(sawForm, "code=the-code") || !strings.Contains(sawForm, "grant_type=authorization_code") {
				t.Fatalf("token form = %q", sawForm)
			}
		})
	}
}

func TestForgeExchangeTokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		writeJSONTest(w, map[string]string{"error": "bad_verification_code"})
	}))
	defer srv.Close()
	_, err := forgeExchangeToken(context.Background(), forgeConfig{Name: "github", BaseURL: srv.URL, ClientID: "c", ClientSecret: "s"}, "x", "y")
	if err == nil {
		t.Fatal("expected error on non-2xx token exchange")
	}
}

func TestForgeFetchUser(t *testing.T) {
	cases := []struct {
		provider string
		body     any
		want     forgeUser
	}{
		{"github", map[string]any{"id": 42, "login": "octo", "avatar_url": "https://a/g.png"}, forgeUser{ExternalID: "42", Login: "octo", AvatarURL: "https://a/g.png"}},
		{"gitlab", map[string]any{"id": 7, "username": "gl", "avatar_url": "https://a/gl.png"}, forgeUser{ExternalID: "7", Login: "gl", AvatarURL: "https://a/gl.png"}},
		{"gitea", map[string]any{"id": 9, "login": "gt", "avatar_url": "https://a/gt.png"}, forgeUser{ExternalID: "9", Login: "gt", AvatarURL: "https://a/gt.png"}},
		{"bitbucket", map[string]any{"uuid": "{u-1}", "username": "bb", "links": map[string]any{"avatar": map[string]string{"href": "https://a/bb.png"}}}, forgeUser{ExternalID: "{u-1}", Login: "bb", AvatarURL: "https://a/bb.png"}},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/user" {
					http.NotFound(w, r)
					return
				}
				writeJSONTest(w, tc.body)
			}))
			defer srv.Close()
			cfg := forgeConfig{Name: tc.provider, APIBase: srv.URL}
			got, err := forgeFetchUser(context.Background(), cfg, "tok")
			if err != nil {
				t.Fatalf("fetch user: %v", err)
			}
			if got != tc.want {
				t.Fatalf("user = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// TestForgeListReposPagination 覆盖各平台的分页聚合（issue: 远程仓库列表分页聚合缺少测试）。
func TestForgeListReposPagination(t *testing.T) {
	t.Run("github", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/user/repos" {
				http.NotFound(w, r)
				return
			}
			page := r.URL.Query().Get("page")
			rows := make([]map[string]any, 0, 100)
			count := 100
			if page == "2" {
				count = 1
			}
			for i := 0; i < count; i++ {
				rows = append(rows, map[string]any{
					"full_name": fmt.Sprintf("o/r-%s-%d", page, i), "name": fmt.Sprintf("r-%d", i),
					"private": i%2 == 0, "clone_url": "https://github.com/o/r.git", "default_branch": "main",
					"owner": map[string]string{"login": "o"},
				})
			}
			writeJSONTest(w, rows)
		}))
		defer srv.Close()
		repos, err := forgeListRepos(context.Background(), forgeConfig{Name: "github", APIBase: srv.URL}, "tok", forgeUser{})
		if err != nil {
			t.Fatal(err)
		}
		if len(repos) != 101 {
			t.Fatalf("github repos = %d, want 101", len(repos))
		}
		if repos[0].Owner != "o" || !repos[0].Private || repos[0].DefaultBranch != "main" {
			t.Fatalf("normalized repo = %+v", repos[0])
		}
	})

	t.Run("gitea", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/user/repos" {
				http.NotFound(w, r)
				return
			}
			page := r.URL.Query().Get("page")
			count := 100
			if page == "2" {
				count = 3
			}
			rows := make([]map[string]any, 0, count)
			for i := 0; i < count; i++ {
				rows = append(rows, map[string]any{
					"full_name": "o/r", "name": "r", "private": true,
					"clone_url": "https://gitea/o/r.git", "default_branch": "main",
					"owner": map[string]string{"login": "o"},
				})
			}
			writeJSONTest(w, rows)
		}))
		defer srv.Close()
		repos, err := forgeListRepos(context.Background(), forgeConfig{Name: "gitea", APIBase: srv.URL}, "tok", forgeUser{})
		if err != nil {
			t.Fatal(err)
		}
		if len(repos) != 103 {
			t.Fatalf("gitea repos = %d, want 103", len(repos))
		}
	})

	t.Run("gitlab", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/projects" {
				http.NotFound(w, r)
				return
			}
			page := r.URL.Query().Get("page")
			count := 100
			if page == "2" {
				count = 2
			}
			rows := make([]map[string]any, 0, count)
			for i := 0; i < count; i++ {
				rows = append(rows, map[string]any{
					"path_with_namespace": "group/sub/proj", "name": "proj", "visibility": "public",
					"http_url_to_repo": "https://gitlab/group/sub/proj.git", "default_branch": "main",
					"namespace": map[string]string{"full_path": "group/sub"},
				})
			}
			writeJSONTest(w, rows)
		}))
		defer srv.Close()
		repos, err := forgeListRepos(context.Background(), forgeConfig{Name: "gitlab", APIBase: srv.URL}, "tok", forgeUser{})
		if err != nil {
			t.Fatal(err)
		}
		if len(repos) != 102 {
			t.Fatalf("gitlab repos = %d, want 102", len(repos))
		}
		if repos[0].Owner != "group/sub" || repos[0].Private {
			t.Fatalf("gitlab normalized = %+v", repos[0])
		}
	})

	t.Run("bitbucket-next", func(t *testing.T) {
		var srv *httptest.Server
		srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.HasPrefix(r.URL.Path, "/repositories/bb") {
				http.NotFound(w, r)
				return
			}
			page := r.URL.Query().Get("page")
			if page == "2" {
				writeJSONTest(w, map[string]any{
					"values": []map[string]any{{
						"full_name": "bb/two", "name": "two", "is_private": true,
						"mainbranch": map[string]string{"name": "main"},
						"links":      map[string]any{"clone": []map[string]string{{"name": "https", "href": "https://bitbucket/bb/two.git"}}},
					}},
				})
				return
			}
			writeJSONTest(w, map[string]any{
				"values": []map[string]any{{
					"full_name": "bb/one", "name": "one", "is_private": false,
					"mainbranch": map[string]string{"name": "main"},
					"links":      map[string]any{"clone": []map[string]string{{"name": "https", "href": "https://bitbucket/bb/one.git"}}},
				}},
				"next": srv.URL + "/repositories/bb?pagelen=100&role=member&page=2",
			})
		}))
		defer srv.Close()
		repos, err := forgeListRepos(context.Background(), forgeConfig{Name: "bitbucket", APIBase: srv.URL}, "tok", forgeUser{Login: "bb"})
		if err != nil {
			t.Fatal(err)
		}
		if len(repos) != 2 || repos[1].FullName != "bb/two" || !repos[1].Private {
			t.Fatalf("bitbucket repos = %+v", repos)
		}
	})
}

func TestForgeCredential(t *testing.T) {
	cases := map[string]string{
		"github":    "x-access-token:tok",
		"gitlab":    "oauth2:tok",
		"bitbucket": "x-token-auth:tok",
	}
	for provider, want := range cases {
		cfg := forgeConfig{Name: provider}
		if got := forgeCredential(cfg, "user", "tok"); got != want {
			t.Fatalf("%s credential = %q, want %q", provider, got, want)
		}
	}
	// Gitea prefers the login when available.
	if got := forgeCredential(forgeConfig{Name: "gitea"}, "alice", "tok"); got != "alice:tok" {
		t.Fatalf("gitea credential = %q", got)
	}
	if got := forgeCredential(forgeConfig{Name: "gitea"}, "", "tok"); got != "oauth2:tok" {
		t.Fatalf("gitea credential without login = %q", got)
	}
}
