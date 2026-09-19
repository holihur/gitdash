// Package blackbox 是对 gitdash 授权面（gRPC AuthzService）的独立黑盒测试。
//
// 与 backend/tests 的进程内集成测试不同，这里：
//   - 不使用 api.New / sshserver / grpcserver 等任何服务端实现，只 import 生成的
//     gRPC 客户端桩（authzv1）；
//   - 现场构建 gitdash 二进制（或复用 GITDASH_BIN），在独立临时数据目录 + 随机端口上
//     以子进程启动，所有交互只经 HTTP / gRPC 网络接口；
//   - 因此它验证的是 main.go 的真实接线（env → gRPC listener → store），而非进程内构造。
//
// 运行：go test ./tests/blackbox/ -v
// 复用预构建二进制：GITDASH_BIN=/tmp/gitdash-server go test ./tests/blackbox/ -v
// `-short` 模式下跳过（构建 + 起进程较重）。
package blackbox

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"gitdash/backend/internal/grpcserver/authzv1"
)

const (
	grpcToken = "blackbox-grpc-token"
	adminUser = "blackbox-admin"
	adminPass = "admin-blackbox-pass-123456"
	userPass  = "pass-123456"
)

// server 描述一个已就绪的被测实例。
type server struct {
	baseURL  string // HTTP API 根
	grpcAddr string // 授权面 gRPC 地址
}

// startServer 构建（或复用）二进制并启动独立实例，等待 HTTP 健康后返回。
func startServer(t *testing.T) *server {
	t.Helper()
	if testing.Short() {
		t.Skip("blackbox: skipped in -short mode")
	}

	bin := buildBinary(t)
	dataDir := t.TempDir()
	httpPort, sshPort, grpcPort := freePort(t), freePort(t), freePort(t)

	baseURL := "http://127.0.0.1:" + strconv.Itoa(httpPort)
	grpcAddr := "127.0.0.1:" + strconv.Itoa(grpcPort)

	cmd := exec.Command(bin, "serve")
	cmd.Env = append(cleanEnv(),
		"GITDASH_DATA="+dataDir,
		"GITDASH_DISABLE_RATE_LIMIT=1",
		"GITDASH_PROFILE_REPO=0",
		"GITDASH_HTTP_ADDR=127.0.0.1:"+strconv.Itoa(httpPort),
		"GITDASH_SSH_ADDR=127.0.0.1:"+strconv.Itoa(sshPort),
		"GITDASH_GRPC_ADDR="+grpcAddr,
		"GITDASH_GRPC_TOKEN="+grpcToken,
		"GITDASH_ADMIN_USER="+adminUser,
		"GITDASH_ADMIN_PASSWORD="+adminPass,
	)

	logPath := filepath.Join(dataDir, "server.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create log: %v", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start server: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
		_ = logFile.Close()
	})

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			t.Fatalf("server exited early; log:\n%s", readFile(logPath))
		default:
		}
		if resp, err := http.Get(baseURL + "/api/health"); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return &server{baseURL: baseURL, grpcAddr: grpcAddr}
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("server not ready in time; log:\n%s", readFile(logPath))
	return nil
}

// buildBinary 返回被测二进制：优先 GITDASH_BIN，否则现场 `go build` 模块根。
func buildBinary(t *testing.T) string {
	t.Helper()
	if bin := os.Getenv("GITDASH_BIN"); bin != "" {
		if _, err := os.Stat(bin); err != nil {
			t.Fatalf("GITDASH_BIN not usable: %v", err)
		}
		return bin
	}
	root, err := filepath.Abs("../..") // backend/tests/blackbox -> backend
	if err != nil {
		t.Fatalf("resolve module root: %v", err)
	}
	out := filepath.Join(t.TempDir(), "gitdash")
	build := exec.Command("go", "build", "-o", out, ".")
	build.Dir = root
	if b, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build failed: %v\n%s", err, b)
	}
	return out
}

// cleanEnv 复制进程环境，但剔除全部 GITDASH_*，避免外部配置污染被测实例。
func cleanEnv() []string {
	env := os.Environ()
	out := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, "GITDASH_") {
			out = append(out, kv)
		}
	}
	return out
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("free port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func readFile(path string) string {
	b, _ := os.ReadFile(path)
	return string(b)
}

// ---- HTTP API 辅助（黑盒：只发请求）----

func apiDoClient(t *testing.T, client *http.Client, base, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, base+"/api"+path, rd)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	return resp.StatusCode, m
}

func mustStatus(t *testing.T, base, method, path, token string, body any, want int) map[string]any {
	t.Helper()
	return mustStatusClient(t, http.DefaultClient, base, method, path, token, body, want)
}

func mustStatusClient(t *testing.T, client *http.Client, base, method, path, token string, body any, want int) map[string]any {
	t.Helper()
	code, m := apiDoClient(t, client, base, method, path, token, body)
	if code != want {
		t.Fatalf("%s %s: status = %d, want %d, body = %v", method, path, code, want, m)
	}
	return m
}

func register(t *testing.T, base, username string) string {
	t.Helper()
	m := mustStatus(t, base, "POST", "/auth/register", "",
		map[string]string{"username": username, "password": userPass}, http.StatusCreated)
	tok, _ := m["token"].(string)
	if tok == "" {
		t.Fatalf("register %s: empty token: %v", username, m)
	}
	return tok
}

// adminClient 以管理端登录（仅下发 HttpOnly cookie）并返回带 cookie jar 的客户端。
func adminClient(t *testing.T, base string) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	c := &http.Client{Jar: jar}
	code, m := apiDoClient(t, c, base, "POST", "/admin/login", "",
		map[string]string{"username": adminUser, "password": adminPass})
	if code != http.StatusOK {
		t.Fatalf("admin login: status = %d, body = %v", code, m)
	}
	return c
}

