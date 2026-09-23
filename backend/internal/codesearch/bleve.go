package codesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/custom"
	"github.com/blevesearch/bleve/v2/analysis/analyzer/keyword"
	tokenfiltercamelcase "github.com/blevesearch/bleve/v2/analysis/token/camelcase"
	"github.com/blevesearch/bleve/v2/analysis/token/lowercase"
	_ "github.com/blevesearch/bleve/v2/analysis/tokenizer/unicode" // 注册 unicode 分词器（CJK 支持）
	"github.com/blevesearch/bleve/v2/index/scorch"
	"github.com/blevesearch/bleve/v2/mapping"
	"github.com/blevesearch/bleve/v2/search/query"

	"gitdash/backend/internal/gitsvc"
	"gitdash/backend/internal/logx"
	"gitdash/backend/internal/metrics"
)

const (
	// IndexSubDir 索引在数据目录下的子目录名。
	IndexSubDir = "code-index"
	// indexName Bleve 索引目录名。
	indexName = "code.bleve"
	// codeAnalyzer 面向代码的自定义分析器：按标识符拆分（camelCase/下划线/点号）+ 小写。
	codeAnalyzer = "gitdash_code"
	// codeAnalyzerFallback 自定义分析器注册失败时的回退：unicode 分词 + 小写。
	codeAnalyzerFallback = "gitdash_code_fb"
	// codeCamelFilter 代码分析器使用的 camelCase 过滤器名。
	codeCamelFilter = "gitdash_camel"
	// maxIndexFileBytes 单个文件的最大索引体积，超过则跳过（ls-tree 阶段已过滤）。
	maxIndexFileBytes = 1 << 20 // 1 MiB
	// maxIndexLineBytes 单行存储上限，超长（压缩产物等）截断。
	maxIndexLineBytes = 1000
	// maxIndexDocsPerRepo 单仓库文档数上限，防止超大仓库撑爆索引。
	maxIndexDocsPerRepo = 100000
	// maxIndexFileLines 单文件索引行数上限（同时用于删除旧文档时的枚举上限）。
	maxIndexFileLines = 20000
	// indexBatchSize Bleve 批量写入大小。
	indexBatchSize = 500
)

// bleveDoc 索引中的一行代码。
type bleveDoc struct {
	Owner string `json:"owner"`
	Name  string `json:"name"`
	Repo  string `json:"repo"`
	Path  string `json:"path"`
	Line  int    `json:"line"`
	Text  string `json:"text"`
}

// metaEntry 记录某仓库已索引的 ref 与提交，用于就绪判断与增量回填。
// RepoID 是数据库仓库 ID：同名仓库被删除后重建时 ID 会变，据此判定索引已陈旧（安全）。
type metaEntry struct {
	RepoID int64  `json:"repo_id"`
	Ref    string `json:"ref"`
	SHA    string `json:"sha"`
	Docs   int    `json:"docs"`
	At     string `json:"at"`
}

// Bleve 基于嵌入式 Bleve 索引的代码搜索实现（全局单索引，按 repo 字段过滤）。
// 只索引仓库默认分支。
type Bleve struct {
	dir      string
	index    bleve.Index
	analyzer string // text 字段使用的分析器名（查询时复用）

	mu   sync.Mutex // 保护 meta
	meta map[string]metaEntry

	write sync.Mutex // 串行化索引写入（删除 + 重建）
}

// OpenBleve 打开（不存在则创建）指定目录下的嵌入式索引。
func OpenBleve(dir string) (*Bleve, error) {
	// 索引含全部仓库（含私有）的代码文本，目录收紧到 0700 防止本机其它用户读取。
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	idxPath := filepath.Join(dir, indexName)
	idx, err := bleve.Open(idxPath)
	if err == bleve.ErrorIndexPathDoesNotExist {
		idx, err = bleve.New(idxPath, newIndexMapping())
	}
	if err != nil {
		return nil, fmt.Errorf("open bleve index: %w", err)
	}
	analyzer := codeAnalyzer
	if im, ok := idx.Mapping().(*mapping.IndexMappingImpl); ok && im.DefaultAnalyzer != "" {
		analyzer = im.DefaultAnalyzer
	}
	b := &Bleve{dir: dir, index: idx, analyzer: analyzer, meta: map[string]metaEntry{}}
	b.loadMeta()
	return b, nil
}

