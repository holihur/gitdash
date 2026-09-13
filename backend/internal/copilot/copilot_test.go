package copilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
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
	session, err := st.CreateCopilotSession(owner, repo, owner, byok.ID, "fix the bug")
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
