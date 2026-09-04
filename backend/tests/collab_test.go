package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// collabPath 构造 owner 限定版仓库路径。
func collabPath(owner, repo, suffix string) string {
	return fmt.Sprintf("/users/%s/repos/%s%s", owner, repo, suffix)
}

type Collab struct {
	Username   string `json:"username"`
	Permission string `json:"permission"`
}

func listCollabs(t *testing.T, c *Client, owner, repo string) []Collab {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api"+collabPath(owner, repo, "/collabs"), nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list collabs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("list collabs: status %d", resp.StatusCode)
	}
	var out []Collab
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return out
}

func TestCollabLifecycle(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "team"}, 201)

	// owner 添加 bob 为 write 协作者
	m := alice.mustStatus("POST", collabPath("alice", "team", "/collabs"),
		map[string]string{"username": "bob", "permission": "write"}, 200)
	if m["username"] != "bob" || m["permission"] != "write" {
		t.Fatalf("add collab = %v", m)
	}
	collabs := listCollabs(t, alice, "alice", "team")
	if len(collabs) != 1 || collabs[0].Username != "bob" || collabs[0].Permission != "write" {
		t.Fatalf("collabs = %+v", collabs)
	}

	// bob 现在可以读取仓库与写 issues（未限定 owner 的旧式路由也能解析到共享仓库）
	bob.mustStatus("GET", collabPath("alice", "team", ""), nil, 200)
	bob.mustStatus("GET", collabPath("alice", "team", "/issues"), nil, 200)
	bob.mustStatus("POST", collabPath("alice", "team", "/issues"),
		map[string]string{"title": "from bob"}, 201)
	bob.mustStatus("GET", "/repos/team/branches", nil, 200) // 旧式解析

	// bob 不能删除 alice 的仓库、不能管理协作者
	bob.mustFail("DELETE", collabPath("alice", "team", ""), nil, 404)
	bob.mustFail("GET", collabPath("alice", "team", "/collabs"), nil, 404)
	bob.mustFail("POST", collabPath("alice", "team", "/collabs"),
		map[string]string{"username": "carol", "permission": "read"}, 404)

	// alice 仓库出现在 bob 的可访问列表中（role=write）
	if got := listRepos(t, bob); len(got) != 1 || got[0].Name != "team" {
		t.Fatalf("bob accessible repos = %+v", got)
	}

	// 降级为 read
	alice.mustStatus("POST", collabPath("alice", "team", "/collabs"),
		map[string]string{"username": "bob", "permission": "read"}, 200)
	bob.mustStatus("GET", collabPath("alice", "team", "/issues"), nil, 200)
	bob.mustFail("POST", collabPath("alice", "team", "/issues"),
		map[string]string{"title": "no write"}, 404)

	// 移除协作者后彻底失去访问
	alice.mustStatus("DELETE", collabPath("alice", "team", "/collabs/bob"), nil, 204)
	bob.mustFail("GET", collabPath("alice", "team", ""), nil, 404)
	bob.mustFail("GET", "/repos/team/branches", nil, 404)
}

func TestCollabBadInputs(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "team"}, 201)

	// 非法 permission
	alice.mustFail("POST", collabPath("alice", "team", "/collabs"),
		map[string]string{"username": "bob", "permission": "admin"}, 400)
	// 不存在的用户
	alice.mustFail("POST", collabPath("alice", "team", "/collabs"),
		map[string]string{"username": "ghost", "permission": "read"}, 404)
	// 自己不能当协作者
	alice.mustFail("POST", collabPath("alice", "team", "/collabs"),
		map[string]string{"username": "alice", "permission": "read"}, 400)
	// 移除非协作者 / 非法用户名
	alice.mustFail("DELETE", collabPath("alice", "team", "/collabs/carol"), nil, 404)
	// 仓库不存在
	alice.mustFail("GET", collabPath("alice", "nope", "/collabs"), nil, 404)
	// 非 owner 管理协作者
	bob := register(t, env, "bob", "bob-pass-123456")
	bob.mustFail("GET", collabPath("alice", "team", "/collabs"), nil, 404)
}

func TestCollabRepoDeleteCascades(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "team"}, 201)
	alice.mustStatus("POST", collabPath("alice", "team", "/collabs"),
		map[string]string{"username": "bob", "permission": "write"}, 200)

	alice.mustStatus("DELETE", "/repos/team", nil, 204)
	// 删除仓库后 bob 列表不再包含它
	if got := listRepos(t, bob); len(got) != 0 {
		t.Fatalf("bob repos after delete = %+v", got)
	}
	bob.mustFail("GET", collabPath("alice", "team", ""), nil, 404)
}

func TestCollabSameNameRepos(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	bob := register(t, env, "bob", "bob-pass-123456")

	// bob 自己有一个同名仓库，同时是 alice 同名仓库的协作者
	bob.mustStatus("POST", "/repos", map[string]string{"name": "app"}, 201)
	alice.mustStatus("POST", "/repos", map[string]string{"name": "app"}, 201)
	alice.mustStatus("POST", collabPath("alice", "app", "/collabs"),
		map[string]string{"username": "bob", "permission": "write"}, 200)

	// 未限定 owner 时解析到自己的仓库
	bob.mustStatus("GET", "/repos/app/issues", nil, 200)
	bob.mustStatus("POST", "/repos/app/issues", map[string]string{"title": "mine"}, 201)
	// owner 限定版访问 alice 的仓库
	bob.mustStatus("GET", collabPath("alice", "app", "/issues"), nil, 200)
	bob.mustStatus("POST", collabPath("alice", "app", "/issues"),
		map[string]string{"title": "shared"}, 201)
}
