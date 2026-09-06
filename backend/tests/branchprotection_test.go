package tests

// branchprotection_test.go 分支保护 + PR 合并门禁黑盒测试：
//   - 保护规则 CRUD 与权限（owner 才能设置）
//   - 合并门禁：min_approvals 未达标拒绝合并；作者自己的 approve 不计；
//     head 前进后旧 approve 过期失效；达标后可合并
//   - web API 删除受保护分支被拒
//   - SSH push：受保护分支禁删 / 禁 force push；普通 push 与未保护分支不受影响

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const bpPath = "/users/alice/repos/gate/branch-protections"

func TestBranchProtectionMergeGate(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "gate"}, 201)

	key, pub := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "e2e", "public_key": pub}, 201)

	work := t.TempDir()
	gitDo(t, work, key, "clone", "-q", fmt.Sprintf("ssh://git@127.0.0.1:%s/gate.git", env.SSHPort), "gate")
	repo := filepath.Join(work, "gate")
	write := func(f, body string) {
		if err := os.WriteFile(filepath.Join(repo, f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		gitDo(t, repo, key, "add", "-A")
		gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "-m", f)
	}
	write("base.txt", "base")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")

	// ---- 保护规则 CRUD 与权限 ----
	bob.mustFail("PUT", bpPath+"/main",
		map[string]any{"min_approvals": 1, "block_deletion": true, "block_force_push": true}, 404)
	alice.mustStatus("PUT", bpPath+"/main",
		map[string]any{"min_approvals": 1, "block_deletion": true, "block_force_push": true}, 200)
	var prots []map[string]any
	if err := json.Unmarshal([]byte(rawGet(t, alice, bpPath)), &prots); err != nil || len(prots) != 1 {
		t.Fatalf("list protections = %s err=%v", rawGet(t, alice, bpPath), err)
	}

	// web API 删除受保护分支被拒
	alice.mustFail("DELETE", "/users/alice/repos/gate/refs/branches/main", nil, 409)

	// ---- 合并门禁 ----
	gitDo(t, repo, key, "checkout", "-q", "-b", "feat")
	write("feature.txt", "feature")
	gitDo(t, repo, key, "push", "-q", "-u", "origin", "feat")

	m := alice.mustStatus("POST", prPath("alice", "gate", ""),
		map[string]string{"title": "feat", "source_branch": "feat", "target_branch": "main"}, 201)
	num := fmt.Sprintf("/%d", int(m["number"].(float64)))

	// 未达标：合并被拒（review_required）
	if code, v := mergeResult(t, alice, "alice", "gate", num); code != 409 || v["code"] != "review_required" {
		t.Fatalf("merge without approval = %d %v", code, v)
	}

	// 作者自己的 approve 不计
	alice.mustStatus("POST", prPath("alice", "gate", num+"/reviews"),
		map[string]string{"state": "approve"}, 201)
	if code, v := mergeResult(t, alice, "alice", "gate", num); code != 409 {
		t.Fatalf("author approve should not count: %d %v", code, v)
	}

	// gate 汇总：approvals=0（作者不算）
	var gate struct {
		Gate map[string]any `json:"gate"`
	}
	if err := json.Unmarshal([]byte(rawGet(t, alice, prPath("alice", "gate", num+"/reviews"))), &gate); err != nil {
		t.Fatal(err)
	}
	if g := gate.Gate; g == nil || g["required"].(float64) != 1 || g["approvals"].(float64) != 0 {
		t.Fatalf("gate = %v", gate.Gate)
	}

	// bob（write 协作者，非作者）approve → 合并成功（reviews 需写权限）
	alice.mustStatus("POST", "/users/alice/repos/gate/collabs",
		map[string]string{"username": "bob", "permission": "write"}, 200)
	bob.mustStatus("POST", prPath("alice", "gate", num+"/reviews"),
		map[string]string{"state": "approve"}, 201)
	alice.mustStatus("POST", prPath("alice", "gate", num+"/merge"),
		map[string]string{"method": "fast-forward"}, 200)

	// ---- 过期 review：head 前进后旧 approve 不计 ----
	gitDo(t, repo, key, "checkout", "-q", "main")
	gitDo(t, repo, key, "pull", "-q", "origin", "main")
	gitDo(t, repo, key, "checkout", "-q", "-b", "feat2")
	write("f2.txt", "f2")
	gitDo(t, repo, key, "push", "-q", "-u", "origin", "feat2")
	head1 := runCmd(t, repo, nil, "git", "rev-parse", "HEAD")
	m = alice.mustStatus("POST", prPath("alice", "gate", ""),
		map[string]string{"title": "feat2", "source_branch": "feat2", "target_branch": "main"}, 201)
	num2 := fmt.Sprintf("/%d", int(m["number"].(float64)))
	bob.mustStatus("POST", prPath("alice", "gate", num2+"/reviews"),
		map[string]string{"state": "approve", "commit_sha": strings.TrimSpace(head1)}, 201)

	// feat2 上再推一个提交（head 前进）
	write("f3.txt", "f3")
	gitDo(t, repo, key, "push", "-q", "origin", "feat2")

	if code, v := mergeResult(t, alice, "alice", "gate", num2); code != 409 || v["code"] != "review_required" {
		t.Fatalf("stale approval should not count: %d %v", code, v)
	}
	// 重新 approve（默认针对当前 head）→ 通过
	bob.mustStatus("POST", prPath("alice", "gate", num2+"/reviews"),
		map[string]string{"state": "approve"}, 201)
	alice.mustStatus("POST", prPath("alice", "gate", num2+"/merge"),
		map[string]string{"method": "fast-forward"}, 200)
}