// ---- gRPC 辅助 ----

func grpcClient(t *testing.T, addr string) authzv1.AuthzServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return authzv1.NewAuthzServiceClient(conn)
}

func authCtx(token string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

func newSSHKey(t *testing.T) (ssh.PublicKey, string) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("new ssh key: %v", err)
	}
	return sshPub, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
}

// ---- 用例 ----

func TestAuthzGRPCBlackBox(t *testing.T) {
	s := startServer(t)
	client := grpcClient(t, s.grpcAddr)
	okCtx := authCtx(grpcToken)

	t.Run("token_required", func(t *testing.T) {
		if _, err := client.IsIPBanned(context.Background(),
			&authzv1.IsIPBannedRequest{Ip: "10.0.0.1"}); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("no token: code = %v, want Unauthenticated", status.Code(err))
		}
		if _, err := client.IsIPBanned(authCtx("wrong-token"),
			&authzv1.IsIPBannedRequest{Ip: "10.0.0.1"}); status.Code(err) != codes.Unauthenticated {
			t.Fatalf("wrong token: code = %v, want Unauthenticated", status.Code(err))
		}
	})

	t.Run("is_ip_banned", func(t *testing.T) {
		r, err := client.IsIPBanned(okCtx, &authzv1.IsIPBannedRequest{Ip: "10.1.2.3"})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		if r.GetBanned() {
			t.Fatal("10.1.2.3 should not be banned before admin action")
		}

		admin := adminClient(t, s.baseURL)
		mustStatusClient(t, admin, s.baseURL, "POST", "/admin/ip-bans", "",
			map[string]string{"cidr": "10.0.0.0/8", "note": "blackbox"}, http.StatusCreated)

		r, err = client.IsIPBanned(okCtx, &authzv1.IsIPBannedRequest{Ip: "10.1.2.3"})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		if !r.GetBanned() {
			t.Fatal("10.1.2.3 should be banned after admin action")
		}
	})

	t.Run("can_read_write", func(t *testing.T) {
		ownerTok := register(t, s.baseURL, "bbowner")
		register(t, s.baseURL, "bbstranger") // 仅创建账号，用于越权断言
		mustStatus(t, s.baseURL, "POST", "/repos", ownerTok,
			map[string]any{"name": "bbpriv", "private": true}, http.StatusCreated)
		mustStatus(t, s.baseURL, "POST", "/repos", ownerTok,
			map[string]any{"name": "bbpub", "private": false}, http.StatusCreated)

		cases := []struct {
			name, owner, repo, user string
			wantRead, wantWrite     bool
		}{
			{"owner-private", "bbowner", "bbpriv", "bbowner", true, true},
			{"stranger-private", "bbowner", "bbpriv", "bbstranger", false, false},
			{"stranger-public-read", "bbowner", "bbpub", "bbstranger", true, false},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				rd, err := client.CanRead(okCtx, &authzv1.CanReadRequest{
					Owner: c.owner, Repo: c.repo, Username: c.user,
				})
				if err != nil {
					t.Fatalf("CanRead: %v", err)
				}
				if rd.GetAllowed() != c.wantRead {
					t.Errorf("CanRead(%s/%s, %s) = %v, want %v", c.owner, c.repo, c.user, rd.GetAllowed(), c.wantRead)
				}
				wr, err := client.CanWrite(okCtx, &authzv1.CanWriteRequest{
					Owner: c.owner, Repo: c.repo, Username: c.user,
				})
				if err != nil {
					t.Fatalf("CanWrite: %v", err)
				}
				if wr.GetAllowed() != c.wantWrite {
					t.Errorf("CanWrite(%s/%s, %s) = %v, want %v", c.owner, c.repo, c.user, wr.GetAllowed(), c.wantWrite)
				}
			})
		}
	})

	t.Run("authorize_public_key", func(t *testing.T) {
		tok := register(t, s.baseURL, "bbkeyuser")
		pub, line := newSSHKey(t)
		mustStatus(t, s.baseURL, "POST", "/keys", tok,
			map[string]string{"name": "laptop", "public_key": line}, http.StatusCreated)

		// 命中
		r, err := client.AuthorizePublicKey(okCtx, &authzv1.AuthorizePublicKeyRequest{
			KeyType: pub.Type(), KeyBlob: pub.Marshal(),
		})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		if !r.GetAuthorized() || r.GetUsername() != "bbkeyuser" {
			t.Fatalf("got authorized=%v username=%q, want true/bbkeyuser", r.GetAuthorized(), r.GetUsername())
		}

		// 未登记
		otherPub, _ := newSSHKey(t)
		r, err = client.AuthorizePublicKey(okCtx, &authzv1.AuthorizePublicKeyRequest{
			KeyType: otherPub.Type(), KeyBlob: otherPub.Marshal(),
		})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		if r.GetAuthorized() || r.GetReason() != "unknown public key" {
			t.Fatalf("got authorized=%v reason=%q, want false/unknown public key", r.GetAuthorized(), r.GetReason())
		}

		// 账号被封禁
		admin := adminClient(t, s.baseURL)
		mustStatusClient(t, admin, s.baseURL, "POST", "/admin/users/bbkeyuser/ban", "",
			map[string]any{"banned": true}, http.StatusNoContent)
		r, err = client.AuthorizePublicKey(okCtx, &authzv1.AuthorizePublicKeyRequest{
			KeyType: pub.Type(), KeyBlob: pub.Marshal(),
		})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		if r.GetAuthorized() || r.GetReason() != "account is banned" {
			t.Fatalf("got authorized=%v reason=%q, want false/account is banned", r.GetAuthorized(), r.GetReason())
		}
	})
}
