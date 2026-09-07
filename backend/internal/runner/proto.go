// Package runner 实现自托管 runner（agent）：
// agent 主动外连服务端 WS（/api/runner/ws），服务端经 Redis pub/sub 跨实例派发任务。
package runner

import "encoding/json"

// WS 消息类型（server <-> agent）
const (
	TypeHello     = "hello"      // agent → server：连接后首个消息（labels/version）
	TypeHeartbeat = "heartbeat"  // agent → server：每 10s
	TypeJob       = "job"        // server → agent：派发任务
	TypeAck       = "ack"        // agent → server：5s 内确认收到
	TypeJobData   = "job_data"   // server → agent：工作区快照分块（tar.gz）
	TypeJobStatus = "job_status" // agent → server：状态变化（running|success|failed）
	TypeJobLog    = "job_log"    // agent → server：日志分片
	TypeJobCancel = "job_cancel" // server → agent：取消任务
	TypeError     = "error"      // 双向：错误
)

// Message WS 帧。
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Hello hello 载荷。
type Hello struct {
	Name    string   `json:"name"`
	Labels  []string `json:"labels,omitempty"`
	Version string   `json:"version,omitempty"`
}

// Ack ack 载荷。
type Ack struct {
	JobID string `json:"job_id"`
}

// Job job 载荷：server → agent。
type Job struct {
	JobID string `json:"job_id"`
	RunID int64  `json:"run_id"`
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	SHA   string `json:"sha"`
	Ref   string `json:"ref"`
	DSL   string `json:"dsl"` // .gitdash.yml 原文（agent 端用统一 parser 解析）
}

// JobData 工作区快照分块（base64 tar.gz 片段；eof=true 结束）。
type JobData struct {
	RunID int64  `json:"run_id"`
	Seq   int    `json:"seq"`
	Eof   bool   `json:"eof"`
	Data  []byte `json:"data"`
}

// JobStatus 状态回传载荷。
type JobStatus struct {
	JobID     string `json:"job_id"`
	RunID     int64  `json:"run_id"`
	Status    string `json:"status"` // running | success | failed
	StepsDone int    `json:"steps_done,omitempty"`
	Error     string `json:"error,omitempty"`
}

// JobLog 日志分片载荷。
type JobLog struct {
	RunID int64  `json:"run_id"`
	Chunk string `json:"chunk"`
}

// JobCancel 取消载荷。
type JobCancel struct {
	RunID int64 `json:"run_id"`
}

// RedisChannel 定向派发 channel（跨实例路由到持有该 agent WS 的实例）。
func RedisChannel(runnerName string) string { return "gitdash:runner:" + runnerName }

// lastSeenKey agent 心跳 TTL key（任意实例可判在线）。
func lastSeenKey(runnerName string) string { return "gitdash:runner:lastseen:" + runnerName }
