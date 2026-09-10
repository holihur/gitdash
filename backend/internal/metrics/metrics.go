package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var registry = prometheus.NewRegistry()

var (
	httpRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "gitdash_http_requests_total",
			Help: "Total number of HTTP requests",
		},
		[]string{"method", "path", "status"},
	)
	httpDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "gitdash_http_request_duration_seconds",
			Help:    "HTTP request latency in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
)

func init() {
	registry.MustRegister(
		httpRequests,
		httpDuration,
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
}

// Handler 返回 /metrics 暴露处理器。
func Handler() http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// normalizePath 将具体路径归一化，避免 /repos/{name} 之类路由产生高基数标签。
func normalizePath(p string) string {
	if len(p) > 6 && p[:5] == "/api/" {
		rest := p[5:]
		for i := 0; i < len(rest); i++ {
			if rest[i] == '/' {
				return "/api/" + rest[:i]
			}
		}
		return p
	}
	return p
}

// Observe 记录一次请求的计数与延迟（由 logMiddleware 在响应结束后调用）。
func Observe(method, path string, status int, d time.Duration) {
	p := normalizePath(path)
	httpRequests.WithLabelValues(method, p, strconv.Itoa(status)).Inc()
	httpDuration.WithLabelValues(method, p).Observe(d.Seconds())
}
