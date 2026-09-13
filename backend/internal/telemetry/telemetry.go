// Package telemetry 提供 OpenTelemetry trace 接入（OTLP/HTTP）。
// 未配置 OTEL_EXPORTER_OTLP_ENDPOINT 时全部为空实现（no-op），对调用方透明。
package telemetry

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const scope = "gitdash"

// Enabled 是否配置了 OTLP 导出端点。
func Enabled() bool { return strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")) != "" }

// Setup 初始化全局 TracerProvider。返回的 shutdown 在进程退出前调用以 flush。
func Setup(ctx context.Context, serviceName, version string) (func(context.Context) error, error) {
	if !Enabled() {
		return func(context.Context) error { return nil }, nil
	}
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	res, err := resource.Merge(resource.Default(), resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(version),
	))
	if err != nil {
		res = resource.Default()
	}
	ratio := 1.0
	if v := strings.TrimSpace(os.Getenv("OTEL_TRACES_SAMPLER_ARG")); v != "" {
		if f, perr := strconv.ParseFloat(v, 64); perr == nil {
			ratio = f
		}
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

// Tracer 返回 gitdash 的 tracer（用于业务代码手动加 span）。
func Tracer() trace.Tracer { return otel.Tracer(scope) }

// Middleware 为每个 HTTP 请求创建 server span，注入/提取 W3C trace context。
func Middleware(next http.Handler) http.Handler {
	tracer := otel.Tracer(scope + "/http")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := otel.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := tracer.Start(ctx, r.Method+" "+r.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.request.method", r.Method),
				attribute.String("url.path", r.URL.Path),
				attribute.String("url.scheme", scheme(r)),
				attribute.String("server.address", r.Host),
			),
		)
		defer span.End()

		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r.WithContext(ctx))
		span.SetAttributes(attribute.Int("http.response.status_code", sw.status))
		if sw.status >= 500 {
			span.SetStatus(codes.Error, http.StatusText(sw.status))
		}
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Unwrap 支持 WebSocket 升级等需要 Hijack/Flush 透传的场景。
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}
	if p := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); p != "" {
		return p
	}
	return "http"
}
