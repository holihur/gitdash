// gitdash-cli 是 gitdash 的命令行客户端（类似 gh/glab）。
//
// 支持两种登录方式：
//   - 浏览器 OAuth 2.0 设备流（RFC 8628）：`gitdash-cli login` → 选 1
//   - 个人访问令牌 PAT：`gitdash-cli login` → 选 2（或 `--method pat`）
//
// 用法：
//
//	gitdash-cli login                      # 交互式登录（默认浏览器设备流）
//	gitdash-cli login --method pat         # 用 PAT 登录
//	gitdash-cli me
//	gitdash-cli repo list
//	gitdash-cli repo create --private demo
//	gitdash-cli issue list alice/demo
//	gitdash-cli issue create alice/demo --title "Bug" --body "..."
//	gitdash-cli pr list alice/demo
//	gitdash-cli pr create alice/demo --title "Fix" --head feature --base main
package main

import (
	"bufio"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/term"
)

// skillContent 内嵌的 Agent Skill（告诉 Claude Code / opencode / pi 怎么用本 CLI）。
//
//go:embed skill/SKILL.md
var skillContent string

const (
	firstPartyClientID = "gitdash-cli"
	defaultScope       = "repo"
)

// cliVersion 由 ldflags 注入。
var cliVersion = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "gitdash-cli:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	args, host, token, jsonOut := extractGlobals(argv)
	if len(args) == 0 {
		usage()
		return nil
	}
	switch args[0] {
	case "login":
		return cmdLogin(args[1:], host)
	case "logout":
		return cmdLogout()
	case "me":
		cl, err := resolveClient(host, token)
		if err != nil {
			return err
		}
		return cmdMe(cl, jsonOut)
	case "repo":
		return cmdRepo(args[1:], host, token, jsonOut)
	case "issue":
		return cmdIssue(args[1:], host, token, jsonOut)
	case "copilot":
		return cmdCopilot(args[1:], host, token, jsonOut)
	case "pr":
		return cmdPr(args[1:], host, token, jsonOut)
	case "skill":
		return cmdSkill(args[1:])
	case "version", "--version", "-v":
		fmt.Println("gitdash-cli", cliVersion)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	default:
		return fmt.Errorf("unknown command %q (try `gitdash-cli help`)", args[0])
	}
}

func usage() {
	fmt.Print(`gitdash-cli — gitdash command-line client

Usage:
  gitdash-cli <command> [flags]

Commands:
  login                 Authenticate (browser OAuth 2.0 device flow or PAT)
  logout                Remove stored credentials
  me                    Show the authenticated user
  repo list             List your repositories
  repo create [flags] <name>
  issue list <owner/repo>
  issue create <owner/repo> --title <t> [--body <b>]
  issue fix <owner/repo> <issue-number> [--byok <name|id>] [--instructions <text>] [--detach]
  copilot list <owner/repo>
  copilot create <owner/repo> [--byok <name|id>] [--issue <n>] [--prompt <text>]
  copilot run <owner/repo> <session-id> --text <message>
  copilot fix <owner/repo> <issue-number> [--byok <name|id>] [--instructions <text>] [--detach]
  pr list <owner/repo>
  pr create <owner/repo> --title <t> --head <branch> --base <branch> [--body <b>]
  skill show            Print the embedded Agent Skill (SKILL.md)
  skill install         Install the skill for Claude Code / opencode / pi
  version               Print the CLI version

Global flags (anywhere):
  --host <url>          gitdash base URL (overrides config; env GITDASH_HOST)
  --token <token>       API token (overrides config; env GITDASH_TOKEN)
  --json                Print raw JSON output
`)
}

// ---- global flags ----

