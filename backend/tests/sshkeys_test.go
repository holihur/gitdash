package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

const testPublicKey = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIB/2bSPKmpoAAjDGrT6JcVNzig2r3ZGo1SvB2x8mNQiq test@example.com"

func TestSSHKeyCRUD(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	// 添加
	m := alice.mustStatus("POST", "/keys", map[string]string{
		"name":       "laptop",
		"public_key": testPublicKey,
	}, 201)
	fp, _ := m["fingerprint"].(string)
	if len(fp) < len("SHA256:") {
		t.Fatalf("fingerprint = %q", fp)
	}
	id, _ := m["id"].(float64)
	if id == 0 {
		t.Fatalf("id = %v", m)
	}

	// 同一把公钥再添加（全局唯一指纹）
	alice.mustFail("POST", "/keys", map[string]string{"name": "dup", "public_key": testPublicKey}, 409)

	// 非法公钥
	alice.mustFail("POST", "/keys", map[string]string{"name": "bad", "public_key": "not-a-key"}, 400)
	alice.mustFail("POST", "/keys", map[string]string{"name": "empty"}, 400)

	// 列表
	keys := listKeys(t, alice)
	if len(keys) != 1 || keys[0].Name != "laptop" || keys[0].Fingerprint != fp {
		t.Fatalf("keys = %+v", keys)
	}

	// 删除
	alice.mustStatus("DELETE", fmt.Sprintf("/keys/%d", int(id)), nil, 204)
	if got := len(listKeys(t, alice)); got != 0 {
		t.Fatalf("keys after delete = %d", got)
	}
	alice.mustFail("DELETE", fmt.Sprintf("/keys/%d", int(id)), nil, 404)
}

func TestSSHKeyUserBinding(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/keys", map[string]string{"name": "a", "public_key": testPublicKey}, 201)

	// bob 无法删除 alice 的 key，也看不到
	aliceKeys := listKeys(t, alice)
	if len(aliceKeys) != 1 {
		t.Fatalf("alice keys = %+v", aliceKeys)
	}
	bob.mustFail("DELETE", fmt.Sprintf("/keys/%d", aliceKeys[0].ID), nil, 404)
	if got := len(listKeys(t, bob)); got != 0 {
		t.Fatalf("bob should see no keys, got %d", got)
	}

	// bob 添加自己的 key
	bob.mustStatus("POST", "/keys", map[string]string{"name": "b", "public_key": testPublicKey2}, 201)
	if got := len(listKeys(t, bob)); got != 1 {
		t.Fatalf("bob keys = %d", got)
	}
}

const testPublicKey2 = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIPl/ZP5Rl9mqJTJPQNLBb7fnWfRkVZPXb3cRrABFHY+z bob@example.com"

type Key struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Fingerprint string `json:"fingerprint"`
}

func listKeys(t *testing.T, c *Client) []Key {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api/keys", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("list keys: status %d", resp.StatusCode)
	}
	var keys []Key
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		t.Fatalf("decode keys: %v", err)
	}
	return keys
}