func newIndexMapping() mapping.IndexMapping {
	m := bleve.NewIndexMapping()
	// 代码标识符分析器：按非字母数字切片 + 拆 camelCase + 转小写（无停用词）。
	// 例如 uniqueAlphaToken -> unique / alpha / token；foo_bar -> foo / bar；fmt.Println -> fmt / println。
	textAnalyzer := codeAnalyzer
	if err := registerCodeAnalyzer(m); err != nil {
		// 自定义分析器注册失败时退回 unicode 分词 + 小写（不拆标识符），保证索引可用。
		logx.Infof("codesearch: analyzer setup: %v; falling back to unicode+lower", err)
		textAnalyzer = codeAnalyzerFallback
		if ferr := m.AddCustomAnalyzer(codeAnalyzerFallback, map[string]interface{}{
			"type":          custom.Name,
			"tokenizer":     "unicode",
			"token_filters": []string{"to_lower"},
		}); ferr != nil {
			logx.Infof("codesearch: fallback analyzer: %v", ferr)
		}
	}
	m.DefaultAnalyzer = textAnalyzer

	doc := bleve.NewDocumentMapping()
	kw := bleve.NewTextFieldMapping()
	kw.Analyzer = keyword.Name
	kw.Store = true
	kw.IncludeTermVectors = false
	kw.IncludeInAll = false
	for _, f := range []string{"owner", "name", "repo", "path"} {
		doc.AddFieldMappingsAt(f, kw)
	}
	text := bleve.NewTextFieldMapping()
	text.Analyzer = textAnalyzer
	text.Store = true
	doc.AddFieldMappingsAt("text", text)
	line := bleve.NewNumericFieldMapping()
	line.Store = true
	doc.AddFieldMappingsAt("line", line)
	m.DefaultMapping = doc
	return m
}

// registerCodeAnalyzer 在 mapping 里定义代码分析器：unicode 分词（CJK 友好）
// + camelCase 拆分（uniqueAlphaToken -> unique/Alpha/Token）+ 小写。
func registerCodeAnalyzer(m *mapping.IndexMappingImpl) error {
	if err := m.AddCustomTokenFilter(codeCamelFilter, map[string]interface{}{
		"type": tokenfiltercamelcase.Name,
	}); err != nil {
		return err
	}
	return m.AddCustomAnalyzer(codeAnalyzer, map[string]interface{}{
		"type":          custom.Name,
		"tokenizer":     "unicode",
		"token_filters": []string{codeCamelFilter, lowercase.Name},
	})
}

// Close 关闭索引。
func (b *Bleve) Close() error { return b.index.Close() }

// repoKey owner/name 组合键（名字不含 `/`，无歧义）。
func repoKey(owner, name string) string { return owner + "/" + name }

// validRepoKey 校验 owner/name 可用于索引键（非空且不含分隔符）。
func validRepoKey(owner, name string) bool {
	return owner != "" && name != "" &&
		!strings.Contains(owner, "/") && !strings.Contains(name, "/")
}

func docID(repo, file string, line int) string {
	return fmt.Sprintf("%s\x00%s\x00%d", repo, file, line)
}

