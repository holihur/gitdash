package store

import (
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
)

func newActivityStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range []string{"alice", "bob", "carol"} {
		if _, err := s.CreateUser(u, u+"-pass-123456"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CreateRepo("alice", "demo", "d", false); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestExtractMentions(t *testing.T) {
	got := ExtractMentions("hi @alice and @bob, ping @alice again; email a@b.com")
	want := []string{"alice", "bob"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractMentions = %v, want %v", got, want)
	}
	if ExtractMentions("no mentions here") != nil {
		t.Fatal("expected nil for no mentions")
	}
}

func TestIssueAssigneesLifecycle(t *testing.T) {
	s := newActivityStore(t)
	if _, err := s.CreateIssue("alice", "demo", "alice", "bug", "body"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIssueAssignees("alice", "demo", 1, []string{"bob", "bob", "carol", " "}); err != nil {
		t.Fatal(err)
	}
	got, err := s.IssueAssignees("alice", "demo", []int64{1})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got[1], []string{"bob", "carol"}) {
		t.Fatalf("assignees = %v", got[1])
	}
	// 全量替换
	if err := s.SetIssueAssignees("alice", "demo", 1, []string{"carol"}); err != nil {
		t.Fatal(err)
	}
	got, _ = s.IssueAssignees("alice", "demo", []int64{1})
	if !reflect.DeepEqual(got[1], []string{"carol"}) {
		t.Fatalf("after replace = %v", got[1])
	}
	// 不存在 issue
	if err := s.SetIssueAssignees("alice", "demo", 99, []string{"bob"}); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestIssueStateReason(t *testing.T) {
	s := newActivityStore(t)
	if _, err := s.CreateIssue("alice", "demo", "alice", "bug", "body"); err != nil {
		t.Fatal(err)
	}
	it, err := s.SetIssueStateWithReason("alice", "demo", 1, "closed", "not_planned")
	if err != nil {
		t.Fatal(err)
	}
	if it.State != "closed" || it.StateReason == nil || *it.StateReason != "not_planned" {
		t.Fatalf("closed issue = %+v", it)
	}
	it, err = s.SetIssueState("alice", "demo", 1, "open")
	if err != nil {
		t.Fatal(err)
	}
	if it.State != "open" || it.StateReason != nil {
		t.Fatalf("reopened issue = %+v", it)
	}
}

func TestIssueEvents(t *testing.T) {
	s := newActivityStore(t)
	if _, err := s.CreateIssue("alice", "demo", "alice", "bug", "body"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddIssueEvent("alice", "demo", "issue", 1, "alice", "opened", ""); err != nil {
		t.Fatal(err)
	}
	if err := s.AddIssueEvent("alice", "demo", "issue", 1, "bob", "labeled", "bug"); err != nil {
		t.Fatal(err)
	}
	evs, err := s.ListIssueEvents("alice", "demo", "issue", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || evs[0].Action != "opened" || evs[1].Detail != "bug" {
		t.Fatalf("events = %+v", evs)
	}
}

func TestIssueSubscriptionsAndRecipients(t *testing.T) {
	s := newActivityStore(t)
	if _, err := s.CreateIssue("alice", "demo", "alice", "bug", "body"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateComment("alice", "demo", "issue", 1, "bob", "me too", nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SubscribeIssue("alice", "demo", "issue", 1, "carol"); err != nil {
		t.Fatal(err)
	}
	if !s.IsSubscribed("alice", "demo", "issue", 1, "carol") {
		t.Fatal("carol should be subscribed")
	}
	// actor = carol：应包含 owner alice、作者 alice、评论者 bob，但不含 carol
	got := s.IssueParticipantRecipients("alice", "demo", "issue", 1, "carol", nil)
	set := map[string]bool{}
	for _, u := range got {
		set[u] = true
	}
	if !set["alice"] || !set["bob"] || set["carol"] {
		t.Fatalf("recipients = %v", got)
	}
	// @提及真实用户应被加入；不存在的用户被过滤
	got = s.IssueParticipantRecipients("alice", "demo", "issue", 1, "nobody", []string{"carol", "ghost"})
	set = map[string]bool{}
	for _, u := range got {
		set[u] = true
	}
	if !set["carol"] || set["ghost"] {
		t.Fatalf("mention recipients = %v", got)
	}
	// 取消订阅
	if err := s.UnsubscribeIssue("alice", "demo", "issue", 1, "carol"); err != nil {
		t.Fatal(err)
	}
	if s.IsSubscribed("alice", "demo", "issue", 1, "carol") {
		t.Fatal("carol should be unsubscribed")
	}
}

func TestUpdateComment(t *testing.T) {
	s := newActivityStore(t)
	c, err := s.CreateComment("alice", "demo", "issue", 1, "bob", "original", nil)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := s.UpdateComment("alice", "demo", c.ID, "edited")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Body != "edited" {
		t.Fatalf("body = %q", updated.Body)
	}
	if _, err := s.UpdateComment("alice", "demo", 9999, "x"); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestIssueFiltersLabelAssigneeSort(t *testing.T) {
	s := newActivityStore(t)
	for _, title := range []string{"one", "two", "three"} {
		if _, err := s.CreateIssue("alice", "demo", "alice", title, "body"); err != nil {
			t.Fatal(err)
		}
	}
	lbl, err := s.ListLabels("alice", "demo")
	if err != nil {
		t.Fatal(err)
	}
	var bugID int64
	for _, l := range lbl {
		if l.Name == "bug" {
			bugID = l.ID
		}
	}
	if bugID == 0 {
		t.Fatal("default bug label missing")
	}
	if err := s.SetIssueLabels("alice", "demo", 1, []int64{bugID}); err != nil {
		t.Fatal(err)
	}
	if err := s.SetIssueAssignees("alice", "demo", 2, []string{"bob"}); err != nil {
		t.Fatal(err)
	}
	byLabel, _ := s.SearchIssuesInRepo("alice", "demo", "", "", "", 0, 0, IssueFilter{Label: strconv.FormatInt(bugID, 10)})
	if len(byLabel) != 1 || byLabel[0].Number != 1 {
		t.Fatalf("by label = %+v", byLabel)
	}
	byAssignee, _ := s.SearchIssuesInRepo("alice", "demo", "", "", "", 0, 0, IssueFilter{Assignee: "bob"})
	if len(byAssignee) != 1 || byAssignee[0].Number != 2 {
		t.Fatalf("by assignee = %+v", byAssignee)
	}
	unassigned, _ := s.SearchIssuesInRepo("alice", "demo", "", "", "", 0, 0, IssueFilter{Assignee: "none"})
	if len(unassigned) != 2 {
		t.Fatalf("unassigned = %+v", unassigned)
	}
	oldest, _ := s.SearchIssuesInRepo("alice", "demo", "", "", "", 0, 0, IssueFilter{Sort: "oldest"})
	if len(oldest) != 3 || oldest[0].Number != 1 {
		t.Fatalf("oldest = %+v", oldest)
	}
	open, closed, err := s.CountIssueStates("alice", "demo", "", "", "", "")
	if err != nil || open != 3 || closed != 0 {
		t.Fatalf("counts = %d/%d, %v", open, closed, err)
	}
}
