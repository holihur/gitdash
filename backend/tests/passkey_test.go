package tests

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"testing"

	"github.com/fxamacker/cbor/v2"
)

// virtualAuthenticator 是一个最小化的软件认证器（ES256 / none attestation），
// 用于在测试中完成完整的 WebAuthn 注册与登录流程。
type virtualAuthenticator struct {
	key       *ecdsa.PrivateKey
	credID    []byte
	rpID      string
	origin    string
	signCount uint32
}

func newVirtualAuthenticator(t *testing.T, rpID, origin string) *virtualAuthenticator {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	credID := make([]byte, 32)
	if _, err := rand.Read(credID); err != nil {
		t.Fatalf("cred id: %v", err)
	}
	return &virtualAuthenticator{key: key, credID: credID, rpID: rpID, origin: origin}
}

func b64url(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }

func (va *virtualAuthenticator) coseKey(t *testing.T) []byte {
	t.Helper()
	// 用 PublicKey.Bytes() 取未压缩点（0x04 || X || Y），避免直接读已废弃的 X/Y 字段。
	pub, err := va.key.PublicKey.Bytes()
	if err != nil {
		t.Fatalf("encode P-256 public key: %v", err)
	}
	if len(pub) != 65 || pub[0] != 0x04 {
		t.Fatalf("unexpected P-256 public key encoding: %x", pub)
	}
	m := map[int]any{
		1:  2,                               // kty: EC2
		3:  -7,                              // alg: ES256
		-1: 1,                               // crv: P-256
		-2: append([]byte{}, pub[1:33]...),  // x
		-3: append([]byte{}, pub[33:65]...), // y
	}
	out, err := cbor.Marshal(m)
	if err != nil {
		t.Fatalf("cbor cose key: %v", err)
	}
	return out
}

// attestation 生成注册响应（PublicKeyCredential JSON 结构）。
func (va *virtualAuthenticator) attestation(t *testing.T, challenge string) map[string]any {
	t.Helper()
	rpIDHash := sha256.Sum256([]byte(va.rpID))
	// UP | UV | AT | BE
	flags := byte(0x01 | 0x04 | 0x40 | 0x08)
	authData := append([]byte{}, rpIDHash[:]...)
	authData = append(authData, flags, 0, 0, 0, 0)   // sign count = 0
	authData = append(authData, make([]byte, 16)...) // AAGUID
	authData = append(authData, byte(len(va.credID)>>8), byte(len(va.credID)))
	authData = append(authData, va.credID...)
	authData = append(authData, va.coseKey(t)...)

	attObj, err := cbor.Marshal(map[string]any{
		"fmt":      "none",
		"attStmt":  map[string]any{},
		"authData": authData,
	})
	if err != nil {
		t.Fatalf("cbor attestation: %v", err)
	}
	clientData := []byte(`{"type":"webauthn.create","challenge":"` + challenge +
		`","origin":"` + va.origin + `","crossOrigin":false}`)
	return map[string]any{
		"id":    b64url(va.credID),
		"rawId": b64url(va.credID),
		"type":  "public-key",
		"response": map[string]any{
			"attestationObject": b64url(attObj),
			"clientDataJSON":    b64url(clientData),
			"transports":        []string{"internal"},
		},
		"clientExtensionResults": map[string]any{},
	}
}

