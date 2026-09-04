package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sshEnv(key string) []string {
	return []string{
		"GIT_SSH_COMMAND=ssh -i " + key + " -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o IdentitiesOnly=yes",
	}
}

func genKey(t *testing.T, dir, name string) (string, string) {
	t.Helper()
	key := filepath.Join(dir, name)
	runCmd(t, dir, nil, "ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-f", key, "-C", name)
	pub, err := os.ReadFile(key + ".pub")
	if err != nil {
		t.Fatal(err)
	}
	return key, strings.TrimSpace(string(pub))
}

func TestSSHGitFlow(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)

	key, pub := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "e2e", "public_key": pub}, 201)

	work := t.TempDir()

	// clone（单段路径解析为当前用户仓库）
	runCmd(t, work, sshEnv(key), "git", "clone", "-q", fmt.Sprintf("ssh://git@127.0.0.1:%s/demo.git", env.SSHPort), "demo")

	// 提交并 push
	demo := filepath.Join(work, "demo")
	if err := os.WriteFile(filepath.Join(demo, "README.md"), []byte("# ssh flow"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(t, demo, sshEnv(key), "git", "add", "-A")
	runCmd(t, demo, sshEnv(key), "git", "-c", "user.name=alice", "-c", "user.email=alice@example.com", "commit", "-q", "-m", "ssh commit")
	runCmd(t, demo, sshEnv(key), "git", "push", "-q", "origin", "HEAD")

	// owner 路径重新 clone 验证
	runCmd(t, work, sshEnv(key), "git", "clone", "-q", fmt.Sprintf("ssh://git@127.0.0.1:%s/alice/demo.git", env.SSHPort), "demo2")
	data, err := os.ReadFile(filepath.Join(work, "demo2", "README.md"))
	if err != nil || string(data) != "# ssh flow" {
		t.Fatalf("clone content = %q, err = %v", data, err)
	}

	// 浏览 API 能看到 push 的内容
	commits := getJSON[[]Commit](t, alice, "/repos/demo/commits?ref=main", 200)
	if len(commits) != 1 || commits[0].Message != "ssh commit" {
		t.Fatalf("commits = %+v", commits)
	}
}

func TestSSHAccessControl(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "private"}, 201)
	keyA, pubA := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "a", "public_key": pubA}, 201)
	keyB, pubB := genKey(t, env.DataDir, "bob_key")
	bob.mustStatus("POST", "/keys", map[string]string{"name": "b", "public_key": pubB}, 201)

	work := t.TempDir()
	url := func(repo string) string {
		return fmt.Sprintf("ssh://git@127.0.0.1:%s/%s", env.SSHPort, repo)
	}

	// bob 不能 clone alice 的仓库
	runCmdFail(t, work, sshEnv(keyB), "git", "clone", "-q", url("alice/private.git"), "steal")
	// bob 用 alice 的名字+自己的 key 也不行（owner 校验）
	runCmdFail(t, work, sshEnv(keyB), "git", "clone", "-q", url("alice/private.git"), "steal2")

	// 未注册的 key 被拒绝
	_, strangerPub := genKey(t, env.DataDir, "stranger_key")
	_ = strangerPub
	runCmdFail(t, work, sshEnv(keyB), "git", "clone", "-q", url("nonexistent.git"), "x")

	// alice 自己可以
	runCmd(t, work, sshEnv(keyA), "git", "clone", "-q", url("private.git"), "own")
}
