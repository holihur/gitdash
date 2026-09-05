package queue

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestMemoryQueueDelivers(t *testing.T) {
	q := NewMemory(16, 2)
	got := make(chan Job, 8)
	var kinds []JobKind
	q.Start(context.Background(), kinds, func(_ context.Context, j Job) error {
		got <- j
		return nil
	})
	payloads := []string{"a", "b", "c"}
	for _, p := range payloads {
		if err := q.Enqueue(context.Background(), Job{Kind: "test:job", Payload: []byte(p)}); err != nil {
			t.Fatalf("enqueue %q: %v", p, err)
		}
	}
	seen := map[string]bool{}
	for range payloads {
		select {
		case j := <-got:
			seen[string(j.Payload)] = true
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out; seen=%v", seen)
		}
	}
	for _, p := range payloads {
		if !seen[p] {
			t.Errorf("job %q not delivered", p)
		}
	}
}

func TestMemoryQueueFull(t *testing.T) {
	// 缓冲 1 且不启动工人：第二条入队应报 ErrQueueFull
	q := NewMemory(1, 0)
	if err := q.Enqueue(context.Background(), Job{Kind: "t", Payload: []byte("1")}); err != nil {
		t.Fatalf("first enqueue: %v", err)
	}
	err := q.Enqueue(context.Background(), Job{Kind: "t", Payload: []byte("2")})
	if !errors.Is(err, ErrQueueFull) {
		t.Fatalf("second enqueue err = %v, want ErrQueueFull", err)
	}
}

func TestMemoryQueueHandlerErrorIgnored(t *testing.T) {
	q := NewMemory(8, 1)
	done := make(chan struct{})
	q.Start(context.Background(), nil, func(_ context.Context, j Job) error {
		close(done)
		return errors.New("boom")
	})
	_ = q.Enqueue(context.Background(), Job{Kind: "t", Payload: []byte("x")})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("handler not called")
	}
}

func TestMemoryQueueConcurrentEnqueue(t *testing.T) {
	q := NewMemory(256, 4)
	var mu sync.Mutex
	count := 0
	done := make(chan struct{})
	var want = 50
	q.Start(context.Background(), nil, func(_ context.Context, _ Job) error {
		mu.Lock()
		count++
		c := count
		mu.Unlock()
		if c == want {
			close(done)
		}
		return nil
	})
	for i := 0; i < want; i++ {
		_ = q.Enqueue(context.Background(), Job{Kind: "t"})
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("processed %d/%d", count, want)
	}
}
