// Package otel 提供带 trace_id 的结构化日志 handler。
package otel

import (
	"context"
	"log/slog"
	"os"
)

// SlogHandler 包装 slog.Handler，自动在每条日志后附加 trace_id 和 span_id。
type SlogHandler struct {
	inner slog.Handler
}

// NewSlogHandler 创建带有 OTel context 注入的日志 handler。
func NewSlogHandler() *SlogHandler {
	base := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	})
	return &SlogHandler{inner: base}
}

// Enabled 实现 slog.Handler。
func (h *SlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

// Handle 实现 slog.Handler，从 ctx 注入 trace_id。
func (h *SlogHandler) Handle(ctx context.Context, record slog.Record) error {
	if ctx != nil {
		attrs := SlogAttr(ctx)
		for _, attr := range attrs {
			record.AddAttrs(attr)
		}
	}
	return h.inner.Handle(ctx, record)
}

// WithAttrs 实现 slog.Handler。
func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogHandler{inner: h.inner.WithAttrs(attrs)}
}

// WithGroup 实现 slog.Handler。
func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return &SlogHandler{inner: h.inner.WithGroup(name)}
}
