// Code Review Agent — powered by agent-go
//
// 启动方式：
//
//	export GITHUB_TOKEN=ghp_xxxx
//	export WEBHOOK_SECRET=mysecret
//	export COGNITION_ADDR=localhost:50051
//	go run ./cmd/server/
//
// 需要 agent-go 认知面已启动（docker compose up agent-go-cognition）。
//
// 可观测性：
//
//	GET /metrics  — Prometheus text format 指标
//	GET /health   — 健康检查
//	X-Trace-ID header + 结构化日志 trace_id 贯穿全链路
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/LingMi1/code-review-agent/internal/cognition"
	"github.com/LingMi1/code-review-agent/internal/diff"
	"github.com/LingMi1/code-review-agent/internal/github"
	"github.com/LingMi1/code-review-agent/internal/metrics"
	"github.com/LingMi1/code-review-agent/internal/middleware"
	"github.com/LingMi1/code-review-agent/internal/otel"
	"github.com/LingMi1/code-review-agent/internal/prompt"
	"github.com/LingMi1/code-review-agent/internal/review"
	"github.com/LingMi1/code-review-agent/internal/sse"
	"github.com/LingMi1/code-review-agent/internal/store"
	"github.com/LingMi1/code-review-agent/internal/webhook"
)

func main() {
	// 结构化日志 + 自动 trace_id 注入
	slog.SetDefault(slog.New(otel.NewSlogHandler()))

	// 环境变量
	githubToken := os.Getenv("GITHUB_TOKEN")
	webhookSecret := os.Getenv("WEBHOOK_SECRET")
	cognitionAddr := os.Getenv("COGNITION_ADDR")
	listenAddr := envDefault("LISTEN_ADDR", ":8080")
	sqlitePath := envDefault("SQLITE_PATH", "./data/reviews.db")

	if githubToken == "" {
		slog.Error("GITHUB_TOKEN is required")
		os.Exit(1)
	}
	if cognitionAddr == "" {
		cognitionAddr = "localhost:50051"
	}

	// Prometheus 指标
	m := metrics.New()

	// 初始化组件
	ghClient := github.New(githubToken)

	cogClient, err := cognition.New(cognitionAddr)
	if err != nil {
		slog.Error("failed to connect to agent-go cognition", "error", err)
		os.Exit(1)
	}
	defer cogClient.Close()

	// 确保数据目录存在
	if err := os.MkdirAll("data", 0o755); err != nil {
		slog.Error("failed to create data dir", "error", err)
		os.Exit(1)
	}

	db, err := store.New(sqlitePath)
	if err != nil {
		slog.Error("failed to open SQLite", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// SSE hub — 实时推送审查进度
	sseHub := sse.NewHub()

	// 并发审查上限，防止突发 PR 打爆 gRPC/SQLite
	const maxConcurrentReviews = 5
	reviewSem := make(chan struct{}, maxConcurrentReviews)

	// processReview 是审查核心逻辑，被 webhook 和手动 API 共享。
	processReview := func(ctx context.Context, owner, repoName string, prNumber int, headSHA string, existingReviewID int64) error {
		repoFullName := owner + "/" + repoName

		// 并发限流，防止突发 PR 打爆 gRPC/SQLite
		select {
		case reviewSem <- struct{}{}:
			defer func() { <-reviewSem }()
		case <-ctx.Done():
			return ctx.Err()
		}

		sessionID := fmt.Sprintf("pr-review-%s/%d", repoFullName, prNumber)

		// 创建根 span：整个 PR 审查流程
		ctx, span := otel.StartSpan(ctx, "review.pr")
		defer span.End()

		reviewStart := time.Now()

		slog.InfoContext(ctx, "processing PR",
			"pr", prNumber,
			"repo", repoFullName,
			"head_sha", headSHA,
		)

		sseHub.Publish(sessionID, sseEvent("review.started", map[string]any{"pr": prNumber, "status": "running"}))

		// reviewID 提前声明，供 defer 中的 panic 恢复统一更新状态。
		reviewID := existingReviewID
		var resultErr error
		defer func() {
			if r := recover(); r != nil {
				resultErr = fmt.Errorf("panic: %v", r)
				slog.Error("processReview panic", "pr", prNumber, "panic", r, "stack", string(debug.Stack()))
				m.RecordFailure()
			}
			if resultErr != nil {
				if reviewID != 0 {
					db.UpdateReview(reviewID, "failed", 0, "", "", resultErr.Error())
				}
				sseHub.Publish(sessionID, sseEvent("review.failed", map[string]any{
					"pr": prNumber, "status": "failed", "error": resultErr.Error(),
				}))
			}
		}()

		// 审计
		db.AuditLog("review.started", prNumber, repoFullName, "")

		// 1. 创建审查记录（手动触发已预创建则复用）
		if reviewID == 0 {
			var err error
			reviewID, err = db.InsertReview(prNumber, repoFullName, headSHA)
			if err != nil {
				m.RecordFailure()
				resultErr = fmt.Errorf("insert review record: %w", err)
				return resultErr
			}
		}
		db.AuditLog("review.started", prNumber, repoFullName, fmt.Sprintf("review_id=%d", reviewID))

		// 2. 获取 PR diff
		{
			_, fetchSpan := otel.StartSpan(ctx, "github.fetch_diff")
			slog.Info("fetching PR diff", "pr", prNumber)
			rawDiff, err := ghClient.PRDiff(ctx, owner, repoName, prNumber)
			fetchSpan.End()
			if err != nil {
				db.AuditLog("review.failed", prNumber, repoFullName, err.Error())
				m.RecordFailure()
				resultErr = fmt.Errorf("fetch PR diff: %w", err)
				return resultErr
			}

			// 3. 解析 diff → 按文件分块
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
				db.UpdateReview(reviewID, "success", 0, "No reviewable files", "", "")
				sseHub.Publish(sessionID, sseEvent("review.completed", map[string]any{
					"pr": prNumber, "status": "success", "issues": 0, "duration_ms": 0,
				}))
				return nil
			}

			// PR 规模判断 → 智能选择 Agent 模式
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

			sseHub.Publish(sessionID, sseEvent("review.progress", map[string]any{
				"pr":     prNumber,
				"status": "analyzing",
				"files":  len(files),
				"chunks": len(chunks),
				"mode":   map[bool]string{true: "plan_execute", false: "react"}[usePlanExecute],
			}))

			// 4. 构造 prompt
			prTitle := fmt.Sprintf("%s#%d", repoFullName, prNumber)
			var reviewPrompt string
			if usePlanExecute {
				reviewPrompt = prompt.BuildPlanExecutePrompt(prTitle, files)
			} else {
				reviewPrompt = prompt.BuildReviewPrompt(prTitle, files)
			}

			const maxPromptBytes = 32_000
			reviewPrompt = prompt.TruncatePrompt(reviewPrompt, maxPromptBytes)

			// 5. 调用 agent-go 认知面
			_, cognitionSpan := otel.StartSpan(ctx, "cognition.run_review")
			slog.InfoContext(ctx, "calling agent-go cognition",
				"pr", prNumber,
				"prompt_bytes", len(reviewPrompt),
				"use_plan_execute", usePlanExecute,
			)
			var result *cognition.ReviewResult
			if usePlanExecute {
				result, err = cogClient.RunPlanExecuteReview(ctx,
					fmt.Sprintf("pr-review-%s-%d", repoName, prNumber),
					reviewPrompt, 8)
			} else {
				result, err = cogClient.RunReview(ctx, cognition.ReviewRequest{
					SessionID: fmt.Sprintf("pr-review-%s-%d", repoName, prNumber),
					Query:     reviewPrompt,
					AgentType: "react",
					MaxSteps:  5,
				})
			}
			cognitionSpan.End()
			if err != nil {
				db.AuditLog("review.failed", prNumber, repoFullName, err.Error())
				m.RecordCognitionError()
				m.RecordFailure()
				resultErr = fmt.Errorf("run review: %w", err)
				return resultErr
			}

			slog.InfoContext(ctx, "review completed",
				"pr", prNumber,
				"duration", result.Duration,
				"output_chars", len(result.Text),
			)

			// 6. 解析结果 + 投递到 GitHub（如果有 headSHA）
			_, postSpan := otel.StartSpan(ctx, "github.post_review")
			postResult, err := review.ParseAndPost(ctx, ghClient, owner, repoName, prNumber, headSHA, result.Text)
			if err != nil {
				db.AuditLog("review.failed", prNumber, repoFullName, err.Error())
				m.RecordFailure()
				postSpan.End()
				resultErr = fmt.Errorf("post review: %w", err)
				return resultErr
			}
			postSpan.End()

			// 7. 更新审查记录（复用 ParseAndPost 返回的结构化结果）
			issues := postResult.Issues
			db.UpdateReview(reviewID, "success", issues, postResult.Summary, result.Duration.String(), "")
			db.AuditLog("review.completed", prNumber, repoFullName,
				fmt.Sprintf("issues=%d duration=%s", issues, result.Duration))

			m.RecordSuccess(result.Duration.Milliseconds(), len(result.Text), issues)

			sseHub.Publish(sessionID, sseEvent("review.completed", map[string]any{
				"pr": prNumber, "status": "success", "issues": issues, "duration_ms": result.Duration.Milliseconds(),
			}))

			slog.InfoContext(ctx, "review posted to GitHub", "pr", prNumber, "issues", issues)
		}
		slog.InfoContext(ctx, "review total duration", "ms", time.Since(reviewStart).Milliseconds())
		return nil
	}

	// webhook 回调：从 webhook event 提取参数后调用 processReview
	onPR := func(event *webhook.PullRequestEvent) error {
		parts := strings.SplitN(event.RepoFullName, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid repo: %s", event.RepoFullName)
		}
		db.AuditLog("webhook.received", event.PRNumber, event.RepoFullName, event.Action)
		return processReview(context.Background(), parts[0], parts[1], event.PRNumber, event.HeadSHA, 0)
	}

	// HTTP server
	wh := webhook.New(webhookSecret, onPR)
	mux := http.NewServeMux()
	mux.Handle("/webhook", wh)
	mux.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	}))
	mux.Handle("/metrics", m) // Prometheus 指标端点

	// SSE 端点：实时审查进度流
	mux.Handle("/api/reviews/stream", sseHub)

	// 审查 API（带 CORS 响应头）
	// GET  /api/reviews        → 列表
	// POST /api/reviews        → 手动触发审查 {owner, repo, pr_number}
	mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPost {
			// 手动触发审查：校验 API token（若已配置）
			if !authorizeManualTrigger(r) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			var req struct {
				Owner    string `json:"owner"`
				Repo     string `json:"repo"`
				PRNumber int    `json:"pr_number"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, "invalid JSON body", http.StatusBadRequest)
				return
			}
			if req.Owner == "" || req.Repo == "" || req.PRNumber <= 0 {
				http.Error(w, "owner, repo, pr_number are required", http.StatusBadRequest)
				return
			}
			slog.Info("manual review trigger",
				"owner", req.Owner,
				"repo", req.Repo,
				"pr", req.PRNumber,
			)
			// 异步处理，立即返回 review ID
			db.AuditLog("manual.trigger", req.PRNumber, req.Owner+"/"+req.Repo, "")
			// 预创建记录并复用其 ID，避免 processReview 内部重复创建。
			reviewID, err := db.InsertReview(req.PRNumber, req.Owner+"/"+req.Repo, "")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			go processReview(context.Background(), req.Owner, req.Repo, req.PRNumber, "", reviewID)
			writeJSON(w, map[string]interface{}{
				"review_id": reviewID,
				"status":    "started",
			})
			return
		}
		// GET: 列表
		reviews, err := db.ListReviews(50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, reviews)
	})

	mux.HandleFunc("/api/reviews/", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w, r)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		id := parseID(r.URL.Path, "/api/reviews/")
		review, err := db.GetReview(int64(id))
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, review)
	})

	// 包裹 OTel trace middleware
	handler := middleware.Tracing(mux)

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: handler,
	}

	// 优雅关闭
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		slog.Info("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	slog.Info("code-review-agent starting",
		"listen", listenAddr,
		"cognition", cognitionAddr,
		"metrics", fmt.Sprintf("%s/metrics", listenAddr),
	)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}

// envDefault 返回环境变量或默认值。
func envDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
