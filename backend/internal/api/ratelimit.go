package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"gitdash/backend/internal/envx"
	"gitdash/backend/internal/logx"
)

// 通用写操作限流。
//
// 背景：登录/注册/找回密码/反馈已有专门限流，但 issue、评论、PR、project、
// release、package 等所有已认证写接口此前没有任何节流，单个账号即可高频创建
// 对象拖垮数据库/磁盘。这里对所有非幂等 HTTP 方法加一层限流。
//
// 多实例部署：队列为 redis 时（main 调用 UseRedisWriteLimit）用 redis 共享计数，
// 多节点共用同一份额度；否则回退进程内令牌桶（每实例独立计数，仅作缓解）。

const writeRateWindow = 10 * time.Minute

// writeLimiter 限流器抽象：in-memory 令牌桶或 redis 共享计数。
type writeLimiter interface {
	allow(key string) bool
}

type rateBucket struct {
	tokens float64
	last   time.Time
}

type writeRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*rateBucket
	rate    float64 // 每秒补充令牌
	burst   float64
}

func newWriteRateLimiter(perMinute int) *writeRateLimiter {
	if perMinute <= 0 {
		perMinute = 240
	}
	l := &writeRateLimiter{
		buckets: map[string]*rateBucket{},
		rate:    float64(perMinute) / 60.0,
		burst:   float64(perMinute),
	}
	go l.cleanupLoop()
	return l
}

func (l *writeRateLimiter) allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	b := l.buckets[key]
	if b == nil {
		b = &rateBucket{tokens: l.burst, last: now}
		l.buckets[key] = b
	}
	b.tokens += now.Sub(b.last).Seconds() * l.rate
	if b.tokens > l.burst {
		b.tokens = l.burst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

func (l *writeRateLimiter) cleanupLoop() {
	t := time.NewTicker(writeRateWindow)
	defer t.Stop()
	for range t.C {
		cutoff := time.Now().Add(-writeRateWindow)
		l.mu.Lock()
		for k, b := range l.buckets {
			if b.last.Before(cutoff) {
				delete(l.buckets, k)
			}
		}
		l.mu.Unlock()
	}
}

// defaultWriteLimiter 进程级单例（回退用）；GITDASH_DISABLE_RATE_LIMIT=1 时关闭。
var defaultWriteLimiter = newWriteRateLimiter(int(envx.Int64("GITDASH_WRITE_RPM", 240)))

// redisWriteLimiter 基于 redis 的固定窗口计数，多实例共享同一份额度。
// Redis 不可用时 fail-open（放行）：限流是缓解手段，不应因依赖故障阻断写入。
type redisWriteLimiter struct {
	rdb    redis.UniversalClient
	limit  int64
	window time.Duration
}

func newRedisWriteLimiter(rdb redis.UniversalClient, perMinute int) *redisWriteLimiter {
	if perMinute <= 0 {
		perMinute = 240
	}
	return &redisWriteLimiter{rdb: rdb, limit: int64(perMinute), window: time.Minute}
}

func (l *redisWriteLimiter) allow(key string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	k := "gitdash:ratelimit:write:" + key
	n, err := l.rdb.Incr(ctx, k).Result()
	if err != nil {
		logx.Infof("write rate limit: redis incr: %v", err)
		return true // fail-open
	}
	if n == 1 {
		l.rdb.Expire(ctx, k, l.window)
	}
	return n <= l.limit
}

// rateKeyForRequest 生成限流键：优先按登录凭证区分，避免同一 NAT/IP 下
// 多个用户共享额度；匿名请求按客户端 IP。
func rateKeyForRequest(r *http.Request) string {
	ip := clientIP(r)
	cred := r.Header.Get("Authorization")
	if cred == "" {
		if c, err := r.Cookie(sessionCookie); err == nil {
			cred = c.Value
		}
	}
	if cred == "" {
		return "ip:" + ip
	}
	sum := sha256.Sum256([]byte(cred))
	return "ip:" + ip + ":t:" + hex.EncodeToString(sum[:8])
}

// writeThrottleExempt 关键路径不参与通用写限流：登录/注册等已有专门限流，
// 入站邮件/runner 注册使用共享密钥或一次性令牌。
func writeThrottleExempt(path string) bool {
	switch {
	case strings.HasPrefix(path, "/api/auth/"):
		return true
	case path == "/api/mail/inbound":
		return true
	case strings.HasPrefix(path, "/api/runner/register"):
		return true
	}
	return false
}

// writeThrottle 对所有非幂等请求按用户/IP 限流；超限返回 429。
func (a *API) writeThrottle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
			if !rateLimitDisabled && !writeThrottleExempt(r.URL.Path) {
				lim := a.writeLimiter
				if lim == nil {
					lim = defaultWriteLimiter
				}
				if !lim.allow(rateKeyForRequest(r)) {
					writeCode(w, http.StatusTooManyRequests, "rate_limited", "too many requests, slow down")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}
