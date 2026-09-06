package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

// rawDo 发送原始请求，返回状态码与响应体（供数组/二进制响应用例使用）。
func rawDo(t *testing.T, env *Env, token, method, path string, body io.Reader, contentType string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, env.BaseURL+"/api"+path, body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func mustJSON(t *testing.T, code int, b []byte, want int) map[string]any {
	t.Helper()
	if code != want {
		t.Fatalf("status = %d, want %d, body = %s", code, want, b)
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}

func uploadAssetReq(t *testing.T, env *Env, token, path, filename string, content []byte) (int, []byte) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("form file: %v", err)
	}
	if _, err := fw.Write(content); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return rawDo(t, env, token, "POST", path, &buf, mw.FormDataContentType())
}

// TestSearchAndReleases 代码搜索 + release/附件生命周期端到端。
func TestSearchAndReleases(t *testing.T) {
	requireBins(t, "git")
	env := start(t)
	alice := register(t, env, "alice", "password-alice")
	bob := register(t, env, "bob", "password-bob")

	alice.mustStatus("POST", "/repos", map[string]any{
		"name": "demo", "template": "readme", "private": false,
	}, 201)

	// 写入含目标串的文件
	alice.mustStatus("POST", "/users/alice/repos/demo/commits", map[string]any{
		"branch":  "main",
		"message": "add code",
		"changes": []map[string]string{
			{"path": "src/hello.go", "action": "create", "content": "package main\n\nfunc main() {\n\tfmt.Println(\"hello uniqueword world\")\n}\n"},
			{"path": "docs/note.md", "action": "create", "content": "some other text\n"},
		},
	}, 201)

	// 搜索命中
	code, b := rawDo(t, env, alice.token, "GET", "/users/alice/repos/demo/search?q=uniqueword", nil, "")
	var hits []map[string]any
	mustJSON(t, code, b, 200)
	if err := json.Unmarshal(b, &hits); err != nil {
		t.Fatalf("decode hits: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d: %s", len(hits), b)
	}
	if hits[0]["path"] != "src/hello.go" || hits[0]["line"] != float64(4) {
		t.Fatalf("bad hit: %v", hits[0])
	}
	if !strings.Contains(fmt.Sprint(hits[0]["text"]), "uniqueword") {
		t.Fatalf("bad text: %v", hits[0])
	}

	// 空查询 400；无命中 200 空数组；二进制跳过不报错
	code, _ = rawDo(t, env, alice.token, "GET", "/users/alice/repos/demo/search", nil, "")
	if code != 400 {
		t.Fatalf("empty q: %d", code)
	}
	code, b = rawDo(t, env, alice.token, "GET", "/users/alice/repos/demo/search?q=zzz-nomatch", nil, "")
	mustJSON(t, code, b, 200)
	if strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("want empty array, got %s", b)
	}

	// 打 tag
	alice.mustStatus("POST", "/users/alice/repos/demo/refs", map[string]any{
		"type": "tag", "name": "v1.0",
	}, 201)

	// tag 不存在 → 400 tag_not_found
	code, b = rawDo(t, env, alice.token, "POST", "/users/alice/repos/demo/releases",
		strings.NewReader(`{"tag_name":"v9.9"}`), "application/json")
	m := mustJSON(t, code, b, 400)
	if m["code"] != "tag_not_found" {
		t.Fatalf("want tag_not_found, got %v", m)
	}

	// 创建 release
	code, b = rawDo(t, env, alice.token, "POST", "/users/alice/repos/demo/releases",
		strings.NewReader(`{"tag_name":"v1.0","name":"First release","body":"# Notes\nhello"}`), "application/json")
	m = mustJSON(t, code, b, 201)
	if m["tag_name"] != "v1.0" || m["author"] != "alice" {
		t.Fatalf("bad release: %v", m)
	}

	// 重复创建 → 409
	code, _ = rawDo(t, env, alice.token, "POST", "/users/alice/repos/demo/releases",
		strings.NewReader(`{"tag_name":"v1.0"}`), "application/json")
	if code != 409 {
		t.Fatalf("dup release: %d", code)
	}

	// bob（非 owner，公开仓库）可读但不可写
	code, b = rawDo(t, env, bob.token, "GET", "/users/alice/repos/demo/releases", nil, "")
	mustJSON(t, code, b, 200)
	code, _ = rawDo(t, env, bob.token, "POST", "/users/alice/repos/demo/releases",
		strings.NewReader(`{"tag_name":"v1.0"}`), "application/json")
	if code != 404 {
		t.Fatalf("bob write release: %d, want 404", code)
	}

	// 上传附件
	code, b = uploadAssetReq(t, env, alice.token, "/users/alice/repos/demo/releases/v1.0/assets", "notes.txt", []byte("asset payload"))
	m = mustJSON(t, code, b, 201)
	if m["filename"] != "notes.txt" || m["size"] != float64(len("asset payload")) {
		t.Fatalf("bad asset: %v", m)
	}

	// 同名重复 → 409
	code, _ = uploadAssetReq(t, env, alice.token, "/users/alice/repos/demo/releases/v1.0/assets", "notes.txt", []byte("dup"))
	if code != 409 {
		t.Fatalf("dup asset: %d, want 409", code)
	}

	// 列出附件
	code, b = rawDo(t, env, alice.token, "GET", "/users/alice/repos/demo/releases/v1.0/assets", nil, "")
	var assets []map[string]any
	mustJSON(t, code, b, 200)
	if err := json.Unmarshal(b, &assets); err != nil {
		t.Fatalf("decode assets: %v", err)
	}
	if len(assets) != 1 || assets[0]["filename"] != "notes.txt" {
		t.Fatalf("bad assets: %s", b)
	}

	// bob 下载附件（公开仓库可读）
	code, b = rawDo(t, env, bob.token, "GET", "/users/alice/repos/demo/releases/v1.0/assets/notes.txt", nil, "")
	if code != 200 || string(b) != "asset payload" {
		t.Fatalf("download: %d %s", code, b)
	}

	// 未登录读私有仓库为 401（auth 中间件）；附件不存在 → 404
	code, _ = rawDo(t, env, alice.token, "GET", "/users/alice/repos/demo/releases/v1.0/assets/nope.bin", nil, "")
	if code != 404 {
		t.Fatalf("missing asset: %d", code)
	}

	// 超 10MB 附件 → 400
	code, _ = uploadAssetReq(t, env, alice.token, "/users/alice/repos/demo/releases/v1.0/assets", "big.bin", bytes.Repeat([]byte("x"), 10<<20+1))
	if code != 400 {
		t.Fatalf("oversize asset: %d, want 400", code)
	}

	// 删除附件 → 204，再删 404
	code, _ = rawDo(t, env, alice.token, "DELETE", "/users/alice/repos/demo/releases/v1.0/assets/notes.txt", nil, "")
	if code != 204 {
		t.Fatalf("delete asset: %d", code)
	}
	code, _ = rawDo(t, env, alice.token, "DELETE", "/users/alice/repos/demo/releases/v1.0/assets/notes.txt", nil, "")
	if code != 404 {
		t.Fatalf("delete asset again: %d", code)
	}

	// 删除 release → 204；列表为空；tag 仍在
	code, _ = rawDo(t, env, alice.token, "DELETE", "/users/alice/repos/demo/releases/v1.0", nil, "")
	if code != 204 {
		t.Fatalf("delete release: %d", code)
	}
	code, b = rawDo(t, env, alice.token, "GET", "/users/alice/repos/demo/releases", nil, "")
	mustJSON(t, code, b, 200)
	if strings.TrimSpace(string(b)) != "[]" {
		t.Fatalf("releases not empty: %s", b)
	}
	code, b = rawDo(t, env, alice.token, "GET", "/users/alice/repos/demo/tags", nil, "")
	mustJSON(t, code, b, 200)
	if !strings.Contains(string(b), "v1.0") {
		t.Fatalf("tag should remain: %s", b)
	}
}
