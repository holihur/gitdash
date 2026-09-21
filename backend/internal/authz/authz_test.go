package authz

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"gitdash/backend/internal/grpcserver"
	"gitdash/backend/internal/store"
)

const testToken = "authz-test-token"

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return st
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
	return sshPub, string(ssh.MarshalAuthorizedKey(sshPub))
}

// seed 准备一个含用户/公钥/IP 封禁/仓库/分支保护的存储，返回登记公钥。
func seed(t *testing.T, st *store.Store) ssh.PublicKey {
	t.Helper()
	for _, u := range []string{"alice", "bob", "carol"} {
		if _, err := st.CreateUser(u, "x"); err != nil {
			t.Fatalf("create user %s: %v", u, err)
		}
	}
	pub, line := newSSHKey(t)
	if _, err := st.CreateKey("alice", "laptop", line, ssh.FingerprintSHA256(pub)); err != nil {
		t.Fatalf("create key: %v", err)
	}
	if _, err := st.AddIPBan("10.0.0.0/8", "test", "admin"); err != nil {
		t.Fatalf("add ip ban: %v", err)
	}
	if _, err := st.CreateRepo("alice", "demo", "", true); err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if err := st.UpsertCollab("alice", "demo", "carol", "write"); err != nil {
		t.Fatalf("upsert collab: %v", err)
	}
	if err := st.SetBranchProtection(&store.BranchProtection{
		Owner: "alice", Repo: "demo", Branch: "main",
		BlockDeletion: true, BlockForcePush: true,
	}); err != nil {
		t.Fatalf("set branch protection: %v", err)
	}
	return pub
}

// assertAuthorizer 对同一组预期断言任意 Authorizer 实现（本地/远端）。
func assertAuthorizer(t *testing.T, az Authorizer, pub ssh.PublicKey) {
	t.Helper()
	ctx := context.Background()

	if _, ok, _, err := az.AuthorizePublicKey(ctx, pub.Type(), pub.Marshal()); err != nil || !ok {
		t.Fatalf("registered key should authorize: ok=%v err=%v", ok, err)
	}
	other, _ := newSSHKey(t)
	if _, ok, reason, err := az.AuthorizePublicKey(ctx, other.Type(), other.Marshal()); err != nil || ok || reason != "unknown public key" {
		t.Fatalf("unknown key: ok=%v reason=%q err=%v", ok, reason, err)
	}
	if banned, err := az.IsIPBanned(ctx, "10.9.8.7"); err != nil || !banned {
		t.Fatalf("10.9.8.7 should be banned: %v %v", banned, err)
	}
	if banned, err := az.IsIPBanned(ctx, "192.168.1.1"); err != nil || banned {
		t.Fatalf("192.168.1.1 should not be banned: %v %v", banned, err)
	}
	if ok, err := az.CanRead(ctx, "alice", "demo", "alice"); err != nil || !ok {
		t.Fatalf("owner can read: %v %v", ok, err)
	}
	if ok, err := az.CanRead(ctx, "alice", "demo", "bob"); err != nil || ok {
		t.Fatalf("stranger cannot read private repo: %v %v", ok, err)
	}
	if ok, err := az.CanWrite(ctx, "alice", "demo", "carol"); err != nil || !ok {
		t.Fatalf("write collaborator can push: %v %v", ok, err)
	}
	if ok, err := az.CanWrite(ctx, "alice", "demo", "bob"); err != nil || ok {
		t.Fatalf("stranger cannot push: %v %v", ok, err)
	}
	rule, err := az.BranchProtection(ctx, "alice", "demo", "main")
	if err != nil || !rule.Protected || !rule.BlockDeletion || !rule.BlockForcePush {
		t.Fatalf("main protection rule: %+v err=%v", rule, err)
	}
	if rule, err := az.BranchProtection(ctx, "alice", "demo", "dev"); err != nil || rule.Protected {
		t.Fatalf("dev has no rule: %+v err=%v", rule, err)
	}
}

func TestStoreAuthorizer(t *testing.T) {
	st := newStore(t)
	pub := seed(t, st)
	assertAuthorizer(t, NewStore(st), pub)

	// 封禁用户后公钥鉴权应被拒绝。
	if err := st.SetUserBanned("alice", true); err != nil {
		t.Fatalf("ban: %v", err)
	}
	if _, ok, reason, err := NewStore(st).AuthorizePublicKey(context.Background(), pub.Type(), pub.Marshal()); err != nil || ok || reason != "account is banned" {
		t.Fatalf("banned user: ok=%v reason=%q err=%v", ok, reason, err)
	}
}

// TestRemoteAuthorizerEndToEnd 验证远端授权器与本地实现语义一致（经授权面 gRPC）。
func TestRemoteAuthorizerEndToEnd(t *testing.T) {
	st := newStore(t)
	pub := seed(t, st)

	srv, err := grpcserver.NewGRPCServer(st, testToken)
	if err != nil {
		t.Fatalf("grpc server: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Stop()

	conn, remote, err := Dial(DialOptions{Addr: ln.Addr().String(), Token: testToken})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	assertAuthorizer(t, remote, pub)
}

func TestDialRequiresAddrAndToken(t *testing.T) {
	if _, _, err := Dial(DialOptions{Token: "x"}); err == nil {
		t.Fatal("expected error when address is empty")
	}
	if _, _, err := Dial(DialOptions{Addr: "127.0.0.1:1"}); err == nil {
		t.Fatal("expected error when token is empty")
	}
}

func TestRemoteAuthorizerRejectsBadToken(t *testing.T) {
	st := newStore(t)
	srv, err := grpcserver.NewGRPCServer(st, testToken)
	if err != nil {
		t.Fatalf("grpc server: %v", err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Stop()

	conn, remote, err := Dial(DialOptions{Addr: ln.Addr().String(), Token: "wrong"})
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := remote.IsIPBanned(context.Background(), "1.2.3.4"); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("wrong token: code = %v, want Unauthenticated", status.Code(err))
	}
}
