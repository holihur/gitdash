package main

import (
	"context"
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
	if err := pipeline.Init(dataDir); err != nil {
		log.Fatalf("init pipeline: %v", err)
	}

	st, err := store.Open(filepath.Join(dataDir, "gitdash.db"))
	if err != nil {
		log.Fatalf("open store: %v", err)
	}

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

	// webhook 调度：消费 post-receive spool 中的 push 事件（webhook 投递 + 流水线触发）
	go webhooks.Run(gitsvc.SpoolDir(), st, 2*time.Second, pipeline.PushHandler(st))

	// 流水线任务队列：memory（默认，进程内 goroutine）或 redis（asynq 持久化队列）
	queueMode := strings.ToLower(getenv("GITDASH_QUEUE", "memory"))
	switch queueMode {
	case "redis", "asynq":
		redisAddr := getenv("GITDASH_REDIS_ADDR", "127.0.0.1:6379")
		redisDB, _ := strconv.Atoi(getenv("GITDASH_REDIS_DB", "0"))
		concurrency, _ := strconv.Atoi(getenv("GITDASH_QUEUE_CONCURRENCY", "4"))
		q := queue.NewAsynq(redisAddr, os.Getenv("GITDASH_REDIS_PASSWORD"), redisDB, concurrency)
		pipeline.Bind(st, q)
		log.Printf("pipeline queue: asynq (redis %s db %d, concurrency %d)", redisAddr, redisDB, concurrency)
	default:
		pipeline.Bind(st, nil)
		log.Printf("pipeline queue: in-process goroutine")
	}

	if staticDir == "" {
		staticDir = resolveStaticDir()
	}

	log.Printf("gitdash %s: http on %s | ssh on %s | data in %s", version, httpAddr, sshAddr, dataDir)
	log.Fatal(http.ListenAndServe(httpAddr, a.Handler(staticDir)))
}

func main() {
	switch {
	case len(os.Args) < 2 || os.Args[1] == "serve":
		run()
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