// Ready 报告仓库是否已建索引。要求索引归属当前仓库（repoID 相符，防止同名重建
// 后的旧索引泄漏）且 ref 一致（ref 为空表示默认分支，索引即默认分支）。
func (b *Bleve) Ready(owner, name, ref string, repoID int64) bool {
	if repoID == 0 || !validRepoKey(owner, name) {
		return false
	}
	// 默认分支有新 push 且尚未重建完成：回退 grep，保证新内容立即可搜。
	if b.isDirty(owner, name) {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.meta[repoKey(owner, name)]
	if !ok || e.RepoID != repoID {
		return false
	}
	return ref == "" || e.Ref == ref
}

// resolveIndexRef 解析索引目标 ref 及其提交：优先用给定 ref，解析失败（不存在）时
// 回退到实际 HEAD，兼容数据库默认分支与实际 HEAD 不一致的仓库（如导入仓库）。
func resolveIndexRef(owner, name, ref string) (string, string) {
	if ref != "" {
		if sha, err := gitsvc.RevSHA(owner, name, ref); err == nil && sha != "" {
			return ref, sha
		}
	}
	hb, err := gitsvc.HeadBranch(owner, name)
	if err != nil || hb == "" {
		return ref, ""
	}
	sha, _ := gitsvc.RevSHA(owner, name, hb)
	return hb, sha
}

// IndexedRef 返回仓库已索引的 ref（仅当 meta 存在且 repoID 相符）。
// 供 Service 判断请求的 ref 是否在索引范围内（只索引默认分支）。
func (b *Bleve) IndexedRef(owner, name string, repoID int64) (string, bool) {
	if !validRepoKey(owner, name) {
		return "", false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	e, ok := b.meta[repoKey(owner, name)]
	if !ok || e.RepoID != repoID {
		return "", false
	}
	return e.Ref, true
}

// NeedsIndex 报告仓库索引是否缺失、属于旧仓库（repoID 不符）或落后于当前提交。
func (b *Bleve) NeedsIndex(owner, name string, repoID int64, ref string) bool {
	if !validRepoKey(owner, name) {
		return false
	}
	b.mu.Lock()
	e, ok := b.meta[repoKey(owner, name)]
	b.mu.Unlock()
	if !ok || e.RepoID != repoID {
		return true // 从未索引，或仓库已重建（同名不同 ID）
	}
	resolved, sha := resolveIndexRef(owner, name, ref)
	if resolved == "" || e.Ref != resolved {
		return true
	}
	return sha == "" || e.SHA != sha
}

// Index 为仓库的默认分支（ref 为空时按 HEAD 解析）增量重建索引：
// 对比上次的 blob SHA 清单，只重读变更/新增文件，并删除变更/移除文件的旧文档；
// 仓库首次索引、清单缺失或仓库 ID 变化（同名重建）时退化为全量重建。
func (b *Bleve) Index(ctx context.Context, owner, name string, repoID int64, ref string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validRepoKey(owner, name) {
		return fmt.Errorf("invalid repo %q/%q", owner, name)
	}
	ref, sha := resolveIndexRef(owner, name, ref)
	if ref == "" {
		return fmt.Errorf("resolve default branch for %s/%s", owner, name)
	}
	files, err := gitsvc.ListTreeFiles(owner, name, ref)
	if err != nil {
		return err
	}
	repo := repoKey(owner, name)

	// 期望清单：path -> blob SHA（仅含可索引文件）。
	want := make(map[string]string, len(files))
	order := make([]gitsvc.TreeFile, 0, len(files))
	for _, f := range files {
		if f.Size <= 0 || f.Size > maxIndexFileBytes || skipIndexPath(f.Path) {
			continue
		}
		want[f.Path] = f.SHA
		order = append(order, f)
	}

	// 读取旧 meta 与清单，判断全量/增量；repoID 变化（同名重建）必须全量。
	b.mu.Lock()
	oldMeta, hadMeta := b.meta[repo]
	b.mu.Unlock()
	old := b.loadManifest(owner, name)
	full := !hadMeta || oldMeta.RepoID != repoID || old == nil

	b.write.Lock()
	defer b.write.Unlock()
	// 重建期间先标记为未就绪：并发检索会回退到 grep，避免读到部分的索引。
	b.clearMeta(repo)
	if full {
		if err := b.deleteRepoDocs(ctx, repo); err != nil {
			return err
		}
	} else {
		// 删除变更/移除文件的旧文档（未变化文件保留）。
		for p, oldSHA := range old {
			if newSHA, ok := want[p]; ok && newSHA == oldSHA {
				continue
			}
			if err := b.deleteFileDocs(ctx, repo, p); err != nil {
				return err
			}
		}
	}

	// 需要重建的文件：全量 = 全部；增量 = 新增或 SHA 变化。
	changed := make([]gitsvc.TreeFile, 0, len(order))
	for _, f := range order {
		if !full && old[f.Path] == f.SHA {
			continue
		}
		changed = append(changed, f)
	}
	docs, err := b.indexFiles(ctx, owner, name, ref, repo, changed)
	if err != nil {
		return err
	}
	if err := b.saveManifest(owner, name, want); err != nil {
		return err
	}
	mode := "incremental"
	if full {
		mode = "full"
	}
	metrics.ObserveCodeIndexMode(full)
	logx.Infof("codesearch: indexed %s/%s@%s (%s, %d/%d files changed, %d docs)",
		owner, name, ref, mode, len(changed), len(order), docs)
	b.setMeta(repo, metaEntry{RepoID: repoID, Ref: ref, SHA: sha, Docs: docs, At: time.Now().UTC().Format(time.RFC3339)})
	// 仅当 HEAD 仍等于本次索引的提交时才清除「待重建」标记；否则说明索引期间
	// 又有 push，保留标记让新任务重建，避免用旧内容冒充就绪（否则会返回空结果
	// 且不报 indexing）。注意 resolveIndexRef 返回 (ref, sha)，要比的是 sha。
	if _, curSHA := resolveIndexRef(owner, name, ""); curSHA == sha {
		b.clearDirty(owner, name)
	}
	b.publishStats()
	return nil
}

// indexFiles 读取并索引给定文件，返回本次写入的文档数。
func (b *Bleve) indexFiles(ctx context.Context, owner, name, ref, repo string, files []gitsvc.TreeFile) (int, error) {
	paths := make([]string, 0, len(files))
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	batch := b.index.NewBatch()
	docs := 0
	flush := func() error {
		if batch.Size() == 0 {
			return nil
		}
		if err := b.index.Batch(batch); err != nil {
			return err
		}
		batch = b.index.NewBatch()
		return nil
	}
	const chunk = 256
	for start := 0; start < len(paths) && docs < maxIndexDocsPerRepo; start += chunk {
		if err := ctx.Err(); err != nil {
			return docs, err
		}
		end := start + chunk
		if end > len(paths) {
			end = len(paths)
		}
		contents, rerr := gitsvc.ReadBlobsBatch(owner, name, ref, paths[start:end], maxIndexFileBytes)
		if rerr != nil {
			return docs, rerr
		}
		for _, p := range paths[start:end] {
			data, ok := contents[p]
			if !ok || bytes.IndexByte(data, 0) >= 0 { // 缺失或二进制
				continue
			}
			lines := splitIndexLines(string(data))
			if len(lines) > maxIndexFileLines {
				lines = lines[:maxIndexFileLines]
			}
			for i, ln := range lines {
				if strings.TrimSpace(ln) == "" {
					continue
				}
				doc := bleveDoc{Owner: owner, Name: name, Repo: repo, Path: p, Line: i + 1, Text: truncateIndexText(ln)}
				if err := batch.Index(docID(repo, p, i+1), doc); err != nil {
					return docs, err
				}
				docs++
				if batch.Size() >= indexBatchSize {
					if err := flush(); err != nil {
						return docs, err
					}
				}
				if docs >= maxIndexDocsPerRepo {
					break
				}
			}
			if docs >= maxIndexDocsPerRepo {
				break
			}
		}
	}
	if err := flush(); err != nil {
		return docs, err
	}
	return docs, nil
}

// deleteFileDocs 删除某文件在索引中的全部行文档（repo + path 精确匹配）。
func (b *Bleve) deleteFileDocs(ctx context.Context, repo, file string) error {
	rq := bleve.NewTermQuery(repo)
	rq.SetField("repo")
	pq := bleve.NewTermQuery(file)
	pq.SetField("path")
	q := bleve.NewConjunctionQuery(rq, pq)
	res, err := b.index.SearchInContext(ctx, bleve.NewSearchRequestOptions(q, maxIndexFileLines+1, 0, false))
	if err != nil {
		return err
	}
	if len(res.Hits) == 0 {
		return nil
	}
	batch := b.index.NewBatch()
	for _, h := range res.Hits {
		batch.Delete(h.ID)
		if batch.Size() >= indexBatchSize {
			if err := b.index.Batch(batch); err != nil {
				return err
			}
			batch = b.index.NewBatch()
		}
	}
	if batch.Size() > 0 {
		return b.index.Batch(batch)
	}
	return nil
}

// Merge 触发一次强制段合并（scorch 内部也会自动合并，这里用于重建后收敛段数）。
// 不同的 bleve 索引类型可能不支持，此时静默跳过。
func (b *Bleve) Merge(ctx context.Context) {
	adv, err := b.index.Advanced()
	if err != nil {
		return
	}
	if sc, ok := adv.(*scorch.Scorch); ok {
		if err := sc.ForceMerge(ctx, nil); err != nil {
			logx.Infof("codesearch: force merge: %v", err)
		}
	}
}

// publishStats 上报聚合指标（不含仓库名，避免标签高基数与私有信息泄漏）。
func (b *Bleve) publishStats() {
	docs, _ := b.index.DocCount()
	b.mu.Lock()
	repos := len(b.meta)
	b.mu.Unlock()
	metrics.SetCodeIndexStats(repos, int(docs))
	metrics.SetCodeIndexDirty(b.dirtyCount())
}

// stats 返回索引运行态快照。
func (b *Bleve) stats() IndexStats {
	docs, _ := b.index.DocCount()
	b.mu.Lock()
	repos := len(b.meta)
	b.mu.Unlock()
	return IndexStats{Backend: ModeBleve, Repos: repos, Documents: int(docs), Dirty: b.dirtyCount()}
}

// dirtyCount 统计待重建标记数。
func (b *Bleve) dirtyCount() int {
	entries, err := os.ReadDir(filepath.Join(b.dir, dirtySubDir))
	if err != nil {
		return 0
	}
	return len(entries)
}

// KnownRepos 返回已建立索引的仓库键（owner/name），供周期性与数据库对账、
// 清理已删除仓库的陈旧索引。
func (b *Bleve) KnownRepos() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]string, 0, len(b.meta))
	for k := range b.meta {
		out = append(out, k)
	}
	return out
}

