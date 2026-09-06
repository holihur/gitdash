// Package webhooks 消费 post-receive spool 中的 push 事件并投递到配置的 URL。
package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/netip"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/store"
)

// Event 与 post-receive hook / API 侧 publisher 写入的 JSON 行对应。
// push 事件只使用前 8 个字段；issue/pull/评论事件使用后 6 个扩展字段（向后兼容）。
type Event struct {
	Event     string `json:"event"`
	Owner     string `json:"owner"`
	Repo      string `json:"repo"`
	Old       string `json:"old"`
	New       string `json:"new"`
	Ref       string `json:"ref"`
	User      string `json:"user"`
	CreatedAt string `json:"created_at"`
	// API 侧扩展字段
	Kind    string `json:"kind,omitempty"`   // issue | pull
	Action  string `json:"action,omitempty"` // opened | closed | commented | ...
	Number  int64  `json:"number,omitempty"` // issue/PR 编号
	Title   string `json:"title,omitempty"`
	Actor   string `json:"actor,omitempty"`
	Comment string `json:"comment,omitempty"` // 评论内容摘要（截断）
}

var client = &http.Client{Timeout: 10 * time.Second}

// allowHTTP 进程启动时读取一次，避免每次投递都查环境变量。
var allowHTTP = os.Getenv("GITDASH_WEBHOOK_ALLOW_HTTP") != ""

// dnsCache 主机名 -> IP 列表（60s TTL），避免同一次投递/同一事件重复同步 DNS。
var (
	dnsMu     sync.Mutex
	dnsCache  = map[string][]net.IP{}
	dnsTime   = map[string]time.Time{}
	dnsExpire = time.Minute
)

func lookupIP(host string) ([]net.IP, error) {
	dnsMu.Lock()
	if ips, ok := dnsCache[host]; ok && time.Since(dnsTime[host]) < dnsExpire {
		dnsMu.Unlock()
		return ips, nil
	}
	dnsMu.Unlock()
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	dnsMu.Lock()
	dnsCache[host] = ips
	dnsTime[host] = time.Now()
	dnsMu.Unlock()
	return ips, nil
}

// Run 循环扫描 spool 目录并投递（main 中 go 启动）。
// 投递失败落 webhook_deliveries 记录并按退避策略自动重试（最多 5 次）。
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
	processRetries(st)
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
		if ev.Event == "" {
			ev.Event = "push"
		}
		hooks, err := st.ListWebhooks(ev.Owner, ev.Repo)
		if err == nil && len(hooks) > 0 {
			// 并行投递：单个慢端点不再阻塞整个 spool 排空与 CI 触发
			var wg sync.WaitGroup
			for _, h := range hooks {
				wg.Add(1)
				go func(h store.Webhook) {
					defer wg.Done()
					deliverAndRecord(st, h, ev, 0)
				}(h)
			}
			wg.Wait()
		}
		// push mirror 自动同步（仅 push 事件，走异步任务队列，避免无界 goroutine）
		if ev.Event == "push" {
			if m, err := st.GetMirror(ev.Owner, ev.Repo); err == nil && m.URL != "" {
				if err := jobs.EnqueueMirror(ev.Owner, ev.Repo, m.URL, m.PrivateKey); err != nil {
					log.Printf("mirror: enqueue %s/%s -> %s: %v", ev.Owner, ev.Repo, m.URL, err)
				}
			}
		}
		for _, h := range handlers {
			h(ev)
		}
		_ = os.Remove(f)
	}
}

