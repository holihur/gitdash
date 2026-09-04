// Package webhooks 消费 post-receive spool 中的 push 事件并投递到配置的 URL。
package webhooks

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gitdash/backend/internal/store"
)

// Event 与 post-receive hook 写入的 JSON 行对应。
type Event struct {
	Event     string `json:"event"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Old       string `json:"old"`
	New       string `json:"new"`
	Ref       string `json:"ref"`
	User      string `json:"user"`
	CreatedAt string `json:"created_at"`
}

var client = &http.Client{Timeout: 10 * time.Second}

// Run 循环扫描 spool 目录并投递（main 中 go 启动）。失败仅记日志并移除文件，避免死循环。
func Run(spoolDir string, st *store.Store, interval time.Duration) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		drain(spoolDir, st)
	}
}

func drain(spoolDir string, st *store.Store) {
	files, err := filepath.Glob(filepath.Join(spoolDir, "*.json"))
	if err != nil {
		return
	}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var ev Event
		if err := json.Unmarshal(b, &ev); err != nil || ev.Owner == "" || ev.Repo == "" {
			_ = os.Remove(f)
			continue
		}
		ev.Event = "push"
		hooks, err := st.ListWebhooks(ev.Owner, ev.Repo)
		if err == nil {
			for _, h := range hooks {
				post(h.URL, ev)
			}
		}
		_ = os.Remove(f)
	}
}

func post(url string, ev Event) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: invalid url %q: %v", url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gitdash-webhook")
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook: deliver %s/%s -> %s: %v", ev.Owner, ev.Repo, url, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		log.Printf("webhook: deliver %s/%s -> %s: status %d", ev.Owner, ev.Repo, url, resp.StatusCode)
	}
}