func mergeResult(t *testing.T, c *Client, owner, repo, num string) (int, map[string]any) {
	t.Helper()
	return c.do("POST", prPath(owner, repo, num+"/merge"), map[string]string{"method": "fast-forward"})
}

func TestBranchProtectionSSHPush(t *testing.T) {
	requireBins(t, "git", "ssh", "ssh-keygen")
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "gate"}, 201)

	key, pub := genKey(t, env.DataDir, "alice_key")
	alice.mustStatus("POST", "/keys", map[string]string{"name": "e2e", "public_key": pub}, 201)

	work := t.TempDir()
	gitDo(t, work, key, "clone", "-q", fmt.Sprintf("ssh://git@127.0.0.1:%s/gate.git", env.SSHPort), "gate")
	repo := filepath.Join(work, "gate")
	commit := func(msg string) {
		if err := os.WriteFile(filepath.Join(repo, "f.txt"), []byte(msg), 0o644); err != nil {
			t.Fatal(err)
		}
		gitDo(t, repo, key, "add", "-A")
		gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "-m", msg)
	}
	commit("c1")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD") // 普通 push（未设保护前）不受影响

	alice.mustStatus("PUT", bpPath+"/main",
		map[string]any{"min_approvals": 0, "block_deletion": true, "block_force_push": true}, 200)

	// 禁删：git push origin :main
	runCmdFail(t, repo, sshEnv(key), "git", "push", "origin", ":main")

	// 禁 force push：amend 后 --force
	commit("c2")
	gitDo(t, repo, key, "push", "-q", "origin", "HEAD")
	gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "--amend", "-m", "amended")
	runCmdFail(t, repo, sshEnv(key), "git", "push", "--force", "origin", "HEAD")

	// 未保护分支不受影响
	gitDo(t, repo, key, "checkout", "-q", "-b", "free")
	commit("free1")
	gitDo(t, repo, key, "push", "-q", "-u", "origin", "free")
	gitDo(t, repo, key, "-c", "user.name=alice", "-c", "user.email=a@b.c", "commit", "-q", "--amend", "-m", "free1 amended")
	gitDo(t, repo, key, "push", "-q", "--force", "origin", "free")
	gitDo(t, repo, key, "push", "-q", "origin", ":free")
}
