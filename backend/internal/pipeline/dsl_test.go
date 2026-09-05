package pipeline

import (
	"strings"
	"testing"
	"time"
)

func TestParseMinimal(t *testing.T) {
	cfg, err := Parse([]byte("image: alpine:3.19\nsteps:\n  - run: echo hi\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Image != "alpine:3.19" {
		t.Errorf("image = %q", cfg.Image)
	}
	if len(cfg.Steps) != 1 || cfg.Steps[0].Run != "echo hi" {
		t.Fatalf("steps = %+v", cfg.Steps)
	}
	if cfg.Steps[0].Name != "step-1" {
		t.Errorf("default step name = %q", cfg.Steps[0].Name)
	}
	if cfg.Timeout != DefaultStepTimeout {
		t.Errorf("timeout = %s", cfg.Timeout)
	}
}

func TestParseFull(t *testing.T) {
	src := `# pipeline example
image: golang:1.22   # build image
timeout: 30m
env:
  - CGO_ENABLED=0
  - GOFLAGS='-mod=mod' # inline comment
steps:
  - name: build
    run: go build ./...
  - name: test
    run: |
      go test ./...
      go vet ./...
  - name: lint
    run: "gofmt -l ."
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Image != "golang:1.22" {
		t.Errorf("image = %q", cfg.Image)
	}
	if cfg.Timeout != 30*time.Minute {
		t.Errorf("timeout = %s", cfg.Timeout)
	}
	if len(cfg.Env) != 2 || cfg.Env[0] != "CGO_ENABLED=0" || cfg.Env[1] != "GOFLAGS=-mod=mod" {
		t.Errorf("env = %q", cfg.Env)
	}
	if len(cfg.Steps) != 3 {
		t.Fatalf("steps = %+v", cfg.Steps)
	}
	if cfg.Steps[0].Name != "build" || cfg.Steps[0].Run != "go build ./..." {
		t.Errorf("step0 = %+v", cfg.Steps[0])
	}
	want := "go test ./...\ngo vet ./..."
	if cfg.Steps[1].Run != want {
		t.Errorf("step1 run = %q, want %q", cfg.Steps[1].Run, want)
	}
	if cfg.Steps[2].Run != "gofmt -l ." {
		t.Errorf("step2 run = %q", cfg.Steps[2].Run)
	}
}

func TestParseErrors(t *testing.T) {
	cases := map[string]string{
		"missing image":     "steps:\n  - run: x\n",
		"missing steps":     "image: alpine\n",
		"empty run":         "image: alpine\nsteps:\n  - name: a\n",
		"unknown key":       "foo: bar\nimage: alpine\nsteps:\n  - run: x\n",
		"bad step key":      "image: alpine\nsteps:\n  - foo: bar\n",
		"no list dash":      "image: alpine\nsteps:\n    run: x\n",
		"env no equals":     "image: alpine\nenv:\n  - FOO\nsteps:\n  - run: x\n",
		"bad env key":       "image: alpine\nenv:\n  - 1FOO=bar\nsteps:\n  - run: x\n",
		"bad image":         "image: 'alpine newer'\nsteps:\n  - run: x\n",
		"bad timeout":       "image: alpine\ntimeout: soon\nsteps:\n  - run: x\n",
		"tab indent":        "image: alpine\nsteps:\n\t- run: x\n",
		"stray indent":      "image: alpine\n  stray: x\nsteps:\n  - run: x\n",
		"empty":             "",
		"empty step name":   "image: alpine\nsteps:\n  - name: ''\n    run: x\n",
		"bad step name":     "image: alpine\nsteps:\n  - name: 'a b'\n    run: x\n",
		"too many steps":    stepsN(25),
		"step before dash":  "image: alpine\nsteps:\n  name: x\n  run: y\n",
		"missing key colon": "image alpine\nsteps:\n  - run: x\n",
	}
	for name, src := range cases {
		if cfg, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: expected error, got cfg %+v", name, cfg)
		}
	}
}

func stepsN(n int) string {
	var b strings.Builder
	b.WriteString("image: alpine\nsteps:\n")
	for i := 0; i < n; i++ {
		b.WriteString("  - run: echo hi\n")
	}
	return b.String()
}

func TestParseTimeoutClamp(t *testing.T) {
	cfg, err := Parse([]byte("image: alpine\ntimeout: 5h\nsteps:\n  - run: x\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Timeout != MaxStepTimeout {
		t.Errorf("timeout = %s, want clamp to %s", cfg.Timeout, MaxStepTimeout)
	}
}

func TestParseQuotedValues(t *testing.T) {
	cfg, err := Parse([]byte(`image: "alpine:3.19"
env:
  - 'AB=1 2'
steps:
  - name: 'my-step'
    run: 'echo "hello world"'
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Image != "alpine:3.19" {
		t.Errorf("image = %q", cfg.Image)
	}
	if cfg.Env[0] != "AB=1 2" {
		t.Errorf("env = %q", cfg.Env)
	}
	if cfg.Steps[0].Name != "my-step" {
		t.Errorf("name = %q", cfg.Steps[0].Name)
	}
	if cfg.Steps[0].Run != `echo "hello world"` {
		t.Errorf("run = %q", cfg.Steps[0].Run)
	}
}
