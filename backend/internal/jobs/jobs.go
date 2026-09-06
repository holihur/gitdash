// Package jobs 仓库导入 / push mirror 同步的异步任务。
// 复用 queue 抽象（memory 或 asynq/redis）；重启后由 RequeuePending 把
// 卡在 queued/running 的任务重新入队，保证 memory 模式下也不丢任务。
package jobs

import (
	"context"
	"encoding/json"
	"log"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/store"
)

// 任务类型标识。
const (
	KindImport = "gitdash:import" // 从远程 URL 镜像导入
	KindMirror = "gitdash:mirror" // push 到镜像目标
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

var (
	boundStore *store.Store
	boundQueue queue.Queue
)

// Bind 绑定队列消费者。必须在 main 启动时调用一次。
func Bind(st *store.Store, q queue.Queue) {
	boundStore, boundQueue = st, q
	q.Start(context.Background(), []queue.JobKind{KindImport, KindMirror}, handle)
}

// RequeuePending 启动时把残留的 queued/running 任务重新入队（memory 模式重启续跑）。
func RequeuePending() {
	if boundQueue == nil {
		return
	}
	ctx := context.Background()
	if rows, err := boundStore.PendingImports(); err == nil {
		for _, r := range rows {
			if err := enqueue(ctx, KindImport, r.Owner, r.Repo, r.SourceURL, ""); err != nil {
				log.Printf("jobs: requeue import %s/%s: %v", r.Owner, r.Repo, err)
			}
		}
	}
	if rows, err := boundStore.PendingMirrors(); err == nil {
		for _, r := range rows {
			if err := enqueue(ctx, KindMirror, r.Owner, r.Repo, r.URL, r.PrivateKey); err != nil {
				log.Printf("jobs: requeue mirror %s/%s: %v", r.Owner, r.Repo, err)
			}
		}
	}
}

// EnqueueImport 排队一次仓库导入。
func EnqueueImport(owner, repo, url, privateKey string) error {
	return enqueue(context.Background(), KindImport, owner, repo, url, privateKey)
}

// EnqueueMirror 排队一次镜像推送。
func EnqueueMirror(owner, repo, url, privateKey string) error {
	return enqueue(context.Background(), KindMirror, owner, repo, url, privateKey)
}

func enqueue(ctx context.Context, kind, owner, repo, url, privateKey string) error {
	p, err := json.Marshal(payload{Owner: owner, Repo: repo, URL: url, PrivateKey: privateKey})
	if err != nil {
		return err
	}
	// ID 去重：同一仓库同类任务排队期间只保留一条
	return boundQueue.Enqueue(ctx, queue.Job{
		Kind: kind, ID: kind + ":" + owner + "/" + repo, Payload: p,
	})
}

// handle 执行任务并落状态；仓库行已删除（排队期间被删）则直接丢弃。
func handle(_ context.Context, j queue.Job) error {
	var p payload
	uerr := json.Unmarshal(j.Payload, &p)
	if uerr != nil || boundStore == nil {
		return nil //nolint:nilerr // 非法载荷直接丢弃，避免无限重试
	}
	if _, gerr := boundStore.GetRepo(p.Owner, p.Repo); gerr != nil {
		return nil //nolint:nilerr // 仓库已删除（排队期间被删），任务丢弃
	}
	switch j.Kind {
	case KindImport:
		_ = boundStore.SetImportStatus(p.Owner, p.Repo, StatusRunning, "")
		if err := gitsvc.ImportRepo(p.URL, p.Owner, p.Repo, p.PrivateKey); err != nil {
			log.Printf("jobs: import %s/%s: %v", p.Owner, p.Repo, err)
			_ = boundStore.SetImportStatus(p.Owner, p.Repo, StatusFailed, err.Error())
			return nil
		}
		_ = boundStore.SetImportStatus(p.Owner, p.Repo, StatusSynced, "")
	case KindMirror:
		_ = boundStore.SetMirrorStatus(p.Owner, p.Repo, StatusRunning, "")
		if err := gitsvc.PushMirror(p.Owner, p.Repo, p.URL, p.PrivateKey); err != nil {
			log.Printf("jobs: mirror %s/%s -> %s: %v", p.Owner, p.Repo, p.URL, err)
			_ = boundStore.SetMirrorStatus(p.Owner, p.Repo, StatusFailed, err.Error())
			return nil
		}
		_ = boundStore.SetMirrorStatus(p.Owner, p.Repo, StatusSynced, "")
	}
	return nil
}
