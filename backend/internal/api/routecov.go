package api

import (
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
)

// routeMux wraps http.ServeMux to remember every registered route pattern.
// The inventory feeds the black-box endpoint-coverage checks run by the
// pytest and Playwright suites (see GITDASH_ROUTE_COVERAGE_FILE).
type routeMux struct {
	*http.ServeMux
	mu       sync.Mutex
	patterns []string
}

func newRouteMux() *routeMux {
	return &routeMux{ServeMux: http.NewServeMux()}
}

func (m *routeMux) HandleFunc(pattern string, h http.HandlerFunc) {
	m.record(pattern)
	m.ServeMux.HandleFunc(pattern, h)
}

func (m *routeMux) Handle(pattern string, h http.Handler) {
	m.record(pattern)
	m.ServeMux.Handle(pattern, h)
}

func (m *routeMux) record(pattern string) {
	m.mu.Lock()
	m.patterns = append(m.patterns, pattern)
	m.mu.Unlock()
}

// Routes returns the registered patterns, de-duplicated and sorted.
func (m *routeMux) Routes() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := make(map[string]bool, len(m.patterns))
	out := make([]string, 0, len(m.patterns))
	for _, p := range m.patterns {
		if seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// routeRecorder observes which route patterns a black-box run actually hits.
//
// It is enabled by GITDASH_ROUTE_COVERAGE_FILE=<path>: on startup the full
// inventory is written as `route\t<pattern>` lines, and the first hit of each
// pattern appends a `hit\t<pattern>` line. The file is append-only so multiple
// server instances (pytest/Playwright sessions) can share one path; consumers
// de-duplicate. When the env var is unset the middleware is not installed.
type routeRecorder struct {
	next http.Handler
	file string
	mu   sync.Mutex
	seen map[string]bool
}

func newRouteRecorder(next http.Handler, file string, inventory []string) http.Handler {
	r := &routeRecorder{next: next, file: file, seen: make(map[string]bool, len(inventory))}
	var b strings.Builder
	for _, p := range inventory {
		b.WriteString("route\t")
		b.WriteString(p)
		b.WriteByte('\n')
	}
	appendRouteCoverage(file, b.String())
	return r
}

func (r *routeRecorder) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	r.next.ServeHTTP(w, req)
	pattern := req.Pattern
	if pattern == "" {
		return
	}
	r.mu.Lock()
	if r.seen[pattern] {
		r.mu.Unlock()
		return
	}
	r.seen[pattern] = true
	r.mu.Unlock()
	appendRouteCoverage(r.file, "hit\t"+pattern+"\n")
}

func appendRouteCoverage(file, data string) {
	if file == "" || data == "" {
		return
	}
	f, err := os.OpenFile(file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	_, _ = f.WriteString(data)
	_ = f.Close()
}
