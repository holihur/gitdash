package grpcserver

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"gitdash/backend/internal/grpcserver/authzv1"
	"gitdash/backend/internal/store"
)

const testToken = "s3cr3t-service-token"

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
}

// newSSHKey 生成一对 ed25519 公钥及其 authorized_keys 行。
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
	return sshPub, string(ssh.MarshalAuthorizedKey(sshPub))
}

// startTestServer 在随机端口启动授权面服务并返回客户端。
func startTestServer(t *testing.T, st *store.Store, token string) (authzv1.AuthzServiceClient, func()) {
	t.Helper()
	srv, err := NewGRPCServer(st, token)
	if err != nil {
		t.Fatalf("new grpc server: %v", err)
	}
	ln := listenLocal(t)
	go func() { _ = srv.Serve(ln) }()
	conn, err := grpc.NewClient(ln.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return authzv1.NewAuthzServiceClient(conn), func() {
		_ = conn.Close()
		srv.Stop()
		_ = ln.Close()
	}
}

func authCtx(token string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "authorization", "Bearer "+token)
}

// listenLocal 在 127.0.0.1 随机端口监听（仅测试用）。
func listenLocal(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return ln
}

func TestAuthorizePublicKeyOK(t *testing.T) {
	st := newStore(t)
	if _, err := st.CreateUser("alice", "x"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	pub, line := newSSHKey(t)
	if _, err := st.CreateKey("alice", "laptop", line, ssh.FingerprintSHA256(pub)); err != nil {
		t.Fatalf("create key: %v", err)
	}

	client, stop := startTestServer(t, st, testToken)
	defer stop()

	resp, err := client.AuthorizePublicKey(authCtx(testToken), &authzv1.AuthorizePublicKeyRequest{
		KeyType: pub.Type(),
		KeyBlob: pub.Marshal(),
	})
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if !resp.GetAuthorized() || resp.GetUsername() != "alice" {
		t.Fatalf("got authorized=%v username=%q, want true/alice", resp.GetAuthorized(), resp.GetUsername())
	}
}

func TestAuthorizePublicKeyUnknown(t *testing.T) {
	st := newStore(t)
	if _, err := st.CreateUser("alice", "x"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	pub, line := newSSHKey(t)
	if _, err := st.CreateKey("alice", "laptop", line, ssh.FingerprintSHA256(pub)); err != nil {
		t.Fatalf("create key: %v", err)
	}

	// 另一把从未登记的密钥
	other, _ := newSSHKey(t)

	client, stop := startTestServer(t, st, testToken)
	defer stop()

	resp, err := client.AuthorizePublicKey(authCtx(testToken), &authzv1.AuthorizePublicKeyRequest{
		KeyType: other.Type(),
		KeyBlob: other.Marshal(),
	})
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if resp.GetAuthorized() || resp.GetReason() != "unknown public key" {
		t.Fatalf("got authorized=%v reason=%q, want false/unknown public key", resp.GetAuthorized(), resp.GetReason())
	}
}

func TestAuthorizePublicKeyBanned(t *testing.T) {
	st := newStore(t)
	if _, err := st.CreateUser("banned", "x"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	pub, line := newSSHKey(t)
	if _, err := st.CreateKey("banned", "laptop", line, ssh.FingerprintSHA256(pub)); err != nil {
		t.Fatalf("create key: %v", err)
	}
	if err := st.SetUserBanned("banned", true); err != nil {
		t.Fatalf("ban user: %v", err)
	}

	client, stop := startTestServer(t, st, testToken)
	defer stop()

	resp, err := client.AuthorizePublicKey(authCtx(testToken), &authzv1.AuthorizePublicKeyRequest{
		KeyType: pub.Type(),
		KeyBlob: pub.Marshal(),
	})
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if resp.GetAuthorized() || resp.GetReason() != "account is banned" {
		t.Fatalf("got authorized=%v reason=%q, want false/account is banned", resp.GetAuthorized(), resp.GetReason())
	}
}

func TestIsIPBanned(t *testing.T) {
	st := newStore(t)
	if _, err := st.AddIPBan("10.0.0.0/8", "test", "admin"); err != nil {
		t.Fatalf("add ip ban: %v", err)
	}

	client, stop := startTestServer(t, st, testToken)
	defer stop()

	hit, err := client.IsIPBanned(authCtx(testToken), &authzv1.IsIPBannedRequest{Ip: "10.9.8.7"})
	if err != nil {
		t.Fatalf("rpc banned: %v", err)
	}
	if !hit.GetBanned() {
		t.Fatalf("10.9.8.7 should be banned")
	}

	miss, err := client.IsIPBanned(authCtx(testToken), &authzv1.IsIPBannedRequest{Ip: "192.168.1.1"})
	if err != nil {
		t.Fatalf("rpc clean: %v", err)
	}
	if miss.GetBanned() {
		t.Fatalf("192.168.1.1 should not be banned")
	}
}

func TestCanRead(t *testing.T) {
	st := newStore(t)
	for _, u := range []string{"alice", "bob"} {
		if _, err := st.CreateUser(u, "x"); err != nil {
			t.Fatalf("create user %s: %v", u, err)
		}
	}
	if _, err := st.CreateRepo("alice", "private", "", true); err != nil {
		t.Fatalf("create private repo: %v", err)
	}
	if _, err := st.CreateRepo("alice", "public", "", false); err != nil {
		t.Fatalf("create public repo: %v", err)
	}

	client, stop := startTestServer(t, st, testToken)
	defer stop()

	cases := []struct {
		owner, repo, user string
		want              bool
	}{
		{"alice", "private", "alice", true}, // owner
		{"alice", "private", "bob", false},  // 陌生人 + 私有
		{"alice", "public", "bob", true},    // 公开仓库
	}
	for _, c := range cases {
		resp, err := client.CanRead(authCtx(testToken), &authzv1.CanReadRequest{
			Owner: c.owner, Repo: c.repo, Username: c.user,
		})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		if resp.GetAllowed() != c.want {
			t.Errorf("CanRead(%s/%s, %s) = %v, want %v", c.owner, c.repo, c.user, resp.GetAllowed(), c.want)
		}
	}
}

func TestCanWrite(t *testing.T) {
	st := newStore(t)
	for _, u := range []string{"alice", "bob", "carol"} {
		if _, err := st.CreateUser(u, "x"); err != nil {
			t.Fatalf("create user %s: %v", u, err)
		}
	}
	if _, err := st.CreateRepo("alice", "demo", "", true); err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if err := st.UpsertCollab("alice", "demo", "carol", "write"); err != nil {
		t.Fatalf("upsert collab: %v", err)
	}

	client, stop := startTestServer(t, st, testToken)
	defer stop()

	cases := []struct {
		user string
		want bool
	}{
		{"alice", true}, // owner
		{"bob", false},  // 陌生人
		{"carol", true}, // write 协作者
	}
	for _, c := range cases {
		resp, err := client.CanWrite(authCtx(testToken), &authzv1.CanWriteRequest{
			Owner: "alice", Repo: "demo", Username: c.user,
		})
		if err != nil {
			t.Fatalf("rpc: %v", err)
		}
		if resp.GetAllowed() != c.want {
			t.Errorf("CanWrite(alice/demo, %s) = %v, want %v", c.user, resp.GetAllowed(), c.want)
		}
	}
}

func TestTokenRequired(t *testing.T) {
	st := newStore(t)
	client, stop := startTestServer(t, st, testToken)
	defer stop()

	// 无令牌
	_, err := client.IsIPBanned(context.Background(), &authzv1.IsIPBannedRequest{Ip: "1.2.3.4"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no token: code = %v, want Unauthenticated", status.Code(err))
	}

	// 错误令牌
	_, err = client.IsIPBanned(authCtx("wrong"), &authzv1.IsIPBannedRequest{Ip: "1.2.3.4"})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong token: code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestNewGRPCServerRequiresToken(t *testing.T) {
	if _, err := NewGRPCServer(newStore(t), ""); err == nil {
		t.Fatal("expected error when token is empty")
	}
}

func TestBranchProtection(t *testing.T) {
	st := newStore(t)
	if _, err := st.CreateUser("alice", "x"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := st.CreateRepo("alice", "demo", "", true); err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if err := st.SetBranchProtection(&store.BranchProtection{
		Owner: "alice", Repo: "demo", Branch: "main",
		BlockDeletion: true, BlockForcePush: true,
	}); err != nil {
		t.Fatalf("set protection: %v", err)
	}

	client, stop := startTestServer(t, st, testToken)
	defer stop()

	resp, err := client.BranchProtection(authCtx(testToken), &authzv1.BranchProtectionRequest{
		Owner: "alice", Repo: "demo", Branch: "main",
	})
	if err != nil {
		t.Fatalf("rpc: %v", err)
	}
	if !resp.GetProtected() || !resp.GetBlockDeletion() || !resp.GetBlockForcePush() {
		t.Fatalf("main rule mismatch: %+v", resp)
	}

	// 无规则的 branch → protected=false，且不报错
	none, err := client.BranchProtection(authCtx(testToken), &authzv1.BranchProtectionRequest{
		Owner: "alice", Repo: "demo", Branch: "dev",
	})
	if err != nil {
		t.Fatalf("rpc dev: %v", err)
	}
	if none.GetProtected() {
		t.Fatalf("dev should have no rule: %+v", none)
	}
}
