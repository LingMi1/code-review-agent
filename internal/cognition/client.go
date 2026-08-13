// Package cognition 封装对 agent-go 认知面的 gRPC 调用。
package cognition

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	agentv1 "github.com/LingMi1/code-review-agent/internal/genproto/agent/v1"
	"github.com/LingMi1/code-review-agent/internal/otel"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// 认知面调用的默认健壮性参数。
const (
	defaultTimeout = 5 * time.Minute // 单次 gRPC 调用超时
	maxAttempts    = 3               // 瞬时错误的最大重试次数
	maxCBFailures  = 5               // 连续失败多少次后熔断
	cbCooldown     = 30 * time.Second
)

// Client 是 agent-go 认知面的客户端。
type Client struct {
	addr string
	conn *grpc.ClientConn
	svc  agentv1.CognitionServiceClient
	cb   *circuitBreaker
}

// circuitBreaker 是基于连续失败次数的轻量熔断器。
type circuitBreaker struct {
	mu                  sync.Mutex
	maxFailures         int
	cooldown            time.Duration
	consecutiveFailures int
	openUntil           time.Time
}

func newCircuitBreaker(maxFailures int, cooldown time.Duration) *circuitBreaker {
	return &circuitBreaker{maxFailures: maxFailures, cooldown: cooldown}
}

func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return !cb.openUntil.After(time.Now())
}

func (cb *circuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFailures = 0
	cb.openUntil = time.Time{}
}

func (cb *circuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.consecutiveFailures++
	if cb.consecutiveFailures >= cb.maxFailures {
		cb.openUntil = time.Now().Add(cb.cooldown)
		cb.consecutiveFailures = 0
	}
}

// New 创建认知面客户端并建立连接。
func New(addr string) (*Client, error) {
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("cognition: dial %s: %w", addr, err)
	}
	return &Client{
		addr: addr,
		conn: conn,
		svc:  agentv1.NewCognitionServiceClient(conn),
		cb:   newCircuitBreaker(maxCBFailures, cbCooldown),
	}, nil
}

// Close 关闭连接。
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// ReviewRequest 是一次代码审查的入参。
type ReviewRequest struct {
	SessionID string // 用于区分不同 PR 的审查会话
	Query     string // 完整的 review prompt
	AgentType string // "react" 或 "plan_execute"
	MaxSteps  int32  // Agent 思考步数上限
}

// ReviewResult 是审查结果。
type ReviewResult struct {
	Text     string // Agent 返回的原始文本
	Duration time.Duration
}

// RunReview 发送审查请求到 agent-go 认知面，收集并返回最终结果。
func (c *Client) RunReview(ctx context.Context, req ReviewRequest) (*ReviewResult, error) {
	agentType := req.AgentType
	if agentType == "" {
		agentType = "react"
	}
	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 3
	}

	return c.run(ctx, &agentv1.RunRequest{
		RunId:         uuid.New().String(),
		SessionId:     req.SessionID,
		Query:         req.Query,
		AgentType:     agentType,
		MaxSteps:      maxSteps,
		SchemaVersion: "v1",
		Metadata: map[string]string{
			"output_format": "json",
		},
	})
}

// RunPlanExecuteReview 使用 Plan-Execute 模式审查大型 PR。
// Plan-Execute 会自动拆解审查为子任务（安全、风格、性能等），并行执行后汇总。
func (c *Client) RunPlanExecuteReview(ctx context.Context, sessID, query string, maxSteps int32) (*ReviewResult, error) {
	if maxSteps <= 0 {
		maxSteps = 8 // Plan-Execute 需要更多步数（plan + execute + aggregate）
	}

	return c.run(ctx, &agentv1.RunRequest{
		RunId:         uuid.New().String(),
		SessionId:     sessID,
		Query:         query,
		AgentType:     "plan_execute",
		MaxSteps:      maxSteps,
		SchemaVersion: "v1",
		Metadata: map[string]string{
			"output_format": "json",
			"review_mode":   "plan_execute",
		},
	})
}

// run 执行一次认知面调用，带超时、重试与熔断保护。
func (c *Client) run(ctx context.Context, runReq *agentv1.RunRequest) (*ReviewResult, error) {
	start := time.Now()

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if !c.cb.allow() {
			return nil, fmt.Errorf("cognition: circuit breaker open")
		}

		callCtx, cancel := context.WithTimeout(ctx, defaultTimeout)
		finalText, err := c.runOnce(callCtx, runReq)
		cancel()

		if err == nil {
			c.cb.onSuccess()
			return &ReviewResult{Text: finalText, Duration: time.Since(start)}, nil
		}

		c.cb.onFailure()
		lastErr = err
		if !isRetryable(err) {
			break
		}
		slog.Warn("cognition: retrying after transient error",
			"attempt", attempt+1,
			"error", err,
		)
		select {
		case <-time.After(time.Duration(attempt+1) * time.Second):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return nil, lastErr
}

// runOnce 打开 gRPC stream 并收集最终结果文本。
func (c *Client) runOnce(ctx context.Context, runReq *agentv1.RunRequest) (string, error) {
	ctx = injectTraceID(ctx)

	stream, err := c.svc.Run(ctx, runReq, grpc.WaitForReady(true))
	if err != nil {
		return "", fmt.Errorf("cognition: open run stream: %w", err)
	}

	return collectFinalResult(runReq.GetRunId(), stream)
}

// injectTraceID 将 ctx 中的 trace_id 注入 gRPC outgoing metadata，实现跨进程关联。
func injectTraceID(ctx context.Context) context.Context {
	traceID := otel.TraceID(ctx)
	if traceID == "" {
		return ctx
	}
	md, ok := metadata.FromOutgoingContext(ctx)
	if !ok {
		md = metadata.MD{}
	} else {
		md = md.Copy()
	}
	md.Set("x-trace-id", traceID)
	return metadata.NewOutgoingContext(ctx, md)
}

// isRetryable 判断错误是否适合重试（仅瞬时/传输类错误）。
func isRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		// 非 gRPC 状态错误（如缺少结果文本）重试无意义。
		return false
	}
	switch st.Code() {
	case codes.Unavailable, codes.DeadlineExceeded, codes.Aborted, codes.ResourceExhausted:
		return true
	default:
		return false
	}
}

// collectFinalResult 从 gRPC server-stream 中收集最终结果文本。
func collectFinalResult(runID string, stream agentv1.CognitionService_RunClient) (string, error) {
	var finalText string
	for {
		event, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("cognition: recv event: %w", err)
		}

		slog.Debug("cognition: event",
			"run_id", runID,
			"type", event.GetType().String(),
			"seq", event.GetSeq(),
			"step", event.GetStep(),
		)

		if thought := event.GetToolThought(); thought != nil {
			slog.Info("cognition: thought", "text", truncate(thought.GetText(), 200))
		}
		if tool := event.GetToolCall(); tool != nil {
			slog.Info("cognition: tool_call",
				"name", tool.GetToolName(),
				"status", tool.GetStatus().String(),
			)
		}
		if result := event.GetResult(); result != nil {
			finalText = result.GetText()
		}
		if event.GetFinish() {
			break
		}
	}
	if finalText == "" {
		return "", fmt.Errorf("cognition: no result text in run %s", runID)
	}
	return finalText, nil
}

// truncate 截断字符串用于日志。
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