// Forget 移除仓库的全部索引数据与元信息。
func (b *Bleve) Forget(owner, name string) {
	if !validRepoKey(owner, name) {
		return
	}
	repo := repoKey(owner, name)
	b.write.Lock()
	defer b.write.Unlock()
	if err := b.deleteRepoDocs(context.Background(), repo); err != nil {
		logx.Infof("codesearch: delete index for %s: %v", repo, err)
	}
	b.deleteManifest(owner, name)
	b.clearDirty(owner, name)
	b.mu.Lock()
	delete(b.meta, repo)
	b.mu.Unlock()
	if err := b.saveMeta(); err != nil {
		logx.Infof("codesearch: save meta after forget %s: %v", repo, err)
	}
	b.publishStats()
}

// deleteRepoDocs 删除某仓库的全部文档（无 DeleteByQuery，先枚举 ID 再批量删）。
func (b *Bleve) deleteRepoDocs(ctx context.Context, repo string) error {
	tq := bleve.NewTermQuery(repo)
	tq.SetField("repo")
	req := bleve.NewSearchRequestOptions(tq, maxIndexDocsPerRepo+1, 0, false)
	res, err := b.index.SearchInContext(ctx, req)
	if err != nil {
		return err
	}
	if len(res.Hits) == 0 {
		return nil
	}
	batch := b.index.NewBatch()
	for _, h := range res.Hits {
		batch.Delete(h.ID)
		if batch.Size() >= indexBatchSize {
			if err := b.index.Batch(batch); err != nil {
				return err
			}
			batch = b.index.NewBatch()
		}
	}
	if batch.Size() > 0 {
		return b.index.Batch(batch)
	}
	return nil
}

