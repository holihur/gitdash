package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"time"

	"gitdash/backend/internal/store"
)

// 远程执行会话相关错误。
var (
	ErrAckTimeout = errors.New("runner did not acknowledge job")
	ErrNoRunner   = errors.New("no online runner matches")
	ErrCancelled  = errors.New("job cancelled")
)

const (
	workspaceChunkSize = 32 << 10 // 工作区分块大小（base64 前）
	maxWorkspaceBytes  = 512 << 20
)

// session 一次远程执行的会话（server 侧等待 agent 回传）。
type session struct {
	jobID  string
	runID  int64
	events chan Message
	done   chan struct{}
	cancel context.CancelFunc
}

// RunRemote 执行一次远程流水线：
// 派发 job → 等 ack → 流式推送工作区快照（tar.gz）→ 接收日志/状态直至 success/failed。
func (h *Hub) RunRemote(ctx context.Context, runnerName string, job Job, workspace io.Reader, logSink io.Writer, progress func(int)) error {
	if !h.Enabled() {
		return ErrNoRedis
	}
	s := &session{
		jobID:  job.JobID,
		runID:  job.RunID,
		events: make(chan Message, 256),
		done:   make(chan struct{}),
	}
	sctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	defer cancel()

	h.mu.Lock()
	h.sessions[job.RunID] = s
	h.incBusy(runnerName, 1)
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.sessions, job.RunID)
		h.decBusy(runnerName)
		h.mu.Unlock()
	}()

	// 派发 job：pub/sub 存在极小概率丢帧（订阅刚建立/网络抖动），在 ack 窗口内重试
	jobMsg := Message{Type: TypeJob, Payload: mustJSON(job)}
	var acked bool
	for i := 0; i < 5 && !acked; i++ {
		if err := h.Dispatch(runnerName, jobMsg); err != nil {
			return fmt.Errorf("dispatch: %w", err)
		}
		select {
		case msg := <-s.events:
			if msg.Type == TypeAck {
				var ack Ack
				if err := json.Unmarshal(msg.Payload, &ack); err == nil && ack.JobID == job.JobID {
					acked = true
					break
				}
				return fmt.Errorf("expected ack, got %q", msg.Type)
			}
			return fmt.Errorf("expected ack, got %q", msg.Type)
		case <-time.After(time.Second):
		case <-sctx.Done():
			return sctx.Err()
		}
	}
	if !acked {
		return ErrAckTimeout
	}
	log.Printf("runner audit: DISPATCH runner=%s run=%d time=%s", runnerName, job.RunID, time.Now().UTC().Format(time.RFC3339))

	// 流式推送工作区快照
	if workspace != nil {
		if err := h.streamWorkspace(sctx, runnerName, job.RunID, workspace); err != nil {
			return fmt.Errorf("workspace: %w", err)
		}
	}

	// 等待执行结果，同时转发日志
	var runErr error
	total := 0
	for {
		select {
		case msg := <-s.events:
			switch msg.Type {
			case TypeJobLog:
				var jl JobLog
				if json.Unmarshal(msg.Payload, &jl) == nil {
					_, _ = io.WriteString(logSink, jl.Chunk)
				}
			case TypeJobStatus:
				var js JobStatus
				if err := json.Unmarshal(msg.Payload, &js); err != nil {
					continue
				}
				switch js.Status {
				case "running":
					if progress != nil && js.StepsDone > total {
						total = js.StepsDone
						progress(total)
					}
				case "success":
					return nil
				case "failed":
					if js.Error != "" {
						runErr = errors.New(js.Error)
					} else {
						runErr = errors.New("runner reported failure")
					}
					return runErr
				}
			case TypeError:
				return fmt.Errorf("runner error: %s", string(msg.Payload))
			}
		case <-sctx.Done():
			return sctx.Err()
		}
	}
}

// streamWorkspace 把 tar.gz 流分块发给 agent。
func (h *Hub) streamWorkspace(ctx context.Context, runnerName string, runID int64, r io.Reader) error {
	buf := make([]byte, workspaceChunkSize)
	seq := 0
	lr := io.LimitReader(r, maxWorkspaceBytes)
	for {
		n, err := io.ReadFull(lr, buf)
		eof := false
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			eof = true
		} else if err != nil {
			return err
		}
		if n > 0 || eof {
			chunk := JobData{RunID: runID, Seq: seq, Eof: eof, Data: buf[:n]}
			if err := h.Dispatch(runnerName, Message{Type: TypeJobData, Payload: mustJSON(chunk)}); err != nil {
				return err
			}
			seq++
		}
		if eof {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

// Cancel 取消正在远程执行的 run（通知 agent 杀容器）。
func (h *Hub) Cancel(runID int64) {
	h.mu.Lock()
	_, ok := h.sessions[runID]
	h.mu.Unlock()
	if !ok {
		return
	}
	// MVP：向本实例全部连接广播 cancel，agent 端按 runID 过滤
	h.mu.Lock()
	for _, conn := range h.conns {
		select {
		case conn.send <- Message{Type: TypeJobCancel, Payload: mustJSON(JobCancel{RunID: runID})}:
		default:
		}
	}
	h.mu.Unlock()
}

// SelectRunner 按 labels 匹配在线 runner（scope 允许集合内），取当前负载最低者。
// scopes: 允许的 runner scope 集合（含全局 ""）。
func (h *Hub) SelectRunner(labels []string, scopes []string) (store.Runner, bool) {
	if !h.Enabled() {
		return store.Runner{}, false
	}
	runners, err := h.st.ListRunnersByScopes(scopes)
	if err != nil {
		return store.Runner{}, false
	}
	h.mu.Lock()
	busy := make(map[string]int, len(h.busy))
	for k, v := range h.busy {
		busy[k] = v
	}
	h.mu.Unlock()

	ctx := context.Background()
	var best *store.Runner
	for i := range runners {
		r := runners[i]
		if !h.IsOnline(ctx, r.Name) {
			continue
		}
		if !labelsMatch(r.Labels, labels) {
			continue
		}
		if best == nil || busy[r.Name] < busy[best.Name] {
			best = &runners[i]
		}
	}
	if best == nil {
		return store.Runner{}, false
	}
	return *best, true
}

// labelsMatch runner 拥有所有要求标签才算匹配。
func labelsMatch(runnerLabels, want []string) bool {
	set := map[string]bool{}
	for _, l := range runnerLabels {
		set[l] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

// incBusy/decBusy 记录 runner 当前执行的会话数（同实例内做负载均衡）。
func (h *Hub) incBusy(name string, d int) {
	if h.busy == nil {
		h.busy = map[string]int{}
	}
	h.busy[name] += d
}

func (h *Hub) decBusy(name string) { h.incBusy(name, -1) }
