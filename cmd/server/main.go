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
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
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

	// PR 处理回调
	onPR := func(event *webhook.PullRequestEvent) error {
		// SSE session ID
		sessionID := fmt.Sprintf("pr-review-%s/%d", event.RepoFullName, event.PRNumber)

		// 创建根 span：整个 PR 审查流程
		ctx, span := otel.StartSpan(context.Background(), "review.pr")
		defer span.End()

		reviewStart := time.Now()

		slog.InfoContext(ctx, "processing PR",
			"pr", event.PRNumber,
			"repo", event.RepoFullName,
			"action", event.Action,
			"head_sha", event.HeadSHA,
		)

		sseHub.Publish(sessionID, sse.Event{Type: "review.started", Data: fmt.Sprintf(`{"pr":%d,"status":"running"}`, event.PRNumber)})

		// 统一处理失败事件
		var resultErr error
		defer func() {
			if resultErr != nil {
				sseHub.Publish(sessionID, sse.Event{Type: "review.failed", Data: fmt.Sprintf(
					`{"pr":%d,"status":"failed","error":"%s"}`, event.PRNumber, resultErr.Error())})
			}
		}()

		// 审计：webhook 已接收
		db.AuditLog("webhook.received", event.PRNumber, event.RepoFullName, event.Action)

		// 1. 创建审查记录
		reviewID, err := db.InsertReview(event.PRNumber, event.RepoFullName, event.HeadSHA)
		if err != nil {
			m.RecordFailure()
			resultErr = fmt.Errorf("insert review record: %w", err)
			return resultErr
		}
		db.AuditLog("review.started", event.PRNumber, event.RepoFullName, fmt.Sprintf("review_id=%d", reviewID))

		// 2. 解析 owner/repo
		parts := strings.SplitN(event.RepoFullName, "/", 2)
		if len(parts) != 2 {
			m.RecordFailure()
			resultErr = fmt.Errorf("invalid repo: %s", event.RepoFullName)
			return resultErr
		}
		owner, repoName := parts[0], parts[1]

		// 3. 获取 PR diff
		{
			_, fetchSpan := otel.StartSpan(ctx, "github.fetch_diff")
			slog.Info("fetching PR diff", "pr", event.PRNumber)
			rawDiff, err := ghClient.PRDiff(ctx, owner, repoName, event.PRNumber)
			fetchSpan.End()
			if err != nil {
				db.UpdateReview(reviewID, "failed", 0, "", "", err.Error())
				db.AuditLog("review.failed", event.PRNumber, event.RepoFullName, err.Error())
				m.RecordFailure()
				resultErr = fmt.Errorf("fetch PR diff: %w", err)
				return resultErr
			}

			// 4. 解析 diff → 按文件分块
			_, parseSpan := otel.StartSpan(ctx, "diff.parse")
			// 5. 过滤生成文件 + Token 预算优化
			allFiles := diff.Parse(rawDiff)        // 保留全量用于计算 PR size
			files := diff.SkipGeneratedFiles(allFiles)
			files = diff.SkipLockFiles(files)
			files = diff.SkipDataFiles(files)
			const maxChunkLines = 800
			chunks := diff.ChunkBySize(files, maxChunkLines)
			parseSpan.End()

			if len(chunks) == 0 {
				slog.InfoContext(ctx, "no files to review (all generated/skipped)", "pr", event.PRNumber)
				db.UpdateReview(reviewID, "success", 0, "No reviewable files", "", "")
				return nil
			}

			// PR 规模判断 → 智能选择 Agent 模式
			prSize := diff.CalcPRSize(allFiles, files)
			usePlanExecute := diff.ShouldUsePlanExecute(prSize)

			slog.InfoContext(ctx, "diff parsed",
				"pr", event.PRNumber,
				"files", len(files),
				"total_files", len(allFiles),
				"chunks", len(chunks),
				"lines", prSize.Lines,
				"mode", map[bool]string{true: "plan_execute", false: "react"}[usePlanExecute],
			)

			sseHub.Publish(sessionID, sse.Event{Type: "review.progress", Data: fmt.Sprintf(
				`{"pr":%d,"status":"analyzing","files":%d,"chunks":%d,"mode":"%s"}`,
				event.PRNumber, len(files), len(chunks),
				map[bool]string{true: "plan_execute", false: "react"}[usePlanExecute],
			)})

			// 6. 构造 prompt（根据模式选择）
			prTitle := fmt.Sprintf("%s#%d", event.RepoFullName, event.PRNumber)
			var reviewPrompt string
			if usePlanExecute {
				reviewPrompt = prompt.BuildPlanExecutePrompt(prTitle, files)
			} else {
				reviewPrompt = prompt.BuildReviewPrompt(prTitle, files)
			}

			// Token 预算：截断过长 prompt（~8K tokens）
			const maxPromptBytes = 32_000
			reviewPrompt = prompt.TruncatePrompt(reviewPrompt, maxPromptBytes)

			// 7. 调用 agent-go 认知面（智能路由）
			_, cognitionSpan := otel.StartSpan(ctx, "cognition.run_review")
			slog.InfoContext(ctx, "calling agent-go cognition",
				"pr", event.PRNumber,
				"prompt_bytes", len(reviewPrompt),
				"use_plan_execute", usePlanExecute,
			)
			var result *cognition.ReviewResult
			if usePlanExecute {
				result, err = cogClient.RunPlanExecuteReview(ctx,
					fmt.Sprintf("pr-review-%s-%d", repoName, event.PRNumber),
					reviewPrompt, 8)
			} else {
				result, err = cogClient.RunReview(ctx, cognition.ReviewRequest{
					SessionID: fmt.Sprintf("pr-review-%s-%d", repoName, event.PRNumber),
					Query:     reviewPrompt,
					AgentType: "react",
					MaxSteps:  5,
				})
			}
			cognitionSpan.End()
			if err != nil {
				db.UpdateReview(reviewID, "failed", 0, "", "", err.Error())
				db.AuditLog("review.failed", event.PRNumber, event.RepoFullName, err.Error())
				m.RecordCognitionError()
				m.RecordFailure()
				// 降级：发一条 comment 说明 Agent 暂时不可用
				ghClient.PostIssueComment(ctx, owner, repoName, event.PRNumber,
					"## AI Code Review\n\n**Agent temporarily unavailable.**\n\n> "+err.Error()+
						"\n\n---\n*Powered by [agent-go](https://github.com/LingMi1/agent-go)*")
				resultErr = fmt.Errorf("run review: %w", err)
				return resultErr
			}

			slog.InfoContext(ctx, "review completed",
				"pr", event.PRNumber,
				"duration", result.Duration,
				"output_chars", len(result.Text),
			)

			// 8. 解析结果 + 投递到 GitHub
			_, postSpan := otel.StartSpan(ctx, "github.post_review")
			if err := review.ParseAndPost(ctx, ghClient, owner, repoName, event.PRNumber, event.HeadSHA, result.Text); err != nil {
				db.UpdateReview(reviewID, "failed", 0, "", result.Duration.String(), err.Error())
				db.AuditLog("review.failed", event.PRNumber, event.RepoFullName, err.Error())
				m.RecordFailure()
				postSpan.End()
				resultErr = fmt.Errorf("post review: %w", err)
				return resultErr
			}
			postSpan.End()

			// 9. 更新审查记录
			issues := parseIssueCount(result.Text)
			db.UpdateReview(reviewID, "success", issues, parseSummary(result.Text), result.Duration.String(), "")
			db.AuditLog("review.completed", event.PRNumber, event.RepoFullName,
				fmt.Sprintf("issues=%d duration=%s", issues, result.Duration))

			m.RecordSuccess(result.Duration.Milliseconds(), len(result.Text), issues)

			sseHub.Publish(sessionID, sse.Event{Type: "review.completed", Data: fmt.Sprintf(
				`{"pr":%d,"status":"success","issues":%d,"duration_ms":%d}`, event.PRNumber, issues, result.Duration.Milliseconds())})

			slog.InfoContext(ctx, "review posted to GitHub", "pr", event.PRNumber, "issues", issues)
		}
		slog.InfoContext(ctx, "review total duration", "ms", time.Since(reviewStart).Milliseconds())
		return nil
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

	// 审查历史 API（带 CORS 响应头）
	mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		reviews, err := db.ListReviews(50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, reviews)
	})

	mux.HandleFunc("/api/reviews/", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)
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
