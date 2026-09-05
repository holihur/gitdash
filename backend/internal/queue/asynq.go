package queue

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/hibiken/asynq"
)

// AsynqQueue 基于 Redis 的持久化队列（asynq）。
// 生产侧复用进程内 asynq.Client；消费侧 Start 启动 asynq.Server（内部 goroutine）。
type AsynqQueue struct {
	opt         asynq.RedisClientOpt
	concurrency int

	clientOnce sync.Once
	client     *asynq.Client

	startOnce sync.Once
}

// 确认实现 Queue 接口
var _ Queue = (*AsynqQueue)(nil)

// NewAsynq 创建 asynq 队列。concurrency <=0 时取默认 4。
func NewAsynq(addr, password string, db, concurrency int) *AsynqQueue {
	if concurrency <= 0 {
		concurrency = 4
	}
	return &AsynqQueue{
		opt:         asynq.RedisClientOpt{Addr: addr, Password: password, DB: db},
		concurrency: concurrency,
	}
}

// Client 惰性创建 asynq 客户端（复用连接配置）。
func (a *AsynqQueue) Client() *asynq.Client {
	a.clientOnce.Do(func() { a.client = asynq.NewClient(a.opt) })
	return a.client
}

// Enqueue 入队。带 ID 的任务用 TaskID 去重（同 ID 重复入队视为成功）。
// 不重试（MaxRetry 0）：任务成败由业务侧记录，避免重复执行副作用任务。
func (a *AsynqQueue) Enqueue(ctx context.Context, job Job) error {
	opts := []asynq.Option{
		asynq.MaxRetry(0),
		// 流水线单步超时上限 1h，任务整体放宽到 2h
		asynq.Timeout(2 * time.Hour),
		asynq.Queue(job.Kind),
	}
	if job.ID != "" {
		opts = append(opts, asynq.TaskID(job.ID))
	}
	_, err := a.Client().EnqueueContext(ctx, asynq.NewTask(job.Kind, job.Payload), opts...)
	if errors.Is(err, asynq.ErrTaskIDConflict) || errors.Is(err, asynq.ErrDuplicateTask) {
		return nil
	}
	return err
}

// Start 启动 asynq Server 消费任务（内部 goroutine；ctx 取消时优雅停机）。
// kinds 中的每个任务类型注册到 ServeMux，并订阅同名队列，统一交给 Handler。
func (a *AsynqQueue) Start(ctx context.Context, kinds []JobKind, h Handler) {
	a.startOnce.Do(func() {
		mux := asynq.NewServeMux()
		adapter := asynqHandlerAdapter{h}
		for _, k := range kinds {
			mux.Handle(k, adapter)
		}
		// 订阅各 kind 的专属队列 + default（Enqueue 按 kind 分队列）
		queues := map[string]int{"default": 1}
		for _, k := range kinds {
			queues[k] = 1
		}
		srv := asynq.NewServer(a.opt, asynq.Config{
			Concurrency: a.concurrency,
			Queues:      queues,
			RetryDelayFunc: func(_ int, _ error, _ *asynq.Task) time.Duration {
				return time.Second
			},
		})
		go func() {
			<-ctx.Done()
			srv.Shutdown()
		}()
		go func() {
			if err := srv.Run(mux); err != nil {
				log.Printf("queue: asynq server stopped: %v", err)
			}
		}()
	})
}

// asynqHandlerAdapter 把 queue.Handler 适配成 asynq.Handler。
type asynqHandlerAdapter struct{ h Handler }

func (a asynqHandlerAdapter) ProcessTask(ctx context.Context, t *asynq.Task) error {
	return a.h(ctx, Job{Kind: t.Type(), Payload: t.Payload()})
}