// Search 在索引中检索。索引只覆盖默认分支，opts.Ref 由 Service 在调用前核对。
func (b *Bleve) Search(ctx context.Context, owner, name, queryStr string, opts Options) ([]Hit, error) {
	if !validRepoKey(owner, name) {
		return nil, fmt.Errorf("invalid repo %q/%q", owner, name)
	}
	terms := opts.Terms
	if len(terms) == 0 {
		terms = []string{queryStr}
	}
	max := opts.Max
	if max <= 0 {
		max = 50
	}
	if max > 200 {
		max = 200
	}
	sub := make([]query.Query, 0, len(terms)+1)
	repoQ := bleve.NewTermQuery(repoKey(owner, name))
	repoQ.SetField("repo")
	sub = append(sub, repoQ)
	for _, t := range terms {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		// 每个词：精确 token（分析器拆分标识符） 或 前缀匹配（边打边搜）。
		// 两者取并集，再在保留全部词的文档中做 AND（ConjunctionQuery）。
		mq := bleve.NewMatchQuery(t)
		mq.SetField("text")
		mq.Analyzer = b.analyzer
		mq.SetOperator(query.MatchQueryOperatorAnd)
		pq := bleve.NewPrefixQuery(strings.ToLower(t))
		pq.SetField("text")
		pq.SetBoost(0.5)
		sub = append(sub, bleve.NewDisjunctionQuery(mq, pq))
	}
	if len(sub) == 1 {
		return []Hit{}, nil // 只有 repo 过滤，没有关键词
	}
	q := bleve.NewConjunctionQuery(sub...)
	// 预留余量：后置的固定字符串 AND 过滤会淘汰一部分 token 命中。
	size := max*8 + 64
	if size > maxIndexDocsPerRepo {
		size = maxIndexDocsPerRepo
	}
	req := bleve.NewSearchRequestOptions(q, size, 0, false)
	req.Fields = []string{"path", "line", "text"}
	res, err := b.index.SearchInContext(ctx, req)
	if err != nil {
		return nil, err
	}
	out := make([]Hit, 0, max)
	for _, h := range res.Hits {
		p, _ := h.Fields["path"].(string)
		text, _ := h.Fields["text"].(string)
		line := fieldInt(h.Fields["line"])
		if p == "" || line < 1 {
			continue
		}
		if !matchPathspecs(opts.Pathspec, p) || !containsAll(text, terms) {
			continue
		}
		if opts.Word && !wordMatchAll(text, terms) {
			continue
		}
		out = append(out, Hit{Path: p, Line: line, Text: text})
		if len(out) >= max {
			break
		}
	}
	return out, nil
}

