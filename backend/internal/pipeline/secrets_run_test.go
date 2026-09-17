package pipeline

import (
	"bytes"
	"strings"
	"testing"
)

func TestMaskingWriter(t *testing.T) {
	var buf bytes.Buffer
	w := newMaskingWriter(&buf, []string{"sup3r-secret", "ab"}) // "ab" 过短不掩码
	in := "token=sup3r-secret and ab\n"
	n, err := w.Write([]byte(in))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != len(in) {
		t.Fatalf("n = %d, want %d", n, len(in))
	}
	if strings.Contains(buf.String(), "sup3r-secret") {
		t.Fatalf("secret leaked: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "token=***") {
		t.Fatalf("secret not masked: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "and ab") {
		t.Fatalf("short value should not be masked: %q", buf.String())
	}
}

// TestPipelineSecretInjection 验证白名单 secret 会被注入步骤环境，且在日志中被打码。
func TestPipelineSecretInjection(t *testing.T) {
	dir := t.TempDir()
	st := openPipelineStore(t, dir)
	SetHostAllowed(true)
	t.Cleanup(func() { SetHostAllowed(false) })

	sha := seedRepoFiles(t, "alice", "sec", map[string]string{
		".gitdash.yml": "secrets:\n  - MY_SECRET\nsteps:\n  - name: show\n    run: echo \"value=$MY_SECRET\"\n",
	})
	if err := st.SetPipeline("alice", "sec", true); err != nil {
		t.Fatal(err)
	}
	if err := st.SetRepoSecret("alice", "sec", "MY_SECRET", "sup3r-secret-value"); err != nil {
		t.Fatal(err)
	}
	run, err := Trigger(st, TriggerOpts{Owner: "alice", Repo: "sec", SHA: sha, Ref: "main", By: "tester", Event: "manual", Force: true})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	waitRunStatus(t, st, "alice", "sec", run.ID, "success")

	logs, err := ReadLog("alice", "sec", run.ID)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if strings.Contains(logs, "sup3r-secret-value") {
		t.Fatalf("secret leaked into logs:\n%s", logs)
	}
	if !strings.Contains(logs, "value=***") {
		t.Fatalf("secret not injected/masked:\n%s", logs)
	}
}
