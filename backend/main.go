package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"gitdash/backend/internal/api"
	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/jobs"
	"gitdash/backend/internal/notify"
	"gitdash/backend/internal/pipeline"
	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/sshserver"
	"gitdash/backend/internal/store"
	"gitdash/backend/internal/updater"
	"gitdash/backend/internal/webhooks"
)

// -X main.version 注入
var version = "dev"

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func resolveStaticDir() string {
	for _, c := range []string{"./static", "../frontend/dist"} {
		if fi, err := os.Stat(filepath.Join(c, "index.html")); err == nil && !fi.IsDir() {
			abs, err := filepath.Abs(c)
			if err == nil {
				return abs
			}
		}
	}
	return ""
}

// spoolWrite 原子写一个事件 JSON 到 spool 目录（临时文件 + rename）。
func spoolWrite(dir string, ev webhooks.Event) {
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}
	name := fmt.Sprintf("%s__%s-%d-%d.json", ev.Owner, ev.Repo, os.Getpid(), time.Now().UnixNano())
	tmp, err := os.CreateTemp(dir, name+".tmp")
	if err != nil {
		return
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
		return
	}
	_ = tmp.Close()
	if err := os.Rename(tmp.Name(), filepath.Join(dir, name)); err != nil {
		_ = os.Remove(tmp.Name())
	}
}

func run() {
	dataDir := getenv("GITDASH_DATA", "./data")
	httpAddr := getenv("GITDASH_HTTP_ADDR", ":8080")
	sshAddr := getenv("GITDASH_SSH_ADDR", ":2222")
	staticDir := getenv("GITDASH_STATIC", "")

	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}
	if err := gitsvc.Init(dataDir); err != nil {
		log.Fatalf("init git service: %v", err)
	}
	// 存量仓库补装 pre-receive hook（分支保护）
	if err := gitsvc.EnsureHooks(); err != nil {
		log.Printf("ensure hooks: %v", err)
	}
	if err := pipeline.Init(dataDir); err != nil {
		log.Fatalf("init pipeline: %v", err)
	}

	// 数据库：GITDASH_DB 为 postgres:// 连接串时用 PG，否则用 SQLite 文件（默认 data/gitdash.db）
	dbDSN := os.Getenv("GITDASH_DB")
	var st *store.Store
	var err error
	if dbDSN != "" {
		st, err = store.OpenDSN(dbDSN)
	} else {
		st, err = store.Open(filepath.Join(dataDir, "gitdash.db"))
	}
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

	// 后台定期清理过期的登录失败限流行，防止表无限增长
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("login-fails cleanup panic: %v", r)
			}
		}()
		for {
			if n, err := st.CleanupLoginFails(24 * time.Hour); err != nil {
				log.Printf("login-fails cleanup: %v", err)
			} else if n > 0 {
				log.Printf("login-fails cleanup: removed %d expired rows", n)
			}
			if n, err := st.PruneDeliveries(time.Now().UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339)); err != nil {
				log.Printf("webhook-deliveries cleanup: %v", err)
			} else if n > 0 {
				log.Printf("webhook-deliveries cleanup: removed %d old rows", n)
			}
			if n, err := st.PruneOAuthStates(time.Now().UTC().Format(time.RFC3339)); err != nil {
				log.Printf("oauth-state cleanup: %v", err)
			} else if n > 0 {
				log.Printf("oauth-state cleanup: removed %d expired states", n)
			}
			time.Sleep(time.Hour)
		}
	}()

	go func() {
		log.Printf("git ssh server listening on %s", sshAddr)
		if err := sshserver.Serve(sshAddr, st, gitsvc.ReposDir(), dataDir); err != nil {
			log.Fatalf("ssh server: %v", err)
		}
	}()

	// 自动更新默认关闭；需显式 GITDASH_AUTO_UPDATE=1 且非 dev 版本
	if autoUpdateEnabled() && version != "dev" {
		interval := autoUpdateInterval()
		log.Printf("auto-update enabled (interval %s)", interval)
		go autoUpdateLoop(interval)
	}

	// Admin 引导（默认关闭）：首次启动时设置 GITDASH_ADMIN_PASSWORD 即启用管理面板
	adminPW := os.Getenv("GITDASH_ADMIN_PASSWORD")
	if adminPW != "" {
		if n, err := st.AdminCount(); err == nil && n == 0 {
			adminUser := os.Getenv("GITDASH_ADMIN_USER")
			if adminUser == "" {
				adminUser = "admin"
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(adminPW), bcrypt.DefaultCost)
			if err != nil {
				log.Fatalf("admin bootstrap: %v", err)
			}
			if err := st.CreateAdminUser(adminUser, string(hash)); err != nil {
				log.Fatalf("admin bootstrap: %v", err)
			}
			log.Printf("admin panel enabled: username %q (GITDASH_ADMIN_PASSWORD)", adminUser)
		}
	}

	a := api.New(st, version)
	a.SetSSHPort(sshAddr)
	sender := notify.NewSender()
	a.SetEmailSender(sender)

	// webhook 调度：消费 post-receive spool 中的 push 事件（webhook 投递 + 流水线触发）
	go webhooks.Run(gitsvc.SpoolDir(), st, 2*time.Second, pipeline.PushHandler(st))

	// API 侧事件 spool：issue/pull/评论事件（webhook 投递 + 邮件通知）
	apiSpool := filepath.Join(dataDir, "webhooks-spool-api")
	if err := os.MkdirAll(apiSpool, 0o755); err != nil {
		log.Fatalf("create api spool dir: %v", err)
	}
	a.Publish = func(ev webhooks.Event) { spoolWrite(apiSpool, ev) }
	go webhooks.Run(apiSpool, st, 2*time.Second, notify.EmailHandler(st, sender))

	// 流水线任务队列：memory（默认，进程内 goroutine）或 redis（asynq 持久化队列）
	queueMode := strings.ToLower(getenv("GITDASH_QUEUE", "memory"))
	if queueMode == "redis" || queueMode == "asynq" {
		redisAddr := getenv("GITDASH_REDIS_ADDR", "127.0.0.1:6379")
		redisDB, _ := strconv.Atoi(getenv("GITDASH_REDIS_DB", "0"))
		password := os.Getenv("GITDASH_REDIS_PASSWORD")
		concurrency, _ := strconv.Atoi(getenv("GITDASH_QUEUE_CONCURRENCY", "4"))
		pipeline.Bind(st, queue.NewAsynq(redisAddr, password, redisDB, concurrency))
		// 导入 / mirror 任务独立 asynq 实例（Start 一次性注册 kinds，不能与 pipeline 共用）
		jobs.Bind(st, queue.NewAsynq(redisAddr, password, redisDB, 2))
		log.Printf("task queue: asynq (redis %s db %d, concurrency %d)", redisAddr, redisDB, concurrency)
	} else {
		pipeline.Bind(st, nil)
		jobs.Bind(st, queue.NewMemory(256, 2)) // 并发的 git 网络操作限 2
		log.Printf("task queue: in-process")
	}
	// 启动时把残留 queued/running 的导入/镜像任务重新入队（memory 模式重启续跑）
	jobs.RequeuePending()

	// 孤儿 pipeline run 回收：memory 队列重启后 pending/running 不会再执行，标记 failed；
	// asynq（redis）模式任务持久化，只回收明显超时（>1h）的残留。
	orphanCutoff := time.Now().UTC().Add(-time.Hour)
	if queueMode == "memory" {
		orphanCutoff = time.Now().UTC().Add(time.Minute)
	}
	if n, err := st.FailStalePipelineRuns(orphanCutoff.Format(time.RFC3339)); err != nil {
		log.Printf("pipeline orphan recovery: %v", err)
	} else if n > 0 {
		log.Printf("pipeline orphan recovery: marked %d stale runs as failed", n)
	}

	if staticDir == "" {
		staticDir = resolveStaticDir()
	}

	log.Printf("gitdash %s: http on %s | ssh on %s | data in %s", version, httpAddr, sshAddr, dataDir)
	log.Fatal(http.ListenAndServe(httpAddr, a.Handler(staticDir)))
}

