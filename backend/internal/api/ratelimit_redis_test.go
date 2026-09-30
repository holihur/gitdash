package api

import (
	"net"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

// startRedisTest 启动临时 redis-server（随机端口）；不可用时跳过。
func startRedisTest(t *testing.T) (string, func()) {
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
	deadline := time.Now().Add(5 * time.Second)
	for {
		if c, derr := net.DialTimeout("tcp", addr, time.Second); derr == nil {
			_ = c.Close()
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Skip("redis not reachable")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return addr, func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }
}

// 多实例共享额度：两个 limiter 指向同一 redis，总放行次数等于 limit。
func TestRedisWriteLimiterShared(t *testing.T) {
	addr, stop := startRedisTest(t)
	defer stop()

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer func() { _ = rdb.Close() }()

	const limit = 3
	a := newRedisWriteLimiter(rdb, limit)
	b := newRedisWriteLimiter(rdb, limit)

	allowed := 0
	// 交替用两个"实例"请求，模拟多节点共享额度。
	for i := 0; i < limit+2; i++ {
		lim := a
		if i%2 == 1 {
			lim = b
		}
		if lim.allow("ip:1.2.3.4") {
			allowed++
		}
	}
	if allowed != limit {
		t.Fatalf("allowed = %d, want %d", allowed, limit)
	}
}

// Redis 不可用时 fail-open：放行，不因限流组件故障阻断写入。
func TestRedisWriteLimiterFailOpen(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 200 * time.Millisecond, MaxRetries: -1})
	defer func() { _ = rdb.Close() }()
	if !newRedisWriteLimiter(rdb, 1).allow("ip:x") {
		t.Fatal("expected fail-open when redis is unavailable")
	}
}
