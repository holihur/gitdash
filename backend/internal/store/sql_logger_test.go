package store

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"gitdash/backend/internal/logx"

	gormlogger "gorm.io/gorm/logger"
)

// 慢 SQL 必须产出 warn 日志（含 SQL 与耗时），正常 SQL 不产出。
func TestSQLLoggerSlowQuery(t *testing.T) {
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = old
		logx.Setup() // 恢复默认输出
	})

	logx.Setup()

	l := &sqlLogger{level: gormlogger.Warn, slowThreshold: time.Millisecond, ignoreNotFound: true}

	// 正常查询（begin 为当前时刻）不应触发慢日志
	l.Trace(context.Background(), time.Now(), func() (string, int64) { return "SELECT 1", 1 }, nil)
	// 超阈值查询：begin 往前拨 50ms
	l.Trace(context.Background(), time.Now().Add(-50*time.Millisecond),
		func() (string, int64) { return "SELECT slow", 1 }, nil)

	logx.Close()
	_ = w.Close()
	out, _ := io.ReadAll(r)
	got := string(out)

	if !strings.Contains(got, "slow sql") {
		t.Fatalf("expected slow sql warning, got: %q", got)
	}
	if !strings.Contains(got, "SELECT slow") {
		t.Fatalf("slow sql log missing statement, got: %q", got)
	}
	if strings.Contains(got, "SELECT 1") {
		t.Fatalf("fast query should not be logged at warn level, got: %q", got)
	}
}
