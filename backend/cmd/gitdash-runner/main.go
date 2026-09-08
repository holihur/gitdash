// gitdash-runner 自托管 CI agent：
// 注册到 gitdash 服务端后保持 WS 长连接，接收流水线任务，在本地 docker 容器中执行。
//
// 用法：
//
//	# 1. 在 gitdash 网页签发一次性注册 token
//	# 2. 注册（secret 只显示一次，写入配置文件）
//	gitdash-runner register -server http://127.0.0.1:8080 -name build-01 -labels docker -token <TOKEN>
//	# 3. 启动
//	gitdash-runner run
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/coder/websocket"

	"gitdash/backend/internal/pipeline"
	"gitdash/backend/internal/runner"
)

// -X main.version 注入
var version = "dev"

type config struct {
	Server      string   `json:"server"`
	Name        string   `json:"name"`
	Secret      string   `json:"secret"`
	Labels      []string `json:"labels"`
	Concurrency int      `json:"concurrency"`
}

func configPath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".gitdash-runner", "config.json")
	}
	return ".gitdash-runner.json"
}

func loadConfig() (config, error) {
	var c config
	b, err := os.ReadFile(configPath())
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(b, &c)
}

func saveConfig(c config) error {
	p := configPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(p, b, 0o600)
}

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	sub := ""
	if len(os.Args) > 1 {
		sub = os.Args[1]
	}
	switch sub {
	case "register":
		cmdRegister()
	case "run":
		cmdRun()
	default:
		fmt.Fprintln(os.Stderr, "usage: gitdash-runner [register|run]")
		os.Exit(2)
	}
}

func cmdRegister() {
	fs := flag.NewFlagSet("register", flag.ExitOnError)
	server := fs.String("server", "", "gitdash 服务端地址（如 http://127.0.0.1:8080）")
	name := fs.String("name", "", "runner 名称（全局唯一）")
	labels := fs.String("labels", "", "逗号分隔标签（如 docker,go1.22）")
	token := fs.String("token", "", "一次性注册 token")
	_ = fs.Parse(os.Args[2:])
	if *server == "" || *name == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "register 需要 -server -name -token")
		os.Exit(2)
	}
	var labelsOut []string
	for _, l := range strings.Split(*labels, ",") {
		if l = strings.TrimSpace(l); l != "" {
			labelsOut = append(labelsOut, l)
		}
	}
	body, _ := json.Marshal(map[string]any{"name": *name, "labels": labelsOut, "token": *token})
	resp, err := http.Post(strings.TrimRight(*server, "/")+"/api/runner/register", "application/json", strings.NewReader(string(body)))
	if err != nil {
		log.Fatalf("register: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Runner struct {
			Name   string   `json:"name"`
			Scope  string   `json:"scope"`
			ID     int64    `json:"id"`
			Labels []string `json:"labels"`
		} `json:"runner"`
		Secret string `json:"secret"`
		Error  string `json:"error"`
		Code   string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || resp.StatusCode >= 300 {
		log.Fatalf("register failed: status=%d err=%s", resp.StatusCode, out.Error)
	}
	if err := saveConfig(config{Server: *server, Name: out.Runner.Name, Secret: out.Secret, Labels: labelsOut, Concurrency: 2}); err != nil {
		log.Fatalf("save config: %v", err)
	}
	fmt.Printf("已注册 runner %q (scope=%q, id=%d)\nsecret 已写入 %s\n", out.Runner.Name, out.Runner.Scope, out.Runner.ID, configPath())
}

// ---- run ----

type agent struct {
	cfg    config
	conn   *websocket.Conn
	wsMu   sync.Mutex // WS 写串行化
	jobs   map[int64]*jobState
	jobsMu sync.Mutex
}

