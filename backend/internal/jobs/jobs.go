// Package jobs 仓库导入 / push mirror 同步 / webhook 投递的异步任务。
// 复用 queue 抽象（memory 或 asynq/redis）；重启后由 RequeuePending 把
// 卡在 queued/running 的任务重新入队，保证 memory 模式下也不丢任务。
//
// 所有可变状态都封装在 Manager 中，由 main（组合根）显式注入依赖。
package jobs

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"time"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/store"
)

// 任务类型标识。
const (
	KindImport    = "gitdash:import"    // 从远程 URL 镜像导入
	KindMirror    = "gitdash:mirror"    // push 到镜像目标
	KindWebhook   = "gitdash:webhook"   // webhook 投递（异步队列处理）
	KindLanguages = "gitdash:languages" // 代码成分（语言）分析
	KindCodeIndex = "gitdash:codeindex" // 代码搜索索引（Bleve）异步重建
)

// SettingLanguageStats 管理端开关；默认开启（仅显式设为 "0" 时关闭）。
const SettingLanguageStats = "language_stats_enabled"

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
	Credential string `json:"credential,omitempty"`
	// Ref 语言分析目标（分支/标签/commit）；其它任务为空。
	Ref string `json:"ref,omitempty"`
	// RepoID 代码索引任务使用：仓库在数据库中的稳定 ID（识别同名重建）。
	RepoID int64 `json:"repo_id,omitempty"`
}

// WebhookPayload webhook 投递任务载荷。
type WebhookPayload struct {
	HookID     int64  `json:"hook_id"`
	DeliveryID int64  `json:"delivery_id,omitempty"` // >0 表示重试
	Attempts   int    `json:"attempts,omitempty"`    // 已尝试次数（重试时）
	Body       []byte `json:"body"`                  // 事件 JSON
}

// CodeIndexer 代码索引维护接口（由 codesearch.Service 实现，main 注入）。
// 用接口而非具体类型，避免 jobs 直接依赖索引实现。
type CodeIndexer interface {
	// Index 为仓库默认分支重建索引（ref 为空时按 HEAD 解析）。
	Index(ctx context.Context, owner, name string, repoID int64, ref string) error
	// Forget 移除仓库索引（仓库删除时调用）。
	Forget(owner, name string)
	// NeedsIndex 报告索引是否缺失、属于旧仓库或落后。
	NeedsIndex(owner, name string, repoID int64, ref string) bool
}

// Manager 异步任务管理器；依赖均显式注入。
type Manager struct {
	st             *store.Store
	q              queue.Queue
	webhookHandler func(payload []byte) error
	codeIndexer    CodeIndexer
	// codeIndexEnabled 表示本节点负责生产代码索引任务（本地索引或远程索引服务）。
	codeIndexEnabled bool
	// consumeKinds 限制消费者注册的任务类型；为空时消费全部类型。
	consumeKinds []queue.JobKind
	// backfilling 保证同一时间只有一个语言回填任务在跑。
	backfilling atomic.Bool
	// indexBackfilling 保证同一时间只有一个代码索引回填任务在跑。
	indexBackfilling atomic.Bool
}

// New 创建 Manager。q 为 nil 时所有入队返回 queue.ErrQueueFull。
func New(st *store.Store, q queue.Queue) *Manager {
	return &Manager{st: st, q: q}
}

// DefaultKinds 返回 jobs 支持的全部任务类型。
func DefaultKinds() []queue.JobKind {
	return []queue.JobKind{KindImport, KindMirror, KindWebhook, KindLanguages, KindCodeIndex}
}

// SetWebhookHandler 注入 webhook 投递处理器（由 main 绑定，避免 jobs ↔ webhooks 循环依赖）。
func (m *Manager) SetWebhookHandler(h func(payload []byte) error) { m.webhookHandler = h }

// SetCodeIndexer 注入本地代码索引维护器（仅当启用嵌入式索引时注入），
// 同时启用代码索引任务的生产与消费。
func (m *Manager) SetCodeIndexer(ci CodeIndexer) {
	m.codeIndexer = ci
	if ci != nil {
		m.codeIndexEnabled = true
	}
}

// EnableCodeIndex 仅启用代码索引任务的生产（不注入本地索引器），用于 API 节点
// 把索引任务交给独立的索引服务（GITDASH_CODE_SEARCH=remote）消费。
func (m *Manager) EnableCodeIndex(enabled bool) { m.codeIndexEnabled = enabled }

