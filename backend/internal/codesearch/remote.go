package codesearch

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// 内部检索服务的线上协议。
type remoteRequest struct {
	Owner string  `json:"owner"`
	Name  string  `json:"name"`
	Query string  `json:"query"`
	Opts  Options `json:"opts"`
}

type remoteResponse struct {
	Hits   []Hit  `json:"hits"`
	Source string `json:"source"`
}

// InternalSearchPath 是索引服务暴露的内部检索端点。
const InternalSearchPath = "/internal/codesearch"

// InternalStatsPath 是索引服务暴露的内部运行态端点。
const InternalStatsPath = "/internal/codesearch/stats"

// Remote 通过内部 HTTP 调用独立的索引服务执行检索，使 API 节点无需打开索引
// （Bleve 索引为单进程独占，拆服务时由索引服务持有）。网络异常时回退到本机
// fallback（通常是实时 grep），保证可用性。
type Remote struct {
	url      string
	token    string
	client   *http.Client
	fallback Searcher
}

// NewRemote 创建远程检索客户端。url 为索引服务地址（如 http://127.0.0.1:8090）；
// token 为共享密钥（服务端开启鉴权时必填）；fallback 可为 nil（直接返回错误）。
func NewRemote(url, token string, fallback Searcher) *Remote {
	return &Remote{
		url:      strings.TrimRight(strings.TrimSpace(url), "/"),
		token:    token,
		client:   &http.Client{Timeout: 30 * time.Second},
		fallback: fallback,
	}
}

// Search 实现 Searcher。
func (r *Remote) Search(ctx context.Context, owner, name, query string, opts Options) ([]Hit, error) {
	hits, _, err := r.SearchSource(ctx, owner, name, query, opts)
	return hits, err
}

// SearchSource 实现 SourcedSearcher：来源由索引服务返回（index 或 grep）。
func (r *Remote) SearchSource(ctx context.Context, owner, name, query string, opts Options) ([]Hit, Source, error) {
	body, err := json.Marshal(remoteRequest{Owner: owner, Name: name, Query: query, Opts: opts})
	if err != nil {
		return nil, SourceGrep, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url+InternalSearchPath, bytes.NewReader(body))
	if err != nil {
		return nil, SourceGrep, err
	}
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return r.fallbackOrErr(ctx, owner, name, query, opts, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return r.fallbackOrErr(ctx, owner, name, query, opts, errRemoteStatus(resp.StatusCode))
	}
	// 限制响应体大小，避免被异常服务端拖垮。
	var rr remoteResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&rr); err != nil {
		return r.fallbackOrErr(ctx, owner, name, query, opts, err)
	}
	src := SourceIndex
	if rr.Source == string(SourceGrep) {
		src = SourceGrep
	}
	return rr.Hits, src, nil
}

// IndexStats 实现 StatsReporter：代理到索引服务的运行态端点。
func (r *Remote) IndexStats(ctx context.Context) (IndexStats, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url+InternalStatsPath, nil)
	if err != nil {
		return IndexStats{Backend: "remote"}, err
	}
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return IndexStats{Backend: "remote"}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return IndexStats{Backend: "remote"}, errRemoteStatus(resp.StatusCode)
	}
	var st IndexStats
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&st); err != nil {
		return IndexStats{Backend: "remote"}, err
	}
	if st.Backend == "" {
		st.Backend = ModeBleve
	}
	return st, nil
}

func (r *Remote) fallbackOrErr(ctx context.Context, owner, name, query string, opts Options, err error) ([]Hit, Source, error) {
	if r.fallback == nil {
		return nil, SourceGrep, err
	}
	hits, ferr := r.fallback.Search(ctx, owner, name, query, opts)
	if ferr != nil {
		return nil, SourceGrep, ferr
	}
	return hits, SourceGrep, nil
}

type errRemoteStatus int

func (e errRemoteStatus) Error() string {
	return "codesearch: remote index returned status " + http.StatusText(int(e))
}
