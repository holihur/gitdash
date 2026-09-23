package metrics

import "sync/atomic"

// 除 Prometheus 指标外的进程内原子快照，供管理端 JSON 接口读取
// （避免解析 Prometheus DTO）。仅聚合数值，不含任何仓库维度。
var (
	csSearchIndex    atomic.Int64
	csSearchGrep     atomic.Int64
	csSearchIndexing atomic.Int64

	csIndexFull atomic.Int64
	csIndexIncr atomic.Int64
	csIndexOK   atomic.Int64
	csIndexErr  atomic.Int64

	csIndexDurMs  atomic.Int64
	csIndexRuns   atomic.Int64
	csIndexRepos  atomic.Int64
	csIndexDocs   atomic.Int64
	csIndexDirty  atomic.Int64
	csLastIndexAt atomic.Int64 // Unix 秒；0=从未
)

// CodeSearchCounters 是代码搜索 / 索引聚合计数器快照。
type CodeSearchCounters struct {
	SearchIndex    int64 `json:"search_index"`    // 走索引的检索次数
	SearchGrep     int64 `json:"search_grep"`     // 显式 grep 模式的检索次数
	SearchIndexing int64 `json:"search_indexing"` // 命中“索引构建中”的检索次数

	IndexFull int64 `json:"index_full"` // 全量重建次数
	IndexIncr int64 `json:"index_incr"` // 增量重建次数
	IndexOK   int64 `json:"index_ok"`   // 成功次数
	IndexErr  int64 `json:"index_err"`  // 失败次数

	IndexDurMs  int64 `json:"index_dur_ms"`  // 重建总耗时（毫秒）
	IndexRuns   int64 `json:"index_runs"`    // 重建总次数（用于平均）
	IndexRepos  int64 `json:"index_repos"`   // 已索引仓库数
	IndexDocs   int64 `json:"index_docs"`    // 已索引文档数
	IndexDirty  int64 `json:"index_dirty"`   // 待重建仓库数
	LastIndexAt int64 `json:"last_index_at"` // 最近一次重建完成时间（Unix 秒）
}

// SnapshotCodeSearch 返回当前聚合计数器快照。
func SnapshotCodeSearch() CodeSearchCounters {
	return CodeSearchCounters{
		SearchIndex:    csSearchIndex.Load(),
		SearchGrep:     csSearchGrep.Load(),
		SearchIndexing: csSearchIndexing.Load(),
		IndexFull:      csIndexFull.Load(),
		IndexIncr:      csIndexIncr.Load(),
		IndexOK:        csIndexOK.Load(),
		IndexErr:       csIndexErr.Load(),
		IndexDurMs:     csIndexDurMs.Load(),
		IndexRuns:      csIndexRuns.Load(),
		IndexRepos:     csIndexRepos.Load(),
		IndexDocs:      csIndexDocs.Load(),
		IndexDirty:     csIndexDirty.Load(),
		LastIndexAt:    csLastIndexAt.Load(),
	}
}
