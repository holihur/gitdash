package tests

import (
	"strings"
	"testing"
	"time"

	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/queue"
)

// waitForLanguages 轮询仓库语言构成，直到出现或超时。
func waitForLanguages(t *testing.T, c *Client, name string, want bool) map[string]any {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var body map[string]any
	for time.Now().Before(deadline) {
		body = c.mustStatus("GET", "/repos/"+name, nil, 200)
		langs, _ := body["languages"].([]any)
		if want && len(langs) > 0 {
			return body
		}
		if !want && len(langs) == 0 {
			// 关闭路径：多观察一小段时间，确认不是慢半拍。
			time.Sleep(400 * time.Millisecond)
			body = c.mustStatus("GET", "/repos/"+name, nil, 200)
			langs, _ = body["languages"].([]any)
			if len(langs) == 0 {
				return body
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return body
}

func TestLanguagesAnalyzedAfterWebCommit(t *testing.T) {
	env := start(t)
	mgr := jobs.New(env.Store, queue.NewMemory(64, 2))
	mgr.Start()
	env.API.SetJobsManager(mgr)

	alice := register(t, env, "langalice", "lang-pass-123")
	alice.mustStatus("POST", "/repos", map[string]any{"name": "lang"}, 201)
	alice.mustStatus("POST", "/users/langalice/repos/lang/commits", map[string]any{
		"message": "add sources",
		"changes": []map[string]any{
			{"path": "main.go", "action": "create", "content": "package main\n\nfunc main() { println(\"hi\") }\n"},
			{"path": "app.cpp", "action": "create", "content": "int main() { return 0; }\n"},
			{"path": "README.md", "action": "create", "content": "# docs\n"},
		},
	}, 201)

	body := waitForLanguages(t, alice, "lang", true)
	langs, _ := body["languages"].([]any)
	if len(langs) == 0 {
		t.Fatalf("languages not analyzed: %v", body)
	}
	got := map[string]bool{}
	for _, l := range langs {
		m, _ := l.(map[string]any)
		if name, _ := m["language"].(string); name != "" {
			got[name] = true
		}
	}
	if !got["Go"] || !got["C++"] {
		t.Fatalf("languages = %v, want Go and C++", langs)
	}
	if got["Markdown"] {
		t.Errorf("docs should not be counted: %v", langs)
	}

	// 列表页：主要语言已填充
	list := rawGet(t, alice, "/repos")
	if !strings.Contains(list, `"language":"Go"`) {
		t.Fatalf("list repos missing primary language: %s", list)
	}

	// 管理端开关：关闭后新仓库不再分析
	if err := env.Store.SetSetting(jobs.SettingLanguageStats, "0"); err != nil {
		t.Fatal(err)
	}
	alice.mustStatus("POST", "/repos", map[string]any{"name": "off"}, 201)
	alice.mustStatus("POST", "/users/langalice/repos/off/commits", map[string]any{
		"message": "add go",
		"changes": []map[string]any{
			{"path": "x.go", "action": "create", "content": "package x\n"},
		},
	}, 201)
	off := waitForLanguages(t, alice, "off", false)
	if langs, _ := off["languages"].([]any); len(langs) != 0 {
		t.Fatalf("disabled setting still analyzed: %v", off)
	}
}
