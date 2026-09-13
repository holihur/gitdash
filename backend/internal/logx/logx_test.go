package logx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupFileOutputAndRotationOptions(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "app.log")
	t.Setenv("GITDASH_LOG_FILE", logPath)
	t.Setenv("GITDASH_LOG_FORMAT", "json")
	t.Setenv("GITDASH_LOG_LEVEL", "info")
	t.Setenv("GITDASH_LOG_MAX_SIZE_MB", "1")
	t.Setenv("GITDASH_LOG_MAX_BACKUPS", "2")
	t.Setenv("GITDASH_LOG_MAX_AGE_DAYS", "3")
	t.Setenv("GITDASH_LOG_COMPRESS", "false")

	Setup()
	t.Cleanup(Close)

	Info("hello", "key", "value")
	Infof("printf %d", 42)
	Debug("should be filtered at info level")

	b, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	s := string(b)
	if !strings.Contains(s, `"msg":"hello"`) || !strings.Contains(s, `"key":"value"`) {
		t.Fatalf("structured fields missing: %s", s)
	}
	if !strings.Contains(s, "printf 42") {
		t.Fatalf("Infof output missing: %s", s)
	}
	if strings.Contains(s, "should be filtered") {
		t.Fatalf("debug should be filtered at info level: %s", s)
	}

	if file == nil || file.MaxSize != 1 || file.MaxBackups != 2 || file.MaxAge != 3 || file.Compress {
		t.Fatalf("rotation options not applied: %+v", file)
	}
}

func TestSetupDebugLevel(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "debug.log")
	t.Setenv("GITDASH_LOG_FILE", logPath)
	t.Setenv("GITDASH_LOG_FORMAT", "text")
	t.Setenv("GITDASH_LOG_LEVEL", "debug")
	Setup()
	t.Cleanup(Close)

	Debug("verbose")
	b, _ := os.ReadFile(logPath)
	if !strings.Contains(string(b), "verbose") {
		t.Fatalf("debug not logged: %s", string(b))
	}
}
