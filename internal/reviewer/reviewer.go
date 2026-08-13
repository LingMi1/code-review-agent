// Package reviewer 编排一次 PR 审查：拉取 diff、调用认知面、投递结果。
//
// 通过接口注入依赖（store / cognition / github / sse / metrics），
// 使核心编排逻辑可单元测试。
package reviewer

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/debug"
	"time"

	"github.com/LingMi1/code-review-agent/internal/cognition"
	"github.com/LingMi1/code-review-agent/internal/diff"
	"github.com/LingMi1/code-review-agent/internal/otel"
	"github.com/LingMi1/code-review-agent/internal/prompt"
	"github.com/LingMi1/code-review-agent/internal/review"
	"github.com/LingMi1/code-review-agent/internal/sse"
)

// Store 是审查记录的存储抽象。
type Store interface {
	InsertReview(prNumber int, repoURL, headSHA string) (int64, error)
	UpdateReview(id int64, status string, issues int, summary, duration, errMsg string) error
	AuditLog(action string, prNumber int, actor, detail string)
}

// Cognition 是 agent-go 认知面客户端抽象。
type Cognition interface {
	RunReview(ctx context.Context, req cognition.ReviewRequest) (*cognition.ReviewResult, error)
	RunPlanExecuteReview(ctx context.Context, sessID, query string, maxSteps int32) (*cognition.ReviewResult, error)
}

// GitHub 是审查所需的 GitHub 操作抽象。
type GitHub interface {
	PRDiff(ctx context.Context, owner, repo string, prNumber int) (string, error)
	review.Poster
}

// SSE 是实时进度广播抽象。
type SSE interface {
	Publish(sessionID string, evt sse.Event)
}

// Metrics 是审查指标抽象。
type Metrics interface {
	RecordSuccess(latencyMs int64, outputBytes int, issuesFound int)
	RecordFailure()
	RecordCognitionError()
}

// Service 编排一次代码审查。
type Service struct {
	store   Store
	cog     Cognition
	gh      GitHub
	sse     SSE
	metrics Metrics
	sem     chan struct{}
}

// New 创建审查服务，maxConcurrent 为并发审查上限。
func New(st Store, cog Cognition, gh GitHub, sseHub SSE, m Metrics, maxConcurrent int) *Service {
	return &Service{
		store:   st,
		cog:     cog,
		gh:      gh,
		sse:     sseHub,
		metrics: m,
		sem:     make(chan struct{}, maxConcurrent),
	}
}

