package codesearch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gitdash/backend/internal/metrics"
)

// 搜索模式（GITDASH_CODE_SEARCH）。
const (
	// ModeGrep 实时 git grep，无索引（仅显式选择；不再作为自动回退）。
	ModeGrep = "grep"
	// ModeBleve 嵌入式 Bleve 索引，push 后异步（增量）更新。
	ModeBleve = "bleve"
	// DefaultMode 是未显式配置 GITDASH_CODE_SEARCH 时的默认后端。
	DefaultMode = ModeBleve
)

// Source 标识一次检索实际使用的后端。
type Source string

const (
	SourceIndex Source = "index"
	SourceGrep  Source = "grep"
)

// 索引未就绪时的信号（不再回退实时 grep，调用方据此提示/重试）。
var (
	// ErrIndexing 索引正在首次构建或 push 后重建中，本次未返回结果。
	ErrIndexing = errors.New("code index is being built")
	// ErrRefNotIndexed 请求的 ref 不在索引范围内（只索引默认分支）。
	ErrRefNotIndexed = errors.New("ref is not indexed")
)

// SourcedSearcher 是 Searcher 的可选扩展：额外报告本次检索的来源，供指标与
// 响应字段使用。仅 *Service 实现。
type SourcedSearcher interface {
	Searcher
	SearchSource(ctx context.Context, owner, name, query string, opts Options) ([]Hit, Source, error)
}

// Service 是索引优先的搜索实现：
//   - bleve 模式只走索引，索引未就绪时返回 ErrIndexing（**不**回退 git grep）；
//   - grep 仅在显式 GITDASH_CODE_SEARCH=grep 时作为唯一后端（与索引解耦，便于日后移除）。
//
// 索引是最终一致的：push 后到异步重建完成前的检索返回 ErrIndexing，由调用方提示/重试。
type Service struct {
	grep  Searcher // 仅显式 grep 模式；bleve 模式下为 nil
	bleve *Bleve
}

// NewService 按 mode 构造搜索服务。mode 为空时用 DefaultMode（bleve）；
// "grep" 时仅启用实时 git grep；"bleve" 打开 dir 下的嵌入式索引。
// 打开索引失败会返回可用的 grep-only 实现与错误（由调用方决定是否继续）。
func NewService(mode, dir string) (*Service, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = DefaultMode
	}
	switch mode {
	case ModeGrep, "off":
		return &Service{grep: NewGrep()}, nil
	case ModeBleve:
		b, err := OpenBleve(dir)
		if err != nil {
			// 索引打不开：不依赖 grep，返回无后端实现 + 错误供上层记录（不阻断启动）。
			return &Service{}, err
		}
		b.publishStats()
		return &Service{bleve: b}, nil
	default:
		return &Service{grep: NewGrep()}, fmt.Errorf("unknown code search mode %q", mode)
	}
}

// Search 实现 Searcher。
func (s *Service) Search(ctx context.Context, owner, name, query string, opts Options) ([]Hit, error) {
	hits, _, err := s.SearchSource(ctx, owner, name, query, opts)
	return hits, err
}

// SearchSource 实现 SourcedSearcher。bleve 模式不吞索引错误、不回退 grep：
// 未就绪返回 ErrIndexing，ref 超出范围返回 ErrRefNotIndexed。
func (s *Service) SearchSource(ctx context.Context, owner, name, query string, opts Options) ([]Hit, Source, error) {
	if s.bleve != nil {
		hits, err := s.searchIndex(ctx, owner, name, query, opts)
		if err != nil {
			if errors.Is(err, ErrIndexing) {
				metrics.ObserveCodeSearchIndexing()
			}
			return nil, SourceIndex, err
		}
		metrics.ObserveCodeSearch(string(SourceIndex))
		return hits, SourceIndex, nil
	}
	if s.grep != nil {
		hits, err := s.grep.Search(ctx, owner, name, query, opts)
		if err != nil {
			return nil, SourceGrep, err
		}
		metrics.ObserveCodeSearch(string(SourceGrep))
		return hits, SourceGrep, nil
	}
	return nil, "", errors.New("codesearch: no backend configured (index unavailable)")
}

func (s *Service) searchIndex(ctx context.Context, owner, name, query string, opts Options) ([]Hit, error) {
	b := s.bleve
	// ref 超出索引范围（仅默认分支）：明确报错，不静默返回空。
	if ref, ok := b.IndexedRef(owner, name, opts.RepoID); ok && ref != "" && opts.Ref != "" && ref != opts.Ref {
		return nil, ErrRefNotIndexed
	}
	// 最终一致：索引未就绪（首次构建 / push 后重建中）返回 ErrIndexing，不回退 grep。
	if !b.Ready(owner, name, opts.Ref, opts.RepoID) {
		return nil, ErrIndexing
	}
	return b.Search(ctx, owner, name, query, opts)
}

// Index 实现 Indexer（索引关闭时为 no-op）。
func (s *Service) Index(ctx context.Context, owner, name string, repoID int64, ref string) error {
	if s.bleve == nil {
		return nil
	}
	dur := metrics.ObserveCodeIndexStart()
	err := s.bleve.Index(ctx, owner, name, repoID, ref)
	metrics.ObserveCodeIndexDone(dur, err)
	return err
}

// Merge 触发一次强制段合并（索引关闭时为 no-op）。
func (s *Service) Merge(ctx context.Context) {
	if s.bleve != nil {
		s.bleve.Merge(ctx)
	}
}

// Forget 实现 Indexer（索引关闭时为 no-op）。
func (s *Service) Forget(owner, name string) {
	if s.bleve != nil {
		s.bleve.Forget(owner, name)
	}
}

// NeedsIndex 实现 Indexer（索引关闭时恒为 false）。
func (s *Service) NeedsIndex(owner, name string, repoID int64, ref string) bool {
	return s.bleve != nil && s.bleve.NeedsIndex(owner, name, repoID, ref)
}

// IndexingEnabled 报告是否启用了索引（用于决定是否入队索引任务）。
func (s *Service) IndexingEnabled() bool { return s.bleve != nil }

// IndexStats 实现 StatsReporter：返回本地索引运行态（grep 模式为 backend=grep）。
func (s *Service) IndexStats(_ context.Context) (IndexStats, error) {
	if s.bleve == nil {
		if s.grep == nil {
			return IndexStats{Backend: "none"}, nil
		}
		return IndexStats{Backend: ModeGrep}, nil
	}
	return s.bleve.stats(), nil
}

// KnownRepos 返回已建立索引的仓库键（owner/name）；索引关闭时返回 nil。
func (s *Service) KnownRepos() []string {
	if s.bleve == nil {
		return nil
	}
	return s.bleve.KnownRepos()
}

// Close 关闭底层索引（若启用）。
func (s *Service) Close() error {
	if s.bleve != nil {
		return s.bleve.Close()
	}
	return nil
}
