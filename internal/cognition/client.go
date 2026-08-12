// Package cognition 封装对 agent-go 认知面的 gRPC 调用。
package cognition

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	agentv1 "github.com/LingMi1/code-review-agent/internal/genproto/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/google/uuid"
)

// Client 是 agent-go 认知面的客户端。
type Client struct {
	addr string
	conn *grpc.ClientConn
	svc  agentv1.CognitionServiceClient
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
	start := time.Now()

	agentType := req.AgentType
	if agentType == "" {
		agentType = "react"
	}
	maxSteps := req.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 3
	}

	runID := uuid.New().String()
	stream, err := c.svc.Run(ctx, &agentv1.RunRequest{
		RunId:         runID,
		SessionId:     req.SessionID,
		Query:         req.Query,
		AgentType:     agentType,
		MaxSteps:      maxSteps,
		SchemaVersion: "v1",
		Metadata: map[string]string{
			"output_format": "json",
		},
	}, grpc.WaitForReady(true))
	if err != nil {
		return nil, fmt.Errorf("cognition: open run stream: %w", err)
	}

	finalText, err := collectFinalResult(runID, stream)
	if err != nil {
		return nil, err
	}

	return &ReviewResult{
		Text:     finalText,
		Duration: time.Since(start),
	}, nil
}

// RunPlanExecuteReview 使用 Plan-Execute 模式审查大型 PR。
// Plan-Execute 会自动拆解审查为子任务（安全、风格、性能等），并行执行后汇总。
func (c *Client) RunPlanExecuteReview(ctx context.Context, sessID, query string, maxSteps int32) (*ReviewResult, error) {
	if maxSteps <= 0 {
		maxSteps = 8 // Plan-Execute 需要更多步数（plan + execute + aggregate）
	}
	start := time.Now()

	runID := uuid.New().String()
	stream, err := c.svc.Run(ctx, &agentv1.RunRequest{
		RunId:         runID,
		SessionId:     sessID,
		Query:         query,
		AgentType:     "plan_execute",
		MaxSteps:      maxSteps,
		SchemaVersion: "v1",
		Metadata: map[string]string{
			"output_format": "json",
			"review_mode":   "plan_execute",
		},
	}, grpc.WaitForReady(true))
	if err != nil {
		return nil, fmt.Errorf("cognition: open plan-execute stream: %w", err)
	}

	finalText, err := collectFinalResult(runID, stream)
	if err != nil {
		return nil, err
	}

	return &ReviewResult{
		Text:     finalText,
		Duration: time.Since(start),
	}, nil
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
