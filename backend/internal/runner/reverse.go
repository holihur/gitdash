// 反向连接模式：runner 监听、gitdash 服务端主动拨号。
// 用于 gitdash 位于内网（无公网地址）而 runner 暴露在公网的部署。
// 连接建立后与普通模式完全一致（同一套 hello/heartbeat/job/ack 协议与调度）。
package runner

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/coder/websocket"

	"gitdash/backend/internal/ssrf"
)

const (
	// reverseLockTTL 反向拨号选主锁 TTL（多实例只有一个实例拨同一 runner）。
	reverseLockTTL = 15 * time.Second
	// reverseReconnect 连接断开后的重连间隔。
	reverseReconnect = 5 * time.Second
)

// reverseLoop 单个反向 runner 的拨号循环状态。
type reverseLoop struct {
	cancel context.CancelFunc
	url    string
}

// reverseWatcher 周期对账：为 mode=reverse 的 runner 建立/拆除反向拨号循环。
func (h *Hub) reverseWatcher() {
	h.reconcileReverse()
	for {
		select {
		case <-h.stop:
			return
		case <-time.After(5 * time.Second):
		}
		h.reconcileReverse()
	}
}

// reconcileReverse 对齐 DB 中的反向 runner 与本地拨号循环。
func (h *Hub) reconcileReverse() {
	runners, err := h.st.ListReverseRunners()
	if err != nil {
		return
	}
	desired := map[string]string{} // name → url
	for _, r := range runners {
		desired[r.Name] = r.URL
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	// 停止已删除、已不再是反向模式，或 url 已变更的循环
	for name, l := range h.reverse {
		if want, ok := desired[name]; !ok || want != l.url {
			l.cancel()
			delete(h.reverse, name)
		}
	}
	// 为新出现的反向 runner 启动循环
	for name, url := range desired {
		if _, ok := h.reverse[name]; ok {
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		h.reverse[name] = &reverseLoop{cancel: cancel, url: url}
		go h.reverseDialLoop(ctx, name, url)
	}
}

// reverseDialLoop 持续维护与某个反向 runner 的连接：
// 抢 Redis 选主锁 → 拨号并持有连接（期间续租锁）→ 断开后释放锁并重连。
func (h *Hub) reverseDialLoop(ctx context.Context, name, url string) {
	lockKey := reverseLockKey(name)
	for ctx.Err() == nil {
		// 多实例选主：同一时刻只有一个实例去拨该 runner
		ok, err := h.rdb.SetNX(ctx, lockKey, "1", reverseLockTTL).Result()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-time.After(reverseReconnect):
			}
			continue
		}
		if !ok {
			// 其他实例持锁：稍后重试
			select {
			case <-ctx.Done():
				return
			case <-time.After(reverseReconnect):
			}
			continue
		}

		// 连接存续期间持续续租，避免被其他实例抢占
		renewCtx, cancelRenew := context.WithCancel(ctx)
		go func() {
			t := time.NewTicker(reverseLockTTL / 2)
			defer t.Stop()
			for {
				select {
				case <-renewCtx.Done():
					return
				case <-t.C:
					_ = h.rdb.Expire(context.Background(), lockKey, reverseLockTTL).Err()
				}
			}
		}()

		err = h.dialReverse(ctx, name, url)
		cancelRenew()
		_ = h.rdb.Del(context.Background(), lockKey).Err()

		if err != nil && ctx.Err() == nil {
			log.Printf("runner: reverse dial %s (%s): %v (retry in %s)", name, url, err, reverseReconnect)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(reverseReconnect):
		}
	}
}

// dialReverse 拨出到 runner 并服务该连接，阻塞至连接断开。
// 认证：携带 `Bearer {name}:{sha256(secret)}`，runner 端用本地 secret 重算 hash 校验。
func (h *Hub) dialReverse(ctx context.Context, name, url string) error {
	hash, err := h.st.GetRunnerSecretHash(name)
	if err != nil {
		return err
	}
	ws, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{ //nolint:bodyclose // coder/websocket 的 resp.Body 由连接自身管理
		HTTPHeader: map[string][]string{"Authorization": {"Bearer " + name + ":" + hash}},
		// SSRF 防护：只允许公网目标（可经 GITDASH_SSRF_ALLOW_PRIVATE=1 放开）。
		HTTPClient: &http.Client{Transport: &http.Transport{DialContext: ssrf.DialContext}},
	})
	if err != nil {
		return err
	}
	log.Printf("runner audit: REVERSE CONNECT runner=%s url=%s time=%s", name, url, time.Now().UTC().Format(time.RFC3339))
	h.serveConn(ctx, name, ws)
	return nil
}
