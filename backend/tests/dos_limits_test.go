package tests

import (
	"strconv"
	"strings"
	"testing"
)

// 一批针对 UGC 字段/数量上限的回归测试，防止超大或超量的用户输入引发 DoS。

func TestUGCFieldLengthLimits(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)

	long := strings.Repeat("a", 201)

	// issue 标题 200 上限
	alice.mustFail("POST", "/repos/demo/issues", map[string]string{"title": long}, 400)

	// PR 标题/正文上限（在分支校验前触发）
	alice.mustFail("POST", "/users/alice/repos/demo/pulls", map[string]string{
		"title": long, "source_branch": "x", "target_branch": "y",
	}, 400)
	alice.mustFail("POST", "/users/alice/repos/demo/pulls", map[string]string{
		"title": "ok", "body": strings.Repeat("b", 10001),
		"source_branch": "x", "target_branch": "y",
	}, 400)

	// milestone 标题上限
	alice.mustFail("POST", "/users/alice/repos/demo/milestones",
		map[string]string{"title": long}, 400)

	// 标签更新时的名称上限
	lbl := alice.mustStatus("POST", "/users/alice/repos/demo/labels",
		map[string]string{"name": "needs-review"}, 201)
	id := int64(lbl["id"].(float64))
	alice.mustFail("PATCH", "/users/alice/repos/demo/labels/"+strconv.FormatInt(id, 10),
		map[string]string{"name": strings.Repeat("x", 51)}, 400)

	// 看板名称上限
	alice.mustFail("POST", "/users/alice/repos/demo/projects",
		map[string]string{"name": long}, 400)

	// GPG armor 上限
	alice.mustFail("POST", "/gpg", map[string]string{"armor": strings.Repeat("A", 70000)}, 400)

	// 搜索关键词上限
	alice.mustFail("GET", "/search?q="+long, nil, 400)
	alice.mustFail("GET", "/search/code?q="+long, nil, 400)
}

func TestProjectCountLimit(t *testing.T) {
	env := start(t)
	alice := register(t, env, "alice", "alice-pass-123")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "demo"}, 201)

	for i := 0; i < maxTestProjects; i++ {
		alice.mustStatus("POST", "/users/alice/repos/demo/projects",
			map[string]string{"name": "p" + strconv.Itoa(i)}, 201)
	}
	alice.mustFail("POST", "/users/alice/repos/demo/projects",
		map[string]string{"name": "one-too-many"}, 400)
}

// maxTestProjects 与 api 包内 maxProjectsPerRepo 保持一致。
const maxTestProjects = 20
