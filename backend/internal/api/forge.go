package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// forgeHTTPClient 访问第三方 forge API 的 HTTP 客户端（带超时，避免慢响应挂住请求）。
var forgeHTTPClient = &http.Client{Timeout: 20 * time.Second}

// forgeMaxRepos 单次列表最多返回的远程仓库数（分页拉取上限）。
const forgeMaxRepos = 300

// forgeConfig 是某个第三方代码托管平台的连接配置。
type forgeConfig struct {
	Name         string
	Label        string
	Enabled      bool
	BaseURL      string // 网页基址（无尾斜杠）
	APIBase      string // API 基址
	ClientID     string
	ClientSecret string
	Scope        string
}

// forgeRepo 是归一化后的远程仓库条目。
type forgeRepo struct {
	FullName      string `json:"full_name"`
	Name          string `json:"name"`
	Owner         string `json:"owner"`
	Private       bool   `json:"private"`
	CloneURL      string `json:"clone_url"`
	DefaultBranch string `json:"default_branch"`
	Description   string `json:"description"`
}

// forgeUser 是第三方账号信息。
type forgeUser struct {
	ExternalID string
	Login      string
	AvatarURL  string
}

// forgeProviderNames 是支持绑定的平台（顺序即 UI 展示顺序）。
var forgeProviderNames = []string{"github", "gitlab", "gitea", "bitbucket"}

// forgeConfigFor 组装某平台的连接配置；未知平台返回 ok=false，未启用时 Enabled=false。
func (a *API) forgeConfigFor(provider string) (forgeConfig, bool) {
	switch provider {
	case "github":
		enabled := a.store.GetSetting("github_oauth_enabled") == "1"
		id := a.store.GetSetting("github_client_id")
		secret := a.store.GetSetting("github_client_secret")
		return forgeConfig{
			Name: "github", Label: "GitHub",
			Enabled: enabled && id != "" && secret != "",
			BaseURL: a.githubBase(), APIBase: a.githubAPIBase(),
			ClientID: id, ClientSecret: secret,
			Scope: "read:user user:email repo",
		}, true
	case "gitlab":
		base := strings.TrimRight(defaultStr(a.store.GetSetting("gitlab_base_url"), "https://gitlab.com"), "/")
		enabled := a.store.GetSetting("gitlab_enabled") == "1"
		id := a.store.GetSetting("gitlab_client_id")
		secret := a.store.GetSetting("gitlab_client_secret")
		return forgeConfig{
			Name: "gitlab", Label: "GitLab",
			Enabled: enabled && id != "" && secret != "" && base != "",
			BaseURL: base, APIBase: base + "/api/v4",
			ClientID: id, ClientSecret: secret,
			Scope: "read_user read_api read_repository",
		}, true
	case "gitea":
		base := strings.TrimRight(strings.TrimSpace(a.store.GetSetting("gitea_base_url")), "/")
		enabled := a.store.GetSetting("gitea_enabled") == "1"
		id := a.store.GetSetting("gitea_client_id")
		secret := a.store.GetSetting("gitea_client_secret")
		return forgeConfig{
			Name: "gitea", Label: "Gitea",
			Enabled: enabled && id != "" && secret != "" && base != "",
			BaseURL: base, APIBase: base + "/api/v1",
			ClientID: id, ClientSecret: secret,
			Scope: "read:user read:repository",
		}, true
	case "bitbucket":
		enabled := a.store.GetSetting("bitbucket_enabled") == "1"
		id := a.store.GetSetting("bitbucket_client_id")
		secret := a.store.GetSetting("bitbucket_client_secret")
		return forgeConfig{
			Name: "bitbucket", Label: "Bitbucket",
			Enabled: enabled && id != "" && secret != "",
			BaseURL: "https://bitbucket.org", APIBase: "https://api.bitbucket.org/2.0",
			ClientID: id, ClientSecret: secret,
			Scope: "account repository",
		}, true
	}
	return forgeConfig{}, false
}

