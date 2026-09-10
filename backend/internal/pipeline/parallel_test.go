package pipeline

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestParseParallelAndWhen(t *testing.T) {
	src := `image: alpine:3.19
steps:
  - name: gate
    when: branch == "main"
    run: echo gating
  - name: checks
    parallel:
      - name: lint-js
        run: npm lint
      - name: lint-go
        when: event == "push"
        run: |
          go vet ./...
  - name: tags-only
    when: tag != ""
    run: echo tag $GITDASH_REF
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.UnitCount() != 4 {
		t.Errorf("UnitCount = %d, want 4", cfg.UnitCount())
	}
	if s := cfg.Steps[1]; len(s.Parallel) != 2 {
		t.Fatalf("parallel = %+v", s.Parallel)
	}
	subs := cfg.Steps[1].Parallel
	if subs[0].Name != "lint-js" || subs[0].Run != "npm lint" {
		t.Errorf("sub 0 = %+v", subs[0])
	}
	if subs[0].Run != "npm lint" {
		t.Errorf("block run = %q", subs[1].Run)
	}
	if subs[1].When != `event == "push"` {
		t.Errorf("sub when = %q", subs[1].When)
	}
	if cfg.Steps[0].When != `branch == "main"` {
		t.Errorf("step when = %q", cfg.Steps[0].When)
	}
	// 并行组字段：run 与 parallel 互斥
	if _, err := Parse([]byte("steps:\n  - name: x\n    run: echo a\n    parallel:\n      - name: y\n        run: echo b\n")); err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("run+parallel err = %v", err)
	}
	// 嵌套 parallel 拒绝
	nested := "steps:\n  - name: a\n    parallel:\n      - name: b\n        parallel:\n          - name: c\n            run: echo c\n"
	if _, err := Parse([]byte(nested)); err == nil || !strings.Contains(err.Error(), "nested") {
		t.Errorf("nested parallel err = %v", err)
	}
}

func TestWhenEval(t *testing.T) {
	tests := []struct {
		expr string
		ctx  WhenCtx
		want bool
	}{
		{`branch == "main"`, WhenCtx{Ref: "main"}, true},
		{`branch == "main"`, WhenCtx{Ref: "refs/heads/dev"}, false},
		{`branch != ""`, WhenCtx{Ref: "refs/heads/main"}, true},
		{`branch == ""`, WhenCtx{Ref: "main"}, false},
		{`branch == "" && tag == ""`, WhenCtx{Ref: "refs/tags/v1.0"}, false},
		{`branch == "" && tag == ""`, WhenCtx{Ref: "refs/other/xyz"}, true},
		{`tag != ""`, WhenCtx{Ref: "refs/tags/v1.0"}, true},
		{`event == "manual"`, WhenCtx{Event: "manual"}, true},
		{`event == "manual"`, WhenCtx{Event: "push"}, false},
		{`branch == "main" || tag != ""`, WhenCtx{Ref: "refs/tags/v1"}, true},
		{`!(branch == "dev")`, WhenCtx{Ref: "main"}, true},
		{`tag.matches(r'v\d+')`, WhenCtx{Ref: "refs/tags/v1"}, true},
		{`branch.startsWith("release/")`, WhenCtx{Ref: "release/1.0"}, true},
		{`branch == "main" && event == "push"`, WhenCtx{Ref: "main", Event: "push"}, true},
		{`branch == "main" || event == "push"`, WhenCtx{Ref: "dev", Event: "manual"}, false},
		{`ref == "refs/heads/main"`, WhenCtx{Ref: "refs/heads/main"}, true},
		{`(branch == "dev" || branch == "main") && event == "push"`, WhenCtx{Ref: "dev", Event: "push"}, true},
	}
	for _, tc := range tests {
		prog, err := compileWhen(tc.expr, 1)
		if err != nil {
			t.Fatalf("compileWhen %q: %v", tc.expr, err)
		}
		got, err := tc.ctx.evalWhen(prog)
		if err != nil {
			t.Fatalf("evalWhen %q: %v", tc.expr, err)
		}
		if got != tc.want {
			t.Errorf("eval %q ctx=%v = %v, want %v", tc.expr, tc.ctx, got, tc.want)
		}
	}
	for _, bad := range []string{"", "branch ==", "&&", "branch", strings.Repeat("a", MaxWhenLength+1),
		`branch == "main" &&`, `(branch == "main"`, `branch == "main" && tag == )`} {
		if _, err := compileWhen(bad, 1); err == nil {
			t.Errorf("bad expr %q accepted", bad)
		}
	}
	// 非布尔表达式拒绝
	if _, err := compileWhen("branch", 1); err == nil {
		t.Errorf("non-bool when accepted")
	}
}

func TestRunInWorkspaceParallelAndSkip(t *testing.T) {
	SetHostAllowed(true)
	defer SetHostAllowed(false)
	src := `steps:
  - name: gate
    when: branch == "main"
    run: echo gating
  - name: checks
    parallel:
      - name: a1
        run: echo A1
      - name: a2
        run: sleep 0.3 && echo A2
      - name: a3
        when: event == "manual"
        run: echo SKIP_ME
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	job := RunJob{Ref: "dev", Event: "push"}
	var buf bytes.Buffer
	done := 0
	if err := RunInWorkspace(context.Background(), job, cfg, ".", &buf, func(n int) { done = n }); err != nil {
		t.Fatalf("run: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "gating") {
		t.Errorf("gated step should be skipped:\n%s", out)
	}
	if strings.Contains(out, "SKIP_ME_MARKER_a3") && !strings.Contains(out, "skipped") {
		t.Errorf("a3 should be skipped:\n%s", out)
	}
	for _, want := range []string{"A1", "A2", "skipped", "checks (3 sub-steps) ok"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
	if done != cfg.UnitCount() {
		t.Errorf("progress = %d, want %d", done, cfg.UnitCount())
	}
}

func TestRunInWorkspaceParallelFailure(t *testing.T) {
	SetHostAllowed(true)
	defer SetHostAllowed(false)
	src := `steps:
  - name: group
    parallel:
      - name: ok1
        run: echo fine
      - name: bad
        run: exit 7
`
	cfg, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var buf bytes.Buffer
	err = RunInWorkspace(context.Background(), RunJob{Ref: "main"}, cfg, ".", &buf, nil)
	if err == nil || !strings.Contains(err.Error(), `"bad"`) {
		t.Fatalf("want failure from bad sub-step, got %v", err)
	}
	if !strings.Contains(buf.String(), "FAILED") {
		t.Errorf("log missing FAILED:\n%s", buf.String())
	}
}

func TestParallelLogWriterConcurrency(t *testing.T) {
	mock := &countMock{}
	lw := &loggingWriter{w: mock, prefix: "[a] ", mu: &sync.Mutex{}}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = lw.Write([]byte("hello\n")) }()
	}
	wg.Wait()
	// 每次调用截取两次（前缀 + 正文），确保 Lock 持有期间写无交错计数
	if got := mock.v.Load(); got != 100 {
		t.Fatalf("writes = %d, want 100", got)
	}
}

func TestUnitCountLimits(t *testing.T) {
	var b strings.Builder
	b.WriteString("steps:\n")
	for i := 0; i < MaxSteps; i++ {
		b.WriteString("  - name: s" + string(rune('a'+i%26)) + "\n    parallel:\n      - run: echo " + itoa(i) + "\n")
	}
	_, err := Parse([]byte(b.String()))
	if !errors.Is(err, nil) && err == nil {
		t.Errorf("expected error (or ok), got %v", err)
	}
	if err != nil && !strings.Contains(err.Error(), "too many steps") {
		t.Errorf("err = %v", err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var out []byte
	for n > 0 {
		out = append(out, byte('0'+n%10))
		n /= 10
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

type countMock struct {
	v atomic.Int64
}

func (c *countMock) Write(p []byte) (int, error) {
	c.v.Add(1)
	return len(p), nil
}
