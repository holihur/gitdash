package pipeline

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRunHostStep(t *testing.T) {
	dir := t.TempDir()
	cfg := &Config{Timeout: 10 * time.Second, Env: []string{"GREETING=hi"}}
	be := &builtinDockerExecutor{}
	logSink := &bytes.Buffer{}

	// 未开启 host 执行时拒绝
	SetHostAllowed(false)
	err := be.runStep(context.Background(), dir, cfg, Step{Name: "s", Run: "echo ok"}, "o", "r", "main", "abc", logSink)
	if !errors.Is(err, ErrHostDisabled) {
		t.Fatalf("host disabled: want ErrHostDisabled, got %v", err)
	}

	SetHostAllowed(true)
	defer SetHostAllowed(false)
	err = be.runStep(context.Background(), dir, cfg, Step{Name: "s", Run: "echo $GREETING"}, "o", "r", "main", "abc", logSink)
	if err != nil {
		t.Fatalf("host step: %v", err)
	}
	if out := logSink.String(); !strings.Contains(out, "hi") {
		t.Fatalf("stdout missing env injection: %q", out)
	}

	// 失败退出码
	logSink.Reset()
	err = be.runStep(context.Background(), dir, cfg, Step{Name: "s", Run: "exit 3"}, "o", "r", "main", "abc", logSink)
	if err == nil || !strings.Contains(err.Error(), "exit code 3") {
		t.Fatalf("want exit code 3 error, got %v", err)
	}
}

func TestBuiltinExecuteHostNoDockerCheck(t *testing.T) {
	// image 为空 + host 开启：即使 docker 不存在也应进入 clone/执行阶段
	SetHostAllowed(true)
	defer SetHostAllowed(false)
	be := &builtinDockerExecutor{}
	cfg := &Config{Timeout: 5 * time.Second}
	err := be.Execute(context.Background(), RunJob{Owner: "no-such", Repo: "no-such", SHA: strings.Repeat("0", 40)}, cfg, os.Stderr, nil)
	if err != nil && strings.Contains(err.Error(), ErrDockerMissing.Error()) {
		t.Fatalf("host mode should not require docker: %v", err)
	}
}
