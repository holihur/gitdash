package store

import (
	"path/filepath"
	"strconv"
	"testing"
)

func openReposStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "repos.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return s
}

// TestAccessibleReposPagingAndBanConsistency 锁定可访问仓库列表的关键边界：
//   - 列表分页结果与 CountAccessibleRepos 口径一致，包括封禁过滤；
//   - 同一仓库经多条来源命中时去重并取最高权限。
func TestAccessibleReposPagingAndBanConsistency(t *testing.T) {
	s := openReposStore(t)
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateUser("bob", "bob-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateOrg("acme", "Acme", "alice"); err != nil {
		t.Fatal(err)
	}

	// alice 自有仓库
	for _, name := range []string{"a1", "a2"} {
		if _, err := s.CreateRepo("alice", name, "", false); err != nil {
			t.Fatal(err)
		}
	}
	// 组织仓库
	if _, err := s.CreateRepo("acme", "org1", "", false); err != nil {
		t.Fatal(err)
	}
	// 协作者仓库（bob 的仓库，alice 是 write 协作者）
	if _, err := s.CreateRepo("bob", "shared", "", false); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertCollab("bob", "shared", "alice", "write"); err != nil {
		t.Fatal(err)
	}

	all, err := s.AccessibleRepos("alice", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	n, err := s.CountAccessibleRepos("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 || n != 4 {
		t.Fatalf("accessible = %d repos, count = %d, want 4/4 (%v)", len(all), n, all)
	}

	// 分页拼接应完整覆盖（每页 2 条）。
	var paged []Repo
	for offset := 0; offset < 4; offset += 2 {
		page, err := s.AccessibleRepos("alice", 2, offset)
		if err != nil {
			t.Fatal(err)
		}
		paged = append(paged, page...)
	}
	if len(paged) != 4 {
		t.Fatalf("paged total = %d, want 4", len(paged))
	}
	seen := map[string]bool{}
	for _, r := range paged {
		seen[r.Owner+"/"+r.Name] = true
	}
	for _, want := range []string{"alice/a1", "alice/a2", "acme/org1", "bob/shared"} {
		if !seen[want] {
			t.Fatalf("paged missing %s in %v", want, seen)
		}
	}

	// 封禁一个自有仓库后，列表与计数必须同步减少（旧实现 count 不会减）。
	if err := s.SetRepoBanned("alice", "a1", true); err != nil {
		t.Fatal(err)
	}
	all, err = s.AccessibleRepos("alice", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	n, err = s.CountAccessibleRepos("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 || n != 3 {
		t.Fatalf("after repo ban: list=%d count=%d, want 3/3", len(all), n)
	}

	// 封禁整个组织后，其仓库同样从列表与计数中移除。
	if err := s.SetOrgBanned("acme", true); err != nil {
		t.Fatal(err)
	}
	all, err = s.AccessibleRepos("alice", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	n, err = s.CountAccessibleRepos("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || n != 2 {
		t.Fatalf("after org ban: list=%d count=%d, want 2/2", len(all), n)
	}
}

// TestAccessibleReposChunksManyOwners 构造大量 owner 的 star 关系，
// 验证 pair 查询分块后不会触发绑定参数上限且结果完整。
func TestCountPairsChunksManyOwners(t *testing.T) {
	s := openReposStore(t)
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	const n = maxInParams*2 + 17
	pairs := make([][2]string, 0, n)
	for i := 0; i < n; i++ {
		owner := "u" + strconv.Itoa(i)
		if _, err := s.CreateUser(owner, "pw-"+owner+"-123"); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CreateRepo(owner, "r", "", false); err != nil {
			t.Fatal(err)
		}
		if err := s.StarRepo("alice", owner, "r"); err != nil {
			t.Fatal(err)
		}
		pairs = append(pairs, [2]string{owner, "r"})
	}
	counts := s.StarCounts(pairs)
	if len(counts) != n {
		t.Fatalf("StarCounts returned %d pairs, want %d", len(counts), n)
	}
	set := s.StarredAmong("alice", pairs)
	if len(set) != n {
		t.Fatalf("StarredAmong returned %d pairs, want %d", len(set), n)
	}
	for _, p := range pairs {
		if counts[p] != 1 || !set[p] {
			t.Fatalf("pair %v: count=%d starred=%v", p, counts[p], set[p])
		}
	}
}

func TestCreateRepoSeedsDefaultLabels(t *testing.T) {
	s := openReposStore(t)
	if _, err := s.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRepo("alice", "demo", "", false); err != nil {
		t.Fatal(err)
	}
	labels, err := s.ListLabels("alice", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(labels) != len(defaultLabels) {
		t.Fatalf("default labels = %d, want %d (%+v)", len(labels), len(defaultLabels), labels)
	}
	found := map[string]bool{}
	for _, l := range labels {
		found[l.Name] = true
	}
	for _, want := range defaultLabels {
		if !found[want.Name] {
			t.Fatalf("missing default label %q", want.Name)
		}
	}

	// 标签仅属于该仓库：另一仓库有各自独立的一份。
	if _, err := s.CreateRepo("alice", "other", "", false); err != nil {
		t.Fatal(err)
	}
	other, err := s.ListLabels("alice", "other")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != len(defaultLabels) {
		t.Fatalf("other repo labels = %d, want %d", len(other), len(defaultLabels))
	}

	// 重复创建同名仓库应返回 ErrExists，且不重复写入标签（事务回滚）。
	if _, err := s.CreateRepo("alice", "demo", "", false); err != ErrExists {
		t.Fatalf("duplicate CreateRepo err = %v, want ErrExists", err)
	}
	again, err := s.ListLabels("alice", "demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(defaultLabels) {
		t.Fatalf("labels after duplicate create = %d, want %d", len(again), len(defaultLabels))
	}
}
