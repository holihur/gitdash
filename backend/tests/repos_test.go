package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func repoOnDisk(env *Env, owner, name string) string {
	return filepath.Join(env.ReposDir, owner, name+".git")
}

func repoExists(env *Env, owner, name string) bool {
	fi, err := os.Stat(repoOnDisk(env, owner, name))
	return err == nil && fi.IsDir()
}

// listRepos 请求数组形式的仓库列表。
func listRepos(t *testing.T, c *Client) []Repo {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api/repos", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list repos: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("list repos: status %d", resp.StatusCode)
	}
	var repos []Repo
	if err := json.NewDecoder(resp.Body).Decode(&repos); err != nil {
		t.Fatalf("decode repos: %v", err)
	}
	return repos
}

type Repo struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
}

func TestRepoLifecycle(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")

	// 创建
	m := alice.mustStatus("POST", "/repos", map[string]string{"name": "demo", "description": "d"}, 201)
	if m["owner"] != "alice" || m["name"] != "demo" {
		t.Fatalf("create = %v", m)
	}

	// 磁盘上的 bare 仓库
	if !repoExists(env, "alice", "demo") {
		t.Fatal("bare repo missing on disk")
	}

	// 重复创建
	alice.mustFail("POST", "/repos", map[string]string{"name": "demo"}, 409)

	// 非法名称
	for _, bad := range []string{"", "-x", ".hidden", "a/b", "has space"} {
		alice.mustFail("POST", "/repos", map[string]string{"name": bad}, 400)
	}

	// 查询
	alice.mustStatus("GET", "/repos/demo", nil, 200)

	// 删除
	alice.mustStatus("DELETE", "/repos/demo", nil, 204)
	alice.mustFail("GET", "/repos/demo", nil, 404)
	if repoExists(env, "alice", "demo") {
		t.Fatal("repo still on disk after delete")
	}
	// 重复删除：现在返回 404（此前 500）
	alice.mustFail("DELETE", "/repos/demo", nil, 404)
}

func TestRepoUserIsolation(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)

	// bob 看不到 alice 的仓库
	bob.mustFail("GET", "/repos/demo", nil, 404)
	bob.mustFail("GET", "/repos/demo/branches", nil, 404)

	// 不同用户允许同名仓库
	bob.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)
	if !repoExists(env, "bob", "demo") || !repoExists(env, "alice", "demo") {
		t.Fatal("same-name repos across users missing")
	}

	// 各自只能看到自己的
	if got := len(listRepos(t, alice)); got != 1 {
		t.Fatalf("alice repo count = %d", got)
	}
	if got := len(listRepos(t, bob)); got != 1 {
		t.Fatalf("bob repo count = %d", got)
	}
}

func TestRepoListOnlyOwn(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	alice.mustStatus("POST", "/repos", map[string]string{"name": "a1"}, 201)
	alice.mustStatus("POST", "/repos", map[string]string{"name": "a2"}, 201)
	bob.mustStatus("POST", "/repos", map[string]string{"name": "b1"}, 201)

	aRepos := listRepos(t, alice)
	if len(aRepos) != 2 {
		t.Fatalf("alice repos = %+v", aRepos)
	}
	for _, r := range aRepos {
		if r.Owner != "alice" {
			t.Fatalf("alice sees foreign repo: %+v", r)
		}
	}

	bRepos := listRepos(t, bob)
	if len(bRepos) != 1 || bRepos[0].Name != "b1" || bRepos[0].Owner != "bob" {
		t.Fatalf("bob repos = %+v", bRepos)
	}
}
