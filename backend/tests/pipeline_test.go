package tests

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"gitdash/backend/internal/pipeline"
	"gitdash/backend/internal/queue"
)

const testPipelineYAML = `image: gitdash-ci-nonexistent-image-x
env:
  - CGO_ENABLED=0
steps:
  - name: build
    run: echo build
  - name: test
    run: echo test
`

type pipelineRun struct {
	ID         int64  `json:"id"`
	SHA        string `json:"sha"`
	Ref        string `json:"ref"`
	Status     string `json:"status"`
	StepsTotal int    `json:"steps_total"`
	StepsDone  int    `json:"steps_done"`
	Error      string `json:"error"`
	Log        string `json:"log"`
}

func commitFile(t *testing.T, c *Client, owner, repo, path, content string) {
	t.Helper()
	c.mustStatus("POST", "/users/"+owner+"/repos/"+repo+"/commits", map[string]any{
		"branch":  "main",
		"message": "add " + path,
		"changes": []map[string]string{{"path": path, "action": "create", "content": content}},
	}, 201)
}

func listPipelineRuns(t *testing.T, c *Client, owner, repo string) []pipelineRun {
	t.Helper()
	req, err := http.NewRequest("GET", c.env.BaseURL+"/api/users/"+owner+"/repos/"+repo+"/pipeline/runs", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != 200 {
		t.Fatalf("list runs: status %d", resp.StatusCode)
	}
	var runs []pipelineRun
	if err := json.NewDecoder(resp.Body).Decode(&runs); err != nil {
		t.Fatalf("decode runs: %v", err)
	}
	return runs
}

func getRun(t *testing.T, c *Client, owner, repo string, id int64) pipelineRun {
	t.Helper()
	m := c.mustStatus("GET", "/users/"+owner+"/repos/"+repo+"/pipeline/runs/"+strconv.FormatInt(id, 10), nil, 200)
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal run: %v", err)
	}
	var run pipelineRun
	if err := json.Unmarshal(b, &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	return run
}

// firstRunID 从 {runs:[...]} 触发响应中取出第一个运行 ID（多流水线触发可能返回多个）。
func firstRunID(t *testing.T, m map[string]any) int64 {
	t.Helper()
	runs, ok := m["runs"].([]any)
	if !ok || len(runs) == 0 {
		t.Fatalf("no runs in trigger response: %v", m)
	}
	first, _ := runs[0].(map[string]any)
	id, _ := first["id"].(float64)
	if id <= 0 {
		t.Fatalf("bad run id: %v", m)
	}
	return int64(id)
}

func waitTerminalRun(t *testing.T, c *Client, owner, repo string, id int64) pipelineRun {
	t.Helper()
	// 拉取失败/网络慢时 docker 可能要等较久才返回错误，放宽到 120s
	deadline := time.Now().Add(120 * time.Second)
	for {
		run := getRun(t, c, owner, repo, id)
		if run.Status == "success" || run.Status == "failed" {
			return run
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %d not terminal: %+v", id, run)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func TestPipelineSettingsAndRuns(t *testing.T) {
	// 缩短单步超时：本用例依赖 docker 对不存在镜像的失败，避免等完整默认 10m/网络超时
	t.Setenv("GITDASH_PIPELINE_DEFAULT_TIMEOUT", "8s")
	env := start(t)
	if err := pipeline.Init(env.DataDir); err != nil {
		t.Fatalf("pipeline init: %v", err)
	}

	alice := register(t, env, "pipea", "pipe-pass-123")
	alice.mustStatus("POST", "/repos",
		map[string]any{"name": "ci", "private": false}, 201)
	// 建立一个只有 README、不含 .gitdash.yml 的 main 分支，用于验证缺少 DSL 文件的报错
	commitFile(t, alice, "pipea", "ci", "README.md", "# ci\n")

	// 默认关闭
	m := alice.mustStatus("GET", "/users/pipea/repos/ci/pipeline", nil, 200)
	if v, _ := m["enabled"].(bool); v {
		t.Fatalf("default enabled = %v, want false", m["enabled"])
	}

	// 开启后可读
	m = alice.mustStatus("PUT", "/users/pipea/repos/ci/pipeline", map[string]any{"enabled": true}, 200)
	if v, _ := m["enabled"].(bool); !v {
		t.Fatalf("set enabled = %v", m)
	}
	m = alice.mustStatus("GET", "/users/pipea/repos/ci/pipeline", nil, 200)
	if v, _ := m["enabled"].(bool); !v {
		t.Fatalf("enabled after set = %v", m)
	}

	// 非 owner 不能改设置（404 隐藏）
	bob := register(t, env, "pipeb", "pipe-pass-456")
	bob.mustFail("PUT", "/users/pipea/repos/ci/pipeline", map[string]any{"enabled": false}, 404)
	// 但可读公开仓库的流水线状态
	bob.mustStatus("GET", "/users/pipea/repos/ci/pipeline", nil, 200)
	bob.mustStatus("GET", "/users/pipea/repos/ci/pipeline/runs", nil, 200)
	// bob 不能手动触发（无写权限）
	bob.mustFail("POST", "/users/pipea/repos/ci/pipeline/runs", map[string]string{}, 404)

	// 未定义 .gitdash.yml 时手动触发 -> 400
	m = alice.mustStatus("POST", "/users/pipea/repos/ci/pipeline/runs", map[string]string{}, 400)
	if m["code"] != "pipeline_file_missing" {
		t.Fatalf("expect pipeline_file_missing, got %v", m)
	}

	commitFile(t, alice, "pipea", "ci", ".gitdash.yml", testPipelineYAML)

	// 手动触发一次运行（docker 缺失或镜像拉取失败都会以 failed 结束）
	m = alice.mustStatus("POST", "/users/pipea/repos/ci/pipeline/runs", map[string]string{}, 201)
	run := waitTerminalRun(t, alice, "pipea", "ci", firstRunID(t, m))
	if run.Status != "failed" {
		t.Fatalf("run status = %q (want failed: docker missing or image pull failure)", run.Status)
	}
	if run.StepsTotal != 2 {
		t.Fatalf("steps_total = %d", run.StepsTotal)
	}
	if run.Log == "" {
		t.Fatalf("run log empty")
	}
	if run.SHA == "" || run.Ref != "main" {
		t.Fatalf("run meta = %+v", run)
	}

	// 运行列表包含该运行
	runs := listPipelineRuns(t, alice, "pipea", "ci")
	if len(runs) != 1 || runs[0].ID != run.ID {
		t.Fatalf("runs = %+v", runs)
	}

	// DSL 解析错误 -> 直接记为 failed 运行
	commitFile(t, alice, "pipea", "ci", ".gitdash.yml", "foo: bar\n")
	m = alice.mustStatus("POST", "/users/pipea/repos/ci/pipeline/runs", map[string]string{}, 201)
	run2 := waitTerminalRun(t, alice, "pipea", "ci", firstRunID(t, m))
	if run2.Status != "failed" {
		t.Fatalf("bad dsl run status = %q", run2.Status)
	}
	if run2.Error == "" {
		t.Fatalf("bad dsl run error empty")
	}
	if got := len(listPipelineRuns(t, alice, "pipea", "ci")); got != 2 {
		t.Fatalf("runs count = %d", got)
	}

	// 删除仓库级联清理流水线数据
	alice.mustStatus("DELETE", "/repos/ci", nil, 204)
	alice.mustFail("GET", "/users/pipea/repos/ci/pipeline", nil, 404)
}

// TestPipelineMultipleFiles 验证多流水线文件：发现、批量触发、按文件触发与运行记录带 file。
func TestPipelineMultipleFiles(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_DEFAULT_TIMEOUT", "5s")
	env := start(t)
	if err := pipeline.Init(env.DataDir); err != nil {
		t.Fatalf("pipeline init: %v", err)
	}
	alice := register(t, env, "pipem", "pipe-pass-123")
	alice.mustStatus("POST", "/repos", map[string]any{"name": "multi", "private": false}, 201)
	commitFile(t, alice, "pipem", "multi", "README.md", "# multi\n")
	alice.mustStatus("PUT", "/users/pipem/repos/multi/pipeline", map[string]any{"enabled": true}, 200)
	commitFile(t, alice, "pipem", "multi", ".gitdash/ci.yml", "steps:\n  - run: echo ci\n")
	commitFile(t, alice, "pipem", "multi", ".gitdash/deploy.yaml", "steps:\n  - run: echo deploy\n")
	commitFile(t, alice, "pipem", "multi", ".gitdash.yml", "steps:\n  - run: echo legacy\n")

	// 发现全部流水线文件
	m := alice.mustStatus("GET", "/users/pipem/repos/multi/pipeline", nil, 200)
	files, _ := m["files"].([]any)
	if len(files) != 3 {
		t.Fatalf("files = %v", m["files"])
	}

	// 手动触发（未指定 file）→ 为每个文件各建一个运行
	resp := alice.mustStatus("POST", "/users/pipem/repos/multi/pipeline/runs", map[string]string{}, 201)
	runsRaw, _ := resp["runs"].([]any)
	if len(runsRaw) != 3 {
		t.Fatalf("trigger runs = %v", resp)
	}
	seen := map[string]bool{}
	for _, r := range runsRaw {
		id := int64(r.(map[string]any)["id"].(float64))
		file, _ := r.(map[string]any)["file"].(string)
		if file == "" {
			t.Fatalf("run missing file: %v", r)
		}
		seen[file] = true
		waitTerminalRun(t, alice, "pipem", "multi", id)
	}
	for _, want := range []string{".gitdash.yml", ".gitdash/ci.yml", ".gitdash/deploy.yaml"} {
		if !seen[want] {
			t.Fatalf("missing run for %s: %v", want, seen)
		}
	}

	// 指定文件触发 → 仅一个运行
	resp = alice.mustStatus("POST", "/users/pipem/repos/multi/pipeline/runs", map[string]string{"file": ".gitdash/ci.yml"}, 201)
	runsRaw, _ = resp["runs"].([]any)
	if len(runsRaw) != 1 {
		t.Fatalf("single-file trigger = %v", resp)
	}
	if f, _ := runsRaw[0].(map[string]any)["file"].(string); f != ".gitdash/ci.yml" {
		t.Fatalf("single-file run file = %q", f)
	}
	waitTerminalRun(t, alice, "pipem", "multi", int64(runsRaw[0].(map[string]any)["id"].(float64)))

	// 运行列表带 file，总计 4 条
	all := listPipelineRuns(t, alice, "pipem", "multi")
	if len(all) != 4 {
		t.Fatalf("runs count = %d, want 4", len(all))
	}

	// 可视化图支持指定 file
	g := alice.mustStatus("GET", "/users/pipem/repos/multi/pipeline/graph?file=.gitdash/deploy.yaml", nil, 200)
	if g["file"] != ".gitdash/deploy.yaml" {
		t.Fatalf("graph file = %v", g["file"])
	}
}

// TestPipelineDelayCancelRerun 验证延迟执行、取消与重跑：
// 延迟触发的运行停在 pending（带 run_at），可取消；取消后可重跑。
func TestPipelineDelayCancelRerun(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_DEFAULT_TIMEOUT", "5s")
	env := start(t)
	if err := pipeline.Init(env.DataDir); err != nil {
		t.Fatalf("pipeline init: %v", err)
	}
	alice := register(t, env, "piped", "pipe-pass-123")
	alice.mustStatus("POST", "/repos", map[string]any{"name": "delay", "private": false}, 201)
	alice.mustStatus("PUT", "/users/piped/repos/delay/pipeline", map[string]any{"enabled": true}, 200)
	commitFile(t, alice, "piped", "delay", ".gitdash.yml", "steps:\n  - run: echo x\n")

	// 非法 delay → 400
	m := alice.mustStatus("POST", "/users/piped/repos/delay/pipeline/runs", map[string]string{"delay": "nope"}, 400)
	if m["code"] != "invalid_delay" {
		t.Fatalf("expect invalid_delay, got %v", m)
	}

	// 延迟 1h：运行停在 pending 且带 run_at
	m = alice.mustStatus("POST", "/users/piped/repos/delay/pipeline/runs", map[string]string{"delay": "1h"}, 201)
	runsRaw, _ := m["runs"].([]any)
	if len(runsRaw) != 1 {
		t.Fatalf("trigger runs = %v", m)
	}
	first := runsRaw[0].(map[string]any)
	id := int64(first["id"].(float64))
	if first["status"] != "pending" || first["run_at"] == nil || first["run_at"] == "" {
		t.Fatalf("delayed run = %v", first)
	}
	// 确认未执行
	if got := getRun(t, alice, "piped", "delay", id); got.Status != "pending" {
		t.Fatalf("delayed run status = %q, want pending", got.Status)
	}

	// 取消 pending 运行
	m = alice.mustStatus("POST", "/users/piped/repos/delay/pipeline/runs/"+strconv.FormatInt(id, 10)+"/cancel", map[string]string{}, 200)
	if m["cancelled"] != true {
		t.Fatalf("cancel resp = %v", m)
	}
	if got := getRun(t, alice, "piped", "delay", id); got.Status != "cancelled" {
		t.Fatalf("after cancel = %q, want cancelled", got.Status)
	}

	// 重跑已取消的运行 → 新运行（立即执行，host 未开启→failed）
	m = alice.mustStatus("POST", "/users/piped/repos/delay/pipeline/runs/"+strconv.FormatInt(id, 10)+"/rerun", map[string]string{}, 201)
	newID := int64(m["id"].(float64))
	if newID == id {
		t.Fatalf("rerun should create a new run: %v", m)
	}
	if got := waitTerminalRun(t, alice, "piped", "delay", newID); got.Status != "failed" {
		t.Fatalf("rerun status = %q, want failed (host disabled)", got.Status)
	}
}

func TestPipelineDeleteLogsCleanup(t *testing.T) {
	env := start(t)
	if err := pipeline.Init(env.DataDir); err != nil {
		t.Fatalf("pipeline init: %v", err)
	}
	alice := register(t, env, "pipec", "pipe-pass-789")
	alice.mustStatus("POST", "/repos", map[string]string{"name": "logs", "template": "readme"}, 201)
	if err := pipeline.DeleteLogs("pipec", "logs"); err != nil {
		t.Fatalf("delete logs: %v", err)
	}
	if _, err := os.Stat(pipeline.LogPath("pipec", "logs", 1)); !os.IsNotExist(err) {
		t.Fatalf("log should not exist")
	}
}

// startRedis 启动临时 redis-server，返回地址与清理函数（不可用时 skip）。
func startRedis(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	cmd := exec.Command("redis-server", "--port", strconv.Itoa(port), "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		t.Skipf("redis-server: %v", err)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, derr := net.DialTimeout("tcp", addr, time.Second)
		if derr == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Skipf("redis not reachable: %v", derr)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return addr, func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }
}

// TestPipelineAsynqQueue 走 redis(asynq) 队列的完整链路：
// 手动触发 -> 入队 -> asynq 工人执行 -> docker 失败 -> failed 终态。
func TestPipelineAsynqQueue(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_DEFAULT_TIMEOUT", "8s")
	redisAddr, stopRedis := startRedis(t)
	defer stopRedis()

	env := start(t)
	if err := pipeline.Init(env.DataDir); err != nil {
		t.Fatalf("pipeline init: %v", err)
	}
	q := queue.NewAsynq(redisAddr, "", 0, 2)
	pipeline.Bind(env.Store, q, nil)
	defer pipeline.Bind(nil, nil, nil) // 还原默认（进程内调度），避免影响其他用例

	alice := register(t, env, "pipeq", "pipe-pass-123")
	alice.mustStatus("POST", "/repos",
		map[string]any{"name": "ciq", "template": "readme", "private": false}, 201)
	alice.mustStatus("PUT", "/users/pipeq/repos/ciq/pipeline", map[string]any{"enabled": true}, 200)
	commitFile(t, alice, "pipeq", "ciq", ".gitdash.yml", testPipelineYAML)

	// 手动触发：任务应经 redis 队列被 asynq 工人取走执行
	m := alice.mustStatus("POST", "/users/pipeq/repos/ciq/pipeline/runs", map[string]string{}, 201)
	run := waitTerminalRun(t, alice, "pipeq", "ciq", firstRunID(t, m))
	if run.Status != "failed" {
		t.Fatalf("asynq run status = %q (want failed: docker missing or image pull failure)", run.Status)
	}
	if run.StepsTotal != 2 || run.Log == "" {
		t.Fatalf("asynq run = %+v", run)
	}

	// 同一 run 的幂等：直接再次入队同 ID 不会产生第二个执行（此处仅验证 API 正常）
	runs := listPipelineRuns(t, alice, "pipeq", "ciq")
	if len(runs) != 1 {
		t.Fatalf("runs count = %d, want 1", len(runs))
	}
}
