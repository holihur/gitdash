package queue

import (
	"context"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"
)

// startRedis 启动一个临时 redis-server（随机端口），返回 addr 与清理函数。
func startRedis(t *testing.T) (string, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cmd := exec.Command("redis-server", "--port", strconv.Itoa(port), "--save", "", "--appendonly", "no")
	if err := cmd.Start(); err != nil {
		t.Skipf("redis-server start: %v", err)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	// 等待 redis 可连
	deadline := time.Now().Add(5 * time.Second)
	for {
		c, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Skipf("redis not reachable: %v", err)
		}
		time.Sleep(100 * time.Millisecond)
	}
	return addr, func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }
}

func TestAsynqQueueDelivers(t *testing.T) {
	addr, stop := startRedis(t)
	defer stop()

	const kind = "test:job"
	q := NewAsynq(addr, "", 0, 2)

	var mu sync.Mutex
	got := map[string]int{}
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx, []JobKind{kind}, func(_ context.Context, j Job) error {
		mu.Lock()
		got[string(j.Payload)]++
		n := len(got)
		mu.Unlock()
		if n == 3 {
			close(done)
		}
		return nil
	})

	for _, p := range []string{"a", "b", "c"} {
		if err := q.Enqueue(context.Background(), Job{Kind: kind, ID: "id-" + p, Payload: []byte(p)}); err != nil {
			t.Fatalf("enqueue %q: %v", p, err)
		}
	}
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		mu.Lock()
		defer mu.Unlock()
		t.Fatalf("timed out; got=%v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, p := range []string{"a", "b", "c"} {
		if got[p] != 1 {
			t.Errorf("payload %q delivered %d times, want 1", p, got[p])
		}
	}
}

func TestAsynqQueueTaskIDDedupe(t *testing.T) {
	addr, stop := startRedis(t)
	defer stop()

	const kind = "test:dedupe"
	q := NewAsynq(addr, "", 0, 1)

	var count int
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx, []JobKind{kind}, func(_ context.Context, j Job) error {
		mu.Lock()
		count++
		mu.Unlock()
		_ = j
		return nil
	})

	for i := 0; i < 5; i++ {
		if err := q.Enqueue(context.Background(), Job{Kind: kind, ID: "same-id", Payload: []byte("x")}); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		c := count
		mu.Unlock()
		if c >= 1 {
			time.Sleep(500 * time.Millisecond) // 给潜在的重复处理留时间
			mu.Lock()
			c = count
			mu.Unlock()
			if c != 1 {
				t.Fatalf("task id dedupe failed: processed %d times, want 1", c)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("task never processed")
}

func TestAsynqQueueUnknownKindNotConsumed(t *testing.T) {
	addr, stop := startRedis(t)
	defer stop()

	q := NewAsynq(addr, "", 0, 1)
	var called bool
	var mu sync.Mutex
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q.Start(ctx, []JobKind{"known:kind"}, func(_ context.Context, _ Job) error {
		mu.Lock()
		called = true
		mu.Unlock()
		return nil
	})
	// 未注册类型的任务：不应被消费（入队成功即可，可能报 no handler）
	err := q.Enqueue(context.Background(), Job{Kind: "unknown:kind", Payload: []byte("x")})
	if err != nil && !strings.Contains(err.Error(), "not registered") && !strings.Contains(err.Error(), "ErrHandlerNotFound") {
		t.Fatalf("unexpected enqueue error: %v", err)
	}
	time.Sleep(500 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if called {
		t.Fatal("unknown kind should not be consumed")
	}
	// 类型必须实现（编译期校验放这里避免未使用导入）
	var _ asynq.Handler = asynqHandlerAdapter{}
}

func TestAsynqQueueStartOnce(t *testing.T) {
	addr, stop := startRedis(t)
	defer stop()
	q := NewAsynq(addr, "", 0, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := func(_ context.Context, _ Job) error { return nil }
	q.Start(ctx, nil, h)
	q.Start(ctx, nil, h) // 第二次应为 no-op，不 panic
	q.Client().Close()
}
