package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// genGPGKey 生成一个无口令、仅签名的 RSA 测试密钥，返回 (homedir, fingerprint, armor)。
func genGPGKey(t *testing.T, dir, name, email string) (string, string, string) {
	t.Helper()
	homedir := filepath.Join(dir, "gnupg")
	if err := os.MkdirAll(homedir, 0o755); err != nil {
		t.Fatal(err)
	}
	params := filepath.Join(dir, "gpg-params")
	content := fmt.Sprintf(`%%no-protection
Key-Type: RSA
Key-Length: 2048
Key-Usage: sign
Name-Real: %s
Name-Email: %s
Expire-Date: 0
%%commit
`, name, email)
	if err := os.WriteFile(params, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	runCmd(t, homedir, []string{"GNUPGHOME=" + homedir}, "gpg", "--batch", "--generate-key", params)

	out := runCmd(t, homedir, []string{"GNUPGHOME=" + homedir}, "gpg", "--list-keys", "--with-colons", "--fingerprint")
	re := regexp.MustCompile(`fpr:::::::::([0-9A-F]{40}):`)
	m := re.FindStringSubmatch(out)
	if len(m) != 2 {
		t.Fatalf("no fingerprint in gpg output: %s", out)
	}
	fingerprint := m[1]
	armor := runCmd(t, homedir, []string{"GNUPGHOME=" + homedir}, "gpg", "--armor", "--export", fingerprint)
	if !strings.Contains(armor, "BEGIN PGP PUBLIC KEY BLOCK") {
		t.Fatal("empty armored export")
	}
	return homedir, fingerprint, armor
}

func TestGPGKeyCRUD(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	_, fp, armor := genGPGKey(t, env.DataDir, "alice gpg", "alice@example.com")

	// 添加
	m := alice.mustStatus("POST", "/gpg", map[string]string{"armor": armor}, 201)
	if got, _ := m["fingerprint"].(string); !strings.EqualFold(got, fp) {
		t.Fatalf("fingerprint = %q want %s", got, fp)
	}
	id := int64(m["id"].(float64))

	// 重复添加（同用户 & 跨用户）409
	alice.mustFail("POST", "/gpg", map[string]string{"armor": armor}, 409)
	bob.mustFail("POST", "/gpg", map[string]string{"armor": armor}, 409)

	// 非法 armor / 空
	alice.mustFail("POST", "/gpg", map[string]string{"armor": "not-a-key"}, 400)
	alice.mustFail("POST", "/gpg", map[string]string{"armor": ""}, 400)

	// 列表 & 删除
	// 列表 & 删除（mustStatus 已校验 200）
	alice.mustStatus("GET", "/gpg", nil, 200)
	alice.mustStatus("DELETE", fmt.Sprintf("/gpg/%d", id), nil, 204)
	alice.mustFail("DELETE", fmt.Sprintf("/gpg/%d", id), nil, 404)

	// 未认证
	(&Client{env: env}).mustFail("GET", "/gpg", nil, 401)
}

// TestGPGVerifiedCommit 端到端：gpg 签名提交 -> 提交 API 返回 gpg_verified。
func TestGPGVerifiedCommit(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen", "gpg")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "signed"}, 201)

	key, pub := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "e2e", "public_key": pub}, 201)

	gpgHome, fp, armor := genGPGKey(t, env.DataDir, "alice gpg", "alice@example.com")
	alice.mustStatus("POST", "/gpg", map[string]string{"armor": armor}, 201)

	work := t.TempDir()
	runCmd(t, work, sshEnv(key), "git", "clone", "-q",
		fmt.Sprintf("ssh://git@127.0.0.1:%s/signed.git", env.SSHPort), "signed")
	repo := filepath.Join(work, "signed")
	signedEnv := append(sshEnv(key), "GNUPGHOME="+gpgHome)

	// 未签名提交
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("plain"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, repo, sshEnv(key), "git", "add", "-A")
	runCmd(t, repo, sshEnv(key), "git", "-c", "user.name=alice", "-c", "user.email=alice@example.com",
		"commit", "-q", "-m", "plain commit")
	runCmd(t, repo, sshEnv(key), "git", "push", "-q", "origin", "HEAD")

	// 签名提交（-S）
	if err := os.WriteFile(filepath.Join(repo, "b.txt"), []byte("signed"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, repo, sshEnv(key), "git", "add", "-A")
	runCmd(t, repo, signedEnv, "git",
		"-c", "user.name=alice", "-c", "user.email=alice@example.com",
		"-c", "user.signingkey="+fp, "-c", "commit.gpgsign=true",
		"commit", "-q", "-S", "-m", "signed commit")
	runCmd(t, repo, signedEnv, "git", "push", "-q", "origin", "HEAD")

	// 读取提交列表：签名提交带 gpg_verified，未签名不带
	var commits []map[string]any
	if err := json.Unmarshal([]byte(rawGet(t, alice, "/repos/signed/commits?ref=main")), &commits); err != nil {
		t.Fatalf("decode commits: %v", err)
	}
	signedFound := false
	for _, c := range commits {
		switch c["message"] {
		case "signed commit":
			if v, _ := c["gpg_verified"].(string); v != "alice" {
				t.Fatalf("signed commit gpg_verified = %q want alice", v)
			}
			signedFound = true
		case "plain commit":
			if v, _ := c["gpg_verified"].(string); v != "" {
				t.Fatalf("plain commit should not be verified, got %q", v)
			}
		}
	}
	if !signedFound {
		t.Fatalf("signed commit missing from log; commits = %v", commits)
	}
}