// assertion 生成登录响应，userHandle 可选。
func (va *virtualAuthenticator) assertion(t *testing.T, challenge string, userHandle []byte) map[string]any {
	t.Helper()
	rpIDHash := sha256.Sum256([]byte(va.rpID))
	va.signCount++
	// UP | UV | BE（与注册时的 backup-eligible 保持一致）
	flags := byte(0x01 | 0x04 | 0x08)
	authData := append([]byte{}, rpIDHash[:]...)
	authData = append(authData, flags)
	var sc [4]byte
	binary.BigEndian.PutUint32(sc[:], va.signCount)
	authData = append(authData, sc[:]...)

	clientData := []byte(`{"type":"webauthn.get","challenge":"` + challenge +
		`","origin":"` + va.origin + `","crossOrigin":false}`)
	clientHash := sha256.Sum256(clientData)
	signed := append(append([]byte{}, authData...), clientHash[:]...)
	digest := sha256.Sum256(signed)
	sig, err := ecdsa.SignASN1(rand.Reader, va.key, digest[:])
	if err != nil {
		t.Fatalf("sign assertion: %v", err)
	}
	resp := map[string]any{
		"authenticatorData": b64url(authData),
		"clientDataJSON":    b64url(clientData),
		"signature":         b64url(sig),
	}
	if len(userHandle) > 0 {
		resp["userHandle"] = b64url(userHandle)
	}
	return map[string]any{
		"id":                     b64url(va.credID),
		"rawId":                  b64url(va.credID),
		"type":                   "public-key",
		"response":               resp,
		"clientExtensionResults": map[string]any{},
	}
}

// challengeFrom 从 begin 响应中提取 base64url challenge。
func challengeFrom(t *testing.T, begin map[string]any) string {
	t.Helper()
	pk, ok := begin["public_key"].(map[string]any)
	if !ok {
		t.Fatalf("public_key missing: %v", begin)
	}
	inner, ok := pk["publicKey"].(map[string]any)
	if !ok {
		t.Fatalf("publicKey missing: %v", pk)
	}
	ch, _ := inner["challenge"].(string)
	if ch == "" {
		t.Fatalf("challenge missing: %v", inner)
	}
	return ch
}

// setupPasskeyEnv 固定 WebAuthn 的 RP 配置，使测试不依赖真实域名。
func setupPasskeyEnv(t *testing.T, env *Env) (rpID, origin string) {
	t.Helper()
	rpID = "example.com"
	origin = env.BaseURL
	t.Setenv("GITDASH_WEBAUTHN_RPID", rpID)
	t.Setenv("GITDASH_WEBAUTHN_ORIGINS", origin)
	return rpID, origin
}

func TestPasskeyRegisterListDelete(t *testing.T) {
	env := start(t)
	rpID, origin := setupPasskeyEnv(t, env)
	alice := register(t, env, "alice", "alice-pass-123")

	// providers 广告 passkey 可用
	prov := alice.mustStatus("GET", "/auth/providers", nil, 200)
	if pk, ok := prov["passkey"].(map[string]any); !ok || pk["enabled"] != true {
		t.Fatalf("providers passkey = %v", prov["passkey"])
	}

	// 初始为空
	list := alice.mustStatus("GET", "/me/passkeys", nil, 200)
	if arr, ok := list["passkeys"].([]any); !ok || len(arr) != 0 {
		t.Fatalf("initial passkeys = %v", list)
	}

	// 开始注册
	begin := alice.mustStatus("POST", "/me/passkeys/register/begin",
		map[string]string{"name": "My Laptop"}, 200)
	sessionID, _ := begin["session_id"].(string)
	if sessionID == "" {
		t.Fatalf("register begin = %v", begin)
	}
	challenge := challengeFrom(t, begin)

	va := newVirtualAuthenticator(t, rpID, origin)
	created := alice.mustStatus("POST", "/me/passkeys/register/finish", map[string]any{
		"session_id": sessionID,
		"name":       "My Laptop",
		"credential": va.attestation(t, challenge),
	}, 201)
	if created["name"] != "My Laptop" || created["id"] == nil {
		t.Fatalf("created passkey = %v", created)
	}

	// 列表包含 1 条
	list = alice.mustStatus("GET", "/me/passkeys", nil, 200)
	arr, _ := list["passkeys"].([]any)
	if len(arr) != 1 {
		t.Fatalf("passkeys after register = %v", list)
	}
	first, _ := arr[0].(map[string]any)
	idFloat, _ := first["id"].(float64)
	if idFloat == 0 {
		t.Fatalf("passkey id missing: %v", first)
	}

	// 同名凭据重复注册（exclusion 由客户端保证，这里用同一 credID 直接重放）
	begin2 := alice.mustStatus("POST", "/me/passkeys/register/begin", nil, 200)
	challenge2 := challengeFrom(t, begin2)
	alice.mustFail("POST", "/me/passkeys/register/finish", map[string]any{
		"session_id": begin2["session_id"],
		"credential": va.attestation(t, challenge2),
	}, 409)

	// 删除
	alice.mustStatus("DELETE", "/me/passkeys/1", nil, 204)
	list = alice.mustStatus("GET", "/me/passkeys", nil, 200)
	if arr, _ := list["passkeys"].([]any); len(arr) != 0 {
		t.Fatalf("passkeys after delete = %v", list)
	}
	// 再删返回 404
	alice.mustFail("DELETE", "/me/passkeys/1", nil, 404)

	// 未认证不可管理
	(&Client{env: env}).mustFail("GET", "/me/passkeys", nil, 401)
}

