package api

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"gitdash/backend/internal/codesearch"
	"gitdash/backend/internal/store"
)

// 全局代码搜索的规模上限（MVP 用 git grep，避免无界扫描）。
const (
	codeSearchMaxRepos   = 200
	codeSearchWorkers    = 8
	codeSearchPerRepo    = 50
	codeSearchMaxResults = 200
	codeSearchTimeout    = 20 * time.Second
)

// codeQuery 解析后的搜索条件。
type codeQuery struct {
	Keyword string // 关键词（symbol 存在时即 symbol）
	Repo    string // repo:owner/name 或 repo:name
	Lang    string // lang:go
	Path    string // path:src/
	Symbol  string // symbol:Foo（按单词边界匹配）
}

// langExt 常见语言的扩展名 pathspec。
var langExt = map[string][]string{
	"go":     {"*.go"},
	"ts":     {"*.ts", "*.tsx"},
	"tsx":    {"*.tsx"},
	"js":     {"*.js", "*.jsx", "*.mjs", "*.cjs"},
	"jsx":    {"*.jsx"},
	"py":     {"*.py"},
	"rs":     {"*.rs"},
	"rust":   {"*.rs"},
	"java":   {"*.java"},
	"rb":     {"*.rb"},
	"ruby":   {"*.rb"},
	"c":      {"*.c", "*.h"},
	"cpp":    {"*.cpp", "*.cc", "*.cxx", "*.hpp", "*.hh"},
	"csharp": {"*.cs"},
	"cs":     {"*.cs"},
	"php":    {"*.php"},
	"sh":     {"*.sh", "*.bash"},
	"yaml":   {"*.yml", "*.yaml"},
	"yml":    {"*.yml", "*.yaml"},
	"json":   {"*.json"},
	"md":     {"*.md", "*.markdown"},
	"html":   {"*.html"},
	"css":    {"*.css"},
	"sql":    {"*.sql"},
	"kt":     {"*.kt", "*.kts"},
	"swift":  {"*.swift"},
	"toml":   {"*.toml"},
}

// parseCodeQuery 解析内联限定符（repo:/lang:/path:/symbol:），其余 token 组成关键词。
func parseCodeQuery(raw string) codeQuery {
	var q codeQuery
	var words []string
	for _, tok := range strings.Fields(raw) {
		key, val, ok := strings.Cut(tok, ":")
		if !ok || val == "" {
			words = append(words, tok)
			continue
		}
		switch strings.ToLower(key) {
		case "repo":
			q.Repo = val
		case "lang", "language":
			q.Lang = val
		case "path":
			q.Path = val
		case "symbol":
			q.Symbol = val
		default:
			words = append(words, tok)
		}
	}
	q.Keyword = strings.Join(words, " ")
	if q.Symbol != "" {
		q.Keyword = q.Symbol
	}
	return q
}

// langPathspecs 把 lang: 映射为 git pathspec；未知语言按 `*.<lang>` 处理。
func langPathspecs(lang string) []string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	if lang == "" {
		return nil
	}
	if exts, ok := langExt[lang]; ok {
		return exts
	}
	return []string{"*." + lang}
}

// buildPathspecs 组合 lang 与 path 限定为 git pathspec。两者同时存在时做交集：
// 例如 lang=go + path=src 得到 `src/*.go`（git 通配符可跨目录）。
func buildPathspecs(lang, path string) []string {
	prefix := strings.Trim(strings.TrimSpace(path), "/")
	exts := langPathspecs(lang)
	switch {
	case prefix == "":
		return exts
	case len(exts) == 0:
		return []string{prefix}
	}
	out := make([]string, 0, len(exts))
	for _, e := range exts {
		out = append(out, prefix+"/*"+strings.TrimPrefix(e, "*"))
	}
	return out
}

// codeSearchHit 全局代码搜索的一条命中。
type codeSearchHit struct {
	Owner string `json:"owner"`
	Repo  string `json:"repo"`
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Text  string `json:"text"`
}

// codeSearchCandidates 解析候选仓库：
// repo 限定符带 owner 时直接取该仓库并校验读权限；否则从可访问 + 公开仓库列表过滤。
// 返回 (候选仓库, 是否因上限被截断, 错误)。
func (a *API) codeSearchCandidates(me, repoQual string, max int) ([]store.Repo, bool, error) {
	if repoQual != "" {
		if owner, name, ok := strings.Cut(repoQual, "/"); ok && owner != "" && name != "" {
			rp, _ := a.store.GetRepo(owner, name)
			if rp.ID == 0 {
				return []store.Repo{}, false, nil // 不存在（或不可见）→ 无候选，不算错误
			}
			if rp.Banned || a.store.IsOrgBanned(owner) || (rp.Private && !a.store.CanRead(owner, name, me)) {
				return []store.Repo{}, false, nil
			}
			return []store.Repo{rp}, false, nil
		}
	}
	all, err := a.store.CodeSearchRepos(me, max+1)
	if err != nil {
		return nil, false, err
	}
	truncated := false
	if len(all) > max {
		all = all[:max]
		truncated = true
	}
	if repoQual == "" {
		return all, truncated, nil
	}
	// repo:name（无 owner）：在候选列表中按名字过滤
	out := make([]store.Repo, 0, len(all))
	for _, rp := range all {
		if rp.Name == repoQual {
			out = append(out, rp)
		}
	}
	return out, truncated, nil
}