// extractGlobals 抽离 --host/--token/--json（可出现在任意位置），返回其余参数。
func extractGlobals(args []string) (rest []string, host, token string, jsonOut bool) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--host" && i+1 < len(args):
			host = args[i+1]
			i++
		case strings.HasPrefix(a, "--host="):
			host = strings.TrimPrefix(a, "--host=")
		case a == "--token" && i+1 < len(args):
			token = args[i+1]
			i++
		case strings.HasPrefix(a, "--token="):
			token = strings.TrimPrefix(a, "--token=")
		case a == "--json":
			jsonOut = true
		default:
			rest = append(rest, a)
		}
	}
	return rest, host, token, jsonOut
}

// ---- config ----

type config struct {
	Host  string `json:"host"`
	Token string `json:"token"`
	User  string `json:"user,omitempty"`
}

func configPath() string {
	dir := os.Getenv("GITDASH_CONFIG_DIR")
	if dir == "" {
		if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
			dir = filepath.Join(x, "gitdash")
		} else if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, ".config", "gitdash")
		}
	}
	if dir == "" {
		dir = "."
	}
	return filepath.Join(dir, "config.json")
}

func loadConfig() config {
	var c config
	b, err := os.ReadFile(configPath())
	if err == nil {
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func saveConfig(c config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, b, 0o600)
}

func resolveClient(host, token string) (*client, error) {
	c := loadConfig()
	if host == "" {
		host = os.Getenv("GITDASH_HOST")
	}
	if host == "" {
		host = c.Host
	}
	if token == "" {
		token = os.Getenv("GITDASH_TOKEN")
	}
	if token == "" {
		token = c.Token
	}
	if strings.TrimSpace(host) == "" {
		return nil, errors.New("no host configured; run `gitdash-cli login` or set GITDASH_HOST")
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("not logged in; run `gitdash-cli login` or set GITDASH_TOKEN")
	}
	return &client{host: strings.TrimRight(host, "/"), token: token}, nil
}

// ---- API client ----

type client struct {
	host  string
	token string
}

type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("HTTP %d: %s (%s)", e.Status, e.Message, e.Code)
	}
	return fmt.Sprintf("HTTP %d: %s", e.Status, e.Message)
}

// do 请求 /api 下的接口（自动加 /api 前缀）。
func (c *client) do(method, path string, body []byte, contentType string, out any) error {
	return c.doURL(method, "/api"+path, body, contentType, out)
}

// doURL 请求完整路径（供 /login/oauth/* 等非 /api 端点使用）。
func (c *client) doURL(method, fullPath string, body []byte, contentType string, out any) error {
	req, err := http.NewRequest(method, c.host+fullPath, bytes.NewReader(body))
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("User-Agent", "gitdash-cli/"+cliVersion)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if res.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
			Code  string `json:"code"`
		}
		_ = json.Unmarshal(data, &e)
		return &apiError{Status: res.StatusCode, Code: e.Code, Message: firstNonEmpty(e.Error, res.Status)}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *client) get(path string, out any) error {
	return c.do(http.MethodGet, path, nil, "", out)
}

func (c *client) postJSON(path string, in, out any) error {
	b, err := json.Marshal(in)
	if err != nil {
		return err
	}
	return c.do(http.MethodPost, path, b, "application/json", out)
}

func (c *client) postForm(fullPath string, form url.Values, out any) error {
	return c.doURL(http.MethodPost, fullPath, []byte(form.Encode()), "application/x-www-form-urlencoded", out)
}

