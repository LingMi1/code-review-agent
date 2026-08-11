// Package otel 提供轻量级链路追踪：trace_id 和 span_id 的 Context 传播。
//
// 生产环境建议替换为 OpenTelemetry Go SDK（go.opentelemetry.io/otel），
// 并将 trace_id 注入 gRPC metadata 和 HTTP header。
package otel

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"
)

type contextKey string

const (
	keyTraceID contextKey = "otel.trace_id"
	keySpanID  contextKey = "otel.span_id"
)

// Span 代表一个操作段。
type Span struct {
	Name     string
	TraceID  string
	SpanID   string
	ParentID string
	Start    time.Time
	end      time.Time
	mu       sync.Mutex
	ended    bool
}

// NewTrace 在当前 context 中注入一个新 trace，返回新 context 和 span。
// span 必须通过 span.End() 关闭以记录耗时。
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	traceID := traceIDFromCtx(ctx)
	if traceID == "" {
		traceID = newID()
	}

	spanID := newID()
	parentID := ""
	if parent := spanFromCtx(ctx); parent != nil {
		parentID = parent.SpanID
	}

	span := &Span{
		Name:     name,
		TraceID:  traceID,
		SpanID:   spanID,
		ParentID: parentID,
		Start:    time.Now(),
	}

	slog.Debug("otel: span start",
		"trace_id", traceID,
		"span_id", spanID,
		"parent_id", parentID,
		"name", name,
	)

	ctx = context.WithValue(ctx, keyTraceID, traceID)
	ctx = context.WithValue(ctx, keySpanID, span)
	return ctx, span
}

// End 标记 span 结束。
func (s *Span) End() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return
	}
	s.ended = true
	s.end = time.Now()

	slog.Debug("otel: span end",
		"trace_id", s.TraceID,
		"span_id", s.SpanID,
		"name", s.Name,
		"duration_ms", s.end.Sub(s.Start).Milliseconds(),
	)
}

// Duration 返回 span 耗时。
func (s *Span) Duration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ended {
		return s.end.Sub(s.Start)
	}
	return time.Since(s.Start)
}

// TraceID 从 ctx 中提取 trace_id。如果不存在返回空字符串。
func TraceID(ctx context.Context) string {
	return traceIDFromCtx(ctx)
}

// SlogAttr 返回 slog 属性对 (trace_id, span_id)，用于日志行。
func SlogAttr(ctx context.Context) []slog.Attr {
	traceID := traceIDFromCtx(ctx)
	span := spanFromCtx(ctx)
	if traceID == "" && span == nil {
		return nil
	}
	attrs := []slog.Attr{}
	if traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	if span != nil {
		attrs = append(attrs, slog.String("span_id", span.SpanID))
	}
	return attrs
}

// --- 内部函数 ---

func traceIDFromCtx(ctx context.Context) string {
	if v, ok := ctx.Value(keyTraceID).(string); ok {
		return v
	}
	return ""
}

func spanFromCtx(ctx context.Context) *Span {
	if v, ok := ctx.Value(keySpanID).(*Span); ok {
		return v
	}
	return nil
}

func newID() string {
	var b [16]byte
	rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
