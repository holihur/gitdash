package store

import (
	"errors"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIssueSearchAndPin(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateIssue("alice", "demo", "alice", "login crash", "steps to reproduce"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateIssue("alice", "demo", "alice", "dark mode", "feature request"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateIssue("alice", "demo", "alice", "docs", "update readme"); err != nil {
		t.Fatal(err)
	}

	// 关键词：命中标题 / 正文 / 作者
	got, _ := s.SearchIssuesInRepo("alice", "demo", "crash", "", "", 0, 0)
	if len(got) != 1 || got[0].Number != 1 {
		t.Fatalf("search crash = %+v", got)
	}
	got, _ = s.SearchIssuesInRepo("alice", "demo", "feature", "", "", 0, 0)
	if len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("search feature = %+v", got)
	}
	got, _ = s.SearchIssuesInRepo("alice", "demo", "ALICE", "", "", 0, 0)
	if len(got) != 3 {
		t.Fatalf("search author = %d", len(got))
	}
	if n, _ := s.CountSearchIssuesInRepo("alice", "demo", "crash", "", ""); n != 1 {
		t.Fatalf("count = %d", n)
	}

	// 状态过滤
	if _, err := s.SetIssueState("alice", "demo", 2, "closed"); err != nil {
		t.Fatal(err)
	}
	closed, _ := s.SearchIssuesInRepo("alice", "demo", "", "closed", "", 0, 0)
	if len(closed) != 1 || closed[0].Number != 2 {
		t.Fatalf("closed = %+v", closed)
	}

	// 置顶：列表置顶排最前
	if _, err := s.SetIssuePinned("alice", "demo", 3, true); err != nil {
		t.Fatal(err)
	}
	list, _ := s.ListIssues("alice", "demo", 0, 0)
	if len(list) == 0 || list[0].Number != 3 || !list[0].Pinned {
		t.Fatalf("pinned first = %+v", list)
	}
	// 取消置顶
	if it, err := s.SetIssuePinned("alice", "demo", 3, false); err != nil || it.Pinned {
		t.Fatalf("unpin = %+v, %v", it, err)
	}
}

func TestUpdateAndDeleteIssue(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	it, err := s.CreateIssue("alice", "demo", "alice", "old title", "old body")
	if err != nil {
		t.Fatal(err)
	}

	// 只改标题
	newTitle := "new title"
	got, err := s.UpdateIssue("alice", "demo", it.Number, &newTitle, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new title" || got.Body != "old body" {
		t.Fatalf("after title update: %+v", got)
	}

	// 只改正文
	newBody := "new body"
	got, err = s.UpdateIssue("alice", "demo", it.Number, nil, &newBody)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new title" || got.Body != "new body" {
		t.Fatalf("after body update: %+v", got)
	}

	// 不存在的 issue
	if _, err := s.UpdateIssue("alice", "demo", 999, &newTitle, nil); !errors.Is(err, ErrNotFound) {
		t.Fatalf("update missing issue: %v", err)
	}

	// 建立评论 + 标签，删除后应级联清理
	if _, err := s.CreateComment("alice", "demo", "issue", it.Number, "alice", "hi", nil); err != nil {
		t.Fatal(err)
	}
	lbl, err := s.CreateLabel("alice", "demo", "triage", "ff0000")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetIssueLabels("alice", "demo", it.Number, []int64{lbl.ID}); err != nil {
		t.Fatal(err)
	}

	if err := s.DeleteIssue("alice", "demo", it.Number); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetIssue("alice", "demo", it.Number); !errors.Is(err, ErrNotFound) {
		t.Fatalf("issue should be gone: %v", err)
	}
	comments, err := s.ListComments("alice", "demo", "issue", it.Number, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(comments) != 0 {
		t.Fatalf("comments should be cascaded, got %d", len(comments))
	}
	var labelLinks int64
	if err := s.db.Model(&issueLabelRow{}).Where("issue_id = ?", it.ID).Count(&labelLinks).Error; err != nil {
		t.Fatal(err)
	}
	if labelLinks != 0 {
		t.Fatalf("issue labels should be cascaded, got %d", labelLinks)
	}

	// 再次删除 -> ErrNotFound
	if err := s.DeleteIssue("alice", "demo", it.Number); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing issue: %v", err)
	}
}

func TestIssueMilestoneFilter(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"a", "b", "c"} {
		if _, err := s.CreateIssue("alice", "demo", "alice", title, ""); err != nil {
			t.Fatal(err)
		}
	}
	ms, err := s.CreateMilestone("alice", "demo", "v1", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetIssueMilestone("alice", "demo", 1, ms.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIssueMilestone("alice", "demo", 2, ms.ID); err != nil {
		t.Fatal(err)
	}

	id := strconv.FormatInt(ms.ID, 10)
	if got, err := s.SearchIssuesInRepo("alice", "demo", "", "", id, 0, 0); err != nil || len(got) != 2 {
		t.Fatalf("milestone filter = %+v, %v", got, err)
	}
	if got, err := s.SearchIssuesInRepo("alice", "demo", "", "", "none", 0, 0); err != nil || len(got) != 1 || got[0].Number != 3 {
		t.Fatalf("no-milestone filter = %+v, %v", got, err)
	}
	if n, _ := s.CountSearchIssuesInRepo("alice", "demo", "", "", "none"); n != 1 {
		t.Fatalf("no-milestone count = %d", n)
	}
	// 组合状态 + 里程碑
	if _, err := s.SetIssueState("alice", "demo", 1, "closed"); err != nil {
		t.Fatal(err)
	}
	if got, err := s.SearchIssuesInRepo("alice", "demo", "", "open", id, 0, 0); err != nil || len(got) != 1 || got[0].Number != 2 {
		t.Fatalf("combined filter = %+v, %v", got, err)
	}
	// 未知 / 非法 id 应返回空集而非全部
	if got, _ := s.SearchIssuesInRepo("alice", "demo", "", "", "9999", 0, 0); len(got) != 0 {
		t.Fatalf("unknown milestone should be empty, got %+v", got)
	}
	if got, _ := s.SearchIssuesInRepo("alice", "demo", "", "", "abc", 0, 0); len(got) != 0 {
		t.Fatalf("invalid milestone should be empty, got %+v", got)
	}
}
