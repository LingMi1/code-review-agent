package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recovery 是 panic 恢复中间件：捕获 handler 中的 panic，
// 记录堆栈日志，返回 500 而非让进程崩溃。
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered",
					"method", r.Method,
					"path", r.URL.Path,
					"panic", rec,
					"stack", string(debug.Stack()),
				)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
