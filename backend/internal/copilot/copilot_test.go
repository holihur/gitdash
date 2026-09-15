package copilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/store"
)

func setup(t *testing.T) (*store.Store, store.CopilotSession) {
	t.Helper()
	dataDir := t.TempDir()
	if err := gitsvc.Init(dataDir); err != nil {
		t.Fatal(err)
	}
	if err := Init(dataDir); err != nil {
		t.Fatal(err)
	}
	owner, repo := "alice", "demo"
	if err := gitsvc.CreateBare(owner, repo); err != nil {
		t.Fatal(err)
	}
	if err := gitsvc.InitTemplate(owner, repo); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dataDir, "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(owner, "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	byok, err := st.CreateByokKey(owner, "k", "anthropic", "sk-test", "", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := st.CreateCopilotSession(owner, repo, owner, byok.ID, 0, "fix the bug")
	if err != nil {
		t.Fatal(err)
	}
	return st, session
}

// TestWorkspaceCloneAndClosedLoop 验证工作区克隆、会话分支与提交推送（闭环）。
func TestWorkspaceCloneAndClosedLoop(t *testing.T) {
	st, session := setup(t)
	m := NewManager(st)

	ws, branch, err := m.ensureWorkspace(session)
	if err != nil {
		t.Fatalf("ensureWorkspace: %v", err)
	}
	if branch != BranchName(session.ID) {
		t.Fatalf("branch = %q", branch)
	}
	if _, err := os.Stat(filepath.Join(ws, "README.md")); err != nil {
		t.Fatalf("workspace not cloned: %v", err)
	}

	// 模拟 agent 改动 → 提交推送。
	if err := os.WriteFile(filepath.Join(ws, "hello.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sha, err := m.commitAndPush(ws, branch, "add hello")
	if err != nil {
		t.Fatalf("commitAndPush: %v", err)
	}
	if sha == "" {
		t.Fatal("empty head sha")
	}

	branches, err := gitsvc.Branches(session.Owner, session.Repo)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, b := range branches {
		if b.Name == branch {
			found = true
		}
	}
	if !found {
		t.Fatalf("branch %q not pushed: %+v", branch, branches)
	}
	out, err := gitsvc.GitOut(gitsvc.RepoPath(session.Owner, session.Repo), "show", branch+":hello.txt")
	if err != nil || !strings.Contains(out, "hi") {
		t.Fatalf("pushed content = %q, %v", out, err)
	}
}

// TestRunTurnWithoutAgentBinary 验证找不到 agent 运行时时的清晰报错。
func TestRunTurnWithoutAgentBinary(t *testing.T) {
	t.Setenv("GITDASH_COPILOT_AGENT_BIN", filepath.Join(t.TempDir(), "does-not-exist"))
	t.Setenv("GITDASH_COPILOT_AGENT_URL", "")
	st, session := setup(t)
	m := NewManager(st)

	err := m.RunTurn(context.Background(), session, "hi", nil)
	if !errors.Is(err, ErrAgentUnavailable) {
		t.Fatalf("err = %v, want ErrAgentUnavailable", err)
	}
}

// TestMaybeOpenPullForIssue 验证关联 issue 的会话在闭环后自动开 PR，且幂等。
func TestMaybeOpenPullForIssue(t *testing.T) {
	st, _ := setup(t)
	owner, repo := "alice", "demo"
	issue, err := st.CreateIssue(owner, repo, owner, "crash on start", "steps to reproduce")
	if err != nil {
		t.Fatal(err)
	}
	byok, err := st.CreateByokKey(owner, "k2", "anthropic", "sk", "", "")
	if err != nil {
		t.Fatal(err)
	}
	session, err := st.CreateCopilotSession(owner, repo, owner, byok.ID, issue.Number, "fix it")
	if err != nil {
		t.Fatal(err)
	}
	if session.IssueNumber != issue.Number {
		t.Fatalf("issue number = %d, want %d", session.IssueNumber, issue.Number)
	}
	m := NewManager(st)
	hooked := 0
	m.SetPullHook(func(store.CopilotSession, store.PullRequest) { hooked++ })

	ws, branch, err := m.ensureWorkspace(session)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ws, "fix.txt"), []byte("fixed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.commitAndPush(ws, branch, "fix"); err != nil {
		t.Fatal(err)
	}
	m.maybeOpenPull(session, branch)

	got, err := st.GetCopilotSession(owner, repo, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PRNumber == 0 {
		t.Fatal("expected auto-opened PR")
	}
	pr, err := st.GetPull(owner, repo, got.PRNumber)
	if err != nil {
		t.Fatal(err)
	}
	if pr.SourceBranch != branch {
		t.Fatalf("source = %q, want %q", pr.SourceBranch, branch)
	}
	if !strings.Contains(pr.Body, "Closes #"+strconv.FormatInt(issue.Number, 10)) {
		t.Fatalf("body = %q, missing Closes", pr.Body)
	}
	if hooked != 1 {
		t.Fatalf("hook fired %d times, want 1", hooked)
	}

	// 幂等：再次调用不产生第二个 PR。
	m.maybeOpenPull(got, branch)
	pulls, err := st.ListOpenPullsBySource(owner, repo, branch)
	if err != nil {
		t.Fatal(err)
	}
	if len(pulls) != 1 {
		t.Fatalf("open pulls = %d, want 1", len(pulls))
	}
}
