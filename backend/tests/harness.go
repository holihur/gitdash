// Package tests 是按功能划分的集成测试目录：
//
//	auth_test.go    注册 / 登录 / 登出 / 会话
//	repos_test.go   仓库 CRUD 与用户隔离
//	collab_test.go  协作者（读写权限 / 管理 / 同名仓库 / 级联）
//	issues_test.go  Issue CRUD / 状态流转 / 删除级联
//	sshkeys_test.go SSH 公钥 CRUD 与用户绑定
//	browse_test.go  代码浏览（tree / blob / commits / branches）
//	sshgit_test.go  SSH git clone / push / 权限拒绝
//	updater_test.go 自更新纯函数（版本比较 / 校验和 / 解包）
//	store_test.go   存储层（含旧 schema 迁移）
//	webui_test.go   前端静态托管 / SPA fallback
//
// harness.go 提供 in-process 服务器（httptest + 随机端口 SSH）与 API 客户端。
package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"gitdash/backend/internal/api"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/sshserver"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/webui"
)

func readInto(r io.Reader, w io.Writer) {
	_, _ = io.Copy(w, r)
}

func embeddedAssetsPresent() bool {
	return webui.HasAssets()
}

type Env struct {
	t        *testing.T
	BaseURL  string
	SSHPort  string
	ReposDir string
	DataDir  string
}

// start 启动 in-process 的 HTTP + SSH 服务（每测一个独立实例）。
func start(t *testing.T) *Env {
	t.Helper()
	dir := t.TempDir()
	if err := gitsvc.Init(dir); err != nil {
		t.Fatalf("gitsvc init: %v", err)
	}
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	hs := httptest.NewServer(api.New(st, "test").Handler(""))
	t.Cleanup(hs.Close)

	srv, err := sshserver.NewServer(st, gitsvc.ReposDir(), dir)
	if err != nil {
		t.Fatalf("ssh server: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = srv.ServeOn(ln) }()

	_, port, _ := net.SplitHostPort(ln.Addr().String())
	return &Env{t: t, BaseURL: hs.URL, SSHPort: port, ReposDir: gitsvc.ReposDir(), DataDir: dir}
}

// Client 是最简单的 API 客户端（带可选 Bearer token）。
type Client struct {
	env   *Env
	token string
}

func (c *Client) do(method, path string, body any) (int, map[string]any) {
	c.env.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			c.env.t.Fatalf("marshal body: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.env.BaseURL+"/api"+path, rd)
	if err != nil {
		c.env.t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.env.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m
}

// mustStatus 断言状态码并返回 body。
func (c *Client) mustStatus(method, path string, body any, want int) map[string]any {
	c.env.t.Helper()
	code, m := c.do(method, path, body)
	if code != want {
		c.env.t.Fatalf("%s %s: status = %d, want %d, body = %v", method, path, code, want, m)
	}
	return m
}

func (c *Client) mustFail(method, path string, body any, want int) {
	c.env.t.Helper()
	code, _ := c.do(method, path, body)
	if code != want {
		c.env.t.Fatalf("%s %s: status = %d, want failure %d", method, path, code, want)
	}
}

// register 注册用户并返回带会话的客户端。
func register(t *testing.T, env *Env, username, password string) *Client {
	t.Helper()
	c := &Client{env: env}
	m := c.mustStatus("POST", "/auth/register",
		map[string]string{"username": username, "password": password}, 201)
	token, _ := m["token"].(string)
	if token == "" {
		t.Fatal("register: empty token")
	}
	return &Client{env: env, token: token}
}

func hasBin(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func requireBins(t *testing.T, names ...string) {
	t.Helper()
	for _, n := range names {
		if !hasBin(n) {
			t.Skipf("%s not found, skipping", n)
		}
	}
}

func runCmd(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v\n%s", name, args, err, out)
	}
	return string(out)
}

func runCmdFail(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("%s %v: expected failure, got success\n%s", name, args, out)
	}
}
