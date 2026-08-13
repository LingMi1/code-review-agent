// Package otel 基于 OpenTelemetry Go SDK 提供分布式追踪。
//
// 使用标准 W3C Trace Context 在 HTTP / gRPC 之间传播，可通过 OTLP 导出到
// Jaeger、Tempo 等后端。未配置 OTEL_EXPORTER_OTLP_ENDPOINT 时，仅在进程内
// 记录 span 并注入日志（trace_id / span_id），不导出。
package otel

import (
	"context"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

const serviceName = "code-review-agent"

// Init 初始化 OpenTelemetry TracerProvider 与跨进程传播器。
// 返回 shutdown 函数用于优雅关闭。
func Init(ctx context.Context) (func(context.Context) error, error) {
	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(resource.Default()),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	}

	// 配置了 OTLP endpoint 才导出到 collector，否则仅进程内记录。
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		exp, err := otlptracegrpc.New(ctx,
			otlptracegrpc.WithEndpoint(endpoint),
			otlptracegrpc.WithInsecure(),
		)
		if err != nil {
			return nil, err
		}
		opts = append(opts, sdktrace.WithBatcher(exp))
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
	return tp.Shutdown, nil
}

// StartSpan 创建一个 span 并注入 context。
func StartSpan(ctx context.Context, name string) (context.Context, trace.Span) {
	return otel.Tracer(serviceName).Start(ctx, name)
}

// TraceID 从 ctx 提取 trace_id 字符串（W3C 32 位 hex）。
func TraceID(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.IsValid() {
		return sc.TraceID().String()
	}
	return ""
}

// SlogAttr 返回 (trace_id, span_id) slog 属性，用于结构化日志关联。
func SlogAttr(ctx context.Context) []slog.Attr {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return nil
	}
	return []slog.Attr{
		slog.String("trace_id", sc.TraceID().String()),
		slog.String("span_id", sc.SpanID().String()),
	}
}
