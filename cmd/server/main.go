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
//	X-Trace-ID header + OpenTelemetry trace 贯穿全链路
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/LingMi1/code-review-agent/internal/cognition"
	"github.com/LingMi1/code-review-agent/internal/github"
	"github.com/LingMi1/code-review-agent/internal/metrics"
	"github.com/LingMi1/code-review-agent/internal/middleware"
	"github.com/LingMi1/code-review-agent/internal/otel"
	"github.com/LingMi1/code-review-agent/internal/reviewer"
	"github.com/LingMi1/code-review-agent/internal/sse"
	"github.com/LingMi1/code-review-agent/internal/store"
	"github.com/LingMi1/code-review-agent/internal/webhook"
)

func main() {
	// 结构化日志 + 自动 trace_id 注入
	slog.SetDefault(slog.New(otel.NewSlogHandler()))

	// OpenTelemetry 追踪
	if os.Getenv("OTEL_SERVICE_NAME") == "" {
		os.Setenv("OTEL_SERVICE_NAME", "code-review-agent")
	}
	shutdown, err := otel.Init(context.Background())
	if err != nil {
		slog.Error("failed to init OpenTelemetry", "error", err)
		os.Exit(1)
	}
	defer shutdown(context.Background())

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

	// 审查服务：编排 diff 拉取 → 认知面调用 → 结果投递。
	reviewSvc := reviewer.New(db, cogClient, ghClient, sseHub, m, 5)

	// webhook 回调：从 webhook event 提取参数后调用审查服务
	onPR := func(event *webhook.PullRequestEvent) error {
		parts := strings.SplitN(event.RepoFullName, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid repo: %s", event.RepoFullName)
		}
		db.AuditLog("webhook.received", event.PRNumber, event.RepoFullName, event.Action)
		return reviewSvc.Review(context.Background(), parts[0], parts[1], event.PRNumber, event.HeadSHA, 0)
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
			db.AuditLog("manual.trigger", req.PRNumber, req.Owner+"/"+req.Repo, "")
			// 预创建记录并复用其 ID，避免审查服务内部重复创建。
			reviewID, err := db.InsertReview(req.PRNumber, req.Owner+"/"+req.Repo, "")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			go reviewSvc.Review(context.Background(), req.Owner, req.Repo, req.PRNumber, "", reviewID)
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