// preReceiveHook 分支保护校验：由仓库 pre-receive hook 以 `gitdash pre-receive owner repo` 调用，
// stdin 每行 `oldrev newrev refname`。拒绝时进程非零退出（push 被拒）。
// 任何内部错误（如打不开库）都放行，避免保护逻辑故障阻断所有 push。
func preReceiveHook(owner, repo string) error {
	dataDir := getenv("GITDASH_DATA", "./data")
	var refs []gitsvc.PushRef
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		if len(f) != 3 {
			continue
		}
		refs = append(refs, gitsvc.PushRef{Old: f[0], New: f[1], Ref: f[2]})
	}
	if len(refs) == 0 {
		return nil
	}
	if err := gitsvc.Init(dataDir); err != nil {
		// 目录初始化失败放行：保护逻辑故障不应阻断 push（fail-open）
		return nil //nolint:nilerr // intentional fail-open
	}
	dbPath := os.Getenv("GITDASH_DB")
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, "gitdash.db")
	}
	return gitsvc.CheckBranchProtection(dbPath, owner, repo, refs)
}

func main() {
	switch {
	case len(os.Args) < 2 || os.Args[1] == "serve":
		run()
	case os.Args[1] == "pre-receive":
		// pre-receive hook 子命令：分支保护校验（stdin: oldrev newrev refname）
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "pre-receive: missing owner/repo args")
			os.Exit(1)
		}
		if err := preReceiveHook(os.Args[2], os.Args[3]); err != nil {
			fmt.Fprintln(os.Stderr, "gitdash:", err)
			os.Exit(1)
		}
	case os.Args[1] == "update":
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		newVer, updated, err := updater.SelfUpdate(ctx, version)
		cancel()
		if err != nil {
			fmt.Fprintln(os.Stderr, "更新失败:", err)
			os.Exit(1)
		}
		if updated {
			fmt.Printf("已更新到 %s（重启 gitdash 生效）\n", newVer)
		} else {
			fmt.Printf("已是最新版本 %s\n", newVer)
		}
	case os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v":
		fmt.Println("gitdash", version)
	default:
		fmt.Fprintf(os.Stderr, "usage: gitdash [serve|update|version]\n")
		os.Exit(2)
	}
}

// 自动更新默认关闭：必须显式设置 GITDASH_AUTO_UPDATE=1/true/yes/on
func autoUpdateEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GITDASH_AUTO_UPDATE"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func autoUpdateInterval() time.Duration {
	d, err := time.ParseDuration(os.Getenv("GITDASH_AUTO_UPDATE_INTERVAL"))
	if err != nil || d < time.Hour {
		return 24 * time.Hour
	}
	return d
}

func autoUpdateLoop(interval time.Duration) {
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		newVer, updated, err := updater.SelfUpdate(ctx, version)
		cancel()
		if err != nil {
			log.Printf("auto-update: %v", err)
		} else if updated {
			log.Printf("auto-update: 已更新到 %s，进程退出以便重启加载新版本", newVer)
			os.Exit(0)
		}
		time.Sleep(interval)
	}
}