func fieldInt(v interface{}) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int64:
		return int(n)
	case int:
		return n
	default:
		return 0
	}
}

// wordMatchAll 按单词边界匹配全部关键词；智能大小写（含大写则区分，否则忽略）。
func wordMatchAll(text string, terms []string) bool {
	for _, t := range terms {
		if t == "" {
			continue
		}
		pat := `\b` + regexp.QuoteMeta(t) + `\b`
		if !hasUpper(t) {
			pat = `(?i)` + pat
		}
		re, err := regexp.Compile(pat)
		if err != nil || !re.MatchString(text) {
			return false
		}
	}
	return true
}

func splitIndexLines(s string) []string {
	lines := strings.Split(s, "\n")
	for i := range lines {
		lines[i] = strings.TrimSuffix(lines[i], "\r")
	}
	return lines
}

func truncateIndexText(s string) string {
	if len(s) <= maxIndexLineBytes {
		return s
	}
	cut := maxIndexLineBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

var skipIndexDirs = map[string]bool{
	".git": true, "vendor": true, "node_modules": true, "bower_components": true,
	"third_party": true, "thirdparty": true, "dist": true, "build": true,
	"__pycache__": true, ".venv": true, "venv": true, ".tox": true, ".mypy_cache": true,
	".pytest_cache": true, "coverage": true, ".next": true, ".nuxt": true, ".cache": true,
	"target": true,
}

var skipIndexFiles = map[string]bool{
	"package-lock.json": true, "yarn.lock": true, "pnpm-lock.yaml": true,
	"bun.lock": true, "bun.lockb": true, "go.sum": true, "cargo.lock": true,
	"composer.lock": true, "gemfile.lock": true, "poetry.lock": true, "pipfile.lock": true,
}

func skipIndexPath(p string) bool {
	base := strings.ToLower(path.Base(p))
	if skipIndexFiles[base] {
		return true
	}
	if strings.HasSuffix(base, ".min.js") || strings.HasSuffix(base, ".min.css") {
		return true
	}
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if skipIndexDirs[strings.ToLower(seg)] {
			return true
		}
	}
	return false
}

// ---- 元信息持久化（记录已索引的仓库 ref/sha，跨重启判断就绪与增量回填） ----

func (b *Bleve) metaPath() string { return filepath.Join(b.dir, "meta.json") }

func (b *Bleve) loadMeta() {
	data, err := os.ReadFile(b.metaPath())
	if err != nil {
		return
	}
	m := map[string]metaEntry{}
	if err := json.Unmarshal(data, &m); err != nil {
		logx.Infof("codesearch: read meta: %v", err)
		return
	}
	b.meta = m
}

func (b *Bleve) setMeta(repo string, e metaEntry) {
	b.mu.Lock()
	b.meta[repo] = e
	b.mu.Unlock()
	if err := b.saveMeta(); err != nil {
		logx.Infof("codesearch: save meta: %v", err)
	}
}

func (b *Bleve) clearMeta(repo string) {
	b.mu.Lock()
	delete(b.meta, repo)
	b.mu.Unlock()
	if err := b.saveMeta(); err != nil {
		logx.Infof("codesearch: save meta before reindex: %v", err)
	}
}

func (b *Bleve) saveMeta() error {
	b.mu.Lock()
	data, err := json.MarshalIndent(b.meta, "", "  ")
	b.mu.Unlock()
	if err != nil {
		return err
	}
	tmp := b.metaPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, b.metaPath())
}