func cmdRun() {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	execMode := fs.String("exec", "docker", "执行模式：docker（容器沙箱）或 host（无 Docker，直接宿主 sh 执行）")
	_ = fs.Parse(os.Args[2:])
	if *execMode == "host" {
		pipeline.SetHostAllowed(true)
		log.Printf("host 执行模式已开启（无容器沙箱，谨慎使用）")
	}
	c, err := loadConfig()
	if err != nil {
		log.Fatalf("读取配置失败（先执行 register）: %v", err)
	}
	if c.Concurrency <= 0 {
		c.Concurrency = 2
	}
	a := &agent{cfg: c, jobs: map[int64]*jobState{}}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	for ctx.Err() == nil {
		if err := a.session(ctx); err != nil && ctx.Err() == nil {
			log.Printf("连接断开: %v（5s 后重连）", err)
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}

// session 单次 WS 连接生命周期：hello → 心跳 → 消息循环。
func (a *agent) session(ctx context.Context) error {
	wsCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	ws, _, err := websocket.Dial(wsCtx, wsURL(a.cfg.Server), &websocket.DialOptions{
		HTTPHeader: map[string][]string{"Authorization": {"Bearer " + a.cfg.Name + ":" + a.cfg.Secret}},
	})
	if err != nil {
		return err
	}
	a.conn = ws
	defer func() { _ = ws.Close(websocket.StatusNormalClosure, "") }()
	log.Printf("gitdash-runner %s 已连接 %s", version, a.cfg.Server)

	a.send(runner.Message{Type: runner.TypeHello, Payload: mustJSON(runner.Hello{
		Name: a.cfg.Name, Labels: a.cfg.Labels, Version: "1",
	})})

	// 心跳
	go func() {
		t := time.NewTicker(10 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-wsCtx.Done():
				return
			case <-t.C:
				a.send(runner.Message{Type: runner.TypeHeartbeat})
			}
		}
	}()

	// 并发执行槽位
	sem := make(chan struct{}, a.cfg.Concurrency)
	for {
		_, data, err := ws.Read(wsCtx)
		if err != nil {
			return err
		}
		var msg runner.Message
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		switch msg.Type {
		case runner.TypeJob:
			var job runner.Job
			if json.Unmarshal(msg.Payload, &job) != nil {
				continue
			}
			a.send(runner.Message{Type: runner.TypeAck, Payload: mustJSON(runner.Ack{JobID: job.JobID})})
			select {
			case sem <- struct{}{}:
			case <-wsCtx.Done():
				return wsCtx.Err()
			}
			// 任务级上下文与 job_data 通道：取消与工作区分块路由都挂在 state 上
			jctx, jcancel := context.WithCancel(wsCtx)
			st := &jobState{cancel: jcancel, data: make(chan runner.JobData, 64)}
			a.jobsMu.Lock()
			a.jobs[job.RunID] = st
			a.jobsMu.Unlock()
			go func(job runner.Job, st *jobState) {
				defer func() { <-sem }()
				a.execJob(jctx, job, st)
				a.jobsMu.Lock()
				delete(a.jobs, job.RunID)
				a.jobsMu.Unlock()
			}(job, st)
		case runner.TypeJobData:
			var jd runner.JobData
			if json.Unmarshal(msg.Payload, &jd) == nil {
				a.jobsMu.Lock()
				st := a.jobs[jd.RunID]
				a.jobsMu.Unlock()
				if st != nil {
					st.data <- jd
				}
			}
		case runner.TypeJobCancel:
			var jc runner.JobCancel
			if json.Unmarshal(msg.Payload, &jc) == nil {
				a.jobsMu.Lock()
				if st, ok := a.jobs[jc.RunID]; ok {
					st.cancel()
				}
				a.jobsMu.Unlock()
			}
		case runner.TypeError:
			var s string
			_ = json.Unmarshal(msg.Payload, &s)
			log.Printf("server error: %s", s)
		}
	}
}

func (a *agent) send(msg runner.Message) {
	a.wsMu.Lock()
	defer a.wsMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	b, _ := json.Marshal(msg)
	_ = a.conn.Write(ctx, websocket.MessageText, b)
}

// wsLogWriter 把执行日志按行/块回传 server。
type wsLogWriter struct {
	a     *agent
	runID int64
	buf   []byte
	mu    sync.Mutex
}

func (w *wsLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	if len(w.buf) >= 16<<10 || bytes.IndexByte(w.buf, '\n') >= 0 {
		w.a.send(runner.Message{Type: runner.TypeJobLog, Payload: mustJSON(runner.JobLog{RunID: w.runID, Chunk: string(w.buf)})})
		w.buf = nil
	}
	return len(p), nil
}

func (w *wsLogWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) > 0 {
		w.a.send(runner.Message{Type: runner.TypeJobLog, Payload: mustJSON(runner.JobLog{RunID: w.runID, Chunk: string(w.buf)})})
		w.buf = nil
	}
}

// jobState 单个执行中任务的会话状态（cancel + 工作区分块路由）。
type jobState struct {
	cancel context.CancelFunc
	data   chan runner.JobData // 由唯一的 WS 读循环推送（禁止多 goroutine 读同一连接）
}

