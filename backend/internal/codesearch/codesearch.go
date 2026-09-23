// Package codesearch 是代码搜索的抽象层，提供两种可互换的实现：
//
//   - Grep：实时调用 `git grep`，零索引、固定字符串匹配（默认）。
//   - Bleve：嵌入式全文索引（github.com/blevesearch/bleve/v2），
//     由异步任务在 push 后建立/更新，检索时无需为每个仓库启动 git 进程。
//
// 两种实现都只负责「单仓库」检索；跨仓库的候选筛选、并发、授权与截断仍由
// API 层负责，因此可以逐个仓库地切换实现而不改变响应结构。
//
// 索引实现（Bleve）只覆盖仓库的默认分支——与探索页全局代码搜索的范围一致。
// 对非默认分支的检索（单仓库搜索接口可显式传 ref）由 Service 回退到 Grep。
package codesearch

import "context"

// Hit 一条代码搜索命中（与 git grep 输出一致：文件、行号、该行文本）。
type Hit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// Options 单仓库搜索选项。字段语义与 gitsvc.SearchOpts 保持一致。
type Options struct {
	// Ref 分支/标签/commit；为空时使用仓库默认分支。
	Ref string `json:"ref,omitempty"`
	// Pathspec 额外的 git pathspec（如 "*.go"、"src/"）。
	Pathspec []string `json:"pathspec,omitempty"`
	// Word 为 true 时按单词边界匹配。
	Word bool `json:"word,omitempty"`
	// Max 返回条数上限（默认 50，最大 200）。
	Max int `json:"max,omitempty"`
	// Terms 关键词列表（AND 语义）；为空时 query 作为唯一关键词。
	Terms []string `json:"terms,omitempty"`
	// RepoID 是仓库在数据库中的稳定 ID。索引实现只在 meta 记录的 RepoID 与之
	// 相等时才采用索引：仓库被删除后以同名重建（ID 变化、可见性可能改变）时，
	// 索引会自动失效并回退到实时 grep，避免陈旧私有内容被当作新仓库返回。
	// 0 表示未知，索引一律不采用。
	RepoID int64 `json:"repo_id,omitempty"`
}

// Searcher 单仓库代码搜索的抽象。
type Searcher interface {
	Search(ctx context.Context, owner, name, query string, opts Options) ([]Hit, error)
}

// IndexStats 索引运行态快照（聚合，无仓库维度，供管理端展示）。
type IndexStats struct {
	Backend   string `json:"backend"` // grep | bleve | remote
	Repos     int    `json:"repos"`
	Documents int    `json:"documents"`
	Dirty     int    `json:"dirty"` // 待重建仓库数
}

// StatsReporter 报告索引运行态：*Service 返回本地索引快照，*Remote 代理到索引服务。
type StatsReporter interface {
	IndexStats(ctx context.Context) (IndexStats, error)
}

// Indexer 维护代码索引；仅基于索引的实现（Bleve）需要实现。
// jobs 包通过一个小接口消费它，避免与索引实现直接耦合。
type Indexer interface {
	// Index 为仓库默认分支重建索引；ref 为空时按 HEAD 解析。repoID 为仓库在
	// 数据库中的稳定 ID，写入 meta 用于识别“删除后同名重建”的陈旧索引。
	Index(ctx context.Context, owner, name string, repoID int64, ref string) error
	// Forget 移除仓库的全部索引数据（仓库删除时调用）。
	Forget(owner, name string)
	// NeedsIndex 报告仓库索引是否缺失、属于旧仓库（repoID 不符）或落后于 ref。
	NeedsIndex(owner, name string, repoID int64, ref string) bool
}
