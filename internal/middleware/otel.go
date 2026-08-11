// Package middleware 提供 HTTP 中间件。
package middleware

import (
	"net/http"

	"github.com/LingMi1/code-review-agent/internal/otel"
)

// Tracing 为每个 HTTP 请求创建一个 otel span，注入 trace_id 到 context。
func Tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := otel.StartSpan(r.Context(), r.Method+" "+r.URL.Path)
		defer span.End()

		// 将 trace_id 写入 response header，方便客户端关联
		w.Header().Set("X-Trace-ID", span.TraceID)

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
