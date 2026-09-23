package codesearch

import (
	"context"

	"gitdash/backend/internal/gitsvc"
)

// Grep 使用实时 `git grep` 的实现：无索引、无外部依赖，始终可用。
type Grep struct{}

// NewGrep 返回默认的实时 grep 搜索器。
func NewGrep() *Grep { return &Grep{} }

// Search 实现 Searcher，转调 gitsvc.SearchWith。
func (g *Grep) Search(ctx context.Context, owner, name, query string, opts Options) ([]Hit, error) {
	hits, err := gitsvc.SearchWith(ctx, owner, name, query, gitsvc.SearchOpts{
		Ref:      opts.Ref,
		Pathspec: opts.Pathspec,
		Word:     opts.Word,
		Max:      opts.Max,
		Terms:    opts.Terms,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Hit, len(hits))
	for i, h := range hits {
		out[i] = Hit{Path: h.Path, Line: h.Line, Text: h.Text}
	}
	return out, nil
}
