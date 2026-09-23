package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// 代码搜索 / 索引指标。刻意只使用聚合标签（source、result），
// 不含 owner/repo，避免标签高基数与私有仓库名泄漏到 /metrics。
var (
	codeSearchRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gitdash_code_search_requests_total",
			Help: "Code search executions by backend (index or grep)",
		},
		[]string{"source"},
	)
	codeSearchIndexing = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "gitdash_code_search_indexing_total",
			Help: "Code search requests that hit a repo whose index was still being built",
		},
	)
	codeIndexRuns = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gitdash_code_index_runs_total",
			Help: "Code index rebuild runs by result",
		},
		[]string{"result"},
	)
	codeIndexMode = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gitdash_code_index_mode_total",
			Help: "Code index rebuilds by mode (full or incremental)",
		},
		[]string{"mode"},
	)
	codeIndexDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "gitdash_code_index_duration_seconds",
			Help:    "Code index rebuild duration in seconds",
			Buckets: prometheus.DefBuckets,
		},
	)
	codeIndexRepos = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gitdash_code_index_repos",
			Help: "Number of repositories currently indexed",
		},
	)
	codeIndexDocuments = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "gitdash_code_index_documents",
			Help: "Number of line documents currently in the code index",
		},
	)
)

func init() {
	registry.MustRegister(
		codeSearchRequests,
		codeSearchIndexing,
		codeIndexRuns,
		codeIndexMode,
		codeIndexDuration,
		codeIndexRepos,
		codeIndexDocuments,
	)
}

// ObserveCodeSearch 记录一次代码检索及其后端来源。
func ObserveCodeSearch(source string) {
	codeSearchRequests.WithLabelValues(source).Inc()
	if source == "grep" {
		csSearchGrep.Add(1)
	} else {
		csSearchIndex.Add(1)
	}
}

// ObserveCodeIndexStart 返回开始时间，配合 ObserveCodeIndexDone 记录一次索引重建。
func ObserveCodeIndexStart() time.Time { return time.Now() }

// ObserveCodeIndexDone 记录一次索引重建的耗时与结果。
func ObserveCodeIndexDone(start time.Time, err error) {
	d := time.Since(start)
	codeIndexDuration.Observe(d.Seconds())
	result := "ok"
	if err != nil {
		result = "error"
		csIndexErr.Add(1)
	} else {
		csIndexOK.Add(1)
	}
	codeIndexRuns.WithLabelValues(result).Inc()
	csIndexDurMs.Add(d.Milliseconds())
	csIndexRuns.Add(1)
	csLastIndexAt.Store(time.Now().Unix())
}

// ObserveCodeSearchIndexing 记录一次检索遇到「索引构建中」的仓库。
func ObserveCodeSearchIndexing() {
	codeSearchIndexing.Inc()
	csSearchIndexing.Add(1)
}

// ObserveCodeIndexMode 记录一次索引重建是全量还是增量。
func ObserveCodeIndexMode(full bool) {
	mode := "incremental"
	if full {
		mode = "full"
		csIndexFull.Add(1)
	} else {
		csIndexIncr.Add(1)
	}
	codeIndexMode.WithLabelValues(mode).Inc()
}

// SetCodeIndexStats 更新索引规模聚合指标（仓库数、文档数）。
func SetCodeIndexStats(repos, docs int) {
	codeIndexRepos.Set(float64(repos))
	codeIndexDocuments.Set(float64(docs))
	csIndexRepos.Store(int64(repos))
	csIndexDocs.Store(int64(docs))
}

// SetCodeIndexDirty 更新待重建仓库数。
func SetCodeIndexDirty(n int) {
	csIndexDirty.Store(int64(n))
}