// Review 执行一次完整的 PR 审查。existingReviewID > 0 时复用已有记录（手动触发场景）。
func (s *Service) Review(ctx context.Context, owner, repoName string, prNumber int, headSHA string, existingReviewID int64) (err error) {
	repoFullName := owner + "/" + repoName

	// 并发限流，防止突发 PR 打爆 gRPC/SQLite。
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	sessionID := fmt.Sprintf("pr-review-%s/%d", repoFullName, prNumber)

	ctx, span := otel.StartSpan(ctx, "review.pr")
	defer span.End()

	reviewStart := time.Now()

	slog.InfoContext(ctx, "processing PR",
		"pr", prNumber,
		"repo", repoFullName,
		"head_sha", headSHA,
	)

	s.sse.Publish(sessionID, jsonEvent("review.started", map[string]any{"pr": prNumber, "status": "running"}))

	reviewID := existingReviewID
	var resultErr error
	defer func() {
		if r := recover(); r != nil {
			resultErr = fmt.Errorf("panic: %v", r)
			slog.Error("review panic", "pr", prNumber, "panic", r, "stack", string(debug.Stack()))
			s.metrics.RecordFailure()
		}
		if resultErr != nil {
			if reviewID != 0 {
				s.store.UpdateReview(reviewID, "failed", 0, "", "", resultErr.Error())
			}
			s.sse.Publish(sessionID, jsonEvent("review.failed", map[string]any{
				"pr": prNumber, "status": "failed", "error": resultErr.Error(),
			}))
			err = resultErr
		}
	}()

	s.store.AuditLog("review.started", prNumber, repoFullName, "")

	// 1. 创建审查记录（手动触发已预创建则复用）。
	if reviewID == 0 {
		var err error
		reviewID, err = s.store.InsertReview(prNumber, repoFullName, headSHA)
		if err != nil {
			s.metrics.RecordFailure()
			resultErr = fmt.Errorf("insert review record: %w", err)
			return resultErr
		}
	}
	s.store.AuditLog("review.started", prNumber, repoFullName, fmt.Sprintf("review_id=%d", reviewID))

	// 2. 获取 PR diff。
	_, fetchSpan := otel.StartSpan(ctx, "github.fetch_diff")
	rawDiff, err := s.gh.PRDiff(ctx, owner, repoName, prNumber)
	fetchSpan.End()
	if err != nil {
		s.store.AuditLog("review.failed", prNumber, repoFullName, err.Error())
		s.metrics.RecordFailure()
		resultErr = fmt.Errorf("fetch PR diff: %w", err)
		return resultErr
	}

	// 3. 解析 diff → 按文件分块。
	_, parseSpan := otel.StartSpan(ctx, "diff.parse")
	allFiles := diff.Parse(rawDiff)
	files := diff.SkipGeneratedFiles(allFiles)
	files = diff.SkipLockFiles(files)
	files = diff.SkipDataFiles(files)
	const maxChunkLines = 800
	chunks := diff.ChunkBySize(files, maxChunkLines)
	parseSpan.End()

	if len(chunks) == 0 {
		slog.InfoContext(ctx, "no files to review (all generated/skipped)", "pr", prNumber)
		s.store.UpdateReview(reviewID, "success", 0, "No reviewable files", "", "")
		s.sse.Publish(sessionID, jsonEvent("review.completed", map[string]any{
			"pr": prNumber, "status": "success", "issues": 0, "duration_ms": 0,
		}))
		return nil
	}

	prSize := diff.CalcPRSize(allFiles, files)
	usePlanExecute := diff.ShouldUsePlanExecute(prSize)

	slog.InfoContext(ctx, "diff parsed",
		"pr", prNumber,
		"files", len(files),
		"total_files", len(allFiles),
		"chunks", len(chunks),
		"lines", prSize.Lines,
		"mode", map[bool]string{true: "plan_execute", false: "react"}[usePlanExecute],
	)

	s.sse.Publish(sessionID, jsonEvent("review.progress", map[string]any{
		"pr":     prNumber,
		"status": "analyzing",
		"files":  len(files),
		"chunks": len(chunks),
		"mode":   map[bool]string{true: "plan_execute", false: "react"}[usePlanExecute],
	}))

	// 4. 构造 prompt。
	prTitle := fmt.Sprintf("%s#%d", repoFullName, prNumber)
	var reviewPrompt string
	if usePlanExecute {
		reviewPrompt = prompt.BuildPlanExecutePrompt(prTitle, files)
	} else {
		reviewPrompt = prompt.BuildReviewPrompt(prTitle, files)
	}
	const maxPromptBytes = 32_000
	reviewPrompt = prompt.TruncatePrompt(reviewPrompt, maxPromptBytes)

	// 5. 调用 agent-go 认知面。
	_, cognitionSpan := otel.StartSpan(ctx, "cognition.run_review")
	slog.InfoContext(ctx, "calling agent-go cognition",
		"pr", prNumber,
		"prompt_bytes", len(reviewPrompt),
		"use_plan_execute", usePlanExecute,
	)
	var result *cognition.ReviewResult
	if usePlanExecute {
		result, err = s.cog.RunPlanExecuteReview(ctx,
			fmt.Sprintf("pr-review-%s-%d", repoName, prNumber),
			reviewPrompt, 8)
	} else {
		result, err = s.cog.RunReview(ctx, cognition.ReviewRequest{
			SessionID: fmt.Sprintf("pr-review-%s-%d", repoName, prNumber),
			Query:     reviewPrompt,
			AgentType: "react",
			MaxSteps:  5,
		})
	}
	cognitionSpan.End()
	if err != nil {
		s.store.AuditLog("review.failed", prNumber, repoFullName, err.Error())
		s.metrics.RecordCognitionError()
		s.metrics.RecordFailure()
		resultErr = fmt.Errorf("run review: %w", err)
		return resultErr
	}

	slog.InfoContext(ctx, "review completed",
		"pr", prNumber,
		"duration", result.Duration,
		"output_chars", len(result.Text),
	)

	// 6. 解析结果 + 投递到 GitHub。
	_, postSpan := otel.StartSpan(ctx, "github.post_review")
	postResult, err := review.ParseAndPost(ctx, s.gh, owner, repoName, prNumber, headSHA, result.Text)
	if err != nil {
		s.store.AuditLog("review.failed", prNumber, repoFullName, err.Error())
		s.metrics.RecordFailure()
		postSpan.End()
		resultErr = fmt.Errorf("post review: %w", err)
		return resultErr
	}
	postSpan.End()

	// 7. 更新审查记录（复用 ParseAndPost 返回的结构化结果）。
	issues := postResult.Issues
	s.store.UpdateReview(reviewID, "success", issues, postResult.Summary, result.Duration.String(), "")
	s.store.AuditLog("review.completed", prNumber, repoFullName,
		fmt.Sprintf("issues=%d duration=%s", issues, result.Duration))

	s.metrics.RecordSuccess(result.Duration.Milliseconds(), len(result.Text), issues)

	s.sse.Publish(sessionID, jsonEvent("review.completed", map[string]any{
		"pr": prNumber, "status": "success", "issues": issues, "duration_ms": result.Duration.Milliseconds(),
	}))

	slog.InfoContext(ctx, "review posted to GitHub", "pr", prNumber, "issues", issues)
	slog.InfoContext(ctx, "review total duration", "ms", time.Since(reviewStart).Milliseconds())
	return nil
}

// jsonEvent 构造一条 SSE 事件，data 用 JSON 序列化避免手工拼接导致的转义问题。
func jsonEvent(typ string, v any) sse.Event {
	b, err := json.Marshal(v)
	if err != nil {
		b = []byte("{}")
	}
	return sse.Event{Type: typ, Data: string(b)}
}