// searchCode 全局代码搜索（跨仓库）。
//
//	@Summary     全局代码搜索
//	@Description 在当前用户可访问的仓库 + 公开仓库上做固定字符串搜索。
//	@Description q 支持内联限定符 repo:owner/name、lang:go、path:src/、symbol:Foo。
//	@Tags        search
//	@Produce     json
//	@Param       q      query string true  "关键词与内联限定符"
//	@Param       repo   query string false "仓库限定（owner/name 或 name）"
//	@Param       lang   query string false "语言限定"
//	@Param       path   query string false "路径前缀限定"
//	@Param       symbol query string false "符号（单词边界匹配）"
//	@Success     200 {object} object "results / truncated / repos_searched"
//	@Failure     400 {object} map[string]string
//	@Security    BearerAuth
//	@Router      /search/code [get]
func (a *API) searchCode(w http.ResponseWriter, r *http.Request) {
	me := userFrom(r)
	q := parseCodeQuery(r.URL.Query().Get("q"))
	if v := strings.TrimSpace(r.URL.Query().Get("repo")); v != "" {
		q.Repo = v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("lang")); v != "" {
		q.Lang = v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("path")); v != "" {
		q.Path = v
	}
	if v := strings.TrimSpace(r.URL.Query().Get("symbol")); v != "" {
		q.Symbol = v
		q.Keyword = v
	}
	if q.Keyword == "" {
		writeCode(w, http.StatusBadRequest, "query_required", "query parameter q is required")
		return
	}
	if tooLong(w, "q", q.Keyword, maxTitleRunes) {
		return
	}
	candidates, truncated, err := a.codeSearchCandidates(me, q.Repo, codeSearchMaxRepos)
	if err != nil {
		internalError(w, err)
		return
	}
	pathspecs := buildPathspecs(q.Lang, q.Path)
	word := q.Symbol != ""
	// 空格分隔的多个关键词按 AND 语义：命中行需同时包含全部关键词。
	terms := strings.Fields(q.Keyword)

	ctx, cancel := context.WithTimeout(r.Context(), codeSearchTimeout)
	defer cancel()

	var (
		mu            sync.Mutex
		wg            sync.WaitGroup
		results       = make([]codeSearchHit, 0, codeSearchMaxResults)
		searched      int
		indexedRepos  int
		grepRepos     int
		indexingRepos int
		limitHit      bool
	)
	sem := make(chan struct{}, codeSearchWorkers)
loop:
	for _, rp := range candidates {
		mu.Lock()
		done := len(results) >= codeSearchMaxResults
		mu.Unlock()
		if done {
			mu.Lock()
			limitHit = true
			mu.Unlock()
			break
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			mu.Lock()
			limitHit = true
			mu.Unlock()
			break loop
		}
		wg.Add(1)
		go func(rp store.Repo) {
			defer wg.Done()
			defer func() { <-sem }()
			// Ref 留空 = 默认分支（以索引的 ref 为准）；索引只覆盖默认分支，
			// 这样也能兼容 DB 默认分支名与实际 HEAD 不一致的仓库（如导入仓库）。
			opts := codesearch.Options{
				Ref: "", Pathspec: pathspecs, Word: word,
				Max: codeSearchPerRepo, Terms: terms, RepoID: rp.ID,
			}
			hits, src, err := a.searchOne(ctx, rp.Owner, rp.Name, q.Keyword, opts)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				// 索引构建中（最终一致）：该仓库本次无结果，标记 indexing。
				if errors.Is(err, codesearch.ErrIndexing) {
					searched++
					indexingRepos++
				}
				return
			}
			searched++
			if src == codesearch.SourceIndex {
				indexedRepos++
			} else {
				grepRepos++
			}
			for _, h := range hits {
				if len(results) >= codeSearchMaxResults {
					limitHit = true
					break
				}
				results = append(results, codeSearchHit{
					Owner: rp.Owner, Repo: rp.Name, Path: h.Path, Line: h.Line, Text: h.Text,
				})
			}
		}(rp)
	}
	wg.Wait()

	sort.Slice(results, func(i, j int) bool {
		if results[i].Owner != results[j].Owner {
			return results[i].Owner < results[j].Owner
		}
		if results[i].Repo != results[j].Repo {
			return results[i].Repo < results[j].Repo
		}
		if results[i].Path != results[j].Path {
			return results[i].Path < results[j].Path
		}
		return results[i].Line < results[j].Line
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"results":        results,
		"truncated":      truncated || limitHit,
		"repos_searched": searched,
		"indexed_repos":  indexedRepos,
		"grep_repos":     grepRepos,
		"indexing":       indexingRepos > 0,
		"indexing_repos": indexingRepos,
	})
}
