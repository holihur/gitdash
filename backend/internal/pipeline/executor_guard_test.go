package pipeline

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// TestBuiltinExecutorDisabled 覆盖 H1/P0：内置执行器改为 opt-in（默认关闭）。
func TestBuiltinExecutorDisabled(t *testing.T) {
	SetHostAllowed(false)
	t.Setenv("GITDASH_DISABLE_REGISTRATION", "")
	// 未设置 / 非法值 / off → 默认关闭。
	for _, v := range []string{"", "off", "OFF", " off ", "weird"} {
		t.Setenv("GITDASH_PIPELINE_EXEC", v)
		if !BuiltinExecutorDisabled() {
			t.Fatalf("BuiltinExecutorDisabled() = false for %q; want true (opt-in)", v)
		}
	}
	// docker：开放注册下仍关闭；关闭注册后启用。
	t.Setenv("GITDASH_PIPELINE_EXEC", "docker")
	if !BuiltinExecutorDisabled() {
		t.Fatal("docker mode with open registration must stay disabled")
	}
	t.Setenv("GITDASH_DISABLE_REGISTRATION", "1")
	if BuiltinExecutorDisabled() {
		t.Fatal("docker mode with registration disabled must be enabled")
	}
	// host：hostAllowed 由 Init 单独门控，不在此处报告为 disabled。
	t.Setenv("GITDASH_PIPELINE_EXEC", "host")
	if BuiltinExecutorDisabled() {
		t.Fatal("host mode must not be reported disabled")
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

// TestImageAllowed 覆盖 H1/P0：镜像白名单默认拒绝（未配置时非空镜像一律拒绝），
// 配置后仅放行列出的镜像；host 模式（空 image）不受限。
func TestImageAllowed(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_IMAGES", "")
	if imageAllowed("alpine:latest") {
		t.Fatal("unset allowlist must deny images (default deny)")
	}
	if !imageAllowed("") {
		t.Fatal("host mode (empty image) must remain allowed")
	}
	t.Setenv("GITDASH_PIPELINE_IMAGES", "alpine:latest, golang:1.22")
	if !imageAllowed("alpine:latest") || !imageAllowed("golang:1.22") {
		t.Fatal("listed images should be allowed")
	}
	if imageAllowed("ubuntu:latest") {
		t.Fatal("unlisted image should be rejected")
	}
}

func TestBuiltinExecuteRejectedWhenImageNotAllowed(t *testing.T) {
	t.Setenv("GITDASH_PIPELINE_EXEC", "docker")
	t.Setenv("GITDASH_DISABLE_REGISTRATION", "1")
	t.Setenv("GITDASH_PIPELINE_IMAGES", "alpine:latest")
	be := &builtinDockerExecutor{}
	cfg := &Config{Timeout: time.Second, Image: "ubuntu:latest"}
	err := be.Execute(context.Background(), RunJob{Owner: "o", Repo: "r", SHA: "abc"}, cfg, os.Stderr, nil)
	if !errors.Is(err, ErrImageNotAllowed) {
		t.Fatalf("want ErrImageNotAllowed, got %v", err)
	}
}