func printJSON(v any) {
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---- login / logout ----

func cmdLogin(args []string, hostFlag string) error {
	fs := flag.NewFlagSet("login", flag.ContinueOnError)
	host := fs.String("host", hostFlag, "gitdash base URL")
	method := fs.String("method", "", "authentication method: oauth | pat")
	if err := fs.Parse(args); err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	h := strings.TrimSpace(*host)
	if h == "" {
		h = os.Getenv("GITDASH_HOST")
	}
	if h == "" {
		h = loadConfig().Host
	}
	if h == "" {
		fmt.Print("gitdash host (e.g. http://localhost:8080): ")
		line, _ := reader.ReadString('\n')
		h = strings.TrimSpace(line)
	}
	h = strings.TrimRight(h, "/")
	if h == "" {
		return errors.New("host is required")
	}

	m := strings.ToLower(strings.TrimSpace(*method))
	if m == "" {
		fmt.Println("How do you want to authenticate?")
		fmt.Println("  1) Browser — OAuth 2.0 device flow (recommended)")
		fmt.Println("  2) Personal access token (PAT)")
		fmt.Print("Choice [1]: ")
		line, _ := reader.ReadString('\n')
		if strings.TrimSpace(line) == "2" {
			m = "pat"
		} else {
			m = "oauth"
		}
	}

	var (
		token string
		err   error
	)
	switch m {
	case "pat", "token":
		fmt.Print("Personal access token: ")
		token, err = readSecret(reader)
		if err != nil {
			return err
		}
		token = strings.TrimSpace(token)
	case "oauth", "browser", "device":
		token, err = deviceLogin(h)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown method %q (use oauth or pat)", m)
	}
	if token == "" {
		return errors.New("no token obtained")
	}

	cl := &client{host: h, token: token}
	var me struct {
		Username string `json:"username"`
	}
	if err := cl.get("/me", &me); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}
	c := loadConfig()
	c.Host, c.Token, c.User = h, token, me.Username
	if err := saveConfig(c); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s as %s\n", h, me.Username)
	return nil
}

func cmdLogout() error {
	c := loadConfig()
	c.Token = ""
	c.User = ""
	if err := saveConfig(c); err != nil {
		return err
	}
	fmt.Println("Logged out")
	return nil
}

// deviceLogin 走 OAuth 2.0 设备流，返回 access token。
func deviceLogin(host string) (string, error) {
	cl := &client{host: host}
	var dc struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int    `json:"expires_in"`
		Interval                int    `json:"interval"`
	}
	if err := cl.postForm("/login/oauth/device/code",
		url.Values{"client_id": {firstPartyClientID}, "scope": {defaultScope}}, &dc); err != nil {
		return "", fmt.Errorf("failed to start device flow: %w", err)
	}
	if dc.DeviceCode == "" {
		return "", errors.New("server did not return a device code")
	}

	fmt.Printf("\nFirst, open this URL in your browser:\n  %s\n\n", firstNonEmpty(dc.VerificationURI, host+"/login/oauth/device"))
	fmt.Printf("Then enter the code: %s\n\n", dc.UserCode)
	if dc.VerificationURIComplete != "" {
		_ = openBrowser(dc.VerificationURIComplete)
	}

	interval := time.Duration(dc.Interval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Second
	}
	expiresIn := time.Duration(dc.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = 15 * time.Minute
	}
	deadline := time.Now().Add(expiresIn)
	for time.Now().Before(deadline) {
		time.Sleep(interval)
		var tok struct {
			AccessToken string `json:"access_token"`
		}
		err := cl.postForm("/login/oauth/access_token", url.Values{
			"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
			"client_id":   {firstPartyClientID},
			"device_code": {dc.DeviceCode},
		}, &tok)
		if err == nil {
			if tok.AccessToken != "" {
				return tok.AccessToken, nil
			}
			continue
		}
		var ae *apiError
		if errors.As(err, &ae) {
			switch ae.Code {
			case "authorization_pending":
				continue
			case "slow_down":
				interval += 5 * time.Second
				continue
			case "access_denied":
				return "", errors.New("authorization was denied")
			case "expired_token":
				return "", errors.New("the device code expired; try again")
			}
		}
		return "", err
	}
	return "", errors.New("timed out waiting for authorization")
}

// ---- commands ----

func cmdMe(cl *client, jsonOut bool) error {
	var me map[string]any
	if err := cl.get("/me", &me); err != nil {
		return err
	}
	if jsonOut {
		printJSON(me)
		return nil
	}
	fmt.Printf("username: %v\n", me["username"])
	fmt.Printf("email:    %v\n", me["email"])
	fmt.Printf("created:  %v\n", me["created_at"])
	return nil
}

