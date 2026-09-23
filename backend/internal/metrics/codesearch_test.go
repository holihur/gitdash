package metrics

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCodeSearchMetrics(t *testing.T) {
	ObserveCodeSearch("index")
	ObserveCodeSearch("grep")
	ObserveCodeSearchIndexing()
	ObserveCodeIndexMode(true)
	ObserveCodeIndexMode(false)
	start := ObserveCodeIndexStart()
	ObserveCodeIndexDone(start, nil)
	ObserveCodeIndexDone(start, errors.New("boom"))
	SetCodeIndexStats(3, 100)
	SetCodeIndexDirty(2)

	snap := SnapshotCodeSearch()
	if snap.SearchIndex < 1 || snap.SearchGrep < 1 || snap.SearchIndexing < 1 {
		t.Fatalf("search counters = %+v", snap)
	}
	if snap.IndexFull < 1 || snap.IndexIncr < 1 || snap.IndexOK < 1 || snap.IndexErr < 1 {
		t.Fatalf("index counters = %+v", snap)
	}
	if snap.IndexRuns < 2 || snap.IndexDurMs < 0 || snap.IndexRepos != 3 || snap.IndexDocs != 100 || snap.IndexDirty != 2 {
		t.Fatalf("index gauges = %+v", snap)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, name := range []string{
		"gitdash_code_search_requests_total",
		"gitdash_code_index_runs_total",
		"gitdash_code_index_mode_total",
		"gitdash_code_index_repos",
		"gitdash_code_index_documents",
	} {
		if !strings.Contains(body, name) {
			t.Errorf("/metrics missing %s", name)
		}
	}
	// 不得出现仓库维度标签（防私有信息泄漏）。
	if strings.Contains(body, "owner=") || strings.Contains(body, "repo=") {
		t.Errorf("code metrics must not carry repo/owner labels")
	}
}
