package runner

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
	"github.com/redis/go-redis/v9"

	"gitdash/backend/internal/store"
)

const (
	heartbeatTTL   = 30 * time.Second
	pendingTimeout = 5 * time.Second // ack 超时
	logShardMax    = 64 << 10        // 单帧日志上限
)

// ErrNoRedis runner 功能需要 Redis（未配置时 API 返回 503）。
var ErrNoRedis = errors.New("runner hub requires redis (set GITDASH_QUEUE=redis)")

// Hub 管理本实例持有的 agent WS 连接，并经 Redis 实现跨实例派发与在线状态。
type Hub struct {
	st  *store.Store
	rdb *redis.Client

	mu       sync.Mutex
	conns    map[string]*agentConn // runnerName → 连接（仅本实例持有的）
	sessions map[int64]*session    // runID → 远程执行会话
	busy     map[string]int        // runnerName → 执行中会话数

	// reverse 反向拨号循环，runnerName → 循环状态（含取消函数），受 mu 保护。
	reverse map[string]*reverseLoop

	stopOnce sync.Once
	stop     chan struct{}
}

// agentConn 单个 agent 连接。
type agentConn struct {
	name string
	ws   *websocket.Conn
	send chan Message
}

// NewHub 创建 Hub；rdb 为 nil 时 runner 功能不可用（WS/注册返回 503）。
func NewHub(st *store.Store, rdb *redis.Client) *Hub {
	h := &Hub{
		st:       st,
		rdb:      rdb,
		conns:    map[string]*agentConn{},
		sessions: map[int64]*session{},
		busy:     map[string]int{},
		reverse:  map[string]*reverseLoop{},
		stop:     make(chan struct{}),
	}
	if rdb != nil {
		go h.offlineWatcher()
		go h.reverseWatcher()
	}
	return h
}

// Stop 停止后台协程（测试/优雅停机用）。
func (h *Hub) Stop() {
	h.stopOnce.Do(func() {
		close(h.stop)
		h.mu.Lock()
		for name, l := range h.reverse {
			l.cancel()
			delete(h.reverse, name)
		}
		h.mu.Unlock()
	})
}

// Enabled 是否已启用（Redis 就绪）。
func (h *Hub) Enabled() bool { return h != nil && h.rdb != nil }

// Dispatch 向指定 runner 派发一条消息（跨实例：经 Redis pub/sub）。
func (h *Hub) Dispatch(runnerName string, msg Message) error {
	if !h.Enabled() {
		return ErrNoRedis
	}
	// 本实例直连的 agent 直接投递，同时仍走 Redis（多实例时其他实例也可持有该 agent，
	// agent 名全局唯一，正常只有一个实例会命中连接）
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return h.rdb.Publish(context.Background(), RedisChannel(runnerName), b).Err()
}

// IsOnline 通过 Redis lastseen TTL 判断 agent 是否在线。
func (h *Hub) IsOnline(ctx context.Context, runnerName string) bool {
	if !h.Enabled() {
		return false
	}
	n, err := h.rdb.Exists(ctx, lastSeenKey(runnerName)).Result()
	return err == nil && n > 0
}

// onlineRunners 过滤出当前在线的 runner 名。
func (h *Hub) onlineRunners(ctx context.Context, names []string) map[string]bool {
	out := map[string]bool{}
	if !h.Enabled() {
		return out
	}
	pipe := h.rdb.Pipeline()
	cmds := make([]*redis.IntCmd, len(names))
	for i, n := range names {
		cmds[i] = pipe.Exists(ctx, lastSeenKey(n))
	}
	_, _ = pipe.Exec(ctx)
	for i, cmd := range cmds {
		if cmd.Val() > 0 {
			out[names[i]] = true
		}
	}
	return out
}

// offlineWatcher 周期检查 DB 标记 online 但 lastseen 过期的 runner：
// 标记 offline 并把其 running 的 run 记为 failed（不重派，避免双执行）。
func (h *Hub) offlineWatcher() {
	for {
		select {
		case <-h.stop:
			return
		case <-time.After(15 * time.Second):
		}
		h.sweepOffline(context.Background())
	}
}

func (h *Hub) sweepOffline(ctx context.Context) {
	// 遍历全部 runner：agent 优雅断开时状态会立即置 offline，此时其执行中的
	// run 仍需回收 —— 因此不能只看 DB 里 status=online 的记录。
	runners, err := h.st.ListAllRunners()
	if err != nil {
		return
	}
	names := make([]string, 0, len(runners))
	for _, r := range runners {
		names = append(names, r.Name)
	}
	online := h.onlineRunners(ctx, names)
	for _, n := range names {
		if online[n] {
			continue
		}
		_ = h.st.SetRunnerStatus(n, "offline")
		ids, err := h.st.RunningRunIDsByRunner(n)
		if err != nil || len(ids) == 0 {
			continue
		}
		log.Printf("runner audit: OFFLINE runner=%s time=%s", n, time.Now().UTC().Format(time.RFC3339))
		for _, id := range ids {
			_ = h.st.FinishPipelineRun(id, "failed", "runner "+n+" went offline")
		}
	}
}

// ---- WS 处理 ----

// HandleWS 服务 agent WS 连接：secret 认证 → 注册连接 → 双向泵。
func (h *Hub) HandleWS(w http.ResponseWriter, r *http.Request) {
	if !h.Enabled() {
		http.Error(w, ErrNoRedis.Error(), http.StatusServiceUnavailable)
		return
	}
	name, secret, ok := BearerCredentials(r)
	if !ok {
		http.Error(w, "missing credentials", http.StatusUnauthorized)
		return
	}
	if _, err := h.st.ValidateRunnerSecret(name, secret); err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	ws, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	h.serveConn(r.Context(), name, ws)
}