// SetConsumeKinds 限制本节点消费者注册的任务类型（拆服务时只消费某个队列）。
// 不调用时消费 DefaultKinds()。
func (m *Manager) SetConsumeKinds(kinds ...queue.JobKind) { m.consumeKinds = kinds }

// Start 注册队列消费者。必须在 main 启动时调用一次。
func (m *Manager) Start() { m.StartContext(context.Background()) }

// StartContext 同 Start，但消费者随 ctx 取消而停机（供独立 worker 优雅退出）。
func (m *Manager) StartContext(ctx context.Context) {
	if m.q == nil {
		return
	}
	kinds := m.consumeKinds
	if len(kinds) == 0 {
		kinds = DefaultKinds()
	}
	m.q.Start(ctx, kinds, m.handle)
}

// RequeuePending 启动时把残留的 queued/running 任务重新入队（memory 模式重启续跑）。
func (m *Manager) RequeuePending() {
	if m.q == nil || m.st == nil {
		return
	}
	ctx := context.Background()
	if rows, err := m.st.PendingImports(); err == nil {
		for _, r := range rows {
			if err := m.enqueue(ctx, KindImport, r.Owner, r.Repo, r.SourceURL, "", r.Credential); err != nil {
				logx.Infof("jobs: requeue import %s/%s: %v", r.Owner, r.Repo, err)
			}
		}
	}
	if rows, err := m.st.PendingMirrors(); err == nil {
		for _, r := range rows {
			if err := m.enqueue(ctx, KindMirror, r.Owner, r.Repo, r.URL, r.PrivateKey, ""); err != nil {
				logx.Infof("jobs: requeue mirror %s/%s: %v", r.Owner, r.Repo, err)
			}
		}
	}
}

// languageEnabled 报告代码成分分析是否开启（默认开启）。
func (m *Manager) languageEnabled() bool {
	return m.st != nil && m.st.GetSetting(SettingLanguageStats) != "0"
}

// EnqueueLanguages 排队一次代码成分分析。功能未开启时静默跳过。
func (m *Manager) EnqueueLanguages(owner, repo, ref string) error {
	if !m.languageEnabled() {
		return nil
	}
	if m.q == nil {
		return queue.ErrQueueFull
	}
	p, err := json.Marshal(payload{Owner: owner, Repo: repo, Ref: ref})
	if err != nil {
		return err
	}
	return m.q.Enqueue(context.Background(), queue.Job{
		Kind: KindLanguages, ID: KindLanguages + ":" + owner + "/" + repo, Payload: p,
	})
}

// BackfillLanguages 为尚无语言记录的仓库排队分析（启动/开启功能时调用）。
// 功能关闭时直接返回。按 id 游标分批，保证每个仓库只入队一次；队列满时等待重试。
func (m *Manager) BackfillLanguages() {
	if !m.languageEnabled() {
		return
	}
	if !m.backfilling.CompareAndSwap(false, true) {
		return // 已有回填在跑
	}
	go func() {
		defer m.backfilling.Store(false)
		var cursor int64
		for {
			rows, err := m.st.ReposMissingLanguages(cursor, 50)
			if err != nil || len(rows) == 0 {
				return
			}
			for _, r := range rows {
				for {
					if err := m.EnqueueLanguages(r.Owner, r.Repo, r.DefaultBranch); err == nil {
						break
					}
					time.Sleep(time.Second) // 队列满：等待消费者排空后重试
				}
				cursor = r.ID
			}
			time.Sleep(500 * time.Millisecond) // 限速，避免一次性压满队列
		}
	}()
}

// EnqueueCodeIndex 排队一次代码索引重建。未启用索引时静默跳过。
// 仓库 ID 从数据库读取并写入任务，索引侧据此识别“删除后同名重建”。
func (m *Manager) EnqueueCodeIndex(owner, repo, ref string) error {
	if !m.codeIndexEnabled || m.q == nil || m.st == nil {
		return nil
	}
	rp, err := m.st.GetRepo(owner, repo)
	if err != nil {
		return nil //nolint:nilerr // 仓库已不存在，丢弃任务
	}
	p, err := json.Marshal(payload{Owner: owner, Repo: repo, Ref: ref, RepoID: rp.ID})
	if err != nil {
		return err
	}
	return m.q.Enqueue(context.Background(), queue.Job{
		Kind: KindCodeIndex, ID: KindCodeIndex + ":" + owner + "/" + repo, Payload: p,
	})
}

