package gitsvc

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestCredEnvEmpty(t *testing.T) {
	env, cleanup, err := credEnv("")
	if err != nil || env != nil || cleanup != nil {
		t.Fatalf("credEnv(\"\") should return nil env, nil cleanup and nil error (err=%v)", err)
	}
}

func TestCredEnvAskpass(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	env, cleanup, err := credEnv("alice:tok-123")
	if err != nil {
		t.Fatalf("credEnv: %v", err)
	}
	if cleanup == nil {
		t.Fatal("expected a cleanup function")
	}
	m := map[string]string{}
	for _, e := range env {
		k, v, _ := strings.Cut(e, "=")
		m[k] = v
	}
	if m["GITDASH_GIT_USER"] != "alice" || m["GITDASH_GIT_PASS"] != "tok-123" {
		t.Fatalf("credential env = %v", m)
	}
	if m["GIT_TERMINAL_PROMPT"] != "0" {
		t.Fatalf("GIT_TERMINAL_PROMPT = %q, want 0", m["GIT_TERMINAL_PROMPT"])
	}
	script := m["GIT_ASKPASS"]
	if script == "" {
		t.Fatal("GIT_ASKPASS not set")
	}
	ask := func(prompt string) string {
		cmd := exec.Command(script, prompt)
		cmd.Env = append(os.Environ(),
			"GITDASH_GIT_USER="+m["GITDASH_GIT_USER"],
			"GITDASH_GIT_PASS="+m["GITDASH_GIT_PASS"],
		)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("run askpass: %v", err)
		}
		return string(out)
	}
	if got := ask("Username for 'https://example.com': "); got != "alice" {
		t.Fatalf("askpass username = %q, want alice", got)
	}
	if got := ask("Password for 'https://alice@example.com': "); got != "tok-123" {
		t.Fatalf("askpass password = %q, want tok-123", got)
	}
	dir := filepath.Dir(script)
	cleanup()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("cleanup did not remove %s", dir)
	}
}

// TestImportRepoWithCredentialOverHTTP 端到端验证：从需要 Basic 认证的 HTTPS/HTTP
// git 服务导入私有仓库时，GIT_ASKPASS 会提供凭据（issue: 带凭据的私有仓库导入缺少测试）。
func TestImportRepoWithCredentialOverHTTP(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	backend := gitHTTPBackendPath()
	if backend == "" {
		t.Skip("git-http-backend not available")
	}

	dir := t.TempDir()
	t.Setenv("GITDASH_DATA", dir)
	t.Setenv("GITDASH_SSRF_ALLOW_PRIVATE", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	if err := Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	if err := CreateBare("owner", "src"); err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := WriteCommit("owner", "src", "main", "init", "tester",
		[]FileChange{{Path: "README.md", Action: "create", Content: "hello\n"}}); err != nil {
		t.Fatalf("seed commit: %v", err)
	}

	const user, pass = "alice", "s3cret"
	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(user+":"+pass))
	root := ReposDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != wantAuth {
			w.Header().Set("WWW-Authenticate", `Basic realm="git"`)
			http.Error(w, "authentication required", http.StatusUnauthorized)
			return
		}
		runGitHTTPBackend(t, backend, root, w, r)
	}))
	defer srv.Close()

	// 带正确凭据：导入成功。
	if err := importRepo(srv.URL+"/owner/src.git", "tester", "dst", "", user+":"+pass); err != nil {
		t.Fatalf("import with credential: %v", err)
	}
	if !Exists("tester", "dst") {
		t.Fatal("target repo was not created")
	}
	head, err := HeadBranch("tester", "dst")
	if err != nil || head != "main" {
		t.Fatalf("imported head = %q, %v; want main", head, err)
	}

	// 凭据错误：导入失败（证明认证确实生效）。
	if err := importRepo(srv.URL+"/owner/src.git", "tester", "bad", "", "alice:wrong"); err == nil {
		t.Fatal("import with wrong credential should fail")
	}
}

func gitHTTPBackendPath() string {
	if p, err := exec.LookPath("git"); err == nil {
		cand := filepath.Join(filepath.Dir(p), "git-http-backend")
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
		// Debian/Ubuntu layout
		cand = "/usr/lib/git-core/git-http-backend"
		if _, err := os.Stat(cand); err == nil {
			return cand
		}
	}
	for _, p := range []string{"/usr/lib/git-core/git-http-backend", "/usr/local/libexec/git-core/git-http-backend"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// runGitHTTPBackend 以 CGI 方式调用 git-http-backend 并把结果写回响应。
func runGitHTTPBackend(t *testing.T, backend, root string, w http.ResponseWriter, r *http.Request) {
	t.Helper()
	cmd := exec.Command(backend)
	cmd.Env = append(os.Environ(),
		"GIT_PROJECT_ROOT="+root,
		"GIT_HTTP_EXPORT_ALL=1",
		"PATH_INFO="+r.URL.Path,
		"QUERY_STRING="+r.URL.RawQuery,
		"REQUEST_METHOD="+r.Method,
		"CONTENT_TYPE="+r.Header.Get("Content-Type"),
	)
	if r.ContentLength > 0 {
		cmd.Env = append(cmd.Env, "CONTENT_LENGTH="+strconv.FormatInt(r.ContentLength, 10))
	}
	cmd.Stdin = r.Body
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		http.Error(w, "git http backend: "+err.Error()+" "+stderr.String(), http.StatusInternalServerError)
		return
	}
	// 解析 CGI 头（到空行为止），随后是响应体。
	raw := stdout.Bytes()
	sep := bytes.Index(raw, []byte("\r\n\r\n"))
	if sep < 0 {
		sep = bytes.Index(raw, []byte("\n\n"))
	}
	if sep < 0 {
		http.Error(w, "malformed CGI response", http.StatusInternalServerError)
		return
	}
	headerBlock := string(raw[:sep])
	body := raw[sep:]
	body = bytes.TrimPrefix(body, []byte("\r\n\r\n"))
	body = bytes.TrimPrefix(body, []byte("\n\n"))

	status := http.StatusOK
	for _, line := range strings.Split(headerBlock, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		k, v, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if strings.EqualFold(k, "Status") {
			if code, _, _ := strings.Cut(v, " "); code != "" {
				if n, err := strconv.Atoi(code); err == nil {
					status = n
				}
			}
			continue
		}
		w.Header().Set(k, v)
	}
	w.WriteHeader(status)
	_, _ = io.Copy(w, bytes.NewReader(body))
}
