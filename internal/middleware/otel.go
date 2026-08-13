// Package middleware 提供 HTTP 中间件。
package middleware

import (
	"net/http"

	otelapi "go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	"github.com/LingMi1/code-review-agent/internal/otel"
)

// Tracing 为每个 HTTP 请求提取上游 trace context 并创建 span。
func Tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 从请求头提取上游 trace（W3C traceparent），无则新建 trace。
		ctx := otelapi.GetTextMapPropagator().Extract(r.Context(), propagation.HeaderCarrier(r.Header))
		ctx, span := otel.StartSpan(ctx, r.Method+" "+r.URL.Path)
		defer span.End()

		// 将 trace_id 写入 response header，方便客户端关联。
		w.Header().Set("X-Trace-ID", otel.TraceID(ctx))

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
