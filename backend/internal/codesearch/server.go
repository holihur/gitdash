package codesearch

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"gitdash/backend/internal/gitsvc"
)

// SearchServer 把本地搜索服务（Bleve + grep 回退）暴露为内部 HTTP 端点，供 API
// 节点远程调用。仅应在索引服务（GITDASH_ROLE=codeindex）中启用。
type SearchServer struct {
	svc   *Service
	token string
}

// NewSearchServer 创建内部检索服务；token 非空时要求 `Authorization: Bearer <token>`。
func NewSearchServer(svc *Service, token string) *SearchServer {
	return &SearchServer{svc: svc, token: token}
}

// Handler 返回内部检索与运行态端点。
func (s *SearchServer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+InternalSearchPath, s.handle)
	mux.HandleFunc("GET "+InternalStatsPath, s.handleStats)
	return mux
}

// authorized 校验共享 Bearer token（未配置 token 时放行，依赖网络隔离）。
func (s *SearchServer) authorized(w http.ResponseWriter, r *http.Request) bool {
	if s.token == "" {
		return true
	}
	got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
		writeJSONStatus(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return false
	}
	return true
}

func (s *SearchServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	st, err := s.svc.IndexStats(r.Context())
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]string{"error": "stats failed"})
		return
	}
	writeJSONStatus(w, http.StatusOK, st)
}

const (
	maxRemoteQuery = 4096
	maxRemoteBody  = 1 << 20 // 1 MiB
	maxRemoteTerms = 32
)

func (s *SearchServer) handle(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRemoteBody)
	var req remoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	q := strings.TrimSpace(req.Query)
	if !gitsvc.ValidName(req.Owner) || !gitsvc.ValidName(req.Name) || q == "" || len(q) > maxRemoteQuery {
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	// 上限保护：远程传入的 Max/pathspec/terms 不得拖垮索引服务。
	opts := req.Opts
	if opts.Max <= 0 || opts.Max > 200 {
		opts.Max = 50
	}
	if len(opts.Pathspec) > maxRemoteTerms {
		opts.Pathspec = opts.Pathspec[:maxRemoteTerms]
	}
	if len(opts.Terms) > maxRemoteTerms {
		opts.Terms = opts.Terms[:maxRemoteTerms]
	}
	hits, src, err := s.svc.SearchSource(r.Context(), req.Owner, req.Name, q, opts)
	if err != nil {
		// 不回传内部错误细节（可能含路径），仅返回通用错误。
		writeJSONStatus(w, http.StatusBadRequest, map[string]string{"error": "search failed"})
		return
	}
	writeJSONStatus(w, http.StatusOK, remoteResponse{Hits: hits, Source: string(src)})
}

func writeJSONStatus(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