func cmdRepo(args []string, host, token string, jsonOut bool) error {
	if len(args) == 0 {
		return errors.New("usage: gitdash-cli repo <list|create>")
	}
	cl, err := resolveClient(host, token)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list", "ls":
		var repos []struct {
			Owner       string `json:"owner"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Private     bool   `json:"private"`
		}
		if err := cl.get("/repos", &repos); err != nil {
			return err
		}
		if jsonOut {
			printJSON(repos)
			return nil
		}
		if len(repos) == 0 {
			fmt.Println("No repositories.")
			return nil
		}
		for _, r := range repos {
			vis := "public"
			if r.Private {
				vis = "private"
			}
			fmt.Printf("%-40s %-8s %s\n", r.Owner+"/"+r.Name, vis, r.Description)
		}
		return nil
	case "create":
		fs := flag.NewFlagSet("repo create", flag.ContinueOnError)
		private := fs.Bool("private", true, "make the repository private")
		description := fs.String("description", "", "repository description")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() < 1 {
			return errors.New("usage: gitdash-cli repo create [--private=false] <name>")
		}
		name := fs.Arg(0)
		var out map[string]any
		if err := cl.postJSON("/repos", map[string]any{
			"name":        name,
			"private":     *private,
			"description": *description,
		}, &out); err != nil {
			return err
		}
		if jsonOut {
			printJSON(out)
			return nil
		}
		fmt.Printf("Created repository %s/%v\n", out["owner"], out["name"])
		return nil
	default:
		return fmt.Errorf("unknown repo subcommand %q", args[0])
	}
}

func cmdIssue(args []string, host, token string, jsonOut bool) error {
	if len(args) == 0 {
		return errors.New("usage: gitdash-cli issue <list|create> <owner/repo>")
	}
	cl, err := resolveClient(host, token)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list", "ls":
		owner, repo, err := needRepo(args[1:])
		if err != nil {
			return err
		}
		var issues []struct {
			Number int64  `json:"number"`
			Title  string `json:"title"`
			State  string `json:"state"`
			Author string `json:"author"`
		}
		if err := cl.get(fmt.Sprintf("/users/%s/repos/%s/issues", owner, repo), &issues); err != nil {
			return err
		}
		if jsonOut {
			printJSON(issues)
			return nil
		}
		if len(issues) == 0 {
			fmt.Println("No issues.")
			return nil
		}
		for _, it := range issues {
			fmt.Printf("#%-5d %-7s %-40s @%s\n", it.Number, it.State, it.Title, it.Author)
		}
		return nil
	case "create":
		owner, repo, rest, err := needRepoThenFlags(args[1:])
		if err != nil {
			return err
		}
		fs := flag.NewFlagSet("issue create", flag.ContinueOnError)
		title := fs.String("title", "", "issue title (required)")
		body := fs.String("body", "", "issue body")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if strings.TrimSpace(*title) == "" {
			return errors.New("--title is required")
		}
		var out map[string]any
		if err := cl.postJSON(fmt.Sprintf("/users/%s/repos/%s/issues", owner, repo),
			map[string]any{"title": *title, "body": *body}, &out); err != nil {
			return err
		}
		if jsonOut {
			printJSON(out)
			return nil
		}
		fmt.Printf("Created issue #%v in %s/%s\n", out["number"], owner, repo)
		return nil
	case "fix":
		return cmdCopilotFix(cl, args[1:], jsonOut)
	default:
		return fmt.Errorf("unknown issue subcommand %q", args[0])
	}
}

// ---- skill（Agent Skills：Claude Code / opencode / pi 共用 SKILL.md 约定）----

func cmdSkill(args []string) error {
	if len(args) == 0 {
		args = []string{"show"}
	}
	switch args[0] {
	case "show", "print", "cat":
		fmt.Print(skillContent)
		if !strings.HasSuffix(skillContent, "\n") {
			fmt.Println()
		}
		return nil
	case "install":
		fs := flag.NewFlagSet("skill install", flag.ContinueOnError)
		target := fs.String("target", "all", "target harness: claude | agents | opencode | pi | all")
		project := fs.Bool("project", false, "install into the current project (./.claude/skills, ./.agents/skills)")
		dir := fs.String("dir", "", "custom parent skills directory (overrides --target/--project)")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		return installSkill(*target, *project, *dir)
	default:
		return fmt.Errorf("unknown skill subcommand %q (use show|install)", args[0])
	}
}

// installSkill 把内嵌 SKILL.md 写到各代理的 skills 目录。
// Claude Code 读 ~/.claude/skills，opencode 与 pi 读 ~/.agents/skills（opencode 两者都读）。
func installSkill(target string, project bool, dir string) error {
	var parents []string
	if dir != "" {
		parents = []string{dir}
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		claude := filepath.Join(home, ".claude", "skills")
		agents := filepath.Join(home, ".agents", "skills")
		if project {
			claude = filepath.Join(".claude", "skills")
			agents = filepath.Join(".agents", "skills")
		}
		switch strings.ToLower(strings.TrimSpace(target)) {
		case "claude":
			parents = []string{claude}
		case "agents", "agent", "opencode", "pi":
			parents = []string{agents}
		case "all", "":
			parents = []string{claude, agents}
		default:
			return fmt.Errorf("unknown target %q (use claude | agents | all)", target)
		}
	}
	for _, parent := range parents {
		dest := filepath.Join(parent, "gitdash-cli")
		if err := os.MkdirAll(dest, 0o755); err != nil {
			return err
		}
		path := filepath.Join(dest, "SKILL.md")
		if err := os.WriteFile(path, []byte(skillContent), 0o644); err != nil {
			return err
		}
		fmt.Printf("Installed gitdash-cli skill → %s\n", path)
	}
	return nil
}

func cmdPr(args []string, host, token string, jsonOut bool) error {
	if len(args) == 0 {
		return errors.New("usage: gitdash-cli pr <list|create> <owner/repo>")
	}
	cl, err := resolveClient(host, token)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list", "ls":
		owner, repo, err := needRepo(args[1:])
		if err != nil {
			return err
		}
		var prs []struct {
			Number       int64  `json:"number"`
			Title        string `json:"title"`
			State        string `json:"state"`
			SourceBranch string `json:"source_branch"`
			TargetBranch string `json:"target_branch"`
		}
		if err := cl.get(fmt.Sprintf("/users/%s/repos/%s/pulls", owner, repo), &prs); err != nil {
			return err
		}
		if jsonOut {
			printJSON(prs)
			return nil
		}
		if len(prs) == 0 {
			fmt.Println("No pull requests.")
			return nil
		}
		for _, pr := range prs {
			fmt.Printf("#%-5d %-7s %-40s %s → %s\n", pr.Number, pr.State, pr.Title, pr.SourceBranch, pr.TargetBranch)
		}
		return nil
	case "create":
		owner, repo, rest, err := needRepoThenFlags(args[1:])
		if err != nil {
			return err
		}
		fs := flag.NewFlagSet("pr create", flag.ContinueOnError)
		title := fs.String("title", "", "pull request title (required)")
		body := fs.String("body", "", "pull request body")
		head := fs.String("head", "", "source branch (required)")
		base := fs.String("base", "", "target branch (required)")
		if err := fs.Parse(rest); err != nil {
			return err
		}
		if strings.TrimSpace(*title) == "" || strings.TrimSpace(*head) == "" || strings.TrimSpace(*base) == "" {
			return errors.New("--title, --head and --base are required")
		}
		var out map[string]any
		if err := cl.postJSON(fmt.Sprintf("/users/%s/repos/%s/pulls", owner, repo), map[string]any{
			"title":         *title,
			"body":          *body,
			"source_branch": *head,
			"target_branch": *base,
		}, &out); err != nil {
			return err
		}
		if jsonOut {
			printJSON(out)
			return nil
		}
		fmt.Printf("Created pull request #%v in %s/%s\n", out["number"], owner, repo)
		return nil
	default:
		return fmt.Errorf("unknown pr subcommand %q", args[0])
	}
}

// ---- copilot ----

// copilotSession 是 API 返回的 copilot 会话（只取 CLI 需要的字段）。
type copilotSession struct {
	ID          int64  `json:"id"`
	ByokID      int64  `json:"byok_id"`
	IssueNumber int64  `json:"issue_number"`
	PRNumber    int64  `json:"pr_number"`
	Prompt      string `json:"prompt"`
	Branch      string `json:"branch"`
	HeadSHA     string `json:"head_sha"`
	Status      string `json:"status"`
	Error       string `json:"error"`
}

func cmdCopilot(args []string, host, token string, jsonOut bool) error {
	if len(args) == 0 {
		return errors.New("usage: gitdash-cli copilot <list|create|run|fix> <owner/repo>")
	}
	cl, err := resolveClient(host, token)
	if err != nil {
		return err
	}
	switch args[0] {
	case "list", "ls":
		owner, repo, err := needRepo(args[1:])
		if err != nil {
			return err
		}
		var sessions []copilotSession
		if err := cl.get(fmt.Sprintf("/users/%s/repos/%s/copilots", owner, repo), &sessions); err != nil {
			return err
		}
		if jsonOut {
			printJSON(sessions)
			return nil
		}
		if len(sessions) == 0 {
			fmt.Println("No copilot sessions.")
			return nil
		}
		for _, s := range sessions {
			issue, pr := "", ""
			if s.IssueNumber > 0 {
				issue = fmt.Sprintf("issue #%d", s.IssueNumber)
			}
			if s.PRNumber > 0 {
				pr = fmt.Sprintf("PR #%d", s.PRNumber)
			}
			fmt.Printf("#%-4d %-8s %-14s %-10s %s\n", s.ID, s.Status, issue, pr, s.Branch)
		}
		return nil
	case "create":
		return cmdCopilotCreate(cl, args[1:], jsonOut)
	case "run":
		return cmdCopilotRun(cl, args[1:], jsonOut)
	case "fix":
		return cmdCopilotFix(cl, args[1:], jsonOut)
	default:
		return fmt.Errorf("unknown copilot subcommand %q", args[0])
	}
}

func cmdCopilotCreate(cl *client, args []string, jsonOut bool) error {
	owner, repo, rest, err := needRepoThenFlags(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("copilot create", flag.ContinueOnError)
	byok := fs.String("byok", "", "BYOK key name or id (default: the only configured key)")
	issue := fs.Int64("issue", 0, "link an issue by number")
	prompt := fs.String("prompt", "", "extra instructions for the agent")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	byokID, err := cl.resolveByok(*byok)
	if err != nil {
		return err
	}
	body := map[string]any{"byok_id": byokID, "prompt": strings.TrimSpace(*prompt)}
	if *issue > 0 {
		body["issue_number"] = *issue
	}
	var s copilotSession
	if err := cl.postJSON(fmt.Sprintf("/users/%s/repos/%s/copilots", owner, repo), body, &s); err != nil {
		return err
	}
	if jsonOut {
		printJSON(s)
		return nil
	}
	fmt.Printf("Created copilot session #%d (%s) in %s/%s\n", s.ID, s.Branch, owner, repo)
	return nil
}

func cmdCopilotRun(cl *client, args []string, jsonOut bool) error {
	owner, repo, rest, err := needRepoThenFlags(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("copilot run", flag.ContinueOnError)
	text := fs.String("text", "", "message to send to the agent (default: the session prompt)")
	timeout := fs.Duration("timeout", 30*time.Minute, "max time to wait for the turn")
	if err := fs.Parse(permuteFlags(rest, map[string]bool{"--text": true, "--timeout": true})); err != nil {
		return err
	}
	id, err := parseSessionID(fs.Arg(0))
	if err != nil {
		return err
	}
	msg := strings.TrimSpace(*text)
	if msg == "" {
		var s copilotSession
		if err := cl.get(fmt.Sprintf("/users/%s/repos/%s/copilots/%d", owner, repo, id), &s); err != nil {
			return err
		}
		msg = s.Prompt
	}
	if msg == "" {
		return errors.New("--text is required (session has no prompt)")
	}
	return runCopilotTurn(cl, owner, repo, id, msg, *timeout, jsonOut)
}

func cmdCopilotFix(cl *client, args []string, jsonOut bool) error {
	owner, repo, rest, err := needRepoThenFlags(args)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("copilot fix", flag.ContinueOnError)
	byok := fs.String("byok", "", "BYOK key name or id (default: the only configured key)")
	instructions := fs.String("instructions", "", "extra instructions for the agent")
	detach := fs.Bool("detach", false, "create the session but do not run the agent")
	timeout := fs.Duration("timeout", 30*time.Minute, "max time to wait for the turn")
	if err := fs.Parse(permuteFlags(rest, map[string]bool{"--byok": true, "--instructions": true, "--timeout": true})); err != nil {
		return err
	}
	number, err := strconv.ParseInt(strings.TrimSpace(fs.Arg(0)), 10, 64)
	if err != nil || number <= 0 {
		return errors.New("usage: gitdash-cli copilot fix <owner/repo> <issue-number>")
	}
	byokID, err := cl.resolveByok(*byok)
	if err != nil {
		return err
	}
	var s copilotSession
	if err := cl.postJSON(fmt.Sprintf("/users/%s/repos/%s/copilots", owner, repo), map[string]any{
		"byok_id":      byokID,
		"issue_number": number,
		"prompt":       strings.TrimSpace(*instructions),
	}, &s); err != nil {
		return err
	}
	if *detach {
		if jsonOut {
			printJSON(s)
			return nil
		}
		fmt.Printf("Created copilot session #%d for issue #%d (%s)\n", s.ID, number, s.Branch)
		return nil
	}
	fmt.Fprintf(os.Stderr, "copilot: session #%d working on issue #%d…\n", s.ID, number)
	return runCopilotTurn(cl, owner, repo, s.ID, s.Prompt, *timeout, jsonOut)
}

// runCopilotTurn 连接会话 WebSocket，发送一条消息并流式接收，直到 done。
func runCopilotTurn(cl *client, owner, repo string, id int64, text string, timeout time.Duration, jsonOut bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	wsURL := strings.Replace(cl.host, "http", "ws", 1) +
		fmt.Sprintf("/api/users/%s/repos/%s/copilots/%d/chat", owner, repo, id)
	header := http.Header{}
	header.Set("Authorization", "Bearer "+cl.token)
	conn, _, err := websocket.Dial(ctx, wsURL, &websocket.DialOptions{HTTPHeader: header}) //nolint:bodyclose // coder/websocket manages the handshake body: "You never need to close resp.Body yourself."
	if err != nil {
		return fmt.Errorf("connect chat: %w", err)
	}
	defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()

	payload, _ := json.Marshal(map[string]string{"type": "user", "text": text})
	if err := conn.Write(ctx, websocket.MessageText, payload); err != nil {
		return fmt.Errorf("send: %w", err)
	}

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			return fmt.Errorf("stream: %w", err)
		}
		var ev struct {
			Type   string `json:"type"`
			Text   string `json:"text"`
			Name   string `json:"name"`
			Result string `json:"result"`
			Error  string `json:"error"`
		}
		if json.Unmarshal(data, &ev) != nil {
			continue
		}
		switch ev.Type {
		case "tool_start":
			fmt.Fprintf(os.Stderr, "  → %s\n", ev.Name)
		case "tool_end":
			if ev.Result != "" {
				fmt.Fprintf(os.Stderr, "    %s\n", firstLine(ev.Result, 200))
			}
		case "delta":
			fmt.Fprint(os.Stderr, ev.Text)
		case "done":
			if ev.Text != "" {
				fmt.Fprintf(os.Stderr, "\n%s\n", ev.Text)
			}
			var s copilotSession
			if err := cl.get(fmt.Sprintf("/users/%s/repos/%s/copilots/%d", owner, repo, id), &s); err == nil {
				if jsonOut {
					printJSON(s)
					return nil
				}
				if s.PRNumber > 0 {
					fmt.Printf("Opened pull request #%d\n", s.PRNumber)
				} else {
					fmt.Printf("Session #%d finished on %s (no PR linked)\n", s.ID, s.Branch)
				}
			}
			return nil
		case "error":
			return fmt.Errorf("agent: %s", ev.Error)
		}
	}
}

// resolveByok 把 --byok（名称或 id）解析为已配置的 BYOK 密钥 id。
func (c *client) resolveByok(ref string) (int64, error) {
	var keys []struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}
	if err := c.get("/me/byok", &keys); err != nil {
		return 0, err
	}
	if len(keys) == 0 {
		return 0, errors.New("no BYOK key configured; add one under Profile → BYOK")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		if len(keys) == 1 {
			return keys[0].ID, nil
		}
		return 0, fmt.Errorf("multiple BYOK keys; pass --byok <name|id> (available: %s)", byokNames(keys))
	}
	if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
		for _, k := range keys {
			if k.ID == id {
				return id, nil
			}
		}
		return 0, fmt.Errorf("byok id %d not found (available: %s)", id, byokNames(keys))
	}
	for _, k := range keys {
		if strings.EqualFold(k.Name, ref) {
			return k.ID, nil
		}
	}
	return 0, fmt.Errorf("byok %q not found (available: %s)", ref, byokNames(keys))
}

func byokNames(keys []struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}) string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, fmt.Sprintf("%s (#%d)", k.Name, k.ID))
	}
	return strings.Join(out, ", ")
}

func parseSessionID(s string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("usage: gitdash-cli copilot run <owner/repo> <session-id>")
	}
	return id, nil
}

// permuteFlags 把 flags 提到位置参数之前（Go 标准 flag 遇首个位置参数即停止解析）。
// valueFlags 声明哪些 flag 需要紧跟一个值（如 --byok x）。仅支持 --flag / --flag=value 形式。
func permuteFlags(args []string, valueFlags map[string]bool) []string {
	flags := make([]string, 0, len(args))
	pos := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && valueFlags[a] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, a)
		}
	}
	return append(flags, pos...)
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// ---- helpers ----

// needRepo 取第一个位置参数并解析为 owner/repo。
func needRepo(args []string) (string, string, error) {
	if len(args) < 1 {
		return "", "", errors.New("missing <owner/repo> argument")
	}
	return splitRepo(args[0])
}

// needRepoThenFlags 解析 owner/repo 后返回剩余参数（用于需要 flag 的子命令）。
func needRepoThenFlags(args []string) (string, string, []string, error) {
	if len(args) < 1 {
		return "", "", nil, errors.New("missing <owner/repo> argument")
	}
	owner, repo, err := splitRepo(args[0])
	if err != nil {
		return "", "", nil, err
	}
	return owner, repo, args[1:], nil
}

func splitRepo(s string) (string, string, error) {
	parts := strings.SplitN(strings.Trim(s, "/"), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid repository %q (expected owner/repo)", s)
	}
	return parts[0], parts[1], nil
}

func readSecret(reader *bufio.Reader) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println()
		return string(b), err
	}
	return reader.ReadString('\n')
}

func openBrowser(u string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", u)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", u)
	default:
		cmd = exec.Command("xdg-open", u)
	}
	return cmd.Start()
}