// serveConn 注册并运行一条 agent 连接的生命周期（双向泵 + Redis 订阅 + 在线状态）。
// 无论连接由 agent 拨入（HandleWS）还是本服务端拨出（反向模式），后续处理完全一致。
func (h *Hub) serveConn(ctx context.Context, name string, ws *websocket.Conn) {
	conn := &agentConn{name: name, ws: ws, send: make(chan Message, 64)}

	h.mu.Lock()
	if old, ok := h.conns[name]; ok { // 同名重连：踢掉旧连接
		close(old.send)
	}
	h.conns[name] = conn
	h.mu.Unlock()

	defer func() {
		h.mu.Lock()
		if c, ok := h.conns[name]; ok && c == conn {
			delete(h.conns, name)
		}
		h.mu.Unlock()
		_ = ws.Close(websocket.StatusNormalClosure, "")
		_ = h.st.SetRunnerStatus(name, "offline")
	}()

	if err := h.st.SetRunnerStatus(name, "online"); err != nil {
		log.Printf("runner: mark online %s: %v", name, err)
	}
	_ = h.rdb.Set(ctx, lastSeenKey(name), time.Now().Unix(), heartbeatTTL).Err()
	log.Printf("runner audit: CONNECT runner=%s time=%s", name, time.Now().UTC().Format(time.RFC3339))

	// 同步建立 Redis 订阅：保证随后的 Dispatch 不会因订阅未就绪而丢失
	liveCtx, cancelLive := context.WithCancel(context.WithoutCancel(ctx))
	defer cancelLive()
	sub := h.rdb.Subscribe(liveCtx, RedisChannel(name))
	if err := sub.Subscribe(liveCtx, RedisChannel(name)); err != nil {
		log.Printf("runner: subscribe %s: %v", name, err)
		return
	}
	defer func() { _ = sub.Close() }()

	go h.redisPump(liveCtx, sub, conn)
	go h.writePump(liveCtx, conn)
	h.readPump(ctx, name, conn)
}

// readPump 读取 agent 消息（hello/heartbeat/ack/status/log）。
func (h *Hub) readPump(ctx context.Context, name string, conn *agentConn) {
	for {
		_, data, err := conn.ws.Read(ctx)
		if err != nil {
			log.Printf("runner: readPump %s exit: %v", name, err)
			return
		}
		if len(data) > logShardMax {
			continue
		}
		var msg Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case TypeHello:
			var hello Hello
			if json.Unmarshal(msg.Payload, &hello) == nil && hello.Name != name {
				// hello 名与凭证不符：拒绝
				conn.send <- Message{Type: TypeError, Payload: mustJSON("hello name mismatch")}
				return
			}
		case TypeHeartbeat:
			_ = h.rdb.Set(ctx, lastSeenKey(name), time.Now().Unix(), heartbeatTTL).Err()
			_ = h.st.SetRunnerStatus(name, "online")
		case TypeAck, TypeJobStatus, TypeJobLog:
			h.handleAgentEvent(ctx, name, msg)
		}
	}
}

// handleAgentEvent 路由 agent 事件：命中活跃会话则入队，否则走全局回调（测试/扩展用）。
var AgentEvent func(ctx context.Context, runner string, msg Message)

func (h *Hub) handleAgentEvent(ctx context.Context, name string, msg Message) {
	// 按 runID 路由到会话
	runID := int64(0)
	switch msg.Type {
	case TypeJobLog:
		var jl JobLog
		if json.Unmarshal(msg.Payload, &jl) == nil {
			runID = jl.RunID
		}
	case TypeJobStatus:
		var js JobStatus
		if json.Unmarshal(msg.Payload, &js) == nil {
			runID = js.RunID
		}
	case TypeJobData, TypeJobCancel:
		return // 不会出现在 agent → server 方向
	}
	if runID != 0 {
		h.mu.Lock()
		s, ok := h.sessions[runID]
		h.mu.Unlock()
		if ok {
			select {
			case s.events <- msg:
			default:
			}
			return
		}
	}
	if msg.Type == TypeAck {
		// ack 只带 jobID，遍历会话匹配
		var ack Ack
		if json.Unmarshal(msg.Payload, &ack) == nil {
			matched := false
			h.mu.Lock()
			for _, s := range h.sessions {
				if s.jobID == ack.JobID {
					select {
					case s.events <- msg:
					default:
					}
					matched = true
					break
				}
			}
			h.mu.Unlock()
			if matched {
				return
			}
		}
	}
	if AgentEvent != nil {
		AgentEvent(ctx, name, msg)
	}
}

// writePump 把 send 队列写给 agent。
func (h *Hub) writePump(ctx context.Context, conn *agentConn) {
	for msg := range conn.send {
		b, err := json.Marshal(msg)
		if err != nil {
			continue
		}
		if err := conn.ws.Write(ctx, websocket.MessageText, b); err != nil {
			return
		}
	}
}

// redisPump 消费订阅 channel，把派发消息转入 send 队列。
func (h *Hub) redisPump(ctx context.Context, sub *redis.PubSub, conn *agentConn) {
	defer func() { _ = sub.Close() }()
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case m, ok := <-ch:
			if !ok {
				return
			}
			var msg Message
			if err := json.Unmarshal([]byte(m.Payload), &msg); err != nil {
				continue
			}
			select {
			case conn.send <- msg:
			default: // 队列满（agent 卡死）：丢弃，由 ack 超时机制重派
			}
		}
	}
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
