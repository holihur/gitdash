// SSH 网关黑盒测试：真二进制 + 真 SSH + 真 git，验证「API 与 SSH 分机部署、
// 鉴权全部经授权面 gRPC」这条 main.go 接线（而非进程内构造）。
//
// 运行：go test ./tests/blackbox/ -run TestSSHGatewayBlackBox -v
// `-short` 下跳过（构建 + 起两个进程较重）。
package blackbox

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// newSSHKeyPair 生成 ed25519 密钥对，返回签名器、authorized_keys 行与 OpenSSH 私钥 PEM。
func newSSHKeyPair(t *testing.T) (ssh.Signer, string, []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh pub: %v", err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatalf("signer: %v", err)
	}
	line := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(sshPub)))
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	return signer, line, pem.EncodeToMemory(block)
}

// startSSHGateway 以「仅 SSH 网关」角色启动同一二进制的独立进程：
// 共享 dataDir（仓库目录 + spool），鉴权指向 authzAddr；返回 SSH 地址。
func startSSHGateway(t *testing.T, bin, dataDir, authzAddr string, httpPort int) string {
	t.Helper()
	sshPort := freePort(t)
	sshAddr := "127.0.0.1:" + strconv.Itoa(sshPort)

	cmd := exec.Command(bin, "serve")
	cmd.Env = append(cleanEnv(),
		"GITDASH_ROLE=ssh",
		"GITDASH_DATA="+dataDir,
		"GITDASH_SSH_ADDR="+sshAddr,
		"GITDASH_HTTP_ADDR=127.0.0.1:"+strconv.Itoa(httpPort),
		"GITDASH_GRPC_ADDR="+authzAddr,
		"GITDASH_GRPC_TOKEN="+grpcToken,
	)
	logPath := filepath.Join(dataDir, "gateway.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create gateway log: %v", err)
	}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		t.Fatalf("start gateway: %v", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
		_ = logFile.Close()
	})

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-done:
			t.Fatalf("gateway exited early; log:\n%s", readFile(logPath))
		default:
		}
		if conn, err := net.DialTimeout("tcp", sshAddr, 300*time.Millisecond); err == nil {
			_ = conn.Close()
			return sshAddr
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("gateway ssh not ready; log:\n%s", readFile(logPath))
	return ""
}

func portOf(t *testing.T, addr string) int {
	t.Helper()
	_, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("split %q: %v", addr, err)
	}
	n, err := strconv.Atoi(p)
	if err != nil {
		t.Fatalf("port %q: %v", p, err)
	}
	return n
}

func runGit(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestSSHGatewayBlackBox(t *testing.T) {
	if testing.Short() {
		t.Skip("blackbox: skipped in -short mode")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	bin := buildBinary(t)
	s := startServerWithBinary(t, bin)

	// 权威数据源：经 HTTP API 建用户 / 仓库 / 公钥（网关不碰 DB）。
	tok := register(t, s.baseURL, "gwuser")
	mustStatus(t, s.baseURL, "POST", "/repos", tok,
		map[string]any{"name": "gwdemo", "private": true}, http.StatusCreated)
	_, line, privPEM := newSSHKeyPair(t)
	mustStatus(t, s.baseURL, "POST", "/keys", tok,
		map[string]string{"name": "gw", "public_key": line}, http.StatusCreated)

	// 独立 SSH 网关进程（共享 dataDir，不打开数据库）。
	httpPort := freePort(t)
	sshAddr := startSSHGateway(t, bin, s.dataDir, s.grpcAddr, httpPort)

	// 运行的是「仅 SSH」角色：不应启动 HTTP API。
	if conn, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(httpPort), 500*time.Millisecond); err == nil {
		_ = conn.Close()
		t.Fatal("ssh gateway must not start the HTTP API")
	}

	t.Run("registered_key_can_push_and_clone", func(t *testing.T) {
		keyFile := filepath.Join(t.TempDir(), "id_ed25519")
		if err := os.WriteFile(keyFile, privPEM, 0o600); err != nil {
			t.Fatalf("write key: %v", err)
		}
		sshCmd := fmt.Sprintf(
			"ssh -i %s -o IdentitiesOnly=yes -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o LogLevel=ERROR",
			keyFile,
		)
		env := append(os.Environ(), "GIT_SSH_COMMAND="+sshCmd)
		remote := fmt.Sprintf("ssh://git@127.0.0.1:%d/gwuser/gwdemo.git", portOf(t, sshAddr))

		// push：走 CanWrite（授权面）+ receive-pack + pre/post-receive hooks。
		work := t.TempDir()
		runGit(t, work, env, "init", "-q", "-b", "main")
		runGit(t, work, env, "-c", "user.email=bb@example.com", "-c", "user.name=bb",
			"commit", "-q", "--allow-empty", "-m", "init")
		runGit(t, work, env, "push", "-q", remote, "main")

		// clone：走 CanRead（授权面），验证数据面（共享仓库目录）可用。
		clone := t.TempDir()
		runGit(t, clone, env, "clone", "-q", remote, "out")
		if _, err := os.Stat(filepath.Join(clone, "out", ".git")); err != nil {
			t.Fatalf("clone output missing: %v", err)
		}
	})

	t.Run("unknown_key_rejected", func(t *testing.T) {
		otherSigner, _, _ := newSSHKeyPair(t)
		if _, err := ssh.Dial("tcp", sshAddr, &ssh.ClientConfig{
			User:            "git",
			Auth:            []ssh.AuthMethod{ssh.PublicKeys(otherSigner)},
			HostKeyCallback: ssh.InsecureIgnoreHostKey(),
			Timeout:         5 * time.Second,
		}); err == nil {
			t.Fatal("unregistered key should be rejected via the authorization plane")
		}
	})
}