// forgeAuthorizeURL 构造平台授权页 URL（授权码模式）。
func forgeAuthorizeURL(cfg forgeConfig, redirect, state string) string {
	q := url.Values{}
	q.Set("client_id", cfg.ClientID)
	q.Set("redirect_uri", redirect)
	q.Set("response_type", "code")
	q.Set("state", state)
	if cfg.Scope != "" {
		q.Set("scope", cfg.Scope)
	}
	switch cfg.Name {
	case "gitlab":
		return cfg.BaseURL + "/oauth/authorize?" + q.Encode()
	case "bitbucket":
		return cfg.BaseURL + "/site/oauth2/authorize?" + q.Encode()
	default: // github / gitea
		return cfg.BaseURL + "/login/oauth/authorize?" + q.Encode()
	}
}

// forgeToken 是 token 交换结果。
type forgeToken struct {
	AccessToken  string
	RefreshToken string
	Scope        string
}

// forgeExchangeToken 用授权码换取访问令牌。
func forgeExchangeToken(ctx context.Context, cfg forgeConfig, code, redirect string) (forgeToken, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirect)
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", cfg.ClientSecret)

	var endpoint string
	useBasicAuth := false
	switch cfg.Name {
	case "gitlab":
		endpoint = cfg.BaseURL + "/oauth/token"
	case "bitbucket":
		// Bitbucket 要求 client credentials 走 Basic，正文不再重复携带。
		endpoint = cfg.BaseURL + "/site/oauth2/access_token"
		useBasicAuth = true
		form.Del("client_id")
		form.Del("client_secret")
	default: // github / gitea
		endpoint = cfg.BaseURL + "/login/oauth/access_token"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return forgeToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	if useBasicAuth {
		req.SetBasicAuth(cfg.ClientID, cfg.ClientSecret)
	}

	resp, err := forgeHTTPClient.Do(req)
	if err != nil {
		return forgeToken{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return forgeToken{}, fmt.Errorf("%s token exchange failed: %d %s", cfg.Name, resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		Error        string `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return forgeToken{}, err
	}
	if out.AccessToken == "" {
		if out.Error != "" {
			return forgeToken{}, fmt.Errorf("%s token exchange failed: %s", cfg.Name, out.Error)
		}
		return forgeToken{}, fmt.Errorf("%s token exchange failed", cfg.Name)
	}
	return forgeToken{AccessToken: out.AccessToken, RefreshToken: out.RefreshToken, Scope: out.Scope}, nil
}

// forgeGetJSON 发起带 Bearer 的 GET 请求并解码 JSON。
func forgeGetJSON(ctx context.Context, rawURL, token string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "gitdash")
	resp, err := forgeHTTPClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return resp.StatusCode, fmt.Errorf("%s api error: %d %s", rawURL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return resp.StatusCode, json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(out)
}

// forgeFetchUser 拉取第三方账号资料。
func forgeFetchUser(ctx context.Context, cfg forgeConfig, token string) (forgeUser, error) {
	switch cfg.Name {
	case "github":
		var u struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		}
		if _, err := forgeGetJSON(ctx, cfg.APIBase+"/user", token, &u); err != nil {
			return forgeUser{}, err
		}
		return forgeUser{ExternalID: strconv.FormatInt(u.ID, 10), Login: u.Login, AvatarURL: u.AvatarURL}, nil
	case "gitlab":
		var u struct {
			ID        int64  `json:"id"`
			Username  string `json:"username"`
			AvatarURL string `json:"avatar_url"`
		}
		if _, err := forgeGetJSON(ctx, cfg.APIBase+"/user", token, &u); err != nil {
			return forgeUser{}, err
		}
		return forgeUser{ExternalID: strconv.FormatInt(u.ID, 10), Login: u.Username, AvatarURL: u.AvatarURL}, nil
	case "gitea":
		var u struct {
			ID        int64  `json:"id"`
			Login     string `json:"login"`
			AvatarURL string `json:"avatar_url"`
		}
		if _, err := forgeGetJSON(ctx, cfg.APIBase+"/user", token, &u); err != nil {
			return forgeUser{}, err
		}
		return forgeUser{ExternalID: strconv.FormatInt(u.ID, 10), Login: u.Login, AvatarURL: u.AvatarURL}, nil
	case "bitbucket":
		var u struct {
			UUID        string `json:"uuid"`
			Username    string `json:"username"`
			DisplayName string `json:"display_name"`
			Links       struct {
				Avatar struct {
					Href string `json:"href"`
				} `json:"avatar"`
			} `json:"links"`
		}
		if _, err := forgeGetJSON(ctx, cfg.APIBase+"/user", token, &u); err != nil {
			return forgeUser{}, err
		}
		login := u.Username
		if login == "" {
			login = u.DisplayName
		}
		return forgeUser{ExternalID: u.UUID, Login: login, AvatarURL: u.Links.Avatar.Href}, nil
	}
	return forgeUser{}, fmt.Errorf("unsupported provider %q", cfg.Name)
}

// forgeListRepos 列出账号可访问的远程仓库（跨页聚合，最多 forgeMaxRepos 条）。
func forgeListRepos(ctx context.Context, cfg forgeConfig, token string, user forgeUser) ([]forgeRepo, error) {
	switch cfg.Name {
	case "github":
		return forgeListGitHub(ctx, cfg, token)
	case "gitlab":
		return forgeListGitLab(ctx, cfg, token)
	case "gitea":
		return forgeListGitea(ctx, cfg, token)
	case "bitbucket":
		return forgeListBitbucket(ctx, cfg, token, user)
	}
	return nil, fmt.Errorf("unsupported provider %q", cfg.Name)
}

func forgeListGitHub(ctx context.Context, cfg forgeConfig, token string) ([]forgeRepo, error) {
	var out []forgeRepo
	for page := 1; page <= 3; page++ {
		var rows []struct {
			FullName      string `json:"full_name"`
			Name          string `json:"name"`
			Private       bool   `json:"private"`
			CloneURL      string `json:"clone_url"`
			DefaultBranch string `json:"default_branch"`
			Description   string `json:"description"`
			Owner         struct {
				Login string `json:"login"`
			} `json:"owner"`
		}
		u := fmt.Sprintf("%s/user/repos?per_page=100&page=%d&sort=updated&affiliation=owner,collaborator,organization_member", cfg.APIBase, page)
		if _, err := forgeGetJSON(ctx, u, token, &rows); err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, forgeRepo{
				FullName: r.FullName, Name: r.Name, Owner: r.Owner.Login, Private: r.Private,
				CloneURL: r.CloneURL, DefaultBranch: r.DefaultBranch, Description: r.Description,
			})
		}
		if len(rows) < 100 || len(out) >= forgeMaxRepos {
			break
		}
	}
	return out, nil
}

func forgeListGitLab(ctx context.Context, cfg forgeConfig, token string) ([]forgeRepo, error) {
	var out []forgeRepo
	for page := 1; page <= 3; page++ {
		var rows []struct {
			PathWithNamespace string `json:"path_with_namespace"`
			Name              string `json:"name"`
			Visibility        string `json:"visibility"`
			HTTPURLToRepo     string `json:"http_url_to_repo"`
			DefaultBranch     string `json:"default_branch"`
			Description       string `json:"description"`
			Namespace         struct {
				FullPath string `json:"full_path"`
			} `json:"namespace"`
		}
		u := fmt.Sprintf("%s/projects?membership=true&per_page=100&page=%d&order_by=last_activity_at", cfg.APIBase, page)
		if _, err := forgeGetJSON(ctx, u, token, &rows); err != nil {
			return nil, err
		}
		for _, r := range rows {
			owner := r.Namespace.FullPath
			if owner == "" {
				if i := strings.LastIndex(r.PathWithNamespace, "/"); i > 0 {
					owner = r.PathWithNamespace[:i]
				}
			}
			out = append(out, forgeRepo{
				FullName: r.PathWithNamespace, Name: r.Name, Owner: owner,
				Private:       !strings.EqualFold(r.Visibility, "public"),
				CloneURL:      r.HTTPURLToRepo,
				DefaultBranch: r.DefaultBranch, Description: r.Description,
			})
		}
		if len(rows) < 100 || len(out) >= forgeMaxRepos {
			break
		}
	}
	return out, nil
}

func forgeListGitea(ctx context.Context, cfg forgeConfig, token string) ([]forgeRepo, error) {
	var out []forgeRepo
	for page := 1; page <= 3; page++ {
		var rows []struct {
			FullName      string `json:"full_name"`
			Name          string `json:"name"`
			Private       bool   `json:"private"`
			CloneURL      string `json:"clone_url"`
			DefaultBranch string `json:"default_branch"`
			Description   string `json:"description"`
			Owner         struct {
				Login string `json:"login"`
			} `json:"owner"`
		}
		u := fmt.Sprintf("%s/user/repos?page=%d&limit=100", cfg.APIBase, page)
		if _, err := forgeGetJSON(ctx, u, token, &rows); err != nil {
			return nil, err
		}
		for _, r := range rows {
			out = append(out, forgeRepo{
				FullName: r.FullName, Name: r.Name, Owner: r.Owner.Login, Private: r.Private,
				CloneURL: r.CloneURL, DefaultBranch: r.DefaultBranch, Description: r.Description,
			})
		}
		if len(rows) < 100 || len(out) >= forgeMaxRepos {
			break
		}
	}
	return out, nil
}

func forgeListBitbucket(ctx context.Context, cfg forgeConfig, token string, user forgeUser) ([]forgeRepo, error) {
	workspace := user.Login
	if workspace == "" {
		workspace = user.ExternalID
	}
	var out []forgeRepo
	next := fmt.Sprintf("%s/repositories/%s?pagelen=100&role=member", cfg.APIBase, url.PathEscape(workspace))
	for page := 0; page < 3 && next != ""; page++ {
		var resp struct {
			Values []struct {
				FullName    string `json:"full_name"`
				Name        string `json:"name"`
				IsPrivate   bool   `json:"is_private"`
				Description string `json:"description"`
				MainBranch  struct {
					Name string `json:"name"`
				} `json:"mainbranch"`
				Links struct {
					Clone []struct {
						Name string `json:"name"`
						Href string `json:"href"`
					} `json:"clone"`
				} `json:"links"`
			} `json:"values"`
			Next string `json:"next"`
		}
		if _, err := forgeGetJSON(ctx, next, token, &resp); err != nil {
			return nil, err
		}
		for _, r := range resp.Values {
			clone := ""
			for _, c := range r.Links.Clone {
				if c.Name == "https" {
					clone = c.Href
					break
				}
			}
			owner := ""
			if i := strings.Index(r.FullName, "/"); i > 0 {
				owner = r.FullName[:i]
			}
			out = append(out, forgeRepo{
				FullName: r.FullName, Name: r.Name, Owner: owner, Private: r.IsPrivate,
				CloneURL: clone, DefaultBranch: r.MainBranch.Name, Description: r.Description,
			})
		}
		next = resp.Next
		if len(out) >= forgeMaxRepos {
			break
		}
	}
	return out, nil
}

// forgeCredential 返回 HTTPS Basic 认证凭据 "username:token"，供 GIT_ASKPASS 使用。
func forgeCredential(cfg forgeConfig, login, token string) string {
	switch cfg.Name {
	case "github":
		return "x-access-token:" + token
	case "gitlab":
		return "oauth2:" + token
	case "bitbucket":
		return "x-token-auth:" + token
	case "gitea":
		if login != "" {
			return login + ":" + token
		}
		return "oauth2:" + token
	}
	return "oauth2:" + token
}
