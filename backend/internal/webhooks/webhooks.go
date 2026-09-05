// Package webhooks 消费 post-receive spool 中的 push 事件并投递到配置的 URL。
package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/netip"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitdash/backend/internal/gitsvc"
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
// handlers 为额外的 push 事件消费者（如 pipeline），在删除 spool 文件前依次调用。
func Run(spoolDir string, st *store.Store, interval time.Duration, handlers ...func(Event)) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		drain(spoolDir, st, handlers)
	}
}

func drain(spoolDir string, st *store.Store, handlers []func(Event)) {
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
				post(h.URL, ev, h.Secret)
			}
		}
		// push mirror 自动同步（异步，避免阻塞 webhook 投递）
		if m, err := st.GetMirror(ev.Owner, ev.Repo); err == nil && m.URL != "" {
			go syncMirror(ev.Owner, ev.Repo, m.URL, m.PrivateKey)
		}
		for _, h := range handlers {
			h(ev)
		}
		_ = os.Remove(f)
	}
}

func syncMirror(owner, repo, url, privateKey string) {
	if err := gitsvc.PushMirror(owner, repo, url, privateKey); err != nil {
		log.Printf("mirror: sync %s/%s -> %s: %v", owner, repo, url, err)
	}
}

// blockedLinkLocal 防 SSRF：不允许投递到 link-local / 云元数据网段。
func blockedLinkLocal(u *urlpkg.URL) bool {
	host := u.Hostname()
	if host == "" {
		return false
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return true // 无法解析视为不可达，跳过避免误投递
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		if addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() {
			return true
		}
		if addr.IsPrivate() && strings.HasPrefix(addr.String(), "169.254.") {
			return true
		}
		if addr.String() == "100.100.100.200" {
			return true
		}
	}
	return false
}

// httpAllowed: 仅回环地址允许明文 http（其余要求 https，除非显式 GITDASH_WEBHOOK_ALLOW_HTTP=1）。
func httpAllowed(u *urlpkg.URL) bool {
	if os.Getenv("GITDASH_WEBHOOK_ALLOW_HTTP") != "" {
		return true
	}
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if ok && addr.Unmap().IsLoopback() {
			return true
		}
	}
	return false
}

func post(url string, ev Event, secret string) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	u, perr := urlpkg.Parse(url)
	if perr != nil || blockedLinkLocal(u) {
		log.Printf("webhook: blocked delivery to %q (ssrf guard)", url)
		return
	}
	if u.Scheme != "https" && !httpAllowed(u) {
		log.Printf("webhook: rejected plaintext delivery to %q (use https or GITDASH_WEBHOOK_ALLOW_HTTP=1)", url)
		return
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: invalid url %q: %v", url, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "gitdash-webhook")
	if secret != "" {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req.Header.Set("X-Gitdash-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("webhook: deliver %s/%s -> %s: %v", ev.Owner, ev.Repo, url, err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		log.Printf("webhook: deliver %s/%s -> %s: status %d", ev.Owner, ev.Repo, url, resp.StatusCode)
	}
}
