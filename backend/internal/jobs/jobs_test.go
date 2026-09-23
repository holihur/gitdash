package jobs

import (
	"context"
	"encoding/json"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gitdash/backend/internal/queue"
	"gitdash/backend/internal/store"
)

type fakeIndexer struct {
	indexed []string
	forgot  []string
	needs   bool
}

func (f *fakeIndexer) Index(_ context.Context, owner, name string, _ int64, ref string) error {
	f.indexed = append(f.indexed, owner+"/"+name+"@"+ref)
	return nil
}
func (f *fakeIndexer) Forget(owner, name string) {
	f.forgot = append(f.forgot, owner+"/"+name)
}
func (f *fakeIndexer) NeedsIndex(_, _ string, _ int64, _ string) bool { return f.needs }

func newTestManager(t *testing.T) (*Manager, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "jobs.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRepo("alice", "demo", "", false); err != nil {
		t.Fatal(err)
	}
	return New(st, queue.NewMemory(8, 1)), st
}

func TestCodeIndexJobRunsIndexer(t *testing.T) {
	m, _ := newTestManager(t)
	fi := &fakeIndexer{}
	m.SetCodeIndexer(fi)
	if got := fi.forgot; got != nil {
		t.Fatalf("unexpected forgot: %v", got)
	}
	p, _ := json.Marshal(payload{Owner: "alice", Repo: "demo", Ref: "main"})
	if err := m.handle(context.Background(), queue.Job{Kind: KindCodeIndex, Payload: p}); err != nil {
		t.Fatal(err)
	}
	if len(fi.indexed) != 1 || fi.indexed[0] != "alice/demo@main" {
		t.Fatalf("indexed = %v", fi.indexed)
	}
}

// 未注入索引器时，入队与任务处理都应静默跳过（no-op），不产生错误。
func TestCodeIndexDisabled(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.EnqueueCodeIndex("alice", "demo", "main"); err != nil {
		t.Fatalf("EnqueueCodeIndex with index disabled = %v", err)
	}
	p, _ := json.Marshal(payload{Owner: "alice", Repo: "demo", Ref: "main"})
	if err := m.handle(context.Background(), queue.Job{Kind: KindCodeIndex, Payload: p}); err != nil {
		t.Fatal(err)
	}
}

func TestEnqueueCodeIndex(t *testing.T) {
	m, _ := newTestManager(t)
	fi := &fakeIndexer{}
	m.SetCodeIndexer(fi)
	if err := m.EnqueueCodeIndex("alice", "demo", "main"); err != nil {
		t.Fatal(err)
	}
}

// fakeQueue 记录入队任务与 Start 时注册的任务类型。
type fakeQueue struct {
	mu      sync.Mutex
	jobs    []queue.Job
	kinds   []queue.JobKind
	started bool
}

func (q *fakeQueue) Enqueue(_ context.Context, j queue.Job) error {
	q.mu.Lock()
	q.jobs = append(q.jobs, j)
	q.mu.Unlock()
	return nil
}

func (q *fakeQueue) Start(_ context.Context, kinds []queue.JobKind, _ queue.Handler) {
	q.mu.Lock()
	q.kinds = append([]queue.JobKind{}, kinds...)
	q.started = true
	q.mu.Unlock()
}

func (q *fakeQueue) count() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.jobs)
}

func (q *fakeQueue) jobKinds() []queue.JobKind {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]queue.JobKind{}, q.kinds...)
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "j.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser("alice", "alice-pass-123"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateRepo("alice", "demo", "", false); err != nil {
		t.Fatal(err)
	}
	return st
}

// 消费类型过滤与启动上下文。
func TestManagerConsumeKinds(t *testing.T) {
	st := newStore(t)
	fq := &fakeQueue{}
	m := New(st, fq)
	m.SetConsumeKinds(KindCodeIndex)
	m.StartContext(context.Background())
	if !fq.started {
		t.Fatal("queue not started")
	}
	if k := fq.jobKinds(); len(k) != 1 || k[0] != KindCodeIndex {
		t.Fatalf("kinds = %v, want [codeindex]", k)
	}

	fq2 := &fakeQueue{}
	New(st, fq2).Start()
	if got := len(fq2.jobKinds()); got != len(DefaultKinds()) {
		t.Fatalf("default kinds = %v, want %v", fq2.jobKinds(), DefaultKinds())
	}
}

// 仅生产模式（remote）：没有本地索引器也能入队代码索引任务。
func TestEnableCodeIndexProducer(t *testing.T) {
	st := newStore(t)
	fq := &fakeQueue{}
	m := New(st, fq)
	m.EnableCodeIndex(true)
	if err := m.EnqueueCodeIndex("alice", "demo", "main"); err != nil {
		t.Fatal(err)
	}
	if fq.count() != 1 {
		t.Fatalf("enqueued = %d, want 1", fq.count())
	}
}

// 启动回填：对 NeedsIndex 的仓库入队索引任务。
func TestBackfillCodeIndex(t *testing.T) {
	st := newStore(t)
	fq := &fakeQueue{}
	m := New(st, fq)
	m.SetCodeIndexer(&fakeIndexer{needs: true})
	m.BackfillCodeIndex()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fq.count() > 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("BackfillCodeIndex did not enqueue")
}
