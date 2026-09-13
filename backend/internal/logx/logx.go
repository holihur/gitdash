// Package logx 统一日志：基于 log/slog 的结构化日志 + 可选滚动文件输出。
//
// 环境变量：
//
//	GITDASH_LOG_LEVEL        debug|info|warn|error（默认 info）
//	GITDASH_LOG_FORMAT       text|json（默认 text）
//	GITDASH_LOG_FILE         启用文件日志（滚动）；空则仅输出到 stderr
//	GITDASH_LOG_MAX_SIZE_MB  单个日志文件上限 MB（默认 100）
//	GITDASH_LOG_MAX_BACKUPS  保留的历史文件数（默认 7）
//	GITDASH_LOG_MAX_AGE_DAYS 历史文件保留天数（默认 28）
//	GITDASH_LOG_COMPRESS     是否 gzip 压缩历史文件（默认 true）
//	GITDASH_LOG_ADD_SOURCE   是否记录调用位置 source（默认 false）
package logx

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	mu     sync.Mutex
	logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	file   *lumberjack.Logger
)

// Setup 依据环境变量配置全局 slog logger，并把标准库 log 一并重定向到同一目标，
// 使尚未迁移的调用点也能写入滚动文件。可重复调用（测试中切换配置）。
func Setup() {
	mu.Lock()
	defer mu.Unlock()

	level := parseLevel(os.Getenv("GITDASH_LOG_LEVEL"))
	opts := &slog.HandlerOptions{Level: level}
	if envBool("GITDASH_LOG_ADD_SOURCE", false) {
		opts.AddSource = true
	}

	var w io.Writer = os.Stderr
	if path := strings.TrimSpace(os.Getenv("GITDASH_LOG_FILE")); path != "" {
		file = &lumberjack.Logger{
			Filename:   path,
			MaxSize:    envInt("GITDASH_LOG_MAX_SIZE_MB", 100),
			MaxBackups: envInt("GITDASH_LOG_MAX_BACKUPS", 7),
			MaxAge:     envInt("GITDASH_LOG_MAX_AGE_DAYS", 28),
			Compress:   envBool("GITDASH_LOG_COMPRESS", true),
		}
		// 同时写 stderr（容器/journald 可见）与滚动文件
		w = io.MultiWriter(os.Stderr, file)
	}

	var h slog.Handler
	if strings.EqualFold(strings.TrimSpace(os.Getenv("GITDASH_LOG_FORMAT")), "json") {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	logger = slog.New(h)
	slog.SetDefault(logger)

	log.SetOutput(w)
	log.SetFlags(0)
	log.SetPrefix("")
}

// Close 关闭滚动文件（进程退出前 flush）。
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if file != nil {
		_ = file.Close()
	}
}

// Logger 返回底层 slog.Logger（需要传 context 或自定义 fields 时使用）。
func Logger() *slog.Logger { return logger }

// With 返回带固定字段的 logger。
func With(args ...any) *slog.Logger { return logger.With(args...) }

// 结构化日志（key/value 形式）。
func Debug(msg string, args ...any) { logger.Debug(msg, args...) }
func Info(msg string, args ...any)  { logger.Info(msg, args...) }
func Warn(msg string, args ...any)  { logger.Warn(msg, args...) }
func Error(msg string, args ...any) { logger.Error(msg, args...) }

// printf 风格（保留既有调用点语义，消息经 Sprintf 格式化）。
func Debugf(format string, args ...any) { logger.Debug(fmt.Sprintf(format, args...)) }
func Infof(format string, args ...any)  { logger.Info(fmt.Sprintf(format, args...)) }
func Warnf(format string, args ...any)  { logger.Warn(fmt.Sprintf(format, args...)) }
func Errorf(format string, args ...any) { logger.Error(fmt.Sprintf(format, args...)) }

// Fatalf 记录错误后退出进程（等价 log.Fatalf）。
func Fatalf(format string, args ...any) {
	logger.Error(fmt.Sprintf(format, args...))
	Close()
	os.Exit(1)
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(key))); err == nil && v > 0 {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}
