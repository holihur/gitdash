// Package jobs 仓库导入 / push mirror 同步 / webhook 投递的异步任务。
// 复用 queue 抽象（memory 或 asynq/redis）；重启后由 RequeuePending 把
// 卡在 queued/running 的任务重新入队，保证 memory 模式下也不丢任务。
//
// 所有可变状态都封装在 Manager 中，由 main（组合根）显式注入依赖。
package jobs

import (
	"context"
	"encoding/json"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/store"
)

// 任务类型标识。
const (
	KindImport  = "gitdash:import"  // 从远程 URL 镜像导入
	KindMirror  = "gitdash:mirror"  // push 到镜像目标
	KindWebhook = "gitdash:webhook" // webhook 投递（异步队列处理）
)

// 任务状态（存 DB，GET repo / mirror 可见）。
const (
	StatusQueued  = "queued"
	StatusRunning = "running"
	StatusSynced  = "synced"
	StatusFailed  = "failed"
)

// payload 任务内容（JSON 编码后作为 queue.Job.Payload）。
type payload struct {
	Owner      string `json:"owner"`
	Repo       string `json:"repo"`
	URL        string `json:"url"`
	PrivateKey string `json:"private_key,omitempty"`
}

// WebhookPayload webhook 投递任务载荷。
type WebhookPayload struct {
	HookID     int64  `json:"hook_id"`
	DeliveryID int64  `json:"delivery_id,omitempty"` // >0 表示重试
	Attempts   int    `json:"attempts,omitempty"`    // 已尝试次数（重试时）
	Body       []byte `json:"body"`                  // 事件 JSON
}

// Manager 异步任务管理器；依赖均显式注入。
type Manager struct {
	st             *store.Store
	q              queue.Queue
	webhookHandler func(payload []byte) error
}

// New 创建 Manager。q 为 nil 时所有入队返回 queue.ErrQueueFull。
func New(st *store.Store, q queue.Queue) *Manager {
	return &Manager{st: st, q: q}
}

// SetWebhookHandler 注入 webhook 投递处理器（由 main 绑定，避免 jobs ↔ webhooks 循环依赖）。
func (m *Manager) SetWebhookHandler(h func(payload []byte) error) { m.webhookHandler = h }

// Start 注册队列消费者。必须在 main 启动时调用一次。
func (m *Manager) Start() {
	if m.q == nil {
		return
	}
	m.q.Start(context.Background(), []queue.JobKind{KindImport, KindMirror, KindWebhook}, m.handle)
}

// RequeuePending 启动时把残留的 queued/running 任务重新入队（memory 模式重启续跑）。
func (m *Manager) RequeuePending() {
	if m.q == nil || m.st == nil {
		return
	}
	ctx := context.Background()
	if rows, err := m.st.PendingImports(); err == nil {
		for _, r := range rows {
			if err := m.enqueue(ctx, KindImport, r.Owner, r.Repo, r.SourceURL, ""); err != nil {
				logx.Infof("jobs: requeue import %s/%s: %v", r.Owner, r.Repo, err)
			}
		}
	}
	if rows, err := m.st.PendingMirrors(); err == nil {
		for _, r := range rows {
			if err := m.enqueue(ctx, KindMirror, r.Owner, r.Repo, r.URL, r.PrivateKey); err != nil {
				logx.Infof("jobs: requeue mirror %s/%s: %v", r.Owner, r.Repo, err)
			}
		}
	}
}

// EnqueueImport 排队一次仓库导入。
func (m *Manager) EnqueueImport(owner, repo, url, privateKey string) error {
	return m.enqueue(context.Background(), KindImport, owner, repo, url, privateKey)
}

// EnqueueMirror 排队一次镜像推送。
func (m *Manager) EnqueueMirror(owner, repo, url, privateKey string) error {
	return m.enqueue(context.Background(), KindMirror, owner, repo, url, privateKey)
}

func (m *Manager) enqueue(ctx context.Context, kind, owner, repo, url, privateKey string) error {
	if m.q == nil {
		return queue.ErrQueueFull
	}
	p, err := json.Marshal(payload{Owner: owner, Repo: repo, URL: url, PrivateKey: privateKey})
	if err != nil {
		return err
	}
	// ID 去重：同一仓库同类任务排队期间只保留一条
	return m.q.Enqueue(ctx, queue.Job{
		Kind: kind, ID: kind + ":" + owner + "/" + repo, Payload: p,
	})
}

// EnqueueWebhook 排队一次 webhook 投递（异步队列处理，不阻塞事件 spool 排空）。
func (m *Manager) EnqueueWebhook(p WebhookPayload) error {
	if m.q == nil {
		return queue.ErrQueueFull
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	return m.q.Enqueue(context.Background(), queue.Job{Kind: KindWebhook, Payload: b})
}

// handle 执行任务并落状态；仓库行已删除（排队期间被删）则直接丢弃。
func (m *Manager) handle(_ context.Context, j queue.Job) error {
	// webhook 投递与仓库无关的独立载荷，先分发处理。
	if j.Kind == KindWebhook {
		if m.webhookHandler == nil {
			return nil
		}
		return m.webhookHandler(j.Payload)
	}
	var p payload
	uerr := json.Unmarshal(j.Payload, &p)
	if uerr != nil || m.st == nil {
		return nil //nolint:nilerr // 非法载荷直接丢弃，避免无限重试
	}
	if _, gerr := m.st.GetRepo(p.Owner, p.Repo); gerr != nil {
		return nil //nolint:nilerr // 仓库已删除（排队期间被删），任务丢弃
	}
	switch j.Kind {
	case KindImport:
		_ = m.st.SetImportStatus(p.Owner, p.Repo, StatusRunning, "")
		if err := gitsvc.ImportRepo(p.URL, p.Owner, p.Repo, p.PrivateKey); err != nil {
			logx.Infof("jobs: import %s/%s: %v", p.Owner, p.Repo, err)
			_ = m.st.SetImportStatus(p.Owner, p.Repo, StatusFailed, err.Error())
			return nil
		}
		_ = m.st.SetImportStatus(p.Owner, p.Repo, StatusSynced, "")
	case KindMirror:
		_ = m.st.SetMirrorStatus(p.Owner, p.Repo, StatusRunning, "")
		if err := gitsvc.PushMirror(p.Owner, p.Repo, p.URL, p.PrivateKey); err != nil {
			logx.Infof("jobs: mirror %s/%s -> %s: %v", p.Owner, p.Repo, logx.RedactURL(p.URL), err)
			_ = m.st.SetMirrorStatus(p.Owner, p.Repo, StatusFailed, err.Error())
			return nil
		}
		_ = m.st.SetMirrorStatus(p.Owner, p.Repo, StatusSynced, "")
	}
	return nil
}
