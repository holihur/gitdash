package sshserver

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"gitdash/backend/internal/authz"
	"gitdash/backend/internal/grpcserver"
	"gitdash/backend/internal/store"
)

// TestSSHGatewayRemoteAuthorizer 端到端验证「独立 SSH 网关」：SSH 进程不直接读库，
// 所有鉴权（公钥 / IP 封禁 / 读权限）都经授权面 gRPC 完成。
func TestSSHGatewayRemoteAuthorizer(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()

	// 权威数据源：store + 用户 + 公钥。
	st, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := st.CreateUser("alice", "x"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh key: %v", err)
	}
	line := string(ssh.MarshalAuthorizedKey(sshPub))
	if _, err := st.CreateKey("alice", "laptop", line, ssh.FingerprintSHA256(sshPub)); err != nil {
		t.Fatalf("create key: %v", err)
	}
	if _, err := st.CreateRepo("alice", "demo", "", true); err != nil {
		t.Fatalf("create repo: %v", err)
	}

	// 授权面服务。
	authzSrv, err := grpcserver.NewGRPCServer(st, "test-token")
	if err != nil {
		t.Fatalf("grpc server: %v", err)
	}
	azLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen authz: %v", err)
	}
	go func() { _ = authzSrv.Serve(azLn) }()
	defer authzSrv.Stop()

	conn, remote, err := authz.Dial(authz.DialOptions{Addr: azLn.Addr().String(), Token: "test-token"})
	if err != nil {
		t.Fatalf("dial authz: %v", err)
	}
	defer func() { _ = conn.Close() }()

	// 独立 SSH 网关：仓库目录与 spool 由共享存储提供。
	reposDir := filepath.Join(dir, "repos")
	repoPath := filepath.Join(reposDir, "alice", "demo.git")
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	if out, err := exec.Command("git", "init", "-q", "--bare", repoPath).CombinedOutput(); err != nil {
		t.Fatalf("git init --bare: %v: %s", err, out)
	}

	srv, err := NewServerWithAuthorizer(remote, reposDir, dir)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	sshLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen ssh: %v", err)
	}
	go func() { _ = srv.ServeOn(sshLn) }()
	defer func() { _ = sshLn.Close() }()

	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}

	// 已登记公钥：允许连接并执行 git-upload-pack。
	client := dialSSH(t, sshLn.Addr().String(), signer)
	defer func() { _ = client.Close() }()
	session, err := client.NewSession()
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	// 发送 flush 包，让 upload-pack 完成协议并正常退出。
	session.Stdin = strings.NewReader("0000")
	if _, err := session.Output("git-upload-pack 'alice/demo.git'"); err != nil {
		t.Fatalf("upload-pack over gateway: %v", err)
	}

	// 未登记公钥：授权面拒绝，握手失败。
	_, otherPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen other key: %v", err)
	}
	otherSigner, err := ssh.NewSignerFromKey(otherPriv)
	if err != nil {
		t.Fatalf("other signer: %v", err)
	}
	if _, err := ssh.Dial("tcp", sshLn.Addr().String(), &ssh.ClientConfig{
		User:            "git",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(otherSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	}); err == nil {
		t.Fatal("unregistered key should be rejected by the authorization plane")
	}
}

func dialSSH(t *testing.T, addr string, signer ssh.Signer) *ssh.Client {
	t.Helper()
	client, err := ssh.Dial("tcp", addr, &ssh.ClientConfig{
		User:            "git",
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatalf("ssh dial: %v", err)
	}
	return client
}