// BackfillCodeIndex 为索引缺失或落后的仓库排队重建（启动时调用）。
// 仅当注入了索引器时生效；按 id 游标分批、限速，避免一次性压满队列。
func (m *Manager) BackfillCodeIndex() {
	if m.codeIndexer == nil || m.st == nil {
		return
	}
	if !m.indexBackfilling.CompareAndSwap(false, true) {
		return // 已有回填在跑
	}
	go func() {
		defer m.indexBackfilling.Store(false)
		var cursor int64
		for {
			rows, err := m.st.AllReposAfter(cursor, 50)
			if err != nil || len(rows) == 0 {
				return
			}
			for _, r := range rows {
				cursor = r.ID
				if !m.codeIndexer.NeedsIndex(r.Owner, r.Repo, r.ID, r.DefaultBranch) {
					continue
				}
				for {
					if err := m.EnqueueCodeIndex(r.Owner, r.Repo, r.DefaultBranch); err == nil {
						break
					}
					time.Sleep(time.Second) // 队列满：等待消费者排空后重试
				}
			}
			time.Sleep(200 * time.Millisecond)
		}
	}()
}

// EnqueueImport 排队一次仓库导入。credential 为可选的 HTTPS 账号凭据（"user:token"）。
func (m *Manager) EnqueueImport(owner, repo, url, privateKey, credential string) error {
	return m.enqueue(context.Background(), KindImport, owner, repo, url, privateKey, credential)
}

// EnqueueMirror 排队一次镜像推送。
func (m *Manager) EnqueueMirror(owner, repo, url, privateKey string) error {
	return m.enqueue(context.Background(), KindMirror, owner, repo, url, privateKey, "")
}

func (m *Manager) enqueue(ctx context.Context, kind, owner, repo, url, privateKey, credential string) error {
	if m.q == nil {
		return queue.ErrQueueFull
	}
	p, err := json.Marshal(payload{Owner: owner, Repo: repo, URL: url, PrivateKey: privateKey, Credential: credential})
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

// indexCode 异步重建仓库的代码搜索索引；未启用索引时跳过。
func (m *Manager) indexCode(p payload) {
	if m.codeIndexer == nil {
		return
	}
	if err := m.codeIndexer.Index(context.Background(), p.Owner, p.Repo, p.RepoID, p.Ref); err != nil {
		logx.Infof("jobs: code index %s/%s@%s: %v", p.Owner, p.Repo, p.Ref, err)
	}
}

// analyzeLanguages 计算并落库仓库的代码成分；功能被关闭时跳过。
func (m *Manager) analyzeLanguages(p payload) {
	if !m.languageEnabled() {
		return
	}
	ref := p.Ref
	if ref == "" {
		if hb, err := gitsvc.HeadBranch(p.Owner, p.Repo); err == nil {
			ref = hb
		}
	}
	if ref == "" {
		return
	}
	stats, err := gitsvc.RepoLanguages(p.Owner, p.Repo, ref)
	if err != nil {
		// 记录的默认分支可能过期（如导入仓库），回退到实际 HEAD 再试一次。
		if hb, herr := gitsvc.HeadBranch(p.Owner, p.Repo); herr == nil && hb != ref {
			ref = hb
			stats, err = gitsvc.RepoLanguages(p.Owner, p.Repo, ref)
		}
	}
	if err != nil {
		logx.Infof("jobs: languages %s/%s@%s: %v", p.Owner, p.Repo, ref, err)
		return
	}
	out := make([]store.LanguageStat, 0, len(stats))
	for _, st := range stats {
		out = append(out, store.LanguageStat{Language: st.Language, Bytes: st.Bytes})
	}
	if err := m.st.ReplaceRepoLanguages(p.Owner, p.Repo, ref, out); err != nil {
		logx.Infof("jobs: languages store %s/%s: %v", p.Owner, p.Repo, err)
	}
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
	case KindLanguages:
		m.analyzeLanguages(p)
	case KindCodeIndex:
		m.indexCode(p)
	case KindImport:
		_ = m.st.SetImportStatus(p.Owner, p.Repo, StatusRunning, "")
		if err := gitsvc.ImportRepo(p.URL, p.Owner, p.Repo, p.PrivateKey, p.Credential); err != nil {
			logx.Infof("jobs: import %s/%s: %v", p.Owner, p.Repo, err)
			_ = m.st.SetImportStatus(p.Owner, p.Repo, StatusFailed, err.Error())
			return nil
		}
		_ = m.st.SetImportStatus(p.Owner, p.Repo, StatusSynced, "")
		// 导入完成后分析代码成分（ref 为空时按仓库 HEAD 解析）
		_ = m.EnqueueLanguages(p.Owner, p.Repo, "")
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