// execJob 执行单个任务：收工作区快照 → 解包 → 解析 DSL → 容器执行 → 回传状态。
// ctx 由调用方创建（session 中已注册到 jobState，用于取消）。
func (a *agent) execJob(ctx context.Context, job runner.Job, st *jobState) {
	// 任务结束后持续排空 data 通道，避免读循环因无人消费而阻塞
	defer func() {
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case _, ok := <-st.data:
					if !ok {
						return
					}
				}
			}
		}()
	}()
	log.Printf("job %s: run=%d repo=%s/%s ref=%s", job.JobID, job.RunID, job.Owner, job.Repo, job.Ref)

	status := func(s, errMsg string, steps int) {
		a.send(runner.Message{Type: runner.TypeJobStatus, Payload: mustJSON(runner.JobStatus{
			JobID: job.JobID, RunID: job.RunID, Status: s, Error: errMsg, StepsDone: steps,
		})})
	}

	// 接收工作区快照（server 在 ack 后立即推送 job_data 分块，由 session 读循环路由而来）
	tgz, err := a.recvWorkspace(ctx, job.RunID, st.data)
	if err != nil {
		if ctx.Err() != nil {
			status("failed", "cancelled", 0)
			return
		}
		status("failed", "recv workspace: "+err.Error(), 0)
		return
	}

	workdir, err := extractWorkspace(tgz)
	if err != nil {
		status("failed", "extract: "+err.Error(), 0)
		return
	}
	defer func() { _ = os.RemoveAll(workdir) }()

	cfg, err := pipeline.Parse([]byte(job.DSL))
	if err != nil {
		status("failed", "invalid .gitdash.yml: "+err.Error(), 0)
		return
	}

	lw := &wsLogWriter{a: a, runID: job.RunID}
	defer lw.flush()
	status("running", "", 0)
	if err := pipeline.RunInWorkspace(ctx, pipeline.RunJob{
		RunID: job.RunID, Owner: job.Owner, Repo: job.Repo, SHA: job.SHA, Ref: job.Ref,
	}, cfg, workdir, lw, func(steps int) { status("running", "", steps) }); err != nil {
		lw.flush()
		if ctx.Err() != nil {
			status("failed", "cancelled", 0)
			return
		}
		status("failed", err.Error(), 0)
		return
	}
	lw.flush()
	status("success", "", 0)
	log.Printf("job %s: success", job.JobID)
}

// recvWorkspace 消费 session 读循环路由来的 job_data 分块直到 eof，写入临时 tar.gz 文件。
func (a *agent) recvWorkspace(ctx context.Context, runID int64, data <-chan runner.JobData) (string, error) {
	tmp, err := os.CreateTemp("", "gitdash-workspace-*.tar.gz")
	if err != nil {
		return "", err
	}
	total := 0
	for {
		select {
		case <-ctx.Done():
			_ = os.Remove(tmp.Name())
			return "", ctx.Err()
		case jd := <-data:
			if jd.RunID != runID {
				continue
			}
			if _, err := tmp.Write(jd.Data); err != nil {
				_ = os.Remove(tmp.Name())
				return "", err
			}
			total += len(jd.Data)
			if jd.Eof {
				return tmp.Name(), nil
			}
			if total > 512<<20 {
				_ = os.Remove(tmp.Name())
				return "", errors.New("workspace too large")
			}
		}
	}
}

// extractWorkspace 解包 tar.gz 到新临时目录。
func extractWorkspace(tgzPath string) (string, error) {
	defer func() { _ = os.Remove(tgzPath) }()
	f, err := os.Open(tgzPath)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer func() { _ = gz.Close() }()

	dir, err := os.MkdirTemp("", "gitdash-agent-ws-*")
	if err != nil {
		return "", err
	}
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return dir, nil
		}
		if err != nil {
			_ = os.RemoveAll(dir)
			return "", err
		}
		// 防路径穿越
		target := filepath.Join(dir, filepath.Clean("/"+hdr.Name))
		if !strings.HasPrefix(target, filepath.Clean(dir)+string(os.PathSeparator)) {
			_ = os.RemoveAll(dir)
			return "", fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				_ = os.RemoveAll(dir)
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				_ = os.RemoveAll(dir)
				return "", err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode)&0o755)
			if err != nil {
				_ = os.RemoveAll(dir)
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil { //nolint:gosec // 上限 512MB 已在 recv 校验
				_ = out.Close()
				_ = os.RemoveAll(dir)
				return "", err
			}
			_ = out.Close()
		}
	}
}

func wsURL(server string) string {
	u := strings.TrimRight(server, "/")
	if strings.HasPrefix(u, "https://") {
		return "wss://" + strings.TrimPrefix(u, "https://") + "/api/runner/ws"
	}
	return "ws://" + strings.TrimPrefix(u, "http://") + "/api/runner/ws"
}

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