// blockedLinkLocal 防 SSRF：不允许投递到 link-local / 云元数据网段。
func blockedLinkLocal(u *urlpkg.URL) bool {
	host := u.Hostname()
	if host == "" {
		return false
	}
	ips, err := lookupIP(host)
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
	if allowHTTP {
		return true
	}
	host := u.Hostname()
	ips, err := lookupIP(host)
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

// 投递重试策略：失败后退避重试，最多 maxAttempts 次（含首次），间隔 30s * 2^n。
const (
	maxAttempts = 5
	baseBackoff = 30 * time.Second
)

// deliverAndRecord 投递一次并落记录；deliverID>0 表示是重试（更新既有记录而非新建）。
func deliverAndRecord(st *store.Store, hook store.Webhook, ev Event, deliveryID int64) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	code, derr := deliver(hook.URL, body, hook.Secret)
	if derr == nil && code >= 200 && code < 300 {
		if deliveryID > 0 {
			_ = st.UpdateDelivery(deliveryID, "success", code, "", "")
		} else {
			_, _ = st.RecordDelivery(hook.ID, ev.Event, string(body), "success", code, "", "")
		}
		return
	}
	errMsg := "status " + strconv.Itoa(code)
	if derr != nil {
		errMsg = derr.Error()
	}
	next, giveUp := backoff(1)
	status := "retry"
	if giveUp {
		status = "failed"
		next = ""
	}
	if deliveryID > 0 {
		_ = st.UpdateDelivery(deliveryID, status, code, errMsg, next)
		return
	}
	_, _ = st.RecordDelivery(hook.ID, ev.Event, string(body), status, code, errMsg, next)
}

// backoff 根据 attempts（已完成次数）计算下次重试时间；次数达上限时放弃。
func backoff(attempts int) (string, bool) {
	nextAttempts := attempts + 1
	if nextAttempts >= maxAttempts {
		return "", true
	}
	t := time.Now().UTC().Add(baseBackoff << uint(nextAttempts-1)).Format(time.RFC3339)
	return t, false
}

// processRetries 处理到期的重试投递（每轮 drain 最多 50 条）。
func processRetries(st *store.Store) {
	due, err := st.DueRetries(time.Now().UTC().Format(time.RFC3339), 50)
	if err != nil {
		return
	}
	for _, d := range due {
		hook, ok, err := st.GetWebhookByID(d.HookID)
		if err != nil {
			continue
		}
		if !ok {
			_ = st.UpdateDelivery(d.ID, "failed", 0, "webhook deleted", "")
			continue
		}
		payload, err := st.GetDeliveryPayload(d.ID)
		if err != nil || payload == "" {
			_ = st.UpdateDelivery(d.ID, "failed", 0, "payload missing", "")
			continue
		}
		var ev Event
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			_ = st.UpdateDelivery(d.ID, "failed", 0, "payload invalid", "")
			continue
		}
		code, derr := deliver(hook.URL, []byte(payload), hook.Secret)
		if derr == nil && code >= 200 && code < 300 {
			_ = st.UpdateDelivery(d.ID, "success", code, "", "")
			continue
		}
		errMsg := "status " + strconv.Itoa(code)
		if derr != nil {
			errMsg = derr.Error()
		}
		next, giveUp := backoff(d.Attempts)
		status := "retry"
		if giveUp {
			status = "failed"
			next = ""
		}
		_ = st.UpdateDelivery(d.ID, status, code, errMsg, next)
	}
}

// deliver 发送一次投递，返回 HTTP 状态码与错误；SSRF/协议校验失败视为不可重试（code=0）。
func deliver(url string, body []byte, secret string) (int, error) {
	u, perr := urlpkg.Parse(url)
	if perr != nil || blockedLinkLocal(u) {
		log.Printf("webhook: blocked delivery to %q (ssrf guard)", url)
		return 0, errors.New("blocked by ssrf guard")
	}
	if u.Scheme != "https" && !httpAllowed(u) {
		log.Printf("webhook: rejected plaintext delivery to %q (use https or GITDASH_WEBHOOK_ALLOW_HTTP=1)", url)
		return 0, errors.New("plaintext http not allowed")
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		log.Printf("webhook: invalid url %q: %v", url, err)
		return 0, err
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
		log.Printf("webhook: deliver failed: %v", err)
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}
