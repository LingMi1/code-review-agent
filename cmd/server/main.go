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
	"github.com/LingMi1/code-review-agent/internal/prompt"
	"github.com/LingMi1/code-review-agent/internal/review"
	"github.com/LingMi1/code-review-agent/internal/store"
	"github.com/LingMi1/code-review-agent/internal/webhook"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

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

	// PR 处理回调
	onPR := func(event *webhook.PullRequestEvent) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		slog.Info("processing PR",
			"pr", event.PRNumber,
			"repo", event.RepoFullName,
			"action", event.Action,
			"head_sha", event.HeadSHA,
		)

		// 审计：webhook 已接收
		db.AuditLog("webhook.received", event.PRNumber, event.RepoFullName, event.Action)

		// 1. 创建审查记录
		reviewID, err := db.InsertReview(event.PRNumber, event.RepoFullName, event.HeadSHA)
		if err != nil {
			return fmt.Errorf("insert review record: %w", err)
		}
		db.AuditLog("review.started", event.PRNumber, event.RepoFullName, fmt.Sprintf("review_id=%d", reviewID))

		// 2. 解析 owner/repo
		parts := strings.SplitN(event.RepoFullName, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid repo: %s", event.RepoFullName)
		}
		owner, repo := parts[0], parts[1]

		// 3. 获取 PR diff
		slog.Info("fetching PR diff", "pr", event.PRNumber)
		rawDiff, err := ghClient.PRDiff(ctx, owner, repo, event.PRNumber)
		if err != nil {
			db.UpdateReview(reviewID, "failed", 0, "", "", err.Error())
			db.AuditLog("review.failed", event.PRNumber, event.RepoFullName, err.Error())
			return fmt.Errorf("fetch PR diff: %w", err)
		}

		// 4. 解析 diff → 按文件分块
		files := diff.Parse(rawDiff)

		// 5. 过滤生成文件
		files = diff.SkipGeneratedFiles(files)

		// 6. Token 预算控制：大 PR 按文件分块
		const maxChunkLines = 800
		chunks := diff.ChunkBySize(files, maxChunkLines)

		if len(chunks) == 0 {
			slog.Info("no files to review (all generated/skipped)", "pr", event.PRNumber)
			db.UpdateReview(reviewID, "success", 0, "No reviewable files", "", "")
			return nil
		}

		slog.Info("diff parsed",
			"pr", event.PRNumber,
			"files", len(files),
			"chunks", len(chunks),
		)

		// 7. 构造 prompt
		prTitle := fmt.Sprintf("%s#%d", event.RepoFullName, event.PRNumber)
		reviewPrompt := prompt.BuildReviewPrompt(prTitle, files)

		// Token 预算：截断过长 prompt
		const maxPromptBytes = 32_000 // ~8K tokens
		reviewPrompt = prompt.TruncatePrompt(reviewPrompt, maxPromptBytes)

		// 8. 调用 agent-go 认知面
		slog.Info("calling agent-go cognition", "pr", event.PRNumber, "prompt_bytes", len(reviewPrompt))
		result, err := cogClient.RunReview(ctx, cognition.ReviewRequest{
			SessionID: fmt.Sprintf("pr-review-%s-%d", repo, event.PRNumber),
			Query:     reviewPrompt,
			AgentType: "react",
			MaxSteps:  5,
		})
		if err != nil {
			db.UpdateReview(reviewID, "failed", 0, "", "", err.Error())
			db.AuditLog("review.failed", event.PRNumber, event.RepoFullName, err.Error())
			// 降级：发一条 comment 说明 Agent 暂时不可用
			ghClient.PostIssueComment(ctx, owner, repo, event.PRNumber,
				"## AI Code Review\n\n**Agent temporarily unavailable.**\n\n> "+err.Error()+
					"\n\n---\n*Powered by [agent-go](https://github.com/yourname/agent-go)*")
			return fmt.Errorf("run review: %w", err)
		}

		slog.Info("review completed",
			"pr", event.PRNumber,
			"duration", result.Duration,
			"output_chars", len(result.Text),
		)

		// 9. 解析结果 + 投递到 GitHub
		if err := review.ParseAndPost(ctx, ghClient, owner, repo, event.PRNumber, event.HeadSHA, result.Text); err != nil {
			db.UpdateReview(reviewID, "failed", 0, "", result.Duration.String(), err.Error())
			db.AuditLog("review.failed", event.PRNumber, event.RepoFullName, err.Error())
			return fmt.Errorf("post review: %w", err)
		}

		// 10. 更新审查记录
		issues := parseIssueCount(result.Text)
		db.UpdateReview(reviewID, "success", issues, parseSummary(result.Text), result.Duration.String(), "")
		db.AuditLog("review.completed", event.PRNumber, event.RepoFullName,
			fmt.Sprintf("issues=%d duration=%s", issues, result.Duration))

		slog.Info("review posted to GitHub", "pr", event.PRNumber, "issues", issues)
		return nil
	}

	// HTTP server
	wh := webhook.New(webhookSecret, onPR)
	mux := http.NewServeMux()
	mux.Handle("/webhook", wh)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// 审查历史 API
	mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
		reviews, err := db.ListReviews(50)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		writeJSON(w, reviews)
	})

	mux.HandleFunc("/api/reviews/", func(w http.ResponseWriter, r *http.Request) {
		id := parseID(r.URL.Path, "/api/reviews/")
		review, err := db.GetReview(int64(id))
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		writeJSON(w, review)
	})

	srv := &http.Server{
		Addr:    listenAddr,
		Handler: mux,
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

	slog.Info("code-review-agent starting", "listen", listenAddr, "cognition", cognitionAddr)
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
