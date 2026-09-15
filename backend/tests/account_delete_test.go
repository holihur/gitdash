package tests

// account_delete_test.go 账号自助注销（彻底删除）黑盒测试。

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"gitdash/backend/internal/store"
)

// listIssuesRaw 解码 issue 列表（返回数组，Client.do 只支持对象）。
func listIssuesRaw(t *testing.T, c *Client, path string) []map[string]any {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api"+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get %s: %v", path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("get %s: status %d", path, resp.StatusCode)
	}
	var out []map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return out
}

func TestDeleteOwnAccountPurgesData(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bobby := register(t, env, "bobby", "bob-pass-123456")

	// alice 名下仓库 + 自有内容
	alice.mustStatus("POST", "/repos", map[string]any{"name": "delrepo", "private": false}, 201)
	alice.mustStatus("POST", "/users/alice/repos/delrepo/issues",
		map[string]string{"title": "mine", "body": "b"}, 201)

	// bobby 的仓库（alice 会 star / watch 它并在其中留下痕迹）
	bobby.mustStatus("POST", "/repos", map[string]any{"name": "hostrepo", "private": false}, 201)

	// 社交关系 / 凭据 / 配置
	alice.mustStatus("POST", "/users/bobby/follow", nil, 200)
	alice.mustStatus("PUT", "/users/bobby/repos/hostrepo/star", nil, 200)
	alice.mustStatus("PUT", "/users/bobby/repos/hostrepo/watch", nil, 200)
	bobby.mustStatus("PUT", "/users/alice/repos/delrepo/star", nil, 200)
	alice.mustStatus("POST", "/keys", map[string]string{"name": "k", "public_key": testPublicKey}, 201)
	alice.mustStatus("POST", "/me/byok",
		map[string]string{"name": "mykey", "provider": "compatible", "api_key": "sk-test"}, 201)

	// alice 作为协作者在 bobby 的仓库里留下痕迹
	bobby.mustStatus("POST", "/users/bobby/repos/hostrepo/collabs",
		map[string]string{"username": "alice", "permission": "write"}, 200)
	issue := alice.mustStatus("POST", "/users/bobby/repos/hostrepo/issues",
		map[string]string{"title": "guest issue", "body": "hi"}, 201)
	num := int(issue["number"].(float64))
	alice.mustStatus("POST", "/users/bobby/repos/hostrepo/issues/"+strconv.Itoa(num)+"/comments",
		map[string]string{"body": "guest comment"}, 201)

	// PAT（不应能用于注销）
	pat := alice.mustStatus("POST", "/tokens",
		map[string]any{"name": "ci", "scopes": []string{"repo"}}, 201)
	patToken, _ := pat["token"].(string)
	if patToken == "" {
		t.Fatal("pat token missing")
	}
	patClient := &Client{env: env, token: patToken}

	// 错误密码被拒绝，账号保留
	alice.mustFail("DELETE", "/me", map[string]string{"password": "wrong-password"}, 401)
	alice.mustStatus("GET", "/me", nil, 200)

	// PAT 不能注销（高风险操作需交互式会话）
	patClient.mustFail("DELETE", "/me", map[string]string{"password": "alice-pass-123"}, 403)

	// 磁盘上 git 仓库存在
	repoPath := filepath.Join(env.ReposDir, "alice", "delrepo.git")
	if _, err := os.Stat(repoPath); err != nil {
		t.Fatalf("repo dir should exist before delete: %v", err)
	}

	// 正确注销
	alice.mustStatus("DELETE", "/me", map[string]string{"password": "alice-pass-123"}, 204)

	// 会话与 PAT 全部失效
	alice.mustFail("GET", "/me", nil, 401)
	patClient.mustFail("GET", "/me", nil, 401)
	// 账号在存储层消失
	if _, err := env.Store.GetByUsername("alice"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted user lookup err = %v, want ErrNotFound", err)
	}
	// 名下仓库（DB + 磁盘）整体消失
	bobby.mustFail("GET", "/users/alice/repos/delrepo", nil, 404)
	if _, err := os.Stat(repoPath); !os.IsNotExist(err) {
		t.Fatalf("repo dir should be removed, stat err = %v", err)
	}

	// 被遗忘权：他人仓库中的内容保留但作者匿名化
	issues := listIssuesRaw(t, bobby, "/users/bobby/repos/hostrepo/issues")
	found := false
	for _, it := range issues {
		if it["title"] == "guest issue" {
			found = true
			if it["author"] != "deleted-user" {
				t.Fatalf("guest issue author = %v, want deleted-user", it["author"])
			}
		}
	}
	if !found {
		t.Fatal("guest issue missing after host account deletion")
	}

	// 注销后可重新注册同名账号
	register(t, env, "alice", "alice-pass-123").mustStatus("GET", "/me", nil, 200)
}

func TestDeleteOwnAccountRequiresMFA(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	e := alice.mustStatus("POST", "/me/mfa/enroll", nil, 200)
	secret, _ := e["secret"].(string)
	alice.mustStatus("POST", "/me/mfa/activate", map[string]string{"code": mfaCode(t, secret)}, 204)

	// 缺少 / 错误验证码被拒绝
	alice.mustFail("DELETE", "/me", map[string]string{"password": "alice-pass-123"}, 400)
	alice.mustFail("DELETE", "/me",
		map[string]string{"password": "alice-pass-123", "code": "000000"}, 400)
	// 正确密码 + 验证码
	alice.mustStatus("DELETE", "/me",
		map[string]string{"password": "alice-pass-123", "code": mfaCode(t, secret)}, 204)
	alice.mustFail("GET", "/me", nil, 401)
	if _, err := env.Store.GetByUsername("alice"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("user with MFA still present after deletion: %v", err)
	}
}

func TestDeleteOwnAccountUnauthenticated(t *testing.T) {
	env := start(t)
	(&Client{env: env}).mustFail("DELETE", "/me", map[string]string{"password": "x"}, 401)
}
