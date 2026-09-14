package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"gitdash/backend/internal/envx"
	"gitdash/backend/internal/logx"

	gormlogger "gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

// slowSQLThreshold 默认 200ms，可用 GITDASH_SLOW_SQL_MS 覆盖（非法值回退默认）。
// 在构建 logger 时读取，便于测试按用例设置环境变量。
func slowSQLThreshold() time.Duration {
	return envx.Millis("GITDASH_SLOW_SQL_MS", 200)
}

// sqlLogger 把 GORM 的 SQL 日志接入 logx（结构化，可进滚动文件/JSON）。
//
// 默认 Warn 级别：只记录 **慢查询** 与 **错误**，正常查询不刷屏；
// 需要排查时可用 GORM 的 LogMode(Info) 临时打开。
type sqlLogger struct {
	level          gormlogger.LogLevel
	slowThreshold  time.Duration
	ignoreNotFound bool
}

func newSQLLogger() gormlogger.Interface {
	return &sqlLogger{
		level:          gormlogger.Warn,
		slowThreshold:  slowSQLThreshold(),
		ignoreNotFound: true,
	}
}

func (l *sqlLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	c := *l
	c.level = level
	return &c
}

func (l *sqlLogger) Info(_ context.Context, msg string, args ...interface{}) {
	if l.level >= gormlogger.Info {
		logx.Info(fmt.Sprintf(msg, args...))
	}
}

func (l *sqlLogger) Warn(_ context.Context, msg string, args ...interface{}) {
	if l.level >= gormlogger.Warn {
		logx.Warn(fmt.Sprintf(msg, args...))
	}
}

func (l *sqlLogger) Error(_ context.Context, msg string, args ...interface{}) {
	if l.level >= gormlogger.Error {
		logx.Error(fmt.Sprintf(msg, args...))
	}
}

// Trace 记录每条 SQL 的耗时；按耗时与错误分流到 warn/error。
func (l *sqlLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	ms := float64(elapsed.Nanoseconds()) / 1e6
	thresholdMS := float64(l.slowThreshold.Nanoseconds()) / 1e6

	switch {
	case err != nil && l.level >= gormlogger.Error &&
		(!errors.Is(err, gormlogger.ErrRecordNotFound) || !l.ignoreNotFound):
		sql, rows := fc()
		logx.Error("sql error",
			"err", err,
			"elapsed_ms", ms,
			"rows", rows,
			"source", utils.FileWithLineNum(),
			"sql", sql,
		)
	case l.slowThreshold != 0 && elapsed > l.slowThreshold && l.level >= gormlogger.Warn:
		sql, rows := fc()
		logx.Warn("slow sql",
			"elapsed_ms", ms,
			"threshold_ms", thresholdMS,
			"rows", rows,
			"source", utils.FileWithLineNum(),
			"sql", sql,
		)
	case l.level >= gormlogger.Info:
		sql, rows := fc()
		logx.Debug("sql",
			"elapsed_ms", ms,
			"rows", rows,
			"sql", sql,
		)
	}
}
