package pipeline

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// TestBuiltinExecutorDisabled 覆盖 S-06 加固：GITDASH_PIPELINE_EXEC=off
// 时内置执行器（docker 与 host）整体禁用。
func TestBuiltinExecutorDisabled(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_EXEC", "")
	if BuiltinExecutorDisabled() {
		t.Fatal("executor should be enabled by default")
	}
	for _, v := range []string{"off", "OFF", " off "} {
		t.Setenv("GITDASH_PIPELINE_EXEC", v)
		if !BuiltinExecutorDisabled() {
			t.Fatalf("BuiltinExecutorDisabled() = false for %q", v)
		}
	}
	t.Setenv("GITDASH_PIPELINE_EXEC", "docker")
	if BuiltinExecutorDisabled() {
		t.Fatal("docker mode must not count as disabled")
	}
}

func TestBuiltinExecuteRejectedWhenDisabled(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_EXEC", "off")
	be := &builtinDockerExecutor{}
	cfg := &Config{Timeout: time.Second, Image: "alpine"}
	err := be.Execute(context.Background(), RunJob{Owner: "o", Repo: "r", SHA: "abc"}, cfg, os.Stderr, nil)
	if !errors.Is(err, ErrExecutorDisabled) {
		t.Fatalf("want ErrExecutorDisabled, got %v", err)
	}
}

// TestImageAllowed 覆盖 GITDASH_PIPELINE_IMAGES 白名单：未配置放行全部，
// 配置后仅放行列出的镜像；host 模式（空 image）不受限。
func TestImageAllowed(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_IMAGES", "")
	if !imageAllowed("alpine:latest") {
		t.Fatal("unset allowlist should allow any image")
	}
	t.Setenv("GITDASH_PIPELINE_IMAGES", "alpine:latest, golang:1.22")
	if !imageAllowed("alpine:latest") || !imageAllowed("golang:1.22") {
		t.Fatal("listed images should be allowed")
	}
	if imageAllowed("ubuntu:latest") {
		t.Fatal("unlisted image should be rejected")
	}
	if !imageAllowed("") {
		t.Fatal("host mode (empty image) must remain allowed")
	}
}

func TestBuiltinExecuteRejectedWhenImageNotAllowed(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_EXEC", "")
	t.Setenv("GITDASH_PIPELINE_IMAGES", "alpine:latest")
	be := &builtinDockerExecutor{}
	cfg := &Config{Timeout: time.Second, Image: "ubuntu:latest"}
	err := be.Execute(context.Background(), RunJob{Owner: "o", Repo: "r", SHA: "abc"}, cfg, os.Stderr, nil)
	if !errors.Is(err, ErrImageNotAllowed) {
		t.Fatalf("want ErrImageNotAllowed, got %v", err)
	}
}