func TestPasskeyLoginUsernameAndDiscoverable(t *testing.T) {
	env := start(t)
	rpID, origin := setupPasskeyEnv(t, env)
	alice := register(t, env, "alice", "alice-pass-123")

	va := newVirtualAuthenticator(t, rpID, origin)
	begin := alice.mustStatus("POST", "/me/passkeys/register/begin", nil, 200)
	alice.mustStatus("POST", "/me/passkeys/register/finish", map[string]any{
		"session_id": begin["session_id"],
		"name":       "Test Key",
		"credential": va.attestation(t, challengeFrom(t, begin)),
	}, 201)

	anon := &Client{env: env}

	// 用户名限定登录（allowCredentials）
	loginBegin := anon.mustStatus("POST", "/auth/passkey/begin",
		map[string]string{"username": "alice"}, 200)
	assertion := va.assertion(t, challengeFrom(t, loginBegin), nil)
	ok := anon.mustStatus("POST", "/auth/passkey/finish", map[string]any{
		"session_id": loginBegin["session_id"],
		"credential": assertion,
	}, 200)
	if ok["token"] == nil || ok["username"] != "alice" {
		t.Fatalf("passkey login = %v", ok)
	}

	// 会话有效
	authed := &Client{env: env, token: ok["token"].(string)}
	if m := authed.mustStatus("GET", "/me", nil, 200); m["username"] != "alice" {
		t.Fatalf("session after passkey login = %v", m)
	}

	// 密码less / discoverable 登录：带 userHandle
	disc := anon.mustStatus("POST", "/auth/passkey/begin", nil, 200)
	handle := make([]byte, 8)
	binary.BigEndian.PutUint64(handle, uint64(userIDOf(t, env, "alice")))
	assertion2 := va.assertion(t, challengeFrom(t, disc), handle)
	ok2 := anon.mustStatus("POST", "/auth/passkey/finish", map[string]any{
		"session_id": disc["session_id"],
		"credential": assertion2,
	}, 200)
	if ok2["username"] != "alice" {
		t.Fatalf("discoverable login = %v", ok2)
	}

	// challenge 一次性：重放失败
	anon.mustFail("POST", "/auth/passkey/finish", map[string]any{
		"session_id": disc["session_id"],
		"credential": assertion2,
	}, 401)

	// 未注册用户 begin 仍返回 discoverable 挑战（避免用户名枚举）
	anon.mustStatus("POST", "/auth/passkey/begin",
		map[string]string{"username": "nobody"}, 200)

	// 伪造签名被拒
	fake := newVirtualAuthenticator(t, rpID, origin)
	disc2 := anon.mustStatus("POST", "/auth/passkey/begin", nil, 200)
	anon.mustFail("POST", "/auth/passkey/finish", map[string]any{
		"session_id": disc2["session_id"],
		"credential": fake.assertion(t, challengeFrom(t, disc2), handle),
	}, 401)
}

// userIDOf 返回用户的内部主键 ID（discoverable 登录需要 userHandle）。
func userIDOf(t *testing.T, env *Env, username string) int64 {
	t.Helper()
	ua, err := env.Store.GetByUsername(username)
	if err != nil {
		t.Fatalf("get user %s: %v", username, err)
	}
	return ua.ID
}
