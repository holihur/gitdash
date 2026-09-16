// Package webhooks 消费 post-receive / API spool 中的事件，异步投递到配置的 webhook URL。
// 投递本身由任务队列 worker（jobs.KindWebhook）执行，spool 排空不再阻塞。
package webhooks

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	urlpkg "net/url"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/ssrf"
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

// EventTypes 出站 webhook 可订阅的事件类型（供 API / 前端展示）。
var EventTypes = []string{
	"push", "issues", "pulls", "comment", "create", "delete", "release", "pipeline", "fork", "star", "watch",
}

// Subscribes 判断某 webhook 是否订阅了给定事件类型；events 为空表示订阅全部。
func Subscribes(events []string, event string) bool {
	if len(events) == 0 {
		return true
	}
	for _, e := range events {
		if e == event {
			return true
		}
	}
	return false
}

// WriteSpool 把事件原子写入 spool 目录（临时文件 + rename）。
// 并发调用安全：文件名含 pid 与纳秒时间戳，保证唯一。
func WriteSpool(dir string, ev Event) error {
	b, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := fmt.Sprintf("%s__%s-%d-%d.json", ev.Owner, ev.Repo, os.Getpid(), time.Now().UnixNano())
	tmp, err := os.CreateTemp(dir, name+".tmp")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, name))
}

// client 使用 ssrf.DialContext：解析后直接拨已校验的 IP，消除 DNS 重绑定（TOCTOU）窗口。
var client = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		DialContext:         ssrf.DialContext,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: 10 * time.Second,
	},
}

// Enqueuer 异步任务入队接口（由 jobs.Manager 实现），避免与具体实现耦合。
type Enqueuer interface {
	EnqueueWebhook(jobs.WebhookPayload) error
	EnqueueMirror(owner, repo, url, privateKey string) error
}

// Dispatcher 消费事件 spool 并调度投递。所有依赖显式注入（无包级可变状态）。
type Dispatcher struct {
	st *store.Store
	q  Enqueuer
}

// New 创建 Dispatcher。q 为 nil 时仅记录日志、不入队。
func New(st *store.Store, q Enqueuer) *Dispatcher { return &Dispatcher{st: st, q: q} }

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
func (d *Dispatcher) Run(spoolDir string, interval time.Duration, handlers ...func(Event)) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		d.drain(spoolDir, handlers)
	}
}

func (d *Dispatcher) drain(spoolDir string, handlers []func(Event)) {
	st := d.st
	d.processRetries()
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
			// 异步投递：入队后立即返回，由队列 worker 投递（不阻塞 spool 排空与 CI 触发）
			body, merr := json.Marshal(ev)
			if merr != nil {
				logx.Infof("webhook: marshal event %s/%s: %v", ev.Owner, ev.Repo, merr)
			} else {
				for _, h := range hooks {
					if d.q == nil {
						break
					}
					if !Subscribes(h.Events, ev.Event) {
						continue // 该 webhook 未订阅此类事件
					}
					if err := d.q.EnqueueWebhook(jobs.WebhookPayload{HookID: h.ID, Body: body}); err != nil {
						logx.Infof("webhook: enqueue %s/%s hook %d: %v", ev.Owner, ev.Repo, h.ID, err)
					}
				}
			}
		}
		// push mirror 自动同步（仅 push 事件，走异步任务队列，避免无界 goroutine）
		if ev.Event == "push" && d.q != nil {
			if m, err := st.GetMirror(ev.Owner, ev.Repo); err == nil && m.URL != "" {
				if err := d.q.EnqueueMirror(ev.Owner, ev.Repo, m.URL, m.PrivateKey); err != nil {
					logx.Infof("mirror: enqueue %s/%s -> %s: %v", ev.Owner, ev.Repo, m.URL, err)
				}
			}
		}
		for _, h := range handlers {
			h(ev)
		}
		_ = os.Remove(f)
	}
}

// blockedLinkLocal 防 SSRF：默认禁止投递到回环/私有/链路本地/云元数据地址，
// 仅允许公网目标；GITDASH_SSRF_ALLOW_PRIVATE=1 可放开私有网段（自托管内网场景）。
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
		if ssrf.IsDangerous(addr) {
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

// HandleJob 队列 worker：投递一条 webhook（异步队列处理）。
func (d *Dispatcher) HandleJob(payload []byte) error {
	st := d.st
	if st == nil {
		return nil
	}
	var p jobs.WebhookPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return nil //nolint:nilerr // 非法载荷直接丢弃
	}
	hook, ok, err := st.GetWebhookByID(p.HookID)
	if err != nil {
		return err
	}
	if !ok {
		if p.DeliveryID > 0 {
			_ = st.UpdateDelivery(p.DeliveryID, "failed", 0, "webhook deleted", "")
		}
		return nil
	}
	var ev Event
	if err := json.Unmarshal(p.Body, &ev); err != nil {
		return nil //nolint:nilerr // 非法载荷直接丢弃
	}
	deliverAndRecord(st, hook, ev, p.DeliveryID, p.Attempts)
	return nil
}

// deliverAndRecord 投递一次并落记录；deliveryID>0 表示是重试（更新既有记录而非新建）。
func deliverAndRecord(st *store.Store, hook store.Webhook, ev Event, deliveryID int64, attempts int) {
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
	n := attempts
	if n < 1 {
		n = 1
	}
	next, giveUp := backoff(n)
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
func (d *Dispatcher) processRetries() {
	st := d.st
	due, err := st.DueRetries(time.Now().UTC().Format(time.RFC3339), 50)
	if err != nil {
		return
	}
	for _, dly := range due {
		if _, ok, err := st.GetWebhookByID(dly.HookID); err != nil {
			continue
		} else if !ok {
			_ = st.UpdateDelivery(dly.ID, "failed", 0, "webhook deleted", "")
			continue
		}
		payload, err := st.GetDeliveryPayload(dly.ID)
		if err != nil || payload == "" {
			_ = st.UpdateDelivery(dly.ID, "failed", 0, "payload missing", "")
			continue
		}
		// 推后 next_retry，避免 worker 处理完成前被重复取出（不递增 attempts）。
		_ = st.DeferDelivery(dly.ID, time.Now().UTC().Add(2*baseBackoff).Format(time.RFC3339))
		if d.q == nil {
			continue
		}
		if err := d.q.EnqueueWebhook(jobs.WebhookPayload{
			HookID: dly.HookID, DeliveryID: dly.ID, Attempts: dly.Attempts, Body: []byte(payload),
		}); err != nil {
			logx.Infof("webhook: enqueue retry %d: %v", dly.ID, err)
		}
	}
}

// deliver 发送一次投递，返回 HTTP 状态码与错误；SSRF/协议校验失败视为不可重试（code=0）。
func deliver(url string, body []byte, secret string) (int, error) {
	u, perr := urlpkg.Parse(url)
	if perr != nil || blockedLinkLocal(u) {
		logx.Infof("webhook: blocked delivery to %q (ssrf guard)", url)
		return 0, errors.New("blocked by ssrf guard")
	}
	if u.Scheme != "https" && !httpAllowed(u) {
		logx.Infof("webhook: rejected plaintext delivery to %q (use https or GITDASH_WEBHOOK_ALLOW_HTTP=1)", url)
		return 0, errors.New("plaintext http not allowed")
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		logx.Infof("webhook: invalid url %q: %v", url, err)
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
		logx.Infof("webhook: deliver failed: %v", err)
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}
