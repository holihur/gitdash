// Package queue 是任务队列的抽象层：
// 同一套 Job/Producer/Queue 接口，两个实现——
//   - memory：进程内 goroutine 工人池（默认，零外部依赖）
//   - asynq：Redis 持久化队列（github.com/hibiken/asynq），支持多实例/重启后继续
//
// main 按环境变量选择实现，业务侧只依赖接口。
package queue

import (
	"context"
	"errors"
	"sync"
)

// JobKind 任务类型标识（如 "pipeline:run"）。
type JobKind = string

// Job 一条待处理任务。ID 非空时用于幂等去重（同一 ID 只会入队一次）。
type Job struct {
	Kind    JobKind
	ID      string
	Payload []byte
}

// Handler 处理一条任务。返回错误表示处理失败（是否重试由具体实现决定）。
type Handler func(ctx context.Context, job Job) error

// ErrQueueFull 队列已满（memory 实现的背压信号）。
var ErrQueueFull = errors.New("queue full")

// Producer 入队接口（业务侧只依赖这个）。
type Producer interface {
	Enqueue(ctx context.Context, job Job) error
}

// Queue 生产 + 消费生命周期。Start 应立即返回（内部启动工人），
// kinds 声明要消费的任务类型（memory 实现忽略）；ctx 取消时工人退出（asynq 尽力优雅停机）。
type Queue interface {
	Producer
	Start(ctx context.Context, kinds []JobKind, h Handler)
}

// MemoryQueue 进程内实现：带缓冲 channel + N 个工人 goroutine。
type MemoryQueue struct {
	ch          chan Job
	workers     int
	once        sync.Once
	stopWorkers sync.WaitGroup
}

// NewMemory 创建进程内队列。buf 为缓冲长度，workers 为并发处理数（均 <=0 时取默认值）。
func NewMemory(buf, workers int) *MemoryQueue {
	if buf <= 0 {
		buf = 1024
	}
	if workers <= 0 {
		workers = 4
	}
	return &MemoryQueue{ch: make(chan Job, buf), workers: workers}
}

// Enqueue 非阻塞入队；缓冲满时返回 ErrQueueFull。
func (m *MemoryQueue) Enqueue(_ context.Context, job Job) error {
	select {
	case m.ch <- job:
		return nil
	default:
		return ErrQueueFull
	}
}

// Start 启动工人池（仅首次调用生效；忽略 kinds）。
func (m *MemoryQueue) Start(ctx context.Context, _ []JobKind, h Handler) {
	m.once.Do(func() {
		for i := 0; i < m.workers; i++ {
			m.stopWorkers.Add(1)
			go func() {
				defer m.stopWorkers.Done()
				for {
					select {
					case <-ctx.Done():
						return
					case job, ok := <-m.ch:
						if !ok {
							return
						}
						_ = h(ctx, job)
					}
				}
			}()
		}
	})
}

// Close 关闭内部 channel（等待工人退出；仅在测试/停机时使用）。
func (m *MemoryQueue) Close() {
	close(m.ch)
	m.stopWorkers.Wait()
}
